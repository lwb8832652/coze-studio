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
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func TestCanonicalThreadArtifactListReturnsProjectedArtifactsAndPagination(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "artifacts", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "artifact fixture")
	first := createCanonicalArtifactFixtureForRun(t, thread.ThreadID, run, "report-a.txt", "text/plain; charset=utf-8", []byte("alpha"), "clean")
	second := createCanonicalArtifactFixtureForRun(t, thread.ThreadID, run, "report-b.txt", "text/plain; charset=utf-8", []byte("beta"), "clean")
	h := canonicalArtifactTestServer()

	emptyPath := fmt.Sprintf(
		"/api/workbench/threads/%d/artifacts?run_id=%d&limit=10&offset=0",
		thread.ThreadID,
		first.Run.RunID+999,
	)
	empty := performCanonicalArtifactRequest(t, h, http.MethodGet, emptyPath, nil)
	require.Equal(t, http.StatusOK, empty.Code, empty.Result().Body())
	require.Contains(t, string(empty.Result().Body()), `"artifacts":[]`)
	require.Contains(t, string(empty.Result().Body()), `"total":0`)

	path := fmt.Sprintf(
		"/api/workbench/threads/%d/artifacts?run_id=%d&limit=1&offset=0",
		thread.ThreadID,
		first.Run.RunID,
	)
	var output bytes.Buffer
	logs.SetOutput(&output)
	t.Cleanup(func() { logs.SetOutput(os.Stderr) })
	response := performCanonicalArtifactRequest(t, h, http.MethodGet, path, nil)
	body := string(response.Result().Body())

	require.Equal(t, http.StatusOK, response.Code, body)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"has_more":true`)
	require.Contains(t, body, `"next_cursor":"1"`)
	require.Contains(t, body, `"artifact_id":"`+strconv.FormatInt(second.Artifact.ID, 10)+`"`)
	require.NotContains(t, body, `"artifact_id":"`+strconv.FormatInt(first.Artifact.ID, 10)+`"`)
	require.Contains(t, body, `"created_at":"`)
	require.Contains(t, body, `"updated_at":"`)
	require.NotContains(t, body, `"code"`)
	require.NotContains(t, body, first.ObjectURI)
	require.NotContains(t, body, "/mnt/user-data")
	require.NotContains(t, body, "provider_body")
	require.Contains(t, output.String(), "limit=1")
	require.Contains(t, output.String(), "offset=0")
	require.NotContains(t, output.String(), second.ObjectURI)
}

func TestCanonicalArtifactCollectionProjectionKeepsVisibleOrderAndPosition(t *testing.T) {
	currentIndex := int32(1)
	collections, err := canonicalProductArtifactCollectionsToAPI(
		[]*appagentthread.ArtifactCollectionSummary{
			{
				CollectionID: "collection-safe",
				ArtifactIDs:  []int64{101, 103},
				CurrentIndex: &currentIndex,
				TotalCount:   2,
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, collections, 1)
	require.Equal(t, "collection-safe", collections[0].CollectionID)
	require.Equal(t, []string{"101", "103"}, collections[0].ArtifactIDs)
	require.NotNil(t, collections[0].CurrentIndex)
	require.Equal(t, int32(1), *collections[0].CurrentIndex)
	require.Equal(t, int32(2), collections[0].TotalCount)
}

func TestCanonicalThreadArtifactContentAndSignedURLAreSafe(t *testing.T) {
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects:   map[string][]byte{},
		signedURL: "https://storage.example.test/signed/report.txt?token=signed-url-secret",
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	thread := createCanonicalTestThread(t, 1001, "artifact content", `{}`)
	fixture := createCanonicalArtifactFixture(
		t,
		thread.ThreadID,
		"report.txt",
		"text/plain; charset=utf-8",
		[]byte("artifact body"),
		"clean",
	)
	storage.objects[fixture.ObjectURI] = fixture.Content
	h := canonicalArtifactTestServer()

	content := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/content?mode=preview", thread.ThreadID, fixture.Artifact.ID),
		nil,
	)
	result := content.Result()
	require.Equal(t, http.StatusOK, result.StatusCode())
	require.Equal(t, fixture.Content, result.Body())
	require.Contains(t, string(result.Header.Peek("Content-Type")), "text/plain")
	require.Contains(t, string(result.Header.Peek("Content-Disposition")), "inline")
	require.Contains(t, string(result.Header.Peek("Content-Disposition")), "filename*=UTF-8''report.txt")
	require.Equal(t, "nosniff", string(result.Header.Peek("X-Content-Type-Options")))

	var output bytes.Buffer
	logs.SetOutput(&output)
	t.Cleanup(func() { logs.SetOutput(os.Stderr) })
	signed := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/signed_url?mode=download&ttl_seconds=5", thread.ThreadID, fixture.Artifact.ID),
		nil,
	)
	body := string(signed.Result().Body())
	require.Equal(t, http.StatusOK, signed.Code, body)
	require.Contains(t, body, `"url":"https://storage.example.test/signed/report.txt?token=signed-url-secret"`)
	require.Contains(t, body, `"expires_in_seconds":60`)
	require.Contains(t, body, `"preview_mode":"text"`)
	require.Equal(t, fixture.ObjectURI, storage.signKey)
	require.Equal(t, int64(60), storage.signExpire)
	require.Equal(t, "attachment; filename*=UTF-8''report.txt", storage.signContentDisposition)
	require.Contains(t, storage.signContentType, "text/plain")
	require.NotContains(t, output.String(), "signed-url-secret")
	require.NotContains(t, output.String(), fixture.ObjectURI)
}

func TestCanonicalArtifactContentSupportsBoundedRangeWithoutBuffering(t *testing.T) {
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{objects: map[string][]byte{}}
	appagentthread.SVC.ArtifactObjectStorage = storage
	thread := createCanonicalTestThread(t, 1001, "artifact range", `{}`)
	fixture := createCanonicalArtifactFixture(
		t,
		thread.ThreadID,
		"range.txt",
		"text/plain; charset=utf-8",
		[]byte("0123456789"),
		"clean",
	)
	storage.objects[fixture.ObjectURI] = fixture.Content
	h := canonicalArtifactTestServer()

	response := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/content?mode=preview", thread.ThreadID, fixture.Artifact.ID),
		nil,
		ut.Header{Key: "Range", Value: "bytes=2-5"},
	)
	result := response.Result()

	require.Equal(t, http.StatusPartialContent, result.StatusCode())
	require.Equal(t, []byte("2345"), result.Body())
	require.Equal(t, "bytes 2-5/10", string(result.Header.Peek("Content-Range")))
	require.Equal(t, "bytes", string(result.Header.Peek("Accept-Ranges")))
	require.Equal(t, "private, no-store", string(result.Header.Peek("Cache-Control")))
	require.Zero(t, storage.getCalls, "range delivery must not buffer through GetObject")
	require.Equal(t, 1, storage.openCalls)
}

func TestCanonicalLegacyArtifactDownloadStreamsAsAttachmentWithoutRange(t *testing.T) {
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{objects: map[string][]byte{}}
	appagentthread.SVC.ArtifactObjectStorage = storage
	thread := createCanonicalTestThread(t, 1001, "legacy artifact", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "legacy artifact")
	objectURI := fmt.Sprintf("agent-runtime/1001/%d/runs/%d/outputs/legacy.html", thread.ThreadID, run.RunID)
	content := []byte("<script>unsafe()</script>")
	storage.objects[objectURI] = content
	legacyArtifact := &domainentity.AgentArtifact{
		ID:               909,
		SpaceID:          1001,
		UserID:           2,
		ThreadID:         thread.ThreadID,
		RunID:            run.RunID,
		JournalRunID:     run.RunID,
		FileID:           808,
		Title:            "legacy.html",
		ArtifactType:     "document",
		ObjectURI:        objectURI,
		ContentType:      "text/html",
		SizeBytes:        int64(len(content)),
		PreviewMode:      domainentity.AgentArtifactPreviewModeText,
		GenerationStatus: domainentity.AgentArtifactGenerationStatusReady,
		Metadata:         `{"scan_status":"clean"}`,
	}
	appagentthread.SVC.ArtifactSVC = &canonicalLegacyArtifactService{
		ArtifactService: appagentthread.SVC.ArtifactSVC,
		artifact:        legacyArtifact,
	}
	h := canonicalArtifactTestServer()

	response := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/content?mode=download", thread.ThreadID, legacyArtifact.ID),
		nil,
	)
	result := response.Result()

	require.Equal(t, http.StatusOK, result.StatusCode())
	require.Equal(t, content, result.Body())
	require.Equal(t, "application/octet-stream", string(result.Header.Peek("Content-Type")))
	require.Contains(t, string(result.Header.Peek("Content-Disposition")), "attachment")
	require.Empty(t, result.Header.Peek("Accept-Ranges"))
	require.Zero(t, storage.getCalls)
	require.Equal(t, 1, storage.openCalls)

	ranged := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/content?mode=download", thread.ThreadID, legacyArtifact.ID),
		nil,
		ut.Header{Key: "Range", Value: "bytes=0-3"},
	)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, ranged.Code)
	require.NotContains(t, string(ranged.Result().Body()), "unsafe")
	require.Equal(t, 1, storage.openCalls)
}

func TestCanonicalArtifactSignedPreviewDoesNotReadWholeObject(t *testing.T) {
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects:   map[string][]byte{},
		signedURL: "https://storage.example.test/signed/report.txt?token=preview-secret",
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	thread := createCanonicalTestThread(t, 1001, "artifact signed preview", `{}`)
	fixture := createCanonicalArtifactFixture(
		t,
		thread.ThreadID,
		"report.txt",
		"text/plain; charset=utf-8",
		[]byte("artifact body"),
		"clean",
	)
	storage.objects[fixture.ObjectURI] = fixture.Content
	h := canonicalArtifactTestServer()

	response := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/signed_url?mode=preview", thread.ThreadID, fixture.Artifact.ID),
		nil,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	require.Zero(t, storage.getCalls, "signed preview must use trusted scan metadata")
	require.Equal(t, "inline; filename*=UTF-8''report.txt", storage.signContentDisposition)
	require.Contains(t, storage.signContentType, "text/plain")
	require.Equal(t, "private, no-store", string(response.Result().Header.Peek("Cache-Control")))
}

func TestCanonicalArtifactCopyLinkIsShortLivedAndAudited(t *testing.T) {
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects:   map[string][]byte{},
		signedURL: "https://storage.example.test/signed/report.txt?token=copy-secret",
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	thread := createCanonicalTestThread(t, 1001, "artifact copy", `{}`)
	fixture := createCanonicalArtifactFixture(
		t,
		thread.ThreadID,
		"report.txt",
		"text/plain; charset=utf-8",
		[]byte("artifact body"),
		"clean",
	)
	storage.objects[fixture.ObjectURI] = fixture.Content
	h := canonicalArtifactTestServer()

	response := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/copy_link", thread.ThreadID, fixture.Artifact.ID),
		nil,
	)
	body := string(response.Result().Body())

	require.Equal(t, http.StatusOK, response.Code, body)
	require.Contains(t, body, `"artifact_id":"`+strconv.FormatInt(fixture.Artifact.ID, 10)+`"`)
	require.Contains(t, body, `"copy_url":"https://storage.example.test/signed/report.txt?token=copy-secret"`)
	require.Contains(t, body, `"expires_at":"`)
	require.Equal(t, "private, no-store", string(response.Result().Header.Peek("Cache-Control")))
	require.Zero(t, storage.getCalls, "copy link issuance must not read artifact bytes")

	events, err := appagentthread.SVC.ListRunEvents(context.Background(), &appagentthread.ListRunEventsRequest{
		ThreadID: fixture.ThreadID,
		RunID:    fixture.Run.RunID,
		Page:     1,
		PageSize: 100,
	})
	require.NoError(t, err)
	require.NotNil(t, events)
	foundCopyAudit := false
	for _, event := range events.Events {
		if event != nil && event.EventType == "artifact.content.accessed" && strings.Contains(event.Payload, `"action":"copy"`) {
			foundCopyAudit = true
			require.Contains(t, event.Payload, `"permission_result":"allowed"`)
			require.NotContains(t, event.Payload, "copy-secret")
			require.NotContains(t, event.Payload, fixture.ObjectURI)
		}
	}
	require.True(t, foundCopyAudit, "copy must write a content-free access audit")
}

func TestCanonicalThreadArtifactScanReviewAndRetry(t *testing.T) {
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{objects: map[string][]byte{}}
	appagentthread.SVC.ArtifactObjectStorage = storage
	thread := createCanonicalTestThread(t, 1001, "artifact scan", `{}`)
	blocked := createCanonicalArtifactFixture(
		t,
		thread.ThreadID,
		"blocked.txt",
		"text/plain; charset=utf-8",
		[]byte("blocked body"),
		"blocked",
	)
	storage.objects[blocked.ObjectURI] = blocked.Content
	h := canonicalArtifactTestServer()

	blockedContent := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/content", thread.ThreadID, blocked.Artifact.ID),
		nil,
	)
	blockedBody := string(blockedContent.Result().Body())
	require.Equal(t, http.StatusConflict, blockedContent.Code, blockedBody)
	require.Contains(t, blockedBody, `"code":"artifact_content_blocked"`)
	require.NotContains(t, blockedBody, "blocked body")
	require.NotContains(t, blockedBody, blocked.ObjectURI)

	review := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/scan_review", thread.ThreadID, blocked.Artifact.ID),
		canonicalArtifactJSONBody(`{"decision":"release","reason":"approved by security reviewer"}`),
	)
	reviewBody := string(review.Result().Body())
	require.Equal(t, http.StatusOK, review.Code, reviewBody)
	require.Contains(t, reviewBody, `"artifact_id":"`+strconv.FormatInt(blocked.Artifact.ID, 10)+`"`)
	require.Contains(t, reviewBody, `"decision":"release"`)
	require.Contains(t, reviewBody, `"scan_status":"clean"`)
	require.Contains(t, reviewBody, `"reviewed":true`)
	require.NotContains(t, reviewBody, "approved by security reviewer")

	failed := createCanonicalArtifactFixtureForRun(
		t,
		thread.ThreadID,
		blocked.Run,
		"failed.txt",
		"text/plain; charset=utf-8",
		[]byte("failed body"),
		"",
	)
	jobID := failCanonicalArtifactScanJob(t, failed.Artifact.ID, "scan-worker-a")

	jobs := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf(
			"/api/workbench/threads/%d/artifact_scan_jobs?artifact_id=%d&status=failed&scanner=default&limit=10&offset=0",
			thread.ThreadID,
			failed.Artifact.ID,
		),
		nil,
	)
	jobsBody := string(jobs.Result().Body())
	require.Equal(t, http.StatusOK, jobs.Code, jobsBody)
	require.Contains(t, jobsBody, `"total":1`)
	require.Contains(t, jobsBody, `"status":"failed"`)
	require.Contains(t, jobsBody, `"worker_ref":"`)
	require.NotContains(t, jobsBody, "scan-worker-a")
	require.NotContains(t, jobsBody, "scanner unavailable")
	require.NotContains(t, jobsBody, failed.ObjectURI)

	retry := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/artifact_scan_jobs/%d/retry", thread.ThreadID, jobID),
		nil,
	)
	retryBody := string(retry.Result().Body())
	require.Equal(t, http.StatusOK, retry.Code, retryBody)
	require.Contains(t, retryBody, `"retried":true`)
	require.Contains(t, retryBody, `"status":"pending"`)
	require.NotContains(t, retryBody, "manual retry requested")
}

func TestCanonicalThreadArtifactDeleteRestoreAndCrossThreadIsolation(t *testing.T) {
	installAgentThreadTestService(t)
	ownerThread := createCanonicalTestThread(t, 1001, "artifact owner", `{}`)
	otherThread := createCanonicalTestThread(t, 1001, "artifact other", `{}`)
	fixture := createCanonicalArtifactFixture(
		t,
		ownerThread.ThreadID,
		"restore.txt",
		"text/plain; charset=utf-8",
		[]byte("restore body"),
		"clean",
	)
	h := canonicalArtifactTestServer()

	crossThreadDelete := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodDelete,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d", otherThread.ThreadID, fixture.Artifact.ID),
		nil,
	)
	require.Equal(t, http.StatusNotFound, crossThreadDelete.Code, crossThreadDelete.Result().Body())
	require.Contains(t, string(crossThreadDelete.Result().Body()), `"code":"resource_not_found"`)

	deleted := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodDelete,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d", ownerThread.ThreadID, fixture.Artifact.ID),
		nil,
	)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Result().Body())
	require.Empty(t, deleted.Result().Body())

	hiddenFromActiveList := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts", ownerThread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusOK, hiddenFromActiveList.Code, hiddenFromActiveList.Result().Body())
	require.Contains(t, string(hiddenFromActiveList.Result().Body()), `"total":0`)

	restored := performCanonicalArtifactRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/restore", ownerThread.ThreadID, fixture.Artifact.ID),
		nil,
	)
	restoredBody := string(restored.Result().Body())
	require.Equal(t, http.StatusOK, restored.Code, restoredBody)
	require.Contains(t, restoredBody, `"restored":true`)
	require.Contains(t, restoredBody, `"artifact_id":"`+strconv.FormatInt(fixture.Artifact.ID, 10)+`"`)
	require.NotContains(t, restoredBody, fixture.ObjectURI)
	require.NotContains(t, restoredBody, "/mnt/user-data")
}

func TestCanonicalThreadArtifactDependenciesFailClosed(t *testing.T) {
	t.Run("artifact service", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "artifact dependency", `{}`)
		appagentthread.SVC.ArtifactSVC = nil
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/artifacts", thread.ThreadID),
			nil,
		)

		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		body := string(response.Result().Body())
		require.Contains(t, body, `"code":"dependency_unavailable"`)
		require.Contains(t, body, `"retryable":true`)
	})

	t.Run("object storage", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "artifact storage dependency", `{}`)
		fixture := createCanonicalArtifactFixture(
			t,
			thread.ThreadID,
			"storage.txt",
			"text/plain; charset=utf-8",
			[]byte("storage body"),
			"clean",
		)
		appagentthread.SVC.ArtifactObjectStorage = nil
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/content", thread.ThreadID, fixture.Artifact.ID),
			nil,
		)

		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"dependency_unavailable"`)
	})

	t.Run("signed url signer", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "artifact signer dependency", `{}`)
		fixture := createCanonicalArtifactFixture(
			t,
			thread.ThreadID,
			"signer.txt",
			"text/plain; charset=utf-8",
			[]byte("signer body"),
			"clean",
		)
		appagentthread.SVC.ArtifactObjectStorage = &canonicalArtifactObjectReader{
			objects: map[string][]byte{fixture.ObjectURI: fixture.Content},
		}
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/signed_url", thread.ThreadID, fixture.Artifact.ID),
			nil,
		)

		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"dependency_unavailable"`)
	})
}

func TestCanonicalThreadArtifactErrors(t *testing.T) {
	t.Run("malformed path id", func(t *testing.T) {
		installAgentThreadTestService(t)
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(t, h, http.MethodGet, "/api/workbench/threads/bad/artifacts", nil)

		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"invalid_path_parameter"`)
	})

	t.Run("wrong workspace", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "artifact denied", `{}`)
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/artifacts", thread.ThreadID),
			nil,
			ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
		)

		require.Equal(t, http.StatusNotFound, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"resource_not_found"`)
	})

	t.Run("artifact authorizer denial", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "artifact denied", `{}`)
		appagentthread.SVC.ArtifactAuthorizer = &recordingWorkbenchArtifactAuthorizer{
			err: appagentthread.ErrArtifactAccessDenied,
		}
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/artifacts", thread.ThreadID),
			nil,
		)

		require.Equal(t, http.StatusNotFound, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"resource_not_found"`)
	})

	t.Run("unsupported signed preview", func(t *testing.T) {
		installAgentThreadTestService(t)
		storage := &recordingWorkbenchArtifactStorage{objects: map[string][]byte{}}
		appagentthread.SVC.ArtifactObjectStorage = storage
		thread := createCanonicalTestThread(t, 1001, "artifact unsupported preview", `{}`)
		fixture := createCanonicalArtifactFixture(
			t,
			thread.ThreadID,
			"unsupported.bin",
			"text/plain; charset=utf-8",
			[]byte("\x00\x01\x02\x03\x04\x05\x06\x07"),
			"clean",
		)
		storage.objects[fixture.ObjectURI] = fixture.Content
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/artifacts/%d/signed_url?mode=preview", thread.ThreadID, fixture.Artifact.ID),
			nil,
		)

		require.Equal(t, http.StatusConflict, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"artifact_signed_url_not_supported"`)
	})

	t.Run("retry non failed scan job", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "artifact retry conflict", `{}`)
		createCanonicalArtifactFixture(t, thread.ThreadID, "pending.txt", "text/plain; charset=utf-8", []byte("pending"), "")
		jobs, err := appagentthread.SVC.ArtifactSVC.ClaimArtifactScanJobs(
			context.Background(),
			&domainservice.ClaimArtifactScanJobsRequest{
				Scanner:        "default",
				WorkerID:       "scan-worker-a",
				Limit:          1,
				LeaseTTLMillis: 60000,
			},
		)
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		h := canonicalArtifactTestServer()

		response := performCanonicalArtifactRequest(
			t,
			h,
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/artifact_scan_jobs/%d/retry", thread.ThreadID, jobs[0].ID),
			nil,
		)

		require.Equal(t, http.StatusConflict, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"artifact_scan_job_not_retryable"`)
	})
}

type canonicalArtifactFixture struct {
	ThreadID  int64
	Run       *appagentthread.RunSummary
	Artifact  *domainentity.AgentArtifact
	ObjectURI string
	Content   []byte
}

type canonicalArtifactObjectReader struct {
	objects map[string][]byte
}

type canonicalLegacyArtifactService struct {
	domainservice.ArtifactService
	artifact *domainentity.AgentArtifact
}

func (s *canonicalLegacyArtifactService) GetArtifact(
	_ context.Context,
	req *domainservice.GetArtifactRequest,
) (*domainentity.AgentArtifact, error) {
	if s == nil || s.artifact == nil || req == nil ||
		req.ThreadID != s.artifact.ThreadID || req.ArtifactID != s.artifact.ID {
		return nil, nil
	}
	cloned := *s.artifact
	return &cloned, nil
}

func (s *canonicalArtifactObjectReader) GetObject(
	_ context.Context,
	objectKey string,
) ([]byte, error) {
	return append([]byte(nil), s.objects[objectKey]...), nil
}

func canonicalArtifactTestServer() *server.Hertz {
	h := authenticatedAgentThreadTestServer()
	registerCanonicalArtifactRoutes(h)
	return h
}

func registerCanonicalArtifactRoutes(h *server.Hertz) {
	h.GET("/api/workbench/threads/:thread_id/artifacts", ListCanonicalThreadArtifacts)
	h.POST("/api/workbench/threads/:thread_id/artifacts/:artifact_id/copy_link", CopyCanonicalThreadArtifactLink)
	h.GET("/api/workbench/threads/:thread_id/artifacts/:artifact_id/content", GetCanonicalThreadArtifactContent)
	h.GET("/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url", GetCanonicalThreadArtifactSignedURL)
	h.DELETE("/api/workbench/threads/:thread_id/artifacts/:artifact_id", DeleteCanonicalThreadArtifact)
	h.POST("/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore", RestoreCanonicalThreadArtifact)
	h.POST("/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review", ReviewCanonicalThreadArtifactScan)
	h.GET("/api/workbench/threads/:thread_id/artifact_scan_jobs", ListCanonicalThreadArtifactScanJobs)
	h.POST("/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry", RetryCanonicalThreadArtifactScanJob)
}

func performCanonicalArtifactRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body *ut.Body,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := make([]ut.Header, 0, len(headers)+1)
	hasSpaceHeader := false
	for _, header := range headers {
		if strings.EqualFold(header.Key, canonicalSpaceIDHeader) {
			hasSpaceHeader = true
			break
		}
	}
	if !hasSpaceHeader {
		requestHeaders = append(requestHeaders, ut.Header{Key: canonicalSpaceIDHeader, Value: "1001"})
	}
	requestHeaders = append(requestHeaders, headers...)
	return ut.PerformRequest(h.Engine, method, path, body, requestHeaders...)
}

func canonicalArtifactJSONBody(body string) *ut.Body {
	return &ut.Body{Body: strings.NewReader(body), Len: len(body)}
}

func createCanonicalArtifactFixture(
	t *testing.T,
	threadID int64,
	title string,
	contentType string,
	content []byte,
	scanStatus string,
) *canonicalArtifactFixture {
	t.Helper()
	run := createCanonicalRunFixture(t, threadID, "artifact fixture")
	return createCanonicalArtifactFixtureForRun(t, threadID, run, title, contentType, content, scanStatus)
}

func createCanonicalArtifactFixtureForRun(
	t *testing.T,
	threadID int64,
	run *appagentthread.RunSummary,
	title string,
	contentType string,
	content []byte,
	scanStatus string,
) *canonicalArtifactFixture {
	t.Helper()
	require.NotNil(t, run)
	runIDText := strconv.FormatInt(run.RunID, 10)
	hashChar := "a"
	for _, candidate := range []string{"b", "c", "d", "e", "f"} {
		if strings.Contains(title, candidate) {
			hashChar = candidate
			break
		}
	}
	fileName := strings.Repeat(hashChar, 64) + ".txt"
	objectURI := "agent-runtime/1001/" + strconv.FormatInt(threadID, 10) + "/runs/" + runIDText + "/tool-results/trunc/" + fileName
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            run.RunID,
			FileName:         fileName,
			OriginalFileName: "secret-" + title,
			FileKind:         domainentity.AgentFileKindWorkspace,
			VirtualPath:      "/mnt/user-data/workspace/.coze/tool-results/runs/" + runIDText + "/trunc/" + fileName,
			ObjectURI:        objectURI,
			ContentType:      contentType,
			SizeBytes:        int64(len(content)),
			Digest:           strings.Repeat("b", 64),
			Metadata:         `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1001,
			ThreadID:     threadID,
			RunID:        run.RunID,
			FileID:       fileResp.ID,
			Title:        title,
			ArtifactType: "report",
			Metadata:     `{"source":"test","provider_body":{"secret":"hidden"}}`,
		},
	)
	require.NoError(t, err)
	if strings.TrimSpace(scanStatus) != "" {
		digest := fmt.Sprintf("%x", sha256.Sum256(content))
		_, err = appagentthread.SVC.RecordArtifactScanResult(
			context.Background(),
			&appagentthread.RecordArtifactScanResultRequest{
				ThreadID:            threadID,
				ArtifactID:          artifact.ID,
				ScanStatus:          scanStatus,
				Scanner:             "test",
				Reason:              "signature",
				DetectedContentType: http.DetectContentType(content),
				ScannedSizeBytes:    int64(len(content)),
				ContentHash:         digest,
				ScannedAt:           1,
			},
		)
		require.NoError(t, err)
	}
	return &canonicalArtifactFixture{
		ThreadID:  threadID,
		Run:       run,
		Artifact:  artifact,
		ObjectURI: objectURI,
		Content:   append([]byte(nil), content...),
	}
}

func failCanonicalArtifactScanJob(t *testing.T, artifactID int64, workerID string) int64 {
	t.Helper()
	jobs, err := appagentthread.SVC.ArtifactSVC.ClaimArtifactScanJobs(
		context.Background(),
		&domainservice.ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       workerID,
			Limit:          20,
			LeaseTTLMillis: 60000,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, jobs)
	var target *domainentity.ArtifactScanJob
	for _, job := range jobs {
		if job != nil && job.ArtifactID == artifactID {
			target = job
			break
		}
	}
	require.NotNil(t, target)
	_, ok, err := appagentthread.SVC.ArtifactSVC.FailArtifactScanJob(
		context.Background(),
		&domainservice.FailArtifactScanJobRequest{
			JobID:     target.ID,
			WorkerID:  workerID,
			ErrorText: "scanner unavailable",
		},
	)
	require.NoError(t, err)
	require.True(t, ok)
	return target.ID
}
