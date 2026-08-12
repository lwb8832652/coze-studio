// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
	mysqldriver "github.com/go-sql-driver/mysql"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	providerExecutionDefaultRetryAttempts = 6
	providerExecutionRetryBaseDelay       = 2 * time.Millisecond
	providerExecutionRetryMaxDelay        = 100 * time.Millisecond
)

type ProviderExecutionRepository struct {
	db                           *gorm.DB
	clock                        providerExecutionDBClock
	retry                        providerExecutionRetryPolicy
	startSubmissionCommittedHook func()
}

type providerExecutionDBClock interface {
	nowExpression() clause.Expr
	addExpression(time.Duration) clause.Expr
	read(context.Context, *gorm.DB) (time.Time, error)
}

type providerExecutionMySQLDBClock struct{}

func (providerExecutionMySQLDBClock) nowExpression() clause.Expr {
	return gorm.Expr("UTC_TIMESTAMP(6)")
}

func (providerExecutionMySQLDBClock) addExpression(duration time.Duration) clause.Expr {
	return gorm.Expr("TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6))", duration.Microseconds())
}

func (providerExecutionMySQLDBClock) read(ctx context.Context, db *gorm.DB) (time.Time, error) {
	var value time.Time
	if err := db.WithContext(ctx).Raw("SELECT UTC_TIMESTAMP(6)").Scan(&value).Error; err != nil {
		return time.Time{}, err
	}
	return value.UTC(), nil
}

type providerExecutionSQLiteDBClock struct{}

func (providerExecutionSQLiteDBClock) nowExpression() clause.Expr {
	return gorm.Expr("CURRENT_TIMESTAMP")
}

func (providerExecutionSQLiteDBClock) addExpression(duration time.Duration) clause.Expr {
	return gorm.Expr("DATETIME(CURRENT_TIMESTAMP, ?)", fmt.Sprintf("%+.6f seconds", duration.Seconds()))
}

func (providerExecutionSQLiteDBClock) read(ctx context.Context, db *gorm.DB) (time.Time, error) {
	var value string
	if err := db.WithContext(ctx).Raw("SELECT STRFTIME('%Y-%m-%dT%H:%M:%fZ', 'now')").Row().Scan(&value); err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

type providerExecutionRetryPolicy struct {
	attempts int
	delay    func(int) time.Duration
	wait     func(context.Context, time.Duration) error
}

func newProviderExecutionRetryPolicy() providerExecutionRetryPolicy {
	return providerExecutionRetryPolicy{
		attempts: providerExecutionDefaultRetryAttempts,
		delay: func(attempt int) time.Duration {
			base := providerExecutionRetryBaseDelay << min(attempt, 5)
			if base > providerExecutionRetryMaxDelay {
				base = providerExecutionRetryMaxDelay
			}
			maximumJitter := max(base/2, time.Nanosecond)
			value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(maximumJitter)))
			if err != nil {
				return base
			}
			return base + time.Duration(value.Int64())
		},
		wait: func(ctx context.Context, duration time.Duration) error {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type providerExecutionRecord struct {
	ID                           string                                       `gorm:"column:id;type:varchar(64);primaryKey;index:idx_appdev_provider_exec_recoverable,priority:7"`
	SpaceID                      int64                                        `gorm:"column:space_id;type:bigint unsigned;not null;uniqueIndex:uk_appdev_provider_exec_generation,priority:1;uniqueIndex:uk_appdev_provider_exec_idempotency,priority:1;index:idx_appdev_provider_exec_recoverable,priority:1;index:idx_appdev_provider_exec_provider_id,priority:1;index:idx_appdev_provider_exec_launch,priority:1"`
	ProjectID                    string                                       `gorm:"column:project_id;type:varchar(64);not null;uniqueIndex:uk_appdev_provider_exec_generation,priority:2;uniqueIndex:uk_appdev_provider_exec_idempotency,priority:2;index:idx_appdev_provider_exec_recoverable,priority:2;index:idx_appdev_provider_exec_provider_id,priority:2;index:idx_appdev_provider_exec_launch,priority:2"`
	ActorUserID                  int64                                        `gorm:"column:actor_user_id;type:bigint unsigned;not null;default:0"`
	Generation                   uint64                                       `gorm:"column:generation;type:bigint unsigned;not null;uniqueIndex:uk_appdev_provider_exec_generation,priority:3"`
	IdempotencyKey               string                                       `gorm:"column:idempotency_key;type:varbinary(128);not null;uniqueIndex:uk_appdev_provider_exec_idempotency,priority:3"`
	DesiredState                 domainappdev.ProviderExecutionDesiredState   `gorm:"column:desired_state;type:varchar(32);not null;index:idx_appdev_provider_exec_recoverable,priority:4"`
	ObservedState                domainappdev.ProviderExecutionObservedState  `gorm:"column:observed_state;type:varchar(32);not null;index:idx_appdev_provider_exec_recoverable,priority:3"`
	ProviderKey                  string                                       `gorm:"column:provider_key;type:varchar(64);not null;index:idx_appdev_provider_exec_provider_id,priority:3"`
	ProviderScope                domainsandbox.Scope                          `gorm:"column:provider_scope;type:varchar(32);not null"`
	ProviderExecutionID          string                                       `gorm:"column:provider_execution_id;type:varbinary(128);not null;index:idx_appdev_provider_exec_provider_id,priority:4"`
	SubmissionStartedAt          *time.Time                                   `gorm:"column:submission_started_at;type:datetime(6)"`
	LaunchState                  domainappdev.ProviderExecutionLaunchState    `gorm:"column:launch_state;type:varchar(24);not null;default:none;index:idx_appdev_provider_exec_launch,priority:3"`
	LaunchOperationHash          []byte                                       `gorm:"column:launch_operation_hash;type:binary(32)"`
	LaunchProviderOperationID    string                                       `gorm:"column:launch_provider_operation_id;type:varbinary(128);not null;default:''"`
	LaunchRequestDigest          []byte                                       `gorm:"column:launch_request_digest;type:binary(32)"`
	LaunchExpiresAt              *time.Time                                   `gorm:"column:launch_expires_at;type:datetime(6);index:idx_appdev_provider_exec_launch,priority:4"`
	CheckpointEnvelope           string                                       `gorm:"column:checkpoint_envelope;type:mediumtext;not null;default:''"`
	CheckpointWriteRevision      uint64                                       `gorm:"column:checkpoint_write_revision;type:bigint unsigned;not null;default:0"`
	CheckpointWritePending       bool                                         `gorm:"column:checkpoint_write_pending;type:tinyint(1);not null;default:0"`
	CheckpointWriteOperationHash []byte                                       `gorm:"column:checkpoint_write_operation_hash;type:binary(32)"`
	CheckpointWriteExpiresAt     *time.Time                                   `gorm:"column:checkpoint_write_expires_at;type:datetime(6)"`
	CheckpointLastOperationHash  []byte                                       `gorm:"column:checkpoint_last_operation_hash;type:binary(32)"`
	CleanupOperationHash         []byte                                       `gorm:"column:cleanup_operation_hash;type:binary(32)"`
	TerminalOperationHash        []byte                                       `gorm:"column:terminal_operation_hash;type:binary(32)"`
	ReleaseOwnerOperationHash    []byte                                       `gorm:"column:release_owner_operation_hash;type:binary(32)"`
	ProviderLeaseExpiresAt       *time.Time                                   `gorm:"column:provider_lease_expires_at;type:datetime(6)"`
	OwnerIdentityHash            []byte                                       `gorm:"column:owner_identity_hash;type:binary(32)"`
	OwnerEpoch                   uint64                                       `gorm:"column:owner_epoch;type:bigint unsigned;not null;default:0"`
	OwnerExpiresAt               *time.Time                                   `gorm:"column:owner_expires_at;type:datetime(6);index:idx_appdev_provider_exec_recoverable,priority:5"`
	PreviewRoute                 string                                       `gorm:"column:preview_route;type:varchar(512);not null;default:''"`
	ArtifactObjectKey            string                                       `gorm:"column:artifact_object_key;type:varchar(512);not null;default:''"`
	BuildOperationID             string                                       `gorm:"column:build_operation_id;type:varbinary(128);not null;default:''"`
	BuildOperationHash           []byte                                       `gorm:"column:build_operation_hash;type:binary(32)"`
	ArtifactStatus               domainappdev.ProviderExecutionArtifactStatus `gorm:"column:artifact_status;type:varchar(32);not null;default:none"`
	ArtifactKind                 domainappdev.ProviderExecutionArtifactKind   `gorm:"column:artifact_kind;type:varchar(64);not null;default:''"`
	ArtifactDigest               string                                       `gorm:"column:artifact_digest;type:varchar(71);not null;default:''"`
	ArtifactSize                 int64                                        `gorm:"column:artifact_size;type:bigint unsigned;not null;default:0"`
	ArtifactVersion              uint64                                       `gorm:"column:artifact_version;type:bigint unsigned;not null;default:0"`
	BuildStartedAt               *time.Time                                   `gorm:"column:build_started_at;type:datetime(6)"`
	ArtifactUpdatedAt            *time.Time                                   `gorm:"column:artifact_updated_at;type:datetime(6)"`
	ArtifactSafeErrorCode        string                                       `gorm:"column:artifact_safe_error_code;type:varchar(64);not null;default:''"`
	ArtifactSafeErrorMessage     string                                       `gorm:"column:artifact_safe_error_message;type:varchar(255);not null;default:''"`
	SafeErrorCode                string                                       `gorm:"column:safe_error_code;type:varchar(64);not null;default:''"`
	SafeErrorMessage             string                                       `gorm:"column:safe_error_message;type:varchar(255);not null;default:''"`
	Version                      uint64                                       `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	CreatedAt                    time.Time                                    `gorm:"column:created_at;type:datetime(6);not null"`
	UpdatedAt                    time.Time                                    `gorm:"column:updated_at;type:datetime(6);not null;index:idx_appdev_provider_exec_recoverable,priority:6"`
}

func (providerExecutionRecord) TableName() string { return "appdev_provider_executions" }

func (providerExecutionRecord) String() string {
	return "providerExecutionRecord{checkpoint:<redacted> owner:<redacted> build_operation:<redacted> artifact_object_key:<redacted>}"
}

func (providerExecutionRecord) GoString() string {
	return "providerExecutionRecord{checkpoint:<redacted> owner:<redacted> build_operation:<redacted> artifact_object_key:<redacted>}"
}

type providerExecutionProjectLock struct {
	ID           string `gorm:"column:id"`
	SpaceID      int64  `gorm:"column:space_id"`
	Status       string `gorm:"column:status"`
	ArchiveState string `gorm:"column:archive_state"`
}

func (providerExecutionProjectLock) TableName() string { return "appdev_projects" }

func NewProviderExecutionRepository(db *gorm.DB) *ProviderExecutionRepository {
	clock := providerExecutionDBClock(providerExecutionSQLiteDBClock{})
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "mysql" {
		clock = providerExecutionMySQLDBClock{}
	}
	return &ProviderExecutionRepository{db: db, clock: clock, retry: newProviderExecutionRetryPolicy()}
}

func (r *ProviderExecutionRepository) EnsureStart(ctx context.Context, input domainappdev.EnsureProviderExecutionStartInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionStart(input)
	if err != nil || r == nil || r.db == nil || r.clock == nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	var result *domainappdev.ProviderExecution
	err = runProviderExecutionRetry(ctx, r.retry, func() error {
		var attemptErr error
		result, attemptErr = r.ensureStartAttempt(ctx, spaceID, input)
		return attemptErr
	})
	if err == nil {
		return result, nil
	}
	if providerExecutionDuplicate(err) {
		if existing, loadErr := r.loadByIdempotency(ctx, spaceID, input.ProjectID, input.IdempotencyKey); loadErr == nil {
			return existing, nil
		}
	}
	return nil, normalizeProviderExecutionDatabaseError(err)
}

func (r *ProviderExecutionRepository) ensureStartAttempt(ctx context.Context, spaceID int64, input domainappdev.EnsureProviderExecutionStartInput) (*domainappdev.ProviderExecution, error) {
	var result *domainappdev.ProviderExecution
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project providerExecutionProjectLock
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("space_id = ? AND id = ?", spaceID, input.ProjectID).Take(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domainappdev.ErrProviderExecutionGenerationConflict
			}
			return err
		}
		if project.Status == string(domainappdev.ProjectStatusArchived) ||
			normalizedProjectArchiveState(project.ArchiveState) != domainappdev.ProjectArchiveStateNone {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		var existing providerExecutionRecord
		if err := tx.Where("space_id = ? AND project_id = ? AND idempotency_key = ?", spaceID, input.ProjectID, input.IdempotencyKey).Take(&existing).Error; err == nil {
			var convertErr error
			result, convertErr = providerExecutionRecordToDomain(&existing)
			return convertErr
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.RequireNoActive {
			var activeCount int64
			if err := tx.Model(&providerExecutionRecord{}).
				Where("space_id = ? AND project_id = ? AND observed_state <> ?", spaceID, input.ProjectID, domainappdev.ProviderExecutionObservedCleanupComplete).
				Count(&activeCount).Error; err != nil {
				return err
			}
			if activeCount != 0 {
				return domainappdev.ErrProviderExecutionStateConflict
			}
		}
		var maximum sql.NullInt64
		if err := tx.Model(&providerExecutionRecord{}).Select("MAX(generation)").Where("space_id = ? AND project_id = ?", spaceID, input.ProjectID).Scan(&maximum).Error; err != nil {
			return err
		}
		generation := uint64(1)
		if maximum.Valid {
			generation = uint64(maximum.Int64) + 1
		}
		values := map[string]any{
			"id": input.ID, "space_id": spaceID, "project_id": input.ProjectID, "actor_user_id": input.ActorUserID, "generation": generation,
			"idempotency_key": input.IdempotencyKey, "desired_state": domainappdev.ProviderExecutionDesiredRun,
			"observed_state": domainappdev.ProviderExecutionObservedPending, "provider_key": input.ProviderKey,
			"provider_scope": input.ProviderScope, "provider_execution_id": "", "checkpoint_envelope": "",
			"preview_route": "", "artifact_object_key": "", "safe_error_code": "", "safe_error_message": "",
			"version": domainappdev.ProviderExecutionInitialVersion, "created_at": r.clock.nowExpression(), "updated_at": r.clock.nowExpression(),
		}
		if err := tx.Model(&providerExecutionRecord{}).Create(values).Error; err != nil {
			return err
		}
		var created providerExecutionRecord
		if err := tx.Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.ProjectID, generation).Take(&created).Error; err != nil {
			return err
		}
		var convertErr error
		result, convertErr = providerExecutionRecordToDomain(&created)
		return convertErr
	})
	return result, err
}

func (r *ProviderExecutionRepository) Claim(ctx context.Context, input domainappdev.ClaimProviderExecutionInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionClaim(input)
	if err != nil || r == nil || r.db == nil || r.clock == nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	var claimed *domainappdev.ProviderExecution
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before providerExecutionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.ProjectID, input.Generation).
			Take(&before).Error; err != nil {
			return normalizeProviderExecutionLookupError(err)
		}
		if _, err := providerExecutionRecordToDomain(&before); err != nil {
			return err
		}
		if !domainappdev.IsProviderExecutionRecoverableObservedState(before.ObservedState) {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		if before.Version != input.ExpectedVersion {
			return domainappdev.ErrProviderExecutionVersionConflict
		}
		dbNow, err := r.clock.read(ctx, tx)
		if err != nil {
			return err
		}
		ownerAvailable := len(before.OwnerIdentityHash) == 0 || before.OwnerExpiresAt == nil || !before.OwnerExpiresAt.After(dbNow)
		sameOwner := input.ExpectedOwnerEpoch > 0 && input.OwnerHash.EqualBytes(before.OwnerIdentityHash) && before.OwnerEpoch == input.ExpectedOwnerEpoch && before.OwnerExpiresAt != nil && before.OwnerExpiresAt.After(dbNow)
		if !ownerAvailable && !sameOwner {
			return domainappdev.ErrProviderExecutionOwnerConflict
		}

		hash := input.OwnerHash.Bytes()
		ownershipSQL := "(owner_identity_hash IS NULL OR owner_expires_at IS NULL OR owner_expires_at <= ?)"
		ownershipArgs := []any{r.clock.nowExpression()}
		epochExpression := gorm.Expr("owner_epoch + 1")
		if input.ExpectedOwnerEpoch > 0 {
			ownershipSQL = "(owner_identity_hash IS NULL OR owner_expires_at IS NULL OR owner_expires_at <= ? OR (owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ?))"
			ownershipArgs = []any{r.clock.nowExpression(), hash, input.ExpectedOwnerEpoch, r.clock.nowExpression()}
			epochExpression = gorm.Expr("CASE WHEN owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ? THEN owner_epoch ELSE owner_epoch + 1 END", hash, input.ExpectedOwnerEpoch, r.clock.nowExpression())
		}
		result := tx.Model(&providerExecutionRecord{}).
			Where("space_id = ? AND project_id = ? AND generation = ? AND version = ?", spaceID, input.ProjectID, input.Generation, input.ExpectedVersion).
			Where("observed_state IN ?", domainappdev.ProviderExecutionRecoverableObservedStates()).
			Where(ownershipSQL, ownershipArgs...).
			Updates(map[string]any{
				"owner_identity_hash": hash, "owner_epoch": epochExpression,
				"owner_expires_at": r.clock.addExpression(input.LeaseDuration), "version": gorm.Expr("version + 1"),
				"release_owner_operation_hash": nil, "updated_at": r.clock.nowExpression(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderExecutionConflict
		}
		var after providerExecutionRecord
		if err := tx.Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.ProjectID, input.Generation).Take(&after).Error; err != nil {
			return err
		}
		claimed, err = providerExecutionRecordToDomain(&after)
		return err
	})
	if err != nil {
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	return claimed, nil
}

func (r *ProviderExecutionRepository) RenewOwner(ctx context.Context, input domainappdev.RenewProviderExecutionOwnerInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || !validProviderExecutionLeaseDuration(input.LeaseDuration) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Updates(map[string]any{"owner_expires_at": r.clock.addExpression(input.LeaseDuration), "updated_at": r.clock.nowExpression()})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, providerExecutionConflictOr(r.classifyOwnedConflict(ctx, spaceID, input.OwnerCAS), domainappdev.ErrProviderExecutionConflict)
	}
	return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
}

func (r *ProviderExecutionRepository) ReleaseOwner(ctx context.Context, input domainappdev.ReleaseProviderExecutionOwnerInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).Updates(map[string]any{
		"release_owner_operation_hash": input.OperationHash.Bytes(), "owner_identity_hash": nil, "owner_expires_at": nil,
		"version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
	})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if len(record.OwnerIdentityHash) == 0 && record.OwnerExpiresAt == nil && record.Version == input.OwnerCAS.ExpectedVersion+1 &&
		record.OwnerEpoch == input.OwnerCAS.OwnerEpoch && record.ProviderKey == input.OwnerCAS.ProviderKey &&
		record.ProviderScope == input.OwnerCAS.ProviderScope && input.OperationHash.EqualBytes(record.ReleaseOwnerOperationHash) {
		return providerExecutionRecordToDomain(record)
	}
	if len(record.OwnerIdentityHash) == 0 && record.OwnerExpiresAt == nil && record.OwnerEpoch == input.OwnerCAS.OwnerEpoch {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionConflict)
}

func (r *ProviderExecutionRepository) StartSubmission(ctx context.Context, input domainappdev.StartProviderSubmissionInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() || input.RequestDigest.IsZero() ||
		!validProviderExecutionLeaseDuration(input.LeaseDuration) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if _, err := domainappdev.HashProviderExecutionOperationID(input.ProviderOperationID); err != nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	committed := false
	err = runProviderExecutionRetry(ctx, r.retry, func() error {
		committed = false
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var project providerExecutionProjectLock
			if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("space_id = ? AND id = ?", spaceID, input.OwnerCAS.ProjectID).
				Take(&project).Error; lockErr != nil {
				if errors.Is(lockErr, gorm.ErrRecordNotFound) {
					return domainappdev.ErrProviderExecutionGenerationConflict
				}
				return lockErr
			}
			if project.Status == string(domainappdev.ProjectStatusArchived) ||
				normalizedProjectArchiveState(project.ArchiveState) != domainappdev.ProjectArchiveStateNone {
				return domainappdev.ErrProviderExecutionStateConflict
			}
			transactionRepository := *r
			transactionRepository.db = tx
			result := transactionRepository.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
				Where(
					"desired_state = ? AND observed_state = ? AND submission_started_at IS NULL AND provider_execution_id = '' AND launch_state IN ?",
					domainappdev.ProviderExecutionDesiredRun,
					domainappdev.ProviderExecutionObservedPending,
					[]domainappdev.ProviderExecutionLaunchState{
						domainappdev.ProviderExecutionLaunchNone,
						domainappdev.ProviderExecutionLaunchAborted,
					},
				).
				Updates(map[string]any{
					"launch_state":                 domainappdev.ProviderExecutionLaunchPrepared,
					"launch_operation_hash":        input.OperationHash.Bytes(),
					"launch_provider_operation_id": input.ProviderOperationID,
					"launch_request_digest":        input.RequestDigest.Bytes(),
					"launch_expires_at":            r.clock.addExpression(input.LeaseDuration),
					"version":                      gorm.Expr("version + 1"),
					"updated_at":                   r.clock.nowExpression(),
				})
			if result.Error != nil {
				return normalizeProviderExecutionDatabaseError(result.Error)
			}
			if result.RowsAffected == 1 {
				committed = true
				return nil
			}
			record, loadErr := transactionRepository.loadRecordByGeneration(
				ctx,
				spaceID,
				input.OwnerCAS.ProjectID,
				input.OwnerCAS.Generation,
			)
			if loadErr != nil {
				return loadErr
			}
			if (record.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
				record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted ||
				record.LaunchState == domainappdev.ProviderExecutionLaunchComplete) &&
				input.OperationHash.EqualBytes(record.LaunchOperationHash) &&
				record.LaunchProviderOperationID == input.ProviderOperationID &&
				input.RequestDigest.EqualBytes(record.LaunchRequestDigest) {
				if ownerErr := transactionRepository.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr != nil {
					return ownerErr
				}
				if record.LaunchState != domainappdev.ProviderExecutionLaunchPrepared {
					return nil
				}
				renew := tx.Model(&providerExecutionRecord{}).
					Where(
						"space_id = ? AND project_id = ? AND generation = ? AND provider_key = ? AND provider_scope = ? AND owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ? AND launch_state = ? AND launch_operation_hash = ?",
						spaceID,
						input.OwnerCAS.ProjectID,
						input.OwnerCAS.Generation,
						input.OwnerCAS.ProviderKey,
						input.OwnerCAS.ProviderScope,
						input.OwnerCAS.OwnerHash.Bytes(),
						input.OwnerCAS.OwnerEpoch,
						r.clock.nowExpression(),
						domainappdev.ProviderExecutionLaunchPrepared,
						input.OperationHash.Bytes(),
					).
					Updates(map[string]any{
						"launch_expires_at": r.clock.addExpression(input.LeaseDuration),
						"updated_at":        r.clock.nowExpression(),
					})
				if renew.Error != nil {
					return normalizeProviderExecutionDatabaseError(renew.Error)
				}
				if renew.RowsAffected != 1 {
					return domainappdev.ErrProviderExecutionConflict
				}
				return nil
			}
			if record.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
				record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted ||
				record.LaunchState == domainappdev.ProviderExecutionLaunchComplete {
				return domainappdev.ErrProviderExecutionOperationConflict
			}
			return providerExecutionConflictOr(
				transactionRepository.classifyOwnedRecord(ctx, record, input.OwnerCAS),
				domainappdev.ErrProviderExecutionStateConflict,
			)
		})
	})
	if err != nil {
		return nil, err
	}
	if committed && r.startSubmissionCommittedHook != nil {
		r.startSubmissionCommittedHook()
	}
	record, err := r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *ProviderExecutionRepository) MarkSubmissionSubmitted(ctx context.Context, input domainappdev.MarkProviderLaunchSubmittedInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() ||
		!validProviderExecutionLeaseDuration(input.DispatchLeaseDuration) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where(
			"desired_state = ? AND observed_state = ? AND submission_started_at IS NULL AND provider_execution_id = '' AND launch_state = ? AND launch_operation_hash = ?",
			domainappdev.ProviderExecutionDesiredRun,
			domainappdev.ProviderExecutionObservedPending,
			domainappdev.ProviderExecutionLaunchPrepared,
			input.OperationHash.Bytes(),
		).
		Updates(map[string]any{
			"observed_state":        domainappdev.ProviderExecutionObservedSubmitting,
			"submission_started_at": r.clock.nowExpression(),
			"launch_state":          domainappdev.ProviderExecutionLaunchSubmitted,
			"launch_expires_at":     r.clock.addExpression(input.DispatchLeaseDuration),
			"owner_expires_at":      r.clock.addExpression(input.DispatchLeaseDuration),
			"version":               gorm.Expr("version + 1"),
			"updated_at":            r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted &&
		input.OperationHash.EqualBytes(record.LaunchOperationHash) &&
		record.ObservedState == domainappdev.ProviderExecutionObservedSubmitting &&
		record.SubmissionStartedAt != nil && record.ProviderExecutionID == "" {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	if (record.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
		record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted) &&
		!input.OperationHash.EqualBytes(record.LaunchOperationHash) {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil, providerExecutionConflictOr(
		r.classifyOwnedRecord(ctx, record, input.OwnerCAS),
		domainappdev.ErrProviderExecutionStateConflict,
	)
}

func (r *ProviderExecutionRepository) ClaimLaunchReconciliation(
	ctx context.Context,
	input domainappdev.ClaimProviderLaunchReconciliationInput,
) (*domainappdev.ProviderExecution, error) {
	claim := domainappdev.ClaimProviderExecutionInput{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID,
		Generation: input.Generation, ExpectedVersion: input.ExpectedVersion,
		OwnerHash: input.OwnerHash, ExpectedOwnerEpoch: input.ExpectedOwnerEpoch,
		LeaseDuration: input.LeaseDuration,
	}
	spaceID, err := validateProviderExecutionClaim(claim)
	if err != nil || r == nil || r.db == nil || r.clock == nil ||
		(input.ExpectedState != domainappdev.ProviderExecutionLaunchPrepared &&
			input.ExpectedState != domainappdev.ProviderExecutionLaunchSubmitted &&
			input.ExpectedState != domainappdev.ProviderExecutionLaunchLegacySubmitted) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	var claimed *domainappdev.ProviderExecution
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before providerExecutionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("space_id = ? AND project_id = ? AND generation = ?",
				spaceID, input.ProjectID, input.Generation).
			Take(&before).Error; err != nil {
			return normalizeProviderExecutionLookupError(err)
		}
		if _, err := providerExecutionRecordToDomain(&before); err != nil {
			return err
		}
		if before.Version != input.ExpectedVersion {
			return domainappdev.ErrProviderExecutionVersionConflict
		}
		if before.LaunchState != input.ExpectedState || before.ProviderExecutionID != "" ||
			len(before.LaunchOperationHash) != 32 || before.LaunchProviderOperationID == "" {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		dbNow, err := r.clock.read(ctx, tx)
		if err != nil {
			return err
		}
		switch input.ExpectedState {
		case domainappdev.ProviderExecutionLaunchPrepared,
			domainappdev.ProviderExecutionLaunchSubmitted:
			if before.LaunchExpiresAt == nil || before.LaunchExpiresAt.After(dbNow) {
				return domainappdev.ErrProviderExecutionStateConflict
			}
		case domainappdev.ProviderExecutionLaunchLegacySubmitted:
			if before.LaunchExpiresAt != nil {
				return domainappdev.ErrProviderExecutionStateConflict
			}
		}
		ownerAvailable := len(before.OwnerIdentityHash) == 0 || before.OwnerExpiresAt == nil ||
			!before.OwnerExpiresAt.After(dbNow)
		sameOwner := input.ExpectedOwnerEpoch > 0 &&
			input.OwnerHash.EqualBytes(before.OwnerIdentityHash) &&
			before.OwnerEpoch == input.ExpectedOwnerEpoch
		if !ownerAvailable && !sameOwner {
			return domainappdev.ErrProviderExecutionOwnerConflict
		}

		hash := input.OwnerHash.Bytes()
		ownershipSQL := "(owner_identity_hash IS NULL OR owner_expires_at IS NULL OR owner_expires_at <= ?)"
		ownershipArgs := []any{r.clock.nowExpression()}
		epochExpression := gorm.Expr("owner_epoch + 1")
		if input.ExpectedOwnerEpoch > 0 {
			ownershipSQL = "(owner_identity_hash IS NULL OR owner_expires_at IS NULL OR owner_expires_at <= ? OR (owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ?))"
			ownershipArgs = []any{
				r.clock.nowExpression(), hash, input.ExpectedOwnerEpoch, r.clock.nowExpression(),
			}
			epochExpression = gorm.Expr(
				"CASE WHEN owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ? THEN owner_epoch ELSE owner_epoch + 1 END",
				hash, input.ExpectedOwnerEpoch, r.clock.nowExpression(),
			)
		}
		query := tx.Model(&providerExecutionRecord{}).
			Where("space_id = ? AND project_id = ? AND generation = ? AND version = ?",
				spaceID, input.ProjectID, input.Generation, input.ExpectedVersion).
			Where("launch_state = ? AND provider_execution_id = ''", input.ExpectedState).
			Where(ownershipSQL, ownershipArgs...)
		if input.ExpectedState == domainappdev.ProviderExecutionLaunchLegacySubmitted {
			query = query.Where("launch_expires_at IS NULL")
		} else {
			query = query.Where("launch_expires_at <= ?", r.clock.nowExpression())
		}
		updates := map[string]any{
			"owner_identity_hash": hash,
			"owner_epoch":         epochExpression,
			"owner_expires_at":    r.clock.addExpression(input.LeaseDuration),
			"version":             gorm.Expr("version + 1"),
			"updated_at":          r.clock.nowExpression(),
		}
		if input.ExpectedState != domainappdev.ProviderExecutionLaunchLegacySubmitted {
			updates["launch_expires_at"] = r.clock.addExpression(input.LeaseDuration)
		}
		if input.SetDesiredStop {
			updates["desired_state"] = domainappdev.ProviderExecutionDesiredStop
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderExecutionConflict
		}
		var after providerExecutionRecord
		if err := tx.Where("space_id = ? AND project_id = ? AND generation = ?",
			spaceID, input.ProjectID, input.Generation).Take(&after).Error; err != nil {
			return err
		}
		claimed, err = providerExecutionRecordToDomain(&after)
		return err
	})
	if err != nil {
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	return claimed, nil
}

func (r *ProviderExecutionRepository) AbortLaunch(ctx context.Context, input domainappdev.AbortProviderLaunchInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() ||
		(input.ExpectedState != domainappdev.ProviderExecutionLaunchPrepared &&
			input.ExpectedState != domainappdev.ProviderExecutionLaunchSubmitted &&
			input.ExpectedState != domainappdev.ProviderExecutionLaunchLegacySubmitted) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	expectedObserved := domainappdev.ProviderExecutionObservedPending
	if input.ExpectedState == domainappdev.ProviderExecutionLaunchSubmitted ||
		input.ExpectedState == domainappdev.ProviderExecutionLaunchLegacySubmitted {
		expectedObserved = domainappdev.ProviderExecutionObservedSubmitting
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where(
			"desired_state IN ? AND observed_state = ? AND provider_execution_id = '' AND launch_state = ? AND launch_operation_hash = ?",
			[]domainappdev.ProviderExecutionDesiredState{
				domainappdev.ProviderExecutionDesiredRun,
				domainappdev.ProviderExecutionDesiredStop,
			},
			expectedObserved,
			input.ExpectedState,
			input.OperationHash.Bytes(),
		).
		Updates(map[string]any{
			"observed_state":            domainappdev.ProviderExecutionObservedPending,
			"submission_started_at":     nil,
			"provider_lease_expires_at": nil,
			"launch_state":              domainappdev.ProviderExecutionLaunchAborted,
			"launch_expires_at":         nil,
			"version":                   gorm.Expr("version + 1"),
			"updated_at":                r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.LaunchState == domainappdev.ProviderExecutionLaunchAborted &&
		input.OperationHash.EqualBytes(record.LaunchOperationHash) &&
		record.ObservedState == domainappdev.ProviderExecutionObservedPending &&
		record.ProviderExecutionID == "" &&
		record.Version == input.OwnerCAS.ExpectedVersion+1 {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	if (record.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
		record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted ||
		record.LaunchState == domainappdev.ProviderExecutionLaunchLegacySubmitted) &&
		!input.OperationHash.EqualBytes(record.LaunchOperationHash) {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil, providerExecutionConflictOr(
		r.classifyOwnedRecord(ctx, record, input.OwnerCAS),
		domainappdev.ErrProviderExecutionStateConflict,
	)
}

func (r *ProviderExecutionRepository) SaveSubmission(ctx context.Context, input domainappdev.SaveProviderSubmissionInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	preview, previewErr := domainappdev.NormalizeProviderExecutionPreviewRoute(input.PreviewRoute)
	if err != nil || input.OperationHash.IsZero() || !domainappdev.ValidProviderExecutionProviderID(input.ProviderExecutionID) ||
		input.ObservedState != domainappdev.ProviderExecutionObservedRunning || !validProviderExecutionLeaseDuration(input.ProviderLeaseDuration) || previewErr != nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	input.PreviewRoute = preview
	updates := providerExecutionSubmissionUpdates(r.clock, input)
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state IN ? AND observed_state = ? AND submission_started_at IS NOT NULL AND launch_state = ? AND launch_operation_hash = ? AND (provider_execution_id = '' OR provider_execution_id = ?)", []domainappdev.ProviderExecutionDesiredState{
			domainappdev.ProviderExecutionDesiredRun,
			domainappdev.ProviderExecutionDesiredStop,
		}, domainappdev.ProviderExecutionObservedSubmitting, domainappdev.ProviderExecutionLaunchSubmitted, input.OperationHash.Bytes(), input.ProviderExecutionID).
		Updates(updates)
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.ProviderExecutionID != "" && record.ProviderExecutionID != input.ProviderExecutionID {
		return nil, domainappdev.ErrProviderExecutionIDConflict
	}
	if record.ProviderExecutionID == input.ProviderExecutionID && record.ObservedState == domainappdev.ProviderExecutionObservedRunning && record.PreviewRoute == preview && record.Version == input.OwnerCAS.ExpectedVersion+1 {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) CompleteReconciledLaunch(ctx context.Context, input domainappdev.CompleteReconciledProviderLaunchInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	preview, previewErr := domainappdev.NormalizeProviderExecutionPreviewRoute(input.PreviewRoute)
	if err != nil || previewErr != nil || input.LaunchOperationHash.IsZero() ||
		input.CheckpointOperationHash.IsZero() || input.CheckpointWriteRevision == 0 ||
		!validProviderExecutionCheckpointEnvelope(input.CheckpointEnvelope) ||
		!domainappdev.ValidProviderExecutionProviderID(input.ProviderExecutionID) ||
		!validProviderExecutionLeaseDuration(input.ProviderLeaseDuration) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state IN ? AND observed_state IN ? AND submission_started_at IS NOT NULL",
			[]domainappdev.ProviderExecutionDesiredState{
				domainappdev.ProviderExecutionDesiredRun,
				domainappdev.ProviderExecutionDesiredStop,
			},
			[]domainappdev.ProviderExecutionObservedState{
				domainappdev.ProviderExecutionObservedSubmitting,
				domainappdev.ProviderExecutionObservedRunning,
			},
		).
		Where("launch_state IN ? AND launch_operation_hash = ?",
			[]domainappdev.ProviderExecutionLaunchState{
				domainappdev.ProviderExecutionLaunchSubmitted,
				domainappdev.ProviderExecutionLaunchLegacySubmitted,
			}, input.LaunchOperationHash.Bytes()).
		Where("provider_execution_id = '' OR provider_execution_id = ?", input.ProviderExecutionID).
		Where("checkpoint_write_pending = ? AND checkpoint_write_revision = ? AND checkpoint_write_operation_hash = ? AND checkpoint_write_expires_at > ?",
			true, input.CheckpointWriteRevision, input.CheckpointOperationHash.Bytes(), r.clock.nowExpression()).
		Updates(map[string]any{
			"provider_execution_id":           input.ProviderExecutionID,
			"observed_state":                  domainappdev.ProviderExecutionObservedRunning,
			"provider_lease_expires_at":       r.clock.addExpression(input.ProviderLeaseDuration),
			"preview_route":                   preview,
			"checkpoint_envelope":             input.CheckpointEnvelope,
			"checkpoint_write_pending":        false,
			"checkpoint_write_operation_hash": nil,
			"checkpoint_write_expires_at":     nil,
			"checkpoint_last_operation_hash":  input.CheckpointOperationHash.Bytes(),
			"launch_state":                    domainappdev.ProviderExecutionLaunchComplete,
			"launch_expires_at":               nil,
			"version":                         gorm.Expr("version + 1"),
			"updated_at":                      r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.ProviderExecutionID != "" && record.ProviderExecutionID != input.ProviderExecutionID {
		return nil, domainappdev.ErrProviderExecutionIDConflict
	}
	if record.LaunchState == domainappdev.ProviderExecutionLaunchComplete &&
		input.LaunchOperationHash.EqualBytes(record.LaunchOperationHash) &&
		record.ProviderExecutionID == input.ProviderExecutionID &&
		record.CheckpointEnvelope == input.CheckpointEnvelope &&
		!record.CheckpointWritePending &&
		input.CheckpointOperationHash.EqualBytes(record.CheckpointLastOperationHash) {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	return nil, providerExecutionConflictOr(
		r.classifyOwnedRecord(ctx, record, input.OwnerCAS),
		domainappdev.ErrProviderExecutionOperationConflict,
	)
}

func (r *ProviderExecutionRepository) ObserveActive(ctx context.Context, input domainappdev.ObserveProviderExecutionActiveInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	preview, previewErr := domainappdev.NormalizeProviderExecutionPreviewRoute(input.PreviewRoute)
	if err != nil || previewErr != nil || !domainappdev.ValidProviderExecutionProviderID(input.ProviderExecutionID) ||
		!validProviderExecutionLeaseDuration(input.ProviderLeaseDuration) ||
		!domainappdev.ValidProviderExecutionSafeError(input.SafeErrorCode, input.SafeErrorMessage) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state = ? AND observed_state = ? AND submission_started_at IS NOT NULL AND provider_execution_id = ?",
			domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning, input.ProviderExecutionID).
		Updates(map[string]any{
			"preview_route": preview, "safe_error_code": input.SafeErrorCode, "safe_error_message": input.SafeErrorMessage,
			"provider_lease_expires_at": r.clock.addExpression(input.ProviderLeaseDuration),
			"version":                   gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.ProviderExecutionID != "" && record.ProviderExecutionID != input.ProviderExecutionID {
		return nil, domainappdev.ErrProviderExecutionIDConflict
	}
	if record.ObservedState == domainappdev.ProviderExecutionObservedRunning && record.ProviderExecutionID == input.ProviderExecutionID &&
		record.PreviewRoute == preview && record.SafeErrorCode == input.SafeErrorCode && record.SafeErrorMessage == input.SafeErrorMessage &&
		record.Version == input.OwnerCAS.ExpectedVersion+1 {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) ReserveBuild(ctx context.Context, input domainappdev.ReserveProviderBuildInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	expectedHash, hashErr := domainappdev.HashProviderExecutionBuildOperationID(input.OperationID, input.OwnerCAS, input.ProviderExecutionID)
	if err != nil || hashErr != nil || !expectedHash.Equal(input.OperationHash) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	var rowsAffected int64
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project providerExecutionProjectLock
		if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("space_id = ? AND id = ?", spaceID, input.OwnerCAS.ProjectID).
			Take(&project).Error; lockErr != nil {
			if errors.Is(lockErr, gorm.ErrRecordNotFound) {
				return domainappdev.ErrProviderExecutionGenerationConflict
			}
			return lockErr
		}
		if project.Status == string(domainappdev.ProjectStatusArchived) ||
			normalizedProjectArchiveState(project.ArchiveState) != domainappdev.ProjectArchiveStateNone {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		result := r.ownedProviderExecutionQueryOn(tx, ctx, spaceID, input.OwnerCAS).
			Where("desired_state = ? AND observed_state = ? AND provider_execution_id = ? AND artifact_status IN ?",
				domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning, input.ProviderExecutionID,
				[]domainappdev.ProviderExecutionArtifactStatus{"", domainappdev.ProviderExecutionArtifactNone, domainappdev.ProviderExecutionArtifactReady, domainappdev.ProviderExecutionArtifactFailed}).
			Updates(map[string]any{
				"build_operation_id": input.OperationID, "build_operation_hash": input.OperationHash.Bytes(),
				"artifact_status": domainappdev.ProviderExecutionArtifactBeginPending,
				"artifact_kind":   "", "artifact_digest": "", "artifact_size": 0, "artifact_object_key": "",
				"artifact_safe_error_code": "", "artifact_safe_error_message": "",
				"artifact_version": gorm.Expr("artifact_version + 1"),
				"build_started_at": r.clock.nowExpression(), "artifact_updated_at": r.clock.nowExpression(),
				"version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
			})
		rowsAffected = result.RowsAffected
		return result.Error
	})
	if err != nil {
		if errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) ||
			errors.Is(err, domainappdev.ErrProviderExecutionGenerationConflict) {
			return nil, err
		}
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	if rowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadBuildRecord(ctx, spaceID, input.OwnerCAS, input.ProviderExecutionID)
	if loadErr != nil {
		return nil, loadErr
	}
	status := normalizedProviderExecutionArtifactStatus(record.ArtifactStatus)
	sameOperation := record.BuildOperationID == input.OperationID && input.OperationHash.EqualBytes(record.BuildOperationHash)
	if sameOperation && status != domainappdev.ProviderExecutionArtifactNone && buildPostVersion(record.Version, input.OwnerCAS.ExpectedVersion) {
		return providerExecutionRecordToDomain(record)
	}
	if providerExecutionBuildInFlight(status) && !sameOperation {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) AdvanceBuildObservation(ctx context.Context, input domainappdev.AdvanceProviderBuildObservationInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderBuildMutation(input.OwnerCAS, input.ProviderExecutionID, input.OperationHash)
	if err != nil {
		return nil, err
	}
	if input.Status == domainappdev.ProviderExecutionArtifactBuilding {
		if input.Descriptor != (domainappdev.ProviderBuildArtifactDescriptor{}) {
			return nil, domainappdev.ErrProviderExecutionInvalid
		}
		result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
			Where("desired_state = ? AND observed_state = ? AND provider_execution_id = ? AND build_operation_hash = ? AND artifact_status = ?",
				domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning, input.ProviderExecutionID,
				input.OperationHash.Bytes(), domainappdev.ProviderExecutionArtifactBeginPending).
			Updates(map[string]any{
				"artifact_status":     domainappdev.ProviderExecutionArtifactBuilding,
				"artifact_updated_at": r.clock.nowExpression(), "version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
			})
		if result.Error != nil {
			return nil, normalizeProviderExecutionDatabaseError(result.Error)
		}
		if result.RowsAffected == 1 {
			return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
		}
		record, loadErr := r.loadBuildRecord(ctx, spaceID, input.OwnerCAS, input.ProviderExecutionID)
		if loadErr != nil {
			return nil, loadErr
		}
		status := normalizedProviderExecutionArtifactStatus(record.ArtifactStatus)
		if input.OperationHash.EqualBytes(record.BuildOperationHash) &&
			(status == domainappdev.ProviderExecutionArtifactBuilding || status == domainappdev.ProviderExecutionArtifactDescriptorReady ||
				status == domainappdev.ProviderExecutionArtifactPublishing || status == domainappdev.ProviderExecutionArtifactReady) &&
			buildPostVersion(record.Version, input.OwnerCAS.ExpectedVersion) {
			return providerExecutionRecordToDomain(record)
		}
		if !input.OperationHash.EqualBytes(record.BuildOperationHash) {
			return nil, domainappdev.ErrProviderExecutionOperationConflict
		}
		if status == domainappdev.ProviderExecutionArtifactBeginPending {
			return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
		}
		return nil, domainappdev.ErrProviderExecutionStateConflict
	}
	if input.Status != domainappdev.ProviderExecutionArtifactDescriptorReady {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	descriptor, descriptorErr := domainappdev.NormalizeProviderBuildArtifactDescriptor(input.Descriptor)
	if descriptorErr != nil {
		return nil, descriptorErr
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state = ? AND observed_state = ? AND provider_execution_id = ? AND build_operation_hash = ? AND artifact_status IN ?",
			domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning, input.ProviderExecutionID,
			input.OperationHash.Bytes(), []domainappdev.ProviderExecutionArtifactStatus{
				domainappdev.ProviderExecutionArtifactBeginPending, domainappdev.ProviderExecutionArtifactBuilding,
			}).
		Updates(map[string]any{
			"artifact_status": domainappdev.ProviderExecutionArtifactDescriptorReady,
			"artifact_kind":   descriptor.Kind, "artifact_digest": descriptor.Digest, "artifact_size": descriptor.Size,
			"artifact_updated_at": r.clock.nowExpression(), "version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadBuildRecord(ctx, spaceID, input.OwnerCAS, input.ProviderExecutionID)
	if loadErr != nil {
		return nil, loadErr
	}
	if !input.OperationHash.EqualBytes(record.BuildOperationHash) {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	status := normalizedProviderExecutionArtifactStatus(record.ArtifactStatus)
	if (status == domainappdev.ProviderExecutionArtifactDescriptorReady || status == domainappdev.ProviderExecutionArtifactPublishing || status == domainappdev.ProviderExecutionArtifactReady) &&
		record.ArtifactKind == descriptor.Kind && record.ArtifactDigest == descriptor.Digest && record.ArtifactSize == descriptor.Size &&
		buildPostVersion(record.Version, input.OwnerCAS.ExpectedVersion) {
		return providerExecutionRecordToDomain(record)
	}
	if status == domainappdev.ProviderExecutionArtifactDescriptorReady || status == domainappdev.ProviderExecutionArtifactPublishing ||
		status == domainappdev.ProviderExecutionArtifactReady {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	if status != domainappdev.ProviderExecutionArtifactBeginPending && status != domainappdev.ProviderExecutionArtifactBuilding {
		return nil, domainappdev.ErrProviderExecutionStateConflict
	}
	return nil, domainappdev.ErrProviderExecutionOperationConflict
}

func (r *ProviderExecutionRepository) BeginArtifactPublish(ctx context.Context, input domainappdev.BeginProviderArtifactPublishInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderBuildMutation(input.OwnerCAS, input.ProviderExecutionID, input.OperationHash)
	if err != nil {
		return nil, err
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state = ? AND observed_state = ? AND provider_execution_id = ? AND build_operation_hash = ? AND artifact_status = ?",
			domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning, input.ProviderExecutionID,
			input.OperationHash.Bytes(), domainappdev.ProviderExecutionArtifactDescriptorReady).
		Updates(map[string]any{
			"artifact_status":     domainappdev.ProviderExecutionArtifactPublishing,
			"artifact_updated_at": r.clock.nowExpression(), "version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadBuildRecord(ctx, spaceID, input.OwnerCAS, input.ProviderExecutionID)
	if loadErr != nil {
		return nil, loadErr
	}
	if !input.OperationHash.EqualBytes(record.BuildOperationHash) {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	if record.ArtifactStatus == domainappdev.ProviderExecutionArtifactPublishing && buildPostVersion(record.Version, input.OwnerCAS.ExpectedVersion) {
		return providerExecutionRecordToDomain(record)
	}
	return nil, domainappdev.ErrProviderExecutionStateConflict
}

func (r *ProviderExecutionRepository) CompleteArtifactPublish(ctx context.Context, input domainappdev.CompleteProviderArtifactPublishInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderBuildMutation(input.OwnerCAS, input.ProviderExecutionID, input.OperationHash)
	objectKey, keyErr := domainappdev.NormalizeProviderExecutionArtifactObjectKey(input.ObjectKey)
	descriptor, descriptorErr := domainappdev.NormalizeProviderBuildArtifactDescriptor(domainappdev.ProviderBuildArtifactDescriptor{
		Kind: domainappdev.ProviderExecutionArtifactKindAppDevBuildArchive, Digest: input.Digest, Size: input.Size,
	})
	if err != nil || keyErr != nil || descriptorErr != nil || objectKey == "" {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("provider_execution_id = ? AND build_operation_hash = ? AND artifact_status = ? AND artifact_kind = ? AND artifact_digest = ? AND artifact_size = ?",
			input.ProviderExecutionID, input.OperationHash.Bytes(), domainappdev.ProviderExecutionArtifactPublishing,
			descriptor.Kind, descriptor.Digest, descriptor.Size).
		Where("((desired_state = ? AND observed_state = ?) OR (desired_state = ? AND observed_state <> ?))",
			domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning,
			domainappdev.ProviderExecutionDesiredStop, domainappdev.ProviderExecutionObservedCleanupComplete).
		Updates(map[string]any{
			"artifact_status": domainappdev.ProviderExecutionArtifactReady, "artifact_object_key": objectKey,
			"artifact_updated_at": r.clock.nowExpression(), "version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadBuildRecord(ctx, spaceID, input.OwnerCAS, input.ProviderExecutionID)
	if loadErr != nil {
		return nil, loadErr
	}
	if !input.OperationHash.EqualBytes(record.BuildOperationHash) {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	if record.ArtifactStatus == domainappdev.ProviderExecutionArtifactReady && record.ArtifactObjectKey == objectKey &&
		record.ArtifactKind == descriptor.Kind && record.ArtifactDigest == descriptor.Digest && record.ArtifactSize == descriptor.Size &&
		buildPostVersion(record.Version, input.OwnerCAS.ExpectedVersion) {
		return providerExecutionRecordToDomain(record)
	}
	if record.ArtifactDigest != descriptor.Digest || record.ArtifactSize != descriptor.Size {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil, domainappdev.ErrProviderExecutionStateConflict
}

func (r *ProviderExecutionRepository) FailBuild(ctx context.Context, input domainappdev.FailProviderBuildInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderBuildMutation(input.OwnerCAS, input.ProviderExecutionID, input.OperationHash)
	if err != nil || input.SafeErrorCode == "" || !domainappdev.ValidProviderExecutionSafeError(input.SafeErrorCode, input.SafeErrorMessage) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("provider_execution_id = ? AND build_operation_hash = ?", input.ProviderExecutionID, input.OperationHash.Bytes()).
		Where("((desired_state = ? AND observed_state = ? AND artifact_status IN ?) OR (desired_state = ? AND observed_state <> ? AND artifact_status = ?))",
			domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedRunning,
			[]domainappdev.ProviderExecutionArtifactStatus{
				domainappdev.ProviderExecutionArtifactBeginPending, domainappdev.ProviderExecutionArtifactBuilding,
				domainappdev.ProviderExecutionArtifactDescriptorReady, domainappdev.ProviderExecutionArtifactPublishing,
			}, domainappdev.ProviderExecutionDesiredStop, domainappdev.ProviderExecutionObservedCleanupComplete,
			domainappdev.ProviderExecutionArtifactPublishing).
		Updates(map[string]any{
			"artifact_status":          domainappdev.ProviderExecutionArtifactFailed,
			"artifact_safe_error_code": input.SafeErrorCode, "artifact_safe_error_message": input.SafeErrorMessage,
			"artifact_object_key": "", "artifact_updated_at": r.clock.nowExpression(),
			"version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadBuildRecord(ctx, spaceID, input.OwnerCAS, input.ProviderExecutionID)
	if loadErr != nil {
		return nil, loadErr
	}
	if !input.OperationHash.EqualBytes(record.BuildOperationHash) {
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	if record.ArtifactStatus == domainappdev.ProviderExecutionArtifactFailed && record.ArtifactSafeErrorCode == input.SafeErrorCode &&
		record.ArtifactSafeErrorMessage == input.SafeErrorMessage && buildPostVersion(record.Version, input.OwnerCAS.ExpectedVersion) {
		return providerExecutionRecordToDomain(record)
	}
	return nil, domainappdev.ErrProviderExecutionStateConflict
}

func validateProviderBuildMutation(cas domainappdev.ProviderExecutionOwnerCAS, providerExecutionID string, operationHash domainappdev.ProviderExecutionOperationHash) (int64, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(cas)
	if err != nil || !domainappdev.ValidProviderExecutionProviderID(providerExecutionID) || operationHash.IsZero() {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	return spaceID, nil
}

func (r *ProviderExecutionRepository) loadBuildRecord(ctx context.Context, spaceID int64, cas domainappdev.ProviderExecutionOwnerCAS, providerExecutionID string) (*providerExecutionRecord, error) {
	record, err := r.loadRecordByGeneration(ctx, spaceID, cas.ProjectID, cas.Generation)
	if err != nil {
		return nil, err
	}
	if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, cas); ownerErr != nil {
		return nil, ownerErr
	}
	if record.ProviderExecutionID != providerExecutionID {
		return nil, domainappdev.ErrProviderExecutionIDConflict
	}
	if record.DesiredState != domainappdev.ProviderExecutionDesiredRun || record.ObservedState != domainappdev.ProviderExecutionObservedRunning {
		return nil, domainappdev.ErrProviderExecutionStateConflict
	}
	return record, nil
}

func normalizedProviderExecutionArtifactStatus(status domainappdev.ProviderExecutionArtifactStatus) domainappdev.ProviderExecutionArtifactStatus {
	if status == "" {
		return domainappdev.ProviderExecutionArtifactNone
	}
	return status
}

func providerExecutionBuildInFlight(status domainappdev.ProviderExecutionArtifactStatus) bool {
	return status == domainappdev.ProviderExecutionArtifactBeginPending || status == domainappdev.ProviderExecutionArtifactBuilding || status == domainappdev.ProviderExecutionArtifactDescriptorReady ||
		status == domainappdev.ProviderExecutionArtifactPublishing
}

func buildPostVersion(current, expected uint64) bool {
	return current == expected || (expected < ^uint64(0) && current == expected+1)
}

func (r *ProviderExecutionRepository) ReserveCheckpointWrite(ctx context.Context, input domainappdev.ReserveProviderCheckpointWriteInput) (*domainappdev.ProviderExecutionCheckpointReservation, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() || !validProviderExecutionLeaseDuration(input.ReservationDuration) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	var reservation *domainappdev.ProviderExecutionCheckpointReservation
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record providerExecutionRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation).Take(&record).Error; err != nil {
			return normalizeProviderExecutionLookupError(err)
		}
		dbNow, err := r.clock.read(ctx, tx)
		if err != nil {
			return err
		}
		if err := classifyProviderExecutionOwnerIdentity(&record, input.OwnerCAS, dbNow); err != nil {
			return err
		}
		if record.ObservedState == domainappdev.ProviderExecutionObservedCleanupComplete {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		if input.OperationHash.EqualBytes(record.CheckpointLastOperationHash) && !record.CheckpointWritePending {
			entity, err := providerExecutionRecordToDomain(&record)
			if err != nil {
				return err
			}
			reservation = &domainappdev.ProviderExecutionCheckpointReservation{
				Revision: record.CheckpointWriteRevision, ReservedVersion: record.Version,
				AlreadyCommitted: true, Record: entity,
			}
			return nil
		}
		if record.CheckpointWritePending && input.OperationHash.EqualBytes(record.CheckpointWriteOperationHash) {
			if record.CheckpointWriteExpiresAt == nil || !record.CheckpointWriteExpiresAt.After(dbNow) {
				result := tx.Model(&providerExecutionRecord{}).
					Where("space_id = ? AND project_id = ? AND generation = ? AND version = ?", spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation, record.Version).
					Where("owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ?", input.OwnerCAS.OwnerHash.Bytes(), input.OwnerCAS.OwnerEpoch, r.clock.nowExpression()).
					Where("checkpoint_write_pending = ? AND checkpoint_write_revision = ? AND checkpoint_write_operation_hash = ?", true, record.CheckpointWriteRevision, input.OperationHash.Bytes()).
					Where("checkpoint_write_expires_at IS NULL OR checkpoint_write_expires_at <= ?", r.clock.nowExpression()).
					Updates(map[string]any{
						"checkpoint_write_revision":   gorm.Expr("checkpoint_write_revision + 1"),
						"checkpoint_write_expires_at": r.clock.addExpression(input.ReservationDuration),
						"version":                     gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
					})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return domainappdev.ErrProviderExecutionOperationConflict
				}
				var renewed providerExecutionRecord
				if err := tx.Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation).Take(&renewed).Error; err != nil {
					return err
				}
				reservation = &domainappdev.ProviderExecutionCheckpointReservation{Revision: renewed.CheckpointWriteRevision, ReservedVersion: renewed.Version}
				return nil
			}
			reservation = &domainappdev.ProviderExecutionCheckpointReservation{Revision: record.CheckpointWriteRevision, ReservedVersion: record.Version}
			return nil
		}
		if record.CheckpointWritePending && record.CheckpointWriteExpiresAt != nil && record.CheckpointWriteExpiresAt.After(dbNow) {
			return domainappdev.ErrProviderExecutionOperationConflict
		}
		if record.Version != input.OwnerCAS.ExpectedVersion && !(record.CheckpointWritePending && record.Version == input.OwnerCAS.ExpectedVersion+1) {
			return domainappdev.ErrProviderExecutionVersionConflict
		}
		query := tx.Model(&providerExecutionRecord{}).
			Where("space_id = ? AND project_id = ? AND generation = ? AND version = ?", spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation, record.Version).
			Where("owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ?", input.OwnerCAS.OwnerHash.Bytes(), input.OwnerCAS.OwnerEpoch, r.clock.nowExpression()).
			Where("observed_state <> ?", domainappdev.ProviderExecutionObservedCleanupComplete)
		if record.CheckpointWritePending {
			query = query.Where("checkpoint_write_pending = ? AND (checkpoint_write_expires_at IS NULL OR checkpoint_write_expires_at <= ?)", true, r.clock.nowExpression())
		} else {
			query = query.Where("checkpoint_write_pending = ?", false)
		}
		result := query.
			Updates(map[string]any{
				"checkpoint_write_revision": gorm.Expr("checkpoint_write_revision + 1"),
				"checkpoint_write_pending":  true, "checkpoint_write_operation_hash": input.OperationHash.Bytes(),
				"checkpoint_write_expires_at": r.clock.addExpression(input.ReservationDuration),
				"version":                     gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderExecutionConflict
		}
		var reserved providerExecutionRecord
		if err := tx.Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation).Take(&reserved).Error; err != nil {
			return err
		}
		reservation = &domainappdev.ProviderExecutionCheckpointReservation{Revision: reserved.CheckpointWriteRevision, ReservedVersion: reserved.Version}
		return nil
	})
	if err != nil {
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	return reservation, nil
}

func (r *ProviderExecutionRepository) AbortCheckpointWrite(ctx context.Context, input domainappdev.AbortProviderCheckpointWriteInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() || input.CheckpointWriteRevision == 0 {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("checkpoint_write_pending = ? AND checkpoint_write_revision = ? AND checkpoint_write_operation_hash = ?", true, input.CheckpointWriteRevision, input.OperationHash.Bytes()).
		Updates(map[string]any{
			"checkpoint_write_pending": false, "checkpoint_write_operation_hash": nil,
			"checkpoint_write_expires_at": nil, "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, providerExecutionConflictOr(r.classifyOwnedConflict(ctx, spaceID, input.OwnerCAS), domainappdev.ErrProviderExecutionOperationConflict)
	}
	return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
}

func (r *ProviderExecutionRepository) SaveCheckpoint(ctx context.Context, input domainappdev.SaveProviderCheckpointInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() || input.CheckpointWriteRevision == 0 || !validProviderExecutionCheckpointEnvelope(input.CheckpointEnvelope) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	query := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("observed_state <> ? AND checkpoint_write_pending = ? AND checkpoint_write_revision = ? AND checkpoint_write_operation_hash = ?", domainappdev.ProviderExecutionObservedCleanupComplete, true, input.CheckpointWriteRevision, input.OperationHash.Bytes()).
		Where("checkpoint_write_expires_at > ?", r.clock.nowExpression())
	updates := map[string]any{
		"checkpoint_envelope": input.CheckpointEnvelope, "checkpoint_write_pending": false,
		"checkpoint_write_operation_hash": nil, "checkpoint_write_expires_at": nil,
		"checkpoint_last_operation_hash": input.OperationHash.Bytes(),
		"version":                        gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
	}
	if input.LaunchOperationHash.IsZero() {
		query = query.Where("launch_state <> ?", domainappdev.ProviderExecutionLaunchSubmitted)
	} else {
		query = query.Where(
			"launch_state = ? AND launch_operation_hash = ?",
			domainappdev.ProviderExecutionLaunchSubmitted,
			input.LaunchOperationHash.Bytes(),
		)
		updates["launch_state"] = domainappdev.ProviderExecutionLaunchComplete
		updates["launch_expires_at"] = nil
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if !record.CheckpointWritePending && record.CheckpointWriteRevision == input.CheckpointWriteRevision &&
		input.OperationHash.EqualBytes(record.CheckpointLastOperationHash) {
		launchCommitted := input.LaunchOperationHash.IsZero() && record.LaunchState != domainappdev.ProviderExecutionLaunchSubmitted
		if !input.LaunchOperationHash.IsZero() {
			launchCommitted = record.LaunchState == domainappdev.ProviderExecutionLaunchComplete &&
				input.LaunchOperationHash.EqualBytes(record.LaunchOperationHash)
		}
		if !launchCommitted {
			return nil, domainappdev.ErrProviderExecutionOperationConflict
		}
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionOperationConflict)
}

func (r *ProviderExecutionRepository) SetDesiredStop(ctx context.Context, input domainappdev.SetProviderExecutionDesiredStopInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil {
		return nil, err
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state = ? AND observed_state <> ?", domainappdev.ProviderExecutionDesiredRun, domainappdev.ProviderExecutionObservedCleanupComplete).
		Where("(launch_state NOT IN ? OR launch_expires_at <= ?) AND launch_state <> ?",
			[]domainappdev.ProviderExecutionLaunchState{
				domainappdev.ProviderExecutionLaunchPrepared,
				domainappdev.ProviderExecutionLaunchSubmitted,
			},
			r.clock.nowExpression(),
			domainappdev.ProviderExecutionLaunchLegacySubmitted,
		).
		Updates(map[string]any{"desired_state": domainappdev.ProviderExecutionDesiredStop, "version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression()})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.DesiredState == domainappdev.ProviderExecutionDesiredStop &&
		(record.Version == input.OwnerCAS.ExpectedVersion || record.Version == input.OwnerCAS.ExpectedVersion+1) {
		if record.LaunchState == domainappdev.ProviderExecutionLaunchLegacySubmitted {
			return nil, domainappdev.ErrProviderExecutionStateConflict
		}
		if record.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
			record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted {
			dbNow, clockErr := r.clock.read(ctx, r.db.WithContext(ctx))
			if clockErr != nil {
				return nil, normalizeProviderExecutionDatabaseError(clockErr)
			}
			if record.LaunchExpiresAt == nil || record.LaunchExpiresAt.After(dbNow) {
				return nil, domainappdev.ErrProviderExecutionStateConflict
			}
		}
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) BeginCleanup(ctx context.Context, input domainappdev.BeginProviderExecutionCleanupInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil {
		return nil, err
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state = ? AND observed_state IN ? AND artifact_status <> ?", domainappdev.ProviderExecutionDesiredStop,
			domainappdev.ProviderExecutionCleanupSourceStates(), domainappdev.ProviderExecutionArtifactPublishing).
		Where("launch_state NOT IN ?", []domainappdev.ProviderExecutionLaunchState{
			domainappdev.ProviderExecutionLaunchPrepared,
			domainappdev.ProviderExecutionLaunchSubmitted,
			domainappdev.ProviderExecutionLaunchLegacySubmitted,
			domainappdev.ProviderExecutionLaunchQuarantined,
		}).
		Updates(map[string]any{
			"observed_state": domainappdev.ProviderExecutionObservedCleanupPending,
			"version":        gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.DesiredState == domainappdev.ProviderExecutionDesiredStop && record.ObservedState == domainappdev.ProviderExecutionObservedCleanupPending &&
		record.ArtifactStatus != domainappdev.ProviderExecutionArtifactPublishing &&
		(record.Version == input.OwnerCAS.ExpectedVersion || record.Version == input.OwnerCAS.ExpectedVersion+1) {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			return providerExecutionRecordToDomain(record)
		}
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) AdvanceTerminal(ctx context.Context, input domainappdev.AdvanceProviderExecutionTerminalInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	if err != nil || input.OperationHash.IsZero() || !domainappdev.ValidProviderExecutionSafeError(input.SafeErrorCode, input.SafeErrorMessage) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if !isProviderExecutionTerminal(input.ObservedState) {
		return nil, domainappdev.ErrProviderExecutionStateConflict
	}
	preview, err := domainappdev.NormalizeProviderExecutionPreviewRoute(input.PreviewRoute)
	if err != nil {
		return nil, err
	}
	artifact, err := domainappdev.NormalizeProviderExecutionArtifactObjectKey(input.ArtifactObjectKey)
	if err != nil {
		return nil, err
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("observed_state = ? AND submission_started_at IS NOT NULL AND provider_execution_id <> '' AND terminal_operation_hash IS NULL", domainappdev.ProviderExecutionObservedRunning).
		Updates(map[string]any{
			"observed_state": input.ObservedState, "preview_route": preview, "artifact_object_key": artifact,
			"safe_error_code": input.SafeErrorCode, "safe_error_message": input.SafeErrorMessage,
			"terminal_operation_hash": input.OperationHash.Bytes(),
			"version":                 gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	postVersion := record.Version == input.OwnerCAS.ExpectedVersion || record.Version == input.OwnerCAS.ExpectedVersion+1
	if isProviderExecutionTerminal(record.ObservedState) && postVersion {
		if ownerErr := r.classifyOwnedRecordIgnoringVersion(ctx, record, input.OwnerCAS); ownerErr == nil {
			strictPostcondition := record.ObservedState == input.ObservedState && input.OperationHash.EqualBytes(record.TerminalOperationHash) &&
				record.SubmissionStartedAt != nil && record.ProviderExecutionID != "" && record.PreviewRoute == preview &&
				record.ArtifactObjectKey == artifact && record.SafeErrorCode == input.SafeErrorCode && record.SafeErrorMessage == input.SafeErrorMessage
			if strictPostcondition {
				return providerExecutionRecordToDomain(record)
			}
			return nil, domainappdev.ErrProviderExecutionOperationConflict
		}
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) CompleteCleanup(ctx context.Context, input domainappdev.CompleteProviderExecutionCleanupInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input.OwnerCAS)
	operationHash, hashErr := domainappdev.HashProviderExecutionCleanupOperationID(input.OperationID, input.OwnerCAS)
	if err != nil || hashErr != nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	result := r.ownedProviderExecutionQuery(ctx, spaceID, input.OwnerCAS).
		Where("desired_state = ? AND artifact_status <> ?", domainappdev.ProviderExecutionDesiredStop, domainappdev.ProviderExecutionArtifactPublishing).
		Where("observed_state = ? OR (observed_state = ? AND provider_execution_id = '' AND submission_started_at IS NULL)",
			domainappdev.ProviderExecutionObservedCleanupPending, domainappdev.ProviderExecutionObservedPending).
		Updates(map[string]any{
			"observed_state":            domainappdev.ProviderExecutionObservedCleanupComplete,
			"provider_lease_expires_at": nil, "checkpoint_envelope": "", "checkpoint_write_pending": false,
			"checkpoint_write_operation_hash": nil, "checkpoint_write_expires_at": nil,
			"launch_state": domainappdev.ProviderExecutionLaunchNone, "launch_operation_hash": nil,
			"launch_provider_operation_id": "", "launch_request_digest": nil, "launch_expires_at": nil,
			"owner_identity_hash": nil, "owner_expires_at": nil, "cleanup_operation_hash": operationHash.Bytes(),
			"version": gorm.Expr("version + 1"), "updated_at": r.clock.nowExpression(),
		})
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	if result.RowsAffected == 1 {
		return r.loadByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	}
	record, loadErr := r.loadRecordByGeneration(ctx, spaceID, input.OwnerCAS.ProjectID, input.OwnerCAS.Generation)
	if loadErr != nil {
		return nil, loadErr
	}
	if record.ProviderKey == input.OwnerCAS.ProviderKey && record.ProviderScope == input.OwnerCAS.ProviderScope &&
		record.DesiredState == domainappdev.ProviderExecutionDesiredStop &&
		record.ObservedState == domainappdev.ProviderExecutionObservedCleanupComplete &&
		record.ArtifactStatus != domainappdev.ProviderExecutionArtifactPublishing &&
		len(record.OwnerIdentityHash) == 0 && record.OwnerExpiresAt == nil && record.ProviderLeaseExpiresAt == nil &&
		record.CheckpointEnvelope == "" && !record.CheckpointWritePending && record.CheckpointWriteExpiresAt == nil &&
		record.LaunchState == domainappdev.ProviderExecutionLaunchNone && len(record.LaunchOperationHash) == 0 && record.LaunchExpiresAt == nil &&
		record.OwnerEpoch == input.OwnerCAS.OwnerEpoch &&
		operationHash.EqualBytes(record.CleanupOperationHash) &&
		(record.Version == input.OwnerCAS.ExpectedVersion || record.Version == input.OwnerCAS.ExpectedVersion+1) {
		return providerExecutionRecordToDomain(record)
	}
	if record.ObservedState == domainappdev.ProviderExecutionObservedCleanupComplete {
		if record.OwnerEpoch != input.OwnerCAS.OwnerEpoch {
			return nil, domainappdev.ErrProviderExecutionOwnerConflict
		}
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil, providerExecutionConflictOr(r.classifyOwnedRecord(ctx, record, input.OwnerCAS), domainappdev.ErrProviderExecutionStateConflict)
}

func (r *ProviderExecutionRepository) LoadOwnedRecovery(ctx context.Context, input domainappdev.ProviderExecutionOwnerCAS) (*domainappdev.ProviderExecution, error) {
	spaceID, err := validateProviderExecutionOwnerCAS(input)
	if err != nil {
		return nil, err
	}
	var record providerExecutionRecord
	if err := r.ownedProviderExecutionQuery(ctx, spaceID, input).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, providerExecutionConflictOr(r.classifyOwnedConflict(ctx, spaceID, input), domainappdev.ErrProviderExecutionConflict)
		}
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	return providerExecutionRecordToDomain(&record)
}

func (r *ProviderExecutionRepository) LoadCurrent(ctx context.Context, input domainappdev.LoadCurrentProviderExecutionInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := parseProviderExecutionSpaceID(input.SpaceID)
	if err != nil || !domainappdev.ValidProviderExecutionProjectID(input.ProjectID) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if r == nil || r.db == nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var record providerExecutionRecord
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND project_id = ? AND observed_state <> ?", spaceID, input.ProjectID, domainappdev.ProviderExecutionObservedCleanupComplete).
		Order("generation DESC").
		Limit(1).
		Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, domainappdev.ErrProviderExecutionNotFound
	}
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	return providerExecutionRecordToDomain(&record)
}

func (r *ProviderExecutionRepository) LoadLatestReadyArtifact(ctx context.Context, input domainappdev.LoadReadyProviderArtifactInput) (*domainappdev.ProviderExecution, error) {
	spaceID, err := parseProviderExecutionSpaceID(input.SpaceID)
	if err != nil || !domainappdev.ValidProviderExecutionProjectID(input.ProjectID) || r == nil || r.db == nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var record providerExecutionRecord
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND project_id = ? AND artifact_status = ? AND artifact_object_key <> ''", spaceID, input.ProjectID, domainappdev.ProviderExecutionArtifactReady).
		Order("artifact_updated_at DESC, generation DESC").
		Limit(1).
		Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, domainappdev.ErrProviderExecutionNotFound
	}
	if result.Error != nil {
		return nil, normalizeProviderExecutionDatabaseError(result.Error)
	}
	return providerExecutionRecordToDomain(&record)
}

func (r *ProviderExecutionRepository) ListRecoverable(ctx context.Context, input domainappdev.ListRecoverableProviderExecutionsInput) ([]*domainappdev.RecoverableProviderExecution, error) {
	spaceID, err := parseProviderExecutionSpaceID(input.SpaceID)
	if err != nil || !domainappdev.ValidProviderExecutionProjectID(input.ProjectID) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	var records []providerExecutionRecord
	err = r.db.WithContext(ctx).Where("space_id = ? AND project_id = ?", spaceID, input.ProjectID).
		Where("observed_state IN ?", domainappdev.ProviderExecutionRecoverableObservedStates()).
		Where("owner_identity_hash IS NULL OR owner_expires_at IS NULL OR owner_expires_at <= ?", r.clock.nowExpression()).
		Order("updated_at ASC, id ASC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	items := make([]*domainappdev.RecoverableProviderExecution, 0, len(records))
	for index := range records {
		entity, convertErr := providerExecutionRecordToDomain(&records[index])
		if convertErr != nil {
			return nil, convertErr
		}
		items = append(items, &domainappdev.RecoverableProviderExecution{
			ID: entity.ID, SpaceID: entity.SpaceID, ProjectID: entity.ProjectID, ActorUserID: entity.ActorUserID, Generation: entity.Generation,
			HasProviderExecution: entity.ProviderExecutionID != "", HasCheckpoint: entity.CheckpointEnvelope != "",
			DesiredState: entity.DesiredState, ObservedState: entity.ObservedState, ProviderKey: entity.ProviderKey,
			ProviderScope: entity.ProviderScope, ProviderLeaseExpiresAt: cloneProviderExecutionTime(entity.ProviderLeaseExpiresAt),
			LaunchState: entity.LaunchState,
			OwnerEpoch:  entity.OwnerEpoch, OwnerExpiresAt: cloneProviderExecutionTime(entity.OwnerExpiresAt),
			PreviewRoute: entity.PreviewRoute, SafeErrorCode: entity.SafeErrorCode, SafeErrorMessage: entity.SafeErrorMessage,
			Version: entity.Version, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
		})
	}
	return items, nil
}

func (r *ProviderExecutionRepository) ownedProviderExecutionQuery(ctx context.Context, spaceID int64, cas domainappdev.ProviderExecutionOwnerCAS) *gorm.DB {
	return r.ownedProviderExecutionQueryOn(r.db, ctx, spaceID, cas)
}

func (r *ProviderExecutionRepository) ownedProviderExecutionQueryOn(
	db *gorm.DB,
	ctx context.Context,
	spaceID int64,
	cas domainappdev.ProviderExecutionOwnerCAS,
) *gorm.DB {
	query := db.WithContext(ctx).Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ? AND version = ?", spaceID, cas.ProjectID, cas.Generation, cas.ExpectedVersion).
		Where("owner_identity_hash = ? AND owner_epoch = ? AND owner_expires_at > ?", cas.OwnerHash.Bytes(), cas.OwnerEpoch, r.clock.nowExpression())
	if cas.ProviderKey != "" {
		query = query.Where("provider_key = ?", cas.ProviderKey)
	}
	if cas.ProviderScope != "" {
		query = query.Where("provider_scope = ?", cas.ProviderScope)
	}
	return query
}

func (r *ProviderExecutionRepository) classifyClaimConflict(ctx context.Context, spaceID int64, input domainappdev.ClaimProviderExecutionInput) error {
	record, err := r.loadRecordByGeneration(ctx, spaceID, input.ProjectID, input.Generation)
	if err != nil {
		return err
	}
	if record.Version != input.ExpectedVersion {
		return domainappdev.ErrProviderExecutionVersionConflict
	}
	dbNow, err := r.clock.read(ctx, r.db)
	if err != nil {
		return normalizeProviderExecutionDatabaseError(err)
	}
	if len(record.OwnerIdentityHash) == 0 || record.OwnerExpiresAt == nil || !record.OwnerExpiresAt.After(dbNow) {
		return domainappdev.ErrProviderExecutionUnavailable
	}
	if input.ExpectedOwnerEpoch > 0 && input.OwnerHash.EqualBytes(record.OwnerIdentityHash) && record.OwnerEpoch == input.ExpectedOwnerEpoch {
		return domainappdev.ErrProviderExecutionUnavailable
	}
	return domainappdev.ErrProviderExecutionOwnerConflict
}

func (r *ProviderExecutionRepository) classifyOwnedConflict(ctx context.Context, spaceID int64, cas domainappdev.ProviderExecutionOwnerCAS) error {
	record, err := r.loadRecordByGeneration(ctx, spaceID, cas.ProjectID, cas.Generation)
	if err != nil {
		return err
	}
	return r.classifyOwnedRecord(ctx, record, cas)
}

func (r *ProviderExecutionRepository) classifyOwnedRecord(ctx context.Context, record *providerExecutionRecord, cas domainappdev.ProviderExecutionOwnerCAS) error {
	if record.Version != cas.ExpectedVersion {
		return domainappdev.ErrProviderExecutionVersionConflict
	}
	return r.classifyOwnedRecordIgnoringVersion(ctx, record, cas)
}

func (r *ProviderExecutionRepository) classifyOwnedRecordIgnoringVersion(ctx context.Context, record *providerExecutionRecord, cas domainappdev.ProviderExecutionOwnerCAS) error {
	dbNow, err := r.clock.read(ctx, r.db)
	if err != nil {
		return normalizeProviderExecutionDatabaseError(err)
	}
	return classifyProviderExecutionOwnerIdentity(record, cas, dbNow)
}

func classifyProviderExecutionOwnerIdentity(record *providerExecutionRecord, cas domainappdev.ProviderExecutionOwnerCAS, dbNow time.Time) error {
	if record == nil || record.ProjectID != cas.ProjectID || record.Generation != cas.Generation {
		return domainappdev.ErrProviderExecutionGenerationConflict
	}
	if cas.ProviderKey != "" && record.ProviderKey != cas.ProviderKey {
		return domainappdev.ErrProviderExecutionGenerationConflict
	}
	if cas.ProviderScope != "" && record.ProviderScope != cas.ProviderScope {
		return domainappdev.ErrProviderExecutionGenerationConflict
	}
	if !cas.OwnerHash.EqualBytes(record.OwnerIdentityHash) || record.OwnerEpoch != cas.OwnerEpoch ||
		record.OwnerExpiresAt == nil || !record.OwnerExpiresAt.After(dbNow) {
		return domainappdev.ErrProviderExecutionOwnerConflict
	}
	return nil
}

func (r *ProviderExecutionRepository) loadByIdempotency(ctx context.Context, spaceID int64, projectID, idempotencyKey string) (*domainappdev.ProviderExecution, error) {
	var record providerExecutionRecord
	if err := r.db.WithContext(ctx).Where("space_id = ? AND project_id = ? AND idempotency_key = ?", spaceID, projectID, idempotencyKey).Take(&record).Error; err != nil {
		return nil, normalizeProviderExecutionLookupError(err)
	}
	return providerExecutionRecordToDomain(&record)
}

func (r *ProviderExecutionRepository) loadByGeneration(ctx context.Context, spaceID int64, projectID string, generation uint64) (*domainappdev.ProviderExecution, error) {
	record, err := r.loadRecordByGeneration(ctx, spaceID, projectID, generation)
	if err != nil {
		return nil, err
	}
	return providerExecutionRecordToDomain(record)
}

func (r *ProviderExecutionRepository) loadRecordByGeneration(ctx context.Context, spaceID int64, projectID string, generation uint64) (*providerExecutionRecord, error) {
	var record providerExecutionRecord
	if err := r.db.WithContext(ctx).Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, projectID, generation).Take(&record).Error; err != nil {
		return nil, normalizeProviderExecutionLookupError(err)
	}
	return &record, nil
}

func providerExecutionRecordToDomain(record *providerExecutionRecord) (*domainappdev.ProviderExecution, error) {
	if record == nil {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	ownerHash, ownerHashValid := providerExecutionOwnerHashFromBytes(record.OwnerIdentityHash)
	if len(record.OwnerIdentityHash) != 0 && !ownerHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	writeHash, writeHashValid := providerExecutionOperationHashFromBytes(record.CheckpointWriteOperationHash)
	if len(record.CheckpointWriteOperationHash) != 0 && !writeHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	lastHash, lastHashValid := providerExecutionOperationHashFromBytes(record.CheckpointLastOperationHash)
	if len(record.CheckpointLastOperationHash) != 0 && !lastHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	cleanupHash, cleanupHashValid := providerExecutionOperationHashFromBytes(record.CleanupOperationHash)
	if len(record.CleanupOperationHash) != 0 && !cleanupHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	terminalHash, terminalHashValid := providerExecutionOperationHashFromBytes(record.TerminalOperationHash)
	if len(record.TerminalOperationHash) != 0 && !terminalHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	releaseHash, releaseHashValid := providerExecutionOperationHashFromBytes(record.ReleaseOwnerOperationHash)
	if len(record.ReleaseOwnerOperationHash) != 0 && !releaseHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	buildHash, buildHashValid := providerExecutionOperationHashFromBytes(record.BuildOperationHash)
	if len(record.BuildOperationHash) != 0 && !buildHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	launchHash, launchHashValid := providerExecutionOperationHashFromBytes(record.LaunchOperationHash)
	if len(record.LaunchOperationHash) != 0 && !launchHashValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	launchDigest, launchDigestValid := providerExecutionLaunchDigestFromBytes(record.LaunchRequestDigest)
	if len(record.LaunchRequestDigest) != 0 && !launchDigestValid {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	entity := &domainappdev.ProviderExecution{
		ID: record.ID, SpaceID: strconv.FormatInt(record.SpaceID, 10), ProjectID: record.ProjectID, ActorUserID: record.ActorUserID,
		Generation: record.Generation, IdempotencyKey: record.IdempotencyKey, DesiredState: record.DesiredState,
		ObservedState: record.ObservedState, ProviderKey: record.ProviderKey, ProviderScope: record.ProviderScope,
		ProviderExecutionID: record.ProviderExecutionID, SubmissionStartedAt: cloneProviderExecutionTime(record.SubmissionStartedAt),
		LaunchState: record.LaunchState, LaunchOperationHash: launchHash,
		LaunchProviderOperationID: record.LaunchProviderOperationID, LaunchRequestDigest: launchDigest,
		LaunchExpiresAt:    cloneProviderExecutionTime(record.LaunchExpiresAt),
		CheckpointEnvelope: record.CheckpointEnvelope, CheckpointWriteRevision: record.CheckpointWriteRevision,
		CheckpointWritePending: record.CheckpointWritePending, CheckpointWriteOperationHash: writeHash,
		CheckpointWriteExpiresAt: cloneProviderExecutionTime(record.CheckpointWriteExpiresAt), CheckpointLastOperationHash: lastHash,
		CleanupOperationHash: cleanupHash, TerminalOperationHash: terminalHash, ReleaseOwnerOperationHash: releaseHash,
		ProviderLeaseExpiresAt: cloneProviderExecutionTime(record.ProviderLeaseExpiresAt),
		OwnerIdentityHash:      ownerHash, OwnerEpoch: record.OwnerEpoch, OwnerExpiresAt: cloneProviderExecutionTime(record.OwnerExpiresAt),
		PreviewRoute: record.PreviewRoute, ArtifactObjectKey: record.ArtifactObjectKey,
		BuildOperationID: record.BuildOperationID, BuildOperationHash: buildHash,
		ArtifactStatus: record.ArtifactStatus, ArtifactKind: record.ArtifactKind, ArtifactDigest: record.ArtifactDigest,
		ArtifactSize: record.ArtifactSize, ArtifactVersion: record.ArtifactVersion,
		BuildStartedAt: cloneProviderExecutionTime(record.BuildStartedAt), ArtifactUpdatedAt: cloneProviderExecutionTime(record.ArtifactUpdatedAt),
		ArtifactSafeErrorCode: record.ArtifactSafeErrorCode, ArtifactSafeErrorMessage: record.ArtifactSafeErrorMessage,
		SafeErrorCode: record.SafeErrorCode, SafeErrorMessage: record.SafeErrorMessage,
		Version: record.Version, CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
	}
	hydrated, err := domainappdev.HydrateProviderExecution(entity)
	if err != nil {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	return hydrated, nil
}

func providerExecutionOwnerHashFromBytes(value []byte) (domainappdev.ProviderExecutionOwnerHash, bool) {
	var hash domainappdev.ProviderExecutionOwnerHash
	if len(value) == 0 {
		return hash, true
	}
	var fixed [sha256.Size]byte
	copy(fixed[:], value)
	valid := subtle.ConstantTimeEq(int32(len(value)), sha256.Size) == 1
	copy(hash[:], fixed[:])
	return hash, valid
}

func providerExecutionOperationHashFromBytes(value []byte) (domainappdev.ProviderExecutionOperationHash, bool) {
	var hash domainappdev.ProviderExecutionOperationHash
	if len(value) == 0 {
		return hash, true
	}
	var fixed [sha256.Size]byte
	copy(fixed[:], value)
	valid := subtle.ConstantTimeEq(int32(len(value)), sha256.Size) == 1
	copy(hash[:], fixed[:])
	return hash, valid
}

func providerExecutionLaunchDigestFromBytes(value []byte) (domainappdev.ProviderExecutionLaunchRequestDigest, bool) {
	var digest domainappdev.ProviderExecutionLaunchRequestDigest
	if len(value) == 0 {
		return digest, true
	}
	var fixed [sha256.Size]byte
	copy(fixed[:], value)
	valid := subtle.ConstantTimeEq(int32(len(value)), sha256.Size) == 1
	copy(digest[:], fixed[:])
	return digest, valid
}

func validateProviderExecutionStart(input domainappdev.EnsureProviderExecutionStartInput) (int64, error) {
	spaceID, err := parseProviderExecutionSpaceID(input.SpaceID)
	if err != nil || !domainappdev.ValidProviderExecutionProjectID(input.ProjectID) || !domainappdev.ValidProviderExecutionID(input.ID) ||
		!domainappdev.ValidProviderExecutionIdempotencyKey(input.IdempotencyKey) ||
		domainsandbox.ValidateProviderKey(input.ProviderKey) != nil || input.ProviderScope != domainsandbox.ScopeAppDev {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	return spaceID, nil
}

func validateProviderExecutionClaim(input domainappdev.ClaimProviderExecutionInput) (int64, error) {
	spaceID, err := parseProviderExecutionSpaceID(input.SpaceID)
	if err != nil || !domainappdev.ValidProviderExecutionProjectID(input.ProjectID) || input.Generation == 0 ||
		input.ExpectedVersion == 0 || input.OwnerHash.IsZero() || !validProviderExecutionLeaseDuration(input.LeaseDuration) {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	return spaceID, nil
}

func validateProviderExecutionOwnerCAS(input domainappdev.ProviderExecutionOwnerCAS) (int64, error) {
	if err := domainappdev.ValidateProviderExecutionOwnerCAS(input); err != nil {
		return 0, err
	}
	return parseProviderExecutionSpaceID(input.SpaceID)
}

func validProviderExecutionLeaseDuration(value time.Duration) bool {
	return value > 0 && value <= time.Hour
}

func providerExecutionSubmissionUpdates(clock providerExecutionDBClock, input domainappdev.SaveProviderSubmissionInput) map[string]any {
	return map[string]any{
		"provider_execution_id":     input.ProviderExecutionID,
		"observed_state":            domainappdev.ProviderExecutionObservedRunning,
		"provider_lease_expires_at": clock.addExpression(input.ProviderLeaseDuration),
		"preview_route":             input.PreviewRoute,
		"version":                   gorm.Expr("version + 1"),
		"updated_at":                clock.nowExpression(),
	}
}

func providerExecutionConflictOr(classified, fallback error) error {
	if classified != nil {
		return classified
	}
	return fallback
}

func parseProviderExecutionSpaceID(value string) (int64, error) {
	if !domainappdev.ValidProviderExecutionSpaceID(value) {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, domainappdev.ErrProviderExecutionInvalid
	}
	return parsed, nil
}

func validProviderExecutionCheckpointEnvelope(value string) bool {
	if value == "" || len(value) > sandboxcontract.MaxExecutionCheckpointEnvelopeBytes || value != strings.TrimSpace(value) {
		return false
	}
	for index := range value {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func validGenericProviderExecutionObservedTarget(state domainappdev.ProviderExecutionObservedState) bool {
	switch state {
	case domainappdev.ProviderExecutionObservedSucceeded,
		domainappdev.ProviderExecutionObservedFailed,
		domainappdev.ProviderExecutionObservedCanceled,
		domainappdev.ProviderExecutionObservedTimedOut:
		return true
	default:
		return false
	}
}

func isProviderExecutionTerminal(state domainappdev.ProviderExecutionObservedState) bool {
	return state == domainappdev.ProviderExecutionObservedSucceeded || state == domainappdev.ProviderExecutionObservedFailed ||
		state == domainappdev.ProviderExecutionObservedCanceled || state == domainappdev.ProviderExecutionObservedTimedOut
}

func providerExecutionObservedPredecessors(target domainappdev.ProviderExecutionObservedState) []domainappdev.ProviderExecutionObservedState {
	states := []domainappdev.ProviderExecutionObservedState{
		domainappdev.ProviderExecutionObservedPending, domainappdev.ProviderExecutionObservedSubmitting,
		domainappdev.ProviderExecutionObservedRunning, domainappdev.ProviderExecutionObservedSucceeded,
		domainappdev.ProviderExecutionObservedFailed, domainappdev.ProviderExecutionObservedCanceled,
		domainappdev.ProviderExecutionObservedTimedOut, domainappdev.ProviderExecutionObservedCleanupPending,
	}
	result := make([]domainappdev.ProviderExecutionObservedState, 0, len(states))
	for _, state := range states {
		if domainappdev.CanAdvanceProviderExecutionObserved(state, target) {
			result = append(result, state)
		}
	}
	return result
}

func runProviderExecutionRetry(ctx context.Context, policy providerExecutionRetryPolicy, operation func() error) error {
	if ctx == nil || operation == nil || policy.attempts <= 0 || policy.delay == nil || policy.wait == nil {
		return domainappdev.ErrProviderExecutionInvalid
	}
	var err error
	for attempt := 0; attempt < policy.attempts; attempt++ {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		err = operation()
		if err == nil || !providerExecutionRetryable(err) || attempt == policy.attempts-1 {
			return err
		}
		if waitErr := policy.wait(ctx, policy.delay(attempt)); waitErr != nil {
			return waitErr
		}
	}
	return err
}

func providerExecutionDuplicate(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) {
		return mysqlError.Number == 1062
	}
	var sqliteError sqlite3.Error
	if errors.As(err, &sqliteError) {
		return sqliteError.Code == sqlite3.ErrConstraint &&
			(sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique || sqliteError.ExtendedCode == sqlite3.ErrConstraintPrimaryKey)
	}
	return false
}

func providerExecutionRetryable(err error) bool {
	var sqliteError sqlite3.Error
	if errors.As(err, &sqliteError) {
		return sqliteError.Code == sqlite3.ErrBusy || sqliteError.Code == sqlite3.ErrLocked
	}
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) {
		return false
	}
	return mysqlError.Number == 1205 || mysqlError.Number == 1213 || string(mysqlError.SQLState[:]) == "40001"
}

func normalizeProviderExecutionLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainappdev.ErrProviderExecutionGenerationConflict
	}
	return normalizeProviderExecutionDatabaseError(err)
}

func normalizeProviderExecutionDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domainappdev.ErrProviderExecutionInvalid) || errors.Is(err, domainappdev.ErrProviderExecutionConflict) ||
		errors.Is(err, domainappdev.ErrProviderExecutionNotFound) || errors.Is(err, domainappdev.ErrProviderExecutionUnavailable) {
		return err
	}
	if providerExecutionDuplicate(err) {
		return domainappdev.ErrProviderExecutionConflict
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return domainappdev.ErrProviderExecutionUnavailable
}

func cloneProviderExecutionTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

type ProviderExecutionRecoveryProject struct {
	SpaceID   string
	ProjectID string
}

type ProviderExecutionRecoveryCursor struct {
	SpaceID   string
	ProjectID string
}

// ListProviderExecutionRecoveryProjects is the production startup scan. It
// emits tenant/project identities only and leaves claim/epoch fencing to the
// application service.
func (repository *ProviderExecutionRepository) ListProviderExecutionRecoveryProjects(
	ctx context.Context,
	cursor *ProviderExecutionRecoveryCursor,
	limit int,
) ([]ProviderExecutionRecoveryProject, *ProviderExecutionRecoveryCursor, error) {
	if repository == nil || repository.db == nil || ctx == nil || limit <= 0 || limit > 1000 {
		return nil, nil, domainappdev.ErrProviderExecutionInvalid
	}
	var cursorSpaceID int64
	if cursor != nil {
		var err error
		cursorSpaceID, err = parseProviderExecutionSpaceID(cursor.SpaceID)
		if err != nil || !domainappdev.ValidProviderExecutionProjectID(cursor.ProjectID) {
			return nil, nil, domainappdev.ErrProviderExecutionInvalid
		}
	}
	var rows []struct {
		SpaceID   int64  `gorm:"column:space_id"`
		ProjectID string `gorm:"column:project_id"`
	}
	states := domainappdev.ProviderExecutionRecoverableObservedStates()
	latest := repository.db.WithContext(ctx).
		Model(&providerExecutionRecord{}).
		Select("space_id, project_id, MAX(generation) AS generation").
		Where("observed_state IN ?", states).
		Group("space_id, project_id")
	nowExpression := repository.clock.nowExpression()
	query := repository.db.WithContext(ctx).
		Model(&providerExecutionRecord{}).
		Select("space_id, project_id").
		Where("(space_id, project_id, generation) IN (?)", latest).
		Where("observed_state IN ?", states).
		Where(
			"(owner_identity_hash IS NULL OR LENGTH(owner_identity_hash) = 0 OR owner_epoch = 0 OR owner_expires_at <= "+nowExpression.SQL+")",
			nowExpression.Vars...,
		).
		Order("space_id ASC, project_id ASC").
		Limit(limit + 1)
	if cursor != nil {
		query = query.Where(
			"(space_id > ? OR (space_id = ? AND project_id > ?))",
			cursorSpaceID, cursorSpaceID, cursor.ProjectID,
		)
	}
	err := query.Scan(&rows).Error
	if err != nil {
		return nil, nil, normalizeProviderExecutionDatabaseError(err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	result := make([]ProviderExecutionRecoveryProject, 0, len(rows))
	for _, row := range rows {
		spaceID := strconv.FormatInt(row.SpaceID, 10)
		if !domainappdev.ValidProviderExecutionSpaceID(spaceID) ||
			!domainappdev.ValidProviderExecutionProjectID(row.ProjectID) {
			return nil, nil, domainappdev.ErrProviderExecutionUnavailable
		}
		result = append(result, ProviderExecutionRecoveryProject{SpaceID: spaceID, ProjectID: row.ProjectID})
	}
	if !hasMore || len(result) == 0 {
		return result, nil, nil
	}
	last := result[len(result)-1]
	return result, &ProviderExecutionRecoveryCursor{SpaceID: last.SpaceID, ProjectID: last.ProjectID}, nil
}
