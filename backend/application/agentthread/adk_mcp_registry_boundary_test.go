// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

type runtimeOnlyMCPRegistry struct {
	spaceID int64
	calls   int
}

func (r *runtimeOnlyMCPRegistry) ListMCPToolRegistryEntriesForRuntime(
	_ context.Context,
	spaceID int64,
) ([]*toolapi.MCPToolRegistryEntry, error) {
	r.spaceID = spaceID
	r.calls++
	return []*toolapi.MCPToolRegistryEntry{}, nil
}

func TestADKMCPRuntimeToolCatalogUsesRuntimeOnlyRegistryWithoutSession(t *testing.T) {
	registry := &runtimeOnlyMCPRegistry{}
	catalog := NewADKMCPRuntimeToolCatalog(registry)

	tools, err := catalog.LoadADKRuntimeTools(context.Background(), &RunSummary{
		SpaceID: 30,
		Config:  `{"mcp_tools":{"enabled":true}}`,
	})

	require.NoError(t, err)
	require.Empty(t, tools)
	require.Equal(t, 1, registry.calls)
	require.Equal(t, int64(30), registry.spaceID)
}
