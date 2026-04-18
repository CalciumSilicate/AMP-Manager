package model

import "time"

type CouponCodeMode string

const (
	CouponCodeModeShared    CouponCodeMode = "shared"
	CouponCodeModeSingleUse CouponCodeMode = "single_use"
)

type CouponDiscountType string

const (
	CouponDiscountTypeFixed      CouponDiscountType = "fixed_amount"
	CouponDiscountTypePercentage CouponDiscountType = "percentage"
)

type CouponCodeStatus string

const (
	CouponCodeStatusActive   CouponCodeStatus = "active"
	CouponCodeStatusDisabled CouponCodeStatus = "disabled"
	CouponCodeStatusConsumed CouponCodeStatus = "consumed"
)

type CouponUsageStatus string

const (
	CouponUsageStatusApplied  CouponUsageStatus = "applied"
	CouponUsageStatusReversed CouponUsageStatus = "reversed"
)

type CouponConfigResponse struct {
	Enabled bool `json:"enabled"`
}

type CouponConfigRequest struct {
	Enabled bool `json:"enabled"`
}

type CouponCampaign struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Description          string             `json:"description"`
	CodeMode             CouponCodeMode     `json:"codeMode"`
	SharedCode           string             `json:"sharedCode"`
	DiscountType         CouponDiscountType `json:"discountType"`
	FixedDiscountCNYCent int64              `json:"fixedDiscountCnyCent"`
	PercentOffBPS        int                `json:"percentOffBps"`
	MaxDiscountCNYCent   int64              `json:"maxDiscountCnyCent"`
	TotalUsageLimit      int                `json:"totalUsageLimit"`
	UsedCount            int                `json:"usedCount"`
	PerUserLimit         int                `json:"perUserLimit"`
	StartsAt             *time.Time         `json:"startsAt"`
	EndsAt               *time.Time         `json:"endsAt"`
	Enabled              bool               `json:"enabled"`
	CreatedAt            time.Time          `json:"createdAt"`
	UpdatedAt            time.Time          `json:"updatedAt"`
}

type CouponCampaignRequest struct {
	Name                 string             `json:"name" binding:"required,min=1,max=64"`
	Description          string             `json:"description" binding:"max=255"`
	CodeMode             CouponCodeMode     `json:"codeMode" binding:"required,oneof=shared single_use"`
	SharedCode           string             `json:"sharedCode" binding:"omitempty,max=32"`
	DiscountType         CouponDiscountType `json:"discountType" binding:"required,oneof=fixed_amount percentage"`
	FixedDiscountCNYCent int64              `json:"fixedDiscountCnyCent"`
	PercentOffBPS        int                `json:"percentOffBps"`
	MaxDiscountCNYCent   int64              `json:"maxDiscountCnyCent"`
	TotalUsageLimit      int                `json:"totalUsageLimit"`
	PerUserLimit         int                `json:"perUserLimit"`
	StartsAt             *time.Time         `json:"startsAt"`
	EndsAt               *time.Time         `json:"endsAt"`
	Enabled              bool               `json:"enabled"`
}

type CouponBatch struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaignId"`
	Name       string    `json:"name"`
	Prefix     string    `json:"prefix"`
	CodeCount  int       `json:"codeCount"`
	CodeLength int       `json:"codeLength"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type CouponBatchRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=64"`
	Prefix     string `json:"prefix" binding:"max=16"`
	CodeCount  int    `json:"codeCount" binding:"required,min=1,max=5000"`
	CodeLength int    `json:"codeLength" binding:"required,min=6,max=24"`
}

type CouponCode struct {
	ID                   string             `json:"id"`
	CampaignID           string             `json:"campaignId"`
	CampaignName         string             `json:"campaignName"`
	BatchID              string             `json:"batchId"`
	BatchName            string             `json:"batchName"`
	CodeMode             CouponCodeMode     `json:"codeMode"`
	CodeValue            string             `json:"codeValue"`
	CodeHash             string             `json:"-"`
	CodeMask             string             `json:"codeMask"`
	DiscountType         CouponDiscountType `json:"discountType"`
	FixedDiscountCNYCent int64              `json:"fixedDiscountCnyCent"`
	PercentOffBPS        int                `json:"percentOffBps"`
	MaxDiscountCNYCent   int64              `json:"maxDiscountCnyCent"`
	Status               CouponCodeStatus   `json:"status"`
	MaxUsages            int                `json:"maxUsages"`
	UsedCount            int                `json:"usedCount"`
	LastUsedAt           *time.Time         `json:"lastUsedAt"`
	PerUserLimit         int                `json:"perUserLimit"`
	StartsAt             *time.Time         `json:"startsAt"`
	EndsAt               *time.Time         `json:"endsAt"`
	CreatedAt            time.Time          `json:"createdAt"`
	UpdatedAt            time.Time          `json:"updatedAt"`
}

type CouponUsageResponse struct {
	ID                 string            `json:"id"`
	CampaignID         string            `json:"campaignId"`
	CampaignName       string            `json:"campaignName"`
	CodeID             string            `json:"codeId"`
	CodeMask           string            `json:"codeMask"`
	UserID             string            `json:"userId"`
	Username           string            `json:"username"`
	OrderID            string            `json:"orderId"`
	OrderNo            string            `json:"orderNo"`
	Status             CouponUsageStatus `json:"status"`
	DiscountCNYCent    int64             `json:"discountCnyCent"`
	FinalAmountCNYCent int64             `json:"finalAmountCnyCent"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type CouponUsage struct {
	ID                 string            `json:"id"`
	CampaignID         string            `json:"campaignId"`
	CodeID             string            `json:"codeId"`
	UserID             string            `json:"userId"`
	OrderID            string            `json:"orderId"`
	OrderNo            string            `json:"orderNo"`
	Status             CouponUsageStatus `json:"status"`
	DiscountCNYCent    int64             `json:"discountCnyCent"`
	FinalAmountCNYCent int64             `json:"finalAmountCnyCent"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type PurchaseCouponQuote struct {
	CampaignID           string             `json:"campaignId"`
	CampaignName         string             `json:"campaignName"`
	CodeID               string             `json:"codeId"`
	CodeValue            string             `json:"codeValue"`
	CodeMask             string             `json:"codeMask"`
	DiscountType         CouponDiscountType `json:"discountType"`
	FixedDiscountCNYCent int64              `json:"fixedDiscountCnyCent"`
	PercentOffBPS        int                `json:"percentOffBps"`
	MaxDiscountCNYCent   int64              `json:"maxDiscountCnyCent"`
	DiscountCNYCent      int64              `json:"discountCnyCent"`
}

type PurchaseQuoteRequest struct {
	Kind         PurchaseOrderKind    `json:"kind" binding:"required,oneof=subscription balance_topup"`
	ProductID    string               `json:"productId,omitempty"`
	AmountUsd    string               `json:"amountUsd,omitempty"`
	DeliveryMode PurchaseDeliveryMode `json:"deliveryMode" binding:"omitempty,oneof=account redeem_code"`
	CouponCode   string               `json:"couponCode,omitempty"`
}

type PurchaseQuoteResponse struct {
	Kind                  PurchaseOrderKind    `json:"kind"`
	ProductID             string               `json:"productId"`
	ProductName           string               `json:"productName"`
	OrderKindLabel        string               `json:"orderKindLabel"`
	DeliveryMode          PurchaseDeliveryMode `json:"deliveryMode"`
	OriginalAmountCNYCent int64                `json:"originalAmountCnyCent"`
	DiscountCNYCent       int64                `json:"discountCnyCent"`
	FinalAmountCNYCent    int64                `json:"finalAmountCnyCent"`
	BalanceTopupMicros    int64                `json:"balanceTopupMicros"`
	Coupon                *PurchaseCouponQuote `json:"coupon"`
}
