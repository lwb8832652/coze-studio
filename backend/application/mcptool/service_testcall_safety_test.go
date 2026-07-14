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

func TestApplicationServiceRuntimeTestCallReturnsOnlySafeSummary(t *testing.T) {
	ctx := managementContext(7)
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	executor := &managementRuntimeExecutor{result: &RuntimeToolResult{
		Status: "success",
		Output: `{
			"content":"raw provider content",
			"credential":"runtime-secret",
			"provider_body":{"token":"provider-token"}
		}`,
		LatencyMs: 17,
	}}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
	})

	response, err := svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.NoError(t, err)
	require.Equal(t, "success", response.Data.Status)
	require.Equal(t, int64(17), response.Data.LatencyMs)
	require.JSONEq(t, `{"result":"completed"}`, response.Data.Output)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"raw provider content", "runtime-secret", "provider-token", "provider_body", "credential",
	} {
		require.NotContains(t, string(encoded), forbidden)
	}
}
