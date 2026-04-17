package repository

import (
	"database/sql"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type PurchaseAdminRepository struct{}

func NewPurchaseAdminRepository() *PurchaseAdminRepository {
	return &PurchaseAdminRepository{}
}

func (r *PurchaseAdminRepository) ListWebhookTargets() ([]*model.PurchaseWebhookTarget, error) {
	rows, err := database.GetDB().Query(`SELECT id, name, target_url, body_template, headers_template, enabled, created_at, updated_at FROM purchase_webhook_targets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWebhookTargets(rows)
}

func (r *PurchaseAdminRepository) GetWebhookTargetByID(id string) (*model.PurchaseWebhookTarget, error) {
	row := database.GetDB().QueryRow(`SELECT id, name, target_url, body_template, headers_template, enabled, created_at, updated_at FROM purchase_webhook_targets WHERE id = ?`, id)
	return scanWebhookTarget(row)
}

func (r *PurchaseAdminRepository) CreateWebhookTarget(target *model.PurchaseWebhookTarget) error {
	now := time.Now().UTC()
	target.ID = uuid.NewString()
	target.CreatedAt = now
	target.UpdatedAt = now
	_, err := database.GetDB().Exec(
		`INSERT INTO purchase_webhook_targets (id, name, target_url, body_template, headers_template, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		target.ID,
		target.Name,
		target.TargetURL,
		target.BodyTemplate,
		target.HeadersTemplate,
		target.Enabled,
		target.CreatedAt,
		target.UpdatedAt,
	)
	return err
}

func (r *PurchaseAdminRepository) UpdateWebhookTarget(target *model.PurchaseWebhookTarget) error {
	target.UpdatedAt = time.Now().UTC()
	_, err := database.GetDB().Exec(
		`UPDATE purchase_webhook_targets SET name = ?, target_url = ?, body_template = ?, headers_template = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		target.Name,
		target.TargetURL,
		target.BodyTemplate,
		target.HeadersTemplate,
		target.Enabled,
		target.UpdatedAt,
		target.ID,
	)
	return err
}

func (r *PurchaseAdminRepository) SetWebhookTargetEnabled(id string, enabled bool) error {
	_, err := database.GetDB().Exec(`UPDATE purchase_webhook_targets SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, time.Now().UTC(), id)
	return err
}

func (r *PurchaseAdminRepository) DeleteWebhookTarget(id string) error {
	_, err := database.GetDB().Exec(`DELETE FROM purchase_webhook_targets WHERE id = ?`, id)
	return err
}

func (r *PurchaseAdminRepository) QueueWebhookEventsTx(tx *sql.Tx, orderID, orderNo string) error {
	rows, err := tx.Query(`SELECT id FROM purchase_webhook_targets WHERE enabled = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var targetID string
		if err := rows.Scan(&targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO purchase_webhook_events (id, order_id, order_no, target_id, status, next_attempt_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(order_id, target_id) DO NOTHING`,
			uuid.NewString(),
			orderID,
			orderNo,
			targetID,
			model.PurchaseWebhookEventStatusPending,
			time.Now().UTC(),
			time.Now().UTC(),
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *PurchaseAdminRepository) ClaimNextWebhookEvent(now, claimUntil time.Time, claimToken string) (*model.PurchaseWebhookEvent, error) {
	rows, err := database.GetDB().Query(
		`SELECT id, order_id, order_no, target_id, status, attempt_count, last_error, response_status_code, claim_token, claimed_at, claim_until, next_attempt_at, last_attempt_at, delivered_at, created_at, updated_at
		   FROM purchase_webhook_events
		  WHERE status IN (?, ?)
		    AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		    AND (claim_until IS NULL OR claim_until < ?)
		  ORDER BY created_at ASC
		  LIMIT 20`,
		model.PurchaseWebhookEventStatusPending,
		model.PurchaseWebhookEventStatusFailed,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		event, err := scanWebhookEventFromRows(rows)
		if err != nil {
			return nil, err
		}
		result, err := database.GetDB().Exec(
			`UPDATE purchase_webhook_events
			    SET status = ?, claim_token = ?, claimed_at = ?, claim_until = ?, updated_at = ?
			  WHERE id = ?
			    AND status IN (?, ?)
			    AND (claim_until IS NULL OR claim_until < ?)`,
			model.PurchaseWebhookEventStatusProcessing,
			claimToken,
			now,
			claimUntil,
			now,
			event.ID,
			model.PurchaseWebhookEventStatusPending,
			model.PurchaseWebhookEventStatusFailed,
			now,
		)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 1 {
			event.Status = model.PurchaseWebhookEventStatusProcessing
			event.ClaimToken = claimToken
			event.ClaimedAt = &now
			event.ClaimUntil = &claimUntil
			return event, nil
		}
	}
	return nil, rows.Err()
}

func (r *PurchaseAdminRepository) MarkWebhookEventSucceeded(id, claimToken string, statusCode int, deliveredAt time.Time) error {
	_, err := database.GetDB().Exec(
		`UPDATE purchase_webhook_events
		    SET status = ?, response_status_code = ?, delivered_at = ?, last_error = '', claim_token = '', claim_until = NULL, last_attempt_at = ?, updated_at = ?
		  WHERE id = ? AND claim_token = ?`,
		model.PurchaseWebhookEventStatusSucceeded,
		statusCode,
		deliveredAt,
		deliveredAt,
		deliveredAt,
		id,
		claimToken,
	)
	return err
}

func (r *PurchaseAdminRepository) MarkWebhookEventFailed(id, claimToken string, statusCode int, lastError string, nextAttemptAt, attemptedAt time.Time) error {
	_, err := database.GetDB().Exec(
		`UPDATE purchase_webhook_events
		    SET status = ?, attempt_count = attempt_count + 1, response_status_code = ?, last_error = ?, claim_token = '', claim_until = NULL, last_attempt_at = ?, next_attempt_at = ?, updated_at = ?
		  WHERE id = ? AND claim_token = ?`,
		model.PurchaseWebhookEventStatusFailed,
		statusCode,
		lastError,
		attemptedAt,
		nextAttemptAt,
		attemptedAt,
		id,
		claimToken,
	)
	return err
}

func (r *PurchaseAdminRepository) AppendPaymentStatusHistoryTx(tx *sql.Tx, item *model.PurchaseOrderPaymentStatusHistory) error {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	_, err := tx.Exec(
		`INSERT INTO purchase_order_payment_status_history (id, order_id, order_no, from_status, to_status, note, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID,
		item.OrderID,
		item.OrderNo,
		item.FromStatus,
		item.ToStatus,
		item.Note,
		item.CreatedBy,
		item.CreatedAt,
	)
	return err
}

func (r *PurchaseAdminRepository) ListPaymentStatusHistory(orderID string) ([]*model.PurchaseOrderPaymentStatusHistory, error) {
	rows, err := database.GetDB().Query(
		`SELECT id, order_id, order_no, from_status, to_status, note, created_by, created_at
		   FROM purchase_order_payment_status_history
		  WHERE order_id = ?
		  ORDER BY created_at DESC`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.PurchaseOrderPaymentStatusHistory, 0)
	for rows.Next() {
		item := &model.PurchaseOrderPaymentStatusHistory{}
		if err := rows.Scan(&item.ID, &item.OrderID, &item.OrderNo, &item.FromStatus, &item.ToStatus, &item.Note, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PurchaseAdminRepository) CreateManualSettlementBatchTx(tx *sql.Tx, batch *model.PurchaseManualSettlementBatch) error {
	if batch.ID == "" {
		batch.ID = uuid.NewString()
	}
	if batch.CreatedAt.IsZero() {
		batch.CreatedAt = time.Now().UTC()
	}
	_, err := tx.Exec(
		`INSERT INTO purchase_manual_settlement_batches (id, batch_no, mode, created_by, note, order_count, total_amount_cny_cent, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		batch.ID,
		batch.BatchNo,
		batch.Mode,
		batch.CreatedBy,
		batch.Note,
		batch.OrderCount,
		batch.TotalAmountCNYCent,
		batch.CreatedAt,
	)
	return err
}

func (r *PurchaseAdminRepository) CreateManualSettlementBatchItemTx(tx *sql.Tx, batchID string, order *model.PurchaseOrderResponse, createdAt time.Time) error {
	_, err := tx.Exec(
		`INSERT INTO purchase_manual_settlement_batch_items (id, batch_id, order_id, order_no, amount_cny_cent, payment_status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(),
		batchID,
		order.ID,
		order.OrderNo,
		order.AmountCNYCent,
		order.PaymentStatus,
		createdAt,
	)
	return err
}

func scanWebhookTargets(rows *sql.Rows) ([]*model.PurchaseWebhookTarget, error) {
	items := make([]*model.PurchaseWebhookTarget, 0)
	for rows.Next() {
		item := &model.PurchaseWebhookTarget{}
		if err := rows.Scan(&item.ID, &item.Name, &item.TargetURL, &item.BodyTemplate, &item.HeadersTemplate, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanWebhookTarget(row *sql.Row) (*model.PurchaseWebhookTarget, error) {
	item := &model.PurchaseWebhookTarget{}
	if err := row.Scan(&item.ID, &item.Name, &item.TargetURL, &item.BodyTemplate, &item.HeadersTemplate, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return item, nil
}

func scanWebhookEventFromRows(rows *sql.Rows) (*model.PurchaseWebhookEvent, error) {
	item := &model.PurchaseWebhookEvent{}
	if err := rows.Scan(&item.ID, &item.OrderID, &item.OrderNo, &item.TargetID, &item.Status, &item.AttemptCount, &item.LastError, &item.ResponseStatusCode, &item.ClaimToken, &item.ClaimedAt, &item.ClaimUntil, &item.NextAttemptAt, &item.LastAttemptAt, &item.DeliveredAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return item, nil
}
