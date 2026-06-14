/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mcptool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestApplicationServiceUpsertsListsGetsAndTestsMCPServer(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})

	upserted, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     1,
		Name:        "browser-tools",
		Description: "Browser automation tools",
		ServerType:  "stdio",
		Enabled:     true,
		Config:      `{"command":"npx","args":["-y","@example/browser"]}`,
		Auth:        `{"type":"none"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search",
				Description: "Search the web",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(100), upserted.Data.ServerID)
	require.Equal(t, "browser-tools", upserted.Data.Name)
	require.Equal(t, "search", upserted.Data.Tools[0].Name)

	listed, err := svc.ListServers(context.Background(), &toolapi.ListMCPToolServersRequest{SpaceID: 1})
	require.NoError(t, err)
	require.Len(t, listed.Data.Servers, 1)
	require.Equal(t, int64(100), listed.Data.Servers[0].ServerID)

	got, err := svc.GetServer(context.Background(), &toolapi.GetMCPToolServerRequest{ServerID: 100})
	require.NoError(t, err)
	require.Equal(t, "stdio", got.Data.ServerType)

	tested, err := svc.TestCall(context.Background(), &toolapi.TestMCPToolCallRequest{
		ServerID:  100,
		ToolName:  "search",
		Arguments: `{"query":"coze studio"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "success", tested.Data.Status)
	require.GreaterOrEqual(t, tested.Data.LatencyMs, int64(0))

	var output map[string]any
	require.NoError(t, json.Unmarshal([]byte(tested.Data.Output), &output))
	require.Equal(t, "browser-tools", output["server_name"])
	require.Equal(t, "search", output["tool_name"])
	require.Equal(t, map[string]any{"query": "coze studio"}, output["arguments"])
}

func TestApplicationServiceRejectsInvalidMCPConfigJSON(t *testing.T) {
	svc := NewApplicationService(&Components{
		Catalog: NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 101},
	})

	_, err := svc.UpsertServer(context.Background(), &toolapi.UpsertMCPToolServerRequest{
		SpaceID:    1,
		Name:       "bad-tools",
		ServerType: "stdio",
		Config:     `{"command":`,
		Auth:       `{}`,
		Tools: []*toolapi.MCPToolDefinition{
			{Name: "search", InputSchema: `{"type":"object"}`},
		},
	})

	require.ErrorContains(t, err, "invalid config json")
	require.True(t, IsClientError(err))
}

type sequentialIDGen struct {
	next int64
}

func (g *sequentialIDGen) GenID(ctx context.Context) (int64, error) {
	id := g.next
	g.next++

	return id, nil
}

func (g *sequentialIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, 0, counts)
	for i := 0; i < counts; i++ {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}
