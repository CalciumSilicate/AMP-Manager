package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/config"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUsernameExists     = errors.New("用户名已存在")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
)

type UserService struct {
	repo repository.UserRepositoryInterface
}

// NewUserServiceWithRepo 使用指定的仓库实现创建 UserService（用于依赖注入和测试）
func NewUserServiceWithRepo(repo repository.UserRepositoryInterface) *UserService {
	return &UserService{
		repo: repo,
	}
}

// NewUserService 创建使用默认仓库的 UserService（便利方法）
func NewUserService() *UserService {
	return NewUserServiceWithRepo(repository.NewUserRepository())
}

func (s *UserService) Register(req *model.RegisterRequest) (*model.User, error) {
	exists, err := s.repo.ExistsByUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUsernameExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		IsAdmin:      false,
	}

	if err := s.repo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) Login(req *model.LoginRequest) (*model.User, string, error) {
	user, err := s.repo.GetByUsername(req.Username)
	if err != nil {
		return nil, "", err
	}
	if user == nil {
		return nil, "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	jwtService := NewJWTService()
	token, err := jwtService.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

func (s *UserService) EnsureAdmin() error {
	cfg := config.Get()

	// 只在系统没有任何用户时才初始化管理员（首次部署）
	_, userCount, err := s.repo.GetTotalBalanceAndUserCount()
	if err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin := &model.User{
		Username:     cfg.AdminUsername,
		PasswordHash: string(hashedPassword),
		IsAdmin:      true,
	}

	if err := s.repo.Create(admin); err != nil {
		return err
	}

	log.Printf("管理员账户已初始化: %s（首次部署）", cfg.AdminUsername)
	return nil
}

func (s *UserService) ListUsers() ([]*model.UserInfo, error) {
	users, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	return s.buildUserInfos(users)
}

func (s *UserService) ListUsersPaged(page, pageSize int, keyword string) (*model.UserListPage, error) {
	users, total, err := s.repo.ListPaged(page, pageSize, keyword)
	if err != nil {
		return nil, err
	}

	items, err := s.buildUserInfos(users)
	if err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return &model.UserListPage{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *UserService) buildUserInfos(users []*model.User) ([]*model.UserInfo, error) {
	// 批量获取所有用户的 groupID 映射 (1 次查询替代 N 次)
	userGroupMap, err := s.repo.GetAllUserGroupIDs()
	if err != nil {
		return nil, err
	}

	// 收集所有去重的 groupID
	uniqueGroupIDs := make(map[string]struct{})
	for _, gids := range userGroupMap {
		for _, gid := range gids {
			uniqueGroupIDs[gid] = struct{}{}
		}
	}
	allGroupIDs := make([]string, 0, len(uniqueGroupIDs))
	for gid := range uniqueGroupIDs {
		allGroupIDs = append(allGroupIDs, gid)
	}

	// 批量获取所有 group 详情 (1 次查询替代 M 次)
	groupRepo := repository.NewGroupRepository()
	groupMap, err := groupRepo.GetByIDs(allGroupIDs)
	if err != nil {
		return nil, err
	}

	result := make([]*model.UserInfo, len(users))
	for i, u := range users {
		gids := userGroupMap[u.ID]
		groupNames := make([]string, 0, len(gids))
		for _, gid := range gids {
			if g, ok := groupMap[gid]; ok {
				groupNames = append(groupNames, g.Name)
			}
		}
		if gids == nil {
			gids = []string{}
		}
		result[i] = &model.UserInfo{
			ID:               u.ID,
			Username:         u.Username,
			IsAdmin:          u.IsAdmin,
			BalanceMicros:    u.BalanceMicros,
			BalanceUsd:       fmt.Sprintf("%.6f", float64(u.BalanceMicros)/1e6),
			ConcurrencyLimit: u.ConcurrencyLimit,
			GroupIDs:         gids,
			GroupNames:       groupNames,
			CreatedAt:        u.CreatedAt,
			UpdatedAt:        u.UpdatedAt,
		}
	}
	return result, nil
}

func (s *UserService) ChangePassword(userID string, oldPassword, newPassword string) error {
	user, err := s.repo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("用户不存在")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("旧密码错误")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.repo.UpdatePassword(userID, string(hashedPassword))
}

func (s *UserService) ChangeUsername(userID string, newUsername string) error {
	exists, err := s.repo.ExistsByUsername(newUsername)
	if err != nil {
		return err
	}
	if exists {
		return ErrUsernameExists
	}
	return s.repo.UpdateUsername(userID, newUsername)
}

func (s *UserService) SetAdmin(userID string, isAdmin bool) error {
	return s.repo.SetAdmin(userID, isAdmin)
}

func (s *UserService) SetConcurrencyLimit(userID string, concurrencyLimit int) error {
	if concurrencyLimit < 0 {
		concurrencyLimit = 0
	}
	return s.repo.SetConcurrencyLimit(userID, concurrencyLimit)
}

func (s *UserService) SetGroups(userID string, groupIDs []string) error {
	return s.repo.SetGroups(userID, groupIDs)
}

func (s *UserService) DeleteUser(userID string) error {
	return s.repo.Delete(userID)
}

func (s *UserService) ResetPassword(userID string, newPassword string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(userID, string(hashedPassword))
}

func (s *UserService) GetBalance(userID string) (int64, error) {
	return s.repo.GetBalance(userID)
}

func (s *UserService) TopUp(userID string, amountMicros int64) error {
	if err := s.repo.TopUpBalance(userID, amountMicros); err != nil {
		return err
	}
	if runtime := billingstate.Get(); runtime != nil {
		return runtime.ApplyBalanceDelta(context.Background(), userID, amountMicros)
	}
	return nil
}

func (s *UserService) GetTotalBalanceAndUserCount() (int64, int64, error) {
	return s.repo.GetTotalBalanceAndUserCount()
}

func (s *UserService) PreviewBatchUpdate(req *model.UserBatchPreviewRequest) (*model.UserBatchPreviewResponse, error) {
	users, err := s.resolveBatchUsers(req.TargetMode, req.SelectedUserIDs, req.Filters)
	if err != nil {
		return nil, err
	}

	items := make([]model.UserBatchPreviewItem, 0, minInt(len(users), 20))
	for i, user := range users {
		if i >= 20 {
			break
		}
		items = append(items, model.UserBatchPreviewItem{
			ID:         user.ID,
			Username:   user.Username,
			IsAdmin:    user.IsAdmin,
			BalanceUsd: fmt.Sprintf("%.6f", float64(user.BalanceMicros)/1e6),
		})
	}

	return &model.UserBatchPreviewResponse{
		Count: len(users),
		Items: items,
	}, nil
}

func (s *UserService) ApplyBatchUpdate(req *model.UserBatchApplyRequest) (*model.UserBatchApplyResponse, error) {
	if req.Changes.Balance == nil && req.Changes.Groups == nil && req.Changes.ConcurrencyLimit == nil && req.Changes.Subscription == nil {
		return nil, errors.New("至少选择一个批量修改项")
	}

	users, err := s.resolveBatchUsers(req.TargetMode, req.SelectedUserIDs, req.Filters)
	if err != nil {
		return nil, err
	}

	applied := 0
	for _, user := range users {
		if err := s.applyBatchChanges(user, &req.Changes); err != nil {
			return nil, err
		}
		applied++
	}

	return &model.UserBatchApplyResponse{
		MatchedCount: len(users),
		AppliedCount: applied,
	}, nil
}

func (s *UserService) resolveBatchUsers(targetMode model.UserBatchTargetMode, selectedUserIDs []string, filters *model.UserBatchFilter) ([]*model.User, error) {
	if err := validateBatchFilter(filters); err != nil {
		return nil, err
	}

	users, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	switch targetMode {
	case model.UserBatchTargetSelected:
		selectedSet := make(map[string]struct{}, len(selectedUserIDs))
		for _, id := range selectedUserIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				selectedSet[id] = struct{}{}
			}
		}
		if len(selectedSet) == 0 {
			return nil, errors.New("请选择至少一个用户")
		}

		result := make([]*model.User, 0, len(selectedSet))
		for _, user := range users {
			if _, ok := selectedSet[user.ID]; ok {
				result = append(result, user)
			}
		}
		return result, nil
	case model.UserBatchTargetFiltered:
		return s.filterBatchUsers(users, filters)
	default:
		return nil, errors.New("无效的批量目标模式")
	}
}

func (s *UserService) filterBatchUsers(users []*model.User, filters *model.UserBatchFilter) ([]*model.User, error) {
	if filters == nil {
		return users, nil
	}

	groupMatches := map[string]bool(nil)
	if len(filters.GroupIDs) > 0 {
		userGroupMap, err := s.repo.GetAllUserGroupIDs()
		if err != nil {
			return nil, err
		}
		groupMatches = make(map[string]bool, len(users))
		required := make(map[string]struct{}, len(filters.GroupIDs))
		for _, groupID := range filters.GroupIDs {
			groupID = strings.TrimSpace(groupID)
			if groupID != "" {
				required[groupID] = struct{}{}
			}
		}
		for userID, groupIDs := range userGroupMap {
			for _, groupID := range groupIDs {
				if _, ok := required[groupID]; ok {
					groupMatches[userID] = true
					break
				}
			}
		}
	}

	keyword := strings.ToLower(strings.TrimSpace(filters.Keyword))
	result := make([]*model.User, 0, len(users))
	for _, user := range users {
		subscriptionState, err := s.resolveBatchSubscriptionState(user.ID)
		if err != nil {
			return nil, err
		}
		if keyword != "" && !strings.Contains(strings.ToLower(user.Username), keyword) {
			continue
		}
		if filters.IsAdmin != nil && user.IsAdmin != *filters.IsAdmin {
			continue
		}
		if filters.BalanceMinMicros != nil && user.BalanceMicros < *filters.BalanceMinMicros {
			continue
		}
		if filters.BalanceMaxMicros != nil && user.BalanceMicros > *filters.BalanceMaxMicros {
			continue
		}
		if len(filters.GroupIDs) > 0 && !groupMatches[user.ID] {
			continue
		}
		if len(filters.SubscriptionStatuses) > 0 {
			if subscriptionState == nil || !containsSubscriptionStatus(filters.SubscriptionStatuses, subscriptionState.Status) {
				continue
			}
		}
		if len(filters.PlanIDs) > 0 {
			if subscriptionState == nil || !containsString(filters.PlanIDs, subscriptionState.PlanID) {
				continue
			}
		}
		if filters.SubscriptionExpiresAfter != nil {
			if subscriptionState == nil || subscriptionState.ExpiresAt == nil || subscriptionState.ExpiresAt.Before(*filters.SubscriptionExpiresAfter) {
				continue
			}
		}
		if filters.SubscriptionExpiresBefore != nil {
			if subscriptionState == nil || subscriptionState.ExpiresAt == nil || subscriptionState.ExpiresAt.After(*filters.SubscriptionExpiresBefore) {
				continue
			}
		}
		if filters.LimitType != nil {
			if subscriptionState == nil {
				continue
			}
			limit, ok := findSubscriptionLimit(subscriptionState.Limits, *filters.LimitType)
			if !ok {
				continue
			}
			if filters.LimitMinMicros != nil && limit.LimitMicros < *filters.LimitMinMicros {
				continue
			}
			if filters.LimitMaxMicros != nil && limit.LimitMicros > *filters.LimitMaxMicros {
				continue
			}
		}
		result = append(result, user)
	}
	return result, nil
}

func (s *UserService) applyBatchChanges(user *model.User, changes *model.UserBatchChangeSet) error {
	if changes.Balance != nil {
		if err := s.applyBalanceBatchChange(user.ID, changes.Balance); err != nil {
			return err
		}
	}

	if changes.Groups != nil {
		if err := s.applyGroupBatchChange(user.ID, changes.Groups); err != nil {
			return err
		}
	}

	if changes.ConcurrencyLimit != nil {
		if err := s.SetConcurrencyLimit(user.ID, changes.ConcurrencyLimit.ConcurrencyLimit); err != nil {
			return err
		}
	}

	if changes.Subscription != nil {
		if err := s.applySubscriptionBatchChange(user.ID, changes.Subscription); err != nil {
			return err
		}
	}

	return nil
}

func (s *UserService) applyBalanceBatchChange(userID string, change *model.BalanceBatchChange) error {
	if change == nil {
		return nil
	}
	if change.AmountMicros < 0 {
		return errors.New("余额批量修改金额不能为负数")
	}

	currentBalance, err := s.repo.GetBalance(userID)
	if err != nil {
		return err
	}

	nextBalance := currentBalance
	switch change.Mode {
	case model.BalanceBatchChangeAdd:
		nextBalance += change.AmountMicros
	case model.BalanceBatchChangeSet:
		nextBalance = change.AmountMicros
	case model.BalanceBatchChangeSubtract:
		nextBalance -= change.AmountMicros
		if nextBalance < 0 {
			nextBalance = 0
		}
	default:
		return errors.New("无效的余额批量修改模式")
	}

	if err := s.repo.SetBalance(userID, nextBalance); err != nil {
		return err
	}
	return s.applyBillingBalanceDelta(userID, nextBalance-currentBalance)
}

func (s *UserService) applyGroupBatchChange(userID string, change *model.GroupBatchChange) error {
	if change == nil {
		return nil
	}

	currentGroupIDs, err := s.repo.GetGroupIDs(userID)
	if err != nil {
		return err
	}

	groupSet := make(map[string]struct{}, len(currentGroupIDs))
	for _, groupID := range currentGroupIDs {
		if groupID != "" {
			groupSet[groupID] = struct{}{}
		}
	}

	switch change.Mode {
	case model.GroupBatchChangeAdd:
		for _, groupID := range change.GroupIDs {
			groupID = strings.TrimSpace(groupID)
			if groupID != "" {
				groupSet[groupID] = struct{}{}
			}
		}
	case model.GroupBatchChangeRemove:
		for _, groupID := range change.GroupIDs {
			delete(groupSet, strings.TrimSpace(groupID))
		}
	case model.GroupBatchChangeSet:
		groupSet = make(map[string]struct{}, len(change.GroupIDs))
		for _, groupID := range change.GroupIDs {
			groupID = strings.TrimSpace(groupID)
			if groupID != "" {
				groupSet[groupID] = struct{}{}
			}
		}
	default:
		return errors.New("无效的分组批量修改模式")
	}

	nextGroupIDs := make([]string, 0, len(groupSet))
	for groupID := range groupSet {
		nextGroupIDs = append(nextGroupIDs, groupID)
	}
	return s.repo.SetGroups(userID, nextGroupIDs)
}

func (s *UserService) applyBillingBalanceDelta(userID string, delta int64) error {
	if delta == 0 {
		return nil
	}
	if runtime := billingstate.Get(); runtime != nil {
		return runtime.ApplyBalanceDelta(context.Background(), userID, delta)
	}
	return nil
}

func validateBatchFilter(filters *model.UserBatchFilter) error {
	if filters == nil {
		return nil
	}
	if filters.LimitType == nil && (filters.LimitMinMicros != nil || filters.LimitMaxMicros != nil) {
		return errors.New("额度筛选需要先选择限制类型")
	}
	if filters.SubscriptionExpiresAfter != nil && filters.SubscriptionExpiresBefore != nil &&
		filters.SubscriptionExpiresAfter.After(*filters.SubscriptionExpiresBefore) {
		return errors.New("订阅到期时间筛选范围无效")
	}
	return nil
}

func (s *UserService) resolveBatchSubscriptionState(userID string) (*model.UserSubscriptionResponse, error) {
	subSvc := NewUserSubscriptionService()
	active, err := subSvc.GetActive(userID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return active, nil
	}

	subs, err := subSvc.ListByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}
	return subs[0], nil
}

func (s *UserService) applySubscriptionBatchChange(userID string, change *model.SubscriptionBatchChange) error {
	if change == nil {
		return nil
	}

	subSvc := NewUserSubscriptionService()
	current, err := subSvc.GetActive(userID)
	if err != nil {
		return err
	}

	switch change.PlanMode {
	case model.SubscriptionBatchPlanCancel:
		if current == nil {
			return nil
		}
		return subSvc.Cancel(userID)
	case model.SubscriptionBatchPlanAssign:
		if strings.TrimSpace(change.PlanID) == "" {
			return errors.New("批量订阅应用需要指定套餐")
		}
		expiresAt, err := resolveBatchExpiry(current, change)
		if err != nil {
			return err
		}
		_, err = subSvc.Assign(userID, &model.AssignSubscriptionRequest{
			PlanID:    change.PlanID,
			ExpiresAt: expiresAt,
		})
		return err
	case model.SubscriptionBatchPlanKeep:
		if change.ExpiryMode == model.SubscriptionBatchExpiryKeep {
			return nil
		}
		if current == nil {
			return errors.New("存在无活跃订阅用户，无法仅修改订阅时长")
		}
		expiresAt, err := resolveBatchExpiry(current, change)
		if err != nil {
			return err
		}
		if expiresAt == nil {
			return errors.New("未提供目标到期时间")
		}
		return subSvc.UpdateExpiry(userID, *expiresAt)
	default:
		return errors.New("无效的订阅批量修改模式")
	}
}

func resolveBatchExpiry(current *model.UserSubscriptionResponse, change *model.SubscriptionBatchChange) (*time.Time, error) {
	switch change.ExpiryMode {
	case model.SubscriptionBatchExpiryKeep:
		if current == nil {
			return nil, nil
		}
		return current.ExpiresAt, nil
	case model.SubscriptionBatchExpirySet:
		if change.ExpiresAt == nil {
			return nil, errors.New("设定订阅时长时需要提供目标时间")
		}
		expiresAt := change.ExpiresAt.UTC()
		return &expiresAt, nil
	case model.SubscriptionBatchExpiryExtendDays:
		if change.Days <= 0 {
			return nil, errors.New("续期天数必须大于 0")
		}
		base := time.Now().UTC()
		if current != nil && current.ExpiresAt != nil {
			base = current.ExpiresAt.UTC()
		}
		expiresAt := base.AddDate(0, 0, change.Days)
		return &expiresAt, nil
	case model.SubscriptionBatchExpiryShortenDays:
		if change.Days <= 0 {
			return nil, errors.New("缩短天数必须大于 0")
		}
		if current == nil || current.ExpiresAt == nil {
			return nil, errors.New("缩短订阅时长需要存在可到期的活跃订阅")
		}
		expiresAt := current.ExpiresAt.AddDate(0, 0, -change.Days)
		return &expiresAt, nil
	default:
		return nil, errors.New("无效的订阅时长修改模式")
	}
}

func containsSubscriptionStatus(items []model.SubscriptionStatus, target model.SubscriptionStatus) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func findSubscriptionLimit(items []model.SubscriptionPlanLimit, limitType model.LimitType) (*model.SubscriptionPlanLimit, bool) {
	for _, item := range items {
		if item.LimitType == limitType {
			limit := item
			return &limit, true
		}
	}
	return nil, false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
