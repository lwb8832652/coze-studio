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
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	ThreadID          int64          `gorm:"column:thread_id;index:idx_agent_runs_thread_created;index:idx_agent_runs_thread_kind,priority:1"`
	ParentRunID       int64          `gorm:"column:parent_run_id;index:idx_agent_runs_parent_created,priority:1"`
	SpaceID           int64          `gorm:"column:space_id;index:idx_agent_runs_space_status;uniqueIndex:uk_agent_runs_space_idempotency"`
	CreatorID         int64          `gorm:"column:creator_id"`
	AssistantID       string         `gorm:"column:assistant_id"`
	RunKind           string         `gorm:"column:run_kind;index:idx_agent_runs_thread_kind,priority:2"`
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
	CreatedAt         int64          `gorm:"column:created_at;index:idx_agent_runs_thread_created;index:idx_agent_runs_parent_created,priority:2;index:idx_agent_runs_thread_kind,priority:3"`
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
	ThreadID           int64          `gorm:"column:thread_id;index:idx_agent_checkpoints_thread_created;index:idx_agent_checkpoints_runtime_key,priority:1"`
	RunID              int64          `gorm:"column:run_id;index:idx_agent_checkpoints_run_created;index:idx_agent_checkpoints_runtime_key,priority:2"`
	ParentCheckpointID int64          `gorm:"column:parent_checkpoint_id"`
	CheckpointNS       string         `gorm:"column:checkpoint_ns"`
	RuntimeType        string         `gorm:"column:runtime_type;default:legacy;index:idx_agent_checkpoints_runtime_key,priority:3"`
	RuntimeKey         string         `gorm:"column:runtime_key;index:idx_agent_checkpoints_runtime_key,priority:4"`
	EnvelopeVersion    int32          `gorm:"column:envelope_version"`
	RuntimeDeletedAt   int64          `gorm:"column:runtime_deleted_at"`
	ChannelValues      datatypes.JSON `gorm:"column:channel_values;type:json"`
	ChannelVersions    datatypes.JSON `gorm:"column:channel_versions;type:json"`
	PendingSends       datatypes.JSON `gorm:"column:pending_sends;type:json"`
	Metadata           datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt          int64          `gorm:"column:created_at;index:idx_agent_checkpoints_thread_created;index:idx_agent_checkpoints_run_created;index:idx_agent_checkpoints_runtime_key,priority:5"`
}

type memoryPO struct {
	ID                   int64          `gorm:"column:id;primaryKey"`
	ThreadID             int64          `gorm:"column:thread_id;index:idx_agent_thread_memories_thread_run_scope"`
	RunID                int64          `gorm:"column:run_id;index:idx_agent_thread_memories_thread_run_scope"`
	SpaceID              int64          `gorm:"column:space_id;index:idx_agent_thread_memories_space_scope"`
	Scope                string         `gorm:"column:scope;index:idx_agent_thread_memories_thread_run_scope;index:idx_agent_thread_memories_space_scope"`
	Content              string         `gorm:"column:content"`
	Metadata             datatypes.JSON `gorm:"column:metadata;type:json"`
	Score                float64        `gorm:"column:score"`
	Confidence           float64        `gorm:"column:confidence"`
	SourceType           string         `gorm:"column:source_type;index:idx_agent_thread_memories_source"`
	SourceID             string         `gorm:"column:source_id;index:idx_agent_thread_memories_source"`
	CorrectionOfMemoryID int64          `gorm:"column:correction_of_memory_id;index:idx_agent_thread_memories_correction"`
	CorrectedAt          int64          `gorm:"column:corrected_at"`
	ExpiresAt            int64          `gorm:"column:expires_at;index:idx_agent_thread_memories_expires"`
	CreatedAt            int64          `gorm:"column:created_at"`
	UpdatedAt            int64          `gorm:"column:updated_at;index:idx_agent_thread_memories_updated"`
	DeletedAt            int64          `gorm:"column:deleted_at;index:idx_agent_thread_memories_deleted"`
}

type transcriptSnapshotPO struct {
	ID             int64          `gorm:"column:id;primaryKey"`
	ThreadID       int64          `gorm:"column:thread_id;index:idx_agent_transcripts_thread_created"`
	RunID          int64          `gorm:"column:run_id;index:idx_agent_transcripts_run_created;uniqueIndex:uk_agent_transcripts_run_key"`
	SpaceID        int64          `gorm:"column:space_id;index:idx_agent_transcripts_space_created"`
	Kind           string         `gorm:"column:kind"`
	Digest         string         `gorm:"column:digest"`
	IdempotencyKey string         `gorm:"column:idempotency_key;uniqueIndex:uk_agent_transcripts_run_key"`
	MessageCount   int32          `gorm:"column:message_count"`
	Messages       datatypes.JSON `gorm:"column:messages;type:json"`
	Metadata       datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt      int64          `gorm:"column:created_at;index:idx_agent_transcripts_thread_created;index:idx_agent_transcripts_run_created;index:idx_agent_transcripts_space_created"`
}

type memoryFlushJobPO struct {
	ID                   int64  `gorm:"column:id;primaryKey"`
	ThreadID             int64  `gorm:"column:thread_id;index:idx_agent_memory_flush_thread_created"`
	RunID                int64  `gorm:"column:run_id;uniqueIndex:uk_agent_memory_flush_run_key"`
	SpaceID              int64  `gorm:"column:space_id;index:idx_agent_memory_flush_space_created"`
	UserID               int64  `gorm:"column:user_id"`
	AssistantID          string `gorm:"column:assistant_id"`
	TranscriptSnapshotID int64  `gorm:"column:transcript_snapshot_id;index:idx_agent_memory_flush_snapshot"`
	IdempotencyKey       string `gorm:"column:idempotency_key;uniqueIndex:uk_agent_memory_flush_run_key"`
	Status               string `gorm:"column:status;index:idx_agent_memory_flush_pending,priority:1"`
	AttemptCount         int32  `gorm:"column:attempt_count"`
	WorkerID             string `gorm:"column:worker_id;index:idx_agent_memory_flush_worker"`
	LastError            string `gorm:"column:last_error"`
	AvailableAt          int64  `gorm:"column:available_at;index:idx_agent_memory_flush_pending,priority:2"`
	LeaseExpiresAt       int64  `gorm:"column:lease_expires_at;index:idx_agent_memory_flush_pending,priority:4"`
	StartedAt            int64  `gorm:"column:started_at"`
	EndedAt              int64  `gorm:"column:ended_at"`
	CreatedAt            int64  `gorm:"column:created_at;index:idx_agent_memory_flush_thread_created;index:idx_agent_memory_flush_space_created;index:idx_agent_memory_flush_pending,priority:3"`
	UpdatedAt            int64  `gorm:"column:updated_at"`
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

type agentFilePO struct {
	ID               int64          `gorm:"column:id;primaryKey"`
	SpaceID          int64          `gorm:"column:space_id;index:idx_agent_files_space_created"`
	UserID           int64          `gorm:"column:user_id"`
	ThreadID         int64          `gorm:"column:thread_id;index:idx_agent_files_thread_kind"`
	RunID            int64          `gorm:"column:run_id;uniqueIndex:uk_agent_files_run_path"`
	FileName         string         `gorm:"column:file_name"`
	OriginalFileName string         `gorm:"column:original_file_name"`
	FileKind         string         `gorm:"column:file_kind;index:idx_agent_files_thread_kind"`
	VirtualPath      string         `gorm:"column:virtual_path"`
	VirtualPathHash  string         `gorm:"column:virtual_path_hash;size:64;uniqueIndex:uk_agent_files_run_path"`
	ObjectURI        string         `gorm:"column:object_uri"`
	ContentType      string         `gorm:"column:content_type"`
	SizeBytes        int64          `gorm:"column:size_bytes"`
	Digest           string         `gorm:"column:digest"`
	Status           string         `gorm:"column:status"`
	Metadata         datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt        int64          `gorm:"column:created_at;index:idx_agent_files_space_created"`
	UpdatedAt        int64          `gorm:"column:updated_at"`
}

type agentArtifactPO struct {
	ID           int64          `gorm:"column:id;primaryKey"`
	SpaceID      int64          `gorm:"column:space_id"`
	UserID       int64          `gorm:"column:user_id"`
	ThreadID     int64          `gorm:"column:thread_id;index:idx_agent_artifacts_thread_created;index:idx_agent_artifacts_thread_active_created,priority:1"`
	RunID        int64          `gorm:"column:run_id;index:idx_agent_artifacts_run_created;index:idx_agent_artifacts_run_active_created,priority:1"`
	FileID       int64          `gorm:"column:file_id;uniqueIndex:uk_agent_artifacts_file"`
	Title        string         `gorm:"column:title"`
	ArtifactType string         `gorm:"column:artifact_type"`
	VirtualPath  string         `gorm:"column:virtual_path"`
	ObjectURI    string         `gorm:"column:object_uri"`
	ContentType  string         `gorm:"column:content_type"`
	SizeBytes    int64          `gorm:"column:size_bytes"`
	PreviewMode  string         `gorm:"column:preview_mode"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:json"`
	CreatedAt    int64          `gorm:"column:created_at;index:idx_agent_artifacts_thread_created;index:idx_agent_artifacts_run_created;index:idx_agent_artifacts_thread_active_created,priority:3;index:idx_agent_artifacts_run_active_created,priority:3"`
	UpdatedAt    int64          `gorm:"column:updated_at"`
	DeletedAt    int64          `gorm:"column:deleted_at;index:idx_agent_artifacts_thread_active_created,priority:2;index:idx_agent_artifacts_run_active_created,priority:2"`
}

type agentArtifactScanJobPO struct {
	ID             int64  `gorm:"column:id;primaryKey"`
	ThreadID       int64  `gorm:"column:thread_id;index:idx_agent_artifact_scan_jobs_thread_created"`
	RunID          int64  `gorm:"column:run_id;index:idx_agent_artifact_scan_jobs_run_created"`
	SpaceID        int64  `gorm:"column:space_id;index:idx_agent_artifact_scan_jobs_space_created"`
	UserID         int64  `gorm:"column:user_id"`
	ArtifactID     int64  `gorm:"column:artifact_id;uniqueIndex:uk_agent_artifact_scan_jobs_artifact_key"`
	FileID         int64  `gorm:"column:file_id;index:idx_agent_artifact_scan_jobs_file"`
	Scanner        string `gorm:"column:scanner;index:idx_agent_artifact_scan_jobs_pending,priority:2;index:idx_agent_artifact_scan_jobs_lease,priority:2"`
	IdempotencyKey string `gorm:"column:idempotency_key;uniqueIndex:uk_agent_artifact_scan_jobs_artifact_key"`
	Status         string `gorm:"column:status;index:idx_agent_artifact_scan_jobs_pending,priority:1"`
	WorkerID       string `gorm:"column:worker_id"`
	AttemptCount   int32  `gorm:"column:attempt_count"`
	LastError      string `gorm:"column:last_error"`
	AvailableAt    int64  `gorm:"column:available_at;index:idx_agent_artifact_scan_jobs_pending,priority:3"`
	LeaseExpiresAt int64  `gorm:"column:lease_expires_at;index:idx_agent_artifact_scan_jobs_lease,priority:3"`
	StartedAt      int64  `gorm:"column:started_at"`
	EndedAt        int64  `gorm:"column:ended_at"`
	CreatedAt      int64  `gorm:"column:created_at;index:idx_agent_artifact_scan_jobs_thread_created;index:idx_agent_artifact_scan_jobs_run_created;index:idx_agent_artifact_scan_jobs_space_created;index:idx_agent_artifact_scan_jobs_pending,priority:4;index:idx_agent_artifact_scan_jobs_lease,priority:4"`
	UpdatedAt      int64  `gorm:"column:updated_at"`
}

type agentRunPlanPO struct {
	RunID         int64 `gorm:"column:run_id;primaryKey"`
	ThreadID      int64 `gorm:"column:thread_id;index:idx_agent_run_plans_thread_updated"`
	SpaceID       int64 `gorm:"column:space_id;index:idx_agent_run_plans_space_updated"`
	UserID        int64 `gorm:"column:user_id"`
	HighWatermark int64 `gorm:"column:high_watermark"`
	Revision      int64 `gorm:"column:revision"`
	CreatedAt     int64 `gorm:"column:created_at"`
	UpdatedAt     int64 `gorm:"column:updated_at;index:idx_agent_run_plans_thread_updated;index:idx_agent_run_plans_space_updated"`
}

type agentRunPlanItemPO struct {
	ID          int64          `gorm:"column:id;primaryKey"`
	RunID       int64          `gorm:"column:run_id;uniqueIndex:uk_agent_run_plan_items_run_task;index:idx_agent_run_plan_items_active"`
	TaskID      int64          `gorm:"column:task_id;uniqueIndex:uk_agent_run_plan_items_run_task"`
	Subject     string         `gorm:"column:subject"`
	Description string         `gorm:"column:description"`
	Status      string         `gorm:"column:status"`
	ActiveForm  string         `gorm:"column:active_form"`
	Owner       string         `gorm:"column:owner"`
	Blocks      datatypes.JSON `gorm:"column:blocks;type:json"`
	BlockedBy   datatypes.JSON `gorm:"column:blocked_by;type:json"`
	Metadata    datatypes.JSON `gorm:"column:metadata;type:json"`
	Active      bool           `gorm:"column:active;index:idx_agent_run_plan_items_active"`
	Version     int64          `gorm:"column:version"`
	CreatedAt   int64          `gorm:"column:created_at"`
	UpdatedAt   int64          `gorm:"column:updated_at;index:idx_agent_run_plan_items_updated"`
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

func (transcriptSnapshotPO) TableName() string {
	return "agent_transcript_snapshots"
}

func (memoryFlushJobPO) TableName() string {
	return "agent_memory_flush_jobs"
}

func (tokenUsagePO) TableName() string {
	return "agent_token_usage"
}

func (agentFilePO) TableName() string {
	return "agent_files"
}

func (agentArtifactPO) TableName() string {
	return "agent_artifacts"
}

func (agentArtifactScanJobPO) TableName() string {
	return "agent_artifact_scan_jobs"
}

func (agentRunPlanPO) TableName() string {
	return "agent_run_plans"
}

func (agentRunPlanItemPO) TableName() string {
	return "agent_run_plan_items"
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

func (r *threadRepository) GetRunByIdempotencyKey(
	ctx context.Context,
	spaceID int64,
	idempotencyKey string,
) (*entity.Run, error) {
	var po runPO
	err := r.db.WithContext(ctx).
		Where(
			"space_id = ? AND idempotency_key = ?",
			spaceID,
			strings.TrimSpace(idempotencyKey),
		).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
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
	if req.ParentRunID != nil {
		query = query.Where("parent_run_id = ?", *req.ParentRunID)
	} else if !req.IncludeChildRuns {
		query = query.Where("parent_run_id = ?", 0)
	}
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
		Order("id ASC").
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

func (r *threadRepository) GetLatestRuntimeCheckpoint(
	ctx context.Context,
	threadID, runID int64,
	runtimeType, runtimeKey string,
) (*entity.Checkpoint, error) {
	var po checkpointPO
	err := r.db.WithContext(ctx).
		Where(
			"thread_id = ? AND run_id = ? AND runtime_type = ? AND runtime_key = ? AND runtime_deleted_at = 0",
			threadID,
			runID,
			strings.TrimSpace(runtimeType),
			strings.TrimSpace(runtimeKey),
		).
		Order("created_at DESC, id DESC").
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) DeleteRuntimeCheckpoint(
	ctx context.Context,
	threadID, runID int64,
	runtimeType, runtimeKey string,
	deletedAt int64,
) error {
	if deletedAt <= 0 {
		return fmt.Errorf("runtime checkpoint deleted time is required")
	}

	return r.db.WithContext(ctx).
		Model(&checkpointPO{}).
		Where(
			"thread_id = ? AND run_id = ? AND runtime_type = ? AND runtime_key = ? AND runtime_deleted_at = 0",
			threadID,
			runID,
			strings.TrimSpace(runtimeType),
			strings.TrimSpace(runtimeKey),
		).
		Update("runtime_deleted_at", deletedAt).Error
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

func (r *threadRepository) CreateOrGetMemoryBySource(
	ctx context.Context,
	memory *entity.Memory,
) (*entity.Memory, bool, error) {
	if memory == nil {
		return nil, false, fmt.Errorf("memory is required")
	}
	sourceType := strings.TrimSpace(memory.SourceType)
	sourceID := strings.TrimSpace(memory.SourceID)
	if sourceType == "" || sourceID == "" {
		if err := r.CreateMemory(ctx, memory); err != nil {
			return nil, false, err
		}

		return memory, true, nil
	}

	existing, err := r.getMemoryBySource(ctx, memory.ThreadID, sourceType, sourceID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	now := time.Now().UnixMilli()
	if memory.CreatedAt == 0 {
		memory.CreatedAt = now
	}
	if memory.UpdatedAt == 0 {
		memory.UpdatedAt = memory.CreatedAt
	}
	memory.SourceType = sourceType
	memory.SourceID = sourceID
	po, err := memoryToPO(memory)
	if err != nil {
		return nil, false, err
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected > 0 {
		return memory, true, nil
	}

	existing, err = r.getMemoryBySource(ctx, memory.ThreadID, sourceType, sourceID)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

func (r *threadRepository) getMemoryBySource(
	ctx context.Context,
	threadID int64,
	sourceType string,
	sourceID string,
) (*entity.Memory, error) {
	var po memoryPO
	err := r.db.WithContext(ctx).
		Where("thread_id = ? AND source_type = ? AND source_id = ?", threadID, sourceType, sourceID).
		Order("id ASC").
		First(&po).Error
	if err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) ListMemories(ctx context.Context, req ListMemoriesRequest) ([]*entity.Memory, int64, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	page := req.Page
	pageSize := req.PageSize
	usePaging := page > 0 || pageSize > 0
	if usePaging {
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 20
		}
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	query := r.db.WithContext(ctx).
		Model(&memoryPO{}).
		Where("thread_id = ?", req.ThreadID)
	if !req.IncludeDeleted {
		query = query.Where("deleted_at = 0")
	}
	if !req.IncludeExpired {
		query = query.Where("(expires_at = 0 OR expires_at > ?)", now)
	}
	if req.RunID > 0 {
		query = query.Where("(run_id = 0 OR run_id = ?)", req.RunID)
	} else if !usePaging {
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
	if text := strings.TrimSpace(req.Query); text != "" {
		pattern := "%" + escapeSQLLike(text) + "%"
		query = query.Where(
			"(content LIKE ? ESCAPE '\\' OR source_type LIKE ? ESCAPE '\\' OR source_id LIKE ? ESCAPE '\\')",
			pattern,
			pattern,
			pattern,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*memoryPO, 0)
	find := query.
		Order("score DESC, updated_at DESC, id DESC").
		Limit(int(limit))
	if usePaging {
		find = query.
			Order("score DESC, updated_at DESC, id DESC").
			Limit(int(pageSize)).
			Offset(int((page - 1) * pageSize))
	}
	if err := find.Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	memories := make([]*entity.Memory, 0, len(pos))
	for _, po := range pos {
		memories = append(memories, po.toEntity())
	}

	return memories, total, nil
}

func (r *threadRepository) UpdateMemory(
	ctx context.Context,
	req UpdateMemoryRequest,
) (*entity.Memory, bool, error) {
	metadata, err := optionalJSON("metadata", req.Metadata)
	if err != nil {
		return nil, false, err
	}
	updatedAt := req.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UnixMilli()
	}

	updateResult := r.db.WithContext(ctx).
		Model(&memoryPO{}).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", req.ThreadID, req.MemoryID).
		Updates(map[string]any{
			"run_id":                  req.RunID,
			"scope":                   string(req.Scope),
			"content":                 req.Content,
			"metadata":                metadata,
			"score":                   req.Score,
			"confidence":              req.Confidence,
			"source_type":             req.SourceType,
			"source_id":               req.SourceID,
			"correction_of_memory_id": req.CorrectionOfMemoryID,
			"corrected_at":            req.CorrectedAt,
			"expires_at":              req.ExpiresAt,
			"updated_at":              updatedAt,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po memoryPO
	if err := r.db.WithContext(ctx).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", req.ThreadID, req.MemoryID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func (r *threadRepository) DeleteMemory(ctx context.Context, req DeleteMemoryRequest) (bool, error) {
	deletedAt := req.DeletedAt
	if deletedAt <= 0 {
		deletedAt = time.Now().UnixMilli()
	}
	result := r.db.WithContext(ctx).
		Model(&memoryPO{}).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", req.ThreadID, req.MemoryID).
		Updates(map[string]any{
			"deleted_at": deletedAt,
			"updated_at": deletedAt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *threadRepository) RestoreMemory(
	ctx context.Context,
	req RestoreMemoryRequest,
) (*entity.Memory, bool, error) {
	restoredAt := req.RestoredAt
	if restoredAt <= 0 {
		restoredAt = time.Now().UnixMilli()
	}

	result := r.db.WithContext(ctx).
		Model(&memoryPO{}).
		Where("thread_id = ? AND id = ? AND deleted_at > 0", req.ThreadID, req.MemoryID).
		Updates(map[string]any{
			"deleted_at": int64(0),
			"updated_at": restoredAt,
		})
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}

	var po memoryPO
	if err := r.db.WithContext(ctx).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", req.ThreadID, req.MemoryID).
		First(&po).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func (r *threadRepository) ClearMemories(ctx context.Context, req ClearMemoriesRequest) (int64, error) {
	deletedAt := req.DeletedAt
	if deletedAt <= 0 {
		deletedAt = time.Now().UnixMilli()
	}
	query := r.db.WithContext(ctx).
		Model(&memoryPO{}).
		Where("thread_id = ? AND deleted_at = 0", req.ThreadID)
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
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

	result := query.Updates(map[string]any{
		"deleted_at": deletedAt,
		"updated_at": deletedAt,
	})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (r *threadRepository) CreateOrGetTranscriptSnapshot(
	ctx context.Context,
	snapshot *entity.TranscriptSnapshot,
) (*entity.TranscriptSnapshot, bool, error) {
	if snapshot == nil {
		return nil, false, fmt.Errorf("transcript snapshot is required")
	}
	if snapshot.CreatedAt == 0 {
		snapshot.CreatedAt = time.Now().UnixMilli()
	}
	po, err := transcriptSnapshotToPO(snapshot)
	if err != nil {
		return nil, false, err
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected > 0

	stored := &transcriptSnapshotPO{}
	if err := r.db.WithContext(ctx).
		Where(
			"run_id = ? AND idempotency_key = ?",
			snapshot.RunID,
			snapshot.IdempotencyKey,
		).
		First(stored).Error; err != nil {
		return nil, false, err
	}
	return stored.toEntity(), created, nil
}

func (r *threadRepository) GetTranscriptSnapshot(
	ctx context.Context,
	snapshotID int64,
) (*entity.TranscriptSnapshot, error) {
	stored := &transcriptSnapshotPO{}
	if err := r.db.WithContext(ctx).
		Where("id = ?", snapshotID).
		First(stored).Error; err != nil {
		return nil, err
	}
	return stored.toEntity(), nil
}

func (r *threadRepository) CreateOrGetMemoryFlushJob(
	ctx context.Context,
	job *entity.MemoryFlushJob,
) (*entity.MemoryFlushJob, bool, error) {
	if job == nil {
		return nil, false, fmt.Errorf("memory flush job is required")
	}
	now := time.Now().UnixMilli()
	if job.CreatedAt == 0 {
		job.CreatedAt = now
	}
	if job.UpdatedAt == 0 {
		job.UpdatedAt = job.CreatedAt
	}
	if job.AvailableAt == 0 {
		job.AvailableAt = job.CreatedAt
	}
	po := memoryFlushJobToPO(job)

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected > 0

	stored := &memoryFlushJobPO{}
	if err := r.db.WithContext(ctx).
		Where(
			"run_id = ? AND idempotency_key = ?",
			job.RunID,
			job.IdempotencyKey,
		).
		First(stored).Error; err != nil {
		return nil, false, err
	}
	return stored.toEntity(), created, nil
}

func (r *threadRepository) ClaimMemoryFlushJobs(
	ctx context.Context,
	req ClaimMemoryFlushJobsRequest,
) ([]*entity.MemoryFlushJob, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	workerID := strings.TrimSpace(req.WorkerID)
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	leaseExpiresAt := req.LeaseExpiresAt
	if leaseExpiresAt <= now {
		leaseExpiresAt = now + 300000
	}

	claimed := make([]*entity.MemoryFlushJob, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := claimableMemoryFlushJobQuery(
			tx.Model(&memoryFlushJobPO{}),
			now,
		).
			Order("available_at ASC, created_at ASC, id ASC").
			Limit(int(limit))
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		pos := make([]*memoryFlushJobPO, 0)
		if err := query.Find(&pos).Error; err != nil {
			return err
		}

		for _, po := range pos {
			updates := map[string]any{
				"status":           string(entity.MemoryFlushJobStatusProcessing),
				"worker_id":        workerID,
				"lease_expires_at": leaseExpiresAt,
				"attempt_count":    gorm.Expr("attempt_count + ?", 1),
				"updated_at":       now,
			}
			if po.StartedAt == 0 {
				updates["started_at"] = now
			}
			db := claimableMemoryFlushJobQuery(
				tx.Model(&memoryFlushJobPO{}).Where("id = ?", po.ID),
				now,
			).Updates(updates)
			if db.Error != nil {
				return db.Error
			}
			if db.RowsAffected == 0 {
				continue
			}

			var updated memoryFlushJobPO
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

func claimableMemoryFlushJobQuery(db *gorm.DB, now int64) *gorm.DB {
	return db.Where(
		"((status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at > 0 AND lease_expires_at <= ?))",
		string(entity.MemoryFlushJobStatusPending),
		now,
		string(entity.MemoryFlushJobStatusProcessing),
		now,
	)
}

func (r *threadRepository) CompleteMemoryFlushJob(
	ctx context.Context,
	req CompleteMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	return r.finishMemoryFlushJob(ctx, finishMemoryFlushJobRequest{
		JobID:     req.JobID,
		WorkerID:  strings.TrimSpace(req.WorkerID),
		Status:    entity.MemoryFlushJobStatusSucceeded,
		LastError: "",
		Now:       now,
	})
}

func (r *threadRepository) RetryMemoryFlushJob(
	ctx context.Context,
	req RetryMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	availableAt := req.AvailableAt
	if availableAt <= now {
		availableAt = now
	}
	updateResult := r.db.WithContext(ctx).
		Model(&memoryFlushJobPO{}).
		Where("id = ?", req.JobID).
		Where("worker_id = ?", strings.TrimSpace(req.WorkerID)).
		Where("status = ?", string(entity.MemoryFlushJobStatusProcessing)).
		Where("lease_expires_at > ?", now).
		Updates(map[string]any{
			"status":           string(entity.MemoryFlushJobStatusPending),
			"worker_id":        "",
			"last_error":       truncateMemoryFlushJobError(req.ErrorText),
			"available_at":     availableAt,
			"lease_expires_at": 0,
			"ended_at":         0,
			"updated_at":       now,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po memoryFlushJobPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", req.JobID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func (r *threadRepository) FailMemoryFlushJob(
	ctx context.Context,
	req FailMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	return r.finishMemoryFlushJob(ctx, finishMemoryFlushJobRequest{
		JobID:     req.JobID,
		WorkerID:  strings.TrimSpace(req.WorkerID),
		Status:    entity.MemoryFlushJobStatusFailed,
		LastError: truncateMemoryFlushJobError(req.ErrorText),
		Now:       now,
	})
}

type finishMemoryFlushJobRequest struct {
	JobID     int64
	WorkerID  string
	Status    entity.MemoryFlushJobStatus
	LastError string
	Now       int64
}

func (r *threadRepository) finishMemoryFlushJob(
	ctx context.Context,
	req finishMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	updateResult := r.db.WithContext(ctx).
		Model(&memoryFlushJobPO{}).
		Where("id = ?", req.JobID).
		Where("worker_id = ?", req.WorkerID).
		Where("status = ?", string(entity.MemoryFlushJobStatusProcessing)).
		Where("lease_expires_at > ?", req.Now).
		Updates(map[string]any{
			"status":           string(req.Status),
			"last_error":       req.LastError,
			"lease_expires_at": 0,
			"ended_at":         req.Now,
			"updated_at":       req.Now,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po memoryFlushJobPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", req.JobID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func truncateMemoryFlushJobError(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return string(runes)
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

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po).Error
}

func (r *threadRepository) UpsertRuntimeFile(
	ctx context.Context,
	file *entity.AgentFile,
) (*entity.AgentFile, bool, error) {
	if file == nil {
		return nil, false, fmt.Errorf("runtime file is required")
	}
	po, err := agentFileToPO(file)
	if err != nil {
		return nil, false, err
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected > 0
	if !created {
		updates := map[string]any{
			"file_name":          po.FileName,
			"original_file_name": po.OriginalFileName,
			"file_kind":          po.FileKind,
			"object_uri":         po.ObjectURI,
			"content_type":       po.ContentType,
			"size_bytes":         po.SizeBytes,
			"digest":             po.Digest,
			"status":             po.Status,
			"metadata":           po.Metadata,
			"updated_at":         po.UpdatedAt,
		}
		updateResult := r.db.WithContext(ctx).
			Model(&agentFilePO{}).
			Where(
				"run_id = ? AND virtual_path_hash = ? AND virtual_path = ?",
				po.RunID,
				po.VirtualPathHash,
				po.VirtualPath,
			).
			Updates(updates)
		if updateResult.Error != nil {
			return nil, false, updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return nil, false, fmt.Errorf(
				"runtime file upsert lost row for run %d path %s",
				po.RunID,
				po.VirtualPath,
			)
		}
	}

	stored := &agentFilePO{}
	if err := r.db.WithContext(ctx).
		Where(
			"run_id = ? AND virtual_path_hash = ? AND virtual_path = ?",
			po.RunID,
			po.VirtualPathHash,
			po.VirtualPath,
		).
		First(stored).Error; err != nil {
		return nil, false, err
	}
	return stored.toEntity(), created, nil
}

func (r *threadRepository) GetRuntimeFile(
	ctx context.Context,
	runID int64,
	virtualPath string,
) (*entity.AgentFile, error) {
	var stored agentFilePO
	virtualPathHash := agentFileVirtualPathHash(virtualPath)
	err := r.db.WithContext(ctx).
		Where(
			"run_id = ? AND virtual_path_hash = ? AND virtual_path = ?",
			runID,
			virtualPathHash,
			virtualPath,
		).
		First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return stored.toEntity(), nil
}

func (r *threadRepository) GetFileByID(
	ctx context.Context,
	id int64,
) (*entity.AgentFile, error) {
	var stored agentFilePO
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return stored.toEntity(), nil
}

func (r *threadRepository) UpsertArtifact(
	ctx context.Context,
	artifact *entity.AgentArtifact,
) (*entity.AgentArtifact, bool, error) {
	if artifact == nil {
		return nil, false, fmt.Errorf("artifact is required")
	}
	po, err := agentArtifactToPO(artifact)
	if err != nil {
		return nil, false, err
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected > 0
	if !created {
		updates := map[string]any{
			"space_id":      po.SpaceID,
			"user_id":       po.UserID,
			"thread_id":     po.ThreadID,
			"run_id":        po.RunID,
			"title":         po.Title,
			"artifact_type": po.ArtifactType,
			"virtual_path":  po.VirtualPath,
			"object_uri":    po.ObjectURI,
			"content_type":  po.ContentType,
			"size_bytes":    po.SizeBytes,
			"preview_mode":  po.PreviewMode,
			"metadata":      po.Metadata,
			"updated_at":    po.UpdatedAt,
			"deleted_at":    po.DeletedAt,
		}
		updateResult := r.db.WithContext(ctx).
			Model(&agentArtifactPO{}).
			Where("file_id = ?", po.FileID).
			Updates(updates)
		if updateResult.Error != nil {
			return nil, false, updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return nil, false, fmt.Errorf(
				"artifact upsert lost row for file %d",
				po.FileID,
			)
		}
	}

	stored := &agentArtifactPO{}
	if err := r.db.WithContext(ctx).
		Where("file_id = ?", po.FileID).
		First(stored).Error; err != nil {
		return nil, false, err
	}
	return stored.toEntity(), created, nil
}

func (r *threadRepository) GetArtifact(
	ctx context.Context,
	threadID int64,
	artifactID int64,
) (*entity.AgentArtifact, error) {
	var po agentArtifactPO
	err := r.db.WithContext(ctx).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", threadID, artifactID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *threadRepository) DeleteArtifact(
	ctx context.Context,
	threadID int64,
	artifactID int64,
	deletedAt int64,
) (*entity.AgentArtifact, bool, error) {
	var po agentArtifactPO
	err := r.db.WithContext(ctx).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", threadID, artifactID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	updateResult := r.db.WithContext(ctx).
		Model(&agentArtifactPO{}).
		Where("id = ? AND deleted_at = 0", po.ID).
		Updates(map[string]any{
			"deleted_at": deletedAt,
			"updated_at": deletedAt,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	po.DeletedAt = deletedAt
	po.UpdatedAt = deletedAt
	return po.toEntity(), true, nil
}

func (r *threadRepository) RestoreArtifact(
	ctx context.Context,
	threadID int64,
	artifactID int64,
	restoredAt int64,
) (*entity.AgentArtifact, bool, error) {
	var po agentArtifactPO
	err := r.db.WithContext(ctx).
		Where("thread_id = ? AND id = ? AND deleted_at > 0", threadID, artifactID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	updateResult := r.db.WithContext(ctx).
		Model(&agentArtifactPO{}).
		Where("id = ? AND deleted_at > 0", po.ID).
		Updates(map[string]any{
			"deleted_at": int64(0),
			"updated_at": restoredAt,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	po.DeletedAt = 0
	po.UpdatedAt = restoredAt
	return po.toEntity(), true, nil
}

func (r *threadRepository) ListDeletedArtifactCleanupCandidates(
	ctx context.Context,
	req ListDeletedArtifactCleanupCandidatesRequest,
) ([]*entity.AgentArtifact, error) {
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if req.CutoffDeletedAt <= 0 {
		return []*entity.AgentArtifact{}, nil
	}

	pos := make([]*agentArtifactPO, 0)
	err := r.db.WithContext(ctx).
		Model(&agentArtifactPO{}).
		Select("agent_artifacts.*").
		Joins(
			"JOIN agent_files ON agent_files.id = agent_artifacts.file_id AND agent_files.object_uri = agent_artifacts.object_uri",
		).
		Where("agent_artifacts.deleted_at > 0").
		Where("agent_artifacts.deleted_at <= ?", req.CutoffDeletedAt).
		Where("agent_files.status = ?", string(entity.AgentFileStatusActive)).
		Order("agent_artifacts.deleted_at ASC, agent_artifacts.id ASC").
		Limit(int(limit)).
		Find(&pos).Error
	if err != nil {
		return nil, err
	}

	artifacts := make([]*entity.AgentArtifact, 0, len(pos))
	for _, po := range pos {
		artifacts = append(artifacts, po.toEntity())
	}
	return artifacts, nil
}

func (r *threadRepository) MarkArtifactFileDeleted(
	ctx context.Context,
	req MarkArtifactFileDeletedRequest,
) (bool, error) {
	if req.FileID <= 0 || req.ObjectURI == "" || req.DeletedAt <= 0 {
		return false, nil
	}
	result := r.db.WithContext(ctx).
		Model(&agentFilePO{}).
		Where("id = ? AND object_uri = ? AND status = ?",
			req.FileID,
			req.ObjectURI,
			string(entity.AgentFileStatusActive),
		).
		Updates(map[string]any{
			"status":     string(entity.AgentFileStatusDeleted),
			"updated_at": req.DeletedAt,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *threadRepository) UpdateArtifactScanMetadata(
	ctx context.Context,
	threadID int64,
	artifactID int64,
	metadata string,
	updatedAt int64,
) (*entity.AgentArtifact, bool, error) {
	metadataJSON, err := requiredJSON("metadata", metadata)
	if err != nil {
		return nil, false, err
	}
	updateResult := r.db.WithContext(ctx).
		Model(&agentArtifactPO{}).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", threadID, artifactID).
		Updates(map[string]any{
			"metadata":   metadataJSON,
			"updated_at": updatedAt,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po agentArtifactPO
	if err := r.db.WithContext(ctx).
		Where("thread_id = ? AND id = ? AND deleted_at = 0", threadID, artifactID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func (r *threadRepository) CreateOrGetArtifactScanJob(
	ctx context.Context,
	job *entity.ArtifactScanJob,
) (*entity.ArtifactScanJob, bool, error) {
	if job == nil {
		return nil, false, fmt.Errorf("artifact scan job is required")
	}
	now := time.Now().UnixMilli()
	if job.CreatedAt == 0 {
		job.CreatedAt = now
	}
	if job.UpdatedAt == 0 {
		job.UpdatedAt = job.CreatedAt
	}
	if job.AvailableAt == 0 {
		job.AvailableAt = job.CreatedAt
	}
	po := artifactScanJobToPO(job)

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(po)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected > 0

	stored := &agentArtifactScanJobPO{}
	if err := r.db.WithContext(ctx).
		Where(
			"artifact_id = ? AND idempotency_key = ?",
			job.ArtifactID,
			job.IdempotencyKey,
		).
		First(stored).Error; err != nil {
		return nil, false, err
	}
	return stored.toEntity(), created, nil
}

func (r *threadRepository) ClaimArtifactScanJobs(
	ctx context.Context,
	req ClaimArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	scanner := strings.TrimSpace(req.Scanner)
	workerID := strings.TrimSpace(req.WorkerID)
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	leaseExpiresAt := req.LeaseExpiresAt
	if leaseExpiresAt <= now {
		leaseExpiresAt = now + 300000
	}

	claimed := make([]*entity.ArtifactScanJob, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := claimableArtifactScanJobQuery(
			tx.Model(&agentArtifactScanJobPO{}),
			scanner,
			now,
		).
			Order("available_at ASC, created_at ASC, id ASC").
			Limit(int(limit))
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		pos := make([]*agentArtifactScanJobPO, 0)
		if err := query.Find(&pos).Error; err != nil {
			return err
		}

		for _, po := range pos {
			updates := map[string]any{
				"status":           string(entity.ArtifactScanJobStatusProcessing),
				"worker_id":        workerID,
				"lease_expires_at": leaseExpiresAt,
				"attempt_count":    gorm.Expr("attempt_count + ?", 1),
				"updated_at":       now,
			}
			if po.StartedAt == 0 {
				updates["started_at"] = now
			}
			db := claimableArtifactScanJobQuery(
				tx.Model(&agentArtifactScanJobPO{}).Where("id = ?", po.ID),
				scanner,
				now,
			).Updates(updates)
			if db.Error != nil {
				return db.Error
			}
			if db.RowsAffected == 0 {
				continue
			}

			var updated agentArtifactScanJobPO
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

func claimableArtifactScanJobQuery(
	db *gorm.DB,
	scanner string,
	now int64,
) *gorm.DB {
	return db.
		Where("scanner = ?", scanner).
		Where(
			"((status = ? AND available_at <= ?) OR (status = ? AND lease_expires_at > 0 AND lease_expires_at <= ?))",
			string(entity.ArtifactScanJobStatusPending),
			now,
			string(entity.ArtifactScanJobStatusProcessing),
			now,
		)
}

func (r *threadRepository) GetArtifactScanJob(
	ctx context.Context,
	jobID int64,
) (*entity.ArtifactScanJob, error) {
	var po agentArtifactScanJobPO
	err := r.db.WithContext(ctx).
		Where("id = ?", jobID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *threadRepository) CompleteArtifactScanJob(
	ctx context.Context,
	req CompleteArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	return r.finishArtifactScanJob(ctx, finishArtifactScanJobRequest{
		JobID:     req.JobID,
		WorkerID:  strings.TrimSpace(req.WorkerID),
		Status:    entity.ArtifactScanJobStatusSucceeded,
		LastError: "",
		Now:       now,
	})
}

func (r *threadRepository) FailArtifactScanJob(
	ctx context.Context,
	req FailArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	return r.finishArtifactScanJob(ctx, finishArtifactScanJobRequest{
		JobID:     req.JobID,
		WorkerID:  strings.TrimSpace(req.WorkerID),
		Status:    entity.ArtifactScanJobStatusFailed,
		LastError: truncateArtifactScanJobError(req.ErrorText),
		Now:       now,
	})
}

func (r *threadRepository) RetryArtifactScanJob(
	ctx context.Context,
	req RetryArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	availableAt := req.AvailableAt
	if availableAt <= now {
		availableAt = now
	}
	updateResult := r.db.WithContext(ctx).
		Model(&agentArtifactScanJobPO{}).
		Where("id = ?", req.JobID).
		Where("worker_id = ?", strings.TrimSpace(req.WorkerID)).
		Where("status = ?", string(entity.ArtifactScanJobStatusProcessing)).
		Where("lease_expires_at > ?", now).
		Updates(map[string]any{
			"status":           string(entity.ArtifactScanJobStatusPending),
			"worker_id":        "",
			"last_error":       truncateArtifactScanJobError(req.ErrorText),
			"available_at":     availableAt,
			"lease_expires_at": 0,
			"ended_at":         0,
			"updated_at":       now,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po agentArtifactScanJobPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", req.JobID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func (r *threadRepository) RequeueFailedArtifactScanJob(
	ctx context.Context,
	req RequeueFailedArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	availableAt := req.AvailableAt
	if availableAt <= now {
		availableAt = now
	}
	updateResult := r.db.WithContext(ctx).
		Model(&agentArtifactScanJobPO{}).
		Where("id = ?", req.JobID).
		Where("thread_id = ?", req.ThreadID).
		Where("status = ?", string(entity.ArtifactScanJobStatusFailed)).
		Updates(map[string]any{
			"status":           string(entity.ArtifactScanJobStatusPending),
			"worker_id":        "",
			"last_error":       truncateArtifactScanJobError(req.ErrorText),
			"available_at":     availableAt,
			"lease_expires_at": 0,
			"started_at":       0,
			"ended_at":         0,
			"updated_at":       now,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po agentArtifactScanJobPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", req.JobID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func (r *threadRepository) ListArtifactScanJobs(
	ctx context.Context,
	req ListArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).
		Model(&agentArtifactScanJobPO{}).
		Where("thread_id = ?", req.ThreadID)
	if req.RunID != nil {
		query = query.Where("run_id = ?", *req.RunID)
	}
	if req.ArtifactID != nil {
		query = query.Where("artifact_id = ?", *req.ArtifactID)
	}
	if req.Status != nil {
		query = query.Where("status = ?", string(*req.Status))
	}
	if scanner := strings.TrimSpace(req.Scanner); scanner != "" {
		query = query.Where("scanner = ?", scanner)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*agentArtifactScanJobPO, 0)
	if err := query.
		Order("updated_at DESC, id DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	jobs := make([]*entity.ArtifactScanJob, 0, len(pos))
	for _, po := range pos {
		jobs = append(jobs, po.toEntity())
	}
	return jobs, total, nil
}

type finishArtifactScanJobRequest struct {
	JobID     int64
	WorkerID  string
	Status    entity.ArtifactScanJobStatus
	LastError string
	Now       int64
}

func (r *threadRepository) finishArtifactScanJob(
	ctx context.Context,
	req finishArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	updateResult := r.db.WithContext(ctx).
		Model(&agentArtifactScanJobPO{}).
		Where("id = ?", req.JobID).
		Where("worker_id = ?", req.WorkerID).
		Where("status = ?", string(entity.ArtifactScanJobStatusProcessing)).
		Where("lease_expires_at > ?", req.Now).
		Updates(map[string]any{
			"status":           string(req.Status),
			"last_error":       req.LastError,
			"lease_expires_at": 0,
			"ended_at":         req.Now,
			"updated_at":       req.Now,
		})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po agentArtifactScanJobPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", req.JobID).
		First(&po).Error; err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func truncateArtifactScanJobError(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > 512 {
		runes = runes[:512]
	}
	return string(runes)
}

func (r *threadRepository) ListArtifacts(
	ctx context.Context,
	req ListArtifactsRequest,
) ([]*entity.AgentArtifact, int64, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).
		Model(&agentArtifactPO{}).
		Where("thread_id = ?", req.ThreadID)
	if req.DeletedOnly {
		query = query.Where("deleted_at > 0")
	} else {
		query = query.Where("deleted_at = 0")
	}
	if req.RunID != nil {
		query = query.Where("run_id = ?", *req.RunID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pos := make([]*agentArtifactPO, 0)
	if err := query.
		Order("created_at DESC, id DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	artifacts := make([]*entity.AgentArtifact, 0, len(pos))
	for _, po := range pos {
		artifacts = append(artifacts, po.toEntity())
	}
	return artifacts, total, nil
}

func (r *threadRepository) GetPlan(
	ctx context.Context,
	runID int64,
) (*entity.AgentRunPlan, error) {
	var po agentRunPlanPO
	err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *threadRepository) ReservePlanTaskID(
	ctx context.Context,
	plan *entity.AgentRunPlan,
	expected int64,
	next int64,
) (*entity.AgentRunPlan, error) {
	if plan == nil {
		return nil, fmt.Errorf("agent run plan is required")
	}
	po := agentRunPlanToPO(plan)
	var stored *entity.AgentRunPlan
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(po).Error; err != nil {
			return err
		}
		result := tx.Model(&agentRunPlanPO{}).
			Where(
				"run_id = ? AND high_watermark = ?",
				plan.RunID,
				expected,
			).
			Updates(map[string]any{
				"high_watermark": next,
				"revision":       gorm.Expr("revision + 1"),
				"updated_at":     plan.UpdatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrPlanReservationConflict
		}
		var current agentRunPlanPO
		if err := tx.Where("run_id = ?", plan.RunID).
			First(&current).Error; err != nil {
			return err
		}
		stored = current.toEntity()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stored, nil
}

func (r *threadRepository) ListPlanItems(
	ctx context.Context,
	runID int64,
	activeOnly bool,
) ([]*entity.AgentRunPlanItem, error) {
	query := r.db.WithContext(ctx).
		Where("run_id = ?", runID)
	if activeOnly {
		query = query.Where("active = ?", true)
	}
	var pos []*agentRunPlanItemPO
	if err := query.Order("task_id ASC").Find(&pos).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.AgentRunPlanItem, 0, len(pos))
	for _, po := range pos {
		items = append(items, po.toEntity())
	}
	return items, nil
}

func (r *threadRepository) GetPlanItem(
	ctx context.Context,
	runID int64,
	taskID int64,
) (*entity.AgentRunPlanItem, error) {
	var po agentRunPlanItemPO
	err := r.db.WithContext(ctx).
		Where("run_id = ? AND task_id = ?", runID, taskID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlanItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *threadRepository) UpsertPlanItem(
	ctx context.Context,
	item *entity.AgentRunPlanItem,
) (
	*entity.AgentRunPlanItem,
	*entity.AgentRunPlanItem,
	*entity.AgentRunPlan,
	bool,
	error,
) {
	if item == nil {
		return nil, nil, nil, false, fmt.Errorf("agent run plan item is required")
	}
	var (
		stored   *entity.AgentRunPlanItem
		previous *entity.AgentRunPlanItem
		plan     *entity.AgentRunPlan
		created  bool
	)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing agentRunPlanItemPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("run_id = ? AND task_id = ?", item.RunID, item.TaskID).
			First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			created = true
		case err != nil:
			return err
		default:
			previous = existing.toEntity()
			item.ID = existing.ID
			item.CreatedAt = existing.CreatedAt
			item.Version = existing.Version + 1
		}
		po, err := agentRunPlanItemToPO(item)
		if err != nil {
			return err
		}
		if created {
			if err := tx.Create(po).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&agentRunPlanItemPO{}).
				Where("id = ?", po.ID).
				Updates(map[string]any{
					"subject":     po.Subject,
					"description": po.Description,
					"status":      po.Status,
					"active_form": po.ActiveForm,
					"owner":       po.Owner,
					"blocks":      po.Blocks,
					"blocked_by":  po.BlockedBy,
					"metadata":    po.Metadata,
					"active":      po.Active,
					"version":     po.Version,
					"updated_at":  po.UpdatedAt,
				}).Error; err != nil {
				return err
			}
		}
		if err := incrementPlanRevision(tx, item.RunID, item.UpdatedAt); err != nil {
			return err
		}
		var storedPO agentRunPlanItemPO
		if err := tx.Where("run_id = ? AND task_id = ?", item.RunID, item.TaskID).
			First(&storedPO).Error; err != nil {
			return err
		}
		var planPO agentRunPlanPO
		if err := tx.Where("run_id = ?", item.RunID).
			First(&planPO).Error; err != nil {
			return err
		}
		stored = storedPO.toEntity()
		plan = planPO.toEntity()
		return nil
	})
	if err != nil {
		return nil, nil, nil, false, err
	}
	return stored, previous, plan, created, nil
}

func (r *threadRepository) ArchivePlanItem(
	ctx context.Context,
	runID int64,
	taskID int64,
	updatedAt int64,
) (
	*entity.AgentRunPlanItem,
	*entity.AgentRunPlanItem,
	*entity.AgentRunPlan,
	error,
) {
	var (
		stored   *entity.AgentRunPlanItem
		previous *entity.AgentRunPlanItem
		plan     *entity.AgentRunPlan
	)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing agentRunPlanItemPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("run_id = ? AND task_id = ?", runID, taskID).
			First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlanItemNotFound
		}
		if err != nil {
			return err
		}
		previous = existing.toEntity()
		status := existing.Status
		if status != string(entity.AgentRunPlanItemStatusCompleted) {
			status = string(entity.AgentRunPlanItemStatusDeleted)
		}
		if err := tx.Model(&agentRunPlanItemPO{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"status":     status,
				"active":     false,
				"version":    existing.Version + 1,
				"updated_at": updatedAt,
			}).Error; err != nil {
			return err
		}
		if err := incrementPlanRevision(tx, runID, updatedAt); err != nil {
			return err
		}
		var storedPO agentRunPlanItemPO
		if err := tx.Where("id = ?", existing.ID).
			First(&storedPO).Error; err != nil {
			return err
		}
		var planPO agentRunPlanPO
		if err := tx.Where("run_id = ?", runID).
			First(&planPO).Error; err != nil {
			return err
		}
		stored = storedPO.toEntity()
		plan = planPO.toEntity()
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return stored, previous, plan, nil
}

func incrementPlanRevision(tx *gorm.DB, runID int64, updatedAt int64) error {
	result := tx.Model(&agentRunPlanPO{}).
		Where("run_id = ?", runID).
		Updates(map[string]any{
			"revision":   gorm.Expr("revision + 1"),
			"updated_at": updatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPlanNotFound
	}
	return nil
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
	if len(req.RunIDs) > 0 {
		query = query.Where("run_id IN ?", req.RunIDs)
	} else if req.RunID > 0 {
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
	if len(req.RunIDs) > 0 {
		query = query.Where("run_id IN ?", req.RunIDs)
	} else if req.RunID > 0 {
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

type runTokenUsageAggregatePO struct {
	RunID            int64 `gorm:"column:run_id"`
	InputTokens      int64 `gorm:"column:input_tokens"`
	OutputTokens     int64 `gorm:"column:output_tokens"`
	TotalTokens      int64 `gorm:"column:total_tokens"`
	CostMicros       int64 `gorm:"column:cost_micros"`
	CallCount        int64 `gorm:"column:call_count"`
	LeadAgentTokens  int64 `gorm:"column:lead_agent_tokens"`
	SubagentTokens   int64 `gorm:"column:subagent_tokens"`
	MiddlewareTokens int64 `gorm:"column:middleware_tokens"`
	ToolTokens       int64 `gorm:"column:tool_tokens"`
}

func (r *threadRepository) AggregateTokenUsageByRun(ctx context.Context, req AggregateTokenUsageRequest) ([]*entity.RunTokenUsageAggregate, error) {
	query := r.db.WithContext(ctx).Model(&tokenUsagePO{})
	if req.ThreadID > 0 {
		query = query.Where("thread_id = ?", req.ThreadID)
	}
	if len(req.RunIDs) > 0 {
		query = query.Where("run_id IN ?", req.RunIDs)
	} else if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}

	pos := make([]*runTokenUsageAggregatePO, 0)
	err := query.Select(`
		run_id,
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
	).
		Group("run_id").
		Order("run_id ASC").
		Scan(&pos).Error
	if err != nil {
		return nil, err
	}

	aggregates := make([]*entity.RunTokenUsageAggregate, 0, len(pos))
	for _, po := range pos {
		aggregates = append(aggregates, &entity.RunTokenUsageAggregate{
			RunID: po.RunID,
			Aggregate: &entity.TokenUsageAggregate{
				InputTokens:      po.InputTokens,
				OutputTokens:     po.OutputTokens,
				TotalTokens:      po.TotalTokens,
				CostMicros:       po.CostMicros,
				CallCount:        po.CallCount,
				LeadAgentTokens:  po.LeadAgentTokens,
				SubagentTokens:   po.SubagentTokens,
				MiddlewareTokens: po.MiddlewareTokens,
				ToolTokens:       po.ToolTokens,
			},
		})
	}

	return aggregates, nil
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
			Where("parent_run_id = ?", 0).
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
		Where("parent_run_id = ?", 0).
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
		ParentRunID:       run.ParentRunID,
		SpaceID:           run.SpaceID,
		CreatorID:         run.CreatorID,
		AssistantID:       run.AssistantID,
		RunKind:           string(entity.DefaultRunKind(run.RunKind, run.ParentRunID)),
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
		ParentRunID:       po.ParentRunID,
		SpaceID:           po.SpaceID,
		CreatorID:         po.CreatorID,
		AssistantID:       po.AssistantID,
		RunKind:           entity.RunKind(po.RunKind),
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

	runtimeType := strings.TrimSpace(checkpoint.RuntimeType)
	if runtimeType == "" {
		runtimeType = "legacy"
	}

	return &checkpointPO{
		ID:                 checkpoint.ID,
		ThreadID:           checkpoint.ThreadID,
		RunID:              checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       checkpoint.CheckpointNS,
		RuntimeType:        runtimeType,
		RuntimeKey:         strings.TrimSpace(checkpoint.RuntimeKey),
		EnvelopeVersion:    checkpoint.EnvelopeVersion,
		RuntimeDeletedAt:   checkpoint.RuntimeDeletedAt,
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
		RuntimeType:        po.RuntimeType,
		RuntimeKey:         po.RuntimeKey,
		EnvelopeVersion:    po.EnvelopeVersion,
		RuntimeDeletedAt:   po.RuntimeDeletedAt,
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
		ID:                   memory.ID,
		ThreadID:             memory.ThreadID,
		RunID:                memory.RunID,
		SpaceID:              memory.SpaceID,
		Scope:                string(memory.Scope),
		Content:              memory.Content,
		Metadata:             metadata,
		Score:                memory.Score,
		Confidence:           memory.Confidence,
		SourceType:           memory.SourceType,
		SourceID:             memory.SourceID,
		CorrectionOfMemoryID: memory.CorrectionOfMemoryID,
		CorrectedAt:          memory.CorrectedAt,
		ExpiresAt:            memory.ExpiresAt,
		CreatedAt:            memory.CreatedAt,
		UpdatedAt:            memory.UpdatedAt,
		DeletedAt:            memory.DeletedAt,
	}, nil
}

func (po *memoryPO) toEntity() *entity.Memory {
	return &entity.Memory{
		ID:                   po.ID,
		ThreadID:             po.ThreadID,
		RunID:                po.RunID,
		SpaceID:              po.SpaceID,
		Scope:                entity.MemoryScope(po.Scope),
		Content:              po.Content,
		Metadata:             jsonToString(po.Metadata),
		Score:                po.Score,
		Confidence:           po.Confidence,
		SourceType:           po.SourceType,
		SourceID:             po.SourceID,
		CorrectionOfMemoryID: po.CorrectionOfMemoryID,
		CorrectedAt:          po.CorrectedAt,
		ExpiresAt:            po.ExpiresAt,
		CreatedAt:            po.CreatedAt,
		UpdatedAt:            po.UpdatedAt,
		DeletedAt:            po.DeletedAt,
	}
}

func transcriptSnapshotToPO(
	snapshot *entity.TranscriptSnapshot,
) (*transcriptSnapshotPO, error) {
	messages, err := requiredJSON("messages", snapshot.Messages)
	if err != nil {
		return nil, err
	}
	metadata, err := optionalJSON("metadata", snapshot.Metadata)
	if err != nil {
		return nil, err
	}
	return &transcriptSnapshotPO{
		ID:             snapshot.ID,
		ThreadID:       snapshot.ThreadID,
		RunID:          snapshot.RunID,
		SpaceID:        snapshot.SpaceID,
		Kind:           string(snapshot.Kind),
		Digest:         snapshot.Digest,
		IdempotencyKey: snapshot.IdempotencyKey,
		MessageCount:   snapshot.MessageCount,
		Messages:       messages,
		Metadata:       metadata,
		CreatedAt:      snapshot.CreatedAt,
	}, nil
}

func (po *transcriptSnapshotPO) toEntity() *entity.TranscriptSnapshot {
	return &entity.TranscriptSnapshot{
		ID:             po.ID,
		ThreadID:       po.ThreadID,
		RunID:          po.RunID,
		SpaceID:        po.SpaceID,
		Kind:           entity.TranscriptKind(po.Kind),
		Digest:         po.Digest,
		IdempotencyKey: po.IdempotencyKey,
		MessageCount:   po.MessageCount,
		Messages:       jsonToString(po.Messages),
		Metadata:       jsonToString(po.Metadata),
		CreatedAt:      po.CreatedAt,
	}
}

func memoryFlushJobToPO(job *entity.MemoryFlushJob) *memoryFlushJobPO {
	return &memoryFlushJobPO{
		ID:                   job.ID,
		ThreadID:             job.ThreadID,
		RunID:                job.RunID,
		SpaceID:              job.SpaceID,
		UserID:               job.UserID,
		AssistantID:          job.AssistantID,
		TranscriptSnapshotID: job.TranscriptSnapshotID,
		IdempotencyKey:       job.IdempotencyKey,
		Status:               string(job.Status),
		AttemptCount:         job.AttemptCount,
		WorkerID:             job.WorkerID,
		LastError:            job.LastError,
		AvailableAt:          job.AvailableAt,
		LeaseExpiresAt:       job.LeaseExpiresAt,
		StartedAt:            job.StartedAt,
		EndedAt:              job.EndedAt,
		CreatedAt:            job.CreatedAt,
		UpdatedAt:            job.UpdatedAt,
	}
}

func (po *memoryFlushJobPO) toEntity() *entity.MemoryFlushJob {
	return &entity.MemoryFlushJob{
		ID:                   po.ID,
		ThreadID:             po.ThreadID,
		RunID:                po.RunID,
		SpaceID:              po.SpaceID,
		UserID:               po.UserID,
		AssistantID:          po.AssistantID,
		TranscriptSnapshotID: po.TranscriptSnapshotID,
		IdempotencyKey:       po.IdempotencyKey,
		Status:               entity.MemoryFlushJobStatus(po.Status),
		AttemptCount:         po.AttemptCount,
		WorkerID:             po.WorkerID,
		LastError:            po.LastError,
		AvailableAt:          po.AvailableAt,
		LeaseExpiresAt:       po.LeaseExpiresAt,
		StartedAt:            po.StartedAt,
		EndedAt:              po.EndedAt,
		CreatedAt:            po.CreatedAt,
		UpdatedAt:            po.UpdatedAt,
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

func agentFileToPO(file *entity.AgentFile) (*agentFilePO, error) {
	metadata, err := requiredJSON("metadata", file.Metadata)
	if err != nil {
		return nil, err
	}
	return &agentFilePO{
		ID:               file.ID,
		SpaceID:          file.SpaceID,
		UserID:           file.UserID,
		ThreadID:         file.ThreadID,
		RunID:            file.RunID,
		FileName:         file.FileName,
		OriginalFileName: file.OriginalFileName,
		FileKind:         string(file.FileKind),
		VirtualPath:      file.VirtualPath,
		VirtualPathHash:  agentFileVirtualPathHash(file.VirtualPath),
		ObjectURI:        file.ObjectURI,
		ContentType:      file.ContentType,
		SizeBytes:        file.SizeBytes,
		Digest:           file.Digest,
		Status:           string(file.Status),
		Metadata:         metadata,
		CreatedAt:        file.CreatedAt,
		UpdatedAt:        file.UpdatedAt,
	}, nil
}

func agentFileVirtualPathHash(virtualPath string) string {
	sum := sha256.Sum256([]byte(virtualPath))
	return fmt.Sprintf("%x", sum[:])
}

func (po *agentFilePO) toEntity() *entity.AgentFile {
	return &entity.AgentFile{
		ID:               po.ID,
		SpaceID:          po.SpaceID,
		UserID:           po.UserID,
		ThreadID:         po.ThreadID,
		RunID:            po.RunID,
		FileName:         po.FileName,
		OriginalFileName: po.OriginalFileName,
		FileKind:         entity.AgentFileKind(po.FileKind),
		VirtualPath:      po.VirtualPath,
		ObjectURI:        po.ObjectURI,
		ContentType:      po.ContentType,
		SizeBytes:        po.SizeBytes,
		Digest:           po.Digest,
		Status:           entity.AgentFileStatus(po.Status),
		Metadata:         jsonToString(po.Metadata),
		CreatedAt:        po.CreatedAt,
		UpdatedAt:        po.UpdatedAt,
	}
}

func agentArtifactToPO(artifact *entity.AgentArtifact) (*agentArtifactPO, error) {
	metadata, err := requiredJSON("metadata", artifact.Metadata)
	if err != nil {
		return nil, err
	}
	return &agentArtifactPO{
		ID:           artifact.ID,
		SpaceID:      artifact.SpaceID,
		UserID:       artifact.UserID,
		ThreadID:     artifact.ThreadID,
		RunID:        artifact.RunID,
		FileID:       artifact.FileID,
		Title:        artifact.Title,
		ArtifactType: artifact.ArtifactType,
		VirtualPath:  artifact.VirtualPath,
		ObjectURI:    artifact.ObjectURI,
		ContentType:  artifact.ContentType,
		SizeBytes:    artifact.SizeBytes,
		PreviewMode:  string(artifact.PreviewMode),
		Metadata:     metadata,
		CreatedAt:    artifact.CreatedAt,
		UpdatedAt:    artifact.UpdatedAt,
		DeletedAt:    artifact.DeletedAt,
	}, nil
}

func (po *agentArtifactPO) toEntity() *entity.AgentArtifact {
	return &entity.AgentArtifact{
		ID:           po.ID,
		SpaceID:      po.SpaceID,
		UserID:       po.UserID,
		ThreadID:     po.ThreadID,
		RunID:        po.RunID,
		FileID:       po.FileID,
		Title:        po.Title,
		ArtifactType: po.ArtifactType,
		VirtualPath:  po.VirtualPath,
		ObjectURI:    po.ObjectURI,
		ContentType:  po.ContentType,
		SizeBytes:    po.SizeBytes,
		PreviewMode:  entity.AgentArtifactPreviewMode(po.PreviewMode),
		Metadata:     jsonToString(po.Metadata),
		CreatedAt:    po.CreatedAt,
		UpdatedAt:    po.UpdatedAt,
		DeletedAt:    po.DeletedAt,
	}
}

func artifactScanJobToPO(job *entity.ArtifactScanJob) *agentArtifactScanJobPO {
	return &agentArtifactScanJobPO{
		ID:             job.ID,
		ThreadID:       job.ThreadID,
		RunID:          job.RunID,
		SpaceID:        job.SpaceID,
		UserID:         job.UserID,
		ArtifactID:     job.ArtifactID,
		FileID:         job.FileID,
		Scanner:        job.Scanner,
		IdempotencyKey: job.IdempotencyKey,
		Status:         string(job.Status),
		WorkerID:       job.WorkerID,
		AttemptCount:   job.AttemptCount,
		LastError:      job.LastError,
		AvailableAt:    job.AvailableAt,
		LeaseExpiresAt: job.LeaseExpiresAt,
		StartedAt:      job.StartedAt,
		EndedAt:        job.EndedAt,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
	}
}

func (po *agentArtifactScanJobPO) toEntity() *entity.ArtifactScanJob {
	return &entity.ArtifactScanJob{
		ID:             po.ID,
		ThreadID:       po.ThreadID,
		RunID:          po.RunID,
		SpaceID:        po.SpaceID,
		UserID:         po.UserID,
		ArtifactID:     po.ArtifactID,
		FileID:         po.FileID,
		Scanner:        po.Scanner,
		IdempotencyKey: po.IdempotencyKey,
		Status:         entity.ArtifactScanJobStatus(po.Status),
		WorkerID:       po.WorkerID,
		AttemptCount:   po.AttemptCount,
		LastError:      po.LastError,
		AvailableAt:    po.AvailableAt,
		LeaseExpiresAt: po.LeaseExpiresAt,
		StartedAt:      po.StartedAt,
		EndedAt:        po.EndedAt,
		CreatedAt:      po.CreatedAt,
		UpdatedAt:      po.UpdatedAt,
	}
}

func agentRunPlanToPO(plan *entity.AgentRunPlan) *agentRunPlanPO {
	return &agentRunPlanPO{
		RunID:         plan.RunID,
		ThreadID:      plan.ThreadID,
		SpaceID:       plan.SpaceID,
		UserID:        plan.UserID,
		HighWatermark: plan.HighWatermark,
		Revision:      plan.Revision,
		CreatedAt:     plan.CreatedAt,
		UpdatedAt:     plan.UpdatedAt,
	}
}

func (po *agentRunPlanPO) toEntity() *entity.AgentRunPlan {
	return &entity.AgentRunPlan{
		RunID:         po.RunID,
		ThreadID:      po.ThreadID,
		SpaceID:       po.SpaceID,
		UserID:        po.UserID,
		HighWatermark: po.HighWatermark,
		Revision:      po.Revision,
		CreatedAt:     po.CreatedAt,
		UpdatedAt:     po.UpdatedAt,
	}
}

func agentRunPlanItemToPO(
	item *entity.AgentRunPlanItem,
) (*agentRunPlanItemPO, error) {
	blocks, err := requiredJSON("blocks", item.Blocks)
	if err != nil {
		return nil, err
	}
	blockedBy, err := requiredJSON("blocked_by", item.BlockedBy)
	if err != nil {
		return nil, err
	}
	metadata, err := requiredJSON("metadata", item.Metadata)
	if err != nil {
		return nil, err
	}
	return &agentRunPlanItemPO{
		ID:          item.ID,
		RunID:       item.RunID,
		TaskID:      item.TaskID,
		Subject:     item.Subject,
		Description: item.Description,
		Status:      string(item.Status),
		ActiveForm:  item.ActiveForm,
		Owner:       item.Owner,
		Blocks:      blocks,
		BlockedBy:   blockedBy,
		Metadata:    metadata,
		Active:      item.Active,
		Version:     item.Version,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}, nil
}

func (po *agentRunPlanItemPO) toEntity() *entity.AgentRunPlanItem {
	return &entity.AgentRunPlanItem{
		ID:          po.ID,
		RunID:       po.RunID,
		TaskID:      po.TaskID,
		Subject:     po.Subject,
		Description: po.Description,
		Status:      entity.AgentRunPlanItemStatus(po.Status),
		ActiveForm:  po.ActiveForm,
		Owner:       po.Owner,
		Blocks:      jsonToString(po.Blocks),
		BlockedBy:   jsonToString(po.BlockedBy),
		Metadata:    jsonToString(po.Metadata),
		Active:      po.Active,
		Version:     po.Version,
		CreatedAt:   po.CreatedAt,
		UpdatedAt:   po.UpdatedAt,
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

func escapeSQLLike(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
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
