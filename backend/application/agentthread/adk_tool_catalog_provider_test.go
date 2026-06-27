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
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKRuntimeToolCatalogProviderPartitionsAndInvokesTools(t *testing.T) {
	catalog := &recordingADKRuntimeToolCatalog{
		definitions: []ADKRuntimeToolDefinition{
			{
				Name:        "search_docs",
				Description: "Search task documents.",
				InputSchema: `{
					"type":"object",
					"properties":{"query":{"type":"string"}},
					"required":["query"]
				}`,
				Visibility: ADKRuntimeToolVisibilityStatic,
				Invoker: ADKRuntimeToolInvokerFunc(func(
					ctx context.Context,
					call ADKRuntimeToolCall,
				) (string, error) {
					return fmt.Sprintf("static:%d:%s", call.Run.RunID, call.Arguments), nil
				}),
			},
			{
				Name:        "mcp__repo__read_file",
				Description: "Read a repository file.",
				InputSchema: `{
					"type":"object",
					"properties":{"path":{"type":"string"}},
					"required":["path"]
				}`,
				Visibility: ADKRuntimeToolVisibilityDeferred,
				Invoker: ADKRuntimeToolInvokerFunc(func(
					ctx context.Context,
					call ADKRuntimeToolCall,
				) (string, error) {
					return fmt.Sprintf("deferred:%d:%s", call.Run.RunID, call.Arguments), nil
				}),
			},
		},
	}
	provider := NewADKRuntimeToolCatalogProvider(catalog)
	run := &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30}

	set, err := provider.ResolveToolSet(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, run, catalog.run)
	require.Len(t, set.StaticTools, 1)
	require.Len(t, set.DynamicTools, 1)

	staticInfo, err := set.StaticTools[0].Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "search_docs", staticInfo.Name)
	require.NotNil(t, staticInfo.ParamsOneOf)
	staticSchema, err := staticInfo.ParamsOneOf.ToJSONSchema()
	require.NoError(t, err)
	require.NotNil(t, staticSchema)

	dynamicInfo, err := set.DynamicTools[0].Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "mcp__repo__read_file", dynamicInfo.Name)

	invokable, ok := set.DynamicTools[0].(tool.InvokableTool)
	require.True(t, ok)
	output, err := invokable.InvokableRun(context.Background(), `{"path":"README.md"}`)
	require.NoError(t, err)
	require.Equal(t, `deferred:20:{"path":"README.md"}`, output)
}

func TestADKRuntimeToolCatalogProviderRejectsDuplicateNamesAcrossPartitions(t *testing.T) {
	provider := NewADKRuntimeToolCatalogProvider(&recordingADKRuntimeToolCatalog{
		definitions: []ADKRuntimeToolDefinition{
			{
				Name:        "shared_tool",
				Description: "Static shared tool.",
				Visibility:  ADKRuntimeToolVisibilityStatic,
				Invoker: ADKRuntimeToolInvokerFunc(func(
					context.Context,
					ADKRuntimeToolCall,
				) (string, error) {
					return "static", nil
				}),
			},
			{
				Name:        "shared_tool",
				Description: "Deferred shared tool.",
				Visibility:  ADKRuntimeToolVisibilityDeferred,
				Invoker: ADKRuntimeToolInvokerFunc(func(
					context.Context,
					ADKRuntimeToolCall,
				) (string, error) {
					return "deferred", nil
				}),
			},
		},
	})

	_, err := provider.ResolveToolSet(context.Background(), &RunSummary{RunID: 20})

	require.ErrorContains(t, err, "duplicate eino adk runtime tool name: shared_tool")
}

func TestADKCompositeRuntimeToolCatalogLoadsAllCatalogs(t *testing.T) {
	first := &recordingADKRuntimeToolCatalog{
		definitions: []ADKRuntimeToolDefinition{
			{
				Name:        "first_tool",
				Description: "First runtime tool.",
				Invoker: ADKRuntimeToolInvokerFunc(func(
					context.Context,
					ADKRuntimeToolCall,
				) (string, error) {
					return "first", nil
				}),
			},
		},
	}
	second := &recordingADKRuntimeToolCatalog{
		definitions: []ADKRuntimeToolDefinition{
			{
				Name:        "second_tool",
				Description: "Second runtime tool.",
				Invoker: ADKRuntimeToolInvokerFunc(func(
					context.Context,
					ADKRuntimeToolCall,
				) (string, error) {
					return "second", nil
				}),
			},
		},
	}
	run := &RunSummary{RunID: 20}

	definitions, err := NewADKCompositeRuntimeToolCatalog(
		first,
		nil,
		second,
	).LoadADKRuntimeTools(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, run, first.run)
	require.Equal(t, run, second.run)
	require.Len(t, definitions, 2)
	require.Equal(t, []string{"first_tool", "second_tool"}, []string{
		definitions[0].Name,
		definitions[1].Name,
	})
}

func TestADKAgentFactoryUsesToolSetProviderOnce(t *testing.T) {
	staticTool, err := toolutils.InferTool(
		"static_tool",
		"Static tool.",
		func(context.Context, struct{}) (string, error) {
			return "static", nil
		},
	)
	require.NoError(t, err)
	dynamicTool, err := toolutils.InferTool(
		"dynamic_tool",
		"Dynamic tool.",
		func(context.Context, struct{}) (string, error) {
			return "dynamic", nil
		},
	)
	require.NoError(t, err)

	toolProvider := &recordingADKToolSetProvider{
		set: ADKToolSet{
			StaticTools:  []tool.BaseTool{staticTool},
			DynamicTools: []tool.BaseTool{dynamicTool},
		},
	}
	var gotStaticTools []tool.BaseTool
	var gotDynamicTools []tool.BaseTool
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &recordingChatModel{resp: schema.AssistantMessage("done", nil)}, true, nil
		},
		toolProvider,
		ADKMiddlewareFactoryFunc(func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			gotStaticTools = input.StaticTools
			gotDynamicTools = input.DynamicTools
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(context.Background(), &RunSummary{RunID: 20})

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.Equal(t, 1, toolProvider.resolveToolSetCalls)
	require.Zero(t, toolProvider.resolveToolsCalls)
	require.Zero(t, toolProvider.resolveDynamicToolsCalls)
	require.Equal(t, []tool.BaseTool{staticTool}, gotStaticTools)
	require.Equal(t, []tool.BaseTool{dynamicTool}, gotDynamicTools)
}

func TestADKHumanInteractionToolProviderPreservesToolSetProviderDynamicTools(t *testing.T) {
	staticTool, err := toolutils.InferTool(
		"static_tool",
		"Static tool.",
		func(context.Context, struct{}) (string, error) {
			return "static", nil
		},
	)
	require.NoError(t, err)
	dynamicTool, err := toolutils.InferTool(
		"dynamic_tool",
		"Dynamic tool.",
		func(context.Context, struct{}) (string, error) {
			return "dynamic", nil
		},
	)
	require.NoError(t, err)
	base := &recordingADKToolSetProvider{
		set: ADKToolSet{
			StaticTools:  []tool.BaseTool{staticTool},
			DynamicTools: []tool.BaseTool{dynamicTool},
		},
	}
	provider := NewADKHumanInteractionToolProvider(base)
	setProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)

	set, err := setProvider.ResolveToolSet(context.Background(), &RunSummary{RunID: 20})

	require.NoError(t, err)
	require.Equal(t, 1, base.resolveToolSetCalls)
	require.Len(t, set.StaticTools, 3)
	require.Equal(t, []tool.BaseTool{dynamicTool}, set.DynamicTools)
	names := make([]string, 0, len(set.StaticTools))
	for _, item := range set.StaticTools {
		info, infoErr := item.Info(context.Background())
		require.NoError(t, infoErr)
		names = append(names, info.Name)
	}
	require.ElementsMatch(t, []string{
		"static_tool",
		adkClarificationToolName,
		adkConfirmationToolName,
	}, names)
}

type recordingADKRuntimeToolCatalog struct {
	definitions []ADKRuntimeToolDefinition
	run         *RunSummary
}

func (c *recordingADKRuntimeToolCatalog) LoadADKRuntimeTools(
	ctx context.Context,
	run *RunSummary,
) ([]ADKRuntimeToolDefinition, error) {
	c.run = run
	return c.definitions, nil
}

type recordingADKToolSetProvider struct {
	set                      ADKToolSet
	resolveToolSetCalls      int
	resolveToolsCalls        int
	resolveDynamicToolsCalls int
}

func (p *recordingADKToolSetProvider) ResolveToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	p.resolveToolSetCalls++
	return p.set, nil
}

func (p *recordingADKToolSetProvider) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	p.resolveToolsCalls++
	return p.set.StaticTools, nil
}

func (p *recordingADKToolSetProvider) ResolveDynamicTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	p.resolveDynamicToolsCalls++
	return p.set.DynamicTools, nil
}
