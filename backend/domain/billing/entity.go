// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	InitialVersion       int64 = 1
	DefaultReservationTTL      = 15 * time.Minute
	maxBusinessNoLength         = 128
	maxMetadataJSONBytes        = 16 * 1024
)

var (
	ErrInvalidInput        = errors.New("billing: invalid input")
	ErrNotFound            = errors.New("billing: not found")
	ErrInsufficientCredits = errors.New("billing: insufficient credits")
	ErrIdempotencyConflict = errors.New("billing: idempotency conflict")
	ErrVersionConflict     = errors.New("billing: version conflict")
	ErrReservationNotActive = errors.New("billing: reservation is not active")
)

type SubjectType string

const (
	SubjectTypeUser      SubjectType = "user"
	SubjectTypeWorkspace SubjectType = "workspace"
)

type Subject struct {
	Type SubjectType
	ID   int64
}

func (s Subject) Validate() error {
	if s.ID <= 0 || (s.Type != SubjectTypeUser && s.Type != SubjectTypeWorkspace) {
		return ErrInvalidInput
	}
	return nil
}

type Account struct {
	ID              int64
	Subject         Subject
	AvailableMicros int64
	ReservedMicros  int64
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreditBatch struct {
	ID              int64
	AccountID       int64
	SourceType      string
	SourceID        string
	GrantBusinessNo string
	GrantedMicros   int64
	RemainingMicros int64
	ExpiresAt       *time.Time
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Allocation struct {
	BatchID int64 `json:"batch_id"`
	Micros  int64 `json:"micros"`
}

type LedgerDirection string

const (
	LedgerDirectionCredit LedgerDirection = "credit"
	LedgerDirectionDebit  LedgerDirection = "debit"
)

type LedgerEntryType string

const (
	LedgerEntryTypeGrant      LedgerEntryType = "grant"
	LedgerEntryTypeConsume    LedgerEntryType = "consume"
	LedgerEntryTypeExpiration LedgerEntryType = "expiration"
	LedgerEntryTypeAdjustment LedgerEntryType = "adjustment"
)

type LedgerEntry struct {
	ID                  int64
	AccountID           int64
	BatchID             *int64
	Direction           LedgerDirection
	Type                LedgerEntryType
	AmountMicros        int64
	AvailableAfterMicros int64
	ReservedAfterMicros int64
	BusinessNo          string
	ActorUserID         int64
	MetadataJSON        string
	CreatedAt           time.Time
}

type ReservationStatus string

const (
	ReservationStatusReserved ReservationStatus = "reserved"
	ReservationStatusSettled  ReservationStatus = "settled"
	ReservationStatusReleased ReservationStatus = "released"
	ReservationStatusExpired  ReservationStatus = "expired"
)

type Reservation struct {
	ID                   int64
	AccountID            int64
	ReserveBusinessNo    string
	SettlementBusinessNo string
	ReleaseBusinessNo    string
	ReservedMicros       int64
	SettledMicros        int64
	ReleasedMicros       int64
	Status               ReservationStatus
	Allocations          []Allocation
	ExpiresAt            time.Time
	Version              int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Balance struct {
	AccountID       int64
	Subject         Subject
	AvailableMicros int64
	ReservedMicros  int64
	Version         int64
}

type GrantInput struct {
	Subject       Subject
	AmountMicros  int64
	BusinessNo    string
	SourceType    string
	SourceID      string
	ExpiresAt     *time.Time
	ActorUserID   int64
	MetadataJSON  string
}

type ReserveInput struct {
	Subject      Subject
	AmountMicros int64
	BusinessNo   string
	ExpiresAt    time.Time
}

type SettleInput struct {
	ReservationBusinessNo string
	BusinessNo            string
	ActualMicros           int64
	ActorUserID            int64
	MetadataJSON           string
}

type ReleaseInput struct {
	ReservationBusinessNo string
	BusinessNo            string
}

type GrantResult struct {
	Balance Balance
	Batch   *CreditBatch
	Entry   *LedgerEntry
}

type ReservationResult struct {
	Balance     Balance
	Reservation *Reservation
	Entry       *LedgerEntry
}

func normalizeBusinessNo(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxBusinessNoLength {
		return "", ErrInvalidInput
	}
	return value, nil
}

func normalizeMetadataJSON(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}", nil
	}
	if len(value) > maxMetadataJSONBytes || !json.Valid([]byte(value)) {
		return "", ErrInvalidInput
	}
	return value, nil
}

func balanceFromAccount(account *Account) Balance {
	return Balance{
		AccountID:       account.ID,
		Subject:         account.Subject,
		AvailableMicros: account.AvailableMicros,
		ReservedMicros:  account.ReservedMicros,
		Version:         account.Version,
	}
}
