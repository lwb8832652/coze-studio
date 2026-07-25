// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	infrabilling "github.com/coze-dev/coze-studio/backend/infra/billing"
)

type billingTestIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *billingTestIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *billingTestIDGenerator) GenMultiIDs(ctx context.Context, count int) ([]int64, error) {
	ids := make([]int64, count)
	for index := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

type billingMaintenanceTestSchema struct {
	audit  bool
	outbox bool
}

func newBillingMaintenanceTestService(
	t *testing.T,
	schema billingMaintenanceTestSchema,
) (*Service, *gorm.DB, *billingTestIDGenerator) {
	t.Helper()
	databaseName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(
		sqlite.Open("file:"+databaseName+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	statements := []string{
		`CREATE TABLE billing_accounts (id INTEGER PRIMARY KEY, subject_type TEXT NOT NULL, subject_id INTEGER NOT NULL, available_micros INTEGER NOT NULL DEFAULT 0, reserved_micros INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(subject_type, subject_id))`,
		`CREATE TABLE credit_batches (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL, source_type TEXT NOT NULL, source_id TEXT NOT NULL, grant_business_no TEXT NOT NULL UNIQUE, granted_micros INTEGER NOT NULL, remaining_micros INTEGER NOT NULL, expires_at DATETIME, version INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE credit_ledger_entries (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL, batch_id INTEGER, direction TEXT NOT NULL, entry_type TEXT NOT NULL, amount_micros INTEGER NOT NULL, available_after_micros INTEGER NOT NULL, reserved_after_micros INTEGER NOT NULL, business_no TEXT NOT NULL UNIQUE, actor_user_id INTEGER NOT NULL DEFAULT 0, metadata_json TEXT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE TABLE credit_reservations (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL, reserve_business_no TEXT NOT NULL UNIQUE, settlement_business_no TEXT UNIQUE, release_business_no TEXT UNIQUE, reserved_micros INTEGER NOT NULL, settled_micros INTEGER NOT NULL DEFAULT 0, released_micros INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, allocations_json TEXT NOT NULL, expires_at DATETIME NOT NULL, version INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE billing_credit_threshold_configs (subject_type TEXT NOT NULL, subject_id INTEGER NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, threshold_micros INTEGER NOT NULL DEFAULT 0, recovery_margin_micros INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1, updated_by INTEGER NOT NULL DEFAULT 0, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, PRIMARY KEY(subject_type, subject_id))`,
		`CREATE TABLE billing_credit_threshold_episodes (account_id INTEGER PRIMARY KEY, episode_no INTEGER NOT NULL, active INTEGER NOT NULL, threshold_micros INTEGER NOT NULL, recovery_margin_micros INTEGER NOT NULL, version INTEGER NOT NULL, opened_at DATETIME NOT NULL, recovered_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE space_user (id INTEGER PRIMARY KEY AUTOINCREMENT, space_id INTEGER NOT NULL, user_id INTEGER NOT NULL, role_type INTEGER NOT NULL DEFAULT 3, created_at INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL DEFAULT 0, UNIQUE(space_id, user_id))`,
	}
	if schema.audit {
		statements = append(statements,
			`CREATE TABLE billing_audit_logs (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL, actor_user_id INTEGER NOT NULL, action TEXT NOT NULL, business_no TEXT NOT NULL, summary_json TEXT NOT NULL, created_at DATETIME NOT NULL, UNIQUE(business_no, action))`,
		)
	}
	if schema.outbox {
		statements = append(statements,
			`CREATE TABLE notification_outbox (id INTEGER PRIMARY KEY, event_id TEXT NOT NULL UNIQUE, idempotency_key TEXT NOT NULL UNIQUE, event_type TEXT NOT NULL, aggregate_type TEXT NOT NULL, aggregate_id TEXT NOT NULL, aggregate_version INTEGER NOT NULL, occurred_at INTEGER NOT NULL, space_id INTEGER NOT NULL DEFAULT 0, actor_id INTEGER NOT NULL DEFAULT 0, recipient_policy TEXT NOT NULL, payload_schema INTEGER NOT NULL, payload_json BLOB NOT NULL, status TEXT NOT NULL, attempt_count INTEGER NOT NULL DEFAULT 0, available_at INTEGER NOT NULL, locked_at INTEGER NOT NULL DEFAULT 0, locked_by TEXT NOT NULL DEFAULT '', last_error_code TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, delivered_at INTEGER NOT NULL DEFAULT 0)`,
		)
	}
	for _, statement := range statements {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("create billing test schema: %v", err)
		}
	}
	now := time.Date(2026, time.July, 25, 9, 0, 0, 0, time.UTC)
	for _, subjectType := range []domainbilling.SubjectType{
		domainbilling.SubjectTypeUser,
		domainbilling.SubjectTypeWorkspace,
	} {
		if err = db.Exec(
			`INSERT INTO billing_credit_threshold_configs
			 (subject_type, subject_id, enabled, threshold_micros, recovery_margin_micros, version, updated_by, created_at, updated_at)
			 VALUES (?, 0, 0, 0, 0, 1, 0, ?, ?)`,
			string(subjectType), now, now,
		).Error; err != nil {
			t.Fatal(err)
		}
	}
	idGenerator := &billingTestIDGenerator{next: 10_000}
	return NewService(db, idGenerator), db, idGenerator
}

func configureBillingCreditThreshold(
	t *testing.T,
	db *gorm.DB,
	subject domainbilling.Subject,
	enabled bool,
	thresholdMicros int64,
	recoveryMarginMicros int64,
) {
	t.Helper()
	subjectID := subject.ID
	if subjectID < 0 {
		t.Fatalf("invalid threshold subject ID %d", subjectID)
	}
	now := time.Date(2026, time.July, 25, 9, 0, 0, 0, time.UTC)
	if err := db.Exec(
		`INSERT OR REPLACE INTO billing_credit_threshold_configs
		 (subject_type, subject_id, enabled, threshold_micros, recovery_margin_micros, version, updated_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, 9001, ?, ?)`,
		string(subject.Type), subjectID, enabled, thresholdMicros, recoveryMarginMicros, now, now,
	).Error; err != nil {
		t.Fatal(err)
	}
}

func countBillingRows(t *testing.T, db *gorm.DB, table string, where string, args ...any) int64 {
	t.Helper()
	query := db.Table(table)
	if where != "" {
		query = query.Where(where, args...)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAdjustCreditsRollsBackWhenAuditWriteFails(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: false, outbox: true},
	)
	_, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2001},
		AmountMicros: 1_000_000, BusinessNo: "rollback-audit", Reason: "force audit failure", ActorUserID: 3001,
	})
	if err == nil {
		t.Fatal("AdjustCredits() error = nil, want audit table failure")
	}

	for _, table := range []string{"billing_accounts", "credit_batches", "credit_ledger_entries"} {
		var count int64
		if err = db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s row count = %d, want 0 after rollback", table, count)
		}
	}
}

func TestAdjustCreditsRollsBackLedgerAndAuditWhenOutboxAppendFails(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: false},
	)
	_, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2002},
		AmountMicros: 500,
		BusinessNo:   "rollback-outbox",
		Reason:       "force outbox failure",
		ActorUserID:  3002,
	})
	if err == nil {
		t.Fatal("AdjustCredits() error = nil, want outbox table failure")
	}
	for _, table := range []string{
		"billing_accounts",
		"credit_batches",
		"credit_ledger_entries",
		"billing_audit_logs",
		"billing_credit_threshold_episodes",
	} {
		if count := countBillingRows(t, db, table, ""); count != 0 {
			t.Fatalf("%s row count = %d, want 0 after outbox rollback", table, count)
		}
	}
}

func TestAdjustCreditsUsesPersistedWorkspaceRouteAndServerActor(t *testing.T) {
	service, db, idGenerator := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	for _, member := range []struct {
		userID int64
		role   int
	}{
		{userID: 7101, role: 1},
		{userID: 7102, role: 2},
		{userID: 7103, role: 3},
	} {
		if err := db.Exec(
			`INSERT INTO space_user (space_id, user_id, role_type) VALUES (?, ?, ?)`,
			7001, member.userID, member.role,
		).Error; err != nil {
			t.Fatal(err)
		}
	}

	result, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 7001},
		AmountMicros: 1_000,
		BusinessNo:   "workspace-adjustment",
		Reason:       "support correction",
		ActorUserID:  9001,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance.Subject.Type != domainbilling.SubjectTypeWorkspace ||
		result.Balance.Subject.ID != 7001 {
		t.Fatalf("adjustment balance subject = %+v, want persisted workspace subject", result.Balance.Subject)
	}

	var row struct {
		EventType       string
		SpaceID         int64
		ActorID         int64
		RecipientPolicy string
		OccurredAt       int64
		PayloadJSON     []byte
	}
	if err = db.Table("notification_outbox").
		Where("event_type = ?", string(domainnotification.EventBillingCreditAdjusted)).
		Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var payload domainnotification.EventPayload
	if err = json.Unmarshal(row.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if row.SpaceID != 7001 || row.ActorID != 9001 ||
		row.RecipientPolicy != string(domainnotification.RecipientExplicitInternalUsers) {
		t.Fatalf("adjustment route = %+v, want persisted workspace route and server actor", row)
	}
	if fmt.Sprint(payload.ExplicitRecipientIDs) != fmt.Sprint([]int64{7101, 7102}) {
		t.Fatalf("adjustment recipients = %v, want workspace owner/admin", payload.ExplicitRecipientIDs)
	}
	payloadText := strings.ToLower(string(row.PayloadJSON))
	for _, forbidden := range []string{"support correction", "amount_micros", "metadata", "ledger", "provider"} {
		if strings.Contains(payloadText, forbidden) {
			t.Fatalf("adjustment payload %s contains forbidden detail %q", row.PayloadJSON, forbidden)
		}
	}

	if err = db.Exec(
		`DELETE FROM space_user
		 WHERE space_id = ? AND role_type IN (?, ?)`,
		7001,
		1,
		2,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(
		`INSERT INTO space_user (space_id, user_id, role_type) VALUES (?, ?, ?)`,
		7001,
		7201,
		1,
	).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 7001},
		AmountMicros: 1_000,
		BusinessNo:   "workspace-adjustment",
		Reason:       "idempotent replay after membership change",
		ActorUserID:  9001,
	}); err != nil {
		t.Fatal(err)
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditAdjusted),
	); count != 1 {
		t.Fatalf("replayed adjustment outbox count = %d, want 1", count)
	}
	if err = db.Table("notification_outbox").
		Where("event_type = ?", string(domainnotification.EventBillingCreditAdjusted)).
		Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(row.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(payload.ExplicitRecipientIDs) != fmt.Sprint([]int64{7101, 7102}) {
		t.Fatalf(
			"replayed adjustment recipients = %v, want original snapshot",
			payload.ExplicitRecipientIDs,
		)
	}

	notificationRepository := infrabilling.NewMaintenanceRepository(db, idGenerator)
	err = notificationRepository.AppendCreditAdjustmentNotification(
		context.Background(),
		result.Balance.AccountID,
		"admin-credit:workspace-adjustment",
		9002,
		time.UnixMilli(row.OccurredAt),
	)
	if !errors.Is(err, domainnotification.ErrIdempotencyConflict) {
		t.Fatalf("different adjustment content error = %v, want idempotency conflict", err)
	}
}

func TestAdjustCreditsRollsBackLowEpisodeAndOutboxTogether(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		100,
		20,
	)
	if err := db.Exec(`
		CREATE TRIGGER reject_credit_adjustment_outbox
		BEFORE INSERT ON notification_outbox
		WHEN NEW.event_type = 'billing.credit_adjusted'
		BEGIN
			SELECT RAISE(FAIL, 'forced adjustment outbox failure');
		END`).Error; err != nil {
		t.Fatal(err)
	}

	_, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2101},
		AmountMicros: 50,
		BusinessNo:   "rollback-threshold-outbox",
		Reason:       "force terminal append rollback",
		ActorUserID:  9001,
	})
	if err == nil {
		t.Fatal("AdjustCredits() error = nil, want forced adjustment append failure")
	}
	for _, table := range []string{
		"billing_accounts",
		"credit_batches",
		"credit_ledger_entries",
		"billing_audit_logs",
		"billing_credit_threshold_episodes",
		"notification_outbox",
	} {
		if count := countBillingRows(t, db, table, ""); count != 0 {
			t.Fatalf("%s row count = %d, want full outer transaction rollback", table, count)
		}
	}
}

func TestLowCreditThresholdLifecycleIgnoresReserveReleaseAndUsesHysteresis(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2201}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		100,
		20,
	)
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 200, BusinessNo: "threshold-grant-1",
		SourceType: "test", SourceID: "threshold-lifecycle", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reserve(context.Background(), domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 20, BusinessNo: "threshold-reserve-release",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Release(context.Background(), domainbilling.ReleaseInput{
		ReservationBusinessNo: "threshold-reserve-release",
		BusinessNo:            "threshold-release",
	}); err != nil {
		t.Fatal(err)
	}
	if count := countBillingRows(
		t, db, "notification_outbox", "event_type = ?", string(domainnotification.EventBillingCreditLow),
	); count != 0 {
		t.Fatalf("low-credit outbox count after reserve/release = %d, want 0", count)
	}

	settleBillingCredits(t, service, subject, 150, "threshold-first")
	if count := countBillingRows(
		t, db, "notification_outbox", "event_type = ?", string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("first crossing outbox count = %d, want 1", count)
	}
	settleBillingCredits(t, service, subject, 10, "threshold-repeat")
	if count := countBillingRows(
		t, db, "notification_outbox", "event_type = ?", string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("repeated-low outbox count = %d, want 1", count)
	}

	for index, amount := range []int64{70, 10} {
		if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
			Subject: subject, AmountMicros: amount,
			BusinessNo: fmt.Sprintf("threshold-recovery-%d", index),
			SourceType: "test", SourceID: "threshold-recovery", ActorUserID: subject.ID,
		}); err != nil {
			t.Fatal(err)
		}
		expectedActive := index == 0
		var active bool
		if err := db.Table("billing_credit_threshold_episodes").
			Select("active").Where("account_id = ?", 10_001).Scan(&active).Error; err != nil {
			t.Fatal(err)
		}
		if active != expectedActive {
			t.Fatalf("episode active after recovery grant %d = %v, want %v", index, active, expectedActive)
		}
	}

	settleBillingCredits(t, service, subject, 30, "threshold-second")
	if count := countBillingRows(
		t, db, "notification_outbox", "event_type = ?", string(domainnotification.EventBillingCreditLow),
	); count != 2 {
		t.Fatalf("second crossing outbox count = %d, want 2", count)
	}
	var episode struct {
		EpisodeNo int64
		Active    bool
	}
	if err := db.Table("billing_credit_threshold_episodes").Take(&episode).Error; err != nil {
		t.Fatal(err)
	}
	if episode.EpisodeNo != 2 || !episode.Active {
		t.Fatalf("episode = %+v, want active episode 2", episode)
	}
}

func TestLowCreditThresholdDoesNotBackfillInitialLowBalanceAfterRestart(t *testing.T) {
	service, db, idGenerator := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2202}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		100,
		20,
	)
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 40, BusinessNo: "initial-low-grant",
		SourceType: "test", SourceID: "initial-low", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewService(db, idGenerator)
	if _, err := restarted.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 30, BusinessNo: "restarted-low-grant",
		SourceType: "test", SourceID: "initial-low", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	for _, adjustment := range []struct {
		amount     int64
		businessNo string
	}{
		{amount: 20, businessNo: "initial-low-admin-credit"},
		{amount: -10, businessNo: "initial-low-admin-debit"},
	} {
		if _, err := restarted.AdjustCredits(context.Background(), CreditAdjustmentInput{
			Subject:      subject,
			AmountMicros: adjustment.amount,
			BusinessNo:   adjustment.businessNo,
			Reason:       "initial low crossing test",
			ActorUserID:  9001,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditLow),
	); count != 0 {
		t.Fatalf("initial low outbox count = %d, want no backfill", count)
	}
	if count := countBillingRows(t, db, "billing_credit_threshold_episodes", "1 = 1"); count != 0 {
		t.Fatalf("initial low episode count = %d, want 0", count)
	}

	if _, err := restarted.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      subject,
		AmountMicros: 40,
		BusinessNo:   "initial-low-recovery",
		Reason:       "recover before next crossing",
		ActorUserID:  9001,
	}); err != nil {
		t.Fatal(err)
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditLow),
	); count != 0 {
		t.Fatalf("recovery outbox count = %d, want 0", count)
	}

	for index, amount := range []int64{-30, -10} {
		if _, err := restarted.AdjustCredits(context.Background(), CreditAdjustmentInput{
			Subject:      subject,
			AmountMicros: amount,
			BusinessNo:   fmt.Sprintf("post-recovery-admin-debit-%d", index),
			Reason:       "post recovery crossing test",
			ActorUserID:  9001,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("post-recovery low outbox count = %d, want 1", count)
	}
	var episode struct {
		EpisodeNo int64
		Active    bool
	}
	if err := db.Table("billing_credit_threshold_episodes").Take(&episode).Error; err != nil {
		t.Fatal(err)
	}
	if episode.EpisodeNo != 1 || !episode.Active {
		t.Fatalf("post-recovery episode = %+v, want active episode 1", episode)
	}
}

func settleBillingCredits(
	t *testing.T,
	service *Service,
	subject domainbilling.Subject,
	amount int64,
	key string,
) {
	t.Helper()
	reserveBusinessNo := key + "-reserve"
	if _, err := service.Reserve(context.Background(), domainbilling.ReserveInput{
		Subject: subject, AmountMicros: amount, BusinessNo: reserveBusinessNo,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Settle(context.Background(), domainbilling.SettleInput{
		ReservationBusinessNo: reserveBusinessNo,
		BusinessNo:            key + "-settle",
		ActualMicros:           amount,
		ActorUserID:            subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSerializedSQLiteSettlementAndRestartReplayCreateOneLowCreditEpisode(t *testing.T) {
	service, db, idGenerator := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2301}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		230,
		20,
	)
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 260, BusinessNo: "serialized-grant",
		SourceType: "test", SourceID: "serialized-settlement", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 2; index++ {
		if _, err := service.Reserve(context.Background(), domainbilling.ReserveInput{
			Subject: subject, AmountMicros: 60,
			BusinessNo: fmt.Sprintf("serialized-reserve-%d", index),
		}); err != nil {
			t.Fatal(err)
		}
	}

	for index := 1; index <= 2; index++ {
		if _, err := service.Settle(context.Background(), domainbilling.SettleInput{
			ReservationBusinessNo: fmt.Sprintf("serialized-reserve-%d", index),
			BusinessNo:            fmt.Sprintf("serialized-settle-%d", index),
			ActualMicros:           60,
			ActorUserID:            subject.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if count := countBillingRows(
		t, db, "notification_outbox", "event_type = ?", string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("serialized low-credit outbox count = %d, want 1", count)
	}
	if count := countBillingRows(t, db, "billing_credit_threshold_episodes", "active = 1"); count != 1 {
		t.Fatalf("serialized active episode count = %d, want 1", count)
	}

	restarted := NewService(db, idGenerator)
	if _, err := restarted.Settle(context.Background(), domainbilling.SettleInput{
		ReservationBusinessNo: "serialized-reserve-1",
		BusinessNo:            "serialized-settle-1",
		ActualMicros:           60,
		ActorUserID:            subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if count := countBillingRows(
		t, db, "notification_outbox", "event_type = ?", string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("restart/replay low-credit outbox count = %d, want 1", count)
	}
}

func TestAdminAdjustmentDirectionIsPartOfLedgerAuditAndOutboxIdentity(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2701}
	const businessNo = "same-external-adjustment"

	for _, amount := range []int64{100, -40} {
		if _, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
			Subject:      subject,
			AmountMicros: amount,
			BusinessNo:   businessNo,
			Reason:       "direction identity test",
			ActorUserID:  9001,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      subject,
		AmountMicros: 100,
		BusinessNo:   businessNo,
		Reason:       "same direction replay",
		ActorUserID:  9001,
	}); err != nil {
		t.Fatal(err)
	}

	for _, ledgerBusinessNo := range []string{
		"admin-credit:" + businessNo,
		"admin-debit:" + businessNo,
	} {
		if count := countBillingRows(
			t,
			db,
			"credit_ledger_entries",
			"business_no = ?",
			ledgerBusinessNo,
		); count != 1 {
			t.Fatalf("ledger %q count = %d, want 1", ledgerBusinessNo, count)
		}
		if count := countBillingRows(
			t,
			db,
			"billing_audit_logs",
			"business_no = ?",
			ledgerBusinessNo,
		); count != 1 {
			t.Fatalf("audit %q count = %d, want 1", ledgerBusinessNo, count)
		}
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditAdjusted),
	); count != 2 {
		t.Fatalf("directional adjustment outbox count = %d, want 2", count)
	}
	var identities []struct {
		EventID        string
		IdempotencyKey string
	}
	if err := db.Table("notification_outbox").
		Select("event_id, idempotency_key").
		Where("event_type = ?", string(domainnotification.EventBillingCreditAdjusted)).
		Order("event_id").
		Scan(&identities).Error; err != nil {
		t.Fatal(err)
	}
	if len(identities) != 2 ||
		identities[0].EventID == identities[1].EventID ||
		identities[0].IdempotencyKey == identities[1].IdempotencyKey {
		t.Fatalf("directional outbox identities = %+v, want two unique identities", identities)
	}
}

func TestReserveEvaluatesThresholdOnlyWhenBatchExpirationChangesSettledBalance(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2801}
	future := time.Now().UTC().Add(time.Hour)
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 150, BusinessNo: "reserve-expiring-grant",
		SourceType: "test", SourceID: "reserve-expiration", ExpiresAt: &future,
		ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 150, BusinessNo: "reserve-active-grant",
		SourceType: "test", SourceID: "reserve-expiration", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		200,
		20,
	)
	if err := db.Exec(
		"UPDATE credit_batches SET expires_at = ? WHERE grant_business_no = ?",
		time.Now().UTC().Add(-time.Hour),
		"reserve-expiring-grant",
	).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.Reserve(context.Background(), domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 25, BusinessNo: "reserve-after-expiration",
	}); err != nil {
		t.Fatal(err)
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("expiration crossing outbox count = %d, want 1", count)
	}
	if count := countBillingRows(t, db, "billing_credit_threshold_episodes", "active = 1"); count != 1 {
		t.Fatalf("expiration crossing active episode count = %d, want 1", count)
	}
}

func TestReleaseEvaluatesThresholdWhenExpiredAllocationCannotReturnToSettledBalance(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2802}
	future := time.Now().UTC().Add(time.Hour)
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 150, BusinessNo: "release-expiring-grant",
		SourceType: "test", SourceID: "release-expiration", ExpiresAt: &future,
		ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 150, BusinessNo: "release-active-grant",
		SourceType: "test", SourceID: "release-expiration", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reserve(context.Background(), domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 100, BusinessNo: "release-expired-allocation-reserve",
	}); err != nil {
		t.Fatal(err)
	}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		200,
		20,
	)
	if err := db.Exec(
		"UPDATE credit_batches SET expires_at = ? WHERE grant_business_no = ?",
		time.Now().UTC().Add(-time.Hour),
		"release-expiring-grant",
	).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.Release(context.Background(), domainbilling.ReleaseInput{
		ReservationBusinessNo: "release-expired-allocation-reserve",
		BusinessNo:            "release-expired-allocation",
	}); err != nil {
		t.Fatal(err)
	}
	if count := countBillingRows(
		t,
		db,
		"notification_outbox",
		"event_type = ?",
		string(domainnotification.EventBillingCreditLow),
	); count != 1 {
		t.Fatalf("expired release crossing outbox count = %d, want 1", count)
	}
}

func TestReserveExpirationThresholdAppendFailureRollsBackExpirationAndReservation(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2803}
	future := time.Now().UTC().Add(time.Hour)
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 150, BusinessNo: "rollback-expiring-grant",
		SourceType: "test", SourceID: "expiration-rollback", ExpiresAt: &future,
		ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: subject, AmountMicros: 150, BusinessNo: "rollback-active-grant",
		SourceType: "test", SourceID: "expiration-rollback", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		200,
		20,
	)
	if err := db.Exec(
		"UPDATE credit_batches SET expires_at = ? WHERE grant_business_no = ?",
		time.Now().UTC().Add(-time.Hour),
		"rollback-expiring-grant",
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_expiration_credit_low_outbox
		BEFORE INSERT ON notification_outbox
		WHEN NEW.event_type = 'billing.credit_low'
		BEGIN
			SELECT RAISE(FAIL, 'forced expiration threshold append failure');
		END`).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.Reserve(context.Background(), domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 25, BusinessNo: "rollback-expiration-reserve",
	}); err == nil {
		t.Fatal("Reserve() error = nil, want outbox append failure")
	}
	if count := countBillingRows(
		t,
		db,
		"credit_reservations",
		"reserve_business_no = ?",
		"rollback-expiration-reserve",
	); count != 0 {
		t.Fatalf("rolled-back reservation count = %d, want 0", count)
	}
	if count := countBillingRows(
		t,
		db,
		"credit_ledger_entries",
		"entry_type = ?",
		string(domainbilling.LedgerEntryTypeExpiration),
	); count != 0 {
		t.Fatalf("rolled-back expiration ledger count = %d, want 0", count)
	}
	if count := countBillingRows(t, db, "billing_credit_threshold_episodes", "1 = 1"); count != 0 {
		t.Fatalf("rolled-back episode count = %d, want 0", count)
	}
	var account struct {
		AvailableMicros int64
		ReservedMicros  int64
	}
	if err := db.Table("billing_accounts").
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Take(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicros != 300 || account.ReservedMicros != 0 {
		t.Fatalf("account after rollback = %+v, want available=300 reserved=0", account)
	}
}

func TestAdminDebitLowCreditOutboxFailureRollsBackAdjustmentAndEpisode(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2804}
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		100,
		20,
	)
	if _, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      subject,
		AmountMicros: 150,
		BusinessNo:   "admin-debit-rollback-seed",
		Reason:       "seed balance above threshold",
		ActorUserID:  9001,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_admin_debit_credit_low_outbox
		BEFORE INSERT ON notification_outbox
		WHEN NEW.event_type = 'billing.credit_low'
		BEGIN
			SELECT RAISE(FAIL, 'forced admin debit threshold append failure');
		END`).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.AdjustCredits(context.Background(), CreditAdjustmentInput{
		Subject:      subject,
		AmountMicros: -60,
		BusinessNo:   "admin-debit-rollback-crossing",
		Reason:       "force low credit outbox rollback",
		ActorUserID:  9001,
	}); err == nil {
		t.Fatal("AdjustCredits() error = nil, want low-credit outbox failure")
	}
	var account struct {
		AvailableMicros int64
		ReservedMicros  int64
	}
	if err := db.Table("billing_accounts").
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Take(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicros != 150 || account.ReservedMicros != 0 {
		t.Fatalf("account after admin debit rollback = %+v, want available=150 reserved=0", account)
	}
	for _, check := range []struct {
		table string
		where string
		args  []any
	}{
		{
			table: "credit_ledger_entries",
			where: "business_no = ?",
			args:  []any{"admin-debit:admin-debit-rollback-crossing"},
		},
		{
			table: "billing_audit_logs",
			where: "business_no = ?",
			args:  []any{"admin-debit:admin-debit-rollback-crossing"},
		},
		{
			table: "billing_credit_threshold_episodes",
			where: "1 = 1",
		},
		{
			table: "notification_outbox",
			where: "event_type = ?",
			args:  []any{string(domainnotification.EventBillingCreditLow)},
		},
	} {
		if count := countBillingRows(t, db, check.table, check.where, check.args...); count != 0 {
			t.Fatalf("%s rolled-back row count = %d, want 0", check.table, count)
		}
	}
}

func TestLowCreditRoutesRemainIsolatedForUserAndWorkspaceSubjects(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	for _, subjectType := range []domainbilling.SubjectType{
		domainbilling.SubjectTypeUser,
		domainbilling.SubjectTypeWorkspace,
	} {
		configureBillingCreditThreshold(
			t,
			db,
			domainbilling.Subject{Type: subjectType, ID: 0},
			true,
			100,
			20,
		)
	}
	for _, member := range []struct {
		userID int64
		role   int
	}{
		{userID: 2501, role: 1},
		{userID: 2502, role: 2},
		{userID: 2503, role: 3},
	} {
		if err := db.Exec(
			`INSERT INTO space_user (space_id, user_id, role_type) VALUES (2401, ?, ?)`,
			member.userID, member.role,
		).Error; err != nil {
			t.Fatal(err)
		}
	}
	subjects := []domainbilling.Subject{
		{Type: domainbilling.SubjectTypeUser, ID: 2401},
		{Type: domainbilling.SubjectTypeWorkspace, ID: 2401},
	}
	for index, subject := range subjects {
		if _, err := service.Grant(context.Background(), domainbilling.GrantInput{
			Subject: subject, AmountMicros: 150,
			BusinessNo: fmt.Sprintf("isolated-grant-%d", index),
			SourceType: "test", SourceID: "subject-isolation", ActorUserID: 9001,
		}); err != nil {
			t.Fatal(err)
		}
		settleBillingCredits(t, service, subject, 60, fmt.Sprintf("isolated-consume-%d", index))
	}

	var rows []struct {
		SpaceID     int64
		AggregateID string
		PayloadJSON []byte
	}
	if err := db.Table("notification_outbox").
		Select("space_id, aggregate_id, payload_json").
		Where("event_type = ?", string(domainnotification.EventBillingCreditLow)).
		Order("space_id ASC").
		Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].SpaceID != 0 || rows[1].SpaceID != 2401 ||
		rows[0].AggregateID == rows[1].AggregateID {
		t.Fatalf("isolated low-credit rows = %+v, want distinct user/workspace routes", rows)
	}
	recipients := make([][]int64, 0, len(rows))
	for _, row := range rows {
		var payload domainnotification.EventPayload
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			t.Fatal(err)
		}
		sort.Slice(payload.ExplicitRecipientIDs, func(left, right int) bool {
			return payload.ExplicitRecipientIDs[left] < payload.ExplicitRecipientIDs[right]
		})
		recipients = append(recipients, payload.ExplicitRecipientIDs)
	}
	if fmt.Sprint(recipients[0]) != fmt.Sprint([]int64{2401}) ||
		fmt.Sprint(recipients[1]) != fmt.Sprint([]int64{2501, 2502}) {
		t.Fatalf("isolated recipients = %v, want user and workspace owner/admin routes", recipients)
	}
}

func TestInvalidPersistedCreditThresholdConfigFailsClosed(t *testing.T) {
	service, db, _ := newBillingMaintenanceTestService(
		t,
		billingMaintenanceTestSchema{audit: true, outbox: true},
	)
	configureBillingCreditThreshold(
		t,
		db,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 0},
		true,
		0,
		20,
	)
	_, err := service.Grant(context.Background(), domainbilling.GrantInput{
		Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2601},
		AmountMicros: 50, BusinessNo: "invalid-config-grant",
		SourceType: "test", SourceID: "invalid-config", ActorUserID: 2601,
	})
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("Grant() error = %v, want invalid threshold config failure", err)
	}
	if count := countBillingRows(t, db, "billing_accounts", ""); count != 0 {
		t.Fatalf("billing account count = %d, want fail-closed rollback", count)
	}
}
