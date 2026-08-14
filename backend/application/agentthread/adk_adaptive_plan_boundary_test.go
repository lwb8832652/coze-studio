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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestADKAdaptivePlanBoundaryCommitsPlanAndRealCheckpointOnce(t *testing.T) {
	ctx := context.Background()
	planScopeRunID := int64(20)
	run := &RunSummary{
		RunID: 20, PlanScopeRunID: planScopeRunID, ThreadID: 10,
		SpaceID: 7, CreatorID: 9, LeaseOwner: "worker-1", LeaseToken: "lease-1",
		ExecutionGeneration: 3,
	}
	ctx = withAdaptiveBootstrapFacts(ctx, &AdaptiveBootstrapFacts{
		Admission: domainentity.AdaptiveAdmissionSnapshot{
			Schema:       domainentity.AdaptiveAdmissionSchemaV1,
			Capabilities: domainentity.AdaptiveCapabilities{PlanAllowed: true},
		},
		Decision: domainentity.ExecutionDecision{
			Schema:     domainentity.ExecutionDecisionSchemaV1,
			DecisionID: "decision-1", DecisionRevision: 1,
			ExecutionRunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: "att-1", ExecutionGeneration: run.ExecutionGeneration,
			PlanScopeRunID: &planScopeRunID,
			Decision:       domainentity.ExecutionDecisionExecute,
			ExecutionShape: domainentity.ExecutionShapeMultiStep,
		},
	})
	planStore := newMemoryADKPlanStore()
	repository := &recordingADKAdaptivePlanBoundaryRepository{}
	checkpointService := &recordingADKCheckpointService{}
	reader := &recordingADKJournalCheckpointStateReader{attempt: &domainentity.RunAttempt{
		ID: 100, ThreadID: run.ThreadID, JournalRunID: run.RunID,
		ExecutionRunID: run.RunID, AttemptID: "att-1", NextSequence: 8,
		LastCommittedSequence: 7,
	}}
	store, err := NewADKCheckpointStore(
		checkpointService,
		run,
		WithADKCheckpointClock(func() int64 { return 1234 }),
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(
			repository,
			&sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000},
			planStore,
		),
	)
	require.NoError(t, err)

	barrier := store.ADKInternalCheckpointBarrier()
	coordinator, ok := barrier.(*ADKAdaptivePlanBoundaryCoordinator)
	require.True(t, ok)
	require.NoError(t, coordinator.stageHighWatermark(
		ctx, "tool-call-1", "agent:lead;tool:plan", 0, 1,
	))
	require.NoError(t, coordinator.stageTask(
		ctx, "tool-call-1", "agent:lead;tool:plan", &ADKPlanTask{
			TaskID: 1, ID: "1", Subject: "Ship MVP", Description: "Finish the boundary",
			Status: "pending", Blocks: []string{}, BlockedBy: []string{}, Active: true,
		},
	))
	require.Equal(t, uint64(1), barrier.PendingGeneration())

	realCheckpoint := []byte{0x03, 0x01, 0x7f, 0x45, 0x49, 0x4e, 0x4f}
	require.NoError(t, store.Set(ctx, "checkpoint-20", realCheckpoint))

	require.Len(t, repository.requests, 1)
	request := repository.requests[0]
	require.Equal(t, "plan.task.created", request.Event.EventType)
	require.NotNil(t, request.PlanMutation)
	require.Equal(t, int64(0), request.PlanMutation.ExpectedRevision)
	require.Equal(t, int64(1), request.PlanMutation.NextRevision)
	require.Equal(t, int64(0), request.PlanMutation.ExpectedHighWatermark)
	require.Equal(t, int64(1), request.PlanMutation.NextHighWatermark)
	require.Len(t, request.PlanMutation.Items, 1)
	require.Equal(t, int64(0), request.PlanMutation.Items[0].ExpectedVersion)
	require.Equal(t, int64(1), request.PlanMutation.Items[0].NextItem.Version)
	require.NotZero(t, request.PlanMutation.Items[0].NextItem.ID)
	require.Equal(t, uint64(8), mustADKJournalEnvelope(t, request.Checkpoint.ChannelValues).LastCommittedSequence)
	require.Equal(t, realCheckpoint, mustADKJournalEnvelope(t, request.Checkpoint.ChannelValues).RuntimeState.Checkpoint)
	require.Nil(t, checkpointService.created)
	require.Equal(t, int64(0), planStore.highWatermark)
	require.Equal(t, int64(0), planStore.revision)
	require.Equal(t, barrier.PendingGeneration(), barrier.CommittedGeneration())
}

func TestADKAdaptivePlanBoundaryUsesLegacyCheckpointWhenNoPlanIsPending(t *testing.T) {
	ctx, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(
			repository, &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}, planStore,
		),
	)
	require.NoError(t, err)

	require.NoError(t, store.Set(ctx, "checkpoint-20", []byte{1, 2, 3}))
	require.Empty(t, repository.requests)
	require.NotNil(t, checkpointService.created)
}

func TestADKExecutorSharesAdaptivePlanBoundaryCoordinatorWithFactory(t *testing.T) {
	_, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(
			repository, &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}, planStore,
		),
	)
	require.NoError(t, err)
	factoryErr := errors.New("stop after adaptive plan coordinator inspection")
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			require.Same(t, store.AdaptivePlanBoundaryCoordinator(), ADKAdaptivePlanBoundaryCoordinatorFromContext(ctx))
			return nil, factoryErr
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) { return store, nil },
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		RunID: run.RunID, ThreadID: run.ThreadID,
		Input: `{"messages":[{"role":"user","content":"checkpoint"}]}`,
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, factoryErr)
}

func TestADKAdaptivePlanBackendStagesTaskCreateWritesInOneGeneration(t *testing.T) {
	ctx, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(
			repository, &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}, planStore,
		),
	)
	require.NoError(t, err)
	ctx = withADKInternalCheckpointBarrier(ctx, store.ADKInternalCheckpointBarrier())
	backend, err := NewADKPlanBackend(run, planStore, RunEventSinkFunc(func(context.Context, RunEvent) error {
		t.Fatal("adaptive plan mutation must not use the legacy event sink")
		return nil
	}), WithADKAdaptivePlanBoundaryCoordinator(store.AdaptivePlanBoundaryCoordinator()))
	require.NoError(t, err)
	tracker, _, err := store.ParityStateTracker(ctx)
	require.NoError(t, err)
	require.NoError(t, backend.setParityStateTracker(tracker))

	middleware, err := plantaskMiddlewareForTest(ctx, backend)
	require.NoError(t, err)
	toolCall := findADKInvokableTool(t, ctx, middleware, "TaskCreate")
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: []tool.BaseTool{toolCall}})
	require.NoError(t, err)
	output, err := toolsNode.Invoke(ctx, &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "tool-call-1", Type: "function",
			Function: schema.FunctionCall{Name: "TaskCreate", Arguments: `{"subject":"Ship MVP","description":"Finish boundary"}`},
		}},
	})
	require.NoError(t, err)
	require.Len(t, output, 1)
	require.Equal(t, uint64(1), store.ADKInternalCheckpointBarrier().PendingGeneration())
	require.Equal(t, int64(0), planStore.highWatermark)
	require.Equal(t, int64(0), planStore.revision)

	snapshot, err := backend.LsInfo(ctx, &plantask.LsInfoRequest{Path: adkPlanBaseDir})
	require.NoError(t, err)
	require.Len(t, snapshot, 2)
	require.NoError(t, store.Set(ctx, "checkpoint-20", []byte{1, 2, 3}))
	require.Len(t, repository.requests, 1)
	require.Len(t, repository.requests[0].PlanMutation.Items, 1)
	envelope := mustADKJournalEnvelope(t, repository.requests[0].Checkpoint.ChannelValues)
	require.Len(t, envelope.ParityState.Todos, 1)
	require.Equal(t, "1", envelope.ParityState.Todos[0].ID)
	require.Equal(t, "Ship MVP", envelope.ParityState.Todos[0].Title)
	require.Nil(t, checkpointService.created)
}

func TestADKAdaptivePlanBoundaryResumeCommitsInheritedMultiHopPlanScope(t *testing.T) {
	ctx := context.Background()
	inheritedScopeRunID := int64(19)
	run := &RunSummary{
		RunID: 21, PlanScopeRunID: inheritedScopeRunID, ThreadID: 10,
		SpaceID: 7, CreatorID: 9, LeaseOwner: "worker-2", LeaseToken: "lease-2",
		ExecutionGeneration: 4,
	}
	ctx = withAdaptiveBootstrapFacts(ctx, &AdaptiveBootstrapFacts{
		Admission: baselineAdaptiveAdmission(),
		Decision: domainentity.ExecutionDecision{
			Schema:     domainentity.ExecutionDecisionSchemaV1,
			DecisionID: "decision-resume-21", DecisionRevision: 1,
			ExecutionRunID: run.RunID, JournalRunID: 30,
			AttemptID: "attempt-2", ExecutionGeneration: run.ExecutionGeneration,
			PlanScopeRunID: &inheritedScopeRunID,
			Decision:       domainentity.ExecutionDecisionExecute,
			ExecutionShape: domainentity.ExecutionShapeMultiStep,
		},
	})
	planStore := newMemoryADKPlanStore()
	repository := &recordingADKAdaptivePlanBoundaryRepository{}
	checkpointService := &recordingADKCheckpointService{}
	reader := &recordingADKJournalCheckpointStateReader{attempt: &domainentity.RunAttempt{
		ID: 100, ThreadID: run.ThreadID, JournalRunID: 30,
		ExecutionRunID: run.RunID, AttemptID: "attempt-2", NextSequence: 8,
		LastCommittedSequence: 7,
	}}
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(
			repository, &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}, planStore,
		),
	)
	require.NoError(t, err)
	ctx = withADKInternalCheckpointBarrier(ctx, store.ADKInternalCheckpointBarrier())
	legacyEventCalls := 0
	backend, err := NewADKPlanBackend(
		run, planStore,
		RunEventSinkFunc(func(context.Context, RunEvent) error {
			legacyEventCalls++
			return nil
		}),
		WithADKAdaptivePlanBoundaryCoordinator(store.AdaptivePlanBoundaryCoordinator()),
	)
	require.NoError(t, err)
	tracker, _, err := store.ParityStateTracker(ctx)
	require.NoError(t, err)
	require.NoError(t, backend.setParityStateTracker(tracker))

	middleware, err := plantaskMiddlewareForTest(ctx, backend)
	require.NoError(t, err)
	toolCall := findADKInvokableTool(t, ctx, middleware, "TaskCreate")
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: []tool.BaseTool{toolCall}})
	require.NoError(t, err)
	output, err := toolsNode.Invoke(ctx, &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: "resume-plan-call-1", Type: "function",
			Function: schema.FunctionCall{
				Name:      "TaskCreate",
				Arguments: `{"subject":"Resume MVP","description":"Commit inherited plan"}`,
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, output, 1)
	require.Equal(t, uint64(1), store.ADKInternalCheckpointBarrier().PendingGeneration())

	realCheckpoint := []byte{0x03, 0x21, 0x45, 0x49, 0x4e, 0x4f}
	require.NoError(t, store.Set(ctx, "coze-run-20", realCheckpoint))

	require.Len(t, repository.requests, 1)
	request := repository.requests[0]
	require.Equal(t, run.RunID, request.ExecutionRunID)
	require.Equal(t, inheritedScopeRunID, request.PlanMutation.PlanScopeRunID)
	require.Equal(t, inheritedScopeRunID, request.PlanMutation.Items[0].NextItem.RunID)
	require.Equal(t, realCheckpoint, mustADKJournalEnvelope(t, request.Checkpoint.ChannelValues).RuntimeState.Checkpoint)
	require.Nil(t, checkpointService.created)
	require.Zero(t, legacyEventCalls)
	require.Equal(t, int64(0), planStore.highWatermark)
	require.Equal(t, int64(0), planStore.revision)
	require.Equal(t, store.ADKInternalCheckpointBarrier().PendingGeneration(), store.ADKInternalCheckpointBarrier().CommittedGeneration())
}

func TestADKAdaptivePlanBoundaryCommitErrorHasNoFallbackAndReusesFrozenRequest(t *testing.T) {
	ctx, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	repository.err = errors.New("commit unavailable")
	idGen := &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKCheckpointClock(func() int64 { return 1234 }),
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(
			repository, idGen, planStore,
		),
	)
	require.NoError(t, err)
	coordinator := store.ADKInternalCheckpointBarrier().(*ADKAdaptivePlanBoundaryCoordinator)
	require.NoError(t, coordinator.stageHighWatermark(ctx, "tool-1", "addr", 0, 1))
	require.NoError(t, coordinator.stageTask(ctx, "tool-1", "addr", &ADKPlanTask{
		TaskID: 1, ID: "1", Subject: "Ship", Status: "pending", Blocks: []string{}, BlockedBy: []string{}, Active: true,
	}))

	err = store.Set(ctx, "checkpoint-20", []byte{1, 2, 3})
	require.ErrorContains(t, err, "commit unavailable")
	require.Nil(t, checkpointService.created)
	require.Equal(t, uint64(0), coordinator.CommittedGeneration())
	require.Len(t, repository.requests, 1)
	first := repository.requests[0]

	err = store.Set(ctx, "checkpoint-20", []byte{1, 2, 3})
	require.ErrorContains(t, err, "commit unavailable")
	require.Len(t, repository.requests, 2)
	second := repository.requests[1]
	require.Equal(t, first.Now, second.Now)
	require.Equal(t, first.IdempotencyKey, second.IdempotencyKey)
	require.Equal(t, first.Event.ID, second.Event.ID)
	require.Equal(t, first.Checkpoint.ID, second.Checkpoint.ID)
	require.Equal(t, 3, idGen.calls)
}

func TestADKAdaptivePlanBoundaryReadsCommittedHeadBeforeAllocatingIDs(t *testing.T) {
	ctx, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	repository.readResult = &domainrepo.CommitAdaptiveExecutionBoundaryResult{
		Event: &domainentity.RunEvent{
			ID: 900, ThreadID: run.ThreadID, RunID: run.RunID,
			EventType: "plan.task.created", CreatedAt: 1234,
		},
		Checkpoint: &domainentity.Checkpoint{
			ID: 901, ThreadID: run.ThreadID, RunID: run.RunID,
			RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "checkpoint-20",
		},
		Plan: &domainentity.AgentRunPlan{
			RunID: run.PlanScopeRunID, ThreadID: run.ThreadID,
			HighWatermark: 1, Revision: 1,
		},
		Items: []*domainentity.AgentRunPlanItem{{
			ID: 902, RunID: run.PlanScopeRunID, TaskID: 1,
			Subject: "Ship", Status: domainentity.AgentRunPlanItemStatusPending,
			Blocks: `[]`, BlockedBy: `[]`, Metadata: `{}`,
			Active: true, Version: 1,
		}},
		LastCommittedSequence: 8, Replayed: true,
	}
	idGen := &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}
	nowCalls := 0
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKCheckpointClock(func() int64 {
			nowCalls++
			return 1234
		}),
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(repository, idGen, planStore),
	)
	require.NoError(t, err)
	coordinator := store.AdaptivePlanBoundaryCoordinator()
	require.NoError(t, coordinator.stageHighWatermark(ctx, "tool-1", "addr", 0, 1))
	require.NoError(t, coordinator.stageTask(ctx, "tool-1", "addr", &ADKPlanTask{
		TaskID: 1, ID: "1", Subject: "Ship", Status: "pending",
		Blocks: []string{}, BlockedBy: []string{}, Active: true,
	}))
	require.Equal(t, 0, idGen.calls)
	require.Equal(t, 0, nowCalls)

	require.NoError(t, store.Set(ctx, "checkpoint-20", []byte{1, 2, 3}))
	require.Len(t, repository.readRequests, 1)
	require.Empty(t, repository.requests)
	require.Equal(t, 0, idGen.calls)
	require.Equal(t, 0, nowCalls)
	require.Nil(t, checkpointService.created)
	require.Equal(t, coordinator.PendingGeneration(), coordinator.CommittedGeneration())
	require.Equal(t, "checkpoint-20", repository.readRequests[0].RuntimeKey)
	require.Len(t, repository.readRequests[0].ExpectedMutationDigest, 64)
}

func TestADKAdaptivePlanBoundaryFrozenRetryRejectsCheckpointAuthorityDrift(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*ADKSideEffectCheckpointInput)
	}{
		{
			name: "runtime version",
			mutate: func(input *ADKSideEffectCheckpointInput) {
				input.RuntimeVersion = "different-runtime-version"
			},
		},
		{
			name: "run revision",
			mutate: func(input *ADKSideEffectCheckpointInput) {
				input.RunRevision++
			},
		},
		{
			name: "parity state",
			mutate: func(input *ADKSideEffectCheckpointInput) {
				input.ParityState = &ADKParityState{Todos: []ADKParityTodo{{ID: "drift"}}}
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, run, planStore, repository, _, _ := newADKAdaptivePlanBoundaryTestFixture(t)
			repository.err = errors.New("commit unavailable")
			coordinator, err := NewADKAdaptivePlanBoundaryCoordinator(
				run, repository, &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000},
				planStore, func() int64 { return 1234 },
			)
			require.NoError(t, err)
			require.NoError(t, coordinator.stageHighWatermark(ctx, "tool-1", "addr", 0, 1))
			require.NoError(t, coordinator.stageTask(ctx, "tool-1", "addr", &ADKPlanTask{
				TaskID: 1, ID: "1", Subject: "Ship", Status: "pending",
				Blocks: []string{}, BlockedBy: []string{}, Active: true,
			}))
			attempt := &domainentity.RunAttempt{
				ThreadID: run.ThreadID, JournalRunID: run.RunID, ExecutionRunID: run.RunID,
				AttemptID: "att-1", NextSequence: 8,
			}
			parityTracker, trackerErr := NewADKParityStateTracker(run, nil)
			require.NoError(t, trackerErr)
			parityState := parityTracker.Snapshot()
			parityState.Todos = []ADKParityTodo{{ID: "1", Title: "Ship", Status: "pending"}}
			input := ADKSideEffectCheckpointInput{
				RuntimeVersion: "eino-adk-test", RuntimeKey: "checkpoint-20",
				RuntimeState: []byte{1, 2, 3}, RunRevision: 7,
				ParityState: &parityState,
			}
			_, handled, err := coordinator.commitCheckpoint(ctx, input, attempt)
			require.True(t, handled)
			require.ErrorContains(t, err, "commit unavailable")
			require.Len(t, repository.requests, 1)

			testCase.mutate(&input)
			_, handled, err = coordinator.commitCheckpoint(ctx, input, attempt)
			require.True(t, handled)
			require.ErrorContains(t, err, "drift")
			require.Len(t, repository.readRequests, 1)
			require.Len(t, repository.requests, 1)
		})
	}
}

func TestADKAdaptivePlanBoundaryReadFailureDoesNotAllocateOrFallback(t *testing.T) {
	ctx, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	repository.readErr = errors.New("read unavailable")
	idGen := &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKJournalCheckpointStateReader(reader),
		WithADKAdaptivePlanBoundary(repository, idGen, planStore),
	)
	require.NoError(t, err)
	coordinator := store.AdaptivePlanBoundaryCoordinator()
	require.NoError(t, coordinator.stageHighWatermark(ctx, "tool-1", "addr", 0, 1))
	require.NoError(t, coordinator.stageTask(ctx, "tool-1", "addr", &ADKPlanTask{
		TaskID: 1, ID: "1", Subject: "Ship", Status: "pending",
		Blocks: []string{}, BlockedBy: []string{}, Active: true,
	}))

	err = store.Set(ctx, "checkpoint-20", []byte{1, 2, 3})
	require.ErrorContains(t, err, "read unavailable")
	require.Len(t, repository.readRequests, 1)
	require.Empty(t, repository.requests)
	require.Equal(t, 0, idGen.calls)
	require.Nil(t, checkpointService.created)
}

func TestADKAdaptivePlanBoundaryRejectsMixedSideEffectBeforeEitherCommit(t *testing.T) {
	ctx, run, planStore, repository, checkpointService, reader := newADKAdaptivePlanBoundaryTestFixture(t)
	sideEffectRepository := newRecordingADKSideEffectRepository()
	store, err := NewADKCheckpointStore(
		checkpointService, run,
		WithADKJournalCheckpointStateReader(reader),
		WithADKSideEffectBoundary(sideEffectRepository, &sequentialADKSideEffectIDGenerator{next: 2000}),
		WithADKAdaptivePlanBoundary(
			repository, &sequenceADKAdaptivePlanBoundaryIDGenerator{next: 1000}, planStore,
		),
	)
	require.NoError(t, err)
	coordinator := store.ADKInternalCheckpointBarrier().(*ADKAdaptivePlanBoundaryCoordinator)
	require.NoError(t, coordinator.stageHighWatermark(ctx, "tool-1", "addr", 0, 1))
	require.NoError(t, coordinator.stageTask(ctx, "tool-1", "addr", &ADKPlanTask{
		TaskID: 1, ID: "1", Subject: "Ship", Status: "pending", Blocks: []string{}, BlockedBy: []string{}, Active: true,
	}))
	sideEffect := store.SideEffectBoundaryCoordinator()
	require.NotNil(t, sideEffect)
	sideEffect.mu.Lock()
	sideEffect.pending = append(sideEffect.pending, &adkPendingSideEffect{})
	sideEffect.mu.Unlock()

	err = store.Set(ctx, "checkpoint-20", []byte{1, 2, 3})
	require.ErrorContains(t, err, "mixed")
	require.Empty(t, repository.requests)
	require.NotContains(t, sideEffectRepository.callsSnapshot(), "commit")
	require.Nil(t, checkpointService.created)
}

func newADKAdaptivePlanBoundaryTestFixture(t *testing.T) (
	context.Context,
	*RunSummary,
	*memoryADKPlanStore,
	*recordingADKAdaptivePlanBoundaryRepository,
	*recordingADKCheckpointService,
	*recordingADKJournalCheckpointStateReader,
) {
	t.Helper()
	planScopeRunID := int64(20)
	run := &RunSummary{
		RunID: 20, PlanScopeRunID: planScopeRunID, ThreadID: 10,
		SpaceID: 7, CreatorID: 9, LeaseOwner: "worker-1", LeaseToken: "lease-1",
		ExecutionGeneration: 3,
	}
	admission := baselineAdaptiveAdmission()
	ctx := withAdaptiveBootstrapFacts(context.Background(), &AdaptiveBootstrapFacts{
		Admission: admission,
		Decision: domainentity.ExecutionDecision{
			Schema:     domainentity.ExecutionDecisionSchemaV1,
			DecisionID: "decision-1", DecisionRevision: 1,
			ExecutionRunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: "att-1", ExecutionGeneration: run.ExecutionGeneration,
			PlanScopeRunID: &planScopeRunID,
			Decision:       domainentity.ExecutionDecisionExecute,
			ExecutionShape: domainentity.ExecutionShapeMultiStep,
			GoalSummary:    "Ship the MVP", Deliverables: []string{}, AcceptanceChecks: []domainentity.AdaptiveAcceptanceCheck{},
			SafeSummary: "Ship safely", CreatedAt: 100,
		},
	})
	return ctx, run, newMemoryADKPlanStore(), &recordingADKAdaptivePlanBoundaryRepository{},
		&recordingADKCheckpointService{}, &recordingADKJournalCheckpointStateReader{attempt: &domainentity.RunAttempt{
			ID: 100, ThreadID: run.ThreadID, JournalRunID: run.RunID,
			ExecutionRunID: run.RunID, AttemptID: "att-1", NextSequence: 8,
			LastCommittedSequence: 7,
		}}
}

func plantaskMiddlewareForTest(
	ctx context.Context,
	backend *ADKPlanBackend,
) (adk.ChatModelAgentMiddleware, error) {
	return plantask.New(ctx, &plantask.Config{Backend: backend, BaseDir: adkPlanBaseDir})
}

func findADKInvokableTool(
	t *testing.T,
	ctx context.Context,
	middleware adk.ChatModelAgentMiddleware,
	name string,
) tool.InvokableTool {
	t.Helper()
	_, runCtx, err := middleware.BeforeAgent(ctx, &adk.ChatModelAgentContext{})
	require.NoError(t, err)
	for _, candidate := range runCtx.Tools {
		info, infoErr := candidate.Info(ctx)
		require.NoError(t, infoErr)
		if info.Name == name {
			invokable, ok := candidate.(tool.InvokableTool)
			require.True(t, ok)
			return invokable
		}
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func mustADKJournalEnvelope(t *testing.T, raw string) ADKCheckpointEnvelope {
	t.Helper()
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(raw))
	require.NoError(t, err)
	return envelope
}

type recordingADKAdaptivePlanBoundaryRepository struct {
	readRequests []domainrepo.ReadAdaptiveExecutionBoundaryRequest
	readResult   *domainrepo.CommitAdaptiveExecutionBoundaryResult
	readErr      error
	requests     []domainrepo.CommitAdaptiveExecutionBoundaryRequest
	err          error
}

func (r *recordingADKAdaptivePlanBoundaryRepository) ReadAdaptiveExecutionBoundary(
	_ context.Context,
	req domainrepo.ReadAdaptiveExecutionBoundaryRequest,
) (*domainrepo.CommitAdaptiveExecutionBoundaryResult, error) {
	r.readRequests = append(r.readRequests, req)
	if r.readErr != nil {
		return nil, r.readErr
	}
	if r.readResult == nil {
		return nil, domainrepo.ErrAdaptiveExecutionBoundaryNotFound
	}
	return r.readResult, nil
}

func (r *recordingADKAdaptivePlanBoundaryRepository) CommitAdaptiveExecutionBoundary(
	_ context.Context,
	req domainrepo.CommitAdaptiveExecutionBoundaryRequest,
) (*domainrepo.CommitAdaptiveExecutionBoundaryResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	items := make([]*domainentity.AgentRunPlanItem, 0, len(req.PlanMutation.Items))
	for _, mutation := range req.PlanMutation.Items {
		item := *mutation.NextItem
		items = append(items, &item)
	}
	return &domainrepo.CommitAdaptiveExecutionBoundaryResult{
		Event: req.Event, Checkpoint: req.Checkpoint,
		Plan: &domainentity.AgentRunPlan{
			RunID:         req.PlanMutation.PlanScopeRunID,
			ThreadID:      req.ThreadID,
			HighWatermark: req.PlanMutation.NextHighWatermark,
			Revision:      req.PlanMutation.NextRevision,
		},
		Items: items, LastCommittedSequence: 8,
	}, nil
}

type sequenceADKAdaptivePlanBoundaryIDGenerator struct {
	next  int64
	calls int
}

func (g *sequenceADKAdaptivePlanBoundaryIDGenerator) GenID(context.Context) (int64, error) {
	g.calls++
	g.next++
	return g.next, nil
}
