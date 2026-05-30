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

package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/task/service"
)

func TestListTasksInvalidStatusIsClientError(t *testing.T) {
	app := &ApplicationService{DomainSVC: noopDomainService{}}
	invalid := taskapi.TaskStatus(99)

	_, err := app.ListTasks(context.Background(), &taskapi.ListTasksRequest{SpaceID: 1, Status: &invalid})

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

type noopDomainService struct{}

func (noopDomainService) Create(ctx context.Context, req *domain.CreateRequest) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) Enqueue(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) Get(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error) {
	return nil, 0, nil
}

func (noopDomainService) Cancel(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) Retry(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) Fail(ctx context.Context, id int64, errMsg string) error {
	return nil
}

func (noopDomainService) Complete(ctx context.Context, id int64, result string) error {
	return nil
}

func (noopDomainService) ListEvents(ctx context.Context, taskID int64) ([]*entity.Event, error) {
	return nil, nil
}
