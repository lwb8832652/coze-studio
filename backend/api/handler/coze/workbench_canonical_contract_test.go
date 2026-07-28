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
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	projectconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestCanonicalGateDefaultsToNotFound(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "")

	var c app.RequestContext
	require.False(t, canonicalAPIEnabled(os.Getenv))
	require.False(t, requireCanonicalAPI(context.Background(), &c))
	require.Equal(t, hertzconsts.StatusNotFound, c.Response.StatusCode())
	require.Empty(t, c.Response.Body())
}

func TestCanonicalGateAcceptsOnlyExplicitTrue(t *testing.T) {
	for _, value := range []string{"", "1", "TRUE", " true", "true ", "yes"} {
		value := value
		t.Run("reject_"+strings.ReplaceAll(value, " ", "_"), func(t *testing.T) {
			require.False(t, canonicalAPIEnabled(func(string) string { return value }))
		})
	}

	require.True(t, canonicalAPIEnabled(func(string) string { return "true" }))
}

func TestCanonicalDecodeRejectsUnknownField(t *testing.T) {
	type request struct {
		AssistantID string `json:"assistant_id"`
	}

	var c app.RequestContext
	c.Request.SetBody([]byte(`{"assistant_id":"agent","checkpoint_during":"secret-payload"}`))
	var req request
	public := decodeCanonicalJSON(&c, &req)

	require.NotNil(t, public)
	require.Equal(t, hertzconsts.StatusUnprocessableEntity, public.status)
	require.Equal(t, "unsupported_sdk_field", public.Code)
	require.Equal(t, "Unsupported field: checkpoint_during", public.Detail)
	require.NotContains(t, public.Detail, "secret-payload")
}

func TestCanonicalDecodeAcceptsSingleObject(t *testing.T) {
	type request struct {
		AssistantID string `json:"assistant_id"`
	}

	var c app.RequestContext
	c.Request.SetBody([]byte(`{"assistant_id":"agent"}`))
	var req request
	require.Nil(t, decodeCanonicalJSON(&c, &req))
	require.Equal(t, "agent", req.AssistantID)
}

func TestCanonicalDecodeRejectsTrailingJSON(t *testing.T) {
	type request struct {
		AssistantID string `json:"assistant_id"`
	}

	for _, body := range []string{
		`{"assistant_id":"agent"}{"assistant_id":"second"}`,
		`{"assistant_id":"agent"} trailing`,
		``,
		`[]`,
		`null`,
	} {
		body := body
		t.Run(body, func(t *testing.T) {
			var c app.RequestContext
			c.Request.SetBody([]byte(body))
			var req request
			public := decodeCanonicalJSON(&c, &req)

			require.NotNil(t, public)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_json", public.Code)
		})
	}
}

func TestCanonicalParseIDRejectsZeroNegativeAndNonDecimal(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "+1", " 1", "1.0", "0x10", "abc"} {
		value := value
		t.Run("path_"+value, func(t *testing.T) {
			c := app.NewContext(1)
			c.Params = param.Params{{Key: "thread_id", Value: value}}

			id, public := canonicalPathID(c, "thread_id")
			require.Zero(t, id)
			require.NotNil(t, public)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_path_parameter", public.Code)
		})
	}

	c := app.NewContext(1)
	c.Params = param.Params{{Key: "thread_id", Value: "42"}}
	id, public := canonicalPathID(c, "thread_id")
	require.Nil(t, public)
	require.Equal(t, int64(42), id)

	var query app.RequestContext
	require.Nil(t, canonicalQueryValue(t, &query, "run_id", nil))
	query.Request.SetRequestURI("/?run_id=84")
	require.Equal(t, int64(84), *canonicalQueryValue(t, &query, "run_id", nil))
	query.Request.SetRequestURI("/?run_id=0")
	canonicalQueryValue(t, &query, "run_id", func(public *canonicalError) {
		require.Equal(t, hertzconsts.StatusBadRequest, public.status)
		require.Equal(t, "invalid_query_parameter", public.Code)
	})
}

func TestCanonicalSpaceIDRequiresAuthorizedHeader(t *testing.T) {
	previous := appagentthread.SVC.WorkspaceAuthorizer
	t.Cleanup(func() { appagentthread.SVC.WorkspaceAuthorizer = previous })

	t.Run("missing header", func(t *testing.T) {
		var c app.RequestContext
		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusBadRequest, public.status)
		require.Equal(t, "invalid_space_id", public.Code)
	})

	for _, value := range []string{"0", "-1", "+1", " 1001", "workspace"} {
		value := value
		t.Run("invalid header "+value, func(t *testing.T) {
			var c app.RequestContext
			c.Request.Header.Set("X-Coze-Space-ID", value)
			spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
			require.Zero(t, spaceID)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_space_id", public.Code)
		})
	}

	t.Run("unauthenticated", func(t *testing.T) {
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")
		spaceID, public := canonicalSpaceID(context.Background(), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusUnauthorized, public.status)
		require.Equal(t, "unauthenticated", public.Code)
	})

	t.Run("authorized server side", func(t *testing.T) {
		authorizer := &canonicalRecordingWorkspaceAuthorizer{}
		appagentthread.SVC.WorkspaceAuthorizer = authorizer
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Nil(t, public)
		require.Equal(t, int64(1001), spaceID)
		require.Equal(t, 1, authorizer.calls)
		require.Equal(t, appagentthread.WorkspaceAccessRequest{
			ViewerID: 42,
			SpaceID:  1001,
		}, authorizer.req)
	})

	t.Run("denied space is masked", func(t *testing.T) {
		appagentthread.SVC.WorkspaceAuthorizer = &canonicalRecordingWorkspaceAuthorizer{
			err: appagentthread.ErrThreadAccessDenied,
		}
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusNotFound, public.status)
		require.Equal(t, "workspace_not_found", public.Code)
	})

	t.Run("authorization dependency fails closed", func(t *testing.T) {
		appagentthread.SVC.WorkspaceAuthorizer = &canonicalRecordingWorkspaceAuthorizer{
			err: appagentthread.ErrThreadAuthorizationUnavailable,
		}
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusServiceUnavailable, public.status)
		require.Equal(t, "dependency_unavailable", public.Code)
		require.True(t, public.Retryable)
	})

	t.Run("missing authorizer fails closed", func(t *testing.T) {
		appagentthread.SVC.WorkspaceAuthorizer = nil
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusServiceUnavailable, public.status)
		require.Equal(t, "dependency_unavailable", public.Code)
		require.True(t, public.Retryable)
	})
}

func TestCanonicalErrorNeverLeaksCauseOrPayload(t *testing.T) {
	ctx := context.WithValue(context.Background(), projectconsts.CtxLogIDKey, "trace-test")
	var c app.RequestContext
	writeCanonicalError(ctx, &c, hertzconsts.StatusUnprocessableEntity, canonicalError{
		Detail: "Unsupported field: checkpoint_during",
		Code:   "unsupported_sdk_field",
	})

	require.Equal(t, hertzconsts.StatusUnprocessableEntity, c.Response.StatusCode())
	require.Equal(t,
		`{"detail":"Unsupported field: checkpoint_during","code":"unsupported_sdk_field","retryable":false,"trace_id":"trace-test"}`,
		string(c.Response.Body()),
	)

	mapped := mapCanonicalApplicationError(errors.New(
		"database failed with sk-secret and raw provider payload",
	))
	var internal app.RequestContext
	writeCanonicalError(ctx, &internal, mapped.status, mapped)
	body := string(internal.Response.Body())
	require.Equal(t, hertzconsts.StatusInternalServerError, internal.Response.StatusCode())
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "provider payload")
	require.NotContains(t, body, "database failed")
}

func TestCanonicalErrorMapsApplicationFailures(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		status    int
		code      string
		retryable bool
	}{
		{"invalid argument", domainservice.ErrInvalidArgument, hertzconsts.StatusBadRequest, "invalid_request", false},
		{"access denied", appagentthread.ErrThreadAccessDenied, hertzconsts.StatusNotFound, "resource_not_found", false},
		{"active run", appagentthread.ErrActiveRunExists, hertzconsts.StatusConflict, "run_conflict", false},
		{"idempotency conflict", appagentthread.ErrRunIdempotencyConflict, hertzconsts.StatusConflict, "idempotency_conflict", false},
		{"invalid resume", appagentthread.ErrHumanInteractionResumeInvalid, hertzconsts.StatusUnprocessableEntity, "invalid_resume", false},
		{"resume conflict", appagentthread.ErrHumanInteractionResumeConflict, hertzconsts.StatusConflict, "run_not_resumable", false},
		{"journal budget", errCanonicalJournalBudgetExceeded, hertzconsts.StatusUnprocessableEntity, "journal_too_large", false},
		{"deadline", context.DeadlineExceeded, hertzconsts.StatusGatewayTimeout, "run_wait_timeout", true},
		{"canceled", context.Canceled, hertzconsts.StatusRequestTimeout, "request_canceled", true},
		{"unsupported value", appagentthread.ErrUnsupportedMultitaskStrategy, hertzconsts.StatusUnprocessableEntity, "unsupported_value", false},
		{"runtime config", appagentthread.ErrInvalidRuntimeConfig, hertzconsts.StatusUnprocessableEntity, "invalid_runtime_config", false},
		{"dependency", appagentthread.ErrThreadAuthorizationUnavailable, hertzconsts.StatusServiceUnavailable, "dependency_unavailable", true},
		{"unknown", errors.New("sensitive internal cause"), hertzconsts.StatusInternalServerError, "internal_error", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			public := mapCanonicalApplicationError(tt.err)
			require.Equal(t, tt.status, public.status)
			require.Equal(t, tt.code, public.Code)
			require.Equal(t, tt.retryable, public.Retryable)
			require.NotContains(t, public.Detail, tt.err.Error())
		})
	}
}

func TestCanonicalErrorLogFieldsNeverExposeRawCause(t *testing.T) {
	const sensitiveCause = "database failed with sk-secret and raw provider payload"

	errorCode, errorClass := canonicalErrorLogFields(errors.New(sensitiveCause))

	require.Equal(t, "internal_error", errorCode)
	require.Equal(t, "internal_error", errorClass)
	require.NotContains(t, errorCode, sensitiveCause)
	require.NotContains(t, errorClass, sensitiveCause)
}

func TestCanonicalTraceIDUsesRequestLogID(t *testing.T) {
	ctx := context.WithValue(context.Background(), projectconsts.CtxLogIDKey, "trace-test")
	require.Equal(t, "trace-test", canonicalTraceID(ctx))
	require.Empty(t, canonicalTraceID(context.Background()))
	require.Empty(t, canonicalTraceID(context.WithValue(
		context.Background(),
		projectconsts.CtxLogIDKey,
		int64(42),
	)))
}

func TestCanonicalLogHashIsStableAndNeverEchoesSource(t *testing.T) {
	const source = "canonical-idempotency-secret"
	first := canonicalLogHash(source)
	second := canonicalLogHash(source)

	require.Equal(t, first, second)
	require.Len(t, first, 16)
	require.NotContains(t, first, source)
	require.Equal(t, "none", canonicalLogHash(""))
}

func TestCanonicalLogEnumsFailClosed(t *testing.T) {
	require.Equal(t, "values", canonicalResponseBodyKind("values", hertzconsts.StatusOK))
	require.Equal(t, "error", canonicalResponseBodyKind("values", hertzconsts.StatusBadRequest))
	require.Equal(t, "none", canonicalResponseBodyKind("request-body", hertzconsts.StatusOK))
	require.Equal(t, "run_retry", canonicalSubmissionKind("run_retry"))
	require.Equal(t, "not_applicable", canonicalSubmissionKind("retry-payload"))
	require.Equal(t, "not_applicable", canonicalRaiseErrorMode(nil))
	value := true
	require.Equal(t, "true", canonicalRaiseErrorMode(&value))
}

func TestCanonicalHeadersAreAPIBaseRelative(t *testing.T) {
	require.Equal(t, "/threads/101/runs/202", canonicalRunPath(101, 202))
	require.Equal(t, "/threads/101/runs/202/stream", canonicalRunStreamPath(101, 202))
	require.Equal(t, "/threads/101/runs/202/join", canonicalRunJoinPath(101, 202))

	for _, location := range []string{
		canonicalRunPath(101, 202),
		canonicalRunStreamPath(101, 202),
		canonicalRunJoinPath(101, 202),
	} {
		require.True(t, strings.HasPrefix(location, "/threads/"))
		require.NotContains(t, location, "/api/workbench")
		require.NotContains(t, location, "://")
		require.NotContains(t, location, "?")
	}
}

func TestCanonicalPaginationHeadersAreStable(t *testing.T) {
	var c app.RequestContext
	setCanonicalPaginationHeaders(&c, 53, 20, 10)
	require.Equal(t, "53", string(c.Response.Header.Peek("X-Pagination-Total")))
	require.Equal(t, "30", string(c.Response.Header.Peek("X-Pagination-Next")))

	setCanonicalPaginationHeaders(&c, 25, 20, 10)
	require.Equal(t, "25", string(c.Response.Header.Peek("X-Pagination-Total")))
	require.Empty(t, c.Response.Header.Peek("X-Pagination-Next"))

	setCanonicalPaginationHeaders(&c, int64(^uint64(0)>>1), int(^uint(0)>>1), int(^uint(0)>>1))
	require.Empty(t, c.Response.Header.Peek("X-Pagination-Next"))
}

type canonicalRecordingWorkspaceAuthorizer struct {
	calls int
	req   appagentthread.WorkspaceAccessRequest
	err   error
}

func (a *canonicalRecordingWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	_ context.Context,
	req appagentthread.WorkspaceAccessRequest,
) error {
	a.calls++
	a.req = req
	return a.err
}

func canonicalViewerContext(userID int64) context.Context {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, projectconsts.SessionDataKeyInCtx, &userentity.Session{UserID: userID})
	return ctx
}

func canonicalQueryValue(
	t *testing.T,
	c *app.RequestContext,
	name string,
	assertError func(*canonicalError),
) *int64 {
	t.Helper()
	value, public := canonicalQueryInt64(c, name)
	if assertError == nil {
		require.Nil(t, public)
		return value
	}
	require.NotNil(t, public)
	assertError(public)
	return nil
}
