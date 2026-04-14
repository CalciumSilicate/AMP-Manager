package billingstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

var (
	ErrInsufficientBudget  = errors.New("insufficient budget")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrRedisStateUnhealthy = errors.New("redis billing state unavailable")
	globalRuntime          atomic.Pointer[Runtime]
	globalRuntimeMu        sync.Mutex
)

const (
	defaultProjectorGroup     = "billing-projectors"
	defaultExpiryPoll         = 2 * time.Second
	defaultReconcileBatch     = 100
	defaultProjectorClaimIdle = 30 * time.Second
	defaultCloseTimeout       = 5 * time.Second
)

type Config struct {
	RedisURL           string
	Prefix             string
	ReservationTTL     time.Duration
	ReconcileInterval  time.Duration
	StreamBatchSize    int64
	ReconcileBatchSize int64
	ExpiryBatchSize    int64
	ProjectorWorkers   int
	ProjectorClaimIdle time.Duration
}

type Runtime struct {
	client             *redis.Client
	cfg                Config
	projectorConsumers []string
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	metrics            runtimeMetrics

	reconcileCursor string
}

type RuntimeMetricsSnapshot struct {
	ReserveDurations   []time.Duration
	SettleDurations    []time.Duration
	ProjectDurations   []time.Duration
	ReclaimDurations   []time.Duration
	ReconcileDurations []time.Duration
	ReserveFailures    int64
	SettleFailures     int64
	ProjectFailures    int64
	ReclaimFailures    int64
	ReclaimClaimed     int64
	ReconcileFailures  int64
	ReconcileRepairs   int64
}

type runtimeMetrics struct {
	mu                 sync.Mutex
	reserveDurations   []time.Duration
	settleDurations    []time.Duration
	projectDurations   []time.Duration
	reclaimDurations   []time.Duration
	reconcileDurations []time.Duration
	reserveFailures    int64
	settleFailures     int64
	projectFailures    int64
	reclaimFailures    int64
	reclaimClaimed     int64
	reconcileFailures  int64
	reconcileRepairs   int64
}

type WindowRef struct {
	StateID         string `json:"stateId"`
	SubscriptionID  string `json:"subscriptionId"`
	PlanID          string `json:"planId"`
	LimitType       string `json:"limitType"`
	WindowMode      string `json:"windowMode"`
	WindowStartUnix int64  `json:"windowStartUnix"`
	WindowEndUnix   int64  `json:"windowEndUnix"`
}

type SettleResult struct {
	Status                    string
	ChargedSubscriptionMicros int64
	ChargedBalanceMicros      int64
}

type hotAccountState struct {
	UserID                string
	PrimarySource         model.BillingSource
	SecondarySource       model.BillingSource
	BalanceMicros         int64
	ActiveSubscriptionID  string
	ActivePlanID          string
	SubscriptionStartsAt  time.Time
	SubscriptionExpiresAt *time.Time
	WindowRefs            []WindowRef
	WindowRefsJSON        string
	WindowKeyList         string
}

type accountStateRow struct {
	UserID                string
	PrimarySource         model.BillingSource
	SecondarySource       model.BillingSource
	BalanceMicros         int64
	ActiveSubscriptionID  sql.NullString
	ActivePlanID          sql.NullString
	SubscriptionStartsAt  sql.NullTime
	SubscriptionExpiresAt sql.NullTime
}

type windowStateRow struct {
	StateID            string
	UserSubscriptionID string
	PlanID             string
	LimitType          model.LimitType
	WindowMode         model.WindowMode
	WindowStart        time.Time
	WindowEnd          time.Time
	LimitMicros        int64
	UsedMicros         int64
	ReservedMicros     int64
	RemainingMicros    int64
}

type reservationRow struct {
	RequestID                  string
	UserID                     string
	UserSubscriptionID         string
	EstimatedCostMicros        int64
	ActualCostMicros           int64
	ReservedSubscriptionMicros int64
	ReservedBalanceMicros      int64
	ChargedSubscriptionMicros  int64
	ChargedBalanceMicros       int64
	Status                     string
	ExpiresAt                  time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	WindowRefsJSON             string
}

func Init(cfg Config) error {
	rt, err := Build(cfg)
	if err != nil {
		return err
	}

	previous := Replace(rt)
	if previous != nil {
		previous.Close()
	}

	if rt == nil {
		log.Info("billing state: redis runtime disabled")
		return nil
	}

	log.Infof("billing state: redis runtime initialized with prefix %s", rt.cfg.Prefix)
	return nil
}

func Get() *Runtime {
	return globalRuntime.Load()
}

func Close() {
	if rt := Replace(nil); rt != nil {
		rt.Close()
	}
}

func Build(cfg Config) (*Runtime, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.RedisURL) == "" {
		return nil, nil
	}

	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	host, _ := os.Hostname()
	rt := &Runtime{
		client:             client,
		cfg:                cfg,
		projectorConsumers: buildProjectorConsumerNames(host, os.Getpid(), uuid.NewString(), cfg.ProjectorWorkers),
		stopCh:             make(chan struct{}),
	}

	if err := rt.ensureConsumerGroup(context.Background()); err != nil {
		_ = client.Close()
		return nil, err
	}

	rt.wg.Add(cfg.ProjectorWorkers + 2)
	for _, consumerName := range rt.projectorConsumers {
		go rt.projectorLoop(consumerName)
	}
	go rt.expiryLoop()
	go rt.reconcileLoop()

	return rt, nil
}

func Replace(rt *Runtime) *Runtime {
	globalRuntimeMu.Lock()
	defer globalRuntimeMu.Unlock()

	previous := globalRuntime.Load()
	globalRuntime.Store(rt)
	return previous
}

func normalizeConfig(cfg Config) Config {
	cfg.RedisURL = strings.TrimSpace(cfg.RedisURL)
	cfg.Prefix = strings.TrimSpace(cfg.Prefix)
	if cfg.Prefix == "" {
		cfg.Prefix = "ampmanager"
	}
	if cfg.ReservationTTL <= 0 {
		cfg.ReservationTTL = 10 * time.Minute
	}
	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = time.Minute
	}
	if cfg.StreamBatchSize <= 0 {
		cfg.StreamBatchSize = 100
	}
	if cfg.ReconcileBatchSize <= 0 {
		cfg.ReconcileBatchSize = cfg.StreamBatchSize
	}
	if cfg.ExpiryBatchSize <= 0 {
		cfg.ExpiryBatchSize = cfg.StreamBatchSize
	}
	if cfg.ProjectorWorkers <= 0 {
		cfg.ProjectorWorkers = 1
	}
	if cfg.ProjectorClaimIdle <= 0 {
		cfg.ProjectorClaimIdle = defaultProjectorClaimIdle
	}
	return cfg
}

func buildProjectorConsumerNames(host string, pid int, instanceID string, workers int) []string {
	names := make([]string, 0, workers)
	base := fmt.Sprintf("%s-%d-%s", host, pid, instanceID)
	for idx := 0; idx < workers; idx++ {
		names = append(names, fmt.Sprintf("%s-projector-%d", base, idx+1))
	}
	return names
}

func (r *Runtime) Close() {
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}
	r.wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCloseTimeout)
	defer cancel()
	r.cleanupProjectorConsumers(ctx)
	_ = r.client.Close()
}

func (r *Runtime) ReserveRequest(ctx context.Context, requestID, userID string, estimatedCostMicros int64) error {
	start := time.Now()
	defer func() {
		r.metrics.recordReserve(time.Since(start))
	}()
	if requestID == "" || userID == "" {
		r.metrics.addReserveFailure()
		return ErrRedisStateUnhealthy
	}

	state, err := r.ensureHotState(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	windowRefsJSON := state.WindowRefsJSON
	windowKeyList := state.WindowKeyList

	result, err := reserveScript.Run(
		ctx,
		r.client,
		[]string{
			r.accountKey(userID),
			r.reservationKey(requestID),
			r.reservationExpiryKey(),
			r.streamKey(),
		},
		estimatedCostMicros,
		now.Add(r.cfg.ReservationTTL).Unix(),
		userID,
		state.ActiveSubscriptionID,
		windowKeyList,
		windowRefsJSON,
		now.Unix(),
		requestID,
	).Result()
	if err != nil {
		r.metrics.addReserveFailure()
		return err
	}

	values := stringifyRedisResult(result)
	if len(values) == 0 {
		return ErrRedisStateUnhealthy
	}

	switch values[0] {
	case "OK", "EXISTS":
		return nil
	case "MISS":
		state, err = r.hydrateHotState(ctx, userID)
		if err != nil {
			r.metrics.addReserveFailure()
			return err
		}
		result, err = reserveScript.Run(
			ctx,
			r.client,
			[]string{
				r.accountKey(userID),
				r.reservationKey(requestID),
				r.reservationExpiryKey(),
				r.streamKey(),
			},
			estimatedCostMicros,
			now.Add(r.cfg.ReservationTTL).Unix(),
			userID,
			state.ActiveSubscriptionID,
			state.WindowKeyList,
			state.WindowRefsJSON,
			now.Unix(),
			requestID,
		).Result()
		if err != nil {
			r.metrics.addReserveFailure()
			return err
		}
		values = stringifyRedisResult(result)
		if len(values) > 0 && values[0] == "OK" {
			return nil
		}
		if len(values) > 0 && values[0] == "REJECT" {
			return ErrInsufficientBudget
		}
		r.metrics.addReserveFailure()
		return ErrRedisStateUnhealthy
	case "REJECT":
		return ErrInsufficientBudget
	default:
		r.metrics.addReserveFailure()
		return ErrRedisStateUnhealthy
	}
}

func (r *Runtime) SettleRequest(ctx context.Context, requestID, userID string, actualCostMicros int64) (*SettleResult, error) {
	start := time.Now()
	defer func() {
		r.metrics.recordSettle(time.Since(start))
	}()
	if requestID == "" || userID == "" {
		r.metrics.addSettleFailure()
		return nil, ErrReservationNotFound
	}

	now := time.Now().UTC()
	values, err := r.runSettleScript(ctx, requestID, userID, actualCostMicros, now.Unix())
	if err != nil {
		r.metrics.addSettleFailure()
		return nil, err
	}

	if len(values) > 0 && values[0] == "NOT_FOUND" {
		if err := r.ensureHotReservation(ctx, requestID, userID); err != nil {
			r.metrics.addSettleFailure()
			return nil, err
		}

		values, err = r.runSettleScript(ctx, requestID, userID, actualCostMicros, now.Unix())
		if err != nil {
			r.metrics.addSettleFailure()
			return nil, err
		}
	}

	switch values[0] {
	case "OK":
		return &SettleResult{
			Status:                    values[1],
			ChargedSubscriptionMicros: parseInt64(values[2]),
			ChargedBalanceMicros:      parseInt64(values[3]),
		}, nil
	case "DONE":
		return &SettleResult{
			Status: values[1],
		}, nil
	case "NOT_FOUND":
		r.metrics.addSettleFailure()
		return nil, ErrReservationNotFound
	default:
		r.metrics.addSettleFailure()
		return nil, ErrRedisStateUnhealthy
	}
}

func (r *Runtime) RefreshUserState(ctx context.Context, userID string) error {
	_, err := r.hydrateHotState(ctx, userID)
	return err
}

func (r *Runtime) ApplyBalanceDelta(ctx context.Context, userID string, deltaMicros int64) error {
	if userID == "" || deltaMicros == 0 {
		return nil
	}

	now := time.Now().UTC()
	_, err := database.GetDB().Exec(
		`INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision, created_at, updated_at)
		 VALUES (?, 'subscription', 'balance', ?, 0, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET balance_micros = billing_account_state.balance_micros + excluded.balance_micros, updated_at = excluded.updated_at`,
		userID, deltaMicros, now, now,
	)
	if err != nil {
		return err
	}

	if err := r.client.HIncrBy(ctx, r.accountKey(userID), "balance_micros", deltaMicros).Err(); err != nil && err != redis.Nil {
		return err
	}
	return nil
}

func (r *Runtime) ensureConsumerGroup(ctx context.Context) error {
	err := r.client.XGroupCreateMkStream(ctx, r.streamKey(), defaultProjectorGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (r *Runtime) projectorLoop(consumerName string) {
	defer r.wg.Done()

	ctx := context.Background()
	nextSweep := time.Now().Add(r.projectorStaleConsumerSweepInterval())
	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		reclaimed, err := r.reclaimPendingEntries(ctx, consumerName)
		if err != nil {
			log.Warnf("billing state: projector reclaim failed for %s: %v", consumerName, err)
			time.Sleep(time.Second)
			continue
		}
		if reclaimed > 0 {
			continue
		}
		if r.shouldSweepStaleProjectorConsumers(consumerName) && time.Now().After(nextSweep) {
			if err := r.cleanupStaleProjectorConsumers(ctx); err != nil {
				log.Warnf("billing state: stale projector consumer cleanup failed: %v", err)
			}
			nextSweep = time.Now().Add(r.projectorStaleConsumerSweepInterval())
		}

		streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    defaultProjectorGroup,
			Consumer: consumerName,
			Streams:  []string{r.streamKey(), ">"},
			Count:    r.cfg.StreamBatchSize,
			Block:    time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			log.Warnf("billing state: projector read failed: %v", err)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			ackedIDs := r.projectMessages(ctx, stream.Messages)
			if len(ackedIDs) == 0 {
				continue
			}
			if _, err := r.client.XAck(ctx, r.streamKey(), defaultProjectorGroup, ackedIDs...).Result(); err != nil {
				log.Warnf("billing state: projector ack failed for %d messages: %v", len(ackedIDs), err)
			}
		}
	}
}

func (r *Runtime) reclaimPendingEntries(ctx context.Context, consumerName string) (int, error) {
	start := time.Now()
	messages, _, err := r.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   r.streamKey(),
		Group:    defaultProjectorGroup,
		Consumer: consumerName,
		MinIdle:  r.cfg.ProjectorClaimIdle,
		Start:    "0-0",
		Count:    r.cfg.StreamBatchSize,
	}).Result()
	if err != nil {
		if err == redis.Nil || isProjectorGroupUnavailable(err) {
			return 0, nil
		}
		r.metrics.addReclaimFailure()
		return 0, err
	}
	if len(messages) == 0 {
		return 0, nil
	}

	ackedIDs := r.projectMessages(ctx, messages)
	if len(ackedIDs) > 0 {
		if _, err := r.client.XAck(ctx, r.streamKey(), defaultProjectorGroup, ackedIDs...).Result(); err != nil && !isProjectorGroupUnavailable(err) {
			r.metrics.addReclaimFailure()
			log.Warnf("billing state: projector ack failed for %d reclaimed messages: %v", len(ackedIDs), err)
		}
	}
	r.metrics.recordReclaim(time.Since(start), int64(len(messages)))
	return len(messages), nil
}

func (r *Runtime) projectorStaleConsumerSweepInterval() time.Duration {
	return r.cfg.ProjectorClaimIdle
}

func (r *Runtime) shouldSweepStaleProjectorConsumers(consumerName string) bool {
	if len(r.projectorConsumers) == 0 {
		return true
	}
	return consumerName == r.projectorConsumers[0]
}

func (r *Runtime) cleanupStaleProjectorConsumers(ctx context.Context) error {
	consumers, err := r.client.XInfoConsumers(ctx, r.streamKey(), defaultProjectorGroup).Result()
	if err != nil {
		if err == redis.Nil || isProjectorGroupUnavailable(err) {
			return nil
		}
		return err
	}

	for _, consumerName := range r.staleProjectorConsumerNames(consumers) {
		if _, err := r.client.XGroupDelConsumer(ctx, r.streamKey(), defaultProjectorGroup, consumerName).Result(); err != nil {
			if err == redis.Nil || isProjectorGroupUnavailable(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func (r *Runtime) staleProjectorConsumerNames(consumers []redis.XInfoConsumer) []string {
	sweepThreshold := r.cfg.ProjectorClaimIdle
	owned := make(map[string]struct{}, len(r.projectorConsumers))
	for _, consumerName := range r.projectorConsumers {
		owned[consumerName] = struct{}{}
	}

	names := make([]string, 0, len(consumers))
	for _, consumer := range consumers {
		if _, ok := owned[consumer.Name]; ok {
			continue
		}
		if consumer.Pending > 0 || consumer.Idle < 0 || consumer.Idle <= sweepThreshold {
			continue
		}
		names = append(names, consumer.Name)
	}
	return names
}

func (r *Runtime) cleanupProjectorConsumers(ctx context.Context) {
	for _, consumerName := range r.projectorConsumers {
		if err := r.cleanupProjectorConsumer(ctx, consumerName); err != nil {
			log.Warnf("billing state: projector consumer cleanup failed for %s: %v", consumerName, err)
		}
	}
}

func (r *Runtime) cleanupProjectorConsumer(ctx context.Context, consumerName string) error {
	pending, err := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream:   r.streamKey(),
		Group:    defaultProjectorGroup,
		Start:    "-",
		End:      "+",
		Count:    1,
		Consumer: consumerName,
	}).Result()
	if err != nil {
		if err == redis.Nil || isProjectorGroupUnavailable(err) {
			return nil
		}
		return err
	}
	if len(pending) > 0 {
		return nil
	}

	if _, err := r.client.XGroupDelConsumer(ctx, r.streamKey(), defaultProjectorGroup, consumerName).Result(); err != nil {
		if err == redis.Nil || isProjectorGroupUnavailable(err) {
			return nil
		}
		return err
	}
	return nil
}

func isProjectorGroupUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NOGROUP") ||
		strings.Contains(msg, "no such key") ||
		strings.Contains(msg, "requires the key to exist")
}

func (r *Runtime) reconcileLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.ReconcileInterval)
	defer ticker.Stop()

	ctx := context.Background()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			start := time.Now()
			repairs, err := r.reconcileNextPage(ctx)
			r.metrics.recordReconcile(time.Since(start), repairs)
			if err != nil {
				r.metrics.addReconcileFailure()
				log.Warnf("billing state: reconcile failed: %v", err)
			}
		}
	}
}

func (r *Runtime) expiryLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(defaultExpiryPoll)
	defer ticker.Stop()

	ctx := context.Background()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			now := time.Now().Unix()
			if _, err := r.releaseExpiredReservations(ctx, now); err != nil && err != redis.Nil {
				log.Warnf("billing state: expiry scan failed: %v", err)
			}
		}
	}
}

func (r *Runtime) releaseExpiredReservations(ctx context.Context, nowUnix int64) (int, error) {
	batchSize := r.expiryBatchSize()
	total := 0
	offset := int64(0)

	for {
		requestIDs, err := r.listExpiredReservationsPage(ctx, nowUnix, offset, batchSize)
		if err != nil {
			return total, err
		}
		if len(requestIDs) == 0 {
			return total, nil
		}

		processed := r.releaseExpiredReservationsPage(ctx, requestIDs, nowUnix)
		total += processed
		if processed > 0 {
			offset = 0
			continue
		}
		if int64(len(requestIDs)) < batchSize {
			return total, nil
		}
		offset += int64(len(requestIDs))
	}
}

func (r *Runtime) releaseExpiredReservationsBatch(ctx context.Context, nowUnix int64) (int, error) {
	requestIDs, err := r.listExpiredReservationsPage(ctx, nowUnix, 0, r.expiryBatchSize())
	if err != nil {
		return 0, err
	}
	return r.releaseExpiredReservationsPage(ctx, requestIDs, nowUnix), nil
}

func (r *Runtime) listExpiredReservationsPage(ctx context.Context, nowUnix, offset, count int64) ([]string, error) {
	return r.client.ZRangeByScore(ctx, r.reservationExpiryKey(), &redis.ZRangeBy{
		Min:    "0",
		Max:    fmt.Sprintf("%d", nowUnix),
		Offset: offset,
		Count:  count,
	}).Result()
}

func (r *Runtime) releaseExpiredReservationsPage(ctx context.Context, requestIDs []string, nowUnix int64) int {
	processed := 0
	for _, requestID := range requestIDs {
		if err := r.releaseExpiredReservation(ctx, requestID, nowUnix); err != nil {
			log.Warnf("billing state: expiry release failed for %s: %v", requestID, err)
			continue
		}
		processed++
	}
	return processed
}

func (r *Runtime) releaseExpiredReservation(ctx context.Context, requestID string, nowUnix int64) error {
	userID, err := r.client.HGet(ctx, r.reservationKey(requestID), "user_id").Result()
	if err != nil {
		if err == redis.Nil {
			_, _ = r.client.ZRem(ctx, r.reservationExpiryKey(), requestID).Result()
			return nil
		}
		return err
	}

	result, err := expireScript.Run(
		ctx,
		r.client,
		[]string{
			r.accountKey(userID),
			r.reservationKey(requestID),
			r.reservationExpiryKey(),
			r.streamKey(),
		},
		nowUnix,
	).Result()
	if err != nil {
		return err
	}

	values := stringifyRedisResult(result)
	if len(values) > 0 && values[0] == "OK" {
		return nil
	}
	if len(values) > 0 && (values[0] == "DONE" || values[0] == "NOT_FOUND") {
		_, _ = r.client.ZRem(ctx, r.reservationExpiryKey(), requestID).Result()
		return nil
	}
	return ErrRedisStateUnhealthy
}

func (r *Runtime) runSettleScript(ctx context.Context, requestID, userID string, actualCostMicros, nowUnix int64) ([]string, error) {
	result, err := settleScript.Run(
		ctx,
		r.client,
		[]string{
			r.accountKey(userID),
			r.reservationKey(requestID),
			r.reservationExpiryKey(),
			r.streamKey(),
		},
		actualCostMicros,
		nowUnix,
	).Result()
	if err != nil {
		return nil, err
	}

	values := stringifyRedisResult(result)
	if len(values) == 0 {
		return nil, ErrRedisStateUnhealthy
	}
	return values, nil
}

func (r *Runtime) ensureHotReservation(ctx context.Context, requestID, userID string) error {
	if requestID == "" || userID == "" {
		return ErrReservationNotFound
	}
	exists, err := r.client.Exists(ctx, r.reservationKey(requestID)).Result()
	if err == nil && exists > 0 {
		return nil
	}

	row, err := r.getReservationRow(requestID)
	if err != nil {
		return err
	}
	if row == nil || row.Status != "reserved" {
		return ErrReservationNotFound
	}

	state, err := r.ensureHotState(ctx, userID)
	if err != nil {
		return err
	}

	windowRefsJSON := state.WindowRefsJSON
	if row.WindowRefsJSON != "" {
		windowRefsJSON = row.WindowRefsJSON
	}
	if err := r.client.HSet(ctx, r.reservationKey(requestID), map[string]any{
		"request_id":                   row.RequestID,
		"user_id":                      row.UserID,
		"user_subscription_id":         row.UserSubscriptionID,
		"status":                       row.Status,
		"estimated_cost_micros":        row.EstimatedCostMicros,
		"actual_cost_micros":           row.ActualCostMicros,
		"reserved_subscription_micros": row.ReservedSubscriptionMicros,
		"reserved_balance_micros":      row.ReservedBalanceMicros,
		"charged_subscription_micros":  row.ChargedSubscriptionMicros,
		"charged_balance_micros":       row.ChargedBalanceMicros,
		"window_key_list":              state.WindowKeyList,
		"window_refs_json":             windowRefsJSON,
		"expires_at_unix":              row.ExpiresAt.Unix(),
		"created_at_unix":              row.CreatedAt.Unix(),
		"updated_at_unix":              row.UpdatedAt.Unix(),
	}).Err(); err != nil {
		return err
	}
	if err := r.client.ZAdd(ctx, r.reservationExpiryKey(), redis.Z{
		Score:  float64(row.ExpiresAt.Unix()),
		Member: row.RequestID,
	}).Err(); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) reconcileAll(ctx context.Context) (int64, error) {
	userIDs, err := r.listAccountStateUserIDs()
	if err != nil {
		return 0, err
	}

	var repairs int64
	for _, userID := range userIDs {
		fixed, err := r.reconcileUserState(ctx, userID)
		if err != nil {
			return repairs, err
		}
		repairs += fixed
	}
	return repairs, nil
}

func (r *Runtime) reconcileNextPage(ctx context.Context) (int64, error) {
	batchSize := r.reconcileBatchSize()
	userIDs, err := r.listAccountStateUserIDsPage(r.reconcileCursor, batchSize)
	if err != nil {
		return 0, err
	}
	if len(userIDs) == 0 {
		r.reconcileCursor = ""
		return 0, nil
	}

	var repairs int64
	for _, userID := range userIDs {
		fixed, err := r.reconcileUserState(ctx, userID)
		if err != nil {
			return repairs, err
		}
		repairs += fixed
	}

	if int64(len(userIDs)) < batchSize {
		r.reconcileCursor = ""
	} else {
		r.reconcileCursor = userIDs[len(userIDs)-1]
	}
	return repairs, nil
}

func (r *Runtime) reconcileUserState(ctx context.Context, userID string) (int64, error) {
	accountRow, err := r.getAccountStateRow(userID)
	if err != nil {
		return 0, err
	}
	if accountRow == nil {
		return 0, nil
	}

	state := &hotAccountState{
		UserID:          accountRow.UserID,
		PrimarySource:   accountRow.PrimarySource,
		SecondarySource: accountRow.SecondarySource,
		BalanceMicros:   accountRow.BalanceMicros,
	}
	if accountRow.ActiveSubscriptionID.Valid {
		state.ActiveSubscriptionID = accountRow.ActiveSubscriptionID.String
	}
	if accountRow.ActivePlanID.Valid {
		state.ActivePlanID = accountRow.ActivePlanID.String
	}
	if accountRow.SubscriptionStartsAt.Valid {
		state.SubscriptionStartsAt = accountRow.SubscriptionStartsAt.Time.UTC()
	}
	if accountRow.SubscriptionExpiresAt.Valid {
		exp := accountRow.SubscriptionExpiresAt.Time.UTC()
		state.SubscriptionExpiresAt = &exp
	}

	var windowRows []windowStateRow
	if state.ActiveSubscriptionID != "" {
		windowRows, err = r.listWindowStates(state.ActiveSubscriptionID)
		if err != nil {
			return 0, err
		}
		for _, row := range windowRows {
			state.WindowRefs = append(state.WindowRefs, WindowRef{
				StateID:         row.StateID,
				SubscriptionID:  row.UserSubscriptionID,
				PlanID:          row.PlanID,
				LimitType:       string(row.LimitType),
				WindowMode:      string(row.WindowMode),
				WindowStartUnix: row.WindowStart.Unix(),
				WindowEndUnix:   row.WindowEnd.Unix(),
			})
		}
	}

	var repairs int64
	windowKeys := make([]string, 0, len(state.WindowRefs))
	for _, ref := range state.WindowRefs {
		windowKeys = append(windowKeys, r.windowKey(ref.StateID))
	}
	state.WindowKeyList = strings.Join(windowKeys, "|")
	if len(state.WindowRefs) > 0 {
		encoded, _ := json.Marshal(state.WindowRefs)
		state.WindowRefsJSON = string(encoded)
	}

	accountRepair, err := r.upsertRedisAccountState(ctx, state)
	if err != nil {
		return repairs, err
	}
	repairs += accountRepair

	for _, row := range windowRows {
		windowRepair, err := r.upsertRedisWindowState(ctx, &row)
		if err != nil {
			return repairs, err
		}
		repairs += windowRepair
	}

	reservations, err := r.listReservedReservationsForUser(userID)
	if err != nil {
		return repairs, err
	}
	for _, row := range reservations {
		fixed, err := r.upsertRedisReservation(ctx, row, state)
		if err != nil {
			return repairs, err
		}
		repairs += fixed
	}

	return repairs, nil
}

func (r *Runtime) upsertRedisAccountState(ctx context.Context, state *hotAccountState) (int64, error) {
	fields, err := r.client.HGetAll(ctx, r.accountKey(state.UserID)).Result()
	if err != nil {
		return 0, err
	}

	target := map[string]any{
		"user_id":                  state.UserID,
		"primary_source":           string(state.PrimarySource),
		"secondary_source":         string(state.SecondarySource),
		"balance_micros":           state.BalanceMicros,
		"active_subscription_id":   state.ActiveSubscriptionID,
		"active_plan_id":           state.ActivePlanID,
		"current_window_refs_json": state.WindowRefsJSON,
		"current_window_key_list":  state.WindowKeyList,
	}
	if !state.SubscriptionStartsAt.IsZero() {
		target["subscription_starts_at_unix"] = state.SubscriptionStartsAt.Unix()
	}
	if state.SubscriptionExpiresAt != nil {
		target["subscription_expires_at_unix"] = state.SubscriptionExpiresAt.UTC().Unix()
	}

	if redisHashMatches(fields, target) {
		return 0, nil
	}
	if err := r.client.HSet(ctx, r.accountKey(state.UserID), target).Err(); err != nil {
		return 0, err
	}
	return 1, nil
}

func (r *Runtime) upsertRedisWindowState(ctx context.Context, row *windowStateRow) (int64, error) {
	fields, err := r.client.HGetAll(ctx, r.windowKey(row.StateID)).Result()
	if err != nil {
		return 0, err
	}
	target := map[string]any{
		"state_id":             row.StateID,
		"user_subscription_id": row.UserSubscriptionID,
		"plan_id":              row.PlanID,
		"limit_type":           string(row.LimitType),
		"window_mode":          string(row.WindowMode),
		"window_start_unix":    row.WindowStart.Unix(),
		"window_end_unix":      row.WindowEnd.Unix(),
		"limit_micros":         row.LimitMicros,
		"used_micros":          row.UsedMicros,
		"reserved_micros":      row.ReservedMicros,
		"remaining_micros":     row.RemainingMicros,
	}
	if redisHashMatches(fields, target) {
		return 0, nil
	}
	if err := r.client.HSet(ctx, r.windowKey(row.StateID), target).Err(); err != nil {
		return 0, err
	}
	return 1, nil
}

func (r *Runtime) upsertRedisReservation(ctx context.Context, row reservationRow, state *hotAccountState) (int64, error) {
	fields, err := r.client.HGetAll(ctx, r.reservationKey(row.RequestID)).Result()
	if err != nil {
		return 0, err
	}
	windowRefsJSON := row.WindowRefsJSON
	if windowRefsJSON == "" {
		windowRefsJSON = state.WindowRefsJSON
	}
	target := map[string]any{
		"request_id":                   row.RequestID,
		"user_id":                      row.UserID,
		"user_subscription_id":         row.UserSubscriptionID,
		"status":                       row.Status,
		"estimated_cost_micros":        row.EstimatedCostMicros,
		"actual_cost_micros":           row.ActualCostMicros,
		"reserved_subscription_micros": row.ReservedSubscriptionMicros,
		"reserved_balance_micros":      row.ReservedBalanceMicros,
		"charged_subscription_micros":  row.ChargedSubscriptionMicros,
		"charged_balance_micros":       row.ChargedBalanceMicros,
		"window_key_list":              state.WindowKeyList,
		"window_refs_json":             windowRefsJSON,
		"expires_at_unix":              row.ExpiresAt.Unix(),
		"created_at_unix":              row.CreatedAt.Unix(),
		"updated_at_unix":              row.UpdatedAt.Unix(),
	}
	var repairs int64
	if !redisHashMatches(fields, target) {
		if err := r.client.HSet(ctx, r.reservationKey(row.RequestID), target).Err(); err != nil {
			return repairs, err
		}
		repairs++
	}
	score, err := r.client.ZScore(ctx, r.reservationExpiryKey(), row.RequestID).Result()
	if err == redis.Nil || int64(score) != row.ExpiresAt.Unix() {
		if err := r.client.ZAdd(ctx, r.reservationExpiryKey(), redis.Z{
			Score:  float64(row.ExpiresAt.Unix()),
			Member: row.RequestID,
		}).Err(); err != nil {
			return repairs, err
		}
		repairs++
	} else if err != nil {
		return repairs, err
	}
	return repairs, nil
}

func (r *Runtime) SnapshotMetrics(reset bool) RuntimeMetricsSnapshot {
	return r.metrics.snapshot(reset)
}

func (r *Runtime) ensureHotState(ctx context.Context, userID string) (*hotAccountState, error) {
	fields, err := r.client.HGetAll(ctx, r.accountKey(userID)).Result()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return r.hydrateHotState(ctx, userID)
	}

	state := &hotAccountState{
		UserID:               userID,
		PrimarySource:        model.BillingSource(fields["primary_source"]),
		SecondarySource:      model.BillingSource(fields["secondary_source"]),
		BalanceMicros:        parseInt64(fields["balance_micros"]),
		ActiveSubscriptionID: fields["active_subscription_id"],
		ActivePlanID:         fields["active_plan_id"],
		WindowRefsJSON:       fields["current_window_refs_json"],
		WindowKeyList:        fields["current_window_key_list"],
	}
	if state.PrimarySource == "" {
		return r.hydrateHotState(ctx, userID)
	}
	if state.WindowRefsJSON != "" {
		_ = json.Unmarshal([]byte(state.WindowRefsJSON), &state.WindowRefs)
	}
	return state, nil
}

func (r *Runtime) hydrateHotState(ctx context.Context, userID string) (*hotAccountState, error) {
	now := time.Now().UTC()
	settingRepo := repository.NewBillingSettingRepository()
	subRepo := repository.NewUserSubscriptionRepository()
	planRepo := repository.NewSubscriptionPlanRepository()
	userRepo := repository.NewUserRepository()

	setting, err := settingRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	sub, err := subRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}

	stateRow, err := r.getAccountStateRow(userID)
	if err != nil {
		return nil, err
	}

	balanceMicros := int64(0)
	if stateRow != nil {
		balanceMicros = stateRow.BalanceMicros
	} else {
		balanceMicros, err = userRepo.GetBalance(userID)
		if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
			return nil, err
		}
	}

	state := &hotAccountState{
		UserID:          userID,
		PrimarySource:   setting.PrimarySource,
		SecondarySource: setting.SecondarySource,
		BalanceMicros:   balanceMicros,
	}

	if sub != nil {
		state.ActiveSubscriptionID = sub.ID
		state.SubscriptionStartsAt = sub.StartsAt.UTC()
		state.SubscriptionExpiresAt = sub.ExpiresAt
		plan, limits, err := planRepo.GetByID(sub.PlanID)
		if err != nil {
			return nil, err
		}
		if plan != nil {
			state.ActivePlanID = plan.ID
		}
		for _, limit := range limits {
			windowStart, windowEnd, err := getWindowBounds(limit.LimitType, limit.WindowMode, now, sub.StartsAt)
			if err != nil {
				return nil, err
			}
			row, err := r.ensureWindowState(sub, &limit, windowStart, windowEnd)
			if err != nil {
				return nil, err
			}
			state.WindowRefs = append(state.WindowRefs, WindowRef{
				StateID:         row.StateID,
				SubscriptionID:  row.UserSubscriptionID,
				PlanID:          row.PlanID,
				LimitType:       string(row.LimitType),
				WindowMode:      string(row.WindowMode),
				WindowStartUnix: row.WindowStart.Unix(),
				WindowEndUnix:   row.WindowEnd.Unix(),
			})
		}
	}

	if len(state.WindowRefs) > 0 {
		windowKeys := make([]string, 0, len(state.WindowRefs))
		for _, ref := range state.WindowRefs {
			windowKeys = append(windowKeys, r.windowKey(ref.StateID))
		}
		state.WindowKeyList = strings.Join(windowKeys, "|")
		encoded, _ := json.Marshal(state.WindowRefs)
		state.WindowRefsJSON = string(encoded)
	}

	if err := r.upsertAccountState(state); err != nil {
		return nil, err
	}

	pipe := r.client.Pipeline()
	accountFields := map[string]any{
		"user_id":                  state.UserID,
		"primary_source":           string(state.PrimarySource),
		"secondary_source":         string(state.SecondarySource),
		"balance_micros":           state.BalanceMicros,
		"active_subscription_id":   state.ActiveSubscriptionID,
		"active_plan_id":           state.ActivePlanID,
		"current_window_refs_json": state.WindowRefsJSON,
		"current_window_key_list":  state.WindowKeyList,
	}
	if !state.SubscriptionStartsAt.IsZero() {
		accountFields["subscription_starts_at_unix"] = state.SubscriptionStartsAt.Unix()
	}
	if state.SubscriptionExpiresAt != nil {
		accountFields["subscription_expires_at_unix"] = state.SubscriptionExpiresAt.UTC().Unix()
	}
	pipe.HSet(ctx, r.accountKey(userID), accountFields)

	for _, ref := range state.WindowRefs {
		row, err := r.getWindowState(ref.StateID)
		if err != nil {
			return nil, err
		}
		if row == nil {
			continue
		}
		pipe.HSet(ctx, r.windowKey(ref.StateID), map[string]any{
			"state_id":             row.StateID,
			"user_subscription_id": row.UserSubscriptionID,
			"plan_id":              row.PlanID,
			"limit_type":           string(row.LimitType),
			"window_mode":          string(row.WindowMode),
			"window_start_unix":    row.WindowStart.Unix(),
			"window_end_unix":      row.WindowEnd.Unix(),
			"limit_micros":         row.LimitMicros,
			"used_micros":          row.UsedMicros,
			"reserved_micros":      row.ReservedMicros,
			"remaining_micros":     row.RemainingMicros,
		})
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	return state, nil
}

func (r *Runtime) projectEvent(ctx context.Context, msg redis.XMessage) error {
	db := database.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	applied, err := ensureProjectionEvent(tx, msg.ID, stringField(msg.Values["event_type"]), stringField(msg.Values["request_id"]))
	if err != nil {
		return err
	}
	if !applied {
		return tx.Commit()
	}

	switch stringField(msg.Values["event_type"]) {
	case "reserve":
		err = applyReserveEvent(tx, msg.Values)
	case "settle":
		err = applySettleEvent(tx, msg.Values)
	case "expire":
		err = applyExpireEvent(tx, msg.Values)
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Runtime) getAccountStateRow(userID string) (*accountStateRow, error) {
	row := &accountStateRow{}
	err := database.GetDB().QueryRow(
		`SELECT user_id, primary_source, secondary_source, balance_micros, active_subscription_id, active_plan_id, subscription_starts_at, subscription_expires_at
		 FROM billing_account_state WHERE user_id = ?`,
		userID,
	).Scan(
		&row.UserID,
		&row.PrimarySource,
		&row.SecondarySource,
		&row.BalanceMicros,
		&row.ActiveSubscriptionID,
		&row.ActivePlanID,
		&row.SubscriptionStartsAt,
		&row.SubscriptionExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *Runtime) upsertAccountState(state *hotAccountState) error {
	now := time.Now().UTC()
	var startsAt any
	var expiresAt any
	if !state.SubscriptionStartsAt.IsZero() {
		startsAt = state.SubscriptionStartsAt.UTC()
	}
	if state.SubscriptionExpiresAt != nil {
		expiresAt = state.SubscriptionExpiresAt.UTC()
	}
	_, err := database.GetDB().Exec(
		`INSERT INTO billing_account_state (
			user_id, primary_source, secondary_source, balance_micros,
			active_subscription_id, active_plan_id, subscription_starts_at, subscription_expires_at,
			revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			primary_source = excluded.primary_source,
			secondary_source = excluded.secondary_source,
			balance_micros = excluded.balance_micros,
			active_subscription_id = excluded.active_subscription_id,
			active_plan_id = excluded.active_plan_id,
			subscription_starts_at = excluded.subscription_starts_at,
			subscription_expires_at = excluded.subscription_expires_at,
			updated_at = excluded.updated_at`,
		state.UserID,
		state.PrimarySource,
		state.SecondarySource,
		state.BalanceMicros,
		nullIfEmpty(state.ActiveSubscriptionID),
		nullIfEmpty(state.ActivePlanID),
		startsAt,
		expiresAt,
		now,
		now,
	)
	return err
}

func (r *Runtime) ensureWindowState(sub *model.UserSubscription, limit *model.SubscriptionPlanLimit, windowStart, windowEnd time.Time) (*windowStateRow, error) {
	stateID := windowStateID(sub.ID, limit.LimitType, windowStart)
	if existing, err := r.getWindowState(stateID); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	used, err := repository.NewBillingEventRepository().GetUsageInWindow(sub.ID, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	if used < 0 {
		used = 0
	}
	remaining := limit.LimitMicros - used
	if remaining < 0 {
		remaining = 0
	}

	row := &windowStateRow{
		StateID:            stateID,
		UserSubscriptionID: sub.ID,
		PlanID:             sub.PlanID,
		LimitType:          limit.LimitType,
		WindowMode:         limit.WindowMode,
		WindowStart:        windowStart.UTC(),
		WindowEnd:          windowEnd.UTC(),
		LimitMicros:        limit.LimitMicros,
		UsedMicros:         used,
		ReservedMicros:     0,
		RemainingMicros:    remaining,
	}

	now := time.Now().UTC()
	_, err = database.GetDB().Exec(
		`INSERT INTO subscription_window_state (
			id, user_subscription_id, plan_id, limit_type, window_mode, window_start, window_end,
			limit_micros, used_micros, reserved_micros, remaining_micros, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(user_subscription_id, limit_type, window_start) DO NOTHING`,
		row.StateID, row.UserSubscriptionID, row.PlanID, row.LimitType, row.WindowMode, row.WindowStart, row.WindowEnd,
		row.LimitMicros, row.UsedMicros, row.ReservedMicros, row.RemainingMicros, now, now,
	)
	if err != nil {
		return nil, err
	}

	return r.getWindowState(stateID)
}

func (r *Runtime) getWindowState(stateID string) (*windowStateRow, error) {
	row := &windowStateRow{}
	err := database.GetDB().QueryRow(
		`SELECT id, user_subscription_id, plan_id, limit_type, window_mode, window_start, window_end, limit_micros, used_micros, reserved_micros, remaining_micros
		 FROM subscription_window_state WHERE id = ?`,
		stateID,
	).Scan(
		&row.StateID,
		&row.UserSubscriptionID,
		&row.PlanID,
		&row.LimitType,
		&row.WindowMode,
		&row.WindowStart,
		&row.WindowEnd,
		&row.LimitMicros,
		&row.UsedMicros,
		&row.ReservedMicros,
		&row.RemainingMicros,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *Runtime) listWindowStates(subscriptionID string) ([]windowStateRow, error) {
	rows, err := database.GetDB().Query(
		`SELECT id, user_subscription_id, plan_id, limit_type, window_mode, window_start, window_end, limit_micros, used_micros, reserved_micros, remaining_micros
		 FROM subscription_window_state
		 WHERE user_subscription_id = ?
		 ORDER BY window_start, limit_type`,
		subscriptionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []windowStateRow
	for rows.Next() {
		var row windowStateRow
		if err := rows.Scan(
			&row.StateID,
			&row.UserSubscriptionID,
			&row.PlanID,
			&row.LimitType,
			&row.WindowMode,
			&row.WindowStart,
			&row.WindowEnd,
			&row.LimitMicros,
			&row.UsedMicros,
			&row.ReservedMicros,
			&row.RemainingMicros,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *Runtime) getReservationRow(requestID string) (*reservationRow, error) {
	row := &reservationRow{}
	var subscriptionID sql.NullString
	var windowRefsJSON sql.NullString
	err := database.GetDB().QueryRow(
		`SELECT request_id, user_id, user_subscription_id, estimated_cost_micros, actual_cost_micros,
		        reserved_subscription_micros, reserved_balance_micros, charged_subscription_micros, charged_balance_micros,
		        status, window_refs_json, expires_at, created_at, updated_at
		 FROM billing_reservations
		 WHERE request_id = ?`,
		requestID,
	).Scan(
		&row.RequestID,
		&row.UserID,
		&subscriptionID,
		&row.EstimatedCostMicros,
		&row.ActualCostMicros,
		&row.ReservedSubscriptionMicros,
		&row.ReservedBalanceMicros,
		&row.ChargedSubscriptionMicros,
		&row.ChargedBalanceMicros,
		&row.Status,
		&windowRefsJSON,
		&row.ExpiresAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.UserSubscriptionID = subscriptionID.String
	row.WindowRefsJSON = windowRefsJSON.String
	return row, nil
}

func (r *Runtime) listReservedReservationsForUser(userID string) ([]reservationRow, error) {
	rows, err := database.GetDB().Query(
		`SELECT request_id, user_id, user_subscription_id, estimated_cost_micros, actual_cost_micros,
		        reserved_subscription_micros, reserved_balance_micros, charged_subscription_micros, charged_balance_micros,
		        status, window_refs_json, expires_at, created_at, updated_at
		 FROM billing_reservations
		 WHERE user_id = ? AND status = 'reserved'
		 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []reservationRow
	for rows.Next() {
		var row reservationRow
		var subscriptionID sql.NullString
		var windowRefsJSON sql.NullString
		if err := rows.Scan(
			&row.RequestID,
			&row.UserID,
			&subscriptionID,
			&row.EstimatedCostMicros,
			&row.ActualCostMicros,
			&row.ReservedSubscriptionMicros,
			&row.ReservedBalanceMicros,
			&row.ChargedSubscriptionMicros,
			&row.ChargedBalanceMicros,
			&row.Status,
			&windowRefsJSON,
			&row.ExpiresAt,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.UserSubscriptionID = subscriptionID.String
		row.WindowRefsJSON = windowRefsJSON.String
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *Runtime) listAccountStateUserIDs() ([]string, error) {
	rows, err := database.GetDB().Query(`SELECT user_id FROM billing_account_state ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

func (r *Runtime) listAccountStateUserIDsPage(afterUserID string, limit int64) ([]string, error) {
	if limit <= 0 {
		limit = defaultReconcileBatch
	}

	var (
		rows *sql.Rows
		err  error
	)
	if afterUserID == "" {
		rows, err = database.GetDB().Query(`SELECT user_id FROM billing_account_state ORDER BY user_id LIMIT ?`, limit)
	} else {
		rows, err = database.GetDB().Query(`SELECT user_id FROM billing_account_state WHERE user_id > ? ORDER BY user_id LIMIT ?`, afterUserID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

func (r *Runtime) accountKey(userID string) string {
	return r.cfg.Prefix + ":billing:acct:" + userID
}

func (r *Runtime) reservationKey(requestID string) string {
	return r.cfg.Prefix + ":billing:resv:" + requestID
}

func (r *Runtime) reservationExpiryKey() string {
	return r.cfg.Prefix + ":billing:resv:expiry"
}

func (r *Runtime) streamKey() string {
	return r.cfg.Prefix + ":billing:events"
}

func (r *Runtime) windowKey(stateID string) string {
	return r.cfg.Prefix + ":billing:quota:" + stateID
}

func windowStateID(subscriptionID string, limitType model.LimitType, windowStart time.Time) string {
	return fmt.Sprintf("%s:%s:%d", subscriptionID, limitType, windowStart.UTC().Unix())
}

func (r *Runtime) reconcileBatchSize() int64 {
	if r.cfg.ReconcileBatchSize > 0 {
		return r.cfg.ReconcileBatchSize
	}
	if r.cfg.StreamBatchSize > 0 {
		return r.cfg.StreamBatchSize
	}
	return defaultReconcileBatch
}

func (r *Runtime) expiryBatchSize() int64 {
	if r.cfg.ExpiryBatchSize > 0 {
		return r.cfg.ExpiryBatchSize
	}
	if r.cfg.StreamBatchSize > 0 {
		return r.cfg.StreamBatchSize
	}
	return defaultReconcileBatch
}

func (r *Runtime) projectMessages(ctx context.Context, messages []redis.XMessage) []string {
	if len(messages) == 0 {
		return nil
	}

	start := time.Now()
	if err := r.projectEventsBatch(ctx, messages); err == nil {
		perMessage := time.Since(start) / time.Duration(len(messages))
		ackedIDs := make([]string, 0, len(messages))
		for _, msg := range messages {
			r.metrics.recordProject(perMessage)
			ackedIDs = append(ackedIDs, msg.ID)
		}
		return ackedIDs
	}
	log.Warnf("billing state: projector batch apply failed for %d messages, retrying individually", len(messages))

	ackedIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		start := time.Now()
		if err := r.projectEvent(ctx, msg); err != nil {
			r.metrics.addProjectFailure()
			log.Warnf("billing state: projector apply failed for %s: %v", msg.ID, err)
			continue
		}
		r.metrics.recordProject(time.Since(start))
		ackedIDs = append(ackedIDs, msg.ID)
	}
	return ackedIDs
}

func (r *Runtime) projectEventsBatch(ctx context.Context, messages []redis.XMessage) error {
	if len(messages) == 0 {
		return nil
	}

	db := database.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, msg := range messages {
		applied, err := ensureProjectionEvent(tx, msg.ID, stringField(msg.Values["event_type"]), stringField(msg.Values["request_id"]))
		if err != nil {
			return err
		}
		if !applied {
			continue
		}

		switch stringField(msg.Values["event_type"]) {
		case "reserve":
			err = applyReserveEvent(tx, msg.Values)
		case "settle":
			err = applySettleEvent(tx, msg.Values)
		case "expire":
			err = applyExpireEvent(tx, msg.Values)
		}
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func ensureProjectionEvent(tx *sql.Tx, streamID, eventType, requestID string) (bool, error) {
	result, err := tx.Exec(
		`INSERT INTO billing_projection_events (stream_id, request_id, event_type, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(stream_id) DO NOTHING`,
		streamID, nullIfEmpty(requestID), eventType, time.Now().UTC(),
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func applyReserveEvent(tx *sql.Tx, values map[string]any) error {
	now := time.Now().UTC()
	requestID := stringField(values["request_id"])
	userID := stringField(values["user_id"])
	subscriptionID := stringField(values["user_subscription_id"])
	status := stringField(values["status"])
	if status == "" {
		status = "reserved"
	}
	estimated := parseInt64(stringField(values["estimated_cost_micros"]))
	reservedSub := parseInt64(stringField(values["reserved_subscription_micros"]))
	reservedBal := parseInt64(stringField(values["reserved_balance_micros"]))
	expiresAtUnix := parseInt64(stringField(values["expires_at_unix"]))
	windowRefsJSON := stringField(values["window_refs_json"])

	_, err := tx.Exec(
		`INSERT INTO billing_reservations (
			request_id, user_id, user_subscription_id, estimated_cost_micros,
			reserved_subscription_micros, reserved_balance_micros, charged_subscription_micros, charged_balance_micros,
			status, window_refs_json, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET
			user_id = excluded.user_id,
			user_subscription_id = excluded.user_subscription_id,
			estimated_cost_micros = excluded.estimated_cost_micros,
			reserved_subscription_micros = excluded.reserved_subscription_micros,
			reserved_balance_micros = excluded.reserved_balance_micros,
			status = excluded.status,
			window_refs_json = excluded.window_refs_json,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`,
		requestID, userID, nullIfEmpty(subscriptionID), estimated, reservedSub, reservedBal, status, nullIfEmpty(windowRefsJSON), time.Unix(expiresAtUnix, 0).UTC(), now, now,
	)
	if err != nil {
		return err
	}

	if reservedBal > 0 {
		if _, err := tx.Exec(`UPDATE billing_account_state SET balance_micros = balance_micros - ?, revision = revision + 1, updated_at = ? WHERE user_id = ?`, reservedBal, now, userID); err != nil {
			return err
		}
	}

	refs, err := decodeWindowRefs(stringField(values["window_refs_json"]))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if reservedSub == 0 {
			break
		}
		if _, err := tx.Exec(
			`UPDATE subscription_window_state
			 SET reserved_micros = reserved_micros + ?, remaining_micros = remaining_micros - ?, revision = revision + 1, updated_at = ?
			 WHERE id = ?`,
			reservedSub, reservedSub, now, ref.StateID,
		); err != nil {
			return err
		}
	}

	return nil
}

func applySettleEvent(tx *sql.Tx, values map[string]any) error {
	now := time.Now().UTC()
	requestID := stringField(values["request_id"])
	userID := stringField(values["user_id"])
	status := stringField(values["status"])
	actual := parseInt64(stringField(values["actual_cost_micros"]))
	reservedSub := parseInt64(stringField(values["reserved_subscription_micros"]))
	reservedBal := parseInt64(stringField(values["reserved_balance_micros"]))
	chargedSub := parseInt64(stringField(values["charged_subscription_micros"]))
	releasedSub := parseInt64(stringField(values["released_subscription_micros"]))
	chargedBal := parseInt64(stringField(values["charged_balance_micros"]))
	releasedBal := parseInt64(stringField(values["released_balance_micros"]))
	extraSub := chargedSub - minInt64(actual, reservedSub)
	if extraSub < 0 {
		extraSub = 0
	}
	extraBal := chargedBal - minInt64(maxInt64(0, actual-reservedSub), reservedBal)
	if extraBal < 0 {
		extraBal = 0
	}

	_, err := tx.Exec(
		`UPDATE billing_reservations SET
			actual_cost_micros = ?,
			charged_subscription_micros = ?,
			charged_balance_micros = ?,
			status = ?,
			updated_at = ?
		WHERE request_id = ?`,
		actual, chargedSub, chargedBal, status, now, requestID,
	)
	if err != nil {
		return err
	}

	if chargedSub > 0 {
		subID, err := lookupReservationSubscriptionID(tx, requestID)
		if err != nil {
			return err
		}
		if subID != "" {
			if err := insertBillingEventTx(tx, requestID, userID, subID, model.BillingSourceSubscription, chargedSub, now); err != nil {
				return err
			}
		}
	}
	if chargedBal > 0 {
		if err := insertBillingEventTx(tx, requestID, userID, "", model.BillingSourceBalance, chargedBal, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET balance_micros = CASE WHEN balance_micros >= ? THEN balance_micros - ? ELSE 0 END, updated_at = ? WHERE id = ?`, chargedBal, chargedBal, now, userID); err != nil {
			return err
		}
	}
	if releasedBal > 0 {
		if _, err := tx.Exec(`UPDATE billing_account_state SET balance_micros = balance_micros + ?, revision = revision + 1, updated_at = ? WHERE user_id = ?`, releasedBal, now, userID); err != nil {
			return err
		}
	}
	if extraBal > 0 {
		if _, err := tx.Exec(`UPDATE billing_account_state SET balance_micros = balance_micros - ?, revision = revision + 1, updated_at = ? WHERE user_id = ?`, extraBal, now, userID); err != nil {
			return err
		}
	}

	refs, err := decodeWindowRefs(stringField(values["window_refs_json"]))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if reservedSub == 0 {
			break
		}
		if _, err := tx.Exec(
			`UPDATE subscription_window_state
			 SET reserved_micros = reserved_micros - ?, used_micros = used_micros + ?, remaining_micros = remaining_micros + ?, revision = revision + 1, updated_at = ?
			 WHERE id = ?`,
			reservedSub, chargedSub, releasedSub-extraSub, now, ref.StateID,
		); err != nil {
			return err
		}
	}

	return nil
}

func applyExpireEvent(tx *sql.Tx, values map[string]any) error {
	now := time.Now().UTC()
	requestID := stringField(values["request_id"])
	userID := stringField(values["user_id"])
	releasedSub := parseInt64(stringField(values["released_subscription_micros"]))
	releasedBal := parseInt64(stringField(values["released_balance_micros"]))

	if _, err := tx.Exec(`UPDATE billing_reservations SET status = 'expired', updated_at = ? WHERE request_id = ?`, now, requestID); err != nil {
		return err
	}
	if releasedBal > 0 {
		if _, err := tx.Exec(`UPDATE billing_account_state SET balance_micros = balance_micros + ?, revision = revision + 1, updated_at = ? WHERE user_id = ?`, releasedBal, now, userID); err != nil {
			return err
		}
	}

	refs, err := decodeWindowRefs(stringField(values["window_refs_json"]))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if releasedSub == 0 {
			break
		}
		if _, err := tx.Exec(
			`UPDATE subscription_window_state
			 SET reserved_micros = reserved_micros - ?, remaining_micros = remaining_micros + ?, revision = revision + 1, updated_at = ?
			 WHERE id = ?`,
			releasedSub, releasedSub, now, ref.StateID,
		); err != nil {
			return err
		}
	}

	return nil
}

func updateRequestLogBillingTx(tx *sql.Tx, requestID, status string, chargedSub, chargedBal int64) error {
	_, err := tx.Exec(
		`UPDATE request_logs
		 SET charged_subscription_micros = ?, charged_balance_micros = ?, billing_status = ?
		 WHERE id = ?
		   AND (
		     COALESCE(charged_subscription_micros, -1) <> ?
		     OR COALESCE(charged_balance_micros, -1) <> ?
		     OR COALESCE(billing_status, '') <> ?
		   )`,
		chargedSub, chargedBal, status, requestID,
		chargedSub, chargedBal, status,
	)
	return err
}

func lookupReservationSubscriptionID(tx *sql.Tx, requestID string) (string, error) {
	var subID sql.NullString
	err := tx.QueryRow(`SELECT user_subscription_id FROM billing_reservations WHERE request_id = ?`, requestID).Scan(&subID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return subID.String, nil
}

func insertBillingEventTx(tx *sql.Tx, requestID, userID, subscriptionID string, source model.BillingSource, amountMicros int64, now time.Time) error {
	if amountMicros <= 0 {
		return nil
	}
	id := uuid.New().String()
	var subID any
	if subscriptionID != "" {
		subID = subscriptionID
	}
	_, err := tx.Exec(
		`INSERT INTO billing_events (id, request_log_id, user_id, user_subscription_id, source, event_type, amount_micros, created_at)
		 VALUES (?, ?, ?, ?, ?, 'charge', ?, ?)
		 ON CONFLICT DO NOTHING`,
		id, requestID, userID, subID, source, amountMicros, now,
	)
	return err
}

func decodeWindowRefs(raw string) ([]WindowRef, error) {
	if raw == "" {
		return nil, nil
	}
	var refs []WindowRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func stringifyRedisResult(result any) []string {
	switch typed := result.(type) {
	case []interface{}:
		out := make([]string, len(typed))
		for i, item := range typed {
			out[i] = stringField(item)
		}
		return out
	default:
		return nil
	}
}

func stringField(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	default:
		return fmt.Sprint(typed)
	}
}

func parseInt64(raw string) int64 {
	var value int64
	fmt.Sscanf(raw, "%d", &value)
	return value
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func getWindowBounds(limitType model.LimitType, windowMode model.WindowMode, now time.Time, subscriptionStartsAt time.Time) (start, end time.Time, err error) {
	switch limitType {
	case model.LimitTypeDaily:
		if windowMode == model.WindowModeFixed {
			start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			end = start.Add(24 * time.Hour)
		} else {
			start = now.Add(-24 * time.Hour)
			end = now
		}
	case model.LimitTypeWeekly:
		if windowMode == model.WindowModeFixed {
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			start = time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
			end = start.AddDate(0, 0, 7)
		} else {
			start = now.AddDate(0, 0, -7)
			end = now
		}
	case model.LimitTypeMonthly:
		if windowMode == model.WindowModeFixed {
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			end = start.AddDate(0, 1, 0)
		} else {
			start = now.AddDate(0, -1, 0)
			end = now
		}
	case model.LimitTypeRolling5h:
		const windowSec int64 = 18000
		if windowMode == model.WindowModeFixed {
			unix := now.Unix()
			floorUnix := (unix / windowSec) * windowSec
			start = time.Unix(floorUnix, 0).UTC()
			end = start.Add(5 * time.Hour)
		} else {
			start = now.Add(-5 * time.Hour)
			end = now
		}
	case model.LimitTypeTotal:
		start = subscriptionStartsAt.UTC()
		end = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unknown limit type %s", limitType)
	}
	return start, end, nil
}

func maxInt64(values ...int64) int64 {
	out := int64(math.MinInt64)
	for _, value := range values {
		if value > out {
			out = value
		}
	}
	return out
}

func minInt64(values ...int64) int64 {
	out := int64(math.MaxInt64)
	for _, value := range values {
		if value < out {
			out = value
		}
	}
	return out
}

func redisHashMatches(current map[string]string, target map[string]any) bool {
	if len(current) == 0 && len(target) == 0 {
		return true
	}
	for key, expected := range target {
		if current[key] != stringField(expected) {
			return false
		}
	}
	return true
}

func (m *runtimeMetrics) recordReserve(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reserveDurations = appendSampleDuration(m.reserveDurations, d)
}

func (m *runtimeMetrics) recordSettle(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settleDurations = appendSampleDuration(m.settleDurations, d)
}

func (m *runtimeMetrics) recordProject(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectDurations = appendSampleDuration(m.projectDurations, d)
}

func (m *runtimeMetrics) recordReclaim(d time.Duration, claimed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reclaimDurations = appendSampleDuration(m.reclaimDurations, d)
	atomic.AddInt64(&m.reclaimClaimed, claimed)
}

func (m *runtimeMetrics) recordReconcile(d time.Duration, repairs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileDurations = appendSampleDuration(m.reconcileDurations, d)
	atomic.AddInt64(&m.reconcileRepairs, repairs)
}

func appendSampleDuration(dst []time.Duration, d time.Duration) []time.Duration {
	const maxSamples = 50000
	dst = append(dst, d)
	if len(dst) <= maxSamples {
		return dst
	}
	copy(dst, dst[len(dst)-maxSamples:])
	return dst[:maxSamples]
}

func (m *runtimeMetrics) addReserveFailure() { atomic.AddInt64(&m.reserveFailures, 1) }
func (m *runtimeMetrics) addSettleFailure()  { atomic.AddInt64(&m.settleFailures, 1) }
func (m *runtimeMetrics) addProjectFailure() { atomic.AddInt64(&m.projectFailures, 1) }
func (m *runtimeMetrics) addReclaimFailure() { atomic.AddInt64(&m.reclaimFailures, 1) }
func (m *runtimeMetrics) addReconcileFailure() {
	atomic.AddInt64(&m.reconcileFailures, 1)
}

func (m *runtimeMetrics) snapshot(reset bool) RuntimeMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := RuntimeMetricsSnapshot{
		ReserveDurations:   append([]time.Duration(nil), m.reserveDurations...),
		SettleDurations:    append([]time.Duration(nil), m.settleDurations...),
		ProjectDurations:   append([]time.Duration(nil), m.projectDurations...),
		ReclaimDurations:   append([]time.Duration(nil), m.reclaimDurations...),
		ReconcileDurations: append([]time.Duration(nil), m.reconcileDurations...),
		ReserveFailures:    atomic.LoadInt64(&m.reserveFailures),
		SettleFailures:     atomic.LoadInt64(&m.settleFailures),
		ProjectFailures:    atomic.LoadInt64(&m.projectFailures),
		ReclaimFailures:    atomic.LoadInt64(&m.reclaimFailures),
		ReclaimClaimed:     atomic.LoadInt64(&m.reclaimClaimed),
		ReconcileFailures:  atomic.LoadInt64(&m.reconcileFailures),
		ReconcileRepairs:   atomic.LoadInt64(&m.reconcileRepairs),
	}

	if reset {
		m.reserveDurations = nil
		m.settleDurations = nil
		m.projectDurations = nil
		m.reclaimDurations = nil
		m.reconcileDurations = nil
		atomic.StoreInt64(&m.reserveFailures, 0)
		atomic.StoreInt64(&m.settleFailures, 0)
		atomic.StoreInt64(&m.projectFailures, 0)
		atomic.StoreInt64(&m.reclaimFailures, 0)
		atomic.StoreInt64(&m.reclaimClaimed, 0)
		atomic.StoreInt64(&m.reconcileFailures, 0)
		atomic.StoreInt64(&m.reconcileRepairs, 0)
	}
	return out
}
