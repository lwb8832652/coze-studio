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

package agentthread

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestADKMCPRuntimeToolCatalogDisabledByDefault(t *testing.T) {
	registry := &recordingADKMCPToolRegistry{
		entries: []*toolapi.MCPToolRegistryEntry{
			{
				Name:        "mcp_100_search_docs",
				Source:      "mcp",
				Category:    "mcp",
				Visibility:  "static",
				ServerID:    100,
				ServerName:  "docs-mcp",
				ToolName:    "search-docs",
				Description: "Search internal documentation.",
				Enabled:     true,
			},
		},
	}
	catalog := NewADKMCPRuntimeToolCatalog(registry)

	definitions, err := catalog.LoadADKRuntimeTools(
		context.Background(),
		&RunSummary{RunID: 20, SpaceID: 30},
	)

	require.NoError(t, err)
	require.Empty(t, definitions)
	require.Zero(t, registry.listCalls)
}

func TestADKMCPRuntimeToolCatalogLoadsDeferredMetadataOnlyAndFailsClosed(
	t *testing.T,
) {
	registry := &recordingADKMCPToolRegistry{
		entries: []*toolapi.MCPToolRegistryEntry{
			{
				Name:        "mcp_100_search_docs",
				Source:      "mcp",
				Category:    "mcp",
				Visibility:  "static",
				ServerID:    100,
				ServerName:  "docs-mcp",
				ToolName:    "search-docs",
				Description: "Search internal documentation.",
				Enabled:     true,
			},
			{
				Name:        "not-mcp",
				Source:      "workflow",
				Description: "Wrong source.",
				Enabled:     true,
			},
			{
				Name:        "mcp_101_disabled",
				Source:      "mcp",
				Description: "Disabled tool.",
				Enabled:     false,
			},
			{
				Name:        "bad-name",
				Source:      "mcp",
				Description: "Invalid Eino tool name.",
				Enabled:     true,
			},
		},
	}
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKMCPRuntimeToolCatalog(registry),
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:   20,
			SpaceID: 30,
			Config: `{
				"mcp_tools":{
					"enabled":true,
					"allowed_tools":["mcp_100_search_docs"]
				}
			}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(30), registry.spaceID)
	require.Empty(t, set.StaticTools)
	require.Len(t, set.DynamicTools, 1)

	info, err := set.DynamicTools[0].Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "mcp_100_search_docs", info.Name)
	require.Equal(t, "Search internal documentation.", info.Desc)
	require.Nil(t, info.ParamsOneOf)

	invokable, ok := set.DynamicTools[0].(tool.InvokableTool)
	require.True(t, ok)
	result, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"secret customer path"}`,
	)
	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime tool execution is not enabled")
	require.NotContains(t, err.Error(), "secret customer path")
	require.NotContains(t, err.Error(), "docs-mcp")
}

func TestADKMCPRuntimeToolCatalogInvokesExecutorWithSafeIdentity(t *testing.T) {
	executor := &recordingADKMCPRuntimeToolExecutor{
		result: `{"schema":"coze.mcp_tool_result.v1","content":"ok"}`,
	}
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKMCPRuntimeToolCatalog(
			&recordingADKMCPToolRegistry{
				entries: []*toolapi.MCPToolRegistryEntry{
					{
						Name:        "mcp_100_search_docs",
						Source:      "mcp",
						ServerID:    100,
						ServerName:  "docs-mcp",
						ToolName:    "search-docs",
						Description: "Search internal documentation.",
						Enabled:     true,
					},
				},
			},
			WithADKMCPRuntimeToolExecutor(executor),
		),
	)
	run := &RunSummary{
		RunID:    20,
		ThreadID: 10,
		SpaceID:  30,
		Config:   `{"mcp_tools":{"enabled":true}}`,
	}
	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	invokable := requireADKInvokableTool(
		t,
		context.Background(),
		set.DynamicTools,
		"mcp_100_search_docs",
	)

	result, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"coze studio"}`,
	)

	require.NoError(t, err)
	require.Equal(t, executor.result, result)
	require.Equal(t, int64(20), executor.call.Run.RunID)
	require.Equal(t, "mcp_100_search_docs", executor.call.Name)
	require.Equal(t, int64(100), executor.call.ServerID)
	require.Equal(t, "search-docs", executor.call.ToolName)
	require.Equal(t, `{"query":"coze studio"}`, executor.call.Arguments)
}

func TestADKMCPRuntimeToolCatalogSanitizesExecutorErrors(t *testing.T) {
	executor := &recordingADKMCPRuntimeToolExecutor{
		err: errString(
			`dial docs-mcp failed with arguments {"query":"secret customer path"}`,
		),
	}
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKMCPRuntimeToolCatalog(
			&recordingADKMCPToolRegistry{
				entries: []*toolapi.MCPToolRegistryEntry{
					{
						Name:        "mcp_100_search_docs",
						Source:      "mcp",
						ServerID:    100,
						ServerName:  "docs-mcp",
						ToolName:    "search-docs",
						Description: "Search internal documentation.",
						Enabled:     true,
					},
				},
			},
			WithADKMCPRuntimeToolExecutor(executor),
		),
	)
	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:   20,
			SpaceID: 30,
			Config:  `{"mcp_tools":{"enabled":true}}`,
		},
	)
	require.NoError(t, err)
	invokable := requireADKInvokableTool(
		t,
		context.Background(),
		set.DynamicTools,
		"mcp_100_search_docs",
	)

	result, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"secret customer path"}`,
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime tool execution failed")
	require.Contains(t, err.Error(), "mcp_100_search_docs")
	require.NotContains(t, err.Error(), "secret customer path")
	require.NotContains(t, err.Error(), "docs-mcp")
}

func TestDefaultADKToolProviderCanWireMCPRegistry(t *testing.T) {
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(
		nil,
		WithDefaultADKToolProviderMCPRegistry(&recordingADKMCPToolRegistry{
			entries: []*toolapi.MCPToolRegistryEntry{
				{
					Name:        "mcp_100_search_docs",
					Source:      "mcp",
					ServerID:    100,
					ToolName:    "search-docs",
					Description: "Search internal documentation.",
					Enabled:     true,
				},
			},
		}),
	)
	setProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)

	set, err := setProvider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:   20,
			SpaceID: 30,
			Config:  `{"mcp_tools":{"enabled":true}}`,
		},
	)

	require.NoError(t, err)
	require.Contains(
		t,
		adkToolNames(t, context.Background(), set.DynamicTools),
		"mcp_100_search_docs",
	)
}

func TestDefaultADKToolProviderCanWireMCPExecutor(t *testing.T) {
	executor := &recordingADKMCPRuntimeToolExecutor{result: "mcp-result"}
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(
		nil,
		WithDefaultADKToolProviderMCPRegistry(&recordingADKMCPToolRegistry{
			entries: []*toolapi.MCPToolRegistryEntry{
				{
					Name:        "mcp_100_search_docs",
					Source:      "mcp",
					ServerID:    100,
					ToolName:    "search-docs",
					Description: "Search internal documentation.",
					Enabled:     true,
				},
			},
		}),
		WithDefaultADKToolProviderMCPExecutor(executor),
	)
	setProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)
	set, err := setProvider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:   20,
			SpaceID: 30,
			Config:  `{"mcp_tools":{"enabled":true}}`,
		},
	)
	require.NoError(t, err)
	invokable := requireADKInvokableTool(
		t,
		context.Background(),
		set.DynamicTools,
		"mcp_100_search_docs",
	)

	result, err := invokable.InvokableRun(context.Background(), `{"query":"docs"}`)

	require.NoError(t, err)
	require.Equal(t, "mcp-result", result)
	require.Equal(t, int64(100), executor.call.ServerID)
	require.Equal(t, "search-docs", executor.call.ToolName)
}

type recordingADKMCPToolRegistry struct {
	entries   []*toolapi.MCPToolRegistryEntry
	spaceID   int64
	listCalls int
}

func (r *recordingADKMCPToolRegistry) ListMCPToolRegistryEntries(
	ctx context.Context,
	spaceID int64,
) ([]*toolapi.MCPToolRegistryEntry, error) {
	r.spaceID = spaceID
	r.listCalls++
	return r.entries, nil
}

type recordingADKMCPRuntimeToolExecutor struct {
	call   ADKMCPRuntimeToolCall
	result string
	err    error
}

func (e *recordingADKMCPRuntimeToolExecutor) InvokeADKMCPRuntimeTool(
	ctx context.Context,
	call ADKMCPRuntimeToolCall,
) (string, error) {
	e.call = call
	if e.err != nil {
		return "", e.err
	}
	return e.result, nil
}

type errString string

func (e errString) Error() string {
	return string(e)
}
