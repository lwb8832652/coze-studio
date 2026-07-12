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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const publicProjectionSensitiveSentinel = "sensitive-runtime-sentinel"

func TestPublicRunRedactsInternalFields(t *testing.T) {
	run := &RunSummary{
		RunID:               10,
		ThreadID:            20,
		ParentRunID:         5,
		SpaceID:             30,
		CreatorID:           40,
		AssistantID:         "researcher",
		RunKind:             RunKindSubagent,
		Status:              RunStatusFailed,
		Command:             `{"resume":"` + publicProjectionSensitiveSentinel + `"}`,
		Input:               `{"messages":["` + publicProjectionSensitiveSentinel + `"]}`,
		Config:              `{"api_key":"` + publicProjectionSensitiveSentinel + `"}`,
		Context:             `{"reasoning":"` + publicProjectionSensitiveSentinel + `"}`,
		Metadata:            `{"source":"subagent_retry","source_run_id":5,"requested_at":123,"subagent":{"name":"researcher","prompt":"` + publicProjectionSensitiveSentinel + `"},"credential":"` + publicProjectionSensitiveSentinel + `"}`,
		StreamMode:          `events`,
		MultitaskStrategy:   `reject`,
		OnDisconnect:        `cancel`,
		Durability:          `async`,
		IdempotencyKey:      publicProjectionSensitiveSentinel,
		WorkerID:            publicProjectionSensitiveSentinel,
		LeaseOwner:          publicProjectionSensitiveSentinel,
		LeaseToken:          publicProjectionSensitiveSentinel,
		LeaseExpiresAt:      999,
		HeartbeatAt:         998,
		CancelRequestedAt:   997,
		ExecutionGeneration: 96,
		ErrorCode:           "model_provider_error",
		ErrorMessage:        publicProjectionSensitiveSentinel,
		StartedAt:           100,
		EndedAt:             200,
		CreatedAt:           90,
		UpdatedAt:           210,
	}

	got := ProjectPublicRun(run)
	require.NotNil(t, got)
	require.Equal(t, run.RunID, got.RunID)
	require.Equal(t, run.ThreadID, got.ThreadID)
	require.Equal(t, run.ParentRunID, got.ParentRunID)
	require.Equal(t, run.AssistantID, got.AssistantID)
	require.Equal(t, run.RunKind, got.RunKind)
	require.Equal(t, run.Status, got.Status)
	require.JSONEq(t, `{"source":"subagent_retry","source_run_id":5,"requested_at":123,"subagent":{"name":"researcher"}}`, got.Metadata)
	require.NotNil(t, got.Error)
	require.Equal(t, "model_provider_error", got.Error.Code)
	require.Equal(t, "Model request failed", got.Error.Message)

	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicMessageMetadataRedactsSensitiveFields(t *testing.T) {
	got := ProjectPublicMessageMetadata(`{
		"source":"human_interaction",
		"source_run_id":12,
		"interrupt_id":"interrupt-1",
		"answer":"` + publicProjectionSensitiveSentinel + `",
		"provider_body":{"reasoning":"` + publicProjectionSensitiveSentinel + `"}
	}`)

	require.Equal(t, "human_interaction", got.Source)
	require.Equal(t, int64(12), got.SourceRunID)
	require.Equal(t, "interrupt-1", got.InterruptID)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicMessageRedactsInternalRolesAndMetadata(t *testing.T) {
	visible := ProjectPublicMessage(&MessageSummary{
		MessageID: 1,
		ThreadID:  2,
		RunID:     3,
		Role:      MessageRoleAssistant,
		Content:   "visible response",
		Metadata:  `{"source":"runtime","provider_body":"` + publicProjectionSensitiveSentinel + `"}`,
		CreatedAt: 4,
	})
	require.NotNil(t, visible)
	require.Equal(t, "visible response", visible.Content)
	requirePublicProjectionDoesNotContain(t, visible, publicProjectionSensitiveSentinel)

	hidden := ProjectPublicMessage(&MessageSummary{
		MessageID: 5,
		ThreadID:  2,
		RunID:     3,
		Role:      MessageRoleSystem,
		Content:   publicProjectionSensitiveSentinel,
	})
	require.Nil(t, hidden)
}

func TestPublicRunEventRedactsKnownPayload(t *testing.T) {
	event := &RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "tool.completed",
		Payload: `{
			"tool_name":"web_search",
			"tool_call_id":"call-1",
			"status":"completed",
			"arguments":{"query":"` + publicProjectionSensitiveSentinel + `"},
			"result":"` + publicProjectionSensitiveSentinel + `",
			"credential":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 4,
	}

	got := ProjectPublicRunEvent(event)
	require.NotNil(t, got)
	require.Equal(t, event.EventID, got.EventID)
	require.Equal(t, event.EventType, got.EventType)
	require.JSONEq(t, `{
		"redacted":true,
		"tool_name":"web_search",
		"tool_call_id":"call-1",
		"status":"completed",
		"arguments_present":true,
		"result_present":true
	}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicRunEventKeepsApprovedVisibleMessageContent(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "message.completed",
		Payload: `{
			"role":"assistant",
			"content":"这是用户可见回答",
			"reasoning_content":"` + publicProjectionSensitiveSentinel + `",
			"provider_body":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 4,
	})

	require.NotNil(t, got)
	require.JSONEq(t, `{"redacted":true,"role":"assistant","content":"这是用户可见回答"}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicRunEventKeepsApprovedStreamingContent(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "llm.token",
		Payload: `{
			"node":"agent",
			"chunk":{"role":"assistant","content":"你","reasoning":"` + publicProjectionSensitiveSentinel + `"},
			"provider_body":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 4,
	})

	require.NotNil(t, got)
	require.JSONEq(t, `{"node":"agent","chunk":{"role":"assistant","content":"你"}}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicRunEventKeepsBoundedProviderCapabilityDowngrade(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "model.capability_downgraded",
		Payload: `{
			"schema":"coze.provider_capability_downgrade.v1",
			"capabilities":["reasoning","thinking"],
			"requested_thinking_enabled":true,
			"effective_thinking_enabled":false,
			"requested_reasoning_effort":"high",
			"effective_reasoning_effort":"none",
			"provider_body":"` + publicProjectionSensitiveSentinel + `",
			"prompt":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 4,
	})

	require.NotNil(t, got)
	require.JSONEq(t, `{
		"schema":"coze.provider_capability_downgrade.v1",
		"capabilities":["reasoning","thinking"],
		"requested_thinking_enabled":true,
		"effective_thinking_enabled":false,
		"requested_reasoning_effort":"high",
		"effective_reasoning_effort":"none"
	}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicRunEventKeepsApprovedNodeUpdate(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "node.update",
		Payload: `{
			"node":"agent",
			"delta":{"status":"planning","messages":[{"role":"assistant","content":"收到","reasoning":"` + publicProjectionSensitiveSentinel + `"}]},
			"provider_body":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 4,
	})

	require.NotNil(t, got)
	require.JSONEq(t, `{"node":"agent","delta":{"status":"planning","messages":[{"role":"assistant","content":"收到"}]}}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicRunEventRedactsUnknownPayload(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "provider.experimental",
		Payload:   `{"value":"` + publicProjectionSensitiveSentinel + `"}`,
		CreatedAt: 4,
	})

	require.NotNil(t, got)
	require.Equal(t, `{}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicRuntimeErrorRedactsProviderDetails(t *testing.T) {
	got := ProjectPublicRuntimeError(
		"model_provider_error",
		"provider request failed: "+publicProjectionSensitiveSentinel,
	)

	require.NotNil(t, got)
	require.Equal(t, "model_provider_error", got.Code)
	require.Equal(t, "Model request failed", got.Message)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicCheckpointRedactsRuntimeBytes(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkCheckpointEnvelopeVersion,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  adkCheckpointRuntimeVersion,
		RuntimeKey:      "runtime-key",
		MessageType:     adkCheckpointMessageType,
		Checkpoint:      []byte(publicProjectionSensitiveSentinel),
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {ID: "interrupt-1"},
		},
		RunRevision: 1,
		CreatedAt:   10,
	}
	rawEnvelope, err := envelope.Marshal()
	require.NoError(t, err)

	got := ProjectPublicCheckpoint(&CheckpointSummary{
		CheckpointID:       1,
		ThreadID:           2,
		RunID:              3,
		ParentCheckpointID: 4,
		CheckpointNS:       "eino.adk",
		RuntimeType:        string(RuntimeModeEinoADK),
		RuntimeKey:         publicProjectionSensitiveSentinel,
		EnvelopeVersion:    1,
		ChannelValues:      string(rawEnvelope),
		ChannelVersions:    publicProjectionSensitiveSentinel,
		PendingSends:       publicProjectionSensitiveSentinel,
		Metadata:           `{"provider":"` + publicProjectionSensitiveSentinel + `"}`,
		CreatedAt:          20,
	})

	require.NotNil(t, got)
	require.Equal(t, []string{"interrupt-1"}, got.InterruptIDs)
	require.Equal(t, string(RuntimeModeEinoADK), got.Runtime)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicCheckpointKeepsBoundedPendingSendTargets(t *testing.T) {
	got := ProjectPublicCheckpoint(&CheckpointSummary{
		CheckpointID:  1,
		ThreadID:      2,
		RunID:         3,
		PendingSends:  `[{"node":"planner","arguments":"` + publicProjectionSensitiveSentinel + `"},{"target":"tools"}]`,
		ChannelValues: `{"artifacts":{"report_id":"artifact-1","object_uri":"` + publicProjectionSensitiveSentinel + `"},"memory":{"content":"` + publicProjectionSensitiveSentinel + `"},"tool_results":{"value":"` + publicProjectionSensitiveSentinel + `"}}`,
		Metadata:      `{"runtime":"go","provider_body":"` + publicProjectionSensitiveSentinel + `"}`,
	})

	require.NotNil(t, got)
	require.Equal(t, []string{"planner", "tools"}, got.InterruptIDs)
	require.Equal(t, map[string]any{"report_id": "artifact-1"}, got.Values["artifacts"])
	require.Equal(t, "go", got.Metadata["runtime"])
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicTokenUsageRedactsProviderPayload(t *testing.T) {
	got := ProjectPublicTokenUsage(&TokenUsageSummary{
		UsageID:      1,
		ThreadID:     2,
		RunID:        3,
		SpaceID:      4,
		Source:       TokenUsageSourceLeadAgent,
		StepID:       "step-1",
		StepIndex:    2,
		StepName:     "lead-agent",
		ModelName:    "safe-model",
		Provider:     "safe-provider",
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
		RawUsage:     publicProjectionSensitiveSentinel,
		Metadata:     publicProjectionSensitiveSentinel,
		CreatedAt:    5,
	})

	require.NotNil(t, got)
	require.Equal(t, int64(30), got.TotalTokens)
	require.Equal(t, "safe-model", got.ModelName)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicArtifactRedactsObjectMetadata(t *testing.T) {
	got := ProjectPublicArtifact(&ArtifactSummary{
		ArtifactID:   1,
		SpaceID:      2,
		ThreadID:     3,
		RunID:        4,
		FileID:       5,
		Title:        "report.md",
		ArtifactType: "markdown",
		VirtualPath:  "/mnt/user-data/outputs/report.md",
		ContentType:  "text/markdown",
		SizeBytes:    100,
		PreviewMode:  ArtifactPreviewModeText,
		Metadata:     `{"scan_status":"clean","object_uri":"` + publicProjectionSensitiveSentinel + `"}`,
		CreatedAt:    6,
		UpdatedAt:    7,
	})

	require.NotNil(t, got)
	require.Equal(t, "/mnt/user-data/outputs/report.md", got.VirtualPath)
	require.JSONEq(t, `{"scan_status":"clean"}`, got.Metadata)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicArtifactKeepsLegitimateSecurityTitle(t *testing.T) {
	got := ProjectPublicArtifact(&ArtifactSummary{
		ArtifactID: 1,
		ThreadID:   2,
		RunID:      3,
		Title:      "Password Rotation Guide.md",
		Metadata:   `{}`,
	})

	require.NotNil(t, got)
	require.Equal(t, "Password Rotation Guide.md", got.Title)
}

func TestPublicRunJournalMessageRedactsInternalPayloads(t *testing.T) {
	got := ProjectPublicRunJournalMessage(&RunJournalMessage{
		ID:       "message-1",
		ThreadID: 2,
		RunID:    3,
		Type:     RunJournalMessageTypeAI,
		Role:     MessageRoleAssistant,
		Content:  "visible assistant answer",
		ToolCalls: []RunJournalToolCall{
			{
				ID:   "call-1",
				Name: "web_search",
				Type: "function",
				Args: map[string]any{"query": publicProjectionSensitiveSentinel},
			},
		},
		AdditionalKwargs: map[string]any{
			"reasoning_content": publicProjectionSensitiveSentinel,
		},
		Usage: map[string]any{
			"input_tokens":  10,
			"output_tokens": 20,
			"raw_usage":     publicProjectionSensitiveSentinel,
		},
		CreatedAt:     4,
		SourceEventID: 5,
	})

	require.NotNil(t, got)
	require.Equal(t, "visible assistant answer", got.Content)
	require.Len(t, got.ToolCalls, 1)
	require.Empty(t, got.ToolCalls[0].Arguments)
	require.Empty(t, got.AdditionalKwargs)
	require.Equal(t, int64(10), got.Usage["input_tokens"])
	require.Equal(t, int64(20), got.Usage["output_tokens"])
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)

	tool := ProjectPublicRunJournalMessage(&RunJournalMessage{
		ID:         "message-2",
		ThreadID:   2,
		RunID:      3,
		Type:       RunJournalMessageTypeTool,
		Role:       MessageRoleTool,
		Content:    publicProjectionSensitiveSentinel,
		Name:       "web_search",
		ToolCallID: "call-1",
	})
	require.NotNil(t, tool)
	require.Empty(t, tool.Content)
	requirePublicProjectionDoesNotContain(t, tool, publicProjectionSensitiveSentinel)
}

func requirePublicProjectionDoesNotContain(t *testing.T, value any, forbidden string) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NotContains(t, string(raw), forbidden)
}
