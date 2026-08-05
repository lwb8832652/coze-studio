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
	"strconv"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/coze-studio/backend/internal/deerflowparity"
	"github.com/stretchr/testify/require"
)

func TestADKParitySimpleAnswer(t *testing.T) {
	chatModel := &contractChatModel{
		message: contractAssistantMessage("contract answer"),
	}

	legacy := runRuntimeContract(t, RuntimeModeLegacy, chatModel)
	current := runRuntimeContract(t, RuntimeModeEinoADK, chatModel)

	require.Equal(t, legacy.Terminal, current.Terminal)
	require.Equal(t, legacy.Message, current.Message)
	require.Equal(t, legacy.CompletedMessage, current.CompletedMessage)
	require.Equal(t, legacy.SemanticEvents, current.SemanticEvents)
	require.Equal(t, runtimeContractSucceeded, current.Terminal)
}

func TestADKParityModelFailure(t *testing.T) {
	chatModel := &contractChatModel{err: errors.New("model unavailable")}

	legacy := runRuntimeContract(t, RuntimeModeLegacy, chatModel)
	current := runRuntimeContract(t, RuntimeModeEinoADK, chatModel)

	require.Equal(t, legacy.Terminal, current.Terminal)
	require.Equal(t, legacy.Failed, current.Failed)
	require.Equal(t, runtimeContractFailed, current.Terminal)
}

func TestADKParityTokenUsage(t *testing.T) {
	chatModel := &contractChatModel{
		message: contractAssistantMessage("usage answer"),
	}

	legacy := runRuntimeContract(t, RuntimeModeLegacy, chatModel)
	current := runRuntimeContract(t, RuntimeModeEinoADK, chatModel)

	require.Equal(t, int64(12), legacy.InputTokens)
	require.Equal(t, int64(5), legacy.OutputTokens)
	require.Equal(t, int64(17), legacy.TotalTokens)
	require.Equal(t, legacy.InputTokens, current.InputTokens)
	require.Equal(t, legacy.OutputTokens, current.OutputTokens)
	require.Equal(t, legacy.TotalTokens, current.TotalTokens)
}

func TestADKParityToolCallAndEventOrdering(t *testing.T) {
	legacySink := &recordingRunEventSink{}
	legacyExecutor := NewHarnessExecutor(
		contractPlannerFunc(func(context.Context, *RunSummary, AgentHarnessState) (*AgentPlan, error) {
			return &AgentPlan{Steps: []AgentStep{
				{
					ID:       "tool-1",
					Type:     AgentStepTypeTool,
					Name:     "search",
					ToolName: "search",
				},
				{
					ID:    "model-1",
					Type:  AgentStepTypeModel,
					Name:  "answer",
					Final: true,
				},
			}}, nil
		}),
		contractStepRunnerFunc(func(
			_ context.Context,
			_ *RunSummary,
			step AgentStep,
			_ AgentHarnessState,
		) (*AgentStepResult, error) {
			if step.Type == AgentStepTypeTool {
				return &AgentStepResult{Message: `{"result":"ok"}`}, nil
			}
			return &AgentStepResult{Message: "tool answer", Final: true}, nil
		}),
		HarnessExecutorOptions{EventSink: legacySink},
	)
	legacy := observeRuntimeContract(t, legacyExecutor, legacySink, nil)

	currentSink := &recordingRunEventSink{}
	currentExecutor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return &scriptedADKAgent{
				run: func(context.Context) []*adk.AgentEvent {
					return []*adk.AgentEvent{
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
									Message: schema.AssistantMessage("tool answer", nil),
									Role:    schema.Assistant,
								},
							},
						},
					}
				},
			}, nil
		}),
		currentSink,
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
	)
	current := observeRuntimeContract(t, currentExecutor, currentSink, nil)

	require.Equal(t, legacy.Terminal, current.Terminal)
	require.Equal(t, legacy.Message, current.Message)
	require.Equal(t, []string{"tool.completed", "message.completed"}, legacy.SemanticEvents)
	require.Equal(t, legacy.SemanticEvents, current.SemanticEvents)
}

func TestADKParityCancellationTerminal(t *testing.T) {
	legacySink := &recordingRunEventSink{}
	legacyExecutor := NewHarnessExecutor(
		contractPlannerFunc(func(context.Context, *RunSummary, AgentHarnessState) (*AgentPlan, error) {
			return &AgentPlan{Steps: []AgentStep{{
				ID:    "model-1",
				Type:  AgentStepTypeModel,
				Name:  "answer",
				Final: true,
			}}}, nil
		}),
		contractStepRunnerFunc(func(
			context.Context,
			*RunSummary,
			AgentStep,
			AgentHarnessState,
		) (*AgentStepResult, error) {
			return nil, context.Canceled
		}),
		HarnessExecutorOptions{EventSink: legacySink},
	)
	legacy := observeRuntimeContract(t, legacyExecutor, legacySink, nil)

	current, _ := runADKCancellationContract(t)

	require.Equal(t, runtimeContractCanceled, legacy.Terminal)
	require.Equal(t, legacy.Terminal, current.Terminal)
}

func TestADKSemanticCoreAcceptanceModeProjection(t *testing.T) {
	suite, err := deerflowparity.LoadCases()
	require.NoError(t, err)
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	for _, testCase := range suite.Cases {
		t.Run(testCase.ID, func(t *testing.T) {
			input := deerflowparity.BuildRunInput(testCase)
			rawContext, marshalErr := json.Marshal(input.Context)
			require.NoError(t, marshalErr)

			_, config, normalizeErr := normalizeNewDeerFlowRunConfig("{}", policy, string(rawContext))
			require.NoError(t, normalizeErr)
			require.Equal(t, RuntimeModeEinoADK, config.Runtime)
			require.Equal(t, DeerFlowMode(testCase.Mode), config.Mode)
			require.Equal(t, input.Context["thinking_enabled"], config.ThinkingEnabled)
			require.Equal(t, input.Context["is_plan_mode"], config.IsPlanMode)
			require.Equal(t, input.Context["subagent_enabled"], config.SubagentEnabled)
			expectedEffort, _ := input.Context["reasoning_effort"].(string)
			require.Equal(t, expectedEffort, config.ReasoningEffort)
			if config.SubagentEnabled {
				require.Equal(t, defaultDeerFlowMaxConcurrentSubagents, config.MaxConcurrentSubagents)
			}
		})
	}
}

func TestADKSemanticCoreAcceptanceTodoStateSurvivesReload(t *testing.T) {
	testCase := semanticCoreContractCase(t, "core.pro.todo")
	require.True(t, semanticCaseHasAction(testCase, deerflowparity.ActionReloadState))
	require.True(t, testCase.Expect.Todo.Required)
	require.True(t, testCase.Expect.Todo.AllCompleted)

	initial, err := NewADKParityStateTracker(contractRun(20), nil)
	require.NoError(t, err)
	require.NoError(t, initial.ReplaceTodos([]ADKParityTodo{
		{ID: "one", Title: "first", Status: "completed"},
		{ID: "two", Title: "second", Status: "completed"},
		{ID: "three", Title: "third", Status: "completed"},
	}))
	persisted, err := json.Marshal(initial.Snapshot())
	require.NoError(t, err)
	var seed ADKParityState
	require.NoError(t, json.Unmarshal(persisted, &seed))

	reloaded, err := NewADKParityStateTracker(contractRun(21), &seed)
	require.NoError(t, err)
	snapshot := reloaded.Snapshot()
	require.Equal(t, int64(21), snapshot.LastRunID)
	require.Len(t, snapshot.Todos, 3)
	for _, todo := range snapshot.Todos {
		require.Equal(t, "completed", todo.Status)
	}
}

func TestADKSemanticCoreAcceptanceLifecycleContracts(t *testing.T) {
	for _, caseID := range []string{"core.pro.direct", "core.ultra.direct"} {
		testCase := semanticCoreContractCase(t, caseID)
		observation := runRuntimeContract(t, RuntimeModeEinoADK, &contractChatModel{
			message: contractAssistantMessage("contract answer"),
		})
		families := []string{"run.started"}
		for _, eventType := range observation.SemanticEvents {
			if eventType == "message.completed" {
				families = append(families, "assistant.completed")
			}
		}
		if observation.TotalTokens > 0 {
			families = append(families, "token.usage")
		}
		if observation.Terminal == runtimeContractSucceeded {
			families = append(families, "run.completed")
		}
		requireSemanticEventOrder(t, testCase, families)
	}

	clarifyCase := semanticCoreContractCase(t, "core.clarify.followup")
	require.True(t, clarifyCase.Expect.Clarification.Required)
	require.True(t, clarifyCase.Expect.Clarification.FollowUpRequired)
	require.True(t, semanticCaseHasAction(clarifyCase, deerflowparity.ActionFollowUp))

	cancelCase := semanticCoreContractCase(t, "core.cancel")
	require.True(t, cancelCase.Expect.NoSuccessAfterCancel)
	canceled, events := runADKCancellationContract(t)
	require.Equal(t, runtimeContractCanceled, canceled.Terminal)
	for _, event := range events {
		require.NotEqual(t, "message.completed", event.EventType)
		require.NotEqual(t, "step.completed", event.EventType)
	}
}

func TestADKParityClarificationInterruptAndResume(t *testing.T) {
	legacy := runLegacyClarificationContract(t)
	current := runADKClarificationContract(t)

	require.Equal(t, runtimeContractPaused, legacy.Interrupted.Terminal)
	require.Equal(t, legacy.Interrupted.Terminal, current.Interrupted.Terminal)
	require.Equal(t, runtimeContractSucceeded, legacy.Resumed.Terminal)
	require.Equal(t, legacy.Resumed.Terminal, current.Resumed.Terminal)
	require.Equal(t, legacy.Resumed.Message, current.Resumed.Message)
	require.True(t, legacy.FreshRuntimeUsed)
	require.True(t, current.FreshRuntimeUsed)
}

func TestADKCheckpointRestartUsesFreshExecutorAndAgent(t *testing.T) {
	current := runADKClarificationContract(t)

	require.Equal(t, runtimeContractPaused, current.Interrupted.Terminal)
	require.Equal(t, runtimeContractSucceeded, current.Resumed.Terminal)
	require.True(t, current.FreshRuntimeUsed)
	require.True(t, current.ResumeLoadedCheckpoint)
	require.Equal(t, "clarified answer", current.Resumed.Message)
}

type runtimeContractTerminal string

const (
	runtimeContractSucceeded runtimeContractTerminal = "succeeded"
	runtimeContractFailed    runtimeContractTerminal = "failed"
	runtimeContractCanceled  runtimeContractTerminal = "canceled"
	runtimeContractPaused    runtimeContractTerminal = "interrupted"
)

type runtimeContractObservation struct {
	Terminal         runtimeContractTerminal
	Message          string
	CompletedMessage bool
	Failed           bool
	SemanticEvents   []string
	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
}

type runtimeContractLifecycle struct {
	Interrupted            runtimeContractObservation
	Resumed                runtimeContractObservation
	FreshRuntimeUsed       bool
	ResumeLoadedCheckpoint bool
}

func runRuntimeContract(
	t *testing.T,
	runtimeMode RuntimeMode,
	chatModel model.BaseChatModel,
) runtimeContractObservation {
	t.Helper()

	eventSink := &recordingRunEventSink{}
	usageCollector := &recordingADKUsageCollector{}
	provider := ChatModelProvider(func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return chatModel, true, nil
	})

	var executor RunExecutor
	switch runtimeMode {
	case RuntimeModeLegacy:
		executor = NewHarnessExecutor(nil, nil, HarnessExecutorOptions{
			ModelProvider:  provider,
			EventSink:      eventSink,
			UsageCollector: usageCollector,
		})
	case RuntimeModeEinoADK:
		executor = NewADKExecutor(
			NewApplicationADKAgentFactory(provider, nil, nil),
			eventSink,
			func(*RunSummary) (adk.CheckPointStore, error) {
				return newMemoryADKCheckpointStore(), nil
			},
			usageCollector,
		)
	default:
		t.Fatalf("unsupported contract runtime: %s", runtimeMode)
	}

	return observeRuntimeContract(t, executor, eventSink, usageCollector)
}

func runADKCancellationContract(t *testing.T) (runtimeContractObservation, []RunEvent) {
	t.Helper()

	eventSink := &recordingRunEventSink{}
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return &scriptedADKAgent{
				run: func(context.Context) []*adk.AgentEvent {
					return []*adk.AgentEvent{{
						AgentName: "lead",
						Err: &adk.CancelError{
							Info: &adk.AgentCancelInfo{Mode: adk.CancelAfterChatModel},
						},
					}}
				},
			}, nil
		}),
		eventSink,
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
	)

	observation := observeRuntimeContract(t, executor, eventSink, nil)
	return observation, append([]RunEvent(nil), eventSink.events...)
}

func semanticCoreContractCase(t *testing.T, caseID string) deerflowparity.Case {
	t.Helper()

	suite, err := deerflowparity.LoadCases()
	require.NoError(t, err)
	for _, testCase := range suite.Cases {
		if testCase.ID == caseID {
			return testCase
		}
	}
	t.Fatalf("semantic core contract case %q is missing", caseID)
	return deerflowparity.Case{}
}

func semanticCaseHasAction(testCase deerflowparity.Case, actionType deerflowparity.ActionType) bool {
	for _, action := range testCase.Actions {
		if action.Type == actionType {
			return true
		}
	}
	return false
}

func requireSemanticEventOrder(t *testing.T, testCase deerflowparity.Case, families []string) {
	t.Helper()

	for _, required := range testCase.Expect.RequiredEvents {
		require.Contains(t, families, required, testCase.ID)
	}
	for _, pair := range testCase.Expect.EventOrder {
		require.Len(t, pair, 2)
		before, after := -1, -1
		for index, family := range families {
			if family == pair[0] && before < 0 {
				before = index
			}
			if family == pair[1] && after < 0 {
				after = index
			}
		}
		require.GreaterOrEqual(t, before, 0, testCase.ID)
		require.GreaterOrEqual(t, after, 0, testCase.ID)
		require.Less(t, before, after, testCase.ID)
	}
}

func observeRuntimeContract(
	t *testing.T,
	executor RunExecutor,
	eventSink *recordingRunEventSink,
	usageCollector *recordingADKUsageCollector,
) runtimeContractObservation {
	t.Helper()

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Config:   `{"agent_name":"lead"}`,
		Input:    `{"messages":[{"role":"user","content":"contract input"}]}`,
	})
	return observeRuntimeContractResult(result, err, eventSink, usageCollector)
}

func runLegacyClarificationContract(t *testing.T) runtimeContractLifecycle {
	t.Helper()

	initialSink := &recordingRunEventSink{}
	checkpointSink := &recordingCheckpointSink{nextID: 900}
	initialRunner := contractStepRunnerFunc(func(
		context.Context,
		*RunSummary,
		AgentStep,
		AgentHarnessState,
	) (*AgentStepResult, error) {
		return nil, &RunInterruptedError{
			Interrupts: []ADKInterruptItem{{
				ID:          "clarification",
				Address:     "agent:lead;step:clarify",
				Info:        map[string]any{"question": "Which region?"},
				IsRootCause: true,
			}},
		}
	})
	initialExecutor := NewHarnessExecutor(
		contractPlannerFunc(func(context.Context, *RunSummary, AgentHarnessState) (*AgentPlan, error) {
			return &AgentPlan{Steps: []AgentStep{{
				ID:    "clarify-1",
				Type:  AgentStepTypeModel,
				Name:  "clarify",
				Final: true,
			}}}, nil
		}),
		initialRunner,
		HarnessExecutorOptions{
			EventSink:      initialSink,
			CheckpointSink: checkpointSink,
		},
	)
	initialResult, initialErr := initialExecutor.Execute(
		context.Background(),
		contractRun(20),
	)
	interruptedObservation := observeRuntimeContractResult(
		initialResult,
		initialErr,
		initialSink,
		nil,
	)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, initialErr, &interrupted)
	require.NotEmpty(t, interrupted.CheckpointKey)
	require.NotEmpty(t, checkpointSink.checkpoints)

	checkpointID, err := strconv.ParseInt(interrupted.CheckpointKey, 10, 64)
	require.NoError(t, err)
	persisted := checkpointSink.checkpoints[len(checkpointSink.checkpoints)-1]
	resumeRun := contractRun(21)
	resumeInput, err := loadHarnessResumeInput(
		resumeRun,
		resumeRunPayload{
			CheckpointID: checkpointID,
			CheckpointNS: persisted.CheckpointNS,
			ResumeFrom:   "interrupt",
		},
		&CheckpointSummary{
			CheckpointID:    checkpointID,
			ThreadID:        10,
			RunID:           20,
			CheckpointNS:    persisted.CheckpointNS,
			RuntimeType:     string(RuntimeModeLegacy),
			ChannelValues:   persisted.ChannelValues,
			ChannelVersions: persisted.ChannelVersions,
			PendingSends:    persisted.PendingSends,
			Metadata:        persisted.Metadata,
		},
	)
	require.NoError(t, err)

	resumeSink := &recordingRunEventSink{}
	resumeRunnerCalls := 0
	resumeExecutor := NewHarnessExecutor(
		contractPlannerFunc(func(context.Context, *RunSummary, AgentHarnessState) (*AgentPlan, error) {
			return nil, errors.New("planner must not run during resume")
		}),
		contractStepRunnerFunc(func(
			context.Context,
			*RunSummary,
			AgentStep,
			AgentHarnessState,
		) (*AgentStepResult, error) {
			resumeRunnerCalls++
			return &AgentStepResult{
				Message: "clarified answer",
				Final:   true,
			}, nil
		}),
		HarnessExecutorOptions{
			EventSink:      resumeSink,
			CheckpointSink: &recordingCheckpointSink{nextID: 1000},
		},
	)
	resumedResult, resumedErr := resumeExecutor.Resume(
		context.Background(),
		resumeRun,
		resumeInput,
	)

	return runtimeContractLifecycle{
		Interrupted: interruptedObservation,
		Resumed: observeRuntimeContractResult(
			resumedResult,
			resumedErr,
			resumeSink,
			nil,
		),
		FreshRuntimeUsed:       resumeRunnerCalls == 1,
		ResumeLoadedCheckpoint: len(resumeInput.PendingSteps) == 1,
	}
}

func runADKClarificationContract(t *testing.T) runtimeContractLifecycle {
	t.Helper()

	checkpointService := newContractCheckpointService()
	storeInstances := 0
	storeFactory := func(run *RunSummary) (adk.CheckPointStore, error) {
		storeInstances++
		return NewADKCheckpointStore(checkpointService, run)
	}
	initialSink := &recordingRunEventSink{}
	initialAgent := &scriptedADKAgent{
		run: func(ctx context.Context) []*adk.AgentEvent {
			return []*adk.AgentEvent{
				adk.Interrupt(ctx, map[string]any{"question": "Which region?"}),
			}
		},
	}
	initialExecutor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			return initialAgent, nil
		}),
		initialSink,
		storeFactory,
		nil,
	)
	initialResult, initialErr := initialExecutor.Execute(
		context.Background(),
		contractRun(20),
	)
	interruptedObservation := observeRuntimeContractResult(
		initialResult,
		initialErr,
		initialSink,
		nil,
	)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, initialErr, &interrupted)
	require.NotEmpty(t, interrupted.Interrupts)
	persisted, err := checkpointService.GetLatestRuntimeCheckpoint(
		context.Background(),
		&GetLatestRuntimeCheckpointRequest{
			ThreadID:    10,
			RunID:       20,
			RuntimeType: string(RuntimeModeEinoADK),
			RuntimeKey:  interrupted.CheckpointKey,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Checkpoint)
	envelope, err := UnmarshalADKCheckpointEnvelope(
		[]byte(persisted.Checkpoint.ChannelValues),
	)
	require.NoError(t, err)
	require.NotEmpty(t, envelope.Checkpoint)
	require.NotEmpty(t, envelope.Interrupts)

	targetID := interrupted.Interrupts[0].ID
	resumeAgent := &scriptedADKAgent{
		resume: func(context.Context, *adk.ResumeInfo) []*adk.AgentEvent {
			return []*adk.AgentEvent{{
				AgentName: "lead",
				Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{
						Message: schema.AssistantMessage("clarified answer", nil),
						Role:    schema.Assistant,
					},
				},
			}}
		},
	}
	resumeSink := &recordingRunEventSink{}
	resumeFactoryCalls := 0
	resumeExecutor := NewADKExecutor(
		ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
			resumeFactoryCalls++
			return resumeAgent, nil
		}),
		resumeSink,
		storeFactory,
		nil,
	)
	resumedResult, resumedErr := resumeExecutor.Resume(
		context.Background(),
		contractRun(21),
		&HarnessResumeInput{
			Runtime:      RuntimeModeEinoADK,
			RuntimeKey:   interrupted.CheckpointKey,
			ThreadID:     10,
			RunID:        21,
			SourceRunID:  20,
			CheckpointNS: adkCheckpointNamespace,
			ResumeFrom:   "interrupt",
			ADKResumeTargets: map[string]any{
				targetID: "APAC",
			},
			ADKCheckpoint: &envelope,
		},
	)

	return runtimeContractLifecycle{
		Interrupted: interruptedObservation,
		Resumed: observeRuntimeContractResult(
			resumedResult,
			resumedErr,
			resumeSink,
			nil,
		),
		FreshRuntimeUsed: resumeFactoryCalls == 1 &&
			initialAgent != resumeAgent,
		ResumeLoadedCheckpoint: resumeAgent.resumeInfo != nil &&
			resumeAgent.resumeInfo.WasInterrupted &&
			resumeAgent.resumeInfo.ResumeData == "APAC" &&
			storeInstances >= 3,
	}
}

func contractRun(runID int64) *RunSummary {
	return &RunSummary{
		ThreadID:  10,
		RunID:     runID,
		SpaceID:   7,
		CreatorID: 9,
		Config:    `{"agent_name":"lead"}`,
		Input:     `{"messages":[{"role":"user","content":"contract input"}]}`,
	}
}

type contractCheckpointService struct {
	mu     sync.Mutex
	nextID int64
	rows   map[string]*CheckpointSummary
}

func newContractCheckpointService() *contractCheckpointService {
	return &contractCheckpointService{
		nextID: 1100,
		rows:   make(map[string]*CheckpointSummary),
	}
}

func (s *contractCheckpointService) CreateCheckpoint(
	_ context.Context,
	req *CreateCheckpointRequest,
) (*CreateCheckpointResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	checkpoint := &CheckpointSummary{
		CheckpointID:       s.nextID,
		ThreadID:           req.ThreadID,
		RunID:              req.RunID,
		ParentCheckpointID: req.ParentCheckpointID,
		CheckpointNS:       req.CheckpointNS,
		RuntimeType:        req.RuntimeType,
		RuntimeKey:         req.RuntimeKey,
		EnvelopeVersion:    int32(req.EnvelopeVersion),
		ChannelValues:      req.ChannelValues,
		ChannelVersions:    req.ChannelVersions,
		PendingSends:       req.PendingSends,
		Metadata:           req.Metadata,
	}
	s.nextID++
	s.rows[contractCheckpointKey(
		req.ThreadID,
		req.RunID,
		req.RuntimeType,
		req.RuntimeKey,
	)] = checkpoint

	copy := *checkpoint
	return &CreateCheckpointResponse{Checkpoint: &copy}, nil
}

func (s *contractCheckpointService) GetLatestRuntimeCheckpoint(
	_ context.Context,
	req *GetLatestRuntimeCheckpointRequest,
) (*GetLatestRuntimeCheckpointResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	checkpoint := s.rows[contractCheckpointKey(
		req.ThreadID,
		req.RunID,
		req.RuntimeType,
		req.RuntimeKey,
	)]
	if checkpoint == nil {
		return &GetLatestRuntimeCheckpointResponse{}, nil
	}
	copy := *checkpoint
	return &GetLatestRuntimeCheckpointResponse{Checkpoint: &copy}, nil
}

func (s *contractCheckpointService) DeleteRuntimeCheckpoint(
	_ context.Context,
	req *DeleteRuntimeCheckpointRequest,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, contractCheckpointKey(
		req.ThreadID,
		req.RunID,
		req.RuntimeType,
		req.RuntimeKey,
	))
	return nil
}

func contractCheckpointKey(
	threadID int64,
	runID int64,
	runtimeType string,
	runtimeKey string,
) string {
	return strconv.FormatInt(threadID, 10) + ":" +
		strconv.FormatInt(runID, 10) + ":" +
		runtimeType + ":" + runtimeKey
}

func observeRuntimeContractResult(
	result *RunExecutionResult,
	err error,
	eventSink *recordingRunEventSink,
	usageCollector *recordingADKUsageCollector,
) runtimeContractObservation {
	observation := runtimeContractObservation{}
	switch {
	case err == nil:
		observation.Terminal = runtimeContractSucceeded
		observation.Message = resultMessage(result)
	case errors.As(err, new(*RunCanceledError)), errors.Is(err, context.Canceled):
		observation.Terminal = runtimeContractCanceled
	case errors.As(err, new(*RunInterruptedError)):
		observation.Terminal = runtimeContractPaused
	default:
		observation.Terminal = runtimeContractFailed
	}

	for _, event := range eventSink.events {
		switch event.EventType {
		case "step.completed":
			observation.CompletedMessage = true
			observation.SemanticEvents = append(observation.SemanticEvents, "message.completed")
		case "message.completed":
			observation.CompletedMessage = true
			observation.SemanticEvents = append(observation.SemanticEvents, "message.completed")
		case "tool.completed":
			observation.SemanticEvents = append(observation.SemanticEvents, "tool.completed")
		case "step.failed", "run.runtime_error":
			observation.Failed = true
		}
	}
	if usageCollector != nil {
		for _, usage := range usageCollector.usages {
			observation.InputTokens += usage.InputTokens
			observation.OutputTokens += usage.OutputTokens
			observation.TotalTokens += usage.TotalTokens
		}
	}
	return observation
}

type contractPlannerFunc func(
	ctx context.Context,
	run *RunSummary,
	state AgentHarnessState,
) (*AgentPlan, error)

func (f contractPlannerFunc) Plan(
	ctx context.Context,
	run *RunSummary,
	state AgentHarnessState,
) (*AgentPlan, error) {
	return f(ctx, run, state)
}

type contractStepRunnerFunc func(
	ctx context.Context,
	run *RunSummary,
	step AgentStep,
	state AgentHarnessState,
) (*AgentStepResult, error)

func (f contractStepRunnerFunc) RunStep(
	ctx context.Context,
	run *RunSummary,
	step AgentStep,
	state AgentHarnessState,
) (*AgentStepResult, error) {
	return f(ctx, run, step, state)
}

type contractChatModel struct {
	message *schema.Message
	err     error
}

func (m *contractChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.message, nil
}

func (m *contractChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if m.err != nil {
		return nil, m.err
	}
	return schema.StreamReaderFromArray([]*schema.Message{m.message}), nil
}

func contractAssistantMessage(content string) *schema.Message {
	return &schema.Message{
		Role:    schema.Assistant,
		Content: content,
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{
				PromptTokens:     12,
				CompletionTokens: 5,
				TotalTokens:      17,
			},
		},
	}
}
