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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioStaticPolicyAllowsExplicitCommandWorkdirAndEnv(
	t *testing.T,
) {
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{
			AllowedCommands:           []string{"node", "npx"},
			AllowedWorkingDirPrefixes: []string{"/mnt/coze/mcp"},
			AllowedEnvKeys:            []string{"API_TOKEN", "MCP_MODE"},
			MaxArgs:                   4,
			MaxArgBytes:               64,
			MaxEnvVars:                2,
			MaxEnvValueBytes:          32,
			RequireWorkingDir:         true,
		},
	)

	err := policy.ValidateADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.NoError(t, err)
}

func TestADKMCPRuntimeStdioStaticPolicyDeniesByDefault(t *testing.T) {
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{},
	)

	err := policy.ValidateADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp runtime stdio command is not allowed")
	assertADKMCPStdioPolicyErrorDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeStdioStaticPolicyRejectsUnsafeInputsSafely(t *testing.T) {
	tests := []struct {
		name           string
		allowedEnvKeys []string
		mutate         func(*ADKMCPRuntimeStdioSandboxCall)
		errContains    string
	}{
		{
			name: "command not allowed",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Command = "python"
			},
			errContains: "mcp runtime stdio command is not allowed",
		},
		{
			name: "missing required workdir",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.WorkingDir = ""
			},
			errContains: "mcp runtime stdio working directory is required",
		},
		{
			name: "relative workdir",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.WorkingDir = "relative/path"
			},
			errContains: "mcp runtime stdio working directory is not allowed",
		},
		{
			name: "workdir outside prefix",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.WorkingDir = "/secret/workdir"
			},
			errContains: "mcp runtime stdio working directory is not allowed",
		},
		{
			name: "workdir prefix escape",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.WorkingDir = "/mnt/coze/mcp-secret"
			},
			errContains: "mcp runtime stdio working directory is not allowed",
		},
		{
			name: "arg count budget",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Args = []string{"-y", "a", "b", "c", "d"}
			},
			errContains: "mcp runtime stdio args exceed budget",
		},
		{
			name: "arg byte budget",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Args = []string{"-y", strings.Repeat("a", 65)}
			},
			errContains: "mcp runtime stdio args exceed budget",
		},
		{
			name: "env key not allowed",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Env["SECRET_KEY"] = "stdio-secret-token"
			},
			errContains: "mcp runtime stdio env is not allowed",
		},
		{
			name: "env count budget",
			allowedEnvKeys: []string{
				"API_TOKEN",
				"MCP_MODE",
				"EXTRA",
			},
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Env["EXTRA"] = "x"
			},
			errContains: "mcp runtime stdio env exceeds budget",
		},
		{
			name: "env value budget",
			mutate: func(call *ADKMCPRuntimeStdioSandboxCall) {
				call.Config.Env["API_TOKEN"] = strings.Repeat("s", 33)
			},
			errContains: "mcp runtime stdio env exceeds budget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowedEnvKeys := []string{"API_TOKEN", "MCP_MODE"}
			if len(tt.allowedEnvKeys) > 0 {
				allowedEnvKeys = tt.allowedEnvKeys
			}
			policy := NewADKMCPRuntimeStdioStaticPolicy(
				ADKMCPRuntimeStdioStaticPolicyOptions{
					AllowedCommands:           []string{"npx"},
					AllowedWorkingDirPrefixes: []string{"/mnt/coze/mcp"},
					AllowedEnvKeys:            allowedEnvKeys,
					MaxArgs:                   4,
					MaxArgBytes:               64,
					MaxEnvVars:                2,
					MaxEnvValueBytes:          32,
					RequireWorkingDir:         true,
				},
			)
			call := validADKMCPRuntimeStdioPolicyCall()
			tt.mutate(&call)

			err := policy.ValidateADKMCPRuntimeStdio(context.Background(), call)

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errContains)
			assertADKMCPStdioPolicyErrorDoesNotLeak(t, err.Error())
		})
	}
}

func TestADKMCPRuntimeStdioTransportUsesStaticPolicyBeforeSandbox(t *testing.T) {
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{
			AllowedCommands:           []string{"node"},
			AllowedWorkingDirPrefixes: []string{"/mnt/coze/mcp"},
			AllowedEnvKeys:            []string{"API_TOKEN", "MCP_MODE"},
			MaxArgs:                   4,
			MaxArgBytes:               64,
			MaxEnvVars:                2,
			MaxEnvValueBytes:          32,
			RequireWorkingDir:         true,
		},
	)
	sandbox := &recordingADKMCPRuntimeStdioSandbox{
		result: "should-not-run",
	}
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{
			Policy:  policy,
			Sandbox: sandbox,
		},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio policy denied")
	require.Zero(t, sandbox.calls)
	assertADKMCPStdioPolicyErrorDoesNotLeak(t, err.Error())
}

func validADKMCPRuntimeStdioPolicyCall() ADKMCPRuntimeStdioSandboxCall {
	return ADKMCPRuntimeStdioSandboxCall{
		Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		Name:      "mcp_100_search_docs",
		ServerID:  100,
		ToolName:  "search-docs",
		Arguments: `{"query":"secret customer path"}`,
		Config: ADKMCPRuntimeStdioConfig{
			Command:    "npx",
			Args:       []string{"-y", "@example/secret-mcp-server"},
			WorkingDir: "/mnt/coze/mcp/run-20",
			Env: map[string]string{
				"API_TOKEN": "stdio-secret-token",
				"MCP_MODE":  "readonly",
			},
		},
	}
}

func assertADKMCPStdioPolicyErrorDoesNotLeak(t *testing.T, text string) {
	t.Helper()
	assertADKMCPStdioErrorDoesNotLeak(t, text)
	require.NotContains(t, text, "python")
	require.NotContains(t, text, "relative/path")
	require.NotContains(t, text, "/mnt/coze/mcp-secret")
	require.NotContains(t, text, "SECRET_KEY")
}
