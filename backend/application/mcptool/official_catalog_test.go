/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

package mcptool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestOfficialMCPCatalogMigratesExistingWorkspaceServerWithoutLeakingCredentials(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(ctx, &toolapi.MCPToolServer{
		ServerID: 22, SpaceID: 10, CreatorID: 7,
		SourceType: toolapi.MCPServerSourceTypeCustom,
		Name:       "postgres", ServerType: "stdio", Config: `{}`,
		CreatedAt: 1, UpdatedAt: 1,
	}))
	enabled := true
	service := NewApplicationService(&Components{
		Enabled: &enabled, Catalog: catalog, UserSpaceRoleReader: ownerRoleReader(10),
	})

	listed, err := service.ListOfficialCatalog(ctx, &toolapi.ListMCPOfficialCatalogRequest{SpaceID: 10})
	require.NoError(t, err)
	require.True(t, listed.Data.CanManage)
	require.Equal(t, toolapi.MCPOfficialInstallStatusNeedsMigration, listed.Data.Entries[1].InstallStatus)

	secretURL := "postgresql://reader:private-password@db.internal:5432/app"
	installed, err := service.InstallOfficialCatalogEntry(ctx, &toolapi.InstallMCPOfficialCatalogRequest{
		CatalogID: officialMCPCatalogPostgres, SpaceID: 10,
		Credentials: map[string]string{"database_url": secretURL},
	})
	require.NoError(t, err)
	require.Equal(t, int64(22), installed.Data.ServerID)
	require.Equal(t, toolapi.MCPServerSourceTypeOfficial, installed.Data.SourceType)
	responseJSON, err := json.Marshal(installed)
	require.NoError(t, err)
	require.NotContains(t, string(responseJSON), "private-password")

	stored, err := catalog.Get(context.Background(), 22)
	require.NoError(t, err)
	require.Equal(t, toolapi.MCPServerSourceTypeOfficial, stored.SourceType)
	require.False(t, stored.Enabled)
}

func TestOfficialMCPConfigRejectsInvalidOrMissingCredentials(t *testing.T) {
	github := findOfficialMCPCatalogDefinition(officialMCPCatalogGitHub)
	_, err := buildOfficialMCPConfig(github, nil)
	require.Error(t, err)

	postgres := findOfficialMCPCatalogDefinition(officialMCPCatalogPostgres)
	_, err = buildOfficialMCPConfig(postgres, map[string]string{"database_url": "https://example.com/db"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "https://example.com/db")
}
