// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"database/sql"
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const (
	requiredMigrationVersion     = "20260814000100"
	requiredMigrationDescription = "sandbox_shared_aio_core"
	requiredMigrationQuery       = "SELECT description, applied, total, error FROM atlas_schema_revisions WHERE version = ?"
	requiredMigrationSchemaQuery = `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = ? AND (
  (table_name = 'sandbox_scheduler_settings' AND column_name IN (
    'session_settings_json', 'session_settings_version', 'session_settings_updated_by',
    'session_settings_updated_at', 'aio_runtime_generation', 'aio_runtime_deployment_id',
    'aio_runtime_sentinel_id'
  )) OR
  (table_name = 'sandbox_runtime_sessions' AND column_name IN (
    'session_id', 'deployment_id', 'provider_id', 'space_id', 'user_id', 'thread_id',
    'profile', 'state', 'runtime_generation', 'upstream_shell_id', 'recovery_reason',
    'version', 'last_activity_at', 'expires_at', 'created_at', 'updated_at'
  ))
)`
	requiredMigrationSchemaColumnCount = 23
	migrationPreflightTimeout          = 10 * time.Second
)

var ErrMigrationPreflight = errors.New("required database migration is unavailable")

type migrationDatabaseOpener func(*mysqldriver.Config) (*sql.DB, error)

// CheckRequiredMigration verifies the deployment prerequisite without
// modifying either the application schema or Atlas revision metadata.
func CheckRequiredMigration(ctx context.Context, getenv func(string) string) error {
	return checkRequiredMigration(ctx, getenv, openMigrationDatabase)
}

func checkRequiredMigration(ctx context.Context, getenv func(string) string, open migrationDatabaseOpener) (result error) {
	if ctx == nil || getenv == nil || open == nil {
		return ErrMigrationPreflight
	}
	config, err := mysqldriver.ParseDSN(getenv("MYSQL_DSN"))
	if err != nil || config.DBName == "" || config.MultiStatements {
		return ErrMigrationPreflight
	}
	db, err := open(config)
	if err != nil || db == nil {
		return ErrMigrationPreflight
	}
	defer func() {
		if err := db.Close(); result == nil && err != nil {
			result = ErrMigrationPreflight
		}
	}()

	boundedCtx, cancel := migrationPreflightContext(ctx)
	defer cancel()
	tx, err := db.BeginTx(boundedCtx, migrationPreflightTxOptions())
	if err != nil {
		return ErrMigrationPreflight
	}
	rollbackPending := true
	defer func() {
		if rollbackPending {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(boundedCtx, requiredMigrationQuery, requiredMigrationVersion)
	if err != nil {
		return ErrMigrationPreflight
	}
	defer rows.Close()
	if !rows.Next() {
		return ErrMigrationPreflight
	}
	var (
		description    string
		applied        int
		total          int
		migrationError sql.NullString
	)
	if err := rows.Scan(&description, &applied, &total, &migrationError); err != nil {
		return ErrMigrationPreflight
	}
	if rows.Next() || rows.Err() != nil {
		return ErrMigrationPreflight
	}
	if err := rows.Close(); err != nil {
		return ErrMigrationPreflight
	}
	if description != requiredMigrationDescription || total <= 0 || applied != total || migrationError.Valid && migrationError.String != "" {
		return ErrMigrationPreflight
	}
	var requiredColumns int
	if err := tx.QueryRowContext(boundedCtx, requiredMigrationSchemaQuery, config.DBName).Scan(&requiredColumns); err != nil || requiredColumns != requiredMigrationSchemaColumnCount {
		return ErrMigrationPreflight
	}
	if err := tx.Rollback(); err != nil {
		return ErrMigrationPreflight
	}
	rollbackPending = false
	return nil
}

func openMigrationDatabase(config *mysqldriver.Config) (*sql.DB, error) {
	connector, err := mysqldriver.NewConnector(config)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db, nil
}

func migrationPreflightContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, migrationPreflightTimeout)
}

func migrationPreflightTxOptions() *sql.TxOptions {
	return &sql.TxOptions{ReadOnly: true}
}
