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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type ThreadRepository interface {
	CreateThread(ctx context.Context, thread *entity.Thread) error
	GetThread(ctx context.Context, id int64) (*entity.Thread, error)
	ListThreads(ctx context.Context, req ListThreadsRequest) ([]*entity.Thread, int64, error)
	CreateMessage(ctx context.Context, message *entity.Message) error
	ListMessages(ctx context.Context, req ListMessagesRequest) ([]*entity.Message, int64, error)
	CreateRun(ctx context.Context, run *entity.Run) error
	GetRun(ctx context.Context, id int64) (*entity.Run, error)
	GetRunByIdempotencyKey(ctx context.Context, spaceID int64, idempotencyKey string) (*entity.Run, error)
	ListRuns(ctx context.Context, req ListRunsRequest) ([]*entity.Run, int64, error)
	ClaimPendingRuns(ctx context.Context, req ClaimPendingRunsRequest) ([]*entity.Run, error)
	ClaimQueuedResumeRuns(ctx context.Context, req ClaimQueuedResumeRunsRequest) ([]*entity.Run, error)
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
	CompleteMemoryFlushJob(ctx context.Context, req CompleteMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	RetryMemoryFlushJob(ctx context.Context, req RetryMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	FailMemoryFlushJob(ctx context.Context, req FailMemoryFlushJobRequest) (*entity.MemoryFlushJob, bool, error)
	CreateTokenUsage(ctx context.Context, usage *entity.TokenUsage) error
	ListTokenUsage(ctx context.Context, req ListTokenUsageRequest) ([]*entity.TokenUsage, int64, error)
	AggregateTokenUsage(ctx context.Context, req AggregateTokenUsageRequest) (*entity.TokenUsageAggregate, error)
	AggregateTokenUsageByRun(ctx context.Context, req AggregateTokenUsageRequest) ([]*entity.RunTokenUsageAggregate, error)
}

type ListThreadsRequest struct {
	SpaceID  int64
	UserID   int64
	Status   *entity.ThreadStatus
	Page     int32
	PageSize int32
}

type ListMessagesRequest struct {
	ThreadID int64
	Page     int32
	PageSize int32
}

type ListRunsRequest struct {
	ThreadID         int64
	ParentRunID      *int64
	IncludeChildRuns bool
	Status           *entity.RunStatus
	Page             int32
	PageSize         int32
}

type ListRunEventsRequest struct {
	ThreadID int64
	RunID    int64
	Page     int32
	PageSize int32
}

type ListCheckpointsRequest struct {
	ThreadID int64
	RunID    int64
	Limit    int32
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
}

type ClaimMemoryFlushJobsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseExpiresAt int64
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
	WorkerID string
	Limit    int32
}

type ClaimQueuedResumeRunsRequest struct {
	WorkerID string
	Limit    int32
}

type UpdateRunStatusRequest struct {
	RunID        int64
	From         entity.RunStatus
	To           entity.RunStatus
	WorkerID     string
	ErrorCode    string
	ErrorMessage string
}
