package handler

import (
	"database/sql"
	"testing"
)

func TestReinitDatabaseBackedRuntimeServicesUsesBestEffortBillingReload(t *testing.T) {
	previousLogWriter := reinitLogWriter
	previousRequestDetailStore := reinitRequestDetailStore
	previousPendingCleaner := reinitPendingCleaner
	previousReload := reloadBillingRuntimeBestEffort
	defer func() {
		reinitLogWriter = previousLogWriter
		reinitRequestDetailStore = previousRequestDetailStore
		reinitPendingCleaner = previousPendingCleaner
		reloadBillingRuntimeBestEffort = previousReload
	}()

	logWriterCalled := false
	requestDetailCalled := false
	pendingCleanerCalled := false
	reloadReason := ""

	reinitLogWriter = func(_ *sql.DB) {
		logWriterCalled = true
	}
	reinitRequestDetailStore = func(_ *sql.DB) {
		requestDetailCalled = true
	}
	reinitPendingCleaner = func(_ *sql.DB) {
		pendingCleanerCalled = true
	}
	reloadBillingRuntimeBestEffort = func(reason string) {
		reloadReason = reason
	}

	if err := reinitDatabaseBackedRuntimeServices(); err != nil {
		t.Fatalf("reinitDatabaseBackedRuntimeServices returned error: %v", err)
	}
	if !logWriterCalled || !requestDetailCalled || !pendingCleanerCalled {
		t.Fatal("expected all database-backed runtime services to be reinitialized")
	}
	if reloadReason != "database-backed runtime reinitialization" {
		t.Fatalf("reload reason = %q", reloadReason)
	}
}
