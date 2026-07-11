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

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/dynamictool/toolsearch"
	adkfilesystem "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	einoskill "github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ADKMiddlewareName string

const (
	ADKMiddlewareSummarization          ADKMiddlewareName = "summarization"
	ADKMiddlewareReduction              ADKMiddlewareName = "reduction"
	ADKMiddlewareAgentsMD               ADKMiddlewareName = "agentsmd"
	ADKMiddlewareMemory                 ADKMiddlewareName = "memory"
	ADKMiddlewareUploadedFiles          ADKMiddlewareName = "uploaded_files"
	ADKMiddlewareSkill                  ADKMiddlewareName = "skill"
	ADKMiddlewareToolSearch             ADKMiddlewareName = "toolsearch"
	ADKMiddlewarePatchTools             ADKMiddlewareName = "patchtoolcalls"
	ADKMiddlewarePlanTask               ADKMiddlewareName = "plantask"
	ADKMiddlewareContextBudget          ADKMiddlewareName = "contextbudget"
	ADKMiddlewareTranscript             ADKMiddlewareName = "transcript"
	ADKMiddlewareMultimodal             ADKMiddlewareName = "multimodalbudget"
	ADKMiddlewareToolErrorNormalization ADKMiddlewareName = "tool_error_normalization"
	ADKMiddlewareSafetyFinish           ADKMiddlewareName = "safety_finish"
	ADKMiddlewareSemanticLoop           ADKMiddlewareName = "semantic_loop"
	ADKMiddlewarePolicy                 ADKMiddlewareName = "policy"
	ADKMiddlewareAudit                  ADKMiddlewareName = "audit"
	ADKMiddlewareUsage                  ADKMiddlewareName = "usage"
	ADKMiddlewareFilesystem             ADKMiddlewareName = "filesystem"
	ADKMiddlewareProviderCapability     ADKMiddlewareName = "provider_capability"
)

var adkMiddlewareOrder = []ADKMiddlewareName{
	ADKMiddlewareAgentsMD,
	ADKMiddlewareMemory,
	ADKMiddlewareUploadedFiles,
	ADKMiddlewareSkill,
	ADKMiddlewareToolSearch,
	ADKMiddlewarePatchTools,
	ADKMiddlewarePlanTask,
	ADKMiddlewareContextBudget,
	ADKMiddlewareTranscript,
	ADKMiddlewareSummarization,
	ADKMiddlewareReduction,
	ADKMiddlewareToolErrorNormalization,
	ADKMiddlewareSafetyFinish,
	ADKMiddlewareSemanticLoop,
	ADKMiddlewarePolicy,
	ADKMiddlewareAudit,
	ADKMiddlewareUsage,
	ADKMiddlewareFilesystem,
	ADKMiddlewareProviderCapability,
	ADKMiddlewareMultimodal,
}

type ADKModelCapabilities struct {
	NativeToolSearch bool
	Thinking         bool
	Reasoning        bool
	Vision           bool
	PDF              bool
	File             bool
	Audio            bool
	Video            bool
}

type ADKMiddlewareBuildInput struct {
	Run               *RunSummary
	RuntimeConfig     DeerFlowRuntimeConfig
	Model             model.BaseChatModel
	StaticTools       []tool.BaseTool
	DynamicTools      []tool.BaseTool
	ModelCapabilities ADKModelCapabilities
	OffloadBackend    *ADKOffloadBackend
	PlanBackend       plantask.Backend
	ReductionConfig   ADKToolResultReductionConfig
}

type ADKMiddlewareBuilder func(
	ctx context.Context,
	input ADKMiddlewareBuildInput,
) (adk.ChatModelAgentMiddleware, error)

type ADKMiddlewareAssemblerOptions struct {
	Builders              map[ADKMiddlewareName]ADKMiddlewareBuilder
	MemoryProvider        MemoryProvider
	SkillProvider         SkillProvider
	GuardrailEnforcer     ADKGuardrailEnforcer
	TranscriptStore       ADKTranscriptStore
	MemoryFlushQueue      ADKMemoryFlushQueue
	EventSink             RunEventSink
	OffloadBackendFactory ADKOffloadBackendFactory
	PlanBackendFactory    ADKPlanBackendFactory
}

type ADKMiddlewareAssembler struct {
	builders              map[ADKMiddlewareName]ADKMiddlewareBuilder
	offloadBackendFactory ADKOffloadBackendFactory
	planBackendFactory    ADKPlanBackendFactory
}

func NewADKMiddlewareAssembler(options ADKMiddlewareAssemblerOptions) *ADKMiddlewareAssembler {
	builders := make(map[ADKMiddlewareName]ADKMiddlewareBuilder, len(adkMiddlewareOrder))
	for _, name := range adkMiddlewareOrder {
		builders[name] = defaultADKMiddlewareBuilder(name, options)
	}
	for name, builder := range options.Builders {
		if builder != nil {
			builders[name] = builder
		}
	}

	return &ADKMiddlewareAssembler{
		builders:              builders,
		offloadBackendFactory: options.OffloadBackendFactory,
		planBackendFactory:    options.PlanBackendFactory,
	}
}

func (a *ADKMiddlewareAssembler) Build(
	ctx context.Context,
	input ADKMiddlewareBuildInput,
) (ADKMiddlewareBundle, error) {
	if a == nil {
		return ADKMiddlewareBundle{}, fmt.Errorf("eino adk middleware assembler is required")
	}
	if input.Run == nil {
		return ADKMiddlewareBundle{}, fmt.Errorf("run is required")
	}
	if !input.RuntimeConfig.resolved {
		runtimeConfig, err := ParseDeerFlowRuntimeConfig(input.Run.Config)
		if err != nil {
			return ADKMiddlewareBundle{}, err
		}
		input.RuntimeConfig = runtimeConfig
	}
	if err := validateADKToolPartitions(ctx, input.StaticTools, input.DynamicTools); err != nil {
		return ADKMiddlewareBundle{}, err
	}
	contextBudget, err := adkContextBudgetFromRun(input.Run)
	if err != nil {
		return ADKMiddlewareBundle{}, err
	}
	input.ReductionConfig, err = adkToolResultReductionConfigFromRun(
		input.Run,
		contextBudget,
	)
	if err != nil {
		return ADKMiddlewareBundle{}, err
	}
	if a.offloadBackendFactory != nil {
		input.OffloadBackend, err = a.offloadBackendFactory.Build(
			ctx,
			input.Run,
			input.ReductionConfig.OffloadLimits,
		)
		if err != nil {
			return ADKMiddlewareBundle{}, fmt.Errorf(
				"build eino adk offload backend: %w",
				err,
			)
		}
		if input.OffloadBackend == nil {
			return ADKMiddlewareBundle{}, fmt.Errorf(
				"eino adk offload backend factory returned empty backend",
			)
		}
	}
	if a.planBackendFactory != nil && input.RuntimeConfig.PlanCapabilityEnabled() {
		input.PlanBackend, err = a.planBackendFactory.Build(ctx, input.Run)
		if err != nil {
			return ADKMiddlewareBundle{}, fmt.Errorf(
				"build eino adk plan backend: %w",
				err,
			)
		}
		if input.PlanBackend == nil {
			return ADKMiddlewareBundle{}, fmt.Errorf(
				"eino adk plan backend factory returned empty backend",
			)
		}
	}

	bundle := ADKMiddlewareBundle{
		Handlers: make([]adk.ChatModelAgentMiddleware, 0, len(adkMiddlewareOrder)),
	}
	for _, name := range adkMiddlewareOrder {
		builder := a.builders[name]
		if builder == nil {
			return ADKMiddlewareBundle{}, fmt.Errorf("eino adk middleware %s is not configured", name)
		}
		handler, err := builder(ctx, input)
		if err != nil {
			return ADKMiddlewareBundle{}, fmt.Errorf("build eino adk middleware %s: %w", name, err)
		}
		if handler == nil {
			return ADKMiddlewareBundle{}, fmt.Errorf("eino adk middleware %s returned empty handler", name)
		}
		bundle.Handlers = append(bundle.Handlers, handler)
	}

	return bundle, nil
}

func validateADKToolPartitions(
	ctx context.Context,
	staticTools []tool.BaseTool,
	dynamicTools []tool.BaseTool,
) error {
	staticNames := make(map[string]struct{}, len(staticTools))
	for _, candidate := range staticTools {
		if candidate == nil {
			continue
		}
		info, err := candidate.Info(ctx)
		if err != nil {
			return fmt.Errorf("read static tool info: %w", err)
		}
		staticNames[info.Name] = struct{}{}
	}
	for _, candidate := range dynamicTools {
		if candidate == nil {
			continue
		}
		info, err := candidate.Info(ctx)
		if err != nil {
			return fmt.Errorf("read dynamic tool info: %w", err)
		}
		if _, exists := staticNames[info.Name]; exists {
			return fmt.Errorf("tool %s is configured as both static and dynamic", info.Name)
		}
	}

	return nil
}

func defaultADKMiddlewareBuilder(
	name ADKMiddlewareName,
	options ADKMiddlewareAssemblerOptions,
) ADKMiddlewareBuilder {
	switch name {
	case ADKMiddlewareSummarization:
		return func(ctx context.Context, input ADKMiddlewareBuildInput) (adk.ChatModelAgentMiddleware, error) {
			budget, err := adkContextBudgetFromRun(input.Run)
			if err != nil {
				return nil, err
			}
			config := &summarization.Config{
				Model: newADKUsageKindChatModel(
					input.Model,
					string(ADKMiddlewareSummarization),
				),
				TokenCounter: func(
					_ context.Context,
					counterInput *summarization.TokenCounterInput,
				) (int, error) {
					if counterInput == nil {
						return 0, nil
					}
					return estimateADKMessagesTokens(
						counterInput.Messages,
						counterInput.Tools,
					), nil
				},
				Trigger: &summarization.TriggerCondition{
					ContextTokens:   budget.SummarizationTokens,
					ContextMessages: budget.SummarizationMessages,
				},
				EmitInternalEvents: true,
				Finalize:           finalizeADKSummarizationWithDynamicContextReminders,
			}
			multimodalBudget, err := NewADKMultimodalBudgetMiddleware(
				input.Run,
				budget,
				options.EventSink,
			)
			if err != nil {
				return nil, err
			}
			config.GenModelInput = func(
				genCtx context.Context,
				systemInstruction *schema.Message,
				userInstruction *schema.Message,
				originalMessages []*schema.Message,
			) ([]*schema.Message, error) {
				filteredMessages, _ := filterADKDynamicContextReminders(originalMessages)
				return multimodalBudget.buildSummarizationModelInput(
					genCtx,
					systemInstruction,
					userInstruction,
					filteredMessages,
				)
			}
			if options.TranscriptStore != nil {
				hooks := NewADKTranscriptHooks(
					input.Run,
					options.TranscriptStore,
					options.MemoryFlushQueue,
					options.EventSink,
				)
				config.Callback = func(
					callbackCtx context.Context,
					before adk.ChatModelAgentState,
					_ adk.ChatModelAgentState,
				) error {
					return hooks.PersistSummaryInput(
						callbackCtx,
						before.Messages,
					)
				}
			}
			return summarization.New(ctx, config)
		}
	case ADKMiddlewareReduction:
		return func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if input.OffloadBackend == nil {
				return reduction.New(ctx, &reduction.Config{
					SkipTruncation: true,
					SkipClear:      true,
				})
			}
			return reduction.New(ctx, &reduction.Config{
				Backend:                   input.OffloadBackend,
				SkipTruncation:            false,
				SkipClear:                 false,
				ReadFileToolName:          "read_file",
				TruncExcludeTools:         []string{"read_file"},
				ClearExcludeTools:         []string{"read_file"},
				MaxLengthForTrunc:         input.ReductionConfig.MaxLengthForTrunc,
				MaxTokensForClear:         input.ReductionConfig.MaxTokensForClear,
				ClearRetentionSuffixLimit: input.ReductionConfig.ClearRetentionSuffixLimit,
				ClearAtLeastTokens:        input.ReductionConfig.ClearAtLeastTokens,
				TokenCounter: func(
					_ context.Context,
					messages []*schema.Message,
					tools []*schema.ToolInfo,
				) (int64, error) {
					return int64(estimateADKMessagesTokens(messages, tools)), nil
				},
				GenTruncOffloadFilePath: func(
					_ context.Context,
					detail *reduction.ToolDetail,
				) (string, error) {
					return adkReductionOffloadPath(
						input.Run,
						"trunc",
						detail,
					)
				},
				GenClearOffloadFilePath: func(
					_ context.Context,
					detail *reduction.ToolDetail,
				) (string, error) {
					return adkReductionOffloadPath(
						input.Run,
						"clear",
						detail,
					)
				},
			})
		}
	case ADKMiddlewareMultimodal:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			budget, err := adkContextBudgetFromRun(input.Run)
			if err != nil {
				return nil, err
			}
			return NewADKMultimodalBudgetMiddleware(
				input.Run,
				budget,
				options.EventSink,
			)
		}
	case ADKMiddlewareMemory:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if options.MemoryProvider == nil {
				return newReservedADKMiddleware(name), nil
			}
			budget, err := adkContextBudgetFromRun(input.Run)
			if err != nil {
				return nil, err
			}
			return NewADKMemoryMiddleware(input.Run, options.MemoryProvider, budget)
		}
	case ADKMiddlewareUploadedFiles:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			return NewADKUploadedFilesMiddleware(input.Run), nil
		}
	case ADKMiddlewareSkill:
		return func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if options.SkillProvider == nil {
				return newReservedADKMiddleware(name), nil
			}
			budget, err := adkContextBudgetFromRun(input.Run)
			if err != nil {
				return nil, err
			}
			skills, err := options.SkillProvider.Load(ctx, input.Run)
			if err != nil {
				return nil, fmt.Errorf("load run skills: %w", err)
			}
			skillContext := normalizeSkillContext(skills)
			if len(skillContext.Items) == 0 {
				return newReservedADKMiddleware(name), nil
			}
			backend, err := newADKSkillBackend(
				skillContext.Items,
				budget,
				WithADKSkillBackendGuardrail(
					input.Run,
					options.GuardrailEnforcer,
				),
			)
			if err != nil {
				return nil, err
			}
			handler, err := einoskill.NewMiddleware(ctx, &einoskill.Config{
				Backend: backend,
				CustomSystemPrompt: func(
					_ context.Context,
					toolName string,
				) string {
					return fmt.Sprintf(
						"Use the %s tool to load full task-skill instructions only when a listed skill applies.",
						toolName,
					)
				},
				CustomToolDescription: func(
					context.Context,
					[]einoskill.FrontMatter,
				) string {
					return adkSkillToolDescription(backend)
				},
				BuildContent: func(
					_ context.Context,
					skill einoskill.Skill,
					_ string,
				) (string, error) {
					return formatADKSkillContent(skill), nil
				},
			})
			if err != nil {
				return nil, err
			}
			middleware, err := newADKSelectedSkillMiddleware(
				input.Run,
				skillContext,
				backend,
				handler,
			)
			if err != nil {
				return nil, err
			}
			emitSkillsLoadedRunEvent(
				ctx,
				options.EventSink,
				input.Run,
				skillContext,
			)
			return middleware, nil
		}
	case ADKMiddlewareToolSearch:
		return func(ctx context.Context, input ADKMiddlewareBuildInput) (adk.ChatModelAgentMiddleware, error) {
			if len(input.DynamicTools) == 0 {
				return newReservedADKMiddleware(name), nil
			}
			return toolsearch.New(ctx, &toolsearch.Config{
				DynamicTools:       input.DynamicTools,
				UseModelToolSearch: input.ModelCapabilities.NativeToolSearch,
			})
		}
	case ADKMiddlewarePatchTools:
		return func(ctx context.Context, input ADKMiddlewareBuildInput) (adk.ChatModelAgentMiddleware, error) {
			return patchtoolcalls.New(ctx, &patchtoolcalls.Config{
				PatchedContentGenerator: func(
					generatorCtx context.Context,
					toolName string,
					toolCallID string,
				) (string, error) {
					emitADKToolRepairEvent(
						generatorCtx,
						input.Run,
						options.EventSink,
						toolName,
						toolCallID,
					)

					return encodeADKToolRepairResult(
						toolName,
						toolCallID,
					), nil
				},
			})
		}
	case ADKMiddlewareSafetyFinish:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			return NewADKSafetyFinishMiddleware(input.Run, options.EventSink), nil
		}
	case ADKMiddlewarePlanTask:
		return func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if input.PlanBackend == nil {
				return newReservedADKMiddleware(name), nil
			}
			planMiddleware, err := plantask.New(ctx, &plantask.Config{
				Backend: input.PlanBackend,
				BaseDir: adkPlanBaseDir,
			})
			if err != nil {
				return nil, err
			}
			guard, err := newADKPlanCompletionGuardMiddleware(
				input.PlanBackend,
			)
			if err != nil {
				return nil, err
			}
			return newADKPlanTaskWithCompletionGuardMiddleware(
				planMiddleware,
				guard,
			)
		}
	case ADKMiddlewareContextBudget:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			budget, err := adkContextBudgetFromRun(input.Run)
			if err != nil {
				return nil, err
			}
			return NewADKToolDefinitionBudgetMiddleware(budget)
		}
	case ADKMiddlewareToolErrorNormalization:
		return func(
			context.Context,
			ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			return NewADKToolErrorNormalizationMiddleware(), nil
		}
	case ADKMiddlewareSemanticLoop:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			config, err := adkSemanticLoopConfigFromRun(input.Run)
			if err != nil {
				return nil, err
			}
			return NewADKSemanticLoopMiddleware(config), nil
		}
	case ADKMiddlewareTranscript:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if options.TranscriptStore == nil {
				return newReservedADKMiddleware(name), nil
			}
			return NewADKTranscriptMiddleware(NewADKTranscriptHooks(
				input.Run,
				options.TranscriptStore,
				options.MemoryFlushQueue,
				options.EventSink,
			)), nil
		}
	case ADKMiddlewareFilesystem:
		return func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if input.OffloadBackend == nil {
				return newReservedADKMiddleware(name), nil
			}
			readTool, err := newADKReadOffloadTool(input.OffloadBackend)
			if err != nil {
				return nil, err
			}
			disabled := &adkfilesystem.ToolConfig{Disable: true}
			return adkfilesystem.New(
				ctx,
				&adkfilesystem.MiddlewareConfig{
					Backend: input.OffloadBackend,
					ReadFileToolConfig: &adkfilesystem.ToolConfig{
						CustomTool: readTool,
					},
					LsToolConfig:        disabled,
					WriteFileToolConfig: disabled,
					EditFileToolConfig:  disabled,
					GlobToolConfig:      disabled,
					GrepToolConfig:      disabled,
				},
			)
		}
	case ADKMiddlewareProviderCapability:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			return NewADKProviderCapabilityMiddleware(
				input.Run,
				input.ModelCapabilities,
				input.RuntimeConfig,
			)
		}
	default:
		return func(context.Context, ADKMiddlewareBuildInput) (adk.ChatModelAgentMiddleware, error) {
			return newReservedADKMiddleware(name), nil
		}
	}
}

type reservedADKMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	name ADKMiddlewareName
}

func newReservedADKMiddleware(name ADKMiddlewareName) *reservedADKMiddleware {
	return &reservedADKMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		name:                         name,
	}
}
