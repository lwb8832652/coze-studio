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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKMiddlewareAssemblyPreservesHookAndWrapperOrder(t *testing.T) {
	var calls []string
	builders := make(map[ADKMiddlewareName]ADKMiddlewareBuilder, len(adkMiddlewareOrder))
	for _, name := range adkMiddlewareOrder {
		name := name
		builders[name] = func(
			context.Context,
			ADKMiddlewareBuildInput,
		) (adk.ChatModelAgentMiddleware, error) {
			return &recordingOrderedMiddleware{name: string(name), calls: &calls}, nil
		}
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		Builders: builders,
	})
	baseModel := &orderedMiddlewareModel{calls: &calls}

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   &RunSummary{RunID: 20},
		Model: baseModel,
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "middleware order test",
		Model:       baseModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)

	expected := make([]string, 0, len(adkMiddlewareOrder)*4+1)
	for _, name := range adkMiddlewareOrder {
		expected = append(expected, "before:"+string(name))
	}
	for _, name := range adkMiddlewareOrder {
		expected = append(expected, "enter:"+string(name))
	}
	expected = append(expected, "model")
	for i := len(adkMiddlewareOrder) - 1; i >= 0; i-- {
		expected = append(expected, "exit:"+string(adkMiddlewareOrder[i]))
	}
	for _, name := range adkMiddlewareOrder {
		expected = append(expected, "after:"+string(name))
	}
	require.Equal(t, expected, calls)
}

func TestADKMiddlewareOmitsDisabledOptionalCapabilities(t *testing.T) {
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   &RunSummary{RunID: 20, Config: `{"mode":"flash"}`},
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})

	require.NoError(t, err)
	require.Equal(t, []ADKMiddlewareName{
		ADKMiddlewareUploadedFiles,
		ADKMiddlewarePatchTools,
		ADKMiddlewareToolErrorNormalization,
		ADKMiddlewareSummarization,
		ADKMiddlewareProviderCapability,
		ADKMiddlewareMultimodal,
		ADKMiddlewareContextBudget,
		ADKMiddlewareSafetyFinish,
		ADKMiddlewareSemanticLoop,
	}, bundle.HandlerNames)
	require.Len(t, bundle.Handlers, len(bundle.HandlerNames))
	for _, handler := range bundle.Handlers {
		require.NotContains(t, fmt.Sprintf("%T", handler), "reservedADKMiddleware")
	}
}

func TestADKModelProjectionAndAccountingOrder(t *testing.T) {
	require.NotEmpty(t, adkMiddlewareOrder)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareProviderCapability),
		adkMiddlewareIndex(ADKMiddlewareMultimodal),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareMultimodal),
		adkMiddlewareIndex(ADKMiddlewareToolSearch),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolSearch),
		adkMiddlewareIndex(ADKMiddlewareContextBudget),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareContextBudget),
		adkMiddlewareIndex(ADKMiddlewareSafetyFinish),
	)
}

func TestADKProviderCapabilityRunsImmediatelyBeforeMultimodalProjection(t *testing.T) {
	require.Equal(
		t,
		adkMiddlewareIndex(ADKMiddlewareMultimodal)-1,
		adkMiddlewareIndex(ADKMiddlewareProviderCapability),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareFilesystem),
		adkMiddlewareIndex(ADKMiddlewareProviderCapability),
	)
}

func TestADKMiddlewareWiresProviderCapabilityDowngradeEventSink(t *testing.T) {
	events := &recordingRunEventSink{}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		EventSink: events,
	})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   &RunSummary{RunID: 20, ThreadID: 10, Config: `{"mode":"pro"}`},
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})
	require.NoError(t, err)

	_, _, err = requireADKMiddleware(t, bundle, ADKMiddlewareProviderCapability).
		BeforeAgent(context.Background(), &adk.ChatModelAgentContext{})

	require.NoError(t, err)
	require.Equal(t, []string{adkProviderCapabilityDowngradedEventType}, events.eventTypes())
}

func TestADKMiddlewareUsesClientToolSearchByDefault(t *testing.T) {
	dynamicTool, err := toolutils.InferTool(
		"search_docs",
		"Search task documents.",
		func(context.Context, struct{}) (string, error) {
			return "ok", nil
		},
	)
	require.NoError(t, err)
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:          &RunSummary{RunID: 20},
		Model:        &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
		DynamicTools: []tool.BaseTool{dynamicTool},
	})
	require.NoError(t, err)
	runCtx := &adk.ChatModelAgentContext{}
	_, runCtx, err = requireADKMiddleware(t, bundle, ADKMiddlewareToolSearch).
		BeforeAgent(context.Background(), runCtx)

	require.NoError(t, err)
	require.Nil(t, runCtx.ToolSearchTool)
	require.Len(t, runCtx.Tools, 2)
}

func TestADKMiddlewareUsesNativeToolSearchOnlyWhenCapabilityDeclared(t *testing.T) {
	dynamicTool, err := toolutils.InferTool(
		"search_docs",
		"Search task documents.",
		func(context.Context, struct{}) (string, error) {
			return "ok", nil
		},
	)
	require.NoError(t, err)
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:          &RunSummary{RunID: 20},
		Model:        &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
		DynamicTools: []tool.BaseTool{dynamicTool},
		ModelCapabilities: ADKModelCapabilities{
			NativeToolSearch: true,
		},
	})
	require.NoError(t, err)
	runCtx := &adk.ChatModelAgentContext{}
	_, runCtx, err = requireADKMiddleware(t, bundle, ADKMiddlewareToolSearch).
		BeforeAgent(context.Background(), runCtx)

	require.NoError(t, err)
	require.NotNil(t, runCtx.ToolSearchTool)
	require.Len(t, runCtx.Tools, 2)
}

func TestADKMiddlewareRejectsToolAssignedStaticAndDynamic(t *testing.T) {
	sharedTool, err := toolutils.InferTool(
		"search_docs",
		"Search task documents.",
		func(context.Context, struct{}) (string, error) {
			return "ok", nil
		},
	)
	require.NoError(t, err)
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:          &RunSummary{RunID: 20},
		Model:        &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
		StaticTools:  []tool.BaseTool{sharedTool},
		DynamicTools: []tool.BaseTool{sharedTool},
	})

	require.ErrorContains(t, err, "tool search_docs is configured as both static and dynamic")
	require.Empty(t, bundle.Handlers)
}

func TestADKMiddlewareExposesPlanToolsOnlyWithCozeBackend(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
		bundle, err := assembler.Build(
			context.Background(),
			ADKMiddlewareBuildInput{
				Run:   &RunSummary{RunID: 20},
				Model: &recordingChatModel{},
			},
		)
		require.NoError(t, err)
		require.NotContains(t, bundle.HandlerNames, ADKMiddlewarePlanTask)
	})

	t.Run("configured", func(t *testing.T) {
		store := newMemoryADKPlanStore()
		var builtRun *RunSummary
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
			PlanBackendFactory: ADKPlanBackendFactoryFunc(func(
				_ context.Context,
				run *RunSummary,
			) (plantask.Backend, error) {
				builtRun = run
				return NewADKPlanBackend(
					run,
					store,
					&recordingRunEventSink{},
				)
			}),
		})
		run := &RunSummary{
			RunID:     20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
			Config:    `{"mode":"pro"}`,
		}
		bundle, err := assembler.Build(
			context.Background(),
			ADKMiddlewareBuildInput{
				Run:   run,
				Model: &recordingChatModel{},
			},
		)
		require.NoError(t, err)
		require.Same(t, run, builtRun)
		runCtx := &adk.ChatModelAgentContext{}
		planHandler := requireADKMiddleware(t, bundle, ADKMiddlewarePlanTask)
		_, runCtx, err = planHandler.
			BeforeAgent(context.Background(), runCtx)
		require.NoError(t, err)
		require.Len(t, runCtx.Tools, 5)
		var names []string
		var toolInfos []*schema.ToolInfo
		invokableTools := make(map[string]tool.InvokableTool)
		for _, candidate := range runCtx.Tools {
			info, infoErr := candidate.Info(context.Background())
			require.NoError(t, infoErr)
			toolInfos = append(toolInfos, info)
			names = append(names, info.Name)
			invokable, ok := candidate.(tool.InvokableTool)
			require.True(t, ok)
			invokableTools[info.Name] = invokable
		}
		require.ElementsMatch(t, []string{
			plantask.TaskCreateToolName,
			plantask.TaskGetToolName,
			plantask.TaskUpdateToolName,
			plantask.TaskListToolName,
			adkPlanCompletionGuardToolName,
		}, names)
		_, modelState, err := planHandler.
			BeforeModelRewriteState(context.Background(), &adk.ChatModelAgentState{
				ToolInfos: toolInfos,
			}, nil)
		require.NoError(t, err)
		var visibleToolNames []string
		for _, info := range modelState.ToolInfos {
			visibleToolNames = append(visibleToolNames, info.Name)
		}
		require.ElementsMatch(t, []string{
			plantask.TaskCreateToolName,
			plantask.TaskGetToolName,
			plantask.TaskUpdateToolName,
			plantask.TaskListToolName,
		}, visibleToolNames)

		created, err := invokableTools[plantask.TaskCreateToolName].
			InvokableRun(context.Background(), `{
				"subject":"Run tests",
				"description":"Run focused tests.",
				"activeForm":"Running tests"
			}`)
		require.NoError(t, err)
		require.Contains(t, created, "Task #1 created successfully")

		taskDetail, err := invokableTools[plantask.TaskGetToolName].
			InvokableRun(context.Background(), `{"taskId":"1"}`)
		require.NoError(t, err)
		require.Contains(t, taskDetail, "Run tests")

		_, err = invokableTools[plantask.TaskUpdateToolName].
			InvokableRun(
				context.Background(),
				`{"taskId":"1","status":"in_progress"}`,
			)
		require.NoError(t, err)
		taskList, err := invokableTools[plantask.TaskListToolName].
			InvokableRun(context.Background(), `{}`)
		require.NoError(t, err)
		require.Contains(t, taskList, "#1 [in_progress] Run tests")

		_, err = invokableTools[plantask.TaskUpdateToolName].
			InvokableRun(
				context.Background(),
				`{"taskId":"1","status":"completed"}`,
			)
		require.NoError(t, err)
		taskList, err = invokableTools[plantask.TaskListToolName].
			InvokableRun(context.Background(), `{}`)
		require.NoError(t, err)
		require.Contains(t, taskList, "No tasks found")
	})
}

func TestADKMiddlewareDoesNotBuildPlanBackendOutsidePlanMode(t *testing.T) {
	built := 0
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		PlanBackendFactory: ADKPlanBackendFactoryFunc(func(
			context.Context,
			*RunSummary,
		) (plantask.Backend, error) {
			built++
			return nil, nil
		}),
	})

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   &RunSummary{RunID: 20, Config: `{"mode":"thinking"}`},
		Model: &recordingChatModel{},
	})

	require.NoError(t, err)
	require.Zero(t, built)
	require.NotContains(t, bundle.HandlerNames, ADKMiddlewarePlanTask)
}

func TestADKPlanTaskMiddlewarePrecedesProviderVisionAndDeferredToolPhases(t *testing.T) {
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewarePlanTask),
		adkMiddlewareIndex(ADKMiddlewareProviderCapability),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareProviderCapability),
		adkMiddlewareIndex(ADKMiddlewareMultimodal),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareMultimodal),
		adkMiddlewareIndex(ADKMiddlewareToolSearch),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolSearch),
		adkMiddlewareIndex(ADKMiddlewareContextBudget),
	)
}

func TestADKAfterModelSafetyAndAccountingOrder(t *testing.T) {
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolErrorNormalization),
		adkMiddlewareIndex(ADKMiddlewareSafetyFinish),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareSafetyFinish),
		adkMiddlewareIndex(ADKMiddlewareSubagentLimit),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareSubagentLimit),
		adkMiddlewareIndex(ADKMiddlewareSemanticLoop),
	)
}

func TestADKMiddlewareUsesRunContextBudgetForSummarization(t *testing.T) {
	runAgent := func(
		t *testing.T,
		chatModel *recordingChatModel,
		messages []*schema.Message,
	) {
		t.Helper()
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
		bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
			Run: &RunSummary{
				RunID: 20,
				Config: `{
					"context_budget":{
						"summarization_messages":2
					}
				}`,
			},
			Model: chatModel,
		})
		require.NoError(t, err)
		agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "summarization budget test",
			Model:       chatModel,
			Handlers:    bundle.Handlers,
		})
		require.NoError(t, err)
		events := collectADKAgentEvents(t, agent, &adk.AgentInput{
			Messages: messages,
		})
		require.NotEmpty(t, events)
		require.NoError(t, events[len(events)-1].Err)
	}

	t.Run("over threshold", func(t *testing.T) {
		chatModel := &recordingChatModel{
			resp: schema.AssistantMessage("conversation summary", nil),
		}
		runAgent(t, chatModel, []*schema.Message{
			schema.UserMessage("first"),
			schema.AssistantMessage("second", nil),
			schema.UserMessage("third"),
		})

		require.Equal(t, 2, chatModel.calls)
	})

	t.Run("at threshold", func(t *testing.T) {
		chatModel := &recordingChatModel{
			resp: schema.AssistantMessage("unused summary", nil),
		}
		runAgent(t, chatModel, []*schema.Message{
			schema.UserMessage("first"),
			schema.AssistantMessage("second", nil),
		})

		require.Equal(t, 1, chatModel.calls)
	})
}

func TestADKMiddlewareOmitsReductionWithoutCozeFilesystem(t *testing.T) {
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   &RunSummary{RunID: 20},
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})
	require.NoError(t, err)
	require.NotContains(t, bundle.HandlerNames, ADKMiddlewareReduction)
}

func TestADKMiddlewarePatchToolsUsesCozeRepairPayload(t *testing.T) {
	events := &recordingRunEventSink{}
	run := &RunSummary{
		RunID:    20,
		ThreadID: 10,
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		EventSink: events,
	})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   run,
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})
	require.NoError(t, err)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "search_docs",
					Arguments: `{"query":"deployment"}`,
				},
			}},
		},
	}}

	_, got, err := requireADKMiddleware(t, bundle, ADKMiddlewarePatchTools).
		BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})

	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	require.Equal(t, schema.Tool, got.Messages[1].Role)
	require.Equal(t, "search_docs", got.Messages[1].ToolName)
	require.Equal(t, "call-1", got.Messages[1].ToolCallID)
	require.JSONEq(t, `{
		"schema":"coze.tool_repair.v1",
		"status":"patched",
		"tool_name":"search_docs",
		"tool_call_id":"call-1",
		"reason":"missing_tool_result",
		"message":"Tool result was missing and has been patched by Coze runtime. Continue with available context or retry the tool if needed."
	}`, got.Messages[1].Content)
	require.Len(t, events.events, 1)
	require.Equal(t, "tool.repaired", events.events[0].EventType)
	require.Equal(t, int64(10), events.events[0].ThreadID)
	require.Equal(t, int64(20), events.events[0].RunID)
	require.JSONEq(t, `{
		"schema":"coze.tool_repair.v1",
		"tool_name":"search_docs",
		"tool_call_id":"call-1",
		"reason":"missing_tool_result"
	}`, events.events[0].Payload)
}

func TestADKMiddlewareEnablesReadOnlyOffloadAndReduction(t *testing.T) {
	objectStorage := newRecordingADKOffloadStorage()
	registry := &recordingADKRuntimeFileRegistry{}
	events := &recordingRunEventSink{}
	var builtBackend *ADKOffloadBackend
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		OffloadBackendFactory: ADKOffloadBackendFactoryFunc(func(
			_ context.Context,
			run *RunSummary,
			limits ADKOffloadLimits,
		) (*ADKOffloadBackend, error) {
			var err error
			builtBackend, err = NewADKOffloadBackend(
				run,
				objectStorage,
				registry,
				events,
				limits,
			)
			return builtBackend, err
		}),
		EventSink: events,
	})
	run := &RunSummary{
		RunID:    20,
		ThreadID: 10,
		SpaceID:  30,
		Config: `{
			"tool_result_reduction":{
				"max_length_for_trunc":16
			}
		}`,
	}
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   run,
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})
	require.NoError(t, err)
	require.NotNil(t, builtBackend)

	runCtx := &adk.ChatModelAgentContext{}
	_, runCtx, err = requireADKMiddleware(t, bundle, ADKMiddlewareFilesystem).
		BeforeAgent(context.Background(), runCtx)
	require.NoError(t, err)
	require.Len(t, runCtx.Tools, 1)
	info, err := runCtx.Tools[0].Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "read_file", info.Name)

	handler := requireADKMiddleware(t, bundle, ADKMiddlewareReduction)
	wrapped, err := handler.WrapInvokableToolCall(
		context.Background(),
		func(
			context.Context,
			string,
			...tool.Option,
		) (string, error) {
			return strings.Repeat("large-result-", 8), nil
		},
		&adk.ToolContext{Name: "search_docs", CallID: "call-1"},
	)
	require.NoError(t, err)
	result, err := wrapped(context.Background(), `{}`)
	require.NoError(t, err)
	require.Contains(t, result, "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/")
	require.Len(t, registry.calls, 1)
	require.Len(t, objectStorage.objects, 1)
}

func requireADKMiddleware(
	t *testing.T,
	bundle ADKMiddlewareBundle,
	name ADKMiddlewareName,
) adk.ChatModelAgentMiddleware {
	t.Helper()
	for index, activeName := range bundle.HandlerNames {
		if activeName != name {
			continue
		}
		require.Less(t, index, len(bundle.Handlers))
		return bundle.Handlers[index]
	}
	t.Fatalf("middleware %s is not active; active=%v", name, bundle.HandlerNames)
	return nil
}

func TestADKMiddlewareAttributesOnlySummaryModelCallToMiddleware(t *testing.T) {
	chatModel := &usageKindRecordingChatModel{}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run: &RunSummary{
			RunID: 20,
			Config: `{
				"context_budget":{
					"summarization_messages":2
				}
			}`,
		},
		Model: chatModel,
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "summary usage attribution test",
		Model:       chatModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage("first"),
			schema.AssistantMessage("second", nil),
			schema.UserMessage("third"),
		},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, []string{"summarization", ""}, chatModel.usageKinds)
}

type recordingOrderedMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	name  string
	calls *[]string
}

func (m *recordingOrderedMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	*m.calls = append(*m.calls, "before:"+m.name)
	return ctx, state, nil
}

func (m *recordingOrderedMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	*m.calls = append(*m.calls, "after:"+m.name)
	return ctx, state, nil
}

func (m *recordingOrderedMiddleware) WrapModel(
	ctx context.Context,
	base model.BaseChatModel,
	mc *adk.ModelContext,
) (model.BaseChatModel, error) {
	return &orderedMiddlewareWrapper{
		base:  base,
		name:  m.name,
		calls: m.calls,
	}, nil
}

type orderedMiddlewareModel struct {
	calls *[]string
}

func (m *orderedMiddlewareModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	*m.calls = append(*m.calls, "model")
	return schema.AssistantMessage("done", nil), nil
}

func (m *orderedMiddlewareModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	*m.calls = append(*m.calls, "model")
	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage("done", nil),
	}), nil
}

type orderedMiddlewareWrapper struct {
	base  model.BaseChatModel
	name  string
	calls *[]string
}

func (m *orderedMiddlewareWrapper) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	*m.calls = append(*m.calls, "enter:"+m.name)
	output, err := m.base.Generate(ctx, input, options...)
	*m.calls = append(*m.calls, "exit:"+m.name)
	return output, err
}

func (m *orderedMiddlewareWrapper) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	*m.calls = append(*m.calls, "enter:"+m.name)
	output, err := m.base.Stream(ctx, input, options...)
	*m.calls = append(*m.calls, "exit:"+m.name)
	return output, err
}

func adkMiddlewareIndex(name ADKMiddlewareName) int {
	for index, current := range adkMiddlewareOrder {
		if current == name {
			return index
		}
	}
	panic(fmt.Sprintf("middleware %s is not ordered", name))
}

type usageKindRecordingChatModel struct {
	usageKinds []string
}

func (m *usageKindRecordingChatModel) Generate(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.usageKinds = append(m.usageKinds, adkUsageKindFromContext(ctx))
	return schema.AssistantMessage("done", nil), nil
}

func (m *usageKindRecordingChatModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	m.usageKinds = append(m.usageKinds, adkUsageKindFromContext(ctx))
	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage("done", nil),
	}), nil
}
