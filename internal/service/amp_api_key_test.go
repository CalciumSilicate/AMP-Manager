package service

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

func TestCreateAPIKeyRejectsInvalidCustomKey(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-invalid")
	svc := NewAmpService()

	_, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name:      "invalid",
		CustomKey: "bad-key",
	})
	if !errors.Is(err, ErrInvalidAPIKeyFormat) {
		t.Fatalf("expected ErrInvalidAPIKeyFormat, got %v", err)
	}
}

func TestCreateAPIKeyRejectsDuplicateCustomKey(t *testing.T) {
	setupAmpServiceTestDB(t)

	firstUser := createAmpServiceTestUser(t, "user-first")
	secondUser := createAmpServiceTestUser(t, "user-second")
	svc := NewAmpService()

	req := &model.CreateAPIKeyRequest{
		Name:      "custom",
		CustomKey: "Key1234567890ABCD",
	}

	if _, err := svc.CreateAPIKey(firstUser.ID, req); err != nil {
		t.Fatalf("first CreateAPIKey returned error: %v", err)
	}

	_, err := svc.CreateAPIKey(secondUser.ID, req)
	if !errors.Is(err, ErrDuplicateAPIKey) {
		t.Fatalf("expected ErrDuplicateAPIKey, got %v", err)
	}
}

func TestUpdateAPIKeyUpdatesNameAndExpiry(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-update-key")
	svc := NewAmpService()

	created, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name: "before",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	updated, err := svc.UpdateAPIKey(user.ID, created.ID, &model.UpdateAPIKeyRequest{
		Name:      "after",
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("UpdateAPIKey returned error: %v", err)
	}

	if updated.Name != "after" {
		t.Fatalf("expected updated name %q, got %q", "after", updated.Name)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiresAt %v, got %v", expiresAt, updated.ExpiresAt)
	}
}

func TestUpdateAPIKeyUpdatesRawKeyAndPrefix(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-update-raw-key")
	svc := NewAmpService()

	created, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name: "before",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	updated, err := svc.UpdateAPIKey(user.ID, created.ID, &model.UpdateAPIKeyRequest{
		Name:   "after",
		APIKey: "sk-ABCDEFGH12345678",
	})
	if err != nil {
		t.Fatalf("UpdateAPIKey returned error: %v", err)
	}

	if updated.Prefix != "sk-ABCDE" {
		t.Fatalf("expected prefix %q, got %q", "sk-ABCDE", updated.Prefix)
	}

	revealed, err := svc.GetAPIKey(user.ID, created.ID)
	if err != nil {
		t.Fatalf("GetAPIKey returned error: %v", err)
	}
	if revealed.APIKey != "sk-ABCDEFGH12345678" {
		t.Fatalf("expected updated api key, got %q", revealed.APIKey)
	}
}

func TestUpdateAPIKeyRejectsDuplicateRawKey(t *testing.T) {
	setupAmpServiceTestDB(t)

	firstUser := createAmpServiceTestUser(t, "user-dup-a")
	secondUser := createAmpServiceTestUser(t, "user-dup-b")
	svc := NewAmpService()

	first, err := svc.CreateAPIKey(firstUser.ID, &model.CreateAPIKeyRequest{
		Name:      "first",
		CustomKey: "sk-ABCDEFGH12345678",
	})
	if err != nil {
		t.Fatalf("first CreateAPIKey returned error: %v", err)
	}

	second, err := svc.CreateAPIKey(secondUser.ID, &model.CreateAPIKeyRequest{
		Name:      "second",
		CustomKey: "sk-12345678ABCDEFGH",
	})
	if err != nil {
		t.Fatalf("second CreateAPIKey returned error: %v", err)
	}

	_, err = svc.UpdateAPIKey(secondUser.ID, second.ID, &model.UpdateAPIKeyRequest{
		Name:   second.Name,
		APIKey: "sk-ABCDEFGH12345678",
	})
	if !errors.Is(err, ErrDuplicateAPIKey) {
		t.Fatalf("expected ErrDuplicateAPIKey, got %v", err)
	}

	revealed, err := svc.GetAPIKey(firstUser.ID, first.ID)
	if err != nil {
		t.Fatalf("GetAPIKey returned error: %v", err)
	}
	if revealed.APIKey != "sk-ABCDEFGH12345678" {
		t.Fatalf("expected original api key to remain unchanged, got %q", revealed.APIKey)
	}
}

func TestCreateAPIKeyPersistsCircuitBreakerConfig(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-breaker-config")
	svc := NewAmpService()
	threshold := 80
	openMinutes := 15
	halfOpenMinutes := 3

	created, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name:                          "breaker",
		CircuitBreakerThreshold:       &threshold,
		CircuitBreakerOpenMinutes:     &openMinutes,
		CircuitBreakerHalfOpenMinutes: &halfOpenMinutes,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	stored, err := repository.NewAPIKeyRepository().GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored api key")
	}
	if stored.CircuitBreakerThreshold != threshold || stored.CircuitBreakerOpenMinutes != openMinutes || stored.CircuitBreakerHalfOpenMinutes != halfOpenMinutes {
		t.Fatalf("unexpected circuit breaker config: %+v", stored)
	}
}

func TestValidateAPIKeyRejectsOpenCircuitBreaker(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-open-breaker")
	svc := NewAmpService()
	created, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name:      "breaker",
		CustomKey: "sk-OPENBREAKER12345",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	key, err := repository.NewAPIKeyRepository().GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if key == nil {
		t.Fatal("expected stored api key")
	}

	now := time.Now().UTC()
	key.CircuitBreakerState = model.APIKeyCircuitBreakerStateOpen
	key.CircuitBreakerOpenedAt = &now
	if err := repository.NewAPIKeyRepository().UpdateCircuitBreakerState(key); err != nil {
		t.Fatalf("UpdateCircuitBreakerState returned error: %v", err)
	}

	_, err = svc.ValidateAPIKey("sk-OPENBREAKER12345")
	if !errors.Is(err, ErrAPIKeyCircuitOpen) {
		t.Fatalf("expected ErrAPIKeyCircuitOpen, got %v", err)
	}
}

func TestSetAPIKeyDisabledCanRestore(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-disable-key")
	svc := NewAmpService()

	created, err := svc.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{
		Name: "toggle",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	disabled, err := svc.SetAPIKeyDisabled(user.ID, created.ID, true)
	if err != nil {
		t.Fatalf("SetAPIKeyDisabled(true) returned error: %v", err)
	}
	if disabled.Status != "disabled" || disabled.RevokedAt == nil {
		t.Fatalf("expected disabled status with revokedAt, got status=%q revokedAt=%v", disabled.Status, disabled.RevokedAt)
	}

	restored, err := svc.SetAPIKeyDisabled(user.ID, created.ID, false)
	if err != nil {
		t.Fatalf("SetAPIKeyDisabled(false) returned error: %v", err)
	}
	if restored.Status != "active" || restored.RevokedAt != nil {
		t.Fatalf("expected active status after restore, got status=%q revokedAt=%v", restored.Status, restored.RevokedAt)
	}
}

func setupAmpServiceTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "amp-service-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}

	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}

func createAmpServiceTestUser(t *testing.T, username string) *model.User {
	t.Helper()

	user := &model.User{
		Username:      username,
		PasswordHash:  "hash",
		BalanceMicros: 0,
	}

	if err := repository.NewUserRepository().Create(user); err != nil {
		t.Fatalf("create user returned error: %v", err)
	}

	return user
}
