package model

import "time"

type InviteRelationshipStatus string

const (
	InviteRelationshipStatusPending  InviteRelationshipStatus = "pending"
	InviteRelationshipStatusRewarded InviteRelationshipStatus = "rewarded"
)

type InviteRewardEventStatus string

const (
	InviteRewardEventStatusGranted  InviteRewardEventStatus = "granted"
	InviteRewardEventStatusReversed InviteRewardEventStatus = "reversed"
)

type InviteRewardBeneficiaryRole string

const (
	InviteRewardBeneficiaryInviter InviteRewardBeneficiaryRole = "inviter"
	InviteRewardBeneficiaryInvitee InviteRewardBeneficiaryRole = "invitee"
)

type InviteRelationship struct {
	ID                     string                   `json:"id"`
	InviterUserID          string                   `json:"inviterUserId"`
	InviterCode            string                   `json:"inviterCode"`
	InviteeUserID          string                   `json:"inviteeUserId"`
	Status                 InviteRelationshipStatus `json:"status"`
	FirstPaidOrderID       string                   `json:"firstPaidOrderId"`
	FirstPaidOrderNo       string                   `json:"firstPaidOrderNo"`
	FirstPaidOrderKind     string                   `json:"firstPaidOrderKind"`
	FirstPaidAmountCNYCent int64                    `json:"firstPaidAmountCnyCent"`
	FirstPaidAt            *time.Time               `json:"firstPaidAt"`
	RewardedAt             *time.Time               `json:"rewardedAt"`
	LastReversedAt         *time.Time               `json:"lastReversedAt"`
	CreatedAt              time.Time                `json:"createdAt"`
	UpdatedAt              time.Time                `json:"updatedAt"`
}

type InviteRewardEvent struct {
	ID                     string                      `json:"id"`
	RelationID             string                      `json:"relationId"`
	BeneficiaryUserID      string                      `json:"beneficiaryUserId"`
	BeneficiaryRole        InviteRewardBeneficiaryRole `json:"beneficiaryRole"`
	OrderID                string                      `json:"orderId"`
	OrderNo                string                      `json:"orderNo"`
	Status                 InviteRewardEventStatus     `json:"status"`
	AmountMicros           int64                       `json:"amountMicros"`
	OrderPaidAmountCNYCent int64                       `json:"orderPaidAmountCnyCent"`
	CreatedAt              time.Time                   `json:"createdAt"`
}

type InviteConfigResponse struct {
	Enabled             bool  `json:"enabled"`
	ConfigComplete      bool  `json:"configComplete"`
	InviterRewardMicros int64 `json:"inviterRewardMicros"`
	InviteeRewardMicros int64 `json:"inviteeRewardMicros"`
	MinFirstPaidCNYCent int64 `json:"minFirstPaidCnyCent"`
}

type InviteConfigRequest struct {
	Enabled             bool  `json:"enabled"`
	InviterRewardMicros int64 `json:"inviterRewardMicros"`
	InviteeRewardMicros int64 `json:"inviteeRewardMicros"`
	MinFirstPaidCNYCent int64 `json:"minFirstPaidCnyCent"`
}

type InviteSummaryResponse struct {
	Enabled           bool   `json:"enabled"`
	ConfigComplete    bool   `json:"configComplete"`
	InviteCode        string `json:"inviteCode"`
	InvitedUsers      int64  `json:"invitedUsers"`
	PendingInvites    int64  `json:"pendingInvites"`
	RewardedInvites   int64  `json:"rewardedInvites"`
	TotalRewardMicros int64  `json:"totalRewardMicros"`
	InvitedByUserID   string `json:"invitedByUserId"`
	InvitedByUsername string `json:"invitedByUsername"`
	BoundInviteCode   string `json:"boundInviteCode"`
}

type InviteRewardEventResponse struct {
	ID                     string                      `json:"id"`
	RelationID             string                      `json:"relationId"`
	BeneficiaryUserID      string                      `json:"beneficiaryUserId"`
	BeneficiaryUsername    string                      `json:"beneficiaryUsername"`
	BeneficiaryRole        InviteRewardBeneficiaryRole `json:"beneficiaryRole"`
	OrderNo                string                      `json:"orderNo"`
	Status                 InviteRewardEventStatus     `json:"status"`
	AmountMicros           int64                       `json:"amountMicros"`
	OrderPaidAmountCNYCent int64                       `json:"orderPaidAmountCnyCent"`
	CreatedAt              time.Time                   `json:"createdAt"`
}

type InviteRelationshipAdminResponse struct {
	ID                     string                   `json:"id"`
	InviterUserID          string                   `json:"inviterUserId"`
	InviterUsername        string                   `json:"inviterUsername"`
	InviterCode            string                   `json:"inviterCode"`
	InviteeUserID          string                   `json:"inviteeUserId"`
	InviteeUsername        string                   `json:"inviteeUsername"`
	Status                 InviteRelationshipStatus `json:"status"`
	FirstPaidOrderNo       string                   `json:"firstPaidOrderNo"`
	FirstPaidOrderKind     string                   `json:"firstPaidOrderKind"`
	FirstPaidAmountCNYCent int64                    `json:"firstPaidAmountCnyCent"`
	FirstPaidAt            *time.Time               `json:"firstPaidAt"`
	RewardedAt             *time.Time               `json:"rewardedAt"`
	LastReversedAt         *time.Time               `json:"lastReversedAt"`
	CreatedAt              time.Time                `json:"createdAt"`
}

type InviteStatsResponse struct {
	Enabled              bool  `json:"enabled"`
	ConfigComplete       bool  `json:"configComplete"`
	RelationsTotal       int64 `json:"relationsTotal"`
	PendingTotal         int64 `json:"pendingTotal"`
	RewardedTotal        int64 `json:"rewardedTotal"`
	GrantedRewardMicros  int64 `json:"grantedRewardMicros"`
	ReversedRewardMicros int64 `json:"reversedRewardMicros"`
}
