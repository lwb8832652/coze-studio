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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
	"gorm.io/gorm"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	projectconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

const (
	canonicalAPIEnabledEnv   = "COZE_WORKBENCH_CANONICAL_API_ENABLED"
	canonicalContractVersion = "canonical_v1"
	canonicalSpaceIDHeader   = "X-Coze-Space-ID"
)

type canonicalError struct {
	Detail    string `json:"detail"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
	TraceID   string `json:"trace_id"`

	status     int
	errorClass string
}

type canonicalRequestLog struct {
	Operation      string
	RouteTemplate  string
	SubmissionKind string
	ThreadID       int64
	RunID          int64
	StartedAt      time.Time
}

func canonicalAPIEnabled(getenv func(string) string) bool {
	return getenv != nil && getenv(canonicalAPIEnabledEnv) == "true"
}

func requireCanonicalAPI(_ context.Context, c *app.RequestContext) bool {
	if canonicalAPIEnabled(os.Getenv) {
		return true
	}
	c.Status(hertzconsts.StatusNotFound)
	return false
}

func requireCanonicalAgentThreadService(ctx context.Context, c *app.RequestContext) bool {
	if appagentthread.SVC != nil && appagentthread.SVC.ThreadSVC != nil {
		return true
	}
	public := newCanonicalError(
		hertzconsts.StatusServiceUnavailable,
		"dependency_unavailable",
		"Required service is unavailable",
		"agent_thread_service_unavailable",
		true,
	)
	writeCanonicalError(ctx, c, public.status, *public)
	return false
}

func decodeCanonicalJSON(c *app.RequestContext, dst any) *canonicalError {
	// Canonical SDK request bodies are single JSON objects; arrays and concatenated
	// values are rejected before a handler can interpret partial input.
	body := bytes.TrimSpace(c.Request.Body())
	if len(body) == 0 || body[0] != '{' {
		return newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_json",
			"Request body must be a JSON object",
			"invalid_json",
			false,
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	// Keep JSON numbers lossless until the domain or repository applies the
	// MySQL JSON number contract. Decoding through float64 here would collapse
	// distinct large metadata values before validation.
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if field := canonicalUnknownJSONField(err); field != "" {
			return newCanonicalError(
				hertzconsts.StatusUnprocessableEntity,
				"unsupported_sdk_field",
				"Unsupported field: "+field,
				"unsupported_field",
				false,
			)
		}
		return newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_json",
			"Request body is not valid JSON",
			"invalid_json",
			false,
		)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_json",
			"Request body must contain one JSON object",
			"trailing_json",
			false,
		)
	}
	return nil
}

func canonicalPathID(c *app.RequestContext, name string) (int64, *canonicalError) {
	return parseCanonicalPositiveID(
		c.Param(name),
		"invalid_path_parameter",
		"Invalid path parameter: "+name,
		"invalid_path_id",
	)
}

func canonicalQueryInt64(c *app.RequestContext, name string) (*int64, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists {
		return nil, nil
	}
	value, public := parseCanonicalPositiveID(
		raw,
		"invalid_query_parameter",
		"Invalid query parameter: "+name,
		"invalid_query_id",
	)
	if public != nil {
		return nil, public
	}
	return &value, nil
}

func canonicalSpaceID(ctx context.Context, c *app.RequestContext) (int64, *canonicalError) {
	raw := string(c.GetHeader(canonicalSpaceIDHeader))
	spaceID, public := parseCanonicalPositiveID(
		raw,
		"invalid_space_id",
		canonicalSpaceIDHeader+" must be a positive decimal ID",
		"invalid_space_id",
	)
	if public != nil {
		return 0, public
	}

	viewerID := workbenchViewerIDFromCtx(ctx)
	if viewerID <= 0 {
		return 0, newCanonicalError(
			hertzconsts.StatusUnauthorized,
			"unauthenticated",
			"Authentication required",
			"unauthenticated",
			false,
		)
	}
	if appagentthread.SVC == nil || appagentthread.SVC.WorkspaceAuthorizer == nil {
		return 0, newCanonicalError(
			hertzconsts.StatusServiceUnavailable,
			"dependency_unavailable",
			"Required service is unavailable",
			"workspace_authorizer_unavailable",
			true,
		)
	}

	// The header selects a target workspace. Membership is still established only
	// by the authenticated principal and the server-side authorizer.
	err := appagentthread.SVC.AuthorizeWorkspaceAccess(ctx, appagentthread.WorkspaceAccessRequest{
		ViewerID: viewerID,
		SpaceID:  spaceID,
	})
	if err == nil {
		return spaceID, nil
	}
	if errors.Is(err, appagentthread.ErrThreadAccessDenied) {
		return 0, newCanonicalError(
			hertzconsts.StatusNotFound,
			"workspace_not_found",
			"Workspace not found",
			"workspace_access_denied",
			false,
		)
	}
	mapped := mapCanonicalApplicationError(err)
	return 0, &mapped
}

func writeCanonicalError(
	ctx context.Context,
	c *app.RequestContext,
	status int,
	public canonicalError,
) {
	if status < hertzconsts.StatusBadRequest || status > 599 {
		status = hertzconsts.StatusInternalServerError
		public = mapCanonicalApplicationError(errors.New("invalid canonical error status"))
	}
	if public.Code == "" || public.Detail == "" {
		public = mapCanonicalApplicationError(errors.New("incomplete canonical public error"))
		status = public.status
	}
	public.TraceID = canonicalTraceID(ctx)
	c.JSON(status, public)

	errorClass := public.errorClass
	if errorClass == "" {
		errorClass = public.Code
	}
	logs.CtxWarnf(ctx,
		"event_name=workbench.api.request.rejected client_contract=%s trace_id=%s http_status=%d error_code=%s error_class=%s",
		canonicalContractVersion, public.TraceID, status, public.Code, errorClass,
	)
}

func canonicalTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	traceID, ok := ctx.Value(projectconsts.CtxLogIDKey).(string)
	if !ok {
		return ""
	}
	return traceID
}

func canonicalRunPath(threadID, runID int64) string {
	return fmt.Sprintf("/threads/%d/runs/%d", threadID, runID)
}

func canonicalRunStreamPath(threadID, runID int64) string {
	return canonicalRunPath(threadID, runID) + "/stream"
}

func canonicalRunJoinPath(threadID, runID int64) string {
	return canonicalRunPath(threadID, runID) + "/join"
}

func setCanonicalPaginationHeaders(c *app.RequestContext, total int64, offset, limit int) {
	if total < 0 {
		total = 0
	}
	c.Header("X-Pagination-Total", strconv.FormatInt(total, 10))
	c.Response.Header.Del("X-Pagination-Next")
	if offset < 0 || limit <= 0 {
		return
	}

	current := int64(offset)
	if current >= total || int64(limit) >= total-current {
		return
	}
	c.Header("X-Pagination-Next", strconv.FormatInt(current+int64(limit), 10))
}

func logCanonicalRequestCompleted(
	ctx context.Context,
	c *app.RequestContext,
	info canonicalRequestLog,
	outcome string,
) {
	duration := int64(0)
	if !info.StartedAt.IsZero() {
		duration = time.Since(info.StartedAt).Milliseconds()
		if duration < 0 {
			duration = 0
		}
	}
	outcome = canonicalLogOutcome(outcome)
	logs.CtxInfof(ctx,
		"event_name=workbench.api.request.completed client_contract=%s trace_id=%s operation=%s route_template=%s http_method=%s http_status=%d duration_ms=%d outcome=%s submission_kind=%s thread_id=%d run_id=%d",
		canonicalContractVersion, canonicalTraceID(ctx), info.Operation, info.RouteTemplate,
		string(c.Method()), c.Response.StatusCode(), duration, outcome,
		canonicalSubmissionKind(info.SubmissionKind), info.ThreadID, info.RunID,
	)
}

func beginCanonicalRequestLog(operation, routeTemplate string) *canonicalRequestLog {
	return &canonicalRequestLog{
		Operation: operation, RouteTemplate: routeTemplate, StartedAt: time.Now(),
	}
}

func completeCanonicalRequestLog(
	ctx context.Context,
	c *app.RequestContext,
	info *canonicalRequestLog,
) {
	if info == nil {
		return
	}
	logCanonicalRequestCompleted(ctx, c, *info, canonicalHTTPOutcome(c.Response.StatusCode()))
}

func canonicalHTTPOutcome(status int) string {
	switch {
	case status >= hertzconsts.StatusOK && status < hertzconsts.StatusMultipleChoices:
		return "success"
	case status == hertzconsts.StatusConflict:
		return "conflict"
	case status >= hertzconsts.StatusBadRequest && status < hertzconsts.StatusInternalServerError:
		return "rejected"
	default:
		return "failed"
	}
}

func canonicalSubmissionKind(value string) string {
	switch value {
	case "empty_thread", "initial_run", "deferred_initial_run":
		return value
	default:
		return "not_applicable"
	}
}

func mapCanonicalApplicationError(err error) canonicalError {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return *newCanonicalError(
			hertzconsts.StatusNotFound,
			"resource_not_found",
			"Resource not found",
			"resource_not_found",
			false,
		)
	case errors.Is(err, appagentthread.ErrPublicThreadStateConflict):
		return *newCanonicalError(
			hertzconsts.StatusConflict,
			"state_conflict",
			"Thread state cannot be updated",
			"public_state_conflict",
			false,
		)
	case errors.Is(err, appagentthread.ErrUnsupportedPublicStateChannel):
		return *newCanonicalError(
			hertzconsts.StatusUnprocessableEntity,
			"unsupported_state_channel",
			"Unsupported public state channel",
			"unsupported_state_channel",
			false,
		)
	case errors.Is(err, domainservice.ErrInvalidArgument):
		return *newCanonicalError(
			hertzconsts.StatusBadRequest,
			"invalid_request",
			"Invalid request",
			"invalid_argument",
			false,
		)
	case errors.Is(err, appagentthread.ErrThreadAccessDenied),
		errors.Is(err, appagentthread.ErrArtifactAccessDenied),
		errors.Is(err, appagentthread.ErrMemoryAccessDenied),
		errors.Is(err, appagentthread.ErrGuardrailAuditAccessDenied),
		errors.Is(err, appagentthread.ErrMCPRuntimeAuditAccessDenied):
		return *newCanonicalError(
			hertzconsts.StatusNotFound,
			"resource_not_found",
			"Resource not found",
			"access_denied",
			false,
		)
	case errors.Is(err, appagentthread.ErrActiveRunExists):
		return *newCanonicalError(
			hertzconsts.StatusConflict,
			"run_conflict",
			"Thread already has an active run",
			"active_run_exists",
			false,
		)
	case errors.Is(err, appagentthread.ErrUnsupportedMultitaskStrategy):
		return *newCanonicalError(
			hertzconsts.StatusUnprocessableEntity,
			"unsupported_value",
			"Unsupported request value",
			"unsupported_multitask_strategy",
			false,
		)
	case errors.Is(err, appagentthread.ErrInvalidRuntimeConfig):
		return *newCanonicalError(
			hertzconsts.StatusUnprocessableEntity,
			"invalid_runtime_config",
			"Runtime configuration is invalid",
			"invalid_runtime_config",
			false,
		)
	case errors.Is(err, appagentthread.ErrThreadAuthorizationUnavailable),
		errors.Is(err, context.DeadlineExceeded):
		return *newCanonicalError(
			hertzconsts.StatusServiceUnavailable,
			"dependency_unavailable",
			"Required service is unavailable",
			"dependency_unavailable",
			true,
		)
	default:
		return *newCanonicalError(
			hertzconsts.StatusInternalServerError,
			"internal_error",
			"Internal server error",
			"internal_error",
			false,
		)
	}
}

func newCanonicalError(
	status int,
	code string,
	detail string,
	errorClass string,
	retryable bool,
) *canonicalError {
	return &canonicalError{
		Detail:     detail,
		Code:       code,
		Retryable:  retryable,
		status:     status,
		errorClass: errorClass,
	}
}

func parseCanonicalPositiveID(
	raw string,
	code string,
	detail string,
	errorClass string,
) (int64, *canonicalError) {
	if raw == "" {
		return 0, newCanonicalError(
			hertzconsts.StatusBadRequest, code, detail, errorClass, false,
		)
	}
	for i := range raw {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, newCanonicalError(
				hertzconsts.StatusBadRequest, code, detail, errorClass, false,
			)
		}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, newCanonicalError(
			hertzconsts.StatusBadRequest, code, detail, errorClass, false,
		)
	}
	return value, nil
}

func canonicalUnknownJSONField(err error) string {
	const prefix = "json: unknown field "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	field, unquoteErr := strconv.Unquote(strings.TrimPrefix(message, prefix))
	if unquoteErr != nil || field == "" || len(field) > 128 {
		return ""
	}
	return field
}

func canonicalLogOutcome(outcome string) string {
	switch outcome {
	case "success", "rejected", "conflict", "failed", "canceled", "disconnected":
		return outcome
	default:
		return "failed"
	}
}
