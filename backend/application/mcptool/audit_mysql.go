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

package mcptool

import (
	"context"

	"gorm.io/gorm"
)

type mcpManagementAuditEventPO struct {
	EventID      int64  `gorm:"column:event_id;primaryKey;autoIncrement"`
	SpaceID      int64  `gorm:"column:space_id;not null;index:idx_mcp_management_audit_scope,priority:1"`
	ServerID     int64  `gorm:"column:server_id;not null;index:idx_mcp_management_audit_scope,priority:2"`
	ActorID      int64  `gorm:"column:actor_id;not null"`
	ToolName     string `gorm:"column:tool_name;size:256;not null"`
	Status       string `gorm:"column:status;size:16;not null"`
	LatencyMs    int64  `gorm:"column:latency_ms;not null;default:0"`
	ErrorCode    string `gorm:"column:error_code;size:64;not null;default:''"`
	ErrorSummary string `gorm:"column:error_summary;size:256;not null;default:''"`
	CreatedAt    int64  `gorm:"column:created_at;not null;index:idx_mcp_management_audit_scope,priority:3,sort:desc"`
	CompletedAt  int64  `gorm:"column:completed_at;not null;default:0"`
}

func (mcpManagementAuditEventPO) TableName() string {
	return "mcp_management_audit_events"
}

type MySQLManagementAuditRepository struct {
	db *gorm.DB
}

func NewMySQLManagementAuditRepository(db *gorm.DB) *MySQLManagementAuditRepository {
	return &MySQLManagementAuditRepository{db: db}
}

func (r *MySQLManagementAuditRepository) CreatePending(
	ctx context.Context,
	event *ManagementAuditEvent,
) error {
	if r == nil || r.db == nil || event == nil || event.SpaceID <= 0 || event.ServerID <= 0 || event.ActorID <= 0 {
		return ErrManagementAuditUnavailable
	}
	po := &mcpManagementAuditEventPO{
		SpaceID:   event.SpaceID,
		ServerID:  event.ServerID,
		ActorID:   event.ActorID,
		ToolName:  normalizeManagementAuditToolName(event.ToolName),
		Status:    ManagementAuditStatusPending,
		CreatedAt: event.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(po).Error; err != nil {
		return err
	}
	event.EventID = po.EventID
	event.ToolName = po.ToolName
	event.Status = po.Status
	return nil
}

func (r *MySQLManagementAuditRepository) Complete(
	ctx context.Context,
	eventID int64,
	completion ManagementAuditCompletion,
) error {
	if r == nil || r.db == nil || eventID <= 0 {
		return ErrManagementAuditTransition
	}
	event := &ManagementAuditEvent{}
	applyManagementAuditCompletion(event, completion)
	result := r.db.WithContext(ctx).
		Model(&mcpManagementAuditEventPO{}).
		Where("event_id = ? AND status = ?", eventID, ManagementAuditStatusPending).
		Updates(map[string]any{
			"status":        event.Status,
			"latency_ms":    event.LatencyMs,
			"error_code":    event.ErrorCode,
			"error_summary": event.ErrorSummary,
			"completed_at":  completion.CompletedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrManagementAuditTransition
	}
	return nil
}

func (r *MySQLManagementAuditRepository) List(
	ctx context.Context,
	spaceID int64,
	serverID int64,
	cursor *ManagementAuditCursor,
	limit int,
) ([]*ManagementAuditEvent, error) {
	if r == nil || r.db == nil || spaceID <= 0 || serverID <= 0 || limit <= 0 {
		return nil, ErrManagementAuditUnavailable
	}
	query := r.db.WithContext(ctx).
		Where("space_id = ? AND server_id = ?", spaceID, serverID)
	if cursor != nil {
		query = query.Where(
			"created_at < ? OR (created_at = ? AND event_id < ?)",
			cursor.CreatedAt,
			cursor.CreatedAt,
			cursor.EventID,
		)
	}
	var records []*mcpManagementAuditEventPO
	if err := query.Order("created_at DESC").Order("event_id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	events := make([]*ManagementAuditEvent, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		events = append(events, &ManagementAuditEvent{
			EventID:      record.EventID,
			SpaceID:      record.SpaceID,
			ServerID:     record.ServerID,
			ActorID:      record.ActorID,
			ToolName:     record.ToolName,
			Status:       record.Status,
			LatencyMs:    record.LatencyMs,
			ErrorCode:    record.ErrorCode,
			ErrorSummary: record.ErrorSummary,
			CreatedAt:    record.CreatedAt,
			CompletedAt:  record.CompletedAt,
		})
	}
	return events, nil
}

var _ ManagementAuditRepository = (*MySQLManagementAuditRepository)(nil)
