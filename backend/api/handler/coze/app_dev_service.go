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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/coze-dev/coze-studio/backend/api/internal/httputil"
	appdevapi "github.com/coze-dev/coze-studio/backend/api/model/appdev"
	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appworkspace "github.com/coze-dev/coze-studio/backend/application/workspace"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

var appDevApplicationSVC = appdev.SVC

const (
	appDevIdempotencyKeyHeader       = "Idempotency-Key"
	appDevProviderMinOperationLength = 8
	appDevProviderMaxOperationLength = 128
	appDevProviderMaxSafeMessage     = 256
)

var (
	errAppDevProviderHTTPUnavailable = errors.New("appdev provider http control is unavailable")
	appDevPreviewProjectorMu         sync.RWMutex
	appDevConfiguredProjector        AppDevPreviewURLProjector
	appDevProviderHTTPHandlerMu      sync.RWMutex
	appDevProviderHTTPTestOverrideMu sync.Mutex
	defaultAppDevProviderHTTPHandler = &appDevProviderHTTPHandler{
		projectorSource: currentAppDevPreviewURLProjector,
	}
)

// AppDevPreviewURLProjector is a mandatory, startup-wired trust boundary. It
// receives only a validated relative route and must return a trusted HTTPS URL.
type AppDevPreviewURLProjector interface {
	ProjectAppDevPreviewURL(context.Context, string, string, string) (string, error)
}

type appDevPreviewURLProjector struct {
	base url.URL
}

type appDevLoopbackPreviewURLProjector struct {
	base url.URL
}

// NewAppDevPreviewURLProjector constructs the strict production projector.
// Debug HTTP exceptions, if ever needed, belong in Task9.5 environment wiring.
func NewAppDevPreviewURLProjector(rawBase string) (AppDevPreviewURLProjector, error) {
	if rawBase != strings.TrimSpace(rawBase) || rawBase == "" {
		return nil, errAppDevProviderHTTPUnavailable
	}
	parsed, err := url.Parse(rawBase)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errAppDevProviderHTTPUnavailable
	}
	if parsed.Path != "" {
		if _, ok := normalizeAppDevPreviewRoute(parsed.Path); !ok {
			return nil, errAppDevProviderHTTPUnavailable
		}
	}
	return &appDevPreviewURLProjector{base: *parsed}, nil
}

func NewAppDevLoopbackPreviewURLProjector(rawBase string) (AppDevPreviewURLProjector, error) {
	if rawBase != strings.TrimSpace(rawBase) || rawBase == "" {
		return nil, errAppDevProviderHTTPUnavailable
	}
	parsed, err := url.Parse(rawBase)
	ip := net.ParseIP(parsed.Hostname())
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil ||
		parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		ip == nil || !ip.IsLoopback() {
		return nil, errAppDevProviderHTTPUnavailable
	}
	return &appDevLoopbackPreviewURLProjector{base: *parsed}, nil
}

func (projector *appDevPreviewURLProjector) ProjectAppDevPreviewURL(_ context.Context, _, _ string, relativeRoute string) (string, error) {
	decoded, ok := normalizeAppDevPreviewRoute(relativeRoute)
	if projector == nil || !ok {
		return "", errAppDevProviderHTTPUnavailable
	}
	projected := projector.base
	projected.Path = strings.TrimSuffix(projected.Path, "/") + decoded
	projected.RawPath = ""
	projected.RawQuery = ""
	projected.Fragment = ""
	result := projected.String()
	if !validProjectedAppDevPreviewURL(result) {
		return "", errAppDevProviderHTTPUnavailable
	}
	return result, nil
}

func (projector *appDevLoopbackPreviewURLProjector) ProjectAppDevPreviewURL(_ context.Context, _, _ string, relativeRoute string) (string, error) {
	decoded, ok := normalizeAppDevPreviewRoute(relativeRoute)
	if projector == nil || !ok {
		return "", errAppDevProviderHTTPUnavailable
	}
	projected := projector.base
	projected.Path = strings.TrimSuffix(projected.Path, "/") + decoded
	projected.RawPath, projected.RawQuery, projected.Fragment = "", "", ""
	return projected.String(), nil
}

// SetAppDevPreviewURLProjector is a startup-only wiring seam. Until Task9.5
// installs it, every provider AppDev endpoint fails closed after authorization.
func SetAppDevPreviewURLProjector(projector AppDevPreviewURLProjector) error {
	if projector == nil {
		return errAppDevProviderHTTPUnavailable
	}
	appDevPreviewProjectorMu.Lock()
	appDevConfiguredProjector = projector
	appDevPreviewProjectorMu.Unlock()
	return nil
}

func currentAppDevPreviewURLProjector() AppDevPreviewURLProjector {
	appDevPreviewProjectorMu.RLock()
	configured := appDevConfiguredProjector
	appDevPreviewProjectorMu.RUnlock()
	if configured != nil {
		return configured
	}
	dependencies := appdev.CurrentProviderHTTPDependencies()
	if dependencies == nil {
		return nil
	}
	return dependencies.PreviewURLProjector()
}

type appDevProviderHTTPControl interface {
	StartRuntime(context.Context, appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error)
	RuntimeStatus(context.Context, appdev.ProviderRuntimeStatusRequest) (*appdev.ProviderRuntimeAPIProjection, error)
	StopRuntime(context.Context, appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error)
	RestartRuntime(context.Context, appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error)
	BeginBuild(context.Context, appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error)
	PollBuild(context.Context, appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error)
	RestoreSnapshot(context.Context, appdev.ProviderSnapshotRestoreRequest) (*appdev.ProviderRuntimeAPIProjection, error)
	OpenRelease(context.Context, appdev.ProviderReleaseRequest) (*appDevHTTPRelease, error)
}

type appDevProviderServiceControl struct{}

func (appDevProviderServiceControl) facade() (*appdev.ProviderAPIFacade, error) {
	if appDevApplicationSVC == nil {
		return nil, appdev.ErrProviderControlUnavailable
	}
	return appDevApplicationSVC.ProviderAPI()
}

func (control appDevProviderServiceControl) StartRuntime(ctx context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.StartRuntime(ctx, request)
}

func (control appDevProviderServiceControl) RuntimeStatus(ctx context.Context, request appdev.ProviderRuntimeStatusRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.RuntimeStatus(ctx, request)
}

func (control appDevProviderServiceControl) StopRuntime(ctx context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.StopRuntime(ctx, request)
}

func (control appDevProviderServiceControl) RestartRuntime(ctx context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.RestartRuntime(ctx, request)
}

func (control appDevProviderServiceControl) BeginBuild(ctx context.Context, request appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.BeginBuild(ctx, request)
}

func (control appDevProviderServiceControl) PollBuild(ctx context.Context, request appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.PollBuild(ctx, request)
}

func (control appDevProviderServiceControl) RestoreSnapshot(ctx context.Context, request appdev.ProviderSnapshotRestoreRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	return facade.RestoreSnapshot(ctx, request)
}

func (control appDevProviderServiceControl) OpenRelease(ctx context.Context, request appdev.ProviderReleaseRequest) (*appDevHTTPRelease, error) {
	facade, err := control.facade()
	if err != nil {
		return nil, err
	}
	release, err := facade.OpenRelease(ctx, request)
	if err != nil || release == nil {
		return nil, err
	}
	return &appDevHTTPRelease{Body: release, Size: release.Size, Stale: release.Stale, UpdatedAt: release.UpdatedAt}, nil
}

type appDevHTTPRelease struct {
	Body      io.ReadCloser
	Size      int64
	Stale     bool
	UpdatedAt time.Time
}

type appDevProviderHTTPHandler struct {
	control         appDevProviderHTTPControl
	projectorSource func() AppDevPreviewURLProjector
}

func newAppDevProviderHTTPHandler(control appDevProviderHTTPControl, projector AppDevPreviewURLProjector) (*appDevProviderHTTPHandler, error) {
	if control == nil || projector == nil {
		return nil, errAppDevProviderHTTPUnavailable
	}
	return &appDevProviderHTTPHandler{control: control, projectorSource: func() AppDevPreviewURLProjector { return projector }}, nil
}

func (handler *appDevProviderHTTPHandler) authorize(ctx context.Context, c *app.RequestContext, requireManager bool) (*appDevIdentity, AppDevPreviewURLProjector, bool) {
	identity, ok := getAppDevProviderIdentity(ctx, c, requireManager)
	if !ok {
		return nil, nil, false
	}
	if appDevProviderOperationInQuery(c) {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return nil, nil, false
	}
	if handler == nil || handler.control == nil || handler.projectorSource == nil {
		appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
		return nil, nil, false
	}
	projector := handler.projectorSource()
	if projector == nil {
		appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
		return nil, nil, false
	}
	return identity, projector, true
}

type appDevProviderErrorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (handler *appDevProviderHTTPHandler) StartRuntime(ctx context.Context, c *app.RequestContext) {
	handler.runtimeMutation(ctx, c, func(control appDevProviderHTTPControl, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
		return control.StartRuntime(ctx, request)
	})
}

func (handler *appDevProviderHTTPHandler) StopRuntime(ctx context.Context, c *app.RequestContext) {
	handler.runtimeMutation(ctx, c, func(control appDevProviderHTTPControl, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
		return control.StopRuntime(ctx, request)
	})
}

func (handler *appDevProviderHTTPHandler) RestartRuntime(ctx context.Context, c *app.RequestContext) {
	handler.runtimeMutation(ctx, c, func(control appDevProviderHTTPControl, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
		return control.RestartRuntime(ctx, request)
	})
}

func (handler *appDevProviderHTTPHandler) runtimeMutation(ctx context.Context, c *app.RequestContext, invoke func(appDevProviderHTTPControl, appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error)) {
	identity, projector, ok := handler.authorize(ctx, c, true)
	if !ok {
		return
	}
	operationID, ok := appDevProviderOperationID(c)
	if !ok {
		return
	}
	projection, err := invoke(handler.control, appdev.ProviderRuntimeMutationRequest{
		SpaceID: identity.spaceID, ProjectID: identity.projectID, ActorUserID: identity.userID, OperationID: operationID, ActorID: strconv.FormatInt(identity.userID, 10),
	})
	if err != nil {
		appDevProviderHTTPError(c, err)
		return
	}
	status := consts.StatusOK
	if projection != nil && projection.Stopping {
		status = consts.StatusAccepted
	}
	handler.writeRuntimeProjection(ctx, c, identity, projector, projection, status)
}

func (handler *appDevProviderHTTPHandler) RuntimeStatus(ctx context.Context, c *app.RequestContext) {
	identity, projector, ok := handler.authorize(ctx, c, false)
	if !ok {
		return
	}
	projection, err := handler.control.RuntimeStatus(ctx, appdev.ProviderRuntimeStatusRequest{
		SpaceID: identity.spaceID, ProjectID: identity.projectID, ActorID: strconv.FormatInt(identity.userID, 10),
	})
	if err != nil {
		appDevProviderHTTPError(c, err)
		return
	}
	handler.writeRuntimeProjection(ctx, c, identity, projector, projection, consts.StatusOK)
}

func (handler *appDevProviderHTTPHandler) KeepAlive(ctx context.Context, c *app.RequestContext) {
	// Browser activity is not an owner capability. This endpoint is now only a
	// safe status refresh and never reaches the legacy RuntimeManager.
	handler.RuntimeStatus(ctx, c)
}

func (handler *appDevProviderHTTPHandler) RuntimeLogs(ctx context.Context, c *app.RequestContext) {
	identity, _, ok := handler.authorize(ctx, c, false)
	if !ok {
		return
	}
	projection, err := handler.control.RuntimeStatus(ctx, appdev.ProviderRuntimeStatusRequest{
		SpaceID: identity.spaceID, ProjectID: identity.projectID, ActorID: strconv.FormatInt(identity.userID, 10),
	})
	if err != nil || projection == nil || !validAppDevRuntimeState(projection.State) {
		appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
		return
	}
	c.JSON(consts.StatusOK, appdevapi.AppDevProviderLogsResponse{
		State: string(projection.State), Logs: []string{}, SafeMessage: appDevOptionalString(safeAppDevProviderMessage(projection.SafeMessage)),
	})
}

func (handler *appDevProviderHTTPHandler) Build(ctx context.Context, c *app.RequestContext) {
	identity, _, ok := handler.authorize(ctx, c, true)
	if !ok {
		return
	}
	operationID, ok := appDevProviderOperationID(c)
	if !ok {
		return
	}
	projection, err := handler.control.BeginBuild(ctx, appdev.ProviderBuildOperationRequest{SpaceID: identity.spaceID, ProjectID: identity.projectID, OperationID: operationID})
	if err != nil {
		appDevProviderHTTPError(c, err)
		return
	}
	status := consts.StatusOK
	status = appDevProviderBuildHTTPStatus(projection)
	handler.writeBuildProjection(c, projection, status)
}

func (handler *appDevProviderHTTPHandler) BuildStatus(ctx context.Context, c *app.RequestContext) {
	identity, _, ok := handler.authorize(ctx, c, false)
	if !ok {
		return
	}
	if len(strings.TrimSpace(string(c.Request.Body()))) != 0 {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return
	}
	operationID, ok := appDevProviderOperationID(c)
	if !ok {
		return
	}
	projection, err := handler.control.PollBuild(ctx, appdev.ProviderBuildOperationRequest{SpaceID: identity.spaceID, ProjectID: identity.projectID, OperationID: operationID})
	if err != nil {
		appDevProviderHTTPError(c, err)
		return
	}
	handler.writeBuildProjection(c, projection, appDevProviderBuildHTTPStatus(projection))
}

func (handler *appDevProviderHTTPHandler) Release(ctx context.Context, c *app.RequestContext) {
	identity, _, ok := handler.authorize(ctx, c, false)
	if !ok {
		return
	}
	release, err := handler.control.OpenRelease(ctx, appdev.ProviderReleaseRequest{SpaceID: identity.spaceID, ProjectID: identity.projectID})
	if err != nil || release == nil || release.Body == nil || release.Size <= 0 || uint64(release.Size) > uint64(^uint(0)>>1) {
		if release != nil && release.Body != nil {
			_ = release.Body.Close()
		}
		if err == nil {
			err = errAppDevProviderHTTPUnavailable
		}
		appDevProviderHTTPError(c, err)
		return
	}
	c.Response.Header.SetContentType("application/zip")
	c.Response.Header.Set("Content-Length", strconv.FormatInt(release.Size, 10))
	c.Response.Header.Set("Content-Disposition", `attachment; filename="appdev-release.zip"`)
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	if release.Stale {
		c.Response.Header.Set("X-AppDev-Release-Stale", "true")
	}
	c.SetStatusCode(consts.StatusOK)
	c.Response.SetBodyStream(release.Body, int(release.Size))
}

func (handler *appDevProviderHTTPHandler) RestoreSnapshot(ctx context.Context, c *app.RequestContext) {
	identity, projector, ok := handler.authorize(ctx, c, true)
	if !ok {
		return
	}
	operationID, ok := appDevProviderOperationID(c)
	if !ok {
		return
	}
	snapshotID := c.Param("snapshot_id")
	if !validAppDevProviderPathID(snapshotID) {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return
	}
	projection, err := handler.control.RestoreSnapshot(ctx, appdev.ProviderSnapshotRestoreRequest{
		SpaceID: identity.spaceID, ProjectID: identity.projectID, SnapshotID: snapshotID,
		ActorUserID: identity.userID, OperationID: operationID, ActorID: strconv.FormatInt(identity.userID, 10),
	})
	if err != nil {
		appDevProviderHTTPError(c, err)
		return
	}
	status := consts.StatusOK
	if projection != nil && projection.Stopping {
		status = consts.StatusAccepted
	}
	handler.writeRuntimeProjection(ctx, c, identity, projector, projection, status)
}

func (handler *appDevProviderHTTPHandler) writeRuntimeProjection(ctx context.Context, c *app.RequestContext, identity *appDevIdentity, projector AppDevPreviewURLProjector, projection *appdev.ProviderRuntimeAPIProjection, status int) {
	if projection == nil || !validAppDevRuntimeState(projection.State) || projection.Generation > uint64(1<<63-1) {
		appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
		return
	}
	previewURL := ""
	if projection.RelativePreview != "" {
		if _, ok := normalizeAppDevPreviewRoute(projection.RelativePreview); !ok {
			appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
			return
		}
		var err error
		previewURL, err = projector.ProjectAppDevPreviewURL(ctx, identity.spaceID, identity.projectID, projection.RelativePreview)
		if err != nil || !validProjectedAppDevPreviewURL(previewURL) {
			appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
			return
		}
	}
	c.JSON(status, appdevapi.AppDevProviderRuntimeResponse{
		Generation: int64(projection.Generation), State: string(projection.State), CanStart: projection.CanStart,
		Recovering: projection.Recovering, Stopping: projection.Stopping, PreviewURL: appDevOptionalString(previewURL),
		SafeMessage: appDevOptionalString(safeAppDevProviderMessage(projection.SafeMessage)),
	})
}

func (handler *appDevProviderHTTPHandler) writeBuildProjection(c *app.RequestContext, projection *appdev.ProviderBuildAPIProjection, status int) {
	if projection == nil || !validAppDevBuildState(projection.State) || projection.Size < 0 || projection.Generation > uint64(1<<63-1) {
		appDevProviderHTTPError(c, errAppDevProviderHTTPUnavailable)
		return
	}
	var updatedAt *string
	if !projection.UpdatedAt.IsZero() {
		value := projection.UpdatedAt.UTC().Format(time.RFC3339Nano)
		updatedAt = &value
	}
	safeErrorCode := ""
	safeMessage := safeAppDevProviderMessage(projection.SafeMessage)
	if projection.State == appdev.ProviderBuildStateFailed {
		safeErrorCode, safeMessage = appdev.NormalizeProviderBuildPublicError(projection.SafeErrorCode)
	}
	c.JSON(status, appdevapi.AppDevProviderBuildResponse{
		Generation: int64(projection.Generation), State: string(projection.State), ReleaseAvailable: projection.ReleaseAvailable, Size: projection.Size,
		UpdatedAt: updatedAt, Stale: projection.Stale, SafeMessage: appDevOptionalString(safeMessage), SafeErrorCode: appDevOptionalString(safeErrorCode),
	})
}

func appDevOptionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func appDevProviderBuildHTTPStatus(projection *appdev.ProviderBuildAPIProjection) int {
	if projection != nil && projection.State == appdev.ProviderBuildStateBuilding {
		return consts.StatusAccepted
	}
	return consts.StatusOK
}

func currentDefaultAppDevProviderHTTPHandler() *appDevProviderHTTPHandler {
	appDevProviderHTTPHandlerMu.RLock()
	defer appDevProviderHTTPHandlerMu.RUnlock()
	return defaultAppDevProviderHTTPHandler
}

func setAppDevProviderHTTPDependenciesForTest(control appDevProviderHTTPControl, projector AppDevPreviewURLProjector) (func(), error) {
	handler, err := newAppDevProviderHTTPHandler(control, projector)
	if err != nil {
		return nil, err
	}
	appDevProviderHTTPTestOverrideMu.Lock()
	appDevProviderHTTPHandlerMu.Lock()
	previous := defaultAppDevProviderHTTPHandler
	defaultAppDevProviderHTTPHandler = handler
	appDevProviderHTTPHandlerMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			appDevProviderHTTPHandlerMu.Lock()
			if defaultAppDevProviderHTTPHandler == handler {
				defaultAppDevProviderHTTPHandler = previous
			}
			appDevProviderHTTPHandlerMu.Unlock()
			appDevProviderHTTPTestOverrideMu.Unlock()
		})
	}, nil
}

func appDevProviderOperationID(c *app.RequestContext) (string, bool) {
	if appDevProviderOperationInQuery(c) {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return "", false
	}
	value := string(c.GetHeader(appDevIdempotencyKeyHeader))
	if !validAppDevProviderOperationID(value) {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return "", false
	}
	return value, true
}

func appDevProviderOperationInQuery(c *app.RequestContext) bool {
	found := false
	c.Request.URI().QueryArgs().VisitAll(func(key, _ []byte) {
		normalized := strings.ToLower(strings.ReplaceAll(string(key), "_", "-"))
		switch normalized {
		case "idempotency-key", "idempotency", "operation-id", "operationid":
			found = true
		}
	})
	return found
}

func validAppDevProviderOperationID(value string) bool {
	if len(value) < appDevProviderMinOperationLength || len(value) > appDevProviderMaxOperationLength || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for index, char := range []byte(value) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':' {
			if index > 0 || (char != '-' && char != '_' && char != '.' && char != ':') {
				continue
			}
		}
		return false
	}
	return true
}

func validAppDevProviderPathID(value string) bool {
	return value != "" && len(value) <= 128 && value == strings.TrimSpace(value) && utf8.ValidString(value) && !strings.ContainsAny(value, "/\\?#\x00\r\n\t")
}

func normalizeAppDevPreviewRoute(value string) (string, bool) {
	if value == "" || len(value) > 512 || value != strings.TrimSpace(value) || !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "?#\\\x00\r\n\t") {
		return "", false
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "?#\\\x00\r\n\t") {
		return "", false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	return decoded, true
}

func validProjectedAppDevPreviewURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(parsed.Hostname())
	return parsed.Scheme == "http" && ip != nil && ip.IsLoopback()
}

func validAppDevRuntimeState(state appdev.ProviderRuntimeState) bool {
	switch state {
	case appdev.ProviderRuntimeStateStopped, appdev.ProviderRuntimeStatePending, appdev.ProviderRuntimeStateSubmitting, appdev.ProviderRuntimeStateStarting,
		appdev.ProviderRuntimeStateRunning, appdev.ProviderRuntimeStateSucceeded, appdev.ProviderRuntimeStateFailed,
		appdev.ProviderRuntimeStateCanceled, appdev.ProviderRuntimeStateTimedOut,
		appdev.ProviderRuntimeStateCleanupPending, appdev.ProviderRuntimeStateCleanupComplete:
		return true
	default:
		return false
	}
}

func validAppDevBuildState(state appdev.ProviderBuildState) bool {
	switch state {
	case appdev.ProviderBuildStateIdle, appdev.ProviderBuildStateBuilding, appdev.ProviderBuildStateReady, appdev.ProviderBuildStateFailed:
		return true
	default:
		return false
	}
}

func safeAppDevProviderMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > appDevProviderMaxSafeMessage || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\t") {
		return ""
	}
	return value
}

func getAppDevProviderIdentity(ctx context.Context, c *app.RequestContext, requireManager bool) (*appDevIdentity, bool) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		appDevProviderHTTPStatus(c, consts.StatusUnauthorized, "unauthorized", "authentication is required")
		return nil, false
	}
	spaceID := c.Param("space_id")
	spaceInt, err := strconv.ParseInt(spaceID, 10, 64)
	if err != nil || spaceInt <= 0 || strconv.FormatInt(spaceInt, 10) != spaceID {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return nil, false
	}
	projectID := c.Param("project_id")
	if !validAppDevProviderPathID(projectID) {
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
		return nil, false
	}
	if err := appworkspace.SVC.CheckWorkspaceAppDevAccess(ctx, &appworkspace.CheckWorkspaceAppDevAccessRequest{
		SpaceID: spaceInt, CurrentUserID: *currentUserID, RequireManager: requireManager,
	}); err != nil {
		appDevProviderHTTPStatus(c, consts.StatusForbidden, "forbidden", "access is denied")
		return nil, false
	}
	return &appDevIdentity{spaceID: spaceID, spaceInt: spaceInt, projectID: projectID, userID: *currentUserID}, true
}

func appDevProviderHTTPError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, appdev.ErrProviderControlInvalid):
		appDevProviderHTTPStatus(c, consts.StatusBadRequest, "invalid_request", "request is invalid")
	case errors.Is(err, appdev.ErrProviderControlNotFound), errors.Is(err, domainappdev.ErrNotFound), errors.Is(err, domainappdev.ErrProviderExecutionNotFound):
		appDevProviderHTTPStatus(c, consts.StatusNotFound, "not_found", "resource is not available")
	case errors.Is(err, appdev.ErrProviderControlConflict):
		appDevProviderHTTPStatus(c, consts.StatusConflict, "conflict", "operation conflicts with current state")
	case errors.Is(err, appdev.ErrProviderControlUnavailable), errors.Is(err, errAppDevProviderHTTPUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		appDevProviderHTTPStatus(c, consts.StatusServiceUnavailable, "unavailable", "service is temporarily unavailable")
	default:
		appDevProviderHTTPStatus(c, consts.StatusInternalServerError, "internal", "internal service error")
	}
}

func appDevProviderHTTPStatus(c *app.RequestContext, status int, code, message string) {
	c.JSON(status, appDevProviderErrorDTO{Code: code, Message: message})
}

type createAppDevProjectRequest struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type updateAppDevProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type duplicateAppDevProjectRequest struct {
	Name string `json:"name"`
}

type buildAppDevProjectRequest struct {
	PublishType string `json:"publishType"`
}

type createAppDevSnapshotRequest struct {
	Label string `json:"label"`
}

type saveAppDevFileContentRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Version string `json:"version"`
}

type deleteAppDevFileRequest struct {
	Path string `json:"path"`
}

type renameAppDevFileRequest struct {
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
}

type sendAppDevChatRequest struct {
	Message     string                  `json:"message"`
	ModelID     string                  `json:"modelId"`
	DataSources []appdev.ChatDataSource `json:"dataSources"`
	Attachments []appdev.ChatAttachment `json:"attachments"`
}

func ListAppDevProjects(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListProjects(ctx, &appdev.ListProjectsRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Keyword:       c.Query("keyword"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func CreateAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	var req createAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.CreateProject(ctx, &appdev.CreateProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Name:          req.Name,
		Prompt:        req.Prompt,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ImportAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		invalidParamRequestResponse(c, "project archive file is required")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}
	defer file.Close()

	archive, err := io.ReadAll(io.LimitReader(file, 100*1024*1024+1))
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	resp, err := appDevApplicationSVC.ImportProject(ctx, &appdev.ImportProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Name:          c.PostForm("name"),
		FileName:      fileHeader.Filename,
		Archive:       archive,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func GetAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.GetProject(ctx, &appdev.GetProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UpdateAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req updateAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.UpdateProject(ctx, &appdev.UpdateProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Name:          req.Name,
		Description:   req.Description,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func DuplicateAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req duplicateAppDevProjectRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.DuplicateProject(ctx, &appdev.DuplicateProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Name:          req.Name,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ArchiveAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ArchiveProject(ctx, &appdev.ArchiveProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ExportAppDevProject(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ExportProject(ctx, &appdev.ExportProjectRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.FileName))
	c.Data(consts.StatusOK, resp.ContentType, resp.Content)
}

func BuildAppDevProject(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().Build(ctx, c)
}

func GetAppDevBuildStatus(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().BuildStatus(ctx, c)
}

func DownloadAppDevRelease(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().Release(ctx, c)
}

func ListAppDevFiles(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListFiles(ctx, &appdev.ProjectFileRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func GetAppDevFileContent(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.GetFileContent(ctx, &appdev.FileContentRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Path:          c.Query("path"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SaveAppDevFileContent(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req saveAppDevFileContentRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.SaveFileContent(ctx, &appdev.SaveFileContentRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Path:          req.Path,
		Content:       req.Content,
		Version:       req.Version,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UploadAppDevFiles(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		invalidParamRequestResponse(c, "multipart form is required")
		return
	}

	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = form.File["file"]
	}
	filePaths := form.Value["filePaths"]
	if len(filePaths) == 0 {
		filePaths = form.Value["filePath"]
	}
	if len(fileHeaders) == 0 {
		invalidParamRequestResponse(c, "upload files are required")
		return
	}
	if len(fileHeaders) > 100 {
		invalidParamRequestResponse(c, "upload files cannot exceed 100")
		return
	}
	if len(filePaths) != len(fileHeaders) {
		invalidParamRequestResponse(c, "filePaths count must match files count")
		return
	}

	files := make([]appdev.UploadFileItem, 0, len(fileHeaders))
	var totalBytes int64
	for index, header := range fileHeaders {
		if header == nil {
			continue
		}
		if header.Size > 10*1024*1024 {
			invalidParamRequestResponse(c, fmt.Sprintf("upload file %s cannot exceed 10MB", header.Filename))
			return
		}
		opened, err := header.Open()
		if err != nil {
			appDevErrorResponse(c, err)
			return
		}
		content, readErr := io.ReadAll(io.LimitReader(opened, 10*1024*1024+1))
		closeErr := opened.Close()
		if readErr != nil {
			appDevErrorResponse(c, readErr)
			return
		}
		if closeErr != nil {
			appDevErrorResponse(c, closeErr)
			return
		}
		if len(content) > 10*1024*1024 {
			invalidParamRequestResponse(c, fmt.Sprintf("upload file %s cannot exceed 10MB", header.Filename))
			return
		}
		totalBytes += int64(len(content))
		if totalBytes > 100*1024*1024 {
			invalidParamRequestResponse(c, "upload files cannot exceed 100MB")
			return
		}

		files = append(files, appdev.UploadFileItem{
			Path:    filePaths[index],
			Content: content,
		})
	}

	resp, err := appDevApplicationSVC.UploadFiles(ctx, &appdev.UploadFilesRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Files:         files,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListAppDevSnapshots(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListSnapshots(ctx, &appdev.ProjectFileRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func CreateAppDevSnapshot(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req createAppDevSnapshotRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.CreateSnapshot(ctx, &appdev.CreateSnapshotRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Label:         req.Label,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func RestoreAppDevSnapshot(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().RestoreSnapshot(ctx, c)
}

func DeleteAppDevFile(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req deleteAppDevFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.DeletePath(ctx, &appdev.DeletePathRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Path:          req.Path,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func RenameAppDevFile(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req renameAppDevFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.RenamePath(ctx, &appdev.RenamePathRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		SourcePath:    req.SourcePath,
		TargetPath:    req.TargetPath,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func StartAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().StartRuntime(ctx, c)
}

func GetAppDevRuntimeStatus(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().RuntimeStatus(ctx, c)
}

func KeepAliveAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().KeepAlive(ctx, c)
}

func RestartAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().RestartRuntime(ctx, c)
}

func StopAppDevRuntime(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().StopRuntime(ctx, c)
}

func ListAppDevRuntimeLogs(ctx context.Context, c *app.RequestContext) {
	currentDefaultAppDevProviderHTTPHandler().RuntimeLogs(ctx, c)
}

func ListAppDevModels(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, false)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListModels(ctx, &appdev.ListModelsRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		Scenario:      c.Query("scenario"),
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SendAppDevChatMessage(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	var req sendAppDevChatRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appDevApplicationSVC.SendChatMessage(ctx, &appdev.SendChatRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
		Message:       req.Message,
		ModelID:       req.ModelID,
		DataSources:   req.DataSources,
		Attachments:   req.Attachments,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SubscribeAppDevChatEvents(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	events, cleanup, err := appDevApplicationSVC.SubscribeChatEvents(ctx, &appdev.ChatStreamRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.SetStatusCode(consts.StatusOK)
	c.Response.Header.SetContentType("text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	pipeReader, pipeWriter := io.Pipe()
	c.Response.SetBodyStream(pipeReader, -1)

	go func() {
		defer cleanup()
		defer func() {
			_ = pipeWriter.Close()
		}()

		w := bufio.NewWriter(pipeWriter)
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		writeEvent := func(eventName string, data map[string]any) bool {
			payload, err := json.Marshal(data)
			if err != nil {
				return true
			}
			if _, err := w.WriteString("event: " + eventName + "\n"); err != nil {
				return false
			}
			if _, err := w.WriteString("data: " + string(payload) + "\n\n"); err != nil {
				return false
			}
			return w.Flush() == nil
		}

		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if !writeEvent(event.Event, event.Data) {
					return
				}
			case <-ticker.C:
				if !writeEvent("heartbeat", map[string]any{
					"ts": time.Now().UTC().Format(time.RFC3339),
				}) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func CancelAppDevChat(ctx context.Context, c *app.RequestContext) {
	handleAppDevChatStatus(ctx, c, appDevApplicationSVC.CancelChat)
}

func ListAppDevChatHistory(ctx context.Context, c *app.RequestContext) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := appDevApplicationSVC.ListChatHistory(ctx, &appdev.ChatStreamRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func GetAppDevChatStatus(ctx context.Context, c *app.RequestContext) {
	handleAppDevChatStatus(ctx, c, appDevApplicationSVC.GetChatStatus)
}

func handleAppDevChatStatus(
	ctx context.Context,
	c *app.RequestContext,
	handler func(context.Context, *appdev.ChatStreamRequest) (*appdev.ChatStatusResponse, error),
) {
	identity, ok := getAppDevIdentity(ctx, c, true)
	if !ok {
		return
	}

	resp, err := handler(ctx, &appdev.ChatStreamRequest{
		SpaceID:       identity.spaceID,
		CurrentUserID: identity.userID,
		ProjectID:     identity.projectID,
	})
	if err != nil {
		appDevErrorResponse(c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

type appDevErrorPayload struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
}

func appDevErrorResponse(c *app.RequestContext, err error) {
	statusCode := consts.StatusInternalServerError
	if errors.Is(err, domainappdev.ErrInvalidPath) {
		statusCode = consts.StatusBadRequest
	} else if errors.Is(err, domainappdev.ErrNotFound) {
		statusCode = consts.StatusNotFound
	} else if strings.Contains(err.Error(), "required") ||
		strings.Contains(err.Error(), "empty") ||
		strings.Contains(err.Error(), "already") ||
		strings.Contains(err.Error(), "archived") ||
		strings.Contains(err.Error(), "cannot") ||
		strings.Contains(err.Error(), "invalid") ||
		strings.Contains(err.Error(), "only") ||
		strings.Contains(err.Error(), "not available") ||
		strings.Contains(err.Error(), "unsupported") ||
		strings.Contains(err.Error(), "exceed") {
		statusCode = consts.StatusBadRequest
	}

	message := err.Error()
	if statusCode == consts.StatusInternalServerError {
		message = "AppDev 服务异常，请稍后重试"
	}

	c.JSON(statusCode, appDevErrorPayload{
		Code:    -1,
		Message: message,
		Msg:     message,
	})
}

type appDevIdentity struct {
	spaceID   string
	spaceInt  int64
	projectID string
	userID    int64
}

func getAppDevIdentity(
	ctx context.Context,
	c *app.RequestContext,
	requireProject bool,
	requireManager ...bool,
) (*appDevIdentity, bool) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return nil, false
	}

	spaceID := strings.TrimSpace(c.Param("space_id"))
	if spaceID == "" {
		invalidParamRequestResponse(c, "space_id is required")
		return nil, false
	}

	spaceInt, err := strconv.ParseInt(spaceID, 10, 64)
	if err != nil || spaceInt <= 0 {
		invalidParamRequestResponse(c, "invalid space_id")
		return nil, false
	}

	projectID := strings.TrimSpace(c.Param("project_id"))
	if requireProject && projectID == "" {
		invalidParamRequestResponse(c, "project_id is required")
		return nil, false
	}

	managerAccess := len(requireManager) > 0 && requireManager[0]
	if err := appworkspace.SVC.CheckWorkspaceAppDevAccess(ctx, &appworkspace.CheckWorkspaceAppDevAccessRequest{
		SpaceID:        spaceInt,
		CurrentUserID:  *currentUserID,
		RequireManager: managerAccess,
	}); err != nil {
		internalServerErrorResponse(ctx, c, err)
		return nil, false
	}

	return &appDevIdentity{
		spaceID:   spaceID,
		spaceInt:  spaceInt,
		projectID: projectID,
		userID:    *currentUserID,
	}, true
}
