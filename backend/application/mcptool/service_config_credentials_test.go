// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const testMCPConfigWriteOnlySentinel = "__COZE_MCP_WRITE_ONLY__"

func TestApplicationServicePublicConfigNeverLeaksCredentialShapes(t *testing.T) {
	ctx := managementContext(7)
	base := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Config = `{
		"url":"https://mcp.example.com/events",
		"headers":{
			"Authorization":"Bearer authorization-secret",
			"Cookie":"cookie-secret",
			"X-API-Key":"api-key-secret",
			"PAT":"pat-secret",
			"X-Custom":"custom-secret",
			"Nested":{"custom":"nested-secret"},
			"Array":["array-secret",{"custom":"object-secret"}]
		},
		"credentials":{"custom":"unknown-object-secret"},
		"metadata":{"token":"nested-token-secret"}
	}`
	require.NoError(t, base.Upsert(ctx, server))
	catalog := &controlledMigrationCatalog{
		Catalog:   base,
		updateErr: errors.New("migration storage unavailable"),
	}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	got, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.NoError(t, err)
	listed, err := svc.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: 10})
	require.NoError(t, err)
	exported, err := svc.SafeExport(ctx, 41)
	require.NoError(t, err)

	for _, publicConfig := range []string{got.Data.Config, listed.Data.Servers[0].Config} {
		config := decodeJSONObjectForCredentialTest(t, publicConfig)
		headers, ok := config["headers"].(map[string]any)
		require.True(t, ok)
		for _, key := range []string{"Authorization", "Cookie", "X-API-Key", "PAT", "X-Custom", "Nested", "Array"} {
			require.Equal(t, testMCPConfigWriteOnlySentinel, headers[key])
		}
		assertCredentialTestSecretsAbsent(t, publicConfig)
	}
	require.JSONEq(t, `{"url":"https://mcp.example.com/events"}`, exported.Config)
	require.NotContains(t, exported.Config, testMCPConfigWriteOnlySentinel)
	assertCredentialTestSecretsAbsent(t, exported.Config)
}

func TestApplicationServiceUpsertMigratesConfigCredentialsAndSupportsKeyedEdits(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	req := validManagementUpsertRequest(0, 10)
	req.Config = `{
		"url":"https://mcp.example.com/events",
		"headers":{"Authorization":"Bearer original","Cookie":"session=original"}
	}`
	req.Auth = `{"metadata":{"tenant":"tenant-a"}}`

	created, err := svc.UpsertServer(ctx, req)
	require.NoError(t, err)
	stored, err := catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"url":"https://mcp.example.com/events",
		"auth_headers":{"Authorization":"headers.Authorization","Cookie":"headers.Cookie"}
	}`, stored.Config)
	require.JSONEq(t, `{
		"metadata":{"tenant":"tenant-a"},
		"headers":{"Authorization":"Bearer original","Cookie":"session=original"}
	}`, stored.Auth)
	assertMaskedCredentialMap(t, created.Data.Config, "headers", []string{"Authorization", "Cookie"})
	require.JSONEq(t, mcpAuthConfiguredSentinel, created.Data.Auth)
	assertCredentialTestSecretsAbsent(t, created.Data.Config)

	preserve := validManagementUpsertRequest(100, 10)
	preserve.Config = created.Data.Config
	preserve.Auth = created.Data.Auth
	_, err = svc.UpsertServer(ctx, preserve)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.Contains(t, stored.Auth, "Bearer original")
	require.Contains(t, stored.Auth, "session=original")

	editConfig := decodeJSONObjectForCredentialTest(t, created.Data.Config)
	headers := editConfig["headers"].(map[string]any)
	headers["Authorization"] = "Bearer replacement"
	delete(headers, "Cookie")
	authHeaders := editConfig["auth_headers"].(map[string]any)
	delete(authHeaders, "Cookie")
	edit := validManagementUpsertRequest(100, 10)
	edit.Config = encodeJSONObjectForCredentialTest(t, editConfig)
	edit.Auth = mcpAuthConfiguredSentinel
	updated, err := svc.UpsertServer(ctx, edit)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"metadata":{"tenant":"tenant-a"},
		"headers":{"Authorization":"Bearer replacement"}
	}`, stored.Auth)
	require.NotContains(t, stored.Config, "Cookie")
	assertMaskedCredentialMap(t, updated.Data.Config, "headers", []string{"Authorization"})
	require.NotContains(t, updated.Data.Config, "Bearer replacement")

	clearConfig := decodeJSONObjectForCredentialTest(t, updated.Data.Config)
	clearConfig["headers"] = map[string]any{}
	clearConfig["auth_headers"] = map[string]any{}
	clear := validManagementUpsertRequest(100, 10)
	clear.Config = encodeJSONObjectForCredentialTest(t, clearConfig)
	clear.Auth = mcpAuthConfiguredSentinel
	_, err = svc.UpsertServer(ctx, clear)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.NotContains(t, stored.Auth, "headers")
	require.NotContains(t, stored.Config, "auth_headers")
}

func TestApplicationServiceEmptyConfigPreservesWriteOnlyReferences(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Config = `{"url":"https://mcp.example.com","auth_headers":{"Authorization":"headers.Authorization"}}`
	server.Auth = `{"headers":{"Authorization":"Bearer original"}}`
	require.NoError(t, catalog.Upsert(ctx, server))
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	req := validManagementUpsertRequest(41, 10)
	req.Config = ""
	req.Auth = mcpAuthConfiguredSentinel

	_, err := svc.UpsertServer(ctx, req)
	require.NoError(t, err)
	stored, err := catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.JSONEq(t, `{"auth_headers":{"Authorization":"headers.Authorization"}}`, stored.Config)
	require.JSONEq(t, `{"headers":{"Authorization":"Bearer original"}}`, stored.Auth)
}

func TestApplicationServiceUpsertRejectsAmbiguousCredentialMappings(t *testing.T) {
	tests := []struct {
		name   string
		config string
		auth   string
	}{
		{name: "nested header value", config: `{"url":"https://mcp.example.com","headers":{"Authorization":{"token":"nested-secret"}}}`, auth: `{}`},
		{name: "array env", config: `{"command":"npx","env":["array-secret"]}`, auth: `{}`},
		{name: "config and auth conflict", config: `{"url":"https://mcp.example.com","headers":{"Authorization":"config-secret"}}`, auth: `{"headers":{"Authorization":"auth-secret"}}`},
		{name: "mismatched reference", config: `{"url":"https://mcp.example.com","headers":{"Authorization":"config-secret"},"auth_headers":{"Authorization":"credentials.token"}}`, auth: `{}`},
		{name: "sentinel on create", config: `{"url":"https://mcp.example.com","headers":{"Authorization":"__COZE_MCP_WRITE_ONLY__"}}`, auth: `{}`},
		{name: "ambiguous dotted key", config: `{"url":"https://mcp.example.com","headers":{"X.Custom":"config-secret"}}`, auth: `{}`},
		{name: "unknown credential object", config: `{"url":"https://mcp.example.com","credentials":{"custom":"unknown-secret"}}`, auth: `{}`},
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
			req.Auth = tt.auth

			_, err := svc.UpsertServer(managementContext(7), req)
			require.Error(t, err)
			for _, secret := range []string{"nested-secret", "array-secret", "config-secret", "auth-secret", "unknown-secret"} {
				require.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestApplicationServiceLazyCanonicalizationUsesCASAndFailsClosedForRuntime(t *testing.T) {
	t.Run("CAS conflict rereads and runtime receives canonical credentials", func(t *testing.T) {
		ctx := managementContext(7)
		base := NewInMemoryCatalog()
		server := managementServer(41, 10)
		server.Config = `{"url":"https://mcp.example.com","headers":{"Authorization":"Bearer legacy"}}`
		require.NoError(t, base.Upsert(ctx, server))
		catalog := &controlledMigrationCatalog{Catalog: base, conflicts: 1}
		svc := NewApplicationService(&Components{Catalog: catalog})

		resolved, err := svc.ResolveADKMCPRuntimeServer(ctx, 41)
		require.NoError(t, err)
		require.GreaterOrEqual(t, catalog.getCalls, 2)
		require.JSONEq(t, `{"url":"https://mcp.example.com","auth_headers":{"Authorization":"headers.Authorization"}}`, resolved.Config)
		require.JSONEq(t, `{"headers":{"Authorization":"Bearer legacy"}}`, resolved.Auth)
		stored, err := base.Get(ctx, 41)
		require.NoError(t, err)
		require.NotContains(t, stored.Config, "Bearer legacy")
		require.Greater(t, stored.UpdatedAt, int64(100))
	})

	for _, endpoint := range []string{"resolve", "discover", "test_call"} {
		t.Run(endpoint+" fails closed when migration cannot persist", func(t *testing.T) {
			ctx := managementContext(7)
			base := NewInMemoryCatalog()
			server := managementServer(41, 10)
			server.Config = `{"url":"https://mcp.example.com","headers":{"Authorization":"Bearer runtime-secret"}}`
			require.NoError(t, base.Upsert(ctx, server))
			catalog := &controlledMigrationCatalog{Catalog: base, updateErr: errors.New("write failed")}
			executor := &managementRuntimeExecutor{result: &RuntimeToolResult{Status: "success"}}
			discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{}}
			svc := NewApplicationService(&Components{
				Catalog:              catalog,
				UserSpaceRoleReader:  ownerRoleReader(10),
				RuntimeExecutor:      executor,
				CapabilityDiscoverer: discoverer,
			})

			var err error
			switch endpoint {
			case "resolve":
				_, err = svc.ResolveADKMCPRuntimeServer(ctx, 41)
			case "discover":
				_, err = svc.Discover(ctx, 41)
			case "test_call":
				_, err = svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{ServerID: 41, ToolName: "search", Arguments: `{}`})
			}
			require.Error(t, err)
			require.NotContains(t, err.Error(), "runtime-secret")
			require.Empty(t, executor.recordedCalls())
			require.Empty(t, discoverer.recordedConnections())
		})
	}
}

func TestApplicationServicePublicReadRemainsSanitizedWhenLazyMigrationFails(t *testing.T) {
	ctx := managementContext(7)
	base := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Config = `{"url":"https://mcp.example.com","headers":{"Authorization":"Bearer public-secret"}}`
	require.NoError(t, base.Upsert(ctx, server))
	catalog := &controlledMigrationCatalog{Catalog: base, updateErr: errors.New("write failed")}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	got, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.NoError(t, err)
	listed, err := svc.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: 10})
	require.NoError(t, err)
	for _, config := range []string{got.Data.Config, listed.Data.Servers[0].Config} {
		require.NotContains(t, config, "public-secret")
		assertMaskedCredentialMap(t, config, "headers", []string{"Authorization"})
	}
}

type controlledMigrationCatalog struct {
	Catalog
	conflicts int
	updateErr error
	getCalls  int
}

func (c *controlledMigrationCatalog) Get(ctx context.Context, serverID int64) (*toolapi.MCPToolServer, error) {
	c.getCalls++
	return c.Catalog.Get(ctx, serverID)
}

func (c *controlledMigrationCatalog) UpdateServer(
	ctx context.Context,
	server *toolapi.MCPToolServer,
	expectedUpdatedAt int64,
) error {
	if c.conflicts > 0 {
		c.conflicts--
		return ErrMCPConflict
	}
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Catalog.UpdateServer(ctx, server, expectedUpdatedAt)
}

func decodeJSONObjectForCredentialTest(t *testing.T, raw string) map[string]any {
	t.Helper()
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &result))
	require.NotNil(t, result)
	return result
}

func encodeJSONObjectForCredentialTest(t *testing.T, value map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func assertMaskedCredentialMap(t *testing.T, raw, field string, keys []string) {
	t.Helper()
	config := decodeJSONObjectForCredentialTest(t, raw)
	values, ok := config[field].(map[string]any)
	require.True(t, ok, "%s must be an object in %s", field, raw)
	require.Len(t, values, len(keys))
	for _, key := range keys {
		require.Equal(t, testMCPConfigWriteOnlySentinel, values[key])
	}
}

func assertCredentialTestSecretsAbsent(t *testing.T, text string) {
	t.Helper()
	for _, forbidden := range []string{
		"authorization-secret", "cookie-secret", "api-key-secret", "pat-secret",
		"custom-secret", "nested-secret", "array-secret", "object-secret",
		"unknown-object-secret", "nested-token-secret",
	} {
		require.NotContains(t, strings.ToLower(text), strings.ToLower(forbidden))
	}
}
