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

func TestJournalProjectionPublishesOnlyStartedPlanTasks(t *testing.T) {
	pending, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "plan.task.created",
		Payload: `{"plan_task_id":"7","subject":"核验接口","status":"pending"}`,
	})
	require.NoError(t, err)
	require.Nil(t, pending)

	started, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "plan.task.updated",
		Payload: `{"plan_task_id":"7","subject":"核验接口","status":"in_progress","active_form":"正在核验接口"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, started)
	require.Equal(t, "milestone.started", started.EventType)
	require.Equal(t, "running", started.Status)

	completed, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "plan.task.completed",
		Payload: `{"plan_task_id":"7","subject":"核验接口","status":"completed"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, completed)
	require.Equal(t, "milestone.terminal", completed.EventType)
	require.Equal(t, "completed", completed.Status)
	require.Equal(t, journalPayloadString(t, started.Payload, "milestone_id"),
		journalPayloadString(t, completed.Payload, "milestone_id"))
}

func TestJournalProjectionKeepsActionIdentityAndServerOwnedVerbs(t *testing.T) {
	started, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.started",
		Payload: `{"tool_name":"read_file","tool_call_id":"call-1","arguments":{"path":"/private/secret.md","token":"sk-secret"}}`,
	})
	require.NoError(t, err)
	require.NotNil(t, started)
	require.Equal(t, "action.started", started.EventType)
	require.Equal(t, "started", started.Phase)
	require.Equal(t, "read", started.Operation)
	require.Equal(t, "文件", started.Target)
	require.NotContains(t, started.Payload, "/private/secret.md")
	require.NotContains(t, started.Payload, "sk-secret")
	require.Equal(t, "正在读取", journalPayloadString(t, started.Payload, "display_verb_running"))
	require.Equal(t, "已读取", journalPayloadString(t, started.Payload, "display_verb_completed"))
	require.NotContains(t, started.Payload, "content_type")
	var startedEnvelope struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal([]byte(started.Payload), &startedEnvelope))
	require.Equal(t, "generic", startedEnvelope.Type)

	completed, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.completed",
		Payload: `{"tool_name":"read_file","tool_call_id":"call-1","result":"private output"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, completed)
	require.Equal(t, "action.terminal", completed.EventType)
	require.Equal(t, "terminal", completed.Phase)
	require.Equal(t, "completed", completed.Status)
	require.Equal(t, started.ActionID, completed.ActionID)
	require.Equal(t, started.Operation, completed.Operation)
	require.Equal(t, started.Target, completed.Target)
}

func TestJournalProjectionMapsADKToolCallMessageToActionStarted(t *testing.T) {
	started, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "message.completed",
		Payload: `{"role":"assistant","tool_calls":[{"id":"call-1","function":{"name":"read_file","arguments":"{\"path\":\"/private/secret.md\"}"}}]}`,
	})
	require.NoError(t, err)
	require.NotNil(t, started)
	require.Equal(t, "action.started", started.EventType)
	require.Equal(t, "read", started.Operation)
	require.NotContains(t, started.Payload, "/private/secret.md")

	completed, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.completed",
		Payload: `{"role":"tool","tool_name":"read_file","tool_call_id":"call-1","content":"private result"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, completed)
	require.Equal(t, started.ActionID, completed.ActionID)
}

func TestJournalProjectionExpandsParallelADKToolCallsWithoutArguments(t *testing.T) {
	source := RunEvent{
		ThreadID: 1, RunID: 2, EventType: "message.completed",
		Payload: `{"role":"assistant","tool_calls":[
			{"id":"call-1","function":{"name":"read_file","arguments":"{\"path\":\"/private/one.md\"}"}},
			{"id":"call-2","function":{"name":"web_search","arguments":"{\"query\":\"secret\"}"}}
		]}`,
	}

	first, err := ProjectRunEventToJournal(source)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "read", first.Operation)

	supplemental, err := journalSupplementalRunEvents(source)
	require.NoError(t, err)
	require.Len(t, supplemental, 1)
	require.Equal(t, "tool.started", supplemental[0].EventType)
	require.NotContains(t, supplemental[0].Payload, "/private/one.md")
	require.NotContains(t, supplemental[0].Payload, "secret")

	second, err := ProjectRunEventToJournal(supplemental[0])
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, "search", second.Operation)
	require.NotEqual(t, first.ActionID, second.ActionID)
}

func TestJournalProjectionUsesProgressRevisionForAppendOnlyUpdates(t *testing.T) {
	first, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.progress",
		Payload: `{"tool_name":"web_search","tool_call_id":"call-search","progress_revision":"1"}`,
	})
	require.NoError(t, err)
	second, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.progress",
		Payload: `{"tool_name":"web_search","tool_call_id":"call-search","progress_revision":"2"}`,
	})
	require.NoError(t, err)
	require.Equal(t, first.ActionID, second.ActionID)
	require.Equal(t, "progress:1", first.Phase)
	require.Equal(t, "progress:2", second.Phase)
	require.NotEqual(t, first.IdempotencyKey, second.IdempotencyKey)
}

func TestJournalProjectionSeparatesSameNamedSubagentsByPhysicalRun(t *testing.T) {
	first, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "subagent.run.started",
		Payload: `{"child_run_id":101,"status":"running","subagent":{"name":"researcher","step_id":"step-1"}}`,
	})
	require.NoError(t, err)
	second, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "subagent.run.started",
		Payload: `{"child_run_id":102,"status":"running","subagent":{"name":"researcher","step_id":"step-1"}}`,
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.NotEqual(t, first.ActionID, second.ActionID)
	require.Equal(t, "researcher", first.Target)
	require.Equal(t, "researcher", second.Target)
}

func TestJournalProjectionCoversSkillsArtifactsVerificationAndConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		event     RunEvent
		eventType string
		operation string
		target    string
	}{
		{
			name: "skills", event: RunEvent{ThreadID: 1, RunID: 2, EventType: "skills.loaded",
				Payload: `{"skill_count":2,"skill_ids":["10","11"],"skill_names":["research-planner","document-tools"]}`},
			eventType: "action.terminal", operation: "use_skill", target: "research-planner、document-tools",
		},
		{
			name: "artifact", event: RunEvent{ThreadID: 1, RunID: 2, EventType: "artifact.presented",
				Payload: `{"artifacts":[{"artifact_id":99,"title":"评审报告.md","artifact_type":"document"}]}`},
			eventType: "artifact.created", target: "评审报告.md",
		},
		{
			name: "verification", event: RunEvent{ThreadID: 1, RunID: 2, EventType: "verification.completed",
				Payload: `{"verification_id":"verify-1","title":"运行测试","status":"completed","summary":"12 项通过"}`},
			eventType: "verification.terminal", operation: "verify", target: "运行测试",
		},
		{
			name: "confirmation", event: RunEvent{ThreadID: 1, RunID: 2, EventType: "human.interaction.requested",
				Payload: `{"interrupt_id":"confirm-1","kind":"approval","status":"pending","prompt":"允许发布报告吗"}`},
			eventType: "confirmation.requested", target: "允许发布报告吗",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			projection, err := ProjectRunEventToJournal(test.event)
			require.NoError(t, err)
			require.NotNil(t, projection)
			require.Equal(t, test.eventType, projection.EventType)
			require.Equal(t, test.operation, projection.Operation)
			require.Equal(t, test.target, projection.Target)
		})
	}
}

func TestJournalProjectionUsesWholeArtifactCollectionIdentity(t *testing.T) {
	first, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "artifact.presented",
		Payload: `{"artifacts":[{"artifact_id":99,"title":"报告.md"},{"artifact_id":100,"title":"数据.csv"}]}`,
	})
	require.NoError(t, err)
	second, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "artifact.presented",
		Payload: `{"artifacts":[{"artifact_id":99,"title":"报告.md"},{"artifact_id":101,"title":"图片.png"}]}`,
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.NotEqual(t,
		journalPayloadString(t, first.Payload, "collection_id"),
		journalPayloadString(t, second.Payload, "collection_id"),
	)
	require.NotEqual(t, first.IdempotencyKey, second.IdempotencyKey)
}

func TestJournalProjectionRejectsSensitivePublicLabels(t *testing.T) {
	for _, value := range []string{
		"/Users/alice/private/report.md",
		"正在读取/Users/alice/private/report.md",
		"path=/workspace/private.txt",
		"文件:/workspace/private.txt",
		`C:\\Users\\alice\\private\\report.md`,
		"client_secret=oauth-secret",
		"AWS_SECRET_ACCESS_KEY=aws-secret",
		"SECRET=generic-secret",
		"GITHUB_TOKEN=github-secret",
		`{"TOKEN":"quoted-secret"}`,
		`"SECRET"="quoted-secret"`,
	} {
		require.Empty(t, publicLabel(value, maxPublicLabelRunes), value)
	}
	require.Equal(t, "docs/report.md", publicLabel("docs/report.md", maxPublicLabelRunes))
}

func TestJournalProjectionKeepsAsyncArtifactScanEventsOutOfExecutionJournal(t *testing.T) {
	for _, eventType := range []string{"artifact.scan.completed", "artifact.scan.reviewed"} {
		projection, err := ProjectRunEventToJournal(RunEvent{
			ThreadID: 1, RunID: 2, EventType: eventType,
			Payload: `{"artifact_id":99,"scan_status":"infected","scanner":"clamav","decision":"release"}`,
		})
		require.NoError(t, err)
		require.Nil(t, projection)
	}
}

func TestJournalProjectionIgnoresNonTerminalVerificationEvents(t *testing.T) {
	for _, eventType := range []string{"verification.started", "verification.progress"} {
		projection, err := ProjectRunEventToJournal(RunEvent{
			ThreadID: 1, RunID: 2, EventType: eventType,
			Payload: `{"verification_id":"verify-1","title":"运行测试","status":"running"}`,
		})
		require.NoError(t, err)
		require.Nil(t, projection)
	}
}

func TestJournalProjectionMapsInterruptedInteractionAndResolution(t *testing.T) {
	requested, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "run.interrupted",
		Payload: `{
			"interrupts":[{"id":"interrupt-1","info":{
				"schema":"coze.human_interaction.v1",
				"interaction_id":"confirm-1",
				"kind":"confirmation",
				"question":"允许发布报告吗",
				"choices":[{"id":"approve","label":"允许"},{"id":"reject","label":"拒绝"}]
			}}],
			"human_interaction":{
				"schema":"coze.human_interaction.v1",
				"interaction_id":"confirm-1",
				"kind":"confirmation",
				"question":"允许发布报告吗"
			}
		}`,
	})
	require.NoError(t, err)
	require.NotNil(t, requested)
	require.Equal(t, "confirmation.requested", requested.EventType)
	require.Equal(t, "interrupt-1", journalPayloadString(t, requested.Payload, "confirmation_id"))
	require.Equal(t, "允许发布报告吗", requested.Target)

	resolved, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "human.interaction.resolved",
		Payload: `{"interrupt_id":"interrupt-1","interaction_id":"confirm-1","kind":"confirmation","decision":"approved"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Equal(t, "confirmation.resolved", resolved.EventType)
	require.Equal(t,
		journalPayloadString(t, requested.Payload, "confirmation_id"),
		journalPayloadString(t, resolved.Payload, "confirmation_id"),
	)
}

func TestJournalProjectionLeavesDirectAnswersWithoutDisplaySteps(t *testing.T) {
	for _, event := range []RunEvent{
		{ThreadID: 1, RunID: 2, EventType: "message.completed", Payload: `{"role":"assistant","content":"42"}`},
		{ThreadID: 1, RunID: 2, EventType: "run.started", Payload: `{"status":"running"}`},
	} {
		projection, err := ProjectRunEventToJournal(event)
		require.NoError(t, err)
		require.Nil(t, projection)
	}
}

func TestJournalProjectionUsesStandardErrorCodeForTimeout(t *testing.T) {
	require.Equal(t, "task_timeout", publicIdentifier("task_timeout", maxPublicIdentifierRunes))
	require.JSONEq(t, `{"error_code":"task_timeout"}`,
		projectPublicRunEventPayload("run.failed", `{"error_code":"task_timeout","error_message":"redacted"}`))
	timedOut, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "run.failed",
		Payload: `{"error_code":"task_timeout","error_message":"redacted"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "timed_out", timedOut.Status)

	textOnly, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "run.failed",
		Payload: `{"error_code":"runtime_failed","error_message":"context deadline exceeded"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "failed", textOnly.Status)
	require.NotContains(t, textOnly.Payload, "context deadline exceeded")
}

func TestJournalProjectionDoesNotGuessMissingActionIdentity(t *testing.T) {
	started, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.started",
		Payload: `{"tool_name":"read_file","arguments":{"path":"/private/secret.md"}}`,
	})
	require.NoError(t, err)
	require.Nil(t, started)
}

func TestJournalProjectionUnknownToolUsesServerFallbackVerbs(t *testing.T) {
	projection, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 2, EventType: "tool.completed",
		Payload: `{"tool_name":"custom_action","tool_call_id":"call-custom"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, projection)
	require.Equal(t, "process", projection.Operation)
	require.Equal(t, "正在处理", journalPayloadString(t, projection.Payload, "display_verb_running"))
	require.Equal(t, "已处理", journalPayloadString(t, projection.Payload, "display_verb_completed"))
}

func journalPayloadString(t *testing.T, raw, key string) string {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	value, _ := envelope.Data[key].(string)
	return value
}
