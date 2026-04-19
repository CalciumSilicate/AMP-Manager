package handler

import (
	"ampmanager/internal/amp"
	"ampmanager/internal/database"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"
)

var (
	reinitLogWriter                = amp.ReinitLogWriter
	reinitRequestDetailStore       = amp.ReinitRequestDetailStore
	reinitPendingCleaner           = amp.ReinitPendingCleaner
	reloadBillingRuntimeBestEffort = func(reason string) {
		service.NewSystemConfigService().ReloadBillingRuntimeFromSystemConfigBestEffort(reason)
	}
	restartDashboardAggregateWorker = repository.RestartDashboardAggregateWorker
)

func reinitDatabaseBackedRuntimeServices() error {
	reinitLogWriter(database.GetDB())
	reinitRequestDetailStore(database.GetDB())
	reinitPendingCleaner(database.GetDB())
	reloadBillingRuntimeBestEffort("database-backed runtime reinitialization")
	if err := service.NewRequestLogService().EnsureDashboardAggregatesReady(); err != nil {
		return err
	}
	restartDashboardAggregateWorker()
	return nil
}
