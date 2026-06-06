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
	"fmt"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
	"github.com/coze-dev/coze-studio/backend/domain/task/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type taskService struct {
	repo  repository.TaskRepository
	idGen idgen.IDGenerator
}

func NewService(c *Components) TaskService {
	if c == nil {
		return &taskService{}
	}
	return &taskService{
		repo:  c.Repo,
		idGen: c.IDGen,
	}
}

func (s *taskService) Create(ctx context.Context, req *CreateRequest) (*entity.Task, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("create task request is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, InvalidArgumentErrorf("task title is required")
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	task := &entity.Task{
		ID:             id,
		SpaceID:        req.SpaceID,
		CreatorID:      req.CreatorID,
		ConversationID: req.ConversationID,
		MessageID:      req.MessageID,
		SkillID:        req.SkillID,
		Title:          req.Title,
		Status:         entity.StatusCreated,
		Progress:       0,
		Input:          req.Input,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}
	if err := s.createEvent(ctx, task.ID, "created", fmt.Sprintf(`{"status":%q}`, task.Status)); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *taskService) Enqueue(ctx context.Context, id int64) (*entity.Task, error) {
	return s.transition(ctx, id, entity.StatusQueued, 0, "", "")
}

func (s *taskService) Start(ctx context.Context, id int64) (*entity.Task, error) {
	return s.transition(ctx, id, entity.StatusRunning, 10, "", "")
}

func (s *taskService) AppendEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	if strings.TrimSpace(eventType) == "" {
		return InvalidArgumentErrorf("task event type is required")
	}
	if strings.TrimSpace(payload) == "" {
		payload = "{}"
	}
	return s.createEvent(ctx, taskID, eventType, payload)
}

func (s *taskService) Get(ctx context.Context, id int64) (*entity.Task, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, id)
}

func (s *taskService) List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, spaceID, status, page, pageSize)
}

func (s *taskService) ClaimQueued(ctx context.Context, limit int32) ([]*entity.Task, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	tasks, err := s.repo.ListQueued(ctx, limit)
	if err != nil {
		return nil, err
	}

	claimed := make([]*entity.Task, 0, len(tasks))
	for _, task := range tasks {
		running, err := s.transitionFrom(ctx, task, entity.StatusRunning, 10, task.Result, task.Error)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, running)
	}

	return claimed, nil
}

func (s *taskService) Cancel(ctx context.Context, id int64) (*entity.Task, error) {
	task, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	to := entity.StatusCanceled
	if task.Status == entity.StatusRunning {
		to = entity.StatusCanceling
	}
	return s.transitionFrom(ctx, task, to, task.Progress, task.Result, task.Error)
}

func (s *taskService) Retry(ctx context.Context, id int64) (*entity.Task, error) {
	task, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != entity.StatusFailed {
		return nil, InvalidArgumentErrorf("cannot transition task from %s to %s", task.Status, entity.StatusQueued)
	}
	return s.transitionFrom(ctx, task, entity.StatusQueued, 0, "", "")
}

func (s *taskService) Fail(ctx context.Context, id int64, errMsg string) error {
	_, err := s.transition(ctx, id, entity.StatusFailed, 0, "", errMsg)
	return err
}

func (s *taskService) Complete(ctx context.Context, id int64, result string) error {
	_, err := s.transition(ctx, id, entity.StatusSucceeded, 100, result, "")
	return err
}

func (s *taskService) ListEvents(ctx context.Context, taskID int64) ([]*entity.Event, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	return s.repo.ListEvents(ctx, taskID)
}

func (s *taskService) transition(ctx context.Context, id int64, to entity.Status, progress int32, result, errMsg string) (*entity.Task, error) {
	task, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.transitionFrom(ctx, task, to, progress, result, errMsg)
}

func (s *taskService) transitionFrom(ctx context.Context, task *entity.Task, to entity.Status, progress int32, result, errMsg string) (*entity.Task, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if task == nil {
		return nil, InvalidArgumentErrorf("task is required")
	}
	if err := EnsureTransition(task.Status, to); err != nil {
		return nil, err
	}
	from := task.Status
	if err := s.repo.UpdateStatus(ctx, task.ID, from, to, progress, result, errMsg); err != nil {
		return nil, err
	}
	if err := s.createEvent(ctx, task.ID, "status_changed", fmt.Sprintf(`{"from":%q,"to":%q}`, from, to)); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, task.ID)
}

func (s *taskService) createEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	if err := s.requireRepo(); err != nil {
		return err
	}
	return s.repo.CreateEvent(ctx, &entity.Event{
		TaskID:    taskID,
		EventType: eventType,
		Payload:   payload,
		CreatedAt: time.Now().UnixMilli(),
	})
}

func (s *taskService) requireComponents() error {
	if err := s.requireRepo(); err != nil {
		return err
	}
	if s.idGen == nil {
		return fmt.Errorf("task id generator is required")
	}
	return nil
}

func (s *taskService) requireRepo() error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("task repository is required")
	}
	return nil
}
