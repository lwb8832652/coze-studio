// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"time"
)

// Repository exposes transaction-scoped primitives. Implementations must lock
// accounts, batches, and reservations when a method name ends in ForUpdate.
type Repository interface {
	RunInTransaction(ctx context.Context, fn func(Repository) error) error

	GetOrCreateAccountForUpdate(ctx context.Context, subject Subject, now time.Time) (*Account, error)
	GetAccountByIDForUpdate(ctx context.Context, accountID int64) (*Account, error)
	UpdateAccount(ctx context.Context, account *Account, expectedVersion int64) error

	CreateBatch(ctx context.Context, batch *CreditBatch) error
	GetBatchByIDForUpdate(ctx context.Context, batchID int64) (*CreditBatch, error)
	ListSpendableBatchesForUpdate(ctx context.Context, accountID int64, now time.Time) ([]*CreditBatch, error)
	ListExpiredBatchesForUpdate(ctx context.Context, accountID int64, now time.Time) ([]*CreditBatch, error)
	UpdateBatch(ctx context.Context, batch *CreditBatch, expectedVersion int64) error

	FindLedgerByBusinessNo(ctx context.Context, businessNo string) (*LedgerEntry, error)
	CreateLedger(ctx context.Context, entry *LedgerEntry) error

	FindReservationByReserveBusinessNoForUpdate(ctx context.Context, businessNo string) (*Reservation, error)
	CreateReservation(ctx context.Context, reservation *Reservation) error
	UpdateReservation(ctx context.Context, reservation *Reservation, expectedVersion int64) error

	// ApplyCreditThreshold persists the active episode and appends any low
	// credit event in this repository's current ledger transaction.
	ApplyCreditThreshold(ctx context.Context, evaluation CreditThresholdEvaluation) error
}
