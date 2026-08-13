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
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

type recordingAdaptiveBootstrapCoordinator struct {
	resumeFacts *AdaptiveBootstrapFacts
	resumeErr   error
	resumeCalls int
	order       *[]string
}

type testADKInternalCheckpointBarrier struct {
	mu        sync.Mutex
	pending   uint64
	committed uint64
}

func (b *testADKInternalCheckpointBarrier) requestPending() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending++
	return b.pending
}

func (b *testADKInternalCheckpointBarrier) commitPending() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.committed = b.pending
}

func (b *testADKInternalCheckpointBarrier) PendingGeneration() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pending
}

func (b *testADKInternalCheckpointBarrier) CommittedGeneration() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.committed
}

type barrierMemoryADKCheckpointStore struct {
	*memoryADKCheckpointStore
	barrier     *testADKInternalCheckpointBarrier
	commitOnSet bool
	setCalls    int
	checkpoints [][]byte
}

func (s *barrierMemoryADKCheckpointStore) ADKInternalCheckpointBarrier() adkInternalCheckpointBarrier {
	return s.barrier
}

func (s *barrierMemoryADKCheckpointStore) Set(
	ctx context.Context,
	checkpointID string,
	checkpoint []byte,
) error {
	s.setCalls++
	s.checkpoints = append(s.checkpoints, append([]byte(nil), checkpoint...))
	if err := s.memoryADKCheckpointStore.Set(ctx, checkpointID, checkpoint); err != nil {
		return err
	}
	if s.commitOnSet {
		s.barrier.commitPending()
	}
	return nil
}

type internalCheckpointBarrierChatModel struct {
	mu    sync.Mutex
	calls int
}

type resumeInternalCheckpointBarrierChatModel struct {
	mu    sync.Mutex
	calls int
}

func (m *resumeInternalCheckpointBarrierChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	switch m.calls {
	case 1:
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID:       "approval-call-1",
			Function: schema.FunctionCall{Name: "approval", Arguments: `{}`},
		}}), nil
	case 2:
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID:       "plan-call-1",
			Function: schema.FunctionCall{Name: "plan_like", Arguments: `{}`},
		}}), nil
	default:
		return schema.AssistantMessage("finished after resume checkpoint", nil), nil
	}
}

func (m *resumeInternalCheckpointBarrierChatModel) Stream(
	ctx context.Context,
	messages []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *resumeInternalCheckpointBarrierChatModel) WithTools(
	_ []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *resumeInternalCheckpointBarrierChatModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *internalCheckpointBarrierChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{{
			ID: "plan-call-1",
			Function: schema.FunctionCall{
				Name:      "plan_like",
				Arguments: `{}`,
			},
		}}), nil
	}
	return schema.AssistantMessage("finished after checkpoint", nil), nil
}

func (m *internalCheckpointBarrierChatModel) Stream(
	ctx context.Context,
	messages []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *internalCheckpointBarrierChatModel) WithTools(
	_ []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *internalCheckpointBarrierChatModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type internalCheckpointBarrierTool struct {
	barrier *testADKInternalCheckpointBarrier
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release <-chan struct{}
}

func (t *internalCheckpointBarrierTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "plan_like", Desc: "Stage a plan-like mutation."}, nil
}

func (t *internalCheckpointBarrierTool) InvokableRun(
	ctx context.Context,
	_ string,
	_ ...tool.Option,
) (string, error) {
	t.mu.Lock()
	t.calls++
	t.mu.Unlock()
	t.barrier.requestPending()
	if t.entered != nil {
		close(t.entered)
	}
	if t.release != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-t.release:
		}
	}
	return `{"ok":true}`, nil
}

func (t *internalCheckpointBarrierTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

func newInternalCheckpointBarrierAgent(
	t *testing.T,
	chatModel model.ToolCallingChatModel,
	tools ...tool.BaseTool,
) adk.ResumableAgent {
	t.Helper()
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "internal checkpoint barrier integration agent",
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: tools,
		}},
	})
	require.NoError(t, err)
	return agent
}

func TestADKExecutorSharesInternalCheckpointBarrierWithFactory(t *testing.T) {
	barrier := &testADKInternalCheckpointBarrier{}
	store := &barrierMemoryADKCheckpointStore{
		memoryADKCheckpointStore: newMemoryADKCheckpointStore(),
		barrier:                  barrier,
	}
	factoryErr := errors.New("stop after barrier inspection")
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			got, ok := adkInternalCheckpointBarrierFromContext(ctx)
			require.True(t, ok)
			require.Same(t, barrier, got)
			return nil, factoryErr
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) { return store, nil },
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"checkpoint"}]}`,
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, factoryErr)
}

func TestADKExecutorAutoResumesCommittedInternalCheckpointBarrier(t *testing.T) {
	barrier := &testADKInternalCheckpointBarrier{}
	store := &barrierMemoryADKCheckpointStore{
		memoryADKCheckpointStore: newMemoryADKCheckpointStore(),
		barrier:                  barrier,
		commitOnSet:              true,
	}
	chatModel := &internalCheckpointBarrierChatModel{}
	planTool := &internalCheckpointBarrierTool{barrier: barrier}
	agent := newInternalCheckpointBarrierAgent(t, chatModel, planTool)
	sink := &recordingRunEventSink{}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		sink,
		func(*RunSummary) (adk.CheckPointStore, error) { return store, nil },
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"update plan then finish"}]}`,
	})

	require.NoError(t, err)
	require.Equal(t, "finished after checkpoint", result.Message)
	require.Equal(t, 1, planTool.callCount())
	require.Equal(t, 2, chatModel.callCount())
	require.Equal(t, 1, store.setCalls)
	require.Len(t, store.checkpoints, 1)
	require.NotEmpty(t, store.checkpoints[0])
	require.Equal(t, uint64(1), barrier.CommittedGeneration())
	require.Equal(t, 1, countString(sink.eventTypes(), "tool.completed"))
	require.NotContains(t, sink.eventTypes(), "run.canceling")
}

func TestADKExecutorResumeAutoResumesCommittedInternalCheckpointBarrier(t *testing.T) {
	barrier := &testADKInternalCheckpointBarrier{}
	sourceStore := newMemoryADKCheckpointStore()
	targetStore := &barrierMemoryADKCheckpointStore{
		memoryADKCheckpointStore: newMemoryADKCheckpointStore(),
		barrier:                  barrier,
		commitOnSet:              true,
	}
	chatModel := &resumeInternalCheckpointBarrierChatModel{}
	planTool := &internalCheckpointBarrierTool{barrier: barrier}
	agent := newInternalCheckpointBarrierAgent(
		t,
		chatModel,
		&summarizationApprovalTool{},
		planTool,
	)
	sourceExecutor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) { return sourceStore, nil },
		nil,
	)
	sourceRun := &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"approve then update plan"}]}`,
	}

	result, err := sourceExecutor.Execute(context.Background(), sourceRun)

	require.Nil(t, result)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, err, &interrupted)
	require.Len(t, interrupted.Interrupts, 1)
	sink := &recordingRunEventSink{}
	targetExecutor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		sink,
		func(run *RunSummary) (adk.CheckPointStore, error) {
			if run.RunID == sourceRun.RunID {
				return sourceStore, nil
			}
			return targetStore, nil
		},
		nil,
	)
	targetRun := *sourceRun
	targetRun.RunID = 21
	targetID := interrupted.Interrupts[0].ID

	result, err = targetExecutor.Resume(context.Background(), &targetRun, &HarnessResumeInput{
		Runtime:          RuntimeModeEinoADK,
		RuntimeKey:       interrupted.CheckpointKey,
		SourceRunID:      sourceRun.RunID,
		ADKResumeTargets: map[string]any{targetID: "approved"},
		ADKCheckpoint: &ADKCheckpointEnvelope{
			RuntimeKey: interrupted.CheckpointKey,
		},
	})

	require.NoError(t, err)
	require.Equal(t, "finished after resume checkpoint", result.Message)
	require.Equal(t, 1, planTool.callCount())
	require.Equal(t, 3, chatModel.callCount())
	require.Equal(t, 1, targetStore.setCalls)
	require.Len(t, targetStore.checkpoints, 1)
	require.NotEmpty(t, targetStore.checkpoints[0])
	require.Equal(t, uint64(1), barrier.CommittedGeneration())
	require.Equal(t, 2, countString(sink.eventTypes(), "tool.completed"))
	require.NotContains(t, sink.eventTypes(), "run.canceling")
}

func TestADKExecutorDoesNotAutoResumeExternalCancellation(t *testing.T) {
	barrier := &testADKInternalCheckpointBarrier{}
	store := &barrierMemoryADKCheckpointStore{
		memoryADKCheckpointStore: newMemoryADKCheckpointStore(),
		barrier:                  barrier,
		commitOnSet:              true,
	}
	chatModel := &internalCheckpointBarrierChatModel{}
	entered := make(chan struct{})
	release := make(chan struct{})
	planTool := &internalCheckpointBarrierTool{
		barrier: barrier,
		entered: entered,
		release: release,
	}
	agent := newInternalCheckpointBarrierAgent(t, chatModel, planTool)
	registry := NewADKCancelRegistry()
	sink := &recordingRunEventSink{}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		sink,
		func(*RunSummary) (adk.CheckPointStore, error) { return store, nil },
		nil,
		WithADKCancelRegistry(registry),
	)
	type executionOutcome struct {
		result *RunExecutionResult
		err    error
	}
	outcome := make(chan executionOutcome, 1)
	go func() {
		result, err := executor.Execute(context.Background(), &RunSummary{
			ThreadID: 10,
			RunID:    20,
			Input:    `{"messages":[{"role":"user","content":"cancel after plan"}]}`,
		})
		outcome <- executionOutcome{result: result, err: err}
	}()
	<-entered

	require.NoError(t, registry.Cancel(context.Background(), 20, adk.CancelImmediate, false))
	got := <-outcome
	require.Nil(t, got.result)
	var canceled *RunCanceledError
	require.ErrorAs(t, got.err, &canceled)
	require.Equal(t, 1, planTool.callCount())
	require.Equal(t, 1, chatModel.callCount())
	require.Contains(t, sink.eventTypes(), "run.canceling")
}

func TestADKExecutorFailsClosedWhenInternalCheckpointBarrierIsNotCommitted(t *testing.T) {
	barrier := &testADKInternalCheckpointBarrier{}
	store := &barrierMemoryADKCheckpointStore{
		memoryADKCheckpointStore: newMemoryADKCheckpointStore(),
		barrier:                  barrier,
	}
	chatModel := &internalCheckpointBarrierChatModel{}
	planTool := &internalCheckpointBarrierTool{barrier: barrier}
	agent := newInternalCheckpointBarrierAgent(t, chatModel, planTool)
	sink := &recordingRunEventSink{}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		sink,
		func(*RunSummary) (adk.CheckPointStore, error) { return store, nil },
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"checkpoint must commit"}]}`,
	})

	require.Nil(t, result)
	require.ErrorContains(t, err, "internal checkpoint barrier generation 1 was not committed")
	require.Equal(t, 1, planTool.callCount())
	require.Equal(t, 1, chatModel.callCount())
	require.Equal(t, 1, store.setCalls)
	require.NotContains(t, sink.eventTypes(), "run.canceling")
}

func TestADKExecutorFailsClosedWhenPendingBarrierSegmentEndsWithoutCancellation(t *testing.T) {
	barrier := &testADKInternalCheckpointBarrier{}
	barrier.requestPending()
	store := &barrierMemoryADKCheckpointStore{
		memoryADKCheckpointStore: newMemoryADKCheckpointStore(),
		barrier:                  barrier,
	}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return &scriptedADKAgent{run: func(context.Context) []*adk.AgentEvent {
				return []*adk.AgentEvent{{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("must not succeed", nil),
						Role:    schema.Assistant,
					}},
				}}
			}}, nil
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) { return store, nil },
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"pending before terminal"}]}`,
	})

	require.Nil(t, result)
	require.ErrorContains(t, err, "internal checkpoint barrier generation 1 was not committed")
	require.Zero(t, store.setCalls)
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func (c *recordingAdaptiveBootstrapCoordinator) Bootstrap(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error) {
	return nil, nil
}

func (c *recordingAdaptiveBootstrapCoordinator) BootstrapResume(
	context.Context,
	*RunSummary,
	*HarnessResumeInput,
) (*AdaptiveBootstrapFacts, error) {
	c.resumeCalls++
	if c.order != nil {
		*c.order = append(*c.order, "resume-bootstrap")
	}
	return c.resumeFacts, c.resumeErr
}

func TestADKExecutorResumeBootstrapsBeforeBuildingRuntime(t *testing.T) {
	order := make([]string, 0, 3)
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	factoryErr := errors.New("stop after factory context inspection")
	gotFacts := false
	coordinator := &recordingAdaptiveBootstrapCoordinator{resumeFacts: facts, order: &order}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			order = append(order, "factory")
			got, ok := adaptiveBootstrapFactsFromContext(ctx)
			gotFacts = ok
			if ok {
				require.Equal(t, facts, got)
			}
			return nil, factoryErr
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			order = append(order, "store")
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKAdaptiveBootstrapCoordinator(coordinator),
	)
	input := &HarnessResumeInput{
		Runtime: RuntimeModeEinoADK, RuntimeKey: "coze-run-20", SourceRunID: run.RunID,
		ADKCheckpoint: &ADKCheckpointEnvelope{RuntimeKey: "coze-run-20"},
	}

	result, err := executor.Resume(context.Background(), run, input)

	require.Nil(t, result)
	require.ErrorIs(t, err, factoryErr)
	require.Equal(t, 1, coordinator.resumeCalls)
	require.True(t, gotFacts)
	require.Equal(t, []string{"resume-bootstrap", "store", "factory"}, order)
}

func TestADKExecutorResumeBuildsAgentAndCheckpointStoreWithDurablePlanScope(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.RunID = 22
	run.ExecutionGeneration = 5
	inheritedScope := int64(19)
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Decision.PlanScopeRunID = &inheritedScope
	coordinator := &recordingAdaptiveBootstrapCoordinator{resumeFacts: facts}
	factoryErr := errors.New("stop after resume runtime scope inspection")
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(_ context.Context, got *RunSummary) (adk.ResumableAgent, error) {
			require.Equal(t, inheritedScope, got.PlanScopeRunID)
			return nil, factoryErr
		}),
		&recordingRunEventSink{},
		func(got *RunSummary) (adk.CheckPointStore, error) {
			require.Equal(t, run.RunID, got.RunID)
			require.Equal(t, inheritedScope, got.PlanScopeRunID)
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKAdaptiveBootstrapCoordinator(coordinator),
	)
	input := &HarnessResumeInput{
		Runtime: RuntimeModeEinoADK, RuntimeKey: "coze-run-21", SourceRunID: 21,
		ADKCheckpoint: &ADKCheckpointEnvelope{RuntimeKey: "coze-run-21"},
	}

	result, err := executor.Resume(context.Background(), run, input)

	require.Nil(t, result)
	require.ErrorIs(t, err, factoryErr)
}

func TestADKExecutorResumeStopsBeforeBuildingRuntimeWhenBootstrapFails(t *testing.T) {
	bootstrapErr := errors.New("resume bootstrap rejected")
	factoryCalls, storeCalls := 0, 0
	coordinator := &recordingAdaptiveBootstrapCoordinator{resumeErr: bootstrapErr}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			factoryCalls++
			return nil, errors.New("factory must not run")
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			storeCalls++
			return nil, errors.New("store must not run")
		},
		nil,
		WithADKAdaptiveBootstrapCoordinator(coordinator),
	)
	run := freshAdaptiveBootstrapRunForTest()
	input := &HarnessResumeInput{
		Runtime: RuntimeModeEinoADK, RuntimeKey: "coze-run-20", SourceRunID: run.RunID,
		ADKCheckpoint: &ADKCheckpointEnvelope{RuntimeKey: "coze-run-20"},
	}

	result, err := executor.Resume(context.Background(), run, input)

	require.Nil(t, result)
	require.ErrorIs(t, err, bootstrapErr)
	require.Equal(t, 1, coordinator.resumeCalls)
	require.Zero(t, factoryCalls)
	require.Zero(t, storeCalls)
}

func TestADKExecutorBootstrapsBeforeBuildingRuntime(t *testing.T) {
	order := make([]string, 0, 3)
	run := freshAdaptiveBootstrapRunForTest()
	run.Input = `{"messages":[{"role":"user","content":"research"}]}`
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			order = append(order, "factory")
			got, ok := adaptiveBootstrapFactsFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, facts, got)
			return &scriptedADKAgent{run: func(context.Context) []*adk.AgentEvent {
				return []*adk.AgentEvent{{AgentName: "lead", Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{Message: schema.AssistantMessage("done", nil), Role: schema.Assistant},
				}}}
			}}, nil
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			order = append(order, "store")
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorFunc(func(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error) {
			order = append(order, "bootstrap")
			return facts, nil
		})),
	)

	result, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "done", result.Message)
	require.Equal(t, []string{"bootstrap", "store", "factory"}, order)
}

func TestADKExecutorStopsBeforeBuildingRuntimeWhenBootstrapFails(t *testing.T) {
	bootstrapErr := errors.New("bootstrap rejected")
	factoryCalls, storeCalls := 0, 0
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			factoryCalls++
			return nil, errors.New("factory must not run")
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			storeCalls++
			return nil, errors.New("store must not run")
		},
		nil,
		WithADKAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorFunc(func(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error) {
			return nil, bootstrapErr
		})),
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10, RunID: 20,
		Input: `{"messages":[{"role":"user","content":"research"}]}`,
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, bootstrapErr)
	require.Zero(t, factoryCalls)
	require.Zero(t, storeCalls)
}

func TestADKExecutorPersistsEventsAndReturnsFinalAssistantMessage(t *testing.T) {
	agent := &scriptedADKAgent{
		run: func(context.Context) []*adk.AgentEvent {
			return []*adk.AgentEvent{
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{
						MessageOutput: &adk.MessageVariant{
							IsStreaming: true,
							MessageStream: schema.StreamReaderFromArray([]*schema.Message{
								{Role: schema.Assistant, Content: "draft "},
								{Role: schema.Assistant, Content: "answer"},
							}),
							Role: schema.Assistant,
						},
					},
				},
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{
						MessageOutput: &adk.MessageVariant{
							Message: &schema.Message{
								Role:       schema.Tool,
								Content:    `{"result":"ok"}`,
								ToolCallID: "call-1",
							},
							Role:     schema.Tool,
							ToolName: "search",
						},
					},
				},
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{
						MessageOutput: &adk.MessageVariant{
							Message: schema.AssistantMessage("final answer", nil),
							Role:    schema.Assistant,
						},
					},
				},
			}
		},
	}
	eventSink := &recordingRunEventSink{}
	store := newMemoryADKCheckpointStore()
	executor := NewADKExecutor(ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
		return agent, nil
	}), eventSink, func(*RunSummary) (adk.CheckPointStore, error) {
		return store, nil
	}, nil)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"research"}]}`,
	})

	require.NoError(t, err)
	require.Equal(t, "final answer", result.Message)
	require.JSONEq(t, `{"source":"eino_adk","checkpoint_key":"coze-run-20"}`, result.Metadata)
	require.Equal(t, []string{
		"message.completed",
		"tool.completed",
		"message.completed",
	}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"content":"draft answer"`)
	require.Contains(t, eventSink.events[1].Payload, `"tool_name":"search"`)
	require.Contains(t, eventSink.events[2].Payload, `"content":"final answer"`)
}

func TestADKExecutorAssociatesToolEventsWithActivePlanTask(t *testing.T) {
	var parityTracker *ADKParityStateTracker
	bindingReleasedBeforeNextEvent := false
	eventSink := &recordingRunEventSink{onEmit: func(event RunEvent) {
		if event.EventType == "agent.event" && parityTracker != nil {
			bindingReleasedBeforeNextEvent = len(
				journalToolPlanTasksForTest(parityTracker),
			) == 0
		}
	}}
	checkpointService := &recordingADKCheckpointService{}
	agent := &scriptedADKAgent{
		run: func(ctx context.Context) []*adk.AgentEvent {
			parityTracker = adkParityStateTrackerFromContext(ctx)
			require.NotNil(t, parityTracker)
			require.NoError(t, parityTracker.ReplaceTodos([]ADKParityTodo{{
				ID: "1", Title: "Write report", Status: "in_progress",
			}}))
			return []*adk.AgentEvent{
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("", []schema.ToolCall{{
							ID: "call-write",
							Function: schema.FunctionCall{
								Name:      adkWriteFileToolName,
								Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
							},
						}}),
						Role: schema.Assistant,
					}},
				},
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: &schema.Message{
							Role: schema.Tool, Content: `{"ok":true}`,
							ToolCallID: "call-write",
						},
						Role: schema.Tool, ToolName: adkWriteFileToolName,
					}},
				},
				{
					AgentName: "lead",
				},
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("report ready", nil),
						Role:    schema.Assistant,
					}},
				},
			}
		},
	}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		eventSink,
		func(run *RunSummary) (adk.CheckPointStore, error) {
			return NewADKCheckpointStore(checkpointService, run)
		},
		nil,
	)
	run := &RunSummary{
		RunID: 29, ThreadID: 10, SpaceID: 7, CreatorID: 9,
		Input: `{"messages":[{"role":"user","content":"write report"}]}`,
	}

	_, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Len(t, eventSink.events, 4)
	require.Contains(t, eventSink.events[0].Payload, `"plan_task_id":"1"`)
	require.Contains(t, eventSink.events[1].Payload, `"plan_task_id":"1"`)
	require.True(t, bindingReleasedBeforeNextEvent)
}

func TestADKExecutorRetainsToolPlanBindingWhenTerminalEventPersistenceFails(t *testing.T) {
	var parityTracker *ADKParityStateTracker
	eventSink := &recordingRunEventSink{emitErr: func(event RunEvent) error {
		if event.EventType == "tool.completed" {
			return fmt.Errorf("event persistence unavailable")
		}
		return nil
	}}
	agent := &scriptedADKAgent{
		run: func(ctx context.Context) []*adk.AgentEvent {
			parityTracker = adkParityStateTrackerFromContext(ctx)
			require.NotNil(t, parityTracker)
			require.NoError(t, parityTracker.ReplaceTodos([]ADKParityTodo{{
				ID: "1", Title: "Write report", Status: "in_progress",
			}}))
			return []*adk.AgentEvent{
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("", []schema.ToolCall{{
							ID: "call-write",
							Function: schema.FunctionCall{
								Name:      adkWriteFileToolName,
								Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
							},
						}}),
						Role: schema.Assistant,
					}},
				},
				{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: &schema.Message{
							Role: schema.Tool, Content: `{"ok":true}`,
							ToolCallID: "call-write",
						},
						Role: schema.Tool, ToolName: adkWriteFileToolName,
					}},
				},
			}
		},
	}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		eventSink,
		func(run *RunSummary) (adk.CheckPointStore, error) {
			return NewADKCheckpointStore(&recordingADKCheckpointService{}, run)
		},
		nil,
	)
	run := &RunSummary{
		RunID: 30, ThreadID: 10, SpaceID: 7, CreatorID: 9,
		Input: `{"messages":[{"role":"user","content":"write report"}]}`,
	}

	_, err := executor.Execute(context.Background(), run)

	require.ErrorContains(t, err, "event persistence unavailable")
	require.Equal(
		t,
		"1",
		boundADKJournalToolPlanTaskIDFromContext(
			withADKParityStateTracker(context.Background(), parityTracker),
			"call-write",
		),
	)
}

func TestADKExecutorSeedsAndReturnsDurableParityState(t *testing.T) {
	checkpointService := &recordingADKCheckpointService{}
	run := &RunSummary{
		RunID: 20, ThreadID: 10, SpaceID: 7, CreatorID: 9,
		Input: `{
			"messages":[
				{"role":"user","content":"first"},
				{"role":"assistant","content":"previous"},
				{"role":"user","content":"current"}
			],
			"uploaded_files":[{
				"file_id":30,
				"file_name":"brief.md",
				"virtual_path":"/mnt/user-data/uploads/brief.md",
				"content_type":"text/markdown"
			}]
		}`,
	}
	var factorySnapshot ADKParityState
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			tracker := adkParityStateTrackerFromContext(ctx)
			require.NotNil(t, tracker)
			factorySnapshot = tracker.Snapshot()
			return &scriptedADKAgent{run: func(context.Context) []*adk.AgentEvent {
				return []*adk.AgentEvent{{
					AgentName: "lead",
					Output: &adk.AgentOutput{MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("final answer", nil),
						Role:    schema.Assistant,
					}},
				}}
			}}, nil
		}),
		&recordingRunEventSink{},
		func(run *RunSummary) (adk.CheckPointStore, error) {
			return NewADKCheckpointStore(checkpointService, run)
		},
		nil,
	)

	result, err := executor.Execute(context.Background(), run)
	require.NoError(t, err)
	require.Len(t, factorySnapshot.Messages, 3)
	require.Equal(t, []string{"first", "previous", "current"}, parityMessageContents(factorySnapshot.Messages))
	require.Equal(t, []ADKParityUpload{{
		FileID: 30, FileName: "brief.md",
		VirtualPath: "/mnt/user-data/uploads/brief.md",
		ContentType: "text/markdown",
	}}, factorySnapshot.Uploads)
	require.NotNil(t, result.ParityState)
	require.Equal(t, []string{"first", "previous", "current", "final answer"}, parityMessageContents(result.ParityState.Messages))
	require.NotNil(t, result.ParityState.Completion)
	require.Equal(t, "succeeded", result.ParityState.Completion.Status)
	require.Empty(t, result.ParityState.Interrupts)
}

func parityMessageContents(messages []ADKParityMessage) []string {
	result := make([]string, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.Content)
	}
	return result
}

func TestADKExecutorPersistsSummarizationEventsWithBoundedMemory(t *testing.T) {
	chatModel := &summarizationIntegrationChatModel{}
	memoryProvider := &recordingMemoryProvider{memories: []AgentMemory{{
		ID:      "1",
		Scope:   "thread",
		Content: "deployment region is APAC",
	}}}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
			MemoryProvider: memoryProvider,
		}),
	)
	eventSink := &recordingRunEventSink{}
	executor := NewADKExecutor(
		factory,
		eventSink,
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Config: `{
			"agent_name":"lead",
			"context_budget":{
				"summarization_messages":2,
				"memory_tokens":100
			}
		}`,
		Input: `{"messages":[
			{"role":"user","content":"first"},
			{"role":"assistant","content":"second"},
			{"role":"user","content":"third"}
		]}`,
	})

	require.NoError(t, err)
	require.Equal(t, "final after summary", result.Message)
	require.Equal(t, 1, memoryProvider.calls)
	require.Equal(t, []string{
		"context.summarizing",
		"context.summary_model_call",
		"context.summarized",
		"message.completed",
	}, eventSink.eventTypes())
	require.Equal(t, []string{"summarization", ""}, chatModel.usageKindValues())
	require.Equal(t, 0, chatModel.inputMemoryCountAt(0, "deployment region is APAC"))
	require.Equal(t, 1, chatModel.finalInputMemoryCount("deployment region is APAC"))
	for _, event := range eventSink.events {
		require.NotContains(t, event.Payload, "first")
		require.NotContains(t, event.Payload, "second")
		require.NotContains(t, event.Payload, "third")
	}
}

func TestADKSummarizedCheckpointRestartsWithoutOriginalHistoryOrDuplicateMemory(t *testing.T) {
	checkpointService := newContractCheckpointService()
	approvalTool := &summarizationApprovalTool{}
	memoryProvider := &recordingMemoryProvider{memories: []AgentMemory{{
		ID:      "1",
		Scope:   "thread",
		Content: "deployment region is APAC",
	}}}
	newFactory := func(
		chatModel model.BaseChatModel,
		transcriptStore ADKTranscriptStore,
	) *ApplicationADKAgentFactory {
		return NewApplicationADKAgentFactory(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return chatModel, true, nil
			},
			ADKToolProviderFunc(func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
				return []tool.BaseTool{approvalTool}, nil
			}),
			NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
				MemoryProvider:  memoryProvider,
				TranscriptStore: transcriptStore,
			}),
		)
	}
	storeFactory := func(run *RunSummary) (adk.CheckPointStore, error) {
		return NewADKCheckpointStore(checkpointService, run)
	}
	longFirst := "first-" + strings.Repeat("a", 4000)
	longSecond := "second-" + strings.Repeat("b", 4000)
	longThird := "third-" + strings.Repeat("c", 4000)
	run := &RunSummary{
		ThreadID:  10,
		RunID:     20,
		SpaceID:   7,
		CreatorID: 9,
		Config: `{
			"agent_name":"lead",
			"context_budget":{
				"context_window_tokens":10000,
				"summarization_tokens":2000,
				"summarization_messages":200,
				"memory_tokens":32
			}
		}`,
		Input: fmt.Sprintf(`{"messages":[
			{"role":"user","content":%q},
			{"role":"assistant","content":%q},
			{"role":"user","content":%q}
		]}`, longFirst, longSecond, longThird),
	}

	initialModel := &summarizingInterruptChatModel{}
	initialSink := &recordingRunEventSink{}
	initialTranscriptStore := &recordingADKTranscriptStore{}
	initialExecutor := NewADKExecutor(
		newFactory(initialModel, initialTranscriptStore),
		initialSink,
		storeFactory,
		nil,
	)
	initialResult, initialErr := initialExecutor.Execute(context.Background(), run)

	require.Nil(t, initialResult)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, initialErr, &interrupted)
	require.NotEmpty(t, interrupted.Interrupts)
	require.Len(t, initialTranscriptStore.calls, 1)
	require.Equal(
		t,
		TranscriptKindSummaryInput,
		initialTranscriptStore.calls[0].Kind,
	)
	requireOrderedEventTypes(t, initialSink.eventTypes(), []string{
		"context.summarizing",
		"context.summary_model_call",
		"context.summarized",
		"run.interrupted",
	})
	persisted, err := checkpointService.GetLatestRuntimeCheckpoint(
		context.Background(),
		&GetLatestRuntimeCheckpointRequest{
			ThreadID:    run.ThreadID,
			RunID:       run.RunID,
			RuntimeType: string(RuntimeModeEinoADK),
			RuntimeKey:  interrupted.CheckpointKey,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, persisted.Checkpoint)
	envelope, err := UnmarshalADKCheckpointEnvelope(
		[]byte(persisted.Checkpoint.ChannelValues),
	)
	require.NoError(t, err)

	resumeModel := &summarizingInterruptChatModel{}
	resumeSink := &recordingRunEventSink{}
	resumeTranscriptStore := &recordingADKTranscriptStore{}
	resumeExecutor := NewADKExecutor(
		newFactory(resumeModel, resumeTranscriptStore),
		resumeSink,
		storeFactory,
		nil,
	)
	resumeRun := *run
	resumeRun.RunID = 21
	targetID := interrupted.Interrupts[0].ID
	result, err := resumeExecutor.Resume(
		context.Background(),
		&resumeRun,
		&HarnessResumeInput{
			Runtime:      RuntimeModeEinoADK,
			RuntimeKey:   interrupted.CheckpointKey,
			ThreadID:     run.ThreadID,
			RunID:        resumeRun.RunID,
			SourceRunID:  run.RunID,
			CheckpointNS: adkCheckpointNamespace,
			ResumeFrom:   "interrupt",
			ADKResumeTargets: map[string]any{
				targetID: "approved",
			},
			ADKCheckpoint: &envelope,
		},
	)

	require.NoError(t, err)
	require.Equal(t, "final after approval", result.Message)
	require.Len(t, resumeTranscriptStore.calls, 1)
	require.Equal(
		t,
		TranscriptKindTerminal,
		resumeTranscriptStore.calls[0].Kind,
	)
	require.NotContains(t, resumeModel.allInputText(), longFirst)
	require.NotContains(t, resumeModel.allInputText(), longSecond)
	require.NotContains(t, resumeModel.allInputText(), longThird)
	require.Equal(t, 1, strings.Count(
		resumeModel.allInputText(),
		"deployment region is APAC",
	))
	require.Equal(t, []string{""}, resumeModel.usageKindValues())
	require.Equal(t, []string{"tool.completed", "message.completed"}, resumeSink.eventTypes())
}

func TestADKMultimodalProjectionSurvivesInterruptAndFreshResume(t *testing.T) {
	checkpointService := newContractCheckpointService()
	approvalTool := &summarizationApprovalTool{}
	newFactory := func(
		chatModel model.BaseChatModel,
		transcriptStore ADKTranscriptStore,
		eventSink RunEventSink,
	) *ApplicationADKAgentFactory {
		return NewApplicationADKAgentFactory(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return chatModel, true, nil
			},
			ADKToolProviderFunc(func(
				context.Context,
				*RunSummary,
			) ([]tool.BaseTool, error) {
				return []tool.BaseTool{approvalTool}, nil
			}),
			NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
				TranscriptStore: transcriptStore,
				EventSink:       eventSink,
			}),
		)
	}
	storeFactory := func(run *RunSummary) (adk.CheckPointStore, error) {
		return NewADKCheckpointStore(checkpointService, run)
	}
	imageURL := "https://example.test/checkpoint-image.png"
	run := &RunSummary{
		ThreadID:  10,
		RunID:     20,
		SpaceID:   7,
		CreatorID: 9,
		Config: `{
			"agent_name":"lead",
			"provider_capabilities":{
				"vision":true
			},
			"context_budget":{
				"context_window_tokens":10000,
				"summarization_tokens":9000,
				"summarization_messages":100,
				"memory_tokens":100,
				"multimodal_history_tokens":1,
				"file_history_tokens":2048
			}
		}`,
		Input: fmt.Sprintf(`{"messages":[
			{
				"role":"user",
				"content":"",
				"user_input_multi_content":[{
					"type":"image_url",
					"image":{"url":%q,"detail":"low"}
				}]
			},
			{"role":"user","content":"approve analysis"}
		]}`, imageURL),
	}

	initialModel := &summarizingInterruptChatModel{}
	initialSink := &recordingRunEventSink{}
	initialTranscriptStore := &recordingADKTranscriptStore{}
	initialExecutor := NewADKExecutor(
		newFactory(initialModel, initialTranscriptStore, initialSink),
		initialSink,
		storeFactory,
		nil,
	)
	initialResult, initialErr := initialExecutor.Execute(
		context.Background(),
		run,
	)

	require.Nil(t, initialResult)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, initialErr, &interrupted)
	require.NotEmpty(t, interrupted.Interrupts)
	require.Empty(t, initialTranscriptStore.calls)
	require.Len(t, initialModel.inputs, 1)
	require.NotContains(
		t,
		multimodalBudgetMessagesJSON(t, initialModel.inputs[0]),
		imageURL,
	)

	persisted, err := checkpointService.GetLatestRuntimeCheckpoint(
		context.Background(),
		&GetLatestRuntimeCheckpointRequest{
			ThreadID:    run.ThreadID,
			RunID:       run.RunID,
			RuntimeType: string(RuntimeModeEinoADK),
			RuntimeKey:  interrupted.CheckpointKey,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, persisted.Checkpoint)
	envelope, err := UnmarshalADKCheckpointEnvelope(
		[]byte(persisted.Checkpoint.ChannelValues),
	)
	require.NoError(t, err)

	resumeModel := &summarizingInterruptChatModel{}
	resumeSink := &recordingRunEventSink{}
	resumeTranscriptStore := &recordingADKTranscriptStore{}
	resumeExecutor := NewADKExecutor(
		newFactory(resumeModel, resumeTranscriptStore, resumeSink),
		resumeSink,
		storeFactory,
		nil,
	)
	resumeRun := *run
	resumeRun.RunID = 21
	targetID := interrupted.Interrupts[0].ID
	result, err := resumeExecutor.Resume(
		context.Background(),
		&resumeRun,
		&HarnessResumeInput{
			Runtime:      RuntimeModeEinoADK,
			RuntimeKey:   interrupted.CheckpointKey,
			ThreadID:     run.ThreadID,
			RunID:        resumeRun.RunID,
			SourceRunID:  run.RunID,
			CheckpointNS: adkCheckpointNamespace,
			ResumeFrom:   "interrupt",
			ADKResumeTargets: map[string]any{
				targetID: "approved",
			},
			ADKCheckpoint: &envelope,
		},
	)

	require.NoError(t, err)
	require.Equal(t, "final after approval", result.Message)
	require.Len(t, resumeModel.inputs, 1)
	require.NotContains(
		t,
		multimodalBudgetMessagesJSON(t, resumeModel.inputs[0]),
		imageURL,
	)
	require.Len(t, resumeTranscriptStore.calls, 1)
	require.Equal(
		t,
		TranscriptKindTerminal,
		resumeTranscriptStore.calls[0].Kind,
	)
	require.Contains(t, resumeTranscriptStore.calls[0].Messages, imageURL)
}

func TestADKExecutorInterruptsAndResumesWithTargets(t *testing.T) {
	agent := &scriptedADKAgent{
		run: func(ctx context.Context) []*adk.AgentEvent {
			return []*adk.AgentEvent{
				adk.Interrupt(ctx, map[string]any{"question": "approve?"}),
			}
		},
		resume: func(ctx context.Context, info *adk.ResumeInfo) []*adk.AgentEvent {
			return []*adk.AgentEvent{{
				AgentName: "lead",
				Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("approved result", nil),
						Role:    schema.Assistant,
					},
				},
			}}
		},
	}
	eventSink := &recordingRunEventSink{}
	sourceStore := newMemoryADKCheckpointStore()
	currentStore := newMemoryADKCheckpointStore()
	storeRunIDs := make([]int64, 0, 3)
	agentRuns := make([]RunSummary, 0, 2)
	executor := NewADKExecutor(ADKAgentFactoryFunc(func(
		_ context.Context,
		run *RunSummary,
	) (adk.ResumableAgent, error) {
		agentRuns = append(agentRuns, *run)
		return agent, nil
	}), eventSink, func(run *RunSummary) (adk.CheckPointStore, error) {
		storeRunIDs = append(storeRunIDs, run.RunID)
		if run.RunID == 20 {
			return sourceStore, nil
		}
		if run.RunID == 21 {
			return currentStore, nil
		}
		return nil, fmt.Errorf("unexpected store run id: %d", run.RunID)
	}, nil)
	run := &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"requires approval"}]}`,
	}

	result, err := executor.Execute(context.Background(), run)

	require.Nil(t, result)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, err, &interrupted)
	require.Equal(t, "coze-run-20", interrupted.CheckpointKey)
	require.NotEmpty(t, interrupted.Interrupts)
	_, exists, storeErr := sourceStore.Get(context.Background(), interrupted.CheckpointKey)
	require.NoError(t, storeErr)
	require.True(t, exists)

	targetID := interrupted.Interrupts[0].ID
	envelope := &ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      interrupted.CheckpointKey,
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
		Interrupts: map[string]ADKInterruptItem{
			targetID: interrupted.Interrupts[0],
		},
	}
	resumeRun := *run
	resumeRun.RunID = 21
	result, err = executor.Resume(context.Background(), &resumeRun, &HarnessResumeInput{
		Runtime:          RuntimeModeEinoADK,
		RuntimeKey:       interrupted.CheckpointKey,
		ADKCheckpoint:    envelope,
		ADKResumeTargets: map[string]any{targetID: "approved"},
		ThreadID:         10,
		RunID:            21,
		SourceRunID:      20,
	})

	require.NoError(t, err)
	require.Equal(t, "approved result", result.Message)
	require.NotNil(t, agent.resumeInfo)
	require.True(t, agent.resumeInfo.WasInterrupted)
	require.True(t, agent.resumeInfo.IsResumeTarget)
	require.Equal(t, "approved", agent.resumeInfo.ResumeData)
	require.Equal(t, []int64{20, 21, 20}, storeRunIDs)
	require.Len(t, agentRuns, 2)
	require.Equal(t, int64(0), agentRuns[0].PlanScopeRunID)
	require.Equal(t, int64(21), agentRuns[1].RunID)
	require.Equal(t, int64(20), agentRuns[1].PlanScopeRunID)
	require.Equal(t, []string{
		"run.interrupted",
		"message.completed",
	}, eventSink.eventTypes())
}

func TestADKAgentRunForResumeSelectsPlanScope(t *testing.T) {
	run := &RunSummary{RunID: 21, ThreadID: 10}

	for _, testCase := range []struct {
		name          string
		sourceRunID   int64
		wantPlanScope int64
		wantError     string
	}{
		{name: "no source", sourceRunID: 0},
		{name: "same run", sourceRunID: 21, wantPlanScope: 21},
		{name: "source run", sourceRunID: 20, wantPlanScope: 20},
		{
			name:        "invalid source",
			sourceRunID: -1,
			wantError:   "source run id is invalid",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			agentRun, err := adkAgentRunForResume(run, testCase.sourceRunID)

			if testCase.wantError != "" {
				require.ErrorContains(t, err, testCase.wantError)
				require.Nil(t, agentRun)
				return
			}
			require.NoError(t, err)
			require.NotSame(t, run, agentRun)
			require.Equal(t, run.RunID, agentRun.RunID)
			require.Equal(t, testCase.wantPlanScope, agentRun.PlanScopeRunID)
		})
	}
}

func TestADKExecutorReturnsCanceledError(t *testing.T) {
	agent := &scriptedADKAgent{
		run: func(context.Context) []*adk.AgentEvent {
			return []*adk.AgentEvent{{
				AgentName: "lead",
				Err: &adk.CancelError{
					Info: &adk.AgentCancelInfo{Mode: adk.CancelAfterToolCalls},
				},
			}}
		},
	}
	eventSink := &recordingRunEventSink{}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		eventSink,
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Input:    `{"messages":[{"role":"user","content":"cancel"}]}`,
	})

	require.Nil(t, result)
	var canceled *RunCanceledError
	require.ErrorAs(t, err, &canceled)
	require.True(t, canceled.EventPersisted)
	require.Equal(t, []string{"run.canceling"}, eventSink.eventTypes())
}

func TestNormalizeADKExecutionErrorDistinguishesRunCancelFromWorkerShutdown(t *testing.T) {
	err := normalizeADKExecutionError(context.Background(), context.Canceled)
	var canceled *RunCanceledError
	require.ErrorAs(t, err, &canceled)
	require.True(t, canceled.EventPersisted)

	shutdownCtx, shutdown := context.WithCancel(context.Background())
	shutdown()
	require.ErrorIs(t, normalizeADKExecutionError(shutdownCtx, context.Canceled), context.Canceled)

	modelErr := fmt.Errorf("model failed")
	require.ErrorIs(t, normalizeADKExecutionError(context.Background(), modelErr), modelErr)
}

func TestADKExecutorRegistersActiveExecutionForCancel(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	agent := &scriptedADKAgent{
		run: func(context.Context) []*adk.AgentEvent {
			close(entered)
			<-release
			return []*adk.AgentEvent{{
				AgentName: "lead",
				Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("done", nil),
						Role:    schema.Assistant,
					},
				},
			}}
		},
	}
	registry := NewADKCancelRegistry()
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKCancelRegistry(registry),
	)
	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), &RunSummary{
			ThreadID: 10,
			RunID:    20,
			Input:    `{"messages":[{"role":"user","content":"wait"}]}`,
		})
		done <- err
	}()

	<-entered
	registry.mu.Lock()
	_, active := registry.handles[20]
	registry.mu.Unlock()
	require.True(t, active)

	close(release)
	require.NoError(t, <-done)
	registry.mu.Lock()
	_, active = registry.handles[20]
	registry.mu.Unlock()
	require.False(t, active)
}

func TestADKExecutorCollectsChatModelCallbackUsageOnce(t *testing.T) {
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "usage test agent",
		Model:       &usageChatModel{},
	})
	require.NoError(t, err)

	collector := &recordingADKUsageCollector{}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return agent, nil
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		collector,
	)

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Config:   `{"runtime":"eino_adk","agent_name":"lead"}`,
		Input:    `{"messages":[{"role":"user","content":"usage"}]}`,
	})

	require.NoError(t, err)
	require.Equal(t, "callback answer", result.Message)
	require.Len(t, collector.usages, 1)
	require.Equal(t, int64(12), collector.usages[0].InputTokens)
	require.Equal(t, int64(5), collector.usages[0].OutputTokens)
	require.Contains(t, collector.usages[0].Metadata, `"source":"eino_callback"`)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Metadata), &metadata))
	require.Regexp(t, `^[0-9a-f]{32}$`, metadata["trace_id"])
}

func TestADKExecutorReplaysSubagentRetryFromSourceChildRun(t *testing.T) {
	childAgent := &recordingSubagentReplayAgent{
		name:        "researcher",
		description: "Research public information.",
		response:    "child replay answer",
	}
	sourceRun := &RunSummary{
		RunID:       20,
		ThreadID:    10,
		ParentRunID: 15,
		RunKind:     RunKindSubagent,
		AssistantID: "singleagent:1001",
		Input: `{
			"schema":"coze.subagent_tool_call.v1",
			"tool_name":"researcher",
			"arguments":{"request":"redo analysis"}
		}`,
		Config: `{
			"runtime":"eino_adk",
			"agent_name":"researcher",
			"agent_description":"Research public information.",
			"single_agent":{"agent_id":1001,"version":"v1","is_draft":false},
			"full_chat_history":false,
			"tool_policy":{"allowed_tools":[],"allowed_dynamic_tools":[]}
		}`,
	}
	resolver := ADKSubagentRetrySourceResolverFunc(func(
		_ context.Context,
		req ADKSubagentRetrySourceRequest,
	) (*RunSummary, error) {
		require.Equal(t, int64(30), req.RetryRun.RunID)
		require.Equal(t, int64(20), req.SourceRunID)
		require.Equal(t, int64(15), req.ParentRunID)
		return sourceRun, nil
	})
	var factoryRun *RunSummary
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(
			_ context.Context,
			run *RunSummary,
		) (adk.ResumableAgent, error) {
			copied := *run
			factoryRun = &copied
			return childAgent, nil
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKSubagentRetrySourceResolver(resolver),
	)

	result, err := executor.ExecuteSubagentRetry(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    30,
		Command: `{
			"subagent_retry":{
				"schema":"coze.subagent_retry.v1",
				"source_run_id":20,
				"parent_run_id":15
			}
		}`,
	})

	require.NoError(t, err)
	require.Equal(t, "child replay answer", result.Message)
	require.JSONEq(t, `{
		"source":"eino_adk_subagent_retry",
		"source_run_id":20,
		"parent_run_id":15
	}`, result.Metadata)
	require.NotNil(t, factoryRun)
	require.Equal(t, int64(20), factoryRun.RunID)
	require.Equal(t, RunKindSubagent, factoryRun.RunKind)
	require.Equal(t, "redo analysis", childAgent.inputText())
}

func TestApplicationADKSubagentRetrySourceResolverLoadsSourceRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		gotRunsByID: map[int64]*entity.Run{
			20: {
				ID:          20,
				ThreadID:    10,
				ParentRunID: 15,
				RunKind:     entity.RunKindSubagent,
				Status:      entity.RunStatusFailed,
				Input:       `{"schema":"coze.subagent_tool_call.v1","tool_name":"researcher","arguments":{}}`,
			},
		},
	}
	resolver := NewApplicationADKSubagentRetrySourceResolver(
		&ApplicationService{ThreadSVC: domainSVC},
	)

	source, err := resolver.ResolveADKSubagentRetrySource(
		context.Background(),
		ADKSubagentRetrySourceRequest{
			RetryRun:    &RunSummary{RunID: 30, ThreadID: 10},
			SourceRunID: 20,
			ParentRunID: 15,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(20), domainSVC.getRunID)
	require.Equal(t, int64(20), source.RunID)
	require.Equal(t, RunKindSubagent, source.RunKind)
}

type recordingSubagentReplayAgent struct {
	mu          sync.Mutex
	name        string
	description string
	response    string
	inputs      [][]*schema.Message
}

func (a *recordingSubagentReplayAgent) Name(context.Context) string {
	return a.name
}

func (a *recordingSubagentReplayAgent) Description(context.Context) string {
	return a.description
}

func (a *recordingSubagentReplayAgent) Run(
	ctx context.Context,
	input *adk.AgentInput,
	_ ...adk.AgentRunOption,
) *adk.AsyncIterator[*adk.AgentEvent] {
	a.mu.Lock()
	if input != nil {
		a.inputs = append(a.inputs, append([]*schema.Message(nil), input.Messages...))
	}
	a.mu.Unlock()

	return adkAgentEventIterator([]*adk.AgentEvent{{
		AgentName: a.name,
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: schema.AssistantMessage(a.response, nil),
				Role:    schema.Assistant,
			},
		},
	}})
}

func (a *recordingSubagentReplayAgent) Resume(
	context.Context,
	*adk.ResumeInfo,
	...adk.AgentRunOption,
) *adk.AsyncIterator[*adk.AgentEvent] {
	return adkAgentEventIterator(nil)
}

func (a *recordingSubagentReplayAgent) inputText() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var builder strings.Builder
	for _, input := range a.inputs {
		for _, message := range input {
			if message != nil {
				builder.WriteString(message.Content)
			}
		}
	}
	return builder.String()
}

type scriptedADKAgent struct {
	run        func(context.Context) []*adk.AgentEvent
	resume     func(context.Context, *adk.ResumeInfo) []*adk.AgentEvent
	resumeInfo *adk.ResumeInfo
}

func (a *scriptedADKAgent) Name(context.Context) string {
	return "lead"
}

func (a *scriptedADKAgent) Description(context.Context) string {
	return "scripted test agent"
}

func (a *scriptedADKAgent) Run(
	ctx context.Context,
	input *adk.AgentInput,
	options ...adk.AgentRunOption,
) *adk.AsyncIterator[*adk.AgentEvent] {
	if a.run == nil {
		return adkAgentEventIterator(nil)
	}

	return adkAgentEventIterator(a.run(ctx))
}

func (a *scriptedADKAgent) Resume(
	ctx context.Context,
	info *adk.ResumeInfo,
	options ...adk.AgentRunOption,
) *adk.AsyncIterator[*adk.AgentEvent] {
	a.resumeInfo = info
	if a.resume == nil {
		return adkAgentEventIterator(nil)
	}

	return adkAgentEventIterator(a.resume(ctx, info))
}

type usageChatModel struct{}

func (m *usageChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return usageChatModelMessage(), nil
}

func (m *usageChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{usageChatModelMessage()}), nil
}

func usageChatModelMessage() *schema.Message {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: "callback answer",
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{
				PromptTokens:     12,
				CompletionTokens: 5,
				TotalTokens:      17,
				PromptTokenDetails: schema.PromptTokenDetails{
					CachedTokens: 3,
				},
				CompletionTokensDetails: schema.CompletionTokensDetails{
					ReasoningTokens: 2,
				},
			},
		},
	}
}

type summarizationIntegrationChatModel struct {
	mu         sync.Mutex
	calls      int
	usageKinds []string
	inputs     [][]*schema.Message
}

func (m *summarizationIntegrationChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.usageKinds = append(m.usageKinds, adkUsageKindFromContext(ctx))
	m.inputs = append(m.inputs, append([]*schema.Message(nil), input...))
	if m.calls == 1 {
		return schema.AssistantMessage("condensed context", nil), nil
	}
	return schema.AssistantMessage("final after summary", nil), nil
}

func (m *summarizationIntegrationChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *summarizationIntegrationChatModel) usageKindValues() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.usageKinds...)
}

func (m *summarizationIntegrationChatModel) finalInputMemoryCount(value string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.inputs) == 0 {
		return 0
	}
	count := 0
	for _, message := range m.inputs[len(m.inputs)-1] {
		if message != nil {
			count += strings.Count(message.Content, value)
		}
	}
	return count
}

func (m *summarizationIntegrationChatModel) inputMemoryCountAt(index int, value string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.inputs) {
		return 0
	}
	count := 0
	for _, message := range m.inputs[index] {
		if message != nil {
			count += strings.Count(message.Content, value)
		}
	}
	return count
}

type summarizingInterruptChatModel struct {
	mu         sync.Mutex
	usageKinds []string
	inputs     [][]*schema.Message
}

func (m *summarizingInterruptChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageKinds = append(m.usageKinds, adkUsageKindFromContext(ctx))
	m.inputs = append(m.inputs, append([]*schema.Message(nil), input...))
	if adkUsageKindFromContext(ctx) == string(ADKMiddlewareSummarization) {
		return schema.AssistantMessage("condensed context", nil), nil
	}
	for _, message := range input {
		if message != nil && message.Role == schema.Tool {
			return schema.AssistantMessage("final after approval", nil), nil
		}
	}
	return schema.AssistantMessage("", []schema.ToolCall{{
		ID:   "approval-call-1",
		Type: "function",
		Function: schema.FunctionCall{
			Name:      "approval",
			Arguments: `{}`,
		},
	}}), nil
}

func (m *summarizingInterruptChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *summarizingInterruptChatModel) WithTools(
	_ []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *summarizingInterruptChatModel) usageKindValues() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.usageKinds...)
}

func (m *summarizingInterruptChatModel) allInputText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var text strings.Builder
	for _, input := range m.inputs {
		for _, message := range input {
			if message != nil {
				text.WriteString(message.Content)
				text.WriteByte('\n')
			}
		}
	}
	return text.String()
}

type summarizationApprovalTool struct{}

func (t *summarizationApprovalTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "approval",
		Desc: "Require approval before continuing.",
	}, nil
}

func (t *summarizationApprovalTool) InvokableRun(
	ctx context.Context,
	_ string,
	_ ...tool.Option,
) (string, error) {
	wasInterrupted, _, _ := tool.GetInterruptState[string](ctx)
	if !wasInterrupted {
		return "", tool.StatefulInterrupt(ctx, "approval required", "pending")
	}
	isResumeTarget, hasData, data := tool.GetResumeContext[string](ctx)
	if !isResumeTarget || !hasData {
		return "", tool.StatefulInterrupt(ctx, "approval required", "pending")
	}
	return data, nil
}

func requireOrderedEventTypes(
	t *testing.T,
	actual []string,
	expected []string,
) {
	t.Helper()
	position := 0
	for _, eventType := range actual {
		if position < len(expected) && eventType == expected[position] {
			position++
		}
	}
	require.Equal(t, len(expected), position, "actual event types: %v", actual)
}

func adkAgentEventIterator(events []*adk.AgentEvent) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer generator.Close()
		for _, event := range events {
			generator.Send(event)
		}
	}()

	return iter
}

type memoryADKCheckpointStore struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newMemoryADKCheckpointStore() *memoryADKCheckpointStore {
	return &memoryADKCheckpointStore{values: make(map[string][]byte)}
}

func (s *memoryADKCheckpointStore) Get(
	ctx context.Context,
	checkpointID string,
) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, exists := s.values[checkpointID]
	return append([]byte(nil), value...), exists, nil
}

func (s *memoryADKCheckpointStore) Set(
	ctx context.Context,
	checkpointID string,
	checkpoint []byte,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values[checkpointID] = append([]byte(nil), checkpoint...)
	return nil
}

func (s *memoryADKCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.values, checkpointID)
	return nil
}

var _ adk.CheckPointStore = (*memoryADKCheckpointStore)(nil)
var _ adk.CheckPointDeleter = (*memoryADKCheckpointStore)(nil)
