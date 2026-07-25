// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestCreditThresholdMySQLConcurrentSettlementCreatesOneEpisodeAndOutboxEvent(t *testing.T) {
	databases, services, ctx := newCreditThresholdMySQLIntegrationServices(t)
	db := databases[0]
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2901}
	if _, err := services[0].Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 260, BusinessNo: "mysql-threshold-grant",
		SourceType: "test", SourceID: "mysql-concurrent-settlement", ActorUserID: subject.ID,
	}); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 2; index++ {
		if _, err := services[0].Reserve(ctx, domainbilling.ReserveInput{
			Subject:      subject,
			AmountMicros: 60,
			BusinessNo:   "mysql-threshold-reserve-" + strconv.Itoa(index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var beforeSettlementEpisodeCount int64
	require.NoError(t, db.Table("billing_credit_threshold_episodes").
		Count(&beforeSettlementEpisodeCount).Error)
	require.Zero(t, beforeSettlementEpisodeCount)
	var beforeSettlementOutboxCount int64
	require.NoError(t, db.Table("notification_outbox").
		Where("event_type = ?", string(domainnotification.EventBillingCreditLow)).
		Count(&beforeSettlementOutboxCount).Error)
	require.Zero(t, beforeSettlementOutboxCount)

	start := make(chan struct{})
	type settlementAttempt struct {
		index  int
		result *domainbilling.ReservationResult
		err    error
	}
	attempts := make(chan settlementAttempt, 2)
	var wait sync.WaitGroup
	for index := 1; index <= 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			suffix := strconv.Itoa(index)
			result, settleErr := services[index-1].Settle(ctx, domainbilling.SettleInput{
				ReservationBusinessNo: "mysql-threshold-reserve-" + suffix,
				BusinessNo:            "mysql-threshold-settle-" + suffix,
				ActualMicros:           60,
				ActorUserID:            subject.ID,
			})
			attempts <- settlementAttempt{index: index, result: result, err: settleErr}
		}()
	}
	close(start)
	wait.Wait()
	close(attempts)
	committed := make(map[int]*domainbilling.ReservationResult, 2)
	for attempt := range attempts {
		require.NoError(t, attempt.err)
		require.NotNil(t, attempt.result)
		require.NotNil(t, attempt.result.Entry)
		require.NotNil(t, attempt.result.Reservation)
		require.Equal(
			t,
			domainbilling.ReservationStatusSettled,
			attempt.result.Reservation.Status,
		)
		committed[attempt.index] = attempt.result
	}
	require.Len(t, committed, 2)

	var settledReservations int64
	require.NoError(t, db.Table("credit_reservations").
		Where(
			"status = ? AND settlement_business_no LIKE ?",
			string(domainbilling.ReservationStatusSettled),
			"mysql-threshold-settle-%",
		).
		Count(&settledReservations).Error)
	require.Equal(t, int64(2), settledReservations)
	var consumeEntries int64
	require.NoError(t, db.Table("credit_ledger_entries").
		Where(
			"entry_type = ? AND business_no LIKE ?",
			string(domainbilling.LedgerEntryTypeConsume),
			"mysql-threshold-settle-%",
		).
		Count(&consumeEntries).Error)
	require.Equal(t, int64(2), consumeEntries)
	var account accountPO
	require.NoError(t, db.
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Take(&account).Error)
	require.Equal(t, int64(140), account.AvailableMicros)
	require.Zero(t, account.ReservedMicros)

	var episodeCount int64
	require.NoError(t, db.Table("billing_credit_threshold_episodes").
		Where("active = 1").
		Count(&episodeCount).Error)
	require.Equal(t, int64(1), episodeCount)
	var outboxCount int64
	require.NoError(t, db.Table("notification_outbox").
		Where("event_type = ?", string(domainnotification.EventBillingCreditLow)).
		Count(&outboxCount).Error)
	require.Equal(t, int64(1), outboxCount)
}

func TestCreditThresholdMySQLConcurrentFirstOverrideHasSingleCASWinner(t *testing.T) {
	databases, _, ctx := newCreditThresholdMySQLIntegrationServices(t)
	repositories := [2]*AdminRepository{
		NewAdminRepository(databases[0], nil),
		NewAdminRepository(databases[1], nil),
	}
	subject := domainbilling.Subject{
		Type: domainbilling.SubjectTypeWorkspace,
		ID:   2999,
	}
	type saveAttempt struct {
		view *CreditThresholdConfigView
		err  error
	}
	start := make(chan struct{})
	attempts := make(chan saveAttempt, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			view, saveErr := repositories[index].SaveCreditThresholdConfig(
				ctx,
				SaveCreditThresholdConfigInput{
					Subject:              subject,
					Enabled:              true,
					ThresholdMicros:      400 + int64(index),
					RecoveryMarginMicros: 40,
					ExpectedVersion:      0,
				},
				9001+int64(index),
			)
			attempts <- saveAttempt{view: view, err: saveErr}
		}()
	}
	close(start)
	wait.Wait()
	close(attempts)

	successes := 0
	conflicts := 0
	for attempt := range attempts {
		switch {
		case attempt.err == nil:
			successes++
			require.NotNil(t, attempt.view)
			require.Equal(t, int64(1), attempt.view.Version)
		case errors.Is(attempt.err, domainbilling.ErrVersionConflict):
			conflicts++
			require.Nil(t, attempt.view)
		default:
			t.Fatalf("unexpected first-write CAS error: %v", attempt.err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	var rows int64
	require.NoError(t, databases[0].Table("billing_credit_threshold_configs").
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Count(&rows).Error)
	require.Equal(t, int64(1), rows)
	var persisted creditThresholdConfigPO
	require.NoError(t, databases[0].
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Take(&persisted).Error)
	require.Equal(t, int64(1), persisted.Version)
	require.Contains(t, []int64{400, 401}, persisted.ThresholdMicros)
}

func newCreditThresholdMySQLIntegrationServices(
	t *testing.T,
) ([2]*gorm.DB, [2]*domainbilling.Service, context.Context) {
	t.Helper()
	gate, skipReason, err := loadBillingMySQLIntegrationGate(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	require.NoError(t, err)
	openDatabase := func() *gorm.DB {
		dsnConfig := *gate.config
		database, openErr := gorm.Open(
			gormmysql.New(gormmysql.Config{DSNConfig: &dsnConfig}),
			&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
		)
		require.NoError(t, openErr)
		sqlDB, dbErr := database.DB()
		require.NoError(t, dbErr)
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		return database
	}
	db := openDatabase()
	secondDB := openDatabase()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.PingContext(ctx))
	secondSQLDB, err := secondDB.DB()
	require.NoError(t, err)
	require.NoError(t, secondSQLDB.PingContext(ctx))

	tables := []string{
		"notification_outbox",
		"billing_credit_threshold_episodes",
		"billing_credit_threshold_configs",
		"credit_reservations",
		"credit_ledger_entries",
		"credit_batches",
		"billing_accounts",
	}
	cleanup := func() {
		for _, table := range tables {
			_ = db.Session(&gorm.Session{Logger: logger.Discard}).
				Exec("DROP TABLE IF EXISTS " + table).Error
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	statements := []string{
		`CREATE TABLE billing_accounts (
			id BIGINT NOT NULL PRIMARY KEY,
			subject_type VARCHAR(32) NOT NULL,
			subject_id BIGINT NOT NULL,
			available_micros BIGINT NOT NULL DEFAULT 0,
			reserved_micros BIGINT NOT NULL DEFAULT 0,
			version BIGINT NOT NULL DEFAULT 1,
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			UNIQUE KEY uk_billing_accounts_subject (subject_type, subject_id)
		) ENGINE=InnoDB`,
		`CREATE TABLE credit_batches (
			id BIGINT NOT NULL PRIMARY KEY,
			account_id BIGINT NOT NULL,
			source_type VARCHAR(64) NOT NULL,
			source_id VARCHAR(128) NOT NULL,
			grant_business_no VARCHAR(128) NOT NULL,
			granted_micros BIGINT NOT NULL,
			remaining_micros BIGINT NOT NULL,
			expires_at DATETIME(3) NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			UNIQUE KEY uk_credit_batches_grant_business_no (grant_business_no),
			KEY idx_credit_batches_account_expiry (account_id, expires_at)
		) ENGINE=InnoDB`,
		`CREATE TABLE credit_ledger_entries (
			id BIGINT NOT NULL PRIMARY KEY,
			account_id BIGINT NOT NULL,
			batch_id BIGINT NULL,
			direction VARCHAR(16) NOT NULL,
			entry_type VARCHAR(32) NOT NULL,
			amount_micros BIGINT NOT NULL,
			available_after_micros BIGINT NOT NULL,
			reserved_after_micros BIGINT NOT NULL,
			business_no VARCHAR(128) NOT NULL,
			actor_user_id BIGINT NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL,
			created_at DATETIME(3) NOT NULL,
			UNIQUE KEY uk_credit_ledger_business_no (business_no),
			KEY idx_credit_ledger_account_created (account_id, created_at)
		) ENGINE=InnoDB`,
		`CREATE TABLE credit_reservations (
			id BIGINT NOT NULL PRIMARY KEY,
			account_id BIGINT NOT NULL,
			reserve_business_no VARCHAR(128) NOT NULL,
			settlement_business_no VARCHAR(128) NULL,
			release_business_no VARCHAR(128) NULL,
			reserved_micros BIGINT NOT NULL,
			settled_micros BIGINT NOT NULL DEFAULT 0,
			released_micros BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(24) NOT NULL,
			allocations_json TEXT NOT NULL,
			expires_at DATETIME(3) NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			UNIQUE KEY uk_credit_reservations_reserve (reserve_business_no),
			UNIQUE KEY uk_credit_reservations_settle (settlement_business_no),
			UNIQUE KEY uk_credit_reservations_release (release_business_no)
		) ENGINE=InnoDB`,
		`CREATE TABLE billing_credit_threshold_configs (
			subject_type VARCHAR(32) NOT NULL,
			subject_id BIGINT NOT NULL,
			enabled TINYINT(1) NOT NULL DEFAULT 0,
			threshold_micros BIGINT NOT NULL DEFAULT 0,
			recovery_margin_micros BIGINT NOT NULL DEFAULT 0,
			version BIGINT NOT NULL DEFAULT 1,
			updated_by BIGINT NOT NULL DEFAULT 0,
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			PRIMARY KEY (subject_type, subject_id)
		) ENGINE=InnoDB`,
		`CREATE TABLE billing_credit_threshold_episodes (
			account_id BIGINT NOT NULL PRIMARY KEY,
			episode_no BIGINT NOT NULL,
			active TINYINT(1) NOT NULL,
			threshold_micros BIGINT NOT NULL,
			recovery_margin_micros BIGINT NOT NULL,
			version BIGINT NOT NULL,
			opened_at DATETIME(3) NOT NULL,
			recovered_at DATETIME(3) NULL,
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL
		) ENGINE=InnoDB`,
		`CREATE TABLE notification_outbox (
			id BIGINT NOT NULL PRIMARY KEY,
			event_id VARCHAR(160) NOT NULL,
			idempotency_key VARCHAR(160) NOT NULL,
			event_type VARCHAR(96) NOT NULL,
			aggregate_type VARCHAR(96) NOT NULL,
			aggregate_id VARCHAR(128) NOT NULL,
			aggregate_version BIGINT NOT NULL,
			occurred_at BIGINT NOT NULL,
			space_id BIGINT NOT NULL DEFAULT 0,
			actor_id BIGINT NOT NULL DEFAULT 0,
			recipient_policy VARCHAR(64) NOT NULL,
			payload_schema INT NOT NULL,
			payload_json BLOB NOT NULL,
			status VARCHAR(32) NOT NULL,
			attempt_count INT NOT NULL DEFAULT 0,
			available_at BIGINT NOT NULL,
			locked_at BIGINT NOT NULL DEFAULT 0,
			locked_by VARCHAR(128) NOT NULL DEFAULT '',
			last_error_code VARCHAR(128) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			delivered_at BIGINT NOT NULL DEFAULT 0,
			UNIQUE KEY uk_notification_outbox_event_id (event_id),
			UNIQUE KEY uk_notification_outbox_idempotency (idempotency_key)
		) ENGINE=InnoDB`,
	}
	for _, statement := range statements {
		require.NoError(t, db.WithContext(ctx).Exec(statement).Error)
	}
	now := time.Now().UTC()
	require.NoError(t, db.WithContext(ctx).Exec(
		`INSERT INTO billing_credit_threshold_configs
		 (subject_type, subject_id, enabled, threshold_micros, recovery_margin_micros, version, updated_by, created_at, updated_at)
		 VALUES (?, 0, 1, 230, 20, 1, 9001, ?, ?)`,
		string(domainbilling.SubjectTypeUser),
		now,
		now,
	).Error)
	idGenerator := &maintenanceRepositoryTestIDGenerator{next: 300_000}
	return [2]*gorm.DB{db, secondDB}, [2]*domainbilling.Service{
		domainbilling.NewService(NewMySQLRepository(db, idGenerator)),
		domainbilling.NewService(NewMySQLRepository(secondDB, idGenerator)),
	}, ctx
}
