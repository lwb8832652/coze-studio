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
	"errors"
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
	ADKMiddlewareSideEffect             ADKMiddlewareName = "side_effect"
	ADKMiddlewareSummarization          ADKMiddlewareName = "summarization"
	ADKMiddlewareReduction              ADKMiddlewareName = "reduction"
	ADKMiddlewareMemory                 ADKMiddlewareName = "memory"
	ADKMiddlewareUploadedFiles          ADKMiddlewareName = "uploaded_files"
	ADKMiddlewareSkill                  ADKMiddlewareName = "skill"
	ADKMiddlewareToolSearch             ADKMiddlewareName = "toolsearch"
	ADKMiddlewareParityState            ADKMiddlewareName = "parity_state"
	ADKMiddlewarePatchTools             ADKMiddlewareName = "patchtoolcalls"
	ADKMiddlewarePlanTask               ADKMiddlewareName = "plantask"
	ADKMiddlewareContextBudget          ADKMiddlewareName = "contextbudget"
	ADKMiddlewareTranscript             ADKMiddlewareName = "transcript"
	ADKMiddlewareMultimodal             ADKMiddlewareName = "multimodalbudget"
	ADKMiddlewareToolErrorNormalization ADKMiddlewareName = "tool_error_normalization"
	ADKMiddlewareSafetyFinish           ADKMiddlewareName = "safety_finish"
	ADKMiddlewareSubagentLimit          ADKMiddlewareName = "subagent_limit"
	ADKMiddlewareSemanticLoop           ADKMiddlewareName = "semantic_loop"
	ADKMiddlewareFilesystem             ADKMiddlewareName = "filesystem"
	ADKMiddlewareProviderCapability     ADKMiddlewareName = "provider_capability"
)

var adkMiddlewareOrder = []ADKMiddlewareName{
	ADKMiddlewareSideEffect,
	ADKMiddlewareReduction,
	ADKMiddlewareFilesystem,
	ADKMiddlewareUploadedFiles,
	ADKMiddlewarePatchTools,
	ADKMiddlewareToolErrorNormalization,
	ADKMiddlewareMemory,
	ADKMiddlewareSkill,
	ADKMiddlewareTranscript,
	ADKMiddlewareSummarization,
	ADKMiddlewarePlanTask,
	ADKMiddlewareProviderCapability,
	ADKMiddlewareMultimodal,
	ADKMiddlewareToolSearch,
	ADKMiddlewareParityState,
	ADKMiddlewareContextBudget,
	ADKMiddlewareSafetyFinish,
	ADKMiddlewareSubagentLimit,
	ADKMiddlewareSemanticLoop,
}

var errADKMiddlewareNotApplicable = errors.New("eino adk middleware is not applicable")

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
	SubagentToolNames []string
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
	Builders               map[ADKMiddlewareName]ADKMiddlewareBuilder
	MemoryProvider         MemoryProvider
	SkillProvider          SkillProvider
	GuardrailEnforcer      ADKGuardrailEnforcer
	TranscriptStore        ADKTranscriptStore
	MemoryFlushQueue       ADKMemoryFlushQueue
	EventSink              RunEventSink
	JournalContentProducer JournalContentProducer
	OffloadBackendFactory  ADKOffloadBackendFactory
	PlanBackendFactory     ADKPlanBackendFactory
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
		if backend, ok := input.PlanBackend.(*ADKPlanBackend); ok {
			if err := backend.setParityStateTracker(adkParityStateTrackerFromContext(ctx)); err != nil {
				return ADKMiddlewareBundle{}, err
			}
			if err := backend.syncParityState(ctx); err != nil {
				return ADKMiddlewareBundle{}, err
			}
		}
	}

	bundle := ADKMiddlewareBundle{
		Handlers:     make([]adk.ChatModelAgentMiddleware, 0, len(adkMiddlewareOrder)),
		HandlerNames: make([]ADKMiddlewareName, 0, len(adkMiddlewareOrder)),
	}
	for _, name := range adkMiddlewareOrder {
		builder := a.builders[name]
		if builder == nil {
			return ADKMiddlewareBundle{}, fmt.Errorf("eino adk middleware %s is not configured", name)
		}
		handler, err := builder(ctx, input)
		if errors.Is(err, errADKMiddlewareNotApplicable) {
			continue
		}
		if err != nil {
			return ADKMiddlewareBundle{}, fmt.Errorf("build eino adk middleware %s: %w", name, err)
		}
		if handler == nil {
			return ADKMiddlewareBundle{}, fmt.Errorf("eino adk middleware %s returned empty handler", name)
		}
		bundle.Handlers = append(bundle.Handlers, handler)
		bundle.HandlerNames = append(bundle.HandlerNames, name)
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
	case ADKMiddlewareSideEffect:
		return func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			coordinator := adkSideEffectBoundaryCoordinatorFromContext(ctx)
			if coordinator == nil {
				return nil, errADKMiddlewareNotApplicable
			}
			return NewADKSideEffectMiddleware(
				coordinator,
				WithADKSideEffectNonReplayableTools(input.SubagentToolNames),
			), nil
		}
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
				return nil, errADKMiddlewareNotApplicable
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
				return nil, errADKMiddlewareNotApplicable
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
				return nil, errADKMiddlewareNotApplicable
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
				return nil, errADKMiddlewareNotApplicable
			}
			backend, err := newADKSkillBackend(
				skillContext.Items,
				budget,
				WithADKSkillBackendGuardrail(
					input.Run,
					options.GuardrailEnforcer,
				),
				WithADKSkillBackendJournal(
					input.Run,
					options.JournalContentProducer,
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
			if tracker := adkParityStateTrackerFromContext(ctx); tracker != nil {
				paritySkills := make([]ADKParitySkill, 0, len(skillContext.Items))
				for _, skill := range skillContext.Items {
					paritySkills = append(paritySkills, ADKParitySkill{
						ID: skill.ID, Name: skill.Name, Version: skill.Version,
					})
				}
				if err := tracker.ReplaceActiveSkills(paritySkills); err != nil {
					return nil, fmt.Errorf("record eino adk parity skills: %w", err)
				}
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
				return nil, errADKMiddlewareNotApplicable
			}
			return toolsearch.New(ctx, &toolsearch.Config{
				DynamicTools:       input.DynamicTools,
				UseModelToolSearch: input.ModelCapabilities.NativeToolSearch,
			})
		}
	case ADKMiddlewareParityState:
		return func(ctx context.Context, input ADKMiddlewareBuildInput) (adk.ChatModelAgentMiddleware, error) {
			if adkParityStateTrackerFromContext(ctx) == nil {
				return nil, errADKMiddlewareNotApplicable
			}
			return NewADKParityStateMiddleware(ctx, input.DynamicTools)
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
	case ADKMiddlewareSubagentLimit:
		return func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if !input.RuntimeConfig.SubagentCapabilityEnabled() ||
				len(input.SubagentToolNames) == 0 {
				return nil, errADKMiddlewareNotApplicable
			}
			limit := input.RuntimeConfig.MaxConcurrentSubagents
			if limit == 0 {
				limit = defaultDeerFlowMaxConcurrentSubagents
			}
			return NewADKSubagentLimitMiddleware(
				input.Run,
				input.SubagentToolNames,
				limit,
				options.EventSink,
			), nil
		}
	case ADKMiddlewarePlanTask:
		return func(
			ctx context.Context,
			input ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			if input.PlanBackend == nil {
				return nil, errADKMiddlewareNotApplicable
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
				return nil, errADKMiddlewareNotApplicable
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
				return nil, errADKMiddlewareNotApplicable
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
			middleware, err := NewADKProviderCapabilityMiddleware(
				input.Run,
				input.ModelCapabilities,
				input.RuntimeConfig,
			)
			if err != nil {
				return nil, err
			}
			middleware.eventSink = options.EventSink
			return middleware, nil
		}
	default:
		return func(context.Context, ADKMiddlewareBuildInput) (adk.ChatModelAgentMiddleware, error) {
			return nil, fmt.Errorf("unsupported eino adk middleware: %s", name)
		}
	}
}
