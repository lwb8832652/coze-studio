// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
	maxMarkReadBatch = 100
)

var (
	ErrInvalidRequest        = errors.New("invalid notification request")
	ErrInvalidCursor         = errors.New("invalid notification cursor")
	ErrInvalidReadMode       = errors.New("invalid notification read mode")
	ErrInvalidPageSize       = errors.New("invalid notification page size")
	ErrInvalidNotificationID = errors.New("invalid notification ID")
	ErrUnauthenticated       = errors.New("notification authentication required")
	ErrForbidden             = errors.New("notification access forbidden")
)

type Repository interface {
	Append(context.Context, domainnotification.Event) error
	AppendInTransaction(context.Context, *gorm.DB, domainnotification.Event) error
	ListForUser(context.Context, domainnotification.ListFilter) (domainnotification.ListPage, error)
	CountUnread(context.Context, int64) (int64, error)
	MarkRead(context.Context, int64, []int64, int64) (int64, error)
	MarkAllRead(context.Context, int64, int64, int64) (int64, error)
}

type ListRequest struct {
	UserID     int64
	Cursor     string
	Limit      int32
	UnreadOnly bool
}

type Notification struct {
	ID        string
	SpaceID   string
	Title     string
	Content   string
	Category  domainnotification.Category
	Severity  domainnotification.Severity
	Route     domainnotification.TargetType
	TargetID  string
	Read      bool
	CreatedAt int64
}

type Page struct {
	Items          []Notification
	NextCursor     string
	HasMore        bool
	SnapshotCutoff string
}

type Service struct {
	repository Repository
	registry   *domainnotification.TemplateRegistry
	now        func() time.Time
}

var SVC = NewService(nil)

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
		registry:   domainnotification.DefaultTemplateRegistry(),
		now:        time.Now,
	}
}

func SetDefaultService(service *Service) {
	if service == nil {
		SVC = NewService(nil)
		return
	}
	SVC = service
}

func (s *Service) IsConfigured() bool {
	return s != nil && s.repository != nil && s.registry != nil
}

func (s *Service) Append(
	ctx context.Context,
	event domainnotification.Event,
) error {
	if s == nil || s.repository == nil || s.registry == nil {
		return domainnotification.ErrStorage
	}
	event, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return err
	}
	if err := s.registry.ValidateAppendable(event); err != nil {
		return err
	}
	return s.repository.Append(ctx, event)
}

func (s *Service) AppendInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	if s == nil || s.repository == nil || s.registry == nil {
		return domainnotification.ErrStorage
	}
	event, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return err
	}
	if err := s.registry.ValidateAppendable(event); err != nil {
		return err
	}
	return s.repository.AppendInTransaction(ctx, tx, event)
}

func (s *Service) List(
	ctx context.Context,
	request ListRequest,
) (Page, error) {
	if request.UserID <= 0 {
		return Page{}, ErrUnauthenticated
	}
	if s == nil || s.repository == nil {
		return Page{}, domainnotification.ErrStorage
	}
	if request.Limit < 0 || request.Limit > maxListLimit {
		return Page{}, ErrInvalidPageSize
	}
	limit := int(request.Limit)
	if limit == 0 {
		limit = defaultListLimit
	}
	cursor, err := domainnotification.DecodeCursor(strings.TrimSpace(request.Cursor))
	if err != nil {
		return Page{}, ErrInvalidCursor
	}
	result, err := s.repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID:     request.UserID,
		Cursor:     cursor,
		HasCursor:  cursor.CreatedAt > 0,
		Limit:      limit,
		UnreadOnly: request.UnreadOnly,
	})
	if err != nil {
		return Page{}, err
	}
	page := Page{
		Items:   make([]Notification, 0, len(result.Items)),
		HasMore: result.HasMore,
	}
	if result.SnapshotCutoff > 0 {
		page.SnapshotCutoff = strconv.FormatInt(result.SnapshotCutoff, 10)
	}
	if result.HasMore {
		page.NextCursor = domainnotification.EncodeCursor(result.NextCursor)
	}
	for _, row := range result.Items {
		item := Notification{
			ID:        strconv.FormatInt(row.Message.ID, 10),
			Title:     row.Message.Title,
			Content:   row.Message.Content,
			Category:  row.Message.Category,
			Severity:  row.Message.Severity,
			Route:     row.Message.TargetType,
			TargetID:  row.Message.TargetID,
			Read:      row.ReadAt > 0,
			CreatedAt: row.Message.CreatedAt,
		}
		if row.Message.SpaceID > 0 {
			item.SpaceID = strconv.FormatInt(row.Message.SpaceID, 10)
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func (s *Service) GetUnreadCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, ErrUnauthenticated
	}
	if s == nil || s.repository == nil {
		return 0, domainnotification.ErrStorage
	}
	return s.repository.CountUnread(ctx, userID)
}

func (s *Service) MarkRead(
	ctx context.Context,
	userID int64,
	notificationIDs []int64,
) (int64, error) {
	if userID <= 0 {
		return 0, ErrUnauthenticated
	}
	if s == nil || s.repository == nil {
		return 0, domainnotification.ErrStorage
	}
	if len(notificationIDs) == 0 ||
		len(notificationIDs) > maxMarkReadBatch {
		return 0, ErrInvalidNotificationID
	}
	ids := make([]int64, 0, len(notificationIDs))
	seen := make(map[int64]struct{}, len(notificationIDs))
	for _, id := range notificationIDs {
		if id <= 0 {
			return 0, ErrInvalidNotificationID
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return s.repository.MarkRead(ctx, userID, ids, s.nowTime().UnixMilli())
}

func (s *Service) MarkAllRead(
	ctx context.Context,
	userID int64,
	cutoff int64,
) (int64, error) {
	if userID <= 0 {
		return 0, ErrUnauthenticated
	}
	if cutoff <= 0 {
		return 0, ErrInvalidRequest
	}
	if s == nil || s.repository == nil {
		return 0, domainnotification.ErrStorage
	}
	return s.repository.MarkAllRead(
		ctx,
		userID,
		cutoff,
		s.nowTime().UnixMilli(),
	)
}

func (s *Service) nowTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func ParseNotificationIDs(values []string) ([]int64, error) {
	if len(values) == 0 || len(values) > maxMarkReadBatch {
		return nil, ErrInvalidNotificationID
	}
	result := make([]int64, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, ErrInvalidNotificationID
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, ErrInvalidNotificationID
		}
		result = append(result, id)
	}
	return result, nil
}

func WrapInvalidRequest(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
}
