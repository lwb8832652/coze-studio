// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) Grant(ctx context.Context, input GrantInput) (*GrantResult, error) {
	if err := input.Subject.Validate(); err != nil || input.AmountMicros <= 0 {
		return nil, ErrInvalidInput
	}
	businessNo, err := normalizeBusinessNo(input.BusinessNo)
	if err != nil {
		return nil, err
	}
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" || len(sourceType) > 32 || len(input.SourceID) > 128 || input.ActorUserID < 0 {
		return nil, ErrInvalidInput
	}
	metadataJSON, err := normalizeMetadataJSON(input.MetadataJSON)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
		return nil, ErrInvalidInput
	}

	var result *GrantResult
	err = s.repository.RunInTransaction(ctx, func(repository Repository) error {
		account, txErr := repository.GetOrCreateAccountForUpdate(ctx, input.Subject, now)
		if txErr != nil {
			return txErr
		}
		if txErr = s.expireBatches(ctx, repository, account, now); txErr != nil {
			return txErr
		}
		existing, txErr := repository.FindLedgerByBusinessNo(ctx, businessNo)
		if txErr == nil {
			if existing.AccountID != account.ID || existing.Direction != LedgerDirectionCredit || existing.AmountMicros != input.AmountMicros {
				return ErrIdempotencyConflict
			}
			result = &GrantResult{Balance: balanceFromAccount(account), Entry: existing}
			return nil
		}
		if !errors.Is(txErr, ErrNotFound) {
			return txErr
		}

		available, txErr := checkedAdd(account.AvailableMicros, input.AmountMicros)
		if txErr != nil {
			return txErr
		}
		batch := &CreditBatch{
			AccountID:       account.ID,
			SourceType:      sourceType,
			SourceID:        strings.TrimSpace(input.SourceID),
			GrantBusinessNo: businessNo,
			GrantedMicros:   input.AmountMicros,
			RemainingMicros: input.AmountMicros,
			ExpiresAt:       cloneTime(input.ExpiresAt),
			Version:         InitialVersion,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if txErr = repository.CreateBatch(ctx, batch); txErr != nil {
			return txErr
		}
		expectedVersion := account.Version
		account.AvailableMicros = available
		account.UpdatedAt = now
		if txErr = repository.UpdateAccount(ctx, account, expectedVersion); txErr != nil {
			return txErr
		}
		batchID := batch.ID
		entry := &LedgerEntry{
			AccountID:            account.ID,
			BatchID:              &batchID,
			Direction:            LedgerDirectionCredit,
			Type:                 LedgerEntryTypeGrant,
			AmountMicros:         input.AmountMicros,
			AvailableAfterMicros: account.AvailableMicros,
			ReservedAfterMicros:  account.ReservedMicros,
			BusinessNo:           businessNo,
			ActorUserID:          input.ActorUserID,
			MetadataJSON:         metadataJSON,
			CreatedAt:            now,
		}
		if txErr = repository.CreateLedger(ctx, entry); txErr != nil {
			return txErr
		}
		result = &GrantResult{Balance: balanceFromAccount(account), Batch: batch, Entry: entry}
		return nil
	})
	return result, err
}

func (s *Service) Reserve(ctx context.Context, input ReserveInput) (*ReservationResult, error) {
	if err := input.Subject.Validate(); err != nil || input.AmountMicros <= 0 {
		return nil, ErrInvalidInput
	}
	businessNo, err := normalizeBusinessNo(input.BusinessNo)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() {
		expiresAt = now.Add(DefaultReservationTTL)
	}
	if !expiresAt.After(now) {
		return nil, ErrInvalidInput
	}

	var result *ReservationResult
	err = s.repository.RunInTransaction(ctx, func(repository Repository) error {
		account, txErr := repository.GetOrCreateAccountForUpdate(ctx, input.Subject, now)
		if txErr != nil {
			return txErr
		}
		if txErr = s.expireBatches(ctx, repository, account, now); txErr != nil {
			return txErr
		}
		existing, txErr := repository.FindReservationByReserveBusinessNoForUpdate(ctx, businessNo)
		if txErr == nil {
			if existing.AccountID != account.ID || existing.ReservedMicros != input.AmountMicros {
				return ErrIdempotencyConflict
			}
			result = &ReservationResult{Balance: balanceFromAccount(account), Reservation: existing}
			return nil
		}
		if !errors.Is(txErr, ErrNotFound) {
			return txErr
		}
		if account.AvailableMicros < input.AmountMicros {
			return ErrInsufficientCredits
		}
		batches, txErr := repository.ListSpendableBatchesForUpdate(ctx, account.ID, now)
		if txErr != nil {
			return txErr
		}
		remaining := input.AmountMicros
		allocations := make([]Allocation, 0, len(batches))
		for _, batch := range batches {
			if remaining == 0 {
				break
			}
			amount := minInt64(batch.RemainingMicros, remaining)
			if amount <= 0 {
				continue
			}
			expectedVersion := batch.Version
			batch.RemainingMicros -= amount
			batch.UpdatedAt = now
			if txErr = repository.UpdateBatch(ctx, batch, expectedVersion); txErr != nil {
				return txErr
			}
			allocations = append(allocations, Allocation{BatchID: batch.ID, Micros: amount})
			remaining -= amount
			if batch.ExpiresAt != nil && batch.ExpiresAt.Before(expiresAt) {
				expiresAt = batch.ExpiresAt.UTC()
			}
		}
		if remaining != 0 {
			return ErrInsufficientCredits
		}
		expectedVersion := account.Version
		account.AvailableMicros -= input.AmountMicros
		reserved, txErr := checkedAdd(account.ReservedMicros, input.AmountMicros)
		if txErr != nil {
			return txErr
		}
		account.ReservedMicros = reserved
		account.UpdatedAt = now
		if txErr = repository.UpdateAccount(ctx, account, expectedVersion); txErr != nil {
			return txErr
		}
		reservation := &Reservation{
			AccountID:         account.ID,
			ReserveBusinessNo: businessNo,
			ReservedMicros:    input.AmountMicros,
			Status:            ReservationStatusReserved,
			Allocations:       allocations,
			ExpiresAt:         expiresAt,
			Version:           InitialVersion,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if txErr = repository.CreateReservation(ctx, reservation); txErr != nil {
			return txErr
		}
		result = &ReservationResult{Balance: balanceFromAccount(account), Reservation: reservation}
		return nil
	})
	return result, err
}

func (s *Service) Settle(ctx context.Context, input SettleInput) (*ReservationResult, error) {
	reserveBusinessNo, err := normalizeBusinessNo(input.ReservationBusinessNo)
	if err != nil || input.ActualMicros < 0 || input.ActorUserID < 0 {
		return nil, ErrInvalidInput
	}
	businessNo, err := normalizeBusinessNo(input.BusinessNo)
	if err != nil {
		return nil, err
	}
	metadataJSON, err := normalizeMetadataJSON(input.MetadataJSON)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()

	var result *ReservationResult
	err = s.repository.RunInTransaction(ctx, func(repository Repository) error {
		reservation, txErr := repository.FindReservationByReserveBusinessNoForUpdate(ctx, reserveBusinessNo)
		if txErr != nil {
			return txErr
		}
		account, txErr := repository.GetAccountByIDForUpdate(ctx, reservation.AccountID)
		if txErr != nil {
			return txErr
		}
		if txErr = s.expireBatches(ctx, repository, account, now); txErr != nil {
			return txErr
		}
		if reservation.Status == ReservationStatusSettled {
			if reservation.SettlementBusinessNo != businessNo || reservation.SettledMicros != input.ActualMicros {
				return ErrIdempotencyConflict
			}
			entry, findErr := repository.FindLedgerByBusinessNo(ctx, businessNo)
			if findErr != nil && !errors.Is(findErr, ErrNotFound) {
				return findErr
			}
			result = &ReservationResult{Balance: balanceFromAccount(account), Reservation: reservation, Entry: entry}
			return nil
		}
		if reservation.Status != ReservationStatusReserved || input.ActualMicros > reservation.ReservedMicros {
			return ErrReservationNotActive
		}

		remainingConsumption := input.ActualMicros
		refundable := int64(0)
		expiredRefund := int64(0)
		for _, allocation := range reservation.Allocations {
			consumed := minInt64(allocation.Micros, remainingConsumption)
			remainingConsumption -= consumed
			refund := allocation.Micros - consumed
			if refund == 0 {
				continue
			}
			batch, batchErr := repository.GetBatchByIDForUpdate(ctx, allocation.BatchID)
			if batchErr != nil {
				return batchErr
			}
			expectedVersion := batch.Version
			batch.RemainingMicros, batchErr = checkedAdd(batch.RemainingMicros, refund)
			if batchErr != nil || batch.RemainingMicros > batch.GrantedMicros {
				return ErrVersionConflict
			}
			batch.UpdatedAt = now
			if batchErr = repository.UpdateBatch(ctx, batch, expectedVersion); batchErr != nil {
				return batchErr
			}
			if batch.ExpiresAt != nil && !batch.ExpiresAt.After(now) {
				expiredRefund += refund
			} else {
				refundable += refund
			}
		}
		if remainingConsumption != 0 || account.ReservedMicros < reservation.ReservedMicros {
			return ErrVersionConflict
		}
		expectedAccountVersion := account.Version
		account.ReservedMicros -= reservation.ReservedMicros
		account.AvailableMicros, txErr = checkedAdd(account.AvailableMicros, refundable)
		if txErr != nil {
			return txErr
		}
		account.UpdatedAt = now
		if txErr = repository.UpdateAccount(ctx, account, expectedAccountVersion); txErr != nil {
			return txErr
		}

		expectedReservationVersion := reservation.Version
		reservation.Status = ReservationStatusSettled
		reservation.SettlementBusinessNo = businessNo
		reservation.SettledMicros = input.ActualMicros
		reservation.ReleasedMicros = reservation.ReservedMicros - input.ActualMicros
		reservation.UpdatedAt = now
		if txErr = repository.UpdateReservation(ctx, reservation, expectedReservationVersion); txErr != nil {
			return txErr
		}

		var entry *LedgerEntry
		if input.ActualMicros > 0 {
			entry = &LedgerEntry{
				AccountID:            account.ID,
				Direction:            LedgerDirectionDebit,
				Type:                 LedgerEntryTypeConsume,
				AmountMicros:         input.ActualMicros,
				AvailableAfterMicros: account.AvailableMicros,
				ReservedAfterMicros:  account.ReservedMicros,
				BusinessNo:           businessNo,
				ActorUserID:          input.ActorUserID,
				MetadataJSON:         metadataJSON,
				CreatedAt:            now,
			}
			if txErr = repository.CreateLedger(ctx, entry); txErr != nil {
				return txErr
			}
		}
		if expiredRefund > 0 {
			expiryEntry := &LedgerEntry{
				AccountID:            account.ID,
				Direction:            LedgerDirectionDebit,
				Type:                 LedgerEntryTypeExpiration,
				AmountMicros:         expiredRefund,
				AvailableAfterMicros: account.AvailableMicros,
				ReservedAfterMicros:  account.ReservedMicros,
				BusinessNo:           fmt.Sprintf("settle-expiry:%d", reservation.ID),
				MetadataJSON:         "{}",
				CreatedAt:            now,
			}
			if txErr = repository.CreateLedger(ctx, expiryEntry); txErr != nil {
				return txErr
			}
		}
		result = &ReservationResult{Balance: balanceFromAccount(account), Reservation: reservation, Entry: entry}
		return nil
	})
	return result, err
}

func (s *Service) Release(ctx context.Context, input ReleaseInput) (*ReservationResult, error) {
	reserveBusinessNo, err := normalizeBusinessNo(input.ReservationBusinessNo)
	if err != nil {
		return nil, err
	}
	businessNo, err := normalizeBusinessNo(input.BusinessNo)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	var result *ReservationResult
	err = s.repository.RunInTransaction(ctx, func(repository Repository) error {
		reservation, txErr := repository.FindReservationByReserveBusinessNoForUpdate(ctx, reserveBusinessNo)
		if txErr != nil {
			return txErr
		}
		account, txErr := repository.GetAccountByIDForUpdate(ctx, reservation.AccountID)
		if txErr != nil {
			return txErr
		}
		if txErr = s.expireBatches(ctx, repository, account, now); txErr != nil {
			return txErr
		}
		if reservation.Status == ReservationStatusReleased || reservation.Status == ReservationStatusExpired {
			if reservation.ReleaseBusinessNo != businessNo {
				return ErrIdempotencyConflict
			}
			result = &ReservationResult{Balance: balanceFromAccount(account), Reservation: reservation}
			return nil
		}
		if reservation.Status != ReservationStatusReserved {
			return ErrReservationNotActive
		}
		refundable := int64(0)
		expiredRefund := int64(0)
		for _, allocation := range reservation.Allocations {
			batch, batchErr := repository.GetBatchByIDForUpdate(ctx, allocation.BatchID)
			if batchErr != nil {
				return batchErr
			}
			expectedVersion := batch.Version
			batch.RemainingMicros, batchErr = checkedAdd(batch.RemainingMicros, allocation.Micros)
			if batchErr != nil || batch.RemainingMicros > batch.GrantedMicros {
				return ErrVersionConflict
			}
			batch.UpdatedAt = now
			if batchErr = repository.UpdateBatch(ctx, batch, expectedVersion); batchErr != nil {
				return batchErr
			}
			if batch.ExpiresAt != nil && !batch.ExpiresAt.After(now) {
				expiredRefund += allocation.Micros
			} else {
				refundable += allocation.Micros
			}
		}
		if account.ReservedMicros < reservation.ReservedMicros {
			return ErrVersionConflict
		}
		expectedAccountVersion := account.Version
		account.ReservedMicros -= reservation.ReservedMicros
		account.AvailableMicros, txErr = checkedAdd(account.AvailableMicros, refundable)
		if txErr != nil {
			return txErr
		}
		account.UpdatedAt = now
		if txErr = repository.UpdateAccount(ctx, account, expectedAccountVersion); txErr != nil {
			return txErr
		}
		expectedReservationVersion := reservation.Version
		reservation.Status = ReservationStatusReleased
		if !reservation.ExpiresAt.After(now) {
			reservation.Status = ReservationStatusExpired
		}
		reservation.ReleaseBusinessNo = businessNo
		reservation.ReleasedMicros = reservation.ReservedMicros
		reservation.UpdatedAt = now
		if txErr = repository.UpdateReservation(ctx, reservation, expectedReservationVersion); txErr != nil {
			return txErr
		}
		if expiredRefund > 0 {
			entry := &LedgerEntry{
				AccountID:            account.ID,
				Direction:            LedgerDirectionDebit,
				Type:                 LedgerEntryTypeExpiration,
				AmountMicros:         expiredRefund,
				AvailableAfterMicros: account.AvailableMicros,
				ReservedAfterMicros:  account.ReservedMicros,
				BusinessNo:           fmt.Sprintf("release-expiry:%d", reservation.ID),
				MetadataJSON:         "{}",
				CreatedAt:            now,
			}
			if txErr = repository.CreateLedger(ctx, entry); txErr != nil {
				return txErr
			}
		}
		result = &ReservationResult{Balance: balanceFromAccount(account), Reservation: reservation}
		return nil
	})
	return result, err
}

func (s *Service) GetBalance(ctx context.Context, subject Subject) (*Balance, error) {
	if err := subject.Validate(); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	var result *Balance
	err := s.repository.RunInTransaction(ctx, func(repository Repository) error {
		account, txErr := repository.GetOrCreateAccountForUpdate(ctx, subject, now)
		if txErr != nil {
			return txErr
		}
		if txErr = s.expireBatches(ctx, repository, account, now); txErr != nil {
			return txErr
		}
		balance := balanceFromAccount(account)
		result = &balance
		return nil
	})
	return result, err
}

func (s *Service) expireBatches(ctx context.Context, repository Repository, account *Account, now time.Time) error {
	batches, err := repository.ListExpiredBatchesForUpdate(ctx, account.ID, now)
	if err != nil {
		return err
	}
	for _, batch := range batches {
		amount := batch.RemainingMicros
		if amount <= 0 {
			continue
		}
		if account.AvailableMicros < amount {
			return ErrVersionConflict
		}
		expectedBatchVersion := batch.Version
		batch.RemainingMicros = 0
		batch.UpdatedAt = now
		if err = repository.UpdateBatch(ctx, batch, expectedBatchVersion); err != nil {
			return err
		}
		expectedAccountVersion := account.Version
		account.AvailableMicros -= amount
		account.UpdatedAt = now
		if err = repository.UpdateAccount(ctx, account, expectedAccountVersion); err != nil {
			return err
		}
		batchID := batch.ID
		entry := &LedgerEntry{
			AccountID:            account.ID,
			BatchID:              &batchID,
			Direction:            LedgerDirectionDebit,
			Type:                 LedgerEntryTypeExpiration,
			AmountMicros:         amount,
			AvailableAfterMicros: account.AvailableMicros,
			ReservedAfterMicros:  account.ReservedMicros,
			BusinessNo:           fmt.Sprintf("expiry:%d", batch.ID),
			MetadataJSON:         "{}",
			CreatedAt:            now,
		}
		if err = repository.CreateLedger(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func checkedAdd(base, delta int64) (int64, error) {
	if delta > 0 && base > math.MaxInt64-delta {
		return 0, ErrInvalidInput
	}
	if delta < 0 && base < math.MinInt64-delta {
		return 0, ErrInvalidInput
	}
	return base + delta, nil
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
