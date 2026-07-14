// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestMySQLStdioArgsPersistOnlyInEncryptedAuthAndResolveInOrder(t *testing.T) {
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
	req := validManagementUpsertRequest(0, 10)
	req.Name = "encrypted-args"
	req.ServerType = "stdio"
	req.Config = `{"command":"npx","args":["--api-key","mysql-arg-secret","postgresql://user:password@db/app"]}`

	created, err := svc.UpsertServer(managementContext(7), req)
	require.NoError(t, err)
	var po mcpToolServerPO
	require.NoError(t, db.Where("server_id = ?", created.Data.ServerID).First(&po).Error)
	for _, secret := range []string{"--api-key", "mysql-arg-secret", "password"} {
		require.NotContains(t, string(po.Config), secret)
		require.NotContains(t, string(po.Auth), secret)
	}
	assertAESGCMEnvelope(t, string(po.Auth))

	resolved, err := svc.ResolveADKMCPRuntimeServer(context.Background(), created.Data.ServerID)
	require.NoError(t, err)
	stdio, err := mcpruntime.ParseStdioConnection(mcpruntime.Connection{
		ServerType: resolved.ServerType,
		Config:     resolved.Config,
		Auth:       resolved.Auth,
	}, 64*1024)
	require.NoError(t, err)
	require.Equal(t, []string{"--api-key", "mysql-arg-secret", "postgresql://user:password@db/app"}, stdio.Args)
}

func TestMySQLCatalogStrictConfigRejectsUnknownFieldsAndPlaintextArgs(t *testing.T) {
	for name, config := range map[string]string{
		"unknown remote field": `{"url":"https://mcp.example.com","custom":"secret"}`,
		"remote query":         `{"url":"https://mcp.example.com?token=secret"}`,
		"stdio plaintext args": `{"command":"npx","args":["secret"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
			codec, err := NewAESMCPAuthCodec("0123456789abcdef")
			require.NoError(t, err)
			catalog := NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec))
			server := mcpServerForAuthTest(100, `{}`)
			server.Config = config
			if name == "stdio plaintext args" {
				server.ServerType = "stdio"
			}

			err = catalog.Upsert(context.Background(), server)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}
