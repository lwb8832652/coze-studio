// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const (
	maxClaimBatch                      = 200
	maxReadPage                        = 100
	recipientBatchSize                 = 200
	notificationRecipientSequenceKey   = "recipient_materialization"
	maxNotificationRecipientSequenceNo = int64(1<<63 - 1)
	notificationTransactionMaxAttempts = 3
	notificationTransactionRetryDelay  = 5 * time.Millisecond
)

type MySQLRepository struct {
	db            *gorm.DB
	idGen         idgen.IDGenerator
	databaseClock notificationDatabaseClock
}

type notificationDatabaseClock func(
	context.Context,
	*gorm.DB,
	time.Time,
) (time.Time, error)

var ErrNotificationDatabaseClock = errors.New(
	"notification database clock unavailable",
)

func NewMySQLRepository(db *gorm.DB, idGen idgen.IDGenerator) *MySQLRepository {
	return &MySQLRepository{db: db, idGen: idGen}
}

func (r *MySQLRepository) currentDatabaseTime(
	ctx context.Context,
	db *gorm.DB,
	sqliteFallback time.Time,
) (time.Time, error) {
	if r == nil || db == nil {
		return time.Time{}, notificationDatabaseClockError()
	}
	if r.databaseClock != nil {
		now, err := r.databaseClock(ctx, db, sqliteFallback)
		if err != nil || now.IsZero() {
			return time.Time{}, notificationDatabaseClockError()
		}
		return now.UTC().Truncate(time.Millisecond), nil
	}
	switch db.Dialector.Name() {
	case "mysql":
		var epochMillis int64
		err := db.WithContext(ctx).Raw(`
			SELECT TIMESTAMPDIFF(
				MICROSECOND,
				'1970-01-01 00:00:00',
				UTC_TIMESTAMP(3)
			) DIV 1000
		`).Scan(&epochMillis).Error
		if err != nil || epochMillis <= 0 {
			return time.Time{}, notificationDatabaseClockError()
		}
		return time.UnixMilli(epochMillis).UTC(), nil
	case "sqlite":
		// SQLite is an explicit deterministic unit-test dialect. Production
		// MySQL never uses caller time for persisted lease boundaries.
		if sqliteFallback.IsZero() {
			return time.Time{}, notificationDatabaseClockError()
		}
		return sqliteFallback.UTC().Truncate(time.Millisecond), nil
	default:
		return time.Time{}, notificationDatabaseClockError()
	}
}

func notificationDatabaseClockError() error {
	return fmt.Errorf(
		"%w: %w",
		domainnotification.ErrStorage,
		ErrNotificationDatabaseClock,
	)
}

func runNotificationTransaction(
	ctx context.Context,
	db *gorm.DB,
	transaction func(*gorm.DB) error,
) error {
	if db == nil || transaction == nil {
		return domainnotification.ErrStorage
	}
	for attempt := 0; attempt < notificationTransactionMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := db.WithContext(ctx).Transaction(transaction)
		if err == nil ||
			!isRetryableNotificationTransactionError(err) ||
			attempt == notificationTransactionMaxAttempts-1 {
			return err
		}
		timer := time.NewTimer(
			notificationTransactionRetryDelay * time.Duration(attempt+1),
		)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return domainnotification.ErrStorage
}

func isRetryableNotificationTransactionError(err error) bool {
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) {
		return false
	}
	return mysqlError.Number == 1205 ||
		mysqlError.Number == 1213 ||
		string(mysqlError.SQLState[:]) == "40001"
}

func notificationStorageError(operation string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: %s", domainnotification.ErrStorage, operation)
	}
	return fmt.Errorf(
		"%w: %s: %w",
		domainnotification.ErrStorage,
		operation,
		cause,
	)
}

type outboxPO struct {
	ID               int64  `gorm:"column:id;primaryKey"`
	EventID          string `gorm:"column:event_id;size:128;uniqueIndex:uk_notification_outbox_event"`
	IdempotencyKey   string `gorm:"column:idempotency_key;size:64;uniqueIndex:uk_notification_outbox_idempotency"`
	EventType        string `gorm:"column:event_type;size:64"`
	AggregateType    string `gorm:"column:aggregate_type;size:64"`
	AggregateID      string `gorm:"column:aggregate_id;size:128"`
	AggregateVersion int64  `gorm:"column:aggregate_version"`
	OccurredAt       int64  `gorm:"column:occurred_at"`
	SpaceID          int64  `gorm:"column:space_id"`
	ActorID          int64  `gorm:"column:actor_id"`
	RecipientPolicy  string `gorm:"column:recipient_policy;size:64"`
	PayloadSchema    int32  `gorm:"column:payload_schema"`
	PayloadJSON      []byte `gorm:"column:payload_json;type:json"`
	Status           string `gorm:"column:status;size:16;index:idx_notification_outbox_ready"`
	AttemptCount     int    `gorm:"column:attempt_count"`
	AvailableAt      int64  `gorm:"column:available_at;index:idx_notification_outbox_ready"`
	LockedAt         int64  `gorm:"column:locked_at;index:idx_notification_outbox_lease"`
	LockedBy         string `gorm:"column:locked_by;size:128"`
	LastErrorCode    string `gorm:"column:last_error_code;size:64"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
	DeliveredAt      int64  `gorm:"column:delivered_at"`
}

func (outboxPO) TableName() string {
	return "notification_outbox"
}

type messagePO struct {
	ID         int64  `gorm:"column:id;primaryKey"`
	EventID    string `gorm:"column:event_id;size:128;uniqueIndex:uk_notification_messages_event"`
	Scope      string `gorm:"column:scope;size:32"`
	SpaceID    int64  `gorm:"column:space_id"`
	SenderID   int64  `gorm:"column:sender_id"`
	Category   string `gorm:"column:category;size:32"`
	Severity   string `gorm:"column:severity;size:16"`
	EventType  string `gorm:"column:event_type;size:64"`
	Title      string `gorm:"column:title;size:128"`
	Content    string `gorm:"column:content;size:512"`
	TargetType string `gorm:"column:target_type;size:32"`
	TargetID   string `gorm:"column:target_id;size:128"`
	CreatedAt  int64  `gorm:"column:created_at;index:idx_notification_messages_created"`
}

func (messagePO) TableName() string {
	return "notification_messages"
}

type recipientPO struct {
	ID             int64 `gorm:"column:id;primaryKey"`
	SequenceNo     int64 `gorm:"column:sequence_no;not null;uniqueIndex:uk_notification_recipients_sequence;index:idx_notification_recipients_user_read_sequence;index:idx_notification_recipients_user_sequence"`
	NotificationID int64 `gorm:"column:notification_id;uniqueIndex:uk_notification_recipients_message_user"`
	UserID         int64 `gorm:"column:user_id;uniqueIndex:uk_notification_recipients_message_user;index:idx_notification_recipients_user_read_sequence;index:idx_notification_recipients_user_sequence"`
	ReadAt         int64 `gorm:"column:read_at;index:idx_notification_recipients_user_read_sequence"`
	CreatedAt      int64 `gorm:"column:created_at"`
}

func (recipientPO) TableName() string {
	return "notification_recipients"
}

type notificationSequencePO struct {
	SequenceKey string `gorm:"column:sequence_key;size:64;primaryKey"`
	NextValue   int64  `gorm:"column:next_value;not null"`
	UpdatedAt   int64  `gorm:"column:updated_at;not null"`
}

func (notificationSequencePO) TableName() string {
	return "notification_sequence"
}

func (r *MySQLRepository) Append(
	ctx context.Context,
	event domainnotification.Event,
) error {
	_, err := r.AppendWithResult(ctx, event)
	return err
}

func (r *MySQLRepository) AppendWithResult(
	ctx context.Context,
	event domainnotification.Event,
) (bool, error) {
	if r == nil || r.db == nil {
		return false, domainnotification.ErrStorage
	}
	inserted := false
	err := runNotificationTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var appendErr error
		inserted, appendErr = r.AppendInTransactionWithResult(ctx, tx, event)
		return appendErr
	})
	return inserted, err
}

func (r *MySQLRepository) AppendInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	_, err := r.AppendInTransactionWithResult(ctx, tx, event)
	return err
}

func (r *MySQLRepository) AppendInTransactionWithResult(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) (bool, error) {
	if r == nil || r.idGen == nil || tx == nil {
		return false, domainnotification.ErrStorage
	}
	event, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return false, notificationStorageError("encode payload", err)
	}
	databaseNow, err := r.currentDatabaseTime(ctx, tx, event.OccurredAt)
	if err != nil {
		return false, err
	}
	id, err := r.idGen.GenID(ctx)
	if err != nil || id <= 0 {
		return false, notificationStorageError("allocate outbox ID", err)
	}
	now := databaseNow.UnixMilli()
	row := &outboxPO{
		ID:               id,
		EventID:          strings.TrimSpace(event.EventID),
		IdempotencyKey:   event.IdempotencyKey(),
		EventType:        string(event.EventType),
		AggregateType:    strings.TrimSpace(event.AggregateType),
		AggregateID:      strings.TrimSpace(event.AggregateID),
		AggregateVersion: event.AggregateVersion,
		OccurredAt:       event.OccurredAt.UnixMilli(),
		SpaceID:          event.SpaceID,
		ActorID:          event.ActorID,
		RecipientPolicy:  string(event.RecipientPolicy),
		PayloadSchema:    event.PayloadSchema,
		PayloadJSON:      payload,
		Status:           string(domainnotification.OutboxPending),
		AvailableAt:      now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		return false, notificationStorageError("append outbox", result.Error)
	}

	existing, err := findOutboxIdentity(ctx, tx, row.EventID, row.IdempotencyKey)
	if err != nil {
		return false, err
	}
	if !sameOutboxIdentity(existing, row) {
		return false, domainnotification.ErrIdempotencyConflict
	}
	return existing.ID == row.ID, nil
}

func (r *MySQLRepository) RecoverExpiredLeases(
	ctx context.Context,
	now time.Time,
	lease time.Duration,
) (int64, error) {
	if r == nil || r.db == nil || lease <= 0 {
		return 0, domainnotification.ErrStorage
	}
	databaseNow, err := r.currentDatabaseTime(ctx, r.db, now)
	if err != nil {
		return 0, err
	}
	now = databaseNow
	result := r.db.WithContext(ctx).Model(&outboxPO{}).
		Where(
			"status = ? AND locked_at > 0 AND locked_at <= ?",
			domainnotification.OutboxProcessing,
			now.Add(-lease).UnixMilli(),
		).
		Updates(map[string]any{
			"status":       domainnotification.OutboxPending,
			"available_at": now.UnixMilli(),
			"locked_at":    0,
			"locked_by":    "",
			"updated_at":   now.UnixMilli(),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("%w: recover leases", domainnotification.ErrStorage)
	}
	return result.RowsAffected, nil
}

func (r *MySQLRepository) ClaimOutboxBatch(
	ctx context.Context,
	workerID string,
	now time.Time,
	lease time.Duration,
	limit int,
) ([]domainnotification.OutboxClaim, error) {
	if r == nil || r.db == nil ||
		strings.TrimSpace(workerID) == "" ||
		lease <= 0 ||
		limit <= 0 ||
		limit > maxClaimBatch {
		return nil, domainnotification.ErrStorage
	}
	var rows []outboxPO
	err := runNotificationTransaction(ctx, r.db, func(tx *gorm.DB) error {
		rows = nil
		databaseNow, clockErr := r.currentDatabaseTime(ctx, tx, now)
		if clockErr != nil {
			return clockErr
		}
		now = databaseNow
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(
				"status = ? AND available_at <= ?",
				domainnotification.OutboxPending,
				now.UnixMilli(),
			).
			Order("id ASC").
			Limit(limit).
			Find(&rows).Error; err != nil {
			return notificationStorageError("select claim batch", err)
		}
		if len(rows) == 0 {
			return nil
		}
		lockedAt := now
		ids := make([]int64, len(rows))
		for index := range rows {
			ids[index] = rows[index].ID
		}
		result := tx.Model(&outboxPO{}).
			Where("id IN ? AND status = ?", ids, domainnotification.OutboxPending).
			Updates(map[string]any{
				"status":     domainnotification.OutboxProcessing,
				"locked_at":  lockedAt.UnixMilli(),
				"locked_by":  workerID,
				"updated_at": lockedAt.UnixMilli(),
			})
		if result.Error != nil {
			return notificationStorageError("acquire claim batch", result.Error)
		}
		if result.RowsAffected != int64(len(rows)) {
			return notificationStorageError("acquire claim batch", nil)
		}
		for index := range rows {
			rows[index].LockedAt = lockedAt.UnixMilli()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	claimed := make([]domainnotification.OutboxClaim, 0, len(rows))
	for index := range rows {
		event, decodeErr := outboxToEvent(&rows[index])
		claim := domainnotification.OutboxClaim{
			OutboxID:       rows[index].ID,
			LockedBy:       workerID,
			LeaseExpiresAt: time.UnixMilli(rows[index].LockedAt).UTC().Add(lease),
			AttemptCount:   rows[index].AttemptCount,
			Event:          event,
		}
		if decodeErr != nil {
			claim.Event = outboxMetadataToEvent(&rows[index])
			claim.ClaimErrorCode = domainnotification.StableErrorCode(decodeErr)
		}
		claimed = append(claimed, claim)
	}
	return claimed, nil
}

func (r *MySQLRepository) MaterializeAndDeliver(
	ctx context.Context,
	claim domainnotification.OutboxClaim,
	draft domainnotification.MessageDraft,
	recipientIDs []int64,
	now time.Time,
) error {
	if r == nil || r.db == nil || r.idGen == nil ||
		claim.OutboxID <= 0 ||
		strings.TrimSpace(claim.LockedBy) == "" {
		return domainnotification.ErrStorage
	}
	recipients, err := domainnotification.NormalizeRecipientIDs(recipientIDs)
	if err != nil {
		return err
	}
	sort.Slice(recipients, func(left, right int) bool {
		return recipients[left] < recipients[right]
	})
	if err := validateDraft(claim.Event, draft); err != nil {
		return err
	}
	return runNotificationTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var outbox outboxPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", claim.OutboxID).
			Take(&outbox).Error; err != nil {
			return notificationStorageError("lock outbox", err)
		}
		if outbox.Status != string(domainnotification.OutboxProcessing) ||
			outbox.LockedBy != claim.LockedBy ||
			outbox.EventID != claim.Event.EventID {
			return domainnotification.ErrLeaseLost
		}
		databaseNow, clockErr := r.currentDatabaseTime(ctx, tx, now)
		if clockErr != nil {
			return clockErr
		}
		now = databaseNow
		if claim.LeaseExpiresAt.IsZero() ||
			!now.Before(claim.LeaseExpiresAt) {
			return domainnotification.ErrLeaseLost
		}
		message, err := r.findOrCreateMessage(ctx, tx, draft)
		if err != nil {
			return err
		}
		sequences, err := r.allocateRecipientSequences(
			ctx,
			tx,
			len(recipients),
			now.UnixMilli(),
		)
		if err != nil {
			return err
		}
		if err := r.createRecipients(
			ctx,
			tx,
			message.ID,
			recipients,
			sequences,
			now.UnixMilli(),
		); err != nil {
			return err
		}
		result := tx.Model(&outboxPO{}).
			Where(
				"id = ? AND status = ? AND locked_by = ?",
				claim.OutboxID,
				domainnotification.OutboxProcessing,
				claim.LockedBy,
			).
			Updates(map[string]any{
				"status":          domainnotification.OutboxDelivered,
				"delivered_at":    now.UnixMilli(),
				"locked_at":       0,
				"locked_by":       "",
				"last_error_code": "",
				"updated_at":      now.UnixMilli(),
			})
		if result.Error != nil {
			return notificationStorageError("complete outbox", result.Error)
		}
		if result.RowsAffected != 1 {
			return domainnotification.ErrLeaseLost
		}
		return nil
	})
}

func (r *MySQLRepository) FailClaim(
	ctx context.Context,
	claim domainnotification.OutboxClaim,
	errorCode string,
	now time.Time,
	nextAvailableAt time.Time,
	dead bool,
) error {
	if r == nil || r.db == nil || claim.OutboxID <= 0 ||
		claim.LockedBy == "" {
		return domainnotification.ErrStorage
	}
	retryDelay := time.Duration(0)
	if !dead && !nextAvailableAt.IsZero() && !now.IsZero() {
		retryDelay = nextAvailableAt.Sub(now)
		if retryDelay < 0 {
			retryDelay = 0
		}
	}
	errorCode = normalizeErrorCode(errorCode)
	return runNotificationTransaction(ctx, r.db, func(tx *gorm.DB) error {
		var outbox outboxPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", claim.OutboxID).
			Take(&outbox).Error; err != nil {
			return notificationStorageError("lock failed claim", err)
		}
		if outbox.Status != string(domainnotification.OutboxProcessing) ||
			outbox.LockedBy != claim.LockedBy ||
			outbox.EventID != claim.Event.EventID {
			return domainnotification.ErrLeaseLost
		}
		databaseNow, clockErr := r.currentDatabaseTime(ctx, tx, now)
		if clockErr != nil {
			return clockErr
		}
		if claim.LeaseExpiresAt.IsZero() ||
			!databaseNow.Before(claim.LeaseExpiresAt) {
			return domainnotification.ErrLeaseLost
		}
		status := domainnotification.OutboxPending
		if dead {
			status = domainnotification.OutboxDead
		}
		nextAvailableAt = databaseNow.Add(retryDelay)
		result := tx.Model(&outboxPO{}).
			Where(
				"id = ? AND status = ? AND locked_by = ?",
				claim.OutboxID,
				domainnotification.OutboxProcessing,
				claim.LockedBy,
			).
			Updates(map[string]any{
				"status":          status,
				"attempt_count":   claim.AttemptCount + 1,
				"available_at":    nextAvailableAt.UnixMilli(),
				"locked_at":       0,
				"locked_by":       "",
				"last_error_code": errorCode,
				"updated_at":      databaseNow.UnixMilli(),
			})
		if result.Error != nil {
			return notificationStorageError("fail claim", result.Error)
		}
		if result.RowsAffected != 1 {
			return domainnotification.ErrLeaseLost
		}
		return nil
	})
}

func (r *MySQLRepository) ListForUser(
	ctx context.Context,
	filter domainnotification.ListFilter,
) (domainnotification.ListPage, error) {
	if r == nil || r.db == nil ||
		filter.UserID <= 0 ||
		filter.Limit <= 0 ||
		filter.Limit > maxReadPage {
		return domainnotification.ListPage{}, domainnotification.ErrInvalidEvent
	}
	type joinedRow struct {
		RecipientID int64  `gorm:"column:recipient_id"`
		SequenceNo  int64  `gorm:"column:sequence_no"`
		UserID      int64  `gorm:"column:user_id"`
		ReadAt      int64  `gorm:"column:read_at"`
		MessageID   int64  `gorm:"column:message_id"`
		EventID     string `gorm:"column:event_id"`
		Scope       string `gorm:"column:scope"`
		SpaceID     int64  `gorm:"column:space_id"`
		SenderID    int64  `gorm:"column:sender_id"`
		Category    string `gorm:"column:category"`
		Severity    string `gorm:"column:severity"`
		EventType   string `gorm:"column:event_type"`
		Title       string `gorm:"column:title"`
		Content     string `gorm:"column:content"`
		TargetType  string `gorm:"column:target_type"`
		TargetID    string `gorm:"column:target_id"`
		MessageAt   int64  `gorm:"column:message_created_at"`
	}
	page := domainnotification.ListPage{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cutoff := filter.Cursor.SnapshotCutoff
		if filter.HasCursor {
			if filter.Cursor.CreatedAt <= 0 ||
				filter.Cursor.SequenceNo <= 0 ||
				cutoff <= 0 {
				return domainnotification.ErrInvalidCursor
			}
		} else {
			if err := tx.Model(&recipientPO{}).
				Where("user_id = ?", filter.UserID).
				Select("COALESCE(MAX(sequence_no), 0)").
				Scan(&cutoff).Error; err != nil {
				return fmt.Errorf(
					"%w: read list snapshot cutoff",
					domainnotification.ErrStorage,
				)
			}
		}
		page.SnapshotCutoff = cutoff
		page.Items = []domainnotification.RecipientMessage{}
		if cutoff == 0 {
			return nil
		}

		query := tx.
			Table("notification_recipients AS r").
			Select(`
				r.id AS recipient_id,
				r.sequence_no,
				r.user_id,
				r.read_at,
				m.id AS message_id,
				m.event_id,
				m.scope,
				m.space_id,
				m.sender_id,
				m.category,
				m.severity,
				m.event_type,
				m.title,
				m.content,
				m.target_type,
				m.target_id,
				m.created_at AS message_created_at
			`).
			Joins("JOIN notification_messages AS m ON m.id = r.notification_id").
			Where(
				"r.user_id = ? AND r.sequence_no <= ?",
				filter.UserID,
				cutoff,
			)
		if filter.UnreadOnly {
			query = query.Where("r.read_at = 0")
		}
		if filter.HasCursor {
			query = query.Where(
				"(m.created_at < ?) OR (m.created_at = ? AND r.sequence_no < ?)",
				filter.Cursor.CreatedAt,
				filter.Cursor.CreatedAt,
				filter.Cursor.SequenceNo,
			)
		}
		var rows []joinedRow
		if err := query.
			Order("m.created_at DESC").
			Order("r.sequence_no DESC").
			Limit(filter.Limit + 1).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf(
				"%w: list notifications",
				domainnotification.ErrStorage,
			)
		}
		if len(rows) > filter.Limit {
			page.HasMore = true
			rows = rows[:filter.Limit]
		}
		page.Items = make([]domainnotification.RecipientMessage, 0, len(rows))
		for _, row := range rows {
			page.Items = append(page.Items, domainnotification.RecipientMessage{
				RecipientID: row.RecipientID,
				SequenceNo:  row.SequenceNo,
				UserID:      row.UserID,
				ReadAt:      row.ReadAt,
				Message: domainnotification.Message{
					ID:         row.MessageID,
					EventID:    row.EventID,
					Scope:      domainnotification.Scope(row.Scope),
					SpaceID:    row.SpaceID,
					SenderID:   row.SenderID,
					Category:   domainnotification.Category(row.Category),
					Severity:   domainnotification.Severity(row.Severity),
					EventType:  domainnotification.EventType(row.EventType),
					Title:      row.Title,
					Content:    row.Content,
					TargetType: domainnotification.TargetType(row.TargetType),
					TargetID:   row.TargetID,
					CreatedAt:  row.MessageAt,
				},
			})
		}
		if page.HasMore && len(page.Items) > 0 {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = domainnotification.Cursor{
				CreatedAt:      last.Message.CreatedAt,
				SequenceNo:     last.SequenceNo,
				SnapshotCutoff: cutoff,
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return page, err
}

func (r *MySQLRepository) CountUnread(ctx context.Context, userID int64) (int64, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return 0, domainnotification.ErrInvalidEvent
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&recipientPO{}).
		Where("user_id = ? AND read_at = 0", userID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("%w: count unread", domainnotification.ErrStorage)
	}
	return count, nil
}

func (r *MySQLRepository) MarkRead(
	ctx context.Context,
	userID int64,
	notificationIDs []int64,
	readAt int64,
) (int64, error) {
	if r == nil || r.db == nil || userID <= 0 ||
		len(notificationIDs) == 0 ||
		len(notificationIDs) > 100 ||
		readAt <= 0 {
		return 0, domainnotification.ErrInvalidEvent
	}
	for _, id := range notificationIDs {
		if id <= 0 {
			return 0, domainnotification.ErrInvalidEvent
		}
	}
	result := r.db.WithContext(ctx).Model(&recipientPO{}).
		Where(
			"user_id = ? AND notification_id IN ? AND read_at = 0",
			userID,
			notificationIDs,
		).
		Update("read_at", readAt)
	if result.Error != nil {
		return 0, fmt.Errorf("%w: mark read", domainnotification.ErrStorage)
	}
	return result.RowsAffected, nil
}

func (r *MySQLRepository) MarkAllRead(
	ctx context.Context,
	userID int64,
	cutoff int64,
	readAt int64,
) (int64, error) {
	if r == nil || r.db == nil || userID <= 0 || cutoff <= 0 || readAt <= 0 {
		return 0, domainnotification.ErrInvalidEvent
	}
	result := r.db.WithContext(ctx).Model(&recipientPO{}).
		Where(
			"user_id = ? AND sequence_no <= ? AND read_at = 0",
			userID,
			cutoff,
		).
		Update("read_at", readAt)
	if result.Error != nil {
		return 0, fmt.Errorf("%w: mark snapshot read", domainnotification.ErrStorage)
	}
	return result.RowsAffected, nil
}

func findOutboxIdentity(
	ctx context.Context,
	tx *gorm.DB,
	eventID string,
	idempotencyKey string,
) (*outboxPO, error) {
	var row outboxPO
	query := tx.WithContext(ctx)
	if requiresOutboxIdentityCurrentRead(tx.Dialector.Name()) {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.
		Where("idempotency_key = ? OR event_id = ?", idempotencyKey, eventID).
		Order("id ASC").
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainnotification.ErrIdempotencyConflict
		}
		return nil, notificationStorageError("read outbox identity", err)
	}
	return &row, nil
}

func requiresOutboxIdentityCurrentRead(dialect string) bool {
	return strings.EqualFold(strings.TrimSpace(dialect), "mysql")
}

func sameOutboxIdentity(left *outboxPO, right *outboxPO) bool {
	if !(left != nil &&
		right != nil &&
		left.IdempotencyKey == right.IdempotencyKey &&
		left.EventType == right.EventType &&
		strings.TrimSpace(left.AggregateType) == strings.TrimSpace(right.AggregateType) &&
		strings.TrimSpace(left.AggregateID) == strings.TrimSpace(right.AggregateID) &&
		left.AggregateVersion == right.AggregateVersion &&
		left.SpaceID == right.SpaceID &&
		left.ActorID == right.ActorID &&
		strings.EqualFold(
			strings.TrimSpace(left.RecipientPolicy),
			strings.TrimSpace(right.RecipientPolicy),
		) &&
		left.PayloadSchema == right.PayloadSchema) {
		return false
	}
	leftPayload, leftErr := decodeEventPayload(left.PayloadJSON)
	rightPayload, rightErr := decodeEventPayload(right.PayloadJSON)
	return leftErr == nil &&
		rightErr == nil &&
		sameEventPayload(leftPayload, rightPayload)
}

func outboxToEvent(row *outboxPO) (domainnotification.Event, error) {
	if row == nil {
		return domainnotification.Event{}, domainnotification.ErrInvalidEvent
	}
	event := outboxMetadataToEvent(row)
	payload, err := decodeEventPayload(row.PayloadJSON)
	if err != nil {
		return domainnotification.Event{}, fmt.Errorf(
			"%w: decode typed payload",
			domainnotification.ErrInvalidEvent,
		)
	}
	event.Payload = payload
	if err := event.Validate(); err != nil {
		return domainnotification.Event{}, err
	}
	return event, nil
}

type rawEventPayload struct {
	ResourceDisplayName  json.RawMessage `json:"resource_display_name"`
	ActorDisplayName     json.RawMessage `json:"actor_display_name"`
	StatusReasonCode     json.RawMessage `json:"status_reason_code"`
	TargetID             json.RawMessage `json:"target_id"`
	AnnouncementTitle    json.RawMessage `json:"announcement_title"`
	AnnouncementBody     json.RawMessage `json:"announcement_body"`
	AnnouncementSeverity json.RawMessage `json:"announcement_severity"`
	AnnouncementRoute    json.RawMessage `json:"announcement_route"`
	WorkspaceAction      json.RawMessage `json:"workspace_action"`
	WorkspaceAudience    json.RawMessage `json:"workspace_audience"`
	SubjectDisplayName   json.RawMessage `json:"subject_display_name"`
	InternalRoute        json.RawMessage `json:"internal_route"`
	ExplicitRecipientIDs json.RawMessage `json:"explicit_recipient_ids"`
}

func decodeEventPayload(raw []byte) (domainnotification.EventPayload, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var encoded rawEventPayload
	if err := decoder.Decode(&encoded); err != nil {
		return domainnotification.EventPayload{}, domainnotification.ErrInvalidEvent
	}
	if err := requireJSONEOF(decoder); err != nil {
		return domainnotification.EventPayload{}, err
	}
	resourceName, err := decodePayloadString(encoded.ResourceDisplayName)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	actorName, err := decodePayloadString(encoded.ActorDisplayName)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	reason, err := decodePayloadString(encoded.StatusReasonCode)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	targetID, err := decodePayloadString(encoded.TargetID)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	announcementTitle, err := decodePayloadString(encoded.AnnouncementTitle)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	announcementBody, err := decodePayloadString(encoded.AnnouncementBody)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	announcementSeverity, err := decodePayloadString(encoded.AnnouncementSeverity)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	announcementRoute, err := decodeAnnouncementRoute(
		encoded.AnnouncementRoute,
	)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	workspaceAction, err := decodePayloadString(encoded.WorkspaceAction)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	workspaceAudience, err := decodePayloadString(encoded.WorkspaceAudience)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	subjectDisplayName, err := decodePayloadString(encoded.SubjectDisplayName)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	internalRoute, err := decodePayloadString(encoded.InternalRoute)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	if internalRoute != "" {
		if announcementRoute != nil {
			return domainnotification.EventPayload{}, domainnotification.ErrInvalidEvent
		}
		announcementRoute, err = decodeLegacyAnnouncementRoute(internalRoute)
		if err != nil {
			return domainnotification.EventPayload{}, err
		}
	}
	recipientIDs, err := decodePayloadIntegerList(encoded.ExplicitRecipientIDs)
	if err != nil {
		return domainnotification.EventPayload{}, err
	}
	if len(recipientIDs) > 0 {
		recipientIDs, err = domainnotification.NormalizeRecipientIDs(recipientIDs)
		if err != nil {
			return domainnotification.EventPayload{}, domainnotification.ErrInvalidEvent
		}
	}
	return domainnotification.EventPayload{
		ResourceDisplayName:  resourceName,
		ActorDisplayName:     actorName,
		StatusReasonCode:     domainnotification.StatusReasonCode(reason),
		TargetID:             targetID,
		AnnouncementTitle:    announcementTitle,
		AnnouncementBody:     announcementBody,
		AnnouncementSeverity: domainnotification.Severity(announcementSeverity),
		AnnouncementRoute:    announcementRoute,
		WorkspaceAction: domainnotification.WorkspaceMemberAction(
			workspaceAction,
		),
		WorkspaceAudience: domainnotification.WorkspaceMemberAudience(
			workspaceAudience,
		),
		SubjectDisplayName:   subjectDisplayName,
		ExplicitRecipientIDs: recipientIDs,
	}, nil
}

func decodeAnnouncementRoute(
	raw json.RawMessage,
) (*domainnotification.AnnouncementRoute, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, domainnotification.ErrInvalidEvent
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var route domainnotification.AnnouncementRoute
	if err := decoder.Decode(&route); err != nil {
		return nil, domainnotification.ErrInvalidEvent
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	normalized, err := domainnotification.NormalizeAnnouncementRoute(route)
	if err != nil {
		return nil, domainnotification.ErrInvalidEvent
	}
	return &normalized, nil
}

func decodeLegacyAnnouncementRoute(
	value string,
) (*domainnotification.AnnouncementRoute, error) {
	value = strings.TrimSpace(value)
	if value == "/system/announcements" {
		return &domainnotification.AnnouncementRoute{
			Type: domainnotification.AnnouncementRouteSystemAnnouncements,
		}, nil
	}
	const prefix = "/space/"
	const suffix = "/workspace"
	if strings.HasPrefix(value, prefix) && strings.HasSuffix(value, suffix) {
		rawID := strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
		spaceID, err := strconv.ParseInt(rawID, 10, 64)
		if err == nil &&
			spaceID > 0 &&
			strconv.FormatInt(spaceID, 10) == rawID {
			return &domainnotification.AnnouncementRoute{
				Type:    domainnotification.AnnouncementRouteWorkspaceHome,
				SpaceID: spaceID,
			}, nil
		}
	}
	return nil, domainnotification.ErrInvalidEvent
}

func decodePayloadString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	if string(raw) == "null" {
		return "", domainnotification.ErrInvalidEvent
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", domainnotification.ErrInvalidEvent
	}
	return value, nil
}

func decodePayloadIntegerList(raw json.RawMessage) ([]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, domainnotification.ErrInvalidEvent
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var numbers []json.Number
	if err := decoder.Decode(&numbers); err != nil {
		return nil, domainnotification.ErrInvalidEvent
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(numbers))
	for _, number := range numbers {
		value, ok := new(big.Rat).SetString(number.String())
		if !ok || !value.IsInt() || !value.Num().IsInt64() {
			return nil, domainnotification.ErrInvalidEvent
		}
		result = append(result, value.Num().Int64())
	}
	return result, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return domainnotification.ErrInvalidEvent
	}
	return nil
}

func sameEventPayload(
	left domainnotification.EventPayload,
	right domainnotification.EventPayload,
) bool {
	if left.ResourceDisplayName != right.ResourceDisplayName ||
		left.ActorDisplayName != right.ActorDisplayName ||
		left.StatusReasonCode != right.StatusReasonCode ||
		left.TargetID != right.TargetID ||
		left.AnnouncementTitle != right.AnnouncementTitle ||
		left.AnnouncementBody != right.AnnouncementBody ||
		left.AnnouncementSeverity != right.AnnouncementSeverity ||
		!sameAnnouncementRoute(
			left.AnnouncementRoute,
			right.AnnouncementRoute,
		) ||
		left.WorkspaceAction != right.WorkspaceAction ||
		left.WorkspaceAudience != right.WorkspaceAudience ||
		left.SubjectDisplayName != right.SubjectDisplayName ||
		len(left.ExplicitRecipientIDs) != len(right.ExplicitRecipientIDs) {
		return false
	}
	for index := range left.ExplicitRecipientIDs {
		if left.ExplicitRecipientIDs[index] != right.ExplicitRecipientIDs[index] {
			return false
		}
	}
	return true
}

func sameAnnouncementRoute(
	left *domainnotification.AnnouncementRoute,
	right *domainnotification.AnnouncementRoute,
) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Type == right.Type && left.SpaceID == right.SpaceID
}

func outboxMetadataToEvent(row *outboxPO) domainnotification.Event {
	if row == nil {
		return domainnotification.Event{}
	}
	return domainnotification.Event{
		EventID:          row.EventID,
		EventType:        domainnotification.EventType(row.EventType),
		AggregateType:    row.AggregateType,
		AggregateID:      row.AggregateID,
		AggregateVersion: row.AggregateVersion,
		OccurredAt:       time.UnixMilli(row.OccurredAt),
		ActorID:          row.ActorID,
		SpaceID:          row.SpaceID,
		RecipientPolicy:  domainnotification.RecipientPolicy(row.RecipientPolicy),
		PayloadSchema:    row.PayloadSchema,
	}
}

func validateDraft(
	event domainnotification.Event,
	draft domainnotification.MessageDraft,
) error {
	if draft.EventID != event.EventID ||
		draft.EventType != event.EventType ||
		draft.Title == "" ||
		len([]rune(draft.Title)) > domainnotification.MaxNotificationTitleRunes ||
		len([]rune(draft.Content)) > domainnotification.MaxNotificationContentRunes ||
		len([]rune(draft.TargetID)) > domainnotification.MaxTargetIDRunes {
		return domainnotification.ErrInvalidEvent
	}
	return nil
}

func (r *MySQLRepository) findOrCreateMessage(
	ctx context.Context,
	tx *gorm.DB,
	draft domainnotification.MessageDraft,
) (*messagePO, error) {
	var existing messagePO
	err := tx.WithContext(ctx).
		Where("event_id = ?", draft.EventID).
		Take(&existing).Error
	if err == nil {
		if !sameMessageIdentity(&existing, draft) {
			return nil, domainnotification.ErrIdempotencyConflict
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notificationStorageError("find message", err)
	}
	id, err := r.idGen.GenID(ctx)
	if err != nil || id <= 0 {
		return nil, notificationStorageError("allocate message ID", err)
	}
	candidate := messageFromDraft(id, draft)
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(candidate).Error; err != nil {
		return nil, notificationStorageError("create message", err)
	}
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("event_id = ?", draft.EventID).
		Take(&existing).Error; err != nil {
		return nil, notificationStorageError("lock message", err)
	}
	if !sameMessageIdentity(&existing, draft) {
		return nil, domainnotification.ErrIdempotencyConflict
	}
	return &existing, nil
}

func (r *MySQLRepository) createRecipients(
	ctx context.Context,
	tx *gorm.DB,
	messageID int64,
	userIDs []int64,
	sequences []int64,
	createdAt int64,
) error {
	if len(sequences) != len(userIDs) {
		return fmt.Errorf(
			"%w: recipient sequence allocation mismatch",
			domainnotification.ErrStorage,
		)
	}
	ids, err := r.idGen.GenMultiIDs(ctx, len(userIDs))
	if err != nil || len(ids) != len(userIDs) {
		return notificationStorageError("allocate recipient IDs", err)
	}
	rows := make([]recipientPO, len(userIDs))
	for index, userID := range userIDs {
		if ids[index] <= 0 {
			return fmt.Errorf("%w: invalid recipient row ID", domainnotification.ErrStorage)
		}
		rows[index] = recipientPO{
			ID:             ids[index],
			SequenceNo:     sequences[index],
			NotificationID: messageID,
			UserID:         userID,
			CreatedAt:      createdAt,
		}
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "notification_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).CreateInBatches(rows, recipientBatchSize).Error; err != nil {
		return notificationStorageError("create recipients", err)
	}
	var count int64
	if err := tx.WithContext(ctx).Model(&recipientPO{}).
		Where("notification_id = ? AND user_id IN ?", messageID, userIDs).
		Count(&count).Error; err != nil {
		return notificationStorageError("verify recipients", err)
	}
	if count != int64(len(userIDs)) {
		return fmt.Errorf("%w: recipient projection incomplete", domainnotification.ErrStorage)
	}
	return nil
}

func (r *MySQLRepository) allocateRecipientSequences(
	ctx context.Context,
	tx *gorm.DB,
	count int,
	updatedAt int64,
) ([]int64, error) {
	if tx == nil || count < 0 || updatedAt <= 0 {
		return nil, domainnotification.ErrStorage
	}
	if count == 0 {
		return []int64{}, nil
	}

	dialect := strings.ToLower(strings.TrimSpace(tx.Dialector.Name()))
	if dialect == "sqlite" {
		// SQLite has no SELECT FOR UPDATE. Taking a write lock on the same
		// singleton row before reading provides the equivalent transaction
		// serialization used by the MySQL materialization path.
		result := tx.WithContext(ctx).
			Model(&notificationSequencePO{}).
			Where("sequence_key = ?", notificationRecipientSequenceKey).
			UpdateColumn("updated_at", gorm.Expr("updated_at + 1"))
		if result.Error != nil {
			return nil, notificationStorageError(
				"lock recipient sequence",
				result.Error,
			)
		}
		if result.RowsAffected != 1 {
			return nil, fmt.Errorf(
				"%w: recipient sequence row missing",
				domainnotification.ErrStorage,
			)
		}
	}

	query := tx.WithContext(ctx)
	if dialect == "mysql" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sequence notificationSequencePO
	if err := query.
		Where("sequence_key = ?", notificationRecipientSequenceKey).
		Take(&sequence).Error; err != nil {
		return nil, notificationStorageError("read recipient sequence", err)
	}
	if sequence.NextValue <= 0 ||
		int64(count) > maxNotificationRecipientSequenceNo-sequence.NextValue+1 {
		return nil, fmt.Errorf(
			"%w: recipient sequence exhausted",
			domainnotification.ErrStorage,
		)
	}

	start := sequence.NextValue
	next := start + int64(count)
	result := tx.WithContext(ctx).
		Model(&notificationSequencePO{}).
		Where("sequence_key = ?", notificationRecipientSequenceKey).
		Updates(map[string]any{
			"next_value": next,
			"updated_at": updatedAt,
		})
	if result.Error != nil {
		return nil, notificationStorageError(
			"advance recipient sequence",
			result.Error,
		)
	}
	if result.RowsAffected != 1 {
		return nil, fmt.Errorf(
			"%w: recipient sequence advance lost",
			domainnotification.ErrStorage,
		)
	}

	sequences := make([]int64, count)
	for index := range sequences {
		sequences[index] = start + int64(index)
	}
	return sequences, nil
}

func messageFromDraft(id int64, draft domainnotification.MessageDraft) *messagePO {
	return &messagePO{
		ID:         id,
		EventID:    draft.EventID,
		Scope:      string(draft.Scope),
		SpaceID:    draft.SpaceID,
		SenderID:   draft.SenderID,
		Category:   string(draft.Category),
		Severity:   string(draft.Severity),
		EventType:  string(draft.EventType),
		Title:      draft.Title,
		Content:    draft.Content,
		TargetType: string(draft.TargetType),
		TargetID:   draft.TargetID,
		CreatedAt:  draft.CreatedAt,
	}
}

func sameMessageIdentity(row *messagePO, draft domainnotification.MessageDraft) bool {
	return row != nil &&
		row.EventID == draft.EventID &&
		row.Scope == string(draft.Scope) &&
		row.SpaceID == draft.SpaceID &&
		row.SenderID == draft.SenderID &&
		row.Category == string(draft.Category) &&
		row.Severity == string(draft.Severity) &&
		row.EventType == string(draft.EventType) &&
		row.Title == draft.Title &&
		row.Content == draft.Content &&
		row.TargetType == string(draft.TargetType) &&
		row.TargetID == draft.TargetID &&
		row.CreatedAt == draft.CreatedAt
}

func normalizeErrorCode(value string) string {
	switch value {
	case domainnotification.ErrorCodeInvalidEvent,
		domainnotification.ErrorCodeUnsafePayload,
		domainnotification.ErrorCodeUnknownEvent,
		domainnotification.ErrorCodeRecipientResolution,
		domainnotification.ErrorCodeRecipientPolicyUnavailable,
		domainnotification.ErrorCodeIdempotencyConflict,
		domainnotification.ErrorCodeLeaseLost,
		domainnotification.ErrorCodeStorage:
		return value
	default:
		return domainnotification.ErrorCodeStorage
	}
}
