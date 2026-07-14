// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestCatalogEnsureServersIsAtomicAndIdempotent(t *testing.T) {
	for _, factory := range catalogCASFactories() {
		t.Run(factory.name, func(t *testing.T) {
			catalog := factory.new(t)
			first := managementServer(100, 10)
			first.Name = "first"
			first.SourceType = toolapi.MCPServerSourceTypeOfficial
			second := managementServer(101, 10)
			second.Name = "second"
			second.SourceType = toolapi.MCPServerSourceTypeOfficial

			require.NoError(t, catalog.EnsureServers(context.Background(), []*toolapi.MCPToolServer{first, second}))
			duplicateFirst := cloneServer(first)
			duplicateFirst.ServerID = 200
			duplicateSecond := cloneServer(second)
			duplicateSecond.ServerID = 201
			require.NoError(t, catalog.EnsureServers(context.Background(), []*toolapi.MCPToolServer{duplicateFirst, duplicateSecond}))

			listed, err := catalog.List(context.Background(), 10)
			require.NoError(t, err)
			require.Len(t, listed, 2)
			ids := map[int64]struct{}{}
			for _, server := range listed {
				ids[server.ServerID] = struct{}{}
			}
			require.Contains(t, ids, int64(100))
			require.Contains(t, ids, int64(101))
		})
	}
}

func TestInMemoryCatalogEnsureServersRollsBackInvalidBatch(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(100, 10)

	err := catalog.EnsureServers(context.Background(), []*toolapi.MCPToolServer{server, nil})

	require.Error(t, err)
	listed, listErr := catalog.List(context.Background(), 10)
	require.NoError(t, listErr)
	require.Empty(t, listed)
}

func TestMySQLCatalogEnsureServersRollsBackEncodingFailure(t *testing.T) {
	dsn := fmt.Sprintf("file:ensure-rollback-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	catalog := NewMySQLCatalog(db)
	first := managementServer(100, 10)
	first.Name = "first"
	second := managementServer(101, 10)
	second.Name = "second"
	second.Auth = `{"token":"cannot-encode"}`

	err = catalog.EnsureServers(context.Background(), []*toolapi.MCPToolServer{first, second})

	require.Error(t, err)
	listed, listErr := catalog.List(context.Background(), 10)
	require.NoError(t, listErr)
	require.Empty(t, listed)
}

func TestApplicationServiceDefaultDeerFlowSeedIsConcurrentAndIdempotent(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog:                     catalog,
		IDGen:                       newAtomicIDGen(100),
		UserSpaceRoleReader:         ownerRoleReader(10),
		DefaultDeerFlowMCPConfigRaw: DefaultDeerFlowMCPConfigRaw(),
	})

	const callers = 8
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	listed, err := catalog.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	names := make(map[string]struct{}, len(listed))
	for _, server := range listed {
		names[server.Name] = struct{}{}
		require.Equal(t, toolapi.MCPServerSourceTypeOfficial, server.SourceType)
	}
	require.Len(t, names, 2)
	require.NotContains(t, names, "weather")
	require.NotContains(t, names, "openmeteo")
}

func TestApplicationServiceDefaultDeerFlowSeedRollsBackWholeBatch(t *testing.T) {
	dsn := fmt.Sprintf("file:seed-rollback-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpToolServerPO{}))
	catalog := NewMySQLCatalog(db)
	raw := []byte(`{
		"mcpServers": {
			"github": {
				"enabled": true,
				"type": "stdio",
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"],
				"env": {}
			},
			"weather": {
				"enabled": true,
				"type": "stdio",
				"command": "node",
				"args": ["-e", "process.exit(0)"],
				"env": {"PASSPHRASE": "batch-secret"}
			}
		}
	}`)
	svc := NewApplicationService(&Components{
		Catalog:                     catalog,
		IDGen:                       newAtomicIDGen(100),
		UserSpaceRoleReader:         ownerRoleReader(10),
		DefaultDeerFlowMCPConfigRaw: raw,
	})

	_, err = svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})

	require.Error(t, err)
	listed, listErr := catalog.List(context.Background(), 10)
	require.NoError(t, listErr)
	require.Empty(t, listed)
}

type atomicIDGen struct {
	next atomic.Int64
}

func newAtomicIDGen(first int64) *atomicIDGen {
	gen := &atomicIDGen{}
	gen.next.Store(first)
	return gen
}

func (g *atomicIDGen) GenID(context.Context) (int64, error) {
	return g.next.Add(1) - 1, nil
}

func (g *atomicIDGen) GenMultiIDs(ctx context.Context, count int) ([]int64, error) {
	ids := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
