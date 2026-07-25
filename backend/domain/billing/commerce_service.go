// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type CommerceService struct {
	repository CommerceRepository
	ledger     *Service
	now        func() time.Time
}

func NewCommerceService(repository CommerceRepository, ledger *Service) *CommerceService {
	return &CommerceService{repository: repository, ledger: ledger, now: time.Now}
}

func (s *CommerceService) CreateOrder(ctx context.Context, input CreateOrderInput) (*Order, error) {
	if input.UserID <= 0 || input.TargetID <= 0 || input.Subject.Validate() != nil {
		return nil, ErrInvalidInput
	}
	orderNo, err := normalizeBusinessNo(input.OrderNo)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	balance, err := s.ledger.GetBalance(ctx, input.Subject)
	if err != nil {
		return nil, err
	}
	accountID := balance.AccountID
	var snapshot OrderSnapshot
	var total int64
	var currency string
	var versionID int64
	switch input.Type {
	case OrderTypeSubscription:
		plan, version, getErr := s.repository.GetPublishedPlanVersion(ctx, input.TargetID, now)
		if getErr != nil {
			return nil, getErr
		}
		total, currency, versionID = version.PriceMicros, version.Currency, version.ID
		snapshot = OrderSnapshot{Name: plan.Name, CreditMicros: version.CreditGrantMicros, BillingCycle: version.Cycle, PlanID: plan.ID, PlanVersionID: version.ID}
	case OrderTypeCreditPackage:
		pack, getErr := s.repository.GetPublishedCreditPackage(ctx, input.TargetID)
		if getErr != nil {
			return nil, getErr
		}
		total, currency = pack.PriceMicros, pack.Currency
		snapshot = OrderSnapshot{Name: pack.Name, CreditMicros: pack.CreditMicros, ValidityDays: pack.ValidityDays, PackageID: pack.ID}
	default:
		return nil, ErrInvalidInput
	}
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	order := &Order{OrderNo: orderNo, UserID: input.UserID, AccountID: accountID, Type: input.Type,
		Status: OrderStatusPending, PaymentStatus: PaymentStatusPending, FulfillmentStatus: FulfillmentStatusPending,
		TotalMicros: total, Currency: strings.ToUpper(currency), SnapshotJSON: string(snapshotBytes),
		Version: InitialVersion, ExpiresAt: now.Add(30 * time.Minute), CreatedAt: now, UpdatedAt: now}
	item := &OrderItem{Type: input.Type, TargetID: input.TargetID, TargetVersionID: versionID, Quantity: 1,
		UnitPriceMicros: total, SnapshotJSON: string(snapshotBytes), CreatedAt: now}
	if err = s.repository.CreateOrder(ctx, order, item); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *CommerceService) RecordPaymentSucceeded(ctx context.Context, input PaymentSucceededInput) (*Order, error) {
	orderNo, err := normalizeBusinessNo(input.OrderNo)
	if err != nil {
		return nil, err
	}
	gateway := strings.TrimSpace(input.Gateway)
	providerTransactionID := strings.TrimSpace(input.ProviderTransactionID)
	providerEventID := paymentProviderEventID(input)
	eventDigest := strings.ToLower(strings.TrimSpace(input.EventDigest))
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.AmountMicros < 0 || gateway == "" || providerTransactionID == "" || providerEventID == "" || len(eventDigest) != 64 || currency == "" {
		return nil, ErrInvalidInput
	}
	var result *Order
	err = s.repository.RunCommerceTransaction(ctx, func(repository CommerceRepository) error {
		existing, findErr := repository.FindPaymentByEventDigest(ctx, eventDigest)
		if findErr == nil {
			order, _, orderErr := repository.GetOrderByNoForUpdate(ctx, orderNo)
			if orderErr != nil {
				return orderErr
			}
			if !sameSucceededPaymentTransaction(existing, order, input.AmountMicros, currency, gateway, providerTransactionID) {
				return ErrIdempotencyConflict
			}
			result = order
			return nil
		}
		if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}

		order, _, orderErr := repository.GetOrderByNoForUpdate(ctx, orderNo)
		if orderErr != nil {
			return orderErr
		}
		existingByProvider, providerErr := repository.FindPaymentByGatewayTransaction(ctx, gateway, providerTransactionID)
		if providerErr == nil {
			if !sameSucceededPaymentTransaction(existingByProvider, order, input.AmountMicros, currency, gateway, providerTransactionID) {
				return ErrIdempotencyConflict
			}
			result = order
			return nil
		}
		if !errors.Is(providerErr, ErrNotFound) {
			return providerErr
		}
		if order.TotalMicros != input.AmountMicros || !strings.EqualFold(order.Currency, currency) {
			return ErrIdempotencyConflict
		}
		now := s.now().UTC()
		if order.Status == OrderStatusClosed {
			return ErrReservationNotActive
		}
		if order.Status != OrderStatusPending || order.PaymentStatus != PaymentStatusPending {
			return ErrIdempotencyConflict
		}
		transaction := &PaymentTransaction{OrderID: order.ID, Gateway: gateway, ProviderTransactionID: providerTransactionID,
			Status: PaymentStatusSucceeded, AmountMicros: input.AmountMicros, Currency: currency, EventDigest: eventDigest,
			CreatedAt: now, UpdatedAt: now}
		if orderErr = repository.CreatePaymentTransaction(ctx, transaction); orderErr != nil {
			return orderErr
		}
		expectedVersion := order.Version
		order.Status, order.PaymentStatus, order.PaidAt, order.UpdatedAt = OrderStatusPaid, PaymentStatusSucceeded, &now, now
		if orderErr = repository.UpdateOrder(ctx, order, expectedVersion); orderErr != nil {
			return orderErr
		}
		subject, orderErr := billingOrderSubject(ctx, repository, order)
		if orderErr != nil {
			return orderErr
		}
		event := billingPaymentSucceededNotificationEvent(order, subject, gateway, providerEventID, providerTransactionID, now)
		if orderErr = appendBillingNotification(ctx, repository, event); orderErr != nil {
			return orderErr
		}
		result = order
		return nil
	})
	return result, err
}

func (s *CommerceService) FulfillOrder(ctx context.Context, orderNo string) (*Order, error) {
	orderNo, err := normalizeBusinessNo(orderNo)
	if err != nil {
		return nil, err
	}
	var order *Order
	err = s.repository.RunCommerceTransaction(ctx, func(repository CommerceRepository) error {
		locked, item, txErr := repository.GetOrderByNoForUpdate(ctx, orderNo)
		if txErr != nil {
			return txErr
		}
		if locked.FulfillmentStatus == FulfillmentStatusSucceeded {
			order = locked
			return nil
		}
		if locked.PaymentStatus != PaymentStatusSucceeded {
			return ErrReservationNotActive
		}
		var snapshot OrderSnapshot
		if txErr = json.Unmarshal([]byte(item.SnapshotJSON), &snapshot); txErr != nil {
			return txErr
		}
		now := s.now().UTC()
		subject, txErr := billingOrderSubject(ctx, repository, locked)
		if txErr != nil {
			return txErr
		}
		var expiresAt *time.Time
		if snapshot.ValidityDays > 0 {
			value := now.AddDate(0, 0, snapshot.ValidityDays)
			expiresAt = &value
		}
		if snapshot.CreditMicros > 0 {
			_, txErr = repository.GrantCredits(ctx, GrantInput{Subject: subject, AmountMicros: snapshot.CreditMicros,
				BusinessNo: "order-grant:" + locked.OrderNo, SourceType: string(locked.Type), SourceID: locked.OrderNo, ExpiresAt: expiresAt,
				MetadataJSON: locked.SnapshotJSON})
			if txErr != nil {
				return txErr
			}
		}
		var subscriptionID int64
		if locked.Type == OrderTypeSubscription {
			periodEnd := subscriptionPeriodEnd(now, snapshot.BillingCycle)
			subscription := &UserSubscription{AccountID: locked.AccountID, PlanID: snapshot.PlanID, PlanVersionID: snapshot.PlanVersionID,
				SourceOrderID: locked.ID, Status: SubscriptionStatusActive, CurrentPeriodStart: now, CurrentPeriodEnd: periodEnd,
				Version: InitialVersion, CreatedAt: now, UpdatedAt: now}
			if txErr = repository.UpsertSubscriptionForOrder(ctx, subscription); txErr != nil {
				return txErr
			}
			subscriptionID = subscription.ID
		}
		expectedVersion := locked.Version
		locked.Status, locked.FulfillmentStatus, locked.FulfilledAt, locked.UpdatedAt = OrderStatusFulfilled, FulfillmentStatusSucceeded, &now, now
		if txErr = repository.UpdateOrder(ctx, locked, expectedVersion); txErr != nil {
			return txErr
		}
		if locked.Type == OrderTypeSubscription {
			event := billingSubscriptionActivatedNotificationEvent(locked, subject, subscriptionID, now)
			if txErr = appendBillingNotification(ctx, repository, event); txErr != nil {
				return txErr
			}
		}
		order = locked
		return nil
	})
	return order, err
}

func subscriptionPeriodEnd(start time.Time, cycle BillingCycle) time.Time {
	if cycle == BillingCycleYearly {
		return start.AddDate(1, 0, 0)
	}
	return start.AddDate(0, 1, 0)
}

func paymentProviderEventID(input PaymentSucceededInput) string {
	value := strings.TrimSpace(input.ProviderEventID)
	if value == "" {
		value = strings.TrimSpace(input.EventDigest)
	}
	return value
}

func sameSucceededPaymentTransaction(transaction *PaymentTransaction, order *Order, amountMicros int64, currency string, gateway string, providerTransactionID string) bool {
	return transaction != nil &&
		order != nil &&
		transaction.OrderID == order.ID &&
		order.PaymentStatus == PaymentStatusSucceeded &&
		transaction.Status == PaymentStatusSucceeded &&
		transaction.AmountMicros == amountMicros &&
		strings.EqualFold(transaction.Currency, currency) &&
		strings.EqualFold(transaction.Gateway, gateway) &&
		strings.TrimSpace(transaction.ProviderTransactionID) == providerTransactionID
}

func billingOrderSubject(ctx context.Context, repository CommerceRepository, order *Order) (Subject, error) {
	if order == nil || order.AccountID <= 0 || order.UserID <= 0 {
		return Subject{}, ErrInvalidInput
	}
	account, err := repository.GetAccountByIDForUpdate(ctx, order.AccountID)
	if err != nil {
		return Subject{}, err
	}
	if account == nil || account.Subject.Validate() != nil {
		return Subject{}, fmt.Errorf("%w: billing account subject is invalid", domainnotification.ErrRecipientResolution)
	}
	if account.Subject.Type == SubjectTypeUser && account.Subject.ID != order.UserID {
		return Subject{}, fmt.Errorf("%w: billing order user does not match account subject", domainnotification.ErrRecipientResolution)
	}
	return account.Subject, nil
}

func appendBillingNotification(ctx context.Context, repository CommerceRepository, event domainnotification.Event) error {
	event, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return err
	}
	if err = domainnotification.DefaultTemplateRegistry().ValidateAppendable(event); err != nil {
		return err
	}
	return repository.AppendBillingNotificationOutbox(ctx, event)
}

func billingPaymentSucceededNotificationEvent(order *Order, subject Subject, gateway string, providerEventID string, providerTransactionID string, occurredAt time.Time) domainnotification.Event {
	aggregateID := strconv.FormatInt(order.ID, 10)
	return domainnotification.Event{
		EventID: stableBillingEventID(
			"billing.payment_succeeded",
			gateway,
			providerEventID,
			providerTransactionID,
		),
		EventType:        domainnotification.EventBillingPaymentSucceeded,
		AggregateType:    "billing_order",
		AggregateID:      aggregateID,
		AggregateVersion: order.Version,
		OccurredAt:       occurredAt,
		ActorID:          order.UserID,
		SpaceID:          billingNotificationSpaceID(subject),
		RecipientPolicy:  domainnotification.RecipientActor,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName: billingOrderDisplayName(order.Type),
			TargetID:            aggregateID,
		},
	}
}

func billingSubscriptionActivatedNotificationEvent(order *Order, subject Subject, subscriptionID int64, occurredAt time.Time) domainnotification.Event {
	aggregateID := strconv.FormatInt(order.ID, 10)
	return domainnotification.Event{
		EventID: stableBillingEventID(
			"billing.subscription_activated",
			aggregateID,
			strconv.FormatInt(subscriptionID, 10),
			string(FulfillmentStatusSucceeded),
		),
		EventType:        domainnotification.EventBillingSubscriptionActivated,
		AggregateType:    "billing_order_fulfillment",
		AggregateID:      aggregateID,
		AggregateVersion: order.Version,
		OccurredAt:       occurredAt,
		ActorID:          order.UserID,
		SpaceID:          billingNotificationSpaceID(subject),
		RecipientPolicy:  domainnotification.RecipientActor,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName: "Subscription",
			TargetID:            aggregateID,
		},
	}
}

func stableBillingEventID(prefix string, values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strings.TrimSpace(value)))
	}
	return prefix + ":" + hex.EncodeToString(hash.Sum(nil))
}

func billingNotificationSpaceID(subject Subject) int64 {
	if subject.Type == SubjectTypeWorkspace {
		return subject.ID
	}
	return 0
}

func billingOrderDisplayName(orderType OrderType) string {
	switch orderType {
	case OrderTypeSubscription:
		return "Subscription order"
	case OrderTypeCreditPackage:
		return "Credit package order"
	default:
		return "Billing order"
	}
}
