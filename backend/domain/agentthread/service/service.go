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

type AppendMessageRequest struct {
	ThreadID int64
	RunID    int64
	Role     entity.MessageRole
	Content  string
	Metadata string
}

type CreateRunRequest struct {
	ThreadID          int64
	AssistantID       string
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

type GetRunRequest struct {
	RunID int64
}

type ListRunsRequest struct {
	ThreadID int64
	Status   *entity.RunStatus
	Page     int32
	PageSize int32
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
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
}

type ListCheckpointsRequest struct {
	ThreadID int64
	RunID    int64
	Limit    int32
}

type GetCheckpointRequest struct {
	CheckpointID int64
}

type GetLatestCheckpointRequest struct {
	ThreadID int64
}

type RememberMemoryRequest struct {
	ThreadID  int64
	RunID     int64
	Scope     entity.MemoryScope
	Content   string
	Metadata  string
	Score     float64
	ExpiresAt int64
}

type RecallMemoriesRequest struct {
	ThreadID int64
	RunID    int64
	Scopes   []entity.MemoryScope
	Limit    int32
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
	RunID    int64
	Source   entity.TokenUsageSource
	Page     int32
	PageSize int32
}

type GetThreadTokenUsageRequest struct {
	ThreadID int64
	Source   entity.TokenUsageSource
	Page     int32
	PageSize int32
}

type ListRunEventsRequest struct {
	ThreadID int64
	RunID    int64
	Page     int32
	PageSize int32
}

type ClaimPendingRunsRequest struct {
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

type ListMessagesRequest struct {
	ThreadID int64
	Page     int32
	PageSize int32
}

type ThreadService interface {
	CreateThread(ctx context.Context, req *CreateThreadRequest) (*entity.Thread, error)
	GetThread(ctx context.Context, id int64) (*entity.Thread, error)
	ListThreads(ctx context.Context, req *ListThreadsRequest) ([]*entity.Thread, int64, error)
	AppendMessage(ctx context.Context, req *AppendMessageRequest) (*entity.Message, error)
	ListMessages(ctx context.Context, req *ListMessagesRequest) ([]*entity.Message, int64, error)
	CreateRun(ctx context.Context, req *CreateRunRequest) (*entity.Run, error)
	GetRun(ctx context.Context, req *GetRunRequest) (*entity.Run, error)
	ListRuns(ctx context.Context, req *ListRunsRequest) ([]*entity.Run, int64, error)
	AppendRunEvent(ctx context.Context, req *AppendRunEventRequest) (*entity.RunEvent, error)
	ListRunEvents(ctx context.Context, req *ListRunEventsRequest) ([]*entity.RunEvent, int64, error)
	CreateCheckpoint(ctx context.Context, req *CreateCheckpointRequest) (*entity.Checkpoint, error)
	GetCheckpoint(ctx context.Context, req *GetCheckpointRequest) (*entity.Checkpoint, error)
	ListCheckpoints(ctx context.Context, req *ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error)
	GetLatestCheckpoint(ctx context.Context, req *GetLatestCheckpointRequest) (*entity.Checkpoint, error)
	RememberMemory(ctx context.Context, req *RememberMemoryRequest) (*entity.Memory, error)
	RecallMemories(ctx context.Context, req *RecallMemoriesRequest) ([]*entity.Memory, int64, error)
	RecordTokenUsage(ctx context.Context, req *RecordTokenUsageRequest) (*entity.TokenUsage, error)
	GetRunTokenUsage(ctx context.Context, req *GetRunTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, error)
	GetThreadTokenUsage(ctx context.Context, req *GetThreadTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, error)
	ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) ([]*entity.Run, error)
	CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
}

type Components struct {
	Repo  repository.ThreadRepository
	IDGen idgen.IDGenerator
}
