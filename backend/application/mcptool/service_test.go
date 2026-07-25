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
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

func TestApplicationServiceUpsertsListsGetsAndTestsMCPServer(t *testing.T) {
	ctx := managementContext(7)
	discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "search",
			Description: "Search the web",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
		},
	}}}
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:              NewInMemoryCatalog(),
		IDGen:                &sequentialIDGen{next: 100},
		CapabilityDiscoverer: discoverer,
		RuntimeExecutor: &managementRuntimeExecutor{result: &RuntimeToolResult{
			Status: "success",
			Output: `{"server_name":"browser-tools","tool_name":"search","arguments":{"query":"coze studio"}}`,
		}},
	})

	upserted, err := svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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
	require.Len(t, upserted.Data.Tools, 1)
	require.Equal(t, "healthy", upserted.Data.HealthStatus)

	discoverer.result = &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "search",
			Description: "Search the web",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
		},
	}}
	discovered, err := svc.Discover(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, "search", discovered.Tools[0].Name)

	listed, err := svc.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 1)
	require.Equal(t, int64(100), listed.Data.Servers[0].ServerID)
	require.Equal(t, "healthy", listed.Data.Servers[0].HealthStatus)

	got, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.NoError(t, err)
	require.Equal(t, "stdio", got.Data.ServerType)

	tested, err := svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
		ServerID:  100,
		ToolName:  "search",
		Arguments: `{"query":"coze studio"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "success", tested.Data.Status)
	require.GreaterOrEqual(t, tested.Data.LatencyMs, int64(0))

	require.JSONEq(t, `{"result":"completed"}`, tested.Data.Output)

	listed, err = svc.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 1)
	require.Equal(t, "healthy", listed.Data.Servers[0].HealthStatus)
	require.Greater(t, listed.Data.Servers[0].HealthCheckedAt, int64(0))
	require.GreaterOrEqual(t, listed.Data.Servers[0].HealthLatencyMs, int64(0))
	require.Empty(t, listed.Data.Servers[0].HealthError)
}

func TestApplicationServiceRecordsRuntimeHealth(t *testing.T) {
	ctx := managementContext(7)
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:             NewInMemoryCatalog(),
		IDGen:               &sequentialIDGen{next: 100},
		UserSpaceRoleReader: ownerRoleReader(1),
	})
	created, err := svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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
	firstCheckedAt := created.Data.HealthCheckedAt + 1

	err = svc.RecordRuntimeHealth(ctx, MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   true,
		LatencyMs: 25,
		CheckedAt: firstCheckedAt,
	})
	require.NoError(t, err)
	got, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{
		ServerID: created.Data.ServerID,
	})
	require.NoError(t, err)
	require.Equal(t, "healthy", got.Data.HealthStatus)
	require.Equal(t, int64(25), got.Data.HealthLatencyMs)
	require.Equal(t, firstCheckedAt, got.Data.HealthCheckedAt)
	require.Empty(t, got.Data.HealthError)

	secondCheckedAt := firstCheckedAt + 1
	err = svc.RecordRuntimeHealth(ctx, MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   false,
		ErrorCode: "transport_failed",
		LatencyMs: 27,
		CheckedAt: secondCheckedAt,
	})
	require.NoError(t, err)
	got, err = svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{
		ServerID: created.Data.ServerID,
	})
	require.NoError(t, err)
	require.Equal(t, "unhealthy", got.Data.HealthStatus)
	require.Equal(t, int64(27), got.Data.HealthLatencyMs)
	require.Equal(t, secondCheckedAt, got.Data.HealthCheckedAt)
	require.Equal(t, "transport_failed", got.Data.HealthError)

	thirdCheckedAt := secondCheckedAt + 1
	err = svc.RecordRuntimeHealth(ctx, MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   false,
		ErrorCode: "transport_failed:/mnt/coze/mcp?token=stdio-secret-token",
		LatencyMs: 31,
		CheckedAt: thirdCheckedAt,
	})
	require.NoError(t, err)
	got, err = svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{
		ServerID: created.Data.ServerID,
	})
	require.NoError(t, err)
	require.Equal(t, "unhealthy", got.Data.HealthStatus)
	require.Equal(t, int64(31), got.Data.HealthLatencyMs)
	require.Equal(t, thirdCheckedAt, got.Data.HealthCheckedAt)
	require.Equal(t, "runtime_failed", got.Data.HealthError)
	require.NotContains(t, got.Data.HealthError, "/mnt/coze/mcp")
	require.NotContains(t, got.Data.HealthError, "stdio-secret-token")
}

func TestApplicationServiceRecordRuntimeHealthSkipsDisabledAndTransientNotifications(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	repository := installRecordingMCPNotificationService(t)
	roleReader := &recordingMCPHealthRoleReader{
		spaces: []*userentity.Space{{ID: 1, RoleType: spaceRoleOwner}},
		members: []SpaceMemberRole{
			{UserID: 8, RoleType: spaceRoleOwner},
			{UserID: 9, RoleType: spaceRoleAdmin},
			{UserID: 10, RoleType: 3},
		},
	}
	catalog := NewMySQLCatalog(db)
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: roleReader,
		SpaceMemberRoleReader: roleReader,
	})
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		CreatorID:  7,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "disabled-health-mcp",
		ServerType: "stdio",
		Enabled:    false,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}))
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   101,
		SpaceID:    1,
		CreatorID:  7,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "transient-health-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}))

	require.NoError(t, svc.RecordRuntimeHealth(managementContext(7), MCPRuntimeHealthReport{
		ServerID:  100,
		Success:   false,
		ErrorCode: "transport_failed",
		CheckedAt: 30,
	}))
	require.Empty(t, repository.events)
	require.Empty(t, roleReader.memberSpaceIDs)

	for index := int64(0); index < 2; index++ {
		require.NoError(t, svc.RecordRuntimeHealth(managementContext(7), MCPRuntimeHealthReport{
			ServerID:  101,
			Success:   false,
			ErrorCode: "transport_failed",
			CheckedAt: 40 + index,
		}))
	}
	require.Empty(t, repository.events)
	require.Empty(t, roleReader.memberSpaceIDs)
}

func TestApplicationServiceRecordRuntimeHealthResolvesMCPIncidentRecipientsBySource(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	repository := installRecordingMCPNotificationService(t)
	roleReader := &recordingMCPHealthRoleReader{
		spaces: []*userentity.Space{{ID: 1, RoleType: spaceRoleOwner}},
		members: []SpaceMemberRole{
			{UserID: 8, RoleType: spaceRoleOwner},
			{UserID: 9, RoleType: spaceRoleAdmin},
			{UserID: 10, RoleType: 3},
		},
	}
	catalog := NewMySQLCatalog(db)
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: roleReader,
		SpaceMemberRoleReader: roleReader,
	})
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		CreatorID:  7,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "custom-health-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}))
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   101,
		SpaceID:    1,
		CreatorID:  7,
		SourceType: toolapi.MCPServerSourceType("official"),
		Name:       "official-health-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}))

	for index := int64(0); index < 3; index++ {
		require.NoError(t, svc.RecordRuntimeHealth(managementContext(7), MCPRuntimeHealthReport{
			ServerID:  100,
			Success:   false,
			ErrorCode: "transport_failed",
			CheckedAt: 50 + index,
		}))
	}
	require.Len(t, repository.events, 1)
	require.Equal(t, domainnotification.EventMCPConnectionDegraded, repository.events[0].EventType)
	require.Equal(t, []int64{7, 8, 9}, repository.events[0].Payload.ExplicitRecipientIDs)

	for index := int64(0); index < 3; index++ {
		require.NoError(t, svc.RecordRuntimeHealth(managementContext(7), MCPRuntimeHealthReport{
			ServerID:  101,
			Success:   false,
			ErrorCode: "transport_failed",
			CheckedAt: 60 + index,
		}))
	}
	require.Len(t, repository.events, 2)
	require.Equal(t, domainnotification.EventMCPConnectionDegraded, repository.events[1].EventType)
	require.Equal(t, []int64{8, 9}, repository.events[1].Payload.ExplicitRecipientIDs)
}

func TestApplicationServiceListsSkillToolCandidatesFromEnabledMCPServers(t *testing.T) {
	ctx := managementContext(7)
	discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "search-docs",
			Description: "Search internal documentation.",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
		},
		{Name: "missing_description", InputSchema: `{"type":"object"}`},
	}}}
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:              NewInMemoryCatalog(),
		IDGen:                &sequentialIDGen{next: 100},
		CapabilityDiscoverer: discoverer,
	})

	_, err := svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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
	discoverer.result = &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "search-docs",
			Description: "Search internal documentation.",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
		},
		{Name: "missing_description", InputSchema: `{"type":"object"}`},
	}}
	_, err = svc.Discover(ctx, 100)
	require.NoError(t, err)

	_, err = svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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
	discoverer.result = &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "disabled_search",
			Description: "Disabled tools must not be grant candidates.",
			InputSchema: `{"type":"object"}`,
		},
	}}
	_, err = svc.Discover(ctx, 101)
	require.NoError(t, err)

	candidates, err := svc.ListSkillToolCandidates(ctx, 1)

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
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 101},
	})

	_, err := svc.UpsertServer(managementContext(7), &toolapi.UpsertMCPToolServerRequest{
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
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	created, err := svc.UpsertServer(managementContext(7), &toolapi.UpsertMCPToolServerRequest{
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

	_, err = svc.UpsertServer(managementContext(7), &toolapi.UpsertMCPToolServerRequest{
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

	require.ErrorIs(t, err, ErrMCPForbidden)
}

func TestApplicationServiceReturnsOpaqueMCPAuthAndPreservesConfiguredSecrets(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog: catalog,
		IDGen:   &sequentialIDGen{next: 100},
	})

	created, err := svc.UpsertServer(managementContext(7), &toolapi.UpsertMCPToolServerRequest{
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
	require.JSONEq(t, mcpAuthConfiguredSentinel, created.Data.Auth)
	stored, err := catalog.Get(managementContext(7), 100)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"type":"bearer","token":"secret-token","nested":{"api_key":"secret-key"}}`,
		stored.Auth,
	)

	updated, err := svc.UpsertServer(managementContext(7), &toolapi.UpsertMCPToolServerRequest{
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
	require.JSONEq(t, mcpAuthConfiguredSentinel, updated.Data.Auth)
	stored, err = catalog.Get(managementContext(7), 100)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"type":"bearer","token":"secret-token","nested":{"api_key":"secret-key"}}`,
		stored.Auth,
	)
}

func TestApplicationServiceImportsDeerFlowExtensionsConfig(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog: catalog,
		IDGen:   &sequentialIDGen{next: 100},
	})

	_, err := svc.ImportDeerFlowExtensionsConfig(
		managementContext(7),
		1,
		[]byte(`{
			"mcpServers": {
				"github": {
					"enabled": true,
					"type": "stdio",
					"command": "npx",
					"args": ["-y", "@modelcontextprotocol/server-github"],
					"env": {"GITHUB_TOKEN": "raw-secret-token"},
					"description": "GitHub MCP server for repository operations"
				},
				"postgres": {
					"enabled": false,
					"type": "stdio",
					"command": "npx",
					"args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://localhost/mydb"],
					"env": {},
					"description": "PostgreSQL database access"
				},
				"openmeteo": {
					"enabled": true,
					"type": "stdio",
					"command": "npx",
					"args": ["-y", "-p", "open-meteo-mcp-server", "open-meteo-mcp-server"],
					"env": {},
					"description": "Open-Meteo MCP server for weather forecast queries"
				},
				"weather": {
					"enabled": true,
					"type": "stdio",
					"command": "node",
					"args": ["-e", "process.exit(0)"],
					"env": {},
					"description": "Weather MCP server for local weather queries"
				}
			},
			"skills": {}
		}`),
	)

	require.NoError(t, err)
	listed, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 4)

	serversByName := map[string]*toolapi.MCPToolServer{}
	for _, server := range listed.Data.Servers {
		serversByName[server.Name] = server
	}

	github := serversByName["github"]
	require.NotNil(t, github)
	require.Equal(t, int64(7), github.CreatorID)
	require.Equal(t, toolapi.MCPServerSourceTypeCustom, github.SourceType)
	require.Equal(t, "stdio", github.ServerType)
	require.True(t, github.Enabled)
	require.Len(t, github.Tools, 26)
	require.Contains(t, toolNames(github.Tools), "search_repositories")
	require.Contains(t, toolNames(github.Tools), "create_or_update_file")
	require.Contains(t, toolNames(github.Tools), "get_pull_request_reviews")
	searchRepositories := toolByName(github.Tools, "search_repositories")
	require.NotNil(t, searchRepositories)
	require.Equal(t, "Search for GitHub repositories", searchRepositories.Description)
	require.JSONEq(t,
		`{
			"type":"object",
			"properties":{
				"query":{"type":"string"},
				"page":{"type":"number"},
				"perPage":{"type":"number"}
			},
			"required":["query"]
		}`,
		searchRepositories.InputSchema,
	)
	require.JSONEq(t,
		`{"command":"npx","auth_args":{"0":"args.0","1":"args.1"},"env":{"GITHUB_TOKEN":"__COZE_MCP_WRITE_ONLY__"},"auth_env":{"GITHUB_TOKEN":"env.GITHUB_TOKEN"}}`,
		github.Config,
	)
	require.JSONEq(t, mcpAuthConfiguredSentinel, github.Auth)
	require.NotContains(t, github.Config, "raw-secret-token")
	require.NotContains(t, github.Auth, "raw-secret-token")

	stored, err := catalog.Get(managementContext(7), github.ServerID)
	require.NoError(t, err)
	require.JSONEq(t, `{"args":{"0":"-y","1":"@modelcontextprotocol/server-github"},"env":{"GITHUB_TOKEN":"raw-secret-token"}}`, stored.Auth)

	postgres := serversByName["postgres"]
	require.NotNil(t, postgres)
	require.False(t, postgres.Enabled)
	require.Len(t, postgres.Tools, 1)
	require.Equal(t, "query", postgres.Tools[0].Name)
	require.Equal(t, "Run a read-only SQL query", postgres.Tools[0].Description)
	require.JSONEq(t,
		`{"type":"object","properties":{"sql":{"type":"string"}},"required":["sql"]}`,
		postgres.Tools[0].InputSchema,
	)
	require.JSONEq(t,
		`{"command":"npx","auth_args":{"0":"args.0","1":"args.1","2":"args.2"}}`,
		postgres.Config,
	)
	require.JSONEq(t, mcpAuthConfiguredSentinel, postgres.Auth)

	openmeteo := serversByName["openmeteo"]
	require.NotNil(t, openmeteo)
	require.True(t, openmeteo.Enabled)
	require.Len(t, openmeteo.Tools, 2)
	require.Contains(t, toolNames(openmeteo.Tools), "geocoding")
	require.Contains(t, toolNames(openmeteo.Tools), "weather_forecast")
	geocoding := toolByName(openmeteo.Tools, "geocoding")
	require.NotNil(t, geocoding)
	require.Equal(t, "Search locations and return coordinates using Open-Meteo geocoding", geocoding.Description)
	require.JSONEq(t,
		`{"type":"object","properties":{"name":{"type":"string"},"count":{"type":"number"},"language":{"type":"string"},"countryCode":{"type":"string"}},"required":["name"]}`,
		geocoding.InputSchema,
	)
	require.JSONEq(t,
		`{"command":"npx","auth_args":{"0":"args.0","1":"args.1","2":"args.2","3":"args.3"}}`,
		openmeteo.Config,
	)
	require.JSONEq(t, mcpAuthConfiguredSentinel, openmeteo.Auth)

	weather := serversByName["weather"]
	require.NotNil(t, weather)
	require.True(t, weather.Enabled)
	require.Len(t, weather.Tools, 1)
	require.Equal(t, "get_weather", weather.Tools[0].Name)
	require.Equal(t, "Get current weather for a city", weather.Tools[0].Description)
	require.JSONEq(t,
		`{"type":"object","properties":{"city":{"type":"string"},"unit":{"type":"string"}},"required":["city"]}`,
		weather.Tools[0].InputSchema,
	)
	require.JSONEq(t,
		`{"command":"node","auth_args":{"0":"args.0","1":"args.1"}}`,
		weather.Config,
	)
	require.JSONEq(t, mcpAuthConfiguredSentinel, weather.Auth)

	registry, err := svc.ListMCPToolRegistryEntries(managementContext(7), 1)
	require.NoError(t, err)
	require.Contains(t, registryNames(registry), "mcp_100_search_repositories")
	require.Contains(t, registryNames(registry), "mcp_101_geocoding")
	require.Contains(t, registryNames(registry), "mcp_101_weather_forecast")
	require.NotContains(t, registryNames(registry), "mcp_102_query")
	require.Contains(t, registryNames(registry), "mcp_103_get_weather")
}

func TestApplicationServiceSeedsDefaultDeerFlowMCPServersOnFirstList(t *testing.T) {
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:                     NewInMemoryCatalog(),
		IDGen:                       &sequentialIDGen{next: 100},
		DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
	})

	listed, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 2)
	serversByName := map[string]*toolapi.MCPToolServer{}
	for _, server := range listed.Data.Servers {
		serversByName[server.Name] = server
	}
	require.Len(t, serversByName["github"].Tools, 26)
	require.Equal(t, toolapi.MCPServerSourceType("official"), serversByName["github"].SourceType)
	require.Len(t, serversByName["postgres"].Tools, 1)
	require.NotContains(t, serversByName, "openmeteo")
	require.NotContains(t, serversByName, "weather")

	listedAgain, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listedAgain.Data.Servers, 2)
	require.Equal(t, listed.Data.Servers, listedAgain.Data.Servers)
}

func TestApplicationServiceSeedsDefaultDeerFlowMCPServersForRegistry(t *testing.T) {
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:                     NewInMemoryCatalog(),
		IDGen:                       &sequentialIDGen{next: 100},
		DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
	})

	entries, err := svc.ListMCPToolRegistryEntries(managementContext(7), 1)

	require.NoError(t, err)
	names := registryNames(entries)
	require.Contains(t, names, "mcp_100_search_repositories")
	require.Contains(t, names, "mcp_101_query")
	require.NotContains(t, names, "geocoding")
	require.NotContains(t, names, "weather_forecast")
	require.NotContains(t, names, "get_weather")

	listed, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 2)
}

func TestApplicationServiceDeletesMCPServer(t *testing.T) {
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	created, err := svc.UpsertServer(managementContext(7), &toolapi.UpsertMCPToolServerRequest{
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

	deleted, err := svc.DeleteServer(managementContext(7), &toolapi.GetMCPToolServerRequest{
		ServerID: 100,
	})

	require.NoError(t, err)
	require.Equal(t, created.Data.ServerID, deleted.Data.ServerID)
	require.JSONEq(t, mcpAuthConfiguredSentinel, deleted.Data.Auth)

	listed, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Empty(t, listed.Data.Servers)

	_, err = svc.GetServer(managementContext(7), &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.ErrorIs(t, err, ErrMCPForbidden)
}

func TestApplicationServiceListsMCPToolRegistryEntries(t *testing.T) {
	ctx := managementContext(7)
	discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "search-docs",
			Description: "Search internal documentation.",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
		},
		{Name: "missing_description", InputSchema: `{"type":"object"}`},
	}}}
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:              NewInMemoryCatalog(),
		IDGen:                &sequentialIDGen{next: 100},
		CapabilityDiscoverer: discoverer,
		RuntimeExecutor: &managementRuntimeExecutor{result: &RuntimeToolResult{
			Status: "success", Output: `{}`,
		}},
	})

	_, err := svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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
	discoverer.result = &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{
			Name:        "search-docs",
			Description: "Search internal documentation.",
			InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
		},
		{Name: "missing_description", InputSchema: `{"type":"object"}`},
	}}
	_, err = svc.Discover(ctx, 100)
	require.NoError(t, err)

	_, err = svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
		ServerID:  100,
		ToolName:  "search-docs",
		Arguments: `{"query":"coze studio"}`,
	})
	require.NoError(t, err)

	_, err = svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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
	discoverer.result = &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{Name: "disabled_search", Description: "Disabled search.", InputSchema: `{"type":"object"}`},
	}}
	_, err = svc.Discover(ctx, 101)
	require.NoError(t, err)

	resp, err := svc.ListRegistryEntries(ctx, &toolapi.ListMCPToolRegistryEntriesRequest{
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
			InputSchema:     `{"type":"object","properties":{"query":{"type":"string"}}}`,
			Enabled:         true,
			HealthStatus:    "healthy",
			HealthCheckedAt: resp.Data.Tools[0].HealthCheckedAt,
			HealthLatencyMs: resp.Data.Tools[0].HealthLatencyMs,
		},
	}, resp.Data.Tools)
	require.Equal(t, int64(1), resp.Data.Total)

	direct, err := svc.ListMCPToolRegistryEntries(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, resp.Data.Tools, direct)
}

func TestApplicationServiceResolvesRuntimeServerWithRawConfigAndAuth(t *testing.T) {
	ctx := managementContext(7)
	svc := newAuthorizedLegacyApplicationService(&Components{
		Catalog:             NewInMemoryCatalog(),
		IDGen:               &sequentialIDGen{next: 100},
		UserSpaceRoleReader: ownerRoleReader(1),
	})
	_, err := svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
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

	server, err := svc.ResolveADKMCPRuntimeServer(ctx, 100)

	require.NoError(t, err)
	require.Equal(t, int64(100), server.ServerID)
	require.Equal(t, `{"auth_args":{"0":"args.0","1":"args.1"},"command":"npx"}`, server.Config)
	require.Equal(t, `{"args":{"0":"-y","1":"@example/docs"},"token":"raw-secret"}`, server.Auth)

	response, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.NoError(t, err)
	require.NotContains(t, response.Data.Auth, "raw-secret")
}

func installRecordingMCPNotificationService(t *testing.T) *recordingMCPNotificationRepository {
	previous := appnotification.SVC
	repository := &recordingMCPNotificationRepository{}
	appnotification.SetDefaultService(appnotification.NewService(repository))
	t.Cleanup(func() {
		appnotification.SetDefaultService(previous)
	})
	return repository
}

type recordingMCPNotificationRepository struct {
	events []domainnotification.Event
}

func (r *recordingMCPNotificationRepository) Append(
	ctx context.Context,
	event domainnotification.Event,
) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingMCPNotificationRepository) AppendInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingMCPNotificationRepository) ListForUser(
	ctx context.Context,
	filter domainnotification.ListFilter,
) (domainnotification.ListPage, error) {
	return domainnotification.ListPage{}, nil
}

func (r *recordingMCPNotificationRepository) CountUnread(
	ctx context.Context,
	userID int64,
) (int64, error) {
	return 0, nil
}

func (r *recordingMCPNotificationRepository) MarkRead(
	ctx context.Context,
	userID int64,
	notificationIDs []int64,
	readAt int64,
) (int64, error) {
	return 0, nil
}

func (r *recordingMCPNotificationRepository) MarkAllRead(
	ctx context.Context,
	userID int64,
	cutoff int64,
	readAt int64,
) (int64, error) {
	return 0, nil
}

type recordingMCPHealthRoleReader struct {
	spaces         []*userentity.Space
	members        []SpaceMemberRole
	userIDs        []int64
	memberSpaceIDs []int64
}

func (r *recordingMCPHealthRoleReader) GetUserSpaceList(
	ctx context.Context,
	userID int64,
) ([]*userentity.Space, error) {
	r.userIDs = append(r.userIDs, userID)
	return append([]*userentity.Space(nil), r.spaces...), nil
}

func (r *recordingMCPHealthRoleReader) ListSpaceMemberRoles(
	ctx context.Context,
	spaceID int64,
) ([]SpaceMemberRole, error) {
	r.memberSpaceIDs = append(r.memberSpaceIDs, spaceID)
	return append([]SpaceMemberRole(nil), r.members...), nil
}

func newAuthorizedLegacyApplicationService(c *Components) *ApplicationService {
	if c == nil {
		c = &Components{}
	}
	if c.UserSpaceRoleReader == nil {
		c.UserSpaceRoleReader = ownerRoleReader(1)
	}
	if c.SpaceMemberRoleReader == nil {
		c.SpaceMemberRoleReader = ownerRoleReader(1)
	}
	if c.CapabilityDiscoverer == nil {
		c.CapabilityDiscoverer = &managementDiscoverer{result: &DiscoveredCapabilities{}}
	}

	return NewApplicationService(c)
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

func toolNames(tools []*toolapi.MCPToolDefinition) []string {
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		if item != nil {
			names = append(names, item.Name)
		}
	}

	return names
}

func toolByName(
	tools []*toolapi.MCPToolDefinition,
	name string,
) *toolapi.MCPToolDefinition {
	for _, item := range tools {
		if item != nil && item.Name == name {
			return item
		}
	}

	return nil
}

func registryNames(entries []*toolapi.MCPToolRegistryEntry) []string {
	names := make([]string, 0, len(entries))
	for _, item := range entries {
		if item != nil {
			names = append(names, item.Name)
		}
	}

	return names
}
