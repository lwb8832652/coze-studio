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
	"reflect"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestADKGuardrailRuntimeToolCatalogAllowsInvocationWithMetadataOnlyRequest(
	t *testing.T,
) {
	invoker := &recordingADKRuntimeToolInvoker{result: "tool-result"}
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Allowed:  true,
			Decision: GuardrailDecision{Action: GuardrailActionAllow},
		},
	}
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
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}

	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	invokable := requireGuardrailADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"search_docs",
	)
	output, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"secret customer path"}`,
	)

	require.NoError(t, err)
	require.Equal(t, "tool-result", output)
	require.Equal(t, 1, invoker.calls)
	require.Equal(t, `{"query":"secret customer path"}`, invoker.call.Arguments)
	require.Len(t, enforcer.requests, 1)
	require.Equal(t, GuardrailRequest{
		SpaceID:    30,
		ThreadID:   10,
		RunID:      20,
		UserID:     40,
		TargetType: GuardrailTargetToolCall,
		TargetID:   "search_docs",
		Operation:  "invoke",
		Source:     "adk_runtime_tool",
		FailMode:   GuardrailFailClosed,
	}, enforcer.requests[0])
	require.Empty(t, enforcer.requests[0].Metadata)
}

func TestADKGuardrailRuntimeToolCatalogBlocksDeniedDecisions(
	t *testing.T,
) {
	invoker := &recordingADKRuntimeToolInvoker{result: "tool-result"}
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Decision: GuardrailDecision{
				Action:     GuardrailActionDeny,
				ReasonCode: "unsafe",
			},
		},
		err: &GuardrailDeniedError{ReasonCode: "unsafe"},
	}
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
	invokable := requireGuardrailADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"search_docs",
	)

	output, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"secret customer path"}`,
	)

	require.Error(t, err)
	var denied *GuardrailDeniedError
	require.ErrorAs(t, err, &denied)
	require.Empty(t, output)
	require.Zero(t, invoker.calls)
	require.Len(t, enforcer.requests, 1)
	require.NotContains(t, err.Error(), "secret customer path")
}

func TestADKGuardrailRuntimeToolCatalogInterruptsConfirmDecisions(
	t *testing.T,
) {
	invoker := &recordingADKRuntimeToolInvoker{result: "tool-result"}
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Decision: GuardrailDecision{
				Action:     GuardrailActionConfirm,
				Provider:   "scanner",
				ReasonCode: "needs_review",
				Message:    "contains https://internal.example.local/raw",
				RuleIDs:    []string{"tool.review", "unsafe raw/rule"},
				Metadata: map[string]string{
					"raw_provider_body": "secret provider body",
				},
			},
			RequiresConfirmation: true,
		},
		err: &GuardrailConfirmationRequiredError{ReasonCode: "needs_review"},
	}
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
	invokable := requireGuardrailADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"search_docs",
	)

	output, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"secret customer path","url":"https://leak.example/file","object":"s3://bucket/key","password":"p"}`,
	)

	require.Error(t, err)
	require.Empty(t, output)
	require.Zero(t, invoker.calls)
	require.Len(t, enforcer.requests, 1)
	require.Equal(t, "search_docs", enforcer.requests[0].TargetID)
	require.Empty(t, enforcer.requests[0].Metadata)

	var confirmation *GuardrailConfirmationRequiredError
	require.False(t, errors.As(err, &confirmation))
	prompt := requireGuardrailInterruptPrompt(t, err)
	require.Equal(t, HumanInteractionKindConfirmation, prompt.Kind)
	require.Equal(t, HumanInteractionRiskHigh, prompt.RiskLevel)
	require.Equal(t, "search_docs", prompt.ToolName)
	require.Equal(t, "invoke", prompt.Action)
	require.Contains(t, prompt.PolicyRef, "scanner")
	require.Contains(t, prompt.PolicyRef, "needs_review")
	require.Contains(t, prompt.Description, "tool.review")
	require.Contains(t, prompt.Description, "unsafe_raw_rule")
	require.NotEmpty(t, prompt.InteractionID)
	require.True(t, prompt.Required)
	require.True(t, prompt.AllowFreeText)
	require.Equal(t, string(HumanInteractionDecisionRejected), prompt.DefaultDecision)

	rawPrompt, marshalErr := json.Marshal(prompt)
	require.NoError(t, marshalErr)
	promptJSON := string(rawPrompt)
	for _, forbidden := range []string{
		"secret customer path",
		"https://leak.example/file",
		"s3://bucket/key",
		"password",
		"secret provider body",
		"https://internal.example.local/raw",
	} {
		require.NotContains(t, promptJSON, forbidden)
		require.NotContains(t, err.Error(), forbidden)
	}
}

func TestDefaultADKToolProviderCanWireGuardrailRuntimeToolWrapper(t *testing.T) {
	executor := &recordingADKMCPRuntimeToolExecutor{result: "mcp-result"}
	enforcer := &recordingADKGuardrailEnforcer{
		result: GuardrailEnforcementResult{
			Allowed:  true,
			Decision: GuardrailDecision{Action: GuardrailActionAllow},
		},
	}
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(
		nil,
		WithDefaultADKToolProviderMCPRegistry(&recordingADKMCPToolRegistry{
			entries: []*toolapi.MCPToolRegistryEntry{
				{
					Name:        "mcp_100_search_docs",
					Source:      "mcp",
					ServerID:    100,
					ToolName:    "search-docs",
					Description: "Search internal documentation.",
					Enabled:     true,
				},
			},
		}),
		WithDefaultADKToolProviderMCPExecutor(executor),
		WithDefaultADKToolProviderGuardrailEnforcer(enforcer),
	)
	setProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)
	set, err := setProvider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:     20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
			Config:    `{"mcp_tools":{"enabled":true}}`,
		},
	)
	require.NoError(t, err)
	invokable := requireGuardrailADKInvokableTool(
		t,
		context.Background(),
		set.DynamicTools,
		"mcp_100_search_docs",
	)

	result, err := invokable.InvokableRun(
		context.Background(),
		`{"query":"docs"}`,
	)

	require.NoError(t, err)
	require.Equal(t, "mcp-result", result)
	require.Len(t, enforcer.requests, 1)
	require.Equal(t, "mcp_100_search_docs", enforcer.requests[0].TargetID)
	require.Equal(t, `{"query":"docs"}`, executor.call.Arguments)
}

type recordingADKGuardrailEnforcer struct {
	requests []GuardrailRequest
	result   GuardrailEnforcementResult
	err      error
}

func (e *recordingADKGuardrailEnforcer) Evaluate(
	ctx context.Context,
	request GuardrailRequest,
) (GuardrailEnforcementResult, error) {
	e.requests = append(e.requests, request)
	return e.result, e.err
}

type recordingADKRuntimeToolInvoker struct {
	call   ADKRuntimeToolCall
	calls  int
	result string
	err    error
}

func (i *recordingADKRuntimeToolInvoker) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	i.calls++
	i.call = call
	if i.err != nil {
		return "", i.err
	}
	return i.result, nil
}

func requireGuardrailADKInvokableTool(
	t *testing.T,
	ctx context.Context,
	tools []tool.BaseTool,
	name string,
) tool.InvokableTool {
	t.Helper()
	for _, item := range tools {
		info, err := item.Info(ctx)
		require.NoError(t, err)
		if info.Name != name {
			continue
		}
		invokable, ok := item.(tool.InvokableTool)
		require.True(t, ok)
		return invokable
	}
	require.Failf(t, "tool not found", "tool %s not found", name)
	return nil
}

func requireGuardrailInterruptPrompt(
	t *testing.T,
	err error,
) HumanInteractionPrompt {
	t.Helper()
	require.Error(t, err)

	value := reflect.ValueOf(err)
	require.Equal(t, reflect.Pointer, value.Kind())
	value = value.Elem()
	require.Equal(t, "InterruptSignal", value.Type().Name())

	info := value.FieldByName("InterruptInfo").FieldByName("Info").Interface()
	prompt, ok := info.(HumanInteractionPrompt)
	require.True(t, ok)

	stateValue := value.FieldByName("InterruptState").
		FieldByName("State").
		Interface()
	state, ok := stateValue.(humanInteractionToolState)
	require.True(t, ok)
	require.Equal(t, prompt, state.Prompt)

	return prompt
}
