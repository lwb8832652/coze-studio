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

package agentthread

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestRunLeaseRecoveryProcessorCreatesOneResumeAcrossRetry(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(3_000))
	source := expiredRecoveryTestRun(200)
	source.Config = `{
		"runtime":"eino_adk",
		"model":{"id":"model-a"},
		"resources":{"ids":[1]},
		"token_usage":{"input_tokens":3},
		"opaque":{"keep":true},
		"nested":{"requested_policy":"nested","mode":"business","thinking_enabled":true,"reasoning_effort":"high","is_plan_mode":true,"subagent_enabled":true,"max_concurrent_subagents":9},
		"requested_policy":"pro",
		"mode":"pro",
		"thinking_enabled":true,
		"reasoning_effort":"high",
		"is_plan_mode":true,
		"subagent_enabled":true,
		"max_concurrent_subagents":4
	}`
	sourceConfig := source.Config
	source.Context = `{"locale":"zh-CN","configurable":{"reasoning_effort":"high"}}`
	service := newRunLeaseRecoveryTestService(source)
	service.reconcileFailures = 1
	service.checkpoints[source.ID] = []*entity.Checkpoint{
		{
			ID:              503,
			ThreadID:        source.ThreadID,
			RunID:           source.ID,
			CheckpointNS:    "eino.adk",
			RuntimeType:     string(RuntimeModeEinoADK),
			RuntimeKey:      "invalid-newer",
			EnvelopeVersion: 1,
			ChannelValues:   `{"invalid":true}`,
			Metadata:        `{"runtime":"eino_adk"}`,
			CreatedAt:       2_900,
		},
		{
			ID:              502,
			ThreadID:        source.ThreadID,
			RunID:           source.ID,
			CheckpointNS:    "eino.adk",
			RuntimeType:     string(RuntimeModeEinoADK),
			RuntimeKey:      "checkpoint-502",
			EnvelopeVersion: 1,
			ChannelValues: mustADKCheckpointEnvelopeJSON(t, ADKCheckpointEnvelope{
				EnvelopeVersion: 1,
				Runtime:         string(RuntimeModeEinoADK),
				RuntimeVersion:  adkCheckpointRuntimeVersion,
				RuntimeKey:      "checkpoint-502",
				MessageType:     adkCheckpointMessageType,
				Checkpoint:      []byte{1, 2, 3},
				RunRevision:     7,
				CreatedAt:       2_800,
			}),
			Metadata:  `{"runtime":"eino_adk"}`,
			CreatedAt: 2_800,
		},
	}
	processor := NewRunLeaseRecoveryProcessor(
		&ApplicationService{ThreadSVC: service},
		RunLeaseRecoveryProcessorOptions{Limit: 10, Clock: clock},
	)

	first, err := processor.RecoverExpiredRunLeases(context.Background())
	require.ErrorContains(t, err, "temporary reconciliation failure")
	require.Equal(t, RunLeaseRecoveryResult{ExpiredRuns: 1, ErroredRuns: 1}, first)
	require.Len(t, service.createRunReqs, 1)
	require.Equal(t, entity.RunStatusRunning, source.Status)

	second, err := processor.RecoverExpiredRunLeases(context.Background())
	require.NoError(t, err)
	require.Equal(t, RunLeaseRecoveryResult{ExpiredRuns: 1, RecoveredRuns: 1}, second)
	require.Len(t, service.createRunReqs, 1)
	require.Equal(t, entity.RunStatusInterrupted, source.Status)
	require.Equal(t, "run_recovered", source.ErrorCode)
	require.Empty(t, source.LeaseToken)

	created := service.createRunReqs[0]
	require.Equal(t, source.ThreadID, created.ThreadID)
	require.Equal(t, entity.RunStatusQueued, created.Status)
	require.Equal(t, `{"messages":[]}`, created.Input)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"model":{"id":"model-a"},
		"resources":{"ids":[1]},
		"token_usage":{"input_tokens":3},
		"opaque":{"keep":true},
		"nested":{"requested_policy":"nested","mode":"business","thinking_enabled":true,"reasoning_effort":"high","is_plan_mode":true,"subagent_enabled":true,"max_concurrent_subagents":9}
	}`, created.Config)
	require.Equal(t, sourceConfig, source.Config)
	require.Equal(t, source.Context, created.Context)
	require.Equal(t, "reject", created.MultitaskStrategy)
	require.Equal(t, "run-recovery:200:3", created.IdempotencyKey)
	require.JSONEq(t, `{
		"resume": {
			"checkpoint_id": 502,
			"checkpoint_ns": "eino.adk",
			"resume_from": "pending_sends"
		}
	}`, created.Command)
	require.JSONEq(t, `{
		"checkpoint_resume": {
			"protected_from_worker_claim": true,
			"checkpoint_id": 502,
			"checkpoint_ns": "eino.adk",
			"resume_from": "pending_sends",
			"source_run_id": 200
		},
		"run_recovery": {
			"schema": "coze.run_recovery.v1",
			"reason": "expired_lease",
			"source_run_id": 200,
			"source_execution_generation": 3
		}
	}`, created.Metadata)
}

func TestRunLeaseRecoveryProcessorAbandonsExpiredRunWithoutCheckpoint(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(3_000))
	source := expiredRecoveryTestRun(201)
	service := newRunLeaseRecoveryTestService(source)
	processor := NewRunLeaseRecoveryProcessor(
		&ApplicationService{ThreadSVC: service},
		RunLeaseRecoveryProcessorOptions{Limit: 10, Clock: clock},
	)

	result, err := processor.RecoverExpiredRunLeases(context.Background())

	require.NoError(t, err)
	require.Equal(t, RunLeaseRecoveryResult{ExpiredRuns: 1, AbandonedRuns: 1}, result)
	require.Empty(t, service.createRunReqs)
	require.Equal(t, entity.RunStatusFailed, source.Status)
	require.Equal(t, "run_abandoned", source.ErrorCode)
	require.Equal(t, "execution lease expired without a recoverable checkpoint", source.ErrorMessage)
	require.Empty(t, source.LeaseOwner)
	require.Empty(t, source.LeaseToken)
	require.Equal(t, int64(3_000), source.EndedAt)
}

func TestRunLeaseRecoveryProcessorUsesAtomicJournalRecoveryBundle(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(3_000))
	source := expiredRecoveryTestRun(10)
	source.ThreadID = 42
	source.SpaceID = 7
	source.CreatorID = 9
	source.Status = entity.RunStatusRunning
	activeSlot := uint8(1)
	attempt := &entity.RunAttempt{
		ID: 100, ThreadID: 42, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusRunning,
		ActiveSlot: &activeSlot, NextSequence: 5, LastCommittedSequence: 4,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   entity.JournalProjectionStateHealthy,
	}
	ledger := journalRecoveryLedger(
		entity.SideEffectReplayPolicyIdempotentWrite,
		entity.SideEffectLedgerStatusSucceeded,
	)
	repo := &journalRecoveryRepositoryStub{
		attempts: []*entity.RunAttempt{attempt},
		ledgers:  map[string][]*entity.SideEffectLedger{"att_100": {ledger}},
	}
	threadSVC := &recordingThreadService{
		gotRun:           source,
		expiredRunLeases: []*entity.Run{source},
		checkpoints:      []*entity.Checkpoint{journalRecoveryCheckpoint(t, repo.ledgers["att_100"])},
		createdRunBundle: &domainservice.CreateRunBundleResult{
			Run: &entity.Run{ID: 20, ThreadID: 42, SpaceID: 7, CreatorID: 9},
			Attempt: &entity.RunAttempt{
				ID: 200, ThreadID: 42, JournalRunID: 10, ExecutionRunID: 20,
				AttemptID: "att_200", Ordinal: 2, Status: entity.RunAttemptStatusPending,
			},
			Created: true,
		},
	}
	app := &ApplicationService{
		ThreadSVC:                  threadSVC,
		ThreadAuthorizer:           &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:        &recordingWorkspaceAuthorizer{},
		JournalRecoveryRepository:  repo,
		JournalRecoveryIDGenerator: fixedIDGen{},
	}
	processor := NewRunLeaseRecoveryProcessor(
		app,
		RunLeaseRecoveryProcessorOptions{Limit: 10, Clock: clock},
	)

	result, err := processor.RecoverExpiredRunLeases(context.Background())

	require.NoError(t, err)
	require.Equal(t, RunLeaseRecoveryResult{ExpiredRuns: 1, RecoveredRuns: 1}, result)
	require.Nil(t, threadSVC.createRunReq)
	require.Nil(t, threadSVC.reconcileExpiredRunLeaseReq)
	require.NotNil(t, threadSVC.createRunBundleReq)
	recovery := threadSVC.createRunBundleReq.JournalEnrollment.Recovery
	require.NotNil(t, recovery)
	require.Equal(t, "run-recovery:10:10:3", recovery.IdempotencyKey)
	require.NotNil(t, recovery.ExpiredLease)
	require.Equal(t, "worker-a", recovery.ExpiredLease.LeaseOwner)
	require.Equal(t, "lease-10", recovery.ExpiredLease.LeaseToken)
	require.Equal(t, uint64(3), recovery.ExpiredLease.ExecutionGeneration)
}

func expiredRecoveryTestRun(id int64) *entity.Run {
	return &entity.Run{
		ID:                  id,
		ThreadID:            10,
		SpaceID:             20,
		CreatorID:           30,
		AssistantID:         "assistant-a",
		RunKind:             entity.RunKindTask,
		Status:              entity.RunStatusRunning,
		Input:               `{"messages":[{"role":"user","content":"continue"}]}`,
		Config:              `{"runtime":"eino_adk"}`,
		Context:             `{"locale":"zh-CN"}`,
		Metadata:            `{"source":"task"}`,
		StreamMode:          `["messages","updates"]`,
		MultitaskStrategy:   "enqueue",
		OnDisconnect:        "continue",
		Durability:          "async",
		WorkerID:            "worker-a",
		LeaseOwner:          "worker-a",
		LeaseToken:          fmt.Sprintf("lease-%d", id),
		LeaseExpiresAt:      2_000,
		HeartbeatAt:         1_500,
		ExecutionGeneration: 3,
		StartedAt:           1_000,
		CreatedAt:           900,
		UpdatedAt:           1_500,
	}
}

type runLeaseRecoveryTestService struct {
	*recordingThreadService

	mu                sync.Mutex
	sources           map[int64]*entity.Run
	checkpoints       map[int64][]*entity.Checkpoint
	runsByIdempotency map[string]*entity.Run
	createRunReqs     []*domainservice.CreateRunRequest
	reconcileFailures int
	nextRunID         int64
}

func newRunLeaseRecoveryTestService(sources ...*entity.Run) *runLeaseRecoveryTestService {
	service := &runLeaseRecoveryTestService{
		recordingThreadService: &recordingThreadService{},
		sources:                make(map[int64]*entity.Run, len(sources)),
		checkpoints:            make(map[int64][]*entity.Checkpoint),
		runsByIdempotency:      make(map[string]*entity.Run),
		nextRunID:              1_000,
	}
	for _, source := range sources {
		service.sources[source.ID] = source
	}
	return service
}

func (s *runLeaseRecoveryTestService) ListExpiredRunLeases(
	ctx context.Context,
	req *domainservice.ListExpiredRunLeasesRequest,
) ([]*entity.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*entity.Run, 0)
	for _, run := range s.sources {
		if run.Status == entity.RunStatusRunning && run.LeaseExpiresAt > 0 && run.LeaseExpiresAt <= req.Now {
			copy := *run
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (s *runLeaseRecoveryTestService) ListCheckpoints(
	ctx context.Context,
	req *domainservice.ListCheckpointsRequest,
) ([]*entity.Checkpoint, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	checkpoints := s.checkpoints[req.RunID]
	return checkpoints, int64(len(checkpoints)), nil
}

func (s *runLeaseRecoveryTestService) GetRunByIdempotencyKey(
	ctx context.Context,
	spaceID int64,
	idempotencyKey string,
) (*entity.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runsByIdempotency[idempotencyKey], nil
}

func (s *runLeaseRecoveryTestService) CreateRun(
	ctx context.Context,
	req *domainservice.CreateRunRequest,
) (*entity.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.runsByIdempotency[req.IdempotencyKey]; existing != nil {
		return nil, errors.New("duplicate idempotency key")
	}
	s.createRunReqs = append(s.createRunReqs, req)
	run := &entity.Run{
		ID:                  s.nextRunID,
		ThreadID:            req.ThreadID,
		AssistantID:         req.AssistantID,
		RunKind:             req.RunKind,
		Status:              req.Status,
		Command:             req.Command,
		Input:               req.Input,
		Config:              req.Config,
		Context:             req.Context,
		Metadata:            req.Metadata,
		StreamMode:          req.StreamMode,
		MultitaskStrategy:   req.MultitaskStrategy,
		OnDisconnect:        req.OnDisconnect,
		Durability:          req.Durability,
		IdempotencyKey:      req.IdempotencyKey,
		ExecutionGeneration: 0,
	}
	s.nextRunID++
	s.runsByIdempotency[req.IdempotencyKey] = run
	return run, nil
}

func (s *runLeaseRecoveryTestService) ReconcileExpiredRunLease(
	ctx context.Context,
	req *domainservice.ReconcileExpiredRunLeaseRequest,
) (*entity.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reconcileFailures > 0 {
		s.reconcileFailures--
		return nil, errors.New("temporary reconciliation failure")
	}
	run := s.sources[req.RunID]
	if run == nil || run.Status != entity.RunStatusRunning || run.LeaseOwner != req.LeaseOwner ||
		run.LeaseToken != req.LeaseToken || run.ExecutionGeneration != req.ExecutionGeneration ||
		run.LeaseExpiresAt <= 0 || run.LeaseExpiresAt > req.Now {
		return nil, repository.ErrRunLeaseLost
	}
	run.Status = req.ToStatus
	run.ErrorCode = req.ErrorCode
	run.ErrorMessage = req.ErrorMessage
	run.WorkerID = ""
	run.LeaseOwner = ""
	run.LeaseToken = ""
	run.LeaseExpiresAt = 0
	run.HeartbeatAt = 0
	run.CancelRequestedAt = 0
	run.EndedAt = req.Now
	run.UpdatedAt = req.Now
	copy := *run
	return &copy, nil
}
