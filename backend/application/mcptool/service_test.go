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

package mcptool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestApplicationServiceUpsertsListsGetsAndTestsMCPServer(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	upserted, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     1,
		Name:        "browser-tools",
		Description: "Browser automation tools",
		ServerType:  "stdio",
		Enabled:     true,
		Config:      `{"command":"npx","args":["-y","@example/browser"]}`,
		Auth:        `{"type":"none"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search",
				Description: "Search the web",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(100), upserted.Data.ServerID)
	require.Equal(t, "browser-tools", upserted.Data.Name)
	require.Equal(t, "search", upserted.Data.Tools[0].Name)
	require.Equal(t, "unknown", upserted.Data.HealthStatus)

	listed, err := svc.ListServers(context.Background(), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 1)
	require.Equal(t, int64(100), listed.Data.Servers[0].ServerID)
	require.Equal(t, "unknown", listed.Data.Servers[0].HealthStatus)

	got, err := svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.NoError(t, err)
	require.Equal(t, "stdio", got.Data.ServerType)

	tested, err := svc.TestCall(context.Background(), &toolapi.TestMCPToolCallRequest{
		ServerID:  100,
		ToolName:  "search",
		Arguments: `{"query":"coze studio"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "success", tested.Data.Status)
	require.GreaterOrEqual(t, tested.Data.LatencyMs, int64(0))

	var output map[string]any
	require.NoError(t, json.Unmarshal([]byte(tested.Data.Output), &output))
	require.Equal(t, "browser-tools", output["server_name"])
	require.Equal(t, "search", output["tool_name"])
	require.Equal(t, map[string]any{"query": "coze studio"}, output["arguments"])

	listed, err = svc.ListServers(context.Background(), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 1)
	require.Equal(t, "healthy", listed.Data.Servers[0].HealthStatus)
	require.Greater(t, listed.Data.Servers[0].HealthCheckedAt, int64(0))
	require.GreaterOrEqual(t, listed.Data.Servers[0].HealthLatencyMs, int64(0))
	require.Empty(t, listed.Data.Servers[0].HealthError)
}

func TestApplicationServiceRecordsRuntimeHealth(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})
	created, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "runtime-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{"command":"npx"}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{}`},
		},
	})
	require.NoError(t, err)

	err = svc.RecordRuntimeHealth(context.Background(), MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   true,
		LatencyMs: 25,
		CheckedAt: 2000,
	})
	require.NoError(t, err)
	got, err := svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{
		ServerID: created.Data.ServerID,
	})
	require.NoError(t, err)
	require.Equal(t, "healthy", got.Data.HealthStatus)
	require.Equal(t, int64(25), got.Data.HealthLatencyMs)
	require.Equal(t, int64(2000), got.Data.HealthCheckedAt)
	require.Empty(t, got.Data.HealthError)

	err = svc.RecordRuntimeHealth(context.Background(), MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   false,
		ErrorCode: "transport_failed",
		LatencyMs: 27,
		CheckedAt: 2500,
	})
	require.NoError(t, err)
	got, err = svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{
		ServerID: created.Data.ServerID,
	})
	require.NoError(t, err)
	require.Equal(t, "unhealthy", got.Data.HealthStatus)
	require.Equal(t, int64(27), got.Data.HealthLatencyMs)
	require.Equal(t, int64(2500), got.Data.HealthCheckedAt)
	require.Equal(t, "transport_failed", got.Data.HealthError)

	err = svc.RecordRuntimeHealth(context.Background(), MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   false,
		ErrorCode: "transport_failed:/mnt/coze/mcp?token=stdio-secret-token",
		LatencyMs: 31,
		CheckedAt: 3000,
	})
	require.NoError(t, err)
	got, err = svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{
		ServerID: created.Data.ServerID,
	})
	require.NoError(t, err)
	require.Equal(t, "unhealthy", got.Data.HealthStatus)
	require.Equal(t, int64(31), got.Data.HealthLatencyMs)
	require.Equal(t, int64(3000), got.Data.HealthCheckedAt)
	require.Equal(t, "runtime_failed", got.Data.HealthError)
	require.NotContains(t, got.Data.HealthError, "/mnt/coze/mcp")
	require.NotContains(t, got.Data.HealthError, "stdio-secret-token")
}

func TestApplicationServiceListsSkillToolCandidatesFromEnabledMCPServers(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	_, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     1,
		Name:        "docs-mcp",
		Description: "Documentation MCP server",
		ServerType:  "stdio",
		Enabled:     true,
		Config:      `{"command":"npx","args":["-y","@example/docs"]}`,
		Auth:        `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search-docs",
				Description: "Search internal documentation.",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
			{
				Name:        "missing_description",
				Description: "",
				InputSchema: `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)

	_, err = svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "disabled-mcp",
		ServerType: "stdio",
		Enabled:    false,
		Config:     `{"command":"npx"}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "disabled_search",
				Description: "Disabled tools must not be grant candidates.",
				InputSchema: `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)

	candidates, err := svc.ListSkillToolCandidates(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, []*skillapi.SkillToolCandidate{
		{
			Name:        "mcp_100_search_docs",
			DisplayName: "docs-mcp / search-docs",
			Description: "Search internal documentation.",
			Category:    "mcp",
			Visibility:  "static",
			Source:      "mcp",
			SourceID:    "100",
			SourceName:  "docs-mcp",
		},
	}, candidates)
}

func TestApplicationServiceRejectsInvalidMCPConfigJSON(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 101},
	})

	_, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "bad-tools",
		ServerType: "stdio",
		Config:     `{"command":`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", InputSchema: `{"type":"object"}`},
		},
	})

	require.ErrorContains(t, err, "invalid config json")
	require.True(t, IsClientError(err))
}

func TestApplicationServiceRejectsCrossSpaceMCPServerUpdate(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	created, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "docs-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
	})
	require.NoError(t, err)

	_, err = svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		ServerID:   created.Data.ServerID,
		SpaceID:    2,
		Name:       "hijack-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
	})

	require.ErrorContains(t, err, "space mismatch")
	require.True(t, IsClientError(err))
}

func TestApplicationServiceMasksMCPAuthAndPreservesMaskedSecrets(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog: catalog,
		IDGen:   &sequentialIDGen{next: 100},
	})

	created, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     1,
		Name:        "secure-mcp",
		Description: "Secure MCP",
		ServerType:  "streamable_http",
		Enabled:     true,
		Config:      `{"url":"https://mcp.example.test"}`,
		Auth:        `{"type":"bearer","token":"secret-token","nested":{"api_key":"secret-key"}}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
	})

	require.NoError(t, err)
	require.JSONEq(t,
		`{"type":"bearer","token":"********","nested":{"api_key":"********"}}`,
		created.Data.Auth,
	)
	stored, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"type":"bearer","token":"secret-token","nested":{"api_key":"secret-key"}}`,
		stored.Auth,
	)

	updated, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		ServerID:    100,
		SpaceID:     1,
		Name:        "secure-mcp",
		Description: "Updated secure MCP",
		ServerType:  "streamable_http",
		Enabled:     true,
		Config:      `{"url":"https://mcp.example.test"}`,
		Auth:        created.Data.Auth,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
	})

	require.NoError(t, err)
	require.JSONEq(t,
		`{"type":"bearer","token":"********","nested":{"api_key":"********"}}`,
		updated.Data.Auth,
	)
	stored, err = catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"type":"bearer","token":"secret-token","nested":{"api_key":"secret-key"}}`,
		stored.Auth,
	)
}

func TestApplicationServiceDeletesMCPServer(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	created, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "delete-me",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{"command":"npx"}`,
		Auth:       `{"type":"bearer","token":"secret-token"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
	})
	require.NoError(t, err)

	deleted, err := svc.DeleteServer(context.Background(), &toolapi.GetMCPToolServerRequest{
		ServerID: 100,
	})

	require.NoError(t, err)
	require.Equal(t, created.Data.ServerID, deleted.Data.ServerID)
	require.JSONEq(t, `{"type":"bearer","token":"********"}`, deleted.Data.Auth)

	listed, err := svc.ListServers(context.Background(), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Empty(t, listed.Data.Servers)

	_, err = svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestApplicationServiceListsMCPToolRegistryEntries(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	_, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     1,
		Name:        "docs-mcp",
		Description: "Documentation MCP server",
		ServerType:  "stdio",
		Enabled:     true,
		Config:      `{"command":"npx","args":["-y","@example/docs"]}`,
		Auth:        `{"type":"bearer","token":"secret-token"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search-docs",
				Description: "Search internal documentation.",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
			{
				Name:        "missing_description",
				Description: "",
				InputSchema: `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)

	_, err = svc.TestCall(context.Background(), &toolapi.TestMCPToolCallRequest{
		ServerID:  100,
		ToolName:  "search-docs",
		Arguments: `{"query":"coze studio"}`,
	})
	require.NoError(t, err)

	_, err = svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "disabled-mcp",
		ServerType: "stdio",
		Enabled:    false,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "disabled_search", Description: "Disabled search.", InputSchema: `{"type":"object"}`},
		},
	})
	require.NoError(t, err)

	resp, err := svc.ListRegistryEntries(context.Background(), &toolapi.ListMCPToolRegistryEntriesRequest{
		SpaceID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, []*toolapi.MCPToolRegistryEntry{
		{
			Name:            "mcp_100_search_docs",
			Source:          "mcp",
			Category:        "mcp",
			Visibility:      "static",
			ServerID:        100,
			ServerName:      "docs-mcp",
			ToolName:        "search-docs",
			Description:     "Search internal documentation.",
			Enabled:         true,
			HealthStatus:    "healthy",
			HealthCheckedAt: resp.Data.Tools[0].HealthCheckedAt,
			HealthLatencyMs: resp.Data.Tools[0].HealthLatencyMs,
		},
	}, resp.Data.Tools)
	require.Equal(t, int64(1), resp.Data.Total)

	direct, err := svc.ListMCPToolRegistryEntries(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, resp.Data.Tools, direct)
}

func TestApplicationServiceResolvesRuntimeServerWithRawConfigAndAuth(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})
	_, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "docs-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{"command":"npx","args":["-y","@example/docs"]}`,
		Auth:       `{"token":"raw-secret"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search-docs",
				Description: "Search internal documentation.",
				InputSchema: `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)

	server, err := svc.ResolveADKMCPRuntimeServer(context.Background(), 100)

	require.NoError(t, err)
	require.Equal(t, int64(100), server.ServerID)
	require.Equal(t, `{"command":"npx","args":["-y","@example/docs"]}`, server.Config)
	require.Equal(t, `{"token":"raw-secret"}`, server.Auth)

	response, err := svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.NoError(t, err)
	require.NotContains(t, response.Data.Auth, "raw-secret")
}

type sequentialIDGen struct {
	next int64
}

func (g *sequentialIDGen) GenID(ctx context.Context) (int64, error) {
	id := g.next
	g.next++

	return id, nil
}

func (g *sequentialIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, 0, counts)
	for i := 0; i < counts; i++ {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}
