// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	noticeRankAll    = int32(0)
	noticeRankUnread = int32(1)
	readStatusUnread = int32(1)
	readStatusRead   = int32(2)

	noticeReadModeIDs      = int32(1)
	noticeReadModeSnapshot = int32(2)

	noticeSeverityInfo    = int32(1)
	noticeSeveritySuccess = int32(2)
	noticeSeverityWarning = int32(3)
	noticeSeverityError   = int32(4)

	noticeCategoryTask          = int32(1)
	noticeCategoryScheduledTask = int32(2)
	noticeCategoryAppDev        = int32(3)
	noticeCategoryMCP           = int32(4)
	noticeCategoryResource      = int32(5)
	noticeCategoryWorkspace     = int32(6)
	noticeCategoryIM            = int32(7)
	noticeCategoryBilling       = int32(8)
	noticeCategorySystem        = int32(9)

	noticeRouteNone                = int32(0)
	noticeRouteTaskThread          = int32(1)
	noticeRouteScheduledTaskCenter = int32(2)
	noticeRouteAppDev              = int32(3)
	noticeRouteSkill               = int32(4)
	noticeRouteWorkspace           = int32(5)
	noticeRouteBilling             = int32(6)
	noticeRouteSystemAnnouncements = int32(7)

	notificationBusinessCodeSuccess = int64(0)
	notificationBusinessCodeFailure = int64(1)

	notificationCodeInvalidCursor   = "NOTIFICATION_INVALID_CURSOR"
	notificationCodeInvalidReadMode = "NOTIFICATION_INVALID_READ_MODE"
	notificationCodeInvalidPageSize = "NOTIFICATION_INVALID_PAGE_SIZE"
	notificationCodeInvalidRequest  = "NOTIFICATION_INVALID_REQUEST"
	notificationCodeInvalidNotificationID = "NOTIFICATION_INVALID_NOTIFICATION_ID"
	notificationCodeForbidden       = "NOTIFICATION_FORBIDDEN"
	notificationCodeInternal        = "NOTIFICATION_INTERNAL"
)

type notificationHTTPService interface {
	List(context.Context, appnotification.ListRequest) (appnotification.Page, error)
	GetUnreadCount(context.Context, int64) (int64, error)
	MarkRead(context.Context, int64, []int64) (int64, error)
	MarkAllRead(context.Context, int64, int64) (int64, error)
}

type notificationHTTPHandler struct {
	Service  notificationHTTPService
	ViewerID func(context.Context) int64
}

type getNoticeListRequest struct {
	Cursor         string `json:"cursor"`
	Count          int32  `json:"count"`
	NoticeRankType int32  `json:"notice_rank_type"`
}

type noticeMarkReadRequest struct {
	NoticeIDs      []string `json:"notice_ids"`
	ReadMode       int32    `json:"read_mode"`
	SnapshotCutoff string   `json:"snapshot_cutoff"`
}

type noticeSenderResponse struct {
	SenderType    int32  `json:"sender_type"`
	SenderID      string `json:"sender_id"`
	SenderName    string `json:"sender_name"`
	SenderIconURL string `json:"sender_icon_url"`
}

type noticeResponse struct {
	ID         string               `json:"id"`
	Content    string               `json:"content"`
	ReadStatus int32                `json:"read_status"`
	Sender     noticeSenderResponse `json:"sender"`
	CreateTime string               `json:"create_time"`
	Severity   int32                `json:"severity"`
	Category   int32                `json:"category"`
	Route      int32                `json:"route"`
	RouteSpaceID string             `json:"route_space_id,omitempty"`
	RouteTargetID string            `json:"route_target_id,omitempty"`
}

func GetNoticeList(ctx context.Context, c *app.RequestContext) {
	defaultNotificationHTTPHandler().List(ctx, c)
}

func GetNoticeUnreadCount(ctx context.Context, c *app.RequestContext) {
	defaultNotificationHTTPHandler().UnreadCount(ctx, c)
}

func NoticeMarkRead(ctx context.Context, c *app.RequestContext) {
	defaultNotificationHTTPHandler().MarkRead(ctx, c)
}

func defaultNotificationHTTPHandler() *notificationHTTPHandler {
	return &notificationHTTPHandler{
		Service:  appnotification.SVC,
		ViewerID: workbenchViewerIDFromCtx,
	}
}

func (h *notificationHTTPHandler) List(ctx context.Context, c *app.RequestContext) {
	userID, ok := h.authenticatedUserID(ctx, c)
	if !ok {
		return
	}
	var request getNoticeListRequest
	if err := c.BindAndValidate(&request); err != nil {
		notificationErrorResponse(ctx, c, appnotification.ErrInvalidRequest)
		return
	}
	if request.Cursor == "" {
		request.Cursor = "0"
	}
	if request.NoticeRankType != noticeRankAll &&
		request.NoticeRankType != noticeRankUnread {
		notificationErrorResponse(ctx, c, appnotification.ErrInvalidRequest)
		return
	}
	page, err := h.Service.List(ctx, appnotification.ListRequest{
		UserID:     userID,
		Cursor:     request.Cursor,
		Limit:      request.Count,
		UnreadOnly: request.NoticeRankType == noticeRankUnread,
	})
	if err != nil {
		notificationErrorResponse(ctx, c, err)
		return
	}
	notices := make([]noticeResponse, 0, len(page.Items))
	for _, item := range page.Items {
		notices = append(notices, notificationToNotice(item))
	}
	notificationJSONSuccess(c, map[string]any{
		"notice_list":     notices,
		"next_cursor":     page.NextCursor,
		"has_more":        page.HasMore,
		"snapshot_cutoff": page.SnapshotCutoff,
	})
}

func (h *notificationHTTPHandler) UnreadCount(
	ctx context.Context,
	c *app.RequestContext,
) {
	userID, ok := h.authenticatedUserID(ctx, c)
	if !ok {
		return
	}
	count, err := h.Service.GetUnreadCount(ctx, userID)
	if err != nil {
		notificationErrorResponse(ctx, c, err)
		return
	}
	notificationJSONSuccess(c, map[string]any{"unread_count": count})
}

func (h *notificationHTTPHandler) MarkRead(
	ctx context.Context,
	c *app.RequestContext,
) {
	userID, ok := h.authenticatedUserID(ctx, c)
	if !ok {
		return
	}
	var request noticeMarkReadRequest
	if err := c.BindAndValidate(&request); err != nil {
		notificationErrorResponse(ctx, c, appnotification.ErrInvalidRequest)
		return
	}
	switch request.ReadMode {
	case noticeReadModeIDs:
		if len(request.NoticeIDs) == 0 || request.SnapshotCutoff != "" {
			notificationErrorResponse(ctx, c, appnotification.ErrInvalidReadMode)
			return
		}
		ids, err := appnotification.ParseNotificationIDs(request.NoticeIDs)
		if err != nil {
			notificationErrorResponse(ctx, c, err)
			return
		}
		if _, err := h.Service.MarkRead(ctx, userID, ids); err != nil {
			notificationErrorResponse(ctx, c, err)
			return
		}
		notificationJSONSuccess(c, nil)
	case noticeReadModeSnapshot:
		if len(request.NoticeIDs) > 0 {
			notificationErrorResponse(ctx, c, appnotification.ErrInvalidReadMode)
			return
		}
		cutoff, err := strconv.ParseInt(request.SnapshotCutoff, 10, 64)
		if err != nil || cutoff <= 0 {
			notificationErrorResponse(ctx, c, appnotification.ErrInvalidRequest)
			return
		}
		if _, err := h.Service.MarkAllRead(ctx, userID, cutoff); err != nil {
			notificationErrorResponse(ctx, c, err)
			return
		}
		notificationJSONSuccess(c, nil)
	default:
		notificationErrorResponse(ctx, c, appnotification.ErrInvalidReadMode)
	}
}

func (h *notificationHTTPHandler) authenticatedUserID(
	ctx context.Context,
	c *app.RequestContext,
) (int64, bool) {
	if h == nil || h.Service == nil || h.ViewerID == nil {
		notificationErrorResponse(ctx, c, domainnotification.ErrStorage)
		return 0, false
	}
	userID := h.ViewerID(ctx)
	if userID <= 0 {
		notificationErrorResponse(ctx, c, appnotification.ErrForbidden)
		return 0, false
	}
	return userID, true
}

func notificationToNotice(item appnotification.Notification) noticeResponse {
	status := readStatusUnread
	if item.Read {
		status = readStatusRead
	}
	route := notificationRouteValue(item.Route)
	routeSpaceID := item.SpaceID
	routeTargetID := item.TargetID
	if item.Route == domainnotification.TargetInternalRoute {
		route, routeSpaceID, routeTargetID = legacyNotificationRoute(
			item.TargetID,
		)
	}
	if item.Route == domainnotification.TargetSystemAnnouncements {
		routeSpaceID = ""
		routeTargetID = ""
	}
	return noticeResponse{
		ID:         item.ID,
		Content:    item.Content,
		ReadStatus: status,
		Sender: noticeSenderResponse{
			SenderType: 1,
			SenderID:   "0",
			SenderName: item.Title,
		},
		CreateTime: strconv.FormatInt(item.CreatedAt, 10),
		Severity:   notificationSeverityValue(item.Severity),
		Category:   notificationCategoryValue(item.Category),
		Route:         route,
		RouteSpaceID:  routeSpaceID,
		RouteTargetID: routeTargetID,
	}
}

func notificationSeverityValue(value domainnotification.Severity) int32 {
	switch value {
	case domainnotification.SeveritySuccess:
		return noticeSeveritySuccess
	case domainnotification.SeverityWarning:
		return noticeSeverityWarning
	case domainnotification.SeverityError:
		return noticeSeverityError
	default:
		return noticeSeverityInfo
	}
}

func notificationCategoryValue(value domainnotification.Category) int32 {
	switch value {
	case domainnotification.CategoryScheduledTask:
		return noticeCategoryScheduledTask
	case domainnotification.CategoryAppDev:
		return noticeCategoryAppDev
	case domainnotification.CategoryMCP:
		return noticeCategoryMCP
	case domainnotification.CategoryResource:
		return noticeCategoryResource
	case domainnotification.CategoryWorkspace:
		return noticeCategoryWorkspace
	case domainnotification.CategoryIM:
		return noticeCategoryIM
	case domainnotification.CategoryBilling:
		return noticeCategoryBilling
	case domainnotification.CategorySystem:
		return noticeCategorySystem
	default:
		return noticeCategoryTask
	}
}

func notificationRouteValue(value domainnotification.TargetType) int32 {
	switch value {
	case domainnotification.TargetTaskThread:
		return noticeRouteTaskThread
	case domainnotification.TargetScheduledTaskCenter:
		return noticeRouteScheduledTaskCenter
	case domainnotification.TargetAppDev:
		return noticeRouteAppDev
	case domainnotification.TargetSkill:
		return noticeRouteSkill
	case domainnotification.TargetWorkspace:
		return noticeRouteWorkspace
	case domainnotification.TargetBilling:
		return noticeRouteBilling
	case domainnotification.TargetSystemAnnouncements:
		return noticeRouteSystemAnnouncements
	default:
		return noticeRouteNone
	}
}

func legacyNotificationRoute(value string) (int32, string, string) {
	value = strings.TrimSpace(value)
	if value == "/system/announcements" {
		return noticeRouteSystemAnnouncements, "", ""
	}
	const prefix = "/space/"
	const suffix = "/workspace"
	if strings.HasPrefix(value, prefix) && strings.HasSuffix(value, suffix) {
		rawID := strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
		spaceID, err := strconv.ParseInt(rawID, 10, 64)
		if err == nil &&
			spaceID > 0 &&
			strconv.FormatInt(spaceID, 10) == rawID {
			return noticeRouteWorkspace, rawID, ""
		}
	}
	return noticeRouteNone, "", ""
}

func notificationJSONSuccess(c *app.RequestContext, data any) {
	c.JSON(http.StatusOK, map[string]any{
		"code":       notificationBusinessCodeSuccess,
		"error_code": "",
		"msg":        "success",
		"data":       data,
	})
}

func notificationErrorResponse(
	ctx context.Context,
	c *app.RequestContext,
	err error,
) {
	status := http.StatusInternalServerError
	code := notificationCodeInternal
	message := "通知服务暂时不可用"
	switch {
	case errors.Is(err, appnotification.ErrInvalidCursor),
		errors.Is(err, domainnotification.ErrInvalidCursor):
		status = http.StatusBadRequest
		code = notificationCodeInvalidCursor
		message = "通知游标无效"
	case errors.Is(err, appnotification.ErrInvalidReadMode):
		status = http.StatusBadRequest
		code = notificationCodeInvalidReadMode
		message = "通知已读模式无效"
	case errors.Is(err, appnotification.ErrInvalidPageSize):
		status = http.StatusBadRequest
		code = notificationCodeInvalidPageSize
		message = "通知分页大小无效"
	case errors.Is(err, appnotification.ErrInvalidRequest),
		errors.Is(err, domainnotification.ErrInvalidEvent):
		status = http.StatusBadRequest
		code = notificationCodeInvalidRequest
		message = "通知请求参数无效"
	case errors.Is(err, appnotification.ErrInvalidNotificationID):
		status = http.StatusBadRequest
		code = notificationCodeInvalidNotificationID
		message = "通知 ID 无效"
	case errors.Is(err, appnotification.ErrUnauthenticated),
		errors.Is(err, appnotification.ErrForbidden):
		status = http.StatusForbidden
		code = notificationCodeForbidden
		message = "无权访问通知"
	default:
		logs.CtxErrorf(
			ctx,
			"[notification] request_failed error_code=%s",
			domainnotification.StableErrorCode(err),
		)
	}
	c.JSON(status, map[string]any{
		"code":       notificationBusinessCodeFailure,
		"error_code": code,
		"msg":        message,
	})
}
