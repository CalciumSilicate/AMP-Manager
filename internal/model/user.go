package model

import "time"

type User struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	PasswordHash  string    `json:"-"`
	IsAdmin       bool      `json:"is_admin"`
	BalanceMicros int64     `json:"balance_micros"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
	Message  string `json:"message"`
}

type UserInfo struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	IsAdmin       bool      `json:"isAdmin"`
	BalanceMicros int64     `json:"balanceMicros"`
	BalanceUsd    string    `json:"balanceUsd"`
	GroupIDs      []string  `json:"groupIds"`
	GroupNames    []string  `json:"groupNames"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
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
	AmountUsd float64 `json:"amountUsd" binding:"required,gt=0"`
}
