// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type fakeNotificationHTTPService struct {
	listRequest appnotification.ListRequest
	markUserID  int64
	markIDs     []int64
	markAllUser int64
	markCutoff  int64
}

func (f *fakeNotificationHTTPService) List(
	_ context.Context,
	request appnotification.ListRequest,
) (appnotification.Page, error) {
	f.listRequest = request
	return appnotification.Page{
		Items: []appnotification.Notification{{
			ID:        "1001",
			Title:     "任务已完成",
			Content:   "任务已完成。",
			SpaceID:   "202",
			Category:  domainnotification.CategoryTask,
			Severity:  domainnotification.SeveritySuccess,
			Route:     domainnotification.TargetTaskThread,
			TargetID:  "thread-100",
			CreatedAt: 1_721_000_000_000,
		}},
		SnapshotCutoff: "3003",
	}, nil
}

func (f *fakeNotificationHTTPService) GetUnreadCount(context.Context, int64) (int64, error) {
	return 7, nil
}

func (f *fakeNotificationHTTPService) MarkRead(
	_ context.Context,
	userID int64,
	ids []int64,
) (int64, error) {
	f.markUserID = userID
	f.markIDs = append([]int64(nil), ids...)
	return int64(len(ids)), nil
}

func (f *fakeNotificationHTTPService) MarkAllRead(
	_ context.Context,
	userID int64,
	cutoff int64,
) (int64, error) {
	f.markAllUser = userID
	f.markCutoff = cutoff
	return 3, nil
}

func TestNotificationListUsesAuthenticatedContextAndCurrentPlaygroundShape(t *testing.T) {
	service := &fakeNotificationHTTPService{}
	handler := &notificationHTTPHandler{
		Service: service,
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/list", handler.List)

	response := performJSONRequest(
		t,
		h,
		"/notice/list",
		`{"cursor":"0","count":20,"notice_rank_type":0,"user_id":"999"}`,
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(42), service.listRequest.UserID)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	data := payload["data"].(map[string]any)
	list := data["notice_list"].([]any)
	require.Len(t, list, 1)
	notice := list[0].(map[string]any)
	require.Equal(t, float64(noticeRouteTaskThread), notice["route"])
	require.Equal(t, "202", notice["route_space_id"])
	require.Equal(t, "thread-100", notice["route_target_id"])
	require.Equal(t, float64(noticeSeveritySuccess), notice["severity"])
	require.Equal(t, float64(noticeCategoryTask), notice["category"])
	require.Equal(t, "任务已完成", notice["sender"].(map[string]any)["sender_name"])
	require.Equal(t, "3003", data["snapshot_cutoff"])
}

func TestNotificationMarkReadCannotSelectAnotherUser(t *testing.T) {
	service := &fakeNotificationHTTPService{}
	handler := &notificationHTTPHandler{
		Service: service,
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/read", handler.MarkRead)

	response := performJSONRequest(
		t,
		h,
		"/notice/read",
		`{"notice_ids":["1001"],"read_mode":1,"user_id":"999"}`,
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(42), service.markUserID)
	require.Equal(t, []int64{1001}, service.markIDs)
}

func TestNotificationMarkAllUsesClientSnapshotCutoff(t *testing.T) {
	service := &fakeNotificationHTTPService{}
	handler := &notificationHTTPHandler{
		Service: service,
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/read", handler.MarkRead)

	response := performJSONRequest(
		t,
		h,
		"/notice/read",
		`{"read_mode":2,"snapshot_cutoff":"3003"}`,
	)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(42), service.markAllUser)
	require.Equal(t, int64(3003), service.markCutoff)
}

func TestNotificationMarkReadRejectsMixedReadModes(t *testing.T) {
	service := &fakeNotificationHTTPService{}
	handler := &notificationHTTPHandler{
		Service: service,
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/read", handler.MarkRead)

	response := performJSONRequest(
		t,
		h,
		"/notice/read",
		`{"notice_ids":["1001"],"read_mode":2,"snapshot_cutoff":"3003"}`,
	)

	require.Equal(t, http.StatusBadRequest, response.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, float64(notificationBusinessCodeFailure), payload["code"])
	require.Equal(t, notificationCodeInvalidReadMode, payload["error_code"])
	require.Zero(t, service.markAllUser)
	require.Zero(t, service.markUserID)
}

func TestNotificationErrorContractUsesStableCodesAndStatuses(t *testing.T) {
	testCases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "invalid cursor",
			err:    appnotification.ErrInvalidCursor,
			status: http.StatusBadRequest,
			code:   notificationCodeInvalidCursor,
		},
		{
			name:   "invalid read mode",
			err:    appnotification.ErrInvalidReadMode,
			status: http.StatusBadRequest,
			code:   notificationCodeInvalidReadMode,
		},
		{
			name:   "invalid page size",
			err:    appnotification.ErrInvalidPageSize,
			status: http.StatusBadRequest,
			code:   notificationCodeInvalidPageSize,
		},
		{
			name:   "invalid request",
			err:    appnotification.ErrInvalidRequest,
			status: http.StatusBadRequest,
			code:   notificationCodeInvalidRequest,
		},
		{
			name:   "invalid notification ID",
			err:    appnotification.ErrInvalidNotificationID,
			status: http.StatusBadRequest,
			code:   notificationCodeInvalidNotificationID,
		},
		{
			name:   "forbidden",
			err:    appnotification.ErrForbidden,
			status: http.StatusForbidden,
			code:   notificationCodeForbidden,
		},
		{
			name:   "internal",
			err:    domainnotification.ErrStorage,
			status: http.StatusInternalServerError,
			code:   notificationCodeInternal,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			h := server.Default()
			h.POST("/notice/error", func(ctx context.Context, c *app.RequestContext) {
				notificationErrorResponse(ctx, c, testCase.err)
			})

			response := performJSONRequest(t, h, "/notice/error", `{}`)
			require.Equal(t, testCase.status, response.Code)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			require.Equal(
				t,
				float64(notificationBusinessCodeFailure),
				payload["code"],
			)
			require.Equal(t, testCase.code, payload["error_code"])
			_, hasData := payload["data"]
			require.False(t, hasData)
		})
	}
}

func TestNotificationMalformedJSONUsesInvalidRequestCode(t *testing.T) {
	handler := &notificationHTTPHandler{
		Service: &fakeNotificationHTTPService{},
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/list", handler.List)

	response := performJSONRequest(t, h, "/notice/list", `{"cursor":`)
	require.Equal(t, http.StatusBadRequest, response.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, float64(notificationBusinessCodeFailure), payload["code"])
	require.Equal(t, notificationCodeInvalidRequest, payload["error_code"])
}

func TestNotificationMalformedIDUsesInvalidNotificationIDCode(t *testing.T) {
	handler := &notificationHTTPHandler{
		Service: &fakeNotificationHTTPService{},
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/read", handler.MarkRead)

	response := performJSONRequest(
		t,
		h,
		"/notice/read",
		`{"notice_ids":["not-an-id"],"read_mode":1}`,
	)
	require.Equal(t, http.StatusBadRequest, response.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, float64(notificationBusinessCodeFailure), payload["code"])
	require.Equal(
		t,
		notificationCodeInvalidNotificationID,
		payload["error_code"],
	)
}

func TestNotificationSuccessResponsesUseUnifiedNumericCodeContract(t *testing.T) {
	handler := &notificationHTTPHandler{
		Service: &fakeNotificationHTTPService{},
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	testCases := []struct {
		name string
		path string
		body string
		bind func(*server.Hertz)
	}{
		{
			name: "list",
			path: "/notice/list",
			body: `{"cursor":"0","count":20,"notice_rank_type":0}`,
			bind: func(h *server.Hertz) {
				h.POST("/notice/list", handler.List)
			},
		},
		{
			name: "unread count",
			path: "/notice/unread",
			body: `{}`,
			bind: func(h *server.Hertz) {
				h.POST("/notice/unread", handler.UnreadCount)
			},
		},
		{
			name: "mark read",
			path: "/notice/read",
			body: `{"notice_ids":["1001"],"read_mode":1}`,
			bind: func(h *server.Hertz) {
				h.POST("/notice/read", handler.MarkRead)
			},
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			h := server.Default()
			testCase.bind(h)
			response := performJSONRequest(
				t,
				h,
				testCase.path,
				testCase.body,
			)
			require.Equal(t, http.StatusOK, response.Code)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			require.Equal(
				t,
				float64(notificationBusinessCodeSuccess),
				payload["code"],
			)
			require.Equal(t, "", payload["error_code"])
			_, hasData := payload["data"]
			require.True(t, hasData)
		})
	}
}

func TestNotificationOpeningContractDoesNotMarkRead(t *testing.T) {
	service := &fakeNotificationHTTPService{}
	handler := &notificationHTTPHandler{
		Service: service,
		ViewerID: func(context.Context) int64 {
			return 42
		},
	}
	h := server.Default()
	h.POST("/notice/list", handler.List)

	response := performJSONRequest(t, h, "/notice/list", `{"cursor":"0"}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.Zero(t, service.markUserID)
	require.Zero(t, service.markAllUser)
}

func performJSONRequest(
	t *testing.T,
	h *server.Hertz,
	path string,
	body string,
) *ut.ResponseRecorder {
	t.Helper()
	return ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		path,
		&ut.Body{Body: bytes.NewBufferString(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
}
