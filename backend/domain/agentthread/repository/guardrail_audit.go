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
	maxGuardrailAuditEventTypeBytes  = 64
	maxGuardrailAuditTargetTypeBytes = 32
	maxGuardrailAuditTargetIDBytes   = 128
	maxGuardrailAuditOperationBytes  = 64
	maxGuardrailAuditSourceBytes     = 64
	maxGuardrailAuditActionBytes     = 16
	maxGuardrailAuditFailModeBytes   = 16
	maxGuardrailAuditProviderBytes   = 64
	maxGuardrailAuditReasonCodeBytes = 64
	maxGuardrailAuditRuleIDsBytes    = 512
	maxGuardrailAuditDeleteBatchSize = int32(1000)
)

type GuardrailAuditRepository interface {
	CreateGuardrailAuditEvent(
		ctx context.Context,
		event *entity.GuardrailAuditEvent,
	) error
	ListGuardrailAuditEvents(
		ctx context.Context,
		req ListGuardrailAuditEventsRequest,
	) ([]*entity.GuardrailAuditEvent, int64, error)
	ListGuardrailAuditEventsBefore(
		ctx context.Context,
		req ListGuardrailAuditEventsBeforeRequest,
	) ([]*entity.GuardrailAuditEvent, int64, error)
	DeleteGuardrailAuditEventsBefore(
		ctx context.Context,
		req DeleteGuardrailAuditEventsBeforeRequest,
	) (int64, error)
	DeleteGuardrailAuditEventsByIDs(
		ctx context.Context,
		req DeleteGuardrailAuditEventsByIDsRequest,
	) (int64, error)
}

type ListGuardrailAuditEventsRequest struct {
	RunID    int64
	ThreadID int64
	Limit    int32
	Offset   int32
}

type ListGuardrailAuditEventsBeforeRequest struct {
	CutoffCreatedAt int64
	Limit           int32
	Offset          int32
}

type DeleteGuardrailAuditEventsBeforeRequest struct {
	CutoffCreatedAt int64
	Limit           int32
}

type DeleteGuardrailAuditEventsByIDsRequest struct {
	EventIDs []int64
}

type guardrailAuditEventPO struct {
	ID         int64  `gorm:"column:id;primaryKey"`
	SpaceID    int64  `gorm:"column:space_id;index:idx_agent_guardrail_audit_space_created,priority:1"`
	ThreadID   int64  `gorm:"column:thread_id;index:idx_agent_guardrail_audit_thread_created,priority:1"`
	RunID      int64  `gorm:"column:run_id;index:idx_agent_guardrail_audit_run_created,priority:1"`
	ActorID    int64  `gorm:"column:actor_id;index:idx_agent_guardrail_audit_actor_created,priority:1"`
	EventType  string `gorm:"column:event_type;index:idx_agent_guardrail_audit_event_created,priority:1"`
	TargetType string `gorm:"column:target_type;index:idx_agent_guardrail_audit_target_created,priority:1"`
	TargetID   string `gorm:"column:target_id"`
	Operation  string `gorm:"column:operation"`
	Source     string `gorm:"column:source"`
	Action     string `gorm:"column:action"`
	FailMode   string `gorm:"column:fail_mode"`
	Provider   string `gorm:"column:provider"`
	ReasonCode string `gorm:"column:reason_code"`
	RuleIDs    string `gorm:"column:rule_ids"`
	CreatedAt  int64  `gorm:"column:created_at;index:idx_agent_guardrail_audit_space_created,priority:2;index:idx_agent_guardrail_audit_thread_created,priority:2;index:idx_agent_guardrail_audit_run_created,priority:2;index:idx_agent_guardrail_audit_actor_created,priority:2;index:idx_agent_guardrail_audit_event_created,priority:2;index:idx_agent_guardrail_audit_target_created,priority:2"`
}

func (guardrailAuditEventPO) TableName() string {
	return "agent_guardrail_audit_events"
}

func NewGuardrailAuditRepository(db *gorm.DB) GuardrailAuditRepository {
	return &threadRepository{db: db}
}

func (r *threadRepository) CreateGuardrailAuditEvent(
	ctx context.Context,
	event *entity.GuardrailAuditEvent,
) error {
	if event == nil {
		return gorm.ErrInvalidData
	}

	return r.db.WithContext(ctx).Create(newGuardrailAuditEventPO(event)).Error
}

func (r *threadRepository) ListGuardrailAuditEvents(
	ctx context.Context,
	req ListGuardrailAuditEventsRequest,
) ([]*entity.GuardrailAuditEvent, int64, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	query := r.db.WithContext(ctx).Model(&guardrailAuditEventPO{})
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}
	if req.ThreadID > 0 {
		query = query.Where("thread_id = ?", req.ThreadID)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.
		Order("created_at ASC, id ASC").
		Limit(int(limit))
	if req.Offset > 0 {
		query = query.Offset(int(req.Offset))
	}

	pos := make([]*guardrailAuditEventPO, 0, limit)
	if err := query.Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*entity.GuardrailAuditEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}

	return events, total, nil
}

func (r *threadRepository) ListGuardrailAuditEventsBefore(
	ctx context.Context,
	req ListGuardrailAuditEventsBeforeRequest,
) ([]*entity.GuardrailAuditEvent, int64, error) {
	limit := req.Limit
	if limit <= 0 || limit > maxGuardrailAuditDeleteBatchSize {
		limit = maxGuardrailAuditDeleteBatchSize
	}

	query := r.db.WithContext(ctx).
		Model(&guardrailAuditEventPO{}).
		Where("created_at <= ?", req.CutoffCreatedAt)

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.
		Order("created_at ASC, id ASC").
		Limit(int(limit))
	if req.Offset > 0 {
		query = query.Offset(int(req.Offset))
	}

	pos := make([]*guardrailAuditEventPO, 0, limit)
	if err := query.Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*entity.GuardrailAuditEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}

	return events, total, nil
}

func (r *threadRepository) DeleteGuardrailAuditEventsBefore(
	ctx context.Context,
	req DeleteGuardrailAuditEventsBeforeRequest,
) (int64, error) {
	limit := req.Limit
	if limit <= 0 || limit > maxGuardrailAuditDeleteBatchSize {
		limit = maxGuardrailAuditDeleteBatchSize
	}

	ids := make([]int64, 0, limit)
	if err := r.db.WithContext(ctx).
		Model(&guardrailAuditEventPO{}).
		Where("created_at <= ?", req.CutoffCreatedAt).
		Order("created_at ASC, id ASC").
		Limit(int(limit)).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	result := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&guardrailAuditEventPO{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

func (r *threadRepository) DeleteGuardrailAuditEventsByIDs(
	ctx context.Context,
	req DeleteGuardrailAuditEventsByIDsRequest,
) (int64, error) {
	eventIDs := boundedUniquePositiveGuardrailAuditEventIDs(req.EventIDs)
	if len(eventIDs) == 0 {
		return 0, nil
	}

	result := r.db.WithContext(ctx).
		Where("id IN ?", eventIDs).
		Delete(&guardrailAuditEventPO{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

func boundedUniquePositiveGuardrailAuditEventIDs(eventIDs []int64) []int64 {
	if len(eventIDs) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(eventIDs))
	result := make([]int64, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID <= 0 {
			continue
		}
		if _, exists := seen[eventID]; exists {
			continue
		}
		seen[eventID] = struct{}{}
		result = append(result, eventID)
		if int32(len(result)) >= maxGuardrailAuditDeleteBatchSize {
			break
		}
	}

	return result
}

func newGuardrailAuditEventPO(
	event *entity.GuardrailAuditEvent,
) *guardrailAuditEventPO {
	return &guardrailAuditEventPO{
		ID:         event.ID,
		SpaceID:    event.SpaceID,
		ThreadID:   event.ThreadID,
		RunID:      event.RunID,
		ActorID:    event.ActorID,
		EventType:  truncateGuardrailAuditField(event.EventType, maxGuardrailAuditEventTypeBytes),
		TargetType: truncateGuardrailAuditField(event.TargetType, maxGuardrailAuditTargetTypeBytes),
		TargetID:   truncateGuardrailAuditField(event.TargetID, maxGuardrailAuditTargetIDBytes),
		Operation:  truncateGuardrailAuditField(event.Operation, maxGuardrailAuditOperationBytes),
		Source:     truncateGuardrailAuditField(event.Source, maxGuardrailAuditSourceBytes),
		Action:     truncateGuardrailAuditField(event.Action, maxGuardrailAuditActionBytes),
		FailMode:   truncateGuardrailAuditField(event.FailMode, maxGuardrailAuditFailModeBytes),
		Provider:   truncateGuardrailAuditField(event.Provider, maxGuardrailAuditProviderBytes),
		ReasonCode: truncateGuardrailAuditField(event.ReasonCode, maxGuardrailAuditReasonCodeBytes),
		RuleIDs:    truncateGuardrailAuditField(event.RuleIDs, maxGuardrailAuditRuleIDsBytes),
		CreatedAt:  event.CreatedAt,
	}
}

func (po *guardrailAuditEventPO) toEntity() *entity.GuardrailAuditEvent {
	if po == nil {
		return nil
	}

	return &entity.GuardrailAuditEvent{
		ID:         po.ID,
		SpaceID:    po.SpaceID,
		ThreadID:   po.ThreadID,
		RunID:      po.RunID,
		ActorID:    po.ActorID,
		EventType:  po.EventType,
		TargetType: po.TargetType,
		TargetID:   po.TargetID,
		Operation:  po.Operation,
		Source:     po.Source,
		Action:     po.Action,
		FailMode:   po.FailMode,
		Provider:   po.Provider,
		ReasonCode: po.ReasonCode,
		RuleIDs:    po.RuleIDs,
		CreatedAt:  po.CreatedAt,
	}
}

func truncateGuardrailAuditField(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	for len([]byte(value)) > maxBytes {
		value = value[:len(value)-1]
	}
	return value
}
