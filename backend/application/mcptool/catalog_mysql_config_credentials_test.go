// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestMySQLServiceStoresConfigCredentialsOnlyAsEncryptedAuth(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})

	remoteReq := validManagementUpsertRequest(0, 10)
	remoteReq.Name = "remote-credentials"
	remoteReq.Config = `{"url":"https://mcp.example.com/events","headers":{"Authorization":"Bearer database-secret","X-Custom":"custom-database-secret"}}`
	remote, err := svc.UpsertServer(managementContext(7), remoteReq)
	require.NoError(t, err)
	stdioReq := validManagementUpsertRequest(0, 10)
	stdioReq.Name = "stdio-credentials"
	stdioReq.ServerType = "stdio"
	stdioReq.Config = `{"command":"npx","args":["server"],"env":{"MCP_TOKEN":"stdio-database-secret"}}`
	stdio, err := svc.UpsertServer(managementContext(7), stdioReq)
	require.NoError(t, err)

	for _, serverID := range []int64{remote.Data.ServerID, stdio.Data.ServerID} {
		var po mcpToolServerPO
		require.NoError(t, db.Where("server_id = ?", serverID).First(&po).Error)
		require.NotContains(t, string(po.Config), "database-secret")
		require.NotContains(t, string(po.Config), `"headers"`)
		require.NotContains(t, string(po.Config), `"env"`)
		require.NotContains(t, string(po.Auth), "database-secret")
		assertAESGCMEnvelope(t, string(po.Auth))
	}

	resolvedRemote, err := svc.ResolveADKMCPRuntimeServer(context.Background(), remote.Data.ServerID)
	require.NoError(t, err)
	policy := mcpruntime.Policy{
		RemoteEnabled:        true,
		RemoteAllowedHosts:   []string{"mcp.example.com"},
		RemoteMaxConfigBytes: 64 * 1024,
		RemoteMaxHeaders:     16,
		RemoteMaxHeaderBytes: 16 * 1024,
		HTTPTimeout:          2 * time.Second,
	}
	remoteConnection, err := policy.ParseRemote(mcpruntime.Connection{
		ServerType: resolvedRemote.ServerType,
		Config:     resolvedRemote.Config,
		Auth:       resolvedRemote.Auth,
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer database-secret", remoteConnection.Headers["Authorization"])
	require.Equal(t, "custom-database-secret", remoteConnection.Headers["X-Custom"])

	resolvedStdio, err := svc.ResolveADKMCPRuntimeServer(context.Background(), stdio.Data.ServerID)
	require.NoError(t, err)
	stdioConnection, err := mcpruntime.ParseStdioConnection(mcpruntime.Connection{
		ServerType: resolvedStdio.ServerType,
		Config:     resolvedStdio.Config,
		Auth:       resolvedStdio.Auth,
	}, 64*1024)
	require.NoError(t, err)
	require.Equal(t, "stdio-database-secret", stdioConnection.Env["MCP_TOKEN"])
}

func TestMySQLCatalogRejectsPlaintextConfigCredentialsOnDirectWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	server := mcpServerForAuthTest(100, `{}`)
	server.Config = `{"command":"npx","env":{"TOKEN":"direct-write-secret"}}`

	err = catalog.Upsert(context.Background(), server)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "direct-write-secret")
	var count int64
	require.NoError(t, db.Model(&mcpToolServerPO{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestMySQLLegacyConfigCredentialsLazyMigrateWithCASAndEncryption(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	require.NoError(t, db.Create(&mcpToolServerPO{
		ServerID:   100,
		SpaceID:    10,
		CreatorID:  7,
		SourceType: string(toolapi.MCPServerSourceTypeCustom),
		Name:       "legacy-config-credentials",
		ServerType: "stdio",
		Enabled:    true,
		Config:     datatypes.JSON(`{"command":"npx","args":["server"],"env":{"MCP_TOKEN":"legacy-config-secret"}}`),
		Auth:       datatypes.JSON(`{}`),
		Tools:      datatypes.JSON(`[]`),
		Resources:  datatypes.JSON(`[]`),
		Prompts:    datatypes.JSON(`[]`),
		CreatedAt:  10,
		UpdatedAt:  20,
	}).Error)
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
	svc := NewApplicationService(&Components{Catalog: catalog})

	resolved, err := svc.ResolveADKMCPRuntimeServer(context.Background(), 100)
	require.NoError(t, err)
	stdioConnection, err := mcpruntime.ParseStdioConnection(mcpruntime.Connection{
		ServerType: resolved.ServerType,
		Config:     resolved.Config,
		Auth:       resolved.Auth,
	}, 64*1024)
	require.NoError(t, err)
	require.Equal(t, "legacy-config-secret", stdioConnection.Env["MCP_TOKEN"])

	var po mcpToolServerPO
	require.NoError(t, db.Where("server_id = ?", 100).First(&po).Error)
	require.NotContains(t, string(po.Config), "legacy-config-secret")
	require.JSONEq(t, `{"command":"npx","auth_args":{"0":"args.0"},"auth_env":{"MCP_TOKEN":"env.MCP_TOKEN"}}`, string(po.Config))
	require.NotContains(t, string(po.Auth), "legacy-config-secret")
	assertAESGCMEnvelope(t, string(po.Auth))
	require.Greater(t, po.UpdatedAt, int64(20))
}
