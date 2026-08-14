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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestSelectJournalRecoverySourceRejectsInterruptedExplicitAndImplicit(t *testing.T) {
	attempt := &domainentity.RunAttempt{
		ID: 99, AttemptID: "att_interrupted", Ordinal: 9,
		Status: domainentity.RunAttemptStatusInterrupted,
	}

	require.Nil(t, selectJournalRecoverySourceAttempt(
		[]*domainentity.RunAttempt{attempt}, attempt.AttemptID,
	))
	require.Nil(t, selectJournalRecoverySourceAttempt(
		[]*domainentity.RunAttempt{attempt}, "",
	))
}

func TestJournalRecoveryCreatesAtomicRecoveryBundleFromSafeCheckpoint(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	legacyConfig := `{
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
	legacyContext := `{"configurable":{"is_plan_mode":true,"subagent_enabled":true}}`
	threadSVC.gotRun.Config = legacyConfig
	threadSVC.gotRun.Context = legacyContext
	registry := prometheus.NewRegistry()
	metrics, err := NewJournalPrometheusMetricsCollector(registry)
	require.NoError(t, err)
	telemetrySink := &journalTelemetrySinkStub{}
	app.JournalMetrics = metrics
	app.JournalTelemetry = NewJournalTelemetry(telemetrySink)

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-1", TraceID: "trace-1",
	})

	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Equal(t, "att_200", result.Attempt.AttemptID)
	require.NotNil(t, threadSVC.createRunBundleReq)
	require.True(t, threadSVC.createRunBundleReq.EnrollJournal)
	require.Equal(t, "recover-1", threadSVC.createRunBundleReq.Run.IdempotencyKey)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"model":{"id":"model-a"},
		"resources":{"ids":[1]},
		"token_usage":{"input_tokens":3},
		"opaque":{"keep":true},
		"nested":{"requested_policy":"nested","mode":"business","thinking_enabled":true,"reasoning_effort":"high","is_plan_mode":true,"subagent_enabled":true,"max_concurrent_subagents":9}
	}`, threadSVC.createRunBundleReq.Run.Config)
	require.Equal(t, legacyConfig, threadSVC.gotRun.Config)
	require.Equal(t, legacyContext, threadSVC.createRunBundleReq.Run.Context)
	require.NotNil(t, threadSVC.createRunBundleReq.JournalEnrollment)
	require.NotNil(t, threadSVC.createRunBundleReq.JournalEnrollment.Recovery)
	require.Equal(t, int64(10), threadSVC.createRunBundleReq.JournalEnrollment.Recovery.JournalRunID)
	require.Equal(t, int64(700), threadSVC.createRunBundleReq.JournalEnrollment.Recovery.SourceCheckpointID)
	require.Equal(t, "att_100", threadSVC.createRunBundleReq.JournalEnrollment.Recovery.SourceAttemptID)
	require.Equal(t, "recover-1", threadSVC.createRunBundleReq.JournalEnrollment.Recovery.IdempotencyKey)
	require.Empty(t, repo.resolveRequests)
	require.Equal(t, []JournalRecoveryTelemetryEvent{{
		EventName: "journal_recovery_result", RunID: 10, AttemptID: "att_200", TraceID: "trace-1",
		Action: "resume", Result: "success", ErrorCode: "none",
		Version: domainentity.JournalSchemaVersion, RolloutCohort: "treatment", TaskType: "unknown",
	}}, telemetrySink.events)
	require.Equal(t, float64(1), testutil.ToFloat64(metrics.recoveryResultsTotal.With(
		prometheus.Labels((JournalMetricLabels{
			Version: domainentity.JournalSchemaVersion, RolloutCohort: "treatment", TaskType: "unknown",
			ClientVersion: "unknown", Result: "success", ErrorCode: "none",
		}).prometheusLabels()),
	)))
}

func TestJournalRecoveryTelemetryFailureDoesNotChangeRecoveryResult(t *testing.T) {
	app, _, _ := newJournalRecoveryTestService(t)
	app.JournalTelemetry = NewJournalTelemetry(&journalTelemetrySinkStub{err: errors.New("telemetry unavailable")})

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-telemetry-failure",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
}

func TestJournalRecoverySameKeyReturnsExistingPhysicalRunAndAttempt(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	recoveryKey := "recover-replay-1"
	repo.attempts = append(repo.attempts, &domainentity.RunAttempt{
		ID: 200, ThreadID: 42, JournalRunID: 10, ExecutionRunID: 20,
		AttemptID: "att_200", Ordinal: 2, Status: domainentity.RunAttemptStatusPending,
		RecoveryIdempotencyKey: &recoveryKey,
	})
	threadSVC.idempotentRun = &domainentity.Run{
		ID: 20, ThreadID: 42, SpaceID: 7, CreatorID: 9,
		RunKind: domainentity.RunKindTask, Status: domainentity.RunStatusQueued,
		IdempotencyKey: recoveryKey,
	}

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: recoveryKey,
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.False(t, result.Created)
	require.Equal(t, int64(20), result.Run.RunID)
	require.Equal(t, "att_200", result.Attempt.AttemptID)
	require.Nil(t, threadSVC.createRunBundleReq)
}

func TestJournalRecoveryAllowsOnlyLeaseFencedActiveSource(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	activeSlot := uint8(1)
	repo.attempts[0].Status = domainentity.RunAttemptStatusRunning
	repo.attempts[0].ActiveSlot = &activeSlot
	threadSVC.gotRun.Status = domainentity.RunStatusRunning

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		SourceAttemptID: "att_100", Action: JournalRecoveryActionResume,
		IdempotencyKey: "lease-recover-without-fence",
	})
	require.ErrorIs(t, err, ErrJournalRecoveryConflict)
	require.Nil(t, threadSVC.createRunBundleReq)

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		SourceAttemptID: "att_100", Action: JournalRecoveryActionResume,
		IdempotencyKey: "lease-recover-1",
		expiredLease: &journalRecoveryExpiredLease{
			RunID: 10, LeaseOwner: "worker-a", LeaseToken: "lease-10",
			ExecutionGeneration: 3, Now: 2000,
			ErrorCode: runRecoveredErrorCode, ErrorMessage: runRecoveredErrorMessage,
		},
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.NotNil(t, threadSVC.createRunBundleReq)
	recovery := threadSVC.createRunBundleReq.JournalEnrollment.Recovery
	require.NotNil(t, recovery)
	require.NotNil(t, recovery.ExpiredLease)
	require.Equal(t, int64(10), recovery.ExpiredLease.RunID)
	require.Equal(t, "worker-a", recovery.ExpiredLease.LeaseOwner)
	require.Equal(t, uint64(3), recovery.ExpiredLease.ExecutionGeneration)
}

func TestJournalRecoveryUnknownRequiresControlledConfirmation(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	repo.ledgers["att_100"] = []*domainentity.SideEffectLedger{
		journalRecoveryLedger(domainentity.SideEffectReplayPolicyNonReplayable, domainentity.SideEffectLedgerStatusUnknown),
	}
	threadSVC.checkpoints = []*domainentity.Checkpoint{
		journalRecoveryCheckpoint(t, repo.ledgers["att_100"]),
	}

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-unknown-1",
	})
	require.ErrorIs(t, err, ErrJournalRecoveryConfirmRequired)
	require.Nil(t, threadSVC.createRunBundleReq)
	require.Empty(t, repo.resolveRequests)

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionSkip, Confirmed: true,
		IdempotencyKey: "recover-unknown-2", TraceID: "trace-2",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Len(t, repo.resolveRequests, 1)
	require.Equal(t, domainentity.SideEffectResolutionActionSkip, repo.resolveRequests[0].Action)
	require.Contains(t, repo.resolveRequests[0].IdempotencyKey, "recover-unknown-2")
	require.Contains(t, threadSVC.createRunBundleReq.Run.Command, `"source_idempotency_key":"tool-1"`)
	require.Contains(t, threadSVC.createRunBundleReq.Run.Command, `"action_kind":"write_file"`)
	require.Contains(t, threadSVC.createRunBundleReq.Run.Command, `"request_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
	require.NotContains(t, threadSVC.createRunBundleReq.Run.Command, `"arguments"`)
}

func TestJournalRecoveryExecutingSideEffectBecomesUnknownBeforeConfirmation(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	repo.ledgers["att_100"] = []*domainentity.SideEffectLedger{
		journalRecoveryLedger(domainentity.SideEffectReplayPolicyIdempotentWrite, domainentity.SideEffectLedgerStatusExecuting),
	}
	threadSVC.checkpoints = []*domainentity.Checkpoint{
		journalRecoveryCheckpoint(t, repo.ledgers["att_100"]),
	}

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-executing",
	})
	require.ErrorIs(t, err, ErrJournalRecoveryConfirmRequired)
	require.Len(t, repo.transitionRequests, 1)
	require.Equal(t, domainentity.SideEffectLedgerStatusExecuting, repo.transitionRequests[0].FromStatus)
	require.Equal(t, domainentity.SideEffectLedgerStatusUnknown, repo.transitionRequests[0].ToStatus)
}

func TestJournalRecoveryConfirmationAcceptsRecordedExecutingToUnknownProgress(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	repo.ledgers["att_100"] = []*domainentity.SideEffectLedger{
		journalRecoveryLedger(
			domainentity.SideEffectReplayPolicyIdempotentWrite,
			domainentity.SideEffectLedgerStatusExecuting,
		),
	}
	threadSVC.checkpoints = []*domainentity.Checkpoint{
		journalRecoveryCheckpoint(t, repo.ledgers["att_100"]),
	}

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-record-unknown",
	})
	require.ErrorIs(t, err, ErrJournalRecoveryConfirmRequired)
	require.Len(t, repo.transitionRequests, 1)

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionSkip, Confirmed: true,
		IdempotencyKey: "recover-confirm-unknown",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Len(t, repo.resolveRequests, 1)
	require.Equal(t, domainentity.SideEffectResolutionActionSkip, repo.resolveRequests[0].Action)
}

func TestJournalRecoveryNonReplayableTerminalLedgerRequiresConfirmation(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	repo.ledgers["att_100"] = []*domainentity.SideEffectLedger{
		journalRecoveryLedger(
			domainentity.SideEffectReplayPolicyNonReplayable,
			domainentity.SideEffectLedgerStatusSucceeded,
		),
	}
	threadSVC.checkpoints = []*domainentity.Checkpoint{
		journalRecoveryCheckpoint(t, repo.ledgers["att_100"]),
	}

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-non-replayable-1",
	})
	require.ErrorIs(t, err, ErrJournalRecoveryConfirmRequired)
	require.Nil(t, threadSVC.createRunBundleReq)

	result, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, Confirmed: true,
		IdempotencyKey: "recover-non-replayable-2",
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
}

func TestJournalRecoveryAuditIdempotencyKeysStayBounded(t *testing.T) {
	app, threadSVC, repo := newJournalRecoveryTestService(t)
	repo.ledgers["att_100"] = []*domainentity.SideEffectLedger{
		journalRecoveryLedger(
			domainentity.SideEffectReplayPolicyIdempotentWrite,
			domainentity.SideEffectLedgerStatusExecuting,
		),
	}
	threadSVC.checkpoints = []*domainentity.Checkpoint{
		journalRecoveryCheckpoint(t, repo.ledgers["att_100"]),
	}

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionSkip, Confirmed: true,
		IdempotencyKey: strings.Repeat("r", 180),
	})
	require.NoError(t, err)
	require.Len(t, repo.transitionRequests, 1)
	require.LessOrEqual(t, len(repo.transitionRequests[0].AuditEvent.IdempotencyKey), 191)
	require.Len(t, repo.resolveRequests, 1)
	require.LessOrEqual(t, len(repo.resolveRequests[0].AuditEvent.IdempotencyKey), 191)
}

func TestJournalRecoveryRejectsCorruptCheckpointAndActiveDifferentKey(t *testing.T) {
	t.Run("corrupt checkpoint", func(t *testing.T) {
		app, threadSVC, _ := newJournalRecoveryTestService(t)
		threadSVC.checkpoints[0].ChannelValues = `{`

		_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
			ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
			Action: JournalRecoveryActionResume, IdempotencyKey: "recover-corrupt",
		})
		require.ErrorIs(t, err, ErrJournalRecoveryCheckpointInvalid)
		require.Nil(t, threadSVC.createRunBundleReq)
	})

	t.Run("active different key", func(t *testing.T) {
		app, threadSVC, repo := newJournalRecoveryTestService(t)
		otherKey := "recover-other"
		repo.attempts = append(repo.attempts, &domainentity.RunAttempt{
			ID: 201, ThreadID: 42, JournalRunID: 10, ExecutionRunID: 21,
			AttemptID: "att_201", Ordinal: 2, Status: domainentity.RunAttemptStatusRunning,
			RecoveryIdempotencyKey: &otherKey,
		})

		_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
			ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
			Action: JournalRecoveryActionResume, IdempotencyKey: "recover-new",
		})
		require.ErrorIs(t, err, ErrJournalRecoveryConflict)
		require.Nil(t, threadSVC.createRunBundleReq)
	})
}

func TestJournalRecoveryRechecksTaskACL(t *testing.T) {
	app, _, _ := newJournalRecoveryTestService(t)
	app.ThreadAuthorizer = &recordingThreadAuthorizer{err: ErrThreadAccessDenied}

	_, err := app.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 99, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recover-denied",
	})
	require.ErrorIs(t, err, ErrThreadAccessDenied)
}

type journalRecoveryRepositoryStub struct {
	attempts           []*domainentity.RunAttempt
	ledgers            map[string][]*domainentity.SideEffectLedger
	transitionRequests []domainrepo.TransitionSideEffectRequest
	resolveRequests    []domainrepo.ResolveUnknownSideEffectRequest
}

func (r *journalRecoveryRepositoryStub) GetActiveJournalAttempt(
	_ context.Context,
	runID int64,
) (*domainentity.RunAttempt, error) {
	for index := len(r.attempts) - 1; index >= 0; index-- {
		attempt := r.attempts[index]
		if attempt == nil || !attempt.Status.IsActive() {
			continue
		}
		if attempt.JournalRunID == runID || attempt.ExecutionRunID == runID {
			return attempt, nil
		}
	}
	if len(r.attempts) == 0 {
		return nil, domainrepo.ErrJournalNotEnrolled
	}
	return nil, domainrepo.ErrJournalAttemptTerminal
}

func (r *journalRecoveryRepositoryStub) ListJournalAttempts(
	_ context.Context,
	_ int64,
) ([]*domainentity.RunAttempt, error) {
	return r.attempts, nil
}

func (r *journalRecoveryRepositoryStub) ListSideEffectLedgers(
	_ context.Context,
	_ int64,
	attemptID string,
) ([]*domainentity.SideEffectLedger, error) {
	return r.ledgers[attemptID], nil
}

func (r *journalRecoveryRepositoryStub) TransitionSideEffect(
	_ context.Context,
	req domainrepo.TransitionSideEffectRequest,
) (*domainentity.SideEffectLedger, bool, error) {
	r.transitionRequests = append(r.transitionRequests, req)
	ledger := r.findLedger(req.AttemptID, req.LedgerID)
	copy := *ledger
	copy.Status = req.ToStatus
	copy.Version = req.ExpectedVersion + 1
	r.replaceLedger(req.AttemptID, &copy)
	return &copy, true, nil
}

func (r *journalRecoveryRepositoryStub) ResolveUnknownSideEffect(
	_ context.Context,
	req domainrepo.ResolveUnknownSideEffectRequest,
) (*domainentity.SideEffectLedger, bool, error) {
	r.resolveRequests = append(r.resolveRequests, req)
	ledger := r.findLedger(req.AttemptID, req.LedgerID)
	copy := *ledger
	copy.ResolutionAction = req.Action
	copy.ResolutionIdempotencyKey = req.IdempotencyKey
	copy.Version = req.ExpectedVersion + 1
	r.replaceLedger(req.AttemptID, &copy)
	return &copy, true, nil
}

func (r *journalRecoveryRepositoryStub) findLedger(
	attemptID string,
	ledgerID int64,
) *domainentity.SideEffectLedger {
	for _, ledger := range r.ledgers[attemptID] {
		if ledger != nil && ledger.ID == ledgerID {
			return ledger
		}
	}
	return nil
}

func (r *journalRecoveryRepositoryStub) replaceLedger(
	attemptID string,
	updated *domainentity.SideEffectLedger,
) {
	for index, ledger := range r.ledgers[attemptID] {
		if ledger != nil && ledger.ID == updated.ID {
			r.ledgers[attemptID][index] = updated
			return
		}
	}
}

func newJournalRecoveryTestService(
	t *testing.T,
) (*ApplicationService, *recordingThreadService, *journalRecoveryRepositoryStub) {
	t.Helper()
	rootRun := &domainentity.Run{
		ID: 10, ThreadID: 42, SpaceID: 7, CreatorID: 9, AssistantID: "assistant-1",
		RunKind: domainentity.RunKindTask, Status: domainentity.RunStatusFailed,
		Command: `{}`, Input: `{"messages":[]}`, Config: `{}`, Context: `{}`,
		Metadata: `{}`, StreamMode: `["messages","updates"]`, MultitaskStrategy: "reject",
		OnDisconnect: "continue", Durability: "async",
	}
	attempt := &domainentity.RunAttempt{
		ID: 100, ThreadID: 42, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att_100", Ordinal: 1, Status: domainentity.RunAttemptStatusFailed,
		NextSequence: 5, LastCommittedSequence: 4, EnrollmentVersion: domainentity.JournalSchemaVersion,
		ProjectionState: domainentity.JournalProjectionStateHealthy,
	}
	ledger := journalRecoveryLedger(
		domainentity.SideEffectReplayPolicyIdempotentWrite,
		domainentity.SideEffectLedgerStatusSucceeded,
	)
	repo := &journalRecoveryRepositoryStub{
		attempts: []*domainentity.RunAttempt{attempt},
		ledgers:  map[string][]*domainentity.SideEffectLedger{"att_100": {ledger}},
	}
	threadSVC := &recordingThreadService{
		gotRun: rootRun,
		createdRunBundle: &domainservice.CreateRunBundleResult{
			Run: &domainentity.Run{ID: 20, ThreadID: 42},
			Attempt: &domainentity.RunAttempt{
				ID: 200, ThreadID: 42, JournalRunID: 10, ExecutionRunID: 20,
				AttemptID: "att_200", Ordinal: 2, Status: domainentity.RunAttemptStatusPending,
				EnrollmentVersion: domainentity.JournalSchemaVersion,
			},
			Created: true,
		},
	}
	threadSVC.checkpoints = []*domainentity.Checkpoint{journalRecoveryCheckpoint(t, repo.ledgers["att_100"])}
	app := &ApplicationService{
		ThreadSVC:                  threadSVC,
		ThreadAuthorizer:           &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:        &recordingWorkspaceAuthorizer{},
		JournalRecoveryRepository:  repo,
		JournalRecoveryIDGenerator: fixedIDGen{},
	}
	return app, threadSVC, repo
}

func journalRecoveryLedger(
	policy domainentity.SideEffectReplayPolicy,
	status domainentity.SideEffectLedgerStatus,
) *domainentity.SideEffectLedger {
	return &domainentity.SideEffectLedger{
		ID: 500, ThreadID: 42, JournalRunID: 10, AttemptID: "att_100",
		IdempotencyKey: "tool-1", ActionKind: "write_file", ReplayPolicy: policy,
		Status: status, RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestSummary: `{}`, Version: 3,
	}
}

func journalRecoveryCheckpoint(
	t *testing.T,
	ledgers []*domainentity.SideEffectLedger,
) *domainentity.Checkpoint {
	t.Helper()
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 10, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	}, nil)
	require.NoError(t, err)
	state := tracker.Snapshot()
	refs := make([]ADKSideEffectLedgerReference, 0, len(ledgers))
	for _, ledger := range ledgers {
		refs = append(refs, ADKSideEffectLedgerReference{
			LedgerID: ledger.ID, IdempotencyKey: ledger.IdempotencyKey,
			ActionKind: ledger.ActionKind, ReplayPolicy: string(ledger.ReplayPolicy),
			Status: string(ledger.Status), Version: ledger.Version,
			ResolutionAction:         string(ledger.ResolutionAction),
			ResolutionIdempotencyKey: ledger.ResolutionIdempotencyKey,
		})
	}
	raw, err := (ADKCheckpointEnvelope{
		EnvelopeVersion: adkJournalCheckpointEnvelopeVersion,
		SchemaVersion:   adkJournalCheckpointSchemaVersion,
		Runtime:         string(RuntimeModeEinoADK), RuntimeVersion: adkCheckpointRuntimeVersion,
		RuntimeKey: "run-10", MessageType: adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		RuntimeState:    &ADKCheckpointRuntimeState{Checkpoint: []byte{1, 2, 3}},
		AttemptID:       "att_100", LastCommittedSequence: 4,
		SideEffectLedger: refs, ParityState: &state, CreatedAt: 1000,
	}).Marshal()
	require.NoError(t, err)
	return &domainentity.Checkpoint{
		ID: 700, ThreadID: 42, RunID: 10, CheckpointNS: adkCheckpointNamespace,
		RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "run-10",
		EnvelopeVersion: adkJournalCheckpointEnvelopeVersion,
		ChannelValues:   string(raw), ChannelVersions: `{}`, PendingSends: `[]`, Metadata: `{}`,
		CreatedAt: 1000,
	}
}
