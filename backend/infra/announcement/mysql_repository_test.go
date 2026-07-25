// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type announcementTestIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *announcementTestIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *announcementTestIDGenerator) GenMultiIDs(
	ctx context.Context,
	count int,
) ([]int64, error) {
	ids := make([]int64, count)
	for index := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

type announcementUserPO struct {
	ID        int64          `gorm:"column:id;primaryKey"`
	CreatedAt int64          `gorm:"column:created_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (announcementUserPO) TableName() string {
	return "user"
}

type announcementSpacePO struct {
	ID        int64          `gorm:"column:id;primaryKey"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (announcementSpacePO) TableName() string {
	return "space"
}

type announcementSpaceUserPO struct {
	ID        int64 `gorm:"column:id;primaryKey;autoIncrement"`
	SpaceID   int64 `gorm:"column:space_id;uniqueIndex:uk_announcement_space_user"`
	UserID    int64 `gorm:"column:user_id;uniqueIndex:uk_announcement_space_user"`
	CreatedAt int64 `gorm:"column:created_at"`
}

func (announcementSpaceUserPO) TableName() string {
	return "space_user"
}

type announcementOutboxRecord struct {
	EventID    string `gorm:"column:event_id;primaryKey"`
	PayloadJSON []byte `gorm:"column:payload_json;type:json"`
}

func (announcementOutboxRecord) TableName() string {
	return "announcement_test_outbox"
}

type recordingAnnouncementOutbox struct {
	mu       sync.Mutex
	failNext bool
}

func (o *recordingAnnouncementOutbox) AppendInTransactionWithResult(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) (bool, error) {
	o.mu.Lock()
	if o.failNext {
		o.failNext = false
		o.mu.Unlock()
		return false, errors.New("temporary outbox failure with password=secret")
	}
	o.mu.Unlock()
	canonical, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(canonical.Payload)
	if err != nil {
		return false, err
	}
	row := &announcementOutboxRecord{
		EventID:    canonical.EventID,
		PayloadJSON: payload,
	}
	result := tx.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(row)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		var existing announcementOutboxRecord
		if err := tx.WithContext(ctx).
			Where("event_id = ?", canonical.EventID).
			Take(&existing).Error; err != nil {
			return false, err
		}
		if string(existing.PayloadJSON) != string(payload) {
			return false, domainnotification.ErrIdempotencyConflict
		}
	}
	return result.RowsAffected == 1, nil
}

func TestAnnouncementRepositoryWorkspaceAudienceUsesPersistentMembershipFacts(
	t *testing.T,
) {
	db, repository, _ := newAnnouncementRepository(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&announcementSpacePO{ID: 10}).Error)
	require.NoError(t, db.Create([]announcementUserPO{
		{ID: 1},
		{ID: 2},
		{ID: 3, DeletedAt: gorm.DeletedAt{
			Time: time.Now(),
			Valid: true,
		}},
	}).Error)
	require.NoError(t, db.Create([]announcementSpaceUserPO{
		{SpaceID: 10, UserID: 1},
		{SpaceID: 10, UserID: 3},
	}).Error)

	created := createAnnouncementForTest(
		t,
		repository,
		"create.workspace.1",
		domainannouncement.Audience{
			Type:      domainannouncement.AudienceWorkspaces,
			TargetIDs: []int64{10},
		},
	)
	publishAnnouncementForTest(t, repository, created)
	published := drainAnnouncementForTest(t, repository, created.ID)

	require.Equal(t, domainannouncement.StatusPublished, published.Status)
	require.Equal(t, int64(1), published.RecipientCount)
	require.Equal(t, int64(1), published.ProjectedCount)
	payloads := outboxPayloadsForTest(t, db)
	require.Len(t, payloads, 1)
	require.Equal(t, []int64{1}, payloads[0].ExplicitRecipientIDs)

	_, err := repository.Create(ctx, domainannouncement.CreateCommand{
		ActorID:        42,
		IdempotencyKey: "create.missing.1",
		RequestHash:    strings.Repeat("a", 64),
		Draft: domainannouncement.Draft{
			Title:    "Missing",
			Body:     "Missing user",
			Severity: domainannouncement.SeverityInfo,
			Audience: domainannouncement.Audience{
				Type:      domainannouncement.AudienceUsers,
				TargetIDs: []int64{999},
			},
		},
		Now: time.UnixMilli(1_800_000_000_000),
	})
	require.ErrorIs(t, err, domainannouncement.ErrAudienceTargetNotFound)
}

func TestAnnouncementRepositoryPaginatesLargeAudienceIntoStableBatches(
	t *testing.T,
) {
	db, repository, _ := newAnnouncementRepository(t)
	users := make([]announcementUserPO, 1201)
	for index := range users {
		users[index].ID = int64(index + 1)
	}
	require.NoError(t, db.CreateInBatches(users, 200).Error)

	created := createAnnouncementForTest(
		t,
		repository,
		"create.large.001",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	command := publishAnnouncementForTest(t, repository, created)
	replayed, err := repository.RequestPublish(
		context.Background(),
		command,
	)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)

	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(t, int64(1201), published.RecipientCount)
	require.Equal(t, int64(1201), published.ProjectedCount)

	payloads := outboxPayloadsForTest(t, db)
	require.Len(t, payloads, 3)
	require.Len(t, payloads[0].ExplicitRecipientIDs, 500)
	require.Len(t, payloads[1].ExplicitRecipientIDs, 500)
	require.Len(t, payloads[2].ExplicitRecipientIDs, 201)

	var eventIDs []string
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Order("event_id ASC").
		Pluck("event_id", &eventIDs).Error)
	require.Equal(t, []string{
		"announcement." + stringID(created.ID) + ".batch.1",
		"announcement." + stringID(created.ID) + ".batch.2",
		"announcement." + stringID(created.ID) + ".batch.3",
	}, eventIDs)
}

func TestAnnouncementRepositoryRevalidatesTargetStateAfterSnapshot(t *testing.T) {
	db, repository, _ := newAnnouncementRepository(t)
	require.NoError(t, db.Create(&announcementUserPO{ID: 1}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.state.001",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	publishAnnouncementForTest(t, repository, created)
	_, err := repository.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.UnixMilli(1_800_000_000_100),
		500,
	)
	require.NoError(t, err)
	require.NoError(t, db.Delete(&announcementUserPO{}, 1).Error)

	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(t, int64(1), published.RecipientCount)
	require.Zero(t, published.ProjectedCount)
	var outboxCount int64
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Count(&outboxCount).Error)
	require.Zero(t, outboxCount)
}

func TestAnnouncementRepositoryRecoversOutboxFailureAndAuditsReplay(t *testing.T) {
	db, repository, outbox := newAnnouncementRepository(t)
	require.NoError(t, db.Create(&announcementUserPO{ID: 1}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.recover.1",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	publishAnnouncementForTest(t, repository, created)
	_, err := repository.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.UnixMilli(1_800_000_000_100),
		500,
	)
	require.NoError(t, err)
	outbox.mu.Lock()
	outbox.failNext = true
	outbox.mu.Unlock()
	_, err = repository.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.UnixMilli(1_800_000_000_200),
		500,
	)
	require.Error(t, err)
	failed, err := repository.MarkProjectionFailed(
		context.Background(),
		created.ID,
		42,
		domainannouncement.ErrorCodeStorage,
		time.UnixMilli(1_800_000_000_300),
	)
	require.NoError(t, err)
	require.Equal(t, domainannouncement.ProjectionFailed, failed.ProjectionStatus)

	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(t, domainannouncement.ProjectionCompleted, published.ProjectionStatus)
	var actions []string
	require.NoError(t, db.Model(&auditEventPO{}).
		Where("announcement_id = ?", created.ID).
		Order("created_at ASC").
		Pluck("action", &actions).Error)
	require.Contains(t, actions, "projection_failed")
	require.Contains(t, actions, "replay_started")
	require.Contains(t, actions, "published")
}

func TestAnnouncementRepositoryPublishesEmptyAudienceWithoutOutbox(
	t *testing.T,
) {
	db, repository, _ := newAnnouncementRepository(t)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.empty.001",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	publishAnnouncementForTest(t, repository, created)
	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(t, domainannouncement.StatusPublished, published.Status)
	require.Zero(t, published.RecipientCount)
	require.Zero(t, published.ProjectedCount)
	var count int64
	require.NoError(t, db.Model(&announcementOutboxRecord{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAnnouncementRepositoryReleaseBarrierPrecedesOutboxProjection(
	t *testing.T,
) {
	db, repository, outbox := newAnnouncementRepository(t)
	require.NoError(t, db.Create(&announcementUserPO{
		ID:        1,
		CreatedAt: 1,
	}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.barrier.1",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	publishAnnouncementForTest(t, repository, created)
	var snapshotRow announcementPO
	require.NoError(t, db.Where("id = ?", created.ID).Take(&snapshotRow).Error)
	require.True(t, snapshotRow.SnapshotComplete)
	require.Equal(t, int64(1), snapshotRow.RecipientCount)
	var snapshotCount int64
	require.NoError(t, db.Model(&recipientSnapshotPO{}).
		Where("announcement_id = ?", created.ID).
		Count(&snapshotCount).Error)
	require.Equal(t, int64(1), snapshotCount)
	var batchCount int64
	require.NoError(t, db.Model(&deliveryBatchPO{}).
		Where("announcement_id = ?", created.ID).
		Count(&batchCount).Error)
	require.Zero(t, batchCount)

	released, err := repository.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.Now(),
		500,
	)
	require.NoError(t, err)
	require.Equal(
		t,
		domainannouncement.StatusPublished,
		released.Announcement.Status,
	)
	require.Equal(
		t,
		domainannouncement.ProjectionProjecting,
		released.Announcement.ProjectionStatus,
	)
	var outboxCount int64
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Count(&outboxCount).Error)
	require.Zero(t, outboxCount)
	var staged deliveryBatchPO
	require.NoError(t, db.Where(
		"announcement_id = ? AND batch_no = ?",
		created.ID,
		1,
	).Take(&staged).Error)
	require.Equal(t, deliveryBatchStaged, staged.Status)

	outbox.mu.Lock()
	outbox.failNext = true
	outbox.mu.Unlock()
	_, err = repository.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.Now(),
		500,
	)
	require.ErrorIs(t, err, domainannouncement.ErrStorage)
	failed, err := repository.MarkProjectionFailed(
		context.Background(),
		created.ID,
		42,
		domainannouncement.ErrorCodeStorage,
		time.Now(),
	)
	require.NoError(t, err)
	require.Equal(t, domainannouncement.StatusPublished, failed.Status)
	require.Equal(t, domainannouncement.ProjectionFailed, failed.ProjectionStatus)
	require.NoError(t, db.Where(
		"announcement_id = ? AND batch_no = ?",
		created.ID,
		1,
	).Take(&staged).Error)
	require.Equal(t, deliveryBatchStaged, staged.Status)
	require.Equal(t, int64(1), staged.AttemptCount)
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Count(&outboxCount).Error)
	require.Zero(t, outboxCount)

	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(
		t,
		domainannouncement.ProjectionCompleted,
		published.ProjectionStatus,
	)
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Count(&outboxCount).Error)
	require.Equal(t, int64(1), outboxCount)
	var eventIDs []string
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Pluck("event_id", &eventIDs).Error)
	require.Equal(
		t,
		[]string{"announcement." + stringID(created.ID) + ".batch.1"},
		eventIDs,
	)
}

func TestAnnouncementRepositoryPublishAnchorExcludesLaterUsersAndMemberships(
	t *testing.T,
) {
	db, repository, _ := newAnnouncementRepository(t)
	require.NoError(t, db.Create(&announcementSpacePO{ID: 10}).Error)
	require.NoError(t, db.Create([]announcementUserPO{
		{ID: 1, CreatedAt: 1},
		{ID: 2, CreatedAt: 1},
	}).Error)
	require.NoError(t, db.Create(&announcementSpaceUserPO{
		SpaceID:   10,
		UserID:    1,
		CreatedAt: 1,
	}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.anchor.001",
		domainannouncement.Audience{
			Type:      domainannouncement.AudienceWorkspaces,
			TargetIDs: []int64{10},
		},
	)
	snapshotWindowStart := time.Now().UTC().UnixMilli()
	publishAnnouncementForTest(t, repository, created)
	snapshotWindowEnd := time.Now().UTC().UnixMilli()

	require.NoError(t, db.Create(&announcementUserPO{
		ID:        3,
		CreatedAt: 1,
	}).Error)
	require.NoError(t, db.Create(&announcementSpaceUserPO{
		SpaceID:   10,
		UserID:    2,
		CreatedAt: 1,
	}).Error)

	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(t, int64(1), published.RecipientCount)
	payloads := outboxPayloadsForTest(t, db)
	require.Len(t, payloads, 1)
	require.Equal(t, []int64{1}, payloads[0].ExplicitRecipientIDs)
	require.GreaterOrEqual(t, published.SnapshotAt, snapshotWindowStart-1000)
	require.LessOrEqual(t, published.SnapshotAt, snapshotWindowEnd+1000)
}

func TestAnnouncementRepositoryRejectsAudienceAboveConfiguredLimitAtomically(
	t *testing.T,
) {
	db, _, outbox := newAnnouncementRepository(t)
	repository := NewMySQLRepository(
		db,
		&announcementTestIDGenerator{next: 30_000},
		WithNotificationOutbox(outbox),
		WithMaxAudienceSize(1),
	)
	require.NoError(t, db.Create([]announcementUserPO{
		{ID: 1},
		{ID: 2},
	}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.limit.001",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	command := domainannouncement.PublishCommand{
		AnnouncementID: created.ID,
		ActorID:         42,
		ExpectedVersion: created.Version,
		IdempotencyKey:  "publish.limit.001",
		RequestHash:     strings.Repeat("b", 64),
		Now:             time.UnixMilli(1),
	}

	_, err := repository.RequestPublish(context.Background(), command)

	require.ErrorIs(t, err, domainannouncement.ErrAudienceTooLarge)
	current, err := repository.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, domainannouncement.StatusDraft, current.Status)
	require.Equal(t, domainannouncement.ProjectionIdle, current.ProjectionStatus)
	require.Zero(t, current.SnapshotAt)
	var snapshotCount int64
	require.NoError(t, db.Model(&recipientSnapshotPO{}).
		Where("announcement_id = ?", created.ID).
		Count(&snapshotCount).Error)
	require.Zero(t, snapshotCount)
}

func TestAnnouncementRepositoryRequiresWorkspaceRouteToMatchAudience(
	t *testing.T,
) {
	db, repository, _ := newAnnouncementRepository(t)
	require.NoError(t, db.Create([]announcementSpacePO{
		{ID: 10},
		{ID: 11},
	}).Error)

	_, err := repository.Create(
		context.Background(),
		domainannouncement.CreateCommand{
			ActorID:        42,
			IdempotencyKey: "create.route.001",
			RequestHash:    strings.Repeat("a", 64),
			Draft: domainannouncement.Draft{
				Title:    "Workspace",
				Body:     "Workspace announcement",
				Severity: domainannouncement.SeverityInfo,
				Route: domainnotification.AnnouncementRoute{
					Type:    domainnotification.AnnouncementRouteWorkspaceHome,
					SpaceID: 11,
				},
				Audience: domainannouncement.Audience{
					Type:      domainannouncement.AudienceWorkspaces,
					TargetIDs: []int64{10},
				},
			},
			Now: time.Now(),
		},
	)

	require.ErrorIs(t, err, domainannouncement.ErrInvalidInput)
}

func TestAnnouncementRepositoryWorkspaceMembershipRevocationSkipsDelivery(
	t *testing.T,
) {
	db, repository, _ := newAnnouncementRepository(t)
	require.NoError(t, db.Create(&announcementSpacePO{ID: 10}).Error)
	require.NoError(t, db.Create(&announcementUserPO{
		ID:        1,
		CreatedAt: 1,
	}).Error)
	require.NoError(t, db.Create(&announcementSpaceUserPO{
		SpaceID:   10,
		UserID:    1,
		CreatedAt: 1,
	}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.membership.1",
		domainannouncement.Audience{
			Type:      domainannouncement.AudienceWorkspaces,
			TargetIDs: []int64{10},
		},
	)
	publishAnnouncementForTest(t, repository, created)
	released, err := repository.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.Now(),
		500,
	)
	require.NoError(t, err)
	require.Equal(
		t,
		domainannouncement.StatusPublished,
		released.Announcement.Status,
	)
	require.NoError(t, db.Where(
		"space_id = ? AND user_id = ?",
		10,
		1,
	).Delete(&announcementSpaceUserPO{}).Error)

	published := drainAnnouncementForTest(t, repository, created.ID)
	require.Equal(t, int64(1), published.RecipientCount)
	require.Zero(t, published.ProjectedCount)
	var outboxCount int64
	require.NoError(t, db.Model(&announcementOutboxRecord{}).
		Count(&outboxCount).Error)
	require.Zero(t, outboxCount)
}

func TestAnnouncementRepositoryFourStatesCASAndIdempotencyConflict(
	t *testing.T,
) {
	_, repository, _ := newAnnouncementRepository(t)
	ctx := context.Background()
	draft := createAnnouncementForTest(
		t,
		repository,
		"create.states.001",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	require.Equal(t, domainannouncement.StatusDraft, draft.Status)

	_, err := repository.Update(ctx, domainannouncement.UpdateCommand{
		AnnouncementID: draft.ID,
		ActorID:         42,
		ExpectedVersion: draft.Version + 1,
		Draft:           draft.Draft,
		Now:             time.Now(),
	})
	require.ErrorIs(t, err, domainannouncement.ErrVersionConflict)

	scheduled, err := repository.Schedule(ctx, domainannouncement.ScheduleCommand{
		AnnouncementID: draft.ID,
		ActorID:         42,
		ExpectedVersion: draft.Version,
		ScheduledAt:     time.Now().Add(time.Hour),
		Now:             time.Now(),
	})
	require.NoError(t, err)
	require.Equal(t, domainannouncement.StatusScheduled, scheduled.Status)
	cancelled, err := repository.Cancel(ctx, domainannouncement.CancelCommand{
		AnnouncementID: scheduled.ID,
		ActorID:         42,
		ExpectedVersion: scheduled.Version,
		Now:             time.Now(),
	})
	require.NoError(t, err)
	require.Equal(t, domainannouncement.StatusCancelled, cancelled.Status)

	publishable := createAnnouncementForTest(
		t,
		repository,
		"create.states.002",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	command := publishAnnouncementForTest(t, repository, publishable)
	replayed, err := repository.RequestPublish(ctx, command)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	conflicting := command
	conflicting.RequestHash = strings.Repeat("c", 64)
	_, err = repository.RequestPublish(ctx, conflicting)
	require.ErrorIs(t, err, domainannouncement.ErrIdempotencyConflict)
	published := drainAnnouncementForTest(t, repository, publishable.ID)
	require.Equal(t, domainannouncement.StatusPublished, published.Status)
}

func TestAnnouncementRepositoryDueScheduleAppearsInReplayScan(t *testing.T) {
	_, repository, _ := newAnnouncementRepository(t)
	ctx := context.Background()
	draft := createAnnouncementForTest(
		t,
		repository,
		"create.schedule.1",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	scheduled, err := repository.Schedule(ctx, domainannouncement.ScheduleCommand{
		AnnouncementID: draft.ID,
		ActorID:         42,
		ExpectedVersion: draft.Version,
		ScheduledAt:     time.Now().Add(-time.Minute),
		Now:             time.Now(),
	})
	require.NoError(t, err)
	require.Equal(t, domainannouncement.StatusScheduled, scheduled.Status)

	ids, err := repository.ListReplayCandidates(
		ctx,
		time.Now(),
		0,
		10,
	)
	require.NoError(t, err)
	require.Equal(t, []int64{draft.ID}, ids)
	published := drainAnnouncementForTest(t, repository, draft.ID)
	require.Equal(t, domainannouncement.StatusPublished, published.Status)
	require.Equal(
		t,
		domainannouncement.ProjectionCompleted,
		published.ProjectionStatus,
	)
}

func newAnnouncementRepository(
	t *testing.T,
) (*gorm.DB, *MySQLRepository, *recordingAnnouncementOutbox) {
	t.Helper()
	databasePath := filepath.Join(
		t.TempDir(),
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())+".db",
	)
	db, err := gorm.Open(
		sqlite.Open(
			"file:"+databasePath+"?_busy_timeout=5000&_journal_mode=WAL",
		),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&announcementPO{},
		&audienceTargetPO{},
		&recipientSnapshotPO{},
		&deliveryBatchPO{},
		&auditEventPO{},
		&announcementUserPO{},
		&announcementSpacePO{},
		&announcementSpaceUserPO{},
		&announcementOutboxRecord{},
	))
	outbox := &recordingAnnouncementOutbox{}
	repository := NewMySQLRepository(
		db,
		&announcementTestIDGenerator{next: 10_000},
		WithNotificationOutbox(outbox),
	)
	return db, repository, outbox
}

func createAnnouncementForTest(
	t *testing.T,
	repository *MySQLRepository,
	idempotencyKey string,
	audience domainannouncement.Audience,
) *domainannouncement.Announcement {
	t.Helper()
	result, err := repository.Create(
		context.Background(),
		domainannouncement.CreateCommand{
			ActorID:        42,
			IdempotencyKey: idempotencyKey,
			RequestHash:    strings.Repeat("a", 64),
			Draft: domainannouncement.Draft{
				Title:    "系统维护",
				Body:     "系统将在今晚维护。",
				Severity: domainannouncement.SeverityWarning,
				Route: domainnotification.AnnouncementRoute{
					Type: domainnotification.AnnouncementRouteSystemAnnouncements,
				},
				Audience: audience,
			},
			Now: time.UnixMilli(1_800_000_000_000),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result.Announcement
}

func publishAnnouncementForTest(
	t *testing.T,
	repository *MySQLRepository,
	announcement *domainannouncement.Announcement,
) domainannouncement.PublishCommand {
	t.Helper()
	command := domainannouncement.PublishCommand{
		AnnouncementID: announcement.ID,
		ActorID:         42,
		ExpectedVersion: announcement.Version,
		IdempotencyKey:  "publish." + stringID(announcement.ID),
		RequestHash:     strings.Repeat("b", 64),
		Now:             time.UnixMilli(1_800_000_000_050),
	}
	result, err := repository.RequestPublish(context.Background(), command)
	require.NoError(t, err)
	require.False(t, result.Replayed)
	return command
}

func drainAnnouncementForTest(
	t *testing.T,
	repository *MySQLRepository,
	announcementID int64,
) *domainannouncement.Announcement {
	t.Helper()
	for step := 0; step < 20; step++ {
		result, err := repository.AdvancePublication(
			context.Background(),
			announcementID,
			42,
			time.UnixMilli(1_800_000_001_000+int64(step)),
			500,
		)
		require.NoError(t, err)
		if result.Done {
			return result.Announcement
		}
	}
	t.Fatal("announcement publication did not complete")
	return nil
}

func outboxPayloadsForTest(
	t *testing.T,
	db *gorm.DB,
) []domainnotification.EventPayload {
	t.Helper()
	var rows []announcementOutboxRecord
	require.NoError(t, db.Order("event_id ASC").Find(&rows).Error)
	payloads := make([]domainnotification.EventPayload, 0, len(rows))
	for _, row := range rows {
		var payload domainnotification.EventPayload
		require.NoError(t, json.Unmarshal(row.PayloadJSON, &payload))
		payloads = append(payloads, payload)
	}
	return payloads
}

func stringID(value int64) string {
	return strconv.FormatInt(value, 10)
}
