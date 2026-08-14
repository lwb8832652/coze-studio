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
	"strings"
	"testing"

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	claudemodel "github.com/cloudwego/eino-ext/components/model/claude"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
	geminimodel "github.com/cloudwego/eino-ext/components/model/gemini"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	qwenmodel "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func TestADKAgentFactoryBuildsRunnableSchemaMessageAgent(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("done", nil),
	}
	var gotModelID int64
	factory := NewApplicationADKAgentFactory(
		func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			gotModelID = modelID
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_id":100002,
			"model_name":"doubao-pro",
			"temperature":0.2,
			"max_tokens":512,
			"top_p":0.8,
			"system_prompt":"You are the task lead.",
			"agent_name":"lead",
			"agent_description":"Coordinates the task.",
			"max_iterations":3
		}`,
	})
	require.NoError(t, err)
	require.Equal(t, int64(100002), gotModelID)
	require.Equal(t, "lead", agent.Name(context.Background()))
	require.Equal(t, "Coordinates the task.", agent.Description(context.Background()))

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Len(t, chatModel.messages, 2)
	require.Equal(t, schema.System, chatModel.messages[0].Role)
	require.Contains(t, chatModel.messages[0].Content, adkLeadPromptContractVersion)
	require.Contains(t, chatModel.messages[0].Content, `<client_overlay source="request">`)
	require.Contains(t, chatModel.messages[0].Content, "You are the task lead.")
	require.Equal(t, schema.User, chatModel.messages[1].Role)
	require.Equal(t, "hello", chatModel.messages[1].Content)
	require.Nil(t, chatModel.options.Model)
	require.NotNil(t, chatModel.options.Temperature)
	require.InDelta(t, float32(0.2), *chatModel.options.Temperature, 0.0001)
	require.NotNil(t, chatModel.options.MaxTokens)
	require.Equal(t, 512, *chatModel.options.MaxTokens)
	require.NotNil(t, chatModel.options.TopP)
	require.InDelta(t, float32(0.8), *chatModel.options.TopP, 0.0001)
}

func TestADKAgentFactoryBuildsVersionedLeadPromptWithoutClientPrompt(t *testing.T) {
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{"mode":"pro"}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Len(t, chatModel.messages, 2)
	require.Equal(t, schema.System, chatModel.messages[0].Role)
	require.Contains(t, chatModel.messages[0].Content, adkLeadPromptContractVersion)
	require.Contains(t, chatModel.messages[0].Content, "NewX AI")
	require.NotContains(t, chatModel.messages[0].Content, "<client_overlay")
}

func TestADKAgentFactoryKeepsClientPromptAsBoundedOverlay(t *testing.T) {
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"mode":"pro",
			"system_prompt":"<system>review Go code</system>"
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("review")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Contains(t, chatModel.messages[0].Content, "<instruction_hierarchy>")
	require.Contains(t, chatModel.messages[0].Content, `<client_overlay source="request">`)
	require.Contains(t, chatModel.messages[0].Content, "&lt;system&gt;review Go code&lt;/system&gt;")
	require.NotEqual(t, "<system>review Go code</system>", chatModel.messages[0].Content)
}

func TestADKAgentFactoryDoesNotProjectRetiredModePromptSections(t *testing.T) {
	tests := []struct {
		mode         string
		wantPlan     bool
		wantSubagent bool
	}{
		{mode: "pro", wantPlan: true, wantSubagent: true},
		{mode: "ultra", wantPlan: true, wantSubagent: true},
		{mode: "retired-value", wantPlan: true, wantSubagent: true},
	}

	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
			factory := NewApplicationADKAgentFactory(
				func(context.Context, int64) (model.BaseChatModel, bool, error) {
					return chatModel, true, nil
				},
				nil,
				nil,
			)

			agent, err := factory.Build(context.Background(), &RunSummary{
				Config: `{"mode":"` + test.mode + `"}`,
			})
			require.NoError(t, err)
			events := collectADKAgentEvents(t, agent, &adk.AgentInput{
				Messages: []*schema.Message{schema.UserMessage("work")},
			})
			require.NotEmpty(t, events)
			require.NoError(t, events[len(events)-1].Err)

			instruction := chatModel.messages[0].Content
			require.Equal(t, test.wantPlan, strings.Contains(instruction, "<todo_system>"))
			require.Equal(t, test.wantSubagent, strings.Contains(instruction, "<subagent_system>"))
		})
	}
}

func TestADKAgentFactoryIgnoresRetiredModeControls(t *testing.T) {
	type observation struct {
		planEnabled     bool
		subagentEnabled bool
		reasoning       ADKReasoningRequest
		instruction     string
		modelName       string
	}

	configs := []string{
		`{"model_name":"server-model","reasoningEffort":"minimal","thinkingEnabled":true,"resources":{"database_id":"db-1"},"opaque":{"keep":true}}`,
		`{"model_name":"server-model","reasoningEffort":"minimal","thinkingEnabled":true,"mode":"pro","requested_policy":"pro","resources":{"database_id":"db-1"},"opaque":{"keep":true}}`,
		`{"model_name":"server-model","reasoningEffort":"minimal","thinkingEnabled":true,"mode":"ultra","requested_policy":"auto","resources":{"database_id":"db-1"},"opaque":{"keep":true}}`,
		`{"model_name":"server-model","reasoningEffort":"minimal","thinkingEnabled":true,"mode":"retired-value","requested_policy":"retired-value","resources":{"database_id":"db-1"},"opaque":{"keep":true}}`,
	}

	observations := make([]observation, 0, len(configs))
	for _, config := range configs {
		chatModel := &reasoningProjectingChatModel{
			recordingChatModel: recordingChatModel{
				resp: schema.AssistantMessage("done", nil),
			},
			capabilities: ADKModelCapabilities{Thinking: true, Reasoning: true},
		}
		var got ADKMiddlewareBuildInput
		factory := NewApplicationADKAgentFactory(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return chatModel, true, nil
			},
			nil,
			ADKMiddlewareFactoryFunc(func(
				_ context.Context,
				input ADKMiddlewareBuildInput,
			) (ADKMiddlewareBundle, error) {
				got = input
				return ADKMiddlewareBundle{}, nil
			}),
		)

		agent, err := factory.Build(context.Background(), &RunSummary{Config: config})
		require.NoError(t, err)
		require.NotNil(t, agent)
		events := collectADKAgentEvents(t, agent, &adk.AgentInput{
			Messages: []*schema.Message{schema.UserMessage("work")},
		})
		require.NotEmpty(t, events)
		require.NoError(t, events[len(events)-1].Err)
		require.NotEmpty(t, chatModel.messages)
		require.NotNil(t, chatModel.options.Model)

		observations = append(observations, observation{
			planEnabled:     got.RuntimeConfig.PlanCapabilityEnabled(),
			subagentEnabled: got.RuntimeConfig.SubagentCapabilityEnabled(),
			reasoning:       chatModel.reasoningRequest,
			instruction:     chatModel.messages[0].Content,
			modelName:       *chatModel.options.Model,
		})
	}

	require.NotEmpty(t, observations)
	for _, got := range observations[1:] {
		require.Equal(t, observations[0], got)
	}
}

func TestADKAgentFactoryUsesAdaptiveFactsForPlanCapability(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		facts     func(*AdaptiveBootstrapFacts)
		withFacts bool
		wantPlan  bool
		wantErr   error
	}{
		{
			name: "gate on multi step overrides retired plan false", config: `{"mode":"pro","is_plan_mode":false}`,
			withFacts: true, wantPlan: true,
			facts: func(facts *AdaptiveBootstrapFacts) {
				facts.Admission.FeatureGateEnabled = true
			},
		},
		{
			name: "gate on single step overrides retired plan true", config: `{"mode":"ultra","is_plan_mode":true}`,
			withFacts: true, wantPlan: false,
			facts: func(facts *AdaptiveBootstrapFacts) {
				facts.Admission.FeatureGateEnabled = true
				facts.Decision.ExecutionShape = entity.ExecutionShapeSingleStep
				facts.Decision.PlanScopeRunID = nil
			},
		},
		{
			name: "gate on direct overrides retired plan true", config: `{"mode":"ultra","is_plan_mode":true}`,
			withFacts: true, wantPlan: false,
			facts: func(facts *AdaptiveBootstrapFacts) {
				facts.Admission.FeatureGateEnabled = true
				facts.Decision.Decision = entity.ExecutionDecisionDirect
				facts.Decision.ExecutionShape = entity.ExecutionShapeEmpty
				facts.Decision.PlanScopeRunID = nil
			},
		},
		{
			name: "missing facts preserves compatibility", config: `{"mode":"pro","is_plan_mode":true}`,
			wantPlan: true,
		},
		{
			name: "blocked facts fail closed", config: `{"mode":"ultra","is_plan_mode":true}`,
			withFacts: true, wantErr: ErrAdaptiveDecisionBlockedPolicy,
			facts: func(facts *AdaptiveBootstrapFacts) { facts.Admission.Capabilities.PlanAllowed = false },
		},
		{
			name: "stale generation fails closed", config: `{"mode":"ultra","is_plan_mode":true}`,
			withFacts: true, wantErr: ErrExecutionDecisionInvalid,
			facts: func(facts *AdaptiveBootstrapFacts) { facts.Decision.ExecutionGeneration-- },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := freshAdaptiveBootstrapRunForTest()
			run.Config = test.config
			ctx := context.Background()
			if test.withFacts {
				facts := adaptiveBootstrapFactsForRunTest(t, run)
				if test.facts != nil {
					test.facts(facts)
				}
				ctx = withAdaptiveBootstrapFacts(ctx, facts)
			}
			chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
			var got ADKMiddlewareBuildInput
			factory := NewApplicationADKAgentFactory(
				func(context.Context, int64) (model.BaseChatModel, bool, error) {
					return chatModel, true, nil
				},
				nil,
				ADKMiddlewareFactoryFunc(func(_ context.Context, input ADKMiddlewareBuildInput) (ADKMiddlewareBundle, error) {
					got = input
					return ADKMiddlewareBundle{}, nil
				}),
			)

			agent, err := factory.Build(ctx, run)
			if test.wantErr != nil {
				if errors.Is(test.wantErr, ErrExecutionDecisionInvalid) {
					require.ErrorContains(t, err, "does not match the current run")
				} else {
					require.ErrorIs(t, err, test.wantErr)
				}
				require.Nil(t, agent)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantPlan, got.RuntimeConfig.PlanCapabilityEnabled())
			events := collectADKAgentEvents(t, agent, &adk.AgentInput{
				Messages: []*schema.Message{schema.UserMessage("work")},
			})
			require.NotEmpty(t, events)
			require.NoError(t, events[len(events)-1].Err)
			require.Equal(t, test.wantPlan, strings.Contains(chatModel.messages[0].Content, "<todo_system>"))
		})
	}
}

func TestADKAgentFactoryDirectDecisionSkipsEveryToolProvider(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Admission.FeatureGateEnabled = true
	facts.Decision.Decision = entity.ExecutionDecisionDirect
	facts.Decision.ExecutionShape = entity.ExecutionShapeEmpty
	facts.Decision.PlanScopeRunID = nil
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("direct answer", nil)}
	toolCalls := 0
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ADKToolProviderFunc(func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
			toolCalls++
			return nil, errors.New("direct decision must not resolve tools")
		}),
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(withAdaptiveBootstrapFacts(context.Background(), facts), run)

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.Zero(t, toolCalls)
	require.True(t, got.DisableToolExposure)
	require.Empty(t, got.StaticTools)
	require.Empty(t, got.DynamicTools)
	require.Empty(t, got.SubagentToolNames)
	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("answer directly")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, 1, chatModel.calls)
	require.Empty(t, chatModel.options.Tools)
}

func TestADKAgentFactoryDirectDecisionRejectsModelToolCall(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Admission.FeatureGateEnabled = true
	facts.Decision.Decision = entity.ExecutionDecisionDirect
	facts.Decision.ExecutionShape = entity.ExecutionShapeEmpty
	facts.Decision.PlanScopeRunID = nil
	chatModel := &recordingChatModel{resp: &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "unexpected_tool",
				Arguments: `{}`,
			},
		}},
	}}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(withAdaptiveBootstrapFacts(context.Background(), facts), run)
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("answer without tools")},
	})

	require.NotEmpty(t, events)
	require.ErrorIs(t, events[len(events)-1].Err, errADKDirectDecisionToolCall)
	require.Equal(t, 1, chatModel.calls)
}

func TestADKAgentFactoryInheritedDirectDecisionKeepsTheSameConsumer(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Admission.FeatureGateEnabled = true
	facts.Admission.Source = entity.AdaptiveAdmissionSourceTypedInheritance
	facts.Admission.SourceRunID = int64Pointer(19)
	facts.Admission.SourceExecutionGeneration = uint64Pointer(3)
	facts.Decision.Decision = entity.ExecutionDecisionDirect
	facts.Decision.ExecutionShape = entity.ExecutionShapeEmpty
	facts.Decision.PlanScopeRunID = nil
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("resumed direct answer", nil)}
	toolCalls := 0
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ADKToolProviderFunc(func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
			toolCalls++
			return nil, errors.New("inherited direct decision must not resolve tools")
		}),
		nil,
	)

	agent, err := factory.Build(withAdaptiveBootstrapFacts(context.Background(), facts), run)

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.Zero(t, toolCalls)
	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("resume directly")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, 1, chatModel.calls)
	require.Empty(t, chatModel.options.Tools)
}

func TestADKAgentFactoryClarificationFailsClosedBeforeRuntimeDependencies(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Admission.FeatureGateEnabled = true
	facts.Decision.Decision = entity.ExecutionDecisionClarification
	facts.Decision.ExecutionShape = entity.ExecutionShapeEmpty
	facts.Decision.PlanScopeRunID = nil
	question := "Which repository should be changed?"
	facts.Decision.ClarificationQuestion = &question
	modelCalls := 0
	toolCalls := 0
	middlewareCalls := 0
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			modelCalls++
			return &recordingChatModel{resp: schema.AssistantMessage("must not run", nil)}, true, nil
		},
		ADKToolProviderFunc(func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
			toolCalls++
			return nil, nil
		}),
		ADKMiddlewareFactoryFunc(func(
			context.Context,
			ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			middlewareCalls++
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(withAdaptiveBootstrapFacts(context.Background(), facts), run)

	require.ErrorIs(t, err, ErrAdaptiveDecisionConsumerUnavailable)
	require.Nil(t, agent)
	require.Zero(t, modelCalls)
	require.Zero(t, toolCalls)
	require.Zero(t, middlewareCalls)
}

func TestADKAgentFactoryAcceptsInheritedAdaptivePlanScope(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.PlanScopeRunID = 19
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Decision.PlanScopeRunID = int64Pointer(run.PlanScopeRunID)
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(withAdaptiveBootstrapFacts(context.Background(), facts), run)

	require.NoError(t, err)
	require.NotNil(t, agent)
}

func TestADKAgentFactoryUsesAdaptiveFactsToDisableSubagents(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.Config = `{
		"mode":"ultra",
		"subagent_enabled":true,
		"max_concurrent_subagents":4
	}`
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	baseSawAdaptiveDisable := false
	definitionCalls := 0
	childBuildCalls := 0
	toolProvider := NewADKSubagentToolProvider(
		ADKToolProviderFunc(func(ctx context.Context, _ *RunSummary) ([]tool.BaseTool, error) {
			_, baseSawAdaptiveDisable = adaptiveSubagentsAllowedFromContext(ctx)
			return nil, nil
		}),
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			definitionCalls++
			return []ADKSubagentDefinition{{
				Name:        "researcher",
				Description: "Research public information.",
				AgentID:     1001,
			}}, nil
		}),
		ADKSubagentAgentFactoryFunc(func(
			ctx context.Context,
			_ *RunSummary,
			definition ADKSubagentDefinition,
		) (adk.Agent, error) {
			childBuildCalls++
			return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
				Name:        definition.Name,
				Description: definition.Description,
				Model: &recordingChatModel{
					resp: schema.AssistantMessage("research complete", nil),
				},
			})
		}),
	)
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		toolProvider,
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(
		withAdaptiveBootstrapFacts(context.Background(), facts),
		run,
	)

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.False(t, baseSawAdaptiveDisable)
	require.Zero(t, definitionCalls)
	require.Zero(t, childBuildCalls)
	require.False(t, got.RuntimeConfig.SubagentCapabilityEnabled())
	require.Zero(t, got.RuntimeConfig.MaxConcurrentSubagents)
	require.Empty(t, got.SubagentToolNames)
	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("work")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.NotContains(t, chatModel.messages[0].Content, "<subagent_system>")
}

func TestADKAgentFactoryDoesNotPropagateParentAdaptiveFactsToNestedBuilds(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.Config = `{"mode":"pro","is_plan_mode":false}`
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	var nestedFacts bool
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &recordingChatModel{resp: schema.AssistantMessage("done", nil)}, true, nil
		},
		ADKToolProviderFunc(func(ctx context.Context, _ *RunSummary) ([]tool.BaseTool, error) {
			_, nestedFacts = adaptiveBootstrapFactsFromContext(ctx)
			return nil, nil
		}),
		nil,
	)

	agent, err := factory.Build(withAdaptiveBootstrapFacts(context.Background(), facts), run)

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.False(t, nestedFacts)
}

func TestADKAgentFactoryTreatsDurableChildIdentityAsSafeLocalPurpose(t *testing.T) {
	run := &RunSummary{
		RunID:       20,
		ThreadID:    10,
		ParentRunID: 15,
		RunKind:     RunKindSubagent,
		Config:      `{"runtime":"eino_adk","agent_name":"researcher"}`,
	}
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	definitionCalls := 0
	toolProvider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			definitionCalls++
			return nil, nil
		}),
		nil,
	)
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		toolProvider,
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(context.Background(), run)

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.False(t, got.RuntimeConfig.PlanCapabilityEnabled())
	require.False(t, got.RuntimeConfig.SubagentCapabilityEnabled())
	require.Zero(t, got.RuntimeConfig.MaxConcurrentSubagents)
	require.True(t, got.RuntimeConfig.ThinkingExplicit)
	require.True(t, got.RuntimeConfig.ReasoningEffortExplicit)
	require.False(t, got.RuntimeConfig.ThinkingEnabled)
	require.Empty(t, got.RuntimeConfig.ReasoningEffort)
	require.Zero(t, definitionCalls)
}

func TestADKAgentFactoryAppliesDurableLeadPromptOverlay(t *testing.T) {
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	var gotModelID int64
	defaultTemperature := float32(0.4)
	overlayCalls := 0
	factory := NewApplicationADKAgentFactory(
		func(_ context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			gotModelID = modelID
			return chatModel, true, nil
		},
		nil,
		nil,
		WithADKLeadPromptOverlayProvider(ADKLeadPromptOverlayProviderFunc(
			func(context.Context, *RunSummary) (ADKLeadPromptOverlay, bool, error) {
				overlayCalls++
				return ADKLeadPromptOverlay{
					AgentName:        "reviewer",
					AgentDescription: "Reviews production changes",
					Instructions:     "Review correctness and safety.",
					ModelDefaults: modelExecutorConfig{
						ModelID:     2002,
						Temperature: &defaultTemperature,
					},
				}, true, nil
			},
		)),
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		AssistantID: "singleagent:1001",
		Config:      `{"mode":"pro"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "reviewer", agent.Name(context.Background()))
	require.Equal(t, "Reviews production changes", agent.Description(context.Background()))
	require.Equal(t, int64(2002), gotModelID)
	require.Equal(t, 1, overlayCalls)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("review")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Contains(t, chatModel.messages[0].Content, `<agent_overlay source="durable_single_agent">`)
	require.Contains(t, chatModel.messages[0].Content, "Review correctness and safety.")
	require.NotNil(t, chatModel.options.Temperature)
	require.InDelta(t, float32(0.4), *chatModel.options.Temperature, 0.0001)
}

func TestADKAgentFactoryPreservesMaxIterations(t *testing.T) {
	loopTool, err := toolutils.InferTool(
		"loop",
		"Continue the loop.",
		func(context.Context, struct{}) (string, error) {
			return "continue", nil
		},
	)
	require.NoError(t, err)

	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-loop",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "loop",
				Arguments: `{}`,
			},
		}}),
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ADKToolProviderFunc(func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
			return []tool.BaseTool{loopTool}, nil
		}),
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{"max_iterations":2}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("loop")},
	})

	var terminalErr error
	for _, event := range events {
		if event.Err != nil {
			terminalErr = event.Err
		}
	}
	require.ErrorIs(t, terminalErr, adk.ErrExceedMaxIterations)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKAgentFactoryConfiguresModelRetry(t *testing.T) {
	chatModel := &flakyADKChatModel{
		failuresBeforeSuccess: 1,
		success:               schema.AssistantMessage("done after retry", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_retry":{
				"max_retries":1,
				"backoff_ms":0
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("retry once")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKAgentFactoryModelRetryExhaustionUsesEinoError(t *testing.T) {
	chatModel := &flakyADKChatModel{
		failuresBeforeSuccess: 3,
		success:               schema.AssistantMessage("unused", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_retry":{
				"max_retries":1,
				"backoff_ms":0
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("retry exhaust")},
	})

	require.NotEmpty(t, events)
	var exhausted *adk.RetryExhaustedError
	require.ErrorAs(t, events[len(events)-1].Err, &exhausted)
	require.Equal(t, 1, exhausted.TotalRetries)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKAgentFactoryConfiguresModelFailoverCandidates(t *testing.T) {
	primary := &flakyADKChatModel{
		failuresBeforeSuccess: 10,
		success:               schema.AssistantMessage("unused", nil),
	}
	backup := &flakyADKChatModel{
		success: schema.AssistantMessage("done from backup", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(_ context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			switch modelID {
			case 1001:
				return primary, true, nil
			case 2002:
				return backup, true, nil
			default:
				return nil, false, nil
			}
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_id":1001,
			"model_failover":{
				"candidate_model_ids":[2002]
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("fail over")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, 1, primary.calls)
	require.Equal(t, 1, backup.calls)
}

func TestADKAgentFactoryFailoverAfterModelRetryExhaustion(t *testing.T) {
	primary := &flakyADKChatModel{
		failuresBeforeSuccess: 10,
		success:               schema.AssistantMessage("unused", nil),
	}
	backup := &flakyADKChatModel{
		success: schema.AssistantMessage("done after failover", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(_ context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			switch modelID {
			case 1001:
				return primary, true, nil
			case 2002:
				return backup, true, nil
			default:
				return nil, false, nil
			}
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_id":1001,
			"model_retry":{
				"max_retries":1,
				"backoff_ms":0
			},
			"model_failover":{
				"candidate_model_ids":[2002]
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("retry then fail over")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, 2, primary.calls)
	require.Equal(t, 1, backup.calls)
}

func TestADKAgentFactoryFailoverCandidateRequiresCapabilityCoverage(t *testing.T) {
	primary := &capabilityFlakyADKChatModel{
		flakyADKChatModel: flakyADKChatModel{
			failuresBeforeSuccess: 10,
			success:               schema.AssistantMessage("unused", nil),
		},
		capabilities: ADKModelCapabilities{Vision: true},
	}
	backup := &flakyADKChatModel{
		success: schema.AssistantMessage("should not run", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(_ context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			switch modelID {
			case 1001:
				return primary, true, nil
			case 2002:
				return backup, true, nil
			default:
				return nil, false, nil
			}
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_id":1001,
			"model_failover":{
				"candidate_model_ids":[2002]
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("fail over safely")},
	})

	require.NotEmpty(t, events)
	require.ErrorContains(
		t,
		events[len(events)-1].Err,
		"failover model capabilities do not cover primary model",
	)
	require.Equal(t, 1, primary.calls)
	require.Zero(t, backup.calls)
}

func TestADKAgentFactoryPassesProviderCapabilitiesToMiddleware(t *testing.T) {
	chatModel := &providerCapabilityChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("done", nil),
		},
		capabilities: ADKModelCapabilities{
			Thinking:  true,
			Reasoning: true,
			Vision:    true,
			PDF:       true,
			File:      true,
			Audio:     true,
			Video:     true,
		},
	}
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"mode":"pro",
			"thinking_enabled":false,
			"reasoning_effort":"",
			"is_plan_mode":false
		}`,
	})

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.True(t, got.ModelCapabilities.Thinking)
	require.True(t, got.ModelCapabilities.Reasoning)
	require.True(t, got.ModelCapabilities.Vision)
	require.True(t, got.ModelCapabilities.PDF)
	require.True(t, got.ModelCapabilities.File)
	require.True(t, got.ModelCapabilities.Audio)
	require.True(t, got.ModelCapabilities.Video)
	require.False(t, got.RuntimeConfig.ModeExplicit)
	require.False(t, got.RuntimeConfig.ThinkingEnabled)
}

func TestADKAgentFactoryDoesNotProjectRetiredModeDefaultReasoningOptions(t *testing.T) {
	chatModel := &reasoningProjectingChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("done", nil),
		},
		capabilities: ADKModelCapabilities{Thinking: true, Reasoning: true},
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{Config: `{"mode":"pro"}`})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("plan")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Zero(t, chatModel.reasoningProjects)
	require.Equal(t, ADKReasoningRequest{}, chatModel.reasoningRequest)
	require.Empty(t, chatModel.options.Stop)
}

func TestADKAgentFactoryDowngradesUnsupportedModeReasoning(t *testing.T) {
	chatModel := &recordingChatModel{resp: schema.AssistantMessage("done", nil)}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{"mode":"pro"}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("plan")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
}

func TestADKAgentFactoryProjectsReasoningOptions(t *testing.T) {
	chatModel := &reasoningProjectingChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("done", nil),
		},
		capabilities: ADKModelCapabilities{
			Thinking:  true,
			Reasoning: true,
		},
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"reasoning_effort":"high",
			"thinking_enabled":true
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("think")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, ADKReasoningRequest{
		ReasoningEffort: "high",
		ThinkingEnabled: true,
	}, chatModel.reasoningRequest)
	require.Equal(t, []string{"reasoning:high", "thinking:true"}, chatModel.options.Stop)
}

func TestADKAgentFactoryUsesAdaptiveFactsToNeutralizeRetiredReasoningControls(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.Config = `{
		"mode":"ultra",
		"reasoning_effort":"high",
		"thinking_enabled":true
	}`
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	chatModel := &reasoningProjectingChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("done", nil),
		},
		capabilities: ADKModelCapabilities{Thinking: true, Reasoning: true},
	}
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(
		withAdaptiveBootstrapFacts(context.Background(), facts),
		run,
	)

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.True(t, got.RuntimeConfig.ThinkingExplicit)
	require.True(t, got.RuntimeConfig.ReasoningEffortExplicit)
	require.False(t, got.RuntimeConfig.ThinkingEnabled)
	require.Empty(t, got.RuntimeConfig.ReasoningEffort)
	require.Zero(t, chatModel.reasoningProjects)
	require.Nil(t, chatModel.options)
}

func TestADKAgentFactoryPreservesHistoricalCamelCaseReasoningOptions(t *testing.T) {
	chatModel := &reasoningProjectingChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("done", nil),
		},
		capabilities: ADKModelCapabilities{
			Thinking:  true,
			Reasoning: true,
		},
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"reasoningEffort":"high",
			"thinkingEnabled":true
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("think")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, ADKReasoningRequest{
		ReasoningEffort: "high",
		ThinkingEnabled: true,
	}, chatModel.reasoningRequest)
}

func TestADKAgentFactoryRequiresProjectorForSupportedReasoningRequest(t *testing.T) {
	chatModel := &providerCapabilityChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("unused", nil),
		},
		capabilities: ADKModelCapabilities{Reasoning: true},
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{"reasoning_effort":"medium"}`,
	})

	require.ErrorContains(t, err, "reasoning option projector is required")
	require.Nil(t, agent)
}

func TestADKBuiltInReasoningModelCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		model     model.BaseChatModel
		reasoning bool
		thinking  bool
	}{
		{
			name:      "openai",
			model:     (*openaimodel.ChatModel)(nil),
			reasoning: true,
		},
		{
			name:      "ark",
			model:     (*arkmodel.ChatModel)(nil),
			reasoning: true,
			thinking:  true,
		},
		{
			name:     "claude",
			model:    (*claudemodel.ChatModel)(nil),
			thinking: true,
		},
		{
			name:     "qwen",
			model:    (*qwenmodel.ChatModel)(nil),
			thinking: true,
		},
		{
			name:     "gemini",
			model:    (*geminimodel.ChatModel)(nil),
			thinking: true,
		},
		{
			name:     "deepseek",
			model:    (*deepseekmodel.ChatModel)(nil),
			thinking: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capabilities := adkBuiltInModelCapabilities(tt.model)

			require.Equal(t, tt.reasoning, capabilities.Reasoning)
			require.Equal(t, tt.thinking, capabilities.Thinking)
		})
	}
}

func TestADKBuiltInReasoningModelOptions(t *testing.T) {
	tests := []struct {
		name    string
		model   model.BaseChatModel
		request ADKReasoningRequest
		wantLen int
		wantErr string
	}{
		{
			name:    "openai reasoning effort",
			model:   (*openaimodel.ChatModel)(nil),
			request: ADKReasoningRequest{ReasoningEffort: "high"},
			wantLen: 1,
		},
		{
			name:  "ark reasoning and thinking",
			model: (*arkmodel.ChatModel)(nil),
			request: ADKReasoningRequest{
				ReasoningEffort: "medium",
				ThinkingEnabled: true,
			},
			wantLen: 2,
		},
		{
			name:    "qwen thinking",
			model:   (*qwenmodel.ChatModel)(nil),
			request: ADKReasoningRequest{ThinkingEnabled: true},
			wantLen: 1,
		},
		{
			name:    "gemini thinking",
			model:   (*geminimodel.ChatModel)(nil),
			request: ADKReasoningRequest{ThinkingEnabled: true},
			wantLen: 1,
		},
		{
			name:    "claude thinking",
			model:   (*claudemodel.ChatModel)(nil),
			request: ADKReasoningRequest{ThinkingEnabled: true},
			wantLen: 1,
		},
		{
			name:    "deepseek thinking",
			model:   (*deepseekmodel.ChatModel)(nil),
			request: ADKReasoningRequest{ThinkingEnabled: true},
			wantLen: 1,
		},
		{
			name:    "qwen reasoning effort unsupported",
			model:   (*qwenmodel.ChatModel)(nil),
			request: ADKReasoningRequest{ReasoningEffort: "high"},
			wantErr: "reasoning_effort is not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options, ok, err := adkBuiltInReasoningModelOptions(
				tt.model,
				tt.request,
			)

			require.True(t, ok)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, options, tt.wantLen)
		})
	}
}

func TestADKAgentFactoryUsesPolicyFilteredToolSetForModelAndMiddleware(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("done", nil),
	}
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		NewADKToolPolicyProvider(&recordingADKToolSetProvider{
			set: ADKToolSet{
				StaticTools: []tool.BaseTool{
					&namedTestTool{name: "safe_static"},
					&namedTestTool{name: "blocked_static"},
				},
				DynamicTools: []tool.BaseTool{
					&namedTestTool{name: "safe_dynamic"},
					&namedTestTool{name: "blocked_dynamic"},
				},
				SubagentToolNames: []string{"safe_static", "blocked_static"},
			},
		}),
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"tool_policy":{
				"allowed_tools":["safe_static"],
				"allowed_dynamic_tools":["safe_dynamic"]
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(
		t,
		[]string{"safe_static"},
		adkToolInfoNames(chatModel.options.Tools),
	)
	require.Equal(
		t,
		[]string{"safe_static"},
		adkToolNames(t, context.Background(), got.StaticTools),
	)
	require.Equal(t, []string{"safe_static"}, got.SubagentToolNames)
	require.Equal(
		t,
		[]string{"safe_dynamic"},
		adkToolNames(t, context.Background(), got.DynamicTools),
	)
}

func TestDefaultADKToolProviderIncludesHumanInteractionTools(t *testing.T) {
	provider := NewDefaultADKToolProvider()

	tools, err := provider.ResolveTools(context.Background(), &RunSummary{RunID: 1})

	require.NoError(t, err)
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		info, infoErr := item.Info(context.Background())
		require.NoError(t, infoErr)
		names = append(names, info.Name)
	}
	require.ElementsMatch(t, []string{
		adkClarificationToolName,
		adkDeerFlowClarificationToolName,
		adkConfirmationToolName,
	}, names)
}

func adkToolInfoNames(tools []*schema.ToolInfo) []string {
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		if item != nil {
			names = append(names, item.Name)
		}
	}
	return names
}

type providerCapabilityChatModel struct {
	recordingChatModel
	capabilities ADKModelCapabilities
}

func (m *providerCapabilityChatModel) ADKProviderCapabilities() ADKModelCapabilities {
	if m == nil {
		return ADKModelCapabilities{}
	}
	return m.capabilities
}

type capabilityFlakyADKChatModel struct {
	flakyADKChatModel
	capabilities ADKModelCapabilities
}

func (m *capabilityFlakyADKChatModel) ADKProviderCapabilities() ADKModelCapabilities {
	if m == nil {
		return ADKModelCapabilities{}
	}
	return m.capabilities
}

type reasoningProjectingChatModel struct {
	recordingChatModel
	capabilities      ADKModelCapabilities
	reasoningRequest  ADKReasoningRequest
	reasoningProjects int
}

func (m *reasoningProjectingChatModel) ADKProviderCapabilities() ADKModelCapabilities {
	if m == nil {
		return ADKModelCapabilities{}
	}
	return m.capabilities
}

func (m *reasoningProjectingChatModel) ProjectADKReasoningOptions(
	request ADKReasoningRequest,
) ([]model.Option, error) {
	m.reasoningRequest = request
	m.reasoningProjects++
	return []model.Option{
		model.WithStop([]string{
			"reasoning:" + request.ReasoningEffort,
			fmt.Sprintf("thinking:%t", request.ThinkingEnabled),
		}),
	}, nil
}

func TestADKAgentFactoryRejectsUnconfiguredModel(t *testing.T) {
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return nil, false, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{})

	require.ErrorContains(t, err, "agent thread chat model is not configured")
	require.Nil(t, agent)
}

func collectADKAgentEvents(
	t *testing.T,
	agent adk.ResumableAgent,
	input *adk.AgentInput,
) []*adk.AgentEvent {
	t.Helper()

	iter := agent.Run(context.Background(), input)
	events := make([]*adk.AgentEvent, 0)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		events = append(events, event)
	}

	return events
}

type flakyADKChatModel struct {
	failuresBeforeSuccess int
	success               *schema.Message
	calls                 int
}

func (m *flakyADKChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	m.calls++
	if m.calls <= m.failuresBeforeSuccess {
		return nil, fmt.Errorf("temporary provider error %d", m.calls)
	}

	return m.success, nil
}

func (m *flakyADKChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream is not implemented")
}
