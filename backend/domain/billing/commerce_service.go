// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
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
	if input.AmountMicros < 0 || strings.TrimSpace(input.Gateway) == "" || strings.TrimSpace(input.ProviderTransactionID) == "" || len(input.EventDigest) != 64 {
		return nil, ErrInvalidInput
	}
	var result *Order
	err := s.repository.RunCommerceTransaction(ctx, func(repository CommerceRepository) error {
		existing, findErr := repository.FindPaymentByEventDigest(ctx, input.EventDigest)
		if findErr == nil {
			order, _, orderErr := repository.GetOrderByNoForUpdate(ctx, input.OrderNo)
			if orderErr != nil {
				return orderErr
			}
			if existing.OrderID != order.ID || existing.AmountMicros != input.AmountMicros {
				return ErrIdempotencyConflict
			}
			result = order
			return nil
		}
		if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		order, _, orderErr := repository.GetOrderByNoForUpdate(ctx, input.OrderNo)
		if orderErr != nil {
			return orderErr
		}
		if order.TotalMicros != input.AmountMicros || !strings.EqualFold(order.Currency, input.Currency) {
			return ErrIdempotencyConflict
		}
		if order.Status == OrderStatusClosed || time.Now().UTC().After(order.ExpiresAt) {
			return ErrReservationNotActive
		}
		now := s.now().UTC()
		transaction := &PaymentTransaction{OrderID: order.ID, Gateway: input.Gateway, ProviderTransactionID: input.ProviderTransactionID,
			Status: PaymentStatusSucceeded, AmountMicros: input.AmountMicros, Currency: strings.ToUpper(input.Currency), EventDigest: input.EventDigest,
			CreatedAt: now, UpdatedAt: now}
		if orderErr = repository.CreatePaymentTransaction(ctx, transaction); orderErr != nil {
			return orderErr
		}
		expectedVersion := order.Version
		order.Status, order.PaymentStatus, order.PaidAt, order.UpdatedAt = OrderStatusPaid, PaymentStatusSucceeded, &now, now
		if orderErr = repository.UpdateOrder(ctx, order, expectedVersion); orderErr != nil {
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
	order, item, err := s.repository.GetOrderByNoForUpdate(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if order.FulfillmentStatus == FulfillmentStatusSucceeded {
		return order, nil
	}
	if order.PaymentStatus != PaymentStatusSucceeded {
		return nil, ErrReservationNotActive
	}
	var snapshot OrderSnapshot
	if err = json.Unmarshal([]byte(item.SnapshotJSON), &snapshot); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	var expiresAt *time.Time
	if snapshot.ValidityDays > 0 {
		value := now.AddDate(0, 0, snapshot.ValidityDays)
		expiresAt = &value
	}
	if snapshot.CreditMicros > 0 {
		_, err = s.ledger.Grant(ctx, GrantInput{Subject: Subject{Type: SubjectTypeUser, ID: order.UserID}, AmountMicros: snapshot.CreditMicros,
			BusinessNo: "order-grant:" + order.OrderNo, SourceType: string(order.Type), SourceID: order.OrderNo, ExpiresAt: expiresAt,
			MetadataJSON: order.SnapshotJSON})
		if err != nil {
			return nil, err
		}
	}
	err = s.repository.RunCommerceTransaction(ctx, func(repository CommerceRepository) error {
		locked, _, txErr := repository.GetOrderByNoForUpdate(ctx, orderNo)
		if txErr != nil {
			return txErr
		}
		if locked.FulfillmentStatus == FulfillmentStatusSucceeded {
			order = locked
			return nil
		}
		if order.Type == OrderTypeSubscription {
			periodEnd := subscriptionPeriodEnd(now, snapshot.BillingCycle)
			subscription := &UserSubscription{AccountID: order.AccountID, PlanID: snapshot.PlanID, PlanVersionID: snapshot.PlanVersionID,
				SourceOrderID: order.ID, Status: SubscriptionStatusActive, CurrentPeriodStart: now, CurrentPeriodEnd: periodEnd,
				Version: InitialVersion, CreatedAt: now, UpdatedAt: now}
			if txErr = repository.UpsertSubscriptionForOrder(ctx, subscription); txErr != nil {
				return txErr
			}
		}
		expectedVersion := locked.Version
		locked.Status, locked.FulfillmentStatus, locked.FulfilledAt, locked.UpdatedAt = OrderStatusFulfilled, FulfillmentStatusSucceeded, &now, now
		if txErr = repository.UpdateOrder(ctx, locked, expectedVersion); txErr != nil {
			return txErr
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
