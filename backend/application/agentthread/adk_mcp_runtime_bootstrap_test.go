/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestADKMCPRuntimeBootstrapConfigFromEnvDefaultsDisabled(t *testing.T) {
	clearADKMCPRuntimeBootstrapEnv(t)

	config, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.NoError(t, err)
	require.False(t, config.Enabled)
	require.False(t, config.StdioDryRunEnabled)
	require.Equal(t, 30*time.Second, config.ExecutorTimeout)
	require.Equal(t, defaultADKMCPRuntimeExecutorMaxOutputBytes, config.ExecutorMaxOutputBytes)
}

func TestADKMCPRuntimeBootstrapConfigFromEnvParsesDryRunStdio(t *testing.T) {
	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioDryRunEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirRootEnv, "/tmp/coze-mcp")
	t.Setenv(agentThreadMCPStdioWorkerIDEnv, "worker-env")
	t.Setenv(agentThreadMCPStdioWorkerIDEnv, "worker-env")
	t.Setenv(agentThreadMCPStdioAllowedCommandsEnv, "npx, node")
	t.Setenv(agentThreadMCPStdioAllowedEnvKeysEnv, "API_TOKEN,WORKSPACE")
	t.Setenv(agentThreadMCPStdioMaxArgsEnv, "8")
	t.Setenv(agentThreadMCPStdioMaxArgBytesEnv, "256")
	t.Setenv(agentThreadMCPStdioMaxEnvVarsEnv, "4")
	t.Setenv(agentThreadMCPStdioMaxEnvValueBytesEnv, "128")
	t.Setenv(agentThreadMCPStdioLeaseTTLMsEnv, "120000")
	t.Setenv(agentThreadMCPStdioMaxConfigBytesEnv, "4096")
	t.Setenv(agentThreadMCPStdioDryRunOutputBytesEnv, "2048")
	t.Setenv(agentThreadMCPRuntimeTimeoutMsEnv, "15000")
	t.Setenv(agentThreadMCPRuntimeMaxOutputBytesEnv, "8192")

	config, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.NoError(t, err)
	require.True(t, config.Enabled)
	require.True(t, config.StdioDryRunEnabled)
	require.Equal(t, "/tmp/coze-mcp", config.StdioWorkdirRoot)
	require.Equal(t, "worker-env", config.StdioWorkerID)
	require.Equal(t, []string{"npx", "node"}, config.StdioAllowedCommands)
	require.Equal(t, []string{"API_TOKEN", "WORKSPACE"}, config.StdioAllowedEnvKeys)
	require.Equal(t, 8, config.StdioMaxArgs)
	require.Equal(t, 256, config.StdioMaxArgBytes)
	require.Equal(t, 4, config.StdioMaxEnvVars)
	require.Equal(t, 128, config.StdioMaxEnvValueBytes)
	require.Equal(t, int64(120000), config.StdioLeaseTTLMillis)
	require.Equal(t, 4096, config.StdioMaxConfigBytes)
	require.Equal(t, 2048, config.StdioDryRunOutputBytes)
	require.Equal(t, 15*time.Second, config.ExecutorTimeout)
	require.Equal(t, 8192, config.ExecutorMaxOutputBytes)
}

func TestADKMCPRuntimeBootstrapConfigFromEnvParsesEinoStdio(t *testing.T) {
	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioEinoEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirRootEnv, "/tmp/coze-mcp")
	t.Setenv(agentThreadMCPStdioWorkerIDEnv, "worker-env")
	t.Setenv(agentThreadMCPStdioAllowedCommandsEnv, "npx")
	t.Setenv(agentThreadMCPRuntimeMaxOutputBytesEnv, "8192")

	config, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.NoError(t, err)
	require.True(t, config.Enabled)
	require.False(t, config.StdioDryRunEnabled)
	require.True(t, config.StdioEinoEnabled)
	require.Equal(t, "/tmp/coze-mcp", config.StdioWorkdirRoot)
	require.Equal(t, "worker-env", config.StdioWorkerID)
	require.Equal(t, []string{"npx"}, config.StdioAllowedCommands)
	require.Equal(t, 8192, config.ExecutorMaxOutputBytes)
}

func TestADKMCPRuntimeBootstrapConfigFromEnvParsesRemoteEino(t *testing.T) {
	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPRemoteEinoEnabledEnv, "true")
	t.Setenv(agentThreadMCPRemoteAllowedHostsEnv, "mcp.example.test,localhost:3030")
	t.Setenv(agentThreadMCPRemoteAllowInsecureHTTPEnv, "true")
	t.Setenv(agentThreadMCPRemoteMaxConfigBytesEnv, "8192")
	t.Setenv(agentThreadMCPRemoteMaxHeadersEnv, "8")
	t.Setenv(agentThreadMCPRemoteMaxHeaderBytesEnv, "2048")

	config, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.NoError(t, err)
	require.True(t, config.Enabled)
	require.True(t, config.RemoteEinoEnabled)
	require.Equal(
		t,
		[]string{"mcp.example.test", "localhost:3030"},
		config.RemoteAllowedHosts,
	)
	require.True(t, config.RemoteAllowInsecureHTTP)
	require.Equal(t, 8192, config.RemoteMaxConfigBytes)
	require.Equal(t, 8, config.RemoteMaxHeaders)
	require.Equal(t, 2048, config.RemoteMaxHeaderBytes)
}

func TestADKMCPRuntimeBootstrapConfigFromEnvRejectsIncompleteRemote(
	t *testing.T,
) {
	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPRemoteEinoEnabledEnv, "true")

	_, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), agentThreadMCPRemoteAllowedHostsEnv)
}

func TestADKMCPRuntimeBootstrapConfigFromEnvRejectsMultipleStdioModes(
	t *testing.T,
) {
	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioDryRunEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioEinoEnabledEnv, "true")

	_, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), agentThreadMCPStdioEinoEnabledEnv)
	require.Contains(t, err.Error(), agentThreadMCPStdioDryRunEnabledEnv)
}

func TestADKMCPRuntimeBootstrapConfigFromEnvRejectsIncompleteDryRun(
	t *testing.T,
) {
	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioDryRunEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirRootEnv, "/tmp/coze-mcp")
	t.Setenv(agentThreadMCPStdioWorkerIDEnv, "worker-env")

	_, err := ADKMCPRuntimeBootstrapConfigFromEnv()

	require.Error(t, err)
	require.Contains(t, err.Error(), agentThreadMCPStdioAllowedCommandsEnv)
	require.NotContains(t, err.Error(), "/tmp/coze-mcp")
}

func TestADKMCPRuntimeBootstrapConfigFromEnvRejectsInvalidEnvValues(
	t *testing.T,
) {
	tests := []struct {
		name        string
		key         string
		value       string
		errContains string
	}{
		{
			name:        "invalid bool",
			key:         agentThreadMCPRuntimeEnabledEnv,
			value:       "sometimes",
			errContains: agentThreadMCPRuntimeEnabledEnv,
		},
		{
			name:        "invalid integer",
			key:         agentThreadMCPRuntimeMaxOutputBytesEnv,
			value:       "many",
			errContains: agentThreadMCPRuntimeMaxOutputBytesEnv,
		},
		{
			name:        "negative budget",
			key:         agentThreadMCPStdioMaxArgBytesEnv,
			value:       "-1",
			errContains: agentThreadMCPStdioMaxArgBytesEnv,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearADKMCPRuntimeBootstrapEnv(t)
			t.Setenv(tt.key, tt.value)

			_, err := ADKMCPRuntimeBootstrapConfigFromEnv()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestNewADKMCPRuntimeToolExecutorFromConfigReturnsNilWhenDisabled(
	t *testing.T,
) {
	executor := NewADKMCPRuntimeToolExecutorFromConfig(
		ADKMCPRuntimeBootstrapDependencies{
			Config: ADKMCPRuntimeBootstrapConfig{},
		},
	)

	require.Nil(t, executor)
}

func TestNewADKMCPRuntimeToolExecutorFromConfigInvokesDryRunStdio(
	t *testing.T,
) {
	root := t.TempDir()
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		finishedLease: &domainentity.MCPRuntimeWorkdirLease{
			ID:     9101,
			Status: domainentity.MCPRuntimeWorkdirLeaseStatusReleased,
		},
		finishOK: true,
	}
	server := &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    30,
		Name:       "docs-mcp",
		Enabled:    true,
		ServerType: "stdio",
		Config:     `{"command":"npx","args":["mcp-server"],"env":{"API_TOKEN":"stdio-secret-token"}}`,
		Auth:       `{"token":"raw-secret"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search-docs", Description: "Search docs."},
		},
	}
	audit := &recordingADKMCPRuntimeAuditRecorder{}
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeToolExecutorFromConfig(
		ADKMCPRuntimeBootstrapDependencies{
			Resolver:        &recordingADKMCPRuntimeServerResolver{server: server},
			LeaseRepository: repo,
			IDGen:           &mcpWorkdirLeaseSequenceIDGen{next: 9101},
			EventSink:       &recordingRunEventSink{},
			AuditRecorder:   audit,
			HealthReporter:  health,
			Config: ADKMCPRuntimeBootstrapConfig{
				Enabled:                true,
				StdioDryRunEnabled:     true,
				StdioWorkdirRoot:       root,
				StdioWorkerID:          "worker-env",
				StdioAllowedCommands:   []string{"npx"},
				StdioAllowedEnvKeys:    []string{"API_TOKEN"},
				StdioMaxArgs:           4,
				StdioMaxArgBytes:       128,
				StdioMaxEnvVars:        1,
				StdioMaxEnvValueBytes:  64,
				StdioLeaseTTLMillis:    120000,
				StdioDryRunOutputBytes: 4096,
				ExecutorTimeout:        50 * time.Millisecond,
				ExecutorMaxOutputBytes: 4096,
				nowMillis:              func() int64 { return 1000 },
			},
		},
	)
	require.NotNil(t, executor)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret customer path"}`,
		},
	)

	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &payload))
	require.Equal(t, "coze.mcp_stdio_dry_run.v1", payload["schema"])
	require.Equal(t, "dry_run", payload["status"])
	require.NotContains(t, result, "secret customer path")
	require.NotContains(t, result, "stdio-secret-token")
	require.NotContains(t, result, "raw-secret")
	require.NotContains(t, result, root)

	require.Equal(t, 1, repo.createCalls)
	require.True(t, adkMCPRuntimePathWithin(
		filepath.Clean(repo.createdLease.Workdir),
		filepath.Clean(root),
	))
	require.Equal(t, "worker-env", repo.createdLease.WorkerID)
	require.Equal(t, int64(121000), repo.createdLease.LeaseExpiresAt)
	require.Equal(t, 1, repo.finishCalls)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusReleased, repo.finishReq.Status)
	require.Len(t, audit.records, 2)
	require.Equal(t, "mcp.tool.started", audit.records[0].EventType)
	require.Equal(t, "mcp.tool.completed", audit.records[1].EventType)
	require.Len(t, health.reports, 1)
	require.True(t, health.reports[0].Success)
	require.Equal(t, int64(100), health.reports[0].ServerID)
}

func TestNewADKMCPRuntimeToolExecutorFromConfigInjectsOutputOffloader(
	t *testing.T,
) {
	root := t.TempDir()
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		finishedLease: &domainentity.MCPRuntimeWorkdirLease{
			ID:     9101,
			Status: domainentity.MCPRuntimeWorkdirLeaseStatusReleased,
		},
		finishOK: true,
	}
	server := &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    30,
		Name:       "docs-mcp",
		Enabled:    true,
		ServerType: "stdio",
		Config:     `{"command":"npx","args":["mcp-server"]}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search-docs", Description: "Search docs."},
		},
	}
	offloader := &recordingADKMCPRuntimeOutputOffloader{
		result: ADKMCPRuntimeOutputOffloadResult{
			Notice: `{"offloaded":true}`,
		},
	}
	executor := NewADKMCPRuntimeToolExecutorFromConfig(
		ADKMCPRuntimeBootstrapDependencies{
			Resolver:        &recordingADKMCPRuntimeServerResolver{server: server},
			LeaseRepository: repo,
			IDGen:           &mcpWorkdirLeaseSequenceIDGen{next: 9101},
			OutputOffloader: offloader,
			Config: ADKMCPRuntimeBootstrapConfig{
				Enabled:                true,
				StdioDryRunEnabled:     true,
				StdioWorkdirRoot:       root,
				StdioWorkerID:          "worker-env",
				StdioAllowedCommands:   []string{"npx"},
				StdioMaxArgs:           4,
				StdioMaxArgBytes:       128,
				StdioMaxEnvVars:        0,
				StdioMaxEnvValueBytes:  64,
				StdioLeaseTTLMillis:    120000,
				StdioDryRunOutputBytes: 4096,
				ExecutorTimeout:        50 * time.Millisecond,
				ExecutorMaxOutputBytes: 64,
				nowMillis:              func() int64 { return 1000 },
			},
		},
	)
	require.NotNil(t, executor)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"docs"}`,
		},
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"offloaded":true}`, result)
	require.Contains(t, offloader.request.Content, "coze.mcp_stdio_dry_run.v1")
}

func TestNewADKMCPRuntimeToolExecutorFromConfigBuildsEinoStdio(
	t *testing.T,
) {
	root := t.TempDir()
	executor := NewADKMCPRuntimeToolExecutorFromConfig(
		ADKMCPRuntimeBootstrapDependencies{
			Resolver: &recordingADKMCPRuntimeServerResolver{},
			LeaseRepository: &recordingMCPRuntimeWorkdirLeaseRepository{
				finishedLease: &domainentity.MCPRuntimeWorkdirLease{
					ID:     9101,
					Status: domainentity.MCPRuntimeWorkdirLeaseStatusReleased,
				},
				finishOK: true,
			},
			IDGen: &mcpWorkdirLeaseSequenceIDGen{next: 9101},
			Config: ADKMCPRuntimeBootstrapConfig{
				Enabled:                true,
				StdioEinoEnabled:       true,
				StdioWorkdirRoot:       root,
				StdioWorkerID:          "worker-env",
				StdioAllowedCommands:   []string{"npx"},
				StdioLeaseTTLMillis:    120000,
				ExecutorTimeout:        50 * time.Millisecond,
				ExecutorMaxOutputBytes: 4096,
			},
		},
	)

	require.NotNil(t, executor)
}

func TestNewADKMCPRuntimeToolExecutorFromConfigBuildsRemoteHTTP(
	t *testing.T,
) {
	executor := NewADKMCPRuntimeToolExecutorFromConfig(
		ADKMCPRuntimeBootstrapDependencies{
			Resolver: &recordingADKMCPRuntimeServerResolver{},
			Config: ADKMCPRuntimeBootstrapConfig{
				Enabled:                 true,
				RemoteEinoEnabled:       true,
				RemoteAllowedHosts:      []string{"mcp.example.test"},
				RemoteMaxConfigBytes:    8192,
				RemoteMaxHeaders:        8,
				RemoteMaxHeaderBytes:    2048,
				ExecutorTimeout:         50 * time.Millisecond,
				ExecutorMaxOutputBytes:  4096,
				StdioLeaseTTLMillis:     120000,
				StdioMaxArgBytes:        128,
				StdioMaxEnvValueBytes:   128,
				StdioDryRunOutputBytes:  4096,
				StdioMaxConfigBytes:     4096,
				StdioMaxArgs:            4,
				StdioMaxEnvVars:         4,
				RemoteAllowInsecureHTTP: false,
			},
		},
	)

	require.NotNil(t, executor)
}

func clearADKMCPRuntimeBootstrapEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		agentThreadMCPRuntimeEnabledEnv,
		agentThreadMCPRuntimeTimeoutMsEnv,
		agentThreadMCPRuntimeMaxOutputBytesEnv,
		agentThreadMCPStdioDryRunEnabledEnv,
		agentThreadMCPStdioEinoEnabledEnv,
		agentThreadMCPStdioWorkdirRootEnv,
		agentThreadMCPStdioWorkerIDEnv,
		agentThreadMCPStdioAllowedCommandsEnv,
		agentThreadMCPStdioAllowedEnvKeysEnv,
		agentThreadMCPStdioMaxArgsEnv,
		agentThreadMCPStdioMaxArgBytesEnv,
		agentThreadMCPStdioMaxEnvVarsEnv,
		agentThreadMCPStdioMaxEnvValueBytesEnv,
		agentThreadMCPStdioLeaseTTLMsEnv,
		agentThreadMCPStdioMaxConfigBytesEnv,
		agentThreadMCPStdioDryRunOutputBytesEnv,
		agentThreadMCPRemoteEinoEnabledEnv,
		agentThreadMCPRemoteAllowedHostsEnv,
		agentThreadMCPRemoteAllowInsecureHTTPEnv,
		agentThreadMCPRemoteMaxConfigBytesEnv,
		agentThreadMCPRemoteMaxHeadersEnv,
		agentThreadMCPRemoteMaxHeaderBytesEnv,
	} {
		t.Setenv(key, "")
	}
}
