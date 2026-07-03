/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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
	maxMCPRuntimeAuditToolNameBytes  = 128
	maxMCPRuntimeAuditEventTypeBytes = 64
	maxMCPRuntimeAuditErrorCodeBytes = 64
)

type MCPRuntimeAuditRepository interface {
	CreateMCPRuntimeAuditEvent(
		ctx context.Context,
		event *entity.MCPRuntimeAuditEvent,
	) error
	ListMCPRuntimeAuditEvents(
		ctx context.Context,
		req ListMCPRuntimeAuditEventsRequest,
	) ([]*entity.MCPRuntimeAuditEvent, int64, error)
}

type ListMCPRuntimeAuditEventsRequest struct {
	ThreadID int64
	RunID    int64
	Limit    int32
	Offset   int32
}

type mcpRuntimeAuditEventPO struct {
	ID              int64  `gorm:"column:id;primaryKey"`
	SpaceID         int64  `gorm:"column:space_id;index:idx_agent_mcp_runtime_audit_space_created,priority:1"`
	ThreadID        int64  `gorm:"column:thread_id;index:idx_agent_mcp_runtime_audit_thread_created,priority:1"`
	RunID           int64  `gorm:"column:run_id;index:idx_agent_mcp_runtime_audit_run_created,priority:1"`
	ServerID        int64  `gorm:"column:server_id;index:idx_agent_mcp_runtime_audit_server_created,priority:1"`
	RuntimeToolName string `gorm:"column:runtime_tool_name"`
	EventType       string `gorm:"column:event_type;index:idx_agent_mcp_runtime_audit_event_created,priority:1"`
	ErrorCode       string `gorm:"column:error_code"`
	ElapsedMillis   int64  `gorm:"column:elapsed_ms"`
	OutputBytes     int64  `gorm:"column:output_bytes"`
	CreatedAt       int64  `gorm:"column:created_at;index:idx_agent_mcp_runtime_audit_space_created,priority:2;index:idx_agent_mcp_runtime_audit_thread_created,priority:2;index:idx_agent_mcp_runtime_audit_run_created,priority:2;index:idx_agent_mcp_runtime_audit_server_created,priority:2;index:idx_agent_mcp_runtime_audit_event_created,priority:2"`
}

func (mcpRuntimeAuditEventPO) TableName() string {
	return "agent_mcp_runtime_audit_events"
}

func NewMCPRuntimeAuditRepository(db *gorm.DB) MCPRuntimeAuditRepository {
	return &threadRepository{db: db}
}

func (r *threadRepository) CreateMCPRuntimeAuditEvent(
	ctx context.Context,
	event *entity.MCPRuntimeAuditEvent,
) error {
	if event == nil {
		return gorm.ErrInvalidData
	}

	return r.db.WithContext(ctx).Create(newMCPRuntimeAuditEventPO(event)).Error
}

func (r *threadRepository) ListMCPRuntimeAuditEvents(
	ctx context.Context,
	req ListMCPRuntimeAuditEventsRequest,
) ([]*entity.MCPRuntimeAuditEvent, int64, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	base := r.db.WithContext(ctx).
		Model(&mcpRuntimeAuditEventPO{}).
		Order("created_at ASC, id ASC")
	if req.ThreadID > 0 {
		base = base.Where("thread_id = ?", req.ThreadID)
	}
	if req.RunID > 0 {
		base = base.Where("run_id = ?", req.RunID)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query := base.Limit(int(limit))
	if req.Offset > 0 {
		query = query.Offset(int(req.Offset))
	}

	pos := make([]*mcpRuntimeAuditEventPO, 0, limit)
	if err := query.Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*entity.MCPRuntimeAuditEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}

	return events, total, nil
}

func newMCPRuntimeAuditEventPO(
	event *entity.MCPRuntimeAuditEvent,
) *mcpRuntimeAuditEventPO {
	return &mcpRuntimeAuditEventPO{
		ID:              event.ID,
		SpaceID:         event.SpaceID,
		ThreadID:        event.ThreadID,
		RunID:           event.RunID,
		ServerID:        event.ServerID,
		RuntimeToolName: truncateMCPRuntimeAuditField(event.RuntimeToolName, maxMCPRuntimeAuditToolNameBytes),
		EventType:       truncateMCPRuntimeAuditField(event.EventType, maxMCPRuntimeAuditEventTypeBytes),
		ErrorCode:       truncateMCPRuntimeAuditField(event.ErrorCode, maxMCPRuntimeAuditErrorCodeBytes),
		ElapsedMillis:   event.ElapsedMillis,
		OutputBytes:     event.OutputBytes,
		CreatedAt:       event.CreatedAt,
	}
}

func (po *mcpRuntimeAuditEventPO) toEntity() *entity.MCPRuntimeAuditEvent {
	if po == nil {
		return nil
	}

	return &entity.MCPRuntimeAuditEvent{
		ID:              po.ID,
		SpaceID:         po.SpaceID,
		ThreadID:        po.ThreadID,
		RunID:           po.RunID,
		ServerID:        po.ServerID,
		RuntimeToolName: po.RuntimeToolName,
		EventType:       po.EventType,
		ErrorCode:       po.ErrorCode,
		ElapsedMillis:   po.ElapsedMillis,
		OutputBytes:     po.OutputBytes,
		CreatedAt:       po.CreatedAt,
	}
}

func truncateMCPRuntimeAuditField(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	for len([]byte(value)) > maxBytes {
		value = value[:len(value)-1]
	}
	return value
}
