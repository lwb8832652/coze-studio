/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMySQLIntegrationMigrationCompatibleContract(t *testing.T) {
	gate, skipReason, err := loadMySQLIntegrationGate(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil {
		t.Fatalf("unsafe MySQL integration configuration: %v", err)
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{DSNConfig: gate.config}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open SANDBOX_MYSQL_INTEGRATION_DSN: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Errorf("close MySQL integration database: %v", closeErr)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping SANDBOX_MYSQL_INTEGRATION_DSN: %v", err)
	}
	var actualDatabase string
	if err := db.WithContext(ctx).Raw("SELECT DATABASE()").Row().Scan(&actualDatabase); err != nil {
		t.Fatalf("query MySQL integration database name: %v", err)
	}
	if actualDatabase != gate.config.DBName || !mysqlIntegrationDatabasePattern.MatchString(actualDatabase) {
		t.Fatalf("connected database %q does not match the exclusive integration gate", actualDatabase)
	}
	var actualGuard string
	if err := db.WithContext(ctx).Raw(
		"SELECT guard_value FROM sandbox_mysql_integration_guard WHERE guard_key = ?",
		mysqlIntegrationGuardKey,
	).Row().Scan(&actualGuard); err != nil {
		t.Fatalf("read pre-existing MySQL integration guard: %v", err)
	}
	if actualGuard != gate.guard {
		t.Fatal("pre-existing MySQL integration guard value does not match SANDBOX_MYSQL_INTEGRATION_GUARD")
	}
	for _, table := range []string{"sandbox_providers", "sandbox_provider_defaults", "sandbox_provider_audit_events"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("required migrated table %q is missing", table)
		}
	}
	if !db.Migrator().HasColumn(&providerPO{}, "last_health_capabilities_json") {
		t.Fatal("required migrated column last_health_capabilities_json is missing")
	}
	verifyMySQLInformationSchema(t, db)
	keySuffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	verifyMySQLStatusCASDeleteLocking(t, db, keySuffix)
	verifyMySQLDefaultDeleteLockOrdering(t, db, keySuffix)

	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin isolated MySQL test transaction: %v", tx.Error)
	}
	defer func() {
		if rollbackErr := tx.Rollback().Error; rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			t.Errorf("rollback MySQL integration transaction: %v", rollbackErr)
		}
	}()
	repository := NewMySQLRepository(tx)
	created, err := repository.CreateProvider(ctx, validCreateProviderInput("mysql-"+keySuffix))
	if err != nil {
		t.Fatalf("CreateProvider(MySQL) error = %v", err)
	}
	var scopesValid, policyValid, capabilitiesValid int
	if err := tx.Raw(`SELECT JSON_VALID(scopes_json), JSON_VALID(policy_json), JSON_VALID(last_health_capabilities_json) FROM sandbox_providers WHERE id = ?`, created.ID).
		Row().Scan(&scopesValid, &policyValid, &capabilitiesValid); err != nil {
		t.Fatalf("read MySQL JSON validity: %v", err)
	}
	if scopesValid != 1 || policyValid != 1 || capabilitiesValid != 1 {
		t.Fatalf("MySQL JSON validity = %d, %d, %d", scopesValid, policyValid, capabilitiesValid)
	}
	second, err := repository.CreateProvider(ctx, validCreateProviderInput("mysql-second-"+keySuffix))
	if err != nil {
		t.Fatalf("CreateProvider(MySQL second NULL legacy hash) error = %v", err)
	}
	var nullLegacyHashes int64
	if err := tx.Model(&providerPO{}).Where("id IN ? AND legacy_source_hash IS NULL", []int64{created.ID, second.ID}).
		Count(&nullLegacyHashes).Error; err != nil {
		t.Fatalf("count MySQL NULL legacy hashes: %v", err)
	}
	if nullLegacyHashes != 2 {
		t.Fatalf("MySQL nullable unique legacy hashes = %d, want 2", nullLegacyHashes)
	}
	scopeItems, scopeTotal, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		Keyword: "mysql-", Scope: domainsandbox.ScopeAgent, Limit: 100,
	})
	if err != nil || scopeTotal < 2 || len(scopeItems) < 2 {
		t.Fatalf("ListProviders(MySQL JSON_CONTAINS scope) = %d items, total %d, err %v", len(scopeItems), scopeTotal, err)
	}
	verifyMySQLRepeatableReadListSnapshot(t, db, keySuffix)
	if _, err := repository.CreateProvider(ctx, validCreateProviderInput("mysql-"+keySuffix)); !errors.Is(err, domainsandbox.ErrProviderAlreadyExists) {
		t.Fatalf("CreateProvider(MySQL duplicate) error = %v", err)
	}
	nextVersion, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: created.ID, ExpectedVersion: created.Version,
		Status: domainsandbox.ProviderStatusEnabled, ActorUserID: 801,
	})
	if err != nil || nextVersion != 2 {
		t.Fatalf("UpdateProviderStatus(MySQL) = %d, %v", nextVersion, err)
	}
	var storedVersion uint64
	if err := tx.Model(&providerPO{}).Select("version").Where("id = ?", created.ID).Scan(&storedVersion).Error; err != nil {
		t.Fatalf("read MySQL CAS version: %v", err)
	}
	if storedVersion != nextVersion {
		t.Fatalf("MySQL stored CAS version = %d, want %d", storedVersion, nextVersion)
	}
	if _, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: created.ID, ExpectedVersion: created.Version,
		Status: domainsandbox.ProviderStatusDisabled, ActorUserID: 801,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("UpdateProviderStatus(MySQL stale) error = %v", err)
	}

	rollbackCause := errors.New("verify MySQL savepoint rollback")
	rollbackKey := "mysql-rollback-" + keySuffix
	err = repository.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		_, err := repositories.Providers.CreateProvider(txCtx, validCreateProviderInput(rollbackKey))
		if err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("WithinTransaction(MySQL rollback) error = %v", err)
	}
	if _, err := repository.GetProviderByKey(ctx, rollbackKey); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("MySQL rolled-back provider error = %v", err)
	}

	audit, err := repository.AppendProviderAuditEvent(ctx, domainsandbox.AppendProviderAuditEventInput{
		ProviderID: created.ID, ActorUserID: 803, Action: "provider.delete", Result: "success",
		RequestID: "mysql-audit-" + keySuffix, Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "2"},
	})
	if err != nil {
		t.Fatalf("AppendProviderAuditEvent(MySQL) error = %v", err)
	}
	deletedVersion, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: created.ID, ExpectedVersion: nextVersion, ActorUserID: 803,
	})
	if err != nil || deletedVersion != nextVersion+1 {
		t.Fatalf("DeleteProvider(MySQL) = %d, %v", deletedVersion, err)
	}
	audits, auditTotal, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{
		ProviderID: created.ID, Limit: 10,
	})
	if err != nil || auditTotal != 1 || len(audits) != 1 || audits[0].ID != audit.ID {
		t.Fatalf("retained MySQL audit = %#v, total %d, err %v", audits, auditTotal, err)
	}
}

const (
	mysqlIntegrationExclusiveConfirmation = "YES_I_OWN_THIS_SCHEMA"
	mysqlIntegrationGuardKey              = "sandbox_control_plane"
)

var mysqlIntegrationDatabasePattern = regexp.MustCompile(`^coze_sandbox_it_[a-z0-9_]+$`)

type mysqlIntegrationGate struct {
	config *mysqldriver.Config
	guard  string
}

func loadMySQLIntegrationGate(getenv func(string) string) (mysqlIntegrationGate, string, error) {
	dsn := strings.TrimSpace(getenv("SANDBOX_MYSQL_INTEGRATION_DSN"))
	if dsn == "" {
		return mysqlIntegrationGate{}, "SANDBOX_MYSQL_INTEGRATION_DSN is not set; no exclusive MySQL schema was supplied", nil
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return mysqlIntegrationGate{}, "", fmt.Errorf("parse SANDBOX_MYSQL_INTEGRATION_DSN: %w", err)
	}
	if !mysqlIntegrationDatabasePattern.MatchString(config.DBName) {
		return mysqlIntegrationGate{}, "", fmt.Errorf("database name does not match the exclusive integration pattern")
	}
	if getenv("SANDBOX_MYSQL_INTEGRATION_CONFIRM_EXCLUSIVE") != mysqlIntegrationExclusiveConfirmation {
		return mysqlIntegrationGate{}, "", fmt.Errorf("exclusive schema confirmation is missing")
	}
	guard := getenv("SANDBOX_MYSQL_INTEGRATION_GUARD")
	if guard == "" || len(guard) > 256 {
		return mysqlIntegrationGate{}, "", fmt.Errorf("integration guard is missing or invalid")
	}
	return mysqlIntegrationGate{config: config, guard: guard}, "", nil
}

func verifyMySQLInformationSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	type columnExpectation struct {
		table      string
		column     string
		columnType string
		nullable   string
	}
	expectations := []columnExpectation{
		{table: "sandbox_providers", column: "id", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_providers", column: "max_concurrency", columnType: "int unsigned", nullable: "NO"},
		{table: "sandbox_providers", column: "last_health_latency_ms", columnType: "int unsigned", nullable: "NO"},
		{table: "sandbox_providers", column: "version", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_providers", column: "created_by", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_providers", column: "updated_by", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_providers", column: "endpoint_secret", columnType: "text", nullable: "YES"},
		{table: "sandbox_providers", column: "credential_secret", columnType: "text", nullable: "YES"},
		{table: "sandbox_providers", column: "legacy_source_hash", columnType: "varchar(64)", nullable: "YES"},
		{table: "sandbox_providers", column: "last_health_capabilities_json", columnType: "json", nullable: "NO"},
		{table: "sandbox_providers", column: "last_health_at", columnType: "datetime(3)", nullable: "YES"},
		{table: "sandbox_provider_defaults", column: "provider_id", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_provider_defaults", column: "version", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_provider_defaults", column: "updated_by", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_provider_audit_events", column: "event_id", columnType: "bigint unsigned", nullable: "NO"},
		{table: "sandbox_provider_audit_events", column: "provider_id", columnType: "bigint unsigned", nullable: "YES"},
		{table: "sandbox_provider_audit_events", column: "actor_user_id", columnType: "bigint unsigned", nullable: "NO"},
	}
	for _, expectation := range expectations {
		var actual struct {
			ColumnType string `gorm:"column:COLUMN_TYPE"`
			Nullable   string `gorm:"column:IS_NULLABLE"`
		}
		err := db.Raw(`SELECT COLUMN_TYPE, IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, expectation.table, expectation.column).
			Scan(&actual).Error
		if err != nil {
			t.Fatalf("inspect information_schema column %s.%s: %v", expectation.table, expectation.column, err)
		}
		if strings.ToLower(actual.ColumnType) != expectation.columnType || actual.Nullable != expectation.nullable {
			t.Errorf("information_schema %s.%s = (%q, %q), want (%q, %q)",
				expectation.table, expectation.column, actual.ColumnType, actual.Nullable, expectation.columnType, expectation.nullable)
		}
	}

	type indexExpectation struct {
		table   string
		name    string
		unique  bool
		columns string
	}
	indexes := []indexExpectation{
		{table: "sandbox_providers", name: "PRIMARY", unique: true, columns: "id"},
		{table: "sandbox_providers", name: "uk_sandbox_providers_provider_key", unique: true, columns: "provider_key"},
		{table: "sandbox_providers", name: "uk_sandbox_providers_legacy_source_hash", unique: true, columns: "legacy_source_hash"},
		{table: "sandbox_providers", name: "idx_sandbox_providers_status_deleted", columns: "status,deleted_at"},
		{table: "sandbox_providers", name: "idx_sandbox_providers_created_id", columns: "created_at,id"},
		{table: "sandbox_providers", name: "idx_sandbox_providers_updated_id", columns: "updated_at,id"},
		{table: "sandbox_provider_defaults", name: "PRIMARY", unique: true, columns: "scope"},
		{table: "sandbox_provider_defaults", name: "idx_sandbox_provider_defaults_provider_id", columns: "provider_id"},
		{table: "sandbox_provider_audit_events", name: "PRIMARY", unique: true, columns: "event_id"},
		{table: "sandbox_provider_audit_events", name: "idx_sandbox_audit_provider_created_event", columns: "provider_id,created_at,event_id"},
		{table: "sandbox_provider_audit_events", name: "idx_sandbox_audit_created_event", columns: "created_at,event_id"},
		{table: "sandbox_provider_audit_events", name: "idx_sandbox_audit_action_created_event", columns: "action,created_at,event_id"},
		{table: "sandbox_provider_audit_events", name: "idx_sandbox_audit_result_created_event", columns: "result,created_at,event_id"},
	}
	for _, expectation := range indexes {
		var actual struct {
			NonUnique int    `gorm:"column:NON_UNIQUE"`
			Columns   string `gorm:"column:COLUMNS"`
		}
		err := db.Raw(`SELECT MIN(NON_UNIQUE) AS NON_UNIQUE, GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ',') AS COLUMNS FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ? GROUP BY INDEX_NAME`, expectation.table, expectation.name).
			Scan(&actual).Error
		if err != nil {
			t.Fatalf("inspect information_schema index %s.%s: %v", expectation.table, expectation.name, err)
		}
		wantNonUnique := 1
		if expectation.unique {
			wantNonUnique = 0
		}
		if actual.NonUnique != wantNonUnique || actual.Columns != expectation.columns {
			t.Errorf("information_schema index %s.%s = (%d, %q), want (%d, %q)",
				expectation.table, expectation.name, actual.NonUnique, actual.Columns, wantNonUnique, expectation.columns)
		}
	}
}

type mysqlRepeatableReadListContextKey struct{}

func verifyMySQLRepeatableReadListSnapshot(t *testing.T, db *gorm.DB, keySuffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	prefix := "mysql-rr-list-" + keySuffix
	repository := NewMySQLRepository(db)
	baseline, err := repository.CreateProvider(ctx, validCreateProviderInput(prefix+"-baseline"))
	if err != nil {
		t.Fatalf("create MySQL repeatable-read baseline: %v", err)
	}
	fixtureIDs := []uint64{uint64(baseline.ID)}
	t.Cleanup(func() {
		cleanupMySQLIntegrationFixtures(t, db, fixtureIDs, nil)
	})

	countFinished := make(chan struct{})
	releaseFind := make(chan struct{})
	var countOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFind) }) }
	defer release()
	callbackName := "sandbox:mysql_repeatable_read_list_" + keySuffix
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(mysqlRepeatableReadListContextKey{}) != true {
			return
		}
		if !strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), "COUNT(") {
			return
		}
		countOnce.Do(func() {
			close(countFinished)
			select {
			case <-releaseFind:
			case <-ctx.Done():
			}
		})
	}); err != nil {
		t.Fatalf("register MySQL repeatable-read list callback: %v", err)
	}
	defer func() {
		if removeErr := db.Callback().Query().Remove(callbackName); removeErr != nil {
			t.Errorf("remove MySQL repeatable-read list callback: %v", removeErr)
		}
	}()

	type listResult struct {
		items []*domainsandbox.Provider
		total int64
		err   error
	}
	listDone := make(chan listResult, 1)
	go func() {
		items, total, listErr := repository.ListProviders(
			context.WithValue(ctx, mysqlRepeatableReadListContextKey{}, true),
			domainsandbox.ProviderListRequest{Keyword: prefix, Limit: 100},
		)
		listDone <- listResult{items: items, total: total, err: listErr}
	}()
	select {
	case <-countFinished:
	case early := <-listDone:
		t.Fatalf("MySQL repeatable-read list returned before Count hook: %v", early.err)
	case <-ctx.Done():
		t.Fatalf("wait for MySQL repeatable-read Count hook: %v", ctx.Err())
	}

	var committed *domainsandbox.Provider
	err = db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
		var createErr error
		committed, createErr = NewMySQLRepository(pinned).CreateProvider(
			ctx,
			validCreateProviderInput(prefix+"-committed"),
		)
		return createErr
	})
	if err != nil {
		t.Fatalf("commit independent MySQL repeatable-read fixture: %v", err)
	}
	fixtureIDs = append(fixtureIDs, uint64(committed.ID))
	release()

	var snapshot listResult
	select {
	case snapshot = <-listDone:
	case <-ctx.Done():
		t.Fatalf("wait for MySQL repeatable-read list result: %v", ctx.Err())
	}
	if snapshot.err != nil || snapshot.total != 1 || len(snapshot.items) != 1 {
		t.Fatalf("MySQL repeatable-read list snapshot = %d items, total %d, err %v; want 1/1", len(snapshot.items), snapshot.total, snapshot.err)
	}
	items, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{Keyword: prefix, Limit: 100})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("later MySQL list = %d items, total %d, err %v; want 2/2", len(items), total, err)
	}
}

func mysqlSessionLockWaitTimeoutStatement(value uint64) string {
	return "SET SESSION innodb_lock_wait_timeout = " + strconv.FormatUint(value, 10)
}

func withMySQLSessionLockWaitTimeout(
	t *testing.T,
	db *gorm.DB,
	timeout uint64,
	operation func() error,
) (err error) {
	t.Helper()
	var original uint64
	if err := db.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&original).Error; err != nil {
		return err
	}
	if err := db.Exec(mysqlSessionLockWaitTimeoutStatement(timeout)).Error; err != nil {
		return err
	}
	defer func() {
		if restoreErr := db.Exec(mysqlSessionLockWaitTimeoutStatement(original)).Error; restoreErr != nil {
			t.Errorf("restore MySQL session innodb_lock_wait_timeout: %v", restoreErr)
			err = errors.Join(err, restoreErr)
		}
	}()
	return operation()
}

func cleanupMySQLIntegrationFixtures(
	t *testing.T,
	db *gorm.DB,
	providerIDs []uint64,
	scopes []domainsandbox.Scope,
) {
	t.Helper()
	cleanup := db.Session(&gorm.Session{Logger: logger.Discard}).Begin()
	if cleanup.Error != nil {
		t.Errorf("begin idempotent MySQL integration cleanup: %v", cleanup.Error)
		return
	}
	rollback := func() {
		if err := cleanup.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("rollback idempotent MySQL integration cleanup: %v", err)
		}
	}
	for _, scope := range scopes {
		if err := cleanup.Exec("DELETE FROM sandbox_provider_defaults WHERE scope = ?", string(scope)).Error; err != nil {
			t.Errorf("delete MySQL integration default fixture: %v", err)
			rollback()
			return
		}
	}
	if len(providerIDs) > 0 {
		if err := cleanup.Exec("DELETE FROM sandbox_provider_defaults WHERE provider_id IN ?", providerIDs).Error; err != nil {
			t.Errorf("delete MySQL integration provider default fixtures: %v", err)
			rollback()
			return
		}
		if err := cleanup.Exec("DELETE FROM sandbox_provider_audit_events WHERE provider_id IN ?", providerIDs).Error; err != nil {
			t.Errorf("delete MySQL integration audit fixtures: %v", err)
			rollback()
			return
		}
		if err := cleanup.Exec("DELETE FROM sandbox_providers WHERE id IN ?", providerIDs).Error; err != nil {
			t.Errorf("delete MySQL integration provider fixtures: %v", err)
			rollback()
			return
		}
	}
	if err := cleanup.Commit().Error; err != nil {
		t.Errorf("commit idempotent MySQL integration cleanup: %v", err)
		rollback()
	}
}

func verifyMySQLStatusCASDeleteLocking(t *testing.T, db *gorm.DB, keySuffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := NewMySQLRepository(db)
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("mysql-status-lock-"+keySuffix))
	if err != nil {
		t.Fatalf("create MySQL status lock provider: %v", err)
	}
	t.Cleanup(func() {
		cleanupMySQLIntegrationFixtures(t, db, []uint64{uint64(provider.ID)}, nil)
	})
	currentVersion, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: provider.ID, ExpectedVersion: provider.Version,
		Status: domainsandbox.ProviderStatusEnabled, ActorUserID: 830,
	})
	if err != nil {
		t.Fatalf("prepare MySQL status lock version: %v", err)
	}

	lockAcquired := make(chan struct{})
	competitorReached := make(chan struct{})
	releaseStatus := make(chan struct{})
	var lockOnce sync.Once
	var competitorOnce sync.Once
	var releaseOnce sync.Once
	releaseHolder := func() { releaseOnce.Do(func() { close(releaseStatus) }) }
	defer releaseHolder()
	callbackName := "sandbox:mysql_status_lock_" + keySuffix
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "sandbox_providers" {
			return
		}
		if tx.Statement.Context.Value(mysqlStatusLockContextKey{}) == true {
			lockOnce.Do(func() {
				close(lockAcquired)
				select {
				case <-releaseStatus:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		t.Fatalf("register MySQL status lock callback: %v", err)
	}
	competitorCallbackName := callbackName + "_competitor"
	if err := db.Callback().Query().Before("gorm:query").Register(competitorCallbackName, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(mysqlCompetingDeleteContextKey{}) == true {
			competitorOnce.Do(func() { close(competitorReached) })
		}
	}); err != nil {
		t.Fatalf("register competing MySQL delete callback: %v", err)
	}
	defer func() {
		if removeErr := db.Callback().Query().Remove(callbackName); removeErr != nil {
			t.Errorf("remove MySQL status lock callback: %v", removeErr)
		}
		if removeErr := db.Callback().Query().Remove(competitorCallbackName); removeErr != nil {
			t.Errorf("remove competing MySQL delete callback: %v", removeErr)
		}
	}()

	statusDone := make(chan error, 1)
	go func() {
		statusDone <- db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
			_, err := NewMySQLRepository(pinned).UpdateProviderStatus(
				context.WithValue(ctx, mysqlStatusLockContextKey{}, true),
				domainsandbox.UpdateProviderStatusInput{
					ProviderID: provider.ID, ExpectedVersion: provider.Version,
					Status: domainsandbox.ProviderStatusDisabled, ActorUserID: 831,
				},
			)
			return err
		})
	}()
	select {
	case <-lockAcquired:
	case earlyErr := <-statusDone:
		t.Fatalf("MySQL status holder returned before acquiring its row lock: %v", earlyErr)
	case <-ctx.Done():
		t.Fatalf("wait for MySQL status row lock: %v", ctx.Err())
	}

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
			return withMySQLSessionLockWaitTimeout(t, pinned, 1, func() error {
				_, err := NewMySQLRepository(pinned).DeleteProvider(
					context.WithValue(ctx, mysqlCompetingDeleteContextKey{}, true),
					domainsandbox.DeleteProviderInput{
						ProviderID: provider.ID, ExpectedVersion: currentVersion, ActorUserID: 832,
					},
				)
				return err
			})
		})
	}()
	select {
	case <-competitorReached:
	case earlyErr := <-deleteDone:
		t.Fatalf("competing MySQL delete returned before reaching the locked query: %v", earlyErr)
	case <-ctx.Done():
		t.Fatalf("wait for competing MySQL delete query: %v", ctx.Err())
	}
	var lockWaitErr error
	select {
	case lockWaitErr = <-deleteDone:
	case <-ctx.Done():
		t.Fatalf("wait for MySQL lock-wait result: %v", ctx.Err())
	}
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(lockWaitErr, &mysqlError) || mysqlError.Number != 1205 {
		t.Fatalf("competing MySQL delete lock error = %v, want typed MySQL 1205", lockWaitErr)
	}
	releaseHolder()
	select {
	case err := <-statusDone:
		if !errors.Is(err, domainsandbox.ErrVersionConflict) {
			t.Fatalf("stale MySQL status classification error = %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("wait for stale MySQL status result: %v", ctx.Err())
	}
	deletedVersion, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: provider.ID, ExpectedVersion: currentVersion, ActorUserID: 833,
	})
	if err != nil || deletedVersion != currentVersion+1 {
		t.Fatalf("DeleteProvider(after MySQL status conflict) = %d, %v", deletedVersion, err)
	}
}

func verifyMySQLDefaultDeleteLockOrdering(t *testing.T, db *gorm.DB, keySuffix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := NewMySQLRepository(db)

	if err := db.WithContext(ctx).Exec(
		"DELETE FROM sandbox_provider_defaults WHERE scope = ?",
		string(domainsandbox.ScopeAppDev),
	).Error; err != nil {
		t.Fatalf("clear exclusive MySQL appdev default fixture: %v", err)
	}
	fixtureIDs := make([]uint64, 0, 2)
	t.Cleanup(func() {
		cleanupMySQLIntegrationFixtures(t, db, fixtureIDs, []domainsandbox.Scope{domainsandbox.ScopeAppDev})
	})
	first, err := repository.CreateProvider(ctx, validCreateProviderInput("mysql-lock-a-"+keySuffix))
	if err != nil {
		t.Fatalf("create MySQL lock provider A: %v", err)
	}
	fixtureIDs = append(fixtureIDs, uint64(first.ID))
	second, err := repository.CreateProvider(ctx, validCreateProviderInput("mysql-lock-b-"+keySuffix))
	if err != nil {
		t.Fatalf("create MySQL lock provider B: %v", err)
	}
	fixtureIDs = append(fixtureIDs, uint64(second.ID))
	current, err := repository.SetProviderDefault(ctx, domainsandbox.SetProviderDefaultInput{
		Scope: domainsandbox.ScopeAppDev, ProviderID: first.ID, ExpectedVersion: 0, ActorUserID: 820,
	})
	if err != nil {
		t.Fatalf("set MySQL lock default to provider A: %v", err)
	}

	lockAcquired := make(chan struct{})
	competitorReached := make(chan struct{})
	releaseDelete := make(chan struct{})
	var lockOnce sync.Once
	var competitorOnce sync.Once
	var releaseOnce sync.Once
	releaseHolder := func() { releaseOnce.Do(func() { close(releaseDelete) }) }
	defer releaseHolder()
	callbackName := "sandbox:mysql_delete_lock_" + keySuffix
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "sandbox_providers" && tx.Statement.Context.Value(mysqlDeleteLockContextKey{}) == true {
			lockOnce.Do(func() {
				close(lockAcquired)
				select {
				case <-releaseDelete:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		t.Fatalf("register MySQL delete lock callback: %v", err)
	}
	competitorCallbackName := callbackName + "_competitor"
	if err := db.Callback().Query().Before("gorm:query").Register(competitorCallbackName, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(mysqlCompetingDefaultContextKey{}) == true {
			competitorOnce.Do(func() { close(competitorReached) })
		}
	}); err != nil {
		t.Fatalf("register competing MySQL default callback: %v", err)
	}
	defer func() {
		if removeErr := db.Callback().Query().Remove(callbackName); removeErr != nil {
			t.Errorf("remove MySQL delete lock callback: %v", removeErr)
		}
		if removeErr := db.Callback().Query().Remove(competitorCallbackName); removeErr != nil {
			t.Errorf("remove competing MySQL default callback: %v", removeErr)
		}
	}()

	deleteDone := make(chan struct {
		version uint64
		err     error
	}, 1)
	go func() {
		result := struct {
			version uint64
			err     error
		}{}
		result.err = db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
			result.version, result.err = NewMySQLRepository(pinned).DeleteProvider(
				context.WithValue(ctx, mysqlDeleteLockContextKey{}, true),
				domainsandbox.DeleteProviderInput{ProviderID: first.ID, ExpectedVersion: first.Version, ActorUserID: 821},
			)
			return result.err
		})
		deleteDone <- result
	}()
	select {
	case <-lockAcquired:
	case early := <-deleteDone:
		t.Fatalf("MySQL delete holder returned before acquiring provider lock: %v", early.err)
	case <-ctx.Done():
		t.Fatalf("wait for MySQL delete holder lock: %v", ctx.Err())
	}

	moveDone := make(chan struct {
		value *domainsandbox.ProviderDefault
		err   error
	}, 1)
	go func() {
		result := struct {
			value *domainsandbox.ProviderDefault
			err   error
		}{}
		result.err = db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
			return withMySQLSessionLockWaitTimeout(t, pinned, 2, func() error {
				result.value, result.err = NewMySQLRepository(pinned).SetProviderDefault(
					context.WithValue(ctx, mysqlCompetingDefaultContextKey{}, true),
					domainsandbox.SetProviderDefaultInput{
						Scope: domainsandbox.ScopeAppDev, ProviderID: second.ID,
						ExpectedVersion: current.Version, ActorUserID: 822,
					},
				)
				return result.err
			})
		})
		moveDone <- result
	}()
	select {
	case <-competitorReached:
	case early := <-moveDone:
		releaseHolder()
		t.Fatalf("competing MySQL default returned before reaching database query: %v", early.err)
	case <-ctx.Done():
		releaseHolder()
		t.Fatalf("wait for competing MySQL default query: %v", ctx.Err())
	}
	var moveResult struct {
		value *domainsandbox.ProviderDefault
		err   error
	}
	select {
	case moveResult = <-moveDone:
	case <-ctx.Done():
		releaseHolder()
		t.Fatalf("wait for competing MySQL default update: %v", ctx.Err())
	}
	if moveResult.err != nil {
		releaseHolder()
		t.Fatalf("move MySQL default while delete holds provider lock: %v", moveResult.err)
	}
	moved := moveResult.value
	if moved.ProviderID != second.ID || moved.Version != current.Version+1 {
		releaseHolder()
		t.Fatalf("moved MySQL default = %#v", moved)
	}
	releaseHolder()
	var result struct {
		version uint64
		err     error
	}
	select {
	case result = <-deleteDone:
	case <-ctx.Done():
		t.Fatalf("wait for MySQL delete holder result: %v", ctx.Err())
	}
	if result.err != nil || result.version != first.Version+1 {
		t.Fatalf("MySQL locked DeleteProvider() = %d, %v", result.version, result.err)
	}
}

type mysqlDeleteLockContextKey struct{}

type mysqlStatusLockContextKey struct{}

type mysqlCompetingDeleteContextKey struct{}

type mysqlCompetingDefaultContextKey struct{}
