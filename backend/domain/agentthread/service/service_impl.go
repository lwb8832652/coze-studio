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
	"encoding/hex"
	"encoding/json"
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

const defaultMemoryFlushJobLeaseTTLMillis = int64(300000)
const defaultMemoryManagementPageSize = int32(20)
const maxMemoryManagementPageSize = int32(100)
const maxMemoryImportItems = 100

const (
	memoryAuditEventUpdated  = "memory.updated"
	memoryAuditEventDeleted  = "memory.deleted"
	memoryAuditEventCleared  = "memory.cleared"
	memoryAuditEventRestored = "memory.restored"
	memoryAuditEventImported = "memory.imported"
)

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

func (s *threadService) UpdateThreadTitle(
	ctx context.Context,
	req *UpdateThreadTitleRequest,
) (*entity.Thread, bool, error) {
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("update thread title request is required")
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, false, InvalidArgumentErrorf("thread title is required")
	}
	updatedAt := req.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UnixMilli()
	}

	return s.repo.UpdateThreadTitle(ctx, repository.UpdateThreadTitleRequest{
		ThreadID:  req.ThreadID,
		Title:     title,
		UpdatedAt: updatedAt,
	})
}

func (s *threadService) UpdateThreadMetadata(
	ctx context.Context,
	req *UpdateThreadMetadataRequest,
) (*entity.Thread, bool, error) {
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("update thread metadata request is required")
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	metadata := strings.TrimSpace(req.Metadata)
	if metadata == "" {
		metadata = "{}"
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
		return nil, false, InvalidArgumentErrorf("thread metadata must be valid JSON object: %v", err)
	}
	if parsed == nil {
		parsed = map[string]any{}
	}
	metadataJSON, err := json.Marshal(parsed)
	if err != nil {
		return nil, false, err
	}

	updatedAt := req.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UnixMilli()
	}

	return s.repo.UpdateThreadMetadata(ctx, repository.UpdateThreadMetadataRequest{
		ThreadID:  req.ThreadID,
		Metadata:  string(metadataJSON),
		UpdatedAt: updatedAt,
	})
}

func (s *threadService) DeleteThread(
	ctx context.Context,
	req *DeleteThreadRequest,
) (bool, error) {
	if err := s.requireRepo(); err != nil {
		return false, err
	}
	if req == nil {
		return false, InvalidArgumentErrorf("delete thread request is required")
	}
	if req.ThreadID <= 0 {
		return false, InvalidArgumentErrorf("thread id is required")
	}

	return s.repo.DeleteThread(ctx, repository.DeleteThreadRequest{
		ThreadID: req.ThreadID,
	})
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
	runKind, err := normalizeRunKind(req.RunKind, req.ParentRunID)
	if err != nil {
		return nil, err
	}
	if req.ParentRunID > 0 {
		parent, err := s.repo.GetRun(ctx, req.ParentRunID)
		if err != nil {
			return nil, err
		}
		if parent.ThreadID != req.ThreadID {
			return nil, InvalidArgumentErrorf("parent run thread does not match run thread")
		}
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	status, err := normalizeInitialRunStatus(req.Status, runKind)
	if err != nil {
		return nil, err
	}
	run := &entity.Run{
		ID:                id,
		ThreadID:          req.ThreadID,
		ParentRunID:       req.ParentRunID,
		SpaceID:           thread.SpaceID,
		CreatorID:         thread.CreatorID,
		AssistantID:       defaultString(req.AssistantID, "default"),
		RunKind:           runKind,
		Status:            status,
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
	if status == entity.RunStatusRunning {
		run.StartedAt = now
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}

	return run, nil
}

func normalizeInitialRunStatus(
	status entity.RunStatus,
	runKind entity.RunKind,
) (entity.RunStatus, error) {
	if status == "" {
		return entity.RunStatusPending, nil
	}

	switch status {
	case entity.RunStatusPending, entity.RunStatusQueued:
		return status, nil
	case entity.RunStatusRunning:
		if runKind == entity.RunKindSubagent {
			return status, nil
		}
	default:
	}
	return "", InvalidArgumentErrorf("initial run status %q is not supported", status)
}

func normalizeRunKind(
	runKind entity.RunKind,
	parentRunID int64,
) (entity.RunKind, error) {
	normalized := entity.DefaultRunKind(runKind, parentRunID)
	switch normalized {
	case entity.RunKindTask:
		if parentRunID > 0 {
			return "", InvalidArgumentErrorf("task run cannot have parent run")
		}
		return normalized, nil
	case entity.RunKindSubagent:
		if parentRunID <= 0 {
			return "", InvalidArgumentErrorf("subagent run requires parent run")
		}
		return normalized, nil
	default:
		return "", InvalidArgumentErrorf("run kind %q is not supported", runKind)
	}
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

func (s *threadService) GetRunByIdempotencyKey(
	ctx context.Context,
	spaceID int64,
	idempotencyKey string,
) (*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if spaceID <= 0 {
		return nil, InvalidArgumentErrorf("space id is required")
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" || len(key) > 128 {
		return nil, InvalidArgumentErrorf("idempotency key is invalid")
	}

	return s.repo.GetRunByIdempotencyKey(ctx, spaceID, key)
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
		ThreadID:         req.ThreadID,
		ParentRunID:      req.ParentRunID,
		IncludeChildRuns: req.IncludeChildRuns,
		Status:           req.Status,
		Page:             page,
		PageSize:         pageSize,
	})
}

func (s *threadService) AppendRunEvent(ctx context.Context, req *AppendRunEventRequest) (*entity.RunEvent, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("append run event request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}

	eventType := strings.TrimSpace(req.EventType)
	if eventType == "" {
		return nil, InvalidArgumentErrorf("run event type is required")
	}

	run, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	if req.ThreadID > 0 && req.ThreadID != run.ThreadID {
		return nil, InvalidArgumentErrorf("run event thread id does not match run thread id")
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	event := &entity.RunEvent{
		ID:        id,
		ThreadID:  run.ThreadID,
		RunID:     run.ID,
		EventType: eventType,
		Payload:   defaultJSON(req.Payload, "{}"),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.repo.CreateRunEvent(ctx, event); err != nil {
		return nil, err
	}

	return event, nil
}

func (s *threadService) ListRunEvents(ctx context.Context, req *ListRunEventsRequest) ([]*entity.RunEvent, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list run events request is required")
	}
	if req.RunID <= 0 && req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("run id or thread id is required")
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	return s.repo.ListRunEvents(ctx, repository.ListRunEventsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Page:     page,
		PageSize: pageSize,
	})
}

func (s *threadService) CreateCheckpoint(ctx context.Context, req *CreateCheckpointRequest) (*entity.Checkpoint, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("create checkpoint request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}

	run, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	if req.ThreadID > 0 && req.ThreadID != run.ThreadID {
		return nil, InvalidArgumentErrorf("checkpoint thread id does not match run thread id")
	}

	runtimeType := strings.TrimSpace(req.RuntimeType)
	if runtimeType == "" {
		runtimeType = "legacy"
	}
	runtimeKey := strings.TrimSpace(req.RuntimeKey)
	if runtimeType != "legacy" && runtimeKey == "" {
		return nil, InvalidArgumentErrorf("runtime checkpoint key is required")
	}
	if req.EnvelopeVersion < 0 {
		return nil, InvalidArgumentErrorf("checkpoint envelope version must not be negative")
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	checkpoint := &entity.Checkpoint{
		ID:                 id,
		ThreadID:           run.ThreadID,
		RunID:              run.ID,
		ParentCheckpointID: req.ParentCheckpointID,
		CheckpointNS:       strings.TrimSpace(req.CheckpointNS),
		RuntimeType:        runtimeType,
		RuntimeKey:         runtimeKey,
		EnvelopeVersion:    req.EnvelopeVersion,
		ChannelValues:      defaultJSON(req.ChannelValues, "{}"),
		ChannelVersions:    defaultJSON(req.ChannelVersions, "{}"),
		PendingSends:       defaultJSON(req.PendingSends, "[]"),
		Metadata:           defaultJSON(req.Metadata, "{}"),
		CreatedAt:          time.Now().UnixMilli(),
	}
	if err := s.repo.CreateCheckpoint(ctx, checkpoint); err != nil {
		return nil, err
	}

	return checkpoint, nil
}

func (s *threadService) ListCheckpoints(ctx context.Context, req *ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list checkpoints request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}

	return s.repo.ListCheckpoints(ctx, repository.ListCheckpointsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Limit:    normalizeCheckpointLimit(req.Limit),
	})
}

func (s *threadService) GetCheckpoint(ctx context.Context, req *GetCheckpointRequest) (*entity.Checkpoint, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("get checkpoint request is required")
	}
	if req.CheckpointID <= 0 {
		return nil, InvalidArgumentErrorf("checkpoint id is required")
	}

	return s.repo.GetCheckpoint(ctx, req.CheckpointID)
}

func (s *threadService) GetLatestCheckpoint(ctx context.Context, req *GetLatestCheckpointRequest) (*entity.Checkpoint, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("get latest checkpoint request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}

	return s.repo.GetLatestCheckpoint(ctx, req.ThreadID)
}

func (s *threadService) GetLatestRuntimeCheckpoint(
	ctx context.Context,
	req *GetLatestRuntimeCheckpointRequest,
) (*entity.Checkpoint, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("get latest runtime checkpoint request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	runtimeType := strings.TrimSpace(req.RuntimeType)
	if runtimeType == "" {
		return nil, InvalidArgumentErrorf("runtime type is required")
	}
	runtimeKey := strings.TrimSpace(req.RuntimeKey)
	if runtimeKey == "" {
		return nil, InvalidArgumentErrorf("runtime key is required")
	}

	return s.repo.GetLatestRuntimeCheckpoint(
		ctx,
		req.ThreadID,
		req.RunID,
		runtimeType,
		runtimeKey,
	)
}

func (s *threadService) DeleteRuntimeCheckpoint(
	ctx context.Context,
	req *DeleteRuntimeCheckpointRequest,
) error {
	if err := s.requireRepo(); err != nil {
		return err
	}
	if req == nil {
		return InvalidArgumentErrorf("delete runtime checkpoint request is required")
	}
	if req.ThreadID <= 0 {
		return InvalidArgumentErrorf("thread id is required")
	}
	if req.RunID <= 0 {
		return InvalidArgumentErrorf("run id is required")
	}
	runtimeType := strings.TrimSpace(req.RuntimeType)
	if runtimeType == "" {
		return InvalidArgumentErrorf("runtime type is required")
	}
	runtimeKey := strings.TrimSpace(req.RuntimeKey)
	if runtimeKey == "" {
		return InvalidArgumentErrorf("runtime key is required")
	}
	if req.DeletedAt <= 0 {
		return InvalidArgumentErrorf("runtime checkpoint deleted time is required")
	}

	return s.repo.DeleteRuntimeCheckpoint(
		ctx,
		req.ThreadID,
		req.RunID,
		runtimeType,
		runtimeKey,
		req.DeletedAt,
	)
}

func (s *threadService) RememberMemory(ctx context.Context, req *RememberMemoryRequest) (*entity.Memory, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("remember memory request is required")
	}
	memory, _, err := s.createMemory(ctx, createMemoryRequest{
		ThreadID:             req.ThreadID,
		RunID:                req.RunID,
		Scope:                req.Scope,
		Content:              req.Content,
		Metadata:             req.Metadata,
		Score:                req.Score,
		Confidence:           req.Confidence,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		CorrectionOfMemoryID: req.CorrectionOfMemoryID,
		CorrectedAt:          req.CorrectedAt,
		ExpiresAt:            req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	return memory, nil
}

type createMemoryRequest struct {
	ID                   int64
	ThreadID             int64
	RunID                int64
	Scope                entity.MemoryScope
	Content              string
	Metadata             string
	Score                float64
	Confidence           float64
	SourceType           string
	SourceID             string
	CorrectionOfMemoryID int64
	CorrectedAt          int64
	ExpiresAt            int64
}

func (s *threadService) createMemory(ctx context.Context, req createMemoryRequest) (*entity.Memory, bool, error) {
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, false, InvalidArgumentErrorf("memory content is required")
	}

	scope := req.Scope
	if scope == "" {
		scope = entity.MemoryScopeThread
	}
	if !isValidMemoryScope(scope) {
		return nil, false, InvalidArgumentErrorf("memory scope is invalid")
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		return nil, false, InvalidArgumentErrorf("memory confidence must be between 0 and 1")
	}
	if req.CorrectionOfMemoryID < 0 {
		return nil, false, InvalidArgumentErrorf("memory correction id is invalid")
	}
	if req.CorrectedAt < 0 {
		return nil, false, InvalidArgumentErrorf("memory corrected time is invalid")
	}
	if req.ExpiresAt < 0 {
		return nil, false, InvalidArgumentErrorf("memory expiry time is invalid")
	}

	runID := req.RunID
	if scope == entity.MemoryScopeRun {
		if runID <= 0 {
			return nil, false, InvalidArgumentErrorf("run id is required for run memory")
		}
	} else {
		runID = 0
	}

	thread, err := s.repo.GetThread(ctx, req.ThreadID)
	if err != nil {
		return nil, false, err
	}

	id := req.ID
	if id <= 0 {
		id, err = s.idGen.GenID(ctx)
		if err != nil {
			return nil, false, err
		}
	}

	now := time.Now().UnixMilli()
	memory := &entity.Memory{
		ID:                   id,
		ThreadID:             req.ThreadID,
		RunID:                runID,
		SpaceID:              thread.SpaceID,
		Scope:                scope,
		Content:              content,
		Metadata:             strings.TrimSpace(req.Metadata),
		Score:                req.Score,
		Confidence:           req.Confidence,
		SourceType:           strings.TrimSpace(req.SourceType),
		SourceID:             strings.TrimSpace(req.SourceID),
		CorrectionOfMemoryID: req.CorrectionOfMemoryID,
		CorrectedAt:          req.CorrectedAt,
		ExpiresAt:            req.ExpiresAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if memory.SourceType != "" && memory.SourceID != "" {
		return s.repo.CreateOrGetMemoryBySource(ctx, memory)
	}

	if err := s.repo.CreateMemory(ctx, memory); err != nil {
		return nil, false, err
	}
	return memory, true, nil
}

func (s *threadService) ImportMemories(ctx context.Context, req *ImportMemoriesRequest) (*ImportMemoriesResult, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("import memories request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}
	if len(req.Memories) == 0 {
		return nil, InvalidArgumentErrorf("memory import items are required")
	}
	if len(req.Memories) > maxMemoryImportItems {
		return nil, InvalidArgumentErrorf("memory import item count exceeds limit")
	}

	ids, err := s.idGen.GenMultiIDs(ctx, len(req.Memories))
	if err != nil {
		return nil, err
	}
	result := &ImportMemoriesResult{
		Memories: make([]*entity.Memory, 0, len(req.Memories)),
	}
	for index, item := range req.Memories {
		memory, created, createErr := s.createMemory(ctx, createMemoryRequest{
			ID:                   ids[index],
			ThreadID:             req.ThreadID,
			RunID:                item.RunID,
			Scope:                item.Scope,
			Content:              item.Content,
			Metadata:             item.Metadata,
			Score:                item.Score,
			Confidence:           item.Confidence,
			SourceType:           item.SourceType,
			SourceID:             item.SourceID,
			CorrectionOfMemoryID: item.CorrectionOfMemoryID,
			CorrectedAt:          item.CorrectedAt,
			ExpiresAt:            item.ExpiresAt,
		})
		if createErr != nil {
			return nil, createErr
		}
		result.Memories = append(result.Memories, memory)
		if created {
			result.Imported++
		} else {
			result.Skipped++
		}
	}

	if result.Imported > 0 {
		if err := s.recordMemoryAuditEvent(ctx, &entity.MemoryAuditEvent{
			ThreadID:      req.ThreadID,
			ActorID:       req.ActorID,
			EventType:     memoryAuditEventImported,
			AffectedCount: result.Imported,
		}); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *threadService) RecallMemories(ctx context.Context, req *RecallMemoriesRequest) ([]*entity.Memory, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("recall memories request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}

	return s.repo.ListMemories(ctx, repository.ListMemoriesRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Scopes:   req.Scopes,
		Query:    strings.TrimSpace(req.Query),
		Limit:    limit,
		Now:      time.Now().UnixMilli(),
	})
}

func (s *threadService) ListMemories(ctx context.Context, req *ListMemoriesRequest) ([]*entity.Memory, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list memories request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}

	scopes, err := normalizeMemoryScopes(req.Scopes)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizeMemoryManagementPage(req.Page, req.PageSize)
	return s.repo.ListMemories(ctx, repository.ListMemoriesRequest{
		ThreadID:       req.ThreadID,
		RunID:          req.RunID,
		Scopes:         scopes,
		Query:          strings.TrimSpace(req.Query),
		Page:           page,
		PageSize:       pageSize,
		Now:            time.Now().UnixMilli(),
		IncludeExpired: req.IncludeExpired,
		IncludeDeleted: req.IncludeDeleted,
	})
}

func (s *threadService) UpdateMemory(
	ctx context.Context,
	req *UpdateMemoryRequest,
) (*entity.Memory, bool, error) {
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("update memory request is required")
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	if req.MemoryID <= 0 {
		return nil, false, InvalidArgumentErrorf("memory id is required")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, false, InvalidArgumentErrorf("memory content is required")
	}
	scope := req.Scope
	if scope == "" {
		scope = entity.MemoryScopeThread
	}
	if !isValidMemoryScope(scope) {
		return nil, false, InvalidArgumentErrorf("memory scope is invalid")
	}
	runID := req.RunID
	if scope == entity.MemoryScopeRun {
		if runID <= 0 {
			return nil, false, InvalidArgumentErrorf("run id is required for run memory")
		}
	} else {
		runID = 0
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		return nil, false, InvalidArgumentErrorf("memory confidence must be between 0 and 1")
	}
	if req.CorrectionOfMemoryID < 0 {
		return nil, false, InvalidArgumentErrorf("memory correction id is invalid")
	}
	if req.CorrectedAt < 0 {
		return nil, false, InvalidArgumentErrorf("memory corrected time is invalid")
	}
	if req.ExpiresAt < 0 {
		return nil, false, InvalidArgumentErrorf("memory expiry time is invalid")
	}

	memory, updated, err := s.repo.UpdateMemory(ctx, repository.UpdateMemoryRequest{
		ThreadID:             req.ThreadID,
		MemoryID:             req.MemoryID,
		RunID:                runID,
		Scope:                scope,
		Content:              content,
		Metadata:             strings.TrimSpace(req.Metadata),
		Score:                req.Score,
		Confidence:           req.Confidence,
		SourceType:           strings.TrimSpace(req.SourceType),
		SourceID:             strings.TrimSpace(req.SourceID),
		CorrectionOfMemoryID: req.CorrectionOfMemoryID,
		CorrectedAt:          req.CorrectedAt,
		ExpiresAt:            req.ExpiresAt,
		UpdatedAt:            time.Now().UnixMilli(),
	})
	if err != nil || !updated {
		return memory, updated, err
	}
	if err := s.recordMemoryAuditEvent(ctx, &entity.MemoryAuditEvent{
		ThreadID:      memory.ThreadID,
		RunID:         memory.RunID,
		SpaceID:       memory.SpaceID,
		MemoryID:      memory.ID,
		ActorID:       req.ActorID,
		EventType:     memoryAuditEventUpdated,
		Scope:         memory.Scope,
		SourceType:    memory.SourceType,
		SourceID:      memory.SourceID,
		AffectedCount: 1,
	}); err != nil {
		return memory, updated, err
	}
	return memory, updated, nil
}

func (s *threadService) DeleteMemory(ctx context.Context, req *DeleteMemoryRequest) (bool, error) {
	if err := s.requireRepo(); err != nil {
		return false, err
	}
	if req == nil {
		return false, InvalidArgumentErrorf("delete memory request is required")
	}
	if req.ThreadID <= 0 {
		return false, InvalidArgumentErrorf("thread id is required")
	}
	if req.MemoryID <= 0 {
		return false, InvalidArgumentErrorf("memory id is required")
	}

	deleted, err := s.repo.DeleteMemory(ctx, repository.DeleteMemoryRequest{
		ThreadID:  req.ThreadID,
		MemoryID:  req.MemoryID,
		DeletedAt: time.Now().UnixMilli(),
	})
	if err != nil || !deleted {
		return deleted, err
	}
	if err := s.recordMemoryAuditEvent(ctx, &entity.MemoryAuditEvent{
		ThreadID:      req.ThreadID,
		MemoryID:      req.MemoryID,
		ActorID:       req.ActorID,
		EventType:     memoryAuditEventDeleted,
		AffectedCount: 1,
	}); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func (s *threadService) ClearMemories(ctx context.Context, req *ClearMemoriesRequest) (int64, error) {
	if err := s.requireRepo(); err != nil {
		return 0, err
	}
	if req == nil {
		return 0, InvalidArgumentErrorf("clear memories request is required")
	}
	if req.ThreadID <= 0 {
		return 0, InvalidArgumentErrorf("thread id is required")
	}
	scopes, err := normalizeMemoryScopes(req.Scopes)
	if err != nil {
		return 0, err
	}

	deleted, err := s.repo.ClearMemories(ctx, repository.ClearMemoriesRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		Scopes:    scopes,
		DeletedAt: time.Now().UnixMilli(),
	})
	if err != nil || deleted == 0 {
		return deleted, err
	}
	event := &entity.MemoryAuditEvent{
		ThreadID:      req.ThreadID,
		RunID:         req.RunID,
		ActorID:       req.ActorID,
		EventType:     memoryAuditEventCleared,
		AffectedCount: deleted,
	}
	if len(scopes) == 1 {
		event.Scope = scopes[0]
	}
	if err := s.recordMemoryAuditEvent(ctx, event); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func (s *threadService) RestoreMemory(
	ctx context.Context,
	req *RestoreMemoryRequest,
) (*entity.Memory, bool, error) {
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("restore memory request is required")
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	if req.MemoryID <= 0 {
		return nil, false, InvalidArgumentErrorf("memory id is required")
	}

	memory, restored, err := s.repo.RestoreMemory(ctx, repository.RestoreMemoryRequest{
		ThreadID:   req.ThreadID,
		MemoryID:   req.MemoryID,
		ActorID:    req.ActorID,
		RestoredAt: time.Now().UnixMilli(),
	})
	if err != nil || !restored {
		return memory, restored, err
	}
	if err := s.recordMemoryAuditEvent(ctx, &entity.MemoryAuditEvent{
		ThreadID:      memory.ThreadID,
		RunID:         memory.RunID,
		SpaceID:       memory.SpaceID,
		MemoryID:      memory.ID,
		ActorID:       req.ActorID,
		EventType:     memoryAuditEventRestored,
		Scope:         memory.Scope,
		SourceType:    memory.SourceType,
		SourceID:      memory.SourceID,
		AffectedCount: 1,
	}); err != nil {
		return memory, restored, err
	}
	return memory, restored, nil
}

func (s *threadService) ListMemoryAuditEvents(
	ctx context.Context,
	req *ListMemoryAuditEventsRequest,
) ([]*entity.MemoryAuditEvent, int64, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("list memory audit events request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}
	if req.MemoryID < 0 {
		return nil, 0, InvalidArgumentErrorf("memory id is invalid")
	}

	page, pageSize := normalizeMemoryManagementPage(req.Page, req.PageSize)
	return s.repo.ListMemoryAuditEvents(ctx, repository.ListMemoryAuditEventsRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		Page:     page,
		PageSize: pageSize,
	})
}

func (s *threadService) recordMemoryAuditEvent(ctx context.Context, event *entity.MemoryAuditEvent) error {
	if event == nil {
		return nil
	}
	if event.CreatedAt <= 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	return s.repo.CreateMemoryAuditEvent(ctx, event)
}

func (s *threadService) PersistTranscriptSnapshot(
	ctx context.Context,
	req *PersistTranscriptSnapshotRequest,
) (*entity.TranscriptSnapshot, bool, error) {
	if err := s.requireComponents(); err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"persist transcript snapshot request is required",
		)
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	if req.RunID <= 0 {
		return nil, false, InvalidArgumentErrorf("run id is required")
	}
	if !isValidTranscriptKind(req.Kind) {
		return nil, false, InvalidArgumentErrorf("transcript kind is invalid")
	}
	digest := strings.TrimSpace(req.Digest)
	if len(digest) != 64 {
		return nil, false, InvalidArgumentErrorf(
			"transcript digest must be a SHA-256 hex value",
		)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, false, InvalidArgumentErrorf(
			"transcript digest must be a SHA-256 hex value",
		)
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != string(req.Kind)+":"+digest {
		return nil, false, InvalidArgumentErrorf(
			"transcript idempotency key must match kind and digest",
		)
	}
	if req.MessageCount <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"transcript message count must be positive",
		)
	}
	messages := strings.TrimSpace(req.Messages)
	if !json.Valid([]byte(messages)) {
		return nil, false, InvalidArgumentErrorf(
			"transcript messages must be valid JSON",
		)
	}
	metadata := defaultJSON(req.Metadata, "{}")
	if !json.Valid([]byte(metadata)) {
		return nil, false, InvalidArgumentErrorf(
			"transcript metadata must be valid JSON",
		)
	}

	run, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, false, err
	}
	if run.ThreadID != req.ThreadID {
		return nil, false, InvalidArgumentErrorf(
			"transcript run does not belong to thread",
		)
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, false, err
	}
	snapshot := &entity.TranscriptSnapshot{
		ID:             id,
		ThreadID:       run.ThreadID,
		RunID:          run.ID,
		SpaceID:        run.SpaceID,
		Kind:           req.Kind,
		Digest:         digest,
		IdempotencyKey: idempotencyKey,
		MessageCount:   req.MessageCount,
		Messages:       messages,
		Metadata:       metadata,
		CreatedAt:      time.Now().UnixMilli(),
	}
	return s.repo.CreateOrGetTranscriptSnapshot(ctx, snapshot)
}

func (s *threadService) GetTranscriptSnapshot(
	ctx context.Context,
	req *GetTranscriptSnapshotRequest,
) (*entity.TranscriptSnapshot, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("get transcript snapshot request is required")
	}
	if req.SnapshotID <= 0 {
		return nil, InvalidArgumentErrorf("transcript snapshot id is required")
	}

	return s.repo.GetTranscriptSnapshot(ctx, req.SnapshotID)
}

func (s *threadService) EnqueueMemoryFlushJob(
	ctx context.Context,
	req *EnqueueMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	if err := s.requireComponents(); err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"enqueue memory flush job request is required",
		)
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	if req.RunID <= 0 {
		return nil, false, InvalidArgumentErrorf("run id is required")
	}
	if req.TranscriptSnapshotID <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"transcript snapshot id is required",
		)
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return nil, false, InvalidArgumentErrorf(
			"memory flush idempotency key is invalid",
		)
	}

	run, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, false, err
	}
	if run.ThreadID != req.ThreadID {
		return nil, false, InvalidArgumentErrorf(
			"memory flush run does not belong to thread",
		)
	}
	snapshot, err := s.repo.GetTranscriptSnapshot(
		ctx,
		req.TranscriptSnapshotID,
	)
	if err != nil {
		return nil, false, err
	}
	if snapshot.ThreadID != run.ThreadID ||
		snapshot.RunID != run.ID ||
		snapshot.SpaceID != run.SpaceID {
		return nil, false, InvalidArgumentErrorf(
			"transcript snapshot does not belong to run",
		)
	}
	if snapshot.IdempotencyKey != idempotencyKey {
		return nil, false, InvalidArgumentErrorf(
			"memory flush idempotency key does not match transcript snapshot",
		)
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UnixMilli()
	availableAt := req.AvailableAt
	if availableAt <= 0 {
		availableAt = now
	}
	job := &entity.MemoryFlushJob{
		ID:                   id,
		ThreadID:             run.ThreadID,
		RunID:                run.ID,
		SpaceID:              run.SpaceID,
		UserID:               run.CreatorID,
		AssistantID:          run.AssistantID,
		TranscriptSnapshotID: req.TranscriptSnapshotID,
		IdempotencyKey:       idempotencyKey,
		Status:               entity.MemoryFlushJobStatusPending,
		AvailableAt:          availableAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	return s.repo.CreateOrGetMemoryFlushJob(ctx, job)
}

func (s *threadService) ClaimMemoryFlushJobs(
	ctx context.Context,
	req *ClaimMemoryFlushJobsRequest,
) ([]*entity.MemoryFlushJob, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("claim memory flush jobs request is required")
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, InvalidArgumentErrorf("worker id is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	leaseTTL := req.LeaseTTLMillis
	if leaseTTL <= 0 {
		leaseTTL = defaultMemoryFlushJobLeaseTTLMillis
	}
	now := time.Now().UnixMilli()
	return s.repo.ClaimMemoryFlushJobs(ctx, repository.ClaimMemoryFlushJobsRequest{
		WorkerID:       workerID,
		Limit:          limit,
		Now:            now,
		LeaseExpiresAt: now + leaseTTL,
	})
}

func (s *threadService) AggregateMemoryFlushBacklog(
	ctx context.Context,
	req *AggregateMemoryFlushBacklogRequest,
) ([]*entity.MemoryFlushBacklogAggregate, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	statuses := defaultMemoryFlushBacklogStatuses()
	if req != nil && len(req.Statuses) > 0 {
		statuses = append([]entity.MemoryFlushJobStatus(nil), req.Statuses...)
	}
	return s.repo.AggregateMemoryFlushBacklog(ctx, repository.AggregateMemoryFlushBacklogRequest{
		Statuses: statuses,
	})
}

func defaultMemoryFlushBacklogStatuses() []entity.MemoryFlushJobStatus {
	return []entity.MemoryFlushJobStatus{
		entity.MemoryFlushJobStatusPending,
		entity.MemoryFlushJobStatusProcessing,
	}
}

func (s *threadService) CompleteMemoryFlushJob(
	ctx context.Context,
	req *CompleteMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	if req == nil {
		return nil, false, InvalidArgumentErrorf("complete memory flush job request is required")
	}
	workerID, now, err := normalizeMemoryFlushJobUpdate(req.JobID, req.WorkerID, req.Now)
	if err != nil {
		return nil, false, err
	}
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	return s.repo.CompleteMemoryFlushJob(ctx, repository.CompleteMemoryFlushJobRequest{
		JobID:    req.JobID,
		WorkerID: workerID,
		Now:      now,
	})
}

func (s *threadService) RetryMemoryFlushJob(
	ctx context.Context,
	req *RetryMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	if req == nil {
		return nil, false, InvalidArgumentErrorf("retry memory flush job request is required")
	}
	workerID, now, err := normalizeMemoryFlushJobUpdate(req.JobID, req.WorkerID, req.Now)
	if err != nil {
		return nil, false, err
	}
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	return s.repo.RetryMemoryFlushJob(ctx, repository.RetryMemoryFlushJobRequest{
		JobID:       req.JobID,
		WorkerID:    workerID,
		ErrorText:   req.ErrorText,
		AvailableAt: req.AvailableAt,
		Now:         now,
	})
}

func (s *threadService) FailMemoryFlushJob(
	ctx context.Context,
	req *FailMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	if req == nil {
		return nil, false, InvalidArgumentErrorf("fail memory flush job request is required")
	}
	workerID, now, err := normalizeMemoryFlushJobUpdate(req.JobID, req.WorkerID, req.Now)
	if err != nil {
		return nil, false, err
	}
	if err := s.requireRepo(); err != nil {
		return nil, false, err
	}
	return s.repo.FailMemoryFlushJob(ctx, repository.FailMemoryFlushJobRequest{
		JobID:     req.JobID,
		WorkerID:  workerID,
		ErrorText: req.ErrorText,
		Now:       now,
	})
}

func normalizeMemoryFlushJobUpdate(jobID int64, workerID string, now int64) (string, int64, error) {
	if jobID <= 0 {
		return "", 0, InvalidArgumentErrorf("memory flush job id is required")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return "", 0, InvalidArgumentErrorf("worker id is required")
	}
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	return workerID, now, nil
}

func (s *threadService) RecordTokenUsage(ctx context.Context, req *RecordTokenUsageRequest) (*entity.TokenUsage, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("record token usage request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	if !isValidTokenUsageSource(req.Source) {
		return nil, InvalidArgumentErrorf("token usage source is invalid")
	}
	if req.InputTokens < 0 || req.OutputTokens < 0 || req.TotalTokens < 0 || req.CostMicros < 0 {
		return nil, InvalidArgumentErrorf("tokens cannot be negative")
	}

	run, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}

	totalTokens := req.TotalTokens
	if totalTokens == 0 {
		totalTokens = req.InputTokens + req.OutputTokens
	}

	usage := &entity.TokenUsage{
		ID:           id,
		ThreadID:     run.ThreadID,
		RunID:        run.ID,
		SpaceID:      run.SpaceID,
		Source:       req.Source,
		StepID:       strings.TrimSpace(req.StepID),
		StepIndex:    req.StepIndex,
		StepName:     strings.TrimSpace(req.StepName),
		ModelName:    strings.TrimSpace(req.ModelName),
		Provider:     strings.TrimSpace(req.Provider),
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		TotalTokens:  totalTokens,
		CostMicros:   req.CostMicros,
		Currency:     strings.TrimSpace(req.Currency),
		Estimated:    req.Estimated,
		RawUsage:     strings.TrimSpace(req.RawUsage),
		Metadata:     strings.TrimSpace(req.Metadata),
		CreatedAt:    time.Now().UnixMilli(),
	}
	if err := s.repo.CreateTokenUsage(ctx, usage); err != nil {
		return nil, err
	}

	return usage, nil
}

func (s *threadService) GetRunTokenUsage(ctx context.Context, req *GetRunTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, []*entity.RunTokenUsageAggregate, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, nil, nil, err
	}
	if req == nil {
		return nil, 0, nil, nil, InvalidArgumentErrorf("get run token usage request is required")
	}
	if req.RunID <= 0 {
		return nil, 0, nil, nil, InvalidArgumentErrorf("run id is required")
	}
	if req.Source != "" && !isValidTokenUsageSource(req.Source) {
		return nil, 0, nil, nil, InvalidArgumentErrorf("token usage source is invalid")
	}

	page, pageSize := normalizeTokenUsagePage(req.Page, req.PageSize)
	threadID, runIDs, err := s.tokenUsageRunScope(ctx, req.RunID, req.IncludeChildRuns)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	rows, total, err := s.repo.ListTokenUsage(ctx, repository.ListTokenUsageRequest{
		ThreadID: threadID,
		RunID:    req.RunID,
		RunIDs:   runIDs,
		Source:   req.Source,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, 0, nil, nil, err
	}

	aggregate, err := s.repo.AggregateTokenUsage(ctx, repository.AggregateTokenUsageRequest{
		ThreadID: threadID,
		RunID:    req.RunID,
		RunIDs:   runIDs,
	})
	if err != nil {
		return nil, 0, nil, nil, err
	}

	var runAggregates []*entity.RunTokenUsageAggregate
	if req.IncludeChildRuns {
		runAggregates, err = s.repo.AggregateTokenUsageByRun(ctx, repository.AggregateTokenUsageRequest{
			ThreadID: threadID,
			RunIDs:   runIDs,
		})
		if err != nil {
			return nil, 0, nil, nil, err
		}
	}

	return rows, total, aggregate, runAggregates, nil
}

func (s *threadService) tokenUsageRunScope(
	ctx context.Context,
	runID int64,
	includeChildRuns bool,
) (int64, []int64, error) {
	if !includeChildRuns {
		return 0, nil, nil
	}

	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return 0, nil, err
	}

	parentRunID := runID
	children, _, err := s.repo.ListRuns(ctx, repository.ListRunsRequest{
		ThreadID:    run.ThreadID,
		ParentRunID: &parentRunID,
		Page:        1,
		PageSize:    100,
	})
	if err != nil {
		return 0, nil, err
	}

	runIDs := make([]int64, 0, len(children)+1)
	runIDs = append(runIDs, runID)
	for _, child := range children {
		runIDs = append(runIDs, child.ID)
	}

	return run.ThreadID, runIDs, nil
}

func (s *threadService) GetThreadTokenUsage(ctx context.Context, req *GetThreadTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, error) {
	if err := s.requireRepo(); err != nil {
		return nil, 0, nil, err
	}
	if req == nil {
		return nil, 0, nil, InvalidArgumentErrorf("get thread token usage request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, nil, InvalidArgumentErrorf("thread id is required")
	}
	if req.Source != "" && !isValidTokenUsageSource(req.Source) {
		return nil, 0, nil, InvalidArgumentErrorf("token usage source is invalid")
	}

	page, pageSize := normalizeTokenUsagePage(req.Page, req.PageSize)
	rows, total, err := s.repo.ListTokenUsage(ctx, repository.ListTokenUsageRequest{
		ThreadID: req.ThreadID,
		Source:   req.Source,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, 0, nil, err
	}

	aggregate, err := s.repo.AggregateTokenUsage(ctx, repository.AggregateTokenUsageRequest{ThreadID: req.ThreadID})
	if err != nil {
		return nil, 0, nil, err
	}

	return rows, total, aggregate, nil
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
		WorkerID:       workerID,
		Limit:          limit,
		Now:            req.Now,
		LeaseTTLMillis: req.LeaseTTLMillis,
	})
}

func (s *threadService) AggregateRunBacklog(
	ctx context.Context,
	req *AggregateRunBacklogRequest,
) ([]*entity.RunBacklogAggregate, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	statuses := defaultRunBacklogStatuses()
	if req != nil && len(req.Statuses) > 0 {
		statuses = append([]entity.RunStatus(nil), req.Statuses...)
	}

	return s.repo.AggregateRunBacklog(ctx, repository.AggregateRunBacklogRequest{
		Statuses: statuses,
	})
}

func defaultRunBacklogStatuses() []entity.RunStatus {
	return []entity.RunStatus{
		entity.RunStatusPending,
		entity.RunStatusQueued,
		entity.RunStatusRunning,
		entity.RunStatusInterrupted,
	}
}

func (s *threadService) ClaimQueuedResumeRuns(ctx context.Context, req *ClaimQueuedResumeRunsRequest) ([]*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("claim queued resume runs request is required")
	}

	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, InvalidArgumentErrorf("worker id is required")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	return s.repo.ClaimQueuedResumeRuns(ctx, repository.ClaimQueuedResumeRunsRequest{
		WorkerID:       workerID,
		Limit:          limit,
		Now:            req.Now,
		LeaseTTLMillis: req.LeaseTTLMillis,
	})
}

func (s *threadService) RenewRunLease(ctx context.Context, req *RenewRunLeaseRequest) (*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("renew run lease request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	owner := strings.TrimSpace(req.LeaseOwner)
	token := strings.TrimSpace(req.LeaseToken)
	if owner == "" || token == "" || req.ExecutionGeneration == 0 {
		return nil, InvalidArgumentErrorf("run lease credentials are required")
	}

	return s.repo.RenewRunLease(ctx, repository.RenewRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          owner,
		LeaseToken:          token,
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 req.Now,
		LeaseTTLMillis:      req.LeaseTTLMillis,
	})
}

func (s *threadService) ReleaseRunLease(ctx context.Context, req *ReleaseRunLeaseRequest) (*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("release run lease request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	owner := strings.TrimSpace(req.LeaseOwner)
	token := strings.TrimSpace(req.LeaseToken)
	if owner == "" || token == "" || req.ExecutionGeneration == 0 {
		return nil, InvalidArgumentErrorf("run lease credentials are required")
	}
	if req.ToStatus != entity.RunStatusPending && req.ToStatus != entity.RunStatusQueued {
		return nil, InvalidArgumentErrorf("run lease release target status is invalid")
	}

	return s.repo.ReleaseRunLease(ctx, repository.ReleaseRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          owner,
		LeaseToken:          token,
		ExecutionGeneration: req.ExecutionGeneration,
		ToStatus:            req.ToStatus,
		Now:                 req.Now,
	})
}

func (s *threadService) ListExpiredRunLeases(
	ctx context.Context,
	req *ListExpiredRunLeasesRequest,
) ([]*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("list expired run leases request is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	return s.repo.ListExpiredRunLeases(ctx, repository.ListExpiredRunLeasesRequest{
		Now:   req.Now,
		Limit: limit,
	})
}

func (s *threadService) ReconcileExpiredRunLease(
	ctx context.Context,
	req *ReconcileExpiredRunLeaseRequest,
) (*entity.Run, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("reconcile expired run lease request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	owner := strings.TrimSpace(req.LeaseOwner)
	token := strings.TrimSpace(req.LeaseToken)
	if owner == "" || token == "" || req.ExecutionGeneration == 0 {
		return nil, InvalidArgumentErrorf("run lease credentials are required")
	}
	if req.ToStatus != entity.RunStatusInterrupted && req.ToStatus != entity.RunStatusFailed {
		return nil, InvalidArgumentErrorf("expired run lease reconciliation target status is invalid")
	}

	return s.repo.ReconcileExpiredRunLease(ctx, repository.ReconcileExpiredRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          owner,
		LeaseToken:          token,
		ExecutionGeneration: req.ExecutionGeneration,
		ToStatus:            req.ToStatus,
		Now:                 req.Now,
		ErrorCode:           strings.TrimSpace(req.ErrorCode),
		ErrorMessage:        strings.TrimSpace(req.ErrorMessage),
	})
}

func (s *threadService) RequestRunCancellation(
	ctx context.Context,
	req *RequestRunCancellationRequest,
) (*RequestRunCancellationResult, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("request run cancellation request is required")
	}
	if req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	current, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("run %d is missing", req.RunID)
	}
	if current.Status == entity.RunStatusCanceled {
		return &RequestRunCancellationResult{
			Run:            current,
			PreviousStatus: entity.RunStatusCanceled,
			Changed:        false,
		}, nil
	}
	eventID, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	errorCode := strings.TrimSpace(req.ErrorCode)
	if errorCode == "" {
		errorCode = "run_canceled"
	}
	errorMessage := strings.TrimSpace(req.ErrorMessage)
	if errorMessage == "" {
		errorMessage = "run canceled by request"
	}
	payload, err := json.Marshal(map[string]any{"status": string(entity.RunStatusCanceled)})
	if err != nil {
		return nil, fmt.Errorf("marshal run cancellation event: %w", err)
	}

	result, err := s.repo.RequestRunCancellation(ctx, repository.RequestRunCancellationRequest{
		RunID:        req.RunID,
		Now:          now,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Event: &entity.RunEvent{
			ID:        eventID,
			ThreadID:  current.ThreadID,
			RunID:     current.ID,
			EventType: "run.canceled",
			Payload:   string(payload),
			CreatedAt: now,
		},
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Run == nil {
		return nil, fmt.Errorf("run cancellation returned empty result")
	}
	return &RequestRunCancellationResult{
		Run:            result.Run,
		PreviousStatus: result.PreviousStatus,
		Changed:        result.Changed,
	}, nil
}

func (s *threadService) FinalizeRunSuccess(
	ctx context.Context,
	req *FinalizeRunSuccessRequest,
) (*FinalizeRunSuccessResult, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("finalize run success request is required")
	}
	if req.RunID <= 0 || req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("run id and thread id are required")
	}
	owner := strings.TrimSpace(req.LeaseOwner)
	token := strings.TrimSpace(req.LeaseToken)
	if owner == "" || token == "" || req.ExecutionGeneration == 0 {
		return nil, InvalidArgumentErrorf("run lease credentials are required")
	}
	messageContent := strings.TrimSpace(req.Message)
	if messageContent == "" {
		return nil, InvalidArgumentErrorf("assistant message is required")
	}
	messageID, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	result, err := s.repo.FinalizeRunSuccess(ctx, repository.FinalizeRunSuccessRequest{
		RunID:               req.RunID,
		LeaseOwner:          owner,
		LeaseToken:          token,
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 now,
		Message: &entity.Message{
			ID:        messageID,
			ThreadID:  req.ThreadID,
			RunID:     req.RunID,
			Role:      entity.MessageRoleAssistant,
			Content:   messageContent,
			Metadata:  req.MessageMetadata,
			CreatedAt: now,
		},
		ExpectedThreadTitle: strings.TrimSpace(req.ExpectedThreadTitle),
		ThreadTitle:         strings.TrimSpace(req.ThreadTitle),
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Run == nil || result.Message == nil {
		return nil, fmt.Errorf("finalize run success returned empty result")
	}
	return &FinalizeRunSuccessResult{
		Run:          result.Run,
		Message:      result.Message,
		TitleUpdated: result.TitleUpdated,
	}, nil
}

func (s *threadService) CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	return s.transitionRun(ctx, req, entity.RunStatusSucceeded)
}

func (s *threadService) InterruptRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	return s.transitionRun(ctx, req, entity.RunStatusInterrupted)
}

func (s *threadService) FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	return s.transitionRun(ctx, req, entity.RunStatusFailed)
}

func (s *threadService) CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("update run status request is required")
	}
	result, err := s.RequestRunCancellation(ctx, &RequestRunCancellationRequest{
		RunID:        req.RunID,
		Now:          req.Now,
		ErrorCode:    req.ErrorCode,
		ErrorMessage: req.ErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	return result.Run, nil
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
		RunID:               req.RunID,
		From:                from,
		To:                  to,
		WorkerID:            workerID,
		LeaseOwner:          strings.TrimSpace(req.LeaseOwner),
		LeaseToken:          strings.TrimSpace(req.LeaseToken),
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 req.Now,
		ErrorCode:           strings.TrimSpace(req.ErrorCode),
		ErrorMessage:        strings.TrimSpace(req.ErrorMessage),
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

func isValidMemoryScope(scope entity.MemoryScope) bool {
	switch scope {
	case entity.MemoryScopeThread, entity.MemoryScopeRun, entity.MemoryScopeLongTerm:
		return true
	default:
		return false
	}
}

func normalizeMemoryScopes(scopes []entity.MemoryScope) ([]entity.MemoryScope, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	normalized := make([]entity.MemoryScope, 0, len(scopes))
	seen := make(map[entity.MemoryScope]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		if !isValidMemoryScope(scope) {
			return nil, InvalidArgumentErrorf("memory scope is invalid")
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	return normalized, nil
}

func normalizeMemoryManagementPage(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultMemoryManagementPageSize
	}
	if pageSize > maxMemoryManagementPageSize {
		pageSize = maxMemoryManagementPageSize
	}

	return page, pageSize
}

func isValidTranscriptKind(kind entity.TranscriptKind) bool {
	switch kind {
	case entity.TranscriptKindSummaryInput, entity.TranscriptKindTerminal:
		return true
	default:
		return false
	}
}

func isValidTokenUsageSource(source entity.TokenUsageSource) bool {
	switch source {
	case entity.TokenUsageSourceLeadAgent, entity.TokenUsageSourceSubagent, entity.TokenUsageSourceMiddleware, entity.TokenUsageSourceTool:
		return true
	default:
		return false
	}
}

func normalizeTokenUsagePage(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}

	return page, pageSize
}

func normalizeCheckpointLimit(limit int32) int32 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}

	return limit
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
