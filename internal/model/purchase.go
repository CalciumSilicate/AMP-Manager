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
	PurchasePaymentStatusPending  PurchasePaymentStatus = "pending"
	PurchasePaymentStatusPaid     PurchasePaymentStatus = "paid"
	PurchasePaymentStatusExpired  PurchasePaymentStatus = "expired"
	PurchasePaymentStatusRefunded PurchasePaymentStatus = "refunded"
	PurchasePaymentStatusClosed   PurchasePaymentStatus = "closed"
	PurchasePaymentStatusFailed   PurchasePaymentStatus = "failed"
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

type PurchaseDeliveryMode string

const (
	PurchaseDeliveryModeAccount    PurchaseDeliveryMode = "account"
	PurchaseDeliveryModeRedeemCode PurchaseDeliveryMode = "redeem_code"
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

type PurchaseWebhookEventStatus string

const (
	PurchaseWebhookEventStatusPending    PurchaseWebhookEventStatus = "pending"
	PurchaseWebhookEventStatusProcessing PurchaseWebhookEventStatus = "processing"
	PurchaseWebhookEventStatusSucceeded  PurchaseWebhookEventStatus = "succeeded"
	PurchaseWebhookEventStatusFailed     PurchaseWebhookEventStatus = "failed"
)

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
	GroupName          string    `json:"groupName"`
	GroupSort          int       `json:"groupSort"`
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
	GroupName          string `json:"groupName" binding:"max=64"`
	GroupSort          int    `json:"groupSort"`
	IsRecommended      bool   `json:"isRecommended"`
	SortOrder          int    `json:"sortOrder"`
	Enabled            bool   `json:"enabled"`
}

type PurchaseProductResponse struct {
	ID                          string    `json:"id"`
	Name                        string    `json:"name"`
	Summary                     string    `json:"summary"`
	SubscriptionPlanID          string    `json:"subscriptionPlanId"`
	SubscriptionPlanName        string    `json:"subscriptionPlanName"`
	SubscriptionPlanUpgradeRank int       `json:"subscriptionPlanUpgradeRank"`
	DurationDays                int       `json:"durationDays"`
	PriceCNYCent                int64     `json:"priceCnyCent"`
	GroupName                   string    `json:"groupName"`
	GroupSort                   int       `json:"groupSort"`
	IsRecommended               bool      `json:"isRecommended"`
	SortOrder                   int       `json:"sortOrder"`
	Enabled                     bool      `json:"enabled"`
	CreatedAt                   time.Time `json:"createdAt"`
	UpdatedAt                   time.Time `json:"updatedAt"`
}

type PurchaseOrder struct {
	ID                      string                    `json:"id"`
	OrderNo                 string                    `json:"orderNo"`
	UserID                  string                    `json:"userId"`
	ProductID               string                    `json:"productId"`
	SubscriptionPlanID      string                    `json:"subscriptionPlanId"`
	DurationDays            int                       `json:"durationDays"`
	AmountCNYCent           int64                     `json:"amountCnyCent"`
	OrderKind               PurchaseOrderKind         `json:"orderKind"`
	DeliveryMode            PurchaseDeliveryMode      `json:"deliveryMode"`
	BalanceTopupMicros      int64                     `json:"balanceTopupMicros"`
	PaymentChannel          PaymentChannel            `json:"paymentChannel"`
	PaymentStatus           PurchasePaymentStatus     `json:"paymentStatus"`
	FulfillmentStatus       PurchaseFulfillmentStatus `json:"fulfillmentStatus"`
	GeneratedRedeemCodeID   string                    `json:"generatedRedeemCodeId"`
	UpgradeSourcePlanID     string                    `json:"upgradeSourcePlanId"`
	UpgradeSourceExpiresAt  *time.Time                `json:"upgradeSourceExpiresAt"`
	UpgradeCreditCNYCent    int64                     `json:"upgradeCreditCnyCent"`
	UpgradeLockedTargetSecs int64                     `json:"upgradeLockedTargetSeconds"`
	UpgradeStateToken       string                    `json:"upgradeStateToken"`
	AlipayTradeNo           string                    `json:"alipayTradeNo"`
	ManualSettlementDone    bool                      `json:"manualSettlementDone"`
	AlipayQRCode            string                    `json:"-"`
	AlipayQRURL             string                    `json:"-"`
	ExpiresAt               *time.Time                `json:"expiresAt"`
	PaidAt                  *time.Time                `json:"paidAt"`
	FulfilledAt             *time.Time                `json:"fulfilledAt"`
	FailureReason           string                    `json:"failureReason"`
	CreatedAt               time.Time                 `json:"createdAt"`
	UpdatedAt               time.Time                 `json:"updatedAt"`
}

type PurchaseOrderResponse struct {
	ID                        string                    `json:"id"`
	OrderNo                   string                    `json:"orderNo"`
	UserID                    string                    `json:"userId"`
	Username                  string                    `json:"username,omitempty"`
	ProductID                 string                    `json:"productId"`
	ProductName               string                    `json:"productName"`
	ProductSummary            string                    `json:"productSummary"`
	SubscriptionPlanID        string                    `json:"subscriptionPlanId"`
	SubscriptionPlanName      string                    `json:"subscriptionPlanName"`
	DurationDays              int                       `json:"durationDays"`
	AmountCNYCent             int64                     `json:"amountCnyCent"`
	OrderKind                 PurchaseOrderKind         `json:"orderKind"`
	DeliveryMode              PurchaseDeliveryMode      `json:"deliveryMode"`
	BalanceTopupMicros        int64                     `json:"balanceTopupMicros"`
	PaymentChannel            PaymentChannel            `json:"paymentChannel"`
	PaymentStatus             PurchasePaymentStatus     `json:"paymentStatus"`
	FulfillmentStatus         PurchaseFulfillmentStatus `json:"fulfillmentStatus"`
	UpgradeSourcePlanID       string                    `json:"upgradeSourcePlanId"`
	UpgradeSourcePlanName     string                    `json:"upgradeSourcePlanName"`
	UpgradeSourceExpiresAt    *time.Time                `json:"upgradeSourceExpiresAt"`
	UpgradeCreditCnyCent      int64                     `json:"upgradeCreditCnyCent"`
	UpgradeLockedTargetSecs   int64                     `json:"upgradeLockedTargetSeconds"`
	GeneratedRedeemCodeID     string                    `json:"generatedRedeemCodeId"`
	GeneratedRedeemCode       string                    `json:"generatedRedeemCode"`
	GeneratedRedeemCodeMask   string                    `json:"generatedRedeemCodeMask"`
	GeneratedRedeemCodeStatus string                    `json:"generatedRedeemCodeStatus"`
	GeneratedRedeemedAt       *time.Time                `json:"generatedRedeemedAt"`
	AlipayTradeNo             string                    `json:"alipayTradeNo"`
	ManualSettlementDone      bool                      `json:"manualSettlementDone"`
	PaymentQRCode             string                    `json:"paymentQrCode"`
	PaymentQRURL              string                    `json:"paymentQrUrl"`
	PaymentQRImageDataURL     string                    `json:"paymentQrImageDataUrl"`
	ExpiresAt                 *time.Time                `json:"expiresAt"`
	PaidAt                    *time.Time                `json:"paidAt"`
	FulfilledAt               *time.Time                `json:"fulfilledAt"`
	FailureReason             string                    `json:"failureReason"`
	CanRefresh                bool                      `json:"canRefresh"`
	CreatedAt                 time.Time                 `json:"createdAt"`
	UpdatedAt                 time.Time                 `json:"updatedAt"`
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
	ProductID    string               `json:"productId" binding:"required"`
	DeliveryMode PurchaseDeliveryMode `json:"deliveryMode" binding:"omitempty,oneof=account redeem_code"`
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
	Offset            int
}

type PurchaseWebhookTarget struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	TargetURL       string    `json:"targetUrl"`
	BodyTemplate    string    `json:"bodyTemplate"`
	HeadersTemplate string    `json:"headersTemplate"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type PurchaseWebhookTargetRequest struct {
	Name            string `json:"name" binding:"required,min=1,max=64"`
	TargetURL       string `json:"targetUrl" binding:"required,max=2048"`
	BodyTemplate    string `json:"bodyTemplate" binding:"max=8192"`
	HeadersTemplate string `json:"headersTemplate" binding:"max=8192"`
	Enabled         bool   `json:"enabled"`
}

type PurchaseWebhookTestResponse struct {
	OK                 bool              `json:"ok"`
	ResponseStatusCode int               `json:"responseStatusCode"`
	ResponseHeaders    map[string]string `json:"responseHeaders"`
	ResponseBody       string            `json:"responseBody"`
	Variables          map[string]string `json:"variables"`
}

type PurchaseWebhookEvent struct {
	ID                 string                     `json:"id"`
	OrderID            string                     `json:"orderId"`
	OrderNo            string                     `json:"orderNo"`
	TargetID           string                     `json:"targetId"`
	Status             PurchaseWebhookEventStatus `json:"status"`
	AttemptCount       int                        `json:"attemptCount"`
	LastError          string                     `json:"lastError"`
	ResponseStatusCode int                        `json:"responseStatusCode"`
	ClaimToken         string                     `json:"claimToken"`
	ClaimedAt          *time.Time                 `json:"claimedAt"`
	ClaimUntil         *time.Time                 `json:"claimUntil"`
	NextAttemptAt      *time.Time                 `json:"nextAttemptAt"`
	LastAttemptAt      *time.Time                 `json:"lastAttemptAt"`
	DeliveredAt        *time.Time                 `json:"deliveredAt"`
	CreatedAt          time.Time                  `json:"createdAt"`
	UpdatedAt          time.Time                  `json:"updatedAt"`
}

type PurchaseOrderPaymentStatusHistory struct {
	ID         string                `json:"id"`
	OrderID    string                `json:"orderId"`
	OrderNo    string                `json:"orderNo"`
	FromStatus PurchasePaymentStatus `json:"fromStatus"`
	ToStatus   PurchasePaymentStatus `json:"toStatus"`
	Note       string                `json:"note"`
	CreatedBy  string                `json:"createdBy"`
	CreatedAt  time.Time             `json:"createdAt"`
}

type PurchaseOrderPaymentStatusUpdateRequest struct {
	PaymentStatus PurchasePaymentStatus `json:"paymentStatus" binding:"required,oneof=paid expired refunded"`
	Note          string                `json:"note" binding:"max=500"`
}

type PurchaseManualSettlementBatchMode string

const (
	PurchaseManualSettlementBatchModeSingle PurchaseManualSettlementBatchMode = "single"
	PurchaseManualSettlementBatchModeBatch  PurchaseManualSettlementBatchMode = "batch"
)

type PurchaseManualSettlementBatch struct {
	ID                 string                            `json:"id"`
	BatchNo            string                            `json:"batchNo"`
	Mode               PurchaseManualSettlementBatchMode `json:"mode"`
	CreatedBy          string                            `json:"createdBy"`
	Note               string                            `json:"note"`
	OrderCount         int                               `json:"orderCount"`
	TotalAmountCNYCent int64                             `json:"totalAmountCnyCent"`
	CreatedAt          time.Time                         `json:"createdAt"`
}

type PurchaseManualSettlementConfirmRequest struct {
	OrderNos []string `json:"orderNos" binding:"required,min=1"`
	Note     string   `json:"note" binding:"max=500"`
}
