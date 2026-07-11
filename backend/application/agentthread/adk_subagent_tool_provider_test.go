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
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKSubagentToolProviderSkipsDefinitionsWhenCapabilityDisabled(t *testing.T) {
	definitionCalls := 0
	provider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			definitionCalls++
			return []ADKSubagentDefinition{{Name: "writer", Description: "Write"}}, nil
		}),
		nil,
	)

	set, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		Config: `{"mode":"ultra","subagent_enabled":false}`,
	})

	require.NoError(t, err)
	require.Zero(t, definitionCalls)
	require.Empty(t, set.StaticTools)
	require.Empty(t, set.DynamicTools)
}

func TestADKSubagentToolProviderResolvesDefinitionsWhenCapabilityEnabled(t *testing.T) {
	definitionCalls := 0
	provider := NewADKSubagentToolProvider(
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

	set, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		Config: `{"mode":"pro","subagent_enabled":true}`,
	})

	require.NoError(t, err)
	require.Equal(t, 1, definitionCalls)
	require.Empty(t, set.StaticTools)
}

func TestADKRunConfigSubagentDefinitionProviderParsesDefinitions(t *testing.T) {
	provider := NewADKRunConfigSubagentDefinitionProvider()
	run := &RunSummary{
		RunID: 20,
		Config: `{
			"subagents":[{
				"name":"researcher",
				"description":"Research public information.",
				"agent_id":1001,
				"version":"v1",
				"is_draft":true,
				"allowed_tools":["read_file"],
				"allowed_dynamic_tools":["search_docs"],
				"full_chat_history":true
			}]
		}`,
	}

	definitions, err := provider.ResolveADKSubagents(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, []ADKSubagentDefinition{{
		Name:                   "researcher",
		Description:            "Research public information.",
		AgentID:                1001,
		Version:                "v1",
		IsDraft:                true,
		AllowedTools:           []string{"read_file"},
		AllowedDynamicTools:    []string{"search_docs"},
		FullChatHistoryAsInput: true,
	}}, definitions)
}

func TestADKRunConfigSubagentDefinitionProviderValidatesDefinitions(t *testing.T) {
	provider := NewADKRunConfigSubagentDefinitionProvider()

	_, err := provider.ResolveADKSubagents(context.Background(), &RunSummary{
		RunID:  20,
		Config: `{"subagents":[{"name":"bad-name","description":"Bad"}]}`,
	})
	require.ErrorContains(t, err, "invalid subagent tool name")

	_, err = provider.ResolveADKSubagents(context.Background(), &RunSummary{
		RunID: 20,
		Config: `{"subagents":[
			{"name":"researcher","description":"One"},
			{"name":"researcher","description":"Two"}
		]}`,
	})
	require.ErrorContains(t, err, "duplicate subagent tool name")

	_, err = provider.ResolveADKSubagents(context.Background(), &RunSummary{
		RunID:  20,
		Config: `{"subagents":[{"name":"writer"}]}`,
	})
	require.ErrorContains(t, err, "subagent description is required")
}

func TestADKSubagentToolProviderBuildsAgentToolsAndPreservesBaseSet(t *testing.T) {
	base := &recordingADKToolSetProvider{
		set: ADKToolSet{
			StaticTools: []tool.BaseTool{
				&namedTestTool{name: "static_tool"},
			},
			DynamicTools: []tool.BaseTool{
				&namedTestTool{name: "dynamic_tool"},
			},
		},
	}
	definitionProvider := ADKSubagentDefinitionProviderFunc(func(
		context.Context,
		*RunSummary,
	) ([]ADKSubagentDefinition, error) {
		return []ADKSubagentDefinition{{
			Name:        "researcher",
			Description: "Research public information.",
			AgentID:     1001,
			Version:     "v1",
		}}, nil
	})
	factory := ADKSubagentAgentFactoryFunc(func(
		ctx context.Context,
		parent *RunSummary,
		definition ADKSubagentDefinition,
	) (adk.Agent, error) {
		require.Equal(t, int64(20), parent.RunID)
		return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
			Name:        definition.Name,
			Description: definition.Description,
			Model: &recordingChatModel{
				resp: schema.AssistantMessage("research complete", nil),
			},
		})
	})
	provider := NewADKSubagentToolProvider(
		base,
		definitionProvider,
		factory,
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Equal(t, 1, base.resolveToolSetCalls)
	require.ElementsMatch(
		t,
		[]string{"static_tool", "researcher"},
		adkToolNames(t, context.Background(), set.StaticTools),
	)
	require.ElementsMatch(
		t,
		[]string{"dynamic_tool"},
		adkToolNames(t, context.Background(), set.DynamicTools),
	)

	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)
	result, err := researcher.InvokableRun(
		context.Background(),
		`{"request":"find context"}`,
	)
	require.NoError(t, err)
	require.Contains(t, result, "research complete")
}

func TestADKSubagentToolProviderGuardrailAllowsWithMetadataOnlyRequest(
	t *testing.T,
) {
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Allowed:  true,
			Decision: GuardrailDecision{Action: GuardrailActionAllow},
		},
	}
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
				AgentID:     1001,
				Version:     "v1",
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

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:     20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)
	result, err := researcher.InvokableRun(
		context.Background(),
		`{"request":"find secret context"}`,
	)

	require.NoError(t, err)
	require.Contains(t, result, "research complete")
	require.Equal(t, 1, chatModel.calls)
	require.Len(t, enforcer.requests, 1)
	require.Equal(t, GuardrailRequest{
		SpaceID:    30,
		ThreadID:   10,
		RunID:      20,
		UserID:     40,
		TargetType: GuardrailTargetToolCall,
		TargetID:   "researcher",
		Operation:  "invoke",
		Source:     "adk_subagent_tool",
		FailMode:   GuardrailFailClosed,
	}, enforcer.requests[0])
	require.Empty(t, enforcer.requests[0].Metadata)
}

func TestADKSubagentToolProviderGuardrailBlocksBeforeChildRun(
	t *testing.T,
) {
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
	}
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Decision: GuardrailDecision{
				Action:     GuardrailActionDeny,
				ReasonCode: "unsafe_subagent",
			},
		},
		err: &GuardrailDeniedError{ReasonCode: "unsafe_subagent"},
	}
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
		WithADKSubagentToolProviderEventSink(events),
		WithADKSubagentToolProviderRunRecorder(recorder),
		WithADKSubagentToolProviderGuardrailEnforcer(enforcer),
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)
	output, err := researcher.InvokableRun(
		context.Background(),
		`{"request":"find secret context"}`,
	)

	require.Error(t, err)
	var denied *GuardrailDeniedError
	require.ErrorAs(t, err, &denied)
	require.Empty(t, output)
	require.Zero(t, chatModel.calls)
	require.Empty(t, events.events)
	require.Empty(t, recorder.startRequests)
	require.Empty(t, recorder.finishRequests)
	require.Len(t, enforcer.requests, 1)
	require.NotContains(t, err.Error(), "secret context")
}

func TestADKSubagentToolProviderGuardrailInterruptsConfirmBeforeChildRun(
	t *testing.T,
) {
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
	}
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Decision: GuardrailDecision{
				Action:     GuardrailActionConfirm,
				Provider:   "scanner",
				ReasonCode: "needs_review",
				Message:    "contains https://internal.example.local/raw",
				RuleIDs:    []string{"subagent.review"},
			},
			RequiresConfirmation: true,
		},
		err: &GuardrailConfirmationRequiredError{ReasonCode: "needs_review"},
	}
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
		WithADKSubagentToolProviderEventSink(events),
		WithADKSubagentToolProviderRunRecorder(recorder),
		WithADKSubagentToolProviderGuardrailEnforcer(enforcer),
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)
	output, err := researcher.InvokableRun(
		context.Background(),
		`{"request":"find secret context","url":"https://leak.example/file","object":"s3://bucket/key"}`,
	)

	require.Error(t, err)
	require.Empty(t, output)
	require.Zero(t, chatModel.calls)
	require.Empty(t, events.events)
	require.Empty(t, recorder.startRequests)
	require.Empty(t, recorder.finishRequests)
	require.Len(t, enforcer.requests, 1)

	var confirmation *GuardrailConfirmationRequiredError
	require.False(t, errors.As(err, &confirmation))
	prompt := requireGuardrailInterruptPrompt(t, err)
	require.Equal(t, HumanInteractionKindConfirmation, prompt.Kind)
	require.Equal(t, "researcher", prompt.ToolName)
	require.Equal(t, "invoke", prompt.Action)
	require.Contains(t, prompt.Summary, "subagent tool researcher")
	require.Contains(t, prompt.PolicyRef, "needs_review")
	require.Contains(t, prompt.Description, "subagent.review")

	rawPrompt, marshalErr := json.Marshal(prompt)
	require.NoError(t, marshalErr)
	promptJSON := string(rawPrompt)
	for _, forbidden := range []string{
		"secret context",
		"https://leak.example/file",
		"s3://bucket/key",
		"https://internal.example.local/raw",
	} {
		require.NotContains(t, promptJSON, forbidden)
		require.NotContains(t, err.Error(), forbidden)
	}
}

func TestADKSubagentToolProviderEmitsLifecycleEvents(t *testing.T) {
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
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
				AgentID:     1001,
				Version:     "v1",
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
				Model: &recordingChatModel{
					resp: schema.AssistantMessage("research complete", nil),
				},
			})
		}),
		WithADKSubagentToolProviderEventSink(events),
		WithADKSubagentToolProviderRunRecorder(recorder),
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)

	result, err := researcher.InvokableRun(
		context.Background(),
		`{"request":"find secret context"}`,
	)

	require.NoError(t, err)
	require.Contains(t, result, "research complete")
	require.Equal(
		t,
		[]string{"subagent.run.started", "subagent.run.completed"},
		events.eventTypes(),
	)
	require.Equal(t, int64(10), events.events[0].ThreadID)
	require.Equal(t, int64(20), events.events[0].RunID)
	require.Contains(t, events.events[0].Payload, `"status":"running"`)
	require.Contains(t, events.events[0].Payload, `"child_run_id":2001`)
	require.Contains(t, events.events[0].Payload, `"agent_id":1001`)
	require.Contains(t, events.events[0].Payload, `"version":"v1"`)
	require.Contains(t, events.events[0].Payload, `"name":"researcher"`)
	require.Contains(t, events.events[1].Payload, `"status":"succeeded"`)
	require.Contains(t, events.events[1].Payload, `"child_run_id":2001`)
	require.Contains(t, events.events[1].Payload, `"elapsed_ms"`)
	require.NotContains(t, events.events[0].Payload, "secret context")
	require.NotContains(t, events.events[1].Payload, "research complete")
	require.Equal(t, int64(20), recorder.startRequests[0].Parent.RunID)
	require.Equal(t, "researcher", recorder.startRequests[0].Definition.Name)
	require.JSONEq(t, `{"request":"find secret context"}`, recorder.startRequests[0].ArgumentsInJSON)
	require.Equal(t, RunStatusSucceeded, recorder.finishRequests[0].Status)
	require.Equal(t, int64(2001), recorder.finishRequests[0].Child.RunID)
}

func TestADKSubagentToolProviderEmitsFailedLifecycleEvent(t *testing.T) {
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
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
				AgentID:     1001,
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
				Model: &flakyADKChatModel{
					failuresBeforeSuccess: 1,
				},
			})
		}),
		WithADKSubagentToolProviderEventSink(events),
		WithADKSubagentToolProviderRunRecorder(recorder),
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)

	_, err = researcher.InvokableRun(
		context.Background(),
		`{"request":"find secret context"}`,
	)

	require.Error(t, err)
	require.Equal(
		t,
		[]string{"subagent.run.started", "subagent.run.failed"},
		events.eventTypes(),
	)
	require.Contains(t, events.events[1].Payload, `"status":"failed"`)
	require.Contains(t, events.events[1].Payload, `"child_run_id":2001`)
	require.Contains(t, events.events[1].Payload, `"error_message":"temporary provider error 1"`)
	require.NotContains(t, events.events[1].Payload, "secret context")
	require.Equal(t, RunStatusFailed, recorder.finishRequests[0].Status)
	require.Equal(t, "temporary provider error 1", recorder.finishRequests[0].ErrorMessage)
}

func TestADKSubagentToolProviderRecordsChildRunWithoutEventSink(t *testing.T) {
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
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
				AgentID:     1001,
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
				Model: &recordingChatModel{
					resp: schema.AssistantMessage("research complete", nil),
				},
			})
		}),
		WithADKSubagentToolProviderRunRecorder(recorder),
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)

	_, err = researcher.InvokableRun(context.Background(), `{}`)

	require.NoError(t, err)
	require.Len(t, recorder.startRequests, 1)
	require.Len(t, recorder.finishRequests, 1)
	require.Equal(t, RunStatusSucceeded, recorder.finishRequests[0].Status)
}

func TestADKSubagentToolProviderRejectsDuplicateBaseToolName(t *testing.T) {
	provider := NewADKSubagentToolProvider(
		&recordingADKToolSetProvider{
			set: ADKToolSet{
				StaticTools: []tool.BaseTool{
					&namedTestTool{name: "researcher"},
				},
			},
		},
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
			context.Context,
			*RunSummary,
			ADKSubagentDefinition,
		) (adk.Agent, error) {
			return nil, nil
		}),
	)

	_, err := provider.ResolveToolSet(context.Background(), &RunSummary{RunID: 20})

	require.ErrorContains(t, err, "duplicate eino adk tool name")
}

func TestADKSubagentToolProviderEnforcesMaxSubagentsPolicy(t *testing.T) {
	provider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			return []ADKSubagentDefinition{
				{Name: "researcher", Description: "Research public information."},
				{Name: "writer", Description: "Write concise summaries."},
			}, nil
		}),
		ADKSubagentAgentFactoryFunc(func(
			context.Context,
			*RunSummary,
			ADKSubagentDefinition,
		) (adk.Agent, error) {
			require.FailNow(t, "factory should not be called")
			return nil, nil
		}),
	)

	_, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		RunID:  20,
		Config: `{"subagent_policy":{"max_subagents":1}}`,
	})

	require.ErrorContains(t, err, "subagent count limit exceeded")
}

func TestADKSubagentToolProviderEnforcesMaxDepthPolicy(t *testing.T) {
	provider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			return []ADKSubagentDefinition{{
				Name:        "reviewer",
				Description: "Review the child result.",
			}}, nil
		}),
		ADKSubagentAgentFactoryFunc(func(
			context.Context,
			*RunSummary,
			ADKSubagentDefinition,
		) (adk.Agent, error) {
			require.FailNow(t, "factory should not be called")
			return nil, nil
		}),
	)

	_, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		RunID:  20,
		Config: `{"subagent_policy":{"max_depth":1}}`,
		Metadata: `{
			"subagent":{
				"name":"writer",
				"root_name":"lead",
				"parent_name":"lead",
				"step_id":"lead/writer",
				"run_path":["lead","writer"],
				"depth":1
			}
		}`,
	})

	require.ErrorContains(t, err, "subagent depth limit exceeded")
}

func TestADKSubagentToolProviderAppliesTimeoutPolicy(t *testing.T) {
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
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
				Model:       &blockingSubagentChatModel{},
			})
		}),
		WithADKSubagentToolProviderEventSink(events),
		WithADKSubagentToolProviderRunRecorder(recorder),
	)

	set, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
		Config:   `{"subagent_policy":{"timeout_ms":1}}`,
	})
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)

	_, err = researcher.InvokableRun(
		context.Background(),
		`{"request":"find context"}`,
	)

	require.ErrorContains(t, err, "context deadline exceeded")
	require.Equal(
		t,
		[]string{"subagent.run.started", "subagent.run.failed"},
		events.eventTypes(),
	)
	require.Contains(t, events.events[1].Payload, `"error_code":"subagent_timeout"`)
	require.Contains(t, events.events[1].Payload, `"terminal_classification":"timeout"`)
	require.Equal(t, RunStatusFailed, recorder.finishRequests[0].Status)
	require.Equal(t, "subagent_timeout", recorder.finishRequests[0].ErrorCode)
}

func TestADKSubagentToolProviderClassifiesCanceledChildRun(t *testing.T) {
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{
		started: &RunSummary{RunID: 2001, ThreadID: 10, ParentRunID: 20},
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
				Model:       &cancelingSubagentChatModel{},
			})
		}),
		WithADKSubagentToolProviderEventSink(events),
		WithADKSubagentToolProviderRunRecorder(recorder),
	)

	set, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
	})
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)

	_, err = researcher.InvokableRun(context.Background(), `{}`)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(
		t,
		[]string{"subagent.run.started", "subagent.run.canceled"},
		events.eventTypes(),
	)
	require.Contains(t, events.events[1].Payload, `"status":"canceled"`)
	require.Contains(t, events.events[1].Payload, `"error_code":"subagent_canceled"`)
	require.Contains(t, events.events[1].Payload, `"terminal_classification":"canceled"`)
	require.Equal(t, RunStatusCanceled, recorder.finishRequests[0].Status)
	require.Equal(t, "subagent_canceled", recorder.finishRequests[0].ErrorCode)
}

type recordingADKSubagentRunRecorder struct {
	started        *RunSummary
	startErr       error
	finishErr      error
	startRequests  []ADKSubagentRunStartRequest
	finishRequests []ADKSubagentRunFinishRequest
}

func (r *recordingADKSubagentRunRecorder) StartADKSubagentRun(
	_ context.Context,
	req ADKSubagentRunStartRequest,
) (*RunSummary, error) {
	r.startRequests = append(r.startRequests, req)
	if r.startErr != nil {
		return nil, r.startErr
	}
	return r.started, nil
}

func (r *recordingADKSubagentRunRecorder) FinishADKSubagentRun(
	_ context.Context,
	req ADKSubagentRunFinishRequest,
) (*RunSummary, error) {
	r.finishRequests = append(r.finishRequests, req)
	if r.finishErr != nil {
		return nil, r.finishErr
	}
	return req.Child, nil
}

type blockingSubagentChatModel struct{}

func (m *blockingSubagentChatModel) Generate(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *blockingSubagentChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stream is not implemented")
}

type cancelingSubagentChatModel struct{}

func (m *cancelingSubagentChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, context.Canceled
}

func (m *cancelingSubagentChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stream is not implemented")
}
