package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/crypto"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

const (
	adminManagementKeyGraceWindow      = 5 * time.Minute
	adminManagementKeyLastUsedInterval = time.Minute
)

var (
	ErrManagementAPIKeyNotFound         = errors.New("管理 API Key 不存在")
	ErrManagementAPIKeyAlreadyExists    = errors.New("管理 API Key 已存在，请直接轮换")
	ErrManagementAPIKeyPasswordRequired = errors.New("当前密码不能为空")
	ErrManagementAPIKeyPasswordInvalid  = errors.New("当前密码错误")
	ErrManagementAPIKeyInvalid          = errors.New("管理 API Key 无效")
	ErrManagementAPIKeyDisabled         = errors.New("管理 API Key 已停用")
)

type AdminManagementKeyService struct {
	repo     *repository.AdminManagementKeyRepository
	userRepo *repository.UserRepository
}

func NewAdminManagementKeyService() *AdminManagementKeyService {
	return &AdminManagementKeyService{
		repo:     repository.NewAdminManagementKeyRepository(),
		userRepo: repository.NewUserRepository(),
	}
}

func (s *AdminManagementKeyService) GetStatus(userID string) (*model.ManagementAPIKeyStatus, error) {
	item, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	status := &model.ManagementAPIKeyStatus{
		EncryptionReady: hasManagementKeyEncryptionKey(),
	}
	if item == nil || strings.TrimSpace(item.KeyHash) == "" {
		return status, nil
	}

	status.KeyExists = true
	status.Enabled = item.Enabled
	status.Prefix = item.KeyPrefix
	status.CreatedAt = cloneTimePointer(item.CreatedAt)
	status.RotatedAt = cloneOptionalTimePointer(item.RotatedAt)
	status.LastUsedAt = cloneOptionalTimePointer(item.LastUsedAt)
	status.LastAuthMethod = strings.TrimSpace(item.LastAuthMethod)
	if item.PreviousValidUntil != nil && item.PreviousValidUntil.After(time.Now().UTC()) {
		status.GraceUntil = cloneOptionalTimePointer(item.PreviousValidUntil)
	}
	return status, nil
}

func (s *AdminManagementKeyService) Create(userID string) (*model.ManagementAPIKeyRevealResponse, error) {
	item, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if item != nil && strings.TrimSpace(item.KeyHash) != "" {
		return nil, ErrManagementAPIKeyAlreadyExists
	}

	rawKey, hash, prefix, ciphertext, err := buildEncryptedManagementKey()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	next := &model.AdminManagementKey{
		UserID:                userID,
		KeyHash:               hash,
		KeyCiphertext:         ciphertext,
		KeyPrefix:             prefix,
		PreviousKeyHash:       "",
		PreviousKeyCiphertext: "",
		PreviousKeyPrefix:     "",
		PreviousValidUntil:    nil,
		Enabled:               true,
		CreatedAt:             now,
		LastUsedAt:            nil,
		LastAuthMethod:        "",
	}
	if item != nil {
		next.CreatedAt = item.CreatedAt
		if next.CreatedAt.IsZero() {
			next.CreatedAt = now
		}
	}

	if err := s.repo.Upsert(next); err != nil {
		return nil, err
	}

	return &model.ManagementAPIKeyRevealResponse{
		APIKey: rawKey,
		Prefix: prefix,
	}, nil
}

func (s *AdminManagementKeyService) Reveal(userID, currentPassword string) (*model.ManagementAPIKeyRevealResponse, error) {
	if err := s.verifyCurrentPassword(userID, currentPassword); err != nil {
		return nil, err
	}

	item, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if item == nil || strings.TrimSpace(item.KeyHash) == "" {
		return nil, ErrManagementAPIKeyNotFound
	}

	value, err := decryptManagementKey(item.KeyCiphertext)
	if err != nil {
		return nil, err
	}
	resp := &model.ManagementAPIKeyRevealResponse{
		APIKey: value,
		Prefix: item.KeyPrefix,
	}
	if item.PreviousValidUntil != nil && item.PreviousValidUntil.After(time.Now().UTC()) {
		resp.GraceUntil = cloneOptionalTimePointer(item.PreviousValidUntil)
	}
	return resp, nil
}

func (s *AdminManagementKeyService) Rotate(userID, currentPassword string) (*model.ManagementAPIKeyRevealResponse, error) {
	if err := s.verifyCurrentPassword(userID, currentPassword); err != nil {
		return nil, err
	}

	item, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if item == nil || strings.TrimSpace(item.KeyHash) == "" {
		return nil, ErrManagementAPIKeyNotFound
	}

	rawKey, hash, prefix, ciphertext, err := buildEncryptedManagementKey()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	item.PreviousKeyHash = item.KeyHash
	item.PreviousKeyCiphertext = item.KeyCiphertext
	item.PreviousKeyPrefix = item.KeyPrefix
	previousValidUntil := now.Add(adminManagementKeyGraceWindow)
	item.PreviousValidUntil = &previousValidUntil
	item.KeyHash = hash
	item.KeyCiphertext = ciphertext
	item.KeyPrefix = prefix
	item.RotatedAt = &now

	if err := s.repo.Upsert(item); err != nil {
		return nil, err
	}

	return &model.ManagementAPIKeyRevealResponse{
		APIKey:     rawKey,
		Prefix:     prefix,
		GraceUntil: &previousValidUntil,
	}, nil
}

func (s *AdminManagementKeyService) SetEnabled(userID string, enabled bool) (*model.ManagementAPIKeyStatus, error) {
	item, err := s.repo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if item == nil || strings.TrimSpace(item.KeyHash) == "" {
		return nil, ErrManagementAPIKeyNotFound
	}

	item.Enabled = enabled
	if err := s.repo.Upsert(item); err != nil {
		return nil, err
	}
	return s.GetStatus(userID)
}

func (s *AdminManagementKeyService) Validate(rawKey, authMethod string) (*model.AdminManagementKey, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrManagementAPIKeyInvalid
	}

	hash := hashManagementKey(rawKey)
	item, err := s.repo.GetByAnyHash(hash)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrManagementAPIKeyInvalid
	}
	if item.PreviousKeyHash == hash && (item.PreviousValidUntil == nil || !item.PreviousValidUntil.After(time.Now().UTC())) {
		return nil, ErrManagementAPIKeyInvalid
	}
	if !item.Enabled {
		return nil, ErrManagementAPIKeyDisabled
	}

	go s.repo.UpdateUsageThrottled(item.UserID, normalizeManagementKeyAuthMethod(authMethod), adminManagementKeyLastUsedInterval)
	return item, nil
}

func (s *AdminManagementKeyService) verifyCurrentPassword(userID, currentPassword string) error {
	if strings.TrimSpace(currentPassword) == "" {
		return ErrManagementAPIKeyPasswordRequired
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return repository.ErrUserNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrManagementAPIKeyPasswordInvalid
	}
	return nil
}

func buildEncryptedManagementKey() (string, string, string, string, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", "", "", err
	}

	rawKey := "amsk-" + hex.EncodeToString(keyBytes)
	hash := hashManagementKey(rawKey)
	prefix := rawKey
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	ciphertext, err := encryptManagementKey(rawKey)
	if err != nil {
		return "", "", "", "", err
	}
	return rawKey, hash, prefix, ciphertext, nil
}

func hashManagementKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

func encryptManagementKey(rawKey string) (string, error) {
	key := config.Get().GetEncryptionKey()
	if key == nil {
		return rawKey, nil
	}
	return crypto.Encrypt([]byte(rawKey), key)
}

func decryptManagementKey(ciphertext string) (string, error) {
	key := config.Get().GetEncryptionKey()
	if key == nil {
		return ciphertext, nil
	}
	value, err := crypto.Decrypt(ciphertext, key)
	if err != nil {
		// Compatible with plaintext values created while encryption was disabled.
		return ciphertext, nil
	}
	return string(value), nil
}

func hasManagementKeyEncryptionKey() bool {
	return config.Get().GetEncryptionKey() != nil
}

func normalizeManagementKeyAuthMethod(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case model.ManagementAPIKeyAuthMethodXAPIKey:
		return model.ManagementAPIKeyAuthMethodXAPIKey
	default:
		return model.ManagementAPIKeyAuthMethodBearer
	}
}

func cloneTimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}

func cloneOptionalTimePointer(value *time.Time) *time.Time {
	if value == nil || (*value).IsZero() {
		return nil
	}
	copy := *value
	return &copy
}
