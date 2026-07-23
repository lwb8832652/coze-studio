// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import "time"

type CatalogStatus string

const (
	CatalogStatusDraft     CatalogStatus = "draft"
	CatalogStatusPublished CatalogStatus = "published"
	CatalogStatusArchived  CatalogStatus = "archived"
)

type BillingCycle string

const (
	BillingCycleMonthly BillingCycle = "monthly"
	BillingCycleYearly  BillingCycle = "yearly"
)

type SubscriptionPlan struct {
	ID int64; Key, Name, Description string; Status CatalogStatus; SortOrder, CurrentVersion int
	CreatedBy, UpdatedBy int64; CreatedAt, UpdatedAt time.Time
}

type PlanVersion struct {
	ID, PlanID int64; Version int; Cycle BillingCycle; PriceMicros int64; Currency string
	CreditGrantMicros int64; FeaturesJSON string; EffectiveAt time.Time; CreatedBy int64; CreatedAt time.Time
}

type CreditPackage struct {
	ID int64; Key, Name, Description string; Status CatalogStatus; PriceMicros int64; Currency string
	CreditMicros int64; ValidityDays, PurchaseLimit, SortOrder int; Version int64
	CreatedBy, UpdatedBy int64; CreatedAt, UpdatedAt time.Time
}

type OrderType string
type OrderStatus string
type PaymentStatus string
type FulfillmentStatus string

const (
	OrderTypeSubscription OrderType = "subscription"
	OrderTypeCreditPackage OrderType = "credit_package"
	OrderStatusPending OrderStatus = "pending_payment"
	OrderStatusPaid OrderStatus = "paid"
	OrderStatusFulfilled OrderStatus = "fulfilled"
	OrderStatusClosed OrderStatus = "closed"
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusSucceeded PaymentStatus = "succeeded"
	PaymentStatusFailed PaymentStatus = "failed"
	FulfillmentStatusPending FulfillmentStatus = "pending"
	FulfillmentStatusSucceeded FulfillmentStatus = "succeeded"
	FulfillmentStatusFailed FulfillmentStatus = "failed"
)

type Order struct {
	ID int64; OrderNo string; UserID, AccountID int64; Type OrderType; Status OrderStatus
	PaymentStatus PaymentStatus; FulfillmentStatus FulfillmentStatus; TotalMicros int64; Currency string
	SnapshotJSON string; Version int64; ExpiresAt time.Time; PaidAt, FulfilledAt, ClosedAt *time.Time
	CreatedAt, UpdatedAt time.Time
}

type OrderItem struct {
	ID, OrderID int64; Type OrderType; TargetID, TargetVersionID int64; Quantity int
	UnitPriceMicros int64; SnapshotJSON string; CreatedAt time.Time
}

type SubscriptionStatus string

const (
	SubscriptionStatusActive SubscriptionStatus = "active"
	SubscriptionStatusPastDue SubscriptionStatus = "past_due"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
	SubscriptionStatusExpired SubscriptionStatus = "expired"
)

type UserSubscription struct {
	ID, AccountID, PlanID, PlanVersionID, SourceOrderID int64; Status SubscriptionStatus
	CurrentPeriodStart, CurrentPeriodEnd time.Time; AutoRenew bool; ProviderSubscriptionID string
	Version int64; CreatedAt, UpdatedAt time.Time
}

type PaymentTransaction struct {
	ID, OrderID int64; Gateway, ProviderTransactionID string; Status PaymentStatus
	AmountMicros int64; Currency, EventDigest, FailureCode string; CreatedAt, UpdatedAt time.Time
}

type OrderSnapshot struct {
	Name string `json:"name"`; CreditMicros int64 `json:"credit_micros"`; ValidityDays int `json:"validity_days"`
	BillingCycle BillingCycle `json:"billing_cycle,omitempty"`; PlanID, PlanVersionID, PackageID int64
}

type CreateOrderInput struct { UserID int64; Subject Subject; Type OrderType; TargetID int64; OrderNo string }
type PaymentSucceededInput struct { OrderNo, Gateway, ProviderTransactionID, EventDigest, Currency string; AmountMicros int64 }
