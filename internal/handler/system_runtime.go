package handler

import (
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/billingstate"
	"ampmanager/internal/database"
	"ampmanager/internal/service"
)

func reloadBillingRuntimeFromSystemConfig() error {
	cfg, err := service.NewSystemConfigService().GetBillingRuntimeConfig()
	if err != nil {
		return err
	}

	return billingstate.Init(billingstate.Config{
		RedisURL:          cfg.RedisURL,
		Prefix:            cfg.RedisPrefix,
		ReservationTTL:    time.Duration(cfg.ReservationTTLSec) * time.Second,
		ReconcileInterval: time.Duration(cfg.ReconcileIntervalSec) * time.Second,
		StreamBatchSize:   int64(cfg.StreamBatchSize),
	})
}

func reinitDatabaseBackedRuntimeServices() error {
	amp.ReinitLogWriter(database.GetDB())
	amp.ReinitRequestDetailStore(database.GetDB())
	amp.ReinitPendingCleaner(database.GetDB())
	return reloadBillingRuntimeFromSystemConfig()
}
