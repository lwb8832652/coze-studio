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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type threadService struct {
	repo  repository.ThreadRepository
	idGen idgen.IDGenerator
}

func NewService(c *Components) ThreadService {
	if c == nil {
		return &threadService{}
	}

	return &threadService{
		repo:  c.Repo,
		idGen: c.IDGen,
	}
}

func (s *threadService) CreateThread(ctx context.Context, req *CreateThreadRequest) (*entity.Thread, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("create thread request is required")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, InvalidArgumentErrorf("thread title is required")
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	source := req.Source
	if source == "" {
		source = entity.ThreadSourceWeb
	}
	now := time.Now().UnixMilli()
	thread := &entity.Thread{
		ID:            id,
		SpaceID:       req.SpaceID,
		CreatorID:     req.UserID,
		AgentID:       req.AgentID,
		Title:         title,
		Status:        entity.ThreadStatusIdle,
		Source:        source,
		LegacyTaskID:  req.LegacyTaskID,
		Metadata:      req.Metadata,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastMessageAt: now,
	}

	if err := s.repo.CreateThread(ctx, thread); err != nil {
		return nil, err
	}

	return thread, nil
}

func (s *threadService) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}

	return s.repo.GetThread(ctx, id)
}

func (s *threadService) ListThreads(ctx context.Context, req *ListThreadsRequest) ([]*entity.Thread, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list threads request is required")
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	return s.repo.ListThreads(ctx, repository.ListThreadsRequest{
		SpaceID:  req.SpaceID,
		UserID:   req.UserID,
		Status:   req.Status,
		Page:     page,
		PageSize: pageSize,
	})
}

func (s *threadService) AppendMessage(ctx context.Context, req *AppendMessageRequest) (*entity.Message, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("append message request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}
	if !isValidMessageRole(req.Role) {
		return nil, InvalidArgumentErrorf("message role is invalid")
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, InvalidArgumentErrorf("message content is required")
	}

	if _, err := s.repo.GetThread(ctx, req.ThreadID); err != nil {
		return nil, err
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	message := &entity.Message{
		ID:        id,
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		Role:      req.Role,
		Content:   content,
		Metadata:  req.Metadata,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.repo.CreateMessage(ctx, message); err != nil {
		return nil, err
	}

	return message, nil
}

func (s *threadService) ListMessages(ctx context.Context, req *ListMessagesRequest) ([]*entity.Message, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list messages request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}

	return s.repo.ListMessages(ctx, repository.ListMessagesRequest{
		ThreadID: req.ThreadID,
		Page:     page,
		PageSize: pageSize,
	})
}

func (s *threadService) CreateRun(ctx context.Context, req *CreateRunRequest) (*entity.Run, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("create run request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}

	input := strings.TrimSpace(req.Input)
	if input == "" {
		return nil, InvalidArgumentErrorf("run input is required")
	}

	thread, err := s.repo.GetThread(ctx, req.ThreadID)
	if err != nil {
		return nil, err
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	run := &entity.Run{
		ID:                id,
		ThreadID:          req.ThreadID,
		SpaceID:           thread.SpaceID,
		CreatorID:         thread.CreatorID,
		AssistantID:       defaultString(req.AssistantID, "default"),
		Status:            entity.RunStatusPending,
		Command:           defaultJSON(req.Command, "{}"),
		Input:             input,
		Config:            defaultJSON(req.Config, "{}"),
		Context:           defaultJSON(req.Context, "{}"),
		Metadata:          defaultJSON(req.Metadata, "{}"),
		StreamMode:        defaultJSON(req.StreamMode, `["messages","updates"]`),
		MultitaskStrategy: defaultString(req.MultitaskStrategy, "enqueue"),
		OnDisconnect:      defaultString(req.OnDisconnect, "continue"),
		Durability:        defaultString(req.Durability, "async"),
		IdempotencyKey:    strings.TrimSpace(req.IdempotencyKey),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	return run, nil
}

func (s *threadService) GetRun(ctx context.Context, req *GetRunRequest) (*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("get run request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}

	return s.repo.GetRun(ctx, req.RunID)
}

func (s *threadService) ListRuns(ctx context.Context, req *ListRunsRequest) ([]*entity.Run, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list runs request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	return s.repo.ListRuns(ctx, repository.ListRunsRequest{
		ThreadID: req.ThreadID,
		Status:   req.Status,
		Page:     page,
		PageSize: pageSize,
	})
}

func (s *threadService) ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) ([]*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("claim pending runs request is required")
	}

	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, InvalidArgumentErrorf("worker id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	return s.repo.ClaimPendingRuns(ctx, repository.ClaimPendingRunsRequest{
		WorkerID: workerID,
		Limit:    limit,
	})
}

func (s *threadService) CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	return s.transitionRun(ctx, req, entity.RunStatusSucceeded)
}

func (s *threadService) FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	return s.transitionRun(ctx, req, entity.RunStatusFailed)
}

func (s *threadService) CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	return s.transitionRun(ctx, req, entity.RunStatusCanceled)
}

func (s *threadService) transitionRun(
	ctx context.Context,
	req *UpdateRunStatusRequest,
	to entity.RunStatus,
) (*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("update run status request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}

	from := req.From
	if from == "" {
		from = entity.RunStatusRunning
	}
	if err := EnsureRunTransition(from, to); err != nil {
		return nil, err
	}

	workerID := strings.TrimSpace(req.WorkerID)
	if err := s.repo.UpdateRunStatus(ctx, repository.UpdateRunStatusRequest{
		RunID:        req.RunID,
		From:         from,
		To:           to,
		WorkerID:     workerID,
		ErrorCode:    strings.TrimSpace(req.ErrorCode),
		ErrorMessage: strings.TrimSpace(req.ErrorMessage),
	}); err != nil {
		return nil, err
	}

	return s.repo.GetRun(ctx, req.RunID)
}

func isValidMessageRole(role entity.MessageRole) bool {
	switch role {
	case entity.MessageRoleUser, entity.MessageRoleAssistant, entity.MessageRoleTool, entity.MessageRoleSystem:
		return true
	default:
		return false
	}
}

func defaultJSON(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	return trimmed
}

func defaultString(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	return trimmed
}

func (s *threadService) requireComponents() error {
	if err := s.requireRepo(); err != nil {
		return err
	}
	if s.idGen == nil {
		return fmt.Errorf("agent thread id generator is required")
	}

	return nil
}

func (s *threadService) requireRepo() error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("agent thread repository is required")
	}

	return nil
}
