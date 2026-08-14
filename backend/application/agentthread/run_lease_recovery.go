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
	runRecoveryConfirmErrorCode  = "recovery_confirm_required"
	runRecoveryConfirmErrorText  = "execution recovery requires user confirmation"
	runRecoveryUnsafeErrorCode   = "recovery_checkpoint_unsafe"
	runRecoveryUnsafeErrorText   = "execution checkpoint cannot be recovered safely"
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

	attempt, enrolled, err := p.activeJournalRecoveryAttempt(ctx, run)
	if err != nil {
		return runLeaseRecoverySkipped, err
	}
	if enrolled && attempt.ProjectionState != entity.JournalProjectionStateDisabled {
		return p.recoverExpiredJournalRun(ctx, run, attempt, now)
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

	if _, err := p.ensureRecoveryResumeRunAt(ctx, run, checkpoint, attempt, now); err != nil {
		return runLeaseRecoverySkipped, err
	}
	return runLeaseRecoveryRecovered, nil
}

func (p *RunLeaseRecoveryProcessor) activeJournalRecoveryAttempt(
	ctx context.Context,
	run *RunSummary,
) (*entity.RunAttempt, bool, error) {
	if p == nil || p.app == nil || p.app.JournalRecoveryRepository == nil {
		return nil, false, nil
	}
	attempt, err := p.app.JournalRecoveryRepository.GetActiveJournalAttempt(ctx, run.RunID)
	if errors.Is(err, repository.ErrJournalNotEnrolled) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if attempt == nil || attempt.ExecutionRunID != run.RunID ||
		attempt.ThreadID != run.ThreadID || !attempt.Status.IsActive() {
		return nil, true, fmt.Errorf("expired run journal attempt does not belong to execution")
	}
	return attempt, true, nil
}

func (p *RunLeaseRecoveryProcessor) recoverExpiredJournalRun(
	ctx context.Context,
	run *RunSummary,
	attempt *entity.RunAttempt,
	now int64,
) (runLeaseRecoveryOutcome, error) {
	if attempt == nil {
		return runLeaseRecoverySkipped, fmt.Errorf("expired run journal attempt is required")
	}
	idempotencyKey := fmt.Sprintf(
		"run-recovery:%d:%d:%d",
		attempt.JournalRunID,
		run.RunID,
		run.ExecutionGeneration,
	)
	result, err := p.app.RecoverJournal(ctx, RecoverJournalRequest{
		ViewerID: run.CreatorID, SpaceID: run.SpaceID, ThreadID: run.ThreadID,
		RunID: attempt.JournalRunID, SourceAttemptID: attempt.AttemptID,
		Action: JournalRecoveryActionResume, IdempotencyKey: idempotencyKey,
		TraceID: idempotencyKey,
		expiredLease: &journalRecoveryExpiredLease{
			RunID: run.RunID, LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
			ExecutionGeneration: run.ExecutionGeneration, Now: now,
			ErrorCode: runRecoveredErrorCode, ErrorMessage: runRecoveredErrorMessage,
		},
	})
	if err == nil {
		if result == nil || !result.Accepted || result.Run == nil || result.Attempt == nil {
			return runLeaseRecoverySkipped, fmt.Errorf("journal lease recovery returned an incomplete bundle")
		}
		return runLeaseRecoveryRecovered, nil
	}

	switch {
	case errors.Is(err, ErrJournalRecoveryConfirmRequired):
		return p.abandonExpiredJournalRun(
			ctx, run, now, runRecoveryConfirmErrorCode, runRecoveryConfirmErrorText,
		)
	case errors.Is(err, ErrJournalRecoveryCheckpointInvalid),
		errors.Is(err, ErrJournalRecoveryCheckpointUnsafe):
		return p.abandonExpiredJournalRun(
			ctx, run, now, runRecoveryUnsafeErrorCode, runRecoveryUnsafeErrorText,
		)
	default:
		return runLeaseRecoverySkipped, err
	}
}

func (p *RunLeaseRecoveryProcessor) abandonExpiredJournalRun(
	ctx context.Context,
	run *RunSummary,
	now int64,
	errorCode, errorMessage string,
) (runLeaseRecoveryOutcome, error) {
	_, err := p.app.ReconcileExpiredRunLease(ctx, &ReconcileExpiredRunLeaseRequest{
		RunID: run.RunID, LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		ExecutionGeneration: run.ExecutionGeneration,
		ToStatus:            RunStatusFailed, Now: now,
		ErrorCode: errorCode, ErrorMessage: errorMessage,
	})
	return runLeaseRecoveryAbandoned, err
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
	return p.ensureRecoveryResumeRunAt(ctx, source, checkpoint, nil, p.clock.Now().UnixMilli())
}

func (p *RunLeaseRecoveryProcessor) ensureRecoveryResumeRunAt(
	ctx context.Context,
	source *RunSummary,
	checkpoint *CheckpointSummary,
	sourceAttempt *entity.RunAttempt,
	now int64,
) (*entity.Run, error) {
	journalRunID := source.RunID
	sourceAttemptID := ""
	idempotencyKey := fmt.Sprintf("run-recovery:%d:%d", source.RunID, source.ExecutionGeneration)
	if sourceAttempt != nil {
		journalRunID = sourceAttempt.JournalRunID
		sourceAttemptID = sourceAttempt.AttemptID
		idempotencyKey = fmt.Sprintf(
			"run-recovery:%d:%d:%d",
			journalRunID,
			source.RunID,
			source.ExecutionGeneration,
		)
	}
	command, metadata, err := runRecoveryPayloads(source, checkpoint)
	if err != nil {
		return nil, err
	}
	recoveryConfig, err := stripSubmittedExecutionControls(source.Config)
	if err != nil {
		return nil, err
	}
	bundle, err := p.app.ThreadSVC.CreateRunBundle(ctx, &domainservice.CreateRunBundleRequest{
		Run: domainservice.CreateRunRequest{
			ThreadID:          source.ThreadID,
			AssistantID:       source.AssistantID,
			RunKind:           entity.RunKindTask,
			Status:            entity.RunStatusQueued,
			Command:           command,
			Input:             `{"messages":[]}`,
			Config:            recoveryConfig,
			Context:           source.Context,
			Metadata:          metadata,
			StreamMode:        source.StreamMode,
			MultitaskStrategy: "reject",
			OnDisconnect:      source.OnDisconnect,
			Durability:        source.Durability,
			IdempotencyKey:    idempotencyKey,
		},
		EnrollJournal: true,
		JournalEnrollment: &domainservice.JournalEnrollmentOptions{
			OrdinaryLeaseRecovery: &domainservice.JournalOrdinaryLeaseRecoveryEnrollmentOptions{
				JournalRunID: journalRunID, SourceCheckpointID: checkpoint.CheckpointID,
				SourceCheckpoint: checkpointSummaryToDomainCheckpoint(checkpoint),
				SourceAttemptID:  sourceAttemptID, IdempotencyKey: idempotencyKey,
				ExpiredLease: &domainservice.JournalRecoveryExpiredLeaseOptions{
					RunID: source.RunID, LeaseOwner: source.LeaseOwner, LeaseToken: source.LeaseToken,
					ExecutionGeneration: source.ExecutionGeneration, Now: now,
					ErrorCode: runRecoveredErrorCode, ErrorMessage: runRecoveredErrorMessage,
				},
			},
		},
	})
	if err == nil {
		if bundle == nil || bundle.Run == nil || bundle.Attempt == nil {
			return nil, fmt.Errorf("agent thread service returned incomplete recovery bundle")
		}
		return bundle.Run, nil
	}

	return nil, err
}

func checkpointSummaryToDomainCheckpoint(checkpoint *CheckpointSummary) *entity.Checkpoint {
	if checkpoint == nil {
		return nil
	}
	return &entity.Checkpoint{
		ID: checkpoint.CheckpointID, ThreadID: checkpoint.ThreadID, RunID: checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID, CheckpointNS: checkpoint.CheckpointNS,
		RuntimeType: checkpoint.RuntimeType, RuntimeKey: checkpoint.RuntimeKey,
		EnvelopeVersion: checkpoint.EnvelopeVersion, RuntimeDeletedAt: checkpoint.RuntimeDeletedAt,
		ChannelValues: checkpoint.ChannelValues, ChannelVersions: checkpoint.ChannelVersions,
		PendingSends: checkpoint.PendingSends, Metadata: checkpoint.Metadata,
		CreatedAt: checkpoint.CreatedAt,
	}
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
