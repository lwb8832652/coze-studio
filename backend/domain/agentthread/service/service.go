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

package service

import (
	"context"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type CreateThreadRequest struct {
	SpaceID      int64
	UserID       int64
	AgentID      int64
	Title        string
	Source       entity.ThreadSource
	LegacyTaskID int64
	Metadata     string
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

type AppendMessageRequest struct {
	ThreadID int64
	RunID    int64
	Role     entity.MessageRole
	Content  string
	Metadata string
}

type CreateRunRequest struct {
	ThreadID          int64
	ParentRunID       int64
	AssistantID       string
	RunKind           entity.RunKind
	Status            entity.RunStatus
	Command           string
	Input             string
	Config            string
	Context           string
	Metadata          string
	StreamMode        string
	MultitaskStrategy string
	OnDisconnect      string
	Durability        string
	IdempotencyKey    string
}

type CreateMessageSpec struct {
	Role     entity.MessageRole
	Content  string
	Metadata string
}

type RunEventPayloadBuilder func(runID int64) string

type CreateRunEventSpec struct {
	EventType      string
	PayloadBuilder RunEventPayloadBuilder
}

type CreateThreadRunMessageRequest struct {
	Thread  CreateThreadRequest
	Run     CreateRunRequest
	Message CreateMessageSpec
}

type CreateThreadRunMessageResult struct {
	Thread  *entity.Thread
	Run     *entity.Run
	Message *entity.Message
}

type CreateRunBundleRequest struct {
	Run                   CreateRunRequest
	Message               *CreateMessageSpec
	Event                 *CreateRunEventSpec
	SkipTopLevelAdmission bool
}

type CreateRunBundleResult struct {
	Run               *entity.Run
	Message           *entity.Message
	Event             *entity.RunEvent
	InterruptedRuns   []*entity.Run
	InterruptedEvents []*entity.RunEvent
	Created           bool
}

type GetRunRequest struct {
	RunID int64
}

type ListRunsRequest struct {
	ThreadID         int64
	ParentRunID      *int64
	IncludeChildRuns bool
	Status           *entity.RunStatus
	Page             int32
	PageSize         int32
}

type AppendRunEventRequest struct {
	ThreadID  int64
	RunID     int64
	EventType string
	Payload   string
}

type CreateCheckpointRequest struct {
	ThreadID           int64
	RunID              int64
	ParentCheckpointID int64
	CheckpointNS       string
	RuntimeType        string
	RuntimeKey         string
	EnvelopeVersion    int32
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
}

type ListCheckpointsRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	Limit       int32
}

type GetCheckpointRequest struct {
	CheckpointID int64
}

type GetLatestCheckpointRequest struct {
	ThreadID int64
}

type GetLatestRuntimeCheckpointRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	RuntimeKey  string
}

type DeleteRuntimeCheckpointRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	RuntimeKey  string
	DeletedAt   int64
}

type RememberMemoryRequest struct {
	ThreadID             int64
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
}

type ImportMemoryItem struct {
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
}

type ImportMemoriesRequest struct {
	ThreadID int64
	ActorID  int64
	Memories []ImportMemoryItem
}

type ImportMemoriesResult struct {
	Imported int64
	Skipped  int64
	Memories []*entity.Memory
}

type RecallMemoriesRequest struct {
	ThreadID int64
	RunID    int64
	Scopes   []entity.MemoryScope
	Query    string
	Limit    int32
}

type ListMemoriesRequest struct {
	ThreadID       int64
	RunID          int64
	Scopes         []entity.MemoryScope
	Query          string
	IncludeExpired bool
	IncludeDeleted bool
	Page           int32
	PageSize       int32
}

type UpdateMemoryRequest struct {
	ThreadID             int64
	MemoryID             int64
	ActorID              int64
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
}

type DeleteMemoryRequest struct {
	ThreadID int64
	MemoryID int64
	ActorID  int64
}

type ClearMemoriesRequest struct {
	ThreadID int64
	RunID    int64
	Scopes   []entity.MemoryScope
	ActorID  int64
}

type RestoreMemoryRequest struct {
	ThreadID int64
	MemoryID int64
	ActorID  int64
}

type ListMemoryAuditEventsRequest struct {
	ThreadID int64
	MemoryID int64
	Page     int32
	PageSize int32
}

type PersistTranscriptSnapshotRequest struct {
	ThreadID       int64
	RunID          int64
	Kind           entity.TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
	Metadata       string
}

type GetTranscriptSnapshotRequest struct {
	SnapshotID int64
}

type EnqueueMemoryFlushJobRequest struct {
	ThreadID             int64
	RunID                int64
	TranscriptSnapshotID int64
	IdempotencyKey       string
	AvailableAt          int64
}

type ClaimMemoryFlushJobsRequest struct {
	WorkerID       string
	Limit          int32
	LeaseTTLMillis int64
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

type RecordTokenUsageRequest struct {
	RunID        int64
	Source       entity.TokenUsageSource
	StepID       string
	StepIndex    int32
	StepName     string
	ModelName    string
	Provider     string
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CostMicros   int64
	Currency     string
	Estimated    bool
	RawUsage     string
	Metadata     string
}

type GetRunTokenUsageRequest struct {
	RunID            int64
	IncludeChildRuns bool
	Source           entity.TokenUsageSource
	Page             int32
	PageSize         int32
}

type GetThreadTokenUsageRequest struct {
	ThreadID int64
	Source   entity.TokenUsageSource
	Page     int32
	PageSize int32
}

type ListRunEventsRequest struct {
	ThreadID     int64
	RunID        int64
	AfterEventID int64
	Page         int32
	PageSize     int32
}

type ClaimPendingRunsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseTTLMillis int64
}

type AggregateRunBacklogRequest struct {
	Statuses []entity.RunStatus
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
	EventPayload        string
	OutboxIntent        *repository.NotificationOutboxIntent
}

type RequestRunCancellationRequest struct {
	RunID        int64
	Now          int64
	ErrorCode    string
	ErrorMessage string
	OutboxIntent *repository.NotificationOutboxIntent
}

type RequestRunCancellationResult struct {
	Run            *entity.Run
	PreviousStatus entity.RunStatus
	Changed        bool
}

type FinalizeRunSuccessRequest struct {
	RunID                             int64
	ThreadID                          int64
	LeaseOwner                        string
	LeaseToken                        string
	ExecutionGeneration               uint64
	Now                               int64
	Message                           string
	MessageMetadata                   string
	TitleEventPayload                 string
	CompletionEventPayload            string
	ExpectedThreadTitle               string
	ThreadTitle                       string
	TerminalCheckpoint                *CreateCheckpointRequest
	TerminalCheckpointOnTitleConflict *CreateCheckpointRequest
	OutboxIntent                      *repository.NotificationOutboxIntent
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
	EventAlreadyPersisted bool
	OutboxIntent          *repository.NotificationOutboxIntent
}

type ListMessagesRequest struct {
	ThreadID int64
	Page     int32
	PageSize int32
}

type ThreadService interface {
	CreateThread(ctx context.Context, req *CreateThreadRequest) (*entity.Thread, error)
	CreateThreadRunMessage(ctx context.Context, req *CreateThreadRunMessageRequest) (*CreateThreadRunMessageResult, error)
	GetThread(ctx context.Context, id int64) (*entity.Thread, error)
	UpdateThreadTitle(ctx context.Context, req *UpdateThreadTitleRequest) (*entity.Thread, bool, error)
	UpdateThreadMetadata(ctx context.Context, req *UpdateThreadMetadataRequest) (*entity.Thread, bool, error)
	DeleteThread(ctx context.Context, req *DeleteThreadRequest) (bool, error)
	ListThreads(ctx context.Context, req *ListThreadsRequest) ([]*entity.Thread, int64, error)
	AppendMessage(ctx context.Context, req *AppendMessageRequest) (*entity.Message, error)
	ListMessages(ctx context.Context, req *ListMessagesRequest) ([]*entity.Message, int64, error)
	CreateRun(ctx context.Context, req *CreateRunRequest) (*entity.Run, error)
	CreateRunBundle(ctx context.Context, req *CreateRunBundleRequest) (*CreateRunBundleResult, error)
	GetRun(ctx context.Context, req *GetRunRequest) (*entity.Run, error)
	GetRunByIdempotencyKey(ctx context.Context, spaceID int64, idempotencyKey string) (*entity.Run, error)
	ListRuns(ctx context.Context, req *ListRunsRequest) ([]*entity.Run, int64, error)
	AppendRunEvent(ctx context.Context, req *AppendRunEventRequest) (*entity.RunEvent, error)
	ListRunEvents(ctx context.Context, req *ListRunEventsRequest) ([]*entity.RunEvent, int64, error)
	CreateCheckpoint(ctx context.Context, req *CreateCheckpointRequest) (*entity.Checkpoint, error)
	GetCheckpoint(ctx context.Context, req *GetCheckpointRequest) (*entity.Checkpoint, error)
	ListCheckpoints(ctx context.Context, req *ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error)
	GetLatestCheckpoint(ctx context.Context, req *GetLatestCheckpointRequest) (*entity.Checkpoint, error)
	GetLatestRuntimeCheckpoint(
		ctx context.Context,
		req *GetLatestRuntimeCheckpointRequest,
	) (*entity.Checkpoint, error)
	DeleteRuntimeCheckpoint(ctx context.Context, req *DeleteRuntimeCheckpointRequest) error
	RememberMemory(ctx context.Context, req *RememberMemoryRequest) (*entity.Memory, error)
	ImportMemories(ctx context.Context, req *ImportMemoriesRequest) (*ImportMemoriesResult, error)
	RecallMemories(ctx context.Context, req *RecallMemoriesRequest) ([]*entity.Memory, int64, error)
	ListMemories(ctx context.Context, req *ListMemoriesRequest) ([]*entity.Memory, int64, error)
	UpdateMemory(ctx context.Context, req *UpdateMemoryRequest) (*entity.Memory, bool, error)
	DeleteMemory(ctx context.Context, req *DeleteMemoryRequest) (bool, error)
	ClearMemories(ctx context.Context, req *ClearMemoriesRequest) (int64, error)
	RestoreMemory(ctx context.Context, req *RestoreMemoryRequest) (*entity.Memory, bool, error)
	ListMemoryAuditEvents(
		ctx context.Context,
		req *ListMemoryAuditEventsRequest,
	) ([]*entity.MemoryAuditEvent, int64, error)
	PersistTranscriptSnapshot(
		ctx context.Context,
		req *PersistTranscriptSnapshotRequest,
	) (*entity.TranscriptSnapshot, bool, error)
	GetTranscriptSnapshot(ctx context.Context, req *GetTranscriptSnapshotRequest) (*entity.TranscriptSnapshot, error)
	EnqueueMemoryFlushJob(
		ctx context.Context,
		req *EnqueueMemoryFlushJobRequest,
	) (*entity.MemoryFlushJob, bool, error)
	ClaimMemoryFlushJobs(ctx context.Context, req *ClaimMemoryFlushJobsRequest) ([]*entity.MemoryFlushJob, error)
	AggregateMemoryFlushBacklog(ctx context.Context, req *AggregateMemoryFlushBacklogRequest) ([]*entity.MemoryFlushBacklogAggregate, error)
	CompleteMemoryFlushJob(ctx context.Context, req *CompleteMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	RetryMemoryFlushJob(ctx context.Context, req *RetryMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	FailMemoryFlushJob(ctx context.Context, req *FailMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	RecordTokenUsage(ctx context.Context, req *RecordTokenUsageRequest) (*entity.TokenUsage, error)
	GetRunTokenUsage(ctx context.Context, req *GetRunTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, []*entity.RunTokenUsageAggregate, error)
	GetThreadTokenUsage(ctx context.Context, req *GetThreadTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, error)
	ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) ([]*entity.Run, error)
	AggregateRunBacklog(ctx context.Context, req *AggregateRunBacklogRequest) ([]*entity.RunBacklogAggregate, error)
	ClaimQueuedResumeRuns(ctx context.Context, req *ClaimQueuedResumeRunsRequest) ([]*entity.Run, error)
	RenewRunLease(ctx context.Context, req *RenewRunLeaseRequest) (*entity.Run, error)
	ReleaseRunLease(ctx context.Context, req *ReleaseRunLeaseRequest) (*entity.Run, error)
	ListExpiredRunLeases(ctx context.Context, req *ListExpiredRunLeasesRequest) ([]*entity.Run, error)
	ReconcileExpiredRunLease(ctx context.Context, req *ReconcileExpiredRunLeaseRequest) (*entity.Run, error)
	RequestRunCancellation(ctx context.Context, req *RequestRunCancellationRequest) (*RequestRunCancellationResult, error)
	FinalizeRunSuccess(ctx context.Context, req *FinalizeRunSuccessRequest) (*FinalizeRunSuccessResult, error)
	InterruptRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
}

type Components struct {
	Repo  repository.ThreadRepository
	IDGen idgen.IDGenerator
}
