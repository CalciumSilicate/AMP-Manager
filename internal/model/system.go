package model

import "time"

const (
	AmpProxySettingsPolicyDisabled  = "disabled"
	AmpProxySettingsPolicyAdminOnly = "admin_only"
	AmpProxySettingsPolicyAll       = "all"
)

// RetryConfigResponse 重试配置响应
type RetryConfigResponse struct {
	Enabled           bool  `json:"enabled"`
	MaxAttempts       int   `json:"maxAttempts"`
	GateTimeoutMs     int64 `json:"gateTimeoutMs"`
	MaxBodyBytes      int64 `json:"maxBodyBytes"`
	BackoffBaseMs     int64 `json:"backoffBaseMs"`
	BackoffMaxMs      int64 `json:"backoffMaxMs"`
	RetryOn429        bool  `json:"retryOn429"`
	RetryOn5xx        bool  `json:"retryOn5xx"`
	RespectRetryAfter bool  `json:"respectRetryAfter"`
	RetryOnEmptyBody  bool  `json:"retryOnEmptyBody"`
}

// RetryConfigRequest 重试配置请求
type RetryConfigRequest struct {
	Enabled           bool  `json:"enabled"`
	MaxAttempts       int   `json:"maxAttempts"`
	GateTimeoutMs     int64 `json:"gateTimeoutMs"`
	MaxBodyBytes      int64 `json:"maxBodyBytes"`
	BackoffBaseMs     int64 `json:"backoffBaseMs"`
	BackoffMaxMs      int64 `json:"backoffMaxMs"`
	RetryOn429        bool  `json:"retryOn429"`
	RetryOn5xx        bool  `json:"retryOn5xx"`
	RespectRetryAfter bool  `json:"respectRetryAfter"`
	RetryOnEmptyBody  bool  `json:"retryOnEmptyBody"`
}

type RequestPayloadLimitResponse struct {
	MaxBytes int64 `json:"maxBytes"`
}

type RequestPayloadLimitRequest struct {
	MaxBytes int64 `json:"maxBytes"`
}

// SystemConfig 系统配置存储
type SystemConfig struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TimeoutConfigResponse 超时配置响应
type TimeoutConfigResponse struct {
	IdleConnTimeoutSec     int `json:"idleConnTimeoutSec"`
	ReadIdleTimeoutSec     int `json:"readIdleTimeoutSec"`
	KeepAliveIntervalSec   int `json:"keepAliveIntervalSec"`
	DialTimeoutSec         int `json:"dialTimeoutSec"`
	TLSHandshakeTimeoutSec int `json:"tlsHandshakeTimeoutSec"`
}

// TimeoutConfigRequest 超时配置请求
type TimeoutConfigRequest struct {
	IdleConnTimeoutSec     int `json:"idleConnTimeoutSec"`
	ReadIdleTimeoutSec     int `json:"readIdleTimeoutSec"`
	KeepAliveIntervalSec   int `json:"keepAliveIntervalSec"`
	DialTimeoutSec         int `json:"dialTimeoutSec"`
	TLSHandshakeTimeoutSec int `json:"tlsHandshakeTimeoutSec"`
}

// RequestDetailConfigResponse 请求详情配置响应
type RequestDetailConfigResponse struct {
	Enabled              bool   `json:"enabled"`
	TTLSec               int64  `json:"ttlSec"`
	MaxEntries           int    `json:"maxEntries"`
	MaxMemoryMB          int64  `json:"maxMemoryMB"`
	BodyCapKB            int    `json:"bodyCapKB"`
	PersistEnabled       bool   `json:"persistEnabled"`
	HighRPMMode          string `json:"highRpmMode"`
	HighRPMThreshold     int    `json:"highRpmThreshold"`
	HighRPMSamplePercent int    `json:"highRpmSamplePercent"`
}

// RequestDetailConfigRequest 请求详情配置请求
type RequestDetailConfigRequest struct {
	Enabled              bool   `json:"enabled"`
	TTLSec               int64  `json:"ttlSec"`
	MaxEntries           int    `json:"maxEntries"`
	MaxMemoryMB          int64  `json:"maxMemoryMB"`
	BodyCapKB            int    `json:"bodyCapKB"`
	PersistEnabled       bool   `json:"persistEnabled"`
	HighRPMMode          string `json:"highRpmMode"`
	HighRPMThreshold     int    `json:"highRpmThreshold"`
	HighRPMSamplePercent int    `json:"highRpmSamplePercent"`
}

// SiteConfigResponse 站点配置响应
type SiteContactConfig struct {
	Enabled            bool   `json:"enabled"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Link               string `json:"link"`
	QRCodeImageDataURL string `json:"qrCodeImageDataUrl"`
}

type SiteConfigResponse struct {
	SiteName               string                     `json:"siteName"`
	TimeZone               string                     `json:"timeZone"`
	AmpProxySettingsPolicy string                     `json:"ampProxySettingsPolicy"`
	AmpSettingsPolicy      string                     `json:"ampSettingsPolicy"`
	StatusMonitorAvailable bool                       `json:"statusMonitorAvailable"`
	Contact                SiteContactConfig          `json:"contact"`
	InviteEnabled          bool                       `json:"inviteEnabled"`
	SessionSticky          *SessionStickyPublicConfig `json:"sessionSticky,omitempty"`
}

// SiteConfigRequest 站点配置请求
type SiteContactConfigRequest struct {
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Link        string `json:"link"`
}

type SiteConfigRequest struct {
	SiteName               string                    `json:"siteName"`
	TimeZone               string                    `json:"timeZone"`
	AmpProxySettingsPolicy *string                   `json:"ampProxySettingsPolicy,omitempty"`
	AmpSettingsPolicy      *string                   `json:"ampSettingsPolicy,omitempty"`
	Contact                *SiteContactConfigRequest `json:"contact,omitempty"`
}

type BillingRuntimeConfigResponse struct {
	RedisURL              string `json:"redisUrl"`
	RedisURLMasked        string `json:"redisUrlMasked"`
	RedisPrefix           string `json:"redisPrefix"`
	ReservationTTLSec     int    `json:"reservationTtlSec"`
	ReconcileIntervalSec  int    `json:"reconcileIntervalSec"`
	StreamBatchSize       int    `json:"streamBatchSize"`
	ReconcileBatchSize    int    `json:"reconcileBatchSize"`
	ExpiryBatchSize       int    `json:"expiryBatchSize"`
	ProjectorWorkers      int    `json:"projectorWorkers"`
	ProjectorClaimIdleSec int    `json:"projectorClaimIdleSec"`
	RuntimeEnabled        bool   `json:"runtimeEnabled"`
	RuntimeHealthy        bool   `json:"runtimeHealthy"`
}

type BillingRuntimeConfigRequest struct {
	RedisURL              string `json:"redisUrl"`
	RedisPrefix           string `json:"redisPrefix"`
	ReservationTTLSec     int    `json:"reservationTtlSec"`
	ReconcileIntervalSec  int    `json:"reconcileIntervalSec"`
	StreamBatchSize       int    `json:"streamBatchSize"`
	ReconcileBatchSize    int    `json:"reconcileBatchSize"`
	ExpiryBatchSize       int    `json:"expiryBatchSize"`
	ProjectorWorkers      int    `json:"projectorWorkers"`
	ProjectorClaimIdleSec int    `json:"projectorClaimIdleSec"`
}

type BillingRuntimeMetricSummary struct {
	Samples  int   `json:"samples"`
	P95Ms    int64 `json:"p95Ms"`
	P99Ms    int64 `json:"p99Ms"`
	Failures int64 `json:"failures"`
}

type BillingRuntimeStatsResponse struct {
	RuntimeEnabled   bool                        `json:"runtimeEnabled"`
	RuntimeHealthy   bool                        `json:"runtimeHealthy"`
	Reserve          BillingRuntimeMetricSummary `json:"reserve"`
	Settle           BillingRuntimeMetricSummary `json:"settle"`
	Project          BillingRuntimeMetricSummary `json:"project"`
	Reclaim          BillingRuntimeMetricSummary `json:"reclaim"`
	Reconcile        BillingRuntimeMetricSummary `json:"reconcile"`
	ReclaimClaimed   int64                       `json:"reclaimClaimed"`
	ReconcileRepairs int64                       `json:"reconcileRepairs"`
	ConsumerCount    int                         `json:"consumerCount"`
	ActiveConsumers  int                         `json:"activeProjectorConsumers"`
	StaleConsumers   int                         `json:"staleProjectorConsumers"`
	PendingEntries   int64                       `json:"pendingEntries"`
	OldestPendingMs  int64                       `json:"oldestPendingIdleMs"`
}

type BillingDailyResetConfigResponse struct {
	Enabled               bool `json:"enabled"`
	MinRemainingDays      int  `json:"minRemainingDays"`
	UsageThresholdPercent int  `json:"usageThresholdPercent"`
	DailyLimit            int  `json:"dailyLimit"`
}

type BillingDailyResetConfigRequest struct {
	Enabled               bool `json:"enabled"`
	MinRemainingDays      int  `json:"minRemainingDays"`
	UsageThresholdPercent int  `json:"usageThresholdPercent"`
	DailyLimit            int  `json:"dailyLimit"`
}
