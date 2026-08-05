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

package deerflowparity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCaptureCanonicalizesProductEventAliases(t *testing.T) {
	t.Parallel()

	deerFlow, err := NormalizeCapture(RawCapture{
		Product: ProductDeerFlow,
		CaseID:  "core.pro.direct",
		Mode:    ModePro,
		Events: []RawEvent{
			{ID: "df-1", Type: "run.started"},
			{ID: "df-2", Type: "assistant.completed"},
			{ID: "df-3", Type: "token.usage"},
			{ID: "df-4", Type: "run.completed"},
		},
		Messages: []RawMessage{{Role: "assistant", Content: "reference answer"}},
		Tokens:   []RawTokenUsage{{Input: 12, Output: 3, Total: 15}},
		Terminal: "success",
	})
	require.NoError(t, err)

	newX, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.pro.direct",
		Mode:    ModePro,
		Events: []RawEvent{
			{ID: "101", Type: "run.created"},
			{ID: "102", Type: "message.completed", Payload: map[string]any{"role": "assistant"}},
			{ID: "103", Type: "usage.updated"},
			{ID: "104", Type: "run.succeeded"},
		},
		Messages: []RawMessage{{Role: "assistant", Content: "candidate answer"}},
		Tokens:   []RawTokenUsage{{Input: 20, Output: 5, Total: 25}},
		Terminal: "succeeded",
	})
	require.NoError(t, err)

	require.Equal(t, deerFlow.EventFamilies, newX.EventFamilies)
	require.Equal(t, []string{"run.started", "assistant.completed", "token.usage", "run.completed"}, newX.EventFamilies)
	require.True(t, newX.AssistantMessagePresent)
	require.Equal(t, int64(25), newX.Token.Total)
	require.Equal(t, "success", newX.Terminal)
}

func TestNormalizeCaptureCanonicalizesNewXTokenUsageSnapshot(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.pro.direct",
		Mode:    ModePro,
		Events: []RawEvent{
			{ID: "101", Type: "token_usage.snapshot"},
		},
		Terminal: "success",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"token.usage"}, observation.EventFamilies)
}

func TestNormalizeCaptureCanonicalizesNewXPlanMutationsAsTodoUpdates(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.pro.todo",
		Mode:    ModePro,
		Events: []RawEvent{
			{ID: "101", Type: "plan.task.created"},
			{ID: "102", Type: "plan.task.updated"},
			{ID: "103", Type: "plan.task.completed"},
			{ID: "104", Type: "plan.task.deleted"},
		},
		Terminal: "success",
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"todo.updated", "todo.updated", "todo.updated", "todo.updated",
	}, observation.EventFamilies)
}

func TestNormalizeCaptureUsesEffectiveThinkingCapabilityWithoutReasoningText(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.ultra.direct",
		Mode:    ModeUltra,
		Events: []RawEvent{{
			ID:   "102",
			Type: "model.capability_downgraded",
			Payload: map[string]any{
				"effective_thinking_enabled": true,
			},
		}},
		Terminal: "success",
	})
	require.NoError(t, err)
	require.True(t, observation.Capabilities.Thinking)
}

func TestNormalizeCaptureProjectsTodoInterruptAndChildren(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.clarify.followup",
		Mode:    ModePro,
		Events: []RawEvent{
			{ID: "1", Type: "run.started"},
			{ID: "2", Type: "task.started", Payload: map[string]any{"kind": "subagent"}},
			{ID: "3", Type: "interrupt.created"},
			{ID: "4", Type: "resume.accepted"},
			{ID: "5", Type: "task.completed", Payload: map[string]any{"kind": "subagent"}},
			{ID: "6", Type: "run.succeeded"},
		},
		State: map[string]any{
			"todos": []any{
				map[string]any{"content": "first", "status": "completed"},
				map[string]any{"content": "second", "status": "pending"},
			},
		},
		Terminal: "success",
	})
	require.NoError(t, err)
	require.Equal(t, 2, observation.Todo.Total)
	require.Equal(t, 1, observation.Todo.Completed)
	require.Equal(t, 1, observation.ChildRuns)
	require.Equal(t, 1, observation.CompletedChildRuns)
	require.True(t, observation.InterruptPresent)
	require.True(t, observation.ResumeObserved)
}

func TestNormalizeCaptureRejectsCredentialAndCheckpointPayloads(t *testing.T) {
	t.Parallel()

	for name, payload := range map[string]map[string]any{
		"credential": {"access_token": "secret"},
		"checkpoint": {"checkpoint_bytes": "opaque"},
		"object uri": {"object_uri": "s3://bucket/key"},
	} {
		payload := payload
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := NormalizeCapture(RawCapture{
				Product:  ProductNewX,
				CaseID:   "core.pro.direct",
				Mode:     ModePro,
				Events:   []RawEvent{{ID: "1", Type: "run.started", Payload: payload}},
				Terminal: "success",
			})
			require.Error(t, err)
		})
	}
}

func TestNormalizeCaptureDoesNotSerializeModelOrToolContent(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.pro.direct",
		Mode:    ModePro,
		Events: []RawEvent{{
			ID:   "1",
			Type: "message.completed",
			Payload: map[string]any{
				"reasoning":      "HIDDEN_REASONING_SENTINEL",
				"tool_arguments": "HIDDEN_ARGUMENT_SENTINEL",
				"tool_result":    "HIDDEN_RESULT_SENTINEL",
			},
		}},
		Messages: []RawMessage{{Role: "assistant", Content: "PRIVATE_COMPLETION_SENTINEL"}},
		Terminal: "success",
	})
	require.NoError(t, err)

	encoded, err := json.Marshal(observation)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"HIDDEN_REASONING_SENTINEL",
		"HIDDEN_ARGUMENT_SENTINEL",
		"HIDDEN_RESULT_SENTINEL",
		"PRIVATE_COMPLETION_SENTINEL",
	} {
		require.NotContains(t, string(encoded), forbidden)
	}
}

func TestNormalizeCaptureCollapsesUnknownWireSymbolsToAllowlistedValues(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product: ProductNewX,
		CaseID:  "core.pro.direct",
		Mode:    ModePro,
		Events: []RawEvent{{
			ID: "1", Type: "private.secret.event", Payload: map[string]any{},
		}},
		Terminal: "private-secret-terminal",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"runtime.diagnostic"}, observation.EventFamilies)
	require.Equal(t, "unknown", observation.Terminal)
	encoded, err := json.Marshal(observation)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private.secret.event")
	require.NotContains(t, string(encoded), "private-secret-terminal")
}

func TestNormalizeCaptureCountsReconnectDuplicatesAndCancelFence(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product:     ProductNewX,
		CaseID:      "core.cancel",
		Mode:        ModePro,
		Reconnected: true,
		Events: []RawEvent{
			{ID: "1", Type: "run.started"},
			{ID: "2", Type: "run.cancelled"},
			{ID: "2", Type: "run.cancelled"},
			{ID: "3", Type: "run.succeeded"},
		},
		Terminal: "cancelled",
	})
	require.NoError(t, err)
	require.Equal(t, 1, observation.DuplicateEvents)
	require.True(t, observation.SuccessAfterCancel)
}

func TestNormalizeCaptureUsesAuthoritativeCancelledTerminalOverCallbackError(t *testing.T) {
	t.Parallel()

	observation, err := NormalizeCapture(RawCapture{
		Product:         ProductDeerFlow,
		CaseID:          "core.cancel",
		Mode:            ModePro,
		CancelRequested: true,
		Events: []RawEvent{
			{ID: "1", Type: "run.started"},
			{ID: "2", Type: "run.error"},
			{ID: "3", Type: "run.cancelled"},
		},
		Terminal: "cancelled",
		RunCount: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, observation.TerminalEvents)
	require.False(t, observation.SuccessAfterCancel)
	require.Contains(t, observation.EventFamilies, "run.failed")
}
