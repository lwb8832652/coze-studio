// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const billingMySQLTimezoneTestDDLConfirmation = "YES_I_OWN_THIS_DISPOSABLE_SCHEMA"

var billingMySQLTimezoneTestDatabasePattern = regexp.MustCompile(`^coze_billing_it_[a-z0-9_]+$`)

// This optional integration test proves that eligibility is decided by MySQL
// DATETIME predicates even when scanned values decode in Asia/Shanghai.
func TestMaintenanceRepositoryMySQLNonUTCDSNEligibilityBoundaries(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("COZE_BILLING_MYSQL_TEST_DSN"))
	if dsn == "" {
		t.Skip("COZE_BILLING_MYSQL_TEST_DSN is not configured")
	}
	if os.Getenv("COZE_BILLING_MYSQL_TEST_ALLOW_DDL") != billingMySQLTimezoneTestDDLConfirmation {
		t.Skip("disposable MySQL DDL confirmation is not configured")
	}

	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MySQL integration DSN: %v", err)
	}
	if !billingMySQLTimezoneTestDatabasePattern.MatchString(config.DBName) {
		t.Fatalf("MySQL integration database must match %s", billingMySQLTimezoneTestDatabasePattern)
	}
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load Asia/Shanghai: %v", err)
	}
	config.ParseTime = true
	config.Loc = shanghai

	database, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL integration database: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connection, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire MySQL integration connection: %v", err)
	}
	defer connection.Close()

	_, err = connection.ExecContext(ctx, `
		CREATE TEMPORARY TABLE billing_maintenance_timezone_boundaries (
			id BIGINT NOT NULL PRIMARY KEY,
			kind VARCHAR(24) NOT NULL,
			status VARCHAR(24) NOT NULL,
			payment_status VARCHAR(24) NOT NULL,
			fulfillment_status VARCHAR(24) NOT NULL,
			boundary DATETIME(3) NOT NULL
		)`)
	if err != nil {
		t.Fatalf("create temporary boundary table: %v", err)
	}
	defer connection.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS billing_maintenance_timezone_boundaries")

	now := time.Date(2026, time.July, 24, 4, 0, 0, 0, time.UTC)
	window := 8 * time.Hour
	rows := []struct {
		id       int64
		kind     string
		boundary time.Time
	}{
		{id: 1, kind: "subscription", boundary: now},
		{id: 2, kind: "subscription", boundary: now.Add(window)},
		{id: 3, kind: "subscription", boundary: now.Add(window).Add(time.Millisecond)},
		{id: 4, kind: "order", boundary: now},
	}
	for _, row := range rows {
		_, err = connection.ExecContext(
			ctx,
			`INSERT INTO billing_maintenance_timezone_boundaries
				(id, kind, status, payment_status, fulfillment_status, boundary)
			 VALUES (?, ?, 'active', 'pending', 'pending', ?)`,
			row.id,
			row.kind,
			row.boundary,
		)
		if err != nil {
			t.Fatalf("insert boundary row: %v", err)
		}
	}
	if _, err = connection.ExecContext(
		ctx,
		"UPDATE billing_maintenance_timezone_boundaries SET status = 'pending' WHERE id = ?",
		4,
	); err != nil {
		t.Fatalf("prepare order boundary row: %v", err)
	}

	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin boundary transaction: %v", err)
	}
	defer transaction.Rollback()

	assertLockedBoundary := func(query string, args ...any) time.Time {
		t.Helper()
		var decoded time.Time
		if scanErr := transaction.QueryRowContext(ctx, query, args...).Scan(&decoded); scanErr != nil {
			t.Fatalf("lock eligible boundary: %v", scanErr)
		}
		if decoded.Location().String() != "Asia/Shanghai" {
			t.Fatalf("decoded location = %s, want Asia/Shanghai", decoded.Location())
		}
		return decoded
	}

	assertLockedBoundary(
		`SELECT boundary FROM billing_maintenance_timezone_boundaries
		 WHERE id = ? AND status = 'active' AND boundary <= ? FOR UPDATE`,
		1,
		now,
	)
	assertLockedBoundary(
		`SELECT boundary FROM billing_maintenance_timezone_boundaries
		 WHERE id = ? AND status = 'active' AND boundary > ? AND boundary <= ? FOR UPDATE`,
		2,
		now,
		now.Add(window),
	)
	var outside time.Time
	err = transaction.QueryRowContext(
		ctx,
		`SELECT boundary FROM billing_maintenance_timezone_boundaries
		 WHERE id = ? AND status = 'active' AND boundary > ? AND boundary <= ? FOR UPDATE`,
		3,
		now,
		now.Add(window),
	).Scan(&outside)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("row after expiring upper boundary error = %v, want sql.ErrNoRows", err)
	}
	assertLockedBoundary(
		`SELECT boundary FROM billing_maintenance_timezone_boundaries
		 WHERE id = ? AND kind = 'order' AND status = 'pending' AND payment_status = 'pending'
		   AND fulfillment_status = 'pending' AND boundary <= ? FOR UPDATE`,
		4,
		now,
	)
}
