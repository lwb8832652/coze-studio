// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
)

func (r *MySQLRepository) RunCommerceTransaction(ctx context.Context, fn func(domainbilling.CommerceRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&MySQLRepository{db: tx, idGen: r.idGen})
	})
}

func (r *MySQLRepository) AppendBillingNotificationOutbox(ctx context.Context, event domainnotification.Event) error {
	if r == nil || r.db == nil || r.idGen == nil {
		return domainnotification.ErrStorage
	}
	return infranotification.NewMySQLRepository(r.db, r.idGen).AppendInTransaction(ctx, r.db, event)
}

func (r *MySQLRepository) GetPublishedPlanVersion(ctx context.Context, planID int64, now time.Time) (*domainbilling.SubscriptionPlan, *domainbilling.PlanVersion, error) {
	var plan planPO
	if err := r.db.WithContext(ctx).Where("id = ? AND status = ? AND deleted_at IS NULL", planID, string(domainbilling.CatalogStatusPublished)).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, domainbilling.ErrNotFound
		}
		return nil, nil, err
	}
	var version planVersionPO
	if err := r.db.WithContext(ctx).Where("plan_id = ? AND version = ? AND effective_at <= ?", plan.ID, plan.CurrentVersion, now).First(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, domainbilling.ErrNotFound
		}
		return nil, nil, err
	}
	return plan.toDomain(), version.toDomain(), nil
}

func (r *MySQLRepository) GetPublishedCreditPackage(ctx context.Context, packageID int64) (*domainbilling.CreditPackage, error) {
	var po creditPackagePO
	err := r.db.WithContext(ctx).Where("id = ? AND status = ? AND deleted_at IS NULL", packageID, string(domainbilling.CatalogStatusPublished)).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) GetAccountID(ctx context.Context, subject domainbilling.Subject) (int64, error) {
	var po accountPO
	err := r.db.WithContext(ctx).Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, domainbilling.ErrNotFound
	}
	return po.ID, err
}

func (r *MySQLRepository) CreateOrder(ctx context.Context, order *domainbilling.Order, item *domainbilling.OrderItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		orderID, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		itemID, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		order.ID, item.ID, item.OrderID = orderID, itemID, orderID
		if err = tx.Create(orderPOFromDomain(order)).Error; err != nil {
			return err
		}
		return tx.Create(orderItemPOFromDomain(item)).Error
	})
}

func (r *MySQLRepository) GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*domainbilling.Order, *domainbilling.OrderItem, error) {
	var order orderPO
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	var item orderItemPO
	err = r.db.WithContext(ctx).Where("order_id = ?", order.ID).Order("id ASC").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return order.toDomain(), item.toDomain(), nil
}

func (r *MySQLRepository) UpdateOrder(ctx context.Context, order *domainbilling.Order, expectedVersion int64) error {
	result := r.db.WithContext(ctx).Model(&orderPO{}).Where("id = ? AND version = ?", order.ID, expectedVersion).Updates(map[string]any{
		"status": string(order.Status), "payment_status": string(order.PaymentStatus),
		"fulfillment_status": string(order.FulfillmentStatus), "version": expectedVersion + 1,
		"paid_at": order.PaidAt, "fulfilled_at": order.FulfilledAt, "closed_at": order.ClosedAt, "updated_at": order.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainbilling.ErrVersionConflict
	}
	order.Version = expectedVersion + 1
	return nil
}

func (r *MySQLRepository) CreatePaymentTransaction(ctx context.Context, transaction *domainbilling.PaymentTransaction) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	transaction.ID = id
	candidate := paymentPOFromDomain(transaction)
	if err = r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(candidate).Error; err != nil {
		return err
	}
	existing, err := r.findPaymentTransactionIdentity(ctx, candidate.EventDigest, candidate.Gateway, candidate.ProviderTransactionID)
	if err != nil {
		return err
	}
	if !samePaymentTransactionIdentity(existing, candidate) {
		return domainbilling.ErrIdempotencyConflict
	}
	transaction.ID = existing.ID
	return nil
}

func (r *MySQLRepository) FindPaymentByEventDigest(ctx context.Context, digest string) (*domainbilling.PaymentTransaction, error) {
	var po paymentPO
	err := r.db.WithContext(ctx).Where("event_digest = ?", digest).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) FindPaymentByGatewayTransaction(ctx context.Context, gateway string, providerTransactionID string) (*domainbilling.PaymentTransaction, error) {
	var po paymentPO
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("gateway = ? AND provider_transaction_id = ?", gateway, providerTransactionID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) GrantCredits(ctx context.Context, input domainbilling.GrantInput) (*domainbilling.GrantResult, error) {
	ledger := domainbilling.NewService(r)
	return ledger.Grant(ctx, input)
}

func (r *MySQLRepository) UpsertSubscriptionForOrder(ctx context.Context, subscription *domainbilling.UserSubscription) error {
	var existing subscriptionPO
	err := r.db.WithContext(ctx).Where("source_order_id = ?", subscription.SourceOrderID).First(&existing).Error
	if err == nil {
		subscription.ID = existing.ID
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	now := time.Now().UTC()
	if err = r.db.WithContext(ctx).Model(&subscriptionPO{}).
		Where("account_id = ? AND status IN ?", subscription.AccountID, []string{
			string(domainbilling.SubscriptionStatusActive),
			string(domainbilling.SubscriptionStatusPastDue),
		}).Updates(map[string]any{
		"status":     string(domainbilling.SubscriptionStatusCancelled),
		"auto_renew": false,
		"version":    gorm.Expr("version + 1"),
		"updated_at": now,
	}).Error; err != nil {
		return err
	}
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	subscription.ID = id
	return r.db.WithContext(ctx).Create(subscriptionPOFromDomain(subscription)).Error
}

type planPO struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	Key            string    `gorm:"column:plan_key"`
	Name           string    `gorm:"column:name"`
	Description    string    `gorm:"column:description"`
	Status         string    `gorm:"column:status"`
	SortOrder      int       `gorm:"column:sort_order"`
	CurrentVersion int       `gorm:"column:current_version"`
	CreatedBy      int64     `gorm:"column:created_by"`
	UpdatedBy      int64     `gorm:"column:updated_by"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (planPO) TableName() string { return "subscription_plans" }
func (p planPO) toDomain() *domainbilling.SubscriptionPlan {
	return &domainbilling.SubscriptionPlan{ID: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, Status: domainbilling.CatalogStatus(p.Status), SortOrder: p.SortOrder, CurrentVersion: p.CurrentVersion, CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

type planVersionPO struct {
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

func (planVersionPO) TableName() string { return "subscription_plan_versions" }
func (p planVersionPO) toDomain() *domainbilling.PlanVersion {
	return &domainbilling.PlanVersion{ID: p.ID, PlanID: p.PlanID, Version: p.Version, Cycle: domainbilling.BillingCycle(p.Cycle), PriceMicros: p.PriceMicros, Currency: p.Currency, CreditGrantMicros: p.CreditGrantMicros, FeaturesJSON: p.FeaturesJSON, EffectiveAt: p.EffectiveAt, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt}
}

type creditPackagePO struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	Key           string    `gorm:"column:package_key"`
	Name          string    `gorm:"column:name"`
	Description   string    `gorm:"column:description"`
	Status        string    `gorm:"column:status"`
	PriceMicros   int64     `gorm:"column:price_micros"`
	Currency      string    `gorm:"column:currency"`
	CreditMicros  int64     `gorm:"column:credit_micros"`
	ValidityDays  int       `gorm:"column:validity_days"`
	PurchaseLimit int       `gorm:"column:purchase_limit"`
	SortOrder     int       `gorm:"column:sort_order"`
	Version       int64     `gorm:"column:version"`
	CreatedBy     int64     `gorm:"column:created_by"`
	UpdatedBy     int64     `gorm:"column:updated_by"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (creditPackagePO) TableName() string { return "credit_packages" }
func (p creditPackagePO) toDomain() *domainbilling.CreditPackage {
	return &domainbilling.CreditPackage{ID: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, Status: domainbilling.CatalogStatus(p.Status), PriceMicros: p.PriceMicros, Currency: p.Currency, CreditMicros: p.CreditMicros, ValidityDays: p.ValidityDays, PurchaseLimit: p.PurchaseLimit, SortOrder: p.SortOrder, Version: p.Version, CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

type orderPO struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	OrderNo           string     `gorm:"column:order_no"`
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

func (orderPO) TableName() string { return "billing_orders" }
func orderPOFromDomain(o *domainbilling.Order) *orderPO {
	return &orderPO{ID: o.ID, OrderNo: o.OrderNo, UserID: o.UserID, AccountID: o.AccountID, OrderType: string(o.Type), Status: string(o.Status), PaymentStatus: string(o.PaymentStatus), FulfillmentStatus: string(o.FulfillmentStatus), TotalMicros: o.TotalMicros, Currency: o.Currency, SnapshotJSON: o.SnapshotJSON, Version: o.Version, ExpiresAt: o.ExpiresAt, PaidAt: o.PaidAt, FulfilledAt: o.FulfilledAt, ClosedAt: o.ClosedAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt}
}
func (p orderPO) toDomain() *domainbilling.Order {
	return &domainbilling.Order{ID: p.ID, OrderNo: p.OrderNo, UserID: p.UserID, AccountID: p.AccountID, Type: domainbilling.OrderType(p.OrderType), Status: domainbilling.OrderStatus(p.Status), PaymentStatus: domainbilling.PaymentStatus(p.PaymentStatus), FulfillmentStatus: domainbilling.FulfillmentStatus(p.FulfillmentStatus), TotalMicros: p.TotalMicros, Currency: p.Currency, SnapshotJSON: p.SnapshotJSON, Version: p.Version, ExpiresAt: p.ExpiresAt, PaidAt: p.PaidAt, FulfilledAt: p.FulfilledAt, ClosedAt: p.ClosedAt, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

type orderItemPO struct {
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

func (orderItemPO) TableName() string { return "billing_order_items" }
func orderItemPOFromDomain(i *domainbilling.OrderItem) *orderItemPO {
	return &orderItemPO{ID: i.ID, OrderID: i.OrderID, ItemType: string(i.Type), TargetID: i.TargetID, TargetVersionID: i.TargetVersionID, Quantity: i.Quantity, UnitPriceMicros: i.UnitPriceMicros, SnapshotJSON: i.SnapshotJSON, CreatedAt: i.CreatedAt}
}
func (p orderItemPO) toDomain() *domainbilling.OrderItem {
	return &domainbilling.OrderItem{ID: p.ID, OrderID: p.OrderID, Type: domainbilling.OrderType(p.ItemType), TargetID: p.TargetID, TargetVersionID: p.TargetVersionID, Quantity: p.Quantity, UnitPriceMicros: p.UnitPriceMicros, SnapshotJSON: p.SnapshotJSON, CreatedAt: p.CreatedAt}
}

type paymentPO struct {
	ID                    int64     `gorm:"column:id;primaryKey"`
	OrderID               int64     `gorm:"column:order_id"`
	Gateway               string    `gorm:"column:gateway"`
	ProviderTransactionID string    `gorm:"column:provider_transaction_id"`
	Status                string    `gorm:"column:status"`
	AmountMicros          int64     `gorm:"column:amount_micros"`
	Currency              string    `gorm:"column:currency"`
	EventDigest           string    `gorm:"column:event_digest"`
	FailureCode           string    `gorm:"column:failure_code"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (paymentPO) TableName() string { return "payment_transactions" }
func paymentPOFromDomain(p *domainbilling.PaymentTransaction) *paymentPO {
	return &paymentPO{ID: p.ID, OrderID: p.OrderID, Gateway: p.Gateway, ProviderTransactionID: p.ProviderTransactionID, Status: string(p.Status), AmountMicros: p.AmountMicros, Currency: p.Currency, EventDigest: p.EventDigest, FailureCode: p.FailureCode, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}
func (p paymentPO) toDomain() *domainbilling.PaymentTransaction {
	return &domainbilling.PaymentTransaction{ID: p.ID, OrderID: p.OrderID, Gateway: p.Gateway, ProviderTransactionID: p.ProviderTransactionID, Status: domainbilling.PaymentStatus(p.Status), AmountMicros: p.AmountMicros, Currency: p.Currency, EventDigest: p.EventDigest, FailureCode: p.FailureCode, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

func (r *MySQLRepository) findPaymentTransactionIdentity(ctx context.Context, eventDigest string, gateway string, providerTransactionID string) (*paymentPO, error) {
	var po paymentPO
	query := r.db.WithContext(ctx)
	if strings.EqualFold(strings.TrimSpace(r.db.Dialector.Name()), "mysql") {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.
		Where("event_digest = ? OR (gateway = ? AND provider_transaction_id = ?)", eventDigest, gateway, providerTransactionID).
		Order("id ASC").
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrIdempotencyConflict
	}
	if err != nil {
		return nil, err
	}
	return &po, nil
}

func samePaymentTransactionIdentity(existing *paymentPO, candidate *paymentPO) bool {
	return existing != nil &&
		candidate != nil &&
		existing.OrderID == candidate.OrderID &&
		strings.EqualFold(existing.Gateway, candidate.Gateway) &&
		strings.TrimSpace(existing.ProviderTransactionID) == strings.TrimSpace(candidate.ProviderTransactionID) &&
		existing.Status == candidate.Status &&
		existing.AmountMicros == candidate.AmountMicros &&
		strings.EqualFold(existing.Currency, candidate.Currency)
}

type subscriptionPO struct {
	ID                     int64     `gorm:"column:id;primaryKey"`
	AccountID              int64     `gorm:"column:account_id"`
	PlanID                 int64     `gorm:"column:plan_id"`
	PlanVersionID          int64     `gorm:"column:plan_version_id"`
	SourceOrderID          int64     `gorm:"column:source_order_id"`
	Status                 string    `gorm:"column:status"`
	CurrentPeriodStart     time.Time `gorm:"column:current_period_start"`
	CurrentPeriodEnd       time.Time `gorm:"column:current_period_end"`
	AutoRenew              bool      `gorm:"column:auto_renew"`
	ProviderSubscriptionID string    `gorm:"column:provider_subscription_id"`
	Version                int64     `gorm:"column:version"`
	CreatedAt              time.Time `gorm:"column:created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at"`
}

func (subscriptionPO) TableName() string { return "user_subscriptions" }
func subscriptionPOFromDomain(s *domainbilling.UserSubscription) *subscriptionPO {
	return &subscriptionPO{ID: s.ID, AccountID: s.AccountID, PlanID: s.PlanID, PlanVersionID: s.PlanVersionID, SourceOrderID: s.SourceOrderID, Status: string(s.Status), CurrentPeriodStart: s.CurrentPeriodStart, CurrentPeriodEnd: s.CurrentPeriodEnd, AutoRenew: s.AutoRenew, ProviderSubscriptionID: s.ProviderSubscriptionID, Version: s.Version, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}
