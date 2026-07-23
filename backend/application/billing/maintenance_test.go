// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
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

func TestAdjustCreditsRollsBackWhenAuditWriteFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE billing_accounts (id INTEGER PRIMARY KEY, subject_type TEXT NOT NULL, subject_id INTEGER NOT NULL, available_micros INTEGER NOT NULL DEFAULT 0, reserved_micros INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(subject_type, subject_id))`,
		`CREATE TABLE credit_batches (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL, source_type TEXT NOT NULL, source_id TEXT NOT NULL, grant_business_no TEXT NOT NULL UNIQUE, granted_micros INTEGER NOT NULL, remaining_micros INTEGER NOT NULL, expires_at DATETIME, version INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE credit_ledger_entries (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL, batch_id INTEGER, direction TEXT NOT NULL, entry_type TEXT NOT NULL, amount_micros INTEGER NOT NULL, available_after_micros INTEGER NOT NULL, reserved_after_micros INTEGER NOT NULL, business_no TEXT NOT NULL UNIQUE, actor_user_id INTEGER NOT NULL DEFAULT 0, metadata_json TEXT NOT NULL, created_at DATETIME NOT NULL)`,
	} {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(db, &billingTestIDGenerator{next: 100})
	_, err = service.AdjustCredits(context.Background(), CreditAdjustmentInput{
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
