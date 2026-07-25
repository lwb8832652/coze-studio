// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appannouncement "github.com/coze-dev/coze-studio/backend/application/announcement"
	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	typeconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

const announcementCreateBodyForTest = `{
	"idempotency_key":"handler.create.001",
	"title":"系统维护",
	"body":"系统将在今晚维护。",
	"severity":"warning",
	"route":{"type":2},
	"audience":{"type":"all","target_ids":[]}
}`

type announcementHTTPServiceStub struct {
	createCalls   int
	createActor   appannouncement.Actor
	createRequest appannouncement.CreateRequest
	createResult  *domainannouncement.MutationResult
	createErr     error
	replayCalls   int
	replayActor   appannouncement.Actor
	replayID      int64
	replayResult  *appannouncement.ReplayResult
	replayErr     error
}

func (s *announcementHTTPServiceStub) Create(
	_ context.Context,
	actor appannouncement.Actor,
	request appannouncement.CreateRequest,
) (*domainannouncement.MutationResult, error) {
	s.createCalls++
	s.createActor = actor
	s.createRequest = request
	return s.createResult, s.createErr
}

func (*announcementHTTPServiceStub) Update(
	context.Context,
	appannouncement.Actor,
	appannouncement.UpdateRequest,
) (*domainannouncement.Announcement, error) {
	return nil, domainannouncement.ErrStorage
}

func (*announcementHTTPServiceStub) Schedule(
	context.Context,
	appannouncement.Actor,
	appannouncement.ScheduleRequest,
) (*domainannouncement.Announcement, error) {
	return nil, domainannouncement.ErrStorage
}

func (*announcementHTTPServiceStub) Publish(
	context.Context,
	appannouncement.Actor,
	appannouncement.PublishRequest,
) (*appannouncement.PublicationResult, error) {
	return nil, domainannouncement.ErrStorage
}

func (*announcementHTTPServiceStub) Cancel(
	context.Context,
	appannouncement.Actor,
	appannouncement.CancelRequest,
) (*domainannouncement.Announcement, error) {
	return nil, domainannouncement.ErrStorage
}

func (*announcementHTTPServiceStub) Get(
	context.Context,
	appannouncement.Actor,
	int64,
) (*domainannouncement.Announcement, error) {
	return nil, domainannouncement.ErrStorage
}

func (*announcementHTTPServiceStub) List(
	context.Context,
	appannouncement.Actor,
	domainannouncement.ListFilter,
) ([]*domainannouncement.Announcement, int64, error) {
	return nil, 0, domainannouncement.ErrStorage
}

func (*announcementHTTPServiceStub) ListAuditEvents(
	context.Context,
	appannouncement.Actor,
	int64,
	int,
	int,
) ([]*domainannouncement.AuditEvent, int64, error) {
	return nil, 0, domainannouncement.ErrStorage
}

func (s *announcementHTTPServiceStub) Replay(
	_ context.Context,
	actor appannouncement.Actor,
	announcementID int64,
) (*appannouncement.ReplayResult, error) {
	s.replayCalls++
	s.replayActor = actor
	s.replayID = announcementID
	return s.replayResult, s.replayErr
}

func TestAnnouncementHandlerUsesAuthenticatedAdminActorAndSuccessCodeZero(
	t *testing.T,
) {
	service := &announcementHTTPServiceStub{
		createResult: &domainannouncement.MutationResult{
			Announcement: announcementHandlerFixture(),
		},
	}
	h := server.Default()
	h.Use(announcementAuthMiddlewareForTest(42, true))
	handler := &announcementHTTPHandler{service: service}
	h.POST("/api/admin/announcements", handler.create)

	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/announcements",
		&ut.Body{
			Body: bytes.NewBufferString(announcementCreateBodyForTest),
			Len:  len(announcementCreateBodyForTest),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	body := string(response.Result().Body())

	require.Equal(t, http.StatusCreated, response.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"id":"1001"`)
	require.Equal(t, 1, service.createCalls)
	require.Equal(t, int64(42), service.createActor.UserID)
	require.True(t, service.createActor.SystemAdmin)
	require.Equal(
		t,
		domainannouncement.AudienceAll,
		service.createRequest.Draft.Audience.Type,
	)

	spoofedBody := strings.Replace(
		announcementCreateBodyForTest,
		`"title":"系统维护"`,
		`"actor_id":"999","title":"系统维护"`,
		1,
	)
	spoofed := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/announcements",
		&ut.Body{
			Body: bytes.NewBufferString(spoofedBody),
			Len:  len(spoofedBody),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	require.Equal(t, http.StatusBadRequest, spoofed.Code)
	require.Contains(
		t,
		string(spoofed.Result().Body()),
		`"error_code":"ANNOUNCEMENT_INVALID_INPUT"`,
	)
	require.Equal(t, 1, service.createCalls)
}

func TestAnnouncementHandlerRejectsMissingOrNonAdminAuth(t *testing.T) {
	tests := []struct {
		name      string
		userID    int64
		admin     bool
		status    int
		errorCode string
	}{
		{
			name:      "missing session",
			admin:     true,
			status:    http.StatusUnauthorized,
			errorCode: announcementCodeUnauthenticated,
		},
		{
			name:      "non administrator",
			userID:    42,
			status:    http.StatusForbidden,
			errorCode: announcementCodePermissionDenied,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &announcementHTTPServiceStub{
				createResult: &domainannouncement.MutationResult{
					Announcement: announcementHandlerFixture(),
				},
			}
			h := server.Default()
			h.Use(announcementAuthMiddlewareForTest(test.userID, test.admin))
			handler := &announcementHTTPHandler{service: service}
			h.POST("/api/admin/announcements", handler.create)

			response := ut.PerformRequest(
				h.Engine,
				http.MethodPost,
				"/api/admin/announcements",
				&ut.Body{
					Body: bytes.NewBufferString(announcementCreateBodyForTest),
					Len:  len(announcementCreateBodyForTest),
				},
				ut.Header{Key: "content-type", Value: "application/json"},
			)
			body := string(response.Result().Body())

			require.Equal(t, test.status, response.Code)
			require.Contains(
				t,
				body,
				`"code":`+strconv.Itoa(announcementBusinessErrorCode),
			)
			require.Contains(t, body, `"error_code":"`+test.errorCode+`"`)
			require.Zero(t, service.createCalls)
		})
	}
}

func TestAnnouncementHandlerClassifiesAndRedactsErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		status    int
		errorCode string
	}{
		{
			name:      "idempotency conflict",
			err:       domainannouncement.ErrIdempotencyConflict,
			status:    http.StatusConflict,
			errorCode: announcementCodeIdempotencyConflict,
		},
		{
			name:      "announcement not found",
			err:       domainannouncement.ErrNotFound,
			status:    http.StatusNotFound,
			errorCode: announcementCodeNotFound,
		},
		{
			name:      "audience exceeds configured limit",
			err:       domainannouncement.ErrAudienceTooLarge,
			status:    http.StatusRequestEntityTooLarge,
			errorCode: announcementCodeAudienceTooLarge,
		},
		{
			name:      "state conflict",
			err:       domainannouncement.ErrStateConflict,
			status:    http.StatusConflict,
			errorCode: announcementCodeStateConflict,
		},
		{
			name:      "storage unavailable",
			err:       domainannouncement.ErrStorage,
			status:    http.StatusServiceUnavailable,
			errorCode: announcementCodeUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &announcementHTTPServiceStub{
				createErr: errors.Join(
					test.err,
					errors.New("password=top-secret raw_metadata=private"),
				),
			}
			h := server.Default()
			h.Use(announcementAuthMiddlewareForTest(42, true))
			handler := &announcementHTTPHandler{service: service}
			h.POST("/api/admin/announcements", handler.create)

			response := ut.PerformRequest(
				h.Engine,
				http.MethodPost,
				"/api/admin/announcements",
				&ut.Body{
					Body: bytes.NewBufferString(announcementCreateBodyForTest),
					Len:  len(announcementCreateBodyForTest),
				},
				ut.Header{Key: "content-type", Value: "application/json"},
			)
			body := string(response.Result().Body())

			require.Equal(t, test.status, response.Code)
			require.Contains(
				t,
				body,
				`"code":`+strconv.Itoa(announcementBusinessErrorCode),
			)
			require.Contains(t, body, `"error_code":"`+test.errorCode+`"`)
			require.NotContains(t, body, "top-secret")
			require.NotContains(t, body, "password")
			require.NotContains(t, body, "raw_metadata")
		})
	}
}

func TestAnnouncementReplayPropagatesNotFoundAs404(t *testing.T) {
	service := &announcementHTTPServiceStub{
		replayErr: domainannouncement.ErrNotFound,
	}
	h := server.Default()
	h.Use(announcementAuthMiddlewareForTest(42, true))
	handler := &announcementHTTPHandler{service: service}
	h.POST("/api/admin/announcements/replay", handler.replay)

	const requestBody = `{"announcement_id":"999"}`
	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/announcements/replay",
		&ut.Body{
			Body: bytes.NewBufferString(requestBody),
			Len:  len(requestBody),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	body := string(response.Result().Body())

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, body, `"code":1`)
	require.Contains(t, body, `"error_code":"ANNOUNCEMENT_NOT_FOUND"`)
	require.Equal(t, 1, service.replayCalls)
	require.Equal(t, int64(999), service.replayID)
	require.Equal(t, int64(42), service.replayActor.UserID)
	require.True(t, service.replayActor.SystemAdmin)
}

func TestAnnouncementReplayUsesSnakeCaseSuccessContract(t *testing.T) {
	service := &announcementHTTPServiceStub{
		replayResult: &appannouncement.ReplayResult{
			Processed: 2,
			Completed: 1,
			Failed:    1,
			ErrorCodes: map[string]int{
				domainannouncement.ErrorCodeStateConflict: 1,
			},
		},
	}
	h := server.Default()
	h.Use(announcementAuthMiddlewareForTest(42, true))
	handler := &announcementHTTPHandler{service: service}
	h.POST("/api/admin/announcements/replay", handler.replay)
	const requestBody = `{"announcement_id":""}`

	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/announcements/replay",
		&ut.Body{
			Body: bytes.NewBufferString(requestBody),
			Len:  len(requestBody),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	body := string(response.Result().Body())

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"processed":2`)
	require.Contains(t, body, `"failed":1`)
	require.Contains(t, body, `"error_codes":{"state_conflict":1}`)
	require.NotContains(t, body, `"ErrorCodes"`)
}

func announcementAuthMiddlewareForTest(
	userID int64,
	systemAdmin bool,
) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		ctx = ctxcache.Init(ctx)
		if userID > 0 {
			ctxcache.Store(
				ctx,
				typeconsts.SessionDataKeyInCtx,
				&userentity.Session{UserID: userID},
			)
		}
		if systemAdmin {
			ctxcache.Store(ctx, typeconsts.SystemAdminKeyInCtx, true)
		}
		c.Next(ctx)
	}
}

func announcementHandlerFixture() *domainannouncement.Announcement {
	return &domainannouncement.Announcement{
		ID: 1001,
		Draft: domainannouncement.Draft{
			Title:    "系统维护",
			Body:     "系统将在今晚维护。",
			Severity: domainannouncement.SeverityWarning,
			Route: domainnotification.AnnouncementRoute{
				Type: domainnotification.AnnouncementRouteSystemAnnouncements,
			},
			Audience: domainannouncement.Audience{
				Type: domainannouncement.AudienceAll,
			},
		},
		Status:           domainannouncement.StatusDraft,
		ProjectionStatus: domainannouncement.ProjectionIdle,
		CreatedBy:        42,
		UpdatedBy:        42,
		Version:          1,
		CreatedAt:        1_800_000_000_000,
		UpdatedAt:        1_800_000_000_000,
	}
}
