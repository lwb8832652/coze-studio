/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package repository

import (
	"context"
	"errors"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrJournalSnapshotNotFound           = errors.New("journal snapshot not found")
	ErrJournalSnapshotConflict           = errors.New("journal snapshot identity conflict")
	ErrJournalProjectionInactive         = errors.New("journal projection is not active")
	ErrJournalSnapshotReservationExpired = errors.New("journal snapshot reservation expired")
	ErrJournalCursorExpired              = errors.New("journal cursor expired")
	ErrJournalEventGap                   = errors.New("journal event gap")
	ErrSideEffectLedgerNotFound          = errors.New("side effect ledger not found")
	ErrSideEffectLedgerConflict          = errors.New("side effect ledger conflict")
	ErrSideEffectTransitionInvalid       = errors.New("invalid side effect ledger transition")
)

type JournalRepository interface {
	CreateJournalAttempt(ctx context.Context, attempt *entity.RunAttempt) (*entity.RunAttempt, error)
	GetActiveJournalAttempt(ctx context.Context, runID int64) (*entity.RunAttempt, error)
	ListJournalAttempts(ctx context.Context, runID int64) ([]*entity.RunAttempt, error)
	AppendJournalEvent(ctx context.Context, event *entity.JournalEvent) (*entity.JournalEvent, error)
	FinalizeJournalAttempt(
		ctx context.Context,
		req FinalizeJournalAttemptRequest,
	) (*entity.JournalEvent, bool, error)
	GetJournalEvent(ctx context.Context, eventID int64) (*entity.JournalEvent, error)
	ListJournalEvents(ctx context.Context, req ListJournalEventsRequest) (*ListJournalEventsResult, error)
	GetJournalBootstrap(
		ctx context.Context,
		req GetJournalBootstrapRequest,
	) (*GetJournalBootstrapResult, error)
	JournalSnapshotRepository
}

type JournalSnapshotRepository interface {
	ReserveJournalSnapshot(
		ctx context.Context,
		req ReserveJournalSnapshotRequest,
	) (*ReserveJournalSnapshotResult, error)
	DeleteExpiredJournalSnapshotReservations(
		ctx context.Context,
		spaceID int64,
		now int64,
		limit int,
	) (int64, error)
	CreateJournalSnapshot(
		ctx context.Context,
		req CreateJournalSnapshotRequest,
	) (*entity.JournalContentSnapshot, *entity.JournalEvent, bool, error)
	GetJournalSnapshot(
		ctx context.Context,
		req GetJournalSnapshotRequest,
	) (*entity.JournalContentSnapshot, error)
	ListJournalSnapshotFragments(
		ctx context.Context,
		req ListJournalSnapshotFragmentsRequest,
	) (*ListJournalSnapshotFragmentsResult, error)
	RecordJournalSnapshotAccess(
		ctx context.Context,
		audit *entity.JournalSnapshotAccessAudit,
	) (*entity.JournalSnapshotAccessAudit, bool, error)
	IsJournalSnapshotObjectProtected(
		ctx context.Context,
		spaceID int64,
		objectKey string,
		now int64,
	) (bool, error)
}

// JournalExecutionRepository owns the durable write-ahead log and the one
// transaction that commits a tool result, its public event, and its runtime
// checkpoint. Keeping this separate from JournalRepository preserves the
// compatibility contract for repositories that only project Journal events.
type JournalExecutionRepository interface {
	PrepareSideEffect(
		ctx context.Context,
		req PrepareSideEffectRequest,
	) (*entity.SideEffectLedger, bool, error)
	TransitionSideEffect(
		ctx context.Context,
		req TransitionSideEffectRequest,
	) (*entity.SideEffectLedger, bool, error)
	ResolveUnknownSideEffect(
		ctx context.Context,
		req ResolveUnknownSideEffectRequest,
	) (*entity.SideEffectLedger, bool, error)
	CommitExecutionBoundary(
		ctx context.Context,
		req CommitExecutionBoundaryRequest,
	) (*CommitExecutionBoundaryResult, error)
	GetSideEffectLedger(
		ctx context.Context,
		journalRunID int64,
		attemptID, idempotencyKey string,
	) (*entity.SideEffectLedger, error)
	ListSideEffectLedgers(
		ctx context.Context,
		journalRunID int64,
		attemptID string,
	) ([]*entity.SideEffectLedger, error)
}

// RunEventProjectionRepository atomically preserves the existing RunEvent view
// and, when the run is enrolled, enriches the same row with a public Journal
// projection.
type RunEventProjectionRepository interface {
	CreateRunEventWithJournalProjection(
		ctx context.Context,
		req CreateRunEventWithJournalProjectionRequest,
	) (*entity.JournalEvent, error)
}

type CreateRunEventWithJournalProjectionRequest struct {
	Event            *entity.RunEvent
	Journal          *entity.JournalEvent
	ProjectionFailed bool
}

type Repository interface {
	ThreadRepository
	JournalRepository
	JournalExecutionRepository
}

type PrepareSideEffectRequest struct {
	Ledger     *entity.SideEffectLedger
	AuditEvent *entity.JournalEvent
}

type TransitionSideEffectRequest struct {
	JournalRunID            int64
	AttemptID               string
	LedgerID                int64
	ExpectedVersion         uint64
	FromStatus              entity.SideEffectLedgerStatus
	ToStatus                entity.SideEffectLedgerStatus
	OccurredAt              int64
	ExternalReferenceDigest string
	ResultSnapshotID        string
	CompensationRegistered  bool
	CompensationSucceeded   bool
	AuditEvent              *entity.JournalEvent
}

type ResolveUnknownSideEffectRequest struct {
	JournalRunID    int64
	AttemptID       string
	LedgerID        int64
	ExpectedVersion uint64
	Action          entity.SideEffectResolutionAction
	IdempotencyKey  string
	OccurredAt      int64
	AuditEvent      *entity.JournalEvent
}

type CommitExecutionBoundaryRequest struct {
	JournalRunID            int64
	AttemptID               string
	LedgerID                int64
	ExpectedVersion         uint64
	Status                  entity.SideEffectLedgerStatus
	ExternalReferenceDigest string
	ResultSnapshotID        string
	ResultEvent             *entity.JournalEvent
	AuditEvent              *entity.JournalEvent
	CheckpointFactory       func(
		lastCommittedSequence uint64,
		ledgers []*entity.SideEffectLedger,
	) (*entity.Checkpoint, error)
}

type CommitExecutionBoundaryResult struct {
	Ledger                *entity.SideEffectLedger
	Event                 *entity.JournalEvent
	Checkpoint            *entity.Checkpoint
	LastCommittedSequence uint64
	Replayed              bool
}

type FinalizeJournalAttemptRequest struct {
	RunID   int64
	Status  entity.RunAttemptStatus
	Event   *entity.JournalEvent
	EndedAt int64
}

type ListJournalEventsRequest struct {
	RunID         int64
	AttemptID     string
	AfterSequence uint64
	AfterEventID  int64
	Limit         int
}

type ListJournalEventsResult struct {
	Events  []*entity.JournalEvent
	HasMore bool
	Legacy  bool
}

type GetJournalBootstrapRequest struct {
	RunID            int64
	AttemptID        string
	AfterSequence    uint64
	AfterSequenceSet bool
	AfterEventID     int64
	Limit            int
}

type GetJournalBootstrapResult struct {
	Attempts              []*entity.RunAttempt
	SelectedAttempt       *entity.RunAttempt
	Events                []*entity.JournalEvent
	LatestSequence        uint64
	ResolvedAfterSequence uint64
	HasMore               bool
}

type CreateJournalSnapshotRequest struct {
	Snapshot         *entity.JournalContentSnapshot
	Fragments        []*entity.JournalSnapshotFragment
	Event            *entity.JournalEvent
	ReservationToken string
}

type ReserveJournalSnapshotRequest struct {
	Reservation *entity.JournalSnapshotReservation
	Now         int64
}

type ReserveJournalSnapshotResult struct {
	Reservation *entity.JournalSnapshotReservation
	Snapshot    *entity.JournalContentSnapshot
	Event       *entity.JournalEvent
	Replayed    bool
}

type GetJournalSnapshotRequest struct {
	SpaceID    int64
	ThreadID   int64
	RunID      int64
	SnapshotID string
}

type ListJournalSnapshotFragmentsRequest struct {
	SpaceID    int64
	SnapshotID string
	AfterIndex int32
	Limit      int
}

type ListJournalSnapshotFragmentsResult struct {
	Fragments []*entity.JournalSnapshotFragment
	HasMore   bool
	NextIndex int32
}
