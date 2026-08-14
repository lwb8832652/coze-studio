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
	requiredMigrationVersion  = "20260813000100"
	requiredMigrationQuery    = "SELECT applied, total, error FROM atlas_schema_revisions WHERE version = ?"
	migrationPreflightTimeout = 10 * time.Second
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
		applied        int
		total          int
		migrationError sql.NullString
	)
	if err := rows.Scan(&applied, &total, &migrationError); err != nil {
		return ErrMigrationPreflight
	}
	if rows.Next() || rows.Err() != nil {
		return ErrMigrationPreflight
	}
	if err := rows.Close(); err != nil {
		return ErrMigrationPreflight
	}
	if total <= 0 || applied != total || migrationError.Valid && migrationError.String != "" {
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
