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

func (c *getTrackingCatalog) Get(
	ctx context.Context,
	serverID int64,
) (*toolapi.MCPToolServer, error) {
	c.getCalls++
	return c.Catalog.Get(ctx, serverID)
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
