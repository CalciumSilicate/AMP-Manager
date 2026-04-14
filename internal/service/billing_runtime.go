package service

import (
	"errors"
	"fmt"
	"log"
	"time"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/model"
)

var (
	buildBillingRuntime   = billingstate.Build
	replaceBillingRuntime = billingstate.Replace
	closeBillingRuntime   = func(rt *billingstate.Runtime) {
		if rt != nil {
			rt.Close()
		}
	}
	persistBillingRuntimeConfig = func(s *SystemConfigService, req model.BillingRuntimeConfigRequest) error {
		return s.SetBillingRuntimeConfig(req)
	}
)

type BillingRuntimePersistError struct {
	Err error
}

func (e *BillingRuntimePersistError) Error() string {
	if e == nil || e.Err == nil {
		return "persist billing runtime config"
	}
	return e.Err.Error()
}

func (e *BillingRuntimePersistError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsBillingRuntimePersistError(err error) bool {
	var target *BillingRuntimePersistError
	return errors.As(err, &target)
}

func billingRuntimeStateConfig(req model.BillingRuntimeConfigRequest) billingstate.Config {
	req = normalizeBillingRuntimeConfigRequest(req)
	return billingstate.Config{
		RedisURL:           req.RedisURL,
		Prefix:             req.RedisPrefix,
		ReservationTTL:     time.Duration(req.ReservationTTLSec) * time.Second,
		ReconcileInterval:  time.Duration(req.ReconcileIntervalSec) * time.Second,
		StreamBatchSize:    int64(req.StreamBatchSize),
		ReconcileBatchSize: int64(req.ReconcileBatchSize),
		ExpiryBatchSize:    int64(req.ExpiryBatchSize),
		ProjectorWorkers:   req.ProjectorWorkers,
	}
}

func (s *SystemConfigService) ApplyAndStoreBillingRuntimeConfig(req model.BillingRuntimeConfigRequest) error {
	req = normalizeBillingRuntimeConfigRequest(req)

	nextRuntime, err := buildBillingRuntime(billingRuntimeStateConfig(req))
	if err != nil {
		return err
	}

	previousRuntime := replaceBillingRuntime(nextRuntime)
	if err := persistBillingRuntimeConfig(s, req); err != nil {
		currentRuntime := replaceBillingRuntime(previousRuntime)
		closeBillingRuntime(currentRuntime)
		return &BillingRuntimePersistError{Err: fmt.Errorf("persist billing runtime config: %w", err)}
	}

	closeBillingRuntime(previousRuntime)
	return nil
}

func (s *SystemConfigService) ReloadBillingRuntimeFromSystemConfigBestEffort(reason string) {
	req, err := s.GetBillingRuntimeConfigRequest()
	if err != nil {
		previousRuntime := replaceBillingRuntime(nil)
		closeBillingRuntime(previousRuntime)
		log.Printf("warning: billing runtime reload skipped during %s: %v", reason, err)
		return
	}

	nextRuntime, err := buildBillingRuntime(billingRuntimeStateConfig(req))
	if err != nil {
		previousRuntime := replaceBillingRuntime(nil)
		closeBillingRuntime(previousRuntime)
		log.Printf("warning: billing runtime reload skipped during %s, falling back to legacy billing: %v", reason, err)
		return
	}

	previousRuntime := replaceBillingRuntime(nextRuntime)
	closeBillingRuntime(previousRuntime)
}
