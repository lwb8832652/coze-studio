// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"time"
)

type CommerceRepository interface {
	RunCommerceTransaction(ctx context.Context, fn func(CommerceRepository) error) error
	GetPublishedPlanVersion(ctx context.Context, planID int64, now time.Time) (*SubscriptionPlan, *PlanVersion, error)
	GetPublishedCreditPackage(ctx context.Context, packageID int64) (*CreditPackage, error)
	GetAccountID(ctx context.Context, subject Subject) (int64, error)
	CreateOrder(ctx context.Context, order *Order, item *OrderItem) error
	GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, *OrderItem, error)
	UpdateOrder(ctx context.Context, order *Order, expectedVersion int64) error
	CreatePaymentTransaction(ctx context.Context, transaction *PaymentTransaction) error
	FindPaymentByEventDigest(ctx context.Context, digest string) (*PaymentTransaction, error)
	UpsertSubscriptionForOrder(ctx context.Context, subscription *UserSubscription) error
}
