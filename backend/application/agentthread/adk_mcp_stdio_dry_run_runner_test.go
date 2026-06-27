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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioDryRunRunnerReturnsSafeMetadata(t *testing.T) {
	root := t.TempDir()
	runner := NewADKMCPRuntimeStdioDryRunRunner(
		ADKMCPRuntimeStdioDryRunRunnerOptions{},
	)

	result, err := runner.RunADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioSandboxExecution(root),
	)

	require.NoError(t, err)
	payload := decodeADKMCPRuntimeStdioDryRunResult(t, result)
	require.Equal(t, "coze.mcp_stdio_dry_run.v1", payload.Schema)
	require.Equal(t, "dry_run", payload.Status)
	require.Equal(t, "mcp_100_search_docs", payload.RuntimeToolName)
	require.Equal(t, int64(100), payload.ServerID)
	require.True(t, payload.CommandConfigured)
	require.Equal(t, 2, payload.ArgsCount)
	require.Equal(t, 1, payload.EnvCount)
	require.True(t, payload.WorkingDirConfigured)
	assertADKMCPStdioDryRunResultDoesNotLeak(t, result, root)
}

func TestADKMCPRuntimeStdioDryRunRunnerRejectsInvalidExecutionSafely(
	t *testing.T,
) {
	runner := NewADKMCPRuntimeStdioDryRunRunner(
		ADKMCPRuntimeStdioDryRunRunnerOptions{},
	)
	execution := validADKMCPRuntimeStdioSandboxExecution(t.TempDir())
	execution.Run = nil

	result, err := runner.RunADKMCPRuntimeStdio(
		context.Background(),
		execution,
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio dry-run execution is invalid")
	assertADKMCPStdioDryRunResultDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeStdioDryRunRunnerFullTransportChain(t *testing.T) {
	root := t.TempDir()
	manager := NewADKMCPRuntimeStdioWorkdirManager(
		ADKMCPRuntimeStdioWorkdirManagerOptions{Root: root},
	)
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{
			AllowedCommands:           []string{"npx"},
			AllowedWorkingDirPrefixes: []string{root},
			AllowedEnvKeys:            []string{"API_TOKEN"},
			MaxArgs:                   4,
			MaxArgBytes:               96,
			MaxEnvVars:                1,
			MaxEnvValueBytes:          32,
			RequireWorkingDir:         true,
		},
	)
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{
			Runner: NewADKMCPRuntimeStdioDryRunRunner(
				ADKMCPRuntimeStdioDryRunRunnerOptions{},
			),
			WorkdirPreparer: NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
				ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{
					Root: root,
				},
			),
		},
	)
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{
			Policy:         policy,
			Sandbox:        sandbox,
			WorkdirManager: manager,
		},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.NoError(t, err)
	payload := decodeADKMCPRuntimeStdioDryRunResult(t, result)
	require.Equal(t, "coze.mcp_stdio_dry_run.v1", payload.Schema)
	require.Equal(t, "dry_run", payload.Status)
	require.Equal(t, "mcp_100_search_docs", payload.RuntimeToolName)
	require.Equal(t, int64(100), payload.ServerID)
	require.True(t, payload.CommandConfigured)
	require.Equal(t, 2, payload.ArgsCount)
	require.Equal(t, 1, payload.EnvCount)
	require.True(t, payload.WorkingDirConfigured)
	assertADKMCPStdioDryRunResultDoesNotLeak(t, result, root)

	projectedWorkdir := filepath.Join(
		root,
		"spaces",
		"30",
		"threads",
		"10",
		"runs",
		"20",
		"servers",
		"100",
		"tools",
		"mcp_100_search_docs",
	)
	_, statErr := os.Stat(projectedWorkdir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

type adkMCPRuntimeStdioDryRunTestResult struct {
	Schema               string `json:"schema"`
	Status               string `json:"status"`
	RuntimeToolName      string `json:"runtime_tool_name"`
	ServerID             int64  `json:"server_id"`
	CommandConfigured    bool   `json:"command_configured"`
	ArgsCount            int    `json:"args_count"`
	EnvCount             int    `json:"env_count"`
	WorkingDirConfigured bool   `json:"working_dir_configured"`
}

func decodeADKMCPRuntimeStdioDryRunResult(
	t *testing.T,
	result string,
) adkMCPRuntimeStdioDryRunTestResult {
	t.Helper()
	var payload adkMCPRuntimeStdioDryRunTestResult
	require.NoError(t, json.Unmarshal([]byte(result), &payload))

	return payload
}

func assertADKMCPStdioDryRunResultDoesNotLeak(
	t *testing.T,
	text string,
	values ...string,
) {
	t.Helper()
	assertADKMCPStdioSandboxErrorDoesNotLeak(t, text)
	require.NotContains(t, text, "search-docs")
	for _, value := range values {
		require.NotContains(t, text, value)
	}
}
