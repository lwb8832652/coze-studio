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

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestADKMCPRuntimeTransportRouterFailsClosedWithoutHandlers(t *testing.T) {
	router := NewADKMCPRuntimeTransportRouter(
		ADKMCPRuntimeTransportRouterOptions{},
	)

	result, err := router.InvokeADKMCPRuntimeTransport(
		context.Background(),
		ADKMCPRuntimeTransportCall{
			Run:      &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:     "mcp_100_search_docs",
			ToolName: "search-docs",
			Arguments: `{
				"query":"secret customer path"
			}`,
			Server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "docs-mcp-secret",
				ServerType: "stdio",
				Config:     `{"command":"npx","env":{"TOKEN":"config-secret"}}`,
				Auth:       `{"token":"auth-secret"}`,
			},
		},
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio transport is disabled")
	assertADKMCPTransportErrorDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeTransportRouterDispatchesConfiguredHandlers(t *testing.T) {
	stdio := &recordingADKMCPRuntimeTransport{result: "stdio result"}
	sse := &recordingADKMCPRuntimeTransport{result: "sse result"}
	httpTransport := &recordingADKMCPRuntimeTransport{result: "http result"}
	router := NewADKMCPRuntimeTransportRouter(
		ADKMCPRuntimeTransportRouterOptions{
			Stdio:          stdio,
			SSE:            sse,
			StreamableHTTP: httpTransport,
		},
	)

	tests := []struct {
		name     string
		server   *toolapi.MCPToolServer
		expected string
		handler  *recordingADKMCPRuntimeTransport
	}{
		{
			name:     "stdio",
			server:   &toolapi.MCPToolServer{ServerID: 100, ServerType: "stdio"},
			expected: "stdio result",
			handler:  stdio,
		},
		{
			name:     "sse",
			server:   &toolapi.MCPToolServer{ServerID: 101, ServerType: "sse"},
			expected: "sse result",
			handler:  sse,
		},
		{
			name:     "streamable http",
			server:   &toolapi.MCPToolServer{ServerID: 102, ServerType: "streamable_http"},
			expected: "http result",
			handler:  httpTransport,
		},
		{
			name:     "http alias",
			server:   &toolapi.MCPToolServer{ServerID: 103, ServerType: "http"},
			expected: "http result",
			handler:  httpTransport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.InvokeADKMCPRuntimeTransport(
				context.Background(),
				ADKMCPRuntimeTransportCall{
					Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
					Name:      "mcp_100_search_docs",
					Server:    tt.server,
					ToolName:  "search-docs",
					Arguments: `{"query":"coze"}`,
				},
			)

			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
			require.Equal(t, tt.server.ServerID, tt.handler.call.Server.ServerID)
			require.Equal(t, "search-docs", tt.handler.call.ToolName)
			require.Equal(t, `{"query":"coze"}`, tt.handler.call.Arguments)
		})
	}
}

func TestADKMCPRuntimeTransportRouterRejectsUnsupportedServerTypeSafely(
	t *testing.T,
) {
	router := NewADKMCPRuntimeTransportRouter(
		ADKMCPRuntimeTransportRouterOptions{},
	)

	result, err := router.InvokeADKMCPRuntimeTransport(
		context.Background(),
		ADKMCPRuntimeTransportCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ToolName:  "search-docs",
			Arguments: `{"query":"secret customer path"}`,
			Server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "docs-mcp-secret",
				ServerType: "custom-secret-transport",
				Config:     `{"url":"https://secret.example.test/mcp"}`,
				Auth:       `{"token":"auth-secret"}`,
			},
		},
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "unsupported mcp runtime transport")
	assertADKMCPTransportErrorDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeExecutorWithTransportRouterFailsClosedSafely(
	t *testing.T,
) {
	events := &recordingRunEventSink{}
	executor := NewADKMCPRuntimeExecutor(
		&recordingADKMCPRuntimeServerResolver{
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "docs-mcp-secret",
				Enabled:    true,
				ServerType: "stdio",
				Config:     `{"command":"npx","env":{"TOKEN":"config-secret"}}`,
				Auth:       `{"token":"auth-secret"}`,
				Tools: []*toolapi.MCPToolDefinition{
					{Name: "search-docs", Description: "Search docs."},
				},
			},
		},
		NewADKMCPRuntimeTransportRouter(ADKMCPRuntimeTransportRouterOptions{}),
		WithADKMCPRuntimeExecutorEventSink(events),
	)

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

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime transport failed")
	assertADKMCPTransportErrorDoesNotLeak(t, err.Error())
	require.Equal(t, []string{"mcp.tool.started", "mcp.tool.failed"}, events.eventTypes())
	for _, event := range events.events {
		assertADKMCPTransportErrorDoesNotLeak(t, event.Payload)
	}
}

func assertADKMCPTransportErrorDoesNotLeak(t *testing.T, text string) {
	t.Helper()
	require.NotContains(t, text, "secret customer path")
	require.NotContains(t, text, "docs-mcp-secret")
	require.NotContains(t, text, "config-secret")
	require.NotContains(t, text, "auth-secret")
	require.NotContains(t, text, "secret.example.test")
}
