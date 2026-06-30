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
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKClarificationToolInterruptsWithPrompt(t *testing.T) {
	clarification, err := NewADKClarificationTool()
	require.NoError(t, err)

	_, err = clarification.InvokableRun(context.Background(), `{
		"question":"请选择时间范围",
		"description":"用于继续生成报告",
		"choices":[{"id":"last_7_days","label":"最近 7 天","value":"last_7_days"}],
		"allow_free_text":true
	}`)

	require.Error(t, err)
	prompt, err := clarification.(*adkHumanInteractionTool).prompt(context.Background(), `{
		"question":"请选择时间范围",
		"description":"用于继续生成报告",
		"choices":[{"id":"last_7_days","label":"最近 7 天","value":"last_7_days"}],
		"allow_free_text":true
	}`)
	require.NoError(t, err)
	require.Equal(t, humanInteractionSchema, prompt.Schema)
	require.Equal(t, HumanInteractionKindClarification, prompt.Kind)
	require.Equal(t, adkClarificationToolName, prompt.ToolName)
	require.Equal(t, "请选择时间范围", prompt.Question)
	require.Len(t, prompt.Choices, 1)
	require.Equal(t, "last_7_days", prompt.Choices[0].ID)
}

func TestADKHumanInteractionResultFromClarificationResponse(t *testing.T) {
	result, err := humanInteractionToolResultFromResponse(HumanInteractionResponse{
		Schema:        humanInteractionResponseSchema,
		InteractionID: "hi_test",
		Kind:          HumanInteractionKindClarification,
		Decision:      HumanInteractionDecisionAnswered,
		Answer:        "最近 7 天",
		ChoiceID:      "last_7_days",
	})

	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &payload))
	require.Equal(t, humanInteractionResultSchema, payload["schema"])
	require.Equal(t, "clarification", payload["kind"])
	require.Equal(t, true, payload["answered"])
	require.Equal(t, "最近 7 天", payload["answer"])
	require.Equal(t, "last_7_days", payload["choice_id"])
}

func TestADKConfirmationToolReturnsRejectedResultWithoutError(t *testing.T) {
	result, err := humanInteractionToolResultFromResponse(HumanInteractionResponse{
		Schema:        humanInteractionResponseSchema,
		InteractionID: "hi_confirm",
		Kind:          HumanInteractionKindConfirmation,
		Decision:      HumanInteractionDecisionRejected,
		Comment:       "不要删除，先给我备份方案。",
	})

	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &payload))
	require.Equal(t, humanInteractionResultSchema, payload["schema"])
	require.Equal(t, "confirmation", payload["kind"])
	require.Equal(t, false, payload["approved"])
	require.Equal(t, "rejected", payload["decision"])
	require.Equal(t, "不要删除，先给我备份方案。", payload["comment"])
	require.Equal(t, "User rejected the action. Choose a safer alternative.", payload["guidance"])
}

func TestValidateHumanInteractionResponseRejectsInvalidDecision(t *testing.T) {
	err := validateHumanInteractionResponse(HumanInteractionResponse{
		Schema:        humanInteractionResponseSchema,
		InteractionID: "hi_bad",
		Kind:          HumanInteractionKindConfirmation,
		Decision:      HumanInteractionDecisionAnswered,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "confirmation decision must be approved or rejected")
}

func TestADKHumanInteractionToolProviderAppendsBuiltins(t *testing.T) {
	baseTool := &namedTestTool{name: "existing_tool"}
	provider := NewADKHumanInteractionToolProvider(ADKToolProviderFunc(
		func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
			return []tool.BaseTool{baseTool}, nil
		},
	))

	tools, err := provider.ResolveTools(context.Background(), &RunSummary{RunID: 1})

	require.NoError(t, err)
	require.Len(t, tools, 3)
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		info, infoErr := item.Info(context.Background())
		require.NoError(t, infoErr)
		names = append(names, info.Name)
	}
	require.ElementsMatch(t, []string{
		"existing_tool",
		adkClarificationToolName,
		adkConfirmationToolName,
	}, names)
}

func TestDefaultADKToolProviderIgnoresTypedNilGuardrailEnforcer(t *testing.T) {
	var enforcer *GuardrailEnforcer
	options := defaultADKToolProviderOptions{}

	WithDefaultADKToolProviderGuardrailEnforcer(enforcer)(&options)

	require.False(t, options.guardrailEnforcer != nil)
}

type namedTestTool struct {
	name string
}

func (t *namedTestTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name, Desc: "test tool"}, nil
}

func (t *namedTestTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "ok", nil
}
