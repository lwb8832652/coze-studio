// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
)

const (
	defaultProjectionBatchSize = 500
	maxProjectionBatchSize     = 1000
	defaultMaxAudienceSize     = 100000
	hardMaxAudienceSize        = 1000000

	deliveryBatchStaged    = "staged"
	deliveryBatchProjected = "projected"
)

type NotificationOutbox interface {
	AppendInTransactionWithResult(
		context.Context,
		*gorm.DB,
		domainnotification.Event,
	) (bool, error)
}

type MySQLOption func(*MySQLRepository)

func WithNotificationOutbox(outbox NotificationOutbox) MySQLOption {
	return func(repository *MySQLRepository) {
		repository.outbox = outbox
	}
}

func WithMaxAudienceSize(maxAudienceSize int) MySQLOption {
	return func(repository *MySQLRepository) {
		if maxAudienceSize > 0 && maxAudienceSize <= hardMaxAudienceSize {
			repository.maxAudienceSize = maxAudienceSize
		}
	}
}

type MySQLRepository struct {
	db              *gorm.DB
	idGen           idgen.IDGenerator
	outbox          NotificationOutbox
	maxAudienceSize int
}

func NewMySQLRepository(
	db *gorm.DB,
	idGenerator idgen.IDGenerator,
	options ...MySQLOption,
) *MySQLRepository {
	repository := &MySQLRepository{
		db:              db,
		idGen:           idGenerator,
		maxAudienceSize: defaultMaxAudienceSize,
	}
	repository.outbox = infranotification.NewMySQLRepository(db, idGenerator)
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
}

type announcementPO struct {
	ID                    int64   `gorm:"column:id;primaryKey;index:idx_announcements_replay,priority:4;index:idx_announcements_release_projection,priority:3;index:idx_announcements_created,priority:2"`
	Title                 string  `gorm:"column:title;size:128"`
	Body                  string  `gorm:"column:body;size:512"`
	Severity              string  `gorm:"column:severity;size:16"`
	InternalRoute         string  `gorm:"column:internal_route;size:128"`
	AudienceType          string  `gorm:"column:audience_type;size:16"`
	Status                string  `gorm:"column:status;size:16;index:idx_announcements_replay,priority:2;index:idx_announcements_release_projection,priority:1"`
	ProjectionStatus      string  `gorm:"column:projection_status;size:16;index:idx_announcements_replay,priority:1;index:idx_announcements_release_projection,priority:2"`
	ScheduledAt           int64   `gorm:"column:scheduled_at;index:idx_announcements_replay,priority:3"`
	PublishRequestedAt    int64   `gorm:"column:publish_requested_at"`
	SnapshotAt            int64   `gorm:"column:snapshot_at"`
	PublishedAt           int64   `gorm:"column:published_at"`
	CancelledAt           int64   `gorm:"column:cancelled_at"`
	CreatedBy             int64   `gorm:"column:created_by"`
	UpdatedBy             int64   `gorm:"column:updated_by"`
	PublishActorID        int64   `gorm:"column:publish_actor_id"`
	CreateIdempotencyKey  string  `gorm:"column:create_idempotency_key;size:64;uniqueIndex:uk_announcements_create_idempotency"`
	CreateRequestHash     string  `gorm:"column:create_request_hash;size:64"`
	PublishIdempotencyKey *string `gorm:"column:publish_idempotency_key;size:64;uniqueIndex:uk_announcements_publish_idempotency"`
	PublishRequestHash    string  `gorm:"column:publish_request_hash;size:64"`
	SnapshotCursor        int64   `gorm:"column:snapshot_cursor"`
	ProjectionCursor      int64   `gorm:"column:projection_cursor"`
	SnapshotComplete      bool    `gorm:"column:snapshot_complete"`
	RecipientCount        int64   `gorm:"column:recipient_count"`
	ProjectedCount        int64   `gorm:"column:projected_count"`
	NextBatchNo           int64   `gorm:"column:next_batch_no"`
	LastErrorCode         string  `gorm:"column:last_error_code;size:64"`
	Version               int64   `gorm:"column:version"`
	CreatedAt             int64   `gorm:"column:created_at;index:idx_announcements_created,priority:1"`
	UpdatedAt             int64   `gorm:"column:updated_at"`
}

func (announcementPO) TableName() string {
	return "announcements"
}

type audienceTargetPO struct {
	AnnouncementID int64  `gorm:"column:announcement_id;primaryKey"`
	TargetType     string `gorm:"column:target_type;size:16;primaryKey"`
	TargetID       int64  `gorm:"column:target_id;primaryKey"`
	CreatedAt      int64  `gorm:"column:created_at"`
}

func (audienceTargetPO) TableName() string {
	return "announcement_audience_targets"
}

type recipientSnapshotPO struct {
	AnnouncementID int64 `gorm:"column:announcement_id;primaryKey;index:idx_announcement_snapshot_batch,priority:1"`
	UserID         int64 `gorm:"column:user_id;primaryKey;index:idx_announcement_snapshot_batch,priority:3"`
	BatchNo        int64 `gorm:"column:batch_no;index:idx_announcement_snapshot_batch,priority:2"`
	CreatedAt      int64 `gorm:"column:created_at"`
}

func (recipientSnapshotPO) TableName() string {
	return "announcement_recipient_snapshots"
}

type deliveryBatchPO struct {
	AnnouncementID int64  `gorm:"column:announcement_id;primaryKey;index:idx_announcement_delivery_claim,priority:2"`
	BatchNo        int64  `gorm:"column:batch_no;primaryKey;index:idx_announcement_delivery_claim,priority:3"`
	EventID        string `gorm:"column:event_id;size:128;uniqueIndex:uk_announcement_delivery_event"`
	Status         string `gorm:"column:status;size:16;index:idx_announcement_delivery_claim,priority:1"`
	RecipientCount int64  `gorm:"column:recipient_count"`
	ProjectedCount int64  `gorm:"column:projected_count"`
	AttemptCount   int64  `gorm:"column:attempt_count"`
	LastErrorCode  string `gorm:"column:last_error_code;size:64"`
	CreatedAt      int64  `gorm:"column:created_at"`
	UpdatedAt      int64  `gorm:"column:updated_at"`
	ProjectedAt    int64  `gorm:"column:projected_at"`
}

func (deliveryBatchPO) TableName() string {
	return "announcement_delivery_batches"
}

type auditEventPO struct {
	ID               int64  `gorm:"column:id;primaryKey"`
	AnnouncementID   int64  `gorm:"column:announcement_id"`
	ActorID           int64  `gorm:"column:actor_id"`
	Action            string `gorm:"column:action;size:32"`
	FromStatus        string `gorm:"column:from_status;size:16"`
	ToStatus          string `gorm:"column:to_status;size:16"`
	ProjectionStatus  string `gorm:"column:projection_status;size:16"`
	Result            string `gorm:"column:result;size:16"`
	ErrorCode         string `gorm:"column:error_code;size:64"`
	RecipientCount    int64  `gorm:"column:recipient_count"`
	ProjectedCount    int64  `gorm:"column:projected_count"`
	CreatedAt         int64  `gorm:"column:created_at"`
}

func (auditEventPO) TableName() string {
	return "announcement_audit_events"
}

func (r *MySQLRepository) Create(
	ctx context.Context,
	command domainannouncement.CreateCommand,
) (*domainannouncement.MutationResult, error) {
	if !r.configured() {
		return nil, domainannouncement.ErrStorage
	}
	var output *domainannouncement.MutationResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing announcementPO
		err := tx.Where(
			"create_idempotency_key = ?",
			command.IdempotencyKey,
		).Take(&existing).Error
		if err == nil {
			if existing.CreateRequestHash != command.RequestHash {
				return domainannouncement.ErrIdempotencyConflict
			}
			entity, loadErr := r.entityFromRow(ctx, tx, &existing)
			if loadErr != nil {
				return loadErr
			}
			output = &domainannouncement.MutationResult{
				Announcement: entity,
				Replayed:     true,
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return storageError("find create idempotency key", err)
		}
		if err := r.validateDraftTargets(ctx, tx, command.Draft); err != nil {
			return err
		}
		storedRoute, err := encodeAnnouncementRoute(command.Draft.Route)
		if err != nil {
			return err
		}
		ids, err := r.idGen.GenMultiIDs(ctx, 2)
		if err != nil || len(ids) != 2 || ids[0] <= 0 || ids[1] <= 0 {
			return storageError("allocate announcement IDs", err)
		}
		now := command.Now.UnixMilli()
		row := &announcementPO{
			ID:                   ids[0],
			Title:                command.Draft.Title,
			Body:                 command.Draft.Body,
			Severity:             string(command.Draft.Severity),
			InternalRoute:        storedRoute,
			AudienceType:         string(command.Draft.Audience.Type),
			Status:               string(domainannouncement.StatusDraft),
			ProjectionStatus:     string(domainannouncement.ProjectionIdle),
			CreatedBy:            command.ActorID,
			UpdatedBy:            command.ActorID,
			CreateIdempotencyKey: command.IdempotencyKey,
			CreateRequestHash:    command.RequestHash,
			NextBatchNo:          1,
			Version:              1,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
		if result.Error != nil {
			return storageError("create announcement", result.Error)
		}
		if result.RowsAffected == 0 {
			if err := tx.Where(
				"create_idempotency_key = ?",
				command.IdempotencyKey,
			).Take(&existing).Error; err != nil {
				return storageError("read concurrent announcement", err)
			}
			if existing.CreateRequestHash != command.RequestHash {
				return domainannouncement.ErrIdempotencyConflict
			}
			entity, loadErr := r.entityFromRow(ctx, tx, &existing)
			if loadErr != nil {
				return loadErr
			}
			output = &domainannouncement.MutationResult{
				Announcement: entity,
				Replayed:     true,
			}
			return nil
		}
		if err := r.replaceAudienceTargets(
			ctx,
			tx,
			row.ID,
			command.Draft.Audience,
			now,
		); err != nil {
			return err
		}
		if err := r.createAuditWithID(
			ctx,
			tx,
			ids[1],
			row,
			command.ActorID,
			"created",
			"",
			row.Status,
			"succeeded",
			"",
			now,
		); err != nil {
			return err
		}
		output = &domainannouncement.MutationResult{
			Announcement: toEntity(row, command.Draft.Audience.TargetIDs),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (r *MySQLRepository) Update(
	ctx context.Context,
	command domainannouncement.UpdateCommand,
) (*domainannouncement.Announcement, error) {
	if !r.configured() {
		return nil, domainannouncement.ErrStorage
	}
	var output *domainannouncement.Announcement
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.lockAnnouncement(ctx, tx, command.AnnouncementID)
		if err != nil {
			return err
		}
		if row.Version != command.ExpectedVersion {
			return domainannouncement.ErrVersionConflict
		}
		if (row.Status != string(domainannouncement.StatusDraft) &&
			row.Status != string(domainannouncement.StatusScheduled)) ||
			row.ProjectionStatus != string(domainannouncement.ProjectionIdle) {
			return domainannouncement.ErrStateConflict
		}
		if err := r.validateDraftTargets(ctx, tx, command.Draft); err != nil {
			return err
		}
		storedRoute, err := encodeAnnouncementRoute(command.Draft.Route)
		if err != nil {
			return err
		}
		now := command.Now.UnixMilli()
		nextVersion := row.Version + 1
		result := tx.Model(&announcementPO{}).
			Where("id = ? AND version = ?", row.ID, row.Version).
			Updates(map[string]any{
				"title":          command.Draft.Title,
				"body":           command.Draft.Body,
				"severity":       command.Draft.Severity,
				"internal_route": storedRoute,
				"audience_type":  command.Draft.Audience.Type,
				"updated_by":     command.ActorID,
				"updated_at":     now,
				"version":        nextVersion,
			})
		if result.Error != nil {
			return storageError("update announcement", result.Error)
		}
		if result.RowsAffected != 1 {
			return domainannouncement.ErrVersionConflict
		}
		if err := r.replaceAudienceTargets(
			ctx,
			tx,
			row.ID,
			command.Draft.Audience,
			now,
		); err != nil {
			return err
		}
		row.Title = command.Draft.Title
		row.Body = command.Draft.Body
		row.Severity = string(command.Draft.Severity)
		row.InternalRoute = storedRoute
		row.AudienceType = string(command.Draft.Audience.Type)
		row.UpdatedBy = command.ActorID
		row.UpdatedAt = now
		row.Version = nextVersion
		if err := r.appendAudit(
			ctx,
			tx,
			row,
			command.ActorID,
			"updated",
			row.Status,
			row.Status,
			"succeeded",
			"",
			now,
		); err != nil {
			return err
		}
		output = toEntity(row, command.Draft.Audience.TargetIDs)
		return nil
	})
	return output, err
}

func (r *MySQLRepository) Schedule(
	ctx context.Context,
	command domainannouncement.ScheduleCommand,
) (*domainannouncement.Announcement, error) {
	if !r.configured() {
		return nil, domainannouncement.ErrStorage
	}
	var output *domainannouncement.Announcement
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.lockAnnouncement(ctx, tx, command.AnnouncementID)
		if err != nil {
			return err
		}
		if row.Version != command.ExpectedVersion {
			return domainannouncement.ErrVersionConflict
		}
		if (row.Status != string(domainannouncement.StatusDraft) &&
			row.Status != string(domainannouncement.StatusScheduled)) ||
			row.ProjectionStatus != string(domainannouncement.ProjectionIdle) {
			return domainannouncement.ErrStateConflict
		}
		fromStatus := row.Status
		now := command.Now.UnixMilli()
		row.Status = string(domainannouncement.StatusScheduled)
		row.ScheduledAt = command.ScheduledAt.UnixMilli()
		row.UpdatedBy = command.ActorID
		row.UpdatedAt = now
		row.Version++
		if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
			"status":       row.Status,
			"scheduled_at": row.ScheduledAt,
			"updated_by":   row.UpdatedBy,
			"updated_at":   row.UpdatedAt,
			"version":      row.Version,
		}); err != nil {
			return err
		}
		if err := r.appendAudit(
			ctx,
			tx,
			row,
			command.ActorID,
			"scheduled",
			fromStatus,
			row.Status,
			"succeeded",
			"",
			now,
		); err != nil {
			return err
		}
		entity, err := r.entityFromRow(ctx, tx, row)
		if err != nil {
			return err
		}
		output = entity
		return nil
	})
	return output, err
}

func (r *MySQLRepository) RequestPublish(
	ctx context.Context,
	command domainannouncement.PublishCommand,
) (*domainannouncement.MutationResult, error) {
	if !r.configured() {
		return nil, domainannouncement.ErrStorage
	}
	var output *domainannouncement.MutationResult
	err := r.authoritativeTransaction(ctx, func(tx *gorm.DB) error {
		row, err := r.lockAnnouncement(ctx, tx, command.AnnouncementID)
		if err != nil {
			return err
		}
		if row.PublishIdempotencyKey != nil {
			if *row.PublishIdempotencyKey != command.IdempotencyKey ||
				row.PublishRequestHash != command.RequestHash {
				return domainannouncement.ErrIdempotencyConflict
			}
			entity, loadErr := r.entityFromRow(ctx, tx, row)
			if loadErr != nil {
				return loadErr
			}
			output = &domainannouncement.MutationResult{
				Announcement: entity,
				Replayed:     true,
			}
			return nil
		}
		if row.Version != command.ExpectedVersion {
			return domainannouncement.ErrVersionConflict
		}
		if (row.Status != string(domainannouncement.StatusDraft) &&
			row.Status != string(domainannouncement.StatusScheduled)) ||
			row.ProjectionStatus != string(domainannouncement.ProjectionIdle) {
			return domainannouncement.ErrStateConflict
		}
		var conflicting int64
		err = tx.Model(&announcementPO{}).
			Where(
				"publish_idempotency_key = ? AND id <> ?",
				command.IdempotencyKey,
				row.ID,
			).
			Count(&conflicting).Error
		if err != nil {
			return storageError("check publish idempotency key", err)
		}
		if conflicting != 0 {
			return domainannouncement.ErrIdempotencyConflict
		}
		fromStatus := row.Status
		publishKey := command.IdempotencyKey
		row.ProjectionStatus = string(domainannouncement.ProjectionSnapshotting)
		row.ScheduledAt = 0
		row.PublishActorID = command.ActorID
		row.PublishIdempotencyKey = &publishKey
		row.PublishRequestHash = command.RequestHash
		row.SnapshotCursor = 0
		row.ProjectionCursor = 0
		row.SnapshotComplete = false
		row.RecipientCount = 0
		row.ProjectedCount = 0
		row.NextBatchNo = 1
		row.LastErrorCode = ""
		row.UpdatedBy = command.ActorID
		if err := r.materializeAudienceSnapshot(ctx, tx, row); err != nil {
			return err
		}
		now := row.SnapshotAt
		row.PublishRequestedAt = now
		row.UpdatedAt = now
		row.Version++
		if err := r.saveLifecycleRow(ctx, tx, row, publishLifecycleUpdates(row)); err != nil {
			return err
		}
		if err := r.appendAudit(
			ctx,
			tx,
			row,
			command.ActorID,
			"publish_requested",
			fromStatus,
			row.Status,
			"succeeded",
			"",
			now,
		); err != nil {
			return err
		}
		entity, err := r.entityFromRow(ctx, tx, row)
		if err != nil {
			return err
		}
		output = &domainannouncement.MutationResult{Announcement: entity}
		return nil
	})
	return output, err
}

func (r *MySQLRepository) Cancel(
	ctx context.Context,
	command domainannouncement.CancelCommand,
) (*domainannouncement.Announcement, error) {
	if !r.configured() {
		return nil, domainannouncement.ErrStorage
	}
	var output *domainannouncement.Announcement
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.lockAnnouncement(ctx, tx, command.AnnouncementID)
		if err != nil {
			return err
		}
		if row.Version != command.ExpectedVersion {
			return domainannouncement.ErrVersionConflict
		}
		if (row.Status != string(domainannouncement.StatusDraft) &&
			row.Status != string(domainannouncement.StatusScheduled)) ||
			row.ProjectionStatus != string(domainannouncement.ProjectionIdle) {
			return domainannouncement.ErrStateConflict
		}
		fromStatus := row.Status
		now := command.Now.UnixMilli()
		row.Status = string(domainannouncement.StatusCancelled)
		row.CancelledAt = now
		row.UpdatedBy = command.ActorID
		row.UpdatedAt = now
		row.Version++
		if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
			"status":       row.Status,
			"cancelled_at": row.CancelledAt,
			"updated_by":   row.UpdatedBy,
			"updated_at":   row.UpdatedAt,
			"version":      row.Version,
		}); err != nil {
			return err
		}
		if err := r.appendAudit(
			ctx,
			tx,
			row,
			command.ActorID,
			"cancelled",
			fromStatus,
			row.Status,
			"succeeded",
			"",
			now,
		); err != nil {
			return err
		}
		entity, err := r.entityFromRow(ctx, tx, row)
		if err != nil {
			return err
		}
		output = entity
		return nil
	})
	return output, err
}

func (r *MySQLRepository) Get(
	ctx context.Context,
	announcementID int64,
) (*domainannouncement.Announcement, error) {
	if !r.configured() || announcementID <= 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	var row announcementPO
	if err := r.db.WithContext(ctx).
		Where("id = ?", announcementID).
		Take(&row).Error; err != nil {
		return nil, translateReadError("get announcement", err)
	}
	return r.entityFromRow(ctx, r.db.WithContext(ctx), &row)
}

func (r *MySQLRepository) List(
	ctx context.Context,
	filter domainannouncement.ListFilter,
) ([]*domainannouncement.Announcement, int64, error) {
	if !r.configured() {
		return nil, 0, domainannouncement.ErrStorage
	}
	query := r.db.WithContext(ctx).Model(&announcementPO{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, storageError("count announcements", err)
	}
	var rows []*announcementPO
	if err := query.Order("created_at DESC, id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Find(&rows).Error; err != nil {
		return nil, 0, storageError("list announcements", err)
	}
	targets, err := r.targetsByAnnouncementIDs(ctx, r.db, announcementIDs(rows))
	if err != nil {
		return nil, 0, err
	}
	items := make([]*domainannouncement.Announcement, 0, len(rows))
	for _, row := range rows {
		items = append(items, toEntity(row, targets[row.ID]))
	}
	return items, total, nil
}

func (r *MySQLRepository) ListAuditEvents(
	ctx context.Context,
	announcementID int64,
	offset int,
	limit int,
) ([]*domainannouncement.AuditEvent, int64, error) {
	if !r.configured() || announcementID <= 0 {
		return nil, 0, domainannouncement.ErrInvalidInput
	}
	query := r.db.WithContext(ctx).
		Model(&auditEventPO{}).
		Where("announcement_id = ?", announcementID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, storageError("count announcement audit events", err)
	}
	var rows []*auditEventPO
	if err := query.Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, storageError("list announcement audit events", err)
	}
	items := make([]*domainannouncement.AuditEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, &domainannouncement.AuditEvent{
			ID:              row.ID,
			AnnouncementID:  row.AnnouncementID,
			ActorID:          row.ActorID,
			Action:           row.Action,
			FromStatus:       domainannouncement.Status(row.FromStatus),
			ToStatus:         domainannouncement.Status(row.ToStatus),
			ProjectionStatus: domainannouncement.ProjectionStatus(row.ProjectionStatus),
			Result:           row.Result,
			ErrorCode:        row.ErrorCode,
			RecipientCount:   row.RecipientCount,
			ProjectedCount:   row.ProjectedCount,
			CreatedAt:        row.CreatedAt,
		})
	}
	return items, total, nil
}

func (r *MySQLRepository) ListReplayCandidates(
	ctx context.Context,
	now time.Time,
	afterID int64,
	limit int,
) ([]int64, error) {
	if !r.configured() || now.IsZero() || afterID < 0 || limit <= 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	databaseNow, err := databaseUTCUnixMilli(ctx, r.db)
	if err != nil {
		return nil, err
	}
	var ids []int64
	err = r.db.WithContext(ctx).
		Model(&announcementPO{}).
		Where("id > ?", afterID).
		Where(
			"(projection_status IN ? OR "+
				"(status = ? AND projection_status = ? AND scheduled_at > 0 AND scheduled_at <= ?))",
			[]string{
				string(domainannouncement.ProjectionSnapshotting),
				string(domainannouncement.ProjectionProjecting),
				string(domainannouncement.ProjectionFailed),
			},
			domainannouncement.StatusScheduled,
			domainannouncement.ProjectionIdle,
			databaseNow,
		).
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, storageError("list announcement replay candidates", err)
	}
	return ids, nil
}

func (r *MySQLRepository) AdvancePublication(
	ctx context.Context,
	announcementID int64,
	actorID int64,
	_ time.Time,
	batchSize int,
) (*domainannouncement.AdvanceResult, error) {
	if !r.configured() || announcementID <= 0 || actorID < 0 {
		return nil, domainannouncement.ErrInvalidInput
	}
	if batchSize <= 0 {
		batchSize = defaultProjectionBatchSize
	}
	if batchSize > maxProjectionBatchSize {
		batchSize = maxProjectionBatchSize
	}
	var output *domainannouncement.AdvanceResult
	err := r.authoritativeTransaction(ctx, func(tx *gorm.DB) error {
		row, err := r.lockAnnouncement(ctx, tx, announcementID)
		if err != nil {
			return err
		}
		databaseNow, err := databaseUTCUnixMilli(ctx, tx)
		if err != nil {
			return err
		}
		now := time.UnixMilli(databaseNow).UTC()
		progressed := false
		if row.Status == string(domainannouncement.StatusScheduled) &&
			row.ProjectionStatus == string(domainannouncement.ProjectionIdle) {
			if row.ScheduledAt <= 0 || row.ScheduledAt > databaseNow {
				return domainannouncement.ErrStateConflict
			}
			if err := r.activateScheduled(ctx, tx, row); err != nil {
				return err
			}
			now = time.UnixMilli(row.SnapshotAt).UTC()
			progressed = true
		}
		if row.ProjectionStatus == string(domainannouncement.ProjectionFailed) {
			if row.Status == string(domainannouncement.StatusPublished) {
				row.ProjectionStatus = string(domainannouncement.ProjectionProjecting)
			} else {
				row.ProjectionStatus = string(domainannouncement.ProjectionSnapshotting)
			}
			row.LastErrorCode = ""
			row.UpdatedAt = now.UnixMilli()
			if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
				"projection_status": row.ProjectionStatus,
				"last_error_code":  "",
				"updated_at":       row.UpdatedAt,
			}); err != nil {
				return err
			}
			if err := r.appendAudit(
				ctx,
				tx,
				row,
				actorID,
				"replay_started",
				row.Status,
				row.Status,
				"succeeded",
				"",
				now.UnixMilli(),
			); err != nil {
				return err
			}
			progressed = true
		}
		switch domainannouncement.ProjectionStatus(row.ProjectionStatus) {
		case domainannouncement.ProjectionSnapshotting:
			stepProgressed, err := r.stageSnapshotBatch(
				ctx,
				tx,
				row,
				now,
				batchSize,
			)
			if err != nil {
				return err
			}
			progressed = progressed || stepProgressed
		case domainannouncement.ProjectionProjecting:
			stepProgressed, err := r.projectRecipientBatch(
				ctx,
				tx,
				row,
				now,
			)
			if err != nil {
				return err
			}
			progressed = progressed || stepProgressed
		case domainannouncement.ProjectionCompleted:
		case domainannouncement.ProjectionIdle:
			return domainannouncement.ErrStateConflict
		default:
			return domainannouncement.ErrStateConflict
		}
		entity, err := r.entityFromRow(ctx, tx, row)
		if err != nil {
			return err
		}
		output = &domainannouncement.AdvanceResult{
			Announcement: entity,
			Done: row.ProjectionStatus ==
				string(domainannouncement.ProjectionCompleted),
			Progressed: progressed,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (r *MySQLRepository) MarkProjectionFailed(
	ctx context.Context,
	announcementID int64,
	actorID int64,
	errorCode string,
	_ time.Time,
) (*domainannouncement.Announcement, error) {
	if !r.configured() ||
		announcementID <= 0 ||
		actorID < 0 ||
		strings.TrimSpace(errorCode) == "" {
		return nil, domainannouncement.ErrInvalidInput
	}
	var output *domainannouncement.Announcement
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.lockAnnouncement(ctx, tx, announcementID)
		if err != nil {
			return err
		}
		databaseNow, err := databaseUTCUnixMilli(ctx, tx)
		if err != nil {
			return err
		}
		switch domainannouncement.ProjectionStatus(row.ProjectionStatus) {
		case domainannouncement.ProjectionSnapshotting,
			domainannouncement.ProjectionProjecting,
			domainannouncement.ProjectionFailed:
		default:
			entity, loadErr := r.entityFromRow(ctx, tx, row)
			if loadErr != nil {
				return loadErr
			}
			output = entity
			return nil
		}
		row.ProjectionStatus = string(domainannouncement.ProjectionFailed)
		row.LastErrorCode = errorCode
		row.UpdatedAt = databaseNow
		if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
			"projection_status": row.ProjectionStatus,
			"last_error_code":  row.LastErrorCode,
			"updated_at":       row.UpdatedAt,
		}); err != nil {
			return err
		}
		if err := r.appendAudit(
			ctx,
			tx,
			row,
			actorID,
			"projection_failed",
			row.Status,
			row.Status,
			"failed",
			errorCode,
			databaseNow,
		); err != nil {
			return err
		}
		var batch deliveryBatchPO
		batchQuery := tx.WithContext(ctx).
			Where(
				"announcement_id = ? AND status = ?",
				row.ID,
				deliveryBatchStaged,
			).
			Order("batch_no ASC")
		if strings.EqualFold(tx.Dialector.Name(), "mysql") {
			batchQuery = batchQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := batchQuery.Take(&batch).Error; err == nil {
			result := tx.WithContext(ctx).
				Model(&deliveryBatchPO{}).
				Where(
					"announcement_id = ? AND batch_no = ? AND status = ?",
					batch.AnnouncementID,
					batch.BatchNo,
					deliveryBatchStaged,
				).
				Updates(map[string]any{
					"attempt_count":  batch.AttemptCount + 1,
					"last_error_code": errorCode,
					"updated_at":     databaseNow,
				})
			if result.Error != nil {
				return storageError("mark announcement delivery batch failed", result.Error)
			}
			if result.RowsAffected != 1 {
				return domainannouncement.ErrStateConflict
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return storageError("lock failed announcement delivery batch", err)
		}
		entity, err := r.entityFromRow(ctx, tx, row)
		if err != nil {
			return err
		}
		output = entity
		return nil
	})
	return output, err
}

func (r *MySQLRepository) activateScheduled(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
) error {
	fromStatus := row.Status
	key := fmt.Sprintf("scheduled.%d.%d", row.ID, row.Version)
	hashInput := sha256.Sum256([]byte(key))
	row.PublishIdempotencyKey = &key
	row.PublishRequestHash = hex.EncodeToString(hashInput[:])
	row.PublishActorID = row.UpdatedBy
	row.ProjectionStatus = string(domainannouncement.ProjectionSnapshotting)
	row.SnapshotCursor = 0
	row.ProjectionCursor = 0
	row.SnapshotComplete = false
	row.RecipientCount = 0
	row.ProjectedCount = 0
	row.NextBatchNo = 1
	row.LastErrorCode = ""
	if err := r.materializeAudienceSnapshot(ctx, tx, row); err != nil {
		return err
	}
	row.PublishRequestedAt = row.SnapshotAt
	row.UpdatedAt = row.SnapshotAt
	row.Version++
	if err := r.saveLifecycleRow(ctx, tx, row, publishLifecycleUpdates(row)); err != nil {
		return err
	}
	return r.appendAudit(
		ctx,
		tx,
		row,
		0,
		"schedule_replayed",
		fromStatus,
		row.Status,
		"succeeded",
		"",
		row.SnapshotAt,
	)
}

func (r *MySQLRepository) stageSnapshotBatch(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	now time.Time,
	batchSize int,
) (bool, error) {
	if !row.SnapshotComplete {
		return false, domainannouncement.ErrStateConflict
	}
	var userIDs []int64
	if err := tx.WithContext(ctx).
		Model(&recipientSnapshotPO{}).
		Where(
			"announcement_id = ? AND user_id > ?",
			row.ID,
			row.SnapshotCursor,
		).
		Order("user_id ASC").
		Limit(batchSize + 1).
		Pluck("user_id", &userIDs).Error; err != nil {
		return false, storageError("read announcement recipient snapshot", err)
	}
	hasMore := len(userIDs) > batchSize
	if hasMore {
		userIDs = userIDs[:batchSize]
	}
	if len(userIDs) > 0 {
		batchNo := row.NextBatchNo
		batch := &deliveryBatchPO{
			AnnouncementID: row.ID,
			BatchNo:        batchNo,
			EventID: fmt.Sprintf(
				"announcement.%d.batch.%d",
				row.ID,
				batchNo,
			),
			Status:         deliveryBatchStaged,
			RecipientCount: int64(len(userIDs)),
			CreatedAt:      now.UnixMilli(),
			UpdatedAt:      now.UnixMilli(),
		}
		result := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(batch)
		if result.Error != nil {
			return false, storageError("stage announcement delivery batch", result.Error)
		}
		if result.RowsAffected != 1 {
			return false, domainannouncement.ErrStateConflict
		}
		snapshotResult := tx.WithContext(ctx).
			Model(&recipientSnapshotPO{}).
			Where(
				"announcement_id = ? AND user_id IN ? AND batch_no = 0",
				row.ID,
				userIDs,
			).
			Update("batch_no", batchNo)
		if snapshotResult.Error != nil {
			return false, storageError(
				"assign announcement recipient snapshot batch",
				snapshotResult.Error,
			)
		}
		if snapshotResult.RowsAffected != int64(len(userIDs)) {
			return false, domainannouncement.ErrStateConflict
		}
		row.SnapshotCursor = userIDs[len(userIDs)-1]
		row.NextBatchNo++
	}
	updates := map[string]any{
		"snapshot_cursor": row.SnapshotCursor,
		"next_batch_no":  row.NextBatchNo,
		"updated_at":      now.UnixMilli(),
		"last_error_code": "",
	}
	if !hasMore {
		if err := r.releaseAnnouncement(ctx, tx, row, now); err != nil {
			return false, err
		}
		return true, nil
	}
	row.UpdatedAt = now.UnixMilli()
	if err := r.saveLifecycleRow(ctx, tx, row, updates); err != nil {
		return false, err
	}
	return true, nil
}

func (r *MySQLRepository) projectRecipientBatch(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	now time.Time,
) (bool, error) {
	if row.Status != string(domainannouncement.StatusPublished) ||
		row.PublishedAt <= 0 ||
		!row.SnapshotComplete {
		return false, domainannouncement.ErrStateConflict
	}
	var batch deliveryBatchPO
	batchQuery := tx.WithContext(ctx).
		Where(
			"announcement_id = ? AND status = ?",
			row.ID,
			deliveryBatchStaged,
		).
		Order("batch_no ASC")
	if strings.EqualFold(tx.Dialector.Name(), "mysql") {
		batchQuery = batchQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := batchQuery.Take(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := r.completeProjection(ctx, tx, row, now); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, storageError("claim announcement delivery batch", err)
	}
	var snapshotIDs []int64
	if err := tx.WithContext(ctx).
		Model(&recipientSnapshotPO{}).
		Where(
			"announcement_id = ? AND batch_no = ?",
			row.ID,
			batch.BatchNo,
		).
		Order("user_id ASC").
		Pluck("user_id", &snapshotIDs).Error; err != nil {
		return false, storageError("read staged announcement recipients", err)
	}
	if int64(len(snapshotIDs)) != batch.RecipientCount {
		return false, domainannouncement.ErrStateConflict
	}
	deliverableUserIDs, err := r.revalidateDeliveryRecipients(
		ctx,
		tx,
		row,
		snapshotIDs,
	)
	if err != nil {
		return false, err
	}
	if len(deliverableUserIDs) > 0 {
		event := announcementNotificationEvent(row, &batch, deliverableUserIDs)
		if _, err := r.outbox.AppendInTransactionWithResult(
			ctx,
			tx,
			event,
		); err != nil {
			return false, fmt.Errorf(
				"%w: append announcement outbox: %w",
				domainannouncement.ErrStorage,
				err,
			)
		}
		row.ProjectedCount += int64(len(deliverableUserIDs))
	}
	row.UpdatedAt = now.UnixMilli()
	batchResult := tx.WithContext(ctx).
		Model(&deliveryBatchPO{}).
		Where(
			"announcement_id = ? AND batch_no = ? AND status = ?",
			row.ID,
			batch.BatchNo,
			deliveryBatchStaged,
		).
		Updates(map[string]any{
			"status":          deliveryBatchProjected,
			"projected_count": len(deliverableUserIDs),
			"last_error_code": "",
			"projected_at":    row.UpdatedAt,
			"updated_at":      row.UpdatedAt,
		})
	if batchResult.Error != nil {
		return false, storageError("complete announcement delivery batch", batchResult.Error)
	}
	if batchResult.RowsAffected != 1 {
		return false, domainannouncement.ErrStateConflict
	}
	var stagedCount int64
	if err := tx.WithContext(ctx).
		Model(&deliveryBatchPO{}).
		Where(
			"announcement_id = ? AND status = ?",
			row.ID,
			deliveryBatchStaged,
		).
		Count(&stagedCount).Error; err != nil {
		return false, storageError("count staged announcement delivery batches", err)
	}
	if stagedCount == 0 {
		if err := r.completeProjection(ctx, tx, row, now); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
		"projected_count": row.ProjectedCount,
		"last_error_code":   "",
		"updated_at":        row.UpdatedAt,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (r *MySQLRepository) releaseAnnouncement(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	now time.Time,
) error {
	fromStatus := row.Status
	row.Status = string(domainannouncement.StatusPublished)
	row.SnapshotComplete = true
	if row.RecipientCount == 0 {
		row.ProjectionStatus = string(domainannouncement.ProjectionCompleted)
	} else {
		row.ProjectionStatus = string(domainannouncement.ProjectionProjecting)
	}
	row.PublishedAt = now.UnixMilli()
	row.LastErrorCode = ""
	row.UpdatedAt = now.UnixMilli()
	row.Version++
	if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
		"status":            row.Status,
		"projection_status": row.ProjectionStatus,
		"snapshot_cursor":   row.SnapshotCursor,
		"snapshot_complete": true,
		"recipient_count":   row.RecipientCount,
		"projected_count":   row.ProjectedCount,
		"next_batch_no":     row.NextBatchNo,
		"published_at":      row.PublishedAt,
		"last_error_code":   "",
		"updated_at":        row.UpdatedAt,
		"version":           row.Version,
	}); err != nil {
		return err
	}
	if err := r.appendAudit(
		ctx,
		tx,
		row,
		row.PublishActorID,
		"published",
		fromStatus,
		row.Status,
		"succeeded",
		"",
		now.UnixMilli(),
	); err != nil {
		return err
	}
	if row.ProjectionStatus == string(domainannouncement.ProjectionCompleted) {
		return r.appendAudit(
			ctx,
			tx,
			row,
			row.PublishActorID,
			"projection_completed",
			row.Status,
			row.Status,
			"succeeded",
			"",
			now.UnixMilli(),
		)
	}
	return nil
}

func (r *MySQLRepository) completeProjection(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	now time.Time,
) error {
	if row.Status != string(domainannouncement.StatusPublished) {
		return domainannouncement.ErrStateConflict
	}
	row.ProjectionStatus = string(domainannouncement.ProjectionCompleted)
	row.LastErrorCode = ""
	row.UpdatedAt = now.UnixMilli()
	row.Version++
	if err := r.saveLifecycleRow(ctx, tx, row, map[string]any{
		"projection_status": row.ProjectionStatus,
		"projected_count":   row.ProjectedCount,
		"last_error_code":   "",
		"updated_at":        row.UpdatedAt,
		"version":           row.Version,
	}); err != nil {
		return err
	}
	return r.appendAudit(
		ctx,
		tx,
		row,
		row.PublishActorID,
		"projection_completed",
		row.Status,
		row.Status,
		"succeeded",
		"",
		now.UnixMilli(),
	)
}

func (r *MySQLRepository) materializeAudienceSnapshot(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
) error {
	if row == nil || row.SnapshotComplete || row.SnapshotAt != 0 {
		return domainannouncement.ErrStateConflict
	}
	databaseNowExpression, err := databaseUTCUnixMilliExpression(tx)
	if err != nil {
		return err
	}
	maxAudienceSize := r.maxAudienceSize
	if maxAudienceSize <= 0 || maxAudienceSize > hardMaxAudienceSize {
		maxAudienceSize = defaultMaxAudienceSize
	}
	limit := maxAudienceSize + 1
	var statement string
	var arguments []any
	switch domainannouncement.AudienceType(row.AudienceType) {
	case domainannouncement.AudienceAll:
		statement = fmt.Sprintf(
			"INSERT INTO announcement_recipient_snapshots "+
				"(announcement_id, user_id, batch_no, created_at) "+
				"SELECT ?, u.id, 0, %s FROM `user` AS u "+
				"WHERE u.deleted_at IS NULL "+
				"ORDER BY u.id ASC LIMIT %d",
			databaseNowExpression,
			limit,
		)
		arguments = []any{row.ID}
	case domainannouncement.AudienceUsers:
		statement = fmt.Sprintf(
			"INSERT INTO announcement_recipient_snapshots "+
				"(announcement_id, user_id, batch_no, created_at) "+
				"SELECT ?, u.id, 0, %s FROM `user` AS u "+
				"JOIN announcement_audience_targets AS aat "+
				"ON aat.announcement_id = ? "+
				"AND aat.target_type = ? "+
				"AND aat.target_id = u.id "+
				"WHERE u.deleted_at IS NULL "+
				"ORDER BY u.id ASC LIMIT %d",
			databaseNowExpression,
			limit,
		)
		arguments = []any{
			row.ID,
			row.ID,
			domainannouncement.AudienceUsers,
		}
	case domainannouncement.AudienceWorkspaces:
		statement = fmt.Sprintf(
			"INSERT INTO announcement_recipient_snapshots "+
				"(announcement_id, user_id, batch_no, created_at) "+
				"SELECT DISTINCT ?, u.id, 0, %s FROM `user` AS u "+
				"JOIN space_user AS su ON su.user_id = u.id "+
				"JOIN announcement_audience_targets AS aat "+
				"ON aat.announcement_id = ? "+
				"AND aat.target_type = ? "+
				"AND aat.target_id = su.space_id "+
				"JOIN `space` AS s "+
				"ON s.id = su.space_id AND s.deleted_at IS NULL "+
				"WHERE u.deleted_at IS NULL "+
				"ORDER BY u.id ASC LIMIT %d",
			databaseNowExpression,
			limit,
		)
		arguments = []any{
			row.ID,
			row.ID,
			domainannouncement.AudienceWorkspaces,
		}
	default:
		return domainannouncement.ErrStateConflict
	}
	result := tx.WithContext(ctx).Exec(statement, arguments...)
	if result.Error != nil {
		return storageError("materialize announcement audience", result.Error)
	}
	if result.RowsAffected > int64(maxAudienceSize) {
		return domainannouncement.ErrAudienceTooLarge
	}
	snapshotAt, err := databaseUTCUnixMilli(ctx, tx)
	if err != nil {
		return err
	}
	row.SnapshotAt = snapshotAt
	row.SnapshotComplete = true
	row.RecipientCount = result.RowsAffected
	return nil
}

func announcementNotificationEvent(
	row *announcementPO,
	batch *deliveryBatchPO,
	recipientIDs []int64,
) domainnotification.Event {
	route := decodeAnnouncementRoute(row.InternalRoute)
	return domainnotification.Event{
		EventID:          batch.EventID,
		EventType:        domainnotification.EventSystemAnnouncement,
		AggregateType:    "announcement",
		AggregateID:      strconv.FormatInt(row.ID, 10),
		AggregateVersion: batch.BatchNo,
		OccurredAt:       time.UnixMilli(row.PublishedAt),
		ActorID:          row.PublishActorID,
		RecipientPolicy:  domainnotification.RecipientExplicitInternalUsers,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			AnnouncementTitle:    row.Title,
			AnnouncementBody:     row.Body,
			AnnouncementSeverity: domainnotification.Severity(row.Severity),
			AnnouncementRoute:    &route,
			ExplicitRecipientIDs: append([]int64(nil), recipientIDs...),
		},
	}
}

func (r *MySQLRepository) revalidateDeliveryRecipients(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	snapshotIDs []int64,
) ([]int64, error) {
	if len(snapshotIDs) == 0 {
		return nil, nil
	}
	query := tx.WithContext(ctx).
		Table("user AS u").
		Select("DISTINCT u.id").
		Where("u.id IN ? AND u.deleted_at IS NULL", snapshotIDs)
	if domainannouncement.AudienceType(row.AudienceType) ==
		domainannouncement.AudienceWorkspaces {
		query = query.
			Joins("JOIN space_user AS su ON su.user_id = u.id").
			Joins(
				"JOIN announcement_audience_targets AS aat "+
					"ON aat.announcement_id = ? "+
					"AND aat.target_type = ? "+
					"AND aat.target_id = su.space_id",
				row.ID,
				domainannouncement.AudienceWorkspaces,
			).
			Joins(
				"JOIN space AS s ON s.id = su.space_id AND s.deleted_at IS NULL",
			)
	}
	var ids []int64
	if err := query.Order("u.id ASC").Pluck("u.id", &ids).Error; err != nil {
		return nil, storageError("revalidate announcement delivery recipients", err)
	}
	return ids, nil
}

func databaseUTCUnixMilli(ctx context.Context, db *gorm.DB) (int64, error) {
	var value int64
	expression, err := databaseUTCUnixMilliExpression(db)
	if err != nil {
		return 0, err
	}
	result := db.WithContext(ctx).Raw("SELECT " + expression).Scan(&value)
	if result.Error != nil || value <= 0 {
		return 0, storageError("read database UTC time", result.Error)
	}
	return value, nil
}

func databaseUTCUnixMilliExpression(db *gorm.DB) (string, error) {
	switch strings.ToLower(db.Dialector.Name()) {
	case "mysql":
		// TIMESTAMPDIFF compares DATETIME values directly, so UTC_TIMESTAMP is
		// not reinterpreted through the connection's session time zone.
		return "TIMESTAMPDIFF(" +
			"MICROSECOND, '1970-01-01 00:00:00', UTC_TIMESTAMP(3)" +
			") DIV 1000", nil
	case "sqlite":
		// julianday('now') is defined in UTC and is independent of local time.
		return "CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)", nil
	default:
		return "", storageError(
			"read database UTC time",
			fmt.Errorf("unsupported database dialect %q", db.Dialector.Name()),
		)
	}
}

func (r *MySQLRepository) authoritativeTransaction(
	ctx context.Context,
	operation func(*gorm.DB) error,
) error {
	database := r.db.WithContext(ctx)
	if strings.EqualFold(database.Dialector.Name(), "mysql") {
		return database.Transaction(
			operation,
			&sql.TxOptions{Isolation: sql.LevelReadCommitted},
		)
	}
	return database.Transaction(operation)
}

func (r *MySQLRepository) validateDraftTargets(
	ctx context.Context,
	tx *gorm.DB,
	draft domainannouncement.Draft,
) error {
	if err := r.validateAudienceTargets(ctx, tx, draft.Audience); err != nil {
		return err
	}
	if draft.Route.Type != domainnotification.AnnouncementRouteWorkspaceHome {
		return nil
	}
	spaceID := draft.Route.SpaceID
	var count int64
	if err := tx.WithContext(ctx).
		Table("space").
		Where("id = ? AND deleted_at IS NULL", spaceID).
		Count(&count).Error; err != nil {
		return storageError("validate announcement route workspace", err)
	}
	if count != 1 {
		return domainannouncement.ErrAudienceTargetNotFound
	}
	if draft.Audience.Type != domainannouncement.AudienceWorkspaces {
		return nil
	}
	for _, targetID := range draft.Audience.TargetIDs {
		if targetID == spaceID {
			return nil
		}
	}
	return domainannouncement.ErrInvalidInput
}

func (r *MySQLRepository) validateAudienceTargets(
	ctx context.Context,
	tx *gorm.DB,
	audience domainannouncement.Audience,
) error {
	if audience.Type == domainannouncement.AudienceAll {
		return nil
	}
	var count int64
	var err error
	switch audience.Type {
	case domainannouncement.AudienceUsers:
		err = tx.WithContext(ctx).
			Table("user").
			Where("id IN ? AND deleted_at IS NULL", audience.TargetIDs).
			Count(&count).Error
	case domainannouncement.AudienceWorkspaces:
		err = tx.WithContext(ctx).
			Table("space").
			Where("id IN ? AND deleted_at IS NULL", audience.TargetIDs).
			Count(&count).Error
	default:
		return domainannouncement.ErrInvalidInput
	}
	if err != nil {
		return storageError("validate announcement audience targets", err)
	}
	if count != int64(len(audience.TargetIDs)) {
		return domainannouncement.ErrAudienceTargetNotFound
	}
	return nil
}

func (r *MySQLRepository) replaceAudienceTargets(
	ctx context.Context,
	tx *gorm.DB,
	announcementID int64,
	audience domainannouncement.Audience,
	now int64,
) error {
	if err := tx.WithContext(ctx).
		Where("announcement_id = ?", announcementID).
		Delete(&audienceTargetPO{}).Error; err != nil {
		return storageError("delete announcement audience targets", err)
	}
	if audience.Type == domainannouncement.AudienceAll {
		return nil
	}
	rows := make([]audienceTargetPO, 0, len(audience.TargetIDs))
	for _, targetID := range audience.TargetIDs {
		rows = append(rows, audienceTargetPO{
			AnnouncementID: announcementID,
			TargetType:     string(audience.Type),
			TargetID:       targetID,
			CreatedAt:      now,
		})
	}
	if err := tx.WithContext(ctx).
		CreateInBatches(rows, 200).Error; err != nil {
		return storageError("create announcement audience targets", err)
	}
	return nil
}

func (r *MySQLRepository) lockAnnouncement(
	ctx context.Context,
	tx *gorm.DB,
	announcementID int64,
) (*announcementPO, error) {
	query := tx.WithContext(ctx)
	if strings.EqualFold(tx.Dialector.Name(), "mysql") {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row announcementPO
	if err := query.Where("id = ?", announcementID).Take(&row).Error; err != nil {
		return nil, translateReadError("lock announcement", err)
	}
	return &row, nil
}

func (r *MySQLRepository) saveLifecycleRow(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	updates map[string]any,
) error {
	result := tx.WithContext(ctx).
		Model(&announcementPO{}).
		Where("id = ?", row.ID).
		Updates(updates)
	if result.Error != nil {
		return storageError("save announcement lifecycle", result.Error)
	}
	if result.RowsAffected != 1 {
		return domainannouncement.ErrStateConflict
	}
	return nil
}

func publishLifecycleUpdates(row *announcementPO) map[string]any {
	return map[string]any{
		"status":                  row.Status,
		"projection_status":       row.ProjectionStatus,
		"scheduled_at":            row.ScheduledAt,
		"publish_requested_at":    row.PublishRequestedAt,
		"snapshot_at":             row.SnapshotAt,
		"publish_actor_id":        row.PublishActorID,
		"publish_idempotency_key": row.PublishIdempotencyKey,
		"publish_request_hash":    row.PublishRequestHash,
		"snapshot_cursor":         row.SnapshotCursor,
		"projection_cursor":       row.ProjectionCursor,
		"snapshot_complete":       row.SnapshotComplete,
		"recipient_count":         row.RecipientCount,
		"projected_count":         row.ProjectedCount,
		"next_batch_no":           row.NextBatchNo,
		"last_error_code":         row.LastErrorCode,
		"updated_by":              row.UpdatedBy,
		"updated_at":              row.UpdatedAt,
		"version":                 row.Version,
	}
}

func (r *MySQLRepository) appendAudit(
	ctx context.Context,
	tx *gorm.DB,
	row *announcementPO,
	actorID int64,
	action string,
	fromStatus string,
	toStatus string,
	result string,
	errorCode string,
	now int64,
) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil || id <= 0 {
		return storageError("allocate announcement audit ID", err)
	}
	return r.createAuditWithID(
		ctx,
		tx,
		id,
		row,
		actorID,
		action,
		fromStatus,
		toStatus,
		result,
		errorCode,
		now,
	)
}

func (r *MySQLRepository) createAuditWithID(
	ctx context.Context,
	tx *gorm.DB,
	id int64,
	row *announcementPO,
	actorID int64,
	action string,
	fromStatus string,
	toStatus string,
	result string,
	errorCode string,
	now int64,
) error {
	event := &auditEventPO{
		ID:              id,
		AnnouncementID:  row.ID,
		ActorID:          actorID,
		Action:           action,
		FromStatus:       fromStatus,
		ToStatus:         toStatus,
		ProjectionStatus: row.ProjectionStatus,
		Result:           result,
		ErrorCode:        errorCode,
		RecipientCount:   row.RecipientCount,
		ProjectedCount:   row.ProjectedCount,
		CreatedAt:        now,
	}
	if err := tx.WithContext(ctx).Create(event).Error; err != nil {
		return storageError("create announcement audit event", err)
	}
	return nil
}

func (r *MySQLRepository) entityFromRow(
	ctx context.Context,
	db *gorm.DB,
	row *announcementPO,
) (*domainannouncement.Announcement, error) {
	targets, err := r.targetIDs(ctx, db, row.ID)
	if err != nil {
		return nil, err
	}
	return toEntity(row, targets), nil
}

func (r *MySQLRepository) targetIDs(
	ctx context.Context,
	db *gorm.DB,
	announcementID int64,
) ([]int64, error) {
	var targets []int64
	if err := db.WithContext(ctx).
		Model(&audienceTargetPO{}).
		Where("announcement_id = ?", announcementID).
		Order("target_id ASC").
		Pluck("target_id", &targets).Error; err != nil {
		return nil, storageError("read announcement audience targets", err)
	}
	return targets, nil
}

func (r *MySQLRepository) targetsByAnnouncementIDs(
	ctx context.Context,
	db *gorm.DB,
	ids []int64,
) (map[int64][]int64, error) {
	result := make(map[int64][]int64, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []audienceTargetPO
	if err := db.WithContext(ctx).
		Where("announcement_id IN ?", ids).
		Order("announcement_id ASC, target_id ASC").
		Find(&rows).Error; err != nil {
		return nil, storageError("list announcement audience targets", err)
	}
	for _, row := range rows {
		result[row.AnnouncementID] = append(
			result[row.AnnouncementID],
			row.TargetID,
		)
	}
	return result, nil
}

func toEntity(
	row *announcementPO,
	targetIDs []int64,
) *domainannouncement.Announcement {
	if row == nil {
		return nil
	}
	return &domainannouncement.Announcement{
		ID: row.ID,
		Draft: domainannouncement.Draft{
			Title:    row.Title,
			Body:     row.Body,
			Severity: domainannouncement.Severity(row.Severity),
			Route:    decodeAnnouncementRoute(row.InternalRoute),
			Audience: domainannouncement.Audience{
				Type:      domainannouncement.AudienceType(row.AudienceType),
				TargetIDs: append([]int64(nil), targetIDs...),
			},
		},
		Status:             domainannouncement.Status(row.Status),
		ProjectionStatus:   domainannouncement.ProjectionStatus(row.ProjectionStatus),
		ScheduledAt:        row.ScheduledAt,
		PublishRequestedAt: row.PublishRequestedAt,
		SnapshotAt:         row.SnapshotAt,
		PublishedAt:        row.PublishedAt,
		CancelledAt:        row.CancelledAt,
		CreatedBy:          row.CreatedBy,
		UpdatedBy:          row.UpdatedBy,
		PublishActorID:     row.PublishActorID,
		RecipientCount:     row.RecipientCount,
		ProjectedCount:     row.ProjectedCount,
		LastErrorCode:      row.LastErrorCode,
		Version:            row.Version,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func encodeAnnouncementRoute(
	route domainnotification.AnnouncementRoute,
) (string, error) {
	normalized, err := domainnotification.NormalizeAnnouncementRoute(route)
	if err != nil {
		return "", domainannouncement.ErrInvalidInput
	}
	value, err := json.Marshal(normalized)
	if err != nil || len(value) > 128 {
		return "", domainannouncement.ErrInvalidInput
	}
	return string(value), nil
}

func decodeAnnouncementRoute(value string) domainnotification.AnnouncementRoute {
	value = strings.TrimSpace(value)
	if value == "" {
		return domainnotification.AnnouncementRoute{
			Type: domainnotification.AnnouncementRouteNone,
		}
	}
	var route domainnotification.AnnouncementRoute
	if err := json.Unmarshal([]byte(value), &route); err == nil {
		normalized, normalizeErr := domainnotification.NormalizeAnnouncementRoute(
			route,
		)
		if normalizeErr == nil {
			canonical, marshalErr := json.Marshal(normalized)
			if marshalErr == nil && string(canonical) == value {
				return normalized
			}
		}
	}
	if value == "/system/announcements" {
		return domainnotification.AnnouncementRoute{
			Type: domainnotification.AnnouncementRouteSystemAnnouncements,
		}
	}
	const workspacePrefix = "/space/"
	const workspaceSuffix = "/workspace"
	if strings.HasPrefix(value, workspacePrefix) &&
		strings.HasSuffix(value, workspaceSuffix) {
		rawID := strings.TrimSuffix(
			strings.TrimPrefix(value, workspacePrefix),
			workspaceSuffix,
		)
		spaceID, err := strconv.ParseInt(rawID, 10, 64)
		if err == nil &&
			spaceID > 0 &&
			strconv.FormatInt(spaceID, 10) == rawID {
			return domainnotification.AnnouncementRoute{
				Type:    domainnotification.AnnouncementRouteWorkspaceHome,
				SpaceID: spaceID,
			}
		}
	}
	return domainnotification.AnnouncementRoute{
		Type: domainnotification.AnnouncementRouteNone,
	}
}

func announcementIDs(rows []*announcementPO) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			ids = append(ids, row.ID)
		}
	}
	return ids
}

func (r *MySQLRepository) configured() bool {
	return r != nil && r.db != nil && r.idGen != nil && r.outbox != nil
}

func translateReadError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainannouncement.ErrNotFound
	}
	return storageError(operation, err)
}

func storageError(operation string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", domainannouncement.ErrStorage, operation)
	}
	return fmt.Errorf(
		"%w: %s: %w",
		domainannouncement.ErrStorage,
		operation,
		err,
	)
}
