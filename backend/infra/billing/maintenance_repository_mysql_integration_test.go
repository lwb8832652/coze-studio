// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

const billingMySQLDDLConfirmation = "YES_I_OWN_THIS_DISPOSABLE_SCHEMA"

var billingMySQLDatabasePattern = regexp.MustCompile(`^coze_billing_it_[a-z0-9_]+$`)

type billingMySQLIntegrationGate struct {
	config *mysqldriver.Config
}

func loadBillingMySQLIntegrationGate(getenv func(string) string) (billingMySQLIntegrationGate, string, error) {
	dsn := strings.TrimSpace(getenv("COZE_BILLING_MYSQL_TEST_DSN"))
	if dsn == "" {
		return billingMySQLIntegrationGate{}, "COZE_BILLING_MYSQL_TEST_DSN is not set", nil
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return billingMySQLIntegrationGate{}, "", fmt.Errorf("invalid billing MySQL integration DSN")
	}
	if !billingMySQLDatabasePattern.MatchString(config.DBName) {
		return billingMySQLIntegrationGate{}, "", fmt.Errorf("database must be a disposable coze_billing_it_* schema")
	}
	if getenv("COZE_BILLING_MYSQL_TEST_ALLOW_DDL") != billingMySQLDDLConfirmation {
		return billingMySQLIntegrationGate{}, "", fmt.Errorf("explicit disposable-schema DDL confirmation is missing")
	}
	config.ParseTime = true
	config.Loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	return billingMySQLIntegrationGate{config: config}, "", nil
}

func TestMaintenanceRepositoryMySQLConcurrentProjectionUsesRowLockAndAtomicIdempotency(t *testing.T) {
	db, repository, ctx := newBillingMySQLIntegrationRepository(t)
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, time.July, 24, 9, 0, 0, 0, shanghai)
	seedMaintenanceRepositorySubscription(t, db, 801, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9801}, 1801, now.Add(24*time.Hour), 2)

	firstBatch, err := repository.ScanExpiringSubscriptions(ctx, now, 72*time.Hour, 10)
	require.NoError(t, err)
	require.Len(t, firstBatch.Candidates, 1)
	secondBatch, err := repository.ScanExpiringSubscriptions(ctx, now, 72*time.Hour, 10)
	require.NoError(t, err)
	require.Len(t, secondBatch.Candidates, 1)

	type projectionResult struct {
		outcome domainbilling.MaintenanceProjectionOutcome
		err     error
	}
	start := make(chan struct{})
	results := make(chan projectionResult, 2)
	var wait sync.WaitGroup
	for _, candidate := range []domainbilling.MaintenanceSubscriptionCandidate{
		firstBatch.Candidates[0], secondBatch.Candidates[0],
	} {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			outcome, projectErr := repository.ProjectExpiringSubscription(ctx, candidate, now, 72*time.Hour)
			results <- projectionResult{outcome: outcome, err: projectErr}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	inserted, duplicates := 0, 0
	for result := range results {
		require.NoError(t, result.err)
		if result.outcome.NotificationInserted {
			inserted++
		} else if !result.outcome.Stale {
			duplicates++
		}
	}
	require.Equal(t, 1, inserted)
	require.Equal(t, 1, duplicates)

	remaining, err := repository.ScanExpiringSubscriptions(ctx, now, 72*time.Hour, 10)
	require.NoError(t, err)
	require.Empty(t, remaining.Candidates)
	var count int64
	require.NoError(t, db.Model(&maintenanceRepositoryTestOutboxPO{}).
		Where("event_type = ?", string(domainnotification.EventBillingSubscriptionExpiring)).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func newBillingMySQLIntegrationRepository(t *testing.T) (*gorm.DB, *MaintenanceRepository, context.Context) {
	t.Helper()
	gate, skipReason, err := loadBillingMySQLIntegrationGate(os.Getenv)
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
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, sqlDB.PingContext(ctx))
	var databaseName string
	require.NoError(t, db.WithContext(ctx).Raw("SELECT DATABASE()").Row().Scan(&databaseName))
	require.Equal(t, gate.config.DBName, databaseName)
	require.True(t, billingMySQLDatabasePattern.MatchString(databaseName))

	cleanup := func() {
		for _, table := range []any{
			&maintenanceRepositoryTestSubscriptionPO{},
			&maintenanceRepositoryTestOrderPO{},
			&maintenanceRepositoryTestAccountPO{},
			&maintenanceRepositoryTestOutboxPO{},
		} {
			_ = db.Session(&gorm.Session{Logger: logger.Discard}).Migrator().DropTable(table)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	require.NoError(t, db.WithContext(ctx).AutoMigrate(
		&maintenanceRepositoryTestAccountPO{},
		&maintenanceRepositoryTestOrderPO{},
		&maintenanceRepositoryTestSubscriptionPO{},
		&maintenanceRepositoryTestOutboxPO{},
	))
	return db, NewMaintenanceRepository(db, &maintenanceRepositoryTestIDGenerator{next: 200_000}), ctx
}
