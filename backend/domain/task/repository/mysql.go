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

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type taskRepository struct {
	db    *gorm.DB
	idGen idgen.IDGenerator
}

func NewTaskRepository(db *gorm.DB, idGen idgen.IDGenerator) TaskRepository {
	return &taskRepository{
		db:    db,
		idGen: idGen,
	}
}

type taskPO struct {
	ID             int64          `gorm:"column:id;primaryKey"`
	SpaceID        int64          `gorm:"column:space_id"`
	CreatorID      int64          `gorm:"column:creator_id"`
	ConversationID int64          `gorm:"column:conversation_id"`
	MessageID      int64          `gorm:"column:message_id"`
	SkillID        int64          `gorm:"column:skill_id"`
	Title          string         `gorm:"column:title"`
	Status         string         `gorm:"column:status"`
	Progress       int32          `gorm:"column:progress"`
	Input          datatypes.JSON `gorm:"column:input;type:json"`
	Result         datatypes.JSON `gorm:"column:result;type:json"`
	Error          string         `gorm:"column:error"`
	CreatedAt      int64          `gorm:"column:created_at"`
	UpdatedAt      int64          `gorm:"column:updated_at"`
}

func (taskPO) TableName() string {
	return "chat_tasks"
}

type taskAttemptPO struct {
	ID        int64  `gorm:"column:id;primaryKey"`
	TaskID    int64  `gorm:"column:task_id;uniqueIndex:uk_chat_task_attempts_task_attempt"`
	AttemptNo int32  `gorm:"column:attempt_no;uniqueIndex:uk_chat_task_attempts_task_attempt"`
	Status    string `gorm:"column:status"`
	StartedAt int64  `gorm:"column:started_at"`
	EndedAt   int64  `gorm:"column:ended_at"`
	Runtime   string `gorm:"column:runtime"`
	Error     string `gorm:"column:error"`
}

func (taskAttemptPO) TableName() string {
	return "chat_task_attempts"
}

type taskEventPO struct {
	ID        int64          `gorm:"column:id;primaryKey"`
	TaskID    int64          `gorm:"column:task_id;index:idx_chat_task_events_task_created"`
	EventType string         `gorm:"column:event_type"`
	Payload   datatypes.JSON `gorm:"column:payload;type:json"`
	CreatedAt int64          `gorm:"column:created_at;index:idx_chat_task_events_task_created"`
}

func (taskEventPO) TableName() string {
	return "chat_task_events"
}

func (r *taskRepository) Create(ctx context.Context, task *entity.Task) error {
	if task.ID == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		task.ID = id
	}

	now := time.Now().UnixMilli()
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	if task.UpdatedAt == 0 {
		task.UpdatedAt = task.CreatedAt
	}

	po, err := taskToPO(task)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *taskRepository) Get(ctx context.Context, id int64) (*entity.Task, error) {
	var po taskPO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error; err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *taskRepository) List(ctx context.Context, spaceID int64, status *entity.Status, page, pageSize int32) ([]*entity.Task, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&taskPO{}).Where("space_id = ?", spaceID)
	if status != nil {
		query = query.Where("status = ?", string(*status))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*taskPO, 0)
	if err := query.
		Order("updated_at DESC, id DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	tasks := make([]*entity.Task, 0, len(pos))
	for _, po := range pos {
		tasks = append(tasks, po.toEntity())
	}

	return tasks, total, nil
}

func (r *taskRepository) UpdateStatus(ctx context.Context, id int64, from, to entity.Status, progress int32, result, errMsg string) error {
	resultJSON, err := optionalJSON("result", result)
	if err != nil {
		return err
	}

	updates := map[string]any{
		"status":     string(to),
		"progress":   progress,
		"result":     resultJSON,
		"error":      errMsg,
		"updated_at": time.Now().UnixMilli(),
	}

	db := r.db.WithContext(ctx).
		Model(&taskPO{}).
		Where("id = ? AND status = ?", id, string(from)).
		Updates(updates)
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return fmt.Errorf("update task status failed: task %d is not in status %s", id, from)
	}

	return nil
}

func (r *taskRepository) CreateEvent(ctx context.Context, event *entity.Event) error {
	if event.ID == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		event.ID = id
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}

	po, err := eventToPO(event)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *taskRepository) ListEvents(ctx context.Context, taskID int64) ([]*entity.Event, error) {
	pos := make([]*taskEventPO, 0)
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("created_at ASC, id ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}

	events := make([]*entity.Event, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}

	return events, nil
}

func taskToPO(task *entity.Task) (*taskPO, error) {
	input, err := optionalJSON("input", task.Input)
	if err != nil {
		return nil, err
	}
	result, err := optionalJSON("result", task.Result)
	if err != nil {
		return nil, err
	}

	return &taskPO{
		ID:             task.ID,
		SpaceID:        task.SpaceID,
		CreatorID:      task.CreatorID,
		ConversationID: task.ConversationID,
		MessageID:      task.MessageID,
		SkillID:        task.SkillID,
		Title:          task.Title,
		Status:         string(task.Status),
		Progress:       task.Progress,
		Input:          input,
		Result:         result,
		Error:          task.Error,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}, nil
}

func (po *taskPO) toEntity() *entity.Task {
	return &entity.Task{
		ID:             po.ID,
		SpaceID:        po.SpaceID,
		CreatorID:      po.CreatorID,
		ConversationID: po.ConversationID,
		MessageID:      po.MessageID,
		SkillID:        po.SkillID,
		Title:          po.Title,
		Status:         entity.Status(po.Status),
		Progress:       po.Progress,
		Input:          jsonToString(po.Input),
		Result:         jsonToString(po.Result),
		Error:          po.Error,
		CreatedAt:      po.CreatedAt,
		UpdatedAt:      po.UpdatedAt,
	}
}

func eventToPO(event *entity.Event) (*taskEventPO, error) {
	payload, err := optionalJSON("payload", event.Payload)
	if err != nil {
		return nil, err
	}

	return &taskEventPO{
		ID:        event.ID,
		TaskID:    event.TaskID,
		EventType: event.EventType,
		Payload:   payload,
		CreatedAt: event.CreatedAt,
	}, nil
}

func (po *taskEventPO) toEntity() *entity.Event {
	return &entity.Event{
		ID:        po.ID,
		TaskID:    po.TaskID,
		EventType: po.EventType,
		Payload:   jsonToString(po.Payload),
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
