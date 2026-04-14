package service

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/config"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

func TestApplyAndStoreBillingRuntimeConfigRollsBackOnPersistError(t *testing.T) {
	setupBillingRuntimeServiceTestDB(t)

	svc := NewSystemConfigService()
	oldRuntime := &billingstate.Runtime{}
	newRuntime := &billingstate.Runtime{}
	currentRuntime := oldRuntime
	var closed []*billingstate.Runtime

	restore := stubBillingRuntimeServiceHooks(
		t,
		func(cfg billingstate.Config) (*billingstate.Runtime, error) {
			if cfg.RedisURL != "redis://next" {
				t.Fatalf("build got redis URL %q", cfg.RedisURL)
			}
			return newRuntime, nil
		},
		func(rt *billingstate.Runtime) *billingstate.Runtime {
			previous := currentRuntime
			currentRuntime = rt
			return previous
		},
		func(rt *billingstate.Runtime) {
			if rt != nil {
				closed = append(closed, rt)
			}
		},
		func(_ *SystemConfigService, _ model.BillingRuntimeConfigRequest) error {
			return errors.New("write failed")
		},
	)
	defer restore()

	err := svc.ApplyAndStoreBillingRuntimeConfig(model.BillingRuntimeConfigRequest{
		RedisURL: "redis://next",
	})
	if !IsBillingRuntimePersistError(err) {
		t.Fatalf("expected persist error, got %v", err)
	}
	if currentRuntime != oldRuntime {
		t.Fatalf("current runtime = %p, want rollback to %p", currentRuntime, oldRuntime)
	}
	if len(closed) != 1 || closed[0] != newRuntime {
		t.Fatalf("closed runtimes = %v, want [%p]", closed, newRuntime)
	}
}

func TestApplyAndStoreBillingRuntimeConfigCommitsNewRuntime(t *testing.T) {
	setupBillingRuntimeServiceTestDB(t)

	svc := NewSystemConfigService()
	oldRuntime := &billingstate.Runtime{}
	newRuntime := &billingstate.Runtime{}
	currentRuntime := oldRuntime
	var closed []*billingstate.Runtime

	restore := stubBillingRuntimeServiceHooks(
		t,
		func(cfg billingstate.Config) (*billingstate.Runtime, error) {
			if cfg.Prefix != "team-a" {
				t.Fatalf("build got prefix %q", cfg.Prefix)
			}
			if cfg.StreamBatchSize != 8 {
				t.Fatalf("build got stream batch size %d", cfg.StreamBatchSize)
			}
			if cfg.ReconcileBatchSize != 5 {
				t.Fatalf("build got reconcile batch size %d", cfg.ReconcileBatchSize)
			}
			if cfg.ExpiryBatchSize != 3 {
				t.Fatalf("build got expiry batch size %d", cfg.ExpiryBatchSize)
			}
			return newRuntime, nil
		},
		func(rt *billingstate.Runtime) *billingstate.Runtime {
			previous := currentRuntime
			currentRuntime = rt
			return previous
		},
		func(rt *billingstate.Runtime) {
			if rt != nil {
				closed = append(closed, rt)
			}
		},
		nil,
	)
	defer restore()

	err := svc.ApplyAndStoreBillingRuntimeConfig(model.BillingRuntimeConfigRequest{
		RedisURL:             "redis://apply",
		RedisPrefix:          "team-a",
		ReservationTTLSec:    120,
		ReconcileIntervalSec: 30,
		StreamBatchSize:      8,
		ReconcileBatchSize:   5,
		ExpiryBatchSize:      3,
	})
	if err != nil {
		t.Fatalf("ApplyAndStoreBillingRuntimeConfig returned error: %v", err)
	}
	if currentRuntime != newRuntime {
		t.Fatalf("current runtime = %p, want %p", currentRuntime, newRuntime)
	}
	if len(closed) != 1 || closed[0] != oldRuntime {
		t.Fatalf("closed runtimes = %v, want [%p]", closed, oldRuntime)
	}
}

func TestApplyAndStoreBillingRuntimeConfigDisablesRuntime(t *testing.T) {
	setupBillingRuntimeServiceTestDB(t)

	svc := NewSystemConfigService()
	oldRuntime := &billingstate.Runtime{}
	currentRuntime := oldRuntime
	var closed []*billingstate.Runtime

	restore := stubBillingRuntimeServiceHooks(
		t,
		func(cfg billingstate.Config) (*billingstate.Runtime, error) {
			if cfg.RedisURL != "" {
				t.Fatalf("build got redis URL %q, want disable", cfg.RedisURL)
			}
			return nil, nil
		},
		func(rt *billingstate.Runtime) *billingstate.Runtime {
			previous := currentRuntime
			currentRuntime = rt
			return previous
		},
		func(rt *billingstate.Runtime) {
			if rt != nil {
				closed = append(closed, rt)
			}
		},
		nil,
	)
	defer restore()

	err := svc.ApplyAndStoreBillingRuntimeConfig(model.BillingRuntimeConfigRequest{
		RedisURL:             "",
		RedisPrefix:          "",
		ReservationTTLSec:    120,
		ReconcileIntervalSec: 30,
		StreamBatchSize:      8,
		ReconcileBatchSize:   5,
		ExpiryBatchSize:      3,
	})
	if err != nil {
		t.Fatalf("ApplyAndStoreBillingRuntimeConfig returned error: %v", err)
	}
	if currentRuntime != nil {
		t.Fatalf("current runtime = %p, want nil", currentRuntime)
	}
	if len(closed) != 1 || closed[0] != oldRuntime {
		t.Fatalf("closed runtimes = %v, want [%p]", closed, oldRuntime)
	}

	stored, err := svc.GetBillingRuntimeConfigRequest()
	if err != nil {
		t.Fatalf("GetBillingRuntimeConfigRequest returned error: %v", err)
	}
	if stored.RedisURL != "" {
		t.Fatalf("stored redis URL = %q, want empty", stored.RedisURL)
	}
	if stored.RedisPrefix != "ampmanager" {
		t.Fatalf("stored redis prefix = %q, want ampmanager", stored.RedisPrefix)
	}
	if stored.ReconcileBatchSize != 5 {
		t.Fatalf("stored reconcile batch size = %d, want 5", stored.ReconcileBatchSize)
	}
	if stored.ExpiryBatchSize != 3 {
		t.Fatalf("stored expiry batch size = %d, want 3", stored.ExpiryBatchSize)
	}
}

func TestReloadBillingRuntimeFromSystemConfigBestEffortFallsBackToLegacyOnBuildError(t *testing.T) {
	setupBillingRuntimeServiceTestDB(t)

	svc := NewSystemConfigService()
	if err := svc.SetBillingRuntimeConfig(model.BillingRuntimeConfigRequest{
		RedisURL:             "redis://broken",
		RedisPrefix:          "team-a",
		ReservationTTLSec:    120,
		ReconcileIntervalSec: 30,
		StreamBatchSize:      8,
		ReconcileBatchSize:   5,
		ExpiryBatchSize:      3,
	}); err != nil {
		t.Fatalf("SetBillingRuntimeConfig returned error: %v", err)
	}

	oldRuntime := &billingstate.Runtime{}
	currentRuntime := oldRuntime
	replaceCalled := false
	var closed []*billingstate.Runtime

	restore := stubBillingRuntimeServiceHooks(
		t,
		func(cfg billingstate.Config) (*billingstate.Runtime, error) {
			if cfg.RedisURL != "redis://broken" {
				t.Fatalf("build got redis URL %q", cfg.RedisURL)
			}
			if cfg.ReconcileBatchSize != 5 || cfg.ExpiryBatchSize != 3 {
				t.Fatalf("build got batch sizes reconcile=%d expiry=%d", cfg.ReconcileBatchSize, cfg.ExpiryBatchSize)
			}
			return nil, errors.New("dial failed")
		},
		func(rt *billingstate.Runtime) *billingstate.Runtime {
			replaceCalled = true
			if rt != nil {
				t.Fatalf("replace got runtime %p, want nil legacy fallback", rt)
			}
			previous := currentRuntime
			currentRuntime = rt
			return previous
		},
		func(rt *billingstate.Runtime) {
			if rt != nil {
				closed = append(closed, rt)
			}
		},
		nil,
	)
	defer restore()

	svc.ReloadBillingRuntimeFromSystemConfigBestEffort("test reload")

	if !replaceCalled {
		t.Fatal("expected replace to be called for legacy fallback")
	}
	if len(closed) != 1 || closed[0] != oldRuntime {
		t.Fatalf("closed runtimes = %v, want [%p]", closed, oldRuntime)
	}
	if currentRuntime != nil {
		t.Fatalf("current runtime = %p, want nil", currentRuntime)
	}
}

func TestGetBillingRuntimeConfigRequestFallsBackNewBatchSizesToStreamBatchSize(t *testing.T) {
	setupBillingRuntimeServiceTestDB(t)

	svc := NewSystemConfigService()
	if err := svc.repo.Set(billingRuntimeConfigKey, `{"redisUrl":"redis://legacy","redisPrefix":"team-a","reservationTtlSec":120,"reconcileIntervalSec":30,"streamBatchSize":7}`); err != nil {
		t.Fatalf("repo.Set returned error: %v", err)
	}

	req, err := svc.GetBillingRuntimeConfigRequest()
	if err != nil {
		t.Fatalf("GetBillingRuntimeConfigRequest returned error: %v", err)
	}
	if req.StreamBatchSize != 7 {
		t.Fatalf("stream batch size = %d, want 7", req.StreamBatchSize)
	}
	if req.ReconcileBatchSize != 7 {
		t.Fatalf("reconcile batch size = %d, want 7", req.ReconcileBatchSize)
	}
	if req.ExpiryBatchSize != 7 {
		t.Fatalf("expiry batch size = %d, want 7", req.ExpiryBatchSize)
	}
}

func TestGetBillingRuntimeConfigRequestKeepsExplicitEnvBatchSizesForLegacyStoredConfig(t *testing.T) {
	setupBillingRuntimeServiceTestDB(t)

	t.Setenv("BILLING_STREAM_BATCH_SIZE", "7")
	t.Setenv("BILLING_RECONCILE_BATCH_SIZE", "11")
	t.Setenv("BILLING_EXPIRY_BATCH_SIZE", "13")
	config.Load()

	svc := NewSystemConfigService()
	if err := svc.repo.Set(billingRuntimeConfigKey, `{"redisUrl":"redis://legacy","redisPrefix":"team-a","reservationTtlSec":120,"reconcileIntervalSec":30,"streamBatchSize":7}`); err != nil {
		t.Fatalf("repo.Set returned error: %v", err)
	}

	req, err := svc.GetBillingRuntimeConfigRequest()
	if err != nil {
		t.Fatalf("GetBillingRuntimeConfigRequest returned error: %v", err)
	}
	if req.StreamBatchSize != 7 {
		t.Fatalf("stream batch size = %d, want 7", req.StreamBatchSize)
	}
	if req.ReconcileBatchSize != 11 {
		t.Fatalf("reconcile batch size = %d, want 11", req.ReconcileBatchSize)
	}
	if req.ExpiryBatchSize != 13 {
		t.Fatalf("expiry batch size = %d, want 13", req.ExpiryBatchSize)
	}
}

func TestDefaultBillingRuntimeConfigRequestFallsBackEnvBatchSizesToStreamBatchSize(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("REDIS_PREFIX", "ampmanager")
	t.Setenv("BILLING_RESERVATION_TTL_SEC", "600")
	t.Setenv("BILLING_RECONCILE_INTERVAL_SEC", "60")
	t.Setenv("BILLING_STREAM_BATCH_SIZE", "12")
	t.Setenv("BILLING_RECONCILE_BATCH_SIZE", "")
	t.Setenv("BILLING_EXPIRY_BATCH_SIZE", "")
	config.Load()

	req := defaultBillingRuntimeConfigRequest()
	if req.StreamBatchSize != 12 {
		t.Fatalf("stream batch size = %d, want 12", req.StreamBatchSize)
	}
	if req.ReconcileBatchSize != 12 {
		t.Fatalf("reconcile batch size = %d, want 12", req.ReconcileBatchSize)
	}
	if req.ExpiryBatchSize != 12 {
		t.Fatalf("expiry batch size = %d, want 12", req.ExpiryBatchSize)
	}
}

func stubBillingRuntimeServiceHooks(
	t *testing.T,
	build func(cfg billingstate.Config) (*billingstate.Runtime, error),
	replace func(rt *billingstate.Runtime) *billingstate.Runtime,
	closeFn func(rt *billingstate.Runtime),
	persist func(s *SystemConfigService, req model.BillingRuntimeConfigRequest) error,
) func() {
	t.Helper()

	previousBuild := buildBillingRuntime
	previousReplace := replaceBillingRuntime
	previousClose := closeBillingRuntime
	previousPersist := persistBillingRuntimeConfig

	if build != nil {
		buildBillingRuntime = build
	}
	if replace != nil {
		replaceBillingRuntime = replace
	}
	if closeFn != nil {
		closeBillingRuntime = closeFn
	}
	if persist != nil {
		persistBillingRuntimeConfig = persist
	}

	return func() {
		buildBillingRuntime = previousBuild
		replaceBillingRuntime = previousReplace
		closeBillingRuntime = previousClose
		persistBillingRuntimeConfig = previousPersist
	}
}

func setupBillingRuntimeServiceTestDB(t *testing.T) {
	t.Helper()

	t.Setenv("REDIS_URL", "")
	t.Setenv("REDIS_PREFIX", "ampmanager")
	t.Setenv("BILLING_RESERVATION_TTL_SEC", "600")
	t.Setenv("BILLING_RECONCILE_INTERVAL_SEC", "60")
	t.Setenv("BILLING_STREAM_BATCH_SIZE", "100")
	config.Load()

	dbPath := filepath.Join(t.TempDir(), "billing-runtime-test.sqlite")
	if err := database.InitWithOptions(database.Options{
		Type:       database.DBTypeSQLite,
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("database.InitWithOptions returned error: %v", err)
	}

	t.Cleanup(func() {
		billingstate.Close()
		if err := database.CloseAndRelease(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}
