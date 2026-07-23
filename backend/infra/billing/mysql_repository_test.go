// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

type sequenceIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *sequenceIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *sequenceIDGenerator) GenMultiIDs(ctx context.Context, count int) ([]int64, error) {
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

func TestLedgerGrantReserveSettleAndRelease(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1001}

	grant, err := service.Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 100, BusinessNo: "grant-1", SourceType: "admin",
	})
	if err != nil || grant.Balance.AvailableMicros != 100 {
		t.Fatalf("Grant() balance = %#v, err = %v", grant, err)
	}
	duplicateGrant, err := service.Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 100, BusinessNo: "grant-1", SourceType: "admin",
	})
	if err != nil || duplicateGrant.Balance.AvailableMicros != 100 {
		t.Fatalf("duplicate Grant() balance = %#v, err = %v", duplicateGrant, err)
	}

	reservation, err := service.Reserve(ctx, domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 70, BusinessNo: "reserve-1",
	})
	if err != nil || reservation.Balance.AvailableMicros != 30 || reservation.Balance.ReservedMicros != 70 {
		t.Fatalf("Reserve() result = %#v, err = %v", reservation, err)
	}
	settled, err := service.Settle(ctx, domainbilling.SettleInput{
		ReservationBusinessNo: "reserve-1", BusinessNo: "settle-1", ActualMicros: 40,
	})
	if err != nil || settled.Balance.AvailableMicros != 60 || settled.Balance.ReservedMicros != 0 {
		t.Fatalf("Settle() result = %#v, err = %v", settled, err)
	}
	duplicateSettlement, err := service.Settle(ctx, domainbilling.SettleInput{
		ReservationBusinessNo: "reserve-1", BusinessNo: "settle-1", ActualMicros: 40,
	})
	if err != nil || duplicateSettlement.Balance.AvailableMicros != 60 {
		t.Fatalf("duplicate Settle() result = %#v, err = %v", duplicateSettlement, err)
	}

	secondReservation, err := service.Reserve(ctx, domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 50, BusinessNo: "reserve-2",
	})
	if err != nil || secondReservation.Balance.AvailableMicros != 10 {
		t.Fatalf("second Reserve() result = %#v, err = %v", secondReservation, err)
	}
	released, err := service.Release(ctx, domainbilling.ReleaseInput{
		ReservationBusinessNo: "reserve-2", BusinessNo: "release-2",
	})
	if err != nil || released.Balance.AvailableMicros != 60 || released.Balance.ReservedMicros != 0 {
		t.Fatalf("Release() result = %#v, err = %v", released, err)
	}
}

func TestLedgerUsesEarliestExpiryAndRejectsInsufficientBalance(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1002}
	now := time.Now().UTC()
	later := now.Add(48 * time.Hour)
	earlier := now.Add(24 * time.Hour)

	_, err := service.Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 50, BusinessNo: "grant-later", SourceType: "package", ExpiresAt: &later,
	})
	if err != nil {
		t.Fatalf("Grant(later) error = %v", err)
	}
	_, err = service.Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 30, BusinessNo: "grant-earlier", SourceType: "package", ExpiresAt: &earlier,
	})
	if err != nil {
		t.Fatalf("Grant(earlier) error = %v", err)
	}
	_, err = service.Reserve(ctx, domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 40, BusinessNo: "reserve-expiry-order",
	})
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	var earlierBatch, laterBatch batchPO
	if err = db.Where("grant_business_no = ?", "grant-earlier").First(&earlierBatch).Error; err != nil {
		t.Fatalf("read earlier batch: %v", err)
	}
	if err = db.Where("grant_business_no = ?", "grant-later").First(&laterBatch).Error; err != nil {
		t.Fatalf("read later batch: %v", err)
	}
	if earlierBatch.RemainingMicros != 0 || laterBatch.RemainingMicros != 40 {
		t.Fatalf("remaining batches = earlier %d, later %d", earlierBatch.RemainingMicros, laterBatch.RemainingMicros)
	}

	_, err = service.Reserve(ctx, domainbilling.ReserveInput{
		Subject: subject, AmountMicros: 41, BusinessNo: "reserve-too-much",
	})
	if !errors.Is(err, domainbilling.ErrInsufficientCredits) {
		t.Fatalf("insufficient Reserve() error = %v", err)
	}
}

func TestLedgerRejectsConflictingIdempotencyKeys(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1003}
	_, err := service.Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 10, BusinessNo: "grant-conflict", SourceType: "admin",
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	_, err = service.Grant(ctx, domainbilling.GrantInput{
		Subject: subject, AmountMicros: 11, BusinessNo: "grant-conflict", SourceType: "admin",
	})
	if !errors.Is(err, domainbilling.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Grant() error = %v", err)
	}
}

func TestGetOrCreateAccountForUpdateIsIdempotent(t *testing.T) {
	_, db := newTestService(t)
	repository := NewMySQLRepository(db, &sequenceIDGenerator{next: 500})
	ctx := context.Background()
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2002}

	var firstID int64
	for index := 0; index < 8; index++ {
		err := repository.RunInTransaction(ctx, func(txRepository domainbilling.Repository) error {
			account, err := txRepository.GetOrCreateAccountForUpdate(ctx, subject, time.Now().UTC())
			if err != nil {
				return err
			}
			if firstID == 0 {
				firstID = account.ID
			}
			if account.ID != firstID {
				t.Fatalf("account ID changed from %d to %d", firstID, account.ID)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("GetOrCreateAccountForUpdate() error = %v", err)
		}
	}

	var count int64
	if err := db.Model(&accountPO{}).
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("account row count = %d, want 1", count)
	}
}

func TestCreateAuditLogIsIdempotent(t *testing.T) {
	_, db := newTestService(t)
	if err := db.AutoMigrate(&auditLogPO{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	repository := NewAdminRepository(db, &sequenceIDGenerator{next: 700})
	ctx := context.Background()
	for index := 0; index < 2; index++ {
		if err := repository.CreateAuditLog(ctx, 12, 34, "credit_adjustment", "adjust-1", `{"reason":"test"}`); err != nil {
			t.Fatalf("CreateAuditLog() error = %v", err)
		}
	}

	var count int64
	if err := db.Model(&auditLogPO{}).
		Where("business_no = ? AND action = ?", "adjust-1", "credit_adjustment").
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit log count = %d, want 1", count)
	}
}

func TestCommerceFulfillsZeroCreditSubscriptionAndReplacesActivePlan(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	repository := NewMySQLRepository(db, &sequenceIDGenerator{next: 1000})
	commerce := domainbilling.NewCommerceService(repository, ledger)
	ctx := context.Background()
	now := time.Now().UTC()
	plan := planPO{ID: 10, Key: "free", Name: "免费版", Status: string(domainbilling.CatalogStatusPublished), CurrentVersion: 1, CreatedAt: now, UpdatedAt: now}
	version := planVersionPO{ID: 11, PlanID: 10, Version: 1, Cycle: string(domainbilling.BillingCycleMonthly), Currency: "CNY", EffectiveAt: now.Add(-time.Minute), CreatedAt: now}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}

	fulfill := func(orderNo, digest string) *domainbilling.Order {
		order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
			UserID: 2001, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 2001},
			Type: domainbilling.OrderTypeSubscription, TargetID: plan.ID, OrderNo: orderNo,
		})
		if err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
		if _, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
			OrderNo: order.OrderNo, Gateway: "internal_free", ProviderTransactionID: "pay_" + orderNo,
			EventDigest: digest, AmountMicros: 0, Currency: "CNY",
		}); err != nil {
			t.Fatalf("RecordPaymentSucceeded() error = %v", err)
		}
		fulfilled, err := commerce.FulfillOrder(ctx, order.OrderNo)
		if err != nil {
			t.Fatalf("FulfillOrder() error = %v", err)
		}
		return fulfilled
	}

	first := fulfill("order-free-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	second := fulfill("order-free-2", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if first.FulfillmentStatus != domainbilling.FulfillmentStatusSucceeded || second.FulfillmentStatus != domainbilling.FulfillmentStatusSucceeded {
		t.Fatalf("unexpected fulfillment status: first=%s second=%s", first.FulfillmentStatus, second.FulfillmentStatus)
	}
	var active, cancelled int64
	if err := db.Model(&subscriptionPO{}).Where("status = ?", string(domainbilling.SubscriptionStatusActive)).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&subscriptionPO{}).Where("status = ?", string(domainbilling.SubscriptionStatusCancelled)).Count(&cancelled).Error; err != nil {
		t.Fatal(err)
	}
	if active != 1 || cancelled != 1 {
		t.Fatalf("subscription counts active=%d cancelled=%d", active, cancelled)
	}
}

func TestMaintenanceExpiresDueSubscription(t *testing.T) {
	_, db := newCommerceTestService(t)
	repository := NewMaintenanceRepository(db, &sequenceIDGenerator{next: 3000})
	now := time.Now().UTC()
	row := subscriptionPO{
		ID: 50, AccountID: 1, PlanID: 10, PlanVersionID: 11, SourceOrderID: 12,
		Status: string(domainbilling.SubscriptionStatusActive), CurrentPeriodStart: now.AddDate(0, -1, 0),
		CurrentPeriodEnd: now.Add(-time.Minute), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	items, err := repository.ListDueSubscriptions(context.Background(), now, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListDueSubscriptions() items=%#v err=%v", items, err)
	}
	if err = repository.ExpireSubscription(context.Background(), items[0]); err != nil {
		t.Fatalf("ExpireSubscription() error=%v", err)
	}
	var stored subscriptionPO
	if err = db.First(&stored, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != string(domainbilling.SubscriptionStatusExpired) || stored.AutoRenew {
		t.Fatalf("expired subscription=%#v", stored)
	}
}

func newTestService(t *testing.T) (*domainbilling.Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&accountPO{}, &batchPO{}, &ledgerPO{}, &reservationPO{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	repository := NewMySQLRepository(db, &sequenceIDGenerator{next: 100})
	return domainbilling.NewService(repository), db
}

func newCommerceTestService(t *testing.T) (*domainbilling.Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(
		&accountPO{}, &batchPO{}, &ledgerPO{}, &reservationPO{},
		&planPO{}, &planVersionPO{}, &creditPackagePO{}, &orderPO{}, &orderItemPO{},
		&paymentPO{}, &subscriptionPO{},
	); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	if err = db.Exec("ALTER TABLE subscription_plans ADD COLUMN deleted_at datetime").Error; err != nil {
		t.Fatalf("add subscription plan soft-delete column: %v", err)
	}
	repository := NewMySQLRepository(db, &sequenceIDGenerator{next: 100})
	return domainbilling.NewService(repository), db
}
