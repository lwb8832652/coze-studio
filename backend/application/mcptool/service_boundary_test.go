// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

type getTrackingCatalog struct {
	Catalog
	getCalls int
}

type credentialSafeListCatalog struct {
	Catalog
	strictCalls int
	safeCalls   int
	safeServers []*toolapi.MCPToolServer
}

func (c *getTrackingCatalog) Get(
	ctx context.Context,
	serverID int64,
) (*toolapi.MCPToolServer, error) {
	c.getCalls++
	return c.Catalog.Get(ctx, serverID)
}

func (c *credentialSafeListCatalog) List(
	ctx context.Context,
	spaceID int64,
) ([]*toolapi.MCPToolServer, error) {
	c.strictCalls++
	return nil, errors.New("mcp tool auth decode failed")
}

func (c *credentialSafeListCatalog) ListForManagement(
	ctx context.Context,
	spaceID int64,
) ([]*toolapi.MCPToolServer, error) {
	c.safeCalls++
	return c.safeServers, nil
}

func TestApplicationServiceRuntimeRegistryWorksWithoutUserSession(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	require.NoError(t, catalog.Upsert(context.Background(), server))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	entries, err := svc.ListMCPToolRegistryEntriesForRuntime(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "mcp_41_search", entries[0].Name)
	_, err = svc.ListMCPToolRegistryEntries(context.Background(), 10)
	require.ErrorIs(t, err, ErrMCPUnauthenticated)
}

func TestApplicationServiceRegistryUsesCredentialSafeCatalogListing(t *testing.T) {
	unreadable := managementServer(41, 10)
	unreadable.Enabled = false
	unreadable.HealthStatus = mcpToolHealthStatusUnhealthy
	unreadable.HealthError = "credential_unavailable"
	healthy := managementServer(42, 10)
	catalog := &credentialSafeListCatalog{
		Catalog:     NewInMemoryCatalog(),
		safeServers: []*toolapi.MCPToolServer{unreadable, healthy},
	}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})

	runtimeEntries, err := svc.ListMCPToolRegistryEntriesForRuntime(
		context.Background(),
		10,
	)
	require.NoError(t, err)
	require.NotEmpty(t, runtimeEntries)
	for _, entry := range runtimeEntries {
		require.Equal(t, int64(42), entry.ServerID)
	}

	userEntries, err := svc.ListMCPToolRegistryEntries(managementContext(7), 10)
	require.NoError(t, err)
	require.NotEmpty(t, userEntries)
	for _, entry := range userEntries {
		require.Equal(t, int64(42), entry.ServerID)
	}

	require.Zero(t, catalog.strictCalls)
	require.Equal(t, 2, catalog.safeCalls)
}

func TestRuntimeDefaultMCPInitializationUsesCredentialSafeListing(t *testing.T) {
	raw := DefaultDeerFlowMCPConfigRaw()
	config, err := parseDeerFlowExtensionsConfig(raw)
	require.NoError(t, err)

	safeServers := make([]*toolapi.MCPToolServer, 0, len(config.MCPServers))
	serverID := int64(100)
	for name := range config.MCPServers {
		server := managementServer(serverID, 10)
		server.Name = name
		server.Enabled = false
		safeServers = append(safeServers, server)
		serverID++
	}
	catalog := &credentialSafeListCatalog{
		Catalog:     NewInMemoryCatalog(),
		safeServers: safeServers,
	}
	svc := NewApplicationService(&Components{
		Catalog:                     catalog,
		DefaultDeerFlowMCPConfigRaw: raw,
	})

	err = svc.ensureDefaultDeerFlowMCPServersForRuntime(context.Background(), 10)

	require.NoError(t, err)
	require.Zero(t, catalog.strictCalls)
	require.Equal(t, 1, catalog.safeCalls)
}

func TestApplicationServiceAuthorizationChecksRequestedSpaceBeforeUpdateLookup(t *testing.T) {
	tracked := &getTrackingCatalog{Catalog: NewInMemoryCatalog()}
	svc := NewApplicationService(&Components{
		Catalog: tracked,
		UserSpaceRoleReader: &managementRoleReader{spaces: []*userentity.Space{
			{ID: 10, RoleType: spaceRoleOwner},
		}},
	})
	req := validManagementUpsertRequest(41, 20)

	_, err := svc.UpsertServer(managementContext(7), req)

	require.ErrorIs(t, err, ErrMCPForbidden)
	require.Zero(t, tracked.getCalls)
}

func TestApplicationServiceAuthorizationHidesMissingAndCrossSpaceServers(t *testing.T) {
	catalog := NewInMemoryCatalog()
	crossSpace := managementServer(41, 20)
	require.NoError(t, catalog.Upsert(context.Background(), crossSpace))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})
	ctx := managementContext(7)

	_, err := svc.UpsertServer(ctx, validManagementUpsertRequest(404, 10))
	require.ErrorIs(t, err, ErrMCPForbidden)
	_, err = svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 404})
	require.ErrorIs(t, err, ErrMCPForbidden)
	_, err = svc.DeleteServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 404})
	require.ErrorIs(t, err, ErrMCPForbidden)
	_, err = svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.ErrorIs(t, err, ErrMCPForbidden)
	_, err = svc.DeleteServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.ErrorIs(t, err, ErrMCPForbidden)

	for _, got := range []error{err} {
		require.True(t, errors.Is(got, ErrMCPForbidden))
		require.False(t, errors.Is(got, ErrNotFound))
	}
}

func TestApplicationServiceAuthorizationMemberCannotTriggerLazyDefaultSeed(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog:                     catalog,
		IDGen:                       &sequentialIDGen{next: 100},
		UserSpaceRoleReader:         &managementRoleReader{spaces: []*userentity.Space{{ID: 10, RoleType: 3}}},
		DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
	})

	_, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})

	require.ErrorIs(t, err, ErrMCPForbidden)
	stored, listErr := catalog.List(context.Background(), 10)
	require.NoError(t, listErr)
	require.Empty(t, stored)
}
