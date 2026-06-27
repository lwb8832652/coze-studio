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

func TestADKMultimodalBudgetIsLastModelStateRewriter(t *testing.T) {
	require.NotEmpty(t, adkMiddlewareOrder)
	require.Equal(
		t,
		ADKMiddlewareMultimodal,
		adkMiddlewareOrder[len(adkMiddlewareOrder)-1],
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareSummarization),
		adkMiddlewareIndex(ADKMiddlewareMultimodal),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareReduction),
		adkMiddlewareIndex(ADKMiddlewareMultimodal),
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
	_, runCtx, err = bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareToolSearch)].
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
	_, runCtx, err = bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareToolSearch)].
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
		runCtx := &adk.ChatModelAgentContext{}
		_, runCtx, err = bundle.Handlers[adkMiddlewareIndex(ADKMiddlewarePlanTask)].
			BeforeAgent(context.Background(), runCtx)
		require.NoError(t, err)
		require.Empty(t, runCtx.Tools)
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
		_, runCtx, err = bundle.Handlers[adkMiddlewareIndex(ADKMiddlewarePlanTask)].
			BeforeAgent(context.Background(), runCtx)
		require.NoError(t, err)
		require.Len(t, runCtx.Tools, 4)
		var names []string
		invokableTools := make(map[string]tool.InvokableTool)
		for _, candidate := range runCtx.Tools {
			info, infoErr := candidate.Info(context.Background())
			require.NoError(t, infoErr)
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
		}, names)

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

func TestADKPlanTaskMiddlewarePrecedesContextAndPolicyWrappers(t *testing.T) {
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolSearch),
		adkMiddlewareIndex(ADKMiddlewarePlanTask),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewarePlanTask),
		adkMiddlewareIndex(ADKMiddlewareContextBudget),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewarePlanTask),
		adkMiddlewareIndex(ADKMiddlewarePolicy),
	)
}

func TestADKMiddlewareSemanticLoopFollowsToolNormalizationAndPrecedesPolicy(t *testing.T) {
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolErrorNormalization),
		adkMiddlewareIndex(ADKMiddlewareSemanticLoop),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareSemanticLoop),
		adkMiddlewareIndex(ADKMiddlewarePolicy),
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

func TestADKMiddlewareKeepsReductionDisabledWithoutCozeFilesystem(t *testing.T) {
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run:   &RunSummary{RunID: 20},
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})
	require.NoError(t, err)
	largeResult := strings.Repeat("tool output ", 10000)
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
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			ToolName:   "search_docs",
			Content:    largeResult,
		},
	}}

	_, got, err := bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareReduction)].
		BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})

	require.NoError(t, err)
	require.Equal(t, largeResult, got.Messages[1].Content)
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

	_, got, err := bundle.Handlers[adkMiddlewareIndex(ADKMiddlewarePatchTools)].
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
	_, runCtx, err = bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareFilesystem)].
		BeforeAgent(context.Background(), runCtx)
	require.NoError(t, err)
	require.Len(t, runCtx.Tools, 1)
	info, err := runCtx.Tools[0].Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "read_file", info.Name)

	handler := bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareReduction)]
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
