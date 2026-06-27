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
	"strings"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	maxMemoryAuditEventTypeBytes  = 64
	maxMemoryAuditScopeBytes      = 32
	maxMemoryAuditSourceTypeBytes = 64
	maxMemoryAuditSourceIDBytes   = 128
)

type memoryAuditEventPO struct {
	ID            int64  `gorm:"column:id;primaryKey"`
	ThreadID      int64  `gorm:"column:thread_id;index:idx_agent_memory_audit_thread_created,priority:1"`
	RunID         int64  `gorm:"column:run_id;index:idx_agent_memory_audit_run_created,priority:1"`
	SpaceID       int64  `gorm:"column:space_id;index:idx_agent_memory_audit_space_created,priority:1"`
	MemoryID      int64  `gorm:"column:memory_id;index:idx_agent_memory_audit_memory_created,priority:1"`
	ActorID       int64  `gorm:"column:actor_id;index:idx_agent_memory_audit_actor_created,priority:1"`
	EventType     string `gorm:"column:event_type;index:idx_agent_memory_audit_event_created,priority:1"`
	Scope         string `gorm:"column:scope"`
	SourceType    string `gorm:"column:source_type"`
	SourceID      string `gorm:"column:source_id"`
	AffectedCount int64  `gorm:"column:affected_count"`
	CreatedAt     int64  `gorm:"column:created_at;index:idx_agent_memory_audit_thread_created,priority:2;index:idx_agent_memory_audit_run_created,priority:2;index:idx_agent_memory_audit_space_created,priority:2;index:idx_agent_memory_audit_memory_created,priority:2;index:idx_agent_memory_audit_actor_created,priority:2;index:idx_agent_memory_audit_event_created,priority:2"`
}

func (memoryAuditEventPO) TableName() string {
	return "agent_memory_audit_events"
}

func (r *threadRepository) CreateMemoryAuditEvent(
	ctx context.Context,
	event *entity.MemoryAuditEvent,
) error {
	if event == nil {
		return gorm.ErrInvalidData
	}

	return r.db.WithContext(ctx).Create(newMemoryAuditEventPO(event)).Error
}

func (r *threadRepository) ListMemoryAuditEvents(
	ctx context.Context,
	req ListMemoryAuditEventsRequest,
) ([]*entity.MemoryAuditEvent, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&memoryAuditEventPO{}).
		Where("thread_id = ?", req.ThreadID)
	if req.MemoryID > 0 {
		query = query.Where("memory_id = ?", req.MemoryID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*memoryAuditEventPO, 0)
	if err := query.Order("created_at ASC, id ASC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*entity.MemoryAuditEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}

	return events, total, nil
}

func newMemoryAuditEventPO(event *entity.MemoryAuditEvent) *memoryAuditEventPO {
	return &memoryAuditEventPO{
		ID:            event.ID,
		ThreadID:      event.ThreadID,
		RunID:         event.RunID,
		SpaceID:       event.SpaceID,
		MemoryID:      event.MemoryID,
		ActorID:       event.ActorID,
		EventType:     truncateMemoryAuditField(event.EventType, maxMemoryAuditEventTypeBytes),
		Scope:         truncateMemoryAuditField(string(event.Scope), maxMemoryAuditScopeBytes),
		SourceType:    truncateMemoryAuditField(event.SourceType, maxMemoryAuditSourceTypeBytes),
		SourceID:      truncateMemoryAuditField(event.SourceID, maxMemoryAuditSourceIDBytes),
		AffectedCount: event.AffectedCount,
		CreatedAt:     event.CreatedAt,
	}
}

func (po *memoryAuditEventPO) toEntity() *entity.MemoryAuditEvent {
	if po == nil {
		return nil
	}

	return &entity.MemoryAuditEvent{
		ID:            po.ID,
		ThreadID:      po.ThreadID,
		RunID:         po.RunID,
		SpaceID:       po.SpaceID,
		MemoryID:      po.MemoryID,
		ActorID:       po.ActorID,
		EventType:     po.EventType,
		Scope:         entity.MemoryScope(po.Scope),
		SourceType:    po.SourceType,
		SourceID:      po.SourceID,
		AffectedCount: po.AffectedCount,
		CreatedAt:     po.CreatedAt,
	}
}

func truncateMemoryAuditField(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	for len([]byte(value)) > maxBytes {
		value = value[:len(value)-1]
	}
	return value
}
