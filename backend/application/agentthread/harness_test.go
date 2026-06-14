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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestHarnessExecutorRunsPlannedStepAndReturnsFinalMessage(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-1", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{
			Message:  "任务已完成",
			Metadata: `{"source":"step"}`,
			Final:    true,
		},
	}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:  3,
		EventSink: eventSink,
	})
	run := &RunSummary{ThreadID: 5, RunID: 10, Input: `{"message":"生成报告"}`}

	result, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "任务已完成", result.Message)
	require.Contains(t, result.Metadata, `"source":"agent_harness"`)
	require.Contains(t, result.Metadata, `"steps":1`)
	require.Equal(t, 1, planner.calls)
	require.Equal(t, 1, runner.calls)
	require.Equal(t, run, planner.run)
	require.Equal(t, AgentStepTypeModel, runner.step.Type)
	require.Equal(t, 0, runner.state.StepIndex)
	require.Equal(t, []string{"step.started", "step.completed"}, eventSink.eventTypes())
	require.Equal(t, int64(5), eventSink.events[0].ThreadID)
	require.Equal(t, int64(10), eventSink.events[0].RunID)
	require.Contains(t, eventSink.events[0].Payload, `"step_id":"step-1"`)
	require.Contains(t, eventSink.events[0].Payload, `"step_index":0`)
	require.Contains(t, eventSink.events[1].Payload, `"final":true`)
	require.Contains(t, eventSink.events[1].Payload, `"message_present":true`)
}

func TestHarnessExecutorStopsWhenMaxStepsExceeded(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-repeat", Type: AgentStepTypeModel, Name: "retry"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{Final: false},
	}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{MaxSteps: 2})

	result, err := executor.Execute(context.Background(), &RunSummary{RunID: 11, Input: `{"message":"hello"}`})

	require.Error(t, err)
	require.Contains(t, err.Error(), "agent harness exceeded max steps: 2")
	require.Nil(t, result)
	require.Equal(t, 2, planner.calls)
	require.Equal(t, 2, runner.calls)
	require.Len(t, runner.states, 2)
	require.Equal(t, 0, runner.states[0].StepIndex)
	require.Equal(t, 1, runner.states[1].StepIndex)
	require.Len(t, runner.states[1].Results, 1)
}

func TestHarnessExecutorReturnsContextCanceledBeforePlanning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-1", Type: AgentStepTypeModel},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{Message: "should not run", Final: true},
	}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{MaxSteps: 1})

	result, err := executor.Execute(ctx, &RunSummary{RunID: 12, Input: `{"message":"hello"}`})

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, result)
	require.Zero(t, planner.calls)
	require.Zero(t, runner.calls)
}

func TestHarnessExecutorEmitsStepFailedWhenRunnerErrors(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-err", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		err: errors.New("model step failed"),
	}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:  1,
		EventSink: eventSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{ThreadID: 6, RunID: 14, Input: `{"message":"hello"}`})

	require.Error(t, err)
	require.Contains(t, err.Error(), "model step failed")
	require.Nil(t, result)
	require.Equal(t, []string{"step.started", "step.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"step_id":"step-err"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_message":"model step failed"`)
}

func TestHarnessExecutorRunsToolStepAndEmitsToolEvents(t *testing.T) {
	registry := NewInMemoryToolRegistry()
	err := registry.Register("search_web", AgentToolFunc(func(ctx context.Context, call ToolCall) (*ToolResult, error) {
		require.Equal(t, `{"query":"coze studio"}`, call.Arguments)

		return &ToolResult{
			Content:  "找到 Coze Studio 资料",
			Metadata: `{"documents":2}`,
		}, nil
	}))
	require.NoError(t, err)
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{
				ID:            "tool-search",
				Type:          AgentStepTypeTool,
				Name:          "search_web",
				ToolName:      "search_web",
				ToolArguments: `{"query":"coze studio"}`,
				Final:         true,
			},
		}},
	}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(planner, nil, HarnessExecutorOptions{
		ToolRegistry: registry,
		EventSink:    eventSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{ThreadID: 7, RunID: 15})

	require.NoError(t, err)
	require.Equal(t, "找到 Coze Studio 资料", result.Message)
	require.Contains(t, result.Metadata, `"source":"agent_harness"`)
	require.Contains(t, result.Metadata, `"tool_name":"search_web"`)
	require.Contains(t, result.Metadata, `"documents":2`)
	require.Equal(t, []string{"tool.started", "tool.completed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"step_id":"tool-search"`)
	require.Contains(t, eventSink.events[0].Payload, `"step_type":"tool"`)
	require.Contains(t, eventSink.events[0].Payload, `"tool_name":"search_web"`)
	require.Contains(t, eventSink.events[0].Payload, `"arguments_present":true`)
	require.Contains(t, eventSink.events[1].Payload, `"result_present":true`)
}

func TestHarnessExecutorEmitsToolFailedWhenToolRunnerErrors(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{
				ID:       "tool-missing",
				Type:     AgentStepTypeTool,
				Name:     "search_web",
				ToolName: "search_web",
			},
		}},
	}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(planner, nil, HarnessExecutorOptions{
		ToolRegistry: NewInMemoryToolRegistry(),
		EventSink:    eventSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{ThreadID: 8, RunID: 16})

	require.ErrorContains(t, err, "unsupported agent tool: search_web")
	require.Nil(t, result)
	require.Equal(t, []string{"tool.started", "tool.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"tool_name":"search_web"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_message":"unsupported agent tool: search_web"`)
}

func TestHarnessExecutorDefaultModelStepUsesModelExecutor(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("默认模型回答", nil),
	}
	var gotModelID int64
	executor := NewHarnessExecutor(nil, nil, HarnessExecutorOptions{
		ModelProvider: func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			gotModelID = modelID

			return chatModel, true, nil
		},
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		RunID:  13,
		Input:  `{"message":"生成回答"}`,
		Config: `{"model_id":100002}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(100002), gotModelID)
	require.Equal(t, "默认模型回答", result.Message)
	require.Contains(t, result.Metadata, `"source":"agent_harness"`)
	require.Contains(t, result.Metadata, `"model_executor"`)
	require.Len(t, chatModel.messages, 1)
	require.Equal(t, "生成回答", chatModel.messages[0].Content)
}

type recordingPlanner struct {
	plan  *AgentPlan
	err   error
	run   *RunSummary
	state AgentHarnessState
	calls int
}

func (p *recordingPlanner) Plan(ctx context.Context, run *RunSummary, state AgentHarnessState) (*AgentPlan, error) {
	p.calls++
	p.run = run
	p.state = state
	if p.err != nil {
		return nil, p.err
	}

	return p.plan, nil
}

type recordingStepRunner struct {
	result *AgentStepResult
	err    error
	step   AgentStep
	state  AgentHarnessState
	states []AgentHarnessState
	calls  int
}

func (r *recordingStepRunner) RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error) {
	r.calls++
	r.step = step
	r.state = state
	r.states = append(r.states, state)
	if r.err != nil {
		return nil, r.err
	}

	return r.result, nil
}
