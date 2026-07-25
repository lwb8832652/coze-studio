// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

const notificationMySQLDDLConfirmation = "YES_I_OWN_THIS_DISPOSABLE_SCHEMA"

var notificationMySQLDatabasePattern = regexp.MustCompile(
	`^coze_notification_it_[a-z0-9_]+$`,
)

type notificationMySQLIntegrationGate struct {
	config *mysqldriver.Config
}

func loadNotificationMySQLIntegrationGate(
	getenv func(string) string,
) (notificationMySQLIntegrationGate, string, error) {
	dsn := strings.TrimSpace(getenv("COZE_NOTIFICATION_MYSQL_TEST_DSN"))
	if dsn == "" {
		return notificationMySQLIntegrationGate{},
			"COZE_NOTIFICATION_MYSQL_TEST_DSN is not set",
			nil
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return notificationMySQLIntegrationGate{}, "", fmt.Errorf(
			"invalid notification MySQL integration DSN",
		)
	}
	if !notificationMySQLDatabasePattern.MatchString(config.DBName) {
		return notificationMySQLIntegrationGate{}, "", fmt.Errorf(
			"database must be a disposable coze_notification_it_* schema",
		)
	}
	if getenv("COZE_NOTIFICATION_MYSQL_TEST_ALLOW_DDL") !=
		notificationMySQLDDLConfirmation {
		return notificationMySQLIntegrationGate{}, "", fmt.Errorf(
			"explicit disposable-schema DDL confirmation is missing",
		)
	}
	config.ParseTime = true
	return notificationMySQLIntegrationGate{config: config}, "", nil
}

func TestNotificationMySQLIntegrationClaimUsesSkipLocked(t *testing.T) {
	db, repository, ctx := newNotificationMySQLIntegrationRepository(t)
	first := testEvent("mysql-claim-locked", 1)
	second := testEvent("mysql-claim-visible", 2)
	require.NoError(t, repository.Append(ctx, first))
	require.NoError(t, repository.Append(ctx, second))

	lockTx := db.WithContext(ctx).Begin()
	require.NoError(t, lockTx.Error)
	var locked outboxPO
	require.NoError(t, lockTx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("event_id = ?", first.EventID).
		Take(&locked).Error)
	t.Cleanup(func() {
		_ = lockTx.Rollback().Error
	})

	claims, err := repository.ClaimOutboxBatch(
		ctx,
		"mysql-skip-locked-worker",
		time.Now().Add(time.Second),
		30*time.Second,
		10,
	)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, second.EventID, claims[0].Event.EventID)
}

func TestNotificationMySQLIntegrationAppendValidatesCurrentRowRegardlessOfAffectedRows(
	t *testing.T,
) {
	db, repository, ctx := newNotificationMySQLIntegrationRepository(t)
	original := testEvent("mysql-idempotency-original", 1)
	require.NoError(t, repository.Append(ctx, original))

	retry := original
	retry.EventID = "mysql-idempotency-retry"
	retry.OccurredAt = original.OccurredAt.Add(time.Hour)
	require.NoError(t, repository.Append(ctx, retry))

	var count int64
	require.NoError(t, db.Model(&outboxPO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	conflict := retry
	conflict.EventID = "mysql-idempotency-conflict"
	conflict.Payload.TargetID = "thread-conflict"
	require.ErrorIs(
		t,
		repository.Append(ctx, conflict),
		domainnotification.ErrIdempotencyConflict,
	)
}

func TestNotificationMySQLIntegrationSequenceFollowsCommitOrder(t *testing.T) {
	db, repository, ctx := newNotificationMySQLIntegrationRepository(t)
	firstTx := db.WithContext(ctx).Begin()
	require.NoError(t, firstTx.Error)
	t.Cleanup(func() {
		_ = firstTx.Rollback().Error
	})
	first, err := repository.allocateRecipientSequences(
		ctx,
		firstTx,
		2,
		time.Now().UnixMilli(),
	)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, first)

	type allocationResult struct {
		sequences []int64
		err       error
	}
	started := make(chan struct{})
	finished := make(chan allocationResult, 1)
	go func() {
		secondTx := db.WithContext(ctx).Begin()
		if secondTx.Error != nil {
			finished <- allocationResult{err: secondTx.Error}
			return
		}
		close(started)
		sequences, allocationErr := repository.allocateRecipientSequences(
			ctx,
			secondTx,
			1,
			time.Now().UnixMilli(),
		)
		if allocationErr == nil {
			allocationErr = secondTx.Commit().Error
		} else {
			_ = secondTx.Rollback().Error
		}
		finished <- allocationResult{
			sequences: sequences,
			err:       allocationErr,
		}
	}()

	<-started
	select {
	case result := <-finished:
		require.Failf(
			t,
			"later transaction allocated before earlier commit",
			"sequences=%v error=%v",
			result.sequences,
			result.err,
		)
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, firstTx.Commit().Error)
	select {
	case result := <-finished:
		require.NoError(t, result.err)
		require.Equal(t, []int64{3}, result.sequences)
	case <-time.After(5 * time.Second):
		require.Fail(t, "later sequence allocation did not resume")
	}
}

func TestNotificationMySQLIntegrationDeadlockRetries(t *testing.T) {
	db, repository, ctx := newNotificationMySQLIntegrationRepository(t)
	require.NoError(t, repository.Append(
		ctx,
		testEvent("mysql-deadlock-a", 1),
	))
	require.NoError(t, repository.Append(
		ctx,
		testEvent("mysql-deadlock-b", 2),
	))
	var rows []outboxPO
	require.NoError(t, db.Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	t.Cleanup(closeRelease)
	results := make(chan error, 2)
	var firstAttempts atomic.Int32
	var secondAttempts atomic.Int32
	lockInOrder := func(firstID int64, secondID int64, attempts *atomic.Int32) {
		results <- runNotificationTransaction(ctx, db, func(tx *gorm.DB) error {
			attempt := attempts.Add(1)
			if err := lockNotificationOutboxRow(ctx, tx, firstID); err != nil {
				return err
			}
			if attempt == 1 {
				ready <- struct{}{}
				<-release
			}
			return lockNotificationOutboxRow(ctx, tx, secondID)
		})
	}
	go lockInOrder(rows[0].ID, rows[1].ID, &firstAttempts)
	go lockInOrder(rows[1].ID, rows[0].ID, &secondAttempts)

	for range 2 {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "deadlock transactions did not reach barrier")
		}
	}
	closeRelease()
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.GreaterOrEqual(
		t,
		firstAttempts.Load()+secondAttempts.Load(),
		int32(3),
	)
}

func TestNotificationMySQLIntegrationDatabaseClockIgnoresSessionTimeZone(
	t *testing.T,
) {
	db, _, ctx := newNotificationMySQLIntegrationRepository(t)
	err := db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
		var originalTimeZone string
		if err := pinned.
			Raw("SELECT @@session.time_zone").
			Scan(&originalTimeZone).Error; err != nil {
			return fmt.Errorf("read original session time zone")
		}
		if err := pinned.Exec(
			"SET SESSION time_zone = ?",
			"+08:00",
		).Error; err != nil {
			return fmt.Errorf("set non-UTC session time zone")
		}
		defer func() {
			restoreCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if restoreErr := pinned.WithContext(restoreCtx).Exec(
				"SET SESSION time_zone = ?",
				originalTimeZone,
			).Error; restoreErr != nil {
				t.Errorf("restore original session time zone")
			}
		}()

		repository := NewMySQLRepository(
			pinned,
			&sequenceIDGenerator{next: 200_000},
		)
		var expectedEpochMillis int64
		if err := pinned.Raw(`
			SELECT TIMESTAMPDIFF(
				MICROSECOND,
				'1970-01-01 00:00:00',
				UTC_TIMESTAMP(3)
			) DIV 1000
		`).Scan(&expectedEpochMillis).Error; err != nil {
			return fmt.Errorf("read expected database UTC clock")
		}
		actual, err := repository.currentDatabaseTime(
			ctx,
			pinned,
			time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			return err
		}
		require.WithinDuration(
			t,
			time.UnixMilli(expectedEpochMillis).UTC(),
			actual,
			100*time.Millisecond,
		)
		require.WithinDuration(t, time.Now().UTC(), actual, time.Minute)

		event := testEvent("mysql-database-utc", 1)
		if err := repository.Append(ctx, event); err != nil {
			return err
		}
		beforeClaim, err := repository.currentDatabaseTime(
			ctx,
			pinned,
			time.Time{},
		)
		if err != nil {
			return err
		}
		lease := 30 * time.Second
		claims, err := repository.ClaimOutboxBatch(
			ctx,
			"mysql-utc-worker",
			time.UnixMilli(1),
			lease,
			1,
		)
		if err != nil {
			return err
		}
		require.Len(t, claims, 1)
		afterClaim, err := repository.currentDatabaseTime(
			ctx,
			pinned,
			time.Time{},
		)
		if err != nil {
			return err
		}
		require.False(
			t,
			claims[0].LeaseExpiresAt.Before(beforeClaim.Add(lease)),
		)
		require.False(
			t,
			claims[0].LeaseExpiresAt.After(afterClaim.Add(lease)),
		)
		return nil
	})
	require.NoError(t, err)
}

func TestNotificationMySQLIntegrationTwoPoolsRecoverStaleLease(
	t *testing.T,
) {
	dbA, repositoryA, ctx := newNotificationMySQLIntegrationRepository(t)
	gate, skipReason, err := loadNotificationMySQLIntegrationGate(os.Getenv)
	require.Empty(t, skipReason)
	require.NoError(t, err)
	secondConfig := *gate.config
	dbB, err := gorm.Open(
		gormmysql.New(gormmysql.Config{DSNConfig: &secondConfig}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	require.NoError(t, err)
	sqlA, err := dbA.DB()
	require.NoError(t, err)
	sqlB, err := dbB.DB()
	require.NoError(t, err)
	require.NotSame(t, sqlA, sqlB)
	sqlB.SetMaxOpenConns(4)
	sqlB.SetMaxIdleConns(4)
	t.Cleanup(func() {
		if closeErr := sqlB.Close(); closeErr != nil {
			t.Errorf("close second MySQL pool: %v", closeErr)
		}
	})
	require.NoError(t, sqlB.PingContext(ctx))
	repositoryB := NewMySQLRepository(
		dbB,
		&sequenceIDGenerator{next: 300_000},
	)

	event := testEvent("mysql-two-pool-stale-lease", 1)
	require.NoError(t, repositoryA.Append(ctx, event))
	lease := 30 * time.Second
	firstClaims, err := repositoryA.ClaimOutboxBatch(
		ctx,
		"mysql-worker-a",
		time.UnixMilli(1),
		lease,
		1,
	)
	require.NoError(t, err)
	require.Len(t, firstClaims, 1)
	secondClaims, err := repositoryB.ClaimOutboxBatch(
		ctx,
		"mysql-worker-b",
		time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		lease,
		1,
	)
	require.NoError(t, err)
	require.Empty(t, secondClaims)

	databaseNow, err := repositoryA.currentDatabaseTime(
		ctx,
		dbA,
		time.Time{},
	)
	require.NoError(t, err)
	require.NoError(t, dbA.Model(&outboxPO{}).
		Where("event_id = ?", event.EventID).
		Update(
			"locked_at",
			databaseNow.Add(-lease-time.Second).UnixMilli(),
		).Error)
	recovered, err := repositoryB.RecoverExpiredLeases(
		ctx,
		time.UnixMilli(1),
		lease,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), recovered)
	secondClaims, err = repositoryB.ClaimOutboxBatch(
		ctx,
		"mysql-worker-b",
		time.UnixMilli(1),
		lease,
		1,
	)
	require.NoError(t, err)
	require.Len(t, secondClaims, 1)

	draft, err := domainnotification.DefaultTemplateRegistry().Render(event)
	require.NoError(t, err)
	require.ErrorIs(
		t,
		repositoryA.MaterializeAndDeliver(
			ctx,
			firstClaims[0],
			draft,
			[]int64{101},
			time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		),
		domainnotification.ErrLeaseLost,
	)
}

func newNotificationMySQLIntegrationRepository(
	t *testing.T,
) (*gorm.DB, *MySQLRepository, context.Context) {
	t.Helper()
	gate, skipReason, err := loadNotificationMySQLIntegrationGate(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	require.NoError(t, err)
	db, err := gorm.Open(
		gormmysql.New(gormmysql.Config{DSNConfig: gate.config}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(16)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, sqlDB.PingContext(ctx))

	var databaseName string
	require.NoError(t, db.WithContext(ctx).
		Raw("SELECT DATABASE()").
		Row().
		Scan(&databaseName))
	require.Equal(t, gate.config.DBName, databaseName)
	require.True(t, notificationMySQLDatabasePattern.MatchString(databaseName))

	cleanup := func() {
		for _, table := range []any{
			&recipientPO{},
			&messagePO{},
			&outboxPO{},
			&notificationSequencePO{},
		} {
			_ = db.Session(&gorm.Session{Logger: logger.Discard}).
				Migrator().
				DropTable(table)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	require.NoError(t, db.WithContext(ctx).AutoMigrate(
		&outboxPO{},
		&messagePO{},
		&notificationSequencePO{},
		&recipientPO{},
	))
	require.NoError(t, db.WithContext(ctx).Create(&notificationSequencePO{
		SequenceKey: notificationRecipientSequenceKey,
		NextValue:   1,
		UpdatedAt:   time.Now().UnixMilli(),
	}).Error)
	return db, NewMySQLRepository(
		db,
		&sequenceIDGenerator{next: 100_000},
	), ctx
}

func lockNotificationOutboxRow(
	ctx context.Context,
	tx *gorm.DB,
	id int64,
) error {
	var row outboxPO
	return tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		Take(&row).Error
}
