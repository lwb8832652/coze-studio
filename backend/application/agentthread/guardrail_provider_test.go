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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChainGuardrailProviderDecisionPrecedence(t *testing.T) {
	request := GuardrailRequest{
		SpaceID:    10,
		ThreadID:   20,
		RunID:      30,
		UserID:     40,
		TargetType: GuardrailTargetToolCall,
		TargetID:   "runtime_tool:search_docs",
		Operation:  "invoke",
		FailMode:   GuardrailFailClosed,
	}
	tests := []struct {
		name      string
		decisions []GuardrailDecision
		want      GuardrailAction
	}{
		{
			name: "all allow stays allow",
			decisions: []GuardrailDecision{
				{Action: GuardrailActionAllow, Provider: "role"},
				{Action: GuardrailActionAllow, Provider: "scanner"},
			},
			want: GuardrailActionAllow,
		},
		{
			name: "warn overrides allow",
			decisions: []GuardrailDecision{
				{Action: GuardrailActionAllow, Provider: "role"},
				{Action: GuardrailActionWarn, Provider: "scanner"},
			},
			want: GuardrailActionWarn,
		},
		{
			name: "confirm overrides warn",
			decisions: []GuardrailDecision{
				{Action: GuardrailActionWarn, Provider: "scanner"},
				{Action: GuardrailActionConfirm, Provider: "risk"},
			},
			want: GuardrailActionConfirm,
		},
		{
			name: "deny overrides confirm",
			decisions: []GuardrailDecision{
				{Action: GuardrailActionConfirm, Provider: "risk"},
				{Action: GuardrailActionDeny, Provider: "scanner"},
			},
			want: GuardrailActionDeny,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			providers := make([]GuardrailProvider, 0, len(testCase.decisions))
			for _, decision := range testCase.decisions {
				decision := decision
				providers = append(providers, GuardrailProviderFunc(
					func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
						return decision, nil
					},
				))
			}
			provider := NewChainGuardrailProvider(providers...)

			decision, err := provider.EvaluateGuardrail(context.Background(), request)

			require.NoError(t, err)
			require.Equal(t, testCase.want, decision.Action)
			require.Equal(t, string(testCase.want), decision.Action.String())
		})
	}
}

func TestChainGuardrailProviderFailModes(t *testing.T) {
	providerErr := errors.New("provider leaked raw prompt: sk-secret /mnt/raw")
	request := GuardrailRequest{
		SpaceID:    10,
		ThreadID:   20,
		RunID:      30,
		UserID:     40,
		TargetType: GuardrailTargetMCPTool,
		TargetID:   "mcp_100_search_docs",
		Operation:  "invoke",
	}
	provider := NewChainGuardrailProvider(GuardrailProviderFunc(
		func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
			return GuardrailDecision{}, providerErr
		},
	))

	openDecision, openErr := provider.EvaluateGuardrail(
		context.Background(),
		withGuardrailFailMode(request, GuardrailFailOpen),
	)
	closedDecision, closedErr := provider.EvaluateGuardrail(
		context.Background(),
		withGuardrailFailMode(request, GuardrailFailClosed),
	)

	require.NoError(t, openErr)
	require.Equal(t, GuardrailActionAllow, openDecision.Action)
	require.Equal(t, "guardrail_provider_error_fail_open", openDecision.ReasonCode)
	require.NotContains(t, openDecision.Message, "sk-secret")
	require.NotContains(t, openDecision.Message, "/mnt/raw")
	require.NoError(t, closedErr)
	require.Equal(t, GuardrailActionDeny, closedDecision.Action)
	require.Equal(t, "guardrail_provider_error_fail_closed", closedDecision.ReasonCode)
	require.NotContains(t, closedDecision.Message, "sk-secret")
	require.NotContains(t, closedDecision.Message, "/mnt/raw")
}

func TestChainGuardrailProviderSanitizesDecisionMetadata(t *testing.T) {
	provider := NewChainGuardrailProvider(GuardrailProviderFunc(
		func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
			return GuardrailDecision{
				Action:     GuardrailActionConfirm,
				Provider:   " scanner provider /mnt/raw ",
				ReasonCode: " high risk reason with spaces ",
				Message:    "Review secret sk-secret from /mnt/raw/object",
				RuleIDs: []string{
					" rule:high_risk ",
					"",
					strings.Repeat("x", 120),
				},
				Metadata: map[string]string{
					"target_type":       "tool_call",
					"safe_count":        "2",
					"raw_prompt":        "please leak this prompt",
					"object_uri":        "s3://bucket/raw",
					"checkpoint_bytes":  "abcdef",
					"credential_secret": "sk-secret",
					"huge":              strings.Repeat("x", 300),
				},
			}, nil
		},
	))

	decision, err := provider.EvaluateGuardrail(
		context.Background(),
		GuardrailRequest{
			SpaceID:    10,
			ThreadID:   20,
			RunID:      30,
			UserID:     40,
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:write_file",
			Operation:  "invoke",
			FailMode:   GuardrailFailClosed,
		},
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailActionConfirm, decision.Action)
	require.Equal(t, "scanner_provider_mnt_raw", decision.Provider)
	require.Equal(t, "high_risk_reason_with_spaces", decision.ReasonCode)
	require.Equal(t, "guardrail decision requires review", decision.Message)
	require.Equal(t, []string{
		"rule:high_risk",
		strings.Repeat("x", 64),
	}, decision.RuleIDs)
	require.Equal(t, map[string]string{
		"huge":        strings.Repeat("x", 128),
		"safe_count":  "2",
		"target_type": "tool_call",
	}, decision.Metadata)
}

func withGuardrailFailMode(
	request GuardrailRequest,
	failMode GuardrailFailMode,
) GuardrailRequest {
	request.FailMode = failMode
	return request
}
