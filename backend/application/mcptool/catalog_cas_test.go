// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestCatalogCapabilityCASRejectsConcurrentEditAndDelete(t *testing.T) {
	for _, factory := range catalogCASFactories() {
		t.Run(factory.name, func(t *testing.T) {
			catalog := factory.new(t)
			ctx := context.Background()
			server := managementServer(41, 10)
			server.UpdatedAt = 100
			require.NoError(t, catalog.Upsert(ctx, server))

			err := catalog.UpdateCapabilities(ctx, 41, 10, 100, 101, MCPToolCapabilitySnapshot{
				Tools: []*toolapi.MCPToolDefinition{{
					Name: "forecast", Description: "Forecast", InputSchema: `{"type":"object"}`,
				}},
				Resources: []*toolapi.MCPResource{{URI: "resource://guide", Name: "guide"}},
				Prompts:   []*toolapi.MCPPrompt{{Name: "summarize", Arguments: []*toolapi.MCPPromptArgument{}}},
			})
			require.NoError(t, err)
			updated, err := catalog.Get(ctx, 41)
			require.NoError(t, err)
			require.Equal(t, int64(101), updated.UpdatedAt)
			require.Equal(t, server.Config, updated.Config)
			require.Equal(t, "forecast", updated.Tools[0].Name)

			err = catalog.UpdateCapabilities(ctx, 41, 10, 100, 102, MCPToolCapabilitySnapshot{
				Tools: []*toolapi.MCPToolDefinition{{Name: "stale", InputSchema: `{}`}},
			})
			require.ErrorIs(t, err, ErrMCPConflict)
			unchanged, err := catalog.Get(ctx, 41)
			require.NoError(t, err)
			require.Equal(t, "forecast", unchanged.Tools[0].Name)

			unchanged.Config = `{"url":"https://edited.example.com"}`
			unchanged.UpdatedAt = 102
			require.NoError(t, catalog.Upsert(ctx, unchanged))
			err = catalog.UpdateCapabilities(ctx, 41, 10, 101, 103, MCPToolCapabilitySnapshot{
				Tools: []*toolapi.MCPToolDefinition{{Name: "lost-update", InputSchema: `{}`}},
			})
			require.ErrorIs(t, err, ErrMCPConflict)
			edited, err := catalog.Get(ctx, 41)
			require.NoError(t, err)
			require.JSONEq(t, `{"url":"https://edited.example.com"}`, edited.Config)
			require.Equal(t, "forecast", edited.Tools[0].Name)

			require.NoError(t, catalog.Delete(ctx, 41))
			err = catalog.UpdateCapabilities(ctx, 41, 10, 102, 104, MCPToolCapabilitySnapshot{})
			require.ErrorIs(t, err, ErrMCPConflict)
			_, err = catalog.Get(ctx, 41)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestCatalogEnabledCASUpdatesOnlyEnabledAndNeverRevivesDeletedRows(t *testing.T) {
	for _, factory := range catalogCASFactories() {
		t.Run(factory.name, func(t *testing.T) {
			catalog := factory.new(t)
			ctx := context.Background()
			server := managementServer(42, 10)
			server.SourceType = toolapi.MCPServerSourceTypeOfficial
			server.UpdatedAt = 200
			require.NoError(t, catalog.Upsert(ctx, server))

			require.NoError(t, catalog.UpdateEnabled(ctx, 42, 10, 200, 201, false))
			updated, err := catalog.Get(ctx, 42)
			require.NoError(t, err)
			require.False(t, updated.Enabled)
			require.Equal(t, int64(201), updated.UpdatedAt)
			require.Equal(t, server.Config, updated.Config)
			require.Equal(t, server.Auth, updated.Auth)

			err = catalog.UpdateEnabled(ctx, 42, 10, 200, 202, true)
			require.ErrorIs(t, err, ErrMCPConflict)
			require.NoError(t, catalog.Delete(ctx, 42))
			err = catalog.UpdateEnabled(ctx, 42, 10, 201, 203, true)
			require.ErrorIs(t, err, ErrMCPConflict)
			_, err = catalog.Get(ctx, 42)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestCatalogHealthUpdateIsMonotonicAndVersionAware(t *testing.T) {
	for _, factory := range catalogCASFactories() {
		t.Run(factory.name, func(t *testing.T) {
			catalog := factory.new(t)
			ctx := context.Background()
			server := managementServer(43, 10)
			server.UpdatedAt = 300
			require.NoError(t, catalog.Upsert(ctx, server))

			require.NoError(t, catalog.UpdateHealth(ctx, 43, 300, MCPToolHealthSnapshot{
				Status: "healthy", CheckedAt: 500, LatencyMs: 12,
			}))
			require.NoError(t, catalog.UpdateHealth(ctx, 43, 300, MCPToolHealthSnapshot{
				Status: "unhealthy", CheckedAt: 400, LatencyMs: 99, Error: "stale",
			}))
			got, err := catalog.Get(ctx, 43)
			require.NoError(t, err)
			require.Equal(t, "healthy", got.HealthStatus)
			require.Equal(t, int64(500), got.HealthCheckedAt)
			require.Equal(t, int64(12), got.HealthLatencyMs)
			require.Equal(t, int64(300), got.UpdatedAt)

			got.Config = `{"url":"https://new-config.example.com"}`
			got.UpdatedAt = 301
			require.NoError(t, catalog.Upsert(ctx, got))
			err = catalog.UpdateHealth(ctx, 43, 300, MCPToolHealthSnapshot{
				Status: "unhealthy", CheckedAt: 600, Error: "old-config",
			})
			require.ErrorIs(t, err, ErrMCPConflict)
			versioned, err := catalog.Get(ctx, 43)
			require.NoError(t, err)
			require.Equal(t, "healthy", versioned.HealthStatus)
			require.Equal(t, int64(500), versioned.HealthCheckedAt)
			require.Equal(t, int64(301), versioned.UpdatedAt)
		})
	}
}

type catalogCASFactory struct {
	name string
	new  func(t *testing.T) Catalog
}

func catalogCASFactories() []catalogCASFactory {
	return []catalogCASFactory{
		{name: "memory", new: func(t *testing.T) Catalog {
			return NewInMemoryCatalog()
		}},
		{name: "mysql", new: func(t *testing.T) Catalog {
			dsn := fmt.Sprintf("file:catalog-cas-%s?mode=memory&cache=shared", t.Name())
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			return NewMySQLCatalog(db)
		}},
	}
}
