// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type serviceRepositoryStub struct {
	appendErr  error
	listFilter domainnotification.ListFilter
}

func (s *serviceRepositoryStub) Append(
	context.Context,
	domainnotification.Event,
) error {
	return s.appendErr
}

func (s *serviceRepositoryStub) AppendInTransaction(
	context.Context,
	*gorm.DB,
	domainnotification.Event,
) error {
	return s.appendErr
}

func (s *serviceRepositoryStub) ListForUser(
	_ context.Context,
	filter domainnotification.ListFilter,
) (domainnotification.ListPage, error) {
	s.listFilter = filter
	return domainnotification.ListPage{}, nil
}

func (s *serviceRepositoryStub) CountUnread(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *serviceRepositoryStub) MarkRead(
	context.Context,
	int64,
	[]int64,
	int64,
) (int64, error) {
	return 0, nil
}

func (s *serviceRepositoryStub) MarkAllRead(
	context.Context,
	int64,
	int64,
	int64,
) (int64, error) {
	return 0, nil
}

func TestAppendInTransactionReturnsRepositoryFailure(t *testing.T) {
	expected := errors.New("append failed")
	service := NewService(&serviceRepositoryStub{appendErr: expected})

	err := service.AppendInTransaction(
		context.Background(),
		&gorm.DB{},
		workerTestEvent(),
	)

	require.ErrorIs(t, err, expected)
}

func TestAppendRejectsUnsupportedPolicyBeforeRepository(t *testing.T) {
	repository := &serviceRepositoryStub{}
	service := NewService(repository)
	event := workerTestEvent()
	event.EventType = domainnotification.EventAppDevBuildSucceeded
	event.RecipientPolicy = domainnotification.RecipientResourceOwner

	err := service.Append(context.Background(), event)

	require.ErrorIs(t, err, domainnotification.ErrRecipientPolicyUnavailable)
}

func TestListAcceptsMaximumPageSizeAndReturnsSnapshotCutoff(t *testing.T) {
	repository := &serviceRepositoryStub{}
	service := NewService(repository)

	page, err := service.List(context.Background(), ListRequest{
		UserID: 42,
		Limit:  100,
	})

	require.NoError(t, err)
	require.Equal(t, 100, repository.listFilter.Limit)
	require.Zero(t, page.SnapshotCutoff)

	_, err = service.List(context.Background(), ListRequest{
		UserID: 42,
		Limit:  101,
	})
	require.ErrorIs(t, err, ErrInvalidPageSize)
}

func TestListReturnsStableInvalidCursorError(t *testing.T) {
	service := NewService(&serviceRepositoryStub{})

	_, err := service.List(context.Background(), ListRequest{
		UserID: 42,
		Cursor: "not-an-opaque-cursor",
		Limit:  20,
	})

	require.ErrorIs(t, err, ErrInvalidCursor)
}

func TestMarkAllReadRequiresExplicitSnapshotCutoff(t *testing.T) {
	service := NewService(&serviceRepositoryStub{})
	service.now = func() time.Time {
		return time.UnixMilli(1_721_000_000_000)
	}

	_, err := service.MarkAllRead(context.Background(), 42, 0)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func TestNotificationIDValidationUsesDedicatedError(t *testing.T) {
	_, err := ParseNotificationIDs([]string{"not-an-id"})
	require.ErrorIs(t, err, ErrInvalidNotificationID)

	service := NewService(&serviceRepositoryStub{})
	_, err = service.MarkRead(context.Background(), 101, []int64{0})
	require.ErrorIs(t, err, ErrInvalidNotificationID)
}
