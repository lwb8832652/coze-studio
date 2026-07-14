// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestMCPEnabledDefaultSeedUsesEncryptedMySQLCatalog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	codec, err := NewAESMCPAuthCodec("0123456789abcdef")
	require.NoError(t, err)
	enabled := true
	svc := NewApplicationService(&Components{
		Enabled:                     &enabled,
		Catalog:                     NewMySQLCatalog(db, WithMySQLCatalogAuthCodec(codec)),
		IDGen:                       newAtomicIDGen(100),
		UserSpaceRoleReader:         ownerRoleReader(10),
		DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
	})

	_, err = svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})
	require.NoError(t, err)
	registry, err := svc.ListMCPToolRegistryEntriesForRuntime(context.Background(), 10)
	require.NoError(t, err)
	require.NotEmpty(t, registry)

	var defaults struct {
		Servers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(DefaultDeerFlowMCPConfigRaw(), &defaults))
	plaintextArgs := make([]string, 0)
	for _, server := range defaults.Servers {
		plaintextArgs = append(plaintextArgs, server.Args...)
	}
	require.NotEmpty(t, plaintextArgs)

	var rows []mcpToolServerPO
	require.NoError(t, db.Order("server_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	encryptedRows := 0
	for _, row := range rows {
		storedConfig := string(row.Config)
		storedAuth := string(row.Auth)
		require.NotContains(t, storedConfig, `"args"`)
		for _, plaintext := range plaintextArgs {
			if len(plaintext) < 8 {
				continue
			}
			require.NotContains(t, storedConfig, plaintext)
			require.NotContains(t, storedAuth, plaintext)
		}
		if storedAuth != "" && storedAuth != `{}` {
			encryptedRows++
			assertAESGCMEnvelope(t, storedAuth)
		}
	}
	require.Positive(t, encryptedRows)
}

func TestMCPDisabledRejectsManagementRuntimeAndDoesNotSeed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	existing := managementServer(100, 10)
	existing.Name = "existing-server"
	require.NoError(t, NewMySQLCatalog(db).Upsert(context.Background(), existing))

	reads := 0
	writes := 0
	countDatabaseCalls := true
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(
		"mcp_disabled_count_query",
		func(*gorm.DB) {
			if countDatabaseCalls {
				reads++
			}
		},
	))
	countWrite := func(*gorm.DB) {
		if countDatabaseCalls {
			writes++
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("mcp_disabled_count_create", countWrite))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("mcp_disabled_count_update", countWrite))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("mcp_disabled_count_delete", countWrite))
	disabled := false
	svc := NewApplicationService(&Components{
		Enabled:                     &disabled,
		Catalog:                     NewMySQLCatalog(db),
		IDGen:                       newAtomicIDGen(100),
		UserSpaceRoleReader:         ownerRoleReader(10),
		DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
	})

	_, err = svc.ListRegistryEntries(managementContext(7), nil)
	require.ErrorIs(t, err, ErrMCPDisabled)
	_, err = svc.ListRegistryEntries(managementContext(7), &toolapi.ListMCPToolRegistryEntriesRequest{SpaceID: 10})
	require.ErrorIs(t, err, ErrMCPDisabled)
	_, err = svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})
	require.ErrorIs(t, err, ErrMCPDisabled)
	_, err = svc.ListMCPToolRegistryEntriesForRuntime(context.Background(), 0)
	require.ErrorIs(t, err, ErrMCPDisabled)
	_, err = svc.UpsertServer(managementContext(7), validManagementUpsertRequest(0, 10))
	require.ErrorIs(t, err, ErrMCPDisabled)
	require.ErrorIs(t, svc.BindManagementRuntime(nil, nil), ErrMCPDisabled)
	require.Zero(t, reads)
	require.Zero(t, writes)

	countDatabaseCalls = false
	var count int64
	require.NoError(t, db.Model(&mcpToolServerPO{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
