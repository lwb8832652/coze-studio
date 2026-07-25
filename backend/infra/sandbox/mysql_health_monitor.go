// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	maxHealthMonitorWorkerIDBytes                = 80
	healthProjectionStatusPending                = "pending"
	healthProjectionStatusProjected              = "projected"
	defaultHealthProjectionRetryDelay             = 5 * time.Second
)

var ErrHealthMonitorInfrastructure = domainsandbox.ErrHealthMonitorDBClock

type HealthNotificationOutbox interface {
	AppendInTransaction(context.Context, *gorm.DB, domainnotification.Event) error
}

type healthMonitorDatabaseClock func(
	context.Context,
	*gorm.DB,
	time.Time,
) (time.Time, error)

type MySQLHealthMonitorRepository struct {
	db            *gorm.DB
	outbox        HealthNotificationOutbox
	databaseClock healthMonitorDatabaseClock
}

type providerHealthEpisodePO struct {
	ProviderID          uint64 `gorm:"column:provider_id;type:bigint unsigned;primaryKey"`
	ConsecutiveFailures int    `gorm:"column:consecutive_failures;not null;default:0"`
	FailureStartedAt    int64  `gorm:"column:failure_started_at;not null;default:0"`
	IncidentSequence    uint64 `gorm:"column:incident_sequence;type:bigint unsigned;not null;default:0"`
	IncidentID          string `gorm:"column:incident_id;size:128;not null;default:'';index:idx_sandbox_health_episode_incident,priority:2"`
	IncidentStatus      string `gorm:"column:incident_status;size:16;not null;default:none;index:idx_sandbox_health_episode_incident,priority:1"`
	IncidentOpenedAt    int64  `gorm:"column:incident_opened_at;not null;default:0"`
	IncidentNotifiedAt  int64  `gorm:"column:incident_notified_at;not null;default:0"`
	IncidentClosedAt    int64  `gorm:"column:incident_closed_at;not null;default:0"`
	RecoveryNotifiedAt  int64  `gorm:"column:recovery_notified_at;not null;default:0"`
	LastRecoveredAt     int64  `gorm:"column:last_recovered_at;not null;default:0"`
	LastCheckedAt       int64  `gorm:"column:last_checked_at;not null;default:0"`
	NextCheckAt         int64  `gorm:"column:next_check_at;not null;default:0;index:idx_sandbox_health_episode_due,priority:1"`
	LeaseOwner          string `gorm:"column:lease_owner;size:80;not null;default:''"`
	LeaseToken          string `gorm:"column:lease_token;size:128;not null;default:''"`
	LeaseExpiresAt      int64  `gorm:"column:lease_expires_at;not null;default:0;index:idx_sandbox_health_episode_due,priority:2"`
	Version             uint64 `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	CreatedAt           int64  `gorm:"column:created_at;not null"`
	UpdatedAt           int64  `gorm:"column:updated_at;not null"`
}

func (providerHealthEpisodePO) TableName() string {
	return "sandbox_provider_health_episodes"
}

type providerHealthNotificationProjectionPO struct {
	EventID           string `gorm:"column:event_id;size:128;primaryKey"`
	ProviderID        uint64 `gorm:"column:provider_id;type:bigint unsigned;not null;index:idx_sandbox_health_projection_provider,priority:1"`
	IncidentID        string `gorm:"column:incident_id;size:128;not null;uniqueIndex:uk_sandbox_health_projection_incident,priority:1"`
	IncidentSequence  uint64 `gorm:"column:incident_sequence;type:bigint unsigned;not null;index:idx_sandbox_health_projection_provider,priority:2"`
	NotificationType  string `gorm:"column:notification_type;size:16;not null;uniqueIndex:uk_sandbox_health_projection_incident,priority:2"`
	OccurredAt        int64  `gorm:"column:occurred_at;not null"`
	Status            string `gorm:"column:status;size:16;not null;index:idx_sandbox_health_projection_pending,priority:1"`
	AttemptCount      int    `gorm:"column:attempt_count;not null;default:0"`
	NextAttemptAt     int64  `gorm:"column:next_attempt_at;not null;default:0;index:idx_sandbox_health_projection_pending,priority:2"`
	LeaseOwner        string `gorm:"column:lease_owner;size:80;not null;default:''"`
	LeaseToken        string `gorm:"column:lease_token;size:128;not null;default:''"`
	LeaseExpiresAt    int64  `gorm:"column:lease_expires_at;not null;default:0;index:idx_sandbox_health_projection_pending,priority:3"`
	ProjectedAt       int64  `gorm:"column:projected_at;not null;default:0"`
	Version           uint64 `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	CreatedAt         int64  `gorm:"column:created_at;not null"`
	UpdatedAt         int64  `gorm:"column:updated_at;not null"`
}

func (providerHealthNotificationProjectionPO) TableName() string {
	return "sandbox_health_notification_projections"
}

func NewMySQLHealthMonitorRepository(
	db *gorm.DB,
	outbox HealthNotificationOutbox,
) *MySQLHealthMonitorRepository {
	return &MySQLHealthMonitorRepository{db: db, outbox: outbox}
}

func (r *MySQLHealthMonitorRepository) ReconcileDisabledHealthEpisodes(
	ctx context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	if r == nil || r.db == nil || ctx == nil || now.IsZero() ||
		limit < 1 || limit > domainsandbox.MaxHealthMonitorBatchSize {
		return 0, domainsandbox.ErrInvalidInput
	}
	databaseNow, err := r.currentDatabaseTime(ctx, r.db, now)
	if err != nil {
		return 0, err
	}
	nowMillis := databaseNow.UnixMilli()
	var reconciled int64
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var providerIDs []uint64
		if err := tx.Table("sandbox_provider_health_episodes AS episodes").
			Select("episodes.provider_id").
			Joins("LEFT JOIN sandbox_providers AS providers ON providers.id = episodes.provider_id").
			Where("episodes.incident_status IN ?", []string{
				string(domainsandbox.HealthIncidentStatusObserving),
				string(domainsandbox.HealthIncidentStatusOpen),
			}).
			Where("(providers.id IS NULL OR providers.deleted_at IS NOT NULL OR providers.status <> ?)",
				string(domainsandbox.ProviderStatusEnabled)).
			Order("episodes.provider_id ASC").
			Limit(limit).
			Scan(&providerIDs).Error; err != nil {
			return err
		}
		for _, providerID := range providerIDs {
			result := tx.Model(&providerHealthEpisodePO{}).
				Where("provider_id = ? AND incident_status IN ?", providerID, []string{
					string(domainsandbox.HealthIncidentStatusObserving),
					string(domainsandbox.HealthIncidentStatusOpen),
				}).
				Updates(map[string]any{
					"consecutive_failures": 0,
					"failure_started_at": 0,
					"incident_status": string(domainsandbox.HealthIncidentStatusDisabled),
					"incident_closed_at": nowMillis,
					"next_check_at": 0,
					"lease_owner": "",
					"lease_token": "",
					"lease_expires_at": 0,
					"updated_at": nowMillis,
					"version": gorm.Expr("version + 1"),
				})
			if result.Error != nil {
				return result.Error
			}
			reconciled += result.RowsAffected
		}
		return nil
	})
	return reconciled, err
}

func (r *MySQLHealthMonitorRepository) ClaimHealthChecks(
	ctx context.Context,
	request domainsandbox.HealthMonitorClaimRequest,
) ([]domainsandbox.HealthMonitorClaim, error) {
	if r == nil || r.db == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	workerID := strings.TrimSpace(request.WorkerID)
	if workerID == "" ||
		len(workerID) > maxHealthMonitorWorkerIDBytes ||
		request.Now.IsZero() ||
		request.Lease <= 0 ||
		request.Limit < 1 ||
		request.Limit > domainsandbox.MaxHealthMonitorBatchSize {
		return nil, domainsandbox.ErrInvalidInput
	}
	now, err := r.currentDatabaseTime(ctx, r.db, request.Now)
	if err != nil {
		return nil, err
	}
	leaseExpiresAt := now.Add(request.Lease)
	if !leaseExpiresAt.After(now) {
		return nil, domainsandbox.ErrInvalidInput
	}
	claims := make([]domainsandbox.HealthMonitorClaim, 0, request.Limit)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureHealthEpisodeRows(tx, now, request.Limit); err != nil {
			return err
		}
		var candidateIDs []uint64
		if err := tx.Table("sandbox_provider_health_episodes AS episodes").
			Select("episodes.provider_id").
			Joins("JOIN sandbox_providers AS providers ON providers.id = episodes.provider_id").
			Where("providers.deleted_at IS NULL AND providers.status = ?",
				string(domainsandbox.ProviderStatusEnabled)).
			Where("episodes.next_check_at <= ?", now.UnixMilli()).
			Where("(episodes.lease_expires_at = 0 OR episodes.lease_expires_at <= ?)",
				now.UnixMilli()).
			Order("episodes.next_check_at ASC, episodes.provider_id ASC").
			Limit(request.Limit).
			Scan(&candidateIDs).Error; err != nil {
			return err
		}
		for _, providerID := range candidateIDs {
			token := healthMonitorLeaseToken(workerID, providerID, now)
			result := tx.Model(&providerHealthEpisodePO{}).
				Where("provider_id = ? AND next_check_at <= ?", providerID, now.UnixMilli()).
				Where("(lease_expires_at = 0 OR lease_expires_at <= ?)", now.UnixMilli()).
				Updates(map[string]any{
					"lease_owner": workerID,
					"lease_token": token,
					"lease_expires_at": leaseExpiresAt.UnixMilli(),
					"updated_at": now.UnixMilli(),
					"version": gorm.Expr("version + 1"),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			domainProviderID, err := positivePersistedUint64ToInt64(providerID)
			if err != nil {
				return err
			}
			po, err := findLiveProvider(tx, domainProviderID, false)
			if err != nil {
				return err
			}
			if po.Status != string(domainsandbox.ProviderStatusEnabled) {
				if err := releaseHealthMonitorClaim(
					tx,
					providerID,
					token,
					now.UnixMilli(),
					0,
				); err != nil {
					return err
				}
				continue
			}
			provider, err := po.toDomain()
			if err != nil {
				return err
			}
			claims = append(claims, domainsandbox.HealthMonitorClaim{
				Provider: provider,
				LeaseOwner: workerID,
				LeaseToken: token,
				ClaimedAt: now,
				LeaseExpiresAt: leaseExpiresAt,
			})
		}
		return nil
	})
	return claims, err
}

func (r *MySQLHealthMonitorRepository) CompleteHealthCheck(
	ctx context.Context,
	input domainsandbox.CompleteHealthMonitorCheckInput,
) (domainsandbox.HealthMonitorCheckCompletion, error) {
	var completion domainsandbox.HealthMonitorCheckCompletion
	if r == nil || r.db == nil || ctx == nil ||
		input.Claim.Provider == nil ||
		input.Claim.Provider.ID <= 0 ||
		input.Claim.Provider.Version == 0 ||
		strings.TrimSpace(input.Claim.LeaseOwner) == "" ||
		strings.TrimSpace(input.Claim.LeaseToken) == "" ||
		input.CheckedAt.IsZero() ||
		!input.NextCheckAt.After(input.CheckedAt) ||
		input.FailureThreshold <= 0 {
		return completion, domainsandbox.ErrInvalidInput
	}
	checkedAt := input.CheckedAt.UTC().Truncate(time.Millisecond)
	nextCheckAt := input.NextCheckAt.UTC().Truncate(time.Millisecond)
	if !nextCheckAt.After(checkedAt) {
		return completion, domainsandbox.ErrInvalidInput
	}
	staleProviderVersion := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var episodePO providerHealthEpisodePO
		query := withUpdateLock(tx.Where("provider_id = ?", input.Claim.Provider.ID))
		if err := query.Take(&episodePO).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domainsandbox.ErrVersionConflict
			}
			return err
		}
		if episodePO.LeaseOwner != strings.TrimSpace(input.Claim.LeaseOwner) ||
			episodePO.LeaseToken != strings.TrimSpace(input.Claim.LeaseToken) {
			return domainsandbox.ErrVersionConflict
		}
		databaseNow, err := r.currentDatabaseTime(ctx, tx, checkedAt)
		if err != nil {
			return err
		}
		if episodePO.LeaseExpiresAt == 0 ||
			episodePO.LeaseExpiresAt <= databaseNow.UnixMilli() {
			return domainsandbox.ErrVersionConflict
		}
		checkInterval := nextCheckAt.Sub(checkedAt)
		checkedAt = databaseNow
		nextCheckAt = databaseNow.Add(checkInterval)

		providerPO, err := findLiveProvider(tx, input.Claim.Provider.ID, true)
		if errors.Is(err, domainsandbox.ErrProviderNotFound) {
			return closeClaimForDisabledProvider(
				tx,
				&episodePO,
				checkedAt,
				nextCheckAt,
				&completion,
			)
		}
		if err != nil {
			return err
		}
		if providerPO.Status != string(domainsandbox.ProviderStatusEnabled) {
			return closeClaimForDisabledProvider(
				tx,
				&episodePO,
				checkedAt,
				nextCheckAt,
				&completion,
			)
		}
		if providerPO.Version != input.Claim.Provider.Version {
			if err := releaseStaleHealthMonitorClaim(
				tx,
				&episodePO,
				checkedAt,
				nextCheckAt,
			); err != nil {
				return err
			}
			staleProviderVersion = true
			return nil
		}
		provider, err := providerPO.toDomain()
		if err != nil {
			return err
		}
		health := input.Health
		health.CheckedAt = checkedAt
		health, err = domainsandbox.NormalizeHealthSnapshot(health, provider.Scopes)
		if err != nil {
			return err
		}
		episode, err := episodePO.toDomain()
		if err != nil {
			return err
		}
		transition := domainsandbox.ApplyProviderHealthObservation(
			episode,
			domainsandbox.HealthIncidentObservation{
				ProviderID: provider.ID,
				ProviderStatus: provider.Status,
				HealthStatus: health.Status,
				CheckedAt: checkedAt,
				FailureThreshold: input.FailureThreshold,
			},
		)
		if !transition.Applied {
			return releaseStaleHealthMonitorClaim(
				tx,
				&episodePO,
				checkedAt,
				nextCheckAt,
			)
		}
		if err := persistMonitoredProviderHealth(tx, providerPO, health, checkedAt); err != nil {
			return err
		}
		if transition.Notification != domainsandbox.HealthIncidentNotificationNone {
			event, err := sandboxHealthNotificationEvent(provider.ID, transition)
			if err != nil {
				return err
			}
			if err := persistHealthNotificationProjection(
				tx,
				event,
				transition,
				checkedAt,
			); err != nil {
				return err
			}
		}
		transition.State.NextCheckAt = nextCheckAt
		transition.State.LeaseOwner = ""
		transition.State.LeaseToken = ""
		transition.State.LeaseExpiresAt = time.Time{}
		transition.State.UpdatedAt = checkedAt
		if err := persistHealthEpisodeTransition(
			tx,
			&episodePO,
			transition.State,
		); err != nil {
			return err
		}
		completion = domainsandbox.HealthMonitorCheckCompletion{
			Applied: true,
			Episode: transition.State,
			Notification: transition.Notification,
		}
		return nil
	})
	if err == nil && staleProviderVersion {
		return completion, domainsandbox.ErrVersionConflict
	}
	return completion, err
}

type healthNotificationProjectionClaim struct {
	EventID    string
	LeaseOwner string
	LeaseToken string
}

func (r *MySQLHealthMonitorRepository) ProjectPendingHealthNotifications(
	ctx context.Context,
	request domainsandbox.HealthNotificationProjectionRequest,
) (domainsandbox.HealthNotificationProjectionResult, error) {
	var result domainsandbox.HealthNotificationProjectionResult
	if r == nil || r.db == nil || r.outbox == nil || ctx == nil {
		return result, domainsandbox.ErrHealthMonitorOutbox
	}
	workerID := strings.TrimSpace(request.WorkerID)
	if workerID == "" ||
		len(workerID) > maxHealthMonitorWorkerIDBytes ||
		request.Now.IsZero() ||
		request.Lease <= 0 ||
		request.Limit < 1 ||
		request.Limit > domainsandbox.MaxHealthMonitorBatchSize {
		return result, domainsandbox.ErrInvalidInput
	}
	now, err := r.currentDatabaseTime(ctx, r.db, request.Now)
	if err != nil {
		return result, err
	}
	claims, err := r.claimHealthNotificationProjections(
		ctx,
		workerID,
		now,
		request.Lease,
		request.Limit,
	)
	if err != nil {
		return result, err
	}
	result.Claimed = len(claims)
	var combined error
	for _, claim := range claims {
		projected, projectErr := r.projectHealthNotification(ctx, claim, now)
		if errors.Is(projectErr, domainsandbox.ErrVersionConflict) {
			continue
		}
		if projectErr != nil {
			releaseErr := r.releaseHealthNotificationProjection(
				ctx,
				claim,
				now.Add(defaultHealthProjectionRetryDelay),
			)
			if releaseErr != nil {
				combined = errors.Join(combined, projectErr, domainsandbox.ErrUnavailable)
			} else {
				combined = errors.Join(combined, projectErr)
			}
			continue
		}
		if projected {
			result.Projected++
		}
	}
	return result, combined
}

func (r *MySQLHealthMonitorRepository) claimHealthNotificationProjections(
	ctx context.Context,
	workerID string,
	now time.Time,
	lease time.Duration,
	limit int,
) ([]healthNotificationProjectionClaim, error) {
	claims := make([]healthNotificationProjectionClaim, 0, limit)
	leaseExpiresAt := now.Add(lease).UTC().Truncate(time.Millisecond)
	if !leaseExpiresAt.After(now) {
		return nil, domainsandbox.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var eventIDs []string
		if err := tx.Model(&providerHealthNotificationProjectionPO{}).
			Select("event_id").
			Where("status = ? AND next_attempt_at <= ?",
				healthProjectionStatusPending, now.UnixMilli()).
			Where("(lease_expires_at = 0 OR lease_expires_at <= ?)", now.UnixMilli()).
			Where(`
				notification_type <> ?
				OR EXISTS (
					SELECT 1
					FROM sandbox_health_notification_projections AS unhealthy
					WHERE unhealthy.incident_id =
						sandbox_health_notification_projections.incident_id
					  AND unhealthy.notification_type = ?
					  AND unhealthy.status = ?
				)
			`,
				string(domainsandbox.HealthIncidentNotificationRecovered),
				string(domainsandbox.HealthIncidentNotificationUnhealthy),
				healthProjectionStatusProjected,
			).
			Order("next_attempt_at ASC, event_id ASC").
			Limit(limit).
			Find(&eventIDs).Error; err != nil {
			return err
		}
		for index, eventID := range eventIDs {
			token := strings.Join([]string{
				workerID,
				strconv.FormatInt(now.UnixNano(), 36),
				strconv.Itoa(index),
			}, ":")
			update := tx.Model(&providerHealthNotificationProjectionPO{}).
				Where("event_id = ? AND status = ? AND next_attempt_at <= ?",
					eventID, healthProjectionStatusPending, now.UnixMilli()).
				Where("(lease_expires_at = 0 OR lease_expires_at <= ?)", now.UnixMilli()).
				Updates(map[string]any{
					"lease_owner": workerID,
					"lease_token": token,
					"lease_expires_at": leaseExpiresAt.UnixMilli(),
					"updated_at": now.UnixMilli(),
					"version": gorm.Expr("version + 1"),
				})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 1 {
				claims = append(claims, healthNotificationProjectionClaim{
					EventID: eventID,
					LeaseOwner: workerID,
					LeaseToken: token,
				})
			}
		}
		return nil
	})
	return claims, err
}

func (r *MySQLHealthMonitorRepository) projectHealthNotification(
	ctx context.Context,
	claim healthNotificationProjectionClaim,
	fallbackNow time.Time,
) (bool, error) {
	projected := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var projection providerHealthNotificationProjectionPO
		query := withUpdateLock(tx.Where("event_id = ?", claim.EventID))
		if err := query.Take(&projection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domainsandbox.ErrVersionConflict
			}
			return err
		}
		if projection.Status == healthProjectionStatusProjected {
			return nil
		}
		now, err := r.currentDatabaseTime(ctx, tx, fallbackNow)
		if err != nil {
			return err
		}
		if projection.LeaseOwner != claim.LeaseOwner ||
			projection.LeaseToken != claim.LeaseToken ||
			projection.LeaseExpiresAt == 0 ||
			projection.LeaseExpiresAt <= now.UnixMilli() {
			return domainsandbox.ErrVersionConflict
		}
		providerID, err := positivePersistedUint64ToInt64(projection.ProviderID)
		if err != nil {
			return err
		}
		event, err := sandboxHealthNotificationEvent(
			providerID,
			domainsandbox.HealthIncidentTransition{
				Notification: domainsandbox.HealthIncidentNotification(
					projection.NotificationType,
				),
				IncidentID: projection.IncidentID,
				IncidentSequence: projection.IncidentSequence,
				OccurredAt: healthEpisodeTime(projection.OccurredAt),
			},
		)
		if err != nil {
			return err
		}
		if err := r.outbox.AppendInTransaction(ctx, tx, event); err != nil {
			return domainsandbox.ErrHealthMonitorOutbox
		}
		update := tx.Model(&providerHealthNotificationProjectionPO{}).
			Where("event_id = ? AND status = ? AND lease_token = ?",
				projection.EventID,
				healthProjectionStatusPending,
				claim.LeaseToken).
			Updates(map[string]any{
				"status": healthProjectionStatusProjected,
				"projected_at": now.UnixMilli(),
				"lease_owner": "",
				"lease_token": "",
				"lease_expires_at": 0,
				"updated_at": now.UnixMilli(),
				"version": gorm.Expr("version + 1"),
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return domainsandbox.ErrVersionConflict
		}
		projected = true
		return nil
	})
	return projected, err
}

func (r *MySQLHealthMonitorRepository) releaseHealthNotificationProjection(
	ctx context.Context,
	claim healthNotificationProjectionClaim,
	nextAttemptAt time.Time,
) error {
	result := r.db.WithContext(ctx).
		Model(&providerHealthNotificationProjectionPO{}).
		Where("event_id = ? AND status = ? AND lease_token = ?",
			claim.EventID,
			healthProjectionStatusPending,
			claim.LeaseToken).
		Updates(map[string]any{
			"attempt_count": gorm.Expr("attempt_count + 1"),
			"next_attempt_at": nextAttemptAt.UTC().Truncate(time.Millisecond).UnixMilli(),
			"lease_owner": "",
			"lease_token": "",
			"lease_expires_at": 0,
			"updated_at": nextAttemptAt.UTC().Truncate(time.Millisecond).UnixMilli(),
			"version": gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return domainsandbox.ErrUnavailable
	}
	if result.RowsAffected != 1 {
		return domainsandbox.ErrVersionConflict
	}
	return nil
}

func persistHealthNotificationProjection(
	tx *gorm.DB,
	event domainnotification.Event,
	transition domainsandbox.HealthIncidentTransition,
	now time.Time,
) error {
	providerID, err := positiveDomainInt64ToUint64(transition.State.ProviderID)
	if err != nil {
		return err
	}
	row := providerHealthNotificationProjectionPO{
		EventID: event.EventID,
		ProviderID: providerID,
		IncidentID: transition.IncidentID,
		IncidentSequence: transition.IncidentSequence,
		NotificationType: string(transition.Notification),
		OccurredAt: event.OccurredAt.UnixMilli(),
		Status: healthProjectionStatusPending,
		NextAttemptAt: now.UnixMilli(),
		Version: domainsandbox.InitialVersion,
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing providerHealthNotificationProjectionPO
	if err := tx.Where("event_id = ?", row.EventID).Take(&existing).Error; err != nil {
		return err
	}
	if existing.ProviderID != row.ProviderID ||
		existing.IncidentID != row.IncidentID ||
		existing.IncidentSequence != row.IncidentSequence ||
		existing.NotificationType != row.NotificationType ||
		existing.OccurredAt != row.OccurredAt {
		return domainsandbox.ErrVersionConflict
	}
	return nil
}

func (r *MySQLHealthMonitorRepository) currentDatabaseTime(
	ctx context.Context,
	tx *gorm.DB,
	fallback time.Time,
) (time.Time, error) {
	if r != nil && r.databaseClock != nil {
		now, err := r.databaseClock(ctx, tx, fallback)
		if err != nil {
			return time.Time{}, domainsandbox.ErrHealthMonitorDBClock
		}
		now = now.UTC().Truncate(time.Millisecond)
		if now.IsZero() {
			return time.Time{}, domainsandbox.ErrHealthMonitorDBClock
		}
		return now, nil
	}
	if tx == nil || tx.Dialector == nil {
		return time.Time{}, domainsandbox.ErrHealthMonitorDBClock
	}

	// SQLite is used only by repository tests, where the injected/fallback
	// logical clock keeps deterministic cases independent of wall time.
	if tx.Dialector.Name() == "sqlite" {
		now := fallback.UTC().Truncate(time.Millisecond)
		if now.IsZero() {
			return time.Time{}, domainsandbox.ErrHealthMonitorDBClock
		}
		return now, nil
	}

	// Production MySQL compares every worker against one UTC-naive database
	// clock. TIMESTAMPDIFF avoids session timezone conversion on either side.
	var unixMillis int64
	err := tx.WithContext(ctx).Raw(
		"SELECT TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', UTC_TIMESTAMP(3)) DIV 1000",
	).Scan(&unixMillis).Error
	if err != nil || unixMillis <= 0 {
		return time.Time{}, domainsandbox.ErrHealthMonitorDBClock
	}
	return time.UnixMilli(unixMillis).UTC(), nil
}

func ensureHealthEpisodeRows(
	tx *gorm.DB,
	now time.Time,
	limit int,
) error {
	var providerIDs []uint64
	if err := tx.Model(&providerPO{}).
		Select("id").
		Where("status = ? AND deleted_at IS NULL", string(domainsandbox.ProviderStatusEnabled)).
		Where("NOT EXISTS (SELECT 1 FROM sandbox_provider_health_episodes AS episodes WHERE episodes.provider_id = sandbox_providers.id)").
		Order("id ASC").
		Limit(limit).
		Find(&providerIDs).Error; err != nil {
		return err
	}
	for _, providerID := range providerIDs {
		row := providerHealthEpisodePO{
			ProviderID: providerID,
			IncidentStatus: string(domainsandbox.HealthIncidentStatusNone),
			Version: domainsandbox.InitialVersion,
			CreatedAt: now.UnixMilli(),
			UpdatedAt: now.UnixMilli(),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (po providerHealthEpisodePO) toDomain() (domainsandbox.ProviderHealthEpisode, error) {
	providerID, err := positivePersistedUint64ToInt64(po.ProviderID)
	if err != nil {
		return domainsandbox.ProviderHealthEpisode{}, err
	}
	return domainsandbox.ProviderHealthEpisode{
		ProviderID: providerID,
		ConsecutiveFailures: po.ConsecutiveFailures,
		FailureStartedAt: healthEpisodeTime(po.FailureStartedAt),
		IncidentSequence: po.IncidentSequence,
		IncidentID: po.IncidentID,
		IncidentStatus: domainsandbox.HealthIncidentStatus(po.IncidentStatus),
		IncidentOpenedAt: healthEpisodeTime(po.IncidentOpenedAt),
		IncidentNotifiedAt: healthEpisodeTime(po.IncidentNotifiedAt),
		IncidentClosedAt: healthEpisodeTime(po.IncidentClosedAt),
		RecoveryNotifiedAt: healthEpisodeTime(po.RecoveryNotifiedAt),
		LastRecoveredAt: healthEpisodeTime(po.LastRecoveredAt),
		LastCheckedAt: healthEpisodeTime(po.LastCheckedAt),
		NextCheckAt: healthEpisodeTime(po.NextCheckAt),
		LeaseOwner: po.LeaseOwner,
		LeaseToken: po.LeaseToken,
		LeaseExpiresAt: healthEpisodeTime(po.LeaseExpiresAt),
		Version: po.Version,
		CreatedAt: healthEpisodeTime(po.CreatedAt),
		UpdatedAt: healthEpisodeTime(po.UpdatedAt),
	}, nil
}

func persistMonitoredProviderHealth(
	tx *gorm.DB,
	provider *providerPO,
	health domainsandbox.HealthSnapshot,
	checkedAt time.Time,
) error {
	latencyMillis, err := nonNegativeDomainInt64ToUint32(health.LatencyMillis)
	if err != nil {
		return domainsandbox.ErrInvalidInput
	}
	capabilitiesJSON, err := marshalScopes(health.Capabilities)
	if err != nil {
		return err
	}
	result := tx.Model(&providerPO{}).
		Where("id = ? AND version = ? AND status = ? AND deleted_at IS NULL",
			provider.ID,
			provider.Version,
			string(domainsandbox.ProviderStatusEnabled)).
		Updates(map[string]any{
			"health_status": string(health.Status),
			"last_health_capabilities_json": capabilitiesJSON,
			"last_health_code": health.ReasonCode,
			"last_health_message": health.Message,
			"last_health_latency_ms": latencyMillis,
			"last_health_at": checkedAt,
			"updated_at": checkedAt,
			"version": gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainsandbox.ErrVersionConflict
	}
	return nil
}

func persistHealthEpisodeTransition(
	tx *gorm.DB,
	current *providerHealthEpisodePO,
	state domainsandbox.ProviderHealthEpisode,
) error {
	result := tx.Model(&providerHealthEpisodePO{}).
		Where("provider_id = ? AND lease_token = ?", current.ProviderID, current.LeaseToken).
		Updates(map[string]any{
			"consecutive_failures": state.ConsecutiveFailures,
			"failure_started_at": healthEpisodeMillis(state.FailureStartedAt),
			"incident_sequence": state.IncidentSequence,
			"incident_id": state.IncidentID,
			"incident_status": string(state.IncidentStatus),
			"incident_opened_at": healthEpisodeMillis(state.IncidentOpenedAt),
			"incident_notified_at": healthEpisodeMillis(state.IncidentNotifiedAt),
			"incident_closed_at": healthEpisodeMillis(state.IncidentClosedAt),
			"recovery_notified_at": healthEpisodeMillis(state.RecoveryNotifiedAt),
			"last_recovered_at": healthEpisodeMillis(state.LastRecoveredAt),
			"last_checked_at": healthEpisodeMillis(state.LastCheckedAt),
			"next_check_at": healthEpisodeMillis(state.NextCheckAt),
			"lease_owner": "",
			"lease_token": "",
			"lease_expires_at": 0,
			"updated_at": healthEpisodeMillis(state.UpdatedAt),
			"version": gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainsandbox.ErrVersionConflict
	}
	return nil
}

func closeClaimForDisabledProvider(
	tx *gorm.DB,
	current *providerHealthEpisodePO,
	checkedAt time.Time,
	nextCheckAt time.Time,
	completion *domainsandbox.HealthMonitorCheckCompletion,
) error {
	state, err := current.toDomain()
	if err != nil {
		return err
	}
	transition := domainsandbox.ApplyProviderHealthObservation(
		state,
		domainsandbox.HealthIncidentObservation{
			ProviderID: state.ProviderID,
			ProviderStatus: domainsandbox.ProviderStatusDisabled,
			HealthStatus: domainsandbox.HealthStatusUnknown,
			CheckedAt: checkedAt,
			FailureThreshold: domainsandbox.DefaultHealthIncidentFailureThreshold,
		},
	)
	if !transition.Applied {
		return releaseStaleHealthMonitorClaim(tx, current, checkedAt, nextCheckAt)
	}
	transition.State.NextCheckAt = time.Time{}
	transition.State.LeaseOwner = ""
	transition.State.LeaseToken = ""
	transition.State.LeaseExpiresAt = time.Time{}
	transition.State.UpdatedAt = checkedAt
	if err := persistHealthEpisodeTransition(tx, current, transition.State); err != nil {
		return err
	}
	if completion != nil {
		*completion = domainsandbox.HealthMonitorCheckCompletion{
			Applied: true,
			Episode: transition.State,
		}
	}
	return nil
}

func releaseStaleHealthMonitorClaim(
	tx *gorm.DB,
	current *providerHealthEpisodePO,
	checkedAt time.Time,
	nextCheckAt time.Time,
) error {
	return releaseHealthMonitorClaim(
		tx,
		current.ProviderID,
		current.LeaseToken,
		checkedAt.UnixMilli(),
		nextCheckAt.UnixMilli(),
	)
}

func releaseHealthMonitorClaim(
	tx *gorm.DB,
	providerID uint64,
	leaseToken string,
	updatedAt int64,
	nextCheckAt int64,
) error {
	result := tx.Model(&providerHealthEpisodePO{}).
		Where("provider_id = ? AND lease_token = ?", providerID, leaseToken).
		Updates(map[string]any{
			"next_check_at": nextCheckAt,
			"lease_owner": "",
			"lease_token": "",
			"lease_expires_at": 0,
			"updated_at": updatedAt,
			"version": gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainsandbox.ErrVersionConflict
	}
	return nil
}

func sandboxHealthNotificationEvent(
	providerID int64,
	transition domainsandbox.HealthIncidentTransition,
) (domainnotification.Event, error) {
	if providerID <= 0 ||
		transition.IncidentSequence == 0 ||
		strings.TrimSpace(transition.IncidentID) == "" ||
		transition.OccurredAt.IsZero() {
		return domainnotification.Event{}, domainsandbox.ErrInvalidInput
	}
	eventType := domainnotification.EventSystemProviderUnavailable
	suffix := "unhealthy"
	aggregateVersion := int64(1)
	payload := domainnotification.EventPayload{
		ResourceDisplayName: fmt.Sprintf("Sandbox provider %d", providerID),
		StatusReasonCode: domainnotification.StatusReasonProviderUnavailable,
	}
	switch transition.Notification {
	case domainsandbox.HealthIncidentNotificationUnhealthy:
	case domainsandbox.HealthIncidentNotificationRecovered:
		eventType = domainnotification.EventSystemProviderRecovered
		suffix = "recovered"
		aggregateVersion = 2
		payload.StatusReasonCode = domainnotification.StatusReasonNone
	default:
		return domainnotification.Event{}, domainsandbox.ErrInvalidInput
	}
	event := domainnotification.Event{
		EventID: fmt.Sprintf(
			"sandbox-health:%d:%d:%s",
			providerID,
			transition.IncidentSequence,
			suffix,
		),
		EventType: eventType,
		AggregateType: "sandbox_provider_health_incident",
		AggregateID: transition.IncidentID,
		AggregateVersion: aggregateVersion,
		OccurredAt: transition.OccurredAt.UTC(),
		ActorID: 0,
		SpaceID: 0,
		RecipientPolicy: domainnotification.RecipientSystemAdmins,
		PayloadSchema: domainnotification.CurrentPayloadSchema,
		Payload: payload,
	}
	return domainnotification.CanonicalizeEvent(event)
}

func healthMonitorLeaseToken(workerID string, providerID uint64, now time.Time) string {
	return strings.Join([]string{
		workerID,
		strconv.FormatUint(providerID, 36),
		strconv.FormatInt(now.UnixNano(), 36),
	}, ":")
}

func healthEpisodeMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().Truncate(time.Millisecond).UnixMilli()
}

func healthEpisodeTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

var _ domainsandbox.HealthMonitorRepository = (*MySQLHealthMonitorRepository)(nil)
