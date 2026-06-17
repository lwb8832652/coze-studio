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

func TestHarnessExecutorRecallsMemoryBeforePlanning(t *testing.T) {
	memoryProvider := &recordingMemoryProvider{
		memories: []AgentMemory{
			{
				ID:       "mem-1",
				Scope:    "thread",
				Content:  "用户偏好中文回答",
				Metadata: `{"source":"profile"}`,
				Score:    0.92,
			},
			{
				ID:      "mem-2",
				Scope:   "run",
				Content: "本次任务需要输出周报",
				Score:   0.81,
			},
		},
	}
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-memory", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{
			Message: "已按记忆生成周报",
			Final:   true,
		},
	}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		EventSink:      eventSink,
		MemoryProvider: memoryProvider,
	})
	run := &RunSummary{ThreadID: 9, RunID: 17, Input: `{"message":"写周报"}`}

	result, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "已按记忆生成周报", result.Message)
	require.Contains(t, result.Metadata, `"memory_count":2`)
	require.Equal(t, 1, memoryProvider.calls)
	require.Equal(t, run, memoryProvider.run)
	require.Len(t, planner.state.Memory.Items, 2)
	require.Equal(t, "用户偏好中文回答", planner.state.Memory.Items[0].Content)
	require.Equal(t, "thread", planner.state.Memory.Items[0].Scope)
	require.Len(t, runner.state.Memory.Items, 2)
	require.Equal(t, "本次任务需要输出周报", runner.state.Memory.Items[1].Content)
	require.Equal(t, []string{"memory.recalled", "step.started", "step.completed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"memory_count":2`)
	require.Contains(t, eventSink.events[0].Payload, `"scopes":["thread","run"]`)
}

func TestHarnessExecutorRecordsTokenUsageFromStepMetadata(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "model-usage", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{
			Message:  "已完成",
			Metadata: `{"usage":{"input_tokens":12,"output_tokens":8,"total_tokens":20,"model_name":"gpt-test","provider":"openai-compatible","estimated":false,"raw_usage":{"prompt_tokens":12,"completion_tokens":8}}}`,
			Final:    true,
		},
	}
	eventSink := &recordingRunEventSink{}
	usageCollector := &recordingUsageCollector{}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		EventSink:      eventSink,
		UsageCollector: usageCollector,
	})
	run := &RunSummary{ThreadID: 9, RunID: 17, Input: `{"message":"写周报"}`}

	result, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "已完成", result.Message)
	require.Equal(t, 1, usageCollector.calls)
	require.Equal(t, run, usageCollector.run)
	require.Equal(t, TokenUsageSourceLeadAgent, usageCollector.usage.Source)
	require.Equal(t, "model-usage", usageCollector.usage.StepID)
	require.Equal(t, int32(0), usageCollector.usage.StepIndex)
	require.Equal(t, "generate_answer", usageCollector.usage.StepName)
	require.Equal(t, "gpt-test", usageCollector.usage.ModelName)
	require.Equal(t, "openai-compatible", usageCollector.usage.Provider)
	require.Equal(t, int64(12), usageCollector.usage.InputTokens)
	require.Equal(t, int64(8), usageCollector.usage.OutputTokens)
	require.Equal(t, int64(20), usageCollector.usage.TotalTokens)
	require.Equal(t, `{"completion_tokens":8,"prompt_tokens":12}`, usageCollector.usage.RawUsage)
	require.Equal(t, []string{"step.started", "step.completed", "usage.recorded"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[2].Payload, `"step_id":"model-usage"`)
	require.Contains(t, eventSink.events[2].Payload, `"total_tokens":20`)
}

func TestHarnessExecutorWritesCheckpointsAcrossRunLifecycle(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "model-final", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{
			Message:  "任务已完成",
			Metadata: `{"usage":{"input_tokens":3,"output_tokens":4}}`,
			Final:    true,
		},
	}
	checkpointSink := &recordingCheckpointSink{nextID: 100}
	memoryProvider := &recordingMemoryProvider{
		memories: []AgentMemory{
			{ID: "mem-1", Scope: "thread", Content: "用户偏好中文", Score: 0.9},
		},
	}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		MemoryProvider: memoryProvider,
		CheckpointSink: checkpointSink,
	})
	run := &RunSummary{
		ThreadID: 5,
		RunID:    10,
		Input:    `{"messages":[{"role":"user","content":"生成报告"}]}`,
	}

	result, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "任务已完成", result.Message)
	require.Len(t, checkpointSink.checkpoints, 3)
	require.Equal(t, []string{"harness.initial", "harness.step", "harness.terminal"}, checkpointSink.namespaces())
	require.Equal(t, int64(0), checkpointSink.checkpoints[0].ParentCheckpointID)
	require.Equal(t, int64(100), checkpointSink.checkpoints[1].ParentCheckpointID)
	require.Equal(t, int64(101), checkpointSink.checkpoints[2].ParentCheckpointID)

	initialValues := decodeCheckpointValues(t, checkpointSink.checkpoints[0].ChannelValues)
	require.Equal(t, []any{map[string]any{"content": "生成报告", "role": "user"}}, initialValues["messages"])
	require.Contains(t, checkpointSink.checkpoints[0].Metadata, `"checkpoint_phase":"initial"`)
	require.Contains(t, checkpointSink.checkpoints[0].Metadata, `"status":"running"`)
	require.Contains(t, checkpointSink.checkpoints[0].ChannelValues, `"用户偏好中文"`)

	stepValues := decodeCheckpointValues(t, checkpointSink.checkpoints[1].ChannelValues)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"checkpoint_phase":"step"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"step_id":"model-final"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"status":"running"`)
	require.Contains(t, stepValues["messages"], map[string]any{
		"content": "任务已完成",
		"role":    "assistant",
		"step_id": "model-final",
	})

	terminalValues := decodeCheckpointValues(t, checkpointSink.checkpoints[2].ChannelValues)
	require.Contains(t, checkpointSink.checkpoints[2].Metadata, `"checkpoint_phase":"terminal"`)
	require.Contains(t, checkpointSink.checkpoints[2].Metadata, `"status":"succeeded"`)
	require.NotContains(t, checkpointSink.checkpoints[2].PendingSends, "generate_answer")
	require.Contains(t, terminalValues["steps"], map[string]any{
		"final":           true,
		"message_present": true,
		"step_id":         "model-final",
		"step_index":      float64(0),
		"step_name":       "generate_answer",
		"step_type":       "model",
	})
}

func TestHarnessExecutorReturnsCheckpointWriteError(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-1", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{Message: "should not run", Final: true},
	}
	checkpointSink := &recordingCheckpointSink{err: errors.New("checkpoint write failed")}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		CheckpointSink: checkpointSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 5,
		RunID:    10,
		Input:    `{"message":"hello"}`,
	})

	require.ErrorContains(t, err, "checkpoint write failed")
	require.Nil(t, result)
	require.Zero(t, planner.calls)
	require.Zero(t, runner.calls)
	require.Equal(t, 1, checkpointSink.calls)
}

func TestHarnessExecutorWritesFailedCheckpointWhenPlannerErrors(t *testing.T) {
	planner := &recordingPlanner{err: errors.New("planner failed")}
	runner := &recordingStepRunner{
		result: &AgentStepResult{Message: "should not run", Final: true},
	}
	checkpointSink := &recordingCheckpointSink{nextID: 200}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		CheckpointSink: checkpointSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 5,
		RunID:    10,
		Input:    `{"message":"hello"}`,
	})

	require.ErrorContains(t, err, "planner failed")
	require.Nil(t, result)
	require.Equal(t, 1, planner.calls)
	require.Zero(t, runner.calls)
	require.Len(t, checkpointSink.checkpoints, 2)
	require.Equal(t, []string{"harness.initial", "harness.terminal"}, checkpointSink.namespaces())
	require.Equal(t, int64(200), checkpointSink.checkpoints[1].ParentCheckpointID)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"checkpoint_phase":"terminal"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"status":"failed"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"error_type":"planner_error"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"error_message":"planner failed"`)
	require.Equal(t, `[]`, checkpointSink.checkpoints[1].PendingSends)
}

func TestHarnessExecutorWritesFailedCheckpointWhenRunnerErrors(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-err", Type: AgentStepTypeModel, Name: "generate_answer"},
			{ID: "step-next", Type: AgentStepTypeModel, Name: "summarize"},
		}},
	}
	runner := &recordingStepRunner{err: errors.New("model step failed")}
	checkpointSink := &recordingCheckpointSink{nextID: 300}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       2,
		CheckpointSink: checkpointSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 6,
		RunID:    14,
		Input:    `{"message":"hello"}`,
	})

	require.ErrorContains(t, err, "model step failed")
	require.Nil(t, result)
	require.Equal(t, 1, runner.calls)
	require.Len(t, checkpointSink.checkpoints, 2)
	require.Equal(t, []string{"harness.initial", "harness.terminal"}, checkpointSink.namespaces())
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"status":"failed"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"error_type":"step_error"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"step_id":"step-err"`)
	require.Contains(t, checkpointSink.checkpoints[1].PendingSends, `"step_id":"step-err"`)
	require.Contains(t, checkpointSink.checkpoints[1].PendingSends, `"step_id":"step-next"`)
}

func TestHarnessExecutorWritesCanceledCheckpointWhenRunnerIsCanceled(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-canceled", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{err: context.Canceled}
	checkpointSink := &recordingCheckpointSink{nextID: 400}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		CheckpointSink: checkpointSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 7,
		RunID:    15,
		Input:    `{"message":"hello"}`,
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.Len(t, checkpointSink.checkpoints, 2)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"status":"canceled"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"error_type":"context_canceled"`)
	require.Contains(t, checkpointSink.checkpoints[1].PendingSends, `"step_id":"step-canceled"`)
}

func TestHarnessExecutorWritesFailedCheckpointWhenMaxStepsExceeded(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-first", Type: AgentStepTypeModel, Name: "draft"},
			{ID: "step-second", Type: AgentStepTypeModel, Name: "revise"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{Message: "partial", Final: false},
	}
	checkpointSink := &recordingCheckpointSink{nextID: 500}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		CheckpointSink: checkpointSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 8,
		RunID:    16,
		Input:    `{"message":"hello"}`,
	})

	require.ErrorContains(t, err, "agent harness exceeded max steps: 1")
	require.Nil(t, result)
	require.Equal(t, 1, runner.calls)
	require.Len(t, checkpointSink.checkpoints, 3)
	require.Equal(t, []string{"harness.initial", "harness.step", "harness.terminal"}, checkpointSink.namespaces())
	require.Contains(t, checkpointSink.checkpoints[2].Metadata, `"status":"failed"`)
	require.Contains(t, checkpointSink.checkpoints[2].Metadata, `"error_type":"max_steps_exceeded"`)
	require.Contains(t, checkpointSink.checkpoints[2].PendingSends, `"step_id":"step-second"`)
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

func TestNewApplicationHarnessExecutorConfiguresDurableSinks(t *testing.T) {
	executor := NewApplicationHarnessExecutor(&ApplicationService{ThreadSVC: &recordingThreadService{}})

	require.NotNil(t, executor)
	require.NotNil(t, executor.eventSink)
	require.NotNil(t, executor.memoryProvider)
	require.NotNil(t, executor.usageCollector)
	require.NotNil(t, executor.checkpointSink)
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

type recordingMemoryProvider struct {
	memories []AgentMemory
	err      error
	run      *RunSummary
	calls    int
}

func (p *recordingMemoryProvider) Recall(ctx context.Context, run *RunSummary) ([]AgentMemory, error) {
	p.calls++
	p.run = run
	if p.err != nil {
		return nil, p.err
	}

	return append([]AgentMemory(nil), p.memories...), nil
}

type recordingUsageCollector struct {
	usage AgentTokenUsage
	run   *RunSummary
	err   error
	calls int
}

func (c *recordingUsageCollector) Record(ctx context.Context, run *RunSummary, usage AgentTokenUsage) error {
	c.calls++
	c.run = run
	c.usage = usage

	return c.err
}

type recordingCheckpointSink struct {
	checkpoints []AgentCheckpoint
	runs        []*RunSummary
	err         error
	nextID      int64
	calls       int
}

func (s *recordingCheckpointSink) SaveCheckpoint(ctx context.Context, run *RunSummary, checkpoint AgentCheckpoint) (*CheckpointSummary, error) {
	s.calls++
	s.runs = append(s.runs, run)
	s.checkpoints = append(s.checkpoints, checkpoint)
	if s.err != nil {
		return nil, s.err
	}
	id := s.nextID
	if id == 0 {
		id = int64(s.calls)
	}
	s.nextID = id + 1

	return &CheckpointSummary{
		CheckpointID: id,
		ThreadID:     run.ThreadID,
		RunID:        run.RunID,
	}, nil
}

func (s *recordingCheckpointSink) namespaces() []string {
	namespaces := make([]string, 0, len(s.checkpoints))
	for _, checkpoint := range s.checkpoints {
		namespaces = append(namespaces, checkpoint.CheckpointNS)
	}

	return namespaces
}

func decodeCheckpointValues(t *testing.T, raw string) map[string]any {
	t.Helper()

	var values map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &values))

	return values
}
