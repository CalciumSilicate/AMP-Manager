package repository

import (
	"database/sql"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type InviteRepository struct{}

func NewInviteRepository() *InviteRepository {
	return &InviteRepository{}
}

func (r *InviteRepository) CreateRelationshipTx(tx *sql.Tx, relation *model.InviteRelationship) error {
	_, err := tx.Exec(
		`INSERT INTO invite_relationships (
			id, inviter_user_id, inviter_code, invitee_user_id, status,
			first_paid_order_id, first_paid_order_no, first_paid_order_kind, first_paid_amount_cny_cent,
			first_paid_at, rewarded_at, last_reversed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		relation.ID,
		relation.InviterUserID,
		relation.InviterCode,
		relation.InviteeUserID,
		relation.Status,
		relation.FirstPaidOrderID,
		relation.FirstPaidOrderNo,
		relation.FirstPaidOrderKind,
		relation.FirstPaidAmountCNYCent,
		relation.FirstPaidAt,
		relation.RewardedAt,
		relation.LastReversedAt,
		relation.CreatedAt,
		relation.UpdatedAt,
	)
	return err
}

func (r *InviteRepository) GetRelationshipByInviteeUserID(userID string) (*model.InviteRelationship, error) {
	return r.getRelationshipByInvitee(database.GetDB().QueryRow(
		`SELECT id, inviter_user_id, inviter_code, invitee_user_id, status,
		        first_paid_order_id, first_paid_order_no, first_paid_order_kind, first_paid_amount_cny_cent,
		        first_paid_at, rewarded_at, last_reversed_at, created_at, updated_at
		   FROM invite_relationships
		  WHERE invitee_user_id = ?`,
		userID,
	))
}

func (r *InviteRepository) GetRelationshipByInviteeUserIDTx(tx *sql.Tx, userID string) (*model.InviteRelationship, error) {
	return r.getRelationshipByInvitee(tx.QueryRow(
		`SELECT id, inviter_user_id, inviter_code, invitee_user_id, status,
		        first_paid_order_id, first_paid_order_no, first_paid_order_kind, first_paid_amount_cny_cent,
		        first_paid_at, rewarded_at, last_reversed_at, created_at, updated_at
		   FROM invite_relationships
		  WHERE invitee_user_id = ?`,
		userID,
	))
}

func (r *InviteRepository) GetRelationshipByFirstPaidOrderNoTx(tx *sql.Tx, orderNo string) (*model.InviteRelationship, error) {
	return r.getRelationshipByInvitee(tx.QueryRow(
		`SELECT id, inviter_user_id, inviter_code, invitee_user_id, status,
		        first_paid_order_id, first_paid_order_no, first_paid_order_kind, first_paid_amount_cny_cent,
		        first_paid_at, rewarded_at, last_reversed_at, created_at, updated_at
		   FROM invite_relationships
		  WHERE first_paid_order_no = ?`,
		orderNo,
	))
}

func (r *InviteRepository) MarkRewardedTx(tx *sql.Tx, relationID string, order *model.PurchaseOrder, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE invite_relationships
		    SET status = ?, first_paid_order_id = ?, first_paid_order_no = ?, first_paid_order_kind = ?,
		        first_paid_amount_cny_cent = ?, first_paid_at = ?, rewarded_at = ?, updated_at = ?
		  WHERE id = ?`,
		model.InviteRelationshipStatusRewarded,
		order.ID,
		order.OrderNo,
		order.OrderKind,
		order.AmountCNYCent,
		now,
		now,
		now,
		relationID,
	)
	return err
}

func (r *InviteRepository) ResetAfterRefundTx(tx *sql.Tx, relationID string, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE invite_relationships
		    SET status = ?, first_paid_order_id = '', first_paid_order_no = '', first_paid_order_kind = '',
		        first_paid_amount_cny_cent = 0, first_paid_at = NULL, rewarded_at = NULL, last_reversed_at = ?, updated_at = ?
		  WHERE id = ?`,
		model.InviteRelationshipStatusPending,
		now,
		now,
		relationID,
	)
	return err
}

func (r *InviteRepository) CreateRewardEventTx(tx *sql.Tx, event *model.InviteRewardEvent) error {
	_, err := tx.Exec(
		`INSERT INTO invite_reward_events (
			id, relation_id, beneficiary_user_id, beneficiary_role, order_id, order_no,
			status, amount_micros, order_paid_amount_cny_cent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.RelationID,
		event.BeneficiaryUserID,
		event.BeneficiaryRole,
		event.OrderID,
		event.OrderNo,
		event.Status,
		event.AmountMicros,
		event.OrderPaidAmountCNYCent,
		event.CreatedAt,
	)
	return err
}

func (r *InviteRepository) HasRewardEventTx(tx *sql.Tx, relationID string, role model.InviteRewardBeneficiaryRole, status model.InviteRewardEventStatus, orderNo string) (bool, error) {
	var count int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM invite_reward_events WHERE relation_id = ? AND beneficiary_role = ? AND status = ? AND order_no = ?`,
		relationID, role, status, orderNo,
	).Scan(&count)
	return count > 0, err
}

func (r *InviteRepository) ListRewardEventsByRelationOrderTx(tx *sql.Tx, relationID, orderNo string, status model.InviteRewardEventStatus) ([]*model.InviteRewardEvent, error) {
	rows, err := tx.Query(
		`SELECT id, relation_id, beneficiary_user_id, beneficiary_role, order_id, order_no, status, amount_micros, order_paid_amount_cny_cent, created_at
		   FROM invite_reward_events
		  WHERE relation_id = ? AND order_no = ? AND status = ?
		  ORDER BY created_at ASC`,
		relationID, orderNo, status,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.InviteRewardEvent, 0)
	for rows.Next() {
		item := &model.InviteRewardEvent{}
		if err := rows.Scan(
			&item.ID,
			&item.RelationID,
			&item.BeneficiaryUserID,
			&item.BeneficiaryRole,
			&item.OrderID,
			&item.OrderNo,
			&item.Status,
			&item.AmountMicros,
			&item.OrderPaidAmountCNYCent,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InviteRepository) ListRewardEventsByUser(userID string, limit int) ([]*model.InviteRewardEventResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(
		`SELECT e.id, e.relation_id, e.beneficiary_user_id, COALESCE(u.username, ''), e.beneficiary_role,
		        e.order_no, e.status, e.amount_micros, e.order_paid_amount_cny_cent, e.created_at
		   FROM invite_reward_events e
		   LEFT JOIN users u ON u.id = e.beneficiary_user_id
		  WHERE e.beneficiary_user_id = ?
		  ORDER BY e.created_at DESC
		  LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.InviteRewardEventResponse, 0)
	for rows.Next() {
		item := &model.InviteRewardEventResponse{}
		if err := rows.Scan(
			&item.ID,
			&item.RelationID,
			&item.BeneficiaryUserID,
			&item.BeneficiaryUsername,
			&item.BeneficiaryRole,
			&item.OrderNo,
			&item.Status,
			&item.AmountMicros,
			&item.OrderPaidAmountCNYCent,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InviteRepository) GetSummary(userID string) (*model.InviteSummaryResponse, error) {
	db := database.GetDB()
	summary := &model.InviteSummaryResponse{}
	if err := db.QueryRow(`SELECT COALESCE(invite_code, '') FROM users WHERE id = ?`, userID).Scan(&summary.InviteCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if err := db.QueryRow(
		`SELECT COUNT(*),
		        SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status = ? THEN 1 ELSE 0 END)
		   FROM invite_relationships
		  WHERE inviter_user_id = ?`,
		model.InviteRelationshipStatusPending,
		model.InviteRelationshipStatusRewarded,
		userID,
	).Scan(&summary.InvitedUsers, &summary.PendingInvites, &summary.RewardedInvites); err != nil {
		return nil, err
	}
	if err := db.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN status = 'granted' THEN amount_micros ELSE -amount_micros END), 0)
		   FROM invite_reward_events
		  WHERE beneficiary_user_id = ?`,
		userID,
	).Scan(&summary.TotalRewardMicros); err != nil {
		return nil, err
	}
	if err := db.QueryRow(
		`SELECT COALESCE(r.inviter_user_id, ''), COALESCE(u.username, ''), COALESCE(r.inviter_code, '')
		   FROM invite_relationships r
		   LEFT JOIN users u ON u.id = r.inviter_user_id
		  WHERE r.invitee_user_id = ?`,
		userID,
	).Scan(&summary.InvitedByUserID, &summary.InvitedByUsername, &summary.BoundInviteCode); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return summary, nil
}

func (r *InviteRepository) ListRelations(limit int) ([]*model.InviteRelationshipAdminResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(
		`SELECT r.id, r.inviter_user_id, COALESCE(inviter.username, ''), r.inviter_code,
		        r.invitee_user_id, COALESCE(invitee.username, ''), r.status, r.first_paid_order_no,
		        r.first_paid_order_kind, r.first_paid_amount_cny_cent, r.first_paid_at, r.rewarded_at,
		        r.last_reversed_at, r.created_at
		   FROM invite_relationships r
		   LEFT JOIN users inviter ON inviter.id = r.inviter_user_id
		   LEFT JOIN users invitee ON invitee.id = r.invitee_user_id
		  ORDER BY r.created_at DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.InviteRelationshipAdminResponse, 0)
	for rows.Next() {
		item := &model.InviteRelationshipAdminResponse{}
		if err := rows.Scan(
			&item.ID,
			&item.InviterUserID,
			&item.InviterUsername,
			&item.InviterCode,
			&item.InviteeUserID,
			&item.InviteeUsername,
			&item.Status,
			&item.FirstPaidOrderNo,
			&item.FirstPaidOrderKind,
			&item.FirstPaidAmountCNYCent,
			&item.FirstPaidAt,
			&item.RewardedAt,
			&item.LastReversedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InviteRepository) ListRewardEvents(limit int) ([]*model.InviteRewardEventResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(
		`SELECT e.id, e.relation_id, e.beneficiary_user_id, COALESCE(u.username, ''), e.beneficiary_role,
		        e.order_no, e.status, e.amount_micros, e.order_paid_amount_cny_cent, e.created_at
		   FROM invite_reward_events e
		   LEFT JOIN users u ON u.id = e.beneficiary_user_id
		  ORDER BY e.created_at DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.InviteRewardEventResponse, 0)
	for rows.Next() {
		item := &model.InviteRewardEventResponse{}
		if err := rows.Scan(
			&item.ID,
			&item.RelationID,
			&item.BeneficiaryUserID,
			&item.BeneficiaryUsername,
			&item.BeneficiaryRole,
			&item.OrderNo,
			&item.Status,
			&item.AmountMicros,
			&item.OrderPaidAmountCNYCent,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InviteRepository) GetStats() (*model.InviteStatsResponse, error) {
	db := database.GetDB()
	stats := &model.InviteStatsResponse{}
	if err := db.QueryRow(
		`SELECT COUNT(*),
		        SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status = ? THEN 1 ELSE 0 END)
		   FROM invite_relationships`,
		model.InviteRelationshipStatusPending,
		model.InviteRelationshipStatusRewarded,
	).Scan(&stats.RelationsTotal, &stats.PendingTotal, &stats.RewardedTotal); err != nil {
		return nil, err
	}
	if err := db.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN status = 'granted' THEN amount_micros ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'reversed' THEN amount_micros ELSE 0 END), 0)
		   FROM invite_reward_events`,
	).Scan(&stats.GrantedRewardMicros, &stats.ReversedRewardMicros); err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *InviteRepository) getRelationshipByInvitee(row *sql.Row) (*model.InviteRelationship, error) {
	item := &model.InviteRelationship{}
	err := row.Scan(
		&item.ID,
		&item.InviterUserID,
		&item.InviterCode,
		&item.InviteeUserID,
		&item.Status,
		&item.FirstPaidOrderID,
		&item.FirstPaidOrderNo,
		&item.FirstPaidOrderKind,
		&item.FirstPaidAmountCNYCent,
		&item.FirstPaidAt,
		&item.RewardedAt,
		&item.LastReversedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}
