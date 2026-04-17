package model

import (
	"time"

	"ampmanager/internal/precision"
)

type User struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	InviteCode         string    `json:"inviteCode"`
	IsAdmin            bool      `json:"is_admin"`
	BalanceMicros      int64     `json:"balance_micros"`
	ConcurrencyLimit   int       `json:"concurrency_limit"`
	MustChangePassword bool      `json:"mustChangePassword"`
	MustChangeUsername bool      `json:"mustChangeUsername"`
	LegacySource       string    `json:"legacySource"`
	LegacyRefID        string    `json:"legacyRefId"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type RegisterRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=32"`
	Password   string `json:"password" binding:"required,min=6,max=128"`
	InviteCode string `json:"inviteCode" binding:"omitempty,max=32"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	Token              string `json:"token,omitempty"`
	IsAdmin            bool   `json:"isAdmin"`
	MustChangePassword bool   `json:"mustChangePassword"`
	MustChangeUsername bool   `json:"mustChangeUsername"`
	Message            string `json:"message"`
}

type UserInfo struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	IsAdmin          bool      `json:"isAdmin"`
	BalanceMicros    int64     `json:"balanceMicros"`
	BalanceUsd       string    `json:"balanceUsd"`
	ConcurrencyLimit int       `json:"concurrencyLimit"`
	GroupIDs         []string  `json:"groupIds"`
	GroupNames       []string  `json:"groupNames"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type UserListPage struct {
	Items    []*UserInfo `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=128"`
}

type ChangeUsernameRequest struct {
	NewUsername string `json:"newUsername" binding:"required,min=3,max=32"`
}

type CompleteBootstrapCredentialsRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required,min=6,max=128"`
	NewPassword     string `json:"newPassword" binding:"required,min=6,max=128"`
	NewUsername     string `json:"newUsername" binding:"required,min=3,max=32"`
}

type CredentialBootstrapStateResponse struct {
	UserID             string `json:"userId"`
	Username           string `json:"username"`
	MustChangePassword bool   `json:"mustChangePassword"`
	MustChangeUsername bool   `json:"mustChangeUsername"`
}

type SetAdminRequest struct {
	IsAdmin bool `json:"isAdmin"`
}

type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required,min=6,max=128"`
}

type SetGroupsRequest struct {
	GroupIDs []string `json:"groupIds"`
}

type TopUpRequest struct {
	AmountUsd precision.DecimalString `json:"amountUsd"`
}

type UpdateUserConcurrencyLimitRequest struct {
	ConcurrencyLimit int `json:"concurrencyLimit"`
}
