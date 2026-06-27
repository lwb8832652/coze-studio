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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioWorkdirManagerProjectsDeterministicPath(
	t *testing.T,
) {
	manager := NewADKMCPRuntimeStdioWorkdirManager(
		ADKMCPRuntimeStdioWorkdirManagerOptions{
			Root: "/mnt/coze/mcp",
		},
	)

	projection, err := manager.ProjectADKMCPRuntimeStdioWorkdir(
		context.Background(),
		sampleADKMCPRuntimeStdioWorkdirRequest(),
	)

	require.NoError(t, err)
	require.Equal(t,
		filepath.Clean(
			"/mnt/coze/mcp/spaces/30/threads/10/runs/20/servers/100/tools/mcp_100_search_docs",
		),
		projection.WorkingDir,
	)
	require.Equal(t, filepath.Clean("/mnt/coze/mcp"), projection.Root)
}

func TestADKMCPRuntimeStdioWorkdirManagerRejectsUnsafeInputSafely(
	t *testing.T,
) {
	tests := []struct {
		name   string
		root   string
		mutate func(*ADKMCPRuntimeStdioWorkdirRequest)
	}{
		{
			name: "empty root",
			root: "",
		},
		{
			name: "relative root",
			root: "relative/root",
		},
		{
			name: "missing run",
			root: "/mnt/coze/mcp",
			mutate: func(request *ADKMCPRuntimeStdioWorkdirRequest) {
				request.Run = nil
			},
		},
		{
			name: "missing server",
			root: "/mnt/coze/mcp",
			mutate: func(request *ADKMCPRuntimeStdioWorkdirRequest) {
				request.ServerID = 0
			},
		},
		{
			name: "unsafe runtime name",
			root: "/mnt/coze/mcp",
			mutate: func(request *ADKMCPRuntimeStdioWorkdirRequest) {
				request.Name = "unsafe-name"
			},
		},
		{
			name: "missing raw tool",
			root: "/mnt/coze/mcp",
			mutate: func(request *ADKMCPRuntimeStdioWorkdirRequest) {
				request.ToolName = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewADKMCPRuntimeStdioWorkdirManager(
				ADKMCPRuntimeStdioWorkdirManagerOptions{Root: tt.root},
			)
			request := sampleADKMCPRuntimeStdioWorkdirRequest()
			if tt.mutate != nil {
				tt.mutate(&request)
			}

			projection, err := manager.ProjectADKMCPRuntimeStdioWorkdir(
				context.Background(),
				request,
			)

			require.Error(t, err)
			require.Empty(t, projection.WorkingDir)
			require.Contains(t, err.Error(), "mcp runtime stdio workdir is invalid")
			assertADKMCPStdioSandboxErrorDoesNotLeak(t, err.Error())
		})
	}
}

func TestADKMCPRuntimeStdioTransportProjectsWorkdirBeforePolicy(t *testing.T) {
	manager := NewADKMCPRuntimeStdioWorkdirManager(
		ADKMCPRuntimeStdioWorkdirManagerOptions{
			Root: "/mnt/coze/mcp",
		},
	)
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{
			AllowedCommands:           []string{"npx"},
			AllowedWorkingDirPrefixes: []string{"/mnt/coze/mcp"},
			AllowedEnvKeys:            []string{"API_TOKEN"},
			MaxArgs:                   4,
			MaxArgBytes:               96,
			MaxEnvVars:                1,
			MaxEnvValueBytes:          32,
			RequireWorkingDir:         true,
		},
	)
	sandbox := &recordingADKMCPRuntimeStdioSandbox{
		result: `{"schema":"coze.mcp_stdio_result.v1","content":"ok"}`,
	}
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{
			Policy:         policy,
			Sandbox:        sandbox,
			WorkdirManager: manager,
		},
	)
	call := validADKMCPRuntimeStdioCall()
	require.Contains(t, call.Server.Config, "/secret/workdir")

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		call,
	)

	require.NoError(t, err)
	require.Equal(t, sandbox.result, result)
	require.Equal(t, 1, sandbox.calls)
	require.Equal(t,
		filepath.Clean(
			"/mnt/coze/mcp/spaces/30/threads/10/runs/20/servers/100/tools/mcp_100_search_docs",
		),
		sandbox.call.Config.WorkingDir,
	)
}

func sampleADKMCPRuntimeStdioWorkdirRequest() ADKMCPRuntimeStdioWorkdirRequest {
	return ADKMCPRuntimeStdioWorkdirRequest{
		Run:      &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		Name:     "mcp_100_search_docs",
		ServerID: 100,
		ToolName: "search-docs",
	}
}
