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
	"io"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestADKSideEffectToolClassificationFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		policy     domainentity.SideEffectReplayPolicy
		registered bool
	}{
		{name: "read_file", policy: domainentity.SideEffectReplayPolicyReadOnly, registered: true},
		{name: "web_search", policy: domainentity.SideEffectReplayPolicyReadOnly, registered: true},
		{name: "web_fetch", policy: domainentity.SideEffectReplayPolicyReadOnly, registered: true},
		{name: "skill", policy: domainentity.SideEffectReplayPolicyReadOnly, registered: true},
		{name: "tool_search", policy: domainentity.SideEffectReplayPolicyReadOnly, registered: true},
		{name: "write_file", policy: domainentity.SideEffectReplayPolicyIdempotentWrite, registered: true},
		{name: "present_files", policy: domainentity.SideEffectReplayPolicyIdempotentWrite, registered: true},
		{name: "create_skill_package", policy: domainentity.SideEffectReplayPolicyNonReplayable, registered: true},
		{name: "mcp__repo__read_file", policy: domainentity.SideEffectReplayPolicyNonReplayable, registered: false},
		{name: "future_tool", policy: domainentity.SideEffectReplayPolicyNonReplayable, registered: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, registered := ClassifyADKSideEffectTool(test.name)
			require.Equal(t, test.policy, policy)
			require.Equal(t, test.registered, registered)
		})
	}
}

func TestADKSideEffectMiddlewareCommitsPreparedBeforeInvocation(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	coordinator := newTestADKSideEffectCoordinator(t, repo)
	middleware := NewADKSideEffectMiddleware(coordinator)

	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			repo.record("invoke")
			return `{"path":"report.md"}`, nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-write-1"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), `{"path":"report.md","content":"ok"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"path":"report.md"}`, result)
	require.Equal(t, []string{"prepared", "executing", "invoke"}, repo.callsSnapshot())
	require.Equal(t, domainentity.SideEffectLedgerStatusExecuting, repo.ledger.Status)

	state := newTestADKParityStateTracker(t).Snapshot()
	checkpoint, committed, err := coordinator.CommitCheckpoint(
		context.Background(),
		ADKSideEffectCheckpointInput{
			RuntimeKey: "run-1", RuntimeState: []byte{1, 2, 3},
			ParityState: &state, ParentCheckpointID: 700,
		},
	)
	require.NoError(t, err)
	require.True(t, committed)
	require.NotNil(t, checkpoint)
	require.Equal(t, []string{"prepared", "executing", "invoke", "commit"}, repo.callsSnapshot())
	require.Equal(t, domainentity.SideEffectLedgerStatusSucceeded, repo.ledger.Status)
	require.Equal(t, uint64(3), repo.ledger.Version)

	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, adkJournalCheckpointEnvelopeVersion, envelope.EnvelopeVersion)
	require.Equal(t, "att_10", envelope.AttemptID)
	require.Equal(t, uint64(1), envelope.LastCommittedSequence)
	require.Len(t, envelope.SideEffectLedger, 1)
	require.Equal(t, "succeeded", envelope.SideEffectLedger[0].Status)
	require.Equal(t, uint64(3), envelope.SideEffectLedger[0].Version)
}

func TestADKSideEffectMiddlewareTimeoutBecomesUnknown(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	coordinator := newTestADKSideEffectCoordinator(t, repo)
	middleware := NewADKSideEffectMiddleware(coordinator)

	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			repo.record("invoke")
			return "", context.DeadlineExceeded
		},
		&adk.ToolContext{Name: "create_skill_package", CallID: "call-timeout"},
	)
	require.NoError(t, err)

	_, err = wrapped(context.Background(), `{}`)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, []string{"prepared", "executing", "invoke", "unknown"}, repo.callsSnapshot())
	require.Equal(t, domainentity.SideEffectLedgerStatusUnknown, repo.ledger.Status)
}

func TestADKSideEffectMiddlewareBlocksSameCallReplay(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	repo.replayed = &domainentity.SideEffectLedger{
		ID: 500, ThreadID: 1, JournalRunID: 10, AttemptID: "att_10",
		IdempotencyKey: adkSideEffectIdempotencyKey("write_file", "call-replay"),
		ActionKind:     "write_file", ReplayPolicy: domainentity.SideEffectReplayPolicyIdempotentWrite,
		Status: domainentity.SideEffectLedgerStatusSucceeded, RequestHash: adkSideEffectRequestHash(
			"write_file", `{"path":"report.md"}`,
		),
		RequestSummary: `{"argument_bytes":20,"classification":"registered"}`,
		Version:        3, PreparedAt: 100, CreatedAt: 100, UpdatedAt: 100,
	}
	coordinator := newTestADKSideEffectCoordinator(t, repo)
	middleware := NewADKSideEffectMiddleware(coordinator)
	invoked := false
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			invoked = true
			return "duplicate", nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-replay"},
	)
	require.NoError(t, err)

	_, err = wrapped(context.Background(), `{"path":"report.md"}`)
	require.ErrorIs(t, err, ErrADKSideEffectReplayBlocked)
	require.False(t, invoked)
}

func TestADKSideEffectRecoverySkipDoesNotReplayExternalCall(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	sourceAttemptID := "att_source"
	repo.attempt.SourceAttemptID = &sourceAttemptID
	arguments := `{"path":"report.md","content":"ok"}`
	sourceLedger := &domainentity.SideEffectLedger{
		ID: 900, ThreadID: 1, JournalRunID: 10, AttemptID: sourceAttemptID,
		IdempotencyKey: adkSideEffectIdempotencyKey("write_file", "call-recovery"),
		ActionKind:     "write_file", ReplayPolicy: domainentity.SideEffectReplayPolicyIdempotentWrite,
		Status:                   domainentity.SideEffectLedgerStatusUnknown,
		RequestHash:              adkSideEffectRequestHash("write_file", arguments),
		ResolutionAction:         domainentity.SideEffectResolutionActionSkip,
		ResolutionIdempotencyKey: "recovery-key:ledger:900",
		Version:                  4,
	}
	repo.sourceLedgers = map[int64]*domainentity.SideEffectLedger{sourceLedger.ID: sourceLedger}
	command := `{"resume":{"journal":{"journal_run_id":10,"source_attempt_id":"att_source","action":"skip","ledger_resolutions":[{"ledger_id":900,"source_idempotency_key":"` + sourceLedger.IdempotencyKey + `","action_kind":"write_file","request_hash":"` + sourceLedger.RequestHash + `","action":"skip","resolution_idempotency_key":"recovery-key:ledger:900"}]}}}`
	metadata := `{"journal_recovery":{"schema":"coze.journal.recovery.v1","journal_run_id":10,"source_attempt_id":"att_source","action":"skip"}}`
	coordinator, err := NewADKSideEffectBoundaryCoordinator(
		&RunSummary{RunID: 1, ThreadID: 1, SpaceID: 1, Command: command, Metadata: metadata},
		repo,
		&sequentialADKSideEffectIDGenerator{next: 1000},
		WithADKSideEffectClock(func() int64 { return 100 }),
	)
	require.NoError(t, err)
	middleware := NewADKSideEffectMiddleware(coordinator)
	invoked := false
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			invoked = true
			return "duplicate", nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-recovery"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), arguments)
	require.NoError(t, err)
	require.False(t, invoked)
	require.JSONEq(t, `{"status":"skipped","external_call_executed":false}`, result)
	require.Equal(t, []string{"prepared", "executing"}, repo.callsSnapshot())
	state := newTestADKParityStateTracker(t).Snapshot()
	_, committed, err := coordinator.CommitCheckpoint(
		context.Background(),
		ADKSideEffectCheckpointInput{
			RuntimeKey: "recovery-run", RuntimeState: []byte{1, 2, 3},
			ParityState: &state,
		},
	)
	require.NoError(t, err)
	require.True(t, committed)
	require.Equal(t, domainentity.SideEffectLedgerStatusSucceeded, repo.ledger.Status)
}

func TestADKSideEffectRecoveryHonorsConfirmedAction(t *testing.T) {
	tests := []struct {
		name          string
		action        domainentity.SideEffectResolutionAction
		expectInvoke  bool
		expectedValue string
	}{
		{
			name:          "mark succeeded bypasses external call",
			action:        domainentity.SideEffectResolutionActionMarkSucceeded,
			expectedValue: `{"status":"confirmed_succeeded","external_call_executed":false}`,
		},
		{
			name:         "retry explicitly invokes external call",
			action:       domainentity.SideEffectResolutionActionRetry,
			expectInvoke: true, expectedValue: `{"status":"external"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRecordingADKSideEffectRepository()
			sourceAttemptID := "att_source"
			repo.attempt.SourceAttemptID = &sourceAttemptID
			arguments := `{"path":"report.md","content":"ok"}`
			sourceLedger := &domainentity.SideEffectLedger{
				ID: 902, ThreadID: 1, JournalRunID: 10, AttemptID: sourceAttemptID,
				IdempotencyKey: adkSideEffectIdempotencyKey("write_file", "call-recovery-action"),
				ActionKind:     "write_file", ReplayPolicy: domainentity.SideEffectReplayPolicyIdempotentWrite,
				Status:                   domainentity.SideEffectLedgerStatusUnknown,
				RequestHash:              adkSideEffectRequestHash("write_file", arguments),
				ResolutionAction:         test.action,
				ResolutionIdempotencyKey: "recovery-key:ledger:902",
				Version:                  4,
			}
			repo.sourceLedgers = map[int64]*domainentity.SideEffectLedger{sourceLedger.ID: sourceLedger}
			command := `{"resume":{"journal":{"journal_run_id":10,"source_attempt_id":"att_source","action":"` + string(test.action) + `","ledger_resolutions":[{"ledger_id":902,"source_idempotency_key":"` + sourceLedger.IdempotencyKey + `","action_kind":"write_file","request_hash":"` + sourceLedger.RequestHash + `","action":"` + string(test.action) + `","resolution_idempotency_key":"recovery-key:ledger:902"}]}}}`
			metadata := `{"journal_recovery":{"schema":"coze.journal.recovery.v1","journal_run_id":10,"source_attempt_id":"att_source","action":"` + string(test.action) + `"}}`
			coordinator, err := NewADKSideEffectBoundaryCoordinator(
				&RunSummary{RunID: 1, ThreadID: 1, SpaceID: 1, Command: command, Metadata: metadata},
				repo,
				&sequentialADKSideEffectIDGenerator{next: 1000},
				WithADKSideEffectClock(func() int64 { return 100 }),
			)
			require.NoError(t, err)
			middleware := NewADKSideEffectMiddleware(coordinator)
			invoked := false
			wrapped, err := middleware.WrapInvokableToolCall(
				context.Background(),
				func(context.Context, string, ...tool.Option) (string, error) {
					invoked = true
					return `{"status":"external"}`, nil
				},
				&adk.ToolContext{Name: "write_file", CallID: "call-recovery-action"},
			)
			require.NoError(t, err)

			result, err := wrapped(context.Background(), arguments)
			require.NoError(t, err)
			require.Equal(t, test.expectInvoke, invoked)
			require.JSONEq(t, test.expectedValue, result)
		})
	}
}

func TestADKSideEffectRecoveryRejectsUnverifiedResolution(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	sourceAttemptID := "att_source"
	repo.attempt.SourceAttemptID = &sourceAttemptID
	arguments := `{"path":"report.md"}`
	sourceKey := adkSideEffectIdempotencyKey("write_file", "call-recovery")
	requestHash := adkSideEffectRequestHash("write_file", arguments)
	command := `{"resume":{"journal":{"journal_run_id":10,"source_attempt_id":"att_source","action":"mark_succeeded","ledger_resolutions":[{"ledger_id":901,"source_idempotency_key":"` + sourceKey + `","action_kind":"write_file","request_hash":"` + requestHash + `","action":"mark_succeeded","resolution_idempotency_key":"forged"}]}}}`
	metadata := `{"journal_recovery":{"schema":"coze.journal.recovery.v1","journal_run_id":10,"source_attempt_id":"att_source","action":"mark_succeeded"}}`
	coordinator, err := NewADKSideEffectBoundaryCoordinator(
		&RunSummary{RunID: 1, ThreadID: 1, SpaceID: 1, Command: command, Metadata: metadata},
		repo,
		&sequentialADKSideEffectIDGenerator{next: 1000},
		WithADKSideEffectClock(func() int64 { return 100 }),
	)
	require.NoError(t, err)
	middleware := NewADKSideEffectMiddleware(coordinator)
	invoked := false
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			invoked = true
			return "duplicate", nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-recovery"},
	)
	require.NoError(t, err)

	_, err = wrapped(context.Background(), arguments)
	require.ErrorIs(t, err, ErrADKSideEffectReplayBlocked)
	require.False(t, invoked)
}

func TestADKSideEffectRecoveryCommandFailsClosedWithoutServerMetadata(t *testing.T) {
	_, err := NewADKSideEffectBoundaryCoordinator(
		&RunSummary{
			RunID: 1, ThreadID: 1, SpaceID: 1,
			Command: `{"resume":{"journal":{"journal_run_id":10,"source_attempt_id":"att_source","action":"skip","ledger_resolutions":[]}}}`,
		},
		newRecordingADKSideEffectRepository(),
		&sequentialADKSideEffectIDGenerator{next: 1000},
	)
	require.ErrorContains(t, err, "recovery metadata is missing")
}

func TestADKSideEffectMiddlewareCompletesStreamAtEOF(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	coordinator := newTestADKSideEffectCoordinator(t, repo)
	middleware := NewADKSideEffectMiddleware(coordinator)

	wrapped, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			repo.record("invoke")
			return schema.StreamReaderFromArray([]string{"first", "second"}), nil
		},
		&adk.ToolContext{Name: "web_search", CallID: "call-stream"},
	)
	require.NoError(t, err)

	stream, err := wrapped(context.Background(), `{"query":"journal"}`)
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer stream.Close()
	var chunks []string
	for {
		chunk, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		require.NoError(t, recvErr)
		chunks = append(chunks, chunk)
	}
	require.Equal(t, []string{"first", "second"}, chunks)
	require.Equal(t, domainentity.SideEffectLedgerStatusExecuting, repo.ledger.Status)

	state := newTestADKParityStateTracker(t).Snapshot()
	_, committed, err := coordinator.CommitCheckpoint(context.Background(), ADKSideEffectCheckpointInput{
		RuntimeKey: "run-1", RuntimeState: []byte{4, 5, 6}, ParityState: &state,
	})
	require.NoError(t, err)
	require.True(t, committed)
	require.Equal(t, domainentity.SideEffectLedgerStatusSucceeded, repo.ledger.Status)
}

func TestADKCheckpointStoreCommitsPendingSideEffectWithoutIndependentWrite(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	run := &RunSummary{RunID: 1, ThreadID: 1, SpaceID: 1, CreatorID: 9}
	service := &recordingADKCheckpointService{}
	store, err := NewADKCheckpointStore(
		service,
		run,
		WithADKSideEffectBoundary(repo, &sequentialADKSideEffectIDGenerator{next: 2000}),
	)
	require.NoError(t, err)
	tracker, err := NewADKParityStateTracker(run, nil)
	require.NoError(t, err)
	require.NoError(t, store.SetParityStateTracker(tracker, 0))
	coordinator := store.SideEffectBoundaryCoordinator()
	require.NotNil(t, coordinator)
	middleware := NewADKSideEffectMiddleware(coordinator)
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			return `{"path":"report.md"}`, nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-store"},
	)
	require.NoError(t, err)
	_, err = wrapped(context.Background(), `{"path":"report.md"}`)
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), "run-1", []byte{7, 8, 9}))
	require.Nil(t, service.created)
	require.Equal(t, domainentity.SideEffectLedgerStatusSucceeded, repo.ledger.Status)
}

func TestADKMiddlewareBuilderUsesRunSideEffectCoordinator(t *testing.T) {
	repo := newRecordingADKSideEffectRepository()
	coordinator := newTestADKSideEffectCoordinator(t, repo)
	ctx := withADKSideEffectBoundaryCoordinator(context.Background(), coordinator)
	builder := defaultADKMiddlewareBuilder(
		ADKMiddlewareSideEffect,
		ADKMiddlewareAssemblerOptions{},
	)
	handler, err := builder(ctx, ADKMiddlewareBuildInput{
		Run:               &RunSummary{RunID: 1, ThreadID: 1},
		SubagentToolNames: []string{"researcher"},
	})
	require.NoError(t, err)
	middleware, ok := handler.(*ADKSideEffectMiddleware)
	require.True(t, ok)
	policy, registered := middleware.classify("researcher")
	require.True(t, registered)
	require.Equal(t, domainentity.SideEffectReplayPolicyNonReplayable, policy)
}

func newTestADKSideEffectCoordinator(
	t *testing.T,
	repo *recordingADKSideEffectRepository,
) *ADKSideEffectBoundaryCoordinator {
	t.Helper()
	coordinator, err := NewADKSideEffectBoundaryCoordinator(
		&RunSummary{RunID: 1, ThreadID: 1, SpaceID: 1},
		repo,
		&sequentialADKSideEffectIDGenerator{next: 1000},
		WithADKSideEffectClock(func() int64 { return 100 }),
	)
	require.NoError(t, err)
	return coordinator
}

type sequentialADKSideEffectIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *sequentialADKSideEffectIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

type recordingADKSideEffectRepository struct {
	mu            sync.Mutex
	calls         []string
	attempt       *domainentity.RunAttempt
	ledger        *domainentity.SideEffectLedger
	replayed      *domainentity.SideEffectLedger
	sourceLedgers map[int64]*domainentity.SideEffectLedger
}

func newRecordingADKSideEffectRepository() *recordingADKSideEffectRepository {
	active := uint8(1)
	return &recordingADKSideEffectRepository{attempt: &domainentity.RunAttempt{
		ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 1,
		AttemptID: "att_10", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
		ActiveSlot: &active, NextSequence: 1, EnrollmentVersion: domainentity.JournalSchemaVersion,
		ProjectionState: domainentity.JournalProjectionStateHealthy,
	}}
}

func (r *recordingADKSideEffectRepository) record(value string) {
	r.mu.Lock()
	r.calls = append(r.calls, value)
	r.mu.Unlock()
}

func (r *recordingADKSideEffectRepository) callsSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recordingADKSideEffectRepository) GetActiveJournalAttempt(
	context.Context,
	int64,
) (*domainentity.RunAttempt, error) {
	if r.attempt == nil {
		return nil, domainrepo.ErrJournalNotEnrolled
	}
	copy := *r.attempt
	return &copy, nil
}

func (r *recordingADKSideEffectRepository) GetSideEffectLedger(
	_ context.Context,
	journalRunID int64,
	attemptID string,
	idempotencyKey string,
) (*domainentity.SideEffectLedger, error) {
	for _, ledger := range r.sourceLedgers {
		if ledger != nil && ledger.JournalRunID == journalRunID &&
			ledger.AttemptID == attemptID && ledger.IdempotencyKey == idempotencyKey {
			copy := *ledger
			return &copy, nil
		}
	}
	return nil, domainrepo.ErrSideEffectLedgerNotFound
}

func (r *recordingADKSideEffectRepository) PrepareSideEffect(
	_ context.Context,
	req domainrepo.PrepareSideEffectRequest,
) (*domainentity.SideEffectLedger, bool, error) {
	r.record("prepared")
	if r.replayed != nil {
		copy := *r.replayed
		return &copy, false, nil
	}
	copy := *req.Ledger
	r.ledger = &copy
	return &copy, true, nil
}

func (r *recordingADKSideEffectRepository) TransitionSideEffect(
	_ context.Context,
	req domainrepo.TransitionSideEffectRequest,
) (*domainentity.SideEffectLedger, bool, error) {
	if r.ledger == nil {
		return nil, false, domainrepo.ErrSideEffectLedgerNotFound
	}
	r.record(string(req.ToStatus))
	r.ledger.Status = req.ToStatus
	r.ledger.Version++
	copy := *r.ledger
	return &copy, true, nil
}

func (r *recordingADKSideEffectRepository) CommitExecutionBoundary(
	_ context.Context,
	req domainrepo.CommitExecutionBoundaryRequest,
) (*domainrepo.CommitExecutionBoundaryResult, error) {
	if r.ledger == nil {
		return nil, domainrepo.ErrSideEffectLedgerNotFound
	}
	r.record("commit")
	predicted := *r.ledger
	predicted.Status = req.Status
	predicted.Version++
	eventID := req.ResultEvent.ID
	predicted.ResultEventID = &eventID
	predicted.ResultSnapshotID = req.ResultSnapshotID
	checkpoint, err := req.CheckpointFactory(1, []*domainentity.SideEffectLedger{&predicted})
	if err != nil {
		return nil, err
	}
	r.ledger = &predicted
	event := *req.ResultEvent
	event.Sequence = 1
	return &domainrepo.CommitExecutionBoundaryResult{
		Ledger: &predicted, Event: &event, Checkpoint: checkpoint,
		LastCommittedSequence: 1,
	}, nil
}
