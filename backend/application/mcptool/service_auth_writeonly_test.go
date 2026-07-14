// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestApplicationServiceAuthIsOpaqueWriteOnlyForEveryLeafShape(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Auth = `{
		"cookie":"cookie-secret",
		"pat":"pat-secret",
		"passphrase":"passphrase-secret",
		"nested":{"custom":"nested-secret"},
		"items":["array-secret",{"unknown":"object-secret"}]
	}`
	require.NoError(t, catalog.Upsert(ctx, server))
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})

	got, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.NoError(t, err)
	require.JSONEq(t, `{"configured":true}`, got.Data.Auth)
	listed, err := svc.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: 10})
	require.NoError(t, err)
	require.JSONEq(t, `{"configured":true}`, listed.Data.Servers[0].Auth)

	encoded, err := json.Marshal([]any{got.Data, listed.Data.Servers[0]})
	require.NoError(t, err)
	for _, forbidden := range []string{
		"cookie-secret", "pat-secret", "passphrase-secret", "nested-secret",
		"array-secret", "object-secret", "cookie", "pat", "passphrase", "items",
	} {
		require.NotContains(t, string(encoded), forbidden)
	}
}

func TestApplicationServiceAuthSentinelPreservesAndEveryOtherObjectReplaces(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Auth = `{"cookie":"original-secret","items":["original-array-secret"]}`
	require.NoError(t, catalog.Upsert(ctx, server))
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})

	preserve := validManagementUpsertRequest(41, 10)
	preserve.Auth = `{"configured":true}`
	_, err := svc.UpsertServer(ctx, preserve)
	require.NoError(t, err)
	stored, err := catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.JSONEq(t, server.Auth, stored.Auth)

	replace := validManagementUpsertRequest(41, 10)
	replace.Auth = `{"configured": true}`
	_, err = svc.UpsertServer(ctx, replace)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.JSONEq(t, `{"configured":true}`, stored.Auth)

	replace.Auth = `{"configured":true,"cookie":"replacement-secret"}`
	_, err = svc.UpsertServer(ctx, replace)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.JSONEq(t, replace.Auth, stored.Auth)

	clear := validManagementUpsertRequest(41, 10)
	clear.Auth = `{}`
	_, err = svc.UpsertServer(ctx, clear)
	require.NoError(t, err)
	stored, err = catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, stored.Auth)
	response, err := svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, response.Data.Auth)

	invalid := validManagementUpsertRequest(41, 10)
	invalid.Auth = `["not-an-object"]`
	_, err = svc.UpsertServer(ctx, invalid)
	require.Error(t, err)
}

func TestApplicationServiceAuthCreateResponseNeverEchoesSubmittedSecret(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog:              NewInMemoryCatalog(),
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	req := validManagementUpsertRequest(0, 10)
	req.Auth = `{"arbitrary":[{"credential":"create-secret"}]}`

	created, err := svc.UpsertServer(managementContext(7), req)

	require.NoError(t, err)
	require.JSONEq(t, `{"configured":true}`, created.Data.Auth)
	require.NotContains(t, created.Data.Auth, "create-secret")
}

var _ = context.Background
