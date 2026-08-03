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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
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
	invocationIDs := make([]string, 0, len(events.events))
	for _, event := range events.events {
		require.NotContains(t, event.Payload, "secret docs")
		require.NotContains(t, event.Payload, "raw-secret")
		require.NotContains(t, event.Payload, "docs-mcp")
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(event.Payload), &payload))
		invocationID, _ := payload["invocation_id"].(string)
		require.NotEmpty(t, invocationID)
		invocationIDs = append(invocationIDs, invocationID)
	}
	require.Equal(t, invocationIDs[0], invocationIDs[1])
	started, err := ProjectRunEventToJournal(events.events[0])
	require.NoError(t, err)
	completed, err := ProjectRunEventToJournal(events.events[1])
	require.NoError(t, err)
	require.Equal(t, started.ActionID, completed.ActionID)
}

func TestADKMCPRuntimeExecutorIgnoresHealthReporterErrors(t *testing.T) {
	health := &recordingADKMCPRuntimeHealthReporter{
		err: fmt.Errorf("health reporter unavailable"),
	}
	events := &recordingRunEventSink{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Enabled:    true,
				UpdatedAt:  55,
				ServerType: "stdio",
				Tools: []*toolapi.MCPToolDefinition{
					{Name: "search-docs", Description: "Search docs."},
				},
			},
		},
		&recordingADKMCPRuntimeTransport{result: "bounded output"},
		WithADKMCPRuntimeExecutorEventSink(events),
		WithADKMCPRuntimeExecutorHealthReporter(health),
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
	require.Equal(t, "bounded output", result)
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.completed"}, events.eventTypes())
	require.Len(t, health.reports, 1)
	require.True(t, health.reports[0].Success)
	require.Equal(t, int64(100), health.reports[0].ServerID)
	require.Equal(t, int64(55), health.reports[0].ExpectedUpdatedAt)
}

func TestADKMCPRuntimeExecutorIgnoresRealHealthProducerAppendFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	createADKMCPRuntimeHealthTables(t, db)
	catalog := appmcptool.NewMySQLCatalog(db)
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    30,
		CreatorID:  7,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "docs-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search-docs", Description: "Search docs."},
		},
		HealthStatus:    "unhealthy",
		HealthCheckedAt: 90,
		HealthError:     "transport_failed",
		CreatedAt:       10,
		UpdatedAt:       55,
	}))
	require.NoError(t, db.Table("mcp_tool_servers").
		Where("server_id = ?", 100).
		Updates(map[string]any{
			"health_consecutive_failures": 3,
			"health_incident_id":          "mcp-incident-100-90",
			"health_incident_opened_at":   90,
		}).Error)
	previousNotificationService := appnotification.SVC
	notificationRepo := &failingADKMCPNotificationRepository{err: errors.New("append failed")}
	appnotification.SetDefaultService(appnotification.NewService(notificationRepo))
	t.Cleanup(func() {
		appnotification.SetDefaultService(previousNotificationService)
	})
	mcpService := appmcptool.NewApplicationService(&appmcptool.Components{
		Catalog: catalog,
		SpaceMemberRoleReader: &adkMCPRuntimeSpaceMemberRoleReader{
			roles: []appmcptool.SpaceMemberRole{{UserID: 7, RoleType: 1}},
		},
	})
	healthReporter := ADKMCPRuntimeHealthReporterFunc(
		func(ctx context.Context, report ADKMCPRuntimeHealthReport) error {
			return mcpService.RecordRuntimeHealth(ctx, appmcptool.MCPRuntimeHealthReport{
				ServerID:          report.ServerID,
				ExpectedUpdatedAt: report.ExpectedUpdatedAt,
				Success:           report.Success,
				ErrorCode:         report.ErrorCode,
				LatencyMs:         report.LatencyMs,
			})
		},
	)
	events := &recordingRunEventSink{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Enabled:    true,
				UpdatedAt:  55,
				ServerType: "stdio",
				Tools: []*toolapi.MCPToolDefinition{
					{Name: "search-docs", Description: "Search docs."},
				},
			},
		},
		&recordingADKMCPRuntimeTransport{result: "bounded output"},
		WithADKMCPRuntimeExecutorEventSink(events),
		WithADKMCPRuntimeExecutorHealthReporter(healthReporter),
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
	require.Equal(t, "bounded output", result)
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.completed"}, events.eventTypes())
	require.Equal(t, 1, notificationRepo.appendCalls)
	var incidentID string
	require.NoError(t, db.Table("mcp_tool_servers").
		Where("server_id = ?", 100).
		Select("health_incident_id").
		Scan(&incidentID).Error)
	require.Equal(t, "mcp-incident-100-90", incidentID)
}

func TestADKMCPRuntimeExecutorDoesNotReportHealthForDisabledServer(t *testing.T) {
	transport := &recordingADKMCPRuntimeTransport{result: "should-not-run"}
	health := &recordingADKMCPRuntimeHealthReporter{
		err: errors.New("health reporter unavailable"),
	}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100,
				SpaceID:  30,
				Enabled:  false,
				Tools:    []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		transport,
		WithADKMCPRuntimeExecutorHealthReporter(health),
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
	require.Contains(t, err.Error(), "server disabled")
	require.Zero(t, transport.calls)
	require.Empty(t, health.reports)
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
	require.Equal(t, "stdio", audit.records[1].Transport)
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
	require.Equal(t, adkMCPRuntimeTransportStdio, health.reports[0].Transport)
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
	require.Equal(t, adkMCPRuntimeTransportStdio, health.reports[0].Transport)
	require.Equal(t, "transport_failed", health.reports[0].ErrorCode)
	require.NotContains(t, health.reports[0].ErrorCode, "stdio-secret-token")
}

func TestADKMCPRuntimeExecutorProjectsTransportTimeoutAsTimedOut(t *testing.T) {
	events := &recordingRunEventSink{}
	health := &recordingADKMCPRuntimeHealthReporter{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID: 100, SpaceID: 30, Enabled: true, ServerType: "stdio",
				Config: `{"command":"npx"}`,
				Tools:  []*toolapi.MCPToolDefinition{{Name: "search-docs"}},
			},
		},
		&recordingADKMCPRuntimeTransport{err: context.DeadlineExceeded},
		WithADKMCPRuntimeExecutorEventSink(events),
		WithADKMCPRuntimeExecutorHealthReporter(health),
	)

	_, err := executor.InvokeADKMCPRuntimeTool(
		context.Background(),
		ADKMCPRuntimeToolCall{
			Run:  &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name: "mcp_100_search_docs", ServerID: 100, ToolName: "search-docs",
			Arguments: `{}`,
		},
	)

	require.Error(t, err)
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.failed"}, events.eventTypes())
	require.Len(t, health.reports, 1)
	require.Equal(t, "tool_timeout", health.reports[0].ErrorCode)
	projection, projectErr := ProjectRunEventToJournal(events.events[1])
	require.NoError(t, projectErr)
	require.NotNil(t, projection)
	require.Equal(t, "timed_out", projection.Status)
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

func createADKMCPRuntimeHealthTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`
		CREATE TABLE mcp_tool_servers (
			server_id INTEGER PRIMARY KEY,
			space_id INTEGER NOT NULL,
			creator_id INTEGER NOT NULL DEFAULT 0,
			source_type TEXT NOT NULL DEFAULT 'custom',
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			server_type TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			config TEXT NOT NULL,
			auth TEXT NOT NULL,
			tools TEXT NOT NULL,
			resources TEXT,
			prompts TEXT,
			health_status TEXT NOT NULL DEFAULT 'unknown',
			health_checked_at INTEGER NOT NULL DEFAULT 0,
			health_latency_ms INTEGER NOT NULL DEFAULT 0,
			health_error TEXT NOT NULL DEFAULT '',
			health_consecutive_failures INTEGER NOT NULL DEFAULT 0,
			health_incident_id TEXT NOT NULL DEFAULT '',
			health_incident_opened_at INTEGER NOT NULL DEFAULT 0,
			health_last_recovered_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER NOT NULL DEFAULT 0
		)
	`).Error)
}

type adkMCPRuntimeSpaceMemberRoleReader struct {
	roles []appmcptool.SpaceMemberRole
}

func (r *adkMCPRuntimeSpaceMemberRoleReader) ListSpaceMemberRoles(
	ctx context.Context,
	spaceID int64,
) ([]appmcptool.SpaceMemberRole, error) {
	return append([]appmcptool.SpaceMemberRole(nil), r.roles...), nil
}

type failingADKMCPNotificationRepository struct {
	err         error
	appendCalls int
}

func (r *failingADKMCPNotificationRepository) Append(
	ctx context.Context,
	event domainnotification.Event,
) error {
	return r.err
}

func (r *failingADKMCPNotificationRepository) AppendInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	r.appendCalls++
	return r.err
}

func (r *failingADKMCPNotificationRepository) ListForUser(
	ctx context.Context,
	filter domainnotification.ListFilter,
) (domainnotification.ListPage, error) {
	return domainnotification.ListPage{}, r.err
}

func (r *failingADKMCPNotificationRepository) CountUnread(
	ctx context.Context,
	userID int64,
) (int64, error) {
	return 0, r.err
}

func (r *failingADKMCPNotificationRepository) MarkRead(
	ctx context.Context,
	userID int64,
	notificationIDs []int64,
	readAt int64,
) (int64, error) {
	return 0, r.err
}

func (r *failingADKMCPNotificationRepository) MarkAllRead(
	ctx context.Context,
	userID int64,
	cutoff int64,
	readAt int64,
) (int64, error) {
	return 0, r.err
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
