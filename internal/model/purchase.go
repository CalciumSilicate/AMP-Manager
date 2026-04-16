package model

import (
	"time"

	"ampmanager/internal/precision"
)

type PaymentChannel string

const (
	PaymentChannelAlipay PaymentChannel = "alipay"
)

type PurchasePaymentStatus string

const (
	PurchasePaymentStatusPending PurchasePaymentStatus = "pending"
	PurchasePaymentStatusPaid    PurchasePaymentStatus = "paid"
	PurchasePaymentStatusExpired PurchasePaymentStatus = "expired"
	PurchasePaymentStatusClosed  PurchasePaymentStatus = "closed"
	PurchasePaymentStatusFailed  PurchasePaymentStatus = "failed"
)

type PurchaseFulfillmentStatus string

const (
	PurchaseFulfillmentStatusPending   PurchaseFulfillmentStatus = "pending"
	PurchaseFulfillmentStatusFulfilled PurchaseFulfillmentStatus = "fulfilled"
	PurchaseFulfillmentStatusFailed    PurchaseFulfillmentStatus = "failed"
)

type PurchaseOrderKind string

const (
	PurchaseOrderKindSubscription PurchaseOrderKind = "subscription"
	PurchaseOrderKindBalanceTopup PurchaseOrderKind = "balance_topup"
)

type AlipayEnvironment string

const (
	AlipayEnvironmentSandbox    AlipayEnvironment = "sandbox"
	AlipayEnvironmentProduction AlipayEnvironment = "production"
)

type PurchaseSettings struct {
	PurchaseEnabled                bool              `json:"purchaseEnabled"`
	DebugAutoPaid                  bool              `json:"debugAutoPaid"`
	AlipayAppID                    string            `json:"alipayAppId"`
	AlipayPID                      string            `json:"alipayPid"`
	AlipayEnvironment              AlipayEnvironment `json:"alipayEnvironment"`
	AlipayNotifyURL                string            `json:"alipayNotifyUrl"`
	AlipayPublicKey                string            `json:"alipayPublicKey"`
	AlipayPrivateKey               string            `json:"-"`
	BalanceTopupPriceCnyCentPerUSD int64             `json:"-"`
}

type PurchaseSettingsResponse struct {
	PurchaseEnabled                bool              `json:"purchaseEnabled"`
	DebugAutoPaid                  bool              `json:"debugAutoPaid"`
	AlipayAppID                    string            `json:"alipayAppId"`
	AlipayPID                      string            `json:"alipayPid"`
	AlipayEnvironment              AlipayEnvironment `json:"alipayEnvironment"`
	AlipayNotifyURL                string            `json:"alipayNotifyUrl"`
	AlipayPublicKey                string            `json:"alipayPublicKey"`
	PrivateKeySet                  bool              `json:"privateKeySet"`
	PaymentConfigured              bool              `json:"paymentConfigured"`
	BalanceTopupPriceCnyPerUsd     float64           `json:"balanceTopupPriceCnyPerUsd"`
	BalanceTopupPriceCnyCentPerUsd int64             `json:"balanceTopupPriceCnyCentPerUsd"`
}

type PurchaseSettingsRequest struct {
	PurchaseEnabled            bool                     `json:"purchaseEnabled"`
	DebugAutoPaid              bool                     `json:"debugAutoPaid"`
	AlipayAppID                string                   `json:"alipayAppId" binding:"max=64"`
	AlipayPID                  string                   `json:"alipayPid" binding:"max=64"`
	AlipayEnvironment          AlipayEnvironment        `json:"alipayEnvironment" binding:"omitempty,oneof=sandbox production"`
	AlipayNotifyURL            string                   `json:"alipayNotifyUrl" binding:"max=255"`
	AlipayPublicKey            string                   `json:"alipayPublicKey" binding:"max=8192"`
	AlipayPrivateKey           string                   `json:"alipayPrivateKey" binding:"max=8192"`
	BalanceTopupPriceCnyPerUsd *precision.DecimalString `json:"balanceTopupPriceCnyPerUsd,omitempty"`
}

type PurchaseProduct struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Summary            string    `json:"summary"`
	SubscriptionPlanID string    `json:"subscriptionPlanId"`
	DurationDays       int       `json:"durationDays"`
	PriceCNYCent       int64     `json:"priceCnyCent"`
	IsRecommended      bool      `json:"isRecommended"`
	SortOrder          int       `json:"sortOrder"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type PurchaseProductRequest struct {
	Name               string `json:"name" binding:"required,min=1,max=64"`
	Summary            string `json:"summary" binding:"max=120"`
	SubscriptionPlanID string `json:"subscriptionPlanId" binding:"required"`
	DurationDays       int    `json:"durationDays" binding:"required,min=1,max=3650"`
	PriceCNYCent       int64  `json:"priceCnyCent" binding:"required,min=1"`
	IsRecommended      bool   `json:"isRecommended"`
	SortOrder          int    `json:"sortOrder"`
	Enabled            bool   `json:"enabled"`
}

type PurchaseProductResponse struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Summary              string    `json:"summary"`
	SubscriptionPlanID   string    `json:"subscriptionPlanId"`
	SubscriptionPlanName string    `json:"subscriptionPlanName"`
	DurationDays         int       `json:"durationDays"`
	PriceCNYCent         int64     `json:"priceCnyCent"`
	IsRecommended        bool      `json:"isRecommended"`
	SortOrder            int       `json:"sortOrder"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type PurchaseOrder struct {
	ID                 string                    `json:"id"`
	OrderNo            string                    `json:"orderNo"`
	UserID             string                    `json:"userId"`
	ProductID          string                    `json:"productId"`
	SubscriptionPlanID string                    `json:"subscriptionPlanId"`
	DurationDays       int                       `json:"durationDays"`
	AmountCNYCent      int64                     `json:"amountCnyCent"`
	OrderKind          PurchaseOrderKind         `json:"orderKind"`
	BalanceTopupMicros int64                     `json:"balanceTopupMicros"`
	PaymentChannel     PaymentChannel            `json:"paymentChannel"`
	PaymentStatus      PurchasePaymentStatus     `json:"paymentStatus"`
	FulfillmentStatus  PurchaseFulfillmentStatus `json:"fulfillmentStatus"`
	AlipayTradeNo      string                    `json:"alipayTradeNo"`
	AlipayQRCode       string                    `json:"-"`
	AlipayQRURL        string                    `json:"-"`
	ExpiresAt          *time.Time                `json:"expiresAt"`
	PaidAt             *time.Time                `json:"paidAt"`
	FulfilledAt        *time.Time                `json:"fulfilledAt"`
	FailureReason      string                    `json:"failureReason"`
	CreatedAt          time.Time                 `json:"createdAt"`
	UpdatedAt          time.Time                 `json:"updatedAt"`
}

type PurchaseOrderResponse struct {
	ID                    string                    `json:"id"`
	OrderNo               string                    `json:"orderNo"`
	UserID                string                    `json:"userId"`
	Username              string                    `json:"username,omitempty"`
	ProductID             string                    `json:"productId"`
	ProductName           string                    `json:"productName"`
	ProductSummary        string                    `json:"productSummary"`
	SubscriptionPlanID    string                    `json:"subscriptionPlanId"`
	SubscriptionPlanName  string                    `json:"subscriptionPlanName"`
	DurationDays          int                       `json:"durationDays"`
	AmountCNYCent         int64                     `json:"amountCnyCent"`
	OrderKind             PurchaseOrderKind         `json:"orderKind"`
	BalanceTopupMicros    int64                     `json:"balanceTopupMicros"`
	PaymentChannel        PaymentChannel            `json:"paymentChannel"`
	PaymentStatus         PurchasePaymentStatus     `json:"paymentStatus"`
	FulfillmentStatus     PurchaseFulfillmentStatus `json:"fulfillmentStatus"`
	AlipayTradeNo         string                    `json:"alipayTradeNo"`
	PaymentQRCode         string                    `json:"paymentQrCode"`
	PaymentQRURL          string                    `json:"paymentQrUrl"`
	PaymentQRImageDataURL string                    `json:"paymentQrImageDataUrl"`
	ExpiresAt             *time.Time                `json:"expiresAt"`
	PaidAt                *time.Time                `json:"paidAt"`
	FulfilledAt           *time.Time                `json:"fulfilledAt"`
	FailureReason         string                    `json:"failureReason"`
	CanRefresh            bool                      `json:"canRefresh"`
	CreatedAt             time.Time                 `json:"createdAt"`
	UpdatedAt             time.Time                 `json:"updatedAt"`
}

type PurchaseCatalogResponse struct {
	PurchaseEnabled                bool                       `json:"purchaseEnabled"`
	DebugAutoPaid                  bool                       `json:"debugAutoPaid"`
	PaymentConfigured              bool                       `json:"paymentConfigured"`
	RenewalRule                    string                     `json:"renewalRule"`
	CurrentSubscription            *UserSubscriptionResponse  `json:"currentSubscription"`
	Products                       []*PurchaseProductResponse `json:"products"`
	BalanceTopupEnabled            bool                       `json:"balanceTopupEnabled"`
	BalanceTopupPriceCnyPerUsd     float64                    `json:"balanceTopupPriceCnyPerUsd"`
	BalanceTopupPriceCnyCentPerUsd int64                      `json:"balanceTopupPriceCnyCentPerUsd"`
}

type PurchaseOrderListResponse struct {
	Items []*PurchaseOrderResponse `json:"items"`
	Total int64                    `json:"total"`
}

type CreatePurchaseOrderRequest struct {
	ProductID string `json:"productId" binding:"required"`
}

type CreateBalanceTopupOrderRequest struct {
	AmountUsd precision.DecimalString `json:"amountUsd"`
}

type PurchaseOrderFilters struct {
	PaymentStatus     PurchasePaymentStatus
	FulfillmentStatus PurchaseFulfillmentStatus
	Username          string
	ProductID         string
	Limit             int
}
