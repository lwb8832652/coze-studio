// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	appannouncement "github.com/coze-dev/coze-studio/backend/application/announcement"
	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	typeconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

const (
	maxAdminAnnouncementBodyBytes = 64 * 1024

	announcementCodeUnauthenticated       = "ANNOUNCEMENT_UNAUTHENTICATED"
	announcementCodePermissionDenied      = "ANNOUNCEMENT_PERMISSION_DENIED"
	announcementCodeInvalidInput          = "ANNOUNCEMENT_INVALID_INPUT"
	announcementCodeUnsafeContent         = "ANNOUNCEMENT_UNSAFE_CONTENT"
	announcementCodeTargetNotFound        = "ANNOUNCEMENT_AUDIENCE_TARGET_NOT_FOUND"
	announcementCodeAudienceTooLarge      = "ANNOUNCEMENT_AUDIENCE_TOO_LARGE"
	announcementCodeNotFound              = "ANNOUNCEMENT_NOT_FOUND"
	announcementCodeStateConflict         = "ANNOUNCEMENT_STATE_CONFLICT"
	announcementCodeVersionConflict       = "ANNOUNCEMENT_VERSION_CONFLICT"
	announcementCodeIdempotencyConflict   = "ANNOUNCEMENT_IDEMPOTENCY_CONFLICT"
	announcementCodeUnavailable           = "ANNOUNCEMENT_UNAVAILABLE"
	announcementCodeInternal              = "ANNOUNCEMENT_INTERNAL"
)

type announcementHTTPService interface {
	Create(
		context.Context,
		appannouncement.Actor,
		appannouncement.CreateRequest,
	) (*domainannouncement.MutationResult, error)
	Update(
		context.Context,
		appannouncement.Actor,
		appannouncement.UpdateRequest,
	) (*domainannouncement.Announcement, error)
	Schedule(
		context.Context,
		appannouncement.Actor,
		appannouncement.ScheduleRequest,
	) (*domainannouncement.Announcement, error)
	Publish(
		context.Context,
		appannouncement.Actor,
		appannouncement.PublishRequest,
	) (*appannouncement.PublicationResult, error)
	Cancel(
		context.Context,
		appannouncement.Actor,
		appannouncement.CancelRequest,
	) (*domainannouncement.Announcement, error)
	Get(
		context.Context,
		appannouncement.Actor,
		int64,
	) (*domainannouncement.Announcement, error)
	List(
		context.Context,
		appannouncement.Actor,
		domainannouncement.ListFilter,
	) ([]*domainannouncement.Announcement, int64, error)
	ListAuditEvents(
		context.Context,
		appannouncement.Actor,
		int64,
		int,
		int,
	) ([]*domainannouncement.AuditEvent, int64, error)
	Replay(
		context.Context,
		appannouncement.Actor,
		int64,
	) (*appannouncement.ReplayResult, error)
}

type announcementHTTPHandler struct {
	service announcementHTTPService
}

type announcementAudienceRequest struct {
	Type      string   `json:"type"`
	TargetIDs []string `json:"target_ids"`
}

type announcementRouteRequest struct {
	Type    int32  `json:"type"`
	SpaceID string `json:"space_id,omitempty"`
}

type createAnnouncementRequest struct {
	IdempotencyKey string                      `json:"idempotency_key"`
	Title          string                      `json:"title"`
	Body           string                      `json:"body"`
	Severity       string                      `json:"severity"`
	Route          *announcementRouteRequest   `json:"route"`
	Audience       announcementAudienceRequest `json:"audience"`
}

type updateAnnouncementRequest struct {
	ExpectedVersion int64                       `json:"expected_version"`
	Title           string                      `json:"title"`
	Body            string                      `json:"body"`
	Severity        string                      `json:"severity"`
	Route           *announcementRouteRequest   `json:"route"`
	Audience        announcementAudienceRequest `json:"audience"`
}

type scheduleAnnouncementRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	ScheduledAt     string `json:"scheduled_at"`
}

type publishAnnouncementRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type cancelAnnouncementRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type replayAnnouncementRequest struct {
	AnnouncementID string `json:"announcement_id"`
}

type announcementAudienceResponse struct {
	Type      string   `json:"type"`
	TargetIDs []string `json:"target_ids"`
}

type announcementRouteResponse struct {
	Type    int32  `json:"type"`
	SpaceID string `json:"space_id,omitempty"`
}

type announcementResponse struct {
	ID                 string                       `json:"id"`
	Title              string                       `json:"title"`
	Body               string                       `json:"body"`
	Severity           string                       `json:"severity"`
	Route              announcementRouteResponse    `json:"route"`
	Audience           announcementAudienceResponse `json:"audience"`
	Status             string                       `json:"status"`
	ProjectionStatus   string                       `json:"projection_status"`
	ScheduledAt        string                       `json:"scheduled_at,omitempty"`
	PublishRequestedAt string                       `json:"publish_requested_at,omitempty"`
	SnapshotAt         string                       `json:"snapshot_at,omitempty"`
	PublishedAt        string                       `json:"published_at,omitempty"`
	CancelledAt        string                       `json:"cancelled_at,omitempty"`
	CreatedBy          string                       `json:"created_by"`
	UpdatedBy          string                       `json:"updated_by"`
	RecipientCount     int64                        `json:"recipient_count"`
	ProjectedCount     int64                        `json:"projected_count"`
	LastErrorCode      string                       `json:"last_error_code,omitempty"`
	Version            int64                        `json:"version"`
	CreatedAt          string                       `json:"created_at"`
	UpdatedAt          string                       `json:"updated_at"`
}

type announcementAuditResponse struct {
	ID               string `json:"id"`
	AnnouncementID   string `json:"announcement_id"`
	ActorID           string `json:"actor_id"`
	Action            string `json:"action"`
	FromStatus        string `json:"from_status,omitempty"`
	ToStatus          string `json:"to_status,omitempty"`
	ProjectionStatus  string `json:"projection_status"`
	Result            string `json:"result"`
	ErrorCode         string `json:"error_code,omitempty"`
	RecipientCount    int64  `json:"recipient_count"`
	ProjectedCount    int64  `json:"projected_count"`
	CreatedAt         string `json:"created_at"`
}

type announcementEnvelope struct {
	Code      int    `json:"code"`
	ErrorCode string `json:"error_code,omitempty"`
	Msg       string `json:"msg"`
	Data      any    `json:"data,omitempty"`
}

const announcementBusinessErrorCode = 1

func CreateAdminAnnouncement(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().create(ctx, c)
}

func UpdateAdminAnnouncement(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().update(ctx, c)
}

func ScheduleAdminAnnouncement(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().schedule(ctx, c)
}

func PublishAdminAnnouncement(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().publish(ctx, c)
}

func CancelAdminAnnouncement(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().cancel(ctx, c)
}

func GetAdminAnnouncement(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().get(ctx, c)
}

func ListAdminAnnouncements(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().list(ctx, c)
}

func ListAdminAnnouncementAuditEvents(
	ctx context.Context,
	c *app.RequestContext,
) {
	defaultAnnouncementHTTPHandler().auditEvents(ctx, c)
}

func ReplayAdminAnnouncements(ctx context.Context, c *app.RequestContext) {
	defaultAnnouncementHTTPHandler().replay(ctx, c)
}

func defaultAnnouncementHTTPHandler() *announcementHTTPHandler {
	return &announcementHTTPHandler{service: appannouncement.SVC}
}

func (h *announcementHTTPHandler) create(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, ok := adminAnnouncementActor(ctx, c)
	if !ok {
		return
	}
	var request createAnnouncementRequest
	if !decodeAnnouncementRequest(ctx, c, &request) {
		return
	}
	draft, err := announcementDraftFromRequest(
		request.Title,
		request.Body,
		request.Severity,
		request.Route,
		request.Audience,
	)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	result, err := h.service.Create(ctx, actor, appannouncement.CreateRequest{
		IdempotencyKey: request.IdempotencyKey,
		Draft:          draft,
	})
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	adminAnnouncementSuccess(c, status, map[string]any{
		"announcement": announcementToResponse(result.Announcement),
		"replayed":     result.Replayed,
	})
}

func (h *announcementHTTPHandler) update(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, announcementID, ok := adminAnnouncementActorAndID(ctx, c)
	if !ok {
		return
	}
	var request updateAnnouncementRequest
	if !decodeAnnouncementRequest(ctx, c, &request) {
		return
	}
	draft, err := announcementDraftFromRequest(
		request.Title,
		request.Body,
		request.Severity,
		request.Route,
		request.Audience,
	)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	result, err := h.service.Update(ctx, actor, appannouncement.UpdateRequest{
		AnnouncementID: announcementID,
		ExpectedVersion: request.ExpectedVersion,
		Draft:           draft,
	})
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	adminAnnouncementSuccess(c, http.StatusOK, map[string]any{
		"announcement": announcementToResponse(result),
	})
}

func (h *announcementHTTPHandler) schedule(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, announcementID, ok := adminAnnouncementActorAndID(ctx, c)
	if !ok {
		return
	}
	var request scheduleAnnouncementRequest
	if !decodeAnnouncementRequest(ctx, c, &request) {
		return
	}
	scheduledAt, err := time.Parse(time.RFC3339, request.ScheduledAt)
	if err != nil {
		adminAnnouncementError(ctx, c, domainannouncement.ErrInvalidInput)
		return
	}
	result, err := h.service.Schedule(
		ctx,
		actor,
		appannouncement.ScheduleRequest{
			AnnouncementID: announcementID,
			ExpectedVersion: request.ExpectedVersion,
			ScheduledAt:     scheduledAt,
		},
	)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	adminAnnouncementSuccess(c, http.StatusOK, map[string]any{
		"announcement": announcementToResponse(result),
	})
}

func (h *announcementHTTPHandler) publish(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, announcementID, ok := adminAnnouncementActorAndID(ctx, c)
	if !ok {
		return
	}
	var request publishAnnouncementRequest
	if !decodeAnnouncementRequest(ctx, c, &request) {
		return
	}
	result, err := h.service.Publish(ctx, actor, appannouncement.PublishRequest{
		AnnouncementID: announcementID,
		ExpectedVersion: request.ExpectedVersion,
		IdempotencyKey:  request.IdempotencyKey,
	})
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	status := http.StatusOK
	if result.Deferred {
		status = http.StatusAccepted
	}
	adminAnnouncementSuccess(c, status, map[string]any{
		"announcement": announcementToResponse(result.Announcement),
		"deferred":     result.Deferred,
		"error_code":   result.ErrorCode,
		"replayed":     result.Replayed,
	})
}

func (h *announcementHTTPHandler) cancel(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, announcementID, ok := adminAnnouncementActorAndID(ctx, c)
	if !ok {
		return
	}
	var request cancelAnnouncementRequest
	if !decodeAnnouncementRequest(ctx, c, &request) {
		return
	}
	result, err := h.service.Cancel(ctx, actor, appannouncement.CancelRequest{
		AnnouncementID: announcementID,
		ExpectedVersion: request.ExpectedVersion,
	})
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	adminAnnouncementSuccess(c, http.StatusOK, map[string]any{
		"announcement": announcementToResponse(result),
	})
}

func (h *announcementHTTPHandler) get(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, announcementID, ok := adminAnnouncementActorAndID(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.Get(ctx, actor, announcementID)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	adminAnnouncementSuccess(c, http.StatusOK, map[string]any{
		"announcement": announcementToResponse(result),
	})
}

func (h *announcementHTTPHandler) list(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, ok := adminAnnouncementActor(ctx, c)
	if !ok {
		return
	}
	offset, err := parseAnnouncementPageValue(c.Query("offset"), 0, false)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	limit, err := parseAnnouncementPageValue(c.Query("limit"), 20, true)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	status := domainannouncement.Status(strings.TrimSpace(c.Query("status")))
	items, total, err := h.service.List(
		ctx,
		actor,
		domainannouncement.ListFilter{
			Status: status,
			Offset: offset,
			Limit:  limit,
		},
	)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	responses := make([]announcementResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, *announcementToResponse(item))
	}
	adminAnnouncementSuccess(c, http.StatusOK, map[string]any{
		"announcements": responses,
		"total":         total,
	})
}

func (h *announcementHTTPHandler) auditEvents(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, announcementID, ok := adminAnnouncementActorAndID(ctx, c)
	if !ok {
		return
	}
	offset, err := parseAnnouncementPageValue(c.Query("offset"), 0, false)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	limit, err := parseAnnouncementPageValue(c.Query("limit"), 50, true)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	items, total, err := h.service.ListAuditEvents(
		ctx,
		actor,
		announcementID,
		offset,
		limit,
	)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	responses := make([]announcementAuditResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, announcementAuditToResponse(item))
	}
	adminAnnouncementSuccess(c, http.StatusOK, map[string]any{
		"audit_events": responses,
		"total":        total,
	})
}

func (h *announcementHTTPHandler) replay(
	ctx context.Context,
	c *app.RequestContext,
) {
	actor, ok := adminAnnouncementActor(ctx, c)
	if !ok {
		return
	}
	var request replayAnnouncementRequest
	if !decodeAnnouncementRequest(ctx, c, &request) {
		return
	}
	announcementID := int64(0)
	var err error
	if strings.TrimSpace(request.AnnouncementID) != "" {
		announcementID, err = parseAnnouncementID(request.AnnouncementID)
		if err != nil {
			adminAnnouncementError(ctx, c, err)
			return
		}
	}
	result, err := h.service.Replay(ctx, actor, announcementID)
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return
	}
	status := http.StatusOK
	if result.Deferred > 0 {
		status = http.StatusAccepted
	}
	adminAnnouncementSuccess(c, status, result)
}

func adminAnnouncementActor(
	ctx context.Context,
	c *app.RequestContext,
) (appannouncement.Actor, bool) {
	session, authenticated := ctxcache.Get[*userentity.Session](
		ctx,
		typeconsts.SessionDataKeyInCtx,
	)
	if !authenticated || session == nil || session.UserID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, announcementEnvelope{
			Code:      announcementBusinessErrorCode,
			ErrorCode: announcementCodeUnauthenticated,
			Msg:       "authentication required",
		})
		return appannouncement.Actor{}, false
	}
	systemAdmin, authorized := ctxcache.Get[bool](
		ctx,
		typeconsts.SystemAdminKeyInCtx,
	)
	if !authorized || !systemAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, announcementEnvelope{
			Code:      announcementBusinessErrorCode,
			ErrorCode: announcementCodePermissionDenied,
			Msg:       "system administrator permission is required",
		})
		return appannouncement.Actor{}, false
	}
	return appannouncement.Actor{
		UserID:      session.UserID,
		SystemAdmin: true,
	}, true
}

func adminAnnouncementActorAndID(
	ctx context.Context,
	c *app.RequestContext,
) (appannouncement.Actor, int64, bool) {
	actor, ok := adminAnnouncementActor(ctx, c)
	if !ok {
		return appannouncement.Actor{}, 0, false
	}
	announcementID, err := parseAnnouncementID(c.Param("id"))
	if err != nil {
		adminAnnouncementError(ctx, c, err)
		return appannouncement.Actor{}, 0, false
	}
	return actor, announcementID, true
}

func decodeAnnouncementRequest(
	ctx context.Context,
	c *app.RequestContext,
	target any,
) bool {
	body := c.Request.Body()
	if len(body) == 0 || len(body) > maxAdminAnnouncementBodyBytes {
		adminAnnouncementError(ctx, c, domainannouncement.ErrInvalidInput)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		adminAnnouncementError(ctx, c, domainannouncement.ErrInvalidInput)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		adminAnnouncementError(ctx, c, domainannouncement.ErrInvalidInput)
		return false
	}
	return true
}

func announcementDraftFromRequest(
	title string,
	body string,
	severity string,
	routeRequest *announcementRouteRequest,
	audience announcementAudienceRequest,
) (domainannouncement.Draft, error) {
	if routeRequest == nil {
		return domainannouncement.Draft{}, domainannouncement.ErrInvalidInput
	}
	route := domainnotification.AnnouncementRoute{}
	switch routeRequest.Type {
	case 0:
		route.Type = domainnotification.AnnouncementRouteNone
	case 1:
		spaceID, err := parseAnnouncementID(routeRequest.SpaceID)
		if err != nil {
			return domainannouncement.Draft{}, err
		}
		route.Type = domainnotification.AnnouncementRouteWorkspaceHome
		route.SpaceID = spaceID
	case 2:
		route.Type = domainnotification.AnnouncementRouteSystemAnnouncements
	default:
		return domainannouncement.Draft{}, domainannouncement.ErrInvalidInput
	}
	if route.Type != domainnotification.AnnouncementRouteWorkspaceHome &&
		strings.TrimSpace(routeRequest.SpaceID) != "" {
		return domainannouncement.Draft{}, domainannouncement.ErrInvalidInput
	}
	targetIDs := make([]int64, 0, len(audience.TargetIDs))
	for _, value := range audience.TargetIDs {
		targetID, err := parseAnnouncementID(value)
		if err != nil {
			return domainannouncement.Draft{}, err
		}
		targetIDs = append(targetIDs, targetID)
	}
	return domainannouncement.Draft{
		Title:    title,
		Body:     body,
		Severity: domainannouncement.Severity(severity),
		Route:    route,
		Audience: domainannouncement.Audience{
			Type:      domainannouncement.AudienceType(audience.Type),
			TargetIDs: targetIDs,
		},
	}, nil
}

func parseAnnouncementID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, domainannouncement.ErrInvalidInput
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, domainannouncement.ErrInvalidInput
	}
	return parsed, nil
}

func parseAnnouncementPageValue(
	value string,
	defaultValue int,
	limit bool,
) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, domainannouncement.ErrInvalidInput
	}
	if limit && (parsed <= 0 || parsed > domainannouncement.MaxPageSize) {
		return 0, domainannouncement.ErrInvalidInput
	}
	return parsed, nil
}

func announcementToResponse(
	value *domainannouncement.Announcement,
) *announcementResponse {
	if value == nil {
		return nil
	}
	targetIDs := make([]string, 0, len(value.Draft.Audience.TargetIDs))
	for _, targetID := range value.Draft.Audience.TargetIDs {
		targetIDs = append(targetIDs, strconv.FormatInt(targetID, 10))
	}
	route := announcementRouteResponse{}
	switch value.Draft.Route.Type {
	case domainnotification.AnnouncementRouteWorkspaceHome:
		route.Type = 1
		route.SpaceID = strconv.FormatInt(value.Draft.Route.SpaceID, 10)
	case domainnotification.AnnouncementRouteSystemAnnouncements:
		route.Type = 2
	default:
		route.Type = 0
	}
	return &announcementResponse{
		ID:                strconv.FormatInt(value.ID, 10),
		Title:             value.Draft.Title,
		Body:              value.Draft.Body,
		Severity:          string(value.Draft.Severity),
		Route:             route,
		Audience: announcementAudienceResponse{
			Type:      string(value.Draft.Audience.Type),
			TargetIDs: targetIDs,
		},
		Status:             string(value.Status),
		ProjectionStatus:   string(value.ProjectionStatus),
		ScheduledAt:        formatAnnouncementTime(value.ScheduledAt),
		PublishRequestedAt: formatAnnouncementTime(value.PublishRequestedAt),
		SnapshotAt:         formatAnnouncementTime(value.SnapshotAt),
		PublishedAt:        formatAnnouncementTime(value.PublishedAt),
		CancelledAt:        formatAnnouncementTime(value.CancelledAt),
		CreatedBy:          strconv.FormatInt(value.CreatedBy, 10),
		UpdatedBy:          strconv.FormatInt(value.UpdatedBy, 10),
		RecipientCount:     value.RecipientCount,
		ProjectedCount:     value.ProjectedCount,
		LastErrorCode:      value.LastErrorCode,
		Version:            value.Version,
		CreatedAt:          formatAnnouncementTime(value.CreatedAt),
		UpdatedAt:          formatAnnouncementTime(value.UpdatedAt),
	}
}

func announcementAuditToResponse(
	value *domainannouncement.AuditEvent,
) announcementAuditResponse {
	if value == nil {
		return announcementAuditResponse{}
	}
	return announcementAuditResponse{
		ID:              strconv.FormatInt(value.ID, 10),
		AnnouncementID:  strconv.FormatInt(value.AnnouncementID, 10),
		ActorID:          strconv.FormatInt(value.ActorID, 10),
		Action:           value.Action,
		FromStatus:       string(value.FromStatus),
		ToStatus:         string(value.ToStatus),
		ProjectionStatus: string(value.ProjectionStatus),
		Result:           value.Result,
		ErrorCode:        value.ErrorCode,
		RecipientCount:   value.RecipientCount,
		ProjectedCount:   value.ProjectedCount,
		CreatedAt:        formatAnnouncementTime(value.CreatedAt),
	}
}

func formatAnnouncementTime(milliseconds int64) string {
	if milliseconds <= 0 {
		return ""
	}
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339)
}

func adminAnnouncementSuccess(
	c *app.RequestContext,
	status int,
	data any,
) {
	c.JSON(status, announcementEnvelope{
		Code: 0,
		Msg:  "success",
		Data: data,
	})
}

func adminAnnouncementError(
	ctx context.Context,
	c *app.RequestContext,
	err error,
) {
	status, code, message := announcementErrorContract(err)
	if status >= http.StatusInternalServerError {
		logs.CtxErrorf(
			ctx,
			"[announcement] request_failed error_code=%s",
			domainannouncement.StableErrorCode(err),
		)
	}
	c.AbortWithStatusJSON(status, announcementEnvelope{
		Code:      announcementBusinessErrorCode,
		ErrorCode: code,
		Msg:       message,
	})
}

func announcementErrorContract(err error) (int, string, string) {
	switch {
	case errors.Is(err, domainannouncement.ErrPermissionDenied):
		return http.StatusForbidden,
			announcementCodePermissionDenied,
			"system administrator permission is required"
	case errors.Is(err, domainannouncement.ErrUnsafeContent):
		return http.StatusBadRequest,
			announcementCodeUnsafeContent,
			"announcement content is not allowed"
	case errors.Is(err, domainannouncement.ErrInvalidInput):
		return http.StatusBadRequest,
			announcementCodeInvalidInput,
			"announcement request is invalid"
	case errors.Is(err, domainannouncement.ErrAudienceTargetNotFound):
		return http.StatusBadRequest,
			announcementCodeTargetNotFound,
			"announcement audience target does not exist"
	case errors.Is(err, domainannouncement.ErrAudienceTooLarge):
		return http.StatusRequestEntityTooLarge,
			announcementCodeAudienceTooLarge,
			"announcement audience exceeds the configured limit"
	case errors.Is(err, domainannouncement.ErrNotFound):
		return http.StatusNotFound,
			announcementCodeNotFound,
			"announcement was not found"
	case errors.Is(err, domainannouncement.ErrVersionConflict):
		return http.StatusConflict,
			announcementCodeVersionConflict,
			"announcement version is stale"
	case errors.Is(err, domainannouncement.ErrStateConflict):
		return http.StatusConflict,
			announcementCodeStateConflict,
			"announcement state does not allow this operation"
	case errors.Is(err, domainannouncement.ErrIdempotencyConflict):
		return http.StatusConflict,
			announcementCodeIdempotencyConflict,
			"announcement idempotency key conflicts with another request"
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout,
			announcementCodeUnavailable,
			"announcement service timed out"
	case errors.Is(err, domainannouncement.ErrStorage),
		errors.Is(err, context.Canceled):
		return http.StatusServiceUnavailable,
			announcementCodeUnavailable,
			"announcement service is temporarily unavailable"
	default:
		return http.StatusInternalServerError,
			announcementCodeInternal,
			"internal server error"
	}
}
