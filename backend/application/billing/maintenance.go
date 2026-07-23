// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	infrabilling "github.com/coze-dev/coze-studio/backend/infra/billing"
)

type CreditAdjustmentInput struct {
	Subject      domainbilling.Subject
	AmountMicros int64
	BusinessNo   string
	Reason       string
	ActorUserID  int64
}

type CreditAdjustmentResult struct {
	Balance domainbilling.Balance `json:"balance"`
	Action  string                `json:"action"`
}

type MaintenanceResult struct {
	ReleasedReservations int64                              `json:"released_reservations"`
	ExpiredSubscriptions int64                              `json:"expired_subscriptions"`
	ClosedOrders         int64                              `json:"closed_orders"`
	Reconciliation       *infrabilling.ReconciliationResult `json:"reconciliation,omitempty"`
	Errors               []string                           `json:"errors"`
}

func (s *Service) AdjustCredits(ctx context.Context, input CreditAdjustmentInput) (*CreditAdjustmentResult, error) {
	input.BusinessNo = strings.TrimSpace(input.BusinessNo)
	input.Reason = strings.TrimSpace(input.Reason)
	if s == nil || s.db == nil || s.idGenerator == nil || input.Subject.Validate() != nil || input.AmountMicros == 0 || input.ActorUserID <= 0 || input.BusinessNo == "" || len(input.BusinessNo) > 88 || input.Reason == "" || len(input.Reason) > 500 {
		return nil, domainbilling.ErrInvalidInput
	}
	metadata, err := json.Marshal(map[string]any{"reason": input.Reason, "subject_type": input.Subject.Type, "subject_id": input.Subject.ID})
	if err != nil {
		return nil, err
	}
	var result *CreditAdjustmentResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ledger := domainbilling.NewService(infrabilling.NewMySQLRepository(tx, s.idGenerator))
		admin := infrabilling.NewAdminRepository(tx, s.idGenerator)
		var txErr error
		result, txErr = adjustCreditsInTransaction(ctx, ledger, admin, input, string(metadata))
		return txErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func adjustCreditsInTransaction(
	ctx context.Context,
	ledger *domainbilling.Service,
	admin *infrabilling.AdminRepository,
	input CreditAdjustmentInput,
	metadata string,
) (*CreditAdjustmentResult, error) {
	var balance domainbilling.Balance
	action := "credit"
	ledgerBusinessNo := "admin-credit:" + input.BusinessNo
	if input.AmountMicros > 0 {
		grant, grantErr := ledger.Grant(ctx, domainbilling.GrantInput{
			Subject: input.Subject, AmountMicros: input.AmountMicros, BusinessNo: ledgerBusinessNo,
			SourceType: "admin_adjustment", SourceID: input.BusinessNo,
			ActorUserID: input.ActorUserID, MetadataJSON: metadata,
		})
		if grantErr != nil {
			return nil, grantErr
		}
		balance = grant.Balance
	} else {
		action = "debit"
		amount := -input.AmountMicros
		reserveBusinessNo := "admin-reserve:" + input.BusinessNo
		reservation, reserveErr := ledger.Reserve(ctx, domainbilling.ReserveInput{
			Subject: input.Subject, AmountMicros: amount, BusinessNo: reserveBusinessNo,
		})
		if reserveErr != nil {
			return nil, reserveErr
		}
		settled, settleErr := ledger.Settle(ctx, domainbilling.SettleInput{
			ReservationBusinessNo: reservation.Reservation.ReserveBusinessNo,
			BusinessNo:            "admin-debit:" + input.BusinessNo, ActualMicros: amount,
			ActorUserID: input.ActorUserID, MetadataJSON: metadata,
		})
		if settleErr != nil {
			return nil, settleErr
		}
		balance = settled.Balance
		ledgerBusinessNo = "admin-debit:" + input.BusinessNo
	}
	auditSummary, err := json.Marshal(map[string]any{
		"action": action, "amount_micros": input.AmountMicros, "reason": input.Reason,
		"subject_type": input.Subject.Type, "subject_id": input.Subject.ID,
	})
	if err != nil {
		return nil, err
	}
	if err = admin.CreateAuditLog(ctx, balance.AccountID, input.ActorUserID, "credit_"+action, ledgerBusinessNo, string(auditSummary)); err != nil {
		return nil, err
	}
	return &CreditAdjustmentResult{Balance: balance, Action: action}, nil
}

func (s *Service) RunMaintenance(ctx context.Context) (*MaintenanceResult, error) {
	result := &MaintenanceResult{Errors: make([]string, 0)}
	if s == nil || s.maintenance == nil {
		return result, fmt.Errorf("billing maintenance repository is unavailable")
	}
	now := time.Now().UTC()
	var failures []error

	reservations, err := s.maintenance.ListExpiredReservations(ctx, now, 500)
	if err != nil {
		failures = append(failures, fmt.Errorf("list expired reservations: %w", err))
	} else {
		for _, reservation := range reservations {
			_, releaseErr := s.ledger.Release(ctx, domainbilling.ReleaseInput{
				ReservationBusinessNo: reservation.ReserveBusinessNo,
				BusinessNo:            fmt.Sprintf("reservation-expired:%d", reservation.ID),
			})
			if releaseErr != nil && !errors.Is(releaseErr, domainbilling.ErrReservationNotActive) {
				failures = append(failures, fmt.Errorf("release reservation %d: %w", reservation.ID, releaseErr))
				continue
			}
			result.ReleasedReservations++
		}
	}

	subscriptions, err := s.maintenance.ListDueSubscriptions(ctx, now, 500)
	if err != nil {
		failures = append(failures, fmt.Errorf("list due subscriptions: %w", err))
	} else {
		for _, subscription := range subscriptions {
			if expireErr := s.maintenance.ExpireSubscription(ctx, subscription); expireErr != nil && !errors.Is(expireErr, domainbilling.ErrVersionConflict) {
				failures = append(failures, fmt.Errorf("expire subscription %d: %w", subscription.ID, expireErr))
				continue
			}
			result.ExpiredSubscriptions++
		}
	}

	closed, closeErr := s.maintenance.CloseExpiredOrders(ctx, now)
	if closeErr != nil {
		failures = append(failures, fmt.Errorf("close expired orders: %w", closeErr))
	} else {
		result.ClosedOrders = closed
	}
	var afterAccountID int64
	for {
		accounts, listErr := s.maintenance.ListAccountSubjectsAfter(ctx, afterAccountID, 500)
		if listErr != nil {
			failures = append(failures, fmt.Errorf("list billing accounts for expiry: %w", listErr))
			break
		}
		for _, account := range accounts {
			afterAccountID = account.AccountID
			if _, balanceErr := s.ledger.GetBalance(ctx, domainbilling.Subject{
				Type: domainbilling.SubjectType(account.SubjectType),
				ID:   account.SubjectID,
			}); balanceErr != nil {
				failures = append(failures, fmt.Errorf("expire account %d credits: %w", account.AccountID, balanceErr))
			}
		}
		if len(accounts) < 500 {
			break
		}
	}
	result.Reconciliation, err = s.maintenance.ReconcileAccounts(ctx, now)
	if err != nil {
		failures = append(failures, fmt.Errorf("reconcile accounts: %w", err))
	}
	for _, failure := range failures {
		result.Errors = append(result.Errors, failure.Error())
	}
	return result, errors.Join(failures...)
}
