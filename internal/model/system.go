package model

import "time"

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
	Enabled        bool  `json:"enabled"`
	TTLSec         int64 `json:"ttlSec"`
	MaxEntries     int   `json:"maxEntries"`
	MaxMemoryMB    int64 `json:"maxMemoryMB"`
	BodyCapKB      int   `json:"bodyCapKB"`
	PersistEnabled bool  `json:"persistEnabled"`
}

// RequestDetailConfigRequest 请求详情配置请求
type RequestDetailConfigRequest struct {
	Enabled        bool  `json:"enabled"`
	TTLSec         int64 `json:"ttlSec"`
	MaxEntries     int   `json:"maxEntries"`
	MaxMemoryMB    int64 `json:"maxMemoryMB"`
	BodyCapKB      int   `json:"bodyCapKB"`
	PersistEnabled bool  `json:"persistEnabled"`
}

// SiteConfigResponse 站点配置响应
type SiteConfigResponse struct {
	SiteName string `json:"siteName"`
}

// SiteConfigRequest 站点配置请求
type SiteConfigRequest struct {
	SiteName string `json:"siteName"`
}

type BillingRuntimeConfigResponse struct {
	RedisURL             string `json:"redisUrl"`
	RedisURLMasked       string `json:"redisUrlMasked"`
	RedisPrefix          string `json:"redisPrefix"`
	ReservationTTLSec    int    `json:"reservationTtlSec"`
	ReconcileIntervalSec int    `json:"reconcileIntervalSec"`
	StreamBatchSize      int    `json:"streamBatchSize"`
	RuntimeEnabled       bool   `json:"runtimeEnabled"`
	RuntimeHealthy       bool   `json:"runtimeHealthy"`
}

type BillingRuntimeConfigRequest struct {
	RedisURL             string `json:"redisUrl"`
	RedisPrefix          string `json:"redisPrefix"`
	ReservationTTLSec    int    `json:"reservationTtlSec"`
	ReconcileIntervalSec int    `json:"reconcileIntervalSec"`
	StreamBatchSize      int    `json:"streamBatchSize"`
}
