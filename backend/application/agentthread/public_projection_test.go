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

func TestPublicRunEventDoesNotDeriveJournalTargetsFromToolArguments(t *testing.T) {
	event := &RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "message.completed",
		Payload: `{
			"role":"assistant",
			"tool_calls":[
				{"id":"call-1","function":{"name":"read_file","arguments":"{\"path\":\"/private/workspace/requirements.md\"}"}},
				{"id":"call-2","function":{"name":"edit_file","arguments":"{\"file_path\":\"frontend/src/journal-event-model.ts\"}"}},
				{"id":"call-3","function":{"name":"web_search","arguments":"{\"query\":\"private roadmap\"}"}},
				{"id":"call-4","function":{"name":"read_file","arguments":"{\"path\":\"https://internal.example/report.md\"}"}}
			]
		}`,
		CreatedAt: 4,
	}

	got := ProjectPublicRunEvent(event)
	require.NotNil(t, got)
	require.JSONEq(t, `{
		"redacted":true,
		"role":"assistant",
		"tool_calls":[
			{"id":"call-1","name":"read_file","arguments_present":true},
			{"id":"call-2","name":"edit_file","arguments_present":true},
			{"id":"call-3","name":"web_search","arguments_present":true},
			{"id":"call-4","name":"read_file","arguments_present":true}
		]
	}`, got.Payload)
	require.NotContains(t, got.Payload, "/private/workspace")
	require.NotContains(t, got.Payload, "frontend/src")
	require.NotContains(t, got.Payload, "private roadmap")
	require.NotContains(t, got.Payload, "internal.example")
	require.NotContains(t, got.Payload, "requirements.md")
	require.NotContains(t, got.Payload, "journal-event-model.ts")
}

func TestPublicRunEventRejectsJournalAttemptInterruptedPersistenceHelper(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID: 1, ThreadID: 2, RunID: 3,
		EventType: "journal.attempt.interrupted",
		Payload:   `{"schema":"coze.journal_attempt_interrupted.v1","status":"interrupted","resume_run_id":4}`,
		CreatedAt: 5,
	})

	require.Nil(t, got)
}

func TestPublicRunInterruptedKeepsOnlyResumableHumanInteractionMetadata(t *testing.T) {
	prompt := `{
		"schema":"coze.human_interaction.v1",
		"interaction_id":"hi_1",
		"kind":"clarification",
		"title":"选择执行方案",
		"question":"请选择方案 A 或方案 B",
		"description":"继续执行前需要你的选择",
		"summary":"请选择后继续",
		"rejection_guidance":"` + publicProjectionSensitiveSentinel + `",
		"risk_level":"medium",
		"tool_name":"ask_clarification",
		"tool_call_id":"call-private",
		"policy_ref":"/private/policy/path",
		"action":"/private/action/path",
		"default_decision":"approved",
		"required":true,
		"allow_free_text":true,
		"choices":[{"id":"a","label":"方案 A","value":"A","credential":"` + publicProjectionSensitiveSentinel + `"}],
		"consequences":["/private/consequence"],
		"affected_resources":["/private/resource"],
		"created_at":123,
		"context":"` + publicProjectionSensitiveSentinel + `",
		"provider_body":"` + publicProjectionSensitiveSentinel + `"
	}`
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   7,
		ThreadID:  2,
		RunID:     3,
		EventType: "run.interrupted",
		Payload: `{
			"interrupts":[{
				"id":"interrupt-1",
				"address":"` + publicProjectionSensitiveSentinel + `",
				"info":` + prompt + `,
				"is_root_cause":true
			}],
			"human_interaction":` + prompt + `,
			"human_interactions":[` + prompt + `],
			"checkpoint_bytes":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 8,
	})

	require.NotNil(t, got)
	require.JSONEq(t, `{
		"interrupts":{"items":[{
			"id":"interrupt-1",
			"info":{
				"schema":"coze.human_interaction.v1",
				"interaction_id":"hi_1",
				"kind":"clarification",
				"title":"选择执行方案",
				"question":"请选择方案 A 或方案 B",
				"summary":"请选择后继续",
				"risk_level":"medium",
				"required":true,
				"allow_free_text":true,
				"choices":[{"id":"a","label":"方案 A"}]
			}
		}]},
		"human_interaction":{
			"schema":"coze.human_interaction.v1",
			"interaction_id":"hi_1",
			"kind":"clarification",
			"title":"选择执行方案",
			"question":"请选择方案 A 或方案 B",
			"summary":"请选择后继续",
			"risk_level":"medium",
			"required":true,
			"allow_free_text":true,
			"choices":[{"id":"a","label":"方案 A"}]
		},
		"human_interactions":[{
			"schema":"coze.human_interaction.v1",
			"interaction_id":"hi_1",
			"kind":"clarification",
			"title":"选择执行方案",
			"question":"请选择方案 A 或方案 B",
			"summary":"请选择后继续",
			"risk_level":"medium",
			"required":true,
			"allow_free_text":true,
			"choices":[{"id":"a","label":"方案 A"}]
		}]
	}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
	for _, hidden := range []string{
		"继续执行前需要你的选择", "call-private", "/private/policy/path",
		"/private/action/path", "/private/consequence", "/private/resource",
		`"value":"A"`, `"created_at":123`,
	} {
		require.NotContains(t, got.Payload, hidden)
	}
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

func TestPublicRunEventKeepsOnlyApprovedExecutionIntro(t *testing.T) {
	got := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "plan.task.created",
		Payload: `{
			"plan_task_id":"1",
			"subject":"核验执行链路",
			"status":"pending",
			"execution_intro":"收到。我会先核验执行链路，再完成实现与验证。",
			"metadata":{"private_note":"` + publicProjectionSensitiveSentinel + `"},
			"provider_body":"` + publicProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 4,
	})

	require.NotNil(t, got)
	require.JSONEq(t, `{
		"plan_task_id":"1",
		"subject":"核验执行链路",
		"status":"pending",
		"execution_intro":"收到。我会先核验执行链路，再完成实现与验证。"
	}`, got.Payload)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)

	sensitive := ProjectPublicRunEvent(&RunEventSummary{
		EventID:   2,
		ThreadID:  2,
		RunID:     3,
		EventType: "plan.task.created",
		Payload: `{
			"plan_task_id":"1",
			"subject":"核验执行链路",
			"status":"pending",
			"execution_intro":"正在读取 /Users/alice/private/report.md"
		}`,
		CreatedAt: 5,
	})
	require.NotNil(t, sensitive)
	require.NotContains(t, sensitive.Payload, "execution_intro")
	require.NotContains(t, sensitive.Payload, "/Users/alice/private/report.md")
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
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 3, ThreadID: 2, SpaceID: 1, CreatorID: 4,
	}, nil)
	require.NoError(t, err)
	state := tracker.Snapshot()
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkCheckpointEnvelopeVersion,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  adkCheckpointRuntimeVersion,
		RuntimeKey:      "runtime-key",
		MessageType:     adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		Checkpoint:      []byte(publicProjectionSensitiveSentinel),
		ParityState:     &state,
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
		RuntimeKey:         "runtime-key",
		EnvelopeVersion:    int32(adkCheckpointEnvelopeVersion),
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

func TestPublicCheckpointProjectsOnlyCanonicalPublicCustomState(t *testing.T) {
	got := ProjectPublicCheckpoint(&CheckpointSummary{
		CheckpointID: 1,
		ThreadID:     2,
		RunID:        3,
		RuntimeType:  "canonical_public_state",
		ChannelValues: `{"custom":{"theme":"dark","count":1},` +
			`"checkpoint_bytes":"` + publicProjectionSensitiveSentinel + `"}`,
		ChannelVersions: publicProjectionSensitiveSentinel,
		PendingSends:    publicProjectionSensitiveSentinel,
	})

	require.NotNil(t, got)
	require.Equal(t, map[string]any{
		"custom": map[string]any{"theme": "dark", "count": float64(1)},
	}, got.Values)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicCheckpointProjectsBoundedADKParityState(t *testing.T) {
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 3, ThreadID: 2, SpaceID: 1, CreatorID: 4,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, tracker.ReplaceMessages([]ADKParityMessage{
		{
			ID: "message-1", RunID: 3, Role: "user",
			Content:   "<uploaded_files>" + publicProjectionSensitiveSentinel + "</uploaded_files>\n生成旅行计划",
			CreatedAt: 10,
		},
		{ID: "message-2", RunID: 3, Role: "assistant", Content: "计划已完成", CreatedAt: 11},
	}, &ADKParitySummaryBoundary{
		Digest: "summary-digest", OriginalMessageCount: 8, ActiveMessageCount: 2, CreatedAt: 9,
	}))
	require.NoError(t, tracker.SetTitle("青岛旅行计划"))
	require.NoError(t, tracker.ReplaceTodos([]ADKParityTodo{{
		ID: "todo-1", Title: "整理行程", Description: "按天组织", Status: "completed",
		ActiveForm: "正在整理行程", Owner: "lead-agent",
	}}))
	trackerCtx := withADKParityStateTracker(context.Background(), tracker)
	require.True(t, bindADKJournalToolPlanTasks(
		trackerCtx,
		[]string{"private-journal-binding"},
		"todo-1",
	))
	require.NoError(t, tracker.MergeUploads([]ADKParityUpload{{
		FileID: 5, FileName: "需求.txt", VirtualPath: "/mnt/user-data/uploads/需求.txt",
		ContentType: "text/plain", SizeBytes: 12, CreatedAt: 8,
	}}))
	require.NoError(t, tracker.MergeArtifacts([]ADKParityArtifact{{
		ArtifactID: 6, FileID: 7, RunID: 3, Title: "青岛旅行计划.md",
		ArtifactType: "markdown", VirtualPath: "/mnt/user-data/outputs/青岛旅行计划.md",
		ContentType: "text/markdown", SizeBytes: 128, PreviewMode: "markdown",
		ScanStatus: "released", CreatedAt: 12,
	}}))
	require.NoError(t, tracker.MergeViewedImages(map[string]ADKParityViewedImage{
		"/mnt/user-data/outputs/map.png": {
			VirtualPath: "/mnt/user-data/outputs/map.png", ContentType: "image/png", Digest: "image-digest",
		},
	}))
	require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
		CatalogHash: "catalog-digest", Names: []string{"web_search"},
	}))
	require.NoError(t, tracker.ReplaceActiveSkills([]ADKParitySkill{{
		ID: 9, Name: "travel-planner", Version: "v1",
	}}))
	require.NoError(t, tracker.ReplaceInterrupts([]ADKParityInterrupt{{
		ID: "interrupt-1", Address: "agent.ask", IsRootCause: true,
	}}))
	require.NoError(t, tracker.SetCompletion(ADKParityCompletion{
		RunID: 3, Status: "interrupted", Reason: "user_input_required", CompletedAt: 13,
	}))
	state := tracker.Snapshot()
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkCheckpointEnvelopeVersion,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  adkCheckpointRuntimeVersion,
		RuntimeKey:      "thread-2/run-3",
		MessageType:     adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseInterrupt,
		Checkpoint:      []byte(publicProjectionSensitiveSentinel),
		ParityState:     &state,
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {ID: "interrupt-1", Address: "agent.ask", IsRootCause: true},
		},
		RunRevision: state.Revision,
		CreatedAt:   13,
	}
	rawEnvelope, err := envelope.Marshal()
	require.NoError(t, err)

	got := ProjectPublicCheckpoint(&CheckpointSummary{
		CheckpointID: 10, ThreadID: 2, RunID: 3, CheckpointNS: "eino.adk",
		RuntimeType: string(RuntimeModeEinoADK), EnvelopeVersion: int32(adkCheckpointEnvelopeVersion),
		ChannelValues: string(rawEnvelope), Metadata: `{"credential":"` + publicProjectionSensitiveSentinel + `"}`,
	})

	require.NotNil(t, got)
	require.Equal(t, []string{"interrupt-1"}, got.InterruptIDs)
	require.Equal(t, "青岛旅行计划", got.Values["title"])
	messages, ok := got.Values["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, messages, 2)
	require.Equal(t, "生成旅行计划", messages[0]["content"])
	require.Equal(t, []map[string]any{{
		"id": "todo-1", "title": "整理行程", "description": "按天组织", "status": "completed",
		"active_form": "正在整理行程", "owner": "lead-agent",
	}}, got.Values["todos"])
	require.Len(t, got.Values["uploaded_files"], 1)
	require.Len(t, got.Values["artifacts"], 1)
	require.Len(t, got.Values["viewed_images"], 1)
	require.Equal(t, map[string]any{
		"catalog_hash": "catalog-digest", "names": []string{"web_search"},
	}, got.Values["promoted"])
	require.Equal(t, []map[string]any{{
		"id": int64(9), "name": "travel-planner", "version": "v1",
	}}, got.Values["active_skills"])
	require.Equal(t, map[string]any{
		"run_id": int64(3), "status": "interrupted", "reason": "user_input_required", "completed_at": int64(13),
	}, got.Values["completion"])
	require.Equal(t, []string{"interrupt-1"}, got.Values["interrupts"])
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
	requirePublicProjectionDoesNotContain(t, got, "private-journal-binding")
}

func TestPublicCheckpointFailsClosedForMalformedADKEnvelope(t *testing.T) {
	got := ProjectPublicCheckpoint(&CheckpointSummary{
		CheckpointID: 1,
		ThreadID:     2,
		RunID:        3,
		RuntimeType:  string(RuntimeModeEinoADK),
		ChannelValues: `{"envelope_version":99,"checkpoint":"` +
			publicProjectionSensitiveSentinel + `","parity_state":{"title":"unsafe"}}`,
	})

	require.NotNil(t, got)
	require.Equal(t, map[string]any{
		"runtime": string(RuntimeModeEinoADK), "interrupts": []string{},
	}, got.Values)
	requirePublicProjectionDoesNotContain(t, got, publicProjectionSensitiveSentinel)
}

func TestPublicCheckpointFailsClosedForMismatchedADKEnvelopeIdentity(t *testing.T) {
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 3, ThreadID: 2, SpaceID: 1, CreatorID: 4,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, tracker.SetTitle("must not cross projection boundary"))
	state := tracker.Snapshot()
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkCheckpointEnvelopeVersion,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  adkCheckpointRuntimeVersion,
		RuntimeKey:      "runtime-key",
		MessageType:     adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		Checkpoint:      []byte{1},
		ParityState:     &state,
	}
	rawEnvelope, err := envelope.Marshal()
	require.NoError(t, err)

	tests := map[string]CheckpointSummary{
		"indexed version": {
			ThreadID: 2, RunID: 3, RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "runtime-key",
			EnvelopeVersion: 1, ChannelValues: string(rawEnvelope),
		},
		"runtime key": {
			ThreadID: 2, RunID: 3, RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "different-key",
			EnvelopeVersion: int32(adkCheckpointEnvelopeVersion), ChannelValues: string(rawEnvelope),
		},
		"thread": {
			ThreadID: 99, RunID: 3, RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "runtime-key",
			EnvelopeVersion: int32(adkCheckpointEnvelopeVersion), ChannelValues: string(rawEnvelope),
		},
		"run": {
			ThreadID: 2, RunID: 99, RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "runtime-key",
			EnvelopeVersion: int32(adkCheckpointEnvelopeVersion), ChannelValues: string(rawEnvelope),
		},
	}
	for name, checkpoint := range tests {
		t.Run(name, func(t *testing.T) {
			got := ProjectPublicCheckpoint(&checkpoint)
			require.NotNil(t, got)
			require.Equal(t, map[string]any{
				"runtime": string(RuntimeModeEinoADK), "interrupts": []string{},
			}, got.Values)
			require.NotContains(t, got.Values, "title")
		})
	}
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
	require.True(t, got.ReasoningPresent)
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
