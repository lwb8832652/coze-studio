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

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestMySQLCatalogManagementProjectionRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	catalog := NewMySQLCatalog(db)
	server := &toolapi.MCPToolServer{
		ServerID:    100,
		SpaceID:     1,
		CreatorID:   42,
		SourceType:  toolapi.MCPServerSourceTypeOfficial,
		Name:        "official-docs",
		Description: "Official documentation MCP",
		ServerType:  "streamable_http",
		Enabled:     true,
		Config:      `{"url":"https://mcp.example.com"}`,
		Auth:        `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search_docs",
				Description: "Search documentation.",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
		},
		Resources: []*toolapi.MCPResource{
			{
				URI:         "docs://handbook",
				Name:        "handbook",
				Description: "Product handbook.",
				MIMEType:    "text/markdown",
			},
		},
		Prompts: []*toolapi.MCPPrompt{
			{
				Name:        "summarize_docs",
				Description: "Summarize documentation.",
				Arguments: []*toolapi.MCPPromptArgument{
					{Name: "topic", Description: "Topic to summarize.", Required: true},
				},
			},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}

	require.NoError(t, catalog.Upsert(context.Background(), server))

	got, err := catalog.Get(context.Background(), server.ServerID)
	require.NoError(t, err)
	require.Equal(t, server.CreatorID, got.CreatorID)
	require.Equal(t, server.SourceType, got.SourceType)
	require.Equal(t, server.Tools, got.Tools)
	require.Equal(t, server.Resources, got.Resources)
	require.Equal(t, server.Prompts, got.Prompts)

	listed, err := catalog.List(context.Background(), server.SpaceID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, server.CreatorID, listed[0].CreatorID)
	require.Equal(t, server.SourceType, listed[0].SourceType)
	require.Equal(t, server.Tools, listed[0].Tools)
	require.Equal(t, server.Resources, listed[0].Resources)
	require.Equal(t, server.Prompts, listed[0].Prompts)
}

func TestMySQLCatalogManagementDefaultsLegacyProjection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	catalog := NewMySQLCatalog(db)
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		Name:       "legacy-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{}`,
		CreatedAt:  10,
		UpdatedAt:  20,
	}))

	got, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, toolapi.MCPServerSourceTypeCustom, got.SourceType)
	require.NotNil(t, got.Tools)
	require.Empty(t, got.Tools)
	require.NotNil(t, got.Resources)
	require.Empty(t, got.Resources)
	require.NotNil(t, got.Prompts)
	require.Empty(t, got.Prompts)

	require.NoError(t, db.Model(&mcpToolServerPO{}).
		Where("server_id = ?", 100).
		Update("source_type", "").Error)
	got, err = catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, toolapi.MCPServerSourceTypeCustom, got.SourceType)
}

func TestMCPManagementResponseMasksAuthAfterCatalogRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	server := &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		CreatorID:  42,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "private-docs",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{"type":"bearer","token":"raw-secret-token","nested":{"api_key":"raw-secret-key"}}`,
		Resources: []*toolapi.MCPResource{
			{URI: "docs://private", Name: "private-docs", MIMEType: "text/markdown"},
		},
		Prompts: []*toolapi.MCPPrompt{
			{Name: "summarize", Arguments: []*toolapi.MCPPromptArgument{}},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}
	require.NoError(t, catalog.Upsert(context.Background(), server))

	stored, err := catalog.Get(context.Background(), server.ServerID)
	require.NoError(t, err)
	require.JSONEq(t, server.Auth, stored.Auth)

	response := cloneServerForResponse(stored)
	require.JSONEq(t, mcpAuthConfiguredSentinel, response.Auth)
	require.JSONEq(t, server.Auth, stored.Auth)
	require.Equal(t, server.CreatorID, response.CreatorID)
	require.Equal(t, server.SourceType, response.SourceType)
	require.Len(t, response.Resources, 1)
	require.Empty(t, response.Resources[0].URI)
	require.NotEmpty(t, response.Resources[0].ResourceID)
	require.Equal(t, server.Resources[0].Name, response.Resources[0].Name)
	require.Equal(t, server.Resources[0].MIMEType, response.Resources[0].MIMEType)
	require.Equal(t, server.Prompts, response.Prompts)
}
