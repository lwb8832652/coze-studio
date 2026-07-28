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

package coze

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

const canonicalProjectionSensitiveSentinel = "canonical-projection-private-value"

var canonicalRunStatusCases = []struct {
	internal       appagentthread.RunStatus
	sdk            string
	terminalReason *string
}{
	{appagentthread.RunStatusPending, "pending", nil},
	{appagentthread.RunStatusQueued, "pending", nil},
	{appagentthread.RunStatusRunning, "running", nil},
	{appagentthread.RunStatusInterrupted, "interrupted", nil},
	{appagentthread.RunStatusSucceeded, "success", nil},
	{appagentthread.RunStatusFailed, "error", nil},
	{appagentthread.RunStatusCanceled, "interrupted", canonicalProjectionStringPtr("canceled")},
}

func TestCanonicalRunStatusProjection(t *testing.T) {
	for _, testCase := range canonicalRunStatusCases {
		testCase := testCase
		t.Run(string(testCase.internal), func(t *testing.T) {
			projected, err := projectCanonicalRun(&appagentthread.RunSummary{
				RunID:             3001,
				ThreadID:          2001,
				AssistantID:       "agent",
				RunKind:           appagentthread.RunKindTask,
				Status:            testCase.internal,
				MultitaskStrategy: "reject",
				CreatedAt:         1785052800000,
				UpdatedAt:         1785052801000,
			})

			require.NoError(t, err)
			require.Equal(t, testCase.sdk, projected.Status)
			require.Equal(t, testCase.terminalReason, projected.Coze.TerminalReason)
		})
	}

	projected, err := projectCanonicalRun(&appagentthread.RunSummary{
		RunID:    3001,
		ThreadID: 2001,
		Status:   appagentthread.RunStatus("future_internal_status"),
	})
	require.Nil(t, projected)
	require.ErrorContains(t, err, "unsupported canonical run status")
}

func TestCanonicalRunProjectionRedactsInternalFields(t *testing.T) {
	projected, err := projectCanonicalRun(&appagentthread.RunSummary{
		RunID:             3001,
		ThreadID:          2001,
		ParentRunID:       2999,
		SpaceID:           1001,
		CreatorID:         42,
		AssistantID:       "agent",
		RunKind:           appagentthread.RunKindTask,
		Status:            appagentthread.RunStatusFailed,
		Input:             `{"messages":[{"content":"` + canonicalProjectionSensitiveSentinel + `"}]}`,
		Command:           `{"resume":"` + canonicalProjectionSensitiveSentinel + `"}`,
		Config:            `{"api_key":"` + canonicalProjectionSensitiveSentinel + `"}`,
		Context:           `{"provider_body":"` + canonicalProjectionSensitiveSentinel + `"}`,
		Metadata:          `{"source":"workbench_home","source_run_id":9999,"appended_message_id":"9998","_message":{"message_id":"4001"},"checkpoint_resume":{"source_run_id":2999},"human_interaction":{"schema":"coze.human_interaction_resolved.v1","source_run_id":2999},"parent_run_id":2998,"requested_at":1785052799000,"status":"failed","business_key":"release-plan","business_id":9007199254740993,"user_id":"42","error_chain":"` + canonicalProjectionSensitiveSentinel + `","tool_arguments":"` + canonicalProjectionSensitiveSentinel + `","tool_result":"` + canonicalProjectionSensitiveSentinel + `","provider_payload":"` + canonicalProjectionSensitiveSentinel + `","raw_provider":"` + canonicalProjectionSensitiveSentinel + `","credential":"` + canonicalProjectionSensitiveSentinel + `"}`,
		StreamMode:        `["messages-tuple","updates"]`,
		MultitaskStrategy: "reject",
		OnDisconnect:      "continue",
		Durability:        "async",
		IdempotencyKey:    canonicalProjectionSensitiveSentinel,
		WorkerID:          canonicalProjectionSensitiveSentinel,
		LeaseOwner:        canonicalProjectionSensitiveSentinel,
		LeaseToken:        canonicalProjectionSensitiveSentinel,
		ErrorCode:         "model_provider_error",
		ErrorMessage:      "provider stack: " + canonicalProjectionSensitiveSentinel,
		CreatedAt:         1785052800000,
		UpdatedAt:         1785052801000,
	})

	require.NoError(t, err)
	require.Equal(t, "3001", projected.RunID)
	require.Equal(t, "2001", projected.ThreadID)
	require.Equal(t, "2026-07-26T08:00:00Z", projected.CreatedAt)
	require.Equal(t, "2026-07-26T08:00:01Z", projected.UpdatedAt)
	require.Equal(t, map[string]any{
		"source":       "workbench_home",
		"business_key": "release-plan",
		"business_id":  "9007199254740993",
	}, projected.Metadata)
	require.Equal(t, canonicalProjectionStringPtr("4001"), projected.Coze.MessageID)
	require.Equal(t, canonicalProjectionStringPtr("2999"), projected.Coze.SourceRunID)

	encoded := canonicalProjectionJSON(t, projected)
	for _, field := range []string{
		`"input"`, `"command"`, `"config"`, `"context"`, `"worker_id"`,
		`"idempotency_key"`, `"lease_owner"`, `"lease_token"`, `"error_chain"`, `"stack_trace"`,
	} {
		require.NotContains(t, encoded, field)
	}
	require.NotContains(t, encoded, canonicalProjectionSensitiveSentinel)
}

func TestCanonicalRunProjectionMapsInternalAssistantSelectorsToPublicAlias(t *testing.T) {
	for _, internal := range []string{"default", "singleagent:42", ""} {
		projected, err := projectCanonicalRun(&appagentthread.RunSummary{
			RunID: 3001, ThreadID: 2001, AssistantID: internal,
			Status: appagentthread.RunStatusPending,
		})

		require.NoError(t, err)
		require.Equal(t, canonicalPublicAssistantID, projected.AssistantID)
	}
}

func TestCanonicalCoreCozeExtensionsProjectPublicRunAndThreadFields(t *testing.T) {
	parentRunID := int64(2000)
	projectedRun, err := projectCanonicalRun(&appagentthread.RunSummary{
		RunID: 3001, ThreadID: 2001, ParentRunID: parentRunID, RunKind: appagentthread.RunKindSubagent,
		Status: appagentthread.RunStatusSucceeded, StartedAt: 1710000000123, EndedAt: 1710000001123,
	})
	require.NoError(t, err)
	require.Equal(t, "2000", *projectedRun.Coze.ParentRunID)
	require.Equal(t, "subagent", projectedRun.Coze.RunKind)
	require.NotNil(t, projectedRun.Coze.StartedAt)
	require.NotNil(t, projectedRun.Coze.EndedAt)

	projectedThread, err := projectCanonicalThreadSnapshot(&appagentthread.ThreadSummary{
		ThreadID: 2001, Status: appagentthread.ThreadStatusCompleted, Source: appagentthread.ThreadSourceAPI,
		Progress: 80, LastUserMessage: " user message ", LastAgentMessage: " agent message ",
	}, canonicalThreadProjectionSnapshot{LatestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusSucceeded}})
	require.NoError(t, err)
	require.Equal(t, "api", projectedThread.Coze.Source)
	require.Equal(t, int32(80), projectedThread.Coze.Progress)
	require.Equal(t, "user message", projectedThread.Coze.LastUserMessage)
	require.Equal(t, "agent message", projectedThread.Coze.LastAgentMessage)
}

func TestCanonicalThreadStatusProjection(t *testing.T) {
	safeInterrupts := map[string]any{
		"interrupt-1": []map[string]any{{
			"value":     map[string]any{"interrupt_id": "interrupt-1", "type": "clarification"},
			"when":      "during",
			"resumable": true,
		}},
	}
	testCases := []struct {
		name          string
		latestRun     *appagentthread.RunSummary
		interrupts    map[string]any
		sdkStatus     string
		productStatus string
	}{
		{name: "no run", sdkStatus: "idle", productStatus: "idle"},
		{name: "pending", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusPending}, sdkStatus: "busy", productStatus: "running"},
		{name: "queued", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusQueued}, sdkStatus: "busy", productStatus: "running"},
		{name: "running", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusRunning}, sdkStatus: "busy", productStatus: "running"},
		{name: "private interruption", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusInterrupted}, sdkStatus: "idle", productStatus: "idle"},
		{name: "human interruption", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusInterrupted}, interrupts: safeInterrupts, sdkStatus: "interrupted", productStatus: "idle"},
		{name: "succeeded", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusSucceeded}, sdkStatus: "idle", productStatus: "completed"},
		{name: "failed", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusFailed}, sdkStatus: "error", productStatus: "failed"},
		{name: "canceled", latestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusCanceled}, sdkStatus: "idle", productStatus: "canceled"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			projected, err := projectCanonicalThreadSnapshot(
				&appagentthread.ThreadSummary{
					ThreadID:  2001,
					Title:     "产品发布方案",
					Status:    appagentthread.ThreadStatusCompleted,
					CreatedAt: 1785052800000,
					UpdatedAt: 1785052801000,
				},
				canonicalThreadProjectionSnapshot{
					LatestRun:  testCase.latestRun,
					Interrupts: testCase.interrupts,
				},
			)

			require.NoError(t, err)
			require.Equal(t, testCase.sdkStatus, projected.Status)
			require.Equal(t, testCase.productStatus, projected.Coze.ProductStatus)
			if testCase.sdkStatus == "interrupted" {
				require.Equal(t, safeInterrupts, projected.Interrupts)
			} else {
				require.Empty(t, projected.Interrupts)
			}
		})
	}
}

func TestCanonicalThreadProjectionLoadsLatestRunAndSafeInterrupt(t *testing.T) {
	mockey.PatchConvey("canonical thread projection loads the latest top-level run", t, func() {
		var runCalls int
		var eventCalls int
		defer mockey.Mock((*appagentthread.ApplicationService).ListRuns).To(
			func(
				_ *appagentthread.ApplicationService,
				_ context.Context,
				req *appagentthread.ListRunsRequest,
			) (*appagentthread.ListRunsResponse, error) {
				runCalls++
				require.Equal(t, int64(2001), req.ThreadID)
				require.Nil(t, req.ParentRunID)
				require.False(t, req.IncludeChildRuns)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(1), req.PageSize)
				return &appagentthread.ListRunsResponse{
					Runs: []*appagentthread.RunSummary{{
						RunID:    3001,
						ThreadID: 2001,
						Status:   appagentthread.RunStatusInterrupted,
					}},
					Total: 1,
				}, nil
			},
		).Build().UnPatch()
		defer mockey.Mock((*appagentthread.ApplicationService).ListRunEvents).To(
			func(
				_ *appagentthread.ApplicationService,
				_ context.Context,
				req *appagentthread.ListRunEventsRequest,
			) (*appagentthread.ListRunEventsResponse, error) {
				eventCalls++
				require.Equal(t, int64(2001), req.ThreadID)
				require.Equal(t, int64(3001), req.RunID)
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, canonicalThreadEventPageSize, req.PageSize)
				return &appagentthread.ListRunEventsResponse{
					Events: []*appagentthread.RunEventSummary{{
						EventID:   5001,
						ThreadID:  2001,
						RunID:     3001,
						EventType: "run.interrupted",
						Payload: `{
							"interrupts":[{
								"id":"interrupt-1",
								"address":"private-runtime-address",
								"info":{
									"schema":"coze.human_interaction.v1",
									"interaction_id":"interaction-1",
									"kind":"clarification",
									"title":"选择方案",
									"question":"是否继续？",
									"provider_body":"` + canonicalProjectionSensitiveSentinel + `"
								}
							}],
							"checkpoint_bytes":"` + canonicalProjectionSensitiveSentinel + `"
						}`,
						CreatedAt: 1785052801000,
					}},
					Total: 1,
				}, nil
			},
		).Build().UnPatch()

		projected, err := projectCanonicalThread(context.Background(), &appagentthread.ThreadSummary{
			ThreadID:  2001,
			Title:     "产品发布方案",
			Status:    appagentthread.ThreadStatusIdle,
			CreatedAt: 1785052800000,
			UpdatedAt: 1785052801000,
		})

		require.NoError(t, err)
		require.Equal(t, 1, runCalls)
		require.Equal(t, 1, eventCalls)
		require.Equal(t, "interrupted", projected.Status)
		require.Equal(t, "idle", projected.Coze.ProductStatus)
		require.Contains(t, projected.Interrupts, "interrupt-1")

		encoded := canonicalProjectionJSON(t, projected)
		require.Contains(t, encoded, "是否继续？")
		require.NotContains(t, encoded, "private-runtime-address")
		require.NotContains(t, encoded, "provider_body")
		require.NotContains(t, encoded, "checkpoint_bytes")
		require.NotContains(t, encoded, canonicalProjectionSensitiveSentinel)
	})
}

func TestCanonicalThreadProjectionAvoidsRunReadForProjectedNonIdleStatus(t *testing.T) {
	mockey.PatchConvey("canonical thread projection reuses the repository lifecycle projection", t, func() {
		var runCalls int
		defer mockey.Mock((*appagentthread.ApplicationService).ListRuns).To(
			func(
				_ *appagentthread.ApplicationService,
				_ context.Context,
				_ *appagentthread.ListRunsRequest,
			) (*appagentthread.ListRunsResponse, error) {
				runCalls++
				return nil, nil
			},
		).Build().UnPatch()

		projected, err := projectCanonicalThread(context.Background(), &appagentthread.ThreadSummary{
			ThreadID: 2001,
			Status:   appagentthread.ThreadStatusRunning,
		})

		require.NoError(t, err)
		require.Zero(t, runCalls)
		require.Equal(t, "busy", projected.Status)
		require.Equal(t, "running", projected.Coze.ProductStatus)
	})
}

func TestCanonicalThreadProjectionRedactsMetadata(t *testing.T) {
	projected, err := projectCanonicalThreadSnapshot(
		&appagentthread.ThreadSummary{
			ThreadID: 2001,
			Title:    "产品发布方案",
			Metadata: `{
				"title":"untrusted title",
				"business_key":"release-plan",
				"priority":true,
				"status":"running",
				"source_run_id":"3000",
				"appended_message_id":"4000",
				"owner_id":"42",
				"creator_id":"42",
				"space_id":"1001",
				"runtime":"eino_adk",
				"credential":"` + canonicalProjectionSensitiveSentinel + `",
				"secret":"` + canonicalProjectionSensitiveSentinel + `",
				"token":"` + canonicalProjectionSensitiveSentinel + `",
				"legacy_task_id":"retired",
				"nested":{"provider_body":"` + canonicalProjectionSensitiveSentinel + `"}
			}`,
			CreatedAt: 1785052800000,
			UpdatedAt: 1785052801000,
		},
		canonicalThreadProjectionSnapshot{
			LatestRun: &appagentthread.RunSummary{Status: appagentthread.RunStatusRunning},
		},
	)

	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"title":        "产品发布方案",
		"business_key": "release-plan",
		"priority":     true,
	}, projected.Metadata)
	require.NotContains(t, projected.Metadata, "status")
	require.NotContains(t, projected.Metadata, "source_run_id")
	require.NotContains(t, projected.Metadata, "appended_message_id")
	require.Equal(t, map[string]any{"messages": []map[string]any{}}, projected.Values)
	require.Empty(t, projected.Interrupts)
	require.Nil(t, projected.Coze.InitialSubmission)

	encoded := canonicalProjectionJSON(t, projected)
	for _, hidden := range []string{
		"owner_id", "creator_id", "space_id", "runtime", "credential", "secret",
		"token", "legacy_task_id", "provider_body", canonicalProjectionSensitiveSentinel,
	} {
		require.NotContains(t, encoded, hidden)
	}
}

func TestCanonicalThreadProjectionDropsSensitiveTitle(t *testing.T) {
	projected, err := projectCanonicalThreadSnapshot(
		&appagentthread.ThreadSummary{
			ThreadID: 2001,
			Title:    "api_key=sk-private-value",
		},
		canonicalThreadProjectionSnapshot{},
	)

	require.NoError(t, err)
	require.NotContains(t, projected.Metadata, "title")
	require.NotContains(t, canonicalProjectionJSON(t, projected), "sk-private-value")
}

func TestCanonicalMessageProjectionRedactsInternalData(t *testing.T) {
	projected, err := projectCanonicalMessage(&appagentthread.MessageSummary{
		MessageID: 4001,
		ThreadID:  2001,
		RunID:     3001,
		Role:      appagentthread.MessageRoleAssistant,
		Content:   "公开回答",
		Metadata: `{
			"source":"agent",
			"source_run_id":3000,
			"provider_body":"` + canonicalProjectionSensitiveSentinel + `",
			"tool_arguments":{"api_key":"` + canonicalProjectionSensitiveSentinel + `"},
			"legacy_task_id":"retired"
		}`,
		CreatedAt: 1785052801000,
	})

	require.NoError(t, err)
	require.Equal(t, "4001", projected.MessageID)
	require.Equal(t, "2001", projected.ThreadID)
	require.Equal(t, "3001", projected.RunID)
	require.Equal(t, "assistant", projected.Role)
	require.Equal(t, "公开回答", projected.Content)
	require.Equal(t, map[string]any{"source": "agent", "source_run_id": "3000"}, projected.Metadata)
	require.Equal(t, "2026-07-26T08:00:01Z", projected.CreatedAt)

	encoded := canonicalProjectionJSON(t, projected)
	require.NotContains(t, encoded, canonicalProjectionSensitiveSentinel)
	require.NotContains(t, encoded, "provider_body")
	require.NotContains(t, encoded, "tool_arguments")
	require.NotContains(t, encoded, "legacy_task_id")
}

func TestCanonicalRunEventProjectionRedactsInternalData(t *testing.T) {
	projected, err := projectCanonicalRunEvent(&appagentthread.RunEventSummary{
		EventID:   5001,
		ThreadID:  2001,
		RunID:     3001,
		EventType: "tool.completed",
		Payload: `{
			"tool_name":"web_search",
			"tool_call_id":"call-1",
			"status":"completed",
			"arguments":{"query":"` + canonicalProjectionSensitiveSentinel + `"},
			"result":{"provider_body":"` + canonicalProjectionSensitiveSentinel + `"},
			"provider_body":"` + canonicalProjectionSensitiveSentinel + `",
			"checkpoint_bytes":"` + canonicalProjectionSensitiveSentinel + `"
		}`,
		CreatedAt: 1785052801000,
	})

	require.NoError(t, err)
	require.Equal(t, "5001", projected.EventID)
	require.Equal(t, "tool.completed", projected.EventType)
	require.Equal(t, map[string]any{
		"redacted":          true,
		"tool_name":         "web_search",
		"tool_call_id":      "call-1",
		"status":            "completed",
		"arguments_present": true,
		"result_present":    true,
	}, projected.Payload)

	encoded := canonicalProjectionJSON(t, projected)
	require.NotContains(t, encoded, canonicalProjectionSensitiveSentinel)
	require.NotContains(t, encoded, "provider_body")
	require.NotContains(t, encoded, "checkpoint_bytes")
	require.NotContains(t, encoded, `"arguments":`)
	require.NotContains(t, encoded, `"result":`)
}

func TestCanonicalThreadStateProjectionRedactsInternalData(t *testing.T) {
	projected, err := projectCanonicalThreadState(canonicalStateSource{
		ThreadID:           2001,
		CheckpointID:       9001,
		ParentCheckpointID: 9000,
		CheckpointNS:       "canonical.public",
		ParentCheckpointNS: "harness.terminal",
		Values: map[string]any{
			"runtime": "eino_adk",
			"messages": []any{
				map[string]any{
					"id":               "message-1",
					"role":             "assistant",
					"content":          "公开回答",
					"tool_arguments":   canonicalProjectionSensitiveSentinel,
					"provider_body":    canonicalProjectionSensitiveSentinel,
					"checkpoint_bytes": canonicalProjectionSensitiveSentinel,
				},
			},
			"custom": map[string]any{
				"visible":       "ok",
				"result":        "public outcome",
				"config":        "public preference",
				"hidden_config": canonicalProjectionSensitiveSentinel,
				"tool_result":   canonicalProjectionSensitiveSentinel,
			},
			"checkpoint_bytes": canonicalProjectionSensitiveSentinel,
			"artifacts": []any{
				map[string]any{
					"artifact_id": int64(9007199254740993),
					"file_id":     int64(5001),
					"run_id":      int64(3001),
					"size_bytes":  int64(42),
				},
			},
		},
		Next: []string{"agent"},
		Metadata: map[string]any{
			"source":         "checkpoint",
			"legacy_task_id": "retired",
			"provider_body":  canonicalProjectionSensitiveSentinel,
		},
		CreatedAt: 1785052801000,
		Tasks: []map[string]any{{
			"id":     "task-1",
			"result": canonicalProjectionSensitiveSentinel,
			"error":  canonicalProjectionSensitiveSentinel,
		}},
		Interrupts: []map[string]any{{
			"id":   "interrupt-1",
			"when": "during",
			"value": map[string]any{
				"type":          "clarification",
				"question":      "是否继续？",
				"provider_body": canonicalProjectionSensitiveSentinel,
			},
			"checkpoint_bytes": canonicalProjectionSensitiveSentinel,
		}},
	})

	require.NoError(t, err)
	require.Equal(t, canonicalCheckpoint{
		ThreadID:      "2001",
		CheckpointNS:  "canonical.public",
		CheckpointID:  "9001",
		CheckpointMap: map[string]any{},
	}, projected.Checkpoint)
	require.Equal(t, &canonicalCheckpoint{
		ThreadID:      "2001",
		CheckpointNS:  "harness.terminal",
		CheckpointID:  "9000",
		CheckpointMap: map[string]any{},
	}, projected.ParentCheckpoint)
	require.Equal(t, []string{"agent"}, projected.Next)
	require.Empty(t, projected.Tasks)
	require.Equal(t, map[string]any{"source": "checkpoint"}, projected.Metadata)
	require.Equal(t, "2026-07-26T08:00:01Z", projected.CreatedAt)
	require.Equal(t, "ok", projected.Values["custom"].(map[string]any)["visible"])
	require.Equal(t, "public outcome", projected.Values["custom"].(map[string]any)["result"])
	require.Equal(t, "public preference", projected.Values["custom"].(map[string]any)["config"])
	artifacts := projected.Values["artifacts"].([]any)
	require.Equal(t, "9007199254740993", artifacts[0].(map[string]any)["artifact_id"])
	require.Equal(t, "5001", artifacts[0].(map[string]any)["file_id"])
	require.Equal(t, "3001", artifacts[0].(map[string]any)["run_id"])
	require.Equal(t, int64(42), artifacts[0].(map[string]any)["size_bytes"])
	require.Len(t, projected.Interrupts, 1)

	encoded := canonicalProjectionJSON(t, projected)
	for _, hidden := range []string{
		"checkpoint_bytes", "tool_arguments", "tool_result", "provider_body", "hidden_config",
		"legacy_task_id", canonicalProjectionSensitiveSentinel,
	} {
		require.NotContains(t, encoded, hidden)
	}
}

func TestCanonicalThreadUpdateStateProjectionUsesOneCheckpoint(t *testing.T) {
	checkpoint := canonicalCheckpoint{
		ThreadID:      "2001",
		CheckpointNS:  "canonical.public",
		CheckpointID:  "9002",
		CheckpointMap: map[string]any{},
	}

	projected := projectCanonicalThreadUpdateStateResult(checkpoint)

	require.Equal(t, checkpoint, projected.Checkpoint)
	require.Equal(t, checkpoint, projected.Configurable)
	require.Equal(t, projected.Checkpoint, projected.Configurable)
}

func TestCanonicalIDProjectionRejectsImpreciseFloat(t *testing.T) {
	projected, ok := canonicalIDScalar(float64(9007199254740993))
	require.False(t, ok)
	require.Nil(t, projected)

	projected, ok = canonicalIDScalar(float64(9007199254740991))
	require.True(t, ok)
	require.Equal(t, "9007199254740991", projected)

	projected, ok = canonicalIDScalar(int64(9007199254740993))
	require.True(t, ok)
	require.Equal(t, "9007199254740993", projected)
}

func canonicalProjectionStringPtr(value string) *string {
	return &value
}

func canonicalProjectionJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
