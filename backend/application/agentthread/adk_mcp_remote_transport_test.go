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

func TestADKMCPRuntimeRemoteTransportValidatesAndInvokesRunner(t *testing.T) {
	runner := &recordingADKMCPRuntimeRemoteRunner{
		result: `{"content":[{"type":"text","text":"ok"}]}`,
	}
	transport := NewADKMCPRuntimeRemoteTransport(
		ADKMCPRuntimeRemoteTransportOptions{
			Runner:            runner,
			AllowedHosts:      []string{"mcp.example.test"},
			AllowInsecureHTTP: true,
		},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		ADKMCPRuntimeTransportCall{
			Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
			Name:      "mcp_100_search_docs",
			ToolName:  "search-docs",
			Arguments: `{"query":"coze"}`,
			Server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "remote-docs",
				ServerType: "http",
				Config: `{
					"url":"https://mcp.example.test/mcp",
					"headers":{"Authorization":"Bearer remote-secret-token"}
				}`,
				Tools: []*toolapi.MCPToolDefinition{
					{Name: "search-docs", Description: "Search docs."},
				},
			},
		},
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"content":[{"type":"text","text":"ok"}]}`, result)
	require.Equal(t, int64(100), runner.execution.ServerID)
	require.Equal(t, "streamable_http", runner.execution.TransportType)
	require.Equal(t, "https://mcp.example.test/mcp", runner.execution.URL)
	require.Equal(t, "search-docs", runner.execution.ToolName)
	require.Equal(t, `{"query":"coze"}`, runner.execution.Arguments)
	require.Equal(t, "Bearer remote-secret-token", runner.execution.Headers["Authorization"])
	require.Equal(t, []string{"mcp.example.test"}, runner.execution.AllowedHosts)
	require.True(t, runner.execution.AllowLocalDebug)
}

func TestADKMCPRuntimeRemoteTransportRejectsUnsafeConfigSafely(t *testing.T) {
	tests := []struct {
		name      string
		server    *toolapi.MCPToolServer
		transport *ADKMCPRuntimeRemoteTransport
	}{
		{
			name: "host not allowlisted",
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "remote-docs-secret",
				ServerType: "http",
				Config: `{
					"url":"https://secret.example.test/mcp",
					"headers":{"Authorization":"Bearer remote-secret-token"}
				}`,
			},
			transport: NewADKMCPRuntimeRemoteTransport(
				ADKMCPRuntimeRemoteTransportOptions{
					Runner:       &recordingADKMCPRuntimeRemoteRunner{},
					AllowedHosts: []string{"mcp.example.test"},
				},
			),
		},
		{
			name: "non-local insecure http",
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "remote-docs-secret",
				ServerType: "sse",
				Config: `{
					"url":"http://mcp.example.test/sse",
					"headers":{"Authorization":"Bearer remote-secret-token"}
				}`,
			},
			transport: NewADKMCPRuntimeRemoteTransport(
				ADKMCPRuntimeRemoteTransportOptions{
					Runner:       &recordingADKMCPRuntimeRemoteRunner{},
					AllowedHosts: []string{"mcp.example.test"},
				},
			),
		},
		{
			name: "unsafe header value",
			server: &toolapi.MCPToolServer{
				ServerID:   100,
				SpaceID:    30,
				Name:       "remote-docs-secret",
				ServerType: "http",
				Config: `{
					"url":"https://mcp.example.test/mcp",
					"headers":{"Authorization":"Bearer remote-secret-token\r\nX-Leak: yes"}
				}`,
			},
			transport: NewADKMCPRuntimeRemoteTransport(
				ADKMCPRuntimeRemoteTransportOptions{
					Runner:       &recordingADKMCPRuntimeRemoteRunner{},
					AllowedHosts: []string{"mcp.example.test"},
				},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				ADKMCPRuntimeTransportCall{
					Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
					Name:      "mcp_100_search_docs",
					ToolName:  "search-docs",
					Arguments: `{"query":"secret customer path"}`,
					Server:    tt.server,
				},
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), "mcp runtime remote config is invalid")
			assertADKMCPRemoteTransportErrorDoesNotLeak(t, err.Error())
		})
	}
}

type recordingADKMCPRuntimeRemoteRunner struct {
	execution ADKMCPRuntimeRemoteExecution
	result    string
	err       error
}

func (r *recordingADKMCPRuntimeRemoteRunner) RunADKMCPRuntimeRemote(
	ctx context.Context,
	execution ADKMCPRuntimeRemoteExecution,
) (string, error) {
	r.execution = execution
	if r.err != nil {
		return "", r.err
	}

	return r.result, nil
}

func assertADKMCPRemoteTransportErrorDoesNotLeak(t *testing.T, text string) {
	t.Helper()
	require.NotContains(t, text, "secret customer path")
	require.NotContains(t, text, "remote-docs-secret")
	require.NotContains(t, text, "remote-secret-token")
	require.NotContains(t, text, "secret.example.test")
	require.NotContains(t, text, "mcp.example.test")
}
