// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	configTable  = "im_channel_configs"
	sessionTable = "im_channel_sessions"
	eventTable   = "im_channel_events"
)

type MySQLRepository struct {
	db *gorm.DB
}

func NewMySQLRepository(db *gorm.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
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
	err := r.db.WithContext(ctx).Table(eventTable).
		Where("config_id = ? AND attempt_count < ?", configID, 3).
		Where("((status = ?) OR (status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND processing_lease_until < ?))",
			domain.EventStatusPending,
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
		result := r.db.WithContext(ctx).Table(eventTable).
			Where("id = ? AND attempt_count = ?", event.ID, event.AttemptCount).
			Where("((status = ?) OR (status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND processing_lease_until < ?))",
				domain.EventStatusPending,
				domain.EventStatusFailed,
				now,
				domain.EventStatusProcessing,
				now,
			).
			Updates(map[string]any{
				"status":                 domain.EventStatusProcessing,
				"attempt_count":          gorm.Expr("attempt_count + 1"),
				"processing_owner":       owner,
				"processing_lease_until": leaseUntil,
				"updated_at":             now,
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			event.Status = domain.EventStatusProcessing
			event.AttemptCount++
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
			"last_error":             message,
			"next_retry_at":          nextRetryAt,
			"processing_owner":       "",
			"processing_lease_until": nil,
			"updated_at":             time.Now(),
		}).Error
}

func (r *MySQLRepository) CleanupEvents(ctx context.Context, completedBefore time.Time) error {
	return r.db.WithContext(ctx).Table(eventTable).
		Where("status = ? AND completed_at < ?", domain.EventStatusSucceeded, completedBefore).
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

var _ domain.Repository = (*MySQLRepository)(nil)
