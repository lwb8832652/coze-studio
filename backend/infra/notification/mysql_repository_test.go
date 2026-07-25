// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestNotificationRepositoryUnitUsesDatabaseClockForBoundaries(
	t *testing.T,
) {
	db, repository := newTestRepository(t)
	databaseNow := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	repository.databaseClock = func(
		context.Context,
		*gorm.DB,
		time.Time,
	) (time.Time, error) {
		return databaseNow, nil
	}
	event := testEvent("evt-database-clock-boundaries", 1)
	event.OccurredAt = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	inserted, err := repository.AppendInTransactionWithResult(
		context.Background(),
		db,
		event,
	)
	require.NoError(t, err)
	require.True(t, inserted)

	var appended outboxPO
	require.NoError(t, db.
		Where("event_id = ?", event.EventID).
		Take(&appended).Error)
	require.Equal(t, databaseNow.UnixMilli(), appended.AvailableAt)

	callerNow := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	claims, err := repository.ClaimOutboxBatch(
		context.Background(),
		"worker-a",
		callerNow,
		30*time.Second,
		1,
	)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(
		t,
		databaseNow.Add(30*time.Second),
		claims[0].LeaseExpiresAt,
	)

	databaseNow = databaseNow.Add(time.Second)
	err = repository.FailClaim(
		context.Background(),
		claims[0],
		domainnotification.StableErrorCode(
			domainnotification.ErrRecipientResolution,
		),
		callerNow,
		callerNow.Add(5*time.Second),
		false,
	)
	require.NoError(t, err)

	var retried outboxPO
	require.NoError(t, db.
		Where("event_id = ?", event.EventID).
		Take(&retried).Error)
	require.Equal(
		t,
		databaseNow.Add(5*time.Second).UnixMilli(),
		retried.AvailableAt,
	)
	require.Equal(t, databaseNow.UnixMilli(), retried.UpdatedAt)
}

func TestNotificationRepositoryUnitDatabaseClockFailureIsSanitized(
	t *testing.T,
) {
	db, repository := newTestRepository(t)
	event := testEvent("evt-database-clock-failure", 1)
	require.NoError(t, repository.Append(context.Background(), event))

	const secret = "mysql://admin:credential@private-dsn"
	repository.databaseClock = func(
		context.Context,
		*gorm.DB,
		time.Time,
	) (time.Time, error) {
		return time.Time{}, errors.New(secret)
	}

	claims, err := repository.ClaimOutboxBatch(
		context.Background(),
		"worker-a",
		time.Now(),
		time.Minute,
		1,
	)
	require.Nil(t, claims)
	require.ErrorIs(t, err, ErrNotificationDatabaseClock)
	require.ErrorIs(t, err, domainnotification.ErrStorage)
	require.NotContains(t, err.Error(), secret)

	var unchanged outboxPO
	require.NoError(t, db.
		Where("event_id = ?", event.EventID).
		Take(&unchanged).Error)
	require.Equal(t, string(domainnotification.OutboxPending), unchanged.Status)
	require.Empty(t, unchanged.LockedBy)
}

func TestNotificationRepositoryUnitFinalWriteUsesDatabaseLeaseFence(
	t *testing.T,
) {
	db, repository := newTestRepository(t)
	databaseNow := time.Date(2026, 7, 25, 11, 0, 0, 0, time.UTC)
	repository.databaseClock = func(
		context.Context,
		*gorm.DB,
		time.Time,
	) (time.Time, error) {
		return databaseNow, nil
	}
	event := testEvent("evt-database-final-write-fence", 1)
	require.NoError(t, repository.Append(context.Background(), event))
	claims, err := repository.ClaimOutboxBatch(
		context.Background(),
		"worker-db-fence",
		time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		30*time.Second,
		1,
	)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	draft, err := domainnotification.DefaultTemplateRegistry().Render(event)
	require.NoError(t, err)

	databaseNow = claims[0].LeaseExpiresAt
	err = repository.MaterializeAndDeliver(
		context.Background(),
		claims[0],
		draft,
		[]int64{event.ActorID},
		time.UnixMilli(1),
	)
	require.ErrorIs(t, err, domainnotification.ErrLeaseLost)

	var unchanged outboxPO
	require.NoError(t, db.
		Where("event_id = ?", event.EventID).
		Take(&unchanged).Error)
	require.Equal(
		t,
		string(domainnotification.OutboxProcessing),
		unchanged.Status,
	)
	require.Equal(t, "worker-db-fence", unchanged.LockedBy)
	var messageCount int64
	require.NoError(t, db.Model(&messagePO{}).Count(&messageCount).Error)
	require.Zero(t, messageCount)
}

type sequenceIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *sequenceIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *sequenceIDGenerator) GenMultiIDs(ctx context.Context, count int) ([]int64, error) {
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

func TestNotificationRepositoryUnitAppendInTransactionRollsBackWithCaller(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	event := testEvent("evt-rollback", 1)

	tx := db.Begin()
	require.NoError(t, repository.AppendInTransaction(ctx, tx, event))
	require.NoError(t, tx.Rollback().Error)

	var count int64
	require.NoError(t, db.Model(&outboxPO{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestNotificationRepositoryUnitAppendWithResultReportsOnlyAtomicInsert(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	event := testEvent("evt-insert-result", 1)

	inserted, err := repository.AppendWithResult(ctx, event)
	require.NoError(t, err)
	require.True(t, inserted)
	inserted, err = repository.AppendWithResult(ctx, event)
	require.NoError(t, err)
	require.False(t, inserted)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	inserted, err = repository.AppendInTransactionWithResult(ctx, tx, testEvent("evt-insert-result-rollback", 2))
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, tx.Rollback().Error)
	var count int64
	require.NoError(t, db.Model(&outboxPO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestNotificationRepositoryUnitAppendIsIdempotentAndDetectsIdentityConflict(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	event := testEvent("evt-idempotent", 1)

	require.NoError(t, repository.Append(ctx, event))
	require.NoError(t, repository.Append(ctx, event))

	var count int64
	require.NoError(t, db.Model(&outboxPO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	conflict := event
	conflict.EventID = "evt-conflict"
	conflict.Payload.ResourceDisplayName = "不同的资源"
	conflict.AggregateID = event.AggregateID
	require.ErrorIs(t, repository.Append(ctx, conflict), domainnotification.ErrIdempotencyConflict)
}

func TestNotificationRepositoryUnitConcurrentDuplicateAppendKeepsOneSemanticIdentity(t *testing.T) {
	db, repository := newTestRepository(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	ctx := context.Background()
	first := testEvent("evt-concurrent-original", 1)
	retry := first
	retry.EventID = "evt-concurrent-retry"
	retry.OccurredAt = first.OccurredAt.Add(time.Minute)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, event := range []domainnotification.Event{first, retry} {
		event := event
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- repository.Append(ctx, event)
		}()
	}
	close(start)
	wait.Wait()
	close(errs)

	for appendErr := range errs {
		require.NoError(t, appendErr)
	}
	var count int64
	require.NoError(t, db.Model(&outboxPO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.True(t, requiresOutboxIdentityCurrentRead("mysql"))
	require.False(t, requiresOutboxIdentityCurrentRead("sqlite"))
}

func TestNotificationRepositoryUnitSameOutboxIdentityUsesSemanticJSONAndStrictEventFields(t *testing.T) {
	// MySQL JSON stores a normalized binary representation and may return
	// whitespace/key ordering that differs from the producer input. The
	// identity check therefore compares the controlled payload semantically;
	// it also treats equivalent integral JSON numbers as the same value.
	left := &outboxPO{
		EventID:          "evt-semantic-json",
		IdempotencyKey:   strings.Repeat("a", 64),
		EventType:        string(domainnotification.EventWorkspaceRoleChanged),
		AggregateType:    "workspace_member",
		AggregateID:      "member-101",
		AggregateVersion: 2,
		OccurredAt:       1_721_000_000_000,
		SpaceID:          202,
		ActorID:          101,
		RecipientPolicy:  string(domainnotification.RecipientExplicitInternalUsers),
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		PayloadJSON: []byte(`{
			"resource_display_name": "研发空间",
			"target_id": "member-101",
			"explicit_recipient_ids": [101, 202]
		}`),
	}
	right := *left
	right.PayloadJSON = []byte(
		`{"explicit_recipient_ids":[1.01e2,202.0],"target_id":"member-101","resource_display_name":"研发空间"}`,
	)

	require.True(t, sameOutboxIdentity(left, &right))

	right.ActorID = 999
	require.False(t, sameOutboxIdentity(left, &right))
	right.ActorID = left.ActorID
	right.PayloadJSON = []byte(`{"summary":"producer supplied body"}`)
	require.False(t, sameOutboxIdentity(left, &right))
}

func TestNotificationRepositoryUnitMaterializationIsIdempotentAndUserScoped(t *testing.T) {
	_, repository := newTestRepository(t)
	ctx := context.Background()
	event := testEvent("evt-materialize", 1)
	require.NoError(t, repository.Append(ctx, event))

	claims, err := repository.ClaimOutboxBatch(
		ctx,
		"worker-a",
		time.UnixMilli(1_721_000_000_100),
		30*time.Second,
		10,
	)
	require.NoError(t, err)
	require.Len(t, claims, 1)

	draft, err := domainnotification.DefaultTemplateRegistry().Render(event)
	require.NoError(t, err)
	require.NoError(t, repository.MaterializeAndDeliver(
		ctx,
		claims[0],
		draft,
		[]int64{101},
		time.UnixMilli(1_721_000_000_200),
	))

	page, err := repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID: 101,
		Limit:  20,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, event.EventID, page.Items[0].Message.EventID)

	other, err := repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID: 999,
		Limit:  20,
	})
	require.NoError(t, err)
	require.Empty(t, other.Items)

	affected, err := repository.MarkRead(ctx, 999, []int64{page.Items[0].Message.ID}, 123)
	require.NoError(t, err)
	require.Zero(t, affected)

	unread, err := repository.CountUnread(ctx, 101)
	require.NoError(t, err)
	require.Equal(t, int64(1), unread)
}

func TestNotificationRepositoryUnitConcurrentClaimsAreDisjoint(t *testing.T) {
	_, repository := newTestRepository(t)
	ctx := context.Background()
	require.NoError(t, repository.Append(ctx, testEvent("evt-concurrent-a", 1)))
	require.NoError(t, repository.Append(ctx, testEvent("evt-concurrent-b", 2)))

	start := make(chan struct{})
	results := make(chan []domainnotification.OutboxClaim, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, workerID := range []string{"worker-a", "worker-b"} {
		wait.Add(1)
		go func(worker string) {
			defer wait.Done()
			<-start
			claims, err := repository.ClaimOutboxBatch(
				ctx,
				worker,
				time.Now().Add(time.Second),
				30*time.Second,
				1,
			)
			results <- claims
			errs <- err
		}(workerID)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	claimedIDs := map[int64]struct{}{}
	for claims := range results {
		require.Len(t, claims, 1)
		_, duplicate := claimedIDs[claims[0].OutboxID]
		require.False(t, duplicate)
		claimedIDs[claims[0].OutboxID] = struct{}{}
	}
	require.Len(t, claimedIDs, 2)
}

func TestNotificationRepositoryUnitClaimBatchKeepsMalformedPayloadIsolated(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, db.Create(&outboxPO{
		ID:               1,
		EventID:          "evt-malformed",
		IdempotencyKey:   strings.Repeat("a", 64),
		EventType:        string(domainnotification.EventAgentRunSucceeded),
		AggregateType:    "agent_run",
		AggregateID:      "run-malformed",
		AggregateVersion: 1,
		OccurredAt:       now.UnixMilli(),
		SpaceID:          202,
		ActorID:          101,
		RecipientPolicy:  string(domainnotification.RecipientActor),
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		PayloadJSON:      []byte("{"),
		Status:           string(domainnotification.OutboxPending),
		AvailableAt:      now.UnixMilli(),
		CreatedAt:        now.UnixMilli(),
		UpdatedAt:        now.UnixMilli(),
	}).Error)
	require.NoError(t, repository.Append(ctx, testEvent("evt-valid-after-malformed", 2)))

	claims, err := repository.ClaimOutboxBatch(
		ctx,
		"worker-isolation",
		now.Add(time.Second),
		30*time.Second,
		10,
	)

	require.NoError(t, err)
	require.Len(t, claims, 2)
	require.Equal(t, domainnotification.ErrorCodeInvalidEvent, claims[0].ClaimErrorCode)
	require.Equal(t, "evt-malformed", claims[0].Event.EventID)
	require.Empty(t, claims[1].ClaimErrorCode)
	require.Equal(t, "evt-valid-after-malformed", claims[1].Event.EventID)
}

func TestNotificationRepositoryUnitMalformedPayloadRetriesAcrossReclaimThenDeadLetters(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, db.Create(&outboxPO{
		ID:               1,
		EventID:          "evt-malformed-retry",
		IdempotencyKey:   strings.Repeat("b", 64),
		EventType:        string(domainnotification.EventAgentRunSucceeded),
		AggregateType:    "agent_run",
		AggregateID:      "run-malformed-retry",
		AggregateVersion: 1,
		OccurredAt:       now.UnixMilli(),
		SpaceID:          202,
		ActorID:          101,
		RecipientPolicy:  string(domainnotification.RecipientActor),
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		PayloadJSON:      []byte(`{"summary":"legacy free-form body"}`),
		Status:           string(domainnotification.OutboxPending),
		AvailableAt:      now.UnixMilli(),
		CreatedAt:        now.UnixMilli(),
		UpdatedAt:        now.UnixMilli(),
	}).Error)

	first, err := repository.ClaimOutboxBatch(
		ctx,
		"worker-first",
		now.Add(time.Second),
		30*time.Second,
		1,
	)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, 0, first[0].AttemptCount)
	require.Equal(t, domainnotification.ErrorCodeInvalidEvent, first[0].ClaimErrorCode)
	require.NoError(t, repository.FailClaim(
		ctx,
		first[0],
		first[0].ClaimErrorCode,
		now.Add(2*time.Second),
		now.Add(3*time.Second),
		false,
	))

	second, err := repository.ClaimOutboxBatch(
		ctx,
		"worker-second",
		now.Add(4*time.Second),
		30*time.Second,
		1,
	)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, 1, second[0].AttemptCount)
	require.Equal(t, domainnotification.ErrorCodeInvalidEvent, second[0].ClaimErrorCode)
	require.NoError(t, repository.FailClaim(
		ctx,
		second[0],
		second[0].ClaimErrorCode,
		now.Add(5*time.Second),
		time.Time{},
		true,
	))

	var stored outboxPO
	require.NoError(t, db.Where("event_id = ?", "evt-malformed-retry").Take(&stored).Error)
	require.Equal(t, string(domainnotification.OutboxDead), stored.Status)
	require.Equal(t, 2, stored.AttemptCount)
	require.Equal(t, domainnotification.ErrorCodeInvalidEvent, stored.LastErrorCode)
}

func TestNotificationRepositoryUnitCursorKeepsOriginalSnapshotWhenNewRowsArrive(t *testing.T) {
	_, repository := newTestRepository(t)
	ctx := context.Background()
	materializeTestEvent(t, repository, testEvent("evt-cursor-old", 1), 101, "worker-a")
	materializeTestEvent(t, repository, testEvent("evt-cursor-newer", 2), 101, "worker-b")

	first, err := repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID: 101,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.True(t, first.HasMore)
	require.Positive(t, first.SnapshotCutoff)
	require.Equal(t, first.SnapshotCutoff, first.NextCursor.SnapshotCutoff)

	materializeTestEvent(t, repository, testEvent("evt-after-snapshot", 3), 101, "worker-c")
	second, err := repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID:    101,
		Cursor:    first.NextCursor,
		HasCursor: true,
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, "evt-cursor-old", second.Items[0].Message.EventID)
	require.Equal(t, first.SnapshotCutoff, second.SnapshotCutoff)
}

func TestNotificationRepositoryUnitSnapshotAndCursorUseDatabaseSequenceInsteadOfBusinessID(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	materializeTestEvent(t, repository, testEvent("evt-sequence-old", 1), 101, "worker-a")
	materializeTestEvent(t, repository, testEvent("evt-sequence-new", 2), 101, "worker-b")

	var newest recipientPO
	require.NoError(t, db.Order("sequence_no DESC").First(&newest).Error)
	require.NoError(t, db.Model(&recipientPO{}).
		Where("id = ?", newest.ID).
		Update("id", int64(9_000_000)).Error)

	page, err := repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID: 101,
		Limit:  1,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.HasMore)

	var maxSequence int64
	require.NoError(t, db.Model(&recipientPO{}).
		Select("COALESCE(MAX(sequence_no), 0)").
		Scan(&maxSequence).Error)
	var maxBusinessID int64
	require.NoError(t, db.Model(&recipientPO{}).
		Select("COALESCE(MAX(id), 0)").
		Scan(&maxBusinessID).Error)
	require.Equal(t, maxSequence, page.SnapshotCutoff)
	require.NotEqual(t, maxBusinessID, page.SnapshotCutoff)
	require.Equal(t, page.Items[0].SequenceNo, page.NextCursor.SequenceNo)
}

func TestNotificationRepositoryUnitMaterializationSequenceFollowsTransactionCommitOrder(t *testing.T) {
	db, repository := newTestRepository(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)

	firstTx := db.Begin()
	require.NoError(t, firstTx.Error)
	first, err := repository.allocateRecipientSequences(
		context.Background(),
		firstTx,
		2,
		time.Now().UnixMilli(),
	)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, first)

	type allocationResult struct {
		sequences []int64
		err       error
	}
	started := make(chan struct{})
	finished := make(chan allocationResult, 1)
	go func() {
		secondTx := db.Begin()
		if secondTx.Error != nil {
			finished <- allocationResult{err: secondTx.Error}
			return
		}
		close(started)
		sequences, allocationErr := repository.allocateRecipientSequences(
			context.Background(),
			secondTx,
			1,
			time.Now().UnixMilli(),
		)
		if allocationErr == nil {
			allocationErr = secondTx.Commit().Error
		} else {
			_ = secondTx.Rollback().Error
		}
		finished <- allocationResult{
			sequences: sequences,
			err:       allocationErr,
		}
	}()

	<-started
	select {
	case result := <-finished:
		require.Failf(
			t,
			"second allocation completed before first commit",
			"sequences=%v error=%v",
			result.sequences,
			result.err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, firstTx.Commit().Error)
	select {
	case result := <-finished:
		require.NoError(t, result.err)
		require.Equal(t, []int64{3}, result.sequences)
	case <-time.After(5 * time.Second):
		require.Fail(t, "second allocation did not resume after first commit")
	}
}

func TestNotificationRepositoryUnitMarkAllReadUsesSnapshotCutoff(t *testing.T) {
	_, repository := newTestRepository(t)
	ctx := context.Background()

	materializeTestEvent(t, repository, testEvent("evt-before-cutoff", 1), 101, "worker-a")
	page, err := repository.ListForUser(ctx, domainnotification.ListFilter{
		UserID: 101,
		Limit:  20,
	})
	require.NoError(t, err)
	require.Positive(t, page.SnapshotCutoff)

	materializeTestEvent(t, repository, testEvent("evt-after-cutoff", 2), 101, "worker-b")
	affected, err := repository.MarkAllRead(
		ctx,
		101,
		page.SnapshotCutoff,
		1_721_000_000_300,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	unread, err := repository.CountUnread(ctx, 101)
	require.NoError(t, err)
	require.Equal(t, int64(1), unread)
}

func TestNotificationRepositoryUnitRecoverExpiredLease(t *testing.T) {
	db, repository := newTestRepository(t)
	ctx := context.Background()
	event := testEvent("evt-recovery", 1)
	require.NoError(t, repository.Append(ctx, event))

	expiredAt := time.UnixMilli(1_721_000_000_000)
	require.NoError(t, db.Model(&outboxPO{}).
		Where("event_id = ?", event.EventID).
		Updates(map[string]any{
			"status":    domainnotification.OutboxProcessing,
			"locked_by": "lost-worker",
			"locked_at": expiredAt.UnixMilli(),
		}).Error)

	recovered, err := repository.RecoverExpiredLeases(
		ctx,
		expiredAt.Add(time.Minute),
		30*time.Second,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), recovered)
}

func newTestRepository(t *testing.T) (*gorm.DB, *MySQLRepository) {
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
		&outboxPO{},
		&messagePO{},
		&notificationSequencePO{},
		&recipientPO{},
	))
	require.NoError(t, db.Create(&notificationSequencePO{
		SequenceKey: notificationRecipientSequenceKey,
		NextValue:   1,
	}).Error)
	return db, NewMySQLRepository(db, &sequenceIDGenerator{next: 10_000})
}

func materializeTestEvent(
	t *testing.T,
	repository *MySQLRepository,
	event domainnotification.Event,
	userID int64,
	workerID string,
) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repository.Append(ctx, event))
	claims, err := repository.ClaimOutboxBatch(
		ctx,
		workerID,
		time.Now(),
		30*time.Second,
		10,
	)
	require.NoError(t, err)
	require.NotEmpty(t, claims)
	draft, err := domainnotification.DefaultTemplateRegistry().Render(event)
	require.NoError(t, err)
	require.NoError(t, repository.MaterializeAndDeliver(
		ctx,
		claims[0],
		draft,
		[]int64{userID},
		time.Now(),
	))
}

func testEvent(eventID string, version int64) domainnotification.Event {
	return domainnotification.Event{
		EventID:          eventID,
		EventType:        domainnotification.EventAgentRunSucceeded,
		AggregateType:    "agent_run",
		AggregateID:      "run-stable",
		AggregateVersion: version,
		OccurredAt:       time.UnixMilli(1_721_000_000_000 + version),
		ActorID:          101,
		SpaceID:          202,
		RecipientPolicy:  domainnotification.RecipientActor,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			TargetID: "thread-100",
		},
	}
}
