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

func TestCreateTaskQueuesCreatedTask(t *testing.T) {
	domainSVC := &recordingDomainService{
		created: &entity.Task{
			ID:        100,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "generate report",
			Status:    entity.StatusCreated,
		},
		enqueued: &entity.Task{
			ID:        100,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "generate report",
			Status:    entity.StatusQueued,
			Progress:  0,
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.CreateTask(context.Background(), &taskapi.CreateTaskRequest{
		SpaceID: 1,
		Title:   "generate report",
	})

	require.NoError(t, err)
	require.Equal(t, int64(100), domainSVC.enqueuedID)
	require.NotNil(t, resp.Data)
	require.Equal(t, taskapi.TaskStatus_Queued, resp.Data.Status)
}

func TestCreateRunningTaskStartsWithoutQueueWorker(t *testing.T) {
	domainSVC := &recordingDomainService{
		created: &entity.Task{
			ID:        200,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "quick answer",
			Status:    entity.StatusCreated,
		},
		started: &entity.Task{
			ID:        200,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "quick answer",
			Status:    entity.StatusRunning,
			Progress:  10,
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.CreateRunningTask(context.Background(), &taskapi.CreateTaskRequest{
		SpaceID: 1,
		Title:   "quick answer",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Equal(t, taskapi.TaskStatus_Running, resp.Data.Status)
	require.Equal(t, int64(200), domainSVC.startedID)
	require.Zero(t, domainSVC.enqueuedID)
}

func TestAppendAndCompleteTaskHelpers(t *testing.T) {
	domainSVC := &recordingDomainService{}
	app := &ApplicationService{DomainSVC: domainSVC}

	err := app.AppendTaskEvent(context.Background(), 300, "answer.completed", `{"message":"ok"}`)
	require.NoError(t, err)
	require.Len(t, domainSVC.events, 1)
	require.Equal(t, "answer.completed", domainSVC.events[0].eventType)

	err = app.CompleteTask(context.Background(), 300, `{"result_type":"answer"}`)
	require.NoError(t, err)
	require.Equal(t, int64(300), domainSVC.completedID)
	require.Contains(t, domainSVC.completedResult, `"result_type":"answer"`)
}

func TestProcessQueuedTasksCompletesClaimedTasks(t *testing.T) {
	domainSVC := &recordingDomainService{
		claimed: []*entity.Task{
			{ID: 100, Title: "generate report", Status: entity.StatusRunning},
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	err := app.ProcessQueuedTasks(context.Background(), 10)

	require.NoError(t, err)
	require.Equal(t, int32(10), domainSVC.claimLimit)
	require.Equal(t, int64(100), domainSVC.completedID)
	require.Contains(t, domainSVC.completedResult, `"task_id":"100"`)
	require.Contains(t, domainSVC.completedResult, `"result_type":"answer"`)
	require.Contains(t, domainSVC.completedResult, "任务已完成。")
}

func TestProcessQueuedTasksDoesNotEmitFakeAgentTrace(t *testing.T) {
	domainSVC := &recordingDomainService{
		claimed: []*entity.Task{
			{ID: 100, Title: "generate report", Status: entity.StatusRunning},
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	err := app.ProcessQueuedTasks(context.Background(), 10)

	require.NoError(t, err)
	require.Empty(t, domainSVC.events)
	require.Contains(t, domainSVC.completedResult, `"result_type":"answer"`)
	require.NotContains(t, domainSVC.completedResult, "本地占位执行结果")
}

type noopDomainService struct{}

func (noopDomainService) Create(ctx context.Context, req *domain.CreateRequest) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) Enqueue(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) Start(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) AppendEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	return nil
}

func (noopDomainService) Get(ctx context.Context, id int64) (*entity.Task, error) {
	return nil, nil
}

func (noopDomainService) List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error) {
	return nil, 0, nil
}

func (noopDomainService) ClaimQueued(ctx context.Context, limit int32) ([]*entity.Task, error) {
	return nil, nil
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

type recordingDomainService struct {
	noopDomainService

	created         *entity.Task
	enqueued        *entity.Task
	enqueuedID      int64
	started         *entity.Task
	startedID       int64
	claimed         []*entity.Task
	claimLimit      int32
	failedID        int64
	failError       string
	completedID     int64
	completedResult string
	events          []recordedEvent
}

type recordedEvent struct {
	taskID    int64
	eventType string
	payload   string
}

func (s *recordingDomainService) Create(ctx context.Context, req *domain.CreateRequest) (*entity.Task, error) {
	return s.created, nil
}

func (s *recordingDomainService) Enqueue(ctx context.Context, id int64) (*entity.Task, error) {
	s.enqueuedID = id
	return s.enqueued, nil
}

func (s *recordingDomainService) Start(ctx context.Context, id int64) (*entity.Task, error) {
	s.startedID = id
	return s.started, nil
}

func (s *recordingDomainService) ClaimQueued(ctx context.Context, limit int32) ([]*entity.Task, error) {
	s.claimLimit = limit
	return s.claimed, nil
}

func (s *recordingDomainService) Fail(ctx context.Context, id int64, errMsg string) error {
	s.failedID = id
	s.failError = errMsg
	return nil
}

func (s *recordingDomainService) Complete(ctx context.Context, id int64, result string) error {
	s.completedID = id
	s.completedResult = result
	return nil
}

func (s *recordingDomainService) AppendEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	s.events = append(s.events, recordedEvent{
		taskID:    taskID,
		eventType: eventType,
		payload:   payload,
	})
	return nil
}
