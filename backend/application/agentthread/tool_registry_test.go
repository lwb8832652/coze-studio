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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemoryToolRegistryRegistersAndLooksUpTool(t *testing.T) {
	registry := NewInMemoryToolRegistry()
	handler := AgentToolFunc(func(ctx context.Context, call ToolCall) (*ToolResult, error) {
		return &ToolResult{Content: "ok"}, nil
	})

	err := registry.Register("search_web", handler)

	require.NoError(t, err)
	tool, ok := registry.Lookup("search_web")
	require.True(t, ok)
	require.NotNil(t, tool)
	result, err := tool.Call(context.Background(), ToolCall{Name: "search_web"})
	require.NoError(t, err)
	require.Equal(t, "ok", result.Content)
}

func TestInMemoryToolRegistryRejectsInvalidRegistrations(t *testing.T) {
	registry := NewInMemoryToolRegistry()

	require.ErrorContains(t, registry.Register("", AgentToolFunc(func(ctx context.Context, call ToolCall) (*ToolResult, error) {
		return &ToolResult{}, nil
	})), "tool name is required")
	require.ErrorContains(t, registry.Register("search_web", nil), "tool handler is required")

	_, ok := registry.Lookup("")
	require.False(t, ok)
}

func TestToolStepRunnerInvokesRegisteredTool(t *testing.T) {
	registry := NewInMemoryToolRegistry()
	var gotCall ToolCall
	err := registry.Register("search_web", AgentToolFunc(func(ctx context.Context, call ToolCall) (*ToolResult, error) {
		gotCall = call

		return &ToolResult{
			Content:  `{"answer":"Coze"}`,
			Metadata: `{"latency_ms":25}`,
		}, nil
	}))
	require.NoError(t, err)
	runner := NewToolStepRunner(registry)
	run := &RunSummary{RunID: 10, ThreadID: 20}
	state := AgentHarnessState{StepIndex: 2}

	result, err := runner.RunStep(context.Background(), run, AgentStep{
		ID:            "tool-1",
		Type:          AgentStepTypeTool,
		Name:          "search_web",
		ToolName:      "search_web",
		ToolArguments: `{"query":"coze studio"}`,
		Final:         true,
	}, state)

	require.NoError(t, err)
	require.Equal(t, `{"answer":"Coze"}`, result.Message)
	require.True(t, result.Final)
	require.Contains(t, result.Metadata, `"source":"tool_runner"`)
	require.Contains(t, result.Metadata, `"tool_name":"search_web"`)
	require.Contains(t, result.Metadata, `"latency_ms":25`)
	require.Equal(t, "search_web", gotCall.Name)
	require.Equal(t, `{"query":"coze studio"}`, gotCall.Arguments)
	require.Equal(t, run, gotCall.Run)
	require.Equal(t, 2, gotCall.State.StepIndex)
}

func TestToolStepRunnerRejectsMissingTool(t *testing.T) {
	runner := NewToolStepRunner(NewInMemoryToolRegistry())

	result, err := runner.RunStep(context.Background(), &RunSummary{RunID: 11}, AgentStep{
		ID:       "tool-1",
		Type:     AgentStepTypeTool,
		Name:     "search_web",
		ToolName: "search_web",
	}, AgentHarnessState{})

	require.ErrorContains(t, err, "unsupported agent tool: search_web")
	require.Nil(t, result)
}
