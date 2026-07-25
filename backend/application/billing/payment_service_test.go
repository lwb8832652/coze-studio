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

func TestHandlePaymentCallbackIsIdempotentForDuplicateAndDelayedSucceededEvents(t *testing.T) {
	service, db := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	planID := seedPaymentServicePlan(t, db, 21_000_000)
	order := createPaymentServiceOrderForTest(t, service, ctx, domainbilling.CreateOrderInput{
		UserID: 1101, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1101},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "payment-callback-duplicate",
	})
	delayed := succeededPaymentEvent(order.OrderNo, "pay_callback_duplicate", "evt_callback_delayed", strings.Repeat("b", 64), 21_000_000)
	service.ConfigurePaymentGateway(&recordingPaymentGateway{events: []*VerifiedPaymentEvent{
		succeededPaymentEvent(order.OrderNo, "pay_callback_duplicate", "evt_callback_duplicate", strings.Repeat("a", 64), 21_000_000),
		succeededPaymentEvent(order.OrderNo, "pay_callback_duplicate", "evt_callback_duplicate", strings.Repeat("a", 64), 21_000_000),
		delayed,
	}})

	for index := 0; index < 3; index++ {
		if _, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback")); err != nil {
			t.Fatalf("HandlePaymentCallback(%d) error = %v", index, err)
		}
	}

	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingPaymentSucceeded, 1)
	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingSubscriptionActivated, 1)
	var payments int64
	if err := db.Model(&paymentServicePaymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if payments != 1 {
		t.Fatalf("payment transaction count = %d, want 1", payments)
	}
	var stored paymentServiceOrderPO
	if err := db.Where("order_no = ?", order.OrderNo).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PaymentStatus != string(domainbilling.PaymentStatusSucceeded) || stored.FulfillmentStatus != string(domainbilling.FulfillmentStatusSucceeded) {
		t.Fatalf("order was not fulfilled exactly once: %#v", stored)
	}
}

func TestHandlePaymentCallbackAcceptsFirstSucceededCallbackAfterLocalOrderExpiry(t *testing.T) {
	service, db := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	planID := seedPaymentServicePlan(t, db, 21_500_000)
	order := createPaymentServiceOrderForTest(t, service, ctx, domainbilling.CreateOrderInput{
		UserID: 1151, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1151},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "payment-callback-expired-pending",
	})
	if err := db.Model(&paymentServiceOrderPO{}).Where("id = ?", order.ID).Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	service.ConfigurePaymentGateway(&recordingPaymentGateway{events: []*VerifiedPaymentEvent{
		succeededPaymentEvent(order.OrderNo, "pay_callback_expired", "evt_callback_expired", strings.Repeat("9", 64), 21_500_000),
	}})

	if _, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback")); err != nil {
		t.Fatalf("HandlePaymentCallback() should accept authoritative late success: %v", err)
	}
	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingPaymentSucceeded, 1)
	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingSubscriptionActivated, 1)
}

func TestHandlePaymentCallbackRollsBackPaymentWhenNotificationAppendFails(t *testing.T) {
	service, db := newPaymentServiceTestService(t, false)
	ctx := context.Background()
	planID := seedPaymentServicePlan(t, db, 22_000_000)
	order := createPaymentServiceOrderForTest(t, service, ctx, domainbilling.CreateOrderInput{
		UserID: 1201, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1201},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "payment-callback-rollback",
	})
	service.ConfigurePaymentGateway(&recordingPaymentGateway{events: []*VerifiedPaymentEvent{
		succeededPaymentEvent(order.OrderNo, "pay_callback_rollback", "evt_callback_rollback", strings.Repeat("c", 64), 22_000_000),
	}})

	if _, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback")); err == nil {
		t.Fatal("HandlePaymentCallback() succeeded without notification outbox")
	}

	var stored paymentServiceOrderPO
	if err := db.Where("order_no = ?", order.OrderNo).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != string(domainbilling.OrderStatusPending) || stored.PaymentStatus != string(domainbilling.PaymentStatusPending) {
		t.Fatalf("payment terminal transition was not rolled back: %#v", stored)
	}
	var payments int64
	if err := db.Model(&paymentServicePaymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if payments != 0 {
		t.Fatalf("payment transaction count = %d, want 0", payments)
	}
}

func TestHandlePaymentCallbackRejectsNonSucceededVerifiedEventWithoutNotification(t *testing.T) {
	service, db := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	planID := seedPaymentServicePlan(t, db, 23_000_000)
	order := createPaymentServiceOrderForTest(t, service, ctx, domainbilling.CreateOrderInput{
		UserID: 1301, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1301},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "payment-callback-failed",
	})
	failedEvent := succeededPaymentEvent(order.OrderNo, "pay_callback_failed", "evt_callback_failed", strings.Repeat("d", 64), 23_000_000)
	failedEvent.Status = string(domainbilling.PaymentStatusFailed)
	service.ConfigurePaymentGateway(&recordingPaymentGateway{events: []*VerifiedPaymentEvent{failedEvent}})

	if _, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback")); err == nil {
		t.Fatal("HandlePaymentCallback() accepted a non-succeeded event")
	} else if !errors.Is(err, ErrPaymentCallbackInvalid) {
		t.Fatalf("HandlePaymentCallback() error = %v, want invalid callback", err)
	}

	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingPaymentSucceeded, 0)
	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingSubscriptionActivated, 0)
	var payments int64
	if err := db.Model(&paymentServicePaymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if payments != 0 {
		t.Fatalf("payment transaction count = %d, want 0", payments)
	}
}

func TestHandlePaymentCallbackPreservesVerifierRetryableErrors(t *testing.T) {
	service, _ := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "gateway unavailable", err: ErrPaymentGatewayUnavailable},
		{name: "transient verifier internals", err: errors.New("kms signer timeout")},
	} {
		t.Run(test.name, func(t *testing.T) {
			service.ConfigurePaymentGateway(&recordingPaymentGateway{err: test.err})
			_, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback"))
			if !errors.Is(err, test.err) {
				t.Fatalf("HandlePaymentCallback() error = %v, want verifier error %v", err, test.err)
			}
			if errors.Is(err, ErrPaymentCallbackInvalid) {
				t.Fatalf("HandlePaymentCallback() rewrote retryable verifier error to invalid callback: %v", err)
			}
		})
	}
}

func TestHandlePaymentCallbackMarksVerifierInvalidCallbackErrors(t *testing.T) {
	service, _ := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	service.ConfigurePaymentGateway(&recordingPaymentGateway{err: ErrPaymentCallbackInvalid})
	_, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback"))
	if !errors.Is(err, ErrPaymentCallbackInvalid) {
		t.Fatalf("HandlePaymentCallback() error = %v, want invalid callback", err)
	}
}

func TestHandlePaymentCallbackUsesPersistedWorkspaceSubjectForNotificationRouting(t *testing.T) {
	service, db := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	planID := seedPaymentServicePlan(t, db, 24_000_000)
	order := createPaymentServiceOrderForTest(t, service, ctx, domainbilling.CreateOrderInput{
		UserID: 1401, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9901},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "payment-callback-workspace",
	})
	service.ConfigurePaymentGateway(&recordingPaymentGateway{events: []*VerifiedPaymentEvent{
		succeededPaymentEvent(order.OrderNo, "pay_callback_workspace", "evt_callback_workspace", strings.Repeat("e", 64), 24_000_000),
	}})

	if _, err := service.HandlePaymentCallback(ctx, "test", nil, []byte("callback")); err != nil {
		t.Fatalf("HandlePaymentCallback() error = %v", err)
	}

	for _, eventType := range []domainnotification.EventType{
		domainnotification.EventBillingPaymentSucceeded,
		domainnotification.EventBillingSubscriptionActivated,
	} {
		event := readPaymentServiceOutboxEventForTest(t, db, eventType)
		if event.ActorID != order.UserID || event.SpaceID != 9901 {
			t.Fatalf("%s event did not use persisted order subject/user facts: %#v", eventType, event)
		}
	}
}

func TestCreateCheckoutRetriesPaidPendingZeroOrderAndDoesNotDuplicatePaymentNotification(t *testing.T) {
	service, db := newPaymentServiceTestService(t, true)
	ctx := context.Background()
	planID := seedPaymentServicePlan(t, db, 0)
	order := createPaymentServiceOrderForTest(t, service, ctx, domainbilling.CreateOrderInput{
		UserID: 1501, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1501},
		Type: domainbilling.OrderTypeSubscription, TargetID: planID, OrderNo: "zero-checkout-paid-pending-retry",
	})
	if _, err := service.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: order.OrderNo, Gateway: "internal_free", ProviderTransactionID: "free:" + order.OrderNo,
		ProviderEventID: "free:" + order.OrderNo, EventDigest: strings.Repeat("8", 64), AmountMicros: 0, Currency: "CNY",
	}); err != nil {
		t.Fatalf("RecordPaymentSucceeded() error = %v", err)
	}

	response, err := service.CreateCheckout(ctx, CreateCheckoutInput{UserID: order.UserID, OrderNo: order.OrderNo})
	if err != nil {
		t.Fatalf("CreateCheckout() retry error = %v", err)
	}
	if response.Gateway != "internal_free" || response.ProviderTransactionID != "free:"+order.OrderNo {
		t.Fatalf("CreateCheckout() response = %#v", response)
	}
	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingPaymentSucceeded, 1)
	assertPaymentServiceOutboxCount(t, db, domainnotification.EventBillingSubscriptionActivated, 1)
	var payments int64
	if err = db.Model(&paymentServicePaymentPO{}).Where("order_id = ?", order.ID).Count(&payments).Error; err != nil {
		t.Fatal(err)
	}
	if payments != 1 {
		t.Fatalf("payment transaction count = %d, want 1", payments)
	}
}

func newPaymentServiceTestService(t *testing.T, withNotificationOutbox bool) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	tables := []any{
		&paymentServiceAccountPO{}, &paymentServiceCreditBatchPO{},
		&paymentServicePlanPO{}, &paymentServicePlanVersionPO{},
		&paymentServiceOrderPO{}, &paymentServiceOrderItemPO{}, &paymentServicePaymentPO{},
		&paymentServiceSubscriptionPO{}, &paymentServiceSystemConfigPO{},
	}
	if withNotificationOutbox {
		tables = append(tables, &paymentServiceNotificationOutboxPO{})
	}
	if err = db.AutoMigrate(tables...); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	if err = db.Create(&paymentServiceSystemConfigPO{
		ID: 1, CreditName: "Credits", DisplayScale: 2, PaymentEnabled: true,
		DefaultCurrency: "CNY", Version: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return NewService(db, &paymentServiceTestIDGenerator{next: 1000}), db
}

func seedPaymentServicePlan(t *testing.T, db *gorm.DB, priceMicros int64) int64 {
	t.Helper()
	now := time.Now().UTC()
	plan := paymentServicePlanPO{
		ID: 100 + priceMicros/1_000_000, Key: "payment-plan", Name: "Payment Plan",
		Status: string(domainbilling.CatalogStatusPublished), CurrentVersion: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	version := paymentServicePlanVersionPO{
		ID: plan.ID + 1000, PlanID: plan.ID, Version: 1,
		Cycle: string(domainbilling.BillingCycleMonthly), PriceMicros: priceMicros,
		Currency: "CNY", EffectiveAt: now.Add(-time.Minute), CreatedAt: now,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	return plan.ID
}

func createPaymentServiceOrderForTest(t *testing.T, service *Service, ctx context.Context, input domainbilling.CreateOrderInput) *domainbilling.Order {
	t.Helper()
	order, err := service.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	return order
}

func succeededPaymentEvent(orderNo string, providerTransactionID string, providerEventID string, digest string, amountMicros int64) *VerifiedPaymentEvent {
	return &VerifiedPaymentEvent{
		OrderNo: orderNo, Gateway: "test", ProviderTransactionID: providerTransactionID,
		ProviderEventID: providerEventID, EventDigest: digest, Currency: "CNY",
		AmountMicros: amountMicros, Status: string(domainbilling.PaymentStatusSucceeded),
	}
}

func assertPaymentServiceOutboxCount(t *testing.T, db *gorm.DB, eventType domainnotification.EventType, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&paymentServiceNotificationOutboxPO{}).
		Where("event_type = ?", string(eventType)).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s outbox count = %d, want %d", eventType, count, want)
	}
}

func readPaymentServiceOutboxEventForTest(t *testing.T, db *gorm.DB, eventType domainnotification.EventType) paymentServiceNotificationOutboxPO {
	t.Helper()
	var event paymentServiceNotificationOutboxPO
	if err := db.Where("event_type = ?", string(eventType)).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	return event
}

type recordingPaymentGateway struct {
	mu     sync.Mutex
	events []*VerifiedPaymentEvent
	err    error
	calls  int
}

func (g *recordingPaymentGateway) CreatePayment(context.Context, CreatePaymentRequest) (*CreatePaymentResponse, error) {
	return nil, errors.New("unexpected CreatePayment")
}

func (g *recordingPaymentGateway) VerifyCallback(context.Context, map[string]string, []byte) (*VerifiedPaymentEvent, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return nil, g.err
	}
	if len(g.events) == 0 {
		return nil, errors.New("missing payment event")
	}
	index := g.calls
	if index >= len(g.events) {
		index = len(g.events) - 1
	}
	g.calls++
	event := *g.events[index]
	return &event, nil
}

func (g *recordingPaymentGateway) QueryPayment(context.Context, string) (*VerifiedPaymentEvent, error) {
	return nil, errors.New("unexpected QueryPayment")
}

type paymentServiceTestIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *paymentServiceTestIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *paymentServiceTestIDGenerator) GenMultiIDs(ctx context.Context, count int) ([]int64, error) {
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

type paymentServiceAccountPO struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	SubjectType     string    `gorm:"column:subject_type;uniqueIndex:uk_billing_account_subject,priority:1"`
	SubjectID       int64     `gorm:"column:subject_id;uniqueIndex:uk_billing_account_subject,priority:2"`
	AvailableMicros int64     `gorm:"column:available_micros"`
	ReservedMicros  int64     `gorm:"column:reserved_micros"`
	Version         int64     `gorm:"column:version"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (paymentServiceAccountPO) TableName() string { return "billing_accounts" }

type paymentServiceCreditBatchPO struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	AccountID       int64      `gorm:"column:account_id"`
	SourceType      string     `gorm:"column:source_type"`
	SourceID        string     `gorm:"column:source_id"`
	GrantBusinessNo string     `gorm:"column:grant_business_no;uniqueIndex:uk_credit_batch_grant_business_no"`
	GrantedMicros   int64      `gorm:"column:granted_micros"`
	RemainingMicros int64      `gorm:"column:remaining_micros"`
	ExpiresAt       *time.Time `gorm:"column:expires_at"`
	Version         int64      `gorm:"column:version"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (paymentServiceCreditBatchPO) TableName() string { return "credit_batches" }

type paymentServicePlanPO struct {
	ID             int64      `gorm:"column:id;primaryKey"`
	Key            string     `gorm:"column:plan_key"`
	Name           string     `gorm:"column:name"`
	Description    string     `gorm:"column:description"`
	Status         string     `gorm:"column:status"`
	SortOrder      int        `gorm:"column:sort_order"`
	CurrentVersion int        `gorm:"column:current_version"`
	CreatedBy      int64      `gorm:"column:created_by"`
	UpdatedBy      int64      `gorm:"column:updated_by"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

func (paymentServicePlanPO) TableName() string { return "subscription_plans" }

type paymentServicePlanVersionPO struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	PlanID            int64     `gorm:"column:plan_id"`
	Version           int       `gorm:"column:version"`
	Cycle             string    `gorm:"column:billing_cycle"`
	PriceMicros       int64     `gorm:"column:price_micros"`
	Currency          string    `gorm:"column:currency"`
	CreditGrantMicros int64     `gorm:"column:credit_grant_micros"`
	FeaturesJSON      string    `gorm:"column:features_json"`
	EffectiveAt       time.Time `gorm:"column:effective_at"`
	CreatedBy         int64     `gorm:"column:created_by"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (paymentServicePlanVersionPO) TableName() string { return "subscription_plan_versions" }

type paymentServiceOrderPO struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	OrderNo           string     `gorm:"column:order_no;uniqueIndex:uk_billing_order_no"`
	UserID            int64      `gorm:"column:user_id"`
	AccountID         int64      `gorm:"column:account_id"`
	OrderType         string     `gorm:"column:order_type"`
	Status            string     `gorm:"column:status"`
	PaymentStatus     string     `gorm:"column:payment_status"`
	FulfillmentStatus string     `gorm:"column:fulfillment_status"`
	TotalMicros       int64      `gorm:"column:total_micros"`
	Currency          string     `gorm:"column:currency"`
	SnapshotJSON      string     `gorm:"column:snapshot_json"`
	Version           int64      `gorm:"column:version"`
	ExpiresAt         time.Time  `gorm:"column:expires_at"`
	PaidAt            *time.Time `gorm:"column:paid_at"`
	FulfilledAt       *time.Time `gorm:"column:fulfilled_at"`
	ClosedAt          *time.Time `gorm:"column:closed_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (paymentServiceOrderPO) TableName() string { return "billing_orders" }

type paymentServiceOrderItemPO struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	OrderID         int64     `gorm:"column:order_id"`
	ItemType        string    `gorm:"column:item_type"`
	TargetID        int64     `gorm:"column:target_id"`
	TargetVersionID int64     `gorm:"column:target_version_id"`
	Quantity        int       `gorm:"column:quantity"`
	UnitPriceMicros int64     `gorm:"column:unit_price_micros"`
	SnapshotJSON    string    `gorm:"column:snapshot_json"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (paymentServiceOrderItemPO) TableName() string { return "billing_order_items" }

type paymentServicePaymentPO struct {
	ID                    int64     `gorm:"column:id;primaryKey"`
	OrderID               int64     `gorm:"column:order_id"`
	Gateway               string    `gorm:"column:gateway;uniqueIndex:uk_payment_gateway_transaction,priority:1"`
	ProviderTransactionID string    `gorm:"column:provider_transaction_id;uniqueIndex:uk_payment_gateway_transaction,priority:2"`
	Status                string    `gorm:"column:status"`
	AmountMicros          int64     `gorm:"column:amount_micros"`
	Currency              string    `gorm:"column:currency"`
	EventDigest           string    `gorm:"column:event_digest;uniqueIndex:uk_payment_event_digest"`
	FailureCode           string    `gorm:"column:failure_code"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (paymentServicePaymentPO) TableName() string { return "payment_transactions" }

type paymentServiceSubscriptionPO struct {
	ID                     int64     `gorm:"column:id;primaryKey"`
	AccountID              int64     `gorm:"column:account_id"`
	PlanID                 int64     `gorm:"column:plan_id"`
	PlanVersionID          int64     `gorm:"column:plan_version_id"`
	SourceOrderID          int64     `gorm:"column:source_order_id;uniqueIndex:uk_user_subscription_source_order"`
	Status                 string    `gorm:"column:status"`
	CurrentPeriodStart     time.Time `gorm:"column:current_period_start"`
	CurrentPeriodEnd       time.Time `gorm:"column:current_period_end"`
	AutoRenew              bool      `gorm:"column:auto_renew"`
	ProviderSubscriptionID string    `gorm:"column:provider_subscription_id"`
	Version                int64     `gorm:"column:version"`
	CreatedAt              time.Time `gorm:"column:created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at"`
}

func (paymentServiceSubscriptionPO) TableName() string { return "user_subscriptions" }

type paymentServiceSystemConfigPO struct {
	ID                int64  `gorm:"column:id;primaryKey"`
	CreditName        string `gorm:"column:credit_name"`
	DisplayScale      int    `gorm:"column:display_scale"`
	AllowNegative     bool   `gorm:"column:allow_negative"`
	SettlementEnabled bool   `gorm:"column:settlement_enabled"`
	PaymentEnabled    bool   `gorm:"column:payment_enabled"`
	DefaultCurrency   string `gorm:"column:default_currency"`
	Version           int64  `gorm:"column:version"`
}

func (paymentServiceSystemConfigPO) TableName() string { return "billing_system_config" }

type paymentServiceNotificationOutboxPO struct {
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

func (paymentServiceNotificationOutboxPO) TableName() string {
	return "notification_outbox"
}
