// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestCommerceRepositoryLateSucceededCallbackIgnoresLocalExpiryWhilePending(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 1900}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 11_000_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 401, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 401},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-late-callback",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if err = db.Model(&orderPO{}).Where("id = ?", order.ID).Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	paid, err := commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_late_1", ProviderEventID: "evt_late_1",
		EventDigest: strings.Repeat("0", 64), AmountMicros: 11_000_000, Currency: "CNY",
	})
	if err != nil {
		t.Fatalf("RecordPaymentSucceeded() should accept authoritative late success callback: %v", err)
	}
	if paid.PaymentStatus != domainbilling.PaymentStatusSucceeded {
		t.Fatalf("payment status = %s", paid.PaymentStatus)
	}
	assertBillingOutboxCountForTest(t, db, domainnotification.EventBillingPaymentSucceeded, 1)
}

func TestCommerceRepositoryPaymentSucceededAppendsNotificationInSameTransaction(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 2000}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 12_000_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 501, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 501},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-notify-payment",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	paid, err := commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_notify_1", ProviderEventID: "evt_notify_1",
		EventDigest: strings.Repeat("a", 64), AmountMicros: 12_000_000, Currency: "cny",
	})
	if err != nil {
		t.Fatalf("RecordPaymentSucceeded() error = %v", err)
	}

	if paid.PaymentStatus != domainbilling.PaymentStatusSucceeded {
		t.Fatalf("payment status = %s", paid.PaymentStatus)
	}
	event := readBillingOutboxEventForTest(t, db, domainnotification.EventBillingPaymentSucceeded)
	if !strings.HasPrefix(event.EventID, "billing.payment_succeeded:") {
		t.Fatalf("event ID = %s", event.EventID)
	}
	if event.ActorID != order.UserID || event.SpaceID != 0 || event.AggregateVersion != paid.Version {
		t.Fatalf("unexpected event routing/version: %#v", event)
	}
}

func TestCommerceRepositoryPaymentNotificationAppendFailureRollsBackTerminalTransition(t *testing.T) {
	ledger, db := newCommerceTestServiceWithoutNotificationOutbox(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 3000}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 9_000_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 601, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 601},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-notify-rollback",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	_, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_rollback_1", ProviderEventID: "evt_rollback_1",
		EventDigest: strings.Repeat("b", 64), AmountMicros: 9_000_000, Currency: "CNY",
	})
	if err == nil {
		t.Fatal("RecordPaymentSucceeded() succeeded without notification outbox")
	}

	var stored orderPO
	if err = db.Where("order_no = ?", order.OrderNo).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PaymentStatus != string(domainbilling.PaymentStatusPending) || stored.Status != string(domainbilling.OrderStatusPending) {
		t.Fatalf("order terminal transition was not rolled back: %#v", stored)
	}
	var payments int64
	if err = db.Model(&paymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if payments != 0 {
		t.Fatalf("payment transaction count = %d, want 0", payments)
	}
}

func TestCommerceRepositoryDuplicateProviderPaymentCallbackDoesNotAppendDuplicateNotification(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 4000}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 13_000_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 701, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 701},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-notify-duplicate",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	for _, event := range []struct {
		id     string
		digest string
	}{
		{id: "evt_duplicate_1", digest: strings.Repeat("c", 64)},
		{id: "evt_duplicate_1", digest: strings.Repeat("c", 64)},
		{id: "evt_duplicate_delayed", digest: strings.Repeat("d", 64)},
	} {
		_, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
			OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_duplicate_1", ProviderEventID: event.id,
			EventDigest: event.digest, AmountMicros: 13_000_000, Currency: "CNY",
		})
		if err != nil {
			t.Fatalf("RecordPaymentSucceeded(%s) error = %v", event.id, err)
		}
	}

	var paymentEvents, payments int64
	if err = db.Model(&billingNotificationOutboxTestPO{}).
		Where("event_type = ?", string(domainnotification.EventBillingPaymentSucceeded)).
		Count(&paymentEvents).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&paymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if paymentEvents != 1 || payments != 1 {
		t.Fatalf("paymentEvents=%d payments=%d, want 1/1", paymentEvents, payments)
	}
}

func TestCommerceRepositoryConcurrentSameProviderTransactionIsIdempotent(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 4100}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 13_500_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 711, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 711},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-notify-concurrent",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, recordErr := commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
				OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_concurrent_1", ProviderEventID: "evt_concurrent_1",
				EventDigest: strings.Repeat("1", 64), AmountMicros: 13_500_000, Currency: "CNY",
			})
			results <- recordErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	for recordErr := range results {
		if recordErr != nil {
			t.Fatalf("concurrent RecordPaymentSucceeded() error = %v", recordErr)
		}
	}
	assertBillingOutboxCountForTest(t, db, domainnotification.EventBillingPaymentSucceeded, 1)
	var payments int64
	if err = db.Model(&paymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if payments != 1 {
		t.Fatalf("payment transaction count = %d, want 1", payments)
	}
}

func TestCommerceRepositorySameProviderTransactionAcrossOrdersIsConflict(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 4200}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 13_600_000)
	first, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 721, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 721},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-provider-conflict-1",
	})
	if err != nil {
		t.Fatalf("CreateOrder(first) error = %v", err)
	}
	second, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 722, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 722},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-provider-conflict-2",
	})
	if err != nil {
		t.Fatalf("CreateOrder(second) error = %v", err)
	}
	if _, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: first.OrderNo, Gateway: "test", ProviderTransactionID: "pay_cross_order", ProviderEventID: "evt_cross_order_1",
		EventDigest: strings.Repeat("2", 64), AmountMicros: 13_600_000, Currency: "CNY",
	}); err != nil {
		t.Fatalf("RecordPaymentSucceeded(first) error = %v", err)
	}
	_, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: second.OrderNo, Gateway: "test", ProviderTransactionID: "pay_cross_order", ProviderEventID: "evt_cross_order_2",
		EventDigest: strings.Repeat("3", 64), AmountMicros: 13_600_000, Currency: "CNY",
	})
	if !errors.Is(err, domainbilling.ErrIdempotencyConflict) {
		t.Fatalf("cross-order provider transaction error = %v, want idempotency conflict", err)
	}
	assertBillingOutboxCountForTest(t, db, domainnotification.EventBillingPaymentSucceeded, 1)
}

func TestCommerceRepositoryFulfillmentNotificationUsesPersistedWorkspaceSubject(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 5000}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlan(t, db, 14_000_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 801, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9001},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-notify-workspace",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if _, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_workspace_1", ProviderEventID: "evt_workspace_1",
		EventDigest: strings.Repeat("e", 64), AmountMicros: 14_000_000, Currency: "CNY",
	}); err != nil {
		t.Fatalf("RecordPaymentSucceeded() error = %v", err)
	}
	fulfilled, err := commerce.FulfillOrder(ctx, order.OrderNo)
	if err != nil {
		t.Fatalf("FulfillOrder() error = %v", err)
	}

	for _, eventType := range []domainnotification.EventType{
		domainnotification.EventBillingPaymentSucceeded,
		domainnotification.EventBillingSubscriptionActivated,
	} {
		event := readBillingOutboxEventForTest(t, db, eventType)
		if event.ActorID != order.UserID || event.SpaceID != 9001 {
			t.Fatalf("%s event did not use persisted subject/user facts: %#v", eventType, event)
		}
		if eventType == domainnotification.EventBillingSubscriptionActivated && event.AggregateVersion != fulfilled.Version {
			t.Fatalf("subscription event aggregate version = %d, want %d", event.AggregateVersion, fulfilled.Version)
		}
	}
}

func TestCommerceRepositoryWorkspaceFulfillmentGrantsCreditsToPersistedSubject(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 5500}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlanWithCredit(t, db, 14_500_000, 700_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 851, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9701},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-workspace-credit-grant",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if _, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_workspace_credit", ProviderEventID: "evt_workspace_credit",
		EventDigest: strings.Repeat("4", 64), AmountMicros: 14_500_000, Currency: "CNY",
	}); err != nil {
		t.Fatalf("RecordPaymentSucceeded() error = %v", err)
	}
	if _, err = commerce.FulfillOrder(ctx, order.OrderNo); err != nil {
		t.Fatalf("FulfillOrder() error = %v", err)
	}

	var workspaceAccount accountPO
	if err = db.Where("subject_type = ? AND subject_id = ?", string(domainbilling.SubjectTypeWorkspace), int64(9701)).First(&workspaceAccount).Error; err != nil {
		t.Fatal(err)
	}
	if workspaceAccount.AvailableMicros != 700_000 {
		t.Fatalf("workspace credits = %d, want 700000", workspaceAccount.AvailableMicros)
	}
	var userAccounts int64
	if err = db.Model(&accountPO{}).Where("subject_type = ? AND subject_id = ?", string(domainbilling.SubjectTypeUser), order.UserID).Count(&userAccounts).Error; err != nil {
		t.Fatal(err)
	}
	if userAccounts != 0 {
		t.Fatalf("user account count = %d, want 0", userAccounts)
	}
}

func TestCommerceRepositoryFulfillmentNotificationAppendFailureRollsBackFulfillmentAndLedgerGrant(t *testing.T) {
	ledger, db := newCommerceTestService(t)
	commerce := domainbilling.NewCommerceService(NewMySQLRepository(db, &sequenceIDGenerator{next: 6000}), ledger)
	ctx := context.Background()
	planID := seedCommerceNotificationPlanWithCredit(t, db, 15_000_000, 250_000)
	order, err := commerce.CreateOrder(ctx, domainbilling.CreateOrderInput{
		UserID: 901, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 901},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "order-notify-fulfillment-rollback",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if _, err = commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "test", ProviderTransactionID: "pay_fulfillment_rollback", ProviderEventID: "evt_fulfillment_rollback",
		EventDigest: strings.Repeat("f", 64), AmountMicros: 15_000_000, Currency: "CNY",
	}); err != nil {
		t.Fatalf("RecordPaymentSucceeded() error = %v", err)
	}
	if err = db.Migrator().DropTable(&billingNotificationOutboxTestPO{}); err != nil {
		t.Fatalf("drop notification outbox: %v", err)
	}

	_, err = commerce.FulfillOrder(ctx, order.OrderNo)
	if err == nil {
		t.Fatal("FulfillOrder() succeeded without notification outbox")
	}

	var stored orderPO
	if err = db.Where("order_no = ?", order.OrderNo).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != string(domainbilling.OrderStatusPaid) || stored.FulfillmentStatus != string(domainbilling.FulfillmentStatusPending) {
		t.Fatalf("fulfillment terminal transition was not rolled back: %#v", stored)
	}
	var subscriptions, grants int64
	if err = db.Model(&subscriptionPO{}).Where("source_order_id = ?", order.ID).Count(&subscriptions).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&ledgerPO{}).Where("business_no = ?", "order-grant:"+order.OrderNo).Count(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if subscriptions != 0 || grants != 0 {
		t.Fatalf("subscriptions=%d grants=%d, want 0/0", subscriptions, grants)
	}
}

func seedCommerceNotificationPlan(t *testing.T, db *gorm.DB, priceMicros int64) int64 {
	t.Helper()
	return seedCommerceNotificationPlanWithCredit(t, db, priceMicros, 0)
}

func seedCommerceNotificationPlanWithCredit(t *testing.T, db *gorm.DB, priceMicros int64, creditMicros int64) int64 {
	t.Helper()
	now := time.Now().UTC()
	plan := planPO{ID: priceMicros/1_000_000 + 100, Key: "plan-notify", Name: "Notify Plan", Status: string(domainbilling.CatalogStatusPublished), CurrentVersion: 1, CreatedAt: now, UpdatedAt: now}
	version := planVersionPO{ID: plan.ID + 1000, PlanID: plan.ID, Version: 1, Cycle: string(domainbilling.BillingCycleMonthly), PriceMicros: priceMicros, Currency: "CNY", CreditGrantMicros: creditMicros, EffectiveAt: now.Add(-time.Minute), CreatedAt: now}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	return plan.ID
}

func newCommerceTestServiceWithoutNotificationOutbox(t *testing.T) (*domainbilling.Service, *gorm.DB) {
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

func migrateBillingNotificationOutboxForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&billingNotificationOutboxTestPO{}); err != nil {
		t.Fatalf("migrate notification outbox: %v", err)
	}
}

func readBillingOutboxEventForTest(t *testing.T, db *gorm.DB, eventType domainnotification.EventType) billingNotificationOutboxTestPO {
	t.Helper()
	var event billingNotificationOutboxTestPO
	if err := db.Where("event_type = ?", string(eventType)).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	return event
}

func assertBillingOutboxCountForTest(t *testing.T, db *gorm.DB, eventType domainnotification.EventType, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&billingNotificationOutboxTestPO{}).Where("event_type = ?", string(eventType)).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s outbox count = %d, want %d", eventType, count, want)
	}
}

type billingNotificationOutboxTestPO struct {
	ID               int64  `gorm:"column:id;primaryKey"`
	EventID          string `gorm:"column:event_id;size:128;uniqueIndex:uk_notification_outbox_event"`
	IdempotencyKey   string `gorm:"column:idempotency_key;size:64;uniqueIndex:uk_notification_outbox_idempotency"`
	EventType        string `gorm:"column:event_type;size:64"`
	AggregateType    string `gorm:"column:aggregate_type;size:64"`
	AggregateID      string `gorm:"column:aggregate_id;size:128"`
	AggregateVersion int64  `gorm:"column:aggregate_version"`
	OccurredAt       int64  `gorm:"column:occurred_at"`
	SpaceID          int64  `gorm:"column:space_id"`
	ActorID          int64  `gorm:"column:actor_id"`
	RecipientPolicy  string `gorm:"column:recipient_policy;size:64"`
	PayloadSchema    int32  `gorm:"column:payload_schema"`
	PayloadJSON      []byte `gorm:"column:payload_json;type:json"`
	Status           string `gorm:"column:status;size:16;index:idx_notification_outbox_ready"`
	AttemptCount     int    `gorm:"column:attempt_count"`
	AvailableAt      int64  `gorm:"column:available_at;index:idx_notification_outbox_ready"`
	LockedAt         int64  `gorm:"column:locked_at;index:idx_notification_outbox_lease"`
	LockedBy         string `gorm:"column:locked_by;size:128"`
	LastErrorCode    string `gorm:"column:last_error_code;size:64"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
	DeliveredAt      int64  `gorm:"column:delivered_at"`
}

func (billingNotificationOutboxTestPO) TableName() string {
	return "notification_outbox"
}
