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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunnerExecutesOrdinaryCaseAndProducesAlignedReport(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := newPassingFakePlatform(ProductDeerFlow, testCase)
	candidate := newPassingFakePlatform(ProductNewX, testCase)
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   lockedDeerFlowRevision,
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report := runner.RunCase(context.Background(), testCase)
	require.Equal(t, StatusAligned, report.Comparison.Status)
	require.Equal(t, 1, reference.streamRunCalls)
	require.Equal(t, 1, candidate.streamRunCalls)
	require.Equal(t, 1, reference.stateCalls)
	require.Equal(t, 1, candidate.stateCalls)
	require.NotEmpty(t, report.Reference.EventFamilies)
	require.NotEmpty(t, report.Candidate.EventFamilies)
}

func TestNewRunnerRejectsMissingRevisionProvenance(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	_, err := NewRunner(
		newPassingFakePlatform(ProductDeerFlow, testCase),
		newPassingFakePlatform(ProductNewX, testCase),
		RunnerOptions{
			Credentials:      Credentials{Email: "user@example.com", Password: "password"},
			NewXSpaceID:      "7656103552997130240",
			DeerFlowRevision: lockedDeerFlowRevision,
			CaseTimeout:      time.Second,
		},
	)
	require.ErrorContains(t, err, "provenance is not verified")
}

func TestRunnerExecutesCancelResumeAndReconnectActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		caseID string
		assert func(*testing.T, *fakePlatform)
	}{
		{
			caseID: "core.cancel",
			assert: func(t *testing.T, platform *fakePlatform) {
				require.Equal(t, 1, platform.startRunCalls)
				require.Equal(t, 1, platform.cancelCalls)
				require.Equal(t, 1, platform.streamExistingCalls)
			},
		},
		{
			caseID: "core.clarify.followup",
			assert: func(t *testing.T, platform *fakePlatform) {
				require.Equal(t, 1, platform.streamRunCalls)
				require.Equal(t, 1, platform.followUpCalls)
			},
		},
		{
			caseID: "core.stream.reconnect",
			assert: func(t *testing.T, platform *fakePlatform) {
				require.Equal(t, 1, platform.startRunCalls)
				require.Equal(t, 2, platform.streamExistingCalls)
				require.Equal(t, "1", platform.streamCursors[1])
				require.Equal(t, "continue", platform.startInputs[0].OnDisconnect)
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.caseID, func(t *testing.T) {
			t.Parallel()
			testCase := mustAcceptanceCase(t, test.caseID)
			reference := newPassingFakePlatform(ProductDeerFlow, testCase)
			candidate := newPassingFakePlatform(ProductNewX, testCase)
			runner, err := NewRunner(reference, candidate, RunnerOptions{
				Credentials:        Credentials{Email: "user@example.com", Password: "password"},
				NewXSpaceID:        "7656103552997130240",
				DeerFlowRevision:   lockedDeerFlowRevision,
				RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
				CaseTimeout:        time.Second,
			})
			require.NoError(t, err)

			report := runner.RunCase(context.Background(), testCase)
			require.Equal(t, StatusAligned, report.Comparison.Status)
			test.assert(t, reference)
			test.assert(t, candidate)
		})
	}
}

func TestRunnerClassifiesPrerequisiteFailuresAsBlocked(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := newPassingFakePlatform(ProductDeerFlow, testCase)
	candidate := newPassingFakePlatform(ProductNewX, testCase)
	candidate.createErr = NewPrerequisiteError("newx_schema_migration_missing")
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   lockedDeerFlowRevision,
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report := runner.RunCase(context.Background(), testCase)
	require.Equal(t, StatusBlocked, report.Comparison.Status)
	require.Equal(t, "newx_schema_migration_missing", report.Comparison.Blocker)
}

func TestRunnerReloadStateRequiresDurableStateAndCheckpointHistory(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.pro.todo")
	reference := newPassingFakePlatform(ProductDeerFlow, testCase)
	candidate := newPassingFakePlatform(ProductNewX, testCase)
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   lockedDeerFlowRevision,
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report := runner.RunCase(context.Background(), testCase)
	require.Equal(t, StatusAligned, report.Comparison.Status)
	require.Equal(t, 2, reference.stateCalls)
	require.Equal(t, 2, candidate.stateCalls)
	require.True(t, report.Reference.StateReloaded)
	require.Positive(t, report.Reference.HistoryEntries)
	require.True(t, report.Candidate.StateReloaded)
	require.Positive(t, report.Candidate.HistoryEntries)
}

func TestRunnerDoesNotClassifyCaseDeadlineAsUnavailable(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := newPassingFakePlatform(ProductDeerFlow, testCase)
	candidate := newPassingFakePlatform(ProductNewX, testCase)
	candidate.createErr = context.DeadlineExceeded
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   lockedDeerFlowRevision,
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report := runner.RunCase(context.Background(), testCase)
	require.Equal(t, StatusDifferent, report.Comparison.Status)
	require.Empty(t, report.Comparison.Blocker)
}

func TestRunnerReadsEveryMessagePageWithoutRepeatingCursor(t *testing.T) {
	t.Parallel()

	platform := newPassingFakePlatform(ProductNewX, mustAcceptanceCase(t, "core.flash.direct"))
	platform.paginateMessages = true
	messages, usage, err := readAllRunMessages(context.Background(), platform, "newx-thread", "newx-run-1")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Len(t, usage, 2)
	require.Equal(t, []int64{0, 1}, platform.messageAfterSeqs)
}

func TestSuiteReportWritersContainOnlySanitizedObservations(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := newPassingFakePlatform(ProductDeerFlow, testCase)
	candidate := newPassingFakePlatform(ProductNewX, testCase)
	reference.privateMessage = "PRIVATE_REFERENCE_COMPLETION"
	candidate.privateMessage = "PRIVATE_CANDIDATE_COMPLETION"
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   lockedDeerFlowRevision,
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report := SuiteReport{
		Schema:               SuiteReportSchemaV1,
		DeerFlowRevision:     "5851f8250eb150ca23134c79b11ebc5073ac2789",
		RevisionProvenance:   RevisionProvenanceVerifiedSourceAttestedRuntime,
		ReferenceEnvironment: "local_deerflow",
		CandidateEnvironment: "local_newx",
		Cases:                []CaseReport{runner.RunCase(context.Background(), testCase)},
	}
	var jsonOutput bytes.Buffer
	require.NoError(t, WriteJSONReport(&jsonOutput, report))
	var markdownOutput bytes.Buffer
	require.NoError(t, WriteMarkdownReport(&markdownOutput, report))

	combined := jsonOutput.String() + markdownOutput.String()
	require.Contains(t, combined, "core.flash.direct")
	require.Contains(t, combined, "aligned")
	require.Contains(t, markdownOutput.String(), "local_deerflow")
	require.Contains(t, markdownOutput.String(), "local_newx")
	require.Contains(t, combined, RevisionProvenanceVerifiedSourceAttestedRuntime)
	require.Contains(t, markdownOutput.String(), "required_event:assistant.completed")
	require.NotContains(t, combined, "PRIVATE_REFERENCE_COMPLETION")
	require.NotContains(t, combined, "PRIVATE_CANDIDATE_COMPLETION")
}

func TestSuiteReportWritersRejectUnverifiedRevision(t *testing.T) {
	t.Parallel()

	report := SuiteReport{
		Schema:               SuiteReportSchemaV1,
		DeerFlowRevision:     strings.Repeat("a", 40),
		RevisionProvenance:   RevisionProvenanceVerifiedSourceAttestedRuntime,
		ReferenceEnvironment: "local_deerflow",
		CandidateEnvironment: "local_newx",
	}
	require.ErrorContains(t, WriteJSONReport(&bytes.Buffer{}, report), "revision is invalid")
}

func TestSuiteReportUsesConfiguredEnvironmentLabels(t *testing.T) {
	t.Parallel()

	suite, err := LoadCases()
	require.NoError(t, err)
	suite.Cases = suite.Cases[:1]
	runner, err := NewRunner(
		newPassingFakePlatform(ProductDeerFlow, suite.Cases[0]),
		newPassingFakePlatform(ProductNewX, suite.Cases[0]),
		RunnerOptions{
			Credentials:          Credentials{Email: "user@example.com", Password: "password"},
			NewXSpaceID:          "7656103552997130240",
			DeerFlowRevision:     lockedDeerFlowRevision,
			RevisionProvenance:   RevisionProvenanceVerifiedSourceAttestedRuntime,
			ReferenceEnvironment: "local_deerflow",
			CandidateEnvironment: "newx_debug",
			CaseTimeout:          time.Second,
		},
	)
	require.NoError(t, err)

	report, err := runner.RunSuite(context.Background(), suite)
	require.NoError(t, err)
	require.Equal(t, "local_deerflow", report.ReferenceEnvironment)
	require.Equal(t, "newx_debug", report.CandidateEnvironment)
}

func TestRunnerBlocksSuiteWhenReferenceRevisionDoesNotMatch(t *testing.T) {
	t.Parallel()

	suite, err := LoadCases()
	require.NoError(t, err)
	reference := newPassingFakePlatform(ProductDeerFlow, suite.Cases[0])
	candidate := newPassingFakePlatform(ProductNewX, suite.Cases[0])
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   strings.Repeat("a", 40),
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report, err := runner.RunSuite(context.Background(), suite)
	require.NoError(t, err)
	require.Len(t, report.Cases, len(suite.Cases))
	for _, testCase := range report.Cases {
		require.Equal(t, StatusBlocked, testCase.Comparison.Status)
		require.Equal(t, "deerflow_revision_mismatch", testCase.Comparison.Blocker)
	}
	require.Zero(t, reference.loginCalls)
	require.Zero(t, candidate.loginCalls)
}

func TestRunnerBlocksSuiteWhenReferenceConnectionIsRefused(t *testing.T) {
	t.Parallel()

	suite, err := LoadCases()
	require.NoError(t, err)
	reference := newPassingFakePlatform(ProductDeerFlow, suite.Cases[0])
	reference.loginErr = &url.Error{Op: "POST", URL: "http://127.0.0.1:2026", Err: syscall.ECONNREFUSED}
	candidate := newPassingFakePlatform(ProductNewX, suite.Cases[0])
	runner, err := NewRunner(reference, candidate, RunnerOptions{
		Credentials:        Credentials{Email: "user@example.com", Password: "password"},
		NewXSpaceID:        "7656103552997130240",
		DeerFlowRevision:   lockedDeerFlowRevision,
		RevisionProvenance: RevisionProvenanceVerifiedSourceAttestedRuntime,
		CaseTimeout:        time.Second,
	})
	require.NoError(t, err)

	report, err := runner.RunSuite(context.Background(), suite)
	require.NoError(t, err)
	for _, testCase := range report.Cases {
		require.Equal(t, StatusBlocked, testCase.Comparison.Status)
		require.Equal(t, "deerflow_unavailable", testCase.Comparison.Blocker)
	}
	require.Zero(t, candidate.loginCalls)
}

type fakePlatform struct {
	product Product
	caseDef Case

	loginCalls          int
	createCalls         int
	streamRunCalls      int
	startRunCalls       int
	startInputs         []RunInput
	streamExistingCalls int
	cancelCalls         int
	followUpCalls       int
	stateCalls          int
	streamCursors       []string
	createErr           error
	loginErr            error
	privateMessage      string
	paginateMessages    bool
	messageAfterSeqs    []int64
}

func newPassingFakePlatform(product Product, testCase Case) *fakePlatform {
	return &fakePlatform{
		product:        product,
		caseDef:        testCase,
		privateMessage: "private completion",
	}
}

func (f *fakePlatform) Product() Product { return f.product }

func (f *fakePlatform) Login(_ context.Context, _ Credentials) error {
	f.loginCalls++
	return f.loginErr
}

func (f *fakePlatform) CreateThread(_ context.Context, _ ThreadOptions) (string, error) {
	f.createCalls++
	if f.createErr != nil {
		return "", f.createErr
	}
	return string(f.product) + "-thread", nil
}

func (f *fakePlatform) StreamRun(_ context.Context, threadID string, _ RunInput) (StreamResult, error) {
	f.streamRunCalls++
	terminal := f.caseDef.Expect.RequiredTerminal[0]
	if f.caseDef.ID == "core.clarify.followup" {
		terminal = "interrupted"
	}
	return StreamResult{ThreadID: threadID, RunID: string(f.product) + "-run-1", Terminal: terminal}, nil
}

func (f *fakePlatform) StartRun(_ context.Context, threadID string, input RunInput) (RunHandle, error) {
	f.startRunCalls++
	f.startInputs = append(f.startInputs, input)
	return RunHandle{ThreadID: threadID, RunID: string(f.product) + "-run-1", Status: "running"}, nil
}

func (f *fakePlatform) GetRun(_ context.Context, threadID, runID string) (RunHandle, error) {
	status := f.caseDef.Expect.RequiredTerminal[0]
	if f.product == ProductDeerFlow && f.caseDef.ID == "core.cancel" {
		status = "interrupted"
	}
	return RunHandle{ThreadID: threadID, RunID: runID, Status: status}, nil
}

func (f *fakePlatform) StreamExistingRun(_ context.Context, threadID, runID string, options StreamOptions) (StreamResult, error) {
	f.streamExistingCalls++
	f.streamCursors = append(f.streamCursors, options.AfterEventID)
	if f.caseDef.ID == "core.stream.reconnect" && options.AfterEventID == "" {
		return StreamResult{
			ThreadID: threadID,
			RunID:    runID,
			Frames: []SSEFrame{{
				ID: "1", Event: "values", Data: []byte(`{"messages":[]}`),
			}},
			LastEventID: "1",
		}, nil
	}
	terminal := f.caseDef.Expect.RequiredTerminal[0]
	if f.product == ProductDeerFlow && f.caseDef.ID == "core.cancel" {
		terminal = "interrupted"
	}
	return StreamResult{
		ThreadID:              threadID,
		RunID:                 runID,
		Terminal:              terminal,
		TerminalFrameObserved: true,
		Frames: []SSEFrame{
			{ID: "2", Event: "values", Data: []byte(`{"messages":[]}`)},
			{ID: "3", Event: "end", Data: []byte(`null`)},
		},
		LastEventID: "2",
	}, nil
}

func (f *fakePlatform) CancelRun(_ context.Context, _, _ string) error {
	f.cancelCalls++
	return nil
}

func (f *fakePlatform) FollowUpRun(_ context.Context, threadID, _ string, _ RunInput, _ string) (StreamResult, error) {
	f.followUpCalls++
	return StreamResult{ThreadID: threadID, RunID: string(f.product) + "-run-2", Terminal: "success"}, nil
}

func (f *fakePlatform) GetThreadState(_ context.Context, _ string) (map[string]any, error) {
	f.stateCalls++
	values := map[string]any{}
	if f.caseDef.Expect.Todo.Required {
		values["todos"] = []any{map[string]any{"content": "step", "status": "completed"}}
	}
	return map[string]any{"values": values}, nil
}

func (f *fakePlatform) GetThreadHistory(_ context.Context, _ string, _ int) ([]map[string]any, error) {
	return []map[string]any{{"checkpoint_id": "safe"}}, nil
}

func (f *fakePlatform) ListRunMessages(_ context.Context, _, _ string, page PageRequest) (MessagePage, error) {
	if f.caseDef.ID == "core.cancel" {
		return MessagePage{}, nil
	}
	additionalKwargs := map[string]any{}
	if f.caseDef.Expect.Capabilities.Thinking != nil && *f.caseDef.Expect.Capabilities.Thinking {
		additionalKwargs["reasoning_content"] = "private reasoning"
	}
	if f.paginateMessages {
		f.messageAfterSeqs = append(f.messageAfterSeqs, page.AfterSeq)
		sequence := int64(1)
		hasMore := true
		if page.AfterSeq == 1 {
			sequence = 2
			hasMore = false
		}
		return MessagePage{Data: []map[string]any{{
			"seq": sequence, "role": "assistant", "content": f.privateMessage,
			"additional_kwargs": additionalKwargs,
			"usage_metadata": map[string]any{
				"input_tokens": 10, "output_tokens": 2, "total_tokens": 12,
			},
		}}, HasMore: hasMore}, nil
	}
	return MessagePage{Data: []map[string]any{{
		"role":              "assistant",
		"content":           f.privateMessage,
		"additional_kwargs": additionalKwargs,
		"usage_metadata": map[string]any{
			"input_tokens": 10, "output_tokens": 2, "total_tokens": 12,
		},
	}}}, nil
}

func (f *fakePlatform) ListRunEvents(_ context.Context, _, runID string, _ int) ([]map[string]any, error) {
	if f.caseDef.ID == "core.clarify.followup" {
		if strings.HasSuffix(runID, "run-1") {
			terminal := "run.interrupted"
			if f.product == ProductDeerFlow {
				terminal = "run.completed"
			}
			return []map[string]any{
				{"event_id": "1", "event_type": "run.started", "payload": map[string]any{}},
				{"event_id": "2", "event_type": "clarification.requested", "payload": map[string]any{}},
				{"event_id": "3", "event_type": terminal, "payload": map[string]any{}},
			}, nil
		}
		return []map[string]any{
			{"event_id": "4", "event_type": "run.started", "payload": map[string]any{}},
			{"event_id": "5", "event_type": "assistant.completed", "payload": map[string]any{}},
			{"event_id": "6", "event_type": "run.completed", "payload": map[string]any{}},
		}, nil
	}
	events := make([]map[string]any, 0, len(f.caseDef.Expect.RequiredEvents))
	for index, family := range f.caseDef.Expect.RequiredEvents {
		if f.product == ProductDeerFlow && f.caseDef.ID == "core.cancel" && family == "run.cancelled" {
			continue
		}
		events = append(events, map[string]any{
			"event_id":   fmt.Sprintf("%d", index+1),
			"event_type": family,
			"payload":    map[string]any{},
		})
	}
	return events, nil
}

func (f *fakePlatform) WaitForEvent(_ context.Context, _, _ string, _ string) error {
	return nil
}

func TestNewPrerequisiteErrorRejectsUnsafeCodes(t *testing.T) {
	t.Parallel()

	err := NewPrerequisiteError("password=secret")
	require.True(t, errors.Is(err, ErrInvalidBlocker))
	require.NotContains(t, err.Error(), "secret")
}

func TestBuildRunInputUsesLockedModeProjection(t *testing.T) {
	t.Parallel()

	for _, caseID := range []string{"core.flash.direct", "core.thinking.direct", "core.pro.todo", "core.ultra.subagents"} {
		testCase := mustAcceptanceCase(t, caseID)
		input := BuildRunInput(testCase)
		require.Equal(t, "lead_agent", input.AssistantID)
		require.Equal(t, string(testCase.Mode), input.Context["mode"])
		require.Equal(t, testCase.Mode != ModeFlash, input.Context["thinking_enabled"])
		require.Equal(t, testCase.Mode == ModePro || testCase.Mode == ModeUltra, input.Context["is_plan_mode"])
		require.Equal(t, testCase.Mode == ModeUltra, input.Context["subagent_enabled"])
		require.Equal(t, string(testCase.Mode), input.Config["mode"])
		require.Equal(t, testCase.Mode != ModeFlash, input.Config["thinking_enabled"])
		require.Equal(t, testCase.Mode == ModePro || testCase.Mode == ModeUltra, input.Config["is_plan_mode"])
		require.Equal(t, testCase.Mode == ModeUltra, input.Config["subagent_enabled"])
		require.Equal(t, "eino_adk", input.Config["runtime"])
		require.Equal(t, []string{"events"}, input.StreamMode)
	}
}

func TestLockedDeerFlowJournalRowsProduceCanonicalBoundedSignals(t *testing.T) {
	t.Parallel()

	messageRow := map[string]any{
		"seq":        json.Number("2"),
		"event_type": "llm.ai.response",
		"category":   "message",
		"content": map[string]any{
			"type":    "ai",
			"content": "PRIVATE_ASSISTANT_TEXT",
			"additional_kwargs": map[string]any{
				"reasoning_content": "PRIVATE_REASONING_TEXT",
			},
		},
		"metadata": map[string]any{
			"caller": "lead_agent",
			"usage": map[string]any{
				"input_tokens": json.Number("12"), "output_tokens": json.Number("3"), "total_tokens": json.Number("15"),
			},
		},
	}
	message := rawMessageFromMap(messageRow)
	require.Equal(t, "assistant", message.Role)
	require.Equal(t, "PRIVATE_ASSISTANT_TEXT", message.Content)
	require.True(t, message.ReasoningPresent)
	usage, ok := rawTokenUsageFromMap(messageRow)
	require.True(t, ok)
	require.Equal(t, RawTokenUsage{Input: 12, Output: 3, Total: 15}, usage)

	events := rawEventsFromMaps([]map[string]any{
		{"seq": json.Number("1"), "event_type": "run.start", "category": "trace", "metadata": map[string]any{"caller": "lead_agent"}},
		messageRow,
		{"seq": json.Number("3"), "event_type": "run.end", "category": "outputs", "metadata": map[string]any{"status": "success"}},
	})
	families := make([]string, 0, len(events))
	for _, event := range events {
		family, err := canonicalEventFamily(event)
		require.NoError(t, err)
		families = append(families, family)
	}
	require.Equal(t, []string{"run.started", "assistant.completed", "token.usage", "run.completed"}, families)
}

func TestReconnectDuplicateCounterUsesOnlyBoundedSSEIDs(t *testing.T) {
	t.Parallel()

	frames := []SSEFrame{
		{ID: "1", Event: "values", Data: []byte(`{"private":"FIRST"}`)},
		{ID: "1", Event: "values", Data: []byte(`{"private":"REPLAY"}`)},
		{ID: "2", Event: "end", Data: []byte(`null`)},
	}
	require.Equal(t, 1, countDuplicateSSEEventIDs(frames))
}

func TestLockedJournalDerivesTodoClarificationAndDistinctSubagents(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{
		{"seq": 1, "event_type": "llm.tool.result", "content": map[string]any{"name": "write_todos"}},
		{"seq": 2, "event_type": "llm.tool.result", "content": map[string]any{"name": "ask_clarification"}},
		{"seq": 3, "event_type": "llm.ai.response", "content": map[string]any{"type": "ai", "content": "odd"}, "metadata": map[string]any{"caller": "subagent:odd"}},
		{"seq": 4, "event_type": "llm.ai.response", "content": map[string]any{"type": "ai", "content": "odd again"}, "metadata": map[string]any{"caller": "subagent:odd"}},
		{"seq": 5, "event_type": "llm.ai.response", "content": map[string]any{"type": "ai", "content": "even"}, "metadata": map[string]any{"caller": "subagent:even"}},
	})
	families := make([]string, 0, len(events))
	for _, event := range events {
		family, err := canonicalEventFamily(event)
		require.NoError(t, err)
		families = append(families, family)
	}
	require.Equal(t, []string{
		"todo.updated",
		"clarification.requested",
		"subagent.started", "subagent.completed", "subagent.completed",
		"subagent.started", "subagent.completed",
	}, families)
}

func TestLockedJournalDerivesClarificationFromToolCallWithoutArguments(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{{
		"seq":        1,
		"event_type": "llm.ai.response",
		"content": map[string]any{
			"type": "ai",
			"tool_calls": []any{map[string]any{
				"id": "call-1", "name": "ask_clarification", "args": "PRIVATE_CLARIFICATION_ARGUMENTS",
			}},
		},
	}})
	families := make([]string, 0, len(events))
	for _, event := range events {
		family, err := canonicalEventFamily(event)
		require.NoError(t, err)
		families = append(families, family)
	}
	require.Contains(t, families, "clarification.requested")
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE_CLARIFICATION_ARGUMENTS")
}

func TestLockedJournalDerivesTodoFromBoundedTokenAttribution(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{{
		"seq":        1,
		"event_type": "llm.ai.response",
		"content": map[string]any{
			"type": "ai",
			"additional_kwargs": map[string]any{
				"token_usage_attribution": map[string]any{
					"kind": "tool_batch",
					"actions": []any{
						map[string]any{"kind": "todo_start", "description": "PRIVATE_TODO_DESCRIPTION"},
						map[string]any{"kind": "todo_update", "description": "PRIVATE_TODO_DESCRIPTION"},
					},
				},
			},
		},
	}})
	require.Len(t, events, 1)
	family, err := canonicalEventFamily(events[0])
	require.NoError(t, err)
	require.Equal(t, "todo.updated", family)
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE_TODO_DESCRIPTION")
}

func TestLockedJournalDerivesTodoFromBoundedWriteTodosCall(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{{
		"seq":        1,
		"event_type": "llm.ai.response",
		"content": map[string]any{
			"type": "ai",
			"tool_calls": []any{
				map[string]any{
					"name": "write_todos",
					"args": map[string]any{"todos": "PRIVATE_TODO_ARGUMENTS"},
				},
			},
		},
	}})
	require.Len(t, events, 1)
	family, err := canonicalEventFamily(events[0])
	require.NoError(t, err)
	require.Equal(t, "todo.updated", family)
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE_TODO_ARGUMENTS")
}

func TestLockedJournalDerivesParallelSubagentsFromTaskToolLifecycle(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{
		{
			"seq":        3,
			"event_type": "llm.ai.response",
			"content": map[string]any{
				"type": "ai",
				"tool_calls": []any{
					map[string]any{"id": "call-1", "name": "task", "args": "PRIVATE_SUBAGENT_ARGUMENTS"},
					map[string]any{"id": "call-2", "name": "task", "args": "PRIVATE_SUBAGENT_ARGUMENTS"},
				},
			},
		},
		{"seq": 5, "event_type": "llm.tool.result", "content": map[string]any{"name": "task", "tool_call_id": "call-1"}},
		{"seq": 6, "event_type": "llm.tool.result", "content": map[string]any{"name": "task", "tool_call_id": "call-2"}},
	})
	families := make([]string, 0, len(events))
	for _, event := range events {
		family, err := canonicalEventFamily(event)
		require.NoError(t, err)
		families = append(families, family)
	}
	require.Equal(t, 2, countString(families, "subagent.started"))
	require.Equal(t, 2, countString(families, "subagent.completed"))
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE_SUBAGENT_ARGUMENTS")
}

func TestLockedJournalDoesNotCompleteUnpairedOrFailedTaskToolResults(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{
		{
			"seq":        3,
			"event_type": "llm.ai.response",
			"content": map[string]any{
				"tool_calls": []any{
					map[string]any{"id": "call-1", "name": "task"},
					map[string]any{"id": "call-2", "name": "task"},
				},
			},
		},
		{
			"seq":        5,
			"event_type": "llm.tool.result",
			"content":    map[string]any{"name": "task", "tool_call_id": "call-x"},
		},
		{
			"seq":        6,
			"event_type": "llm.tool.result",
			"content": map[string]any{
				"name": "task", "tool_call_id": "call-1", "status": "failed",
			},
		},
	})
	families := make([]string, 0, len(events))
	for _, event := range events {
		family, err := canonicalEventFamily(event)
		require.NoError(t, err)
		families = append(families, family)
	}
	require.Equal(t, 2, countString(families, "subagent.started"))
	require.Zero(t, countString(families, "subagent.completed"))
}

func countString(values []string, expected string) int {
	count := 0
	for _, value := range values {
		if value == expected {
			count++
		}
	}
	return count
}

func TestNewXInterruptDerivesClarificationWithoutExposingInteractionPayload(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{{
		"event_id":   "7",
		"event_type": "run.interrupted",
		"payload": map[string]any{
			"human_interaction": map[string]any{
				"kind": "clarification", "question": "PRIVATE_QUESTION",
			},
		},
	}})
	require.Len(t, events, 2)
	family, err := canonicalEventFamily(events[1])
	require.NoError(t, err)
	require.Equal(t, "clarification.requested", family)
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE_QUESTION")
}

func TestNewXCapabilityEventKeepsOnlyEffectiveThinkingSignal(t *testing.T) {
	t.Parallel()

	events := rawEventsFromMaps([]map[string]any{{
		"event_id":   "8",
		"event_type": "model.capability_downgraded",
		"payload": `{
			"effective_thinking_enabled": true,
			"requested_thinking_enabled": true,
			"private_provider_body": "PRIVATE_PROVIDER_BODY"
		}`,
	}})
	require.Len(t, events, 1)
	require.Equal(t, true, events[0].Payload["effective_thinking_enabled"])
	encoded, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE_PROVIDER_BODY")
	require.NotContains(t, string(encoded), "requested_thinking_enabled")
}

func TestWriteMarkdownReportRejectsUnsafeBlocker(t *testing.T) {
	t.Parallel()

	report := SuiteReport{
		Schema:               SuiteReportSchemaV1,
		ReferenceEnvironment: "deerflow",
		CandidateEnvironment: "newx",
		Cases: []CaseReport{{
			CaseID: "core.flash.direct",
			Comparison: ComparisonResult{
				Status:  StatusBlocked,
				Blocker: strings.Repeat("x", 200) + " secret",
			},
		}},
	}
	var output bytes.Buffer
	err := WriteMarkdownReport(&output, report)
	require.Error(t, err)
	require.Empty(t, output.String())
}
