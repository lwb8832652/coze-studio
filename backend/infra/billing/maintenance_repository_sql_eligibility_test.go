// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

// This SQLite test covers the SQL predicate boundaries only. MySQL locking,
// driver location decoding, and unique-key concurrency are integration tests.
func TestMaintenanceRepositorySQLiteSQLEligibilityBoundaries(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:maintenance-eligibility-%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE billing_accounts (
			id INTEGER PRIMARY KEY,
			subject_type TEXT NOT NULL,
			subject_id INTEGER NOT NULL
		)`,
		`CREATE TABLE billing_orders (
			id INTEGER PRIMARY KEY,
			version INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			payment_status TEXT NOT NULL,
			fulfillment_status TEXT NOT NULL,
			expires_at DATETIME
		)`,
		`CREATE TABLE user_subscriptions (
			id INTEGER PRIMARY KEY,
			version INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			source_order_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			current_period_end DATETIME NOT NULL
		)`,
	}
	for _, statement := range statements {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("create eligibility fixture: %v", err)
		}
	}

	now := time.Date(2026, time.July, 24, 4, 0, 0, 0, time.UTC)
	window := 8 * time.Hour
	if err = db.Exec(
		`INSERT INTO billing_accounts (id, subject_type, subject_id) VALUES (?, ?, ?)`,
		1, string(domainbilling.SubjectTypeUser), 101,
	).Error; err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if err = db.Exec(
		`INSERT INTO billing_orders
			(id, version, account_id, user_id, status, payment_status, fulfillment_status, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		11,
		1,
		1,
		101,
		string(domainbilling.OrderStatusPending),
		string(domainbilling.PaymentStatusPending),
		string(domainbilling.FulfillmentStatusPending),
		now,
	).Error; err != nil {
		t.Fatalf("insert order: %v", err)
	}
	subscriptions := []struct {
		id        int64
		periodEnd time.Time
	}{
		{id: 21, periodEnd: now},
		{id: 22, periodEnd: now.Add(window)},
		{id: 23, periodEnd: now.Add(window).Add(time.Millisecond)},
	}
	for _, subscription := range subscriptions {
		if err = db.Exec(
			`INSERT INTO user_subscriptions
				(id, version, account_id, source_order_id, status, current_period_end)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			subscription.id,
			1,
			1,
			11,
			string(domainbilling.SubscriptionStatusActive),
			subscription.periodEnd,
		).Error; err != nil {
			t.Fatalf("insert subscription %d: %v", subscription.id, err)
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		_, found, lockErr := lockSubscriptionMaintenanceRow(
			context.Background(),
			tx,
			21,
			"subscriptions.status = ? AND subscriptions.current_period_end <= ?",
			string(domainbilling.SubscriptionStatusActive),
			now,
		)
		if lockErr != nil {
			return lockErr
		}
		if !found {
			return fmt.Errorf("subscription exactly at due boundary was not eligible")
		}

		_, found, lockErr = lockSubscriptionMaintenanceRow(
			context.Background(),
			tx,
			22,
			"subscriptions.status = ? AND subscriptions.current_period_end > ? AND subscriptions.current_period_end <= ?",
			string(domainbilling.SubscriptionStatusActive),
			now,
			now.Add(window),
		)
		if lockErr != nil {
			return lockErr
		}
		if !found {
			return fmt.Errorf("subscription exactly at expiring upper boundary was not eligible")
		}

		_, found, lockErr = lockSubscriptionMaintenanceRow(
			context.Background(),
			tx,
			23,
			"subscriptions.status = ? AND subscriptions.current_period_end > ? AND subscriptions.current_period_end <= ?",
			string(domainbilling.SubscriptionStatusActive),
			now,
			now.Add(window),
		)
		if lockErr != nil {
			return lockErr
		}
		if found {
			return fmt.Errorf("subscription after expiring upper boundary was eligible")
		}

		_, found, lockErr = lockExpiredOrderRow(context.Background(), tx, 11, now)
		if lockErr != nil {
			return lockErr
		}
		if !found {
			return fmt.Errorf("order exactly at timeout boundary was not eligible")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("check SQL eligibility: %v", err)
	}
}
