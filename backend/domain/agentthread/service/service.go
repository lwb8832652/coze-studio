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
	ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) ([]*entity.Run, error)
	CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
	CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
}

type Components struct {
	Repo  repository.ThreadRepository
	IDGen idgen.IDGenerator
}
