// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	runtimeSessionDevMySQLDSNEnv          = "SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN"
	runtimeSessionDevMySQLDatabaseEnv     = "SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE"
	runtimeSessionDevMySQLRollbackOnlyEnv = "SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY"
	runtimeSessionDevMySQLRollbackOnlyAck = "ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE"
)

type runtimeSessionDevMySQLGate struct {
	config       *mysqldriver.Config
	databaseName string
}

func TestRuntimeSessionDevMySQLGateRequiresExplicitDSN(t *testing.T) {
	_, skipReason, err := loadRuntimeSessionDevMySQLGate(func(string) string { return "" })
	if err != nil || skipReason == "" {
		t.Fatalf("empty dev MySQL gate = skip %q, error %v; want a clean skip", skipReason, err)
	}
}

func TestRuntimeSessionDevMySQLGateRejectsPartialOrUnsafeConfiguration(t *testing.T) {
	const dsn = "runtime_session_probe@tcp(mysql.dev.invalid:3306)/coze_dev?parseTime=true&loc=UTC"
	tests := map[string]map[string]string{
		"missing explicit database": {
			runtimeSessionDevMySQLDSNEnv:          dsn,
			runtimeSessionDevMySQLRollbackOnlyEnv: runtimeSessionDevMySQLRollbackOnlyAck,
		},
		"database mismatch": {
			runtimeSessionDevMySQLDSNEnv:          dsn,
			runtimeSessionDevMySQLDatabaseEnv:     "another_dev",
			runtimeSessionDevMySQLRollbackOnlyEnv: runtimeSessionDevMySQLRollbackOnlyAck,
		},
		"missing rollback confirmation": {
			runtimeSessionDevMySQLDSNEnv:      dsn,
			runtimeSessionDevMySQLDatabaseEnv: "coze_dev",
		},
		"wrong rollback confirmation": {
			runtimeSessionDevMySQLDSNEnv:          dsn,
			runtimeSessionDevMySQLDatabaseEnv:     "coze_dev",
			runtimeSessionDevMySQLRollbackOnlyEnv: "yes",
		},
		"multiple statements enabled": {
			runtimeSessionDevMySQLDSNEnv:          dsn + "&multiStatements=true",
			runtimeSessionDevMySQLDatabaseEnv:     "coze_dev",
			runtimeSessionDevMySQLRollbackOnlyEnv: runtimeSessionDevMySQLRollbackOnlyAck,
		},
	}
	for name, environment := range tests {
		t.Run(name, func(t *testing.T) {
			_, skipReason, err := loadRuntimeSessionDevMySQLGate(func(key string) string {
				return environment[key]
			})
			if skipReason != "" || err == nil {
				t.Fatalf("unsafe dev MySQL gate = skip %q, error %v; want a hard error", skipReason, err)
			}
		})
	}
}

func TestRuntimeSessionDevMySQLGateAcceptsRollbackOnlyConfiguration(t *testing.T) {
	environment := map[string]string{
		runtimeSessionDevMySQLDSNEnv:          "runtime_session_probe@tcp(mysql.dev.invalid:3306)/coze_dev?parseTime=true&loc=UTC",
		runtimeSessionDevMySQLDatabaseEnv:     "coze_dev",
		runtimeSessionDevMySQLRollbackOnlyEnv: runtimeSessionDevMySQLRollbackOnlyAck,
	}
	gate, skipReason, err := loadRuntimeSessionDevMySQLGate(func(key string) string {
		return environment[key]
	})
	if err != nil || skipReason != "" || gate == nil || gate.config == nil {
		t.Fatalf("valid dev MySQL gate = skip %q, error %v", skipReason, err)
	}
	if gate.databaseName != "coze_dev" || gate.config.DBName != gate.databaseName || !gate.config.ParseTime {
		t.Fatal("valid dev MySQL gate did not preserve the explicit database and parse-time contract")
	}
}

func TestRuntimeSessionDevMySQLRollbackOnlyIntegration(t *testing.T) {
	gate, skipReason, err := loadRuntimeSessionDevMySQLGate(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil {
		t.Fatal("unsafe runtime-session dev MySQL configuration")
	}
	db, err := gorm.Open(
		gormmysql.New(gormmysql.Config{DSNConfig: gate.config}),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatal("open explicitly gated runtime-session dev MySQL failed")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal("get explicitly gated runtime-session dev MySQL pool failed")
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Error("close runtime-session dev MySQL pool failed")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal("ping explicitly gated runtime-session dev MySQL failed")
	}
	var actualDatabase string
	if err := db.WithContext(ctx).Raw("SELECT DATABASE()").Row().Scan(&actualDatabase); err != nil {
		t.Fatal("read runtime-session dev MySQL database name failed")
	}
	if actualDatabase != gate.databaseName {
		t.Fatal("connected runtime-session database does not match the explicit database name")
	}
	verifyRuntimeSessionDevMySQLMigration(t, db)

	tx := db.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if tx.Error != nil {
		t.Fatal("begin runtime-session dev MySQL rollback-only transaction failed")
	}
	defer func() {
		if rollbackErr := tx.Rollback().Error; rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			t.Error("rollback runtime-session dev MySQL transaction failed")
		}
	}()

	reset := tx.Model(&schedulerSettingsPO{}).Where("id = ?", 1).
		UpdateColumns(map[string]any{
			"aio_runtime_generation":    uint64(0),
			"aio_runtime_deployment_id": "",
			"aio_runtime_sentinel_id":   "",
		})
	if reset.Error != nil || reset.RowsAffected != 1 {
		t.Fatal("prepare runtime-session generation fixture inside rollback-only transaction failed")
	}
	repository := NewMySQLRepository(tx)
	fixtureID := strings.ReplaceAll(uuid.NewString(), "-", "")
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("runtime-session-"+fixtureID))
	if err != nil {
		t.Fatal("create runtime-session provider fixture inside rollback-only transaction failed")
	}
	deploymentID := "runtime-session-dev-" + fixtureID
	firstSentinel := "newx-generation-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	firstGeneration, replaced, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID: deploymentID, CandidateSentinelID: firstSentinel,
	})
	if err != nil || !replaced || firstGeneration.Generation != 1 {
		t.Fatal("initialize runtime-session generation inside rollback-only transaction failed")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	created, err := repository.AcquireRuntimeSession(ctx, domainsandbox.AcquireRuntimeSessionInput{
		Key: domainsandbox.SessionKey{
			DeploymentID: deploymentID,
			ProviderID:   provider.ID,
			SpaceID:      42001,
			UserID:       43001,
			ThreadID:     "thread-" + fixtureID,
			Profile:      domainsandbox.SessionProfileCore,
		},
		CandidateSessionID: uuid.NewString(),
		RuntimeGeneration:  firstGeneration.Generation,
		ExpiresAt:          now.Add(20 * time.Minute),
		Now:                now,
	})
	if err != nil {
		t.Fatal("acquire runtime session inside rollback-only transaction failed")
	}
	bound, err := repository.BindRuntimeSessionCAS(ctx, domainsandbox.BindRuntimeSessionInput{
		Ref:             created.Ref,
		ExpectedVersion: created.Version,
		UpstreamShellID: "shell-" + fixtureID,
		ExpiresAt:       now.Add(20 * time.Minute),
		Now:             now.Add(time.Second),
	})
	if err != nil || bound.UpstreamShellID == "" {
		t.Fatal("bind runtime session inside rollback-only transaction failed")
	}

	secondSentinel := "newx-generation-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	secondGeneration, replaced, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID:        deploymentID,
		ExpectedSentinelID:  firstSentinel,
		CandidateSentinelID: secondSentinel,
	})
	if err != nil || !replaced || secondGeneration.Generation != 2 {
		t.Fatal("advance runtime-session generation inside rollback-only transaction failed")
	}
	recovering, err := repository.GetRuntimeSession(ctx, bound.Ref)
	if err != nil || recovering.State != domainsandbox.SessionStateRecovering || recovering.UpstreamShellID != "" {
		t.Fatal("generation CAS did not mark the old runtime session recovering")
	}
	recoverable, err := repository.ListRecoverableRuntimeSessions(ctx, domainsandbox.ListRecoverableRuntimeSessionsInput{
		DeploymentID:     deploymentID,
		BeforeGeneration: secondGeneration.Generation,
		Limit:            10,
	})
	if err != nil || len(recoverable) != 1 || recoverable[0].Ref.SessionID != recovering.Ref.SessionID {
		t.Fatal("list recoverable runtime sessions inside rollback-only transaction failed")
	}
	recovered, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref:                   recovering.Ref,
		ExpectedVersion:       recovering.Version,
		Action:                domainsandbox.SessionActionRecover,
		NextRuntimeGeneration: secondGeneration.Generation,
		UpstreamShellID:       "recovered-shell-" + fixtureID,
		ExpiresAt:             now.Add(20 * time.Minute),
		Now:                   now.Add(2 * time.Second),
	})
	if err != nil || recovered.State != domainsandbox.SessionStateActive ||
		recovered.Ref.RuntimeGeneration != secondGeneration.Generation || recovered.UpstreamShellID == "" {
		t.Fatal("recover runtime session inside rollback-only transaction failed")
	}
}

func loadRuntimeSessionDevMySQLGate(getenv func(string) string) (*runtimeSessionDevMySQLGate, string, error) {
	if getenv == nil {
		return nil, "", errors.New("runtime-session dev MySQL environment reader is missing")
	}
	dsn := strings.TrimSpace(getenv(runtimeSessionDevMySQLDSNEnv))
	if dsn == "" {
		return nil, runtimeSessionDevMySQLDSNEnv + " is not set", nil
	}
	databaseName := strings.TrimSpace(getenv(runtimeSessionDevMySQLDatabaseEnv))
	if databaseName == "" || databaseName != getenv(runtimeSessionDevMySQLDatabaseEnv) {
		return nil, "", errors.New("runtime-session dev MySQL database name is missing or malformed")
	}
	if getenv(runtimeSessionDevMySQLRollbackOnlyEnv) != runtimeSessionDevMySQLRollbackOnlyAck {
		return nil, "", errors.New("runtime-session dev MySQL rollback-only confirmation is missing")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return nil, "", errors.New("runtime-session dev MySQL DSN is invalid")
	}
	if config.DBName == "" || config.DBName != databaseName || config.MultiStatements {
		return nil, "", errors.New("runtime-session dev MySQL DSN violates the database safety gate")
	}
	config.ParseTime = true
	config.Loc = time.UTC
	return &runtimeSessionDevMySQLGate{config: config, databaseName: databaseName}, "", nil
}

func verifyRuntimeSessionDevMySQLMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []any{&schedulerSettingsPO{}, &runtimeSessionPO{}} {
		if !db.Migrator().HasTable(table) {
			t.Fatal("required runtime-session migration table is missing")
		}
	}
	for _, column := range []string{
		"session_settings_json",
		"session_settings_version",
		"session_settings_updated_by",
		"session_settings_updated_at",
		"aio_runtime_generation",
		"aio_runtime_deployment_id",
		"aio_runtime_sentinel_id",
	} {
		if !db.Migrator().HasColumn(&schedulerSettingsPO{}, column) {
			t.Fatal("required runtime-session scheduler migration column is missing")
		}
	}
	for _, column := range []string{
		"session_id",
		"deployment_id",
		"provider_id",
		"space_id",
		"user_id",
		"thread_id",
		"profile",
		"state",
		"runtime_generation",
		"upstream_shell_id",
		"recovery_reason",
		"version",
		"last_activity_at",
		"expires_at",
		"created_at",
		"updated_at",
	} {
		if !db.Migrator().HasColumn(&runtimeSessionPO{}, column) {
			t.Fatal("required runtime-session migration column is missing")
		}
	}
	for _, index := range []string{
		"uk_sandbox_runtime_session_business",
		"idx_sandbox_runtime_session_provider",
		"idx_sandbox_runtime_session_recovery",
		"idx_sandbox_runtime_session_expiry",
	} {
		if !db.Migrator().HasIndex(&runtimeSessionPO{}, index) {
			t.Fatal("required runtime-session migration index is missing")
		}
	}
	for _, constraint := range []string{
		"fk_sandbox_runtime_session_provider",
		"chk_sandbox_runtime_session_profile",
		"chk_sandbox_runtime_session_state",
		"chk_sandbox_runtime_session_generation",
		"chk_sandbox_runtime_session_version",
	} {
		if !db.Migrator().HasConstraint(&runtimeSessionPO{}, constraint) {
			t.Fatal("required runtime-session migration constraint is missing")
		}
	}
}
