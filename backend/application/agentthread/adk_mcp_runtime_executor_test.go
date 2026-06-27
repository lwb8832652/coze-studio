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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestADKMCPRuntimeExecutorValidatesAndInvokesTransport(t *testing.T) {
	resolver := &recordingADKMCPRuntimeServerResolver{
		server: &toolapi.MCPToolServer{
			ServerID:   100,
			SpaceID:    30,
			Name:       "docs-mcp",
			Enabled:    true,
			Config:     `{"command":"npx"}`,
			Auth:       `{"token":"raw-secret"}`,
			ServerType: "stdio",
			Tools: []*toolapi.MCPToolDefinition{
				{Name: "search-docs", Description: "Search docs."},
			},
		},
	}
	transport := &recordingADKMCPRuntimeTransport{result: "bounded output"}
	events := &recordingRunEventSink{}
	executor := NewADKMCPRuntimeExecutor(
		resolver,
		transport,
		WithADKMCPRuntimeExecutorTimeout(50*time.Millisecond),
		WithADKMCPRuntimeExecutorMaxOutputBytes(64),
		WithADKMCPRuntimeExecutorEventSink(events),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, "bounded output", result)
	require.Equal(t, int64(100), resolver.serverID)
	require.Equal(t, int64(100), transport.call.Server.ServerID)
	require.Equal(t, "search-docs", transport.call.ToolName)
	require.Equal(t, `{"query":"secret docs"}`, transport.call.Arguments)
	require.True(t, transport.deadlineSet)
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.completed"}, events.eventTypes())
	for _, event := range events.events {
		require.NotContains(t, event.Payload, "secret docs")
		require.NotContains(t, event.Payload, "raw-secret")
		require.NotContains(t, event.Payload, "docs-mcp")
	}
}

func TestADKMCPRuntimeExecutorRejectsUnsafeCallsBeforeTransport(t *testing.T) {
	tests := []struct {
		name        string
		server      *toolapi.MCPToolServer
		call        ADKMCPRuntimeToolCall
		errContains string
	}{
		{
			name: "space mismatch",
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  999,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
			call: ADKMCPRuntimeToolCall{
				Run:       &RunSummary{RunID: 20, SpaceID: 30},
				Name:      "mcp_100_search_docs",
				ServerID:  100,
				ToolName:  "search-docs",
				Arguments: `{"query":"secret docs"}`,
			},
			errContains: "space mismatch",
		},
		{
			name: "disabled server",
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  false,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
			call: ADKMCPRuntimeToolCall{
				Run:       &RunSummary{RunID: 20, SpaceID: 30},
				Name:      "mcp_100_search_docs",
				ServerID:  100,
				ToolName:  "search-docs",
				Arguments: `{"query":"secret docs"}`,
			},
			errContains: "server disabled",
		},
		{
			name: "tool missing",
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "other-tool"}},
			},
			call: ADKMCPRuntimeToolCall{
				Run:       &RunSummary{RunID: 20, SpaceID: 30},
				Name:      "mcp_100_search_docs",
				ServerID:  100,
				ToolName:  "search-docs",
				Arguments: `{"query":"secret docs"}`,
			},
			errContains: "tool is not configured",
		},
		{
			name: "invalid arguments",
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
			call: ADKMCPRuntimeToolCall{
				Run:       &RunSummary{RunID: 20, SpaceID: 30},
				Name:      "mcp_100_search_docs",
				ServerID:  100,
				ToolName:  "search-docs",
				Arguments: `{"query":"secret docs"`,
			},
			errContains: "arguments must be valid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingADKMCPRuntimeTransport{}
			events := &recordingRunEventSink{}
			executor := NewADKMCPRuntimeExecutor(
				&recordingADKMCPRuntimeServerResolver{server: tt.server},
				transport,
				WithADKMCPRuntimeExecutorEventSink(events),
			)

			result, err := executor.InvokeADKMCPRuntimeTool(
				context.Background(),
				tt.call,
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), tt.errContains)
			require.NotContains(t, err.Error(), "secret docs")
			require.Zero(t, transport.calls)
			require.Equal(t, []string{"mcp.tool.started", "mcp.tool.failed"}, events.eventTypes())
			for _, event := range events.events {
				require.NotContains(t, event.Payload, "secret docs")
			}
		})
	}
}

func TestADKMCPRuntimeExecutorRejectsOversizedOutput(t *testing.T) {
	transport := &recordingADKMCPRuntimeTransport{
		result: "safe prefix " + strings.Repeat("x", 32),
	}
	events := &recordingRunEventSink{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		transport,
		WithADKMCPRuntimeExecutorMaxOutputBytes(8),
		WithADKMCPRuntimeExecutorEventSink(events),
	)

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

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "output exceeds budget")
	require.NotContains(t, err.Error(), "safe prefix")
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.failed"}, events.eventTypes())
	for _, event := range events.events {
		require.NotContains(t, event.Payload, "safe prefix")
	}
}

func TestADKMCPRuntimeExecutorOffloadsOversizedOutput(t *testing.T) {
	rawOutput := "secret output " + strings.Repeat("x", 512)
	offloader := &recordingADKMCPRuntimeOutputOffloader{
		result: ADKMCPRuntimeOutputOffloadResult{
			Notice: fmt.Sprintf(
				`{"schema":"coze.mcp_runtime_output_offload.v1","offloaded":true,"virtual_path":"/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.txt","output_bytes":%d}`,
				len(rawOutput),
			),
		},
	}
	events := &recordingRunEventSink{}
	audit := &recordingADKMCPRuntimeAuditRecorder{}
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{result: rawOutput},
		WithADKMCPRuntimeExecutorMaxOutputBytes(256),
		WithADKMCPRuntimeExecutorEventSink(events),
		WithADKMCPRuntimeExecutorAuditRecorder(audit),
		WithADKMCPRuntimeExecutorHealthReporter(health),
		WithADKMCPRuntimeExecutorOutputOffloader(offloader),
	)

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
	require.JSONEq(t, offloader.result.Notice, result)
	require.Equal(t, rawOutput, offloader.request.Content)
	require.Equal(t, int64(100), offloader.request.ServerID)
	require.Equal(t, "mcp_100_search_docs", offloader.request.Name)
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.completed"}, events.eventTypes())
	require.Len(t, audit.records, 2)
	require.Equal(t, int64(len(rawOutput)), audit.records[1].OutputBytes)
	require.Len(t, health.reports, 1)
	require.True(t, health.reports[0].Success)
	for _, event := range events.events {
		require.NotContains(t, event.Payload, "secret output")
		require.NotContains(t, event.Payload, strings.Repeat("x", 8))
	}
}

func TestADKMCPRuntimeExecutorFailsClosedWhenOutputOffloadFails(
	t *testing.T,
) {
	rawOutput := "secret output " + strings.Repeat("x", 512)
	events := &recordingRunEventSink{}
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{result: rawOutput},
		WithADKMCPRuntimeExecutorMaxOutputBytes(256),
		WithADKMCPRuntimeExecutorEventSink(events),
		WithADKMCPRuntimeExecutorHealthReporter(health),
		WithADKMCPRuntimeExecutorOutputOffloader(
			&recordingADKMCPRuntimeOutputOffloader{
				err: fmt.Errorf("write failed %s", rawOutput),
			},
		),
	)

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

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "output offload failed")
	require.NotContains(t, err.Error(), "secret output")
	require.NotContains(t, err.Error(), strings.Repeat("x", 8))
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.failed"}, events.eventTypes())
	require.Len(t, health.reports, 1)
	require.False(t, health.reports[0].Success)
	require.Equal(t, "output_offload_failed", health.reports[0].ErrorCode)
	for _, event := range events.events {
		require.NotContains(t, event.Payload, "secret output")
	}
}

func TestADKMCPRuntimeOutputOffloadBackendAdapterWritesSafeNotice(
	t *testing.T,
) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	events := &recordingRunEventSink{}
	run := &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30}
	adapter := NewADKMCPRuntimeOutputOffloadBackendAdapter(
		ADKMCPRuntimeOutputOffloadBackendAdapterOptions{
			BackendFactory: ADKOffloadBackendFactoryFunc(func(
				ctx context.Context,
				run *RunSummary,
				limits ADKOffloadLimits,
			) (*ADKOffloadBackend, error) {
				return NewADKOffloadBackend(
					run,
					objectStorage,
					registry,
					events,
					limits,
				)
			}),
			Limits: ADKOffloadLimits{
				MaxOffloadBytes:  1024,
				DefaultReadBytes: 128,
				MaxReadBytes:     256,
			},
		},
	)

	result, err := adapter.OffloadADKMCPRuntimeOutput(
		context.Background(),
		ADKMCPRuntimeOutputOffloadRequest{
			Run:       run,
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Content:   "secret output body",
			StartedAt: time.Unix(100, 20),
		},
	)

	require.NoError(t, err)
	require.NotEmpty(t, result.Notice)
	require.NotEmpty(t, result.VirtualPath)
	require.Equal(t, len("secret output body"), result.OutputBytes)
	require.Len(t, registry.calls, 1)
	require.Equal(t, result.VirtualPath, registry.calls[0].VirtualPath)
	require.Equal(t, int64(len("secret output body")), registry.calls[0].SizeBytes)
	require.NotContains(t, result.Notice, "secret output body")
	require.NotContains(t, result.Notice, registry.calls[0].ObjectURI)
	require.Contains(t, result.Notice, `"schema":"coze.mcp_runtime_output_offload.v1"`)
	require.Contains(t, result.Notice, `"read_tool":"read_file"`)
	require.Equal(t, []string{"context.tool_result_offloaded"}, events.eventTypes())
	for _, value := range objectStorage.objects {
		require.Equal(t, "secret output body", string(value))
	}
}

func TestADKMCPRuntimeExecutorRecordsContentFreeAuditLifecycle(t *testing.T) {
	audit := &recordingADKMCPRuntimeAuditRecorder{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Enabled:    true,
				ServerType: "stdio",
				Config:     `{"command":"npx","env":{"API_TOKEN":"raw-secret"}}`,
				Tools: []*toolapi.MCPToolDefinition{
					{Name: "search-docs"},
				},
			},
		},
		&recordingADKMCPRuntimeTransport{result: "secret output"},
		WithADKMCPRuntimeExecutorAuditRecorder(audit),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, "secret output", result)
	require.Len(t, audit.records, 2)
	require.Equal(t, "mcp.tool.started", audit.records[0].EventType)
	require.Equal(t, "mcp.tool.completed", audit.records[1].EventType)
	require.Equal(t, int64(30), audit.records[1].SpaceID)
	require.Equal(t, int64(10), audit.records[1].ThreadID)
	require.Equal(t, int64(20), audit.records[1].RunID)
	require.Equal(t, int64(100), audit.records[1].ServerID)
	require.Equal(t, "mcp_100_search_docs", audit.records[1].RuntimeToolName)
	require.Equal(t, int64(len("secret output")), audit.records[1].OutputBytes)
	for _, record := range audit.records {
		require.NotContains(t, record.RuntimeToolName, "secret docs")
		require.NotContains(t, record.ErrorCode, "secret docs")
		require.NotContains(t, record.ErrorCode, "raw-secret")
		require.NotContains(t, record.ErrorCode, "secret output")
	}
}

func TestADKMCPRuntimeExecutorRecordsAuditFailure(t *testing.T) {
	audit := &recordingADKMCPRuntimeAuditRecorder{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  999,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{},
		WithADKMCPRuntimeExecutorAuditRecorder(audit),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Len(t, audit.records, 2)
	require.Equal(t, "mcp.tool.started", audit.records[0].EventType)
	require.Equal(t, "mcp.tool.failed", audit.records[1].EventType)
	require.Equal(t, "space_mismatch", audit.records[1].ErrorCode)
}

func TestADKMCPRuntimeExecutorReportsRuntimeHealthAfterTransport(
	t *testing.T,
) {
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Enabled:    true,
				ServerType: "stdio",
				Config:     `{"command":"npx"}`,
				Tools:      []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{result: "bounded output"},
		WithADKMCPRuntimeExecutorHealthReporter(health),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, "bounded output", result)
	require.Len(t, health.reports, 1)
	require.True(t, health.reports[0].Success)
	require.Equal(t, int64(100), health.reports[0].ServerID)
	require.Empty(t, health.reports[0].ErrorCode)
	require.GreaterOrEqual(t, health.reports[0].LatencyMs, int64(0))
}

func TestADKMCPRuntimeExecutorReportsUnhealthyAfterTransportFailure(
	t *testing.T,
) {
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Enabled:    true,
				ServerType: "stdio",
				Config:     `{"command":"npx"}`,
				Tools:      []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{err: fmt.Errorf("spawn failed with stdio-secret-token")},
		WithADKMCPRuntimeExecutorHealthReporter(health),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Len(t, health.reports, 1)
	require.False(t, health.reports[0].Success)
	require.Equal(t, "transport_failed", health.reports[0].ErrorCode)
	require.NotContains(t, health.reports[0].ErrorCode, "stdio-secret-token")
}

func TestADKMCPRuntimeExecutorReportsUnhealthyAfterOutputBudgetExceeded(
	t *testing.T,
) {
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Enabled:    true,
				ServerType: "stdio",
				Config:     `{"command":"npx"}`,
				Tools:      []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{result: "oversized output"},
		WithADKMCPRuntimeExecutorMaxOutputBytes(4),
		WithADKMCPRuntimeExecutorHealthReporter(health),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Len(t, health.reports, 1)
	require.False(t, health.reports[0].Success)
	require.Equal(t, "output_budget_exceeded", health.reports[0].ErrorCode)
}

func TestADKMCPRuntimeExecutorDoesNotReportHealthForPreTransportValidation(
	t *testing.T,
) {
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  999,
				Enabled:  true,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{},
		WithADKMCPRuntimeExecutorHealthReporter(health),
	)

	result, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ServerID:  100,
			ToolName:  "search-docs",
			Arguments: `{"query":"secret docs"}`,
		},
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Empty(t, health.reports)
}

type recordingADKMCPRuntimeServerResolver struct {
	serverID int64
	server   *toolapi.MCPToolServer
	err      error
}

func (r *recordingADKMCPRuntimeServerResolver) ResolveADKMCPRuntimeServer(
	ctx context.Context,
	serverID int64,
) (*toolapi.MCPToolServer, error) {
	r.serverID = serverID
	if r.err != nil {
		return nil, r.err
	}
	return r.server, nil
}

type recordingADKMCPRuntimeTransport struct {
	call        ADKMCPRuntimeTransportCall
	calls       int
	deadlineSet bool
	result      string
	err         error
}

func (t *recordingADKMCPRuntimeTransport) InvokeADKMCPRuntimeTransport(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (string, error) {
	t.calls++
	t.call = call
	_, t.deadlineSet = ctx.Deadline()
	if t.err != nil {
		return "", t.err
	}
	return t.result, nil
}

type recordingADKMCPRuntimeAuditRecorder struct {
	records []ADKMCPRuntimeAuditRecord
	err     error
}

func (r *recordingADKMCPRuntimeAuditRecorder) RecordADKMCPRuntimeAudit(
	ctx context.Context,
	record ADKMCPRuntimeAuditRecord,
) error {
	r.records = append(r.records, record)
	return r.err
}

type recordingADKMCPRuntimeHealthReporter struct {
	reports []ADKMCPRuntimeHealthReport
	err     error
}

func (r *recordingADKMCPRuntimeHealthReporter) ReportADKMCPRuntimeHealth(
	ctx context.Context,
	report ADKMCPRuntimeHealthReport,
) error {
	r.reports = append(r.reports, report)
	return r.err
}

type recordingADKMCPRuntimeOutputOffloader struct {
	request ADKMCPRuntimeOutputOffloadRequest
	result  ADKMCPRuntimeOutputOffloadResult
	err     error
}

func (r *recordingADKMCPRuntimeOutputOffloader) OffloadADKMCPRuntimeOutput(
	ctx context.Context,
	request ADKMCPRuntimeOutputOffloadRequest,
) (ADKMCPRuntimeOutputOffloadResult, error) {
	r.request = request
	if r.err != nil {
		return ADKMCPRuntimeOutputOffloadResult{}, r.err
	}

	return r.result, nil
}
