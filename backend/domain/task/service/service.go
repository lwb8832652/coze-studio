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

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
	"github.com/coze-dev/coze-studio/backend/domain/task/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type CreateRequest struct {
	SpaceID        int64
	CreatorID      int64
	ConversationID int64
	MessageID      int64
	SkillID        int64
	Title          string
	Input          string
}

type TaskService interface {
	Create(ctx context.Context, req *CreateRequest) (*entity.Task, error)
	Enqueue(ctx context.Context, id int64) (*entity.Task, error)
	Start(ctx context.Context, id int64) (*entity.Task, error)
	AppendEvent(ctx context.Context, taskID int64, eventType, payload string) error
	Get(ctx context.Context, id int64) (*entity.Task, error)
	List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error)
	ClaimQueued(ctx context.Context, limit int32) ([]*entity.Task, error)
	Cancel(ctx context.Context, id int64) (*entity.Task, error)
	Retry(ctx context.Context, id int64) (*entity.Task, error)
	Fail(ctx context.Context, id int64, errMsg string) error
	Complete(ctx context.Context, id int64, result string) error
	ListEvents(ctx context.Context, taskID int64) ([]*entity.Event, error)
}

type Components struct {
	Repo  repository.TaskRepository
	IDGen idgen.IDGenerator
}
