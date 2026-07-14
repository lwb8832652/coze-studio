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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestCatalogCreateAndUpdateServerUseLiveRowCAS(t *testing.T) {
	factories := map[string]func(*testing.T) Catalog{
		"memory": func(t *testing.T) Catalog {
			return NewInMemoryCatalog()
		},
		"mysql": func(t *testing.T) Catalog {
			db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			return NewMySQLCatalog(db)
		},
	}

	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			catalog := factory(t)
			ctx := context.Background()
			official := managementServer(41, 10)
			official.Name = "official-search"
			official.SourceType = toolapi.MCPServerSourceTypeOfficial
			official.Auth = `{}`
			official.UpdatedAt = 100
			require.NoError(t, catalog.Create(ctx, official))

			duplicate := managementServer(42, 10)
			duplicate.Name = official.Name
			duplicate.Auth = `{}`
			duplicate.UpdatedAt = 101
			require.ErrorIs(t, catalog.Create(ctx, duplicate), ErrMCPConflict)

			stored, err := catalog.Get(ctx, official.ServerID)
			require.NoError(t, err)
			require.Equal(t, toolapi.MCPServerSourceTypeOfficial, stored.SourceType)
			require.Equal(t, official.ServerID, stored.ServerID)
			atomicUpdate := cloneServer(stored)
			atomicUpdate.Description = "must roll back"
			atomicUpdate.UpdatedAt = stored.UpdatedAt + 1
			require.ErrorIs(t, catalog.ApplyServers(ctx, []MCPToolServerMutation{
				{Server: atomicUpdate, ExpectedUpdatedAt: stored.UpdatedAt},
				{Server: duplicate},
			}), ErrMCPConflict)
			stored, err = catalog.Get(ctx, official.ServerID)
			require.NoError(t, err)
			require.NotEqual(t, "must roll back", stored.Description)

			updated := cloneServer(stored)
			updated.Description = "new description"
			updated.UpdatedAt = stored.UpdatedAt + 1
			require.NoError(t, catalog.UpdateServer(ctx, updated, stored.UpdatedAt))

			stale := cloneServer(updated)
			stale.Description = "stale overwrite"
			stale.UpdatedAt++
			require.ErrorIs(t, catalog.UpdateServer(ctx, stale, stored.UpdatedAt), ErrMCPConflict)

			require.ErrorIs(t, catalog.DeleteServer(ctx, official.ServerID, official.SpaceID, stored.UpdatedAt), ErrMCPConflict)
			require.NoError(t, catalog.DeleteServer(ctx, official.ServerID, official.SpaceID, updated.UpdatedAt))
			require.ErrorIs(t, catalog.UpdateServer(ctx, stale, updated.UpdatedAt), ErrMCPConflict)
		})
	}
}

func TestInMemoryCatalogUpdateServerPreservesConcurrentHealthAndCapabilities(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.UpdatedAt = 100
	server.Resources = []*toolapi.MCPResource{{Name: "docs", URI: "https://example.com/docs"}}
	server.Prompts = []*toolapi.MCPPrompt{{Name: "summarize", Arguments: []*toolapi.MCPPromptArgument{}}}
	require.NoError(t, catalog.Create(context.Background(), server))

	configUpdate := cloneServer(server)
	configUpdate.Description = "updated config"
	configUpdate.Tools = []*toolapi.MCPToolDefinition{{Name: "stale-tool"}}
	configUpdate.Resources = []*toolapi.MCPResource{{Name: "stale-resource"}}
	configUpdate.Prompts = []*toolapi.MCPPrompt{{Name: "stale-prompt"}}
	configUpdate.HealthStatus = mcpToolHealthStatusUnhealthy
	configUpdate.HealthCheckedAt = 1
	configUpdate.HealthError = "stale-health"
	configUpdate.UpdatedAt = 101

	require.NoError(t, catalog.UpdateHealth(context.Background(), server.ServerID, server.UpdatedAt, MCPToolHealthSnapshot{
		Status:    mcpToolHealthStatusHealthy,
		CheckedAt: 200,
		LatencyMs: 12,
	}))
	require.NoError(t, catalog.UpdateServer(context.Background(), configUpdate, server.UpdatedAt))

	stored, err := catalog.Get(context.Background(), server.ServerID)
	require.NoError(t, err)
	require.Equal(t, "updated config", stored.Description)
	require.Equal(t, mcpToolHealthStatusHealthy, stored.HealthStatus)
	require.Equal(t, int64(200), stored.HealthCheckedAt)
	require.Equal(t, int64(12), stored.HealthLatencyMs)
	require.Empty(t, stored.HealthError)
	require.Equal(t, server.Tools, stored.Tools)
	require.Equal(t, server.Resources, stored.Resources)
	require.Equal(t, server.Prompts, stored.Prompts)
}

func TestApplicationServiceCreateCannotOverwriteOfficialName(t *testing.T) {
	catalog := NewInMemoryCatalog()
	official := managementServer(41, 10)
	official.Name = "official-search"
	official.SourceType = toolapi.MCPServerSourceTypeOfficial
	require.NoError(t, catalog.Create(context.Background(), official))
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	req := validManagementUpsertRequest(0, 10)
	req.Name = official.Name

	_, err := svc.UpsertServer(managementContext(7), req)

	require.ErrorIs(t, err, ErrMCPConflict)
	stored, getErr := catalog.Get(context.Background(), official.ServerID)
	require.NoError(t, getErr)
	require.Equal(t, toolapi.MCPServerSourceTypeOfficial, stored.SourceType)
}

func TestApplicationServiceUpdateAdvancesVersionAndPreservesLegacyEmptyAuth(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	ctx := managementContext(7)
	created, err := svc.UpsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
		SpaceID: 10, Name: "legacy-client", ServerType: "stdio", Enabled: true,
		Config: `{"command":"npx"}`, Auth: `{"cookie":"secret-cookie"}`,
	})
	require.NoError(t, err)
	storedBefore, err := catalog.Get(ctx, created.Data.ServerID)
	require.NoError(t, err)

	update := validManagementUpsertRequest(created.Data.ServerID, 10)
	update.Name = created.Data.Name
	update.ServerType = created.Data.ServerType
	update.Config = created.Data.Config
	update.Auth = ""
	updated, err := svc.UpsertServer(ctx, update)
	require.NoError(t, err)
	require.Greater(t, updated.Data.UpdatedAt, storedBefore.UpdatedAt)
	stored, err := catalog.Get(ctx, created.Data.ServerID)
	require.NoError(t, err)
	require.JSONEq(t, `{"cookie":"secret-cookie"}`, stored.Auth)

	update.Auth = mcpAuthConfiguredSentinel
	_, err = svc.UpsertServer(ctx, update)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, created.Data.ServerID)
	require.NoError(t, err)
	require.JSONEq(t, `{"cookie":"secret-cookie"}`, stored.Auth)

	update.Auth = `{}`
	_, err = svc.UpsertServer(ctx, update)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, created.Data.ServerID)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, stored.Auth)
}

func TestApplicationServiceRuntimeOperationsHideServerExistence(t *testing.T) {
	makeService := func(server *toolapi.MCPToolServer, roleSpaceID int64) *ApplicationService {
		catalog := NewInMemoryCatalog()
		if server != nil {
			require.NoError(t, catalog.Upsert(context.Background(), server))
		}
		return NewApplicationService(&Components{
			Catalog:              catalog,
			UserSpaceRoleReader:  ownerRoleReader(roleSpaceID),
			RuntimeExecutor:      &managementRuntimeExecutor{},
			CapabilityDiscoverer: &managementDiscoverer{},
		})
	}
	cases := map[string]*ApplicationService{
		"missing":     makeService(nil, 10),
		"cross-space": makeService(managementServer(41, 20), 10),
		"no-access":   makeService(managementServer(41, 10), 99),
	}

	for name, svc := range cases {
		t.Run(name, func(t *testing.T) {
			_, testErr := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
				ServerID: 41, ToolName: "search", Arguments: `{}`,
			})
			_, discoverErr := svc.Discover(managementContext(7), 41)
			require.ErrorIs(t, testErr, ErrMCPForbidden)
			require.ErrorIs(t, discoverErr, ErrMCPForbidden)
			require.True(t, errors.Is(testErr, ErrMCPForbidden) && errors.Is(discoverErr, ErrMCPForbidden))
		})
	}
}

func TestApplicationServiceRuntimeRegistrySeedsDefaultsWithoutSession(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog: catalog,
		IDGen:   &sequentialIDGen{next: 100},
		DefaultDeerFlowMCPConfigRaw: []byte(`{
			"mcpServers": {
				"github": {"enabled":true,"type":"stdio","command":"npx","args":["github"]}
			}
		}`),
	})

	entries, err := svc.ListMCPToolRegistryEntriesForRuntime(context.Background(), 10)

	require.NoError(t, err)
	require.NotEmpty(t, entries)
	servers, err := catalog.List(context.Background(), 10)
	require.NoError(t, err)
	require.NotEmpty(t, servers)
}

var errInjectedImportFailure = errors.New("injected import failure")

type failingAtomicImportCatalog struct {
	Catalog
	individualWrites int
	batchSize        int
}

func (c *failingAtomicImportCatalog) Create(ctx context.Context, server *toolapi.MCPToolServer) error {
	c.individualWrites++
	if c.individualWrites == 2 {
		return errInjectedImportFailure
	}
	return c.Catalog.Upsert(ctx, server)
}

func (c *failingAtomicImportCatalog) Upsert(ctx context.Context, server *toolapi.MCPToolServer) error {
	c.individualWrites++
	if c.individualWrites == 2 {
		return errInjectedImportFailure
	}
	return c.Catalog.Upsert(ctx, server)
}

func (c *failingAtomicImportCatalog) EnsureServers(_ context.Context, servers []*toolapi.MCPToolServer) error {
	c.batchSize = len(servers)
	return errInjectedImportFailure
}

func (c *failingAtomicImportCatalog) ApplyServers(_ context.Context, mutations []MCPToolServerMutation) error {
	c.batchSize = len(mutations)
	return errInjectedImportFailure
}

func TestApplicationServiceImportDeerFlowIsSingleBatchAndRollsBack(t *testing.T) {
	base := NewInMemoryCatalog()
	catalog := &failingAtomicImportCatalog{Catalog: base}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		IDGen:               &sequentialIDGen{next: 100},
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	_, err := svc.ImportDeerFlowExtensionsConfig(managementContext(7), 10, []byte(`{
		"mcpServers": {
			"github": {"enabled":true,"type":"stdio","command":"npx","args":["github"]},
			"postgres": {"enabled":true,"type":"stdio","command":"npx","args":["postgres"]}
		}
	}`))

	require.ErrorIs(t, err, errInjectedImportFailure)
	require.Equal(t, 2, catalog.batchSize)
	servers, listErr := base.List(context.Background(), 10)
	require.NoError(t, listErr)
	require.Empty(t, servers)
}

func TestApplicationServiceImportDeerFlowRefreshesTrustedCapabilities(t *testing.T) {
	catalog := NewInMemoryCatalog()
	github := managementServer(41, 10)
	github.Name = "github"
	github.Auth = `{}`
	github.Tools = []*toolapi.MCPToolDefinition{{Name: "stale-tool", InputSchema: `{}`}}
	github.Resources = []*toolapi.MCPResource{{Name: "stale-resource", URI: "https://stale.example.com"}}
	github.Prompts = []*toolapi.MCPPrompt{{Name: "stale-prompt", Arguments: []*toolapi.MCPPromptArgument{}}}
	require.NoError(t, catalog.Create(context.Background(), github))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		IDGen:               &sequentialIDGen{next: 100},
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	_, err := svc.ImportDeerFlowExtensionsConfig(managementContext(7), 10, []byte(`{
		"mcpServers": {
			"github": {
				"enabled": true,
				"type": "stdio",
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"]
			}
		}
	}`))

	require.NoError(t, err)
	stored, err := catalog.Get(context.Background(), github.ServerID)
	require.NoError(t, err)
	require.NotNil(t, toolByName(stored.Tools, "search_repositories"))
	require.NotContains(t, toolNames(stored.Tools), "stale-tool")
	require.Empty(t, stored.Resources)
	require.Empty(t, stored.Prompts)
}

func TestCatalogTrustedBatchCapabilityCASRollsBackOnVersionConflict(t *testing.T) {
	factories := map[string]func(*testing.T) Catalog{
		"memory": func(t *testing.T) Catalog {
			return NewInMemoryCatalog()
		},
		"mysql": func(t *testing.T) Catalog {
			db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			return NewMySQLCatalog(db)
		},
	}

	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			catalog := factory(t)
			github := managementServer(41, 10)
			github.Name = "github"
			github.Auth = `{}`
			github.UpdatedAt = 100
			postgres := managementServer(42, 10)
			postgres.Name = "postgres"
			postgres.Auth = `{}`
			postgres.UpdatedAt = 200
			require.NoError(t, catalog.Create(context.Background(), github))
			require.NoError(t, catalog.Create(context.Background(), postgres))
			githubBefore, err := catalog.Get(context.Background(), github.ServerID)
			require.NoError(t, err)

			githubUpdate := cloneServer(githubBefore)
			githubUpdate.Description = "must roll back"
			githubUpdate.Tools = []*toolapi.MCPToolDefinition{{Name: "new-tool", InputSchema: `{}`}}
			githubUpdate.Resources = []*toolapi.MCPResource{{Name: "new-resource", URI: "https://new.example.com"}}
			githubUpdate.Prompts = []*toolapi.MCPPrompt{{Name: "new-prompt", Arguments: []*toolapi.MCPPromptArgument{}}}
			githubUpdate.UpdatedAt = 101
			postgresUpdate := cloneServer(postgres)
			postgresUpdate.Description = "stale concurrent update"
			postgresUpdate.UpdatedAt = 201

			err = catalog.ApplyServers(context.Background(), []MCPToolServerMutation{
				{
					Server:            githubUpdate,
					ExpectedUpdatedAt: githubBefore.UpdatedAt,
					FieldMask: MCPToolServerMutationConnectionFields |
						MCPToolServerMutationCapabilityFields,
				},
				{
					Server:            postgresUpdate,
					ExpectedUpdatedAt: postgres.UpdatedAt - 1,
					FieldMask:         MCPToolServerMutationConnectionFields,
				},
			})

			require.ErrorIs(t, err, ErrMCPConflict)
			stored, getErr := catalog.Get(context.Background(), github.ServerID)
			require.NoError(t, getErr)
			require.NotEqual(t, "must roll back", stored.Description)
			require.Equal(t, githubBefore.Tools, stored.Tools)
			require.Equal(t, githubBefore.Resources, stored.Resources)
			require.Equal(t, githubBefore.Prompts, stored.Prompts)
		})
	}
}

func TestApplicationServicePublicUpsertCannotChangeCapabilities(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Auth = `{}`
	server.Resources = []*toolapi.MCPResource{{Name: "resource", URI: "https://example.com/resource"}}
	server.Prompts = []*toolapi.MCPPrompt{{Name: "prompt", Arguments: []*toolapi.MCPPromptArgument{}}}
	require.NoError(t, catalog.Create(context.Background(), server))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})
	req := validManagementUpsertRequest(server.ServerID, server.SpaceID)
	req.Name = server.Name
	req.Description = "allowed config update"
	req.ServerType = server.ServerType
	req.Config = server.Config
	req.Auth = mcpAuthConfiguredSentinel
	req.Tools = []*toolapi.MCPToolDefinition{{Name: "browser-tool", InputSchema: `{}`}}

	_, err := svc.UpsertServer(managementContext(7), req)

	require.NoError(t, err)
	stored, err := catalog.Get(context.Background(), server.ServerID)
	require.NoError(t, err)
	require.Equal(t, server.Tools, stored.Tools)
	require.Equal(t, server.Resources, stored.Resources)
	require.Equal(t, server.Prompts, stored.Prompts)
}
