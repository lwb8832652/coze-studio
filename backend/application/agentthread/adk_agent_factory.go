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

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	claudemodel "github.com/cloudwego/eino-ext/components/model/claude"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
	geminimodel "github.com/cloudwego/eino-ext/components/model/gemini"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	qwenmodel "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	arkruntime "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"google.golang.org/genai"
)

const (
	defaultADKAgentName                  = "lead"
	defaultADKAgentDescription           = "Coze task lead agent"
	defaultADKClaudeThinkingBudgetTokens = 4096
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
	StaticTools       []tool.BaseTool
	DynamicTools      []tool.BaseTool
	SubagentToolNames []string
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
	Middlewares  []adk.AgentMiddleware
	Handlers     []adk.ChatModelAgentMiddleware
	HandlerNames []ADKMiddlewareName
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

type ADKReasoningOptionProjector interface {
	ProjectADKReasoningOptions(ADKReasoningRequest) ([]model.Option, error)
}

func DetectADKModelCapabilities(
	chatModel model.BaseChatModel,
) ADKModelCapabilities {
	return adkModelCapabilitiesFromChatModel(chatModel)
}

type ApplicationADKAgentFactory struct {
	modelProvider         ChatModelProvider
	toolProvider          ADKToolProvider
	middlewares           ADKMiddlewareFactory
	promptComposer        ADKLeadPromptComposer
	promptOverlayProvider ADKLeadPromptOverlayProvider
}

type ApplicationADKAgentFactoryOption func(*ApplicationADKAgentFactory)

func WithADKLeadPromptComposer(
	composer ADKLeadPromptComposer,
) ApplicationADKAgentFactoryOption {
	return func(factory *ApplicationADKAgentFactory) {
		if composer != nil {
			factory.promptComposer = composer
		}
	}
}

func WithADKLeadPromptOverlayProvider(
	provider ADKLeadPromptOverlayProvider,
) ApplicationADKAgentFactoryOption {
	return func(factory *ApplicationADKAgentFactory) {
		factory.promptOverlayProvider = provider
	}
}

func NewApplicationADKAgentFactory(
	modelProvider ChatModelProvider,
	toolProvider ADKToolProvider,
	middlewares ADKMiddlewareFactory,
	options ...ApplicationADKAgentFactoryOption,
) *ApplicationADKAgentFactory {
	if modelProvider == nil {
		modelProvider = DefaultChatModelProvider
	}

	factory := &ApplicationADKAgentFactory{
		modelProvider:  modelProvider,
		toolProvider:   toolProvider,
		middlewares:    middlewares,
		promptComposer: NewDefaultADKLeadPromptComposer(),
	}
	for _, option := range options {
		if option != nil {
			option(factory)
		}
	}
	return factory
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
	runtimeConfig, err := ParseDeerFlowRuntimeConfig(run.Config)
	if err != nil {
		return nil, err
	}
	overlay := ADKLeadPromptOverlay{}
	if f.promptOverlayProvider != nil {
		resolved, found, resolveErr := f.promptOverlayProvider.
			ResolveADKLeadPromptOverlay(ctx, run)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve lead prompt overlay: %w", resolveErr)
		}
		if found {
			overlay = resolved
			applyADKLeadPromptModelDefaults(&cfg, overlay.ModelDefaults)
		}
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

	chatModel, modelCapabilities, err := prepareADKChatModelForRun(
		chatModel,
		run,
		cfg,
		runtimeConfig,
	)
	if err != nil {
		return nil, err
	}
	chatModel = wrapBillingGuardChatModel(chatModel, run, cfg)
	modelFailoverConfig, err := adkModelFailoverConfigFromRun(
		run,
		func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			candidateModel, candidateConfigured, candidateErr := provider(ctx, modelID)
			if candidateErr != nil || !candidateConfigured || candidateModel == nil {
				return candidateModel, candidateConfigured, candidateErr
			}
			candidateCapabilities := adkModelCapabilitiesFromChatModel(candidateModel)
			if !adkModelCapabilitiesCover(candidateCapabilities, modelCapabilities) {
				return nil, false, fmt.Errorf(
					"agent thread failover model capabilities do not cover primary model",
				)
			}
			candidateConfig := cfg
			candidateConfig.ModelName = ""
			candidateModel, _, candidateErr = prepareADKChatModelForRun(
				candidateModel,
				run,
				candidateConfig,
				runtimeConfig,
			)
			if candidateErr != nil {
				return nil, false, candidateErr
			}
			candidateModel = wrapBillingGuardChatModel(candidateModel, run, candidateConfig)

			return candidateModel, true, nil
		},
		cfg.ModelID,
	)
	if err != nil {
		return nil, err
	}

	var tools []tool.BaseTool
	var dynamicTools []tool.BaseTool
	var subagentToolNames []string
	if f.toolProvider != nil {
		if toolSetProvider, ok := f.toolProvider.(ADKToolSetProvider); ok {
			toolSet, resolveErr := toolSetProvider.ResolveToolSet(ctx, run)
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve eino adk tool set: %w", resolveErr)
			}
			tools = toolSet.StaticTools
			dynamicTools = toolSet.DynamicTools
			subagentToolNames = append(
				[]string(nil),
				toolSet.SubagentToolNames...,
			)
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
	promptComposer := f.promptComposer
	if promptComposer == nil {
		promptComposer = NewDefaultADKLeadPromptComposer()
	}
	leadPrompt, err := promptComposer.Compose(ADKLeadPromptComposeInput{
		RuntimeConfig:    runtimeConfig,
		HasDeferredTools: len(dynamicTools) > 0,
		ClientOverlay:    cfg.SystemPrompt,
		DurableOverlay:   overlay,
	})
	if err != nil {
		return nil, fmt.Errorf("compose eino adk lead prompt: %w", err)
	}

	bundle := ADKMiddlewareBundle{}
	if f.middlewares != nil {
		bundle, err = f.middlewares.Build(ctx, ADKMiddlewareBuildInput{
			Run:               run,
			Model:             chatModel,
			StaticTools:       tools,
			DynamicTools:      dynamicTools,
			SubagentToolNames: subagentToolNames,
			ModelCapabilities: modelCapabilities,
			RuntimeConfig:     runtimeConfig,
		})
		if err != nil {
			return nil, fmt.Errorf("build eino adk middlewares: %w", err)
		}
	}

	agentName := strings.TrimSpace(cfg.AgentName)
	if strings.TrimSpace(overlay.AgentName) != "" {
		agentName = leadPrompt.AgentName
	}
	if agentName == "" {
		agentName = leadPrompt.AgentName
	}
	description := strings.TrimSpace(cfg.AgentDescription)
	if strings.TrimSpace(overlay.AgentDescription) != "" {
		description = leadPrompt.AgentDescription
	}
	if description == "" {
		description = leadPrompt.AgentDescription
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        agentName,
		Description: description,
		Instruction: strings.TrimSpace(leadPrompt.Instruction),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
		MaxIterations:       cfg.MaxIterations,
		Middlewares:         bundle.Middlewares,
		Handlers:            bundle.Handlers,
		ModelRetryConfig:    modelRetryConfig,
		ModelFailoverConfig: modelFailoverConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("build eino adk chat model agent: %w", err)
	}

	return agent, nil
}

func prepareADKChatModelForRun(
	chatModel model.BaseChatModel,
	run *RunSummary,
	cfg modelExecutorConfig,
	runtimeConfig DeerFlowRuntimeConfig,
) (model.BaseChatModel, ADKModelCapabilities, error) {
	modelCapabilities := adkModelCapabilitiesFromChatModel(chatModel)
	providerCapabilityConfig, err := adkProviderCapabilityConfigFromRun(
		run,
		modelCapabilities,
	)
	if err != nil {
		return nil, ADKModelCapabilities{}, err
	}
	providerCapabilityConfig.Reasoning = effectiveADKReasoningRequest(
		runtimeConfig.ExecutionReasoningRequestOr(providerCapabilityConfig.Reasoning),
		modelCapabilities,
	)

	options := modelExecutorOptions(cfg)
	reasoningOptions, err := adkReasoningModelOptions(
		chatModel,
		providerCapabilityConfig.Reasoning,
		modelCapabilities,
	)
	if err != nil {
		return nil, ADKModelCapabilities{}, err
	}
	options = append(options, reasoningOptions...)
	if len(options) > 0 {
		chatModel = &optionedADKChatModel{
			base:    chatModel,
			options: options,
		}
	}
	if collector := NewRuntimePrometheusMetricsCollectorFromEnv(); collector != nil {
		chatModel = newRuntimeInstrumentedChatModel(
			chatModel,
			collector,
			RuntimeModelCallMetricsConfig{
				Runtime:     runtimeMetricsRunRuntime(run),
				ModelFamily: cfg.ModelName,
			},
		)
	}

	return chatModel, modelCapabilities, nil
}

func adkModelCapabilitiesFromChatModel(
	chatModel model.BaseChatModel,
) ADKModelCapabilities {
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
	modelCapabilities = mergeADKModelCapabilities(
		modelCapabilities,
		adkBuiltInModelCapabilities(chatModel),
	)

	return modelCapabilities
}

func adkModelCapabilitiesCover(
	actual ADKModelCapabilities,
	required ADKModelCapabilities,
) bool {
	return (!required.NativeToolSearch || actual.NativeToolSearch) &&
		(!required.Thinking || actual.Thinking) &&
		(!required.Reasoning || actual.Reasoning) &&
		(!required.Vision || actual.Vision) &&
		(!required.PDF || actual.PDF) &&
		(!required.File || actual.File) &&
		(!required.Audio || actual.Audio) &&
		(!required.Video || actual.Video)
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

func adkReasoningModelOptions(
	chatModel model.BaseChatModel,
	request ADKReasoningRequest,
	capabilities ADKModelCapabilities,
) ([]model.Option, error) {
	if strings.TrimSpace(request.ReasoningEffort) == "" &&
		!request.ThinkingEnabled {
		return nil, nil
	}
	if strings.TrimSpace(request.ReasoningEffort) != "" &&
		!capabilities.Reasoning {
		return nil, nil
	}
	if request.ThinkingEnabled && !capabilities.Thinking {
		return nil, nil
	}
	projector, ok := chatModel.(ADKReasoningOptionProjector)
	if ok && projector != nil {
		options, err := projector.ProjectADKReasoningOptions(request)
		if err != nil {
			return nil, fmt.Errorf("project agent thread reasoning options: %w", err)
		}

		return options, nil
	}
	options, ok, err := adkBuiltInReasoningModelOptions(chatModel, request)
	if err != nil {
		return nil, err
	}
	if ok {
		return options, nil
	}

	return nil, fmt.Errorf("agent thread reasoning option projector is required")
}

func adkBuiltInModelCapabilities(chatModel model.BaseChatModel) ADKModelCapabilities {
	switch chatModel.(type) {
	case *openaimodel.ChatModel:
		return ADKModelCapabilities{Reasoning: true}
	case *arkmodel.ChatModel:
		return ADKModelCapabilities{Reasoning: true, Thinking: true}
	case *claudemodel.ChatModel,
		*deepseekmodel.ChatModel,
		*geminimodel.ChatModel,
		*qwenmodel.ChatModel:
		return ADKModelCapabilities{Thinking: true}
	default:
		return ADKModelCapabilities{}
	}
}

func adkBuiltInReasoningModelOptions(
	chatModel model.BaseChatModel,
	request ADKReasoningRequest,
) ([]model.Option, bool, error) {
	switch chatModel.(type) {
	case *openaimodel.ChatModel:
		options := make([]model.Option, 0, 1)
		if request.ThinkingEnabled {
			return nil, true, fmt.Errorf(
				"thinking_enabled is not supported by openai model",
			)
		}
		if effort := strings.TrimSpace(request.ReasoningEffort); effort != "" {
			options = append(
				options,
				openaimodel.WithReasoningEffort(openaimodel.ReasoningEffortLevel(effort)),
			)
		}
		return options, true, nil
	case *arkmodel.ChatModel:
		options := make([]model.Option, 0, 2)
		if effort := strings.TrimSpace(request.ReasoningEffort); effort != "" {
			options = append(
				options,
				arkmodel.WithReasoningEffort(arkruntime.ReasoningEffort(effort)),
			)
		}
		if request.ThinkingEnabled {
			options = append(
				options,
				arkmodel.WithThinking(&arkruntime.Thinking{
					Type: arkruntime.ThinkingTypeEnabled,
				}),
			)
		}
		return options, true, nil
	case *claudemodel.ChatModel:
		if strings.TrimSpace(request.ReasoningEffort) != "" {
			return nil, true, fmt.Errorf(
				"reasoning_effort is not supported by claude model",
			)
		}
		if request.ThinkingEnabled {
			return []model.Option{
				claudemodel.WithThinking(&claudemodel.Thinking{
					Enable:       true,
					BudgetTokens: defaultADKClaudeThinkingBudgetTokens,
				}),
			}, true, nil
		}
		return nil, true, nil
	case *deepseekmodel.ChatModel:
		if strings.TrimSpace(request.ReasoningEffort) != "" {
			return nil, true, fmt.Errorf(
				"reasoning_effort is not supported by deepseek model",
			)
		}
		if request.ThinkingEnabled {
			return []model.Option{
				deepseekmodel.WithExtraFields(map[string]interface{}{
					"thinking": map[string]interface{}{
						"type": "enabled",
					},
				}),
			}, true, nil
		}
		return nil, true, nil
	case *geminimodel.ChatModel:
		if strings.TrimSpace(request.ReasoningEffort) != "" {
			return nil, true, fmt.Errorf(
				"reasoning_effort is not supported by gemini model",
			)
		}
		if request.ThinkingEnabled {
			return []model.Option{
				geminimodel.WithThinkingConfig(&genai.ThinkingConfig{
					IncludeThoughts: true,
				}),
			}, true, nil
		}
		return nil, true, nil
	case *qwenmodel.ChatModel:
		if strings.TrimSpace(request.ReasoningEffort) != "" {
			return nil, true, fmt.Errorf(
				"reasoning_effort is not supported by qwen model",
			)
		}
		if request.ThinkingEnabled {
			return []model.Option{qwenmodel.WithEnableThinking(true)}, true, nil
		}
		return nil, true, nil
	default:
		return nil, false, nil
	}
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
