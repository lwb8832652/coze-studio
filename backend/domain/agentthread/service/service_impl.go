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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
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

func NewService(c *Components) Service {
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

func (s *threadService) CreateThreadRunMessage(
	ctx context.Context,
	req *CreateThreadRunMessageRequest,
) (*CreateThreadRunMessageResult, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("create thread run message request is required")
	}
	title := strings.TrimSpace(req.Thread.Title)
	if title == "" {
		return nil, InvalidArgumentErrorf("thread title is required")
	}
	input := strings.TrimSpace(req.Run.Input)
	if input == "" {
		return nil, InvalidArgumentErrorf("run input is required")
	}
	if req.Run.ParentRunID > 0 {
		return nil, InvalidArgumentErrorf("new thread run cannot have parent run")
	}
	if req.EnrollJournal {
		if req.JournalEnrollment == nil {
			return nil, InvalidArgumentErrorf("journal enrollment options are required")
		}
		if strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion) == "" {
			return nil, InvalidArgumentErrorf("journal enrollment version is required")
		}
		if req.JournalEnrollment.Recovery != nil {
			return nil, InvalidArgumentErrorf("new thread run cannot be a journal recovery")
		}
		if err := validateFreshJournalProjectionState(req.JournalEnrollment); err != nil {
			return nil, err
		}
	} else if req.JournalEnrollment != nil {
		return nil, InvalidArgumentErrorf("journal enrollment options require journal enrollment")
	}
	runKind, err := normalizeRunKind(req.Run.RunKind, req.Run.ParentRunID)
	if err != nil {
		return nil, err
	}
	strategy, err := normalizeMultitaskStrategy(req.Run.MultitaskStrategy, runKind)
	if err != nil {
		return nil, err
	}
	onDisconnect, err := normalizeOnDisconnectMode(req.Run.OnDisconnect)
	if err != nil {
		return nil, err
	}
	status, err := normalizeInitialRunStatus(req.Run.Status, runKind)
	if err != nil {
		return nil, err
	}
	if !isValidMessageRole(req.Message.Role) {
		return nil, InvalidArgumentErrorf("message role is invalid")
	}
	messageContent := strings.TrimSpace(req.Message.Content)
	if messageContent == "" {
		return nil, InvalidArgumentErrorf("message content is required")
	}

	entityCount := 3
	if req.EnrollJournal {
		entityCount++
	}
	ids, err := s.idGen.GenMultiIDs(ctx, entityCount)
	if err != nil {
		return nil, err
	}
	if len(ids) != entityCount {
		return nil, fmt.Errorf("agent thread id generator returned %d ids, expected %d", len(ids), entityCount)
	}
	now := time.Now().UnixMilli()
	source := req.Thread.Source
	if source == "" {
		source = entity.ThreadSourceWeb
	}
	thread := &entity.Thread{
		ID:            ids[0],
		SpaceID:       req.Thread.SpaceID,
		CreatorID:     req.Thread.UserID,
		AgentID:       req.Thread.AgentID,
		Title:         title,
		Status:        entity.ThreadStatusIdle,
		Source:        source,
		Metadata:      req.Thread.Metadata,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastMessageAt: now,
	}
	runReq := req.Run
	runReq.MultitaskStrategy = strategy
	runReq.OnDisconnect = onDisconnect
	run, err := newRunEntity(&runReq, ids[1], thread, runKind, status, input, now)
	if err != nil {
		return nil, err
	}
	message := &entity.Message{
		ID:        ids[2],
		ThreadID:  thread.ID,
		RunID:     run.ID,
		Role:      req.Message.Role,
		Content:   messageContent,
		Metadata:  req.Message.Metadata,
		CreatedAt: now,
	}
	var attempt *entity.RunAttempt
	if req.EnrollJournal {
		attemptStatus, err := journalAttemptStatusForRun(status)
		if err != nil {
			return nil, err
		}
		attemptID := ids[3]
		activeSlot := uint8(1)
		traceID := strings.TrimSpace(req.JournalEnrollment.TraceID)
		attempt = &entity.RunAttempt{
			ID: attemptID, ThreadID: thread.ID,
			JournalRunID: run.ID, ExecutionRunID: run.ID,
			AttemptID: fmt.Sprintf("att_%d", attemptID), Ordinal: 1,
			Status: attemptStatus, ActiveSlot: &activeSlot,
			NextSequence: 1, LastCommittedSequence: 0,
			EnrollmentVersion: strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion),
			SnapshotsEnabled:  req.JournalEnrollment.SnapshotsEnabled,
			ProjectionState:   initialJournalProjectionState(req.JournalEnrollment),
			TraceID:           journalStringPointer(traceID),
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if attempt.ProjectionState == entity.JournalProjectionStateDisabled {
			attempt.SnapshotsEnabled = false
		}
		if attemptStatus == entity.RunAttemptStatusRunning {
			startedAt := run.StartedAt
			if startedAt <= 0 {
				startedAt = now
			}
			attempt.StartedAt = &startedAt
		}
	}
	result, err := s.repo.CreateThreadBundle(ctx, repository.CreateThreadBundleRequest{
		Thread: thread, Run: run, Message: message, Attempt: attempt,
		ValidateIdempotencyReplay: strings.TrimSpace(req.Run.IdempotencyOperation) != "",
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Thread == nil || result.Run == nil || result.Message == nil {
		return nil, fmt.Errorf("agent thread repository returned incomplete thread bundle")
	}
	if req.EnrollJournal && result.Attempt == nil {
		return nil, fmt.Errorf("agent thread repository returned thread bundle without journal attempt")
	}

	return &CreateThreadRunMessageResult{
		Thread: result.Thread, Run: result.Run, Message: result.Message, Attempt: result.Attempt,
	}, nil
}

func (s *threadService) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}

	return s.repo.GetThread(ctx, id)
}

func (s *threadService) GetHumanResumeRolloverReplay(
	ctx context.Context,
	req repository.HumanResumeRolloverReplayRequest,
) (*repository.HumanResumeRolloverReplayResult, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	replayRepo, ok := s.repo.(repository.HumanResumeRolloverReplayRepository)
	if !ok {
		return nil, fmt.Errorf("human resume rollover replay repository is unavailable")
	}
	return replayRepo.GetHumanResumeRolloverReplay(ctx, req)
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

func (s *threadService) ListRecentMessagesByRoles(
	ctx context.Context,
	req *ListRecentMessagesByRolesRequest,
) ([]*entity.Message, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("list recent messages request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}
	if len(req.Roles) == 0 {
		return []*entity.Message{}, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	return s.repo.ListRecentMessagesByRoles(ctx, repository.ListRecentMessagesByRolesRequest{
		ThreadID: req.ThreadID,
		Roles:    append([]entity.MessageRole(nil), req.Roles...),
		Limit:    limit,
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
	strategy, err := normalizeMultitaskStrategy(req.MultitaskStrategy, runKind)
	if err != nil {
		return nil, err
	}
	onDisconnect, err := normalizeOnDisconnectMode(req.OnDisconnect)
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
	runReq := *req
	runReq.MultitaskStrategy = strategy
	runReq.OnDisconnect = onDisconnect
	run, err := newRunEntity(&runReq, id, thread, runKind, status, input, now)
	if err != nil {
		return nil, err
	}
	createRun := s.repo.CreateRun
	if guardedRepo, ok := s.repo.(repository.ThreadGuardedRunRepository); ok {
		createRun = guardedRepo.CreateRunWithThreadLock
	}
	if err := createRun(ctx, run); err != nil {
		return nil, err
	}

	return run, nil
}

func (s *threadService) CreateRunBundle(
	ctx context.Context,
	req *CreateRunBundleRequest,
) (*CreateRunBundleResult, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("create run bundle request is required")
	}
	if req.Run.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}
	input := strings.TrimSpace(req.Run.Input)
	if input == "" {
		return nil, InvalidArgumentErrorf("run input is required")
	}

	thread, err := s.repo.GetThread(ctx, req.Run.ThreadID)
	if err != nil {
		return nil, err
	}
	runKind, err := normalizeRunKind(req.Run.RunKind, req.Run.ParentRunID)
	if err != nil {
		return nil, err
	}
	if req.EnrollJournal && (runKind != entity.RunKindTask || req.Run.ParentRunID != 0) {
		return nil, InvalidArgumentErrorf("only a root task run can enroll in journal")
	}
	if req.EnrollJournal {
		if req.JournalEnrollment == nil {
			return nil, InvalidArgumentErrorf("journal enrollment options are required")
		}
		if req.JournalEnrollment.Recovery != nil && req.JournalEnrollment.HumanResume != nil {
			return nil, InvalidArgumentErrorf("journal recovery and human resume cannot both be set")
		}
		if countJournalEnrollmentSpecializations(req.JournalEnrollment) > 1 {
			return nil, InvalidArgumentErrorf("journal enrollment specialization must be unique")
		}
		if human := req.JournalEnrollment.HumanResume; human != nil {
			if err := validateJournalHumanResumeEnrollment(req, human); err != nil {
				return nil, err
			}
		} else if ordinary := req.JournalEnrollment.OrdinaryLeaseRecovery; ordinary != nil {
			if err := validateJournalOrdinaryLeaseRecoveryEnrollment(req, ordinary); err != nil {
				return nil, err
			}
		} else if req.JournalEnrollment.Recovery == nil {
			if strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion) == "" {
				return nil, InvalidArgumentErrorf("journal enrollment version is required")
			}
			if err := validateFreshJournalProjectionState(req.JournalEnrollment); err != nil {
				return nil, err
			}
		} else {
			recovery := req.JournalEnrollment.Recovery
			recoveryKey := strings.TrimSpace(recovery.IdempotencyKey)
			if recovery.JournalRunID <= 0 || recovery.SourceCheckpointID <= 0 ||
				strings.TrimSpace(recovery.SourceAttemptID) == "" || recoveryKey == "" {
				return nil, InvalidArgumentErrorf("journal recovery enrollment source is required")
			}
			if strings.TrimSpace(req.Run.IdempotencyKey) != recoveryKey {
				return nil, InvalidArgumentErrorf("journal recovery run idempotency key must match enrollment")
			}
			if strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion) != "" {
				return nil, InvalidArgumentErrorf("journal recovery enrollment version is repository assigned")
			}
			if req.JournalEnrollment.ProjectionState != "" {
				return nil, InvalidArgumentErrorf("journal recovery projection state is repository assigned")
			}
			if lease := recovery.ExpiredLease; lease != nil {
				if lease.RunID <= 0 || strings.TrimSpace(lease.LeaseOwner) == "" ||
					strings.TrimSpace(lease.LeaseToken) == "" || lease.ExecutionGeneration == 0 ||
					lease.Now <= 0 {
					return nil, InvalidArgumentErrorf("journal recovery expired lease fence is required")
				}
			}
		}
	} else if req.JournalEnrollment != nil {
		return nil, InvalidArgumentErrorf("journal enrollment options require journal enrollment")
	}
	strategy, err := normalizeMultitaskStrategy(req.Run.MultitaskStrategy, runKind)
	if err != nil {
		return nil, err
	}
	onDisconnect, err := normalizeOnDisconnectMode(req.Run.OnDisconnect)
	if err != nil {
		return nil, err
	}
	if req.Run.ParentRunID > 0 {
		parent, err := s.repo.GetRun(ctx, req.Run.ParentRunID)
		if err != nil {
			return nil, err
		}
		if parent.ThreadID != req.Run.ThreadID {
			return nil, InvalidArgumentErrorf("parent run thread does not match run thread")
		}
	}
	status, err := normalizeInitialRunStatus(req.Run.Status, runKind)
	if err != nil {
		return nil, err
	}
	if req.Message != nil {
		if !isValidMessageRole(req.Message.Role) {
			return nil, InvalidArgumentErrorf("message role is invalid")
		}
		if strings.TrimSpace(req.Message.Content) == "" {
			return nil, InvalidArgumentErrorf("message content is required")
		}
	}
	if req.Event != nil {
		if strings.TrimSpace(req.Event.EventType) == "" {
			return nil, InvalidArgumentErrorf("run event type is required")
		}
		if req.Event.PayloadBuilder == nil {
			return nil, InvalidArgumentErrorf("run event payload builder is required")
		}
		hasJournalProjection := req.Event.Journal != nil || req.Event.JournalProjectionFailed
		if hasJournalProjection && req.Event.JournalSourceRunID <= 0 {
			return nil, InvalidArgumentErrorf("run event journal source run id is required")
		}
		if !hasJournalProjection && req.Event.JournalSourceRunID != 0 {
			return nil, InvalidArgumentErrorf("run event journal source requires a projection")
		}
		if req.Event.JournalSourceRunID > 0 {
			sourceRun, err := s.repo.GetRun(ctx, req.Event.JournalSourceRunID)
			if err != nil {
				return nil, err
			}
			if sourceRun.ThreadID != req.Run.ThreadID {
				return nil, InvalidArgumentErrorf("run event journal source thread does not match run thread")
			}
		}
	}

	entityCount := 1
	if req.Message != nil {
		entityCount++
	}
	if req.Event != nil {
		entityCount++
	}
	if req.EnrollJournal {
		entityCount++
		if req.JournalEnrollment.Recovery != nil &&
			req.JournalEnrollment.Recovery.ExpiredLease != nil {
			entityCount++
		}
		if req.JournalEnrollment.HumanResume != nil {
			entityCount++
		}
		if req.JournalEnrollment.OrdinaryLeaseRecovery != nil {
			entityCount++
		}
	}
	ids, err := s.idGen.GenMultiIDs(ctx, entityCount)
	if err != nil {
		return nil, err
	}
	if len(ids) != entityCount {
		return nil, fmt.Errorf("agent thread id generator returned %d ids, expected %d", len(ids), entityCount)
	}
	now := time.Now().UnixMilli()
	runReq := req.Run
	runReq.MultitaskStrategy = strategy
	runReq.OnDisconnect = onDisconnect
	run, err := newRunEntity(&runReq, ids[0], thread, runKind, status, input, now)
	if err != nil {
		return nil, err
	}
	nextID := 1
	var message *entity.Message
	if req.Message != nil {
		message = &entity.Message{
			ID:        ids[nextID],
			ThreadID:  run.ThreadID,
			RunID:     run.ID,
			Role:      req.Message.Role,
			Content:   strings.TrimSpace(req.Message.Content),
			Metadata:  req.Message.Metadata,
			CreatedAt: now,
		}
		nextID++
	}
	if req.PersistMessageReference {
		if message == nil {
			return nil, InvalidArgumentErrorf("persisted run message reference requires a message")
		}
		run.Metadata, err = entity.MergeRunMessageReference(run.Metadata, message.ID)
		if err != nil {
			return nil, InvalidArgumentErrorf("persist run message reference: %v", err)
		}
	}
	var event *entity.RunEvent
	var eventJournal *entity.JournalEvent
	var eventJournalSourceRunID int64
	var eventJournalProjectionFailed bool
	if req.Event != nil {
		eventJournalSourceRunID = req.Event.JournalSourceRunID
		eventJournalProjectionFailed = req.Event.JournalProjectionFailed
		event = &entity.RunEvent{
			ID:        ids[nextID],
			ThreadID:  run.ThreadID,
			RunID:     run.ID,
			EventType: strings.TrimSpace(req.Event.EventType),
			Payload:   defaultJSON(req.Event.PayloadBuilder(run.ID), "{}"),
			CreatedAt: now,
		}
		if req.Event.Journal != nil {
			eventJournal = journalEventFromServiceRequest(req.Event.Journal, event.ID, event.ThreadID)
			eventJournal.RunID = event.RunID
			eventJournal.CreatedAt = event.CreatedAt
		}
		nextID++
	}
	var attempt *entity.RunAttempt
	var recoverySourceLease *repository.ReconcileExpiredRunLeaseRequest
	var humanResumeRollover *repository.HumanResumeRolloverRequest
	if req.EnrollJournal {
		if lease := journalEnrollmentExpiredLease(req.JournalEnrollment); lease != nil {
			eventID := ids[nextID]
			nextID++
			ordinaryRecovery := req.JournalEnrollment.OrdinaryLeaseRecovery != nil
			sourceStatus := entity.RunStatusFailed
			sourceEventType := "run.failed"
			if ordinaryRecovery {
				sourceStatus = entity.RunStatusInterrupted
				sourceEventType = "run.interrupted"
			}
			eventPayload, err := terminalRunEventPayload(
				"",
				sourceStatus,
				"",
				strings.TrimSpace(lease.ErrorCode),
			)
			if err != nil {
				return nil, err
			}
			sourceEvent := &entity.RunEvent{
				ID: eventID, ThreadID: run.ThreadID, RunID: lease.RunID,
				EventType: sourceEventType, Payload: eventPayload, CreatedAt: lease.Now,
			}
			var sourceJournalEvent *entity.JournalEvent
			if !ordinaryRecovery {
				sourceJournalEvent, err = newRunTerminalJournalEvent(
					sourceEvent,
					entity.RunStatusFailed,
					lease.ErrorCode,
				)
				if err != nil {
					return nil, err
				}
			}
			recoverySourceLease = &repository.ReconcileExpiredRunLeaseRequest{
				RunID: lease.RunID, LeaseOwner: strings.TrimSpace(lease.LeaseOwner),
				LeaseToken:          strings.TrimSpace(lease.LeaseToken),
				ExecutionGeneration: lease.ExecutionGeneration,
				ToStatus:            sourceStatus,
				Now:                 lease.Now,
				ErrorCode:           strings.TrimSpace(lease.ErrorCode),
				ErrorMessage:        strings.TrimSpace(lease.ErrorMessage),
				Event:               sourceEvent,
				JournalEvent:        sourceJournalEvent,
			}
		}
		attemptStatus, err := journalAttemptStatusForRun(status)
		if err != nil {
			return nil, err
		}
		attemptID := ids[nextID]
		nextID++
		traceID := strings.TrimSpace(req.JournalEnrollment.TraceID)
		attempt = &entity.RunAttempt{
			ID: attemptID, ThreadID: run.ThreadID,
			JournalRunID: run.ID, ExecutionRunID: run.ID,
			AttemptID: fmt.Sprintf("att_%d", attemptID), Ordinal: 1,
			Status:       attemptStatus,
			NextSequence: 1, LastCommittedSequence: 0,
			EnrollmentVersion: strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion),
			SnapshotsEnabled:  req.JournalEnrollment.SnapshotsEnabled,
			ProjectionState:   initialJournalProjectionState(req.JournalEnrollment),
			TraceID:           journalStringPointer(traceID),
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if attempt.ProjectionState == entity.JournalProjectionStateDisabled {
			attempt.SnapshotsEnabled = false
		}
		if recovery := req.JournalEnrollment.Recovery; recovery != nil {
			recoveryKey := strings.TrimSpace(recovery.IdempotencyKey)
			sourceAttemptID := strings.TrimSpace(recovery.SourceAttemptID)
			sourceCheckpointID := recovery.SourceCheckpointID
			attempt.JournalRunID = recovery.JournalRunID
			attempt.Ordinal = 0
			attempt.EnrollmentVersion = ""
			attempt.SnapshotsEnabled = false
			attempt.SourceCheckpointID = &sourceCheckpointID
			attempt.SourceAttemptID = &sourceAttemptID
			attempt.RecoveryIdempotencyKey = &recoveryKey
		} else if human := req.JournalEnrollment.HumanResume; human != nil {
			reentryKey := strings.TrimSpace(human.IdempotencyKey)
			sourceAttemptID := strings.TrimSpace(human.SourceAttemptID)
			sourceCheckpointID := human.SourceCheckpointID
			attempt.JournalRunID = human.JournalRunID
			attempt.Ordinal = 0
			attempt.EnrollmentVersion = ""
			attempt.SnapshotsEnabled = false
			attempt.SourceCheckpointID = &sourceCheckpointID
			attempt.SourceAttemptID = &sourceAttemptID
			attempt.RecoveryIdempotencyKey = &reentryKey

			terminalID := ids[nextID]
			nextID++
			terminalBase, terminalJournal, err := newHumanResumeInterruptedTerminal(
				terminalID,
				run.ThreadID,
				human.SourceRunID,
				run.ID,
				event.ID,
				now,
			)
			if err != nil {
				return nil, err
			}
			humanResumeRollover = &repository.HumanResumeRolloverRequest{
				SourceRunID: human.SourceRunID, TerminalBase: terminalBase, TerminalJournal: terminalJournal,
			}
		} else if ordinary := req.JournalEnrollment.OrdinaryLeaseRecovery; ordinary != nil {
			recoveryKey := strings.TrimSpace(ordinary.IdempotencyKey)
			sourceCheckpointID := ordinary.SourceCheckpointID
			attempt.JournalRunID = ordinary.JournalRunID
			attempt.Ordinal = 1
			attempt.EnrollmentVersion = entity.JournalSchemaVersion
			attempt.SnapshotsEnabled = false
			attempt.ProjectionState = entity.JournalProjectionStateDisabled
			attempt.SourceCheckpointID = &sourceCheckpointID
			if sourceAttemptID := strings.TrimSpace(ordinary.SourceAttemptID); sourceAttemptID != "" {
				attempt.SourceAttemptID = &sourceAttemptID
			}
			attempt.RecoveryIdempotencyKey = &recoveryKey
		} else {
			activeSlot := uint8(1)
			attempt.ActiveSlot = &activeSlot
			if attemptStatus == entity.RunAttemptStatusRunning {
				startedAt := run.StartedAt
				if startedAt <= 0 {
					startedAt = now
				}
				attempt.StartedAt = &startedAt
			}
		}
	}

	result, err := s.repo.CreateRunBundle(ctx, repository.CreateRunBundleRequest{
		Run: run, Message: message, Event: event, Attempt: attempt,
		RecoverySourceLease:          recoverySourceLease,
		OrdinaryLeaseRecovery:        ordinaryLeaseRecoveryRepositoryRequest(req.JournalEnrollment, recoverySourceLease),
		HumanResumeRollover:          humanResumeRollover,
		EventJournalSourceRunID:      eventJournalSourceRunID,
		EventJournal:                 eventJournal,
		EventJournalProjectionFailed: eventJournalProjectionFailed,
		SkipTopLevelAdmission:        req.SkipTopLevelAdmission,
		ValidateIdempotencyReplay:    strings.TrimSpace(req.Run.IdempotencyOperation) != "",
		AllocateInterruptedEventIDs: func(count int) ([]int64, error) {
			return s.idGen.GenMultiIDs(ctx, count)
		},
	})
	if err != nil {
		if errors.Is(err, repository.ErrUnsupportedJournalEnrollmentVersion) {
			return nil, InvalidArgumentErrorf(
				"unsupported journal enrollment version %q",
				strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion),
			)
		}
		return nil, err
	}
	if result == nil || result.Run == nil {
		return nil, fmt.Errorf("agent thread repository returned empty run bundle")
	}
	return &CreateRunBundleResult{
		Run: result.Run, Message: result.Message, Event: result.Event, Attempt: result.Attempt,
		InterruptedRuns: result.InterruptedRuns, InterruptedEvents: result.InterruptedEvents,
		Created: result.Created,
	}, nil
}

func validateFreshJournalProjectionState(enrollment *JournalEnrollmentOptions) error {
	if enrollment == nil {
		return InvalidArgumentErrorf("journal enrollment options are required")
	}
	switch enrollment.ProjectionState {
	case "", entity.JournalProjectionStateHealthy, entity.JournalProjectionStateDisabled:
		return nil
	default:
		return InvalidArgumentErrorf("journal projection state must be healthy or disabled")
	}
}

func countJournalEnrollmentSpecializations(enrollment *JournalEnrollmentOptions) int {
	if enrollment == nil {
		return 0
	}
	count := 0
	if enrollment.Recovery != nil {
		count++
	}
	if enrollment.HumanResume != nil {
		count++
	}
	if enrollment.OrdinaryLeaseRecovery != nil {
		count++
	}
	return count
}

func validateJournalOrdinaryLeaseRecoveryEnrollment(
	req *CreateRunBundleRequest,
	ordinary *JournalOrdinaryLeaseRecoveryEnrollmentOptions,
) error {
	if req == nil || ordinary == nil || ordinary.JournalRunID <= 0 ||
		ordinary.SourceCheckpointID <= 0 || strings.TrimSpace(ordinary.IdempotencyKey) == "" ||
		ordinary.SourceCheckpoint == nil || ordinary.ExpiredLease == nil {
		return InvalidArgumentErrorf("ordinary journal lease recovery source is required")
	}
	checkpoint := ordinary.SourceCheckpoint
	sourceAttemptID := strings.TrimSpace(ordinary.SourceAttemptID)
	if ordinary.ExpiredLease.RunID <= 0 ||
		(sourceAttemptID == "" && ordinary.ExpiredLease.RunID != ordinary.JournalRunID) ||
		checkpoint.ID != ordinary.SourceCheckpointID || checkpoint.ThreadID != req.Run.ThreadID ||
		checkpoint.RunID != ordinary.ExpiredLease.RunID || checkpoint.RuntimeDeletedAt != 0 ||
		checkpoint.CheckpointNS != "eino.adk" || checkpoint.RuntimeType != "eino_adk" ||
		strings.TrimSpace(checkpoint.RuntimeKey) == "" || checkpoint.EnvelopeVersion <= 0 ||
		!json.Valid([]byte(checkpoint.ChannelValues)) || !json.Valid([]byte(checkpoint.ChannelVersions)) ||
		!json.Valid([]byte(checkpoint.PendingSends)) || !json.Valid([]byte(checkpoint.Metadata)) ||
		strings.TrimSpace(req.Run.IdempotencyKey) != strings.TrimSpace(ordinary.IdempotencyKey) {
		return InvalidArgumentErrorf("ordinary journal lease recovery identity is invalid")
	}
	lease := ordinary.ExpiredLease
	if strings.TrimSpace(lease.LeaseOwner) == "" || strings.TrimSpace(lease.LeaseToken) == "" ||
		lease.ExecutionGeneration == 0 || lease.Now <= 0 {
		return InvalidArgumentErrorf("ordinary journal lease recovery fence is required")
	}
	if strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion) != "" ||
		req.JournalEnrollment.SnapshotsEnabled || req.JournalEnrollment.ProjectionState != "" ||
		strings.TrimSpace(req.JournalEnrollment.TraceID) != "" {
		return InvalidArgumentErrorf("ordinary journal lease recovery lifecycle is repository assigned")
	}
	return nil
}

func journalEnrollmentExpiredLease(
	enrollment *JournalEnrollmentOptions,
) *JournalRecoveryExpiredLeaseOptions {
	if enrollment == nil {
		return nil
	}
	if enrollment.Recovery != nil {
		return enrollment.Recovery.ExpiredLease
	}
	if enrollment.OrdinaryLeaseRecovery != nil {
		return enrollment.OrdinaryLeaseRecovery.ExpiredLease
	}
	return nil
}

func ordinaryLeaseRecoveryRepositoryRequest(
	enrollment *JournalEnrollmentOptions,
	lease *repository.ReconcileExpiredRunLeaseRequest,
) *repository.OrdinaryLeaseRecoveryRequest {
	if enrollment == nil || enrollment.OrdinaryLeaseRecovery == nil {
		return nil
	}
	ordinary := enrollment.OrdinaryLeaseRecovery
	checkpoint := *ordinary.SourceCheckpoint
	return &repository.OrdinaryLeaseRecoveryRequest{
		JournalRunID: ordinary.JournalRunID, SourceRunID: ordinary.ExpiredLease.RunID,
		SourceAttemptID:    strings.TrimSpace(ordinary.SourceAttemptID),
		SourceCheckpointID: ordinary.SourceCheckpointID,
		SourceCheckpoint:   &checkpoint,
		IdempotencyKey:     strings.TrimSpace(ordinary.IdempotencyKey), ExpiredLease: lease,
	}
}

func initialJournalProjectionState(enrollment *JournalEnrollmentOptions) entity.JournalProjectionState {
	if enrollment != nil && enrollment.ProjectionState == entity.JournalProjectionStateDisabled {
		return entity.JournalProjectionStateDisabled
	}
	return entity.JournalProjectionStateHealthy
}

func validateJournalHumanResumeEnrollment(
	req *CreateRunBundleRequest,
	human *JournalHumanResumeEnrollmentOptions,
) error {
	if req == nil || human == nil {
		return InvalidArgumentErrorf("journal human resume enrollment is required")
	}
	reentryKey := strings.TrimSpace(human.IdempotencyKey)
	if human.JournalRunID <= 0 || human.SourceRunID <= 0 || human.SourceCheckpointID <= 0 ||
		strings.TrimSpace(human.SourceAttemptID) == "" || reentryKey == "" {
		return InvalidArgumentErrorf("journal human resume enrollment source is required")
	}
	if strings.TrimSpace(req.Run.IdempotencyKey) != reentryKey {
		return InvalidArgumentErrorf("journal human resume run idempotency key must match enrollment")
	}
	if req.Run.Status != entity.RunStatusQueued || req.Run.ParentRunID != 0 ||
		(req.Run.RunKind != "" && req.Run.RunKind != entity.RunKindTask) ||
		strings.TrimSpace(req.Run.MultitaskStrategy) != "reject" {
		return InvalidArgumentErrorf("journal human resume run must be a queued root task with reject admission")
	}
	if strings.TrimSpace(req.JournalEnrollment.EnrollmentVersion) != "" ||
		req.JournalEnrollment.SnapshotsEnabled || req.JournalEnrollment.ProjectionState != "" ||
		strings.TrimSpace(req.JournalEnrollment.TraceID) != "" {
		return InvalidArgumentErrorf("journal human resume enrollment lifecycle is repository assigned")
	}
	if req.Message == nil || req.Message.Role != entity.MessageRoleUser {
		return InvalidArgumentErrorf("journal human resume requires a user message")
	}
	if req.Event == nil || strings.TrimSpace(req.Event.EventType) != "human.interaction.resolved" ||
		req.Event.JournalSourceRunID != human.SourceRunID || req.Event.Journal == nil ||
		req.Event.JournalProjectionFailed {
		return InvalidArgumentErrorf("journal human resume requires a healthy resolved projection")
	}
	projection := req.Event.Journal
	if strings.TrimSpace(projection.EventType) != "confirmation.resolved" ||
		strings.TrimSpace(projection.Status) != "completed" ||
		projection.Visibility != entity.JournalVisibilityUser ||
		projection.JournalRunID != 0 || strings.TrimSpace(projection.AttemptID) != "" ||
		strings.TrimSpace(projection.IdempotencyKey) == "" ||
		strings.TrimSpace(projection.SchemaVersion) != entity.JournalSchemaVersion ||
		strings.TrimSpace(projection.PayloadVersion) != entity.JournalPayloadVersion {
		return InvalidArgumentErrorf("journal human resume requires a healthy resolved projection")
	}
	return nil
}

func newHumanResumeInterruptedTerminal(
	id int64,
	threadID int64,
	sourceRunID int64,
	resumeRunID int64,
	parentEventID int64,
	createdAt int64,
) (*entity.RunEvent, *entity.JournalEvent, error) {
	payload := fmt.Sprintf(
		`{"schema":"coze.journal_attempt_interrupted.v1","status":"interrupted","resume_run_id":%d}`,
		resumeRunID,
	)
	journalPayload := `{"type":"terminal","data":{"status":"interrupted"}}`
	base := &entity.RunEvent{
		ID: id, ThreadID: threadID, RunID: sourceRunID,
		EventType: entity.JournalAttemptInterruptedRunEventType,
		Payload:   payload, CreatedAt: createdAt,
	}
	journal := &entity.JournalEvent{
		ID: id, ThreadID: threadID, RunID: sourceRunID,
		IdempotencyKey: fmt.Sprintf("journal:run:%d:terminal:interrupted", sourceRunID),
		ParentEventID:  parentEventID, SchemaVersion: entity.JournalSchemaVersion,
		Status:             string(entity.RunAttemptStatusInterrupted),
		OccurredAtUnixNano: createdAt * int64(1_000_000),
		Visibility:         entity.JournalVisibilityUser, PayloadVersion: entity.JournalPayloadVersion,
		EventType: "run.lifecycle", Payload: journalPayload, CreatedAt: createdAt,
	}
	return base, journal, nil
}

func newRunEntity(
	req *CreateRunRequest,
	id int64,
	thread *entity.Thread,
	runKind entity.RunKind,
	status entity.RunStatus,
	input string,
	now int64,
) (*entity.Run, error) {
	metadata, err := entity.MergeRunIdempotencyContract(
		req.Metadata,
		req.IdempotencyOperation,
		req.IdempotencyFingerprint,
	)
	if err != nil {
		return nil, InvalidArgumentErrorf("run idempotency contract is invalid: %v", err)
	}
	run := &entity.Run{
		ID:                id,
		ThreadID:          thread.ID,
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
		Metadata:          defaultJSON(metadata, "{}"),
		StreamMode:        defaultJSON(req.StreamMode, `["messages","updates"]`),
		MultitaskStrategy: defaultString(req.MultitaskStrategy, "reject"),
		OnDisconnect:      defaultString(req.OnDisconnect, "cancel"),
		Durability:        defaultString(req.Durability, "async"),
		IdempotencyKey:    strings.TrimSpace(req.IdempotencyKey),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if status == entity.RunStatusRunning {
		run.StartedAt = now
	}
	return run, nil
}

func normalizeMultitaskStrategy(strategy string, runKind entity.RunKind) (string, error) {
	strategy = defaultString(strategy, "reject")
	if runKind == entity.RunKindSubagent {
		return strategy, nil
	}

	switch strategy {
	case "reject", "interrupt", "rollback":
		return strategy, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedMultitaskStrategy, strategy)
	}
}

func normalizeOnDisconnectMode(mode string) (string, error) {
	mode = defaultString(mode, "cancel")
	switch mode {
	case "cancel", "continue":
		return mode, nil
	default:
		return "", InvalidArgumentErrorf("on_disconnect must be cancel or continue")
	}
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

func journalAttemptStatusForRun(status entity.RunStatus) (entity.RunAttemptStatus, error) {
	switch status {
	case entity.RunStatusPending, entity.RunStatusQueued:
		return entity.RunAttemptStatusPending, nil
	case entity.RunStatusRunning:
		return entity.RunAttemptStatusRunning, nil
	default:
		return "", InvalidArgumentErrorf("run status %q cannot start a journal attempt", status)
	}
}

func journalAttemptStatusForRecoveryReplay(status entity.RunStatus) (entity.RunAttemptStatus, error) {
	if mapped, err := journalAttemptStatusForRun(status); err == nil {
		return mapped, nil
	}
	switch status {
	case entity.RunStatusInterrupted:
		return entity.RunAttemptStatusRunning, nil
	case entity.RunStatusSucceeded:
		return entity.RunAttemptStatusCompleted, nil
	case entity.RunStatusFailed:
		return entity.RunAttemptStatusFailed, nil
	case entity.RunStatusCanceled:
		return entity.RunAttemptStatusCancelled, nil
	default:
		return "", InvalidArgumentErrorf("run status %q cannot identify a journal recovery attempt", status)
	}
}

func journalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
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
	if err := s.persistRunEventWithOptionalJournal(
		ctx,
		event,
		req.Journal,
		req.JournalProjectionFailed,
	); err != nil {
		return nil, err
	}

	return event, nil
}

func (s *threadService) CreateJournalAttempt(
	ctx context.Context,
	req *CreateJournalAttemptRequest,
) (*entity.RunAttempt, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil || req.JournalRunID <= 0 || req.ExecutionRunID <= 0 {
		return nil, InvalidArgumentErrorf("journal run id and execution run id are required")
	}
	recoveryKey := strings.TrimSpace(req.RecoveryIdempotencyKey)
	if recoveryKey == "" {
		return nil, InvalidArgumentErrorf("recovery idempotency key is required")
	}
	journalRepo, err := s.journalRepository()
	if err != nil {
		return nil, err
	}
	run, err := s.repo.GetRun(ctx, req.ExecutionRunID)
	if err != nil {
		return nil, err
	}
	runKind, err := normalizeRunKind(run.RunKind, run.ParentRunID)
	if err != nil || runKind != entity.RunKindTask {
		return nil, InvalidArgumentErrorf("journal recovery execution run must be a top-level task")
	}
	status, err := journalAttemptStatusForRecoveryReplay(run.Status)
	if err != nil {
		return nil, err
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	activeSlot := uint8(1)
	attempt := &entity.RunAttempt{
		ID: id, ThreadID: run.ThreadID,
		JournalRunID: req.JournalRunID, ExecutionRunID: req.ExecutionRunID,
		AttemptID: fmt.Sprintf("att_%d", id), Status: status, ActiveSlot: &activeSlot,
		NextSequence: 1, LastCommittedSequence: 0,
		SourceCheckpointID:     req.SourceCheckpointID,
		SourceAttemptID:        req.SourceAttemptID,
		RecoveryIdempotencyKey: &recoveryKey,
		ProjectionState:        entity.JournalProjectionStateHealthy,
		TraceID:                journalStringPointer(strings.TrimSpace(req.TraceID)),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if status == entity.RunAttemptStatusRunning {
		startedAt := run.StartedAt
		if startedAt <= 0 {
			startedAt = now
		}
		attempt.StartedAt = &startedAt
	}
	return journalRepo.CreateJournalAttempt(ctx, attempt)
}

func (s *threadService) AppendJournalEvent(
	ctx context.Context,
	req *AppendJournalEventRequest,
) (*entity.JournalEvent, error) {
	if err := s.requireComponents(); err != nil {
		return nil, err
	}
	if req == nil || req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("run id is required")
	}
	journalRepo, err := s.journalRepository()
	if err != nil {
		return nil, err
	}
	run, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	if req.ThreadID > 0 && req.ThreadID != run.ThreadID {
		return nil, InvalidArgumentErrorf("journal event thread id does not match run thread id")
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	event := journalEventFromServiceRequest(req, id, run.ThreadID)
	return journalRepo.AppendJournalEvent(ctx, event)
}

func (s *threadService) FinalizeJournalAttempt(
	ctx context.Context,
	req *FinalizeJournalAttemptRequest,
) (*entity.JournalEvent, bool, error) {
	if err := s.requireComponents(); err != nil {
		return nil, false, err
	}
	if req == nil || req.Event.RunID <= 0 ||
		!entity.IsLegacyFinalizableRunAttemptStatus(req.Status) {
		return nil, false, InvalidArgumentErrorf("terminal run id and attempt status are required")
	}
	journalRepo, err := s.journalRepository()
	if err != nil {
		return nil, false, err
	}
	run, err := s.repo.GetRun(ctx, req.Event.RunID)
	if err != nil {
		return nil, false, err
	}
	if req.Event.ThreadID > 0 && req.Event.ThreadID != run.ThreadID {
		return nil, false, InvalidArgumentErrorf("journal event thread id does not match run thread id")
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, false, err
	}
	event := journalEventFromServiceRequest(&req.Event, id, run.ThreadID)
	return journalRepo.FinalizeJournalAttempt(ctx, repository.FinalizeJournalAttemptRequest{
		RunID: req.Event.RunID, Status: req.Status, Event: event, EndedAt: req.EndedAt,
	})
}

func (s *threadService) GetJournalEvent(
	ctx context.Context,
	eventID int64,
) (*entity.JournalEvent, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if eventID <= 0 {
		return nil, InvalidArgumentErrorf("journal event id is required")
	}
	journalRepo, err := s.journalRepository()
	if err != nil {
		return nil, err
	}
	return journalRepo.GetJournalEvent(ctx, eventID)
}

func (s *threadService) journalRepository() (repository.JournalRepository, error) {
	repo, ok := s.repo.(repository.JournalRepository)
	if !ok {
		return nil, fmt.Errorf("agent thread journal repository is unavailable")
	}
	return repo, nil
}

func journalEventFromServiceRequest(
	req *AppendJournalEventRequest,
	id, threadID int64,
) *entity.JournalEvent {
	createdAt := req.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	return &entity.JournalEvent{
		ID: id, ThreadID: threadID, RunID: req.RunID,
		JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
		IdempotencyKey: req.IdempotencyKey, ParentEventID: req.ParentEventID,
		SchemaVersion: req.SchemaVersion, Status: req.Status,
		OccurredAtUnixNano: req.OccurredAtUnixNano, Visibility: req.Visibility,
		PayloadVersion: req.PayloadVersion, SnapshotID: req.SnapshotID, TraceID: req.TraceID,
		ActionID: req.ActionID, Phase: req.Phase, Operation: req.Operation,
		Target: req.Target, Milestone: req.Milestone,
		EventType: req.EventType, Payload: req.Payload, CreatedAt: createdAt,
	}
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
	if req.AfterEventID < 0 {
		return nil, 0, InvalidArgumentErrorf("after event id cannot be negative")
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
		ThreadID:     req.ThreadID,
		RunID:        req.RunID,
		AfterEventID: req.AfterEventID,
		Page:         page,
		PageSize:     pageSize,
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
		ThreadID:    req.ThreadID,
		RunID:       req.RunID,
		RuntimeType: strings.TrimSpace(req.RuntimeType),
		Limit:       normalizeCheckpointLimit(req.Limit),
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
	snapshot, err := s.repo.GetTokenUsageSnapshot(ctx, repository.ListTokenUsageRequest{
		ThreadID: threadID,
		RunID:    req.RunID,
		RunIDs:   runIDs,
		Source:   req.Source,
		Page:     page,
		PageSize: pageSize,
	}, req.IncludeChildRuns)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	return snapshot.Rows, snapshot.Total, snapshot.Aggregate, snapshot.RunAggregates, nil
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
	snapshot, err := s.repo.GetTokenUsageSnapshot(ctx, repository.ListTokenUsageRequest{
		ThreadID: req.ThreadID,
		Source:   req.Source,
		Page:     page,
		PageSize: pageSize,
	}, false)
	if err != nil {
		return nil, 0, nil, err
	}
	return snapshot.Rows, snapshot.Total, snapshot.Aggregate, nil
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
	if err := s.requireComponents(); err != nil {
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
	current, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	eventID, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	eventType, err := serviceTerminalRunEventType(req.ToStatus)
	if err != nil {
		return nil, err
	}
	eventPayload, err := terminalRunEventPayload(
		req.EventPayload,
		req.ToStatus,
		"",
		strings.TrimSpace(req.ErrorCode),
	)
	if err != nil {
		return nil, err
	}

	event := &entity.RunEvent{
		ID:        eventID,
		ThreadID:  current.ThreadID,
		RunID:     current.ID,
		EventType: eventType,
		Payload:   eventPayload,
		CreatedAt: now,
	}
	journalEvent, err := newRunTerminalJournalEvent(event, req.ToStatus, req.ErrorCode)
	if err != nil {
		return nil, err
	}
	return s.repo.ReconcileExpiredRunLease(ctx, repository.ReconcileExpiredRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          owner,
		LeaseToken:          token,
		ExecutionGeneration: req.ExecutionGeneration,
		ToStatus:            req.ToStatus,
		Now:                 now,
		ErrorCode:           strings.TrimSpace(req.ErrorCode),
		ErrorMessage:        strings.TrimSpace(req.ErrorMessage),
		Event:               event,
		JournalEvent:        journalEvent,
		OutboxIntent:        bindTerminalOutboxIntent(req.OutboxIntent, event, req.ToStatus),
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

	event := &entity.RunEvent{
		ID:        eventID,
		ThreadID:  current.ThreadID,
		RunID:     current.ID,
		EventType: "run.canceled",
		Payload:   string(payload),
		CreatedAt: now,
	}
	journalEvent, err := newRunTerminalJournalEvent(event, entity.RunStatusCanceled, errorCode)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.RequestRunCancellation(ctx, repository.RequestRunCancellationRequest{
		RunID:        req.RunID,
		Now:          now,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Event:        event,
		JournalEvent: journalEvent,
		OutboxIntent: bindTerminalOutboxIntent(req.OutboxIntent, event, entity.RunStatusCanceled),
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
	expectedTitle := strings.TrimSpace(req.ExpectedThreadTitle)
	threadTitle := strings.TrimSpace(req.ThreadTitle)
	wantsTitleUpdate := threadTitle != "" && threadTitle != expectedTitle
	entityCount := 2
	if wantsTitleUpdate {
		entityCount++
	}
	if req.TerminalCheckpoint != nil {
		entityCount++
	}
	ids, err := s.idGen.GenMultiIDs(ctx, entityCount)
	if err != nil {
		return nil, err
	}
	if len(ids) != entityCount {
		return nil, fmt.Errorf("agent thread id generator returned %d ids, expected %d", len(ids), entityCount)
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	completionPayload, err := terminalRunEventPayload(
		req.CompletionEventPayload,
		entity.RunStatusSucceeded,
		owner,
		"",
	)
	if err != nil {
		return nil, err
	}
	nextID := 1
	var titleEvent *entity.RunEvent
	if wantsTitleUpdate {
		if raw := strings.TrimSpace(req.TitleEventPayload); raw != "" {
			var supplied struct {
				ThreadTitle string `json:"thread_title"`
			}
			if err := json.Unmarshal([]byte(raw), &supplied); err != nil ||
				strings.TrimSpace(supplied.ThreadTitle) != threadTitle {
				return nil, InvalidArgumentErrorf("thread title event payload does not match generated title")
			}
		}
		payload, marshalErr := json.Marshal(map[string]any{"thread_title": threadTitle})
		if marshalErr != nil {
			return nil, marshalErr
		}
		titleEvent = &entity.RunEvent{
			ID: ids[nextID], ThreadID: req.ThreadID, RunID: req.RunID,
			EventType: "context.thread_title_updated", Payload: string(payload), CreatedAt: now,
		}
		nextID++
	}
	completionEvent := &entity.RunEvent{
		ID: ids[nextID], ThreadID: req.ThreadID, RunID: req.RunID,
		EventType: "run.completed", Payload: completionPayload, CreatedAt: now,
	}
	journalEvent, err := newRunTerminalJournalEvent(completionEvent, entity.RunStatusSucceeded, "")
	if err != nil {
		return nil, err
	}
	nextID++
	buildTerminalCheckpoint := func(checkpoint *CreateCheckpointRequest, id int64) (*entity.Checkpoint, error) {
		if checkpoint == nil {
			return nil, nil
		}
		if checkpoint.ParentCheckpointID < 0 || strings.TrimSpace(checkpoint.RuntimeType) == "" ||
			strings.TrimSpace(checkpoint.RuntimeKey) == "" || checkpoint.EnvelopeVersion <= 0 {
			return nil, InvalidArgumentErrorf("terminal checkpoint is invalid")
		}
		return &entity.Checkpoint{
			ID: id, ThreadID: req.ThreadID, RunID: req.RunID,
			ParentCheckpointID: checkpoint.ParentCheckpointID,
			CheckpointNS:       strings.TrimSpace(checkpoint.CheckpointNS),
			RuntimeType:        strings.TrimSpace(checkpoint.RuntimeType),
			RuntimeKey:         strings.TrimSpace(checkpoint.RuntimeKey),
			EnvelopeVersion:    checkpoint.EnvelopeVersion,
			ChannelValues:      defaultJSON(checkpoint.ChannelValues, "{}"),
			ChannelVersions:    defaultJSON(checkpoint.ChannelVersions, "{}"),
			PendingSends:       defaultJSON(checkpoint.PendingSends, "[]"),
			Metadata:           defaultJSON(checkpoint.Metadata, "{}"),
			CreatedAt:          now,
		}, nil
	}
	var terminalCheckpoint *entity.Checkpoint
	var terminalCheckpointOnTitleConflict *entity.Checkpoint
	if req.TerminalCheckpoint != nil {
		terminalCheckpoint, err = buildTerminalCheckpoint(req.TerminalCheckpoint, ids[nextID])
		if err != nil {
			return nil, err
		}
		terminalCheckpointOnTitleConflict, err = buildTerminalCheckpoint(
			req.TerminalCheckpointOnTitleConflict,
			ids[nextID],
		)
		if err != nil {
			return nil, err
		}
	} else if req.TerminalCheckpointOnTitleConflict != nil {
		return nil, InvalidArgumentErrorf("terminal title-conflict checkpoint requires primary checkpoint")
	}

	result, err := s.repo.FinalizeRunSuccess(ctx, repository.FinalizeRunSuccessRequest{
		RunID:               req.RunID,
		LeaseOwner:          owner,
		LeaseToken:          token,
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 now,
		Message: &entity.Message{
			ID:        ids[0],
			ThreadID:  req.ThreadID,
			RunID:     req.RunID,
			Role:      entity.MessageRoleAssistant,
			Content:   messageContent,
			Metadata:  req.MessageMetadata,
			CreatedAt: now,
		},
		TitleEvent:                        titleEvent,
		CompletionEvent:                   completionEvent,
		JournalEvent:                      journalEvent,
		TerminalCheckpoint:                terminalCheckpoint,
		TerminalCheckpointOnTitleConflict: terminalCheckpointOnTitleConflict,
		ExpectedThreadTitle:               expectedTitle,
		ThreadTitle:                       threadTitle,
		OutboxIntent:                      bindTerminalOutboxIntent(req.OutboxIntent, completionEvent, entity.RunStatusSucceeded),
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Run == nil || result.Message == nil {
		return nil, fmt.Errorf("finalize run success returned empty result")
	}
	return &FinalizeRunSuccessResult{
		Run:                result.Run,
		Message:            result.Message,
		TitleEvent:         result.TitleEvent,
		CompletionEvent:    result.CompletionEvent,
		TerminalCheckpoint: result.TerminalCheckpoint,
		TitleUpdated:       result.TitleUpdated,
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
		OutboxIntent: req.OutboxIntent,
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
	if err := s.requireComponents(); err != nil {
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
	if req.EventAlreadyPersisted && to != entity.RunStatusInterrupted {
		return nil, InvalidArgumentErrorf("pre-persisted terminal event is only valid for interrupted runs")
	}
	current, err := s.repo.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, err
	}

	workerID := strings.TrimSpace(req.WorkerID)
	var terminalEvent *entity.RunEvent
	var journalEvent *entity.JournalEvent
	if !req.EventAlreadyPersisted {
		eventID, err := s.idGen.GenID(ctx)
		if err != nil {
			return nil, err
		}
		eventType, err := serviceTerminalRunEventType(to)
		if err != nil {
			return nil, err
		}
		eventPayload, err := terminalRunEventPayload(
			req.EventPayload,
			to,
			workerID,
			strings.TrimSpace(req.ErrorCode),
		)
		if err != nil {
			return nil, err
		}
		terminalEvent = &entity.RunEvent{
			ID: eventID, ThreadID: current.ThreadID, RunID: current.ID,
			EventType: eventType, Payload: eventPayload, CreatedAt: req.Now,
		}
		journalEvent, err = newRunTerminalJournalEvent(
			terminalEvent,
			to,
			strings.TrimSpace(req.ErrorCode),
		)
		if err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateRunStatus(ctx, repository.UpdateRunStatusRequest{
		RunID:                 req.RunID,
		From:                  from,
		To:                    to,
		WorkerID:              workerID,
		LeaseOwner:            strings.TrimSpace(req.LeaseOwner),
		LeaseToken:            strings.TrimSpace(req.LeaseToken),
		ExecutionGeneration:   req.ExecutionGeneration,
		Now:                   req.Now,
		ErrorCode:             strings.TrimSpace(req.ErrorCode),
		ErrorMessage:          strings.TrimSpace(req.ErrorMessage),
		EventPayload:          req.EventPayload,
		Event:                 terminalEvent,
		JournalEvent:          journalEvent,
		EventAlreadyPersisted: req.EventAlreadyPersisted,
		OutboxIntent:          bindTerminalOutboxIntent(req.OutboxIntent, terminalEvent, to),
	}); err != nil {
		return nil, err
	}

	return s.repo.GetRun(ctx, req.RunID)
}

func bindTerminalOutboxIntent(
	intent *repository.NotificationOutboxIntent,
	event *entity.RunEvent,
	status entity.RunStatus,
) *repository.NotificationOutboxIntent {
	if intent != nil && intent.Event.EventType == domainnotification.EventTaskAwaitingInput {
		return intent
	}
	if intent == nil || event == nil || event.ID <= 0 {
		return intent
	}
	bound := *intent
	bound.Event.EventID = fmt.Sprintf("run-event:%d:%s", event.ID, status)
	bound.Event.AggregateVersion = event.ID
	return &bound
}

func serviceTerminalRunEventType(status entity.RunStatus) (string, error) {
	switch status {
	case entity.RunStatusSucceeded:
		return "run.completed", nil
	case entity.RunStatusFailed:
		return "run.failed", nil
	case entity.RunStatusInterrupted:
		return "run.interrupted", nil
	case entity.RunStatusCanceled:
		return "run.canceled", nil
	default:
		return "", InvalidArgumentErrorf("run status %s has no terminal event type", status)
	}
}

func terminalRunEventPayload(
	provided string,
	status entity.RunStatus,
	workerID string,
	errorCode string,
) (string, error) {
	payload := make(map[string]any, 8)
	if rawPayload := strings.TrimSpace(provided); rawPayload != "" {
		var providedFields map[string]any
		if err := json.Unmarshal([]byte(rawPayload), &providedFields); err != nil || providedFields == nil {
			return "", InvalidArgumentErrorf("terminal run event payload must be a JSON object")
		}
		for _, key := range []string{
			"worker_id",
			"error_code",
			"checkpoint_key",
			"interrupt_count",
			"checkpoint_id",
			"checkpoint_ns",
			"resume_from",
			entity.RunAwaitingInputInteractionRefPayloadKey,
		} {
			if value, ok := providedFields[key]; ok {
				if key == entity.RunAwaitingInputInteractionRefPayloadKey {
					ref, err := entity.RunAwaitingInputInteractionRefFromAny(value)
					if err != nil {
						return "", InvalidArgumentErrorf("awaiting-input interaction reference is invalid")
					}
					payload[key] = ref
					continue
				}
				payload[key] = value
			}
		}
	}
	payload["status"] = string(status)
	if workerID = strings.TrimSpace(workerID); workerID != "" {
		payload["worker_id"] = workerID
	}
	if errorCode = strings.TrimSpace(errorCode); errorCode != "" {
		payload["error_code"] = errorCode
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
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
