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

func isValidMessageRole(role entity.MessageRole) bool {
	switch role {
	case entity.MessageRoleUser, entity.MessageRoleAssistant, entity.MessageRoleTool, entity.MessageRoleSystem:
		return true
	default:
		return false
	}
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
