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
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	journalcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/journal_contract"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

func TestGetCanonicalRunJournalReturnsFrozenBootstrap(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunFixture(t, 1, "journal bootstrap")
	previousRepository := appagentthread.SVC.JournalQueryRepository
	repository := &canonicalJournalQueryRepositoryStub{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{{
			ThreadID: 1, JournalRunID: run.RunID, ExecutionRunID: run.RunID,
			AttemptID: "att-handler", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
			NextSequence: 3, SnapshotsEnabled: true,
			ProjectionState: domainentity.JournalProjectionStateHealthy,
			CreatedAt:       run.CreatedAt, StartedAt: pointerToInt64(run.CreatedAt),
		}},
		SelectedAttempt: &domainentity.RunAttempt{
			ThreadID: 1, JournalRunID: run.RunID, ExecutionRunID: run.RunID,
			AttemptID: "att-handler", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
			NextSequence: 3, SnapshotsEnabled: true,
			ProjectionState: domainentity.JournalProjectionStateHealthy,
			CreatedAt:       run.CreatedAt, StartedAt: pointerToInt64(run.CreatedAt),
		},
		Events: []*domainentity.JournalEvent{{
			ID: 501, ThreadID: 1, RunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: "att-handler", Sequence: 1, EventType: "milestone.started",
			Payload:        `{"type":"milestone","data":{"milestone_id":"m1","title":"规划"}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: run.CreatedAt,
		}},
		LatestSequence: 2, HasMore: true,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	t.Cleanup(func() { appagentthread.SVC.JournalQueryRepository = previousRepository })

	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/journal", GetCanonicalRunJournal)
	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodGet,
		"/api/workbench/threads/1/runs/"+strconv.FormatInt(run.RunID, 10)+
			"/journal?journal_protocol_version=1.1&attempt_id=att-handler&limit=1",
		"",
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &body))
	require.Equal(t, "att-handler", body["default_attempt_id"])
	require.Equal(t, float64(2), body["latest_sequence"])
	require.NotEmpty(t, body["submit_at"])
	require.NotEmpty(t, body["server_time"])
	enrollment := body["enrollment"].(map[string]any)
	require.Equal(t, true, enrollment["journal_enabled"])
	require.Equal(t, true, enrollment["snapshots_enabled"])
	require.Equal(t, "1.1", enrollment["journal_protocol_version"])
	events := body["events"].(map[string]any)
	require.Equal(t, true, events["has_more"])
	require.Len(t, events["data"], 1)
	firstEvent := events["data"].([]any)[0].(map[string]any)
	require.IsType(t, map[string]any{}, firstEvent["payload"])
	require.NotContains(t, string(response.Result().Body()), "internal_reason")
}

func TestListCanonicalRunEventsUsesJournalAttemptSequence(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunFixture(t, 1, "journal polling")
	attempt := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: run.RunID, ExecutionRunID: run.RunID,
		AttemptID: "att-poll", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
		NextSequence: 3, ProjectionState: domainentity.JournalProjectionStateHealthy,
		CreatedAt: run.CreatedAt,
	}
	repository := &canonicalJournalQueryRepositoryStub{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		Events: []*domainentity.JournalEvent{{
			ID: 601, ThreadID: 1, RunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: attempt.AttemptID, Sequence: 2, EventType: "action.completed",
			IdempotencyKey: "journal-poll-601",
			Payload:        `{"type":"generic","data":{"action_id":"a1","operation":"read","target":"PRD","display_verb_running":"正在读取 PRD","display_verb_completed":"已读取 PRD"}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: run.CreatedAt,
		}},
		LatestSequence: 2,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/events", ListCanonicalRunEvents)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodGet,
		"/api/workbench/threads/1/runs/"+strconv.FormatInt(run.RunID, 10)+
			"/events?attempt_id=att-poll&after_sequence=1&after_event_id=600&limit=10",
		"",
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &body))
	require.Equal(t, "att-poll", body["attempt_id"])
	require.Equal(t, float64(2), body["latest_sequence"])
	require.Equal(t, float64(2), body["next_after_sequence"])
	require.Equal(t, "601", body["next_after_event_id"])
	events := body["data"].([]any)
	require.Len(t, events, 1)
	require.IsType(t, map[string]any{}, events[0].(map[string]any)["payload"])
	require.Equal(t, "journal-poll-601", events[0].(map[string]any)["idempotency_key"])
	require.Equal(t, "att-poll", repository.lastRequest.AttemptID)
	require.Equal(t, uint64(1), repository.lastRequest.AfterSequence)
	require.Equal(t, int64(600), repository.lastRequest.AfterEventID)
}

func TestListCanonicalRunEventsPreservesExplicitZeroSequenceCursor(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunFixture(t, 1, "journal zero cursor")
	attempt := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: run.RunID, ExecutionRunID: run.RunID,
		AttemptID: "att-zero", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
		NextSequence: 1, ProjectionState: domainentity.JournalProjectionStateHealthy,
		CreatedAt: run.CreatedAt,
	}
	repository := &canonicalJournalQueryRepositoryStub{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/events", ListCanonicalRunEvents)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodGet,
		"/api/workbench/threads/1/runs/"+strconv.FormatInt(run.RunID, 10)+
			"/events?attempt_id=att-zero&after_sequence=0&after_event_id=600",
		"",
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	require.True(t, repository.lastRequest.AfterSequenceSet)
	require.Zero(t, repository.lastRequest.AfterSequence)
}

func TestCanonicalJournalSharedEntrypointsUseDualFieldErrorsWhenServiceUnavailable(t *testing.T) {
	installAgentThreadTestService(t)
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/events", ListCanonicalRunEvents)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/stream", ReconnectCanonicalRunStream)
	previous := appagentthread.SVC
	appagentthread.SVC = nil
	t.Cleanup(func() { appagentthread.SVC = previous })

	for _, path := range []string{
		"/api/workbench/threads/1/runs/2/events?attempt_id=attempt-1&after_sequence=0",
		"/api/workbench/threads/1/runs/2/stream?journal_protocol_version=1.1&attempt_id=attempt-1",
	} {
		response := performCanonicalRunJSONRequest(t, h, http.MethodGet, path, "")
		require.Equal(t, http.StatusServiceUnavailable, response.Code, path)

		var body map[string]any
		require.NoError(t, json.Unmarshal(response.Result().Body(), &body))
		require.Equal(t, "dependency_unavailable", body["error_code"], path)
		require.Equal(t, body["error_code"], body["code"], path)
		require.NotContains(t, string(response.Result().Body()), "internal_reason", path)
	}
}

func TestCanonicalJournalCursorFailuresUseFrozenHTTPErrors(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunFixture(t, 1, "journal cursor failures")
	repository := &canonicalJournalQueryRepositoryStub{}
	appagentthread.SVC.JournalQueryRepository = repository
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/journal", GetCanonicalRunJournal)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/events", ListCanonicalRunEvents)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/stream", ReconnectCanonicalRunStream)
	base := "/api/workbench/threads/1/runs/" + strconv.FormatInt(run.RunID, 10)
	paths := []string{
		base + "/journal?attempt_id=attempt-1&after_sequence=1&after_event_id=99",
		base + "/events?attempt_id=attempt-1&after_sequence=1&after_event_id=99",
		base + "/stream?journal_protocol_version=1.1&attempt_id=attempt-1&after_sequence=1&after_event_id=99",
	}

	for _, failure := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "expired", err: domainrepo.ErrJournalCursorExpired, status: http.StatusGone, code: "JOURNAL_CURSOR_EXPIRED"},
		{name: "gap", err: domainrepo.ErrJournalEventGap, status: http.StatusConflict, code: "JOURNAL_EVENT_GAP"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			repository.err = failure.err
			for _, path := range paths {
				response := performCanonicalRunJSONRequest(t, h, http.MethodGet, path, "")
				require.Equal(t, failure.status, response.Code, path)

				var body map[string]any
				require.NoError(t, json.Unmarshal(response.Result().Body(), &body))
				require.Equal(t, failure.code, body["error_code"], path)
				require.Equal(t, body["error_code"], body["code"], path)
				require.Equal(t, true, body["retryable"], path)
				require.NotContains(t, string(response.Result().Body()), "internal_reason", path)
			}
		})
	}
}

func TestCanonicalJournalSettingsUseSessionPrincipalAndCAS(t *testing.T) {
	previousStore := appagentthread.SVC.JournalUserSettingsStore
	previousNow := appagentthread.SVC.JournalSettingsNow
	store := &canonicalJournalSettingsStoreStub{}
	appagentthread.SVC.JournalUserSettingsStore = store
	appagentthread.SVC.JournalSettingsNow = func() int64 { return 9876 }
	t.Cleanup(func() {
		appagentthread.SVC.JournalUserSettingsStore = previousStore
		appagentthread.SVC.JournalSettingsNow = previousNow
	})

	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(42))
	h.GET("/api/workbench/journal/settings", GetCanonicalJournalSettings)
	h.PATCH("/api/workbench/journal/settings", PatchCanonicalJournalSettings)

	getResponse := performCanonicalRunJSONRequest(
		t, h, http.MethodGet, "/api/workbench/journal/settings", "",
	)
	require.Equal(t, http.StatusOK, getResponse.Code, getResponse.Result().Body())
	require.Equal(t, "42", store.getKey)

	patchResponse := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPatch,
		"/api/workbench/journal/settings",
		`{"split_ratio":0.61,"revision":"missing"}`,
	)
	require.Equal(t, http.StatusOK, patchResponse.Code, patchResponse.Result().Body())
	require.Equal(t, "42", store.casKey)
	require.Equal(t, "missing", store.expectedRevision)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(patchResponse.Result().Body(), &settings))
	require.Equal(t, 0.61, settings["split_ratio"])
	require.Equal(t, "rev-handler", settings["revision"])
}

func TestCanonicalJournalAdmissionReturnsControlledRetryAfter(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunFixture(t, 1, "journal admission")
	repository := &canonicalJournalQueryRepositoryStub{}
	appagentthread.SVC.JournalQueryRepository = repository
	appagentthread.SVC.JournalAdmissionLimiter = canonicalJournalAdmissionLimiterStub{
		err: &appagentthread.JournalRateLimitError{RetryAfter: 3 * time.Second},
	}
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/journal", GetCanonicalRunJournal)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodGet,
		"/api/workbench/threads/1/runs/"+strconv.FormatInt(run.RunID, 10)+"/journal",
		"",
	)

	require.Equal(t, http.StatusTooManyRequests, response.Code, response.Result().Body())
	require.Equal(t, "3", response.Result().Header.Get("Retry-After"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &body))
	require.Equal(t, "JOURNAL_RATE_LIMITED", body["error_code"])
	require.Equal(t, body["error_code"], body["code"])
	require.Zero(t, repository.lastRequest.RunID)
}

func TestCanonicalJournalCoreRoutesNoLongerReturnNotImplemented(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunFixture(t, 1, "journal routes")
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id", GetCanonicalRunSnapshot)
	h.POST("/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id/actions", AuditCanonicalRunSnapshotAction)
	h.POST("/api/workbench/threads/:thread_id/runs/:run_id/recover", RecoverCanonicalRunJournal)

	base := "/api/workbench/threads/1/runs/" + strconv.FormatInt(run.RunID, 10)
	requests := []struct {
		method  string
		path    string
		body    string
		headers []ut.Header
	}{
		{method: http.MethodGet, path: base + "/snapshots/missing"},
		{
			method: http.MethodPost, path: base + "/snapshots/missing/actions",
			body:    `{"action":"copy_code"}`,
			headers: []ut.Header{{Key: "Idempotency-Key", Value: "snapshot-action-1"}},
		},
		{
			method: http.MethodPost, path: base + "/recover",
			body:    `{"action":"retry","confirmed":true}`,
			headers: []ut.Header{{Key: "Idempotency-Key", Value: "recovery-1"}},
		},
	}
	for _, request := range requests {
		response := performCanonicalRunJSONRequest(
			t, h, request.method, request.path, request.body, request.headers...,
		)
		require.NotEqual(t, http.StatusNotImplemented, response.Code, response.Result().Body())
		require.NotContains(t, string(response.Result().Body()), "journal_not_implemented")
		if request.path == base+"/recover" {
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.JSONEq(t, `{
				"detail":"Required service is unavailable",
				"error_code":"dependency_unavailable",
				"code":"dependency_unavailable",
				"retryable":true,
				"trace_id":""
			}`, string(response.Result().Body()))
		}
	}
}

func TestProjectCanonicalJournalSnapshotUsesTypedSafeViews(t *testing.T) {
	base := appagentthread.JournalSnapshotEnvelope{
		SnapshotID: "snapshot-1", EventID: 501, AttemptID: "attempt-1",
		Status: domainentity.JournalContentStatusReady, CreatedAt: 1_000,
		Visibility: domainentity.JournalVisibilityUser,
		Fragments: []appagentthread.JournalSnapshotFragmentView{{
			FragmentID: "fragment-1", FragmentIndex: 0,
			Kind:          appagentthread.JournalSnapshotFragmentKindBrowserThumbnail,
			BinaryContent: []byte{1, 2}, MIMEType: "image/png",
			ByteStart: 0, ByteEnd: 2, SizeBytes: 2, ContentHash: "hash-1",
		}},
	}
	tests := []struct {
		name        string
		contentType domainentity.JournalSnapshotContentType
		content     appagentthread.JournalTypedSnapshotContent
		assert      func(*testing.T, *journalcontract.JournalSnapshotContent)
	}{
		{
			name: "document", contentType: domainentity.JournalSnapshotContentTypeDocument,
			content: appagentthread.JournalTypedSnapshotContent{Document: &appagentthread.JournalDocumentContent{
				Title: "验收报告", Format: "markdown", Content: "正文",
				Chapters: []appagentthread.JournalDocumentChapter{{
					ChapterID: "chapter-1", Title: "结论", Level: 1,
				}},
			}},
			assert: func(t *testing.T, content *journalcontract.JournalSnapshotContent) {
				require.NotNil(t, content.Document)
				require.Equal(t, "验收报告", content.Document.Title)
				require.Equal(t, "chapter-1", content.Document.Chapters[0].ChapterID)
			},
		},
		{
			name: "terminal", contentType: domainentity.JournalSnapshotContentTypeTerminal,
			content: appagentthread.JournalTypedSnapshotContent{Terminal: &appagentthread.JournalTerminalContent{
				Command: "go test ./...", Stdout: "PASS", StartedAt: 2_000,
				FinishedAt: 3_000, DurationMS: 1_000,
			}},
			assert: func(t *testing.T, content *journalcontract.JournalSnapshotContent) {
				require.NotNil(t, content.Terminal)
				require.Equal(t, "go test ./...", content.Terminal.Command)
				require.Equal(t, "1970-01-01T00:00:02Z", content.Terminal.GetStartedAt())
			},
		},
		{
			name: "code", contentType: domainentity.JournalSnapshotContentTypeCode,
			content: appagentthread.JournalTypedSnapshotContent{Code: &appagentthread.JournalCodeContent{
				Repository: "coze-studio", Revision: "abcdef1", Path: "main.go",
				Content: "package main", StartLine: 1, EndLine: 1,
			}},
			assert: func(t *testing.T, content *journalcontract.JournalSnapshotContent) {
				require.NotNil(t, content.Code)
				require.Equal(t, "main.go", content.Code.FilePath)
				require.Equal(t, "coze-studio", content.Code.Repository)
			},
		},
		{
			name: "skill list only", contentType: domainentity.JournalSnapshotContentTypeSkill,
			content: appagentthread.JournalTypedSnapshotContent{Skill: &appagentthread.JournalSkillContent{
				Skills: []appagentthread.JournalSkillSummary{{
					SkillID: "research", Name: "Research", Description: "检索公开资料",
					InvocationSummary: "must never be public",
				}},
			}},
			assert: func(t *testing.T, content *journalcontract.JournalSnapshotContent) {
				require.NotNil(t, content.Skill)
				require.Len(t, content.Skill.Skills, 1)
				skill := content.Skill.Skills[0]
				require.Equal(t, "检索公开资料", skill.GetDescription())
				require.Nil(t, skill.InvocationStatus)
				require.Nil(t, skill.InputSummary)
				require.Nil(t, skill.OutputArtifacts)
				require.Nil(t, skill.PurposeSummary)
			},
		},
		{
			name: "browser static only", contentType: domainentity.JournalSnapshotContentTypeBrowser,
			content: appagentthread.JournalTypedSnapshotContent{Browser: &appagentthread.JournalBrowserContent{
				CaptureID: "capture-1", Resource: "https://example.com", Title: "Example",
				Thumbnail: []byte{1, 2}, StaticSnapshot: []byte("static"), MIMEType: "image/png",
				Redacted: true, RedactionEvidenceID: "evidence-1",
				RedactionPolicyVersion: "v1",
			}},
			assert: func(t *testing.T, content *journalcontract.JournalSnapshotContent) {
				require.NotNil(t, content.Browser)
				require.Equal(t, "AQI=", content.Browser.GetThumbnailBase64())
				require.Equal(t, "c3RhdGlj", content.Browser.StaticSnapshotBase64)
				require.True(t, content.Browser.Redacted)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := base
			envelope.ContentType = test.contentType
			envelope.Content = test.content
			projected, err := projectCanonicalJournalSnapshot(&envelope)
			require.NoError(t, err)
			require.Equal(t, "501", projected.EventID)
			require.Equal(t, "1970-01-01T00:00:01Z", projected.CreatedAt)
			require.Len(t, projected.Fragments, 1)
			require.Equal(t, "AQI=", projected.Fragments[0].GetBinaryContentBase64())
			require.Equal(t, int64(0), projected.Fragments[0].GetByteStart())
			test.assert(t, projected.Content)
		})
	}
}

func TestProjectCanonicalJournalActionGrantPreservesAuditedCapability(t *testing.T) {
	projected, err := projectCanonicalJournalActionGrant(
		&appagentthread.JournalSnapshotActionGrant{
			SnapshotID: "snapshot-1", Action: domainentity.JournalSnapshotActionDownloadFragment,
			Allowed: true, AuditedAt: 2_000, CopyText: "copy",
			DownloadURL: "https://download.example/file", DownloadContent: []byte{1, 2},
			DownloadMIMEType: "text/plain",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "1970-01-01T00:00:02Z", projected.AuditedAt)
	require.Equal(t, "AQI=", projected.GetDownloadContentBase64())
	require.Equal(t, "text/plain", projected.GetDownloadMimeType())
}

func TestProjectCanonicalJournalUsesLogicalRunIDForRecoveryAttempt(t *testing.T) {
	attempt := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 20,
		AttemptID: "attempt-recovery", Status: domainentity.RunAttemptStatusRunning,
		ProjectionState: domainentity.JournalProjectionStateHealthy, CreatedAt: 1_000,
	}
	summary := projectCanonicalJournalAttempt(attempt, 1)
	require.Equal(t, "10", summary.RunID)

	event, err := projectCanonicalJournalEvent(&domainentity.JournalEvent{
		ID: 101, ThreadID: 1, RunID: 20, JournalRunID: 10,
		AttemptID: "attempt-recovery", Sequence: 1, EventType: "action.completed",
		IdempotencyKey: "journal-action-101",
		Payload:        `{"type":"generic","data":{}}`,
		SchemaVersion:  domainentity.JournalSchemaVersion,
		PayloadVersion: domainentity.JournalPayloadVersion,
		Visibility:     domainentity.JournalVisibilityUser, CreatedAt: 1_000,
	})
	require.NoError(t, err)
	require.Equal(t, "10", event.RunID)
	require.Equal(t, "journal-action-101", event.GetIdempotencyKey())
}

func pointerToInt64(value int64) *int64 { return &value }

type canonicalJournalQueryRepositoryStub struct {
	result      *domainrepo.GetJournalBootstrapResult
	err         error
	lastRequest domainrepo.GetJournalBootstrapRequest
}

func (r *canonicalJournalQueryRepositoryStub) GetJournalBootstrap(
	_ context.Context,
	req domainrepo.GetJournalBootstrapRequest,
) (*domainrepo.GetJournalBootstrapResult, error) {
	r.lastRequest = req
	return r.result, r.err
}

type canonicalJournalSettingsStoreStub struct {
	getKey           string
	casKey           string
	expectedRevision string
}

type canonicalJournalAdmissionLimiterStub struct {
	err error
}

func (l canonicalJournalAdmissionLimiterStub) Acquire(
	context.Context,
	appagentthread.JournalAdmissionRequest,
) (appagentthread.JournalAdmissionLease, error) {
	return nil, l.err
}

func (s *canonicalJournalSettingsStoreStub) GetVersioned(
	_ context.Context,
	_ string,
	key string,
) (*appagentthread.JournalUserSettingsRecord, string, error) {
	s.getKey = key
	return nil, "", kvstore.ErrKeyNotFound
}

func (s *canonicalJournalSettingsStoreStub) CompareAndSwap(
	_ context.Context,
	_ string,
	key string,
	expectedRevision string,
	_ *appagentthread.JournalUserSettingsRecord,
) (string, error) {
	s.casKey = key
	s.expectedRevision = expectedRevision
	return "rev-handler", nil
}
