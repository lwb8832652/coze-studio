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
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultADKAgentName        = "lead"
	defaultADKAgentDescription = "Coze task lead agent"
)

type ADKAgentFactory interface {
	Build(ctx context.Context, run *RunSummary) (adk.ResumableAgent, error)
}

type ADKAgentFactoryFunc func(
	ctx context.Context,
	run *RunSummary,
) (adk.ResumableAgent, error)

func (f ADKAgentFactoryFunc) Build(
	ctx context.Context,
	run *RunSummary,
) (adk.ResumableAgent, error) {
	if f == nil {
		return nil, fmt.Errorf("eino adk agent factory function is required")
	}

	return f(ctx, run)
}

type ADKToolProvider interface {
	ResolveTools(ctx context.Context, run *RunSummary) ([]tool.BaseTool, error)
}

type ADKToolSet struct {
	StaticTools  []tool.BaseTool
	DynamicTools []tool.BaseTool
}

type ADKToolSetProvider interface {
	ResolveToolSet(ctx context.Context, run *RunSummary) (ADKToolSet, error)
}

type ADKDynamicToolProvider interface {
	ResolveDynamicTools(ctx context.Context, run *RunSummary) ([]tool.BaseTool, error)
}

type ADKToolProviderFunc func(ctx context.Context, run *RunSummary) ([]tool.BaseTool, error)

func (f ADKToolProviderFunc) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	if f == nil {
		return nil, nil
	}

	return f(ctx, run)
}

type ADKMiddlewareBundle struct {
	Middlewares []adk.AgentMiddleware
	Handlers    []adk.ChatModelAgentMiddleware
}

type ADKMiddlewareFactory interface {
	Build(ctx context.Context, input ADKMiddlewareBuildInput) (ADKMiddlewareBundle, error)
}

type ADKMiddlewareFactoryFunc func(
	ctx context.Context,
	input ADKMiddlewareBuildInput,
) (ADKMiddlewareBundle, error)

func (f ADKMiddlewareFactoryFunc) Build(
	ctx context.Context,
	input ADKMiddlewareBuildInput,
) (ADKMiddlewareBundle, error) {
	if f == nil {
		return ADKMiddlewareBundle{}, nil
	}

	return f(ctx, input)
}

type ADKNativeToolSearchModel interface {
	SupportsNativeToolSearch() bool
}

type ADKProviderCapabilityModel interface {
	ADKProviderCapabilities() ADKModelCapabilities
}

type ApplicationADKAgentFactory struct {
	modelProvider ChatModelProvider
	toolProvider  ADKToolProvider
	middlewares   ADKMiddlewareFactory
}

func NewApplicationADKAgentFactory(
	modelProvider ChatModelProvider,
	toolProvider ADKToolProvider,
	middlewares ADKMiddlewareFactory,
) *ApplicationADKAgentFactory {
	if modelProvider == nil {
		modelProvider = DefaultChatModelProvider
	}

	return &ApplicationADKAgentFactory{
		modelProvider: modelProvider,
		toolProvider:  toolProvider,
		middlewares:   middlewares,
	}
}

func (f *ApplicationADKAgentFactory) Build(
	ctx context.Context,
	run *RunSummary,
) (adk.ResumableAgent, error) {
	if f == nil {
		return nil, fmt.Errorf("eino adk agent factory is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}

	cfg, err := parseModelExecutorConfig(run.Config)
	if err != nil {
		return nil, err
	}
	modelRetryConfig, err := adkModelRetryConfigFromRun(run)
	if err != nil {
		return nil, err
	}

	provider := f.modelProvider
	if provider == nil {
		provider = DefaultChatModelProvider
	}
	chatModel, configured, err := provider(ctx, cfg.ModelID)
	if err != nil {
		return nil, fmt.Errorf("resolve agent thread chat model: %w", err)
	}
	if !configured || chatModel == nil {
		return nil, fmt.Errorf("agent thread chat model is not configured")
	}

	modelCapabilities := ADKModelCapabilities{}
	if capable, ok := chatModel.(ADKNativeToolSearchModel); ok {
		modelCapabilities.NativeToolSearch = capable.SupportsNativeToolSearch()
	}
	if capable, ok := chatModel.(ADKProviderCapabilityModel); ok {
		modelCapabilities = mergeADKModelCapabilities(
			modelCapabilities,
			capable.ADKProviderCapabilities(),
		)
	}

	options := modelExecutorOptions(cfg)
	if len(options) > 0 {
		chatModel = &optionedADKChatModel{
			base:    chatModel,
			options: options,
		}
	}

	var tools []tool.BaseTool
	var dynamicTools []tool.BaseTool
	if f.toolProvider != nil {
		if toolSetProvider, ok := f.toolProvider.(ADKToolSetProvider); ok {
			toolSet, resolveErr := toolSetProvider.ResolveToolSet(ctx, run)
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve eino adk tool set: %w", resolveErr)
			}
			tools = toolSet.StaticTools
			dynamicTools = toolSet.DynamicTools
		} else {
			tools, err = f.toolProvider.ResolveTools(ctx, run)
			if err != nil {
				return nil, fmt.Errorf("resolve eino adk tools: %w", err)
			}
			if dynamicProvider, ok := f.toolProvider.(ADKDynamicToolProvider); ok {
				dynamicTools, err = dynamicProvider.ResolveDynamicTools(ctx, run)
				if err != nil {
					return nil, fmt.Errorf("resolve eino adk dynamic tools: %w", err)
				}
			}
		}
	}

	bundle := ADKMiddlewareBundle{}
	if f.middlewares != nil {
		bundle, err = f.middlewares.Build(ctx, ADKMiddlewareBuildInput{
			Run:               run,
			Model:             chatModel,
			StaticTools:       tools,
			DynamicTools:      dynamicTools,
			ModelCapabilities: modelCapabilities,
		})
		if err != nil {
			return nil, fmt.Errorf("build eino adk middlewares: %w", err)
		}
	}

	agentName := strings.TrimSpace(cfg.AgentName)
	if agentName == "" {
		agentName = defaultADKAgentName
	}
	description := strings.TrimSpace(cfg.AgentDescription)
	if description == "" {
		description = defaultADKAgentDescription
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        agentName,
		Description: description,
		Instruction: strings.TrimSpace(cfg.SystemPrompt),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
		MaxIterations:    cfg.MaxIterations,
		Middlewares:      bundle.Middlewares,
		Handlers:         bundle.Handlers,
		ModelRetryConfig: modelRetryConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("build eino adk chat model agent: %w", err)
	}

	return agent, nil
}

func mergeADKModelCapabilities(
	base ADKModelCapabilities,
	override ADKModelCapabilities,
) ADKModelCapabilities {
	if override.NativeToolSearch {
		base.NativeToolSearch = true
	}
	if override.Thinking {
		base.Thinking = true
	}
	if override.Reasoning {
		base.Reasoning = true
	}
	if override.Vision {
		base.Vision = true
	}
	if override.PDF {
		base.PDF = true
	}
	if override.File {
		base.File = true
	}
	if override.Audio {
		base.Audio = true
	}
	if override.Video {
		base.Video = true
	}
	return base
}

type optionedADKChatModel struct {
	base    model.BaseChatModel
	options []model.Option
}

func (m *optionedADKChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	return m.base.Generate(ctx, input, m.mergeOptions(options)...)
}

func (m *optionedADKChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return m.base.Stream(ctx, input, m.mergeOptions(options)...)
}

func (m *optionedADKChatModel) mergeOptions(options []model.Option) []model.Option {
	merged := make([]model.Option, 0, len(m.options)+len(options))
	merged = append(merged, m.options...)
	merged = append(merged, options...)

	return merged
}
