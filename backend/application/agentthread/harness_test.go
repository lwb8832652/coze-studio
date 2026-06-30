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

func TestHarnessExecutorLoadsEnabledSkillsBeforePlanning(t *testing.T) {
	skillProvider := &recordingSkillProvider{
		skills: []AgentSkill{
			{
				ID:          101,
				Name:        "weekly-research",
				Description: "Research weekly changes.",
				Type:        "deer_skill",
				Version:     "1.2.0",
				Body:        "Collect sources and produce a concise brief.",
			},
		},
	}
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{
			{ID: "step-skill", Type: AgentStepTypeModel, Name: "generate_answer"},
		}},
	}
	runner := &recordingStepRunner{
		result: &AgentStepResult{Message: "已按技能生成周报", Final: true},
	}
	eventSink := &recordingRunEventSink{}
	checkpointSink := &recordingCheckpointSink{nextID: 700}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       1,
		EventSink:      eventSink,
		SkillProvider:  skillProvider,
		CheckpointSink: checkpointSink,
	})
	run := &RunSummary{ThreadID: 9, RunID: 18, SpaceID: 7, Input: `{"message":"写周报"}`}

	result, err := executor.Execute(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "已按技能生成周报", result.Message)
	require.Contains(t, result.Metadata, `"skill_count":1`)
	require.Equal(t, 1, skillProvider.calls)
	require.Equal(t, run, skillProvider.run)
	require.Len(t, planner.state.Skills.Items, 1)
	require.Equal(t, "weekly-research", planner.state.Skills.Items[0].Name)
	require.Equal(t, "Collect sources and produce a concise brief.", runner.state.Skills.Items[0].Body)
	require.Equal(t, []string{"skills.loaded", "step.started", "step.completed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"skill_count":1`)
	require.Contains(t, eventSink.events[0].Payload, `"skill_ids":["101"]`)
	require.Contains(t, checkpointSink.checkpoints[0].ChannelValues, `"skills":{"items":[{`)
	require.Contains(t, checkpointSink.checkpoints[0].ChannelValues, `"id":"101"`)
	require.Contains(t, checkpointSink.checkpoints[0].ChannelVersions, `"skills":1`)
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

func TestHarnessInputMessagesPreservesMultimodalFields(t *testing.T) {
	messages := harnessInputMessages(`{
		"messages":[{
			"role":"user",
			"content":"",
			"user_input_multi_content":[{
				"type":"file_url",
				"file":{
					"url":"https://example.test/report.pdf",
					"mime_type":"application/pdf",
					"name":"report.pdf"
				}
			}]
		}]
	}`)

	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0]["role"])
	parts, ok := messages[0]["user_input_multi_content"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	part, ok := parts[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "file_url", part["type"])
	file, ok := part["file"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "report.pdf", file["name"])
	require.Equal(
		t,
		"https://example.test/report.pdf",
		file["url"],
	)
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

func TestHarnessExecutorResumeRunsPendingStepWithoutPlanning(t *testing.T) {
	planner := &recordingPlanner{err: errors.New("planner should not run during resume")}
	runner := &recordingStepRunner{
		result: &AgentStepResult{
			Message:  "resumed answer",
			Metadata: `{"source":"resume_step"}`,
			Final:    true,
		},
	}
	eventSink := &recordingRunEventSink{}
	checkpointSink := &recordingCheckpointSink{nextID: 600}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       2,
		EventSink:      eventSink,
		CheckpointSink: checkpointSink,
	})
	run := &RunSummary{ThreadID: 9, RunID: 18, Input: `{"message":"resume"}`}
	input := &HarnessResumeInput{
		ThreadID:     9,
		RunID:        18,
		CheckpointID: 503,
		CheckpointNS: "harness.terminal",
		ResumeFrom:   "pending_sends",
		State: AgentHarnessState{
			StepIndex: 1,
			Results: []AgentStepResult{
				{Message: "partial", Final: false},
			},
			Steps: []AgentExecutedStep{
				{StepID: "step-1", StepType: AgentStepTypeModel, StepName: "draft", StepIndex: 0, Message: "partial"},
			},
		},
		PendingSteps: []AgentStep{
			{ID: "step-2", Type: AgentStepTypeModel, Name: "finalize", Final: true},
		},
	}

	result, err := executor.Resume(context.Background(), run, input)

	require.NoError(t, err)
	require.Equal(t, "resumed answer", result.Message)
	require.Contains(t, result.Metadata, `"source":"agent_harness"`)
	require.Contains(t, result.Metadata, `"steps":2`)
	require.Zero(t, planner.calls)
	require.Equal(t, 1, runner.calls)
	require.Equal(t, "step-2", runner.step.ID)
	require.Equal(t, 1, runner.state.StepIndex)
	require.Len(t, runner.state.Steps, 1)
	require.Len(t, runner.state.Results, 1)
	require.Equal(t, []string{"step.started", "step.completed"}, eventSink.eventTypes())
	require.Len(t, checkpointSink.checkpoints, 2)
	require.Equal(t, []string{"harness.step", "harness.terminal"}, checkpointSink.namespaces())
	require.Equal(t, int64(503), checkpointSink.checkpoints[0].ParentCheckpointID)
	require.Contains(t, checkpointSink.checkpoints[0].ChannelValues, `"step_id":"step-2"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"status":"succeeded"`)
}

func TestHarnessExecutorPersistsInterruptedStepForResume(t *testing.T) {
	planner := &recordingPlanner{
		plan: &AgentPlan{Steps: []AgentStep{{
			ID:    "clarify-1",
			Type:  AgentStepTypeModel,
			Name:  "clarify",
			Final: true,
		}}},
	}
	runner := &recordingStepRunner{
		err: &RunInterruptedError{
			Interrupts: []ADKInterruptItem{{
				ID:          "clarification",
				Address:     "agent:lead;step:clarify",
				Info:        map[string]any{"question": "Which region?"},
				IsRootCause: true,
			}},
		},
	}
	eventSink := &recordingRunEventSink{}
	checkpointSink := &recordingCheckpointSink{nextID: 800}
	executor := NewHarnessExecutor(planner, runner, HarnessExecutorOptions{
		MaxSteps:       2,
		EventSink:      eventSink,
		CheckpointSink: checkpointSink,
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		ThreadID: 9,
		RunID:    18,
		Input:    `{"message":"prepare report"}`,
	})

	require.Nil(t, result)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, err, &interrupted)
	require.Equal(t, "801", interrupted.CheckpointKey)
	require.Len(t, interrupted.Interrupts, 1)
	require.False(t, interrupted.EventPersisted)
	require.Equal(t, []string{"step.started"}, eventSink.eventTypes())
	require.Len(t, checkpointSink.checkpoints, 2)
	require.Equal(t, []string{"harness.initial", "harness.terminal"}, checkpointSink.namespaces())
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"status":"interrupted"`)
	require.Contains(t, checkpointSink.checkpoints[1].Metadata, `"checkpoint_phase":"interrupt"`)
	require.Contains(t, checkpointSink.checkpoints[1].PendingSends, `"step_id":"clarify-1"`)
}

func TestHarnessExecutorResumeWritesFailedCheckpointWhenPendingStepFails(t *testing.T) {
	runner := &recordingStepRunner{err: errors.New("resume step failed")}
	checkpointSink := &recordingCheckpointSink{nextID: 700}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(&recordingPlanner{err: errors.New("planner should not run")}, runner, HarnessExecutorOptions{
		MaxSteps:       2,
		EventSink:      eventSink,
		CheckpointSink: checkpointSink,
	})
	run := &RunSummary{ThreadID: 9, RunID: 19, Input: `{"message":"resume"}`}
	input := &HarnessResumeInput{
		ThreadID:     9,
		RunID:        19,
		CheckpointID: 503,
		CheckpointNS: "harness.terminal",
		ResumeFrom:   "pending_sends",
		State: AgentHarnessState{
			StepIndex: 1,
			Steps: []AgentExecutedStep{
				{StepID: "step-1", StepType: AgentStepTypeModel, StepName: "draft", StepIndex: 0, Message: "partial"},
			},
		},
		PendingSteps: []AgentStep{
			{ID: "step-2", Type: AgentStepTypeModel, Name: "retry"},
			{ID: "step-3", Type: AgentStepTypeModel, Name: "summarize"},
		},
	}

	result, err := executor.Resume(context.Background(), run, input)

	require.ErrorContains(t, err, "resume step failed")
	require.Nil(t, result)
	require.Zero(t, executor.planner.(*recordingPlanner).calls)
	require.Equal(t, []string{"step.started", "step.failed"}, eventSink.eventTypes())
	require.Len(t, checkpointSink.checkpoints, 1)
	require.Equal(t, "harness.terminal", checkpointSink.checkpoints[0].CheckpointNS)
	require.Equal(t, int64(503), checkpointSink.checkpoints[0].ParentCheckpointID)
	require.Contains(t, checkpointSink.checkpoints[0].Metadata, `"status":"failed"`)
	require.Contains(t, checkpointSink.checkpoints[0].Metadata, `"error_type":"step_error"`)
	require.Contains(t, checkpointSink.checkpoints[0].PendingSends, `"step_id":"step-2"`)
	require.Contains(t, checkpointSink.checkpoints[0].PendingSends, `"step_id":"step-3"`)
}

func TestHarnessExecutorResumePersistsRepeatedInterrupt(t *testing.T) {
	runner := &recordingStepRunner{
		err: &RunInterruptedError{
			Interrupts: []ADKInterruptItem{{
				ID:          "clarification",
				Address:     "agent:lead;step:clarify",
				IsRootCause: true,
			}},
		},
	}
	checkpointSink := &recordingCheckpointSink{nextID: 850}
	eventSink := &recordingRunEventSink{}
	executor := NewHarnessExecutor(
		&recordingPlanner{err: errors.New("planner should not run")},
		runner,
		HarnessExecutorOptions{
			MaxSteps:       2,
			EventSink:      eventSink,
			CheckpointSink: checkpointSink,
		},
	)

	result, err := executor.Resume(
		context.Background(),
		&RunSummary{ThreadID: 9, RunID: 19, Input: `{"message":"APAC"}`},
		&HarnessResumeInput{
			ThreadID:     9,
			RunID:        19,
			CheckpointID: 801,
			CheckpointNS: "harness.terminal",
			ResumeFrom:   "interrupt",
			PendingSteps: []AgentStep{{
				ID:    "clarify-1",
				Type:  AgentStepTypeModel,
				Name:  "clarify",
				Final: true,
			}},
		},
	)

	require.Nil(t, result)
	var interrupted *RunInterruptedError
	require.ErrorAs(t, err, &interrupted)
	require.Equal(t, "850", interrupted.CheckpointKey)
	require.Equal(t, []string{"step.started"}, eventSink.eventTypes())
	require.Len(t, checkpointSink.checkpoints, 1)
	require.Equal(t, int64(801), checkpointSink.checkpoints[0].ParentCheckpointID)
	require.Contains(t, checkpointSink.checkpoints[0].Metadata, `"status":"interrupted"`)
	require.Contains(t, checkpointSink.checkpoints[0].PendingSends, `"step_id":"clarify-1"`)
}

func TestHarnessExecutorResumeRejectsInputForDifferentRun(t *testing.T) {
	executor := NewHarnessExecutor(&recordingPlanner{err: errors.New("planner should not run")}, &recordingStepRunner{
		result: &AgentStepResult{Message: "should not run", Final: true},
	}, HarnessExecutorOptions{MaxSteps: 1})

	result, err := executor.Resume(context.Background(), &RunSummary{ThreadID: 9, RunID: 18}, &HarnessResumeInput{
		ThreadID:     9,
		RunID:        99,
		CheckpointID: 503,
		PendingSteps: []AgentStep{{ID: "step-2", Type: AgentStepTypeModel}},
	})

	require.ErrorContains(t, err, "resume input does not belong to run")
	require.Nil(t, result)
	require.Zero(t, executor.planner.(*recordingPlanner).calls)
	require.Zero(t, executor.runner.(*recordingStepRunner).calls)
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

func TestModelStepRunnerInjectsSkillInstructionsIntoSystemPrompt(t *testing.T) {
	executor := &recordingRunExecutor{
		result: &RunExecutionResult{Message: "完成", Metadata: `{"source":"model"}`},
	}
	runner := NewModelStepRunner(executor)
	run := &RunSummary{
		RunID: 18,
		Input: `{"message":"写周报"}`,
		Config: `{
			"model_id": 100002,
			"system_prompt": "你是任务执行助手"
		}`,
	}

	result, err := runner.RunStep(context.Background(), run, AgentStep{
		ID:   "model-1",
		Type: AgentStepTypeModel,
	}, AgentHarnessState{
		Skills: AgentSkillContext{Items: []AgentSkill{
			{
				ID:          101,
				Name:        "weekly-research",
				Description: "Research weekly changes.",
				Type:        "deer_skill",
				Version:     "1.2.0",
				Body:        "Collect sources and produce a concise brief.",
			},
		}},
	})

	require.NoError(t, err)
	require.Equal(t, "完成", result.Message)
	require.NotNil(t, executor.run)
	require.NotSame(t, run, executor.run)
	require.Equal(t, run.Config, `{
			"model_id": 100002,
			"system_prompt": "你是任务执行助手"
		}`)
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(executor.run.Config), &config))
	require.Equal(t, float64(100002), config["model_id"])
	systemPrompt := config["system_prompt"].(string)
	require.Contains(t, systemPrompt, "你是任务执行助手")
	require.Contains(t, systemPrompt, "## Enabled Skills")
	require.Contains(t, systemPrompt, "weekly-research")
	require.Contains(t, systemPrompt, "Collect sources and produce a concise brief.")
}

func TestModelStepRunnerMarksSlashActivatedSkillInSystemPrompt(t *testing.T) {
	executor := &recordingRunExecutor{
		result: &RunExecutionResult{Message: "完成", Metadata: `{"source":"model"}`},
	}
	runner := NewModelStepRunner(executor)
	run := &RunSummary{
		RunID:  19,
		Input:  `{"messages":[{"role":"user","content":"/weekly-research collect market changes"}]}`,
		Config: `{}`,
	}

	_, err := runner.RunStep(context.Background(), run, AgentStep{
		ID:   "model-1",
		Type: AgentStepTypeModel,
	}, AgentHarnessState{
		Skills: AgentSkillContext{Items: []AgentSkill{
			{
				ID:          101,
				Name:        "weekly-research",
				Description: "Research weekly changes.",
				Type:        "deer_skill",
				Version:     "1.2.0",
				Body:        "Collect sources and produce a concise brief.",
			},
		}},
	})

	require.NoError(t, err)
	require.NotNil(t, executor.run)
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(executor.run.Config), &config))
	systemPrompt := config["system_prompt"].(string)
	require.Contains(t, systemPrompt, "## Slash Skill Activation")
	require.Contains(t, systemPrompt, "explicitly activated the `weekly-research` skill")
	require.Contains(t, systemPrompt, "<user_request>\ncollect market changes\n</user_request>")
	require.Contains(t, systemPrompt, "Follow this skill before choosing a general workflow")
}

func TestNewApplicationHarnessExecutorConfiguresDurableSinks(t *testing.T) {
	skillProvider := &recordingSkillProvider{}
	executor := NewApplicationHarnessExecutor(&ApplicationService{ThreadSVC: &recordingThreadService{}}, skillProvider)

	require.NotNil(t, executor)
	require.NotNil(t, executor.eventSink)
	require.NotNil(t, executor.memoryProvider)
	require.Equal(t, skillProvider, executor.skillProvider)
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

type recordingSkillProvider struct {
	skills []AgentSkill
	err    error
	run    *RunSummary
	calls  int
}

func (p *recordingSkillProvider) Load(ctx context.Context, run *RunSummary) ([]AgentSkill, error) {
	p.calls++
	p.run = run
	if p.err != nil {
		return nil, p.err
	}

	return append([]AgentSkill(nil), p.skills...), nil
}

type recordingRunExecutor struct {
	result *RunExecutionResult
	err    error
	run    *RunSummary
	calls  int
}

func (e *recordingRunExecutor) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
	e.calls++
	e.run = run
	if e.err != nil {
		return nil, e.err
	}

	return e.result, nil
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
