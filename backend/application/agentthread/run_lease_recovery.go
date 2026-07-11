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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const (
	defaultRunLeaseRecoveryLimit = int32(100)
	runRecoveredErrorCode        = "run_recovered"
	runRecoveredErrorMessage     = "execution recovered from a durable checkpoint"
	runAbandonedErrorCode        = "run_abandoned"
	runAbandonedErrorMessage     = "execution lease expired without a recoverable checkpoint"
	runRecoveryMetadataSchema    = "coze.run_recovery.v1"
)

type RunLeaseRecoveryProcessorOptions struct {
	Limit int32
	Clock RunLeaseClock
}

type RunLeaseRecoveryProcessor struct {
	app   *ApplicationService
	limit int32
	clock RunLeaseClock
}

type RunLeaseRecoveryResult struct {
	ExpiredRuns   int
	RecoveredRuns int
	AbandonedRuns int
	SkippedRuns   int
	ErroredRuns   int
}

func NewRunLeaseRecoveryProcessor(
	app *ApplicationService,
	opts RunLeaseRecoveryProcessorOptions,
) *RunLeaseRecoveryProcessor {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultRunLeaseRecoveryLimit
	}
	clock := opts.Clock
	if clock == nil {
		clock = systemRunLeaseClock{}
	}
	return &RunLeaseRecoveryProcessor{app: app, limit: limit, clock: clock}
}

func (p *RunLeaseRecoveryProcessor) RecoverExpiredRunLeases(
	ctx context.Context,
) (RunLeaseRecoveryResult, error) {
	result := RunLeaseRecoveryResult{}
	if p == nil || p.app == nil || p.app.ThreadSVC == nil {
		return result, fmt.Errorf("agent run lease recovery service is required")
	}
	now := p.clock.Now().UnixMilli()
	expired, err := p.app.ListExpiredRunLeases(ctx, &ListExpiredRunLeasesRequest{
		Now:   now,
		Limit: p.limit,
	})
	if err != nil {
		return result, err
	}
	if expired == nil {
		return result, nil
	}

	result.ExpiredRuns = len(expired.Runs)
	var batchErr error
	for _, run := range expired.Runs {
		outcome, err := p.recoverExpiredRun(ctx, run, now)
		if err != nil {
			if errors.Is(err, repository.ErrRunLeaseLost) {
				result.SkippedRuns++
				continue
			}
			result.ErroredRuns++
			batchErr = errors.Join(batchErr, fmt.Errorf("recover expired run %d: %w", runSummaryID(run), err))
			continue
		}
		switch outcome {
		case runLeaseRecoveryRecovered:
			result.RecoveredRuns++
		case runLeaseRecoveryAbandoned:
			result.AbandonedRuns++
		default:
			result.SkippedRuns++
		}
	}

	return result, batchErr
}

type runLeaseRecoveryOutcome string

const (
	runLeaseRecoverySkipped   runLeaseRecoveryOutcome = "skipped"
	runLeaseRecoveryRecovered runLeaseRecoveryOutcome = "recovered"
	runLeaseRecoveryAbandoned runLeaseRecoveryOutcome = "abandoned"
)

func (p *RunLeaseRecoveryProcessor) recoverExpiredRun(
	ctx context.Context,
	run *RunSummary,
	now int64,
) (runLeaseRecoveryOutcome, error) {
	if run == nil {
		return runLeaseRecoverySkipped, nil
	}
	if run.RunID <= 0 || strings.TrimSpace(run.LeaseOwner) == "" ||
		strings.TrimSpace(run.LeaseToken) == "" || run.ExecutionGeneration == 0 {
		return runLeaseRecoverySkipped, fmt.Errorf("expired run lease fence is incomplete")
	}

	checkpoint, err := p.latestRecoverableCheckpoint(ctx, run)
	if err != nil {
		return runLeaseRecoverySkipped, err
	}
	if checkpoint == nil {
		_, err := p.app.ReconcileExpiredRunLease(ctx, &ReconcileExpiredRunLeaseRequest{
			RunID:               run.RunID,
			LeaseOwner:          run.LeaseOwner,
			LeaseToken:          run.LeaseToken,
			ExecutionGeneration: run.ExecutionGeneration,
			ToStatus:            RunStatusFailed,
			Now:                 now,
			ErrorCode:           runAbandonedErrorCode,
			ErrorMessage:        runAbandonedErrorMessage,
		})
		return runLeaseRecoveryAbandoned, err
	}

	if _, err := p.ensureRecoveryResumeRun(ctx, run, checkpoint); err != nil {
		return runLeaseRecoverySkipped, err
	}
	_, err = p.app.ReconcileExpiredRunLease(ctx, &ReconcileExpiredRunLeaseRequest{
		RunID:               run.RunID,
		LeaseOwner:          run.LeaseOwner,
		LeaseToken:          run.LeaseToken,
		ExecutionGeneration: run.ExecutionGeneration,
		ToStatus:            RunStatusInterrupted,
		Now:                 now,
		ErrorCode:           runRecoveredErrorCode,
		ErrorMessage:        runRecoveredErrorMessage,
	})
	return runLeaseRecoveryRecovered, err
}

func (p *RunLeaseRecoveryProcessor) latestRecoverableCheckpoint(
	ctx context.Context,
	run *RunSummary,
) (*CheckpointSummary, error) {
	checkpoints, _, err := p.app.ThreadSVC.ListCheckpoints(ctx, &domainservice.ListCheckpointsRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Limit:    100,
	})
	if err != nil {
		return nil, err
	}

	var latest *CheckpointSummary
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.RuntimeDeletedAt > 0 {
			continue
		}
		summary := DomainCheckpointToSummary(checkpoint)
		resume := resumeRunPayload{
			CheckpointID: summary.CheckpointID,
			CheckpointNS: summary.CheckpointNS,
			ResumeFrom:   "pending_sends",
		}
		if _, err := loadResumeInput(&RunSummary{
			RunID:    run.RunID,
			ThreadID: run.ThreadID,
		}, resume, summary); err != nil {
			continue
		}
		if latest == nil || summary.CreatedAt > latest.CreatedAt ||
			(summary.CreatedAt == latest.CreatedAt && summary.CheckpointID > latest.CheckpointID) {
			latest = summary
		}
	}
	return latest, nil
}

func (p *RunLeaseRecoveryProcessor) ensureRecoveryResumeRun(
	ctx context.Context,
	source *RunSummary,
	checkpoint *CheckpointSummary,
) (*entity.Run, error) {
	idempotencyKey := fmt.Sprintf("run-recovery:%d:%d", source.RunID, source.ExecutionGeneration)
	existing, err := p.app.ThreadSVC.GetRunByIdempotencyKey(ctx, source.SpaceID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	command, metadata, err := runRecoveryPayloads(source, checkpoint)
	if err != nil {
		return nil, err
	}
	run, err := p.app.ThreadSVC.CreateRun(ctx, &domainservice.CreateRunRequest{
		ThreadID:          source.ThreadID,
		AssistantID:       source.AssistantID,
		RunKind:           entity.RunKindTask,
		Status:            entity.RunStatusQueued,
		Command:           command,
		Input:             `{"messages":[]}`,
		Config:            source.Config,
		Context:           source.Context,
		Metadata:          metadata,
		StreamMode:        source.StreamMode,
		MultitaskStrategy: source.MultitaskStrategy,
		OnDisconnect:      source.OnDisconnect,
		Durability:        source.Durability,
		IdempotencyKey:    idempotencyKey,
	})
	if err == nil {
		if run == nil {
			return nil, fmt.Errorf("agent thread service returned empty recovery run")
		}
		return run, nil
	}

	// A concurrent recovery may win the unique idempotency key race.
	existing, getErr := p.app.ThreadSVC.GetRunByIdempotencyKey(ctx, source.SpaceID, idempotencyKey)
	if getErr == nil && existing != nil {
		return existing, nil
	}
	if getErr != nil {
		return nil, errors.Join(err, getErr)
	}
	return nil, err
}

func runRecoveryPayloads(source *RunSummary, checkpoint *CheckpointSummary) (string, string, error) {
	if source == nil || checkpoint == nil {
		return "", "", fmt.Errorf("run recovery source and checkpoint are required")
	}
	resume := map[string]any{
		"checkpoint_id": checkpoint.CheckpointID,
		"checkpoint_ns": checkpoint.CheckpointNS,
		"resume_from":   "pending_sends",
	}
	command, err := json.Marshal(map[string]any{"resume": resume})
	if err != nil {
		return "", "", fmt.Errorf("marshal run recovery command: %w", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"checkpoint_resume": map[string]any{
			"protected_from_worker_claim": true,
			"checkpoint_id":               checkpoint.CheckpointID,
			"checkpoint_ns":               checkpoint.CheckpointNS,
			"resume_from":                 "pending_sends",
			"source_run_id":               source.RunID,
		},
		"run_recovery": map[string]any{
			"schema":                      runRecoveryMetadataSchema,
			"reason":                      "expired_lease",
			"source_run_id":               source.RunID,
			"source_execution_generation": source.ExecutionGeneration,
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("marshal run recovery metadata: %w", err)
	}
	return string(command), string(metadata), nil
}
