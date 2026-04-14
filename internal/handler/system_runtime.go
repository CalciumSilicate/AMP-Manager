package handler

import (
	"ampmanager/internal/amp"
	"ampmanager/internal/database"
	"ampmanager/internal/service"
)

var (
	reinitLogWriter                = amp.ReinitLogWriter
	reinitRequestDetailStore       = amp.ReinitRequestDetailStore
	reinitPendingCleaner           = amp.ReinitPendingCleaner
	reloadBillingRuntimeBestEffort = func(reason string) {
		service.NewSystemConfigService().ReloadBillingRuntimeFromSystemConfigBestEffort(reason)
	}
)

func reinitDatabaseBackedRuntimeServices() error {
	reinitLogWriter(database.GetDB())
	reinitRequestDetailStore(database.GetDB())
	reinitPendingCleaner(database.GetDB())
	reloadBillingRuntimeBestEffort("database-backed runtime reinitialization")
	return nil
}
