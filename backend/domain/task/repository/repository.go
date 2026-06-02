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

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
)

type TaskRepository interface {
	Create(ctx context.Context, task *entity.Task) error
	Get(ctx context.Context, id int64) (*entity.Task, error)
	List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error)
	ListQueued(ctx context.Context, limit int32) ([]*entity.Task, error)
	UpdateStatus(ctx context.Context, id int64, from, to entity.Status, progress int32, result, errMsg string) error
	CreateEvent(ctx context.Context, event *entity.Event) error
	ListEvents(ctx context.Context, taskID int64) ([]*entity.Event, error)
}
