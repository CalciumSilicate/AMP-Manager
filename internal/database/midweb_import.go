package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"ampmanager/internal/model"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type MidwebImportParams struct {
	SourceSQLitePath string
	Target           Options
	ClearTarget      bool
	OnProgress       func(MigrationProgress)
}

const (
	midwebBalanceTopupPlanID    = "system-balance-topup-plan"
	midwebBalanceTopupProductID = "system-balance-topup-product"
)

type midwebTemplateRow struct {
	ID              int64
	Name            string
	DurationDays    sql.NullInt64
	DailyQuotaUSD   sql.NullFloat64
	WeeklyQuotaUSD  sql.NullFloat64
	MonthlyQuotaUSD sql.NullFloat64
	TotalQuotaUSD   sql.NullFloat64
	DailyResetMode  string
	DailyResetTime  string
	TemplateKind    string
}

type midwebBoundCDKRow struct {
	ID                 int64
	CodeValue          string
	TemplateID         int64
	UsedAt             sql.NullString
	RemoteUserID       int64
	RemoteUserName     string
	RemoteAPIKey       string
	SnapshotExpiresAt  sql.NullString
	SnapshotLimitTotal sql.NullFloat64
	TemplateKind       string
	DurationDays       sql.NullInt64
	DailyQuotaUSD      sql.NullFloat64
}

type midwebRechargeRow struct {
	ID                  int64
	TargetCDKID         int64
	RemoteUserID        int64
	Mode                string
	Status              string
	SourceDailyQuotaUSD sql.NullFloat64
	TargetDailyQuotaUSD sql.NullFloat64
	PeakDailyQuotaUSD   sql.NullFloat64
	TargetExpiresBefore sql.NullString
	TargetExpiresAfter  sql.NullString
	PreviewJSON         string
	CreatedAt           sql.NullString
	ConfirmedAt         sql.NullString
	AppliedAt           sql.NullString
}

type midwebRechargePhaseRow struct {
	ID             int64
	RechargeID     int64
	TargetCDKID    int64
	PhaseType      string
	StartAt        sql.NullString
	EndAt          sql.NullString
	DailyQuotaUSD  sql.NullFloat64
	RechargeStatus string
	PreviewJSON    string
}

type midwebResetRow struct {
	ID                 int64
	CDKID              int64
	UsageDateCN        string
	BeforeExpiresAt    sql.NullString
	AfterExpiresAt     sql.NullString
	BeforeDailyUsedUSD sql.NullFloat64
}

type midwebPurchaseSubscriptionRow struct {
	ID         int64
	Name       string
	TemplateID int64
	PriceCNY   sql.NullFloat64
	Enabled    bool
}

type midwebPurchaseOrderRow struct {
	ID                 int64
	OrderNo            string
	OrderKind          string
	Email              string
	TemplateID         sql.NullInt64
	TargetRemoteUserID sql.NullInt64
	AmountCNY          sql.NullFloat64
	MeteredAmountUSD   sql.NullFloat64
	PaymentStatus      string
	CreatedAt          sql.NullString
	PaidAt             sql.NullString
}

type midwebDeliveryRow struct {
	ID           int64
	OrderID      int64
	CDKID        int64
	DeliveryCDK  string
	TemplateID   sql.NullInt64
	DurationDays sql.NullInt64
	TemplateKind sql.NullString
	UsedAt       sql.NullString
	RemoteUserID sql.NullInt64
}

type preservedAdmin struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type importedUserRef struct {
	UserID         string
	SubscriptionID string
	PlanID         string
	CodeValue      string
	TemplateID     int64
	TemplateKind   string
}

type importedUserIndex struct {
	ByCDKID        map[int64]importedUserRef
	ByRemoteUserID map[int64]importedUserRef
}

var midwebImportTables = []string{
	"purchase_manual_settlement_batch_items",
	"purchase_manual_settlement_batches",
	"purchase_order_payment_status_history",
	"purchase_webhook_events",
	"purchase_webhook_targets",
	"purchase_orders",
	"purchase_products",
	"redeem_redemptions",
	"redeem_user_counters",
	"redeem_codes",
	"redeem_code_batches",
	"redeem_campaigns",
	"billing_daily_reset_records",
	"billing_events",
	"billing_projection_events",
	"billing_reservations",
	"subscription_window_state",
	"billing_account_state",
	"user_billing_settings",
	"subscription_entitlements",
	"subscription_timeline_phases",
	"subscription_recharge_history",
	"user_subscriptions",
	"subscription_plan_limits",
	"subscription_plans",
	"user_api_keys",
	"user_amp_settings",
	"user_groups",
	"users",
}

func ImportMidwebSQLite(params MidwebImportParams) error {
	sourcePath := strings.TrimSpace(params.SourceSQLitePath)
	if sourcePath == "" {
		return fmt.Errorf("midweb import requires --source")
	}
	if info, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("midweb source sqlite not found: %w", err)
	} else if info.IsDir() {
		return fmt.Errorf("midweb source sqlite path is a directory: %s", sourcePath)
	}
	targetOptions, err := params.Target.Normalize()
	if err != nil {
		return err
	}

	reportMigrationProgress(params.OnProgress, 5, "打开 Midweb SQLite")
	sourceDB, err := OpenWithOptions(Options{Type: DBTypeSQLite, SQLitePath: sourcePath})
	if err != nil {
		return err
	}
	defer sourceDB.Close()
	if ok, err := hasTableOnDB(sourceDB, "cdk_templates"); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("source sqlite is not a Midweb database: missing table cdk_templates")
	}
	if ok, err := hasTableOnDB(sourceDB, "cdks"); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("source sqlite is not a Midweb database: missing table cdks")
	}

	reportMigrationProgress(params.OnProgress, 15, "初始化目标数据库")
	targetDB, err := prepareStandaloneDatabase(targetOptions)
	if err != nil {
		return err
	}
	defer targetDB.Close()

	admins, err := snapshotAdminUsers(targetDB)
	if err != nil {
		return err
	}
	if params.ClearTarget {
		reportMigrationProgress(params.OnProgress, 20, "清空现有用户/计费/购买域")
		if err := clearNamedTablesOnDB(targetDB, targetOptions, midwebImportTables); err != nil {
			return err
		}
		if err := restoreAdminUsers(targetDB, admins); err != nil {
			return err
		}
	}

	reportMigrationProgress(params.OnProgress, 30, "读取 Midweb 模板和绑定账号")
	templates, err := loadMidwebTemplates(sourceDB)
	if err != nil {
		return err
	}
	boundCDKs, err := loadMidwebBoundCDKs(sourceDB)
	if err != nil {
		return err
	}
	recharges, err := loadMidwebRecharges(sourceDB)
	if err != nil {
		return err
	}
	phases, err := loadMidwebRechargePhases(sourceDB)
	if err != nil {
		return err
	}
	resets, err := loadMidwebQuotaResets(sourceDB)
	if err != nil {
		return err
	}
	purchaseSubscriptions, err := loadMidwebPurchaseSubscriptions(sourceDB)
	if err != nil {
		return err
	}
	purchaseOrders, err := loadMidwebPurchaseOrders(sourceDB)
	if err != nil {
		return err
	}
	deliveries, err := loadMidwebPurchaseDeliveries(sourceDB)
	if err != nil {
		return err
	}
	dailyRechargeLimitUSD, _ := loadMidwebDailyRechargeLimit(sourceDB)

	tx, err := targetDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if dailyRechargeLimitUSD > 0 {
		if _, err := tx.Exec(
			`INSERT INTO system_config (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			"midweb_daily_recharge_limit_usd",
			fmt.Sprintf("%.6f", dailyRechargeLimitUSD),
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}

	reportMigrationProgress(params.OnProgress, 45, "导入套餐与商品")
	planIDs, _, err := importMidwebPlansAndProducts(tx, templates, purchaseSubscriptions)
	if err != nil {
		return err
	}

	reportMigrationProgress(params.OnProgress, 60, "导入绑定账号与 API Key")
	importedUsers, err := importMidwebUsers(tx, boundCDKs, templates, planIDs, recharges)
	if err != nil {
		return err
	}

	legacyOrderOwnerID, err := ensureLegacyOrderOwner(tx, len(purchaseOrders) > 0)
	if err != nil {
		return err
	}

	reportMigrationProgress(params.OnProgress, 72, "导入充值时间线与重置历史")
	if err := importMidwebRechargeHistory(tx, recharges, phases, resets, importedUsers); err != nil {
		return err
	}

	reportMigrationProgress(params.OnProgress, 84, "导入历史订单与未兑礼品码")
	if err := importMidwebOrdersAndCodes(tx, purchaseOrders, deliveries, templates, planIDs, importedUsers, legacyOrderOwnerID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	reportMigrationProgress(params.OnProgress, 95, "Midweb 导入完成")
	return nil
}

func snapshotAdminUsers(db *sql.DB) ([]preservedAdmin, error) {
	rows, err := db.Query(`SELECT id, username, password_hash, created_at, updated_at FROM users WHERE is_admin = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]preservedAdmin, 0)
	for rows.Next() {
		var item preservedAdmin
		if err := rows.Scan(&item.ID, &item.Username, &item.PasswordHash, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func restoreAdminUsers(db *sql.DB, items []preservedAdmin) error {
	for _, item := range items {
		if _, err := db.Exec(
			`INSERT INTO users (id, username, password_hash, is_admin, balance_micros, concurrency_limit, must_change_password, must_change_username, legacy_source, legacy_ref_id, created_at, updated_at)
			 VALUES (?, ?, ?, 1, 0, 0, 0, 0, '', '', ?, ?)
			 ON CONFLICT (id) DO UPDATE SET username = excluded.username, password_hash = excluded.password_hash, is_admin = 1, updated_at = excluded.updated_at`,
			item.ID, item.Username, item.PasswordHash, item.CreatedAt, item.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func clearNamedTablesOnDB(targetDB *sql.DB, options Options, tables []string) error {
	reversed := append([]string{}, tables...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	for _, tableName := range reversed {
		statement := "DELETE FROM " + tableName
		if options.Type == DBTypePostgres {
			statement = "TRUNCATE TABLE " + tableName + " RESTART IDENTITY CASCADE"
		}
		if _, err := targetDB.Exec(statement); err != nil {
			return fmt.Errorf("clear %s: %w", tableName, err)
		}
	}
	return nil
}

func hasTableOnDB(db *sql.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func importMidwebPlansAndProducts(tx *sql.Tx, templates map[int64]midwebTemplateRow, subscriptions []midwebPurchaseSubscriptionRow) (map[int64]string, map[int64]midwebTemplateRow, error) {
	planIDs := make(map[int64]string, len(templates))
	for _, template := range templates {
		if template.TemplateKind != "daily" {
			continue
		}
		planID := midwebPlanID(template.ID)
		planIDs[template.ID] = planID
		if _, err := tx.Exec(
			`INSERT INTO subscription_plans (id, name, description, enabled, upgrade_rank, upgrade_valuation_cny_cent_per_day, legacy_source, legacy_ref_id, created_at, updated_at)
			 VALUES (?, ?, ?, 1, 0, 0, 'midweb_template', ?, ?, ?)
			 ON CONFLICT (id) DO UPDATE SET name = excluded.name, description = excluded.description, enabled = 1, legacy_source = excluded.legacy_source, legacy_ref_id = excluded.legacy_ref_id, updated_at = excluded.updated_at`,
			planID,
			template.Name,
			fmt.Sprintf("从 Midweb 模板 %d 导入", template.ID),
			fmt.Sprintf("%d", template.ID),
			time.Now().UTC(),
			time.Now().UTC(),
		); err != nil {
			return nil, nil, err
		}
		limits, err := buildPlanLimitsFromTemplate(planID, template)
		if err != nil {
			return nil, nil, err
		}
		for _, limit := range limits {
			if _, err := tx.Exec(
				`INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros, fixed_reset_minute, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET limit_micros = excluded.limit_micros, fixed_reset_minute = excluded.fixed_reset_minute, updated_at = excluded.updated_at`,
				limit.ID, limit.PlanID, limit.LimitType, limit.WindowMode, limit.LimitMicros, limitFixedResetMinute(limit), time.Now().UTC(), time.Now().UTC(),
			); err != nil {
				return nil, nil, err
			}
		}
	}

	sort.Slice(subscriptions, func(i, j int) bool {
		return subscriptions[i].PriceCNY.Float64 < subscriptions[j].PriceCNY.Float64
	})
	for index, item := range subscriptions {
		template, ok := templates[item.TemplateID]
		if !ok || template.TemplateKind != "daily" {
			continue
		}
		planID := planIDs[item.TemplateID]
		if planID == "" {
			continue
		}
		priceCNYCent := int64(math.Round(item.PriceCNY.Float64 * 100))
		kinds := []struct {
			kind      model.PurchaseProductKind
			groupName string
			title     string
		}{
			{kind: model.PurchaseProductKindExtendDuration, groupName: "续期包", title: item.Name + " 续期"},
			{kind: model.PurchaseProductKindBoostQuota, groupName: "加额包", title: item.Name + " 加额"},
			{kind: model.PurchaseProductKindOverwrite, groupName: "换档包", title: item.Name + " 换档"},
		}
		limits, err := buildPlanLimitsFromTemplate(planID, template)
		if err != nil {
			return nil, nil, err
		}
		for kindIndex, kind := range kinds {
			snapshot, err := json.Marshal(model.PurchaseActionSnapshot{
				ProductKind:              kind.kind,
				PlanID:                   planID,
				DurationDays:             int(template.DurationDays.Int64),
				SourceDailyLimitMicros:   findLimitMicros(limits, model.LimitTypeDaily),
				SourceWeeklyLimitMicros:  findLimitMicros(limits, model.LimitTypeWeekly),
				SourceMonthlyLimitMicros: findLimitMicros(limits, model.LimitTypeMonthly),
				SourceRolling5hMicros:    findLimitMicros(limits, model.LimitTypeRolling5h),
				SourceTotalLimitMicros:   findLimitMicros(limits, model.LimitTypeTotal),
				FixedResetTime:           findLimitResetTime(limits, model.LimitTypeDaily),
				LegacyTemplateID:         fmt.Sprintf("%d", template.ID),
			})
			if err != nil {
				return nil, nil, err
			}
			productID := fmt.Sprintf("midweb-product-%s-%d", kind.kind, item.ID)
			if _, err := tx.Exec(
				`INSERT INTO purchase_products (id, name, summary, product_kind, subscription_plan_id, duration_days, price_cny_cent, action_snapshot_json, legacy_source, legacy_ref_id, group_name, group_sort, is_recommended, sort_order, enabled, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'midweb_purchase_subscription', ?, ?, ?, 0, ?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET name = excluded.name, summary = excluded.summary, product_kind = excluded.product_kind, subscription_plan_id = excluded.subscription_plan_id, duration_days = excluded.duration_days, price_cny_cent = excluded.price_cny_cent, action_snapshot_json = excluded.action_snapshot_json, group_name = excluded.group_name, group_sort = excluded.group_sort, sort_order = excluded.sort_order, enabled = excluded.enabled, updated_at = excluded.updated_at`,
				productID,
				kind.title,
				fmt.Sprintf("导入自 Midweb 公开商品 %s", item.Name),
				kind.kind,
				planID,
				int(template.DurationDays.Int64),
				priceCNYCent,
				string(snapshot),
				fmt.Sprintf("%d", item.ID),
				kind.groupName,
				kindIndex,
				index,
				item.Enabled,
				time.Now().UTC(),
				time.Now().UTC(),
			); err != nil {
				return nil, nil, err
			}
		}
	}
	return planIDs, templates, nil
}

func importMidwebUsers(tx *sql.Tx, rows []midwebBoundCDKRow, templates map[int64]midwebTemplateRow, planIDs map[int64]string, recharges []midwebRechargeRow) (*importedUserIndex, error) {
	latestRechargeExpiryByCDK := make(map[int64]*time.Time)
	for _, item := range recharges {
		if item.TargetCDKID == 0 {
			continue
		}
		if parsed := parseMidwebTime(item.TargetExpiresAfter.String); parsed != nil {
			if current := latestRechargeExpiryByCDK[item.TargetCDKID]; current == nil || parsed.After(*current) {
				latestRechargeExpiryByCDK[item.TargetCDKID] = parsed
			}
		}
	}

	imported := &importedUserIndex{
		ByCDKID:        make(map[int64]importedUserRef, len(rows)),
		ByRemoteUserID: make(map[int64]importedUserRef, len(rows)),
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, row := range rows {
		userID := fmt.Sprintf("midweb-user-%d", row.ID)
		ref := importedUserRef{
			UserID:       userID,
			PlanID:       planIDs[row.TemplateID],
			CodeValue:    row.CodeValue,
			TemplateID:   row.TemplateID,
			TemplateKind: row.TemplateKind,
		}
		balanceMicros := int64(0)
		primarySource := model.BillingSourceSubscription
		secondarySource := model.BillingSourceBalance
		mustChangePassword := true
		mustChangeUsername := true
		if row.TemplateKind == "metered" {
			balanceMicros = microsFromUSD(row.SnapshotLimitTotal.Float64)
			primarySource = model.BillingSourceBalance
			secondarySource = model.BillingSourceSubscription
		}
		if _, err := tx.Exec(
			`INSERT INTO users (id, username, password_hash, is_admin, balance_micros, concurrency_limit, must_change_password, must_change_username, legacy_source, legacy_ref_id, created_at, updated_at)
			 VALUES (?, ?, ?, 0, ?, 0, ?, ?, 'midweb_cdk', ?, ?, ?)
			 ON CONFLICT (id) DO UPDATE SET username = excluded.username, password_hash = excluded.password_hash, balance_micros = excluded.balance_micros, must_change_password = excluded.must_change_password, must_change_username = excluded.must_change_username, legacy_source = excluded.legacy_source, legacy_ref_id = excluded.legacy_ref_id, updated_at = excluded.updated_at`,
			userID,
			row.CodeValue,
			string(passwordHash),
			balanceMicros,
			mustChangePassword,
			mustChangeUsername,
			fmt.Sprintf("%d", row.ID),
			now,
			now,
		); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(
			`INSERT INTO user_billing_settings (user_id, primary_source, secondary_source, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (user_id) DO UPDATE SET primary_source = excluded.primary_source, secondary_source = excluded.secondary_source, updated_at = excluded.updated_at`,
			userID, primarySource, secondarySource, now, now,
		); err != nil {
			return nil, err
		}
		keyHash := sha256Hex(row.RemoteAPIKey)
		prefix := row.RemoteAPIKey
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		if _, err := tx.Exec(
			`INSERT INTO user_api_keys (id, user_id, name, prefix, key_hash, api_key, expires_at, created_at)
			 VALUES (?, ?, 'default', ?, ?, ?, NULL, ?)
			 ON CONFLICT (id) DO UPDATE SET prefix = excluded.prefix, key_hash = excluded.key_hash, api_key = excluded.api_key`,
			fmt.Sprintf("midweb-key-%d-default", row.ID),
			userID,
			prefix,
			keyHash,
			row.RemoteAPIKey,
			now,
		); err != nil {
			return nil, err
		}

		if row.TemplateKind == "daily" {
			subID := fmt.Sprintf("midweb-sub-%d", row.ID)
			ref.SubscriptionID = subID
			expiresAt := deriveMidwebSubscriptionExpiry(row, latestRechargeExpiryByCDK[row.ID], templates[row.TemplateID])
			status := model.SubscriptionStatusActive
			if expiresAt != nil && !expiresAt.After(now) {
				status = model.SubscriptionStatusExpired
			}
			startsAt := parseMidwebTime(row.UsedAt.String)
			if startsAt == nil {
				startsAt = &now
			}
			if _, err := tx.Exec(
				`INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT (id) DO UPDATE SET plan_id = excluded.plan_id, starts_at = excluded.starts_at, expires_at = excluded.expires_at, status = excluded.status, updated_at = excluded.updated_at`,
				subID,
				userID,
				ref.PlanID,
				startsAt.UTC(),
				expiresAt,
				status,
				now,
				now,
			); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(
				`INSERT INTO subscription_entitlements (id, user_id, plan_id, source_type, source_ref_id, valuation_cny_cent_per_day, starts_at, expires_at, status, consumed_by_purchase_order_no, consumed_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, '', NULL, ?, ?)
				 ON CONFLICT (id) DO NOTHING`,
				fmt.Sprintf("midweb-ent-%d", row.ID),
				userID,
				ref.PlanID,
				model.SubscriptionEntitlementSourceLegacySnapshot,
				fmt.Sprintf("%d", row.ID),
				startsAt.UTC(),
				expiresAt,
				model.SubscriptionEntitlementStatusActive,
				now,
				now,
			); err != nil {
				return nil, err
			}
		}

		imported.ByCDKID[row.ID] = ref
		if _, exists := imported.ByRemoteUserID[row.RemoteUserID]; !exists {
			imported.ByRemoteUserID[row.RemoteUserID] = ref
		}
	}
	return imported, nil
}

func importMidwebRechargeHistory(tx *sql.Tx, recharges []midwebRechargeRow, phases []midwebRechargePhaseRow, resets []midwebResetRow, users *importedUserIndex) error {
	now := time.Now().UTC()
	for _, item := range recharges {
		ref, ok := users.ByCDKID[item.TargetCDKID]
		if !ok {
			ref, ok = users.ByRemoteUserID[item.RemoteUserID]
		}
		if !ok || ref.SubscriptionID == "" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO subscription_recharge_history (
				id, user_id, user_subscription_id, plan_id, mode, status, source_type, source_ref_id,
				source_daily_limit_micros, target_daily_limit_before_micros, peak_daily_limit_micros,
				target_expires_at_before, target_expires_at_after, preview_json, confirmed_at, applied_at,
				legacy_source, legacy_ref_id, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'midweb_recharge', ?, ?, ?, ?, ?, ?, ?, ?, ?, 'midweb_recharge', ?, ?, ?)
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("midweb-recharge-%d", item.ID),
			ref.UserID,
			ref.SubscriptionID,
			ref.PlanID,
			item.Mode,
			item.Status,
			fmt.Sprintf("%d", item.ID),
			nullMicrosFromUSD(item.SourceDailyQuotaUSD),
			nullMicrosFromUSD(item.TargetDailyQuotaUSD),
			nullMicrosFromUSD(item.PeakDailyQuotaUSD),
			parseMidwebTime(item.TargetExpiresBefore.String),
			parseMidwebTime(item.TargetExpiresAfter.String),
			item.PreviewJSON,
			parseMidwebTime(item.ConfirmedAt.String),
			parseMidwebTime(item.AppliedAt.String),
			fmt.Sprintf("%d", item.ID),
			parseMidwebTime(item.CreatedAt.String),
			now,
		); err != nil {
			return err
		}
	}

	rechargeByID := make(map[int64]midwebRechargeRow, len(recharges))
	for _, item := range recharges {
		rechargeByID[item.ID] = item
	}
	for _, item := range phases {
		recharge, ok := rechargeByID[item.RechargeID]
		if !ok {
			continue
		}
		ref, ok := users.ByCDKID[recharge.TargetCDKID]
		if !ok {
			ref, ok = users.ByRemoteUserID[recharge.RemoteUserID]
		}
		if !ok || ref.SubscriptionID == "" {
			continue
		}
		startAt := parseMidwebTime(item.StartAt.String)
		endAt := parseMidwebTime(item.EndAt.String)
		if startAt == nil || endAt == nil {
			continue
		}
		status := phaseStatusForImport(*startAt, *endAt, recharge.Status, now)
		if _, err := tx.Exec(
			`INSERT INTO subscription_timeline_phases (
				id, user_id, user_subscription_id, plan_id, phase_type, status, source_type, source_ref_id,
				daily_limit_micros, fixed_reset_minute, starts_at, ends_at, final_expires_at, preview_json, applied_at,
				legacy_source, legacy_ref_id, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'midweb_recharge', ?, ?, ?, ?, ?, ?, ?, ?, 'midweb_recharge_phase', ?, ?, ?)
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("midweb-phase-%d", item.ID),
			ref.UserID,
			ref.SubscriptionID,
			ref.PlanID,
			item.PhaseType,
			status,
			fmt.Sprintf("%d", recharge.ID),
			nullMicrosFromUSD(item.DailyQuotaUSD),
			0,
			startAt.UTC(),
			endAt.UTC(),
			parseMidwebTime(recharge.TargetExpiresAfter.String),
			recharge.PreviewJSON,
			parseMidwebTime(recharge.AppliedAt.String),
			fmt.Sprintf("%d", item.ID),
			startAt.UTC(),
			now,
		); err != nil {
			return err
		}
	}

	for _, item := range resets {
		ref, ok := users.ByCDKID[item.CDKID]
		if !ok || ref.SubscriptionID == "" {
			continue
		}
		windowStart, windowEnd := midwebUsageDateBounds(item.UsageDateCN)
		if _, err := tx.Exec(
			`INSERT INTO billing_daily_reset_records (
				id, user_id, user_subscription_id, window_start, window_end, used_micros_before_reset, expires_at_before, expires_at_after, legacy_source, legacy_ref_id, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'midweb_quota_reset', ?, ?)
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("midweb-reset-%d", item.ID),
			ref.UserID,
			ref.SubscriptionID,
			windowStart,
			windowEnd,
			microsFromUSD(item.BeforeDailyUsedUSD.Float64),
			parseMidwebTime(item.BeforeExpiresAt.String),
			parseMidwebTime(item.AfterExpiresAt.String),
			fmt.Sprintf("%d", item.ID),
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return nil
}

func importMidwebOrdersAndCodes(tx *sql.Tx, orders []midwebPurchaseOrderRow, deliveries []midwebDeliveryRow, templates map[int64]midwebTemplateRow, planByTemplateID map[int64]string, users *importedUserIndex, legacyOrderOwnerID string) error {
	if err := ensureMidwebBalanceTopupPlaceholderTx(tx); err != nil {
		return err
	}
	deliveryIDsByOrder := make(map[int64][]string)
	for _, item := range deliveries {
		template, ok := templates[item.TemplateID.Int64]
		if !ok {
			continue
		}
		planID := planByTemplateID[item.TemplateID.Int64]
		snapshot := model.PurchaseActionSnapshot{
			ProductKind:              model.PurchaseProductKindOverwrite,
			PlanID:                   planID,
			DurationDays:             int(item.DurationDays.Int64),
			SourceDailyLimitMicros:   microsFromNullableUSD(template.DailyQuotaUSD),
			SourceWeeklyLimitMicros:  microsFromNullableUSD(template.WeeklyQuotaUSD),
			SourceMonthlyLimitMicros: microsFromNullableUSD(template.MonthlyQuotaUSD),
			SourceTotalLimitMicros:   microsFromNullableUSD(template.TotalQuotaUSD),
			LegacyTemplateID:         fmt.Sprintf("%d", item.TemplateID.Int64),
		}
		subscriptionPlanID := planID
		subscriptionDurationDays := int(item.DurationDays.Int64)
		balanceMicros := int64(0)
		if template.TemplateKind == "metered" {
			subscriptionPlanID = midwebBalanceTopupPlanID
			subscriptionDurationDays = 1
			snapshot = model.PurchaseActionSnapshot{
				ProductKind:        model.PurchaseProductKindBalanceTopup,
				BalanceTopupMicros: microsFromNullableUSD(template.TotalQuotaUSD),
				LegacyTemplateID:   fmt.Sprintf("%d", item.TemplateID.Int64),
			}
			balanceMicros = microsFromNullableUSD(template.TotalQuotaUSD)
		} else if template.TemplateKind != "daily" {
			continue
		}
		encodedSnapshot, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		codeID := fmt.Sprintf("midweb-delivery-code-%d", item.ID)
		codeStatus := model.RedeemCodeStatusActive
		redeemedCount := 0
		var lastRedeemedAt any = nil
		if item.RemoteUserID.Valid || strings.TrimSpace(item.UsedAt.String) != "" {
			codeStatus = model.RedeemCodeStatusConsumed
			redeemedCount = 1
			lastRedeemedAt = parseMidwebTime(item.UsedAt.String)
		}
		if _, err := tx.Exec(
			`INSERT INTO redeem_codes (
				id, campaign_id, batch_id, source_type, source_ref_id, code_value, code_hash, code_mask,
				subscription_plan_id, subscription_duration_days, balance_micros, reward_snapshot_json, per_user_limit, starts_at, ends_at,
				status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
			) VALUES (?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, NULL, NULL, ?, 1, ?, ?, ?, ?)
			ON CONFLICT (id) DO NOTHING`,
			codeID,
			model.RedeemCodeSourceTypePurchaseOrder,
			fmt.Sprintf("%d", item.OrderID),
			item.DeliveryCDK,
			sha256Hex(item.DeliveryCDK),
			maskCodeValue(item.DeliveryCDK),
			subscriptionPlanID,
			subscriptionDurationDays,
			balanceMicros,
			string(encodedSnapshot),
			codeStatus,
			redeemedCount,
			lastRedeemedAt,
			time.Now().UTC(),
			time.Now().UTC(),
		); err != nil {
			return err
		}
		deliveryIDsByOrder[item.OrderID] = append(deliveryIDsByOrder[item.OrderID], codeID)
	}

	for _, item := range orders {
		userID := legacyOrderOwnerID
		orderKind := model.PurchaseOrderKindSubscription
		deliveryMode := model.PurchaseDeliveryModeRedeemCode
		productID := ""
		subscriptionPlanID := ""
		durationDays := 1
		balanceTopupMicros := int64(0)
		actionSnapshotJSON := ""
		if item.OrderKind == "metered_topup" {
			orderKind = model.PurchaseOrderKindBalanceTopup
			deliveryMode = model.PurchaseDeliveryModeAccount
			productID = midwebBalanceTopupProductID
			subscriptionPlanID = midwebBalanceTopupPlanID
			if item.TargetRemoteUserID.Valid {
				if ref, ok := users.ByRemoteUserID[item.TargetRemoteUserID.Int64]; ok {
					userID = ref.UserID
				}
			}
			balanceTopupMicros = microsFromUSD(item.MeteredAmountUSD.Float64)
		} else if item.TemplateID.Valid {
			subscriptionPlanID = planByTemplateID[item.TemplateID.Int64]
			productID = fmt.Sprintf("midweb-legacy-order-product-%d", item.TemplateID.Int64)
			if template, ok := templates[item.TemplateID.Int64]; ok {
				if template.DurationDays.Valid {
					durationDays = int(template.DurationDays.Int64)
				}
				if template.TemplateKind == "metered" {
					subscriptionPlanID = midwebBalanceTopupPlanID
					orderKind = model.PurchaseOrderKindBalanceTopup
					deliveryMode = model.PurchaseDeliveryModeRedeemCode
					durationDays = 1
					balanceTopupMicros = microsFromNullableUSD(template.TotalQuotaUSD)
				}
				if err := ensureMidwebLegacyOrderProductTx(tx, productID, subscriptionPlanID, template, item); err != nil {
					return err
				}
				actionSnapshot := model.PurchaseActionSnapshot{
					ProductKind:              model.PurchaseProductKindOverwrite,
					PlanID:                   subscriptionPlanID,
					DurationDays:             durationDays,
					SourceDailyLimitMicros:   microsFromNullableUSD(template.DailyQuotaUSD),
					SourceWeeklyLimitMicros:  microsFromNullableUSD(template.WeeklyQuotaUSD),
					SourceMonthlyLimitMicros: microsFromNullableUSD(template.MonthlyQuotaUSD),
					SourceTotalLimitMicros:   microsFromNullableUSD(template.TotalQuotaUSD),
					LegacyTemplateID:         fmt.Sprintf("%d", item.TemplateID.Int64),
				}
				if template.TemplateKind == "metered" {
					actionSnapshot = model.PurchaseActionSnapshot{
						ProductKind:        model.PurchaseProductKindBalanceTopup,
						BalanceTopupMicros: balanceTopupMicros,
						LegacyTemplateID:   fmt.Sprintf("%d", item.TemplateID.Int64),
					}
				}
				snapshot, err := json.Marshal(actionSnapshot)
				if err != nil {
					return err
				}
				actionSnapshotJSON = string(snapshot)
			}
		}
		if strings.TrimSpace(productID) == "" {
			continue
		}
		generatedCodeID := ""
		if ids := deliveryIDsByOrder[item.ID]; len(ids) == 1 {
			generatedCodeID = ids[0]
		}
		amountCents := int64(math.Round(item.AmountCNY.Float64 * 100))
		if amountCents == 0 {
			amountCents = 1
		}
		fulfillmentStatus := model.PurchaseFulfillmentStatusPending
		if strings.EqualFold(item.PaymentStatus, "paid") {
			if orderKind == model.PurchaseOrderKindBalanceTopup || len(deliveryIDsByOrder[item.ID]) > 0 {
				fulfillmentStatus = model.PurchaseFulfillmentStatusFulfilled
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO purchase_orders (
				id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent, order_kind, delivery_mode, balance_topup_micros,
				payment_channel, payment_status, fulfillment_status, generated_redeem_code_id, upgrade_source_plan_id, upgrade_source_expires_at,
				upgrade_credit_cny_cent, upgrade_locked_target_seconds, upgrade_state_token, action_snapshot_json, timeline_preview_json, legacy_source, legacy_ref_id,
				manual_settlement_done, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'alipay', ?, ?, ?, '', NULL, 0, 0, '', ?, '', 'midweb_purchase_order', ?, 0, '', '', '', NULL, ?, ?, '', ?, ?)
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("midweb-order-%d", item.ID),
			item.OrderNo,
			userID,
			productID,
			subscriptionPlanID,
			durationDays,
			amountCents,
			orderKind,
			deliveryMode,
			balanceTopupMicros,
			normalizePurchasePaymentStatus(item.PaymentStatus),
			fulfillmentStatus,
			generatedCodeID,
			actionSnapshotJSON,
			fmt.Sprintf("%d", item.ID),
			parseMidwebTime(item.PaidAt.String),
			parseMidwebTime(item.PaidAt.String),
			parseMidwebTime(item.CreatedAt.String),
			time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("insert legacy purchase_order %s failed (kind=%s template_id=%v product_id=%s subscription_plan_id=%s): %w", item.OrderNo, item.OrderKind, item.TemplateID, productID, subscriptionPlanID, err)
		}
	}
	return nil
}

func ensureLegacyOrderOwner(tx *sql.Tx, needed bool) (string, error) {
	if !needed {
		return "", nil
	}
	userID := "system-midweb-legacy-order-owner"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`INSERT INTO users (id, username, password_hash, is_admin, balance_micros, concurrency_limit, must_change_password, must_change_username, legacy_source, legacy_ref_id, created_at, updated_at)
		 VALUES (?, ?, ?, 0, 0, 0, 0, 0, 'midweb_order_owner', 'system', ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		userID, "legacy-order-owner", string(passwordHash), now, now,
	); err != nil {
		return "", err
	}
	return userID, nil
}

func ensureMidwebBalanceTopupPlaceholderTx(tx *sql.Tx) error {
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`INSERT INTO subscription_plans (id, name, description, enabled, upgrade_rank, upgrade_valuation_cny_cent_per_day, legacy_source, legacy_ref_id, created_at, updated_at)
		 VALUES (?, ?, ?, 0, 0, 0, 'midweb_placeholder', 'balance_topup', ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		midwebBalanceTopupPlanID,
		"余额充值",
		"Midweb 导入用余额占位套餐",
		now,
		now,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO purchase_products (id, name, summary, product_kind, subscription_plan_id, duration_days, price_cny_cent, action_snapshot_json, legacy_source, legacy_ref_id, group_name, group_sort, is_recommended, sort_order, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, 1, '', 'midweb_placeholder', 'balance_topup', '余额充值', 100, 0, 0, 0, ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		midwebBalanceTopupProductID,
		"余额充值",
		"Midweb 导入用余额占位商品",
		model.PurchaseProductKindBalanceTopup,
		midwebBalanceTopupPlanID,
		now,
		now,
	); err != nil {
		return err
	}
	return nil
}

func ensureMidwebLegacyOrderProductTx(
	tx *sql.Tx,
	productID string,
	planID string,
	template midwebTemplateRow,
	order midwebPurchaseOrderRow,
) error {
	if strings.TrimSpace(productID) == "" || strings.TrimSpace(planID) == "" {
		return nil
	}
	durationDays := 1
	if template.DurationDays.Valid && template.DurationDays.Int64 > 0 {
		durationDays = int(template.DurationDays.Int64)
	}
	priceCNYCent := int64(math.Round(order.AmountCNY.Float64 * 100))
	if priceCNYCent <= 0 {
		priceCNYCent = 1
	}
	productKind := model.PurchaseProductKindOverwrite
	snapshotPayload := model.PurchaseActionSnapshot{
		ProductKind:              model.PurchaseProductKindOverwrite,
		PlanID:                   planID,
		DurationDays:             durationDays,
		SourceDailyLimitMicros:   microsFromNullableUSD(template.DailyQuotaUSD),
		SourceWeeklyLimitMicros:  microsFromNullableUSD(template.WeeklyQuotaUSD),
		SourceMonthlyLimitMicros: microsFromNullableUSD(template.MonthlyQuotaUSD),
		SourceTotalLimitMicros:   microsFromNullableUSD(template.TotalQuotaUSD),
		LegacyTemplateID:         fmt.Sprintf("%d", template.ID),
	}
	if template.TemplateKind == "metered" {
		productKind = model.PurchaseProductKindBalanceTopup
		snapshotPayload = model.PurchaseActionSnapshot{
			ProductKind:        model.PurchaseProductKindBalanceTopup,
			BalanceTopupMicros: microsFromNullableUSD(template.TotalQuotaUSD),
			LegacyTemplateID:   fmt.Sprintf("%d", template.ID),
		}
	}
	snapshot, err := json.Marshal(snapshotPayload)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = tx.Exec(
		`INSERT INTO purchase_products (
			id, name, summary, product_kind, subscription_plan_id, duration_days, price_cny_cent, action_snapshot_json, legacy_source, legacy_ref_id,
			group_name, group_sort, is_recommended, sort_order, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'midweb_legacy_order', ?, '历史订单', 99, 0, 0, 0, ?, ?)
		ON CONFLICT (id) DO NOTHING`,
		productID,
		fmt.Sprintf("Midweb 历史模板 %d", template.ID),
		"导入用历史订单占位商品",
		productKind,
		planID,
		durationDays,
		priceCNYCent,
		string(snapshot),
		fmt.Sprintf("%d", template.ID),
		now,
		now,
	)
	return err
}

func loadMidwebTemplates(db *sql.DB) (map[int64]midwebTemplateRow, error) {
	rows, err := db.Query(
		`SELECT id, name, duration_days, daily_quota_usd, weekly_quota_usd, monthly_quota_usd, total_quota_usd, daily_reset_mode, daily_reset_time, template_kind
		   FROM cdk_templates`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make(map[int64]midwebTemplateRow)
	for rows.Next() {
		var item midwebTemplateRow
		if err := rows.Scan(&item.ID, &item.Name, &item.DurationDays, &item.DailyQuotaUSD, &item.WeeklyQuotaUSD, &item.MonthlyQuotaUSD, &item.TotalQuotaUSD, &item.DailyResetMode, &item.DailyResetTime, &item.TemplateKind); err != nil {
			return nil, err
		}
		items[item.ID] = item
	}
	return items, rows.Err()
}

func loadMidwebBoundCDKs(db *sql.DB) ([]midwebBoundCDKRow, error) {
	rows, err := db.Query(
		`SELECT c.id, c.cdk, c.template_id, c.used_at, c.remote_user_id, c.remote_user_name, c.remote_api_key,
		        s.expires_at, s.limit_total_usd, t.template_kind, t.duration_days, t.daily_quota_usd
		   FROM cdks c
		   LEFT JOIN remote_user_snapshots s ON s.remote_user_id = c.remote_user_id
		   LEFT JOIN cdk_templates t ON t.id = c.template_id
		  WHERE c.remote_user_id IS NOT NULL
		  ORDER BY c.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebBoundCDKRow, 0)
	for rows.Next() {
		var item midwebBoundCDKRow
		if err := rows.Scan(&item.ID, &item.CodeValue, &item.TemplateID, &item.UsedAt, &item.RemoteUserID, &item.RemoteUserName, &item.RemoteAPIKey, &item.SnapshotExpiresAt, &item.SnapshotLimitTotal, &item.TemplateKind, &item.DurationDays, &item.DailyQuotaUSD); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebRecharges(db *sql.DB) ([]midwebRechargeRow, error) {
	rows, err := db.Query(
		`SELECT id, target_cdk_id, remote_user_id, mode, status, source_daily_quota_usd, target_daily_quota_usd, peak_daily_quota_usd,
		        target_expires_at_before, target_expires_at_after, preview_json, created_at, confirmed_at, applied_at
		   FROM cdk_recharges
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebRechargeRow, 0)
	for rows.Next() {
		var item midwebRechargeRow
		if err := rows.Scan(&item.ID, &item.TargetCDKID, &item.RemoteUserID, &item.Mode, &item.Status, &item.SourceDailyQuotaUSD, &item.TargetDailyQuotaUSD, &item.PeakDailyQuotaUSD, &item.TargetExpiresBefore, &item.TargetExpiresAfter, &item.PreviewJSON, &item.CreatedAt, &item.ConfirmedAt, &item.AppliedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebRechargePhases(db *sql.DB) ([]midwebRechargePhaseRow, error) {
	rows, err := db.Query(
		`SELECT p.id, p.recharge_id, r.target_cdk_id, p.phase_type, p.start_at, p.end_at, p.daily_quota_usd, r.status, r.preview_json
		   FROM cdk_recharge_phases p
		   INNER JOIN cdk_recharges r ON r.id = p.recharge_id
		  ORDER BY p.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebRechargePhaseRow, 0)
	for rows.Next() {
		var item midwebRechargePhaseRow
		if err := rows.Scan(&item.ID, &item.RechargeID, &item.TargetCDKID, &item.PhaseType, &item.StartAt, &item.EndAt, &item.DailyQuotaUSD, &item.RechargeStatus, &item.PreviewJSON); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebQuotaResets(db *sql.DB) ([]midwebResetRow, error) {
	rows, err := db.Query(
		`SELECT id, cdk_id, usage_date_cn, before_expires_at, after_expires_at, before_daily_used_usd
		   FROM cdk_quota_resets
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebResetRow, 0)
	for rows.Next() {
		var item midwebResetRow
		if err := rows.Scan(&item.ID, &item.CDKID, &item.UsageDateCN, &item.BeforeExpiresAt, &item.AfterExpiresAt, &item.BeforeDailyUsedUSD); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebPurchaseSubscriptions(db *sql.DB) ([]midwebPurchaseSubscriptionRow, error) {
	rows, err := db.Query(`SELECT id, name, template_id, price_cny, enabled FROM purchase_subscriptions ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebPurchaseSubscriptionRow, 0)
	for rows.Next() {
		var item midwebPurchaseSubscriptionRow
		if err := rows.Scan(&item.ID, &item.Name, &item.TemplateID, &item.PriceCNY, &item.Enabled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebPurchaseOrders(db *sql.DB) ([]midwebPurchaseOrderRow, error) {
	rows, err := db.Query(
		`SELECT id, order_no, order_kind, email, template_id, target_remote_user_id, amount_cny, metered_amount_usd, payment_status, created_at, paid_at
		   FROM purchase_orders
		  ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebPurchaseOrderRow, 0)
	for rows.Next() {
		var item midwebPurchaseOrderRow
		if err := rows.Scan(&item.ID, &item.OrderNo, &item.OrderKind, &item.Email, &item.TemplateID, &item.TargetRemoteUserID, &item.AmountCNY, &item.MeteredAmountUSD, &item.PaymentStatus, &item.CreatedAt, &item.PaidAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebPurchaseDeliveries(db *sql.DB) ([]midwebDeliveryRow, error) {
	rows, err := db.Query(
		`SELECT d.id, d.order_id, d.cdk_id, d.delivery_cdk, c.template_id, t.duration_days, t.template_kind, c.used_at, c.remote_user_id
		   FROM purchase_order_deliveries d
		   INNER JOIN cdks c ON c.id = d.cdk_id
		   LEFT JOIN cdk_templates t ON t.id = c.template_id
		  ORDER BY d.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]midwebDeliveryRow, 0)
	for rows.Next() {
		var item midwebDeliveryRow
		if err := rows.Scan(&item.ID, &item.OrderID, &item.CDKID, &item.DeliveryCDK, &item.TemplateID, &item.DurationDays, &item.TemplateKind, &item.UsedAt, &item.RemoteUserID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadMidwebDailyRechargeLimit(db *sql.DB) (float64, error) {
	var raw sql.NullString
	if err := db.QueryRow(`SELECT value FROM system_settings WHERE key = 'daily_recharge_limit_usd'`).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	if !raw.Valid {
		return 0, nil
	}
	value, err := strconvParseFloat(strings.TrimSpace(raw.String))
	if err != nil {
		return 0, err
	}
	return value, nil
}

func midwebPlanID(templateID int64) string {
	return fmt.Sprintf("midweb-plan-%d", templateID)
}

func buildPlanLimitsFromTemplate(planID string, template midwebTemplateRow) ([]model.SubscriptionPlanLimit, error) {
	limits := make([]model.SubscriptionPlanLimit, 0, 5)
	windowMode := model.WindowModeFixed
	if strings.EqualFold(strings.TrimSpace(template.DailyResetMode), "rolling") {
		windowMode = model.WindowModeSliding
	}
	if template.DailyQuotaUSD.Valid && template.DailyQuotaUSD.Float64 > 0 {
		limits = append(limits, model.SubscriptionPlanLimit{
			ID:             fmt.Sprintf("%s-daily", planID),
			PlanID:         planID,
			LimitType:      model.LimitTypeDaily,
			WindowMode:     windowMode,
			LimitMicros:    microsFromUSD(template.DailyQuotaUSD.Float64),
			FixedResetTime: normalizeMidwebResetTime(template.DailyResetMode, template.DailyResetTime),
		})
	}
	if template.WeeklyQuotaUSD.Valid && template.WeeklyQuotaUSD.Float64 > 0 {
		limits = append(limits, model.SubscriptionPlanLimit{ID: fmt.Sprintf("%s-weekly", planID), PlanID: planID, LimitType: model.LimitTypeWeekly, WindowMode: model.WindowModeFixed, LimitMicros: microsFromUSD(template.WeeklyQuotaUSD.Float64)})
	}
	if template.MonthlyQuotaUSD.Valid && template.MonthlyQuotaUSD.Float64 > 0 {
		limits = append(limits, model.SubscriptionPlanLimit{ID: fmt.Sprintf("%s-monthly", planID), PlanID: planID, LimitType: model.LimitTypeMonthly, WindowMode: model.WindowModeFixed, LimitMicros: microsFromUSD(template.MonthlyQuotaUSD.Float64)})
	}
	if template.TotalQuotaUSD.Valid && template.TotalQuotaUSD.Float64 > 0 {
		limitType := model.LimitTypeTotal
		if strings.EqualFold(strings.TrimSpace(template.TemplateKind), "metered") {
			limitType = model.LimitTypeTotal
		}
		limits = append(limits, model.SubscriptionPlanLimit{ID: fmt.Sprintf("%s-total", planID), PlanID: planID, LimitType: limitType, WindowMode: model.WindowModeFixed, LimitMicros: microsFromUSD(template.TotalQuotaUSD.Float64)})
	}
	return limits, nil
}

func limitFixedResetMinute(limit model.SubscriptionPlanLimit) any {
	if limit.FixedResetTime == nil {
		return nil
	}
	minutes, err := model.ParseFixedResetTime(*limit.FixedResetTime)
	if err != nil {
		return nil
	}
	return minutes
}

func findLimitMicros(limits []model.SubscriptionPlanLimit, limitType model.LimitType) int64 {
	for _, limit := range limits {
		if limit.LimitType == limitType {
			return limit.LimitMicros
		}
	}
	return 0
}

func findLimitResetTime(limits []model.SubscriptionPlanLimit, limitType model.LimitType) *string {
	for _, limit := range limits {
		if limit.LimitType == limitType {
			return limit.FixedResetTime
		}
	}
	return nil
}

func deriveMidwebSubscriptionExpiry(row midwebBoundCDKRow, latestRechargeExpiry *time.Time, template midwebTemplateRow) *time.Time {
	if parsed := parseMidwebTime(row.SnapshotExpiresAt.String); parsed != nil {
		return parsed
	}
	if latestRechargeExpiry != nil {
		return latestRechargeExpiry
	}
	if usedAt := parseMidwebTime(row.UsedAt.String); usedAt != nil && template.DurationDays.Valid && template.DurationDays.Int64 > 0 {
		exp := usedAt.AddDate(0, 0, int(template.DurationDays.Int64))
		return &exp
	}
	return nil
}

func normalizeMidwebResetTime(resetMode, resetTime string) *string {
	if strings.EqualFold(strings.TrimSpace(resetMode), "rolling") {
		return nil
	}
	resetTime = strings.TrimSpace(resetTime)
	if resetTime == "" {
		value := "00:00"
		return &value
	}
	if _, err := model.ParseFixedResetTime(resetTime); err != nil {
		value := "00:00"
		return &value
	}
	return &resetTime
}

func parseMidwebTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	layouts := []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, loc); err == nil {
			value := parsed.UTC()
			return &value
		}
	}
	return nil
}

func phaseStatusForImport(start, end time.Time, rechargeStatus string, now time.Time) string {
	switch strings.ToLower(strings.TrimSpace(rechargeStatus)) {
	case "superseded", "cancelled":
		return string(model.SubscriptionTimelinePhaseStatusSuperseded)
	}
	if !start.After(now) && end.After(now) {
		return string(model.SubscriptionTimelinePhaseStatusActive)
	}
	if !end.After(now) {
		return string(model.SubscriptionTimelinePhaseStatusCompleted)
	}
	return string(model.SubscriptionTimelinePhaseStatusScheduled)
}

func midwebUsageDateBounds(value string) (time.Time, time.Time) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), loc)
	if err != nil {
		now := time.Now().In(loc)
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		return start.UTC(), start.AddDate(0, 0, 1).UTC()
	}
	return parsed.UTC(), parsed.AddDate(0, 0, 1).UTC()
}

func microsFromUSD(value float64) int64 {
	return int64(math.Round(value * 1_000_000))
}

func microsFromNullableUSD(value sql.NullFloat64) int64 {
	if !value.Valid || value.Float64 <= 0 {
		return 0
	}
	return microsFromUSD(value.Float64)
}

func nullMicrosFromUSD(value sql.NullFloat64) any {
	if !value.Valid {
		return nil
	}
	return microsFromUSD(value.Float64)
}

func maskCodeValue(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func strconvParseFloat(value string) (float64, error) {
	return strconv.ParseFloat(value, 64)
}

func normalizePurchasePaymentStatus(value string) model.PurchasePaymentStatus {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "paid":
		return model.PurchasePaymentStatusPaid
	case "expired":
		return model.PurchasePaymentStatusExpired
	case "closed":
		return model.PurchasePaymentStatusClosed
	case "failed":
		return model.PurchasePaymentStatusFailed
	case "refunded":
		return model.PurchasePaymentStatusRefunded
	default:
		return model.PurchasePaymentStatusPending
	}
}
