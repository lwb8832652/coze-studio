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
	"fmt"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/task/service"
)

var SVC = new(ApplicationService)

type ApplicationService struct {
	DomainSVC domain.TaskService
}

func (s *ApplicationService) CreateTask(ctx context.Context, req *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	creatorID := int64(0)
	if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
		creatorID = *uid
	}
	task, err := s.DomainSVC.Create(ctx, &domain.CreateRequest{
		SpaceID:        req.SpaceID,
		CreatorID:      creatorID,
		ConversationID: req.GetConversationID(),
		MessageID:      req.GetMessageID(),
		SkillID:        req.GetSkillID(),
		Title:          req.Title,
		Input:          req.GetInput(),
	})
	if err != nil {
		return nil, err
	}
	return &taskapi.CreateTaskResponse{Code: 0, Msg: "success", Data: entityToAPI(task)}, nil
}

func (s *ApplicationService) ListTasks(ctx context.Context, req *taskapi.ListTasksRequest) (*taskapi.ListTasksResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	var status *entity.Status
	if req.Status != nil {
		mapped, err := apiStatusToEntity(*req.Status)
		if err != nil {
			return nil, err
		}
		status = &mapped
	}
	tasks, total, err := s.DomainSVC.List(ctx, req.SpaceID, status, req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, err
	}
	data := &taskapi.ListTasksData{
		Tasks: make([]*taskapi.ChatTask, 0, len(tasks)),
		Total: total,
	}
	for _, task := range tasks {
		data.Tasks = append(data.Tasks, entityToAPI(task))
	}
	return &taskapi.ListTasksResponse{Code: 0, Msg: "success", Data: data}, nil
}

func (s *ApplicationService) GetTask(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	task, err := s.DomainSVC.Get(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	return &taskapi.GetTaskResponse{Code: 0, Msg: "success", Data: entityToAPI(task)}, nil
}

func (s *ApplicationService) CancelTask(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	task, err := s.DomainSVC.Cancel(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	return &taskapi.GetTaskResponse{Code: 0, Msg: "success", Data: entityToAPI(task)}, nil
}

func (s *ApplicationService) RetryTask(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	task, err := s.DomainSVC.Retry(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	return &taskapi.GetTaskResponse{Code: 0, Msg: "success", Data: entityToAPI(task)}, nil
}

func (s *ApplicationService) ListTaskEvents(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.TaskEventsResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	events, err := s.DomainSVC.ListEvents(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	data := &taskapi.TaskEventsData{Events: make([]*taskapi.TaskEvent, 0, len(events))}
	for _, event := range events {
		data.Events = append(data.Events, eventToAPI(event))
	}
	return &taskapi.TaskEventsResponse{Code: 0, Msg: "success", Data: data}, nil
}

func (s *ApplicationService) requireDomainSVC() error {
	if s == nil || s.DomainSVC == nil {
		return fmt.Errorf("task service is not initialized")
	}
	return nil
}

func IsClientError(err error) bool {
	return domain.IsClientError(err)
}

func entityToAPI(task *entity.Task) *taskapi.ChatTask {
	if task == nil {
		return nil
	}
	apiTask := &taskapi.ChatTask{
		ID:        task.ID,
		SpaceID:   task.SpaceID,
		CreatorID: task.CreatorID,
		Title:     task.Title,
		Status:    entityStatusToAPI(task.Status),
		Progress:  task.Progress,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}
	if task.ConversationID != 0 {
		apiTask.ConversationID = &task.ConversationID
	}
	if task.MessageID != 0 {
		apiTask.MessageID = &task.MessageID
	}
	if task.SkillID != 0 {
		apiTask.SkillID = &task.SkillID
	}
	if task.Input != "" {
		apiTask.Input = &task.Input
	}
	if task.Result != "" {
		apiTask.Result = &task.Result
	}
	if task.Error != "" {
		apiTask.Error = &task.Error
	}
	return apiTask
}

func eventToAPI(event *entity.Event) *taskapi.TaskEvent {
	if event == nil {
		return nil
	}
	apiEvent := &taskapi.TaskEvent{
		ID:        event.ID,
		TaskID:    event.TaskID,
		EventType: event.EventType,
		CreatedAt: event.CreatedAt,
	}
	if event.Payload != "" {
		apiEvent.Payload = &event.Payload
	}
	return apiEvent
}

func entityStatusToAPI(status entity.Status) taskapi.TaskStatus {
	switch status {
	case entity.StatusCreated:
		return taskapi.TaskStatus_Created
	case entity.StatusQueued:
		return taskapi.TaskStatus_Queued
	case entity.StatusRunning:
		return taskapi.TaskStatus_Running
	case entity.StatusSucceeded:
		return taskapi.TaskStatus_Succeeded
	case entity.StatusFailed:
		return taskapi.TaskStatus_Failed
	case entity.StatusCanceling:
		return taskapi.TaskStatus_Canceling
	case entity.StatusCanceled:
		return taskapi.TaskStatus_Canceled
	default:
		return 0
	}
}

func apiStatusToEntity(status taskapi.TaskStatus) (entity.Status, error) {
	switch status {
	case taskapi.TaskStatus_Created:
		return entity.StatusCreated, nil
	case taskapi.TaskStatus_Queued:
		return entity.StatusQueued, nil
	case taskapi.TaskStatus_Running:
		return entity.StatusRunning, nil
	case taskapi.TaskStatus_Succeeded:
		return entity.StatusSucceeded, nil
	case taskapi.TaskStatus_Failed:
		return entity.StatusFailed, nil
	case taskapi.TaskStatus_Canceling:
		return entity.StatusCanceling, nil
	case taskapi.TaskStatus_Canceled:
		return entity.StatusCanceled, nil
	default:
		return "", domain.InvalidArgumentErrorf("unsupported task status: %d", status)
	}
}
