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
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestMySQLCatalogPersistsMCPServers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	catalog := NewMySQLCatalog(db)
	server := &toolapi.MCPToolServer{
		ServerID:    100,
		SpaceID:     1,
		Name:        "docs-mcp",
		Description: "Documentation MCP",
		ServerType:  "stdio",
		Enabled:     true,
		Config:      `{"command":"npx"}`,
		Auth:        `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search-docs",
				Description: "Search docs.",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
		},
		HealthStatus:    "healthy",
		HealthCheckedAt: 30,
		HealthLatencyMs: 12,
		HealthError:     "",
		CreatedAt:       10,
		UpdatedAt:       20,
	}

	require.NoError(t, catalog.Upsert(context.Background(), server))
	got, err := catalog.Get(context.Background(), 100)

	require.NoError(t, err)
	require.Equal(t, server.ServerID, got.ServerID)
	require.Equal(t, server.SpaceID, got.SpaceID)
	require.Equal(t, server.Name, got.Name)
	require.Equal(t, server.Config, got.Config)
	require.Equal(t, server.Auth, got.Auth)
	require.Equal(t, server.Tools, got.Tools)
	require.Equal(t, "healthy", got.HealthStatus)
	require.Equal(t, int64(30), got.HealthCheckedAt)
	require.Equal(t, int64(12), got.HealthLatencyMs)

	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   101,
		SpaceID:    1,
		Name:       "newer-mcp",
		ServerType: "sse",
		Enabled:    false,
		Config:     `{}`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
		CreatedAt: 30,
		UpdatedAt: 40,
	}))

	listed, err := catalog.List(context.Background(), 1)

	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.Equal(t, int64(101), listed[0].ServerID)
	require.Equal(t, int64(100), listed[1].ServerID)
}

func TestMySQLCatalogEncodesAuthAtRest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	codec := &recordingMCPAuthCodec{
		encoded: `{"_coze_mcp_auth":{"version":"test","nonce":"test","ciphertext":"encoded-auth"}}`,
		decoded: `{"token":"raw-secret-token"}`,
	}
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	server := &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		Name:       "docs-mcp",
		ServerType: "stdio",
		Enabled:    true,
		Config:     `{}`,
		Auth:       `{"token":"raw-secret-token"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
		},
		CreatedAt: 10,
		UpdatedAt: 20,
	}

	require.NoError(t, catalog.Upsert(context.Background(), server))

	var po mcpToolServerPO
	require.NoError(t, db.Where("server_id = ?", 100).First(&po).Error)
	require.JSONEq(t, codec.encoded, string(po.Auth))
	require.NotContains(t, string(po.Auth), "raw-secret-token")
	require.Equal(t, server.Auth, codec.encodedInput)

	got, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.JSONEq(t, `{"token":"raw-secret-token"}`, got.Auth)
	require.Equal(t, codec.encoded, codec.decodedInput)

	listed, err := catalog.List(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.JSONEq(t, `{"token":"raw-secret-token"}`, listed[0].Auth)
}

func TestMySQLCatalogSanitizesAuthCodecErrors(t *testing.T) {
	t.Run("encode", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		catalog := NewMySQLCatalog(
			db,
			WithMySQLCatalogAuthCodec(&recordingMCPAuthCodec{
				encodeErr: fmt.Errorf("encode raw-secret-token failed"),
			}),
		)

		err = catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
			ServerID:   100,
			SpaceID:    1,
			Name:       "docs-mcp",
			ServerType: "stdio",
			Enabled:    true,
			Config:     `{}`,
			Auth:       `{"token":"raw-secret-token"}`,
			Tools: []*toolapi.MCPToolDefinition{
				{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
			},
			CreatedAt: 10,
			UpdatedAt: 20,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "mcp tool auth encode failed")
		require.NotContains(t, err.Error(), "raw-secret-token")
	})

	t.Run("decode", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
		catalog := NewMySQLCatalog(
			db,
			WithMySQLCatalogAuthCodec(&recordingMCPAuthCodec{
				encoded:   `{"_coze_mcp_auth":{"version":"test","nonce":"test","ciphertext":"encoded-auth"}}`,
				decodeErr: fmt.Errorf("decode raw-secret-token failed"),
			}),
		)
		require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
			ServerID:   100,
			SpaceID:    1,
			Name:       "docs-mcp",
			ServerType: "stdio",
			Enabled:    true,
			Config:     `{}`,
			Auth:       `{"token":"raw-secret-token"}`,
			Tools: []*toolapi.MCPToolDefinition{
				{Name: "search", Description: "Search.", InputSchema: `{"type":"object"}`},
			},
			CreatedAt: 10,
			UpdatedAt: 20,
		}))

		_, err = catalog.Get(context.Background(), 100)

		require.Error(t, err)
		require.Contains(t, err.Error(), "mcp tool auth decode failed")
		require.NotContains(t, err.Error(), "raw-secret-token")
	})
}

func TestMySQLCatalogReturnsErrNotFound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	_, err = NewMySQLCatalog(db).Get(context.Background(), 404)

	require.True(t, errors.Is(err, ErrNotFound))
}

type recordingMCPAuthCodec struct {
	encoded      string
	decoded      string
	encodedInput string
	decodedInput string
	encodeErr    error
	decodeErr    error
}

func (c *recordingMCPAuthCodec) EncodeMCPAuth(
	ctx context.Context,
	auth string,
) (string, error) {
	c.encodedInput = auth
	if c.encodeErr != nil {
		return "", c.encodeErr
	}
	if c.encoded != "" {
		return c.encoded, nil
	}
	payload, err := json.Marshal(map[string]string{"encoded": auth})
	if err != nil {
		return "", err
	}

	return string(payload), nil
}

func (c *recordingMCPAuthCodec) DecodeMCPAuth(
	ctx context.Context,
	stored string,
) (string, error) {
	c.decodedInput = stored
	if c.decodeErr != nil {
		return "", c.decodeErr
	}
	if c.decoded != "" {
		return c.decoded, nil
	}

	return stored, nil
}

func TestMySQLCatalogSoftDeletesMCPServer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	catalog := NewMySQLCatalog(db)
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		Name:       "delete-me",
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

	require.NoError(t, catalog.Delete(context.Background(), 100))

	_, err = catalog.Get(context.Background(), 100)
	require.True(t, errors.Is(err, ErrNotFound))

	listed, err := catalog.List(context.Background(), 1)
	require.NoError(t, err)
	require.Empty(t, listed)

	var po mcpToolServerPO
	require.NoError(t, db.Unscoped().Where("server_id = ?", 100).First(&po).Error)
	require.Greater(t, po.DeletedAt, int64(0))
}

func TestMySQLCatalogUpdatesMCPServerHealth(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))

	catalog := NewMySQLCatalog(db)
	require.NoError(t, catalog.Upsert(context.Background(), &toolapi.MCPToolServer{
		ServerID:   100,
		SpaceID:    1,
		Name:       "health-mcp",
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

	require.NoError(t, catalog.UpdateHealth(context.Background(), 100, 0, MCPToolHealthSnapshot{
		Status:    "healthy",
		CheckedAt: 40,
		LatencyMs: 11,
		Error:     "",
	}))

	got, err := catalog.Get(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, "healthy", got.HealthStatus)
	require.Equal(t, int64(40), got.HealthCheckedAt)
	require.Equal(t, int64(11), got.HealthLatencyMs)
	require.Empty(t, got.HealthError)
}
