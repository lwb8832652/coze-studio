// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type CommerceRepository interface {
	RunCommerceTransaction(ctx context.Context, fn func(CommerceRepository) error) error
	GetPublishedPlanVersion(ctx context.Context, planID int64, now time.Time) (*SubscriptionPlan, *PlanVersion, error)
	GetPublishedCreditPackage(ctx context.Context, packageID int64) (*CreditPackage, error)
	GetAccountID(ctx context.Context, subject Subject) (int64, error)
	GetAccountByIDForUpdate(ctx context.Context, accountID int64) (*Account, error)
	CreateOrder(ctx context.Context, order *Order, item *OrderItem) error
	GetOrderByNoForUpdate(ctx context.Context, orderNo string) (*Order, *OrderItem, error)
	UpdateOrder(ctx context.Context, order *Order, expectedVersion int64) error
	CreatePaymentTransaction(ctx context.Context, transaction *PaymentTransaction) error
	FindPaymentByEventDigest(ctx context.Context, digest string) (*PaymentTransaction, error)
	FindPaymentByGatewayTransaction(ctx context.Context, gateway string, providerTransactionID string) (*PaymentTransaction, error)
	UpsertSubscriptionForOrder(ctx context.Context, subscription *UserSubscription) error
	GrantCredits(ctx context.Context, input GrantInput) (*GrantResult, error)
	AppendBillingNotificationOutbox(ctx context.Context, event domainnotification.Event) error
}
