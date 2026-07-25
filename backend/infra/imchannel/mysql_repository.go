// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	configTable             = "im_channel_configs"
	sessionTable            = "im_channel_sessions"
	eventTable              = "im_channel_events"
	workspaceRoleOwner int32 = 1
	workspaceRoleAdmin int32 = 2
)

type MySQLRepository struct {
	db                 *gorm.DB
	notificationOutbox NotificationOutboxAppender
}

type NotificationOutboxAppender interface {
	AppendInTransaction(context.Context, *gorm.DB, domainnotification.Event) error
}

type MySQLOption func(*MySQLRepository)

func WithNotificationOutboxAppender(appender NotificationOutboxAppender) MySQLOption {
	return func(r *MySQLRepository) {
		r.notificationOutbox = appender
	}
}

func NewMySQLRepository(db *gorm.DB, options ...MySQLOption) *MySQLRepository {
	repository := &MySQLRepository{db: db}
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
}

func (r *MySQLRepository) ListConfigs(ctx context.Context, spaceID int64) ([]*domain.Config, error) {
	var configs []*domain.Config
	err := r.db.WithContext(ctx).Table(configTable).
		Where("space_id = ? AND deleted_at IS NULL", spaceID).
		Order("created_at DESC, id DESC").
		Find(&configs).Error
	return configs, err
}

func (r *MySQLRepository) GetConfig(ctx context.Context, spaceID, configID int64) (*domain.Config, error) {
	var config domain.Config
	err := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND space_id = ? AND deleted_at IS NULL", configID, spaceID).
		Take(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *MySQLRepository) GetConfigByID(ctx context.Context, configID int64) (*domain.Config, error) {
	var config domain.Config
	err := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND deleted_at IS NULL", configID).
		Take(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *MySQLRepository) CreateConfig(ctx context.Context, config *domain.Config) error {
	if config == nil {
		return domain.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Table(configTable).Create(config).Error
	if isDuplicateEntry(err) {
		return domain.ErrConflict
	}
	return err
}

func (r *MySQLRepository) UpdateConfig(ctx context.Context, config *domain.Config) error {
	if config == nil {
		return domain.ErrInvalidInput
	}
	updates := map[string]any{
		"name":                     config.Name,
		"app_id":                   config.AppID,
		"agent_id":                 config.AgentID,
		"reply_mode":               config.ReplyMode,
		"group_policy":             config.GroupPolicy,
		"updated_by":               config.UpdatedBy,
		"runtime_status":           config.RuntimeStatus,
		"runtime_error":            "",
		"runtime_consecutive_failures": 0,
		"runtime_incident_id":          "",
		"runtime_incident_notified_at": nil,
		"runtime_recovery_notified_at": nil,
		"runtime_last_recovered_at":    nil,
		"runtime_owner":            "",
		"runtime_lease_expires_at": nil,
		"version":                  gorm.Expr("version + 1"),
		"updated_at":               time.Now(),
	}
	if config.AppSecretCiphertext != "" {
		updates["app_secret_ciphertext"] = config.AppSecretCiphertext
		updates["app_secret_fingerprint"] = config.AppSecretFingerprint
	}
	result := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND space_id = ? AND version = ? AND deleted_at IS NULL", config.ID, config.SpaceID, config.Version).
		Updates(updates)
	if isDuplicateEntry(result.Error) {
		return domain.ErrConflict
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *MySQLRepository) SetEnabled(
	ctx context.Context,
	spaceID, configID, actorID int64,
	enabled bool,
	status domain.RuntimeStatus,
) error {
	result := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND space_id = ? AND deleted_at IS NULL", configID, spaceID).
		Updates(map[string]any{
			"enabled":                  enabled,
			"runtime_status":           status,
			"runtime_error":            "",
			"runtime_consecutive_failures": 0,
			"runtime_incident_id":          "",
			"runtime_incident_notified_at": nil,
			"runtime_recovery_notified_at": nil,
			"runtime_last_recovered_at":    nil,
			"runtime_owner":            "",
			"runtime_lease_expires_at": nil,
			"updated_by":               actorID,
			"version":                  gorm.Expr("version + 1"),
			"updated_at":               time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MySQLRepository) DeleteConfig(ctx context.Context, spaceID, configID, actorID int64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND space_id = ? AND deleted_at IS NULL", configID, spaceID).
		Updates(map[string]any{
			"enabled":                  false,
			"app_id":                   gorm.Expr("CONCAT(LEFT(app_id, 80), '#deleted#', id)"),
			"runtime_status":           domain.RuntimeStatusDisabled,
			"runtime_consecutive_failures": 0,
			"runtime_incident_id":          "",
			"runtime_incident_notified_at": nil,
			"runtime_recovery_notified_at": nil,
			"runtime_last_recovered_at":    nil,
			"runtime_owner":            "",
			"runtime_lease_expires_at": nil,
			"updated_by":               actorID,
			"deleted_at":               now,
			"updated_at":               now,
			"version":                  gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MySQLRepository) MarkConnectionTest(
	ctx context.Context,
	configID int64,
	botOpenID, botName string,
	testedAt time.Time,
) error {
	return r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND deleted_at IS NULL", configID).
		Updates(map[string]any{
			"bot_open_id":    botOpenID,
			"bot_name":       botName,
			"last_tested_at": testedAt,
			"updated_at":     testedAt,
		}).Error
}

func (r *MySQLRepository) ListEnabledConfigs(ctx context.Context) ([]*domain.Config, error) {
	var configs []*domain.Config
	err := r.db.WithContext(ctx).Table(configTable).
		Where("enabled = ? AND deleted_at IS NULL", true).
		Order("id ASC").
		Find(&configs).Error
	return configs, err
}

func (r *MySQLRepository) TryAcquireRuntimeLease(
	ctx context.Context,
	configID int64,
	owner string,
	now, expiresAt time.Time,
) (bool, error) {
	result := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND enabled = ? AND deleted_at IS NULL", configID, true).
		Where("(runtime_owner = '' OR runtime_owner = ? OR runtime_lease_expires_at IS NULL OR runtime_lease_expires_at < ?)", owner, now).
		Updates(map[string]any{
			"runtime_owner":            owner,
			"runtime_lease_expires_at": expiresAt,
		})
	return result.RowsAffected == 1, result.Error
}

func (r *MySQLRepository) RenewRuntimeLease(
	ctx context.Context,
	configID int64,
	owner string,
	expiresAt time.Time,
) (bool, error) {
	result := r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND runtime_owner = ? AND enabled = ? AND deleted_at IS NULL", configID, owner, true).
		Update("runtime_lease_expires_at", expiresAt)
	return result.RowsAffected == 1, result.Error
}

func (r *MySQLRepository) ReleaseRuntimeLease(ctx context.Context, configID int64, owner string) error {
	return r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND runtime_owner = ?", configID, owner).
		Updates(map[string]any{
			"runtime_owner":            "",
			"runtime_lease_expires_at": nil,
		}).Error
}

func (r *MySQLRepository) UpdateRuntimeState(ctx context.Context, configID int64, state domain.RuntimeState) error {
	updates := map[string]any{
		"runtime_status": state.Status,
		"runtime_error":  state.Error,
	}
	if state.BotOpenID != "" {
		updates["bot_open_id"] = state.BotOpenID
	}
	if state.BotName != "" {
		updates["bot_name"] = state.BotName
	}
	if state.ConnectedAt != nil {
		updates["last_connected_at"] = *state.ConnectedAt
	}
	if state.ClearConnection {
		updates["last_connected_at"] = nil
	}
	return r.db.WithContext(ctx).Table(configTable).
		Where("id = ? AND deleted_at IS NULL", configID).
		Updates(updates).Error
}

func (r *MySQLRepository) RecordRuntimeFailure(
	ctx context.Context,
	configID int64,
	state domain.RuntimeState,
	stableFailureThreshold int,
) error {
	if stableFailureThreshold <= 0 {
		stableFailureThreshold = domain.DefaultRuntimeStableFailureThreshold
	}
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var config domain.Config
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table(configTable).
			Where("id = ? AND deleted_at IS NULL", configID).
			Take(&config).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}

		failures := config.RuntimeConsecutiveFailures + 1
		incidentID := strings.TrimSpace(config.RuntimeIncidentID)
		if incidentID == "" {
			incidentID = strings.TrimSpace(state.IncidentID)
		}
		if incidentID == "" {
			incidentID = fmt.Sprintf("feishu-im:%d:%d", configID, now.UnixNano())
		}
		updates := map[string]any{
			"runtime_status":               state.Status,
			"runtime_error":                state.Error,
			"runtime_consecutive_failures": failures,
			"runtime_incident_id":          incidentID,
			"updated_at":                   now,
		}
		if config.RuntimeIncidentID == "" {
			updates["runtime_recovery_notified_at"] = nil
			updates["runtime_last_recovered_at"] = nil
		}
		if state.ClearConnection {
			updates["last_connected_at"] = nil
		}
		shouldNotify := int(failures) >= stableFailureThreshold &&
			config.RuntimeIncidentNotifiedAt == nil
		if shouldNotify {
			recipients, err := r.notificationRecipientsForConfig(tx, &config)
			if err != nil {
				return err
			}
			updates["runtime_incident_notified_at"] = now
			if err := updateConfigColumns(tx, configID, updates); err != nil {
				return err
			}
			return r.appendNotificationInTransaction(ctx, tx, imChannelNotificationEvent(
				&config,
				domainnotification.EventIMChannelConnectionFailed,
				fmt.Sprintf("im_channel:%d:%s:failed", config.ID, incidentID),
				incidentID,
				1,
				now,
				domainnotification.StatusReasonConnectionFailed,
				recipients,
			))
		}
		return updateConfigColumns(tx, configID, updates)
	})
}

func (r *MySQLRepository) RecordRuntimeRecovery(
	ctx context.Context,
	configID int64,
	state domain.RuntimeState,
) error {
	now := time.Now()
	if state.ConnectedAt != nil {
		now = *state.ConnectedAt
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var config domain.Config
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table(configTable).
			Where("id = ? AND deleted_at IS NULL", configID).
			Take(&config).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}

		updates := map[string]any{
			"runtime_status":               state.Status,
			"runtime_error":                "",
			"runtime_consecutive_failures": 0,
			"runtime_incident_id":          "",
			"runtime_incident_notified_at": nil,
			"runtime_last_recovered_at":    now,
			"updated_at":                   now,
		}
		if state.BotOpenID != "" {
			updates["bot_open_id"] = state.BotOpenID
		}
		if state.BotName != "" {
			updates["bot_name"] = state.BotName
		}
		if state.ConnectedAt != nil {
			updates["last_connected_at"] = *state.ConnectedAt
		}
		shouldNotify := strings.TrimSpace(config.RuntimeIncidentID) != "" &&
			config.RuntimeIncidentNotifiedAt != nil &&
			config.RuntimeRecoveryNotifiedAt == nil
		if shouldNotify {
			recipients, err := r.notificationRecipientsForConfig(tx, &config)
			if err != nil {
				return err
			}
			updates["runtime_recovery_notified_at"] = now
			if err := updateConfigColumns(tx, configID, updates); err != nil {
				return err
			}
			return r.appendNotificationInTransaction(ctx, tx, imChannelNotificationEvent(
				&config,
				domainnotification.EventIMChannelRecovered,
				fmt.Sprintf("im_channel:%d:%s:recovered", config.ID, config.RuntimeIncidentID),
				config.RuntimeIncidentID,
				1,
				now,
				domainnotification.StatusReasonNone,
				recipients,
			))
		}
		return updateConfigColumns(tx, configID, updates)
	})
}

func (r *MySQLRepository) InsertEvent(ctx context.Context, event *domain.Event) (bool, error) {
	if event == nil {
		return false, domain.ErrInvalidInput
	}
	result := r.db.WithContext(ctx).Table(eventTable).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "config_id"}, {Name: "event_key"}},
			DoNothing: true,
		}).
		Create(event)
	return result.RowsAffected == 1, result.Error
}

func (r *MySQLRepository) ClaimEvents(
	ctx context.Context,
	configID int64,
	owner string,
	now, leaseUntil time.Time,
	limit int,
) ([]*domain.Event, error) {
	if limit <= 0 {
		limit = 20
	}
	var candidates []*domain.Event
	claimCondition := "((attempt_count < ? AND ((status = ?) OR (status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND processing_lease_until < ?))) OR (attempt_count >= ? AND ((status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND processing_lease_until < ?))))"
	err := r.db.WithContext(ctx).Table(eventTable).
		Where("config_id = ?", configID).
		Where(claimCondition,
			domain.EventMaxAttempts,
			domain.EventStatusPending,
			domain.EventStatusFailed,
			now,
			domain.EventStatusProcessing,
			now,
			domain.EventMaxAttempts,
			domain.EventStatusFailed,
			now,
			domain.EventStatusProcessing,
			now,
		).
		Order("id ASC").
		Limit(limit).
		Find(&candidates).Error
	if err != nil {
		return nil, err
	}

	claimed := make([]*domain.Event, 0, len(candidates))
	for _, event := range candidates {
		previousStatus := event.Status
		previousAttemptCount := event.AttemptCount
		result := r.db.WithContext(ctx).Table(eventTable).
			Where("id = ? AND attempt_count = ?", event.ID, event.AttemptCount).
			Where(claimCondition,
				domain.EventMaxAttempts,
				domain.EventStatusPending,
				domain.EventStatusFailed,
				now,
				domain.EventStatusProcessing,
				now,
				domain.EventMaxAttempts,
				domain.EventStatusFailed,
				now,
				domain.EventStatusProcessing,
				now,
			).
			Updates(map[string]any{
				"status":                 domain.EventStatusProcessing,
				"processing_owner":       owner,
				"processing_lease_until": leaseUntil,
				"updated_at":             now,
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			event.Status = domain.EventStatusProcessing
			event.AttemptCount = previousAttemptCount
			if previousAttemptCount >= domain.EventMaxAttempts &&
				(previousStatus == domain.EventStatusFailed ||
					previousStatus == domain.EventStatusProcessing) &&
				event.AttemptCount >= domain.EventMaxAttempts {
				event.Status = domain.EventStatusFailed
			}
			event.ProcessingOwner = owner
			event.ProcessingLeaseUntil = &leaseUntil
			claimed = append(claimed, event)
		}
	}
	return claimed, nil
}

func (r *MySQLRepository) CompleteEvent(ctx context.Context, eventID int64, completedAt time.Time) error {
	return r.db.WithContext(ctx).Table(eventTable).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"status":                 domain.EventStatusSucceeded,
			"payload_json":           "",
			"last_error":             "",
			"processing_owner":       "",
			"processing_lease_until": nil,
			"completed_at":           completedAt,
			"updated_at":             completedAt,
		}).Error
}

func (r *MySQLRepository) FailEvent(
	ctx context.Context,
	eventID int64,
	message string,
	nextRetryAt time.Time,
) error {
	return r.db.WithContext(ctx).Table(eventTable).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"status":                 domain.EventStatusFailed,
			"attempt_count":          gorm.Expr("CASE WHEN attempt_count < ? THEN attempt_count + 1 ELSE attempt_count END", domain.EventMaxAttempts),
			"last_error":             message,
			"next_retry_at":          nextRetryAt,
			"processing_owner":       "",
			"processing_lease_until": nil,
			"updated_at":             time.Now(),
		}).Error
}

func (r *MySQLRepository) DeadLetterEvent(
	ctx context.Context,
	config *domain.Config,
	event *domain.Event,
	message string,
	deadLetteredAt time.Time,
) (bool, error) {
	if config == nil || event == nil {
		return false, domain.ErrInvalidInput
	}
	if deadLetteredAt.IsZero() {
		deadLetteredAt = time.Now()
	}
	deadLettered := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var currentEvent domain.Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table(eventTable).
			Where("id = ? AND config_id = ?", event.ID, config.ID).
			Take(&currentEvent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if currentEvent.Status == domain.EventStatusDeadLetter {
			return nil
		}

		var currentConfig domain.Config
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table(configTable).
			Where("id = ? AND deleted_at IS NULL", config.ID).
			Take(&currentConfig).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		recipients, err := r.notificationRecipientsForConfig(tx, &currentConfig)
		if err != nil {
			return err
		}
		result := tx.Table(eventTable).
			Where("id = ? AND config_id = ? AND status <> ?", event.ID, config.ID, domain.EventStatusDeadLetter).
			Updates(map[string]any{
				"status":                 domain.EventStatusDeadLetter,
				"attempt_count":          gorm.Expr("CASE WHEN attempt_count < ? THEN attempt_count + 1 ELSE attempt_count END", domain.EventMaxAttempts),
				"payload_json":           "",
				"last_error":             message,
				"processing_owner":       "",
				"processing_lease_until": nil,
				"completed_at":           deadLetteredAt,
				"updated_at":             deadLetteredAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		aggregateVersion := int64(currentEvent.AttemptCount)
		if currentEvent.AttemptCount < domain.EventMaxAttempts {
			aggregateVersion = int64(currentEvent.AttemptCount + 1)
		}
		if aggregateVersion <= 0 {
			aggregateVersion = 1
		}
		if err := r.appendNotificationInTransaction(ctx, tx, imChannelNotificationEvent(
			&currentConfig,
			domainnotification.EventIMMessageDeadLettered,
			fmt.Sprintf("im_message:%d:dead_lettered", currentEvent.ID),
			strconv.FormatInt(currentEvent.ID, 10),
			aggregateVersion,
			deadLetteredAt,
			domainnotification.StatusReasonRetryExhausted,
			recipients,
		)); err != nil {
			return err
		}
		deadLettered = true
		return nil
	})
	return deadLettered, err
}

func (r *MySQLRepository) CleanupEvents(ctx context.Context, completedBefore time.Time) error {
	return r.db.WithContext(ctx).Table(eventTable).
		Where("status IN ? AND completed_at < ?", []domain.EventStatus{domain.EventStatusSucceeded, domain.EventStatusDeadLetter}, completedBefore).
		Delete(&domain.Event{}).Error
}

func (r *MySQLRepository) GetSession(ctx context.Context, configID int64, chatID string) (*domain.Session, error) {
	var session domain.Session
	err := r.db.WithContext(ctx).Table(sessionTable).
		Where("config_id = ? AND chat_id = ?", configID, chatID).
		Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *MySQLRepository) SaveSession(ctx context.Context, session *domain.Session) error {
	if session == nil {
		return domain.ErrInvalidInput
	}
	return r.db.WithContext(ctx).Table(sessionTable).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "config_id"}, {Name: "chat_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"chat_type":        session.ChatType,
				"external_user_id": session.ExternalUserID,
				"thread_id":        session.ThreadID,
				"last_message_id":  session.LastMessageID,
				"updated_at":       session.UpdatedAt,
			}),
		}).
		Create(session).Error
}

func isDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate entry")
}

func updateConfigColumns(tx *gorm.DB, configID int64, updates map[string]any) error {
	result := tx.Table(configTable).
		Where("id = ? AND deleted_at IS NULL", configID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MySQLRepository) appendNotificationInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	if r == nil || r.notificationOutbox == nil {
		return domainnotification.ErrStorage
	}
	return r.notificationOutbox.AppendInTransaction(ctx, tx, event)
}

func (r *MySQLRepository) notificationRecipientsForConfig(
	tx *gorm.DB,
	config *domain.Config,
) ([]int64, error) {
	if config == nil {
		return nil, domain.ErrInvalidInput
	}
	candidateIDs := make([]int64, 0, 2)
	if config.CreatorID > 0 {
		candidateIDs = append(candidateIDs, config.CreatorID)
	}
	if config.UpdatedBy > 0 {
		candidateIDs = append(candidateIDs, config.UpdatedBy)
	}
	var members []struct {
		UserID int64 `gorm:"column:user_id"`
	}
	query := tx.Table("space_user").
		Select("user_id").
		Where("space_id = ?", config.SpaceID).
		Where("role_type IN ?", []int32{workspaceRoleOwner, workspaceRoleAdmin})
	if len(candidateIDs) > 0 {
		query = tx.Table("space_user").
			Select("user_id").
			Where("space_id = ?", config.SpaceID).
			Where("(role_type IN ? OR user_id IN ?)", []int32{workspaceRoleOwner, workspaceRoleAdmin}, candidateIDs)
	}
	if err := query.Find(&members).Error; err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	recipients, err := domainnotification.NormalizeRecipientIDs(ids)
	if err != nil {
		return nil, err
	}
	return recipients, nil
}

func imChannelNotificationEvent(
	config *domain.Config,
	eventType domainnotification.EventType,
	eventID string,
	aggregateID string,
	aggregateVersion int64,
	occurredAt time.Time,
	reason domainnotification.StatusReasonCode,
	recipients []int64,
) domainnotification.Event {
	actorID := config.UpdatedBy
	if actorID <= 0 {
		actorID = config.CreatorID
	}
	aggregateType := "im_channel"
	if eventType == domainnotification.EventIMMessageDeadLettered {
		aggregateType = "im_channel_event"
	}
	return domainnotification.Event{
		EventID:          eventID,
		EventType:        eventType,
		AggregateType:    aggregateType,
		AggregateID:      aggregateID,
		AggregateVersion: aggregateVersion,
		OccurredAt:       occurredAt,
		ActorID:          actorID,
		SpaceID:          config.SpaceID,
		RecipientPolicy:  domainnotification.RecipientExplicitInternalUsers,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName:  config.Name,
			StatusReasonCode:     reason,
			TargetID:             strconv.FormatInt(config.SpaceID, 10),
			ExplicitRecipientIDs: recipients,
		},
	}
}

var _ domain.Repository = (*MySQLRepository)(nil)
