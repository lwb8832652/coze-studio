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
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appworkspace "github.com/coze-dev/coze-studio/backend/application/workspace"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

type appDevProviderControlFake struct {
	startRequests   []appdev.ProviderRuntimeMutationRequest
	statusRequests  []appdev.ProviderRuntimeStatusRequest
	stopRequests    []appdev.ProviderRuntimeMutationRequest
	restartRequests []appdev.ProviderRuntimeMutationRequest
	buildRequests   []appdev.ProviderBuildOperationRequest
	pollRequests    []appdev.ProviderBuildOperationRequest
	recoverRequests []appdev.ProviderBuildRecoverRequest
	restoreRequests []appdev.ProviderSnapshotRestoreRequest
	releaseRequests []appdev.ProviderReleaseRequest
	runtimeResult   *appdev.ProviderRuntimeAPIProjection
	buildResult     *appdev.ProviderBuildAPIProjection
	restoreResult   *appdev.ProviderRuntimeAPIProjection
	releaseResult   *appDevHTTPRelease
	err             error
}

func (fake *appDevProviderControlFake) StartRuntime(_ context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	fake.startRequests = append(fake.startRequests, request)
	return fake.runtimeResult, fake.err
}

func (fake *appDevProviderControlFake) RuntimeStatus(_ context.Context, request appdev.ProviderRuntimeStatusRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	fake.statusRequests = append(fake.statusRequests, request)
	return fake.runtimeResult, fake.err
}

func (fake *appDevProviderControlFake) StopRuntime(_ context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	fake.stopRequests = append(fake.stopRequests, request)
	return fake.runtimeResult, fake.err
}

func (fake *appDevProviderControlFake) RestartRuntime(_ context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	fake.restartRequests = append(fake.restartRequests, request)
	return fake.runtimeResult, fake.err
}

func (fake *appDevProviderControlFake) BeginBuild(_ context.Context, request appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error) {
	fake.buildRequests = append(fake.buildRequests, request)
	return fake.buildResult, fake.err
}

func (fake *appDevProviderControlFake) PollBuild(_ context.Context, request appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error) {
	fake.pollRequests = append(fake.pollRequests, request)
	return fake.buildResult, fake.err
}

func (fake *appDevProviderControlFake) RecoverBuild(_ context.Context, request appdev.ProviderBuildRecoverRequest) (*appdev.ProviderBuildAPIProjection, error) {
	fake.recoverRequests = append(fake.recoverRequests, request)
	return fake.buildResult, fake.err
}

func (fake *appDevProviderControlFake) RestoreSnapshot(_ context.Context, request appdev.ProviderSnapshotRestoreRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	fake.restoreRequests = append(fake.restoreRequests, request)
	if fake.restoreResult != nil {
		return fake.restoreResult, fake.err
	}
	return fake.runtimeResult, fake.err
}

func (fake *appDevProviderControlFake) OpenRelease(_ context.Context, request appdev.ProviderReleaseRequest) (*appDevHTTPRelease, error) {
	fake.releaseRequests = append(fake.releaseRequests, request)
	return fake.releaseResult, fake.err
}

type appDevPreviewProjectorFake struct {
	result string
	err    error
	routes []string
}

func (fake *appDevPreviewProjectorFake) ProjectAppDevPreviewURL(_ context.Context, _, _, relativeRoute string) (string, error) {
	fake.routes = append(fake.routes, relativeRoute)
	return fake.result, fake.err
}

type appDevTrackingReadCloser struct {
	reader io.Reader
	closed bool
}

func (stream *appDevTrackingReadCloser) Read(buffer []byte) (int, error) {
	return stream.reader.Read(buffer)
}

func (stream *appDevTrackingReadCloser) Close() error {
	stream.closed = true
	return nil
}

func TestAppDevProviderRuntimeRequiresHeaderAndRejectsQueryOperation(t *testing.T) {
	control := &appDevProviderControlFake{runtimeResult: &appdev.ProviderRuntimeAPIProjection{Generation: 4, State: appdev.ProviderRuntimeStateRunning}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	missing := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/start", nil)
	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.Empty(t, control.startRequests)

	invalid := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/start", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "short"})
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Empty(t, control.startRequests)

	query := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/start?operation_id=hidden-operation", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-operation-001"})
	require.Equal(t, http.StatusBadRequest, query.Code)
	require.Empty(t, control.startRequests)
	statusQuery := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/runtime/status?idempotency_key=hidden-operation", nil)
	require.Equal(t, http.StatusBadRequest, statusQuery.Code)
	require.Empty(t, control.statusRequests)

	valid := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/start", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-operation-001"})
	require.Equal(t, http.StatusOK, valid.Code)
	retry := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/start", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-operation-001"})
	require.Equal(t, http.StatusOK, retry.Code)
	require.Len(t, control.startRequests, 2)
	require.Equal(t, "stable-operation-001", control.startRequests[0].OperationID)
	require.Equal(t, control.startRequests[0].OperationID, control.startRequests[1].OperationID)
	require.Equal(t, "1001", control.startRequests[0].SpaceID)
	require.Equal(t, "project-safe", control.startRequests[0].ProjectID)
}

func TestAppDevProviderRuntimeStoppedProjectionIsSafeAcrossStatusStopAndRestart(t *testing.T) {
	control := &appDevProviderControlFake{runtimeResult: &appdev.ProviderRuntimeAPIProjection{
		Generation: 0, State: appdev.ProviderRuntimeStateStopped, CanStart: true,
	}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	status := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/runtime/status", nil)
	stop := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/stop", nil,
		ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stopped-stop-operation"})
	restart := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/restart", nil,
		ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stopped-restart-operation"})

	assertStopped := func(name string, code int, body []byte) {
		require.Equal(t, http.StatusOK, code, name)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload), name)
		require.Equal(t, string(appdev.ProviderRuntimeStateStopped), payload["state"], name)
		require.Equal(t, true, payload["can_start"], name)
		require.NotContains(t, strings.ToLower(string(body)), "unavailable", name)
	}
	assertStopped("status", status.Code, status.Result().Body())
	assertStopped("stop", stop.Code, stop.Result().Body())
	assertStopped("restart", restart.Code, restart.Result().Body())
}

func TestAppDevProviderRuntimePermissionAndSafeProjection(t *testing.T) {
	t.Run("safe projection", func(t *testing.T) {
		control := &appDevProviderControlFake{runtimeResult: &appdev.ProviderRuntimeAPIProjection{
			Generation: 9, State: appdev.ProviderRuntimeStateRunning, CanStart: false,
			Recovering: true, RelativePreview: "/preview/project-safe", SafeMessage: "runtime is recovering",
		}}
		projector := &appDevPreviewProjectorFake{result: "https://preview.example.test/preview/project-safe"}
		handler := newAppDevProviderHTTPHandlerForTest(t, control, projector, true)
		response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/runtime/status", nil)
		require.Equal(t, http.StatusOK, response.Code)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(response.Result().Body(), &payload))
		require.ElementsMatch(t, []string{"generation", "state", "can_start", "recovering", "stopping", "preview_url", "safe_message"}, mapKeys(payload))
		require.Equal(t, "https://preview.example.test/preview/project-safe", payload["preview_url"])
		require.NotContains(t, string(response.Result().Body()), "provider_execution")
		require.Equal(t, []string{"/preview/project-safe"}, projector.routes)
	})

	t.Run("manager permission", func(t *testing.T) {
		deniedControl := &appDevProviderControlFake{runtimeResult: &appdev.ProviderRuntimeAPIProjection{State: appdev.ProviderRuntimeStateRunning}}
		denied := newAppDevProviderHTTPHandlerForTest(t, deniedControl, &appDevPreviewProjectorFake{}, false)
		forbidden := ut.PerformRequest(denied.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/stop", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-operation-002"})
		require.Equal(t, http.StatusForbidden, forbidden.Code)
		require.Empty(t, deniedControl.stopRequests)
		require.NotContains(t, string(forbidden.Result().Body()), "workspace owner")
	})
}

func TestAppDevProviderRuntimeRejectsPreviewInjection(t *testing.T) {
	control := &appDevProviderControlFake{runtimeResult: &appdev.ProviderRuntimeAPIProjection{
		Generation: 2, State: appdev.ProviderRuntimeStateRunning, RelativePreview: "//evil.example/steal?token=secret",
	}}
	projector := &appDevPreviewProjectorFake{result: "https://evil.example/steal"}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, projector, true)

	response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/runtime/status", nil)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Empty(t, projector.routes)
	require.NotContains(t, string(response.Result().Body()), "evil.example")
	require.NotContains(t, string(response.Result().Body()), "secret")
}

func TestAppDevProviderBuildIsAsyncAndStatusUsesExactPollOperation(t *testing.T) {
	updatedAt := time.Date(2026, 7, 17, 9, 30, 0, 0, time.UTC)
	control := &appDevProviderControlFake{buildResult: &appdev.ProviderBuildAPIProjection{
		Generation: 11, State: appdev.ProviderBuildStateBuilding, ReleaseAvailable: false, Size: 8192, UpdatedAt: updatedAt, SafeMessage: "build queued",
	}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	begin := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/build", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-build-operation"})
	require.Equal(t, http.StatusAccepted, begin.Code)
	require.Len(t, control.buildRequests, 1)
	var beginPayload map[string]any
	require.NoError(t, json.Unmarshal(begin.Result().Body(), &beginPayload))
	require.ElementsMatch(t, []string{"generation", "state", "release_available", "size", "updated_at", "stale", "safe_message"}, mapKeys(beginPayload))
	require.Equal(t, float64(11), beginPayload["generation"])
	require.NotContains(t, string(begin.Result().Body()), "operation")

	missing := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil)
	require.Equal(t, http.StatusBadRequest, missing.Code)
	queryOnly := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build?operation_id=stable-build-operation", nil)
	require.Equal(t, http.StatusBadRequest, queryOnly.Code)
	invalid := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "short"})
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	bodyOperation := `{"operation_id":"body-build-operation"}`
	bodyOnly := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build", &ut.Body{Body: strings.NewReader(bodyOperation), Len: len(bodyOperation)}, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-build-operation"})
	require.Equal(t, http.StatusBadRequest, bodyOnly.Code)
	require.Empty(t, control.pollRequests)
	require.Empty(t, control.recoverRequests)

	poll := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-build-operation"})
	require.Equal(t, http.StatusAccepted, poll.Code)
	require.Len(t, control.pollRequests, 1)
	require.Equal(t, "stable-build-operation", control.pollRequests[0].OperationID)
	require.Equal(t, "1001", control.pollRequests[0].SpaceID)
	require.Equal(t, "project-safe", control.pollRequests[0].ProjectID)
	var pollPayload map[string]any
	require.NoError(t, json.Unmarshal(poll.Result().Body(), &pollPayload))
	require.ElementsMatch(t, []string{"generation", "state", "release_available", "size", "updated_at", "stale", "safe_message"}, mapKeys(pollPayload))
	require.Equal(t, float64(11), pollPayload["generation"])
	require.NotContains(t, string(poll.Result().Body()), "operation")

	control.err = appdev.ErrProviderControlConflict
	wrong := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "different-build-operation"})
	require.Equal(t, http.StatusConflict, wrong.Code)
	require.Len(t, control.pollRequests, 2)
	require.NotContains(t, string(wrong.Result().Body()), "stable-build-operation")
}

func TestAppDevProviderKeepAliveAndLogsUseStatusOnly(t *testing.T) {
	control := &appDevProviderControlFake{runtimeResult: &appdev.ProviderRuntimeAPIProjection{Generation: 3, State: appdev.ProviderRuntimeStateRunning}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	keepAlive := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/runtime/keep-alive", nil)
	require.Equal(t, http.StatusOK, keepAlive.Code)
	logs := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/runtime/logs", nil)
	require.Equal(t, http.StatusOK, logs.Code)
	require.Len(t, control.statusRequests, 2)
	require.Contains(t, string(logs.Result().Body()), `"logs":[]`)
}

func TestAppDevProviderReleaseStreamsSafeHeadersAndCloses(t *testing.T) {
	stream := &appDevTrackingReadCloser{reader: strings.NewReader("release-bytes")}
	control := &appDevProviderControlFake{releaseResult: &appDevHTTPRelease{
		Body: stream, Size: int64(len("release-bytes")), Stale: true, UpdatedAt: time.Now().UTC(),
	}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/release", nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "release-bytes", string(response.Result().Body()))
	require.Equal(t, "application/zip", string(response.Result().Header.Peek("Content-Type")))
	require.Equal(t, strconv.Itoa(len("release-bytes")), string(response.Result().Header.Peek("Content-Length")))
	require.Equal(t, "private, no-store", string(response.Result().Header.Peek("Cache-Control")))
	require.Equal(t, "nosniff", string(response.Result().Header.Peek("X-Content-Type-Options")))
	require.Equal(t, "true", string(response.Result().Header.Peek("X-AppDev-Release-Stale")))
	require.Equal(t, `attachment; filename="appdev-release.zip"`, string(response.Result().Header.Peek("Content-Disposition")))
	require.True(t, stream.closed)
	require.Len(t, control.releaseRequests, 1)
	require.NotContains(t, string(response.Result().Body()), "object-key")
}

func TestAppDevProviderReleaseFailureClosesAndDoesNotLeakBackendError(t *testing.T) {
	stream := &appDevTrackingReadCloser{reader: strings.NewReader("must-not-stream")}
	control := &appDevProviderControlFake{
		releaseResult: &appDevHTTPRelease{Body: stream, Size: 15},
		err:           errors.New("storage object-key=sensitive-internal-uri"),
	}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/release", nil)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.True(t, stream.closed)
	require.NotContains(t, string(response.Result().Body()), "object-key")
	require.NotContains(t, string(response.Result().Body()), "sensitive")
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &envelope))
	require.ElementsMatch(t, []string{"code", "message"}, mapKeys(envelope))
}

func TestAppDevProviderReleaseRejectsNonMemberBeforeOpeningArtifact(t *testing.T) {
	control := &appDevProviderControlFake{releaseResult: &appDevHTTPRelease{Body: &appDevTrackingReadCloser{reader: strings.NewReader("secret")}, Size: 6}}
	handler := newAppDevProviderHTTPHandlerForAccessTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, false, false)
	response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/release", nil)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Empty(t, control.releaseRequests)
	require.NotContains(t, string(response.Result().Body()), "secret")
}

func TestAppDevProviderReleaseNotFoundUsesSafe404(t *testing.T) {
	control := &appDevProviderControlFake{err: appdev.ErrProviderControlNotFound}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/release", nil)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, string(response.Result().Body()), `"code":"not_found"`)
}

func TestAppDevProviderSnapshotRestoreStoppingIsAccepted(t *testing.T) {
	control := &appDevProviderControlFake{restoreResult: &appdev.ProviderRuntimeAPIProjection{
		Generation: 13, State: appdev.ProviderRuntimeStateCleanupPending, Stopping: true,
	}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)

	response := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/snapshots/snapshot-safe/restore", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "stable-restore-operation"})
	require.Equal(t, http.StatusAccepted, response.Code)
	require.Len(t, control.restoreRequests, 1)
	require.Equal(t, "stable-restore-operation", control.restoreRequests[0].OperationID)
}

func TestAppDevProviderPreviewProjectorRequiresTrustedHTTPSBase(t *testing.T) {
	_, err := newAppDevProviderHTTPHandler(nil, &appDevPreviewProjectorFake{})
	require.Error(t, err)
	_, err = newAppDevProviderHTTPHandler(&appDevProviderControlFake{}, nil)
	require.Error(t, err)
	_, err = NewAppDevPreviewURLProjector("http://preview.example.test")
	require.Error(t, err)
	_, err = NewAppDevPreviewURLProjector("https://preview.example.test/%2e%2e/escape")
	require.Error(t, err)
	projector, err := NewAppDevPreviewURLProjector("https://preview.example.test")
	require.NoError(t, err)
	projected, err := projector.ProjectAppDevPreviewURL(context.Background(), "1001", "project-safe", "/preview/project-safe")
	require.NoError(t, err)
	require.Equal(t, "https://preview.example.test/preview/project-safe", projected)
	for _, route := range []string{"//evil.example/x", "https://evil.example/x", "/safe?next=evil", "/%2e%2e/secret", "/safe\\evil"} {
		_, err = projector.ProjectAppDevPreviewURL(context.Background(), "1001", "project-safe", route)
		require.Error(t, err, route)
	}
}

func TestAppDevProviderHTTPDependencyOverrideRestoresFailClosedDefault(t *testing.T) {
	control := &appDevProviderControlFake{buildResult: &appdev.ProviderBuildAPIProjection{Generation: 17, State: appdev.ProviderBuildStateBuilding}}
	restore, err := setAppDevProviderHTTPDependenciesForTest(control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"})
	require.NoError(t, err)
	t.Cleanup(restore)

	userID := int64(42)
	uidMock := mockey.Mock(ctxutil.GetUIDFromCtx).Return(&userID).Build()
	accessMock := mockey.Mock((*appworkspace.ApplicationService).CheckWorkspaceAppDevAccess).Return(nil).Build()
	t.Cleanup(func() { uidMock.UnPatch() })
	t.Cleanup(func() { accessMock.UnPatch() })

	h := server.Default()
	project := h.Group("/api/app-dev/spaces/:space_id/projects/:project_id")
	project.GET("/build", GetAppDevBuildStatus)
	response := ut.PerformRequest(h.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "dependency-build-operation"})
	require.Equal(t, http.StatusAccepted, response.Code)
	require.Len(t, control.pollRequests, 1)

	restore()
	restore()
	failClosed := ut.PerformRequest(h.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil, ut.Header{Key: appDevIdempotencyKeyHeader, Value: "dependency-build-operation"})
	require.Equal(t, http.StatusServiceUnavailable, failClosed.Code)
	require.Len(t, control.pollRequests, 1)
}

const appDevProviderTestPath = "/api/app-dev/spaces/1001/projects/project-safe"

func newAppDevProviderHTTPHandlerForTest(t *testing.T, control appDevProviderHTTPControl, projector AppDevPreviewURLProjector, managerAllowed bool) *server.Hertz {
	return newAppDevProviderHTTPHandlerForAccessTest(t, control, projector, true, managerAllowed)
}

func newAppDevProviderHTTPHandlerForAccessTest(t *testing.T, control appDevProviderHTTPControl, projector AppDevPreviewURLProjector, memberAllowed, managerAllowed bool) *server.Hertz {
	t.Helper()
	userID := int64(42)
	uidMock := mockey.Mock(ctxutil.GetUIDFromCtx).Return(&userID).Build()
	accessMock := mockey.Mock((*appworkspace.ApplicationService).CheckWorkspaceAppDevAccess).To(func(_ *appworkspace.ApplicationService, _ context.Context, request *appworkspace.CheckWorkspaceAppDevAccessRequest) error {
		if !memberAllowed {
			return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace membership secret"))
		}
		if request.RequireManager && !managerAllowed {
			return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace owner capability secret"))
		}
		return nil
	}).Build()
	t.Cleanup(func() { uidMock.UnPatch() })
	t.Cleanup(func() { accessMock.UnPatch() })

	handler, err := newAppDevProviderHTTPHandler(control, projector)
	require.NoError(t, err)
	h := server.Default()
	project := h.Group("/api/app-dev/spaces/:space_id/projects/:project_id")
	project.POST("/build", handler.Build)
	project.GET("/build", handler.BuildStatus)
	project.GET("/release", handler.Release)
	project.POST("/snapshots/:snapshot_id/restore", handler.RestoreSnapshot)
	project.POST("/runtime/start", handler.StartRuntime)
	project.GET("/runtime/status", handler.RuntimeStatus)
	project.POST("/runtime/keep-alive", handler.KeepAlive)
	project.POST("/runtime/restart", handler.RestartRuntime)
	project.POST("/runtime/stop", handler.StopRuntime)
	project.GET("/runtime/logs", handler.RuntimeLogs)
	return h
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
