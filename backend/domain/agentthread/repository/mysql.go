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
	"gorm.io/gorm/clause"

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

type runPO struct {
	ID                int64          `gorm:"column:id;primaryKey"`
	ThreadID          int64          `gorm:"column:thread_id;index:idx_agent_runs_thread_created"`
	SpaceID           int64          `gorm:"column:space_id;index:idx_agent_runs_space_status;uniqueIndex:uk_agent_runs_space_idempotency"`
	CreatorID         int64          `gorm:"column:creator_id"`
	AssistantID       string         `gorm:"column:assistant_id"`
	Status            string         `gorm:"column:status;index:idx_agent_runs_space_status"`
	Command           datatypes.JSON `gorm:"column:command;type:json"`
	Input             datatypes.JSON `gorm:"column:input;type:json"`
	Config            datatypes.JSON `gorm:"column:config;type:json"`
	Context           datatypes.JSON `gorm:"column:context;type:json"`
	Metadata          datatypes.JSON `gorm:"column:metadata;type:json"`
	StreamMode        datatypes.JSON `gorm:"column:stream_mode;type:json"`
	MultitaskStrategy string         `gorm:"column:multitask_strategy"`
	OnDisconnect      string         `gorm:"column:on_disconnect"`
	Durability        string         `gorm:"column:durability"`
	IdempotencyKey    *string        `gorm:"column:idempotency_key;uniqueIndex:uk_agent_runs_space_idempotency"`
	WorkerID          string         `gorm:"column:worker_id"`
	ErrorCode         string         `gorm:"column:error_code"`
	ErrorMessage      string         `gorm:"column:error_message"`
	StartedAt         int64          `gorm:"column:started_at"`
	EndedAt           int64          `gorm:"column:ended_at"`
	CreatedAt         int64          `gorm:"column:created_at;index:idx_agent_runs_thread_created"`
	UpdatedAt         int64          `gorm:"column:updated_at"`
}

type runEventPO struct {
	ID        int64          `gorm:"column:id;primaryKey"`
	ThreadID  int64          `gorm:"column:thread_id;index:idx_agent_run_events_thread_created"`
	RunID     int64          `gorm:"column:run_id;index:idx_agent_run_events_run_created"`
	EventType string         `gorm:"column:event_type"`
	Payload   datatypes.JSON `gorm:"column:payload;type:json"`
	CreatedAt int64          `gorm:"column:created_at;index:idx_agent_run_events_thread_created;index:idx_agent_run_events_run_created"`
}

type checkpointPO struct {
	ID                 int64          `gorm:"column:id;primaryKey"`
	ThreadID           int64          `gorm:"column:thread_id;index:idx_agent_checkpoints_thread_created"`
	RunID              int64          `gorm:"column:run_id;index:idx_agent_checkpoints_run_created"`
	ParentCheckpointID int64          `gorm:"column:parent_checkpoint_id"`
	CheckpointNS       string         `gorm:"column:checkpoint_ns"`
	ChannelValues      datatypes.JSON `gorm:"column:channel_values;type:json"`
	ChannelVersions    datatypes.JSON `gorm:"column:channel_versions;type:json"`
	PendingSends       datatypes.JSON `gorm:"column:pending_sends;type:json"`
	Metadata           datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt          int64          `gorm:"column:created_at;index:idx_agent_checkpoints_thread_created;index:idx_agent_checkpoints_run_created"`
}

type memoryPO struct {
	ID        int64          `gorm:"column:id;primaryKey"`
	ThreadID  int64          `gorm:"column:thread_id;index:idx_agent_thread_memories_thread_run_scope"`
	RunID     int64          `gorm:"column:run_id;index:idx_agent_thread_memories_thread_run_scope"`
	SpaceID   int64          `gorm:"column:space_id;index:idx_agent_thread_memories_space_scope"`
	Scope     string         `gorm:"column:scope;index:idx_agent_thread_memories_thread_run_scope;index:idx_agent_thread_memories_space_scope"`
	Content   string         `gorm:"column:content"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:json"`
	Score     float64        `gorm:"column:score"`
	ExpiresAt int64          `gorm:"column:expires_at;index:idx_agent_thread_memories_expires"`
	CreatedAt int64          `gorm:"column:created_at"`
	UpdatedAt int64          `gorm:"column:updated_at;index:idx_agent_thread_memories_updated"`
}

type tokenUsagePO struct {
	ID           int64          `gorm:"column:id;primaryKey"`
	ThreadID     int64          `gorm:"column:thread_id;index:idx_agent_token_usage_thread_run"`
	RunID        int64          `gorm:"column:run_id;index:idx_agent_token_usage_thread_run;index:idx_agent_token_usage_run_source"`
	SpaceID      int64          `gorm:"column:space_id;index:idx_agent_token_usage_space_source"`
	Source       string         `gorm:"column:source;index:idx_agent_token_usage_space_source;index:idx_agent_token_usage_run_source"`
	StepID       string         `gorm:"column:step_id"`
	StepIndex    int32          `gorm:"column:step_index"`
	StepName     string         `gorm:"column:step_name"`
	ModelName    string         `gorm:"column:model_name"`
	Provider     string         `gorm:"column:provider"`
	InputTokens  int64          `gorm:"column:input_tokens"`
	OutputTokens int64          `gorm:"column:output_tokens"`
	TotalTokens  int64          `gorm:"column:total_tokens"`
	CostMicros   int64          `gorm:"column:cost_micros"`
	Currency     string         `gorm:"column:currency"`
	Estimated    bool           `gorm:"column:estimated"`
	RawUsage     datatypes.JSON `gorm:"column:raw_usage;type:json"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt    int64          `gorm:"column:created_at;index:idx_agent_token_usage_created"`
}

func (threadPO) TableName() string {
	return "agent_threads"
}

func (messagePO) TableName() string {
	return "agent_thread_messages"
}

func (runPO) TableName() string {
	return "agent_runs"
}

func (runEventPO) TableName() string {
	return "agent_run_events"
}

func (checkpointPO) TableName() string {
	return "agent_checkpoints"
}

func (memoryPO) TableName() string {
	return "agent_thread_memories"
}

func (tokenUsagePO) TableName() string {
	return "agent_token_usage"
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

func (r *threadRepository) CreateRun(ctx context.Context, run *entity.Run) error {
	if run == nil {
		return fmt.Errorf("run is required")
	}

	now := time.Now().UnixMilli()
	if run.CreatedAt == 0 {
		run.CreatedAt = now
	}
	if run.UpdatedAt == 0 {
		run.UpdatedAt = run.CreatedAt
	}

	po, err := runToPO(run)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) GetRun(ctx context.Context, id int64) (*entity.Run, error) {
	var po runPO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error; err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) ListRuns(ctx context.Context, req ListRunsRequest) ([]*entity.Run, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&runPO{}).Where("thread_id = ?", req.ThreadID)
	if req.Status != nil {
		query = query.Where("status = ?", string(*req.Status))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*runPO, 0)
	if err := query.
		Order("created_at DESC, id DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	runs := make([]*entity.Run, 0, len(pos))
	for _, po := range pos {
		runs = append(runs, po.toEntity())
	}

	return runs, total, nil
}

func (r *threadRepository) CreateRunEvent(ctx context.Context, event *entity.RunEvent) error {
	if event == nil {
		return fmt.Errorf("run event is required")
	}

	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}

	po, err := runEventToPO(event)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) ListRunEvents(ctx context.Context, req ListRunEventsRequest) ([]*entity.RunEvent, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	query := r.db.WithContext(ctx).Model(&runEventPO{})
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	} else {
		query = query.Where("thread_id = ?", req.ThreadID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*runEventPO, 0)
	if err := query.
		Order("created_at ASC, id ASC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	events := make([]*entity.RunEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}

	return events, total, nil
}

func (r *threadRepository) CreateCheckpoint(ctx context.Context, checkpoint *entity.Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is required")
	}

	if checkpoint.CreatedAt == 0 {
		checkpoint.CreatedAt = time.Now().UnixMilli()
	}

	po, err := checkpointToPO(checkpoint)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) GetCheckpoint(ctx context.Context, checkpointID int64) (*entity.Checkpoint, error) {
	var po checkpointPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", checkpointID).
		First(&po).Error; err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) ListCheckpoints(ctx context.Context, req ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := r.db.WithContext(ctx).Model(&checkpointPO{}).Where("thread_id = ?", req.ThreadID)
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*checkpointPO, 0)
	if err := query.
		Order("created_at DESC, id DESC").
		Limit(int(limit)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	checkpoints := make([]*entity.Checkpoint, 0, len(pos))
	for _, po := range pos {
		checkpoints = append(checkpoints, po.toEntity())
	}

	return checkpoints, total, nil
}

func (r *threadRepository) GetLatestCheckpoint(ctx context.Context, threadID int64) (*entity.Checkpoint, error) {
	var po checkpointPO
	if err := r.db.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("created_at DESC, id DESC").
		First(&po).Error; err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) CreateMemory(ctx context.Context, memory *entity.Memory) error {
	if memory == nil {
		return fmt.Errorf("memory is required")
	}

	now := time.Now().UnixMilli()
	if memory.CreatedAt == 0 {
		memory.CreatedAt = now
	}
	if memory.UpdatedAt == 0 {
		memory.UpdatedAt = memory.CreatedAt
	}

	po, err := memoryToPO(memory)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) ListMemories(ctx context.Context, req ListMemoriesRequest) ([]*entity.Memory, int64, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	query := r.db.WithContext(ctx).
		Model(&memoryPO{}).
		Where("thread_id = ?", req.ThreadID).
		Where("(expires_at = 0 OR expires_at > ?)", now)
	if req.RunID > 0 {
		query = query.Where("(run_id = 0 OR run_id = ?)", req.RunID)
	} else {
		query = query.Where("run_id = 0")
	}
	if len(req.Scopes) > 0 {
		scopes := make([]string, 0, len(req.Scopes))
		for _, scope := range req.Scopes {
			if scope == "" {
				continue
			}
			scopes = append(scopes, string(scope))
		}
		if len(scopes) > 0 {
			query = query.Where("scope IN ?", scopes)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*memoryPO, 0)
	if err := query.
		Order("score DESC, updated_at DESC, id DESC").
		Limit(int(limit)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	memories := make([]*entity.Memory, 0, len(pos))
	for _, po := range pos {
		memories = append(memories, po.toEntity())
	}

	return memories, total, nil
}

func (r *threadRepository) CreateTokenUsage(ctx context.Context, usage *entity.TokenUsage) error {
	if usage == nil {
		return fmt.Errorf("token usage is required")
	}

	if usage.CreatedAt == 0 {
		usage.CreatedAt = time.Now().UnixMilli()
	}

	po, err := tokenUsageToPO(usage)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) ListTokenUsage(ctx context.Context, req ListTokenUsageRequest) ([]*entity.TokenUsage, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	query := r.db.WithContext(ctx).Model(&tokenUsagePO{})
	if req.ThreadID > 0 {
		query = query.Where("thread_id = ?", req.ThreadID)
	}
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}
	if req.Source != "" {
		query = query.Where("source = ?", string(req.Source))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*tokenUsagePO, 0)
	if err := query.
		Order("created_at ASC, id ASC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	usages := make([]*entity.TokenUsage, 0, len(pos))
	for _, po := range pos {
		usages = append(usages, po.toEntity())
	}

	return usages, total, nil
}

func (r *threadRepository) AggregateTokenUsage(ctx context.Context, req AggregateTokenUsageRequest) (*entity.TokenUsageAggregate, error) {
	query := r.db.WithContext(ctx).Model(&tokenUsagePO{})
	if req.ThreadID > 0 {
		query = query.Where("thread_id = ?", req.ThreadID)
	}
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}

	aggregate := &entity.TokenUsageAggregate{}
	err := query.Select(`
		COALESCE(SUM(input_tokens), 0) AS input_tokens,
		COALESCE(SUM(output_tokens), 0) AS output_tokens,
		COALESCE(SUM(total_tokens), 0) AS total_tokens,
		COALESCE(SUM(cost_micros), 0) AS cost_micros,
		COUNT(*) AS call_count,
		COALESCE(SUM(CASE WHEN source = ? THEN total_tokens ELSE 0 END), 0) AS lead_agent_tokens,
		COALESCE(SUM(CASE WHEN source = ? THEN total_tokens ELSE 0 END), 0) AS subagent_tokens,
		COALESCE(SUM(CASE WHEN source = ? THEN total_tokens ELSE 0 END), 0) AS middleware_tokens,
		COALESCE(SUM(CASE WHEN source = ? THEN total_tokens ELSE 0 END), 0) AS tool_tokens
	`,
		string(entity.TokenUsageSourceLeadAgent),
		string(entity.TokenUsageSourceSubagent),
		string(entity.TokenUsageSourceMiddleware),
		string(entity.TokenUsageSourceTool),
	).Scan(aggregate).Error
	if err != nil {
		return nil, err
	}

	return aggregate, nil
}

func (r *threadRepository) ClaimPendingRuns(ctx context.Context, req ClaimPendingRunsRequest) ([]*entity.Run, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	workerID := strings.TrimSpace(req.WorkerID)
	claimed := make([]*entity.Run, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&runPO{}).
			Where("status = ?", string(entity.RunStatusPending)).
			Order("created_at ASC, id ASC").
			Limit(int(limit))
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		pos := make([]*runPO, 0)
		if err := query.Find(&pos).Error; err != nil {
			return err
		}

		now := time.Now().UnixMilli()
		for _, po := range pos {
			db := tx.Model(&runPO{}).
				Where("id = ? AND status = ?", po.ID, string(entity.RunStatusPending)).
				Updates(map[string]any{
					"status":     string(entity.RunStatusRunning),
					"worker_id":  workerID,
					"started_at": now,
					"updated_at": now,
				})
			if db.Error != nil {
				return db.Error
			}
			if db.RowsAffected == 0 {
				continue
			}

			var updated runPO
			if err := tx.Where("id = ?", po.ID).First(&updated).Error; err != nil {
				return err
			}
			claimed = append(claimed, updated.toEntity())
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return claimed, nil
}

func (r *threadRepository) ClaimQueuedResumeRuns(ctx context.Context, req ClaimQueuedResumeRunsRequest) ([]*entity.Run, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	workerID := strings.TrimSpace(req.WorkerID)
	claimed := make([]*entity.Run, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := queuedResumeRunQuery(tx.Model(&runPO{})).
			Order("created_at ASC, id ASC").
			Limit(int(limit))
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		pos := make([]*runPO, 0)
		if err := query.Find(&pos).Error; err != nil {
			return err
		}

		now := time.Now().UnixMilli()
		for _, po := range pos {
			db := queuedResumeRunQuery(tx.Model(&runPO{}).Where("id = ?", po.ID)).
				Updates(map[string]any{
					"status":     string(entity.RunStatusRunning),
					"worker_id":  workerID,
					"started_at": now,
					"updated_at": now,
				})
			if db.Error != nil {
				return db.Error
			}
			if db.RowsAffected == 0 {
				continue
			}

			var updated runPO
			if err := tx.Where("id = ?", po.ID).First(&updated).Error; err != nil {
				return err
			}
			claimed = append(claimed, updated.toEntity())
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return claimed, nil
}

func queuedResumeRunQuery(db *gorm.DB) *gorm.DB {
	return db.
		Where("status = ?", string(entity.RunStatusQueued)).
		Where("JSON_EXTRACT(metadata, '$.checkpoint_resume') IS NOT NULL")
}

func (r *threadRepository) UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) error {
	now := time.Now().UnixMilli()
	updates := map[string]any{
		"status":        string(req.To),
		"error_code":    req.ErrorCode,
		"error_message": req.ErrorMessage,
		"updated_at":    now,
	}
	if isTerminalRunStatus(req.To) {
		updates["ended_at"] = now
	}

	query := r.db.WithContext(ctx).
		Model(&runPO{}).
		Where("id = ? AND status = ?", req.RunID, string(req.From))
	if workerID := strings.TrimSpace(req.WorkerID); workerID != "" {
		query = query.Where("worker_id = ?", workerID)
	}

	db := query.Updates(updates)
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return fmt.Errorf("update run status failed: run %d is not in status %s", req.RunID, req.From)
	}

	return nil
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

func runToPO(run *entity.Run) (*runPO, error) {
	command, err := requiredJSON("command", run.Command)
	if err != nil {
		return nil, err
	}
	input, err := requiredJSON("input", run.Input)
	if err != nil {
		return nil, err
	}
	config, err := requiredJSON("config", run.Config)
	if err != nil {
		return nil, err
	}
	runContext, err := requiredJSON("context", run.Context)
	if err != nil {
		return nil, err
	}
	metadata, err := requiredJSON("metadata", run.Metadata)
	if err != nil {
		return nil, err
	}
	streamMode, err := requiredJSON("stream_mode", run.StreamMode)
	if err != nil {
		return nil, err
	}

	return &runPO{
		ID:                run.ID,
		ThreadID:          run.ThreadID,
		SpaceID:           run.SpaceID,
		CreatorID:         run.CreatorID,
		AssistantID:       run.AssistantID,
		Status:            string(run.Status),
		Command:           command,
		Input:             input,
		Config:            config,
		Context:           runContext,
		Metadata:          metadata,
		StreamMode:        streamMode,
		MultitaskStrategy: run.MultitaskStrategy,
		OnDisconnect:      run.OnDisconnect,
		Durability:        run.Durability,
		IdempotencyKey:    stringPtrOrNil(run.IdempotencyKey),
		WorkerID:          run.WorkerID,
		ErrorCode:         run.ErrorCode,
		ErrorMessage:      run.ErrorMessage,
		StartedAt:         run.StartedAt,
		EndedAt:           run.EndedAt,
		CreatedAt:         run.CreatedAt,
		UpdatedAt:         run.UpdatedAt,
	}, nil
}

func (po *runPO) toEntity() *entity.Run {
	return &entity.Run{
		ID:                po.ID,
		ThreadID:          po.ThreadID,
		SpaceID:           po.SpaceID,
		CreatorID:         po.CreatorID,
		AssistantID:       po.AssistantID,
		Status:            entity.RunStatus(po.Status),
		Command:           jsonToString(po.Command),
		Input:             jsonToString(po.Input),
		Config:            jsonToString(po.Config),
		Context:           jsonToString(po.Context),
		Metadata:          jsonToString(po.Metadata),
		StreamMode:        jsonToString(po.StreamMode),
		MultitaskStrategy: po.MultitaskStrategy,
		OnDisconnect:      po.OnDisconnect,
		Durability:        po.Durability,
		IdempotencyKey:    stringFromPtr(po.IdempotencyKey),
		WorkerID:          po.WorkerID,
		ErrorCode:         po.ErrorCode,
		ErrorMessage:      po.ErrorMessage,
		StartedAt:         po.StartedAt,
		EndedAt:           po.EndedAt,
		CreatedAt:         po.CreatedAt,
		UpdatedAt:         po.UpdatedAt,
	}
}

func runEventToPO(event *entity.RunEvent) (*runEventPO, error) {
	payload, err := requiredJSON("payload", event.Payload)
	if err != nil {
		return nil, err
	}

	return &runEventPO{
		ID:        event.ID,
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		EventType: event.EventType,
		Payload:   payload,
		CreatedAt: event.CreatedAt,
	}, nil
}

func (po *runEventPO) toEntity() *entity.RunEvent {
	return &entity.RunEvent{
		ID:        po.ID,
		ThreadID:  po.ThreadID,
		RunID:     po.RunID,
		EventType: po.EventType,
		Payload:   jsonToString(po.Payload),
		CreatedAt: po.CreatedAt,
	}
}

func checkpointToPO(checkpoint *entity.Checkpoint) (*checkpointPO, error) {
	channelValues, err := requiredJSON("channel_values", checkpoint.ChannelValues)
	if err != nil {
		return nil, err
	}
	channelVersions, err := requiredJSON("channel_versions", checkpoint.ChannelVersions)
	if err != nil {
		return nil, err
	}
	pendingSends, err := requiredJSON("pending_sends", checkpoint.PendingSends)
	if err != nil {
		return nil, err
	}
	metadata, err := requiredJSON("metadata", checkpoint.Metadata)
	if err != nil {
		return nil, err
	}

	return &checkpointPO{
		ID:                 checkpoint.ID,
		ThreadID:           checkpoint.ThreadID,
		RunID:              checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       checkpoint.CheckpointNS,
		ChannelValues:      channelValues,
		ChannelVersions:    channelVersions,
		PendingSends:       pendingSends,
		Metadata:           metadata,
		CreatedAt:          checkpoint.CreatedAt,
	}, nil
}

func (po *checkpointPO) toEntity() *entity.Checkpoint {
	return &entity.Checkpoint{
		ID:                 po.ID,
		ThreadID:           po.ThreadID,
		RunID:              po.RunID,
		ParentCheckpointID: po.ParentCheckpointID,
		CheckpointNS:       po.CheckpointNS,
		ChannelValues:      jsonToString(po.ChannelValues),
		ChannelVersions:    jsonToString(po.ChannelVersions),
		PendingSends:       jsonToString(po.PendingSends),
		Metadata:           jsonToString(po.Metadata),
		CreatedAt:          po.CreatedAt,
	}
}

func memoryToPO(memory *entity.Memory) (*memoryPO, error) {
	metadata, err := optionalJSON("metadata", memory.Metadata)
	if err != nil {
		return nil, err
	}

	return &memoryPO{
		ID:        memory.ID,
		ThreadID:  memory.ThreadID,
		RunID:     memory.RunID,
		SpaceID:   memory.SpaceID,
		Scope:     string(memory.Scope),
		Content:   memory.Content,
		Metadata:  metadata,
		Score:     memory.Score,
		ExpiresAt: memory.ExpiresAt,
		CreatedAt: memory.CreatedAt,
		UpdatedAt: memory.UpdatedAt,
	}, nil
}

func (po *memoryPO) toEntity() *entity.Memory {
	return &entity.Memory{
		ID:        po.ID,
		ThreadID:  po.ThreadID,
		RunID:     po.RunID,
		SpaceID:   po.SpaceID,
		Scope:     entity.MemoryScope(po.Scope),
		Content:   po.Content,
		Metadata:  jsonToString(po.Metadata),
		Score:     po.Score,
		ExpiresAt: po.ExpiresAt,
		CreatedAt: po.CreatedAt,
		UpdatedAt: po.UpdatedAt,
	}
}

func tokenUsageToPO(usage *entity.TokenUsage) (*tokenUsagePO, error) {
	rawUsage, err := optionalJSON("raw_usage", usage.RawUsage)
	if err != nil {
		return nil, err
	}
	metadata, err := optionalJSON("metadata", usage.Metadata)
	if err != nil {
		return nil, err
	}

	return &tokenUsagePO{
		ID:           usage.ID,
		ThreadID:     usage.ThreadID,
		RunID:        usage.RunID,
		SpaceID:      usage.SpaceID,
		Source:       string(usage.Source),
		StepID:       usage.StepID,
		StepIndex:    usage.StepIndex,
		StepName:     usage.StepName,
		ModelName:    usage.ModelName,
		Provider:     usage.Provider,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostMicros:   usage.CostMicros,
		Currency:     usage.Currency,
		Estimated:    usage.Estimated,
		RawUsage:     rawUsage,
		Metadata:     metadata,
		CreatedAt:    usage.CreatedAt,
	}, nil
}

func (po *tokenUsagePO) toEntity() *entity.TokenUsage {
	return &entity.TokenUsage{
		ID:           po.ID,
		ThreadID:     po.ThreadID,
		RunID:        po.RunID,
		SpaceID:      po.SpaceID,
		Source:       entity.TokenUsageSource(po.Source),
		StepID:       po.StepID,
		StepIndex:    po.StepIndex,
		StepName:     po.StepName,
		ModelName:    po.ModelName,
		Provider:     po.Provider,
		InputTokens:  po.InputTokens,
		OutputTokens: po.OutputTokens,
		TotalTokens:  po.TotalTokens,
		CostMicros:   po.CostMicros,
		Currency:     po.Currency,
		Estimated:    po.Estimated,
		RawUsage:     jsonToString(po.RawUsage),
		Metadata:     jsonToString(po.Metadata),
		CreatedAt:    po.CreatedAt,
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

func requiredJSON(field, value string) (datatypes.JSON, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("%s is required", field)
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

func stringPtrOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func stringFromPtr(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func isTerminalRunStatus(status entity.RunStatus) bool {
	switch status {
	case entity.RunStatusSucceeded, entity.RunStatusFailed, entity.RunStatusCanceled:
		return true
	default:
		return false
	}
}
