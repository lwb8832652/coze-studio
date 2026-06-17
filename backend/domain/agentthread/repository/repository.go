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
	CreateMemory(ctx context.Context, memory *entity.Memory) error
	ListMemories(ctx context.Context, req ListMemoriesRequest) ([]*entity.Memory, int64, error)
	CreateTokenUsage(ctx context.Context, usage *entity.TokenUsage) error
	ListTokenUsage(ctx context.Context, req ListTokenUsageRequest) ([]*entity.TokenUsage, int64, error)
	AggregateTokenUsage(ctx context.Context, req AggregateTokenUsageRequest) (*entity.TokenUsageAggregate, error)
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
	ThreadID int64
	Status   *entity.RunStatus
	Page     int32
	PageSize int32
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
	ThreadID int64
	RunID    int64
	Scopes   []entity.MemoryScope
	Limit    int32
	Now      int64
}

type ListTokenUsageRequest struct {
	ThreadID int64
	RunID    int64
	Source   entity.TokenUsageSource
	Page     int32
	PageSize int32
}

type AggregateTokenUsageRequest struct {
	ThreadID int64
	RunID    int64
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
