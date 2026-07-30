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
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"gorm.io/gorm"
)

var (
	ErrRunLeaseLost                        = errors.New("agent run lease lost")
	ErrRunCanceled                         = errors.New("agent run canceled")
	ErrRunIdempotencyConflict              = entity.ErrRunIdempotencyConflict
	ErrActiveRunExists                     = errors.New("agent thread already has an active run")
	ErrUnsupportedMultitaskStrategy        = errors.New("unsupported multitask strategy")
	ErrJournalNotEnrolled                  = errors.New("agent run is not enrolled in journal")
	ErrActiveJournalAttemptExists          = errors.New("agent run already has an active journal attempt")
	ErrJournalAttemptTerminal              = errors.New("agent run journal attempt is terminal")
	ErrJournalParentMismatch               = errors.New("journal parent event belongs to another attempt")
	ErrJournalActionDrift                  = errors.New("journal action fields changed across phases")
	ErrJournalTerminalReplayConflict       = errors.New("journal terminal event replays a non-terminal event")
	ErrJournalUnsafeLegacyEvent            = errors.New("legacy run event is not safe for journal projection")
	ErrJournalInvalidStateTransition       = errors.New("invalid journal attempt state transition")
	ErrUnsupportedJournalEnrollmentVersion = errors.New("unsupported journal enrollment version")
)

type ThreadRepository interface {
	CreateThread(ctx context.Context, thread *entity.Thread) error
	CreateThreadBundle(ctx context.Context, req CreateThreadBundleRequest) (*CreateThreadBundleResult, error)
	GetThread(ctx context.Context, id int64) (*entity.Thread, error)
	UpdateThreadTitle(ctx context.Context, req UpdateThreadTitleRequest) (*entity.Thread, bool, error)
	UpdateThreadMetadata(ctx context.Context, req UpdateThreadMetadataRequest) (*entity.Thread, bool, error)
	DeleteThread(ctx context.Context, req DeleteThreadRequest) (bool, error)
	ListThreads(ctx context.Context, req ListThreadsRequest) ([]*entity.Thread, int64, error)
	CreateMessage(ctx context.Context, message *entity.Message) error
	ListMessages(ctx context.Context, req ListMessagesRequest) ([]*entity.Message, int64, error)
	ListRecentMessagesByRoles(ctx context.Context, req ListRecentMessagesByRolesRequest) ([]*entity.Message, error)
	CreateRun(ctx context.Context, run *entity.Run) error
	CreateRunBundle(ctx context.Context, req CreateRunBundleRequest) (*CreateRunBundleResult, error)
	GetRun(ctx context.Context, id int64) (*entity.Run, error)
	GetRunByIdempotencyKey(ctx context.Context, spaceID int64, idempotencyKey string) (*entity.Run, error)
	ListRuns(ctx context.Context, req ListRunsRequest) ([]*entity.Run, int64, error)
	AggregateRunBacklog(ctx context.Context, req AggregateRunBacklogRequest) ([]*entity.RunBacklogAggregate, error)
	ClaimPendingRuns(ctx context.Context, req ClaimPendingRunsRequest) ([]*entity.Run, error)
	ClaimQueuedResumeRuns(ctx context.Context, req ClaimQueuedResumeRunsRequest) ([]*entity.Run, error)
	RenewRunLease(ctx context.Context, req RenewRunLeaseRequest) (*entity.Run, error)
	ReleaseRunLease(ctx context.Context, req ReleaseRunLeaseRequest) (*entity.Run, error)
	ListExpiredRunLeases(ctx context.Context, req ListExpiredRunLeasesRequest) ([]*entity.Run, error)
	ReconcileExpiredRunLease(ctx context.Context, req ReconcileExpiredRunLeaseRequest) (*entity.Run, error)
	RequestRunCancellation(ctx context.Context, req RequestRunCancellationRequest) (*RequestRunCancellationResult, error)
	FinalizeRunSuccess(ctx context.Context, req FinalizeRunSuccessRequest) (*FinalizeRunSuccessResult, error)
	UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) error
	CreateRunEvent(ctx context.Context, event *entity.RunEvent) error
	ListRunEvents(ctx context.Context, req ListRunEventsRequest) ([]*entity.RunEvent, int64, error)
	CreateCheckpoint(ctx context.Context, checkpoint *entity.Checkpoint) error
	GetCheckpoint(ctx context.Context, checkpointID int64) (*entity.Checkpoint, error)
	ListCheckpoints(ctx context.Context, req ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error)
	GetLatestCheckpoint(ctx context.Context, threadID int64) (*entity.Checkpoint, error)
	GetLatestRuntimeCheckpoint(
		ctx context.Context,
		threadID, runID int64,
		runtimeType, runtimeKey string,
	) (*entity.Checkpoint, error)
	DeleteRuntimeCheckpoint(
		ctx context.Context,
		threadID, runID int64,
		runtimeType, runtimeKey string,
		deletedAt int64,
	) error
	CreateMemory(ctx context.Context, memory *entity.Memory) error
	CreateOrGetMemoryBySource(ctx context.Context, memory *entity.Memory) (*entity.Memory, bool, error)
	ListMemories(ctx context.Context, req ListMemoriesRequest) ([]*entity.Memory, int64, error)
	UpdateMemory(ctx context.Context, req UpdateMemoryRequest) (*entity.Memory, bool, error)
	DeleteMemory(ctx context.Context, req DeleteMemoryRequest) (bool, error)
	RestoreMemory(ctx context.Context, req RestoreMemoryRequest) (*entity.Memory, bool, error)
	ClearMemories(ctx context.Context, req ClearMemoriesRequest) (int64, error)
	CreateMemoryAuditEvent(ctx context.Context, event *entity.MemoryAuditEvent) error
	ListMemoryAuditEvents(ctx context.Context, req ListMemoryAuditEventsRequest) ([]*entity.MemoryAuditEvent, int64, error)
	CreateOrGetTranscriptSnapshot(
		ctx context.Context,
		snapshot *entity.TranscriptSnapshot,
	) (*entity.TranscriptSnapshot, bool, error)
	GetTranscriptSnapshot(
		ctx context.Context,
		snapshotID int64,
	) (*entity.TranscriptSnapshot, error)
	CreateOrGetMemoryFlushJob(
		ctx context.Context,
		job *entity.MemoryFlushJob,
	) (*entity.MemoryFlushJob, bool, error)
	ClaimMemoryFlushJobs(ctx context.Context, req ClaimMemoryFlushJobsRequest) ([]*entity.MemoryFlushJob, error)
	AggregateMemoryFlushBacklog(ctx context.Context, req AggregateMemoryFlushBacklogRequest) ([]*entity.MemoryFlushBacklogAggregate, error)
	CompleteMemoryFlushJob(ctx context.Context, req CompleteMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	RetryMemoryFlushJob(ctx context.Context, req RetryMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	FailMemoryFlushJob(ctx context.Context, req FailMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	CreateTokenUsage(ctx context.Context, usage *entity.TokenUsage) error
	ListTokenUsage(ctx context.Context, req ListTokenUsageRequest) ([]*entity.TokenUsage, int64, error)
	AggregateTokenUsage(ctx context.Context, req AggregateTokenUsageRequest) (*entity.TokenUsageAggregate, error)
	AggregateTokenUsageByRun(ctx context.Context, req AggregateTokenUsageRequest) ([]*entity.RunTokenUsageAggregate, error)
	GetTokenUsageSnapshot(ctx context.Context, req ListTokenUsageRequest, includeRunAggregates bool) (*TokenUsageSnapshot, error)
}

type CreateThreadBundleRequest struct {
	Thread                    *entity.Thread
	Run                       *entity.Run
	Message                   *entity.Message
	ValidateIdempotencyReplay bool
}

type CreateThreadBundleResult struct {
	Thread  *entity.Thread
	Run     *entity.Run
	Message *entity.Message
	Created bool
}

type CreateRunBundleRequest struct {
	Run                         *entity.Run
	Message                     *entity.Message
	Event                       *entity.RunEvent
	Attempt                     *entity.RunAttempt
	SkipTopLevelAdmission       bool
	ValidateIdempotencyReplay   bool
	AllocateInterruptedEventIDs func(count int) ([]int64, error)
}

type CreateRunBundleResult struct {
	Run               *entity.Run
	Message           *entity.Message
	Event             *entity.RunEvent
	Attempt           *entity.RunAttempt
	InterruptedRuns   []*entity.Run
	InterruptedEvents []*entity.RunEvent
	Created           bool
}

type ListThreadsRequest struct {
	SpaceID  int64
	UserID   int64
	Status   *entity.ThreadStatus
	Page     int32
	PageSize int32
}

type UpdateThreadTitleRequest struct {
	ThreadID  int64
	Title     string
	UpdatedAt int64
}

type UpdateThreadMetadataRequest struct {
	ThreadID  int64
	Metadata  string
	UpdatedAt int64
}

type DeleteThreadRequest struct {
	ThreadID int64
}

type ListMessagesRequest struct {
	ThreadID int64
	Page     int32
	PageSize int32
}

type ListRecentMessagesByRolesRequest struct {
	ThreadID int64
	Roles    []entity.MessageRole
	Limit    int32
}

type ListRunsRequest struct {
	ThreadID         int64
	ParentRunID      *int64
	IncludeChildRuns bool
	Status           *entity.RunStatus
	Page             int32
	PageSize         int32
}

type AggregateRunBacklogRequest struct {
	Statuses []entity.RunStatus
}

type ListRunEventsRequest struct {
	ThreadID     int64
	RunID        int64
	AfterEventID int64
	Page         int32
	PageSize     int32
}

type ListCheckpointsRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	Limit       int32
}

type ListMemoriesRequest struct {
	ThreadID       int64
	RunID          int64
	Scopes         []entity.MemoryScope
	Query          string
	Limit          int32
	Page           int32
	PageSize       int32
	Now            int64
	IncludeExpired bool
	IncludeDeleted bool
}

type UpdateMemoryRequest struct {
	ThreadID             int64
	MemoryID             int64
	RunID                int64
	Scope                entity.MemoryScope
	Content              string
	Metadata             string
	Score                float64
	Confidence           float64
	SourceType           string
	SourceID             string
	CorrectionOfMemoryID int64
	CorrectedAt          int64
	ExpiresAt            int64
	UpdatedAt            int64
}

type DeleteMemoryRequest struct {
	ThreadID  int64
	MemoryID  int64
	DeletedAt int64
}

type RestoreMemoryRequest struct {
	ThreadID   int64
	MemoryID   int64
	ActorID    int64
	RestoredAt int64
}

type ClearMemoriesRequest struct {
	ThreadID  int64
	RunID     int64
	Scopes    []entity.MemoryScope
	DeletedAt int64
}

type ListMemoryAuditEventsRequest struct {
	ThreadID int64
	MemoryID int64
	Page     int32
	PageSize int32
}

type ListTokenUsageRequest struct {
	ThreadID int64
	RunID    int64
	RunIDs   []int64
	Source   entity.TokenUsageSource
	Page     int32
	PageSize int32
}

type AggregateTokenUsageRequest struct {
	ThreadID int64
	RunID    int64
	RunIDs   []int64
	Source   entity.TokenUsageSource
}

type TokenUsageSnapshot struct {
	Rows          []*entity.TokenUsage
	Total         int64
	Aggregate     *entity.TokenUsageAggregate
	RunAggregates []*entity.RunTokenUsageAggregate
}

type NotificationOutboxIntent struct {
	Event  domainnotification.Event
	Append func(context.Context, *gorm.DB, domainnotification.Event) error
}

type ClaimMemoryFlushJobsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseExpiresAt int64
}

type AggregateMemoryFlushBacklogRequest struct {
	Statuses []entity.MemoryFlushJobStatus
}

type CompleteMemoryFlushJobRequest struct {
	JobID    int64
	WorkerID string
	Now      int64
}

type RetryMemoryFlushJobRequest struct {
	JobID       int64
	WorkerID    string
	ErrorText   string
	AvailableAt int64
	Now         int64
}

type FailMemoryFlushJobRequest struct {
	JobID     int64
	WorkerID  string
	ErrorText string
	Now       int64
}

type ClaimPendingRunsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseTTLMillis int64
}

type ClaimQueuedResumeRunsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseTTLMillis int64
}

type RenewRunLeaseRequest struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	Now                 int64
	LeaseTTLMillis      int64
}

type ReleaseRunLeaseRequest struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	ToStatus            entity.RunStatus
	Now                 int64
}

type ListExpiredRunLeasesRequest struct {
	Now   int64
	Limit int32
}

type ReconcileExpiredRunLeaseRequest struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	ToStatus            entity.RunStatus
	Now                 int64
	ErrorCode           string
	ErrorMessage        string
	Event               *entity.RunEvent
	OutboxIntent        *NotificationOutboxIntent
}

type RequestRunCancellationRequest struct {
	RunID        int64
	Now          int64
	ErrorCode    string
	ErrorMessage string
	Event        *entity.RunEvent
	OutboxIntent *NotificationOutboxIntent
}

type RequestRunCancellationResult struct {
	Run            *entity.Run
	PreviousStatus entity.RunStatus
	Changed        bool
}

type FinalizeRunSuccessRequest struct {
	RunID                             int64
	LeaseOwner                        string
	LeaseToken                        string
	ExecutionGeneration               uint64
	Now                               int64
	Message                           *entity.Message
	TitleEvent                        *entity.RunEvent
	CompletionEvent                   *entity.RunEvent
	TerminalCheckpoint                *entity.Checkpoint
	TerminalCheckpointOnTitleConflict *entity.Checkpoint
	ExpectedThreadTitle               string
	ThreadTitle                       string
	OutboxIntent                      *NotificationOutboxIntent
}

type FinalizeRunSuccessResult struct {
	Run                *entity.Run
	Message            *entity.Message
	TitleEvent         *entity.RunEvent
	CompletionEvent    *entity.RunEvent
	TerminalCheckpoint *entity.Checkpoint
	TitleUpdated       bool
}

type UpdateRunStatusRequest struct {
	RunID                 int64
	From                  entity.RunStatus
	To                    entity.RunStatus
	WorkerID              string
	LeaseOwner            string
	LeaseToken            string
	ExecutionGeneration   uint64
	Now                   int64
	ErrorCode             string
	ErrorMessage          string
	EventPayload          string
	Event                 *entity.RunEvent
	EventAlreadyPersisted bool
	OutboxIntent          *NotificationOutboxIntent
}
