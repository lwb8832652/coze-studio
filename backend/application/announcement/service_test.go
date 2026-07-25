// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type announcementRepositoryStub struct {
	createCommand       domainannouncement.CreateCommand
	publishCommand      domainannouncement.PublishCommand
	createResult        *domainannouncement.MutationResult
	publishResult       *domainannouncement.MutationResult
	advanceResult       *domainannouncement.AdvanceResult
	advanceErr          error
	advanceResults      map[int64]*domainannouncement.AdvanceResult
	advanceErrors       map[int64]error
	failedAnnouncement  *domainannouncement.Announcement
	failedCode          string
	blockMarkFailed     bool
	markFailedStarted   chan struct{}
	markFailedContextErr error
	replayCandidates    []int64
	getAnnouncement     *domainannouncement.Announcement
	getErr              error
	advanceCalls        int
	markFailedCalls     int
}

func (r *announcementRepositoryStub) Create(
	_ context.Context,
	command domainannouncement.CreateCommand,
) (*domainannouncement.MutationResult, error) {
	r.createCommand = command
	return r.createResult, nil
}

func (r *announcementRepositoryStub) Update(
	context.Context,
	domainannouncement.UpdateCommand,
) (*domainannouncement.Announcement, error) {
	return nil, nil
}

func (r *announcementRepositoryStub) Schedule(
	context.Context,
	domainannouncement.ScheduleCommand,
) (*domainannouncement.Announcement, error) {
	return nil, nil
}

func (r *announcementRepositoryStub) RequestPublish(
	_ context.Context,
	command domainannouncement.PublishCommand,
) (*domainannouncement.MutationResult, error) {
	r.publishCommand = command
	return r.publishResult, nil
}

func (r *announcementRepositoryStub) Cancel(
	context.Context,
	domainannouncement.CancelCommand,
) (*domainannouncement.Announcement, error) {
	return nil, nil
}

func (r *announcementRepositoryStub) Get(
	context.Context,
	int64,
) (*domainannouncement.Announcement, error) {
	return r.getAnnouncement, r.getErr
}

func (r *announcementRepositoryStub) List(
	context.Context,
	domainannouncement.ListFilter,
) ([]*domainannouncement.Announcement, int64, error) {
	return nil, 0, nil
}

func (r *announcementRepositoryStub) ListAuditEvents(
	context.Context,
	int64,
	int,
	int,
) ([]*domainannouncement.AuditEvent, int64, error) {
	return nil, 0, nil
}

func (r *announcementRepositoryStub) ListReplayCandidates(
	context.Context,
	time.Time,
	int64,
	int,
) ([]int64, error) {
	candidates := r.replayCandidates
	r.replayCandidates = nil
	return candidates, nil
}

func (r *announcementRepositoryStub) AdvancePublication(
	_ context.Context,
	announcementID int64,
	_ int64,
	_ time.Time,
	_ int,
) (*domainannouncement.AdvanceResult, error) {
	r.advanceCalls++
	if err, exists := r.advanceErrors[announcementID]; exists {
		return nil, err
	}
	if result, exists := r.advanceResults[announcementID]; exists {
		return result, nil
	}
	return r.advanceResult, r.advanceErr
}

func (r *announcementRepositoryStub) MarkProjectionFailed(
	ctx context.Context,
	_ int64,
	_ int64,
	errorCode string,
	_ time.Time,
) (*domainannouncement.Announcement, error) {
	r.markFailedCalls++
	r.failedCode = errorCode
	if r.blockMarkFailed {
		if r.markFailedStarted != nil {
			select {
			case <-r.markFailedStarted:
			default:
				close(r.markFailedStarted)
			}
		}
		<-ctx.Done()
		r.markFailedContextErr = ctx.Err()
		return nil, ctx.Err()
	}
	return r.failedAnnouncement, nil
}

func TestAnnouncementServiceRequiresSystemAdministrator(t *testing.T) {
	repository := &announcementRepositoryStub{}
	service := NewService(repository)

	_, err := service.Create(context.Background(), Actor{
		UserID:      42,
		SystemAdmin: false,
	}, CreateRequest{
		IdempotencyKey: "request.12345678",
		Draft:          validAnnouncementDraft(),
	})

	require.ErrorIs(t, err, domainannouncement.ErrPermissionDenied)
	require.Zero(t, repository.createCommand.ActorID)
}

func TestAnnouncementServiceRejectsUnsafeContentAndExternalRoute(t *testing.T) {
	service := NewService(&announcementRepositoryStub{})
	actor := Actor{UserID: 42, SystemAdmin: true}

	unsafe := validAnnouncementDraft()
	unsafe.Body = `<script>alert("x")</script>`
	_, err := service.Create(context.Background(), actor, CreateRequest{
		IdempotencyKey: "request.12345678",
		Draft:          unsafe,
	})
	require.ErrorIs(t, err, domainannouncement.ErrUnsafeContent)

	for index, route := range []domainnotification.AnnouncementRoute{
		{Type: "https"},
		{Type: domainnotification.AnnouncementRouteWorkspaceHome},
		{
			Type:    domainnotification.AnnouncementRouteSystemAnnouncements,
			SpaceID: 99,
		},
	} {
		unsafeRoute := validAnnouncementDraft()
		unsafeRoute.Route = route
		_, err = service.Create(context.Background(), actor, CreateRequest{
			IdempotencyKey: fmt.Sprintf("request.route.%02d", index),
			Draft:          unsafeRoute,
		})
		require.ErrorIs(t, err, domainannouncement.ErrInvalidInput)
	}

	secret := validAnnouncementDraft()
	secret.Body = "credential = do-not-store"
	_, err = service.Create(context.Background(), actor, CreateRequest{
		IdempotencyKey: "request.secret01",
		Draft:          secret,
	})
	require.ErrorIs(t, err, domainannouncement.ErrUnsafeContent)
}

func TestAnnouncementAcceptedAtCreateIsAppendableByNotificationProjection(
	t *testing.T,
) {
	draft := validAnnouncementDraft()
	draft.Title = "Prompt 配置维护"
	draft.Body = "checkpoint 状态将在维护结束后继续保留。"
	normalized, err := domainannouncement.NormalizeDraft(draft)
	require.NoError(t, err)

	event := domainnotification.Event{
		EventID:          "announcement-1-batch-1",
		EventType:        domainnotification.EventSystemAnnouncement,
		AggregateType:    "announcement",
		AggregateID:      "1",
		AggregateVersion: 1,
		OccurredAt:       time.UnixMilli(1_800_000_000_000),
		ActorID:          42,
		RecipientPolicy:  domainnotification.RecipientExplicitInternalUsers,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			AnnouncementTitle:    normalized.Title,
			AnnouncementBody:     normalized.Body,
			AnnouncementSeverity: domainnotification.Severity(normalized.Severity),
			AnnouncementRoute:    &normalized.Route,
			ExplicitRecipientIDs: []int64{101},
		},
	}
	require.NoError(
		t,
		domainnotification.DefaultTemplateRegistry().ValidateAppendable(event),
	)
}

func TestAnnouncementPublishReturnsRecoverableStateAfterDurableProjectionFailure(
	t *testing.T,
) {
	pending := &domainannouncement.Announcement{
		ID:               91,
		Status:           domainannouncement.StatusDraft,
		ProjectionStatus: domainannouncement.ProjectionSnapshotting,
		Version:          2,
	}
	failed := *pending
	failed.ProjectionStatus = domainannouncement.ProjectionFailed
	failed.LastErrorCode = domainannouncement.ErrorCodeStorage
	repository := &announcementRepositoryStub{
		publishResult: &domainannouncement.MutationResult{
			Announcement: pending,
		},
		advanceErr: errors.Join(
			domainannouncement.ErrStorage,
			errors.New("mysql failed with password=secret"),
		),
		failedAnnouncement: &failed,
	}
	service := NewService(repository)
	service.now = func() time.Time {
		return time.UnixMilli(1_800_000_000_000)
	}

	result, err := service.Publish(
		context.Background(),
		Actor{UserID: 42, SystemAdmin: true},
		PublishRequest{
			AnnouncementID: 91,
			ExpectedVersion: 1,
			IdempotencyKey:  "publish.12345678",
		},
	)

	require.NoError(t, err)
	require.True(t, result.Deferred)
	require.Equal(t, ProjectionRetryCode, result.ErrorCode)
	require.Equal(t, domainannouncement.ProjectionFailed, result.Announcement.ProjectionStatus)
	require.Equal(t, domainannouncement.ErrorCodeStorage, repository.failedCode)
	require.Equal(t, int64(42), repository.publishCommand.ActorID)
	require.NotContains(t, result.ErrorCode, "password")
}

func TestAnnouncementReplayReturnsNotFoundWithoutAdvancing(t *testing.T) {
	repository := &announcementRepositoryStub{
		getErr: domainannouncement.ErrNotFound,
	}
	service := NewService(repository)

	_, err := service.Replay(
		context.Background(),
		Actor{UserID: 42, SystemAdmin: true},
		404,
	)

	require.ErrorIs(t, err, domainannouncement.ErrNotFound)
	require.Zero(t, repository.advanceCalls)
	require.Zero(t, repository.markFailedCalls)
}

func TestAnnouncementReplayDoesNotDeferBusinessConflicts(t *testing.T) {
	current := &domainannouncement.Announcement{
		ID:               91,
		Status:           domainannouncement.StatusDraft,
		ProjectionStatus: domainannouncement.ProjectionIdle,
	}
	repository := &announcementRepositoryStub{
		getAnnouncement: current,
		advanceErr:      domainannouncement.ErrStateConflict,
	}
	service := NewService(repository)

	_, err := service.Replay(
		context.Background(),
		Actor{UserID: 42, SystemAdmin: true},
		current.ID,
	)

	require.ErrorIs(t, err, domainannouncement.ErrStateConflict)
	require.Equal(t, 1, repository.advanceCalls)
	require.Zero(t, repository.markFailedCalls)
}

func TestAnnouncementPublishDoesNotDeferValidationOrNotFoundErrors(
	t *testing.T,
) {
	for _, advanceErr := range []error{
		domainannouncement.ErrInvalidInput,
		domainannouncement.ErrNotFound,
		domainannouncement.ErrVersionConflict,
		domainannouncement.ErrPermissionDenied,
	} {
		repository := &announcementRepositoryStub{
			publishResult: &domainannouncement.MutationResult{
				Announcement: &domainannouncement.Announcement{
					ID:               91,
					Status:           domainannouncement.StatusDraft,
					ProjectionStatus: domainannouncement.ProjectionSnapshotting,
				},
			},
			advanceErr: advanceErr,
		}
		service := NewService(repository)

		_, err := service.Publish(
			context.Background(),
			Actor{UserID: 42, SystemAdmin: true},
			PublishRequest{
				AnnouncementID: 91,
				ExpectedVersion: 1,
				IdempotencyKey:  "publish.business.1",
			},
		)

		require.ErrorIs(t, err, advanceErr)
		require.Zero(t, repository.markFailedCalls)
	}
}

func TestAnnouncementReplayProcessesDurableCandidates(t *testing.T) {
	repository := &announcementRepositoryStub{
		replayCandidates: []int64{11, 12},
		advanceResult: &domainannouncement.AdvanceResult{
			Announcement: &domainannouncement.Announcement{
				Status:           domainannouncement.StatusPublished,
				ProjectionStatus: domainannouncement.ProjectionCompleted,
			},
			Done:       true,
			Progressed: true,
		},
	}
	service := NewService(repository)

	result, err := service.Replay(
		context.Background(),
		Actor{UserID: 42, SystemAdmin: true},
		0,
	)

	require.NoError(t, err)
	require.Equal(t, 2, result.Processed)
	require.Equal(t, 2, result.Completed)
	require.Zero(t, result.Deferred)
}

func TestAnnouncementReplayContinuesAfterCandidateFailure(t *testing.T) {
	completed := &domainannouncement.AdvanceResult{
		Announcement: &domainannouncement.Announcement{
			ID:               12,
			Status:           domainannouncement.StatusPublished,
			ProjectionStatus: domainannouncement.ProjectionCompleted,
		},
		Done:       true,
		Progressed: true,
	}
	repository := &announcementRepositoryStub{
		replayCandidates: []int64{11, 12},
		advanceErrors: map[int64]error{
			11: domainannouncement.ErrStateConflict,
		},
		advanceResults: map[int64]*domainannouncement.AdvanceResult{
			12: completed,
		},
	}
	service := NewService(repository)

	result, err := service.Replay(
		context.Background(),
		Actor{UserID: 42, SystemAdmin: true},
		0,
	)

	require.NoError(t, err)
	require.Equal(t, 2, result.Processed)
	require.Equal(t, 1, result.Completed)
	require.Equal(t, 1, result.Failed)
	require.Equal(
		t,
		1,
		result.ErrorCodes[domainannouncement.ErrorCodeStateConflict],
	)
}

func TestAnnouncementProjectionFailurePersistenceHasHardDeadline(t *testing.T) {
	pending := &domainannouncement.Announcement{
		ID:               91,
		Status:           domainannouncement.StatusDraft,
		ProjectionStatus: domainannouncement.ProjectionSnapshotting,
		Version:          2,
	}
	repository := &announcementRepositoryStub{
		publishResult: &domainannouncement.MutationResult{
			Announcement: pending,
		},
		advanceErr:      domainannouncement.ErrStorage,
		blockMarkFailed: true,
	}
	service := NewService(repository)
	service.failurePersistenceTimeout = 20 * time.Millisecond
	startedAt := time.Now()

	result, err := service.Publish(
		context.Background(),
		Actor{UserID: 42, SystemAdmin: true},
		PublishRequest{
			AnnouncementID: 91,
			ExpectedVersion: 1,
			IdempotencyKey:  "publish.timeout.1",
		},
	)

	require.NoError(t, err)
	require.True(t, result.Deferred)
	require.ErrorIs(t, repository.markFailedContextErr, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}

func TestAnnouncementWorkerShutdownIsBoundedByFailurePersistenceDeadline(
	t *testing.T,
) {
	markFailedStarted := make(chan struct{})
	repository := &announcementRepositoryStub{
		replayCandidates:  []int64{91},
		advanceErr:         domainannouncement.ErrStorage,
		blockMarkFailed:    true,
		markFailedStarted:  markFailedStarted,
	}
	service := NewService(repository)
	service.failurePersistenceTimeout = 20 * time.Millisecond
	worker := NewWorker(service, WorkerOptions{PollInterval: time.Hour})
	worker.Start(context.Background())
	select {
	case <-markFailedStarted:
	case <-time.After(time.Second):
		t.Fatal("projection failure persistence did not start")
	}
	shutdownContext, cancel := context.WithTimeout(
		context.Background(),
		500*time.Millisecond,
	)
	defer cancel()
	startedAt := time.Now()

	err := worker.Shutdown(shutdownContext)

	require.NoError(t, err)
	require.ErrorIs(t, repository.markFailedContextErr, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), 250*time.Millisecond)
}

func TestAnnouncementScheduleRejectsPastAndUnboundedTimes(t *testing.T) {
	service := NewService(&announcementRepositoryStub{})
	now := time.UnixMilli(1_800_000_000_000)
	service.now = func() time.Time { return now }
	actor := Actor{UserID: 42, SystemAdmin: true}

	_, err := service.Schedule(context.Background(), actor, ScheduleRequest{
		AnnouncementID: 1,
		ExpectedVersion: 1,
		ScheduledAt:     now,
	})
	require.ErrorIs(t, err, domainannouncement.ErrInvalidInput)

	_, err = service.Schedule(context.Background(), actor, ScheduleRequest{
		AnnouncementID: 1,
		ExpectedVersion: 1,
		ScheduledAt:     now.Add(domainannouncement.MaxScheduleHorizon + time.Second),
	})
	require.ErrorIs(t, err, domainannouncement.ErrInvalidInput)
}

func validAnnouncementDraft() domainannouncement.Draft {
	return domainannouncement.Draft{
		Title:    "计划维护通知",
		Body:     "服务将在今晚进行维护，请提前保存工作。",
		Severity: domainannouncement.SeverityWarning,
		Route: domainnotification.AnnouncementRoute{
			Type: domainnotification.AnnouncementRouteSystemAnnouncements,
		},
		Audience: domainannouncement.Audience{
			Type: domainannouncement.AudienceAll,
		},
	}
}
