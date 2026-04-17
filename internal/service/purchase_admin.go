package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

var (
	ErrPurchaseWebhookTargetNotFound = errors.New("订单通知目标不存在")
	ErrPurchaseOrderStatusInvalid    = errors.New("仅支持修改为 paid、expired、refunded")
	ErrPurchaseManualSettlementDup   = errors.New("订单已记入分账台账，请刷新后重试")
	ErrPurchaseManualSettlementState = errors.New("仅已支付或已退款订单可记入分账台账")
	ErrPurchaseManualSettlementEmpty = errors.New("该时间区间内没有可分账订单")
	ErrPurchaseManualSettlementRange = errors.New("请选择有效的支付时间区间")
)

var purchaseWebhookTemplatePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

var purchaseWebhookRetrySchedule = []time.Duration{
	time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
}

type purchaseWebhookWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

var purchaseWebhookWorkerState struct {
	mu     sync.Mutex
	worker *purchaseWebhookWorker
}

func (s *PurchaseService) ListWebhookTargets() ([]*model.PurchaseWebhookTarget, error) {
	return s.adminRepo.ListWebhookTargets()
}

func (s *PurchaseService) CreateWebhookTarget(req *model.PurchaseWebhookTargetRequest) (*model.PurchaseWebhookTarget, error) {
	target, err := normalizeWebhookTargetRequest("", req)
	if err != nil {
		return nil, err
	}
	if err := s.adminRepo.CreateWebhookTarget(target); err != nil {
		return nil, err
	}
	return target, nil
}

func (s *PurchaseService) UpdateWebhookTarget(id string, req *model.PurchaseWebhookTargetRequest) (*model.PurchaseWebhookTarget, error) {
	existing, err := s.adminRepo.GetWebhookTargetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrPurchaseWebhookTargetNotFound
	}
	target, err := normalizeWebhookTargetRequest(id, req)
	if err != nil {
		return nil, err
	}
	target.CreatedAt = existing.CreatedAt
	if err := s.adminRepo.UpdateWebhookTarget(target); err != nil {
		return nil, err
	}
	return s.adminRepo.GetWebhookTargetByID(id)
}

func (s *PurchaseService) SetWebhookTargetEnabled(id string, enabled bool) error {
	target, err := s.adminRepo.GetWebhookTargetByID(id)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrPurchaseWebhookTargetNotFound
	}
	return s.adminRepo.SetWebhookTargetEnabled(id, enabled)
}

func (s *PurchaseService) DeleteWebhookTarget(id string) error {
	target, err := s.adminRepo.GetWebhookTargetByID(id)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrPurchaseWebhookTargetNotFound
	}
	return s.adminRepo.DeleteWebhookTarget(id)
}

func (s *PurchaseService) TestWebhookTarget(ctx context.Context, id string) (*model.PurchaseWebhookTestResponse, error) {
	target, err := s.adminRepo.GetWebhookTargetByID(id)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrPurchaseWebhookTargetNotFound
	}
	variables := buildPurchaseWebhookTestVariables()
	return sendWebhookRequest(ctx, target, variables)
}

func (s *PurchaseService) UpdateOrderPaymentStatusAdmin(orderNo string, req *model.PurchaseOrderPaymentStatusUpdateRequest, adminUsername string) (*model.PurchaseOrderResponse, error) {
	if req.PaymentStatus != model.PurchasePaymentStatusPaid && req.PaymentStatus != model.PurchasePaymentStatusExpired && req.PaymentStatus != model.PurchasePaymentStatusRefunded {
		return nil, ErrPurchaseOrderStatusInvalid
	}
	note := strings.TrimSpace(req.Note)

	tx, err := database.GetDB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	order, err := s.getOrderByOrderNoTx(tx, orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}

	now := time.Now().UTC()
	if req.PaymentStatus == model.PurchasePaymentStatusPaid && order.PaymentStatus != model.PurchasePaymentStatusPaid {
		_ = tx.Rollback()
		updated, err := s.applySuccessfulPayment(orderNo, order.AlipayTradeNo, &now)
		if err != nil {
			return nil, err
		}
		historyTx, err := database.GetDB().Begin()
		if err != nil {
			return nil, err
		}
		defer historyTx.Rollback()
		if err := s.adminRepo.AppendPaymentStatusHistoryTx(historyTx, &model.PurchaseOrderPaymentStatusHistory{
			ID:         uuid.NewString(),
			OrderID:    order.ID,
			OrderNo:    order.OrderNo,
			FromStatus: order.PaymentStatus,
			ToStatus:   req.PaymentStatus,
			Note:       note,
			CreatedBy:  strings.TrimSpace(adminUsername),
			CreatedAt:  now,
		}); err != nil {
			return nil, err
		}
		if err := historyTx.Commit(); err != nil {
			return nil, err
		}
		return updated, nil
	}

	paidAt := order.PaidAt
	switch req.PaymentStatus {
	case model.PurchasePaymentStatusPaid:
		if paidAt == nil {
			paidAt = &now
		}
	case model.PurchasePaymentStatusRefunded:
		if paidAt == nil {
			paidAt = &now
		}
	case model.PurchasePaymentStatusExpired:
		paidAt = nil
	}

	if _, err := tx.Exec(
		`UPDATE purchase_orders
		    SET payment_status = ?, paid_at = ?, updated_at = ?
		  WHERE order_no = ?`,
		req.PaymentStatus,
		paidAt,
		now,
		orderNo,
	); err != nil {
		return nil, err
	}

	if err := s.adminRepo.AppendPaymentStatusHistoryTx(tx, &model.PurchaseOrderPaymentStatusHistory{
		ID:         uuid.NewString(),
		OrderID:    order.ID,
		OrderNo:    order.OrderNo,
		FromStatus: order.PaymentStatus,
		ToStatus:   req.PaymentStatus,
		Note:       note,
		CreatedBy:  strings.TrimSpace(adminUsername),
		CreatedAt:  now,
	}); err != nil {
		return nil, err
	}

	var syncActions []BillingStateSyncAction
	if req.PaymentStatus == model.PurchasePaymentStatusRefunded {
		if err := s.couponSvc.ReverseUsageForOrderTx(tx, order, now); err != nil {
			return nil, err
		}
		syncActions, err = s.inviteSvc.HandleRefundedOrderTx(tx, order, now)
		if err != nil {
			return nil, err
		}
	}
	if req.PaymentStatus == model.PurchasePaymentStatusExpired && order.PaymentStatus == model.PurchasePaymentStatusPending {
		if err := s.couponSvc.ReverseUsageForOrderTx(tx, order, now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if err := SyncBillingStateActions(context.Background(), s.grantSvc, syncActions); err != nil {
		return nil, err
	}
	return s.orderRepo.GetDetailByOrderNo(orderNo)
}

func (s *PurchaseService) ListOrderPaymentStatusHistory(orderNo string) ([]*model.PurchaseOrderPaymentStatusHistory, error) {
	order, err := s.orderRepo.GetByOrderNo(orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	return s.adminRepo.ListPaymentStatusHistory(order.ID)
}

func (s *PurchaseService) CreateSingleManualSettlement(orderNo, note, adminUsername string) (*model.PurchaseManualSettlementBatch, error) {
	return s.createManualSettlementBatch([]string{orderNo}, note, adminUsername, model.PurchaseManualSettlementBatchModeSingle, false)
}

func (s *PurchaseService) CreateBatchManualSettlement(orderNos []string, note, adminUsername string) (*model.PurchaseManualSettlementBatch, error) {
	return s.createManualSettlementBatch(orderNos, note, adminUsername, model.PurchaseManualSettlementBatchModeBatch, false)
}

func (s *PurchaseService) PreviewBatchManualSettlement(req *model.PurchaseManualSettlementPreviewRequest) (*model.PurchaseManualSettlementPreviewResponse, error) {
	filters, err := normalizeManualSettlementFilters(req.PaidFrom, req.PaidTo)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	filters.Limit = limit

	items, total, err := s.orderRepo.ListAdmin(filters)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := s.normalizeOrderState(item); err != nil {
			return nil, err
		}
	}

	totalAmountCNYCent, err := s.orderRepo.SumAdminAmount(filters)
	if err != nil {
		return nil, err
	}

	return &model.PurchaseManualSettlementPreviewResponse{
		Items:              items,
		Total:              total,
		TotalAmountCNYCent: totalAmountCNYCent,
	}, nil
}

func (s *PurchaseService) CreateBatchManualSettlementByRequest(req *model.PurchaseManualSettlementConfirmRequest, adminUsername string) (*model.PurchaseManualSettlementBatch, error) {
	if len(req.OrderNos) > 0 {
		return s.createManualSettlementBatch(req.OrderNos, req.Note, adminUsername, model.PurchaseManualSettlementBatchModeBatch, req.DebugSettlement)
	}

	filters, err := normalizeManualSettlementFilters(req.PaidFrom, req.PaidTo)
	if err != nil {
		return nil, err
	}

	orderNos, err := s.orderRepo.ListAdminOrderNos(filters)
	if err != nil {
		return nil, err
	}
	if len(orderNos) == 0 {
		return nil, ErrPurchaseManualSettlementEmpty
	}

	return s.createManualSettlementBatch(orderNos, req.Note, adminUsername, model.PurchaseManualSettlementBatchModeBatch, req.DebugSettlement)
}

func (s *PurchaseService) createManualSettlementBatch(orderNos []string, note, adminUsername string, mode model.PurchaseManualSettlementBatchMode, debugSettlement bool) (*model.PurchaseManualSettlementBatch, error) {
	uniqueOrderNos := make([]string, 0, len(orderNos))
	seen := map[string]struct{}{}
	for _, orderNo := range orderNos {
		orderNo = strings.TrimSpace(orderNo)
		if orderNo == "" {
			continue
		}
		if _, ok := seen[orderNo]; ok {
			continue
		}
		seen[orderNo] = struct{}{}
		uniqueOrderNos = append(uniqueOrderNos, orderNo)
	}
	if len(uniqueOrderNos) == 0 {
		return nil, ErrPurchaseManualSettlementDup
	}

	tx, err := database.GetDB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	batch := &model.PurchaseManualSettlementBatch{
		BatchNo:         buildPurchaseManualSettlementBatchNo(now),
		Mode:            mode,
		DebugSettlement: debugSettlement,
		CreatedBy:       strings.TrimSpace(adminUsername),
		Note:            strings.TrimSpace(note),
		CreatedAt:       now,
	}

	orders := make([]*model.PurchaseOrderResponse, 0, len(uniqueOrderNos))
	for _, orderNo := range uniqueOrderNos {
		order, err := s.getOrderByOrderNoTx(tx, orderNo)
		if err != nil {
			return nil, err
		}
		if order == nil {
			return nil, ErrPurchaseOrderNotFound
		}
		if !purchaseStatusFundsReceived(order.PaymentStatus) {
			return nil, ErrPurchaseManualSettlementState
		}
		result, err := tx.Exec(
			`UPDATE purchase_orders
			    SET manual_settlement_done = 1, updated_at = ?
			  WHERE order_no = ? AND manual_settlement_done = 0 AND payment_status IN (?, ?)`,
			now,
			orderNo,
			model.PurchasePaymentStatusPaid,
			model.PurchasePaymentStatusRefunded,
		)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected != 1 {
			return nil, ErrPurchaseManualSettlementDup
		}
		orders = append(orders, &model.PurchaseOrderResponse{
			ID:            order.ID,
			OrderNo:       order.OrderNo,
			AmountCNYCent: order.AmountCNYCent,
			PaymentStatus: order.PaymentStatus,
		})
		batch.OrderCount++
		batch.TotalAmountCNYCent += order.AmountCNYCent
	}

	if err := s.adminRepo.CreateManualSettlementBatchTx(tx, batch); err != nil {
		return nil, err
	}
	for _, order := range orders {
		if err := s.adminRepo.CreateManualSettlementBatchItemTx(tx, batch.ID, order, now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return batch, nil
}

func normalizeManualSettlementFilters(paidFrom, paidTo *time.Time) (model.PurchaseOrderFilters, error) {
	if paidFrom == nil || paidTo == nil {
		return model.PurchaseOrderFilters{}, ErrPurchaseManualSettlementRange
	}
	if paidFrom.After(*paidTo) {
		return model.PurchaseOrderFilters{}, ErrPurchaseManualSettlementRange
	}

	manualSettlementDone := false
	return model.PurchaseOrderFilters{
		PaidFrom:                    paidFrom,
		PaidTo:                      paidTo,
		ManualSettlementDone:        &manualSettlementDone,
		EligibleForManualSettlement: true,
	}, nil
}

func StartPurchaseWebhookWorker() {
	purchaseWebhookWorkerState.mu.Lock()
	defer purchaseWebhookWorkerState.mu.Unlock()
	if purchaseWebhookWorkerState.worker != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &purchaseWebhookWorker{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	purchaseWebhookWorkerState.worker = worker
	go runPurchaseWebhookWorker(ctx, worker.done)
}

func StopPurchaseWebhookWorker() {
	purchaseWebhookWorkerState.mu.Lock()
	worker := purchaseWebhookWorkerState.worker
	purchaseWebhookWorkerState.worker = nil
	purchaseWebhookWorkerState.mu.Unlock()
	if worker == nil {
		return
	}
	worker.cancel()
	<-worker.done
}

func runPurchaseWebhookWorker(ctx context.Context, done chan struct{}) {
	defer close(done)
	service := NewPurchaseService()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		if err := service.ProcessPendingWebhookEvents(ctx, 8); err != nil {
			log.Warnf("purchase webhook worker: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *PurchaseService) ProcessPendingWebhookEvents(ctx context.Context, maxCount int) error {
	if maxCount <= 0 {
		maxCount = 1
	}
	for idx := 0; idx < maxCount; idx++ {
		now := time.Now().UTC()
		claimToken := uuid.NewString()
		event, err := s.adminRepo.ClaimNextWebhookEvent(now, now.Add(30*time.Second), claimToken)
		if err != nil {
			return err
		}
		if event == nil {
			return nil
		}
		if err := s.processWebhookEvent(ctx, event); err != nil {
			log.Warnf("purchase webhook worker: process event %s failed: %v", event.ID, err)
		}
	}
	return nil
}

func (s *PurchaseService) processWebhookEvent(ctx context.Context, event *model.PurchaseWebhookEvent) error {
	target, err := s.adminRepo.GetWebhookTargetByID(event.TargetID)
	if err != nil {
		return err
	}
	if target == nil || !target.Enabled {
		return s.adminRepo.MarkWebhookEventSucceeded(event.ID, event.ClaimToken, 204, time.Now().UTC())
	}
	order, err := s.orderRepo.GetDetailByOrderNo(event.OrderNo)
	if err != nil {
		return err
	}
	if order == nil {
		return s.adminRepo.MarkWebhookEventFailed(event.ID, event.ClaimToken, 0, "order not found", time.Now().UTC().Add(nextPurchaseWebhookRetryDelay(event.AttemptCount)), time.Now().UTC())
	}
	variables, err := buildPurchaseWebhookVariables(order)
	if err != nil {
		return err
	}
	result, sendErr := sendWebhookRequest(ctx, target, variables)
	now := time.Now().UTC()
	if sendErr != nil || !result.OK {
		msg := ""
		if sendErr != nil {
			msg = sendErr.Error()
		} else {
			msg = fmt.Sprintf("unexpected status %d", result.ResponseStatusCode)
			if result.ResponseBody != "" {
				msg += ": " + result.ResponseBody
			}
		}
		return s.adminRepo.MarkWebhookEventFailed(event.ID, event.ClaimToken, result.ResponseStatusCode, truncateString(msg, 500), now.Add(nextPurchaseWebhookRetryDelay(event.AttemptCount)), now)
	}
	return s.adminRepo.MarkWebhookEventSucceeded(event.ID, event.ClaimToken, result.ResponseStatusCode, now)
}

func normalizeWebhookTargetRequest(id string, req *model.PurchaseWebhookTargetRequest) (*model.PurchaseWebhookTarget, error) {
	targetURL, err := normalizeWebhookURL(req.TargetURL)
	if err != nil {
		return nil, err
	}
	headersTemplate := strings.TrimSpace(req.HeadersTemplate)
	if headersTemplate == "" {
		headersTemplate = "{}"
	}
	var headerMap map[string]any
	if err := json.Unmarshal([]byte(headersTemplate), &headerMap); err != nil {
		return nil, fmt.Errorf("请求头模板必须是 JSON 对象: %w", err)
	}
	return &model.PurchaseWebhookTarget{
		ID:              id,
		Name:            strings.TrimSpace(req.Name),
		TargetURL:       targetURL,
		BodyTemplate:    strings.TrimSpace(req.BodyTemplate),
		HeadersTemplate: headersTemplate,
		Enabled:         req.Enabled,
	}, nil
}

func buildPurchaseWebhookVariables(order *model.PurchaseOrderResponse) (map[string]string, error) {
	nowShanghai := time.Now().UTC().In(time.FixedZone("UTC+8", 8*3600))
	dayStartShanghai := time.Date(nowShanghai.Year(), nowShanghai.Month(), nowShanghai.Day(), 0, 0, 0, 0, nowShanghai.Location())
	dayEndShanghai := dayStartShanghai.Add(24 * time.Hour)
	dayStartUTC := dayStartShanghai.UTC()
	dayEndUTC := dayEndShanghai.UTC()

	var amountToday, amountTotal sql.NullInt64
	var billsToday, billsTotal int64
	db := database.GetDB()
	if err := db.QueryRow(`SELECT COALESCE(SUM(amount_cny_cent), 0), COUNT(*) FROM purchase_orders WHERE payment_status = ?`, model.PurchasePaymentStatusPaid).Scan(&amountTotal, &billsTotal); err != nil {
		return nil, err
	}
	if err := db.QueryRow(`SELECT COALESCE(SUM(amount_cny_cent), 0), COUNT(*) FROM purchase_orders WHERE payment_status = ? AND created_at >= ? AND created_at < ?`, model.PurchasePaymentStatusPaid, dayStartUTC, dayEndUTC).Scan(&amountToday, &billsToday); err != nil {
		return nil, err
	}

	return map[string]string{
		"amountCny":        purchaseWebhookCentsToYuanString(order.AmountCNYCent),
		"amountToday":      purchaseWebhookCentsToYuanString(amountToday.Int64),
		"amountTotal":      purchaseWebhookCentsToYuanString(amountTotal.Int64),
		"billsToday":       fmt.Sprintf("%d", billsToday),
		"billsTotal":       fmt.Sprintf("%d", billsTotal),
		"subscriptionName": order.ProductName,
		"quantity":         "1",
		"orderNo":          order.OrderNo,
		"deliveryCdk":      order.GeneratedRedeemCode,
		"alipayTradeNo":    order.AlipayTradeNo,
		"createdAt":        order.CreatedAt.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02 15:04:05 +08:00"),
	}, nil
}

func buildPurchaseWebhookTestVariables() map[string]string {
	now := time.Now().UTC().In(time.FixedZone("UTC+8", 8*3600))
	return map[string]string{
		"amountCny":        "99.00",
		"amountToday":      "199.00",
		"amountTotal":      "1299.00",
		"billsToday":       "2",
		"billsTotal":       "17",
		"subscriptionName": "测试订阅",
		"quantity":         "1",
		"orderNo":          "TEST-" + now.Format("20060102150405"),
		"deliveryCdk":      "BUY-TESTCODE",
		"alipayTradeNo":    "2026041700000000",
		"createdAt":        now.Format("2006-01-02 15:04:05 +08:00"),
	}
}

func sendWebhookRequest(ctx context.Context, target *model.PurchaseWebhookTarget, variables map[string]string) (*model.PurchaseWebhookTestResponse, error) {
	if target == nil {
		return nil, ErrPurchaseWebhookTargetNotFound
	}
	body := renderPurchaseWebhookTemplate(target.BodyTemplate, variables)
	headers, err := renderPurchaseWebhookHeaders(target.HeadersTemplate, variables, body)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target.TargetURL, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return &model.PurchaseWebhookTestResponse{Variables: variables}, err
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4000))
	result := &model.PurchaseWebhookTestResponse{
		OK:                 resp.StatusCode >= 200 && resp.StatusCode < 300,
		ResponseStatusCode: resp.StatusCode,
		ResponseHeaders:    flattenHeaderMap(resp.Header),
		ResponseBody:       string(responseBody),
		Variables:          variables,
	}
	return result, nil
}

func renderPurchaseWebhookTemplate(template string, variables map[string]string) string {
	return purchaseWebhookTemplatePattern.ReplaceAllStringFunc(template, func(match string) string {
		keyMatch := purchaseWebhookTemplatePattern.FindStringSubmatch(match)
		if len(keyMatch) != 2 {
			return ""
		}
		return variables[keyMatch[1]]
	})
}

func renderPurchaseWebhookHeaders(template string, variables map[string]string, body string) (map[string]string, error) {
	rendered := renderPurchaseWebhookTemplate(template, variables)
	if strings.TrimSpace(rendered) == "" {
		rendered = "{}"
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(rendered), &raw); err != nil {
		return nil, fmt.Errorf("请求头模板渲染后不是 JSON 对象: %w", err)
	}
	headers := make(map[string]string, len(raw)+1)
	for key, value := range raw {
		headers[key] = fmt.Sprintf("%v", value)
	}
	hasContentType := false
	for key := range headers {
		if strings.EqualFold(key, "Content-Type") {
			hasContentType = true
			break
		}
	}
	if !hasContentType {
		trimmed := strings.TrimSpace(body)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			headers["Content-Type"] = "application/json; charset=utf-8"
		} else {
			headers["Content-Type"] = "text/plain; charset=utf-8"
		}
	}
	return headers, nil
}

func normalizeWebhookURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("订单通知地址无效: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("订单通知地址仅支持 http/https")
	}
	if parsed.Host == "" {
		return "", errors.New("订单通知地址缺少主机名")
	}
	return parsed.String(), nil
}

func flattenHeaderMap(header http.Header) map[string]string {
	result := make(map[string]string, len(header))
	for key, values := range header {
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func purchaseWebhookCentsToYuanString(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func nextPurchaseWebhookRetryDelay(attemptCount int) time.Duration {
	if attemptCount < 0 {
		attemptCount = 0
	}
	if attemptCount >= len(purchaseWebhookRetrySchedule) {
		return purchaseWebhookRetrySchedule[len(purchaseWebhookRetrySchedule)-1]
	}
	return purchaseWebhookRetrySchedule[attemptCount]
}

func purchaseStatusFundsReceived(status model.PurchasePaymentStatus) bool {
	return status == model.PurchasePaymentStatusPaid || status == model.PurchasePaymentStatusRefunded
}

func buildPurchaseManualSettlementBatchNo(now time.Time) string {
	return "PMS" + now.UTC().Format("20060102150405") + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:6]
}
