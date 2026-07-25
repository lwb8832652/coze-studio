// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type spaceUserTestPO struct {
	ID       int64 `gorm:"column:id;primaryKey"`
	SpaceID  int64 `gorm:"column:space_id"`
	UserID   int64 `gorm:"column:user_id"`
	RoleType int32 `gorm:"column:role_type"`
}

func (spaceUserTestPO) TableName() string { return "space_user" }

type configTestPO struct {
	domain.Config
}

func (configTestPO) TableName() string { return configTable }

type eventTestPO struct {
	domain.Event
}

func (eventTestPO) TableName() string { return eventTable }

func TestRepositoryRecordsStableFailureOnlyAfterThreshold(t *testing.T) {
	db, repo, outbox := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{})
	insertIMConfig(t, db, imConfigFixture())
	insertSpaceUsers(t, db,
		spaceUserTestPO{ID: 1, SpaceID: 20, UserID: 9, RoleType: workspaceRoleOwner},
		spaceUserTestPO{ID: 2, SpaceID: 20, UserID: 8, RoleType: workspaceRoleAdmin},
		spaceUserTestPO{ID: 3, SpaceID: 20, UserID: 2, RoleType: 3},
	)

	state := domain.RuntimeState{
		Status:     domain.RuntimeStatusError,
		Error:      "connection_failed",
		IncidentID: "incident-threshold",
	}
	require.NoError(t, repo.RecordRuntimeFailure(context.Background(), 10, state, 4))
	require.NoError(t, repo.RecordRuntimeFailure(context.Background(), 10, state, 4))
	require.NoError(t, repo.RecordRuntimeFailure(context.Background(), 10, state, 4))
	require.Empty(t, outbox.events)

	require.NoError(t, repo.RecordRuntimeFailure(context.Background(), 10, state, 4))
	require.Len(t, outbox.events, 1)
	require.Equal(t, domainnotification.EventIMChannelConnectionFailed, outbox.events[0].EventType)
	require.Equal(t, []int64{2, 8, 9}, outbox.events[0].Payload.ExplicitRecipientIDs)
	require.Equal(t, "20", outbox.events[0].Payload.TargetID)
}

func TestRepositoryRollsBackStableFailureWhenOutboxAppendFails(t *testing.T) {
	db, repo, _ := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{
		err: errors.New("outbox unavailable"),
	})
	config := imConfigFixture()
	config.RuntimeConsecutiveFailures = 2
	insertIMConfig(t, db, config)
	insertSpaceUsers(t, db,
		spaceUserTestPO{ID: 1, SpaceID: 20, UserID: 9, RoleType: workspaceRoleOwner},
	)

	err := repo.RecordRuntimeFailure(context.Background(), 10, domain.RuntimeState{
		Status:     domain.RuntimeStatusError,
		Error:      "connection_failed",
		IncidentID: "incident-rollback",
	}, domain.DefaultRuntimeStableFailureThreshold)
	require.Error(t, err)

	var got domain.Config
	require.NoError(t, db.Table(configTable).Where("id = ?", int64(10)).Take(&got).Error)
	require.Equal(t, int32(2), got.RuntimeConsecutiveFailures)
	require.Empty(t, got.RuntimeIncidentID)
	require.Nil(t, got.RuntimeIncidentNotifiedAt)
}

func TestRepositoryRuntimeRecoveryNotificationIsIdempotent(t *testing.T) {
	db, repo, outbox := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{})
	notifiedAt := time.Unix(100, 0)
	config := imConfigFixture()
	config.RuntimeConsecutiveFailures = 3
	config.RuntimeIncidentID = "incident-recover"
	config.RuntimeIncidentNotifiedAt = &notifiedAt
	insertIMConfig(t, db, config)
	insertSpaceUsers(t, db,
		spaceUserTestPO{ID: 1, SpaceID: 20, UserID: 9, RoleType: workspaceRoleOwner},
		spaceUserTestPO{ID: 2, SpaceID: 20, UserID: 2, RoleType: 3},
	)

	connectedAt := time.Unix(200, 0)
	state := domain.RuntimeState{
		Status:      domain.RuntimeStatusConnected,
		ConnectedAt: &connectedAt,
	}
	require.NoError(t, repo.RecordRuntimeRecovery(context.Background(), 10, state))
	require.NoError(t, repo.RecordRuntimeRecovery(context.Background(), 10, state))

	require.Len(t, outbox.events, 1)
	require.Equal(t, domainnotification.EventIMChannelRecovered, outbox.events[0].EventType)
	require.Equal(t, []int64{2, 9}, outbox.events[0].Payload.ExplicitRecipientIDs)

	var got domain.Config
	require.NoError(t, db.Table(configTable).Where("id = ?", int64(10)).Take(&got).Error)
	require.Equal(t, int32(0), got.RuntimeConsecutiveFailures)
	require.Empty(t, got.RuntimeIncidentID)
	require.Nil(t, got.RuntimeIncidentNotifiedAt)
	require.NotNil(t, got.RuntimeRecoveryNotifiedAt)
	require.NotNil(t, got.RuntimeLastRecoveredAt)
}

func TestRepositoryDeadLetterClearsPayloadAndAppendsOnce(t *testing.T) {
	db, repo, outbox := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{})
	config := imConfigFixture()
	insertIMConfig(t, db, config)
	insertSpaceUsers(t, db,
		spaceUserTestPO{ID: 1, SpaceID: 20, UserID: 9, RoleType: workspaceRoleOwner},
		spaceUserTestPO{ID: 2, SpaceID: 20, UserID: 2, RoleType: 3},
	)
	event := &domain.Event{
		ID:           100,
		ConfigID:     10,
		EventKey:     "event-dead",
		MessageID:    "message-dead",
		PayloadJSON:  `{"content":"must not leak"}`,
		Status:       domain.EventStatusProcessing,
		AttemptCount: domain.EventMaxAttempts - 1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, db.Table(eventTable).Create(event).Error)

	deadLettered, err := repo.DeadLetterEvent(context.Background(), config, event, "agent_execution_failed", time.Unix(300, 0))
	require.NoError(t, err)
	require.True(t, deadLettered)
	deadLettered, err = repo.DeadLetterEvent(context.Background(), config, event, "agent_execution_failed", time.Unix(301, 0))
	require.NoError(t, err)
	require.False(t, deadLettered)

	require.Len(t, outbox.events, 1)
	require.Equal(t, domainnotification.EventIMMessageDeadLettered, outbox.events[0].EventType)
	require.Equal(t, "20", outbox.events[0].Payload.TargetID)
	require.Equal(t, []int64{2, 9}, outbox.events[0].Payload.ExplicitRecipientIDs)
	require.NotContains(t, outbox.events[0].Payload.ResourceDisplayName, "must not leak")

	var got domain.Event
	require.NoError(t, db.Table(eventTable).Where("id = ?", int64(100)).Take(&got).Error)
	require.Equal(t, domain.EventStatusDeadLetter, got.Status)
	require.Empty(t, got.PayloadJSON)
	require.Equal(t, int32(domain.EventMaxAttempts), got.AttemptCount)
}

func TestRepositoryNotificationRecipientsAreConstrainedToCurrentWorkspaceMembers(t *testing.T) {
	db, repo, outbox := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{})
	config := imConfigFixture()
	config.CreatorID = 7
	config.UpdatedBy = 2
	insertIMConfig(t, db, config)
	insertSpaceUsers(t, db,
		spaceUserTestPO{ID: 1, SpaceID: 20, UserID: 9, RoleType: workspaceRoleOwner},
		spaceUserTestPO{ID: 2, SpaceID: 20, UserID: 8, RoleType: workspaceRoleAdmin},
		spaceUserTestPO{ID: 3, SpaceID: 20, UserID: 2, RoleType: 3},
		spaceUserTestPO{ID: 4, SpaceID: 99, UserID: 7, RoleType: workspaceRoleOwner},
		spaceUserTestPO{ID: 5, SpaceID: 99, UserID: 50, RoleType: workspaceRoleAdmin},
	)

	require.NoError(t, repo.RecordRuntimeFailure(context.Background(), 10, domain.RuntimeState{
		Status:     domain.RuntimeStatusError,
		Error:      "connection_failed",
		IncidentID: "incident-recipient",
	}, 1))

	require.Len(t, outbox.events, 1)
	want := []int64{2, 8, 9}
	if !reflect.DeepEqual(outbox.events[0].Payload.ExplicitRecipientIDs, want) {
		t.Fatalf("recipients = %v, want %v", outbox.events[0].Payload.ExplicitRecipientIDs, want)
	}
}

func TestRepositoryClaimDoesNotConsumeFinalAttemptAfterLeaseExpiry(t *testing.T) {
	db, repo, _ := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{})
	insertIMConfig(t, db, imConfigFixture())
	expiredAt := time.Unix(100, 0)
	event := &domain.Event{
		ID:                   100,
		ConfigID:             10,
		EventKey:             "event-final-crash",
		MessageID:            "message-final-crash",
		PayloadJSON:          `{"content":"safe"}`,
		Status:               domain.EventStatusProcessing,
		AttemptCount:         domain.EventMaxAttempts - 1,
		ProcessingOwner:      "worker-a",
		ProcessingLeaseUntil: &expiredAt,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	require.NoError(t, db.Table(eventTable).Create(event).Error)

	claimed, err := repo.ClaimEvents(
		context.Background(),
		10,
		"worker-b",
		time.Unix(200, 0),
		time.Unix(260, 0),
		10,
	)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, domain.EventStatusProcessing, claimed[0].Status)
	require.Equal(t, int32(domain.EventMaxAttempts-1), claimed[0].AttemptCount)

	var got domain.Event
	require.NoError(t, db.Table(eventTable).Where("id = ?", int64(100)).Take(&got).Error)
	require.Equal(t, domain.EventStatusProcessing, got.Status)
	require.Equal(t, int32(domain.EventMaxAttempts-1), got.AttemptCount)
	require.Equal(t, "worker-b", got.ProcessingOwner)
}

func TestRepositoryExpiredExhaustedProcessingEventCanBeReclaimedForDeadLetter(t *testing.T) {
	db, repo, _ := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{})
	insertIMConfig(t, db, imConfigFixture())
	expiredAt := time.Unix(100, 0)
	event := &domain.Event{
		ID:                   100,
		ConfigID:             10,
		EventKey:             "event-exhausted-processing",
		MessageID:            "message-exhausted-processing",
		PayloadJSON:          `{"content":"safe"}`,
		Status:               domain.EventStatusProcessing,
		AttemptCount:         domain.EventMaxAttempts,
		ProcessingOwner:      "worker-a",
		ProcessingLeaseUntil: &expiredAt,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	require.NoError(t, db.Table(eventTable).Create(event).Error)

	claimed, err := repo.ClaimEvents(
		context.Background(),
		10,
		"worker-b",
		time.Unix(200, 0),
		time.Unix(260, 0),
		10,
	)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, domain.EventStatusFailed, claimed[0].Status)
	require.Equal(t, int32(domain.EventMaxAttempts), claimed[0].AttemptCount)

	var got domain.Event
	require.NoError(t, db.Table(eventTable).Where("id = ?", int64(100)).Take(&got).Error)
	require.Equal(t, domain.EventStatusProcessing, got.Status)
	require.Equal(t, int32(domain.EventMaxAttempts), got.AttemptCount)
	require.Equal(t, "worker-b", got.ProcessingOwner)
}

func TestRepositoryExhaustedDeadLetterFailureCanBeRequeued(t *testing.T) {
	db, repo, _ := newIMChannelRepositoryTest(t, &recordingIMNotificationOutbox{
		err: errors.New("outbox unavailable"),
	})
	config := imConfigFixture()
	insertIMConfig(t, db, config)
	insertSpaceUsers(t, db,
		spaceUserTestPO{ID: 1, SpaceID: 20, UserID: 9, RoleType: workspaceRoleOwner},
	)
	event := &domain.Event{
		ID:           100,
		ConfigID:     10,
		EventKey:     "event-exhausted",
		MessageID:    "message-exhausted",
		PayloadJSON:  `{"content":"safe"}`,
		Status:       domain.EventStatusFailed,
		AttemptCount: domain.EventMaxAttempts,
		NextRetryAt:  nil,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, db.Table(eventTable).Create(event).Error)

	claimed, err := repo.ClaimEvents(
		context.Background(),
		10,
		"worker-a",
		time.Unix(200, 0),
		time.Unix(260, 0),
		10,
	)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, domain.EventStatusFailed, claimed[0].Status)

	deadLettered, err := repo.DeadLetterEvent(context.Background(), config, claimed[0], "agent_execution_failed", time.Unix(210, 0))
	require.Error(t, err)
	require.False(t, deadLettered)
	require.NoError(t, repo.FailEvent(context.Background(), claimed[0].ID, "provider_unavailable", time.Unix(220, 0)))

	var got domain.Event
	require.NoError(t, db.Table(eventTable).Where("id = ?", int64(100)).Take(&got).Error)
	require.Equal(t, domain.EventStatusFailed, got.Status)
	require.Equal(t, int32(domain.EventMaxAttempts), got.AttemptCount)

	claimedAgain, err := repo.ClaimEvents(
		context.Background(),
		10,
		"worker-b",
		time.Unix(230, 0),
		time.Unix(290, 0),
		10,
	)
	require.NoError(t, err)
	require.Len(t, claimedAgain, 1)
	require.Equal(t, int64(100), claimedAgain[0].ID)
}

func newIMChannelRepositoryTest(
	t *testing.T,
	appender NotificationOutboxAppender,
) (*gorm.DB, *MySQLRepository, *recordingIMNotificationOutbox) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&configTestPO{}, &eventTestPO{}, &spaceUserTestPO{}))
	outbox, _ := appender.(*recordingIMNotificationOutbox)
	return db, NewMySQLRepository(db, WithNotificationOutboxAppender(appender)), outbox
}

func imConfigFixture() *domain.Config {
	now := time.Now()
	return &domain.Config{
		ID:           10,
		SpaceID:      20,
		CreatorID:    7,
		UpdatedBy:    2,
		AgentID:      30,
		ChannelType:  domain.ChannelTypeFeishu,
		Name:         "飞书客服机器人",
		AppID:        "cli-test",
		Enabled:      true,
		RuntimeStatus: domain.RuntimeStatusConnected,
		Version:      1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func insertIMConfig(t *testing.T, db *gorm.DB, config *domain.Config) {
	t.Helper()
	require.NoError(t, db.Table(configTable).Create(config).Error)
}

func insertSpaceUsers(t *testing.T, db *gorm.DB, members ...spaceUserTestPO) {
	t.Helper()
	for _, member := range members {
		require.NoError(t, db.Create(&member).Error)
	}
}

type recordingIMNotificationOutbox struct {
	events []domainnotification.Event
	err    error
}

func (r *recordingIMNotificationOutbox) AppendInTransaction(
	_ context.Context,
	_ *gorm.DB,
	event domainnotification.Event,
) error {
	r.events = append(r.events, event)
	return r.err
}
