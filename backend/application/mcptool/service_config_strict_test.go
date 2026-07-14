// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestApplicationServiceRejectsNonCanonicalRemoteConfig(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "userinfo", config: `{"url":"https://user:password@mcp.example.com/path"}`},
		{name: "query", config: `{"url":"https://mcp.example.com/path?api_key=query-secret"}`},
		{name: "fragment", config: `{"url":"https://mcp.example.com/path#fragment-secret"}`},
		{name: "unknown scalar", config: `{"url":"https://mcp.example.com/path","timeout_token":"unknown-secret"}`},
		{name: "unknown auth object", config: `{"url":"https://mcp.example.com/path","auth":{"custom":"auth-secret"}}`},
		{name: "unknown oauth object", config: `{"url":"https://mcp.example.com/path","oauth":{"custom":"oauth-secret"}}`},
		{name: "unknown credential object", config: `{"url":"https://mcp.example.com/path","credential":{"custom":"credential-secret"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewApplicationService(&Components{
				Catalog:             NewInMemoryCatalog(),
				IDGen:               &sequentialIDGen{next: 100},
				UserSpaceRoleReader: ownerRoleReader(10),
			})
			req := validManagementUpsertRequest(0, 10)
			req.Config = tt.config

			_, err := svc.UpsertServer(managementContext(7), req)
			require.Error(t, err)
			for _, secret := range []string{"password", "query-secret", "fragment-secret", "unknown-secret", "auth-secret", "oauth-secret", "credential-secret"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestApplicationServiceLegacyInvalidRemoteConfigIsSanitizedAndRuntimeFailsClosed(t *testing.T) {
	ctx := managementContext(7)
	base := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Config = `{
		"url":"https://user:password@mcp.example.com/path?api_key=query-secret#fragment-secret",
		"timeout":"unknown-scalar-secret",
		"oauth":{"custom":"oauth-object-secret"}
	}`
	require.NoError(t, base.Upsert(ctx, server))
	catalog := &controlledMigrationCatalog{Catalog: base, updateErr: errors.New("must not update invalid remote")}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	got, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.NoError(t, err)
	require.JSONEq(t, `{"url":"https://mcp.example.com/path"}`, got.Data.Config)
	exported, err := svc.SafeExport(ctx, 41)
	require.NoError(t, err)
	require.JSONEq(t, `{"url":"https://mcp.example.com/path"}`, exported.Config)
	for _, text := range []string{got.Data.Config, exported.Config} {
		for _, secret := range []string{"user", "password", "query-secret", "fragment-secret", "unknown-scalar-secret", "oauth-object-secret"} {
			require.NotContains(t, text, secret)
		}
	}

	_, err = svc.ResolveADKMCPRuntimeServer(ctx, 41)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "query-secret")
}

func TestApplicationServiceStdioArgsAndEnvAreEncryptedWriteOnlyReferences(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	req := validManagementUpsertRequest(0, 10)
	req.ServerType = "stdio"
	req.Config = `{
		"command":"npx",
		"args":["-y","private-package","--api-key","arg-secret","postgresql://user:password@db.example.com/app"],
		"env":{"MCP_TOKEN":"env-secret"}
	}`

	created, err := svc.UpsertServer(ctx, req)
	require.NoError(t, err)
	stored, err := catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"command":"npx",
		"auth_args":{"0":"args.0","1":"args.1","2":"args.2","3":"args.3","4":"args.4"},
		"auth_env":{"MCP_TOKEN":"env.MCP_TOKEN"}
	}`, stored.Config)
	require.JSONEq(t, `{
		"args":{"0":"-y","1":"private-package","2":"--api-key","3":"arg-secret","4":"postgresql://user:password@db.example.com/app"},
		"env":{"MCP_TOKEN":"env-secret"}
	}`, stored.Auth)
	publicConfig := decodeJSONObjectForCredentialTest(t, created.Data.Config)
	require.NotContains(t, publicConfig, "args")
	require.Contains(t, publicConfig, "auth_args")
	for _, secret := range []string{"private-package", "arg-secret", "password", "env-secret"} {
		require.NotContains(t, created.Data.Config, secret)
	}

	exported, err := svc.SafeExport(ctx, 100)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"command":"npx",
		"auth_args":{"0":"args.0","1":"args.1","2":"args.2","3":"args.3","4":"args.4"},
		"auth_env":{"MCP_TOKEN":"env.MCP_TOKEN"}
	}`, exported.Config)
	require.NotContains(t, exported.Config, testMCPConfigWriteOnlySentinel)

	roundTrip := validManagementUpsertRequest(100, 10)
	roundTrip.ServerType = "stdio"
	roundTrip.Config = created.Data.Config
	roundTrip.Auth = created.Data.Auth
	_, err = svc.UpsertServer(ctx, roundTrip)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.Contains(t, stored.Auth, "arg-secret")
}

func TestApplicationServiceRejectsAmbiguousStdioArgMappingsAndUnknownFields(t *testing.T) {
	tests := []struct {
		name   string
		config string
		auth   string
	}{
		{name: "unknown field", config: `{"command":"npx","custom":"unknown-secret"}`, auth: `{}`},
		{name: "reference hole", config: `{"command":"npx","auth_args":{"1":"args.1"}}`, auth: `{"args":{"1":"secret"}}`},
		{name: "wrong reference", config: `{"command":"npx","auth_args":{"0":"credentials.token"}}`, auth: `{"args":{"0":"secret"}}`},
		{name: "array reference mapping", config: `{"command":"npx","auth_args":["args.0"]}`, auth: `{"args":{"0":"secret"}}`},
		{name: "raw and reference conflict", config: `{"command":"npx","args":["new-secret"],"auth_args":{"0":"args.0"}}`, auth: `{"args":{"0":"old-secret"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewApplicationService(&Components{
				Catalog:             NewInMemoryCatalog(),
				IDGen:               &sequentialIDGen{next: 100},
				UserSpaceRoleReader: ownerRoleReader(10),
			})
			req := validManagementUpsertRequest(0, 10)
			req.ServerType = "stdio"
			req.Config = tt.config
			req.Auth = tt.auth

			_, err := svc.UpsertServer(managementContext(7), req)
			require.Error(t, err)
			for _, secret := range []string{"unknown-secret", "new-secret", "old-secret"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestApplicationServiceLegacyStdioArgsLazyMigrateWithCAS(t *testing.T) {
	ctx := managementContext(7)
	base := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.ServerType = "stdio"
	server.Config = `{"command":"npx","args":["--api-key","legacy-arg-secret","postgresql://user:password@db/app"]}`
	require.NoError(t, base.Upsert(ctx, server))
	catalog := &controlledMigrationCatalog{Catalog: base, conflicts: 1}
	svc := NewApplicationService(&Components{Catalog: catalog})

	resolved, err := svc.ResolveADKMCPRuntimeServer(ctx, 41)
	require.NoError(t, err)
	require.GreaterOrEqual(t, catalog.getCalls, 2)
	require.NotContains(t, resolved.Config, "legacy-arg-secret")
	require.Contains(t, resolved.Config, `"auth_args"`)
	require.Contains(t, resolved.Auth, "legacy-arg-secret")
	stored, err := base.Get(ctx, 41)
	require.NoError(t, err)
	require.NotContains(t, stored.Config, "password")
	require.Greater(t, stored.UpdatedAt, int64(100))
}

func TestApplicationServiceActualDefaultSeedIsSafeForListAndRuntimeRegistry(t *testing.T) {
	newService := func() (*ApplicationService, *InMemoryCatalog) {
		catalog := NewInMemoryCatalog()
		return NewApplicationService(&Components{
			Catalog:                     catalog,
			IDGen:                       &sequentialIDGen{next: 100},
			UserSpaceRoleReader:         ownerRoleReader(10),
			DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
		}), catalog
	}

	svc, catalog := newService()
	listed, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})
	require.NoError(t, err)
	require.NotEmpty(t, listed.Data.Servers)
	var github *toolapi.MCPToolServer
	for _, server := range listed.Data.Servers {
		if server.Name == "github" {
			github = server
			break
		}
	}
	require.NotNil(t, github)
	require.NotContains(t, github.Config, `"args"`)
	require.Contains(t, github.Config, `"auth_args"`)
	require.NotContains(t, github.Config, `"env"`)
	stored, err := catalog.Get(context.Background(), github.ServerID)
	require.NoError(t, err)
	require.NotContains(t, stored.Config, "@modelcontextprotocol/server-github")
	require.Contains(t, stored.Auth, "@modelcontextprotocol/server-github")

	runtimeSVC, _ := newService()
	entries, err := runtimeSVC.ListMCPToolRegistryEntriesForRuntime(context.Background(), 10)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
}
