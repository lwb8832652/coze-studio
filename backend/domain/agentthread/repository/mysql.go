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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type threadRepository struct {
	db *gorm.DB
}

func NewThreadRepository(db *gorm.DB) ThreadRepository {
	return &threadRepository{db: db}
}

type threadPO struct {
	ID            int64          `gorm:"column:id;primaryKey"`
	SpaceID       int64          `gorm:"column:space_id;index:idx_agent_threads_space_updated;index:idx_agent_threads_space_status"`
	CreatorID     int64          `gorm:"column:creator_id;index:idx_agent_threads_creator_updated"`
	AgentID       int64          `gorm:"column:agent_id"`
	Title         string         `gorm:"column:title"`
	Status        string         `gorm:"column:status;index:idx_agent_threads_space_status"`
	Source        string         `gorm:"column:source"`
	LegacyTaskID  int64          `gorm:"column:legacy_task_id;index:idx_agent_threads_legacy_task"`
	Metadata      datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt     int64          `gorm:"column:created_at"`
	UpdatedAt     int64          `gorm:"column:updated_at;index:idx_agent_threads_space_updated;index:idx_agent_threads_creator_updated"`
	LastMessageAt int64          `gorm:"column:last_message_at"`
}

type messagePO struct {
	ID        int64          `gorm:"column:id;primaryKey"`
	ThreadID  int64          `gorm:"column:thread_id;index:idx_agent_thread_messages_thread_created"`
	RunID     int64          `gorm:"column:run_id;index:idx_agent_thread_messages_run_created"`
	Role      string         `gorm:"column:role"`
	Content   string         `gorm:"column:content"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt int64          `gorm:"column:created_at;index:idx_agent_thread_messages_thread_created;index:idx_agent_thread_messages_run_created"`
}

func (threadPO) TableName() string {
	return "agent_threads"
}

func (messagePO) TableName() string {
	return "agent_thread_messages"
}

func (r *threadRepository) CreateThread(ctx context.Context, thread *entity.Thread) error {
	if thread == nil {
		return fmt.Errorf("thread is required")
	}

	now := time.Now().UnixMilli()
	if thread.CreatedAt == 0 {
		thread.CreatedAt = now
	}
	if thread.UpdatedAt == 0 {
		thread.UpdatedAt = thread.CreatedAt
	}
	if thread.LastMessageAt == 0 {
		thread.LastMessageAt = thread.UpdatedAt
	}

	po, err := threadToPO(thread)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	var po threadPO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error; err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) ListThreads(ctx context.Context, req ListThreadsRequest) ([]*entity.Thread, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&threadPO{}).Where("space_id = ?", req.SpaceID)
	if req.UserID > 0 {
		query = query.Where("creator_id = ?", req.UserID)
	}
	if req.Status != nil {
		query = query.Where("status = ?", string(*req.Status))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*threadPO, 0)
	if err := query.
		Order("updated_at DESC, id DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	threads := make([]*entity.Thread, 0, len(pos))
	for _, po := range pos {
		threads = append(threads, po.toEntity())
	}

	return threads, total, nil
}

func (r *threadRepository) CreateMessage(ctx context.Context, message *entity.Message) error {
	if message == nil {
		return fmt.Errorf("message is required")
	}

	if message.CreatedAt == 0 {
		message.CreatedAt = time.Now().UnixMilli()
	}

	po, err := messageToPO(message)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) ListMessages(ctx context.Context, req ListMessagesRequest) ([]*entity.Message, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}

	query := r.db.WithContext(ctx).Model(&messagePO{}).Where("thread_id = ?", req.ThreadID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*messagePO, 0)
	if err := query.
		Order("created_at ASC, id ASC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	messages := make([]*entity.Message, 0, len(pos))
	for _, po := range pos {
		messages = append(messages, po.toEntity())
	}

	return messages, total, nil
}

func threadToPO(thread *entity.Thread) (*threadPO, error) {
	metadata, err := optionalJSON("metadata", thread.Metadata)
	if err != nil {
		return nil, err
	}

	return &threadPO{
		ID:            thread.ID,
		SpaceID:       thread.SpaceID,
		CreatorID:     thread.CreatorID,
		AgentID:       thread.AgentID,
		Title:         thread.Title,
		Status:        string(thread.Status),
		Source:        string(thread.Source),
		LegacyTaskID:  thread.LegacyTaskID,
		Metadata:      metadata,
		CreatedAt:     thread.CreatedAt,
		UpdatedAt:     thread.UpdatedAt,
		LastMessageAt: thread.LastMessageAt,
	}, nil
}

func (po *threadPO) toEntity() *entity.Thread {
	return &entity.Thread{
		ID:            po.ID,
		SpaceID:       po.SpaceID,
		CreatorID:     po.CreatorID,
		AgentID:       po.AgentID,
		Title:         po.Title,
		Status:        entity.ThreadStatus(po.Status),
		Source:        entity.ThreadSource(po.Source),
		LegacyTaskID:  po.LegacyTaskID,
		Metadata:      jsonToString(po.Metadata),
		CreatedAt:     po.CreatedAt,
		UpdatedAt:     po.UpdatedAt,
		LastMessageAt: po.LastMessageAt,
	}
}

func messageToPO(message *entity.Message) (*messagePO, error) {
	metadata, err := optionalJSON("metadata", message.Metadata)
	if err != nil {
		return nil, err
	}

	return &messagePO{
		ID:        message.ID,
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Role:      string(message.Role),
		Content:   message.Content,
		Metadata:  metadata,
		CreatedAt: message.CreatedAt,
	}, nil
}

func (po *messagePO) toEntity() *entity.Message {
	return &entity.Message{
		ID:        po.ID,
		ThreadID:  po.ThreadID,
		RunID:     po.RunID,
		Role:      entity.MessageRole(po.Role),
		Content:   po.Content,
		Metadata:  jsonToString(po.Metadata),
		CreatedAt: po.CreatedAt,
	}
}

func optionalJSON(field, value string) (datatypes.JSON, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("%s must be valid JSON", field)
	}

	return datatypes.JSON([]byte(trimmed)), nil
}

func jsonToString(value datatypes.JSON) string {
	if len(value) == 0 {
		return ""
	}

	return string(value)
}
