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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGuardrailPatternProviderDeniesMatchingTargetWithoutLeakingMetadata(
	t *testing.T,
) {
	provider, err := NewGuardrailPatternProvider(
		GuardrailPatternProviderOptions{
			Rules: []GuardrailPatternRule{
				{
					RuleID:         "dangerous_delete",
					Action:         string(GuardrailActionDeny),
					TargetTypes:    []string{string(GuardrailTargetToolCall)},
					Operations:     []string{"invoke"},
					TargetContains: []string{"delete_all"},
					ReasonCode:     "dangerous_delete",
					Message:        "dangerous delete operation requires blocking",
				},
			},
		},
	)
	require.NoError(t, err)

	decision, err := provider.EvaluateGuardrail(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:delete_all_records",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   GuardrailFailClosed,
			Metadata: map[string]string{
				"raw_prompt":       "secret prompt sk-secret",
				"checkpoint_bytes": strings.Repeat("a", 128),
				"safe_catalog":     "runtime_tools",
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailActionDeny, decision.Action)
	require.Equal(t, "pattern_scanner", decision.Provider)
	require.Equal(t, "dangerous_delete", decision.ReasonCode)
	require.Equal(t, []string{"dangerous_delete"}, decision.RuleIDs)
	require.Equal(t, map[string]string{
		"operation":   "invoke",
		"source":      "adk_runtime_tool",
		"target_type": "tool_call",
	}, decision.Metadata)
	serialized := strings.Join([]string{
		decision.Message,
		strings.Join(decision.RuleIDs, ","),
		decision.Metadata["operation"],
		decision.Metadata["source"],
		decision.Metadata["target_type"],
	}, " ")
	require.NotContains(t, serialized, "sk-secret")
	require.NotContains(t, serialized, "secret prompt")
	require.NotContains(t, serialized, "checkpoint")
}

func TestGuardrailPatternProviderAllowsWhenRuleDoesNotMatch(t *testing.T) {
	provider, err := NewGuardrailPatternProvider(
		GuardrailPatternProviderOptions{
			Rules: []GuardrailPatternRule{
				{
					RuleID:         "network_review",
					Action:         string(GuardrailActionConfirm),
					TargetTypes:    []string{string(GuardrailTargetNetwork)},
					TargetContains: []string{"internal"},
					ReasonCode:     "network_review",
				},
			},
		},
	)
	require.NoError(t, err)

	decision, err := provider.EvaluateGuardrail(
		context.Background(),
		GuardrailRequest{
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:search_docs",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   GuardrailFailClosed,
		},
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailActionAllow, decision.Action)
	require.Equal(t, "pattern_scanner", decision.Provider)
	require.Equal(t, "no_match", decision.ReasonCode)
}

func TestGuardrailEnforcerFromEnvBuildsPatternProviderAndAuditsDecision(
	t *testing.T,
) {
	t.Setenv(agentGuardrailProviderTypeEnv, "pattern")
	t.Setenv(agentGuardrailPatternRulesJSONEnv, `[
		{
			"rule_id": "skill_load_review",
			"action": "confirm",
			"target_types": ["skill"],
			"operations": ["load"],
			"target_contains": ["shell"],
			"reason_code": "skill_load_review"
		}
	]`)
	repo := &recordingGuardrailAuditRepository{}

	enforcer, status := NewGuardrailEnforcerFromEnv(
		repo,
		&mcpWorkdirLeaseSequenceIDGen{next: 9301},
	)

	require.True(t, status.Enabled)
	require.True(t, status.Configured)
	require.Equal(t, "pattern", status.Type)
	require.Empty(t, status.Error)
	require.NotNil(t, enforcer)
	result, err := enforcer.Evaluate(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetSkill,
			TargetID:   "shell-helper",
			Operation:  "load",
			Source:     "adk_skill_backend",
			FailMode:   GuardrailFailClosed,
		},
	)

	require.Error(t, err)
	require.True(t, result.RequiresConfirmation)
	require.Equal(t, GuardrailActionConfirm, result.Decision.Action)
	require.Equal(t, "skill_load_review", result.Decision.ReasonCode)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, int64(9301), repo.createdEvent.ID)
	require.Equal(t, "guardrail.decision.confirm", repo.createdEvent.EventType)
	require.Equal(t, "skill_load_review", repo.createdEvent.ReasonCode)
}

func TestGuardrailEnforcerFromEnvFailsClosedWhenExplicitConfigInvalid(
	t *testing.T,
) {
	t.Setenv(agentGuardrailProviderTypeEnv, "pattern")
	t.Setenv(agentGuardrailPatternRulesJSONEnv, `not-json with sk-secret`)
	repo := &recordingGuardrailAuditRepository{}

	enforcer, status := NewGuardrailEnforcerFromEnv(
		repo,
		&mcpWorkdirLeaseSequenceIDGen{next: 9302},
	)

	require.True(t, status.Enabled)
	require.False(t, status.Configured)
	require.Contains(t, status.Error, "guardrail pattern rules are invalid")
	require.NotContains(t, status.Error, "sk-secret")
	require.NotNil(t, enforcer)
	result, err := enforcer.Evaluate(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:search_docs",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   GuardrailFailClosed,
		},
	)

	require.Error(t, err)
	require.Equal(t, GuardrailActionDeny, result.Decision.Action)
	require.Equal(t, "guardrail_config_invalid", result.Decision.ReasonCode)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, "guardrail_config_invalid", repo.createdEvent.ReasonCode)
}
