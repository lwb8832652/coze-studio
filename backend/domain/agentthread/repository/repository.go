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
