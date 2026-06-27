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

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKGuardrailRuntimeToolApprovedResumeInvokesOriginalToolOnce(
	t *testing.T,
) {
	invoker := &recordingADKRuntimeToolInvoker{result: "tool-result"}
	enforcer := confirmationRequiredGuardrailEnforcer()
	toolSet := guardrailRuntimeToolSet(t, invoker, enforcer)
	runnable := guardrailResumeToolGraph(t, toolSet.StaticTools, "runtime-approve")

	prompt, interruptID := requireGuardrailGraphInterrupt(
		t,
		runnable,
		"runtime-approve",
		"search_docs",
		`{"query":"secret customer path"}`,
	)
	messages, err := runnable.Invoke(
		compose.ResumeWithData(
			context.Background(),
			interruptID,
			HumanInteractionResponse{
				Schema:        humanInteractionResponseSchema,
				InteractionID: prompt.InteractionID,
				Kind:          HumanInteractionKindConfirmation,
				Decision:      HumanInteractionDecisionApproved,
			},
		),
		nil,
		compose.WithCheckPointID("runtime-approve"),
	)

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Contains(t, messages[0].Content, "tool-result")
	require.Equal(t, 1, invoker.calls)
	require.Len(t, enforcer.requests, 1)
}

func TestADKGuardrailRuntimeToolRejectedResumeDoesNotInvokeOriginalTool(
	t *testing.T,
) {
	invoker := &recordingADKRuntimeToolInvoker{result: "tool-result"}
	enforcer := confirmationRequiredGuardrailEnforcer()
	toolSet := guardrailRuntimeToolSet(t, invoker, enforcer)
	runnable := guardrailResumeToolGraph(t, toolSet.StaticTools, "runtime-reject")

	prompt, interruptID := requireGuardrailGraphInterrupt(
		t,
		runnable,
		"runtime-reject",
		"search_docs",
		`{"query":"secret customer path"}`,
	)
	_, err := runnable.Invoke(
		compose.ResumeWithData(
			context.Background(),
			interruptID,
			HumanInteractionResponse{
				Schema:        humanInteractionResponseSchema,
				InteractionID: prompt.InteractionID,
				Kind:          HumanInteractionKindConfirmation,
				Decision:      HumanInteractionDecisionRejected,
				Comment:       "not now",
			},
		),
		nil,
		compose.WithCheckPointID("runtime-reject"),
	)

	require.Error(t, err)
	var rejected *GuardrailConfirmationRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Zero(t, invoker.calls)
	require.Len(t, enforcer.requests, 1)
	require.NotContains(t, err.Error(), "secret customer path")
}

func TestADKGuardrailSubagentToolApprovedResumeInvokesSubagentOnce(
	t *testing.T,
) {
	enforcer := confirmationRequiredGuardrailEnforcer()
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("research complete", nil),
	}
	provider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			return []ADKSubagentDefinition{{
				Name:        "researcher",
				Description: "Research public information.",
			}}, nil
		}),
		ADKSubagentAgentFactoryFunc(func(
			ctx context.Context,
			_ *RunSummary,
			definition ADKSubagentDefinition,
		) (adk.Agent, error) {
			return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
				Name:        definition.Name,
				Description: definition.Description,
				Model:       chatModel,
			})
		}),
		WithADKSubagentToolProviderGuardrailEnforcer(enforcer),
	)
	toolSet, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	runnable := guardrailResumeToolGraph(t, toolSet.StaticTools, "subagent-approve")

	prompt, interruptID := requireGuardrailGraphInterrupt(
		t,
		runnable,
		"subagent-approve",
		"researcher",
		`{"request":"find secret context"}`,
	)
	messages, err := runnable.Invoke(
		compose.ResumeWithData(
			context.Background(),
			interruptID,
			HumanInteractionResponse{
				Schema:        humanInteractionResponseSchema,
				InteractionID: prompt.InteractionID,
				Kind:          HumanInteractionKindConfirmation,
				Decision:      HumanInteractionDecisionApproved,
			},
		),
		nil,
		compose.WithCheckPointID("subagent-approve"),
	)

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Contains(t, messages[0].Content, "research complete")
	require.Equal(t, 1, chatModel.calls)
	require.Len(t, enforcer.requests, 1)
}

func TestADKGuardrailSubagentToolRejectedResumeDoesNotInvokeSubagent(
	t *testing.T,
) {
	enforcer := confirmationRequiredGuardrailEnforcer()
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("research complete", nil),
	}
	provider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			return []ADKSubagentDefinition{{
				Name:        "researcher",
				Description: "Research public information.",
			}}, nil
		}),
		ADKSubagentAgentFactoryFunc(func(
			ctx context.Context,
			_ *RunSummary,
			definition ADKSubagentDefinition,
		) (adk.Agent, error) {
			return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
				Name:        definition.Name,
				Description: definition.Description,
				Model:       chatModel,
			})
		}),
		WithADKSubagentToolProviderGuardrailEnforcer(enforcer),
	)
	toolSet, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	runnable := guardrailResumeToolGraph(t, toolSet.StaticTools, "subagent-reject")

	prompt, interruptID := requireGuardrailGraphInterrupt(
		t,
		runnable,
		"subagent-reject",
		"researcher",
		`{"request":"find secret context"}`,
	)
	_, err = runnable.Invoke(
		compose.ResumeWithData(
			context.Background(),
			interruptID,
			HumanInteractionResponse{
				Schema:        humanInteractionResponseSchema,
				InteractionID: prompt.InteractionID,
				Kind:          HumanInteractionKindConfirmation,
				Decision:      HumanInteractionDecisionRejected,
				Comment:       "not now",
			},
		),
		nil,
		compose.WithCheckPointID("subagent-reject"),
	)

	require.Error(t, err)
	var rejected *GuardrailConfirmationRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Zero(t, chatModel.calls)
	require.Len(t, enforcer.requests, 1)
	require.NotContains(t, err.Error(), "secret context")
}

func confirmationRequiredGuardrailEnforcer() *recordingADKGuardrailEnforcer {
	return &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Decision: GuardrailDecision{
				Action:     GuardrailActionConfirm,
				Provider:   "scanner",
				ReasonCode: "needs_review",
				RuleIDs:    []string{"manual.review"},
			},
			RequiresConfirmation: true,
		},
		err: &GuardrailConfirmationRequiredError{ReasonCode: "needs_review"},
	}
}

func guardrailRuntimeToolSet(
	t *testing.T,
	invoker ADKRuntimeToolInvoker,
	enforcer ADKGuardrailEnforcer,
) ADKToolSet {
	t.Helper()
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKGuardrailRuntimeToolCatalog(
			&recordingADKRuntimeToolCatalog{
				definitions: []ADKRuntimeToolDefinition{
					{
						Name:        "search_docs",
						Description: "Search docs.",
						Invoker:     invoker,
					},
				},
			},
			enforcer,
		),
	)
	toolSet, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	return toolSet
}

func guardrailResumeToolGraph(
	t *testing.T,
	tools []einotool.BaseTool,
	name string,
) compose.Runnable[*schema.Message, []*schema.Message] {
	t.Helper()
	ctx := context.Background()
	node, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: tools,
	})
	require.NoError(t, err)
	graph := compose.NewGraph[*schema.Message, []*schema.Message]()
	require.NoError(t, graph.AddToolsNode("tools", node))
	require.NoError(t, graph.AddEdge(compose.START, "tools"))
	require.NoError(t, graph.AddEdge("tools", compose.END))
	runnable, err := graph.Compile(
		ctx,
		compose.WithGraphName(name),
		compose.WithCheckPointStore(&guardrailResumeCheckpointStore{
			values: map[string][]byte{},
		}),
	)
	require.NoError(t, err)
	return runnable
}

func requireGuardrailGraphInterrupt(
	t *testing.T,
	runnable compose.Runnable[*schema.Message, []*schema.Message],
	checkpointID string,
	toolName string,
	arguments string,
) (*HumanInteractionPrompt, string) {
	t.Helper()
	_, err := runnable.Invoke(
		context.Background(),
		schema.AssistantMessage("", []schema.ToolCall{
			{
				ID: "call_1",
				Function: schema.FunctionCall{
					Name:      toolName,
					Arguments: arguments,
				},
			},
		}),
		compose.WithCheckPointID(checkpointID),
	)
	require.Error(t, err)
	info, ok := compose.ExtractInterruptInfo(err)
	require.True(t, ok)
	require.Len(t, info.InterruptContexts, 1)
	prompt, ok := humanInteractionPromptFromInfo(info.InterruptContexts[0].Info)
	require.True(t, ok)
	require.NotNil(t, prompt)
	require.NotContains(t, prompt.Summary, "secret")
	require.NotContains(t, prompt.Description, "secret")
	return prompt, info.InterruptContexts[0].ID
}

type guardrailResumeCheckpointStore struct {
	values map[string][]byte
}

func (s *guardrailResumeCheckpointStore) Get(
	_ context.Context,
	key string,
) ([]byte, bool, error) {
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	value, ok := s.values[key]
	return append([]byte(nil), value...), ok, nil
}

func (s *guardrailResumeCheckpointStore) Set(
	_ context.Context,
	key string,
	value []byte,
) error {
	if s.values == nil {
		s.values = map[string][]byte{}
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}
