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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioSandboxFailsClosedWithoutRunner(t *testing.T) {
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{},
	)

	result, err := sandbox.InvokeADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio runner is not configured")
	assertADKMCPStdioSandboxErrorDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeStdioSandboxProjectsExecutionForRunner(t *testing.T) {
	runner := &recordingADKMCPRuntimeStdioSandboxRunner{
		result: `{"schema":"coze.mcp_stdio_runner_result.v1","content":"ok"}`,
	}
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{Runner: runner},
	)

	result, err := sandbox.InvokeADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.NoError(t, err)
	require.Equal(t, runner.result, result)
	require.Equal(t, 1, runner.calls)
	require.Equal(t, int64(20), runner.execution.Run.RunID)
	require.Equal(t, int64(100), runner.execution.ServerID)
	require.Equal(t, "mcp_100_search_docs", runner.execution.Name)
	require.Equal(t, "search-docs", runner.execution.ToolName)
	require.Equal(t, `{"query":"secret customer path"}`, runner.execution.Arguments)
	require.Equal(t, "npx", runner.execution.Command)
	require.Equal(t, []string{"-y", "@example/secret-mcp-server"}, runner.execution.Args)
	require.Equal(t, "/mnt/coze/mcp/run-20", runner.execution.WorkingDir)
	require.Equal(t, "stdio-secret-token", runner.execution.Env["API_TOKEN"])
	require.Equal(t, "readonly", runner.execution.Env["MCP_MODE"])
}

func TestADKMCPRuntimeStdioSandboxRejectsInvalidCallsBeforeRunner(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*ADKMCPRuntimeStdioSandboxCall)
		errContains string
	}{
		{
			name: "missing run",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Run = nil
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "missing server",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.ServerID = 0
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "unsafe name",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Name = "unsafe-name"
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "missing tool",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.ToolName = ""
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "invalid arguments",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Arguments = `{"query":`
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "missing command",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Command = ""
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "missing workdir",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.WorkingDir = ""
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
		{
			name: "relative workdir",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.WorkingDir = "relative/path"
			},
			errContains: "mcp runtime stdio sandbox call is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingADKMCPRuntimeStdioSandboxRunner{
				result: "should-not-run",
			}
			sandbox := NewADKMCPRuntimeStdioSandbox(
				ADKMCPRuntimeStdioSandboxOptions{Runner: runner},
			)
			call := validADKMCPRuntimeStdioPolicyCall()
			tt.mutate(&call)

			result, err := sandbox.InvokeADKMCPRuntimeStdio(
				context.Background(),
				call,
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), tt.errContains)
			require.Zero(t, runner.calls)
			assertADKMCPStdioSandboxErrorDoesNotLeak(t, err.Error())
		})
	}
}

func TestADKMCPRuntimeStdioSandboxSanitizesRunnerErrors(t *testing.T) {
	runner := &recordingADKMCPRuntimeStdioSandboxRunner{
		err: fmt.Errorf(
			`spawn npx @example/secret-mcp-server in /mnt/coze/mcp/run-20 with stdio-secret-token`,
		),
	}
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{Runner: runner},
	)

	result, err := sandbox.InvokeADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio runner failed")
	require.Equal(t, 1, runner.calls)
	assertADKMCPStdioSandboxErrorDoesNotLeak(t, err.Error())
}

type recordingADKMCPRuntimeStdioSandboxRunner struct {
	execution ADKMCPRuntimeStdioSandboxExecution
	calls     int
	result    string
	err       error
	order     *[]string
}

func (r *recordingADKMCPRuntimeStdioSandboxRunner) RunADKMCPRuntimeStdio(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (string, error) {
	r.calls++
	r.execution = execution
	if r.order != nil {
		*r.order = append(*r.order, "run")
	}
	if r.err != nil {
		return "", r.err
	}

	return r.result, nil
}

func assertADKMCPStdioSandboxErrorDoesNotLeak(t *testing.T, text string) {
	t.Helper()
	assertADKMCPStdioPolicyErrorDoesNotLeak(t, text)
	require.NotContains(t, text, "spawn")
}
