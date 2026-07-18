/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package appdev

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type PersistentStore struct {
	db                   *gorm.DB
	objects              storage.Storage
	cache                *LocalStore
	projectLocks         sync.Map
	materializedVersions sync.Map

	snapshotRestoreAttemptID    func() (string, error)
	snapshotRestoreTransaction  func(context.Context, *gorm.DB, func(*gorm.DB) error) error
	snapshotRestoreReadProject  func(context.Context, int64, string) (*appDevProjectRecord, error)
	snapshotRestoreBeforeCommit func(*providerSnapshotRestoreObjectCleanupRecord)
	snapshotCleanupAfterClaim   func(*providerSnapshotRestoreObjectCleanupRecord)
	snapshotCleanupDeleteObject func(context.Context, string) error
}

const (
	providerSnapshotRestoreCleanupDelay       = 15 * time.Minute
	providerSnapshotRestoreCleanupClaimLease  = 30 * time.Second
	providerSnapshotRestoreCleanupBaseBackoff = 30 * time.Second
	providerSnapshotRestoreCleanupMaxBackoff  = time.Hour
	providerSnapshotRestoreTombstoneRetention = 24 * time.Hour

	providerSnapshotCleanupStatePending = "pending"
	providerSnapshotCleanupStateClaimed = "claimed"
	providerSnapshotCleanupStateDeleted = "deleted"
)

type providerSnapshotRestoreTime struct {
	Time  time.Time
	Valid bool
}

func (value *providerSnapshotRestoreTime) Scan(source any) error {
	if source == nil {
		value.Time = time.Time{}
		value.Valid = false
		return nil
	}
	if timestamp, ok := source.(time.Time); ok {
		value.Time = timestamp.UTC()
		value.Valid = true
		return nil
	}
	var text string
	switch typed := source.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	default:
		return fmt.Errorf("unsupported snapshot restore timestamp")
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if timestamp, err := time.ParseInLocation(layout, text, time.UTC); err == nil {
			value.Time = timestamp.UTC()
			value.Valid = true
			return nil
		}
	}
	return fmt.Errorf("invalid snapshot restore timestamp")
}

func (value providerSnapshotRestoreTime) Value() (driver.Value, error) {
	if !value.Valid {
		return nil, nil
	}
	return value.Time.UTC(), nil
}

type appDevProjectRecord struct {
	ID                         string                      `gorm:"column:id;primaryKey;size:64"`
	SpaceID                    int64                       `gorm:"column:space_id;not null;index:idx_appdev_projects_space_updated,priority:1"`
	Name                       string                      `gorm:"column:name;size:128;not null"`
	Description                string                      `gorm:"column:description;size:1024"`
	Prompt                     string                      `gorm:"column:prompt;type:text"`
	Status                     string                      `gorm:"column:status;size:32;not null;index"`
	RuntimeStatus              string                      `gorm:"column:runtime_status;size:32;not null"`
	PreviewURL                 string                      `gorm:"column:preview_url;size:2048"`
	LastBuildStatus            string                      `gorm:"column:last_build_status;size:32"`
	LastBuildType              string                      `gorm:"column:last_build_type;size:32"`
	LastBuildArtifact          string                      `gorm:"column:last_build_artifact;size:512"`
	LastBuildMessage           string                      `gorm:"column:last_build_message;size:2048"`
	LastBuildAt                *time.Time                  `gorm:"column:last_build_at"`
	SourceObjectKey            string                      `gorm:"column:source_object_key;size:512;not null"`
	SourceVersion              int64                       `gorm:"column:source_version;not null"`
	SourceUpdatedAt            time.Time                   `gorm:"column:source_updated_at;not null"`
	ArchiveState               string                      `gorm:"column:archive_state;size:16;not null;default:none;index:idx_appdev_projects_archive,priority:1"`
	ArchiveIntentVersion       uint64                      `gorm:"column:archive_intent_version;type:bigint unsigned;not null;default:0"`
	ArchiveOperationHash       []byte                      `gorm:"column:archive_operation_hash;type:binary(32)"`
	ArchiveSourceVersion       int64                       `gorm:"column:archive_source_version;type:bigint unsigned;not null;default:0"`
	ArchiveRuntimeGeneration   uint64                      `gorm:"column:archive_runtime_generation;type:bigint unsigned;not null;default:0"`
	ArchiveStartedAt           providerSnapshotRestoreTime `gorm:"column:archive_started_at;type:datetime(6);index:idx_appdev_projects_archive,priority:2"`
	ArchiveCompletedAt         providerSnapshotRestoreTime `gorm:"column:archive_completed_at;type:datetime(6)"`
	RestoreOperationHash       []byte                      `gorm:"column:restore_operation_hash;type:binary(32)"`
	RestoreParentOperationHash []byte                      `gorm:"column:restore_parent_operation_hash;type:binary(32)"`
	RestoreSnapshotID          string                      `gorm:"column:restore_snapshot_id;size:64"`
	RestorePhase               string                      `gorm:"column:restore_phase;size:24;not null;default:none"`
	RestoreRuntimeGeneration   uint64                      `gorm:"column:restore_runtime_generation;type:bigint unsigned;not null;default:0"`
	RestoreRestartRequired     bool                        `gorm:"column:restore_restart_required;not null;default:false"`
	RestoreSourceVersion       int64                       `gorm:"column:restore_source_version;type:bigint unsigned;not null;default:0"`
	RestoreResultSourceVersion int64                       `gorm:"column:restore_result_source_version;type:bigint unsigned;not null;default:0"`
	RestoreStartedGeneration   uint64                      `gorm:"column:restore_started_generation;type:bigint unsigned;not null;default:0"`
	RestoreSafeErrorCode       string                      `gorm:"column:restore_safe_error_code;size:64"`
	RestoreSafeErrorMessage    string                      `gorm:"column:restore_safe_error_message;size:255"`
	RestoreUpdatedAt           providerSnapshotRestoreTime `gorm:"column:restore_updated_at;type:datetime(6)"`
	CreatorID                  int64                       `gorm:"column:creator_id;not null"`
	CreatorName                string                      `gorm:"column:creator_name;size:128"`
	CreatedAt                  time.Time                   `gorm:"column:created_at;not null"`
	UpdatedAt                  time.Time                   `gorm:"column:updated_at;not null;index:idx_appdev_projects_space_updated,priority:2"`
}

func (*appDevProjectRecord) TableName() string { return "appdev_projects" }

type appDevSnapshotRecord struct {
	ID              string    `gorm:"column:id;primaryKey;size:64"`
	SpaceID         int64     `gorm:"column:space_id;not null;index:idx_appdev_snapshots_project,priority:1"`
	ProjectID       string    `gorm:"column:project_id;size:64;not null;index:idx_appdev_snapshots_project,priority:2"`
	Label           string    `gorm:"column:label;size:128;not null"`
	SourceObjectKey string    `gorm:"column:source_object_key;size:512;not null"`
	SourceVersion   int64     `gorm:"column:source_version;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;index:idx_appdev_snapshots_project,priority:3"`
}

func (*appDevSnapshotRecord) TableName() string { return "appdev_project_snapshots" }

type providerSnapshotRestoreObjectCleanupRecord struct {
	ObjectKey            string                      `gorm:"column:object_key;primaryKey;size:512"`
	SpaceID              int64                       `gorm:"column:space_id;not null;index:idx_appdev_source_cleanup_due,priority:2;index:idx_appdev_source_cleanup_project,priority:1"`
	ProjectID            string                      `gorm:"column:project_id;size:64;not null;index:idx_appdev_source_cleanup_due,priority:3;index:idx_appdev_source_cleanup_project,priority:2"`
	RestoreOperationHash []byte                      `gorm:"column:restore_operation_hash;type:binary(32);not null"`
	SourceVersion        int64                       `gorm:"column:source_version;type:bigint unsigned;not null;index:idx_appdev_source_cleanup_project,priority:3"`
	CleanupAfter         providerSnapshotRestoreTime `gorm:"column:cleanup_after;type:datetime(6);not null;index:idx_appdev_source_cleanup_due,priority:1;index:idx_appdev_source_cleanup_claim,priority:2"`
	CleanupState         string                      `gorm:"column:cleanup_state;size:16;not null;index:idx_appdev_source_cleanup_claim,priority:1;index:idx_appdev_source_cleanup_tombstone,priority:1"`
	ClaimTokenHash       []byte                      `gorm:"column:claim_token_hash;type:binary(32)"`
	ClaimExpiresAt       providerSnapshotRestoreTime `gorm:"column:claim_expires_at;type:datetime(6);index:idx_appdev_source_cleanup_claim,priority:3"`
	AttemptCount         uint32                      `gorm:"column:attempt_count;type:int unsigned;not null"`
	LastAttemptAt        providerSnapshotRestoreTime `gorm:"column:last_attempt_at;type:datetime(6)"`
	DeletedAt            providerSnapshotRestoreTime `gorm:"column:deleted_at;type:datetime(6)"`
	TombstoneExpiresAt   providerSnapshotRestoreTime `gorm:"column:tombstone_expires_at;type:datetime(6);index:idx_appdev_source_cleanup_tombstone,priority:2"`
	CreatedAt            providerSnapshotRestoreTime `gorm:"column:created_at;type:datetime(6);not null"`
	UpdatedAt            providerSnapshotRestoreTime `gorm:"column:updated_at;type:datetime(6);not null"`
}

func (*providerSnapshotRestoreObjectCleanupRecord) TableName() string {
	return "appdev_source_object_cleanups"
}

type appDevRuntimeRecord struct {
	SpaceID        int64      `gorm:"column:space_id;primaryKey"`
	ProjectID      string     `gorm:"column:project_id;size:64;primaryKey"`
	Status         string     `gorm:"column:status;size:32;not null"`
	PreviewURL     string     `gorm:"column:preview_url;size:2048"`
	LastKeepAlive  *time.Time `gorm:"column:last_keep_alive_at"`
	RuntimeMessage string     `gorm:"column:runtime_message;size:1024"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
}

func (*appDevRuntimeRecord) TableName() string { return "appdev_runtime_sessions" }

func NewConfiguredStore(db *gorm.DB, objects storage.Storage) domainappdev.Store {
	if appdevapp.IsAppDevHostExecutionEnabled() && envBool("APP_DEV_LOCAL_STORE_ENABLED") {
		return NewLocalStoreFromEnv()
	}
	return NewPersistentStore(db, objects)
}

func NewPersistentStore(db *gorm.DB, objects storage.Storage) *PersistentStore {
	root := strings.TrimSpace(os.Getenv("APP_DEV_CACHE_ROOT"))
	if root == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			root = filepath.Join(cacheDir, "coze-studio", "appdev-cache")
		} else {
			root = filepath.Join(os.TempDir(), "coze-studio-appdev-cache")
		}
	}
	return NewPersistentStoreForTest(db, objects, root)
}

func NewPersistentStoreForTest(db *gorm.DB, objects storage.Storage, cacheRoot string) *PersistentStore {
	store := &PersistentStore{
		db:      db,
		objects: objects,
		cache:   NewLocalStoreForTest(cacheRoot),
	}
	store.snapshotRestoreAttemptID = newProviderSnapshotRestoreAttemptID
	store.snapshotRestoreTransaction = func(ctx context.Context, database *gorm.DB, callback func(*gorm.DB) error) error {
		return database.WithContext(ctx).Transaction(callback)
	}
	store.snapshotRestoreReadProject = store.readProviderSnapshotRestoreProject
	store.snapshotCleanupDeleteObject = func(ctx context.Context, objectKey string) error {
		return store.objects.DeleteObject(ctx, objectKey)
	}
	return store
}

func (s *PersistentStore) ListProjects(ctx context.Context, spaceID string, keyword string) ([]*domainappdev.Project, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}

	query := s.db.WithContext(ctx).
		Where("space_id = ? AND status <> ?", spaceInt, string(domainappdev.ProjectStatusArchived))
	if normalized := strings.TrimSpace(keyword); normalized != "" {
		like := "%" + normalized + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR prompt LIKE ?", like, like, like)
	}
	var records []*appDevProjectRecord
	if err := query.Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	projects := make([]*domainappdev.Project, 0, len(records))
	for _, record := range records {
		projects = append(projects, projectFromRecord(record))
	}
	return projects, nil
}

func (s *PersistentStore) CreateProject(ctx context.Context, project *domainappdev.Project, initialFiles map[string]string) (*domainappdev.Project, error) {
	return s.createProject(ctx, project, func() (*domainappdev.Project, error) {
		return s.cache.CreateProject(ctx, project, initialFiles)
	})
}

func (s *PersistentStore) ImportProjectArchive(ctx context.Context, project *domainappdev.Project, archive []byte) (*domainappdev.Project, error) {
	return s.createProject(ctx, project, func() (*domainappdev.Project, error) {
		return s.cache.ImportProjectArchive(ctx, project, archive)
	})
}

func (s *PersistentStore) createProject(
	ctx context.Context,
	project *domainappdev.Project,
	materialize func() (*domainappdev.Project, error),
) (*domainappdev.Project, error) {
	if project == nil {
		return nil, fmt.Errorf("appdev project is required")
	}
	spaceInt, err := persistentSpaceID(project.SpaceID)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}

	unlock := s.lockProject(project.SpaceID, project.ID)
	defer unlock()
	created, err := materialize()
	if err != nil {
		return nil, err
	}
	archive, _, err := s.cache.ExportProjectArchive(ctx, project.SpaceID, project.ID)
	if err != nil {
		s.invalidateCache(project.SpaceID, project.ID)
		return nil, err
	}
	objectKey := sourceObjectKey(spaceInt, project.ID, 1)
	if err := s.objects.PutObject(ctx, objectKey, archive); err != nil {
		s.invalidateCache(project.SpaceID, project.ID)
		return nil, fmt.Errorf("store appdev source object: %w", err)
	}
	record, err := projectRecordFromDomain(created, spaceInt, objectKey, 1)
	if err != nil {
		_ = s.objects.DeleteObject(ctx, objectKey)
		s.invalidateCache(project.SpaceID, project.ID)
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		_ = s.objects.DeleteObject(ctx, objectKey)
		s.invalidateCache(project.SpaceID, project.ID)
		return nil, err
	}
	s.materializedVersions.Store(projectCacheKey(project.SpaceID, project.ID), int64(1))
	return projectFromRecord(record), nil
}

func (s *PersistentStore) GetProject(ctx context.Context, spaceID string, projectID string) (*domainappdev.Project, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return nil, err
	}
	return projectFromRecord(record), nil
}

func (s *PersistentStore) UpdateProject(ctx context.Context, spaceID string, projectID string, name string, description string) (*domainappdev.Project, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&appDevProjectRecord{}).
		Where("id = ? AND space_id = ?", projectID, spaceInt).
		Updates(map[string]any{"name": name, "description": description, "updated_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domainappdev.ErrNotFound
	}
	return s.GetProject(ctx, spaceID, projectID)
}

func (s *PersistentStore) ArchiveProject(ctx context.Context, spaceID string, projectID string) (*domainappdev.Project, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	result := s.db.WithContext(ctx).Model(&appDevProjectRecord{}).
		Where("id = ? AND space_id = ?", projectID, spaceInt).
		Updates(map[string]any{
			"status":         string(domainappdev.ProjectStatusArchived),
			"runtime_status": string(domainappdev.RuntimeStatusStopped),
			"preview_url":    "",
			"updated_at":     time.Now().UTC(),
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domainappdev.ErrNotFound
	}
	project, err := s.GetProject(ctx, spaceID, projectID)
	s.invalidateCache(spaceID, projectID)
	return project, err
}

func (s *PersistentStore) LoadProjectArchive(
	ctx context.Context,
	input domainappdev.LoadProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		domainappdev.ValidateLoadProjectArchiveInput(input) != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, domainappdev.ErrProjectArchiveInvalid
	}
	spaceID, err := persistentSpaceID(input.SpaceID)
	if err != nil {
		return nil, domainappdev.ErrProjectArchiveInvalid
	}
	var record appDevProjectRecord
	if err := s.db.WithContext(ctx).
		Where("id = ? AND space_id = ?", input.ProjectID, spaceID).
		Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainappdev.ErrNotFound
		}
		return nil, domainappdev.ErrProjectArchiveUnavailable
	}
	return projectArchiveIntentFromRecord(&record, input.SpaceID)
}

func (s *PersistentStore) ReserveProjectArchive(
	ctx context.Context,
	input domainappdev.ReserveProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		domainappdev.ValidateReserveProjectArchiveInput(input) != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, domainappdev.ErrProjectArchiveInvalid
	}
	spaceID, err := persistentSpaceID(input.SpaceID)
	if err != nil {
		return nil, domainappdev.ErrProjectArchiveInvalid
	}
	var intent *domainappdev.ProjectArchiveIntent
	err = runProviderExecutionRetry(ctx, newProviderExecutionRetryPolicy(), func() error {
		intent = nil
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			record, lockErr := lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
			if lockErr != nil {
				return lockErr
			}
			state := normalizedProjectArchiveState(record.ArchiveState)
			if state == domainappdev.ProjectArchiveStateArchiving ||
				state == domainappdev.ProjectArchiveStateArchived {
				if record.ArchiveSourceVersion != input.ExpectedSourceVersion ||
					record.ArchiveRuntimeGeneration != input.RuntimeGeneration ||
					record.SourceVersion != record.ArchiveSourceVersion {
					return domainappdev.ErrProjectArchiveConflict
				}
				intent, lockErr = projectArchiveIntentFromRecord(record, input.SpaceID)
				return lockErr
			}
			if record.Status == string(domainappdev.ProjectStatusArchived) ||
				record.SourceVersion != input.ExpectedSourceVersion ||
				projectRestoreBlocksArchive(record) {
				return domainappdev.ErrProjectArchiveConflict
			}
			generation, launchPending, generationErr := currentProjectArchiveRuntimeGeneration(
				tx,
				spaceID,
				input.ProjectID,
			)
			if generationErr != nil {
				return generationErr
			}
			if generation != input.RuntimeGeneration || launchPending {
				return domainappdev.ErrProjectArchiveConflict
			}
			nextIntentVersion := record.ArchiveIntentVersion + 1
			identity, identityErr := domainappdev.NewProjectArchiveIntentIdentity(
				input.SpaceID,
				input.ProjectID,
				input.ExpectedSourceVersion,
				input.RuntimeGeneration,
				nextIntentVersion,
			)
			if identityErr != nil {
				return domainappdev.ErrProjectArchiveInvalid
			}
			now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
			if nowErr != nil {
				return nowErr
			}
			result := tx.Model(&appDevProjectRecord{}).
				Where(
					"id = ? AND space_id = ? AND source_version = ? AND archive_intent_version = ? AND archive_state IN ?",
					input.ProjectID,
					spaceID,
					input.ExpectedSourceVersion,
					record.ArchiveIntentVersion,
					[]string{"", string(domainappdev.ProjectArchiveStateNone)},
				).
				Updates(map[string]any{
					"archive_state":              string(domainappdev.ProjectArchiveStateArchiving),
					"archive_intent_version":     nextIntentVersion,
					"archive_operation_hash":     identity.OperationHash.Bytes(),
					"archive_source_version":     input.ExpectedSourceVersion,
					"archive_runtime_generation": input.RuntimeGeneration,
					"archive_started_at":         now,
					"archive_completed_at":       nil,
					"updated_at":                 now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return domainappdev.ErrProjectArchiveConflict
			}
			record, lockErr = lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
			if lockErr == nil {
				intent, lockErr = projectArchiveIntentFromRecord(record, input.SpaceID)
			}
			return lockErr
		})
	})
	if err != nil {
		return nil, projectArchiveRepositoryError(ctx, err)
	}
	return intent, nil
}

func (s *PersistentStore) CompleteProjectArchive(
	ctx context.Context,
	input domainappdev.CompleteProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		domainappdev.ValidateCompleteProjectArchiveInput(input) != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, domainappdev.ErrProjectArchiveInvalid
	}
	spaceID, err := persistentSpaceID(input.Intent.SpaceID)
	if err != nil {
		return nil, domainappdev.ErrProjectArchiveInvalid
	}
	var completed *domainappdev.ProjectArchiveIntent
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, lockErr := lockProviderSnapshotRestoreProject(
			tx,
			spaceID,
			input.Intent.ProjectID,
		)
		if lockErr != nil {
			return lockErr
		}
		state := normalizedProjectArchiveState(record.ArchiveState)
		if state == domainappdev.ProjectArchiveStateArchived {
			completed, lockErr = projectArchiveIntentFromRecord(record, input.Intent.SpaceID)
			if lockErr != nil ||
				completed.IntentVersion != input.Intent.IntentVersion ||
				completed.OperationHash != input.Intent.OperationHash {
				return domainappdev.ErrProjectArchiveConflict
			}
			return nil
		}
		if state != domainappdev.ProjectArchiveStateArchiving ||
			record.ArchiveIntentVersion != input.Intent.IntentVersion ||
			record.ArchiveSourceVersion != input.Intent.SourceVersion ||
			record.ArchiveRuntimeGeneration != input.Intent.RuntimeGeneration ||
			record.SourceVersion != input.Intent.SourceVersion ||
			!input.Intent.OperationHash.EqualBytes(record.ArchiveOperationHash) {
			return domainappdev.ErrProjectArchiveConflict
		}
		generation, launchPending, generationErr := currentProjectArchiveRuntimeGeneration(
			tx,
			spaceID,
			input.Intent.ProjectID,
		)
		if generationErr != nil {
			return generationErr
		}
		if generation != 0 || launchPending {
			return domainappdev.ErrProjectArchiveConflict
		}
		now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
		if nowErr != nil {
			return domainappdev.ErrProjectArchiveUnavailable
		}
		result := tx.Model(&appDevProjectRecord{}).
			Where(
				"id = ? AND space_id = ? AND source_version = ? AND archive_state = ? AND archive_intent_version = ? AND archive_operation_hash = ?",
				input.Intent.ProjectID,
				spaceID,
				input.Intent.SourceVersion,
				string(domainappdev.ProjectArchiveStateArchiving),
				input.Intent.IntentVersion,
				input.Intent.OperationHash.Bytes(),
			).
			Updates(map[string]any{
				"status":               string(domainappdev.ProjectStatusArchived),
				"runtime_status":       string(domainappdev.RuntimeStatusStopped),
				"preview_url":          "",
				"archive_state":        string(domainappdev.ProjectArchiveStateArchived),
				"archive_completed_at": now,
				"updated_at":           now,
			})
		if result.Error != nil {
			return domainappdev.ErrProjectArchiveUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProjectArchiveConflict
		}
		record, lockErr = lockProviderSnapshotRestoreProject(tx, spaceID, input.Intent.ProjectID)
		if lockErr == nil {
			completed, lockErr = projectArchiveIntentFromRecord(record, input.Intent.SpaceID)
		}
		return lockErr
	})
	if err != nil {
		return nil, projectArchiveRepositoryError(ctx, err)
	}
	s.invalidateCache(input.Intent.SpaceID, input.Intent.ProjectID)
	return completed, nil
}

func (s *PersistentStore) ExportProjectArchive(ctx context.Context, spaceID string, projectID string) ([]byte, string, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return nil, "", err
	}
	content, err := s.objects.GetObject(ctx, record.SourceObjectKey)
	if err != nil {
		return nil, "", fmt.Errorf("load appdev source object: %w", err)
	}
	if len(content) > appDevMaxArchiveBytes {
		return nil, "", fmt.Errorf("project archive cannot exceed 100MB")
	}
	return content, safeArchiveName(record.Name), nil
}

func (s *PersistentStore) ListFiles(ctx context.Context, spaceID string, projectID string) ([]*domainappdev.FileNode, error) {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	if _, err := s.ensureMaterializedLocked(ctx, spaceID, projectID); err != nil {
		return nil, err
	}
	return s.cache.ListFiles(ctx, spaceID, projectID)
}

func (s *PersistentStore) GetFileContent(ctx context.Context, spaceID string, projectID string, filePath string) (*domainappdev.FileContent, error) {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	if _, err := s.ensureMaterializedLocked(ctx, spaceID, projectID); err != nil {
		return nil, err
	}
	return s.cache.GetFileContent(ctx, spaceID, projectID, filePath)
}

func (s *PersistentStore) SaveFileContent(ctx context.Context, spaceID string, projectID string, filePath string, content string) (*domainappdev.FileContent, error) {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	record, err := s.ensureMaterializedLocked(ctx, spaceID, projectID)
	if err != nil {
		return nil, err
	}
	saved, err := s.cache.SaveFileContent(ctx, spaceID, projectID, filePath, content)
	if err != nil {
		return nil, err
	}
	if err := s.commitLocalSourceLocked(ctx, spaceID, projectID, record); err != nil {
		return nil, err
	}
	return saved, nil
}

func (s *PersistentStore) SaveUploadedFiles(ctx context.Context, spaceID string, projectID string, files []domainappdev.UploadedFile) (int, error) {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	record, err := s.ensureMaterializedLocked(ctx, spaceID, projectID)
	if err != nil {
		return 0, err
	}
	uploaded, err := s.cache.SaveUploadedFiles(ctx, spaceID, projectID, files)
	if err != nil {
		return 0, err
	}
	if err := s.commitLocalSourceLocked(ctx, spaceID, projectID, record); err != nil {
		return 0, err
	}
	return uploaded, nil
}

func (s *PersistentStore) DeletePath(ctx context.Context, spaceID string, projectID string, filePath string) error {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	record, err := s.ensureMaterializedLocked(ctx, spaceID, projectID)
	if err != nil {
		return err
	}
	if err := s.cache.DeletePath(ctx, spaceID, projectID, filePath); err != nil {
		return err
	}
	return s.commitLocalSourceLocked(ctx, spaceID, projectID, record)
}

func (s *PersistentStore) RenamePath(ctx context.Context, spaceID string, projectID string, sourcePath string, targetPath string) error {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	record, err := s.ensureMaterializedLocked(ctx, spaceID, projectID)
	if err != nil {
		return err
	}
	if err := s.cache.RenamePath(ctx, spaceID, projectID, sourcePath, targetPath); err != nil {
		return err
	}
	return s.commitLocalSourceLocked(ctx, spaceID, projectID, record)
}

func (s *PersistentStore) CreateProjectSnapshot(ctx context.Context, spaceID string, projectID string, label string) (*domainappdev.ProjectSnapshot, error) {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return nil, err
	}
	archive, err := s.objects.GetObject(ctx, record.SourceObjectKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	snapshotID := fmt.Sprintf("snapshot_%d", now.UnixNano())
	objectKey := snapshotObjectKey(record.SpaceID, projectID, snapshotID)
	if err := s.objects.PutObject(ctx, objectKey, archive); err != nil {
		return nil, err
	}
	if strings.TrimSpace(label) == "" {
		label = "手动快照"
	}
	snapshot := &appDevSnapshotRecord{
		ID:              snapshotID,
		SpaceID:         record.SpaceID,
		ProjectID:       projectID,
		Label:           strings.TrimSpace(label),
		SourceObjectKey: objectKey,
		SourceVersion:   record.SourceVersion,
		CreatedAt:       now,
	}
	if err := s.db.WithContext(ctx).Create(snapshot).Error; err != nil {
		_ = s.objects.DeleteObject(ctx, objectKey)
		return nil, err
	}
	return snapshotFromRecord(snapshot), nil
}

func (s *PersistentStore) ListProjectSnapshots(ctx context.Context, spaceID string, projectID string) ([]*domainappdev.ProjectSnapshot, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	var records []*appDevSnapshotRecord
	if err := s.db.WithContext(ctx).
		Where("space_id = ? AND project_id = ?", spaceInt, projectID).
		Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	snapshots := make([]*domainappdev.ProjectSnapshot, 0, len(records))
	for _, record := range records {
		snapshots = append(snapshots, snapshotFromRecord(record))
	}
	return snapshots, nil
}

func (s *PersistentStore) RestoreProjectSnapshot(ctx context.Context, spaceID string, projectID string, snapshotID string) error {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return err
	}
	var snapshot appDevSnapshotRecord
	if err := s.db.WithContext(ctx).
		Where("id = ? AND space_id = ? AND project_id = ?", snapshotID, record.SpaceID, projectID).
		Take(&snapshot).Error; err != nil {
		return persistentNotFound(err)
	}
	archive, err := s.objects.GetObject(ctx, snapshot.SourceObjectKey)
	if err != nil {
		return err
	}
	if len(archive) > appDevMaxArchiveBytes {
		return fmt.Errorf("project archive cannot exceed 100MB")
	}
	if err := s.materializePayloadLocked(ctx, record, archive); err != nil {
		return err
	}
	return s.commitSourcePayloadLocked(ctx, spaceID, projectID, record, archive)
}

func (s *PersistentStore) LoadProviderSnapshotRestore(ctx context.Context, input domainappdev.LoadProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if err := s.providerSnapshotRestoreReady(ctx); err != nil {
		return nil, err
	}
	spaceID, err := persistentSpaceID(input.SpaceID)
	if err != nil || strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.SnapshotID) == "" || input.OperationHash.IsZero() || input.ParentOperationHash.IsZero() {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	var record appDevProjectRecord
	err = s.db.WithContext(ctx).Where("id = ? AND space_id = ?", strings.TrimSpace(input.ProjectID), spaceID).Take(&record).Error
	if err != nil {
		return nil, persistentNotFound(err)
	}
	return providerSnapshotRestoreJournalForLoad(&record, input.SpaceID, input.SnapshotID, input.OperationHash, input.ParentOperationHash)
}

func (s *PersistentStore) ReserveProviderSnapshotRestore(ctx context.Context, input domainappdev.ReserveProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if err := s.providerSnapshotRestoreReady(ctx); err != nil {
		return nil, err
	}
	spaceID, err := persistentSpaceID(input.SpaceID)
	if err != nil || domainappdev.ValidateReserveProviderSnapshotRestoreInput(input) != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	unlock := s.lockProject(input.SpaceID, input.ProjectID)
	defer unlock()
	var journal *domainappdev.ProviderSnapshotRestoreJournal
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var snapshot appDevSnapshotRecord
		if takeErr := tx.Where("id = ? AND space_id = ? AND project_id = ?", input.SnapshotID, spaceID, input.ProjectID).Take(&snapshot).Error; takeErr != nil {
			return persistentNotFound(takeErr)
		}
		record, lockErr := lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
		if lockErr != nil {
			return lockErr
		}
		if projectArchiveBlocksMutation(record) {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		if len(record.RestoreOperationHash) != 0 {
			phase := domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)
			fullMatches := input.OperationHash.EqualBytes(record.RestoreOperationHash)
			parentMatches := input.ParentOperationHash.EqualBytes(record.RestoreParentOperationHash)
			if fullMatches && parentMatches && record.RestoreSnapshotID == input.SnapshotID &&
				record.RestoreRuntimeGeneration == input.RuntimeGeneration && record.RestoreRestartRequired == input.RestartRequired {
				if (phase == domainappdev.ProviderSnapshotRestorePhasePending && record.SourceVersion != record.RestoreSourceVersion) ||
					((phase == domainappdev.ProviderSnapshotRestorePhaseRestored || phase == domainappdev.ProviderSnapshotRestorePhaseStarted) && record.SourceVersion != record.RestoreResultSourceVersion) {
					return domainappdev.ErrProviderSnapshotRestoreConflict
				}
				journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
				return lockErr
			}
			if fullMatches || parentMatches || len(record.RestoreParentOperationHash) != 32 || !providerSnapshotRestoreTerminalPhase(phase) {
				return domainappdev.ErrProviderSnapshotRestoreConflict
			}
		}
		now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
		if nowErr != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		updates := map[string]any{
			"restore_operation_hash": input.OperationHash.Bytes(), "restore_parent_operation_hash": input.ParentOperationHash.Bytes(),
			"restore_snapshot_id":        input.SnapshotID,
			"restore_phase":              string(domainappdev.ProviderSnapshotRestorePhasePending),
			"restore_runtime_generation": input.RuntimeGeneration, "restore_restart_required": input.RestartRequired,
			"restore_source_version": record.SourceVersion, "restore_result_source_version": 0,
			"restore_started_generation": 0, "restore_safe_error_code": "", "restore_safe_error_message": "",
			"restore_updated_at": now, "updated_at": now,
		}
		result := tx.Model(&appDevProjectRecord{}).Where("id = ? AND space_id = ? AND source_version = ?", input.ProjectID, spaceID, record.SourceVersion).Updates(updates)
		if result.Error != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		record, lockErr = lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
		if lockErr == nil {
			journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
		}
		return lockErr
	})
	if err != nil {
		return nil, providerSnapshotRestoreError(ctx, err)
	}
	return journal, nil
}

func (s *PersistentStore) ApplyProviderSnapshotRestore(ctx context.Context, input domainappdev.ApplyProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if err := s.providerSnapshotRestoreReady(ctx); err != nil {
		return nil, err
	}
	spaceID, err := persistentSpaceID(input.SpaceID)
	if err != nil || domainappdev.ValidateApplyProviderSnapshotRestoreInput(input) != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	var snapshot appDevSnapshotRecord
	if err = s.db.WithContext(ctx).Where("id = ? AND space_id = ? AND project_id = ?", input.SnapshotID, spaceID, input.ProjectID).Take(&snapshot).Error; err != nil {
		return nil, persistentNotFound(err)
	}
	archive, err := s.objects.GetObject(ctx, snapshot.SourceObjectKey)
	if err != nil || len(archive) > appDevMaxArchiveBytes {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	attemptID, err := s.snapshotRestoreAttemptID()
	if err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	nextVersion := input.ExpectedSourceVersion + 1
	attemptKey := snapshotRestoreSourceObjectKey(spaceID, input.ProjectID, nextVersion, attemptID)
	now, err := providerSnapshotRestoreDBNow(ctx, s.db)
	if err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	cleanupRecord := &providerSnapshotRestoreObjectCleanupRecord{
		ObjectKey: attemptKey, SpaceID: spaceID, ProjectID: input.ProjectID,
		RestoreOperationHash: input.OperationHash.Bytes(), SourceVersion: nextVersion,
		CleanupAfter: providerSnapshotRestoreTime{Time: now.Add(providerSnapshotRestoreCleanupDelay), Valid: true},
		CleanupState: providerSnapshotCleanupStatePending,
		CreatedAt:    providerSnapshotRestoreTime{Time: now, Valid: true},
		UpdatedAt:    providerSnapshotRestoreTime{Time: now, Valid: true},
	}
	if err := s.db.WithContext(ctx).Create(cleanupRecord).Error; err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	if err := s.objects.PutObject(ctx, attemptKey, archive); err != nil {
		s.cleanupProviderSnapshotRestoreAttempt(context.WithoutCancel(ctx), cleanupRecord)
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	if s.snapshotRestoreBeforeCommit != nil {
		s.snapshotRestoreBeforeCommit(cleanupRecord)
	}
	unlock := s.lockProject(input.SpaceID, input.ProjectID)
	defer unlock()
	var journal *domainappdev.ProviderSnapshotRestoreJournal
	attemptCommitted := false
	err = s.snapshotRestoreTransaction(ctx, s.db, func(tx *gorm.DB) error {
		lockedCleanup, lockErr := lockProviderSnapshotRestoreCleanup(tx, attemptKey)
		if lockErr != nil {
			return lockErr
		}
		if lockedCleanup.CleanupState != providerSnapshotCleanupStatePending ||
			lockedCleanup.SpaceID != spaceID || lockedCleanup.ProjectID != input.ProjectID ||
			lockedCleanup.SourceVersion != nextVersion ||
			!input.OperationHash.EqualBytes(lockedCleanup.RestoreOperationHash) {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		record, lockErr := lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
		if lockErr != nil {
			return lockErr
		}
		if projectArchiveBlocksMutation(record) {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		if !input.OperationHash.EqualBytes(record.RestoreOperationHash) || !input.ParentOperationHash.EqualBytes(record.RestoreParentOperationHash) || record.RestoreSnapshotID != input.SnapshotID {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		phase := domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)
		if phase == domainappdev.ProviderSnapshotRestorePhaseRestored || phase == domainappdev.ProviderSnapshotRestorePhaseStarted || phase == domainappdev.ProviderSnapshotRestorePhaseCompleted {
			if record.SourceVersion != record.RestoreResultSourceVersion || record.RestoreSourceVersion != input.ExpectedSourceVersion {
				return domainappdev.ErrProviderSnapshotRestoreConflict
			}
			journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
			return lockErr
		}
		if phase != domainappdev.ProviderSnapshotRestorePhasePending || record.SourceVersion != input.ExpectedSourceVersion || record.RestoreSourceVersion != input.ExpectedSourceVersion {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		if materializeErr := s.materializePayloadLocked(ctx, record, archive); materializeErr != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
		if nowErr != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		result := tx.Model(&appDevProjectRecord{}).Where(
			"id = ? AND space_id = ? AND source_version = ? AND restore_phase = ? AND restore_operation_hash = ? AND restore_parent_operation_hash = ?",
			input.ProjectID, spaceID, input.ExpectedSourceVersion, string(domainappdev.ProviderSnapshotRestorePhasePending), input.OperationHash.Bytes(), input.ParentOperationHash.Bytes(),
		).Updates(map[string]any{
			"source_object_key": attemptKey, "source_version": nextVersion, "source_updated_at": now,
			"restore_phase": string(domainappdev.ProviderSnapshotRestorePhaseRestored), "restore_result_source_version": nextVersion,
			"restore_updated_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		cleanupResult := tx.Where("object_key = ? AND space_id = ? AND project_id = ? AND restore_operation_hash = ? AND source_version = ? AND cleanup_state = ?",
			attemptKey, spaceID, input.ProjectID, input.OperationHash.Bytes(), nextVersion, providerSnapshotCleanupStatePending).
			Delete(&providerSnapshotRestoreObjectCleanupRecord{})
		if cleanupResult.Error != nil || cleanupResult.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		attemptCommitted = true
		record, lockErr = lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
		if lockErr == nil {
			journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
		}
		return lockErr
	})
	if err != nil {
		authority, authorityErr := s.providerSnapshotRestoreAttemptAuthority(context.WithoutCancel(ctx), cleanupRecord, input)
		if authorityErr == nil && authority.committed != nil {
			s.materializedVersions.Store(projectCacheKey(input.SpaceID, input.ProjectID), authority.committed.ResultSourceVersion)
			return authority.committed, nil
		}
		if authorityErr == nil && authority.unreferenced {
			s.cleanupProviderSnapshotRestoreAttempt(context.WithoutCancel(ctx), cleanupRecord)
		}
		s.invalidateCache(input.SpaceID, input.ProjectID)
		return nil, providerSnapshotRestoreError(ctx, err)
	}
	if !attemptCommitted {
		// A concurrent retry may already have committed the same operation with a
		// different immutable attempt key. This attempt is never shared and is
		// therefore safe to reclaim only after the authoritative read.
		authority, authorityErr := s.providerSnapshotRestoreAttemptAuthority(context.WithoutCancel(ctx), cleanupRecord, input)
		if authorityErr == nil && authority.unreferenced {
			s.cleanupProviderSnapshotRestoreAttempt(context.WithoutCancel(ctx), cleanupRecord)
		}
	}
	s.materializedVersions.Store(projectCacheKey(input.SpaceID, input.ProjectID), journal.ResultSourceVersion)
	return journal, nil
}

func (s *PersistentStore) FailProviderSnapshotRestore(ctx context.Context, input domainappdev.FailProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if err := s.providerSnapshotRestoreReady(ctx); err != nil {
		return nil, err
	}
	if domainappdev.ValidateFailProviderSnapshotRestoreInput(input) != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	spaceID, err := persistentSpaceID(input.SpaceID)
	if err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	unlock := s.lockProject(input.SpaceID, input.ProjectID)
	defer unlock()
	var journal *domainappdev.ProviderSnapshotRestoreJournal
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, lockErr := lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
		if lockErr != nil {
			return lockErr
		}
		if !input.OperationHash.EqualBytes(record.RestoreOperationHash) || !input.ParentOperationHash.EqualBytes(record.RestoreParentOperationHash) ||
			record.RestoreSnapshotID != input.SnapshotID || record.SourceVersion != input.ExpectedSourceVersion || record.RestoreSourceVersion != input.ExpectedSourceVersion {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		phase := domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)
		if phase == domainappdev.ProviderSnapshotRestorePhaseFailed {
			if record.RestoreSafeErrorCode != input.SafeErrorCode || record.RestoreSafeErrorMessage != input.SafeErrorMessage {
				return domainappdev.ErrProviderSnapshotRestoreConflict
			}
			journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
			return lockErr
		}
		if phase != domainappdev.ProviderSnapshotRestorePhasePending || record.RestoreResultSourceVersion != 0 || record.RestoreStartedGeneration != 0 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
		if nowErr != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		result := tx.Model(&appDevProjectRecord{}).Where(
			"id = ? AND space_id = ? AND source_version = ? AND restore_source_version = ? AND restore_phase = ? AND restore_operation_hash = ? AND restore_parent_operation_hash = ?",
			input.ProjectID, spaceID, input.ExpectedSourceVersion, input.ExpectedSourceVersion, string(domainappdev.ProviderSnapshotRestorePhasePending),
			input.OperationHash.Bytes(), input.ParentOperationHash.Bytes(),
		).Updates(map[string]any{
			"restore_phase": string(domainappdev.ProviderSnapshotRestorePhaseFailed), "restore_safe_error_code": input.SafeErrorCode,
			"restore_safe_error_message": input.SafeErrorMessage, "restore_updated_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		record, lockErr = lockProviderSnapshotRestoreProject(tx, spaceID, input.ProjectID)
		if lockErr == nil {
			journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
		}
		return lockErr
	})
	if err != nil {
		return nil, providerSnapshotRestoreError(ctx, err)
	}
	return journal, nil
}

func (s *PersistentStore) MarkProviderSnapshotRestoreStarted(ctx context.Context, input domainappdev.MarkProviderSnapshotRestoreStartedInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if domainappdev.ValidateMarkProviderSnapshotRestoreStartedInput(input) != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	return s.advanceProviderSnapshotRestore(ctx, input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash, input.ExpectedSourceVersion, func(record *appDevProjectRecord) (map[string]any, bool) {
		phase := domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)
		if (phase == domainappdev.ProviderSnapshotRestorePhaseStarted || phase == domainappdev.ProviderSnapshotRestorePhaseCompleted) && record.RestoreStartedGeneration == input.StartedGeneration {
			return nil, true
		}
		if phase != domainappdev.ProviderSnapshotRestorePhaseRestored || !record.RestoreRestartRequired || input.StartedGeneration == 0 {
			return nil, false
		}
		return map[string]any{"restore_phase": string(domainappdev.ProviderSnapshotRestorePhaseStarted), "restore_started_generation": input.StartedGeneration}, true
	})
}

func (s *PersistentStore) CompleteProviderSnapshotRestore(ctx context.Context, input domainappdev.CompleteProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if domainappdev.ValidateCompleteProviderSnapshotRestoreInput(input) != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	return s.advanceProviderSnapshotRestore(ctx, input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash, input.ExpectedSourceVersion, func(record *appDevProjectRecord) (map[string]any, bool) {
		phase := domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)
		if phase == domainappdev.ProviderSnapshotRestorePhaseCompleted {
			return nil, !record.RestoreRestartRequired || record.RestoreStartedGeneration == input.StartedGeneration
		}
		if record.RestoreRestartRequired {
			if phase != domainappdev.ProviderSnapshotRestorePhaseStarted || input.StartedGeneration == 0 || record.RestoreStartedGeneration != input.StartedGeneration {
				return nil, false
			}
		} else if phase != domainappdev.ProviderSnapshotRestorePhaseRestored || input.StartedGeneration != 0 {
			return nil, false
		}
		return map[string]any{"restore_phase": string(domainappdev.ProviderSnapshotRestorePhaseCompleted)}, true
	})
}

func (s *PersistentStore) advanceProviderSnapshotRestore(ctx context.Context, space, project, snapshot string, operationHash domainappdev.ProviderSnapshotRestoreOperationHash, parentOperationHash domainappdev.ProviderSnapshotRestoreParentOperationHash, expectedSourceVersion int64, transition func(*appDevProjectRecord) (map[string]any, bool)) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if err := s.providerSnapshotRestoreReady(ctx); err != nil {
		return nil, err
	}
	if expectedSourceVersion <= 0 {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	spaceID, err := persistentSpaceID(space)
	if err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	unlock := s.lockProject(space, project)
	defer unlock()
	var journal *domainappdev.ProviderSnapshotRestoreJournal
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, lockErr := lockProviderSnapshotRestoreProject(tx, spaceID, project)
		if lockErr != nil {
			return lockErr
		}
		if !operationHash.EqualBytes(record.RestoreOperationHash) || !parentOperationHash.EqualBytes(record.RestoreParentOperationHash) || record.RestoreSnapshotID != snapshot || record.SourceVersion != expectedSourceVersion || record.RestoreResultSourceVersion != expectedSourceVersion {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		updates, valid := transition(record)
		if !valid {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		if len(updates) != 0 {
			now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
			if nowErr != nil {
				return domainappdev.ErrProviderSnapshotRestoreUnavailable
			}
			updates["restore_updated_at"] = now
			updates["updated_at"] = now
			result := tx.Model(&appDevProjectRecord{}).Where("id = ? AND space_id = ? AND source_version = ? AND restore_operation_hash = ? AND restore_parent_operation_hash = ?", project, spaceID, expectedSourceVersion, operationHash.Bytes(), parentOperationHash.Bytes()).Updates(updates)
			if result.Error != nil {
				return domainappdev.ErrProviderSnapshotRestoreUnavailable
			}
			if result.RowsAffected != 1 {
				return domainappdev.ErrProviderSnapshotRestoreConflict
			}
			record, lockErr = lockProviderSnapshotRestoreProject(tx, spaceID, project)
		}
		if lockErr == nil {
			journal, lockErr = providerSnapshotRestoreJournalFromRecord(record, space, operationHash, parentOperationHash)
		}
		return lockErr
	})
	if err != nil {
		return nil, providerSnapshotRestoreError(ctx, err)
	}
	return journal, nil
}

func lockProviderSnapshotRestoreProject(tx *gorm.DB, spaceID int64, projectID string) (*appDevProjectRecord, error) {
	var record appDevProjectRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND space_id = ?", strings.TrimSpace(projectID), spaceID).Take(&record).Error; err != nil {
		return nil, persistentNotFound(err)
	}
	return &record, nil
}

func normalizedProjectArchiveState(value string) domainappdev.ProjectArchiveState {
	state := domainappdev.ProjectArchiveState(value)
	if state == "" {
		return domainappdev.ProjectArchiveStateNone
	}
	return state
}

func projectArchiveIntentFromRecord(
	record *appDevProjectRecord,
	spaceID string,
) (*domainappdev.ProjectArchiveIntent, error) {
	if record == nil {
		return nil, domainappdev.ErrProjectArchiveUnavailable
	}
	state := normalizedProjectArchiveState(record.ArchiveState)
	if state == domainappdev.ProjectArchiveStateNone ||
		record.ArchiveIntentVersion == 0 ||
		len(record.ArchiveOperationHash) == 0 {
		return nil, domainappdev.ErrProjectArchiveNotFound
	}
	identity, err := domainappdev.NewProjectArchiveIntentIdentity(
		spaceID,
		record.ID,
		record.ArchiveSourceVersion,
		record.ArchiveRuntimeGeneration,
		record.ArchiveIntentVersion,
	)
	if err != nil || !identity.OperationHash.EqualBytes(record.ArchiveOperationHash) {
		return nil, domainappdev.ErrProjectArchiveUnavailable
	}
	intent := &domainappdev.ProjectArchiveIntent{
		ProjectArchiveIntentIdentity: identity,
		State:                        state,
	}
	if record.ArchiveStartedAt.Valid {
		intent.StartedAt = record.ArchiveStartedAt.Time.UTC()
	}
	if record.ArchiveCompletedAt.Valid {
		intent.CompletedAt = record.ArchiveCompletedAt.Time.UTC()
	}
	if intent.Validate() != nil {
		return nil, domainappdev.ErrProjectArchiveUnavailable
	}
	return intent, nil
}

func projectRestoreBlocksArchive(record *appDevProjectRecord) bool {
	if record == nil || len(record.RestoreOperationHash) == 0 {
		return false
	}
	return !providerSnapshotRestoreTerminalPhase(
		domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase),
	)
}

func projectArchiveBlocksMutation(record *appDevProjectRecord) bool {
	if record == nil {
		return true
	}
	return record.Status == string(domainappdev.ProjectStatusArchived) ||
		normalizedProjectArchiveState(record.ArchiveState) != domainappdev.ProjectArchiveStateNone
}

func currentProjectArchiveRuntimeGeneration(
	tx *gorm.DB,
	spaceID int64,
	projectID string,
) (uint64, bool, error) {
	if tx == nil {
		return 0, false, domainappdev.ErrProjectArchiveUnavailable
	}
	if !tx.Migrator().HasTable(&providerExecutionRecord{}) {
		return 0, false, nil
	}
	var record struct {
		Generation  uint64                                    `gorm:"column:generation"`
		LaunchState domainappdev.ProviderExecutionLaunchState `gorm:"column:launch_state"`
	}
	err := tx.Table("appdev_provider_executions").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("generation, launch_state").
		Where(
			"space_id = ? AND project_id = ? AND observed_state <> ?",
			spaceID,
			projectID,
			domainappdev.ProviderExecutionObservedCleanupComplete,
		).
		Order("generation DESC").
		Limit(1).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, domainappdev.ErrProjectArchiveUnavailable
	}
	return record.Generation,
		record.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
			record.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted ||
			record.LaunchState == domainappdev.ProviderExecutionLaunchLegacySubmitted ||
			record.LaunchState == domainappdev.ProviderExecutionLaunchQuarantined,
		nil
}

func projectArchiveRepositoryError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	switch {
	case errors.Is(err, domainappdev.ErrNotFound):
		return domainappdev.ErrNotFound
	case errors.Is(err, domainappdev.ErrProjectArchiveInvalid):
		return domainappdev.ErrProjectArchiveInvalid
	case errors.Is(err, domainappdev.ErrProjectArchiveConflict):
		return domainappdev.ErrProjectArchiveConflict
	case errors.Is(err, domainappdev.ErrProjectArchiveNotFound):
		return domainappdev.ErrProjectArchiveNotFound
	default:
		return domainappdev.ErrProjectArchiveUnavailable
	}
}

func providerSnapshotRestoreJournalForLoad(record *appDevProjectRecord, space, snapshot string, operationHash domainappdev.ProviderSnapshotRestoreOperationHash, parentOperationHash domainappdev.ProviderSnapshotRestoreParentOperationHash) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if record == nil || len(record.RestoreOperationHash) == 0 {
		return nil, domainappdev.ErrNotFound
	}
	fullMatches := operationHash.EqualBytes(record.RestoreOperationHash)
	parentMatches := parentOperationHash.EqualBytes(record.RestoreParentOperationHash)
	if fullMatches {
		if !parentMatches || record.RestoreSnapshotID != snapshot {
			return nil, domainappdev.ErrProviderSnapshotRestoreConflict
		}
		return providerSnapshotRestoreJournalFromRecord(record, space, operationHash, parentOperationHash)
	}
	if parentMatches || len(record.RestoreParentOperationHash) != 32 || !providerSnapshotRestoreTerminalPhase(domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)) {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	return nil, domainappdev.ErrNotFound
}

func providerSnapshotRestoreJournalFromRecord(record *appDevProjectRecord, space string, operationHash domainappdev.ProviderSnapshotRestoreOperationHash, parentOperationHash domainappdev.ProviderSnapshotRestoreParentOperationHash) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if record == nil || len(record.RestoreOperationHash) == 0 || !operationHash.EqualBytes(record.RestoreOperationHash) || !parentOperationHash.EqualBytes(record.RestoreParentOperationHash) {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	updatedAt := time.Time{}
	if record.RestoreUpdatedAt.Valid {
		updatedAt = record.RestoreUpdatedAt.Time.UTC()
	}
	journal := &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: space, ProjectID: record.ID, SnapshotID: record.RestoreSnapshotID,
		OperationHash: operationHash, ParentOperationHash: parentOperationHash,
		Phase: domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase), RuntimeGeneration: record.RestoreRuntimeGeneration,
		RestartRequired: record.RestoreRestartRequired, SourceVersion: record.RestoreSourceVersion,
		ResultSourceVersion: record.RestoreResultSourceVersion, StartedGeneration: record.RestoreStartedGeneration,
		SafeErrorCode: record.RestoreSafeErrorCode, SafeErrorMessage: record.RestoreSafeErrorMessage, UpdatedAt: updatedAt,
	}
	if err := journal.Validate(); err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	return journal, nil
}

func providerSnapshotRestoreTerminalPhase(phase domainappdev.ProviderSnapshotRestorePhase) bool {
	return phase == domainappdev.ProviderSnapshotRestorePhaseCompleted || phase == domainappdev.ProviderSnapshotRestorePhaseFailed
}

func providerSnapshotRestoreDBNow(ctx context.Context, tx *gorm.DB) (time.Time, error) {
	clock := providerExecutionDBClock(providerExecutionSQLiteDBClock{})
	if tx != nil && tx.Dialector != nil && tx.Dialector.Name() == "mysql" {
		clock = providerExecutionMySQLDBClock{}
	}
	return clock.read(ctx, tx)
}

func providerSnapshotRestoreError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, domainappdev.ErrNotFound) || errors.Is(err, domainappdev.ErrProviderSnapshotRestoreInvalid) || errors.Is(err, domainappdev.ErrProviderSnapshotRestoreConflict) {
		return err
	}
	return domainappdev.ErrProviderSnapshotRestoreUnavailable
}

func (s *PersistentStore) providerSnapshotRestoreReady(ctx context.Context) error {
	if ctx == nil {
		return domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.available(); err != nil {
		return domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	return nil
}

type providerSnapshotRestoreAttemptAuthority struct {
	committed    *domainappdev.ProviderSnapshotRestoreJournal
	unreferenced bool
}

func (s *PersistentStore) providerSnapshotRestoreAttemptAuthority(ctx context.Context, cleanup *providerSnapshotRestoreObjectCleanupRecord, input domainappdev.ApplyProviderSnapshotRestoreInput) (providerSnapshotRestoreAttemptAuthority, error) {
	if s == nil || cleanup == nil || s.snapshotRestoreReadProject == nil {
		return providerSnapshotRestoreAttemptAuthority{}, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	record, err := s.snapshotRestoreReadProject(ctx, cleanup.SpaceID, cleanup.ProjectID)
	if err == nil && record != nil {
		phase := domainappdev.ProviderSnapshotRestorePhase(record.RestorePhase)
		if record.SourceObjectKey == cleanup.ObjectKey && record.SourceVersion == cleanup.SourceVersion &&
			record.RestoreResultSourceVersion == cleanup.SourceVersion && record.RestoreSourceVersion == input.ExpectedSourceVersion &&
			input.OperationHash.EqualBytes(record.RestoreOperationHash) && input.ParentOperationHash.EqualBytes(record.RestoreParentOperationHash) &&
			record.RestoreSnapshotID == input.SnapshotID &&
			(phase == domainappdev.ProviderSnapshotRestorePhaseRestored || phase == domainappdev.ProviderSnapshotRestorePhaseStarted || phase == domainappdev.ProviderSnapshotRestorePhaseCompleted) {
			journal, journalErr := providerSnapshotRestoreJournalFromRecord(record, input.SpaceID, input.OperationHash, input.ParentOperationHash)
			return providerSnapshotRestoreAttemptAuthority{committed: journal}, journalErr
		}
		if record.SourceObjectKey == cleanup.ObjectKey {
			return providerSnapshotRestoreAttemptAuthority{}, domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
	} else if !errors.Is(err, domainappdev.ErrNotFound) {
		return providerSnapshotRestoreAttemptAuthority{}, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	var references int64
	query := s.db.WithContext(ctx).Model(&appDevProjectRecord{}).Where("source_object_key = ?", cleanup.ObjectKey).Count(&references)
	if query.Error != nil {
		return providerSnapshotRestoreAttemptAuthority{}, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	if references != 0 {
		return providerSnapshotRestoreAttemptAuthority{}, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	return providerSnapshotRestoreAttemptAuthority{unreferenced: true}, nil
}

func (s *PersistentStore) readProviderSnapshotRestoreProject(ctx context.Context, spaceID int64, projectID string) (*appDevProjectRecord, error) {
	if s == nil || s.db == nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	var record appDevProjectRecord
	if err := s.db.WithContext(ctx).Where("id = ? AND space_id = ?", projectID, spaceID).Take(&record).Error; err != nil {
		return nil, persistentNotFound(err)
	}
	return &record, nil
}

type providerSnapshotCleanupClaim struct {
	record    providerSnapshotRestoreObjectCleanupRecord
	tokenHash [sha256.Size]byte
}

func providerSnapshotCleanupHashEqual(stored []byte, expected []byte) bool {
	var storedHash, expectedHash [sha256.Size]byte
	storedLengthMatches := subtle.ConstantTimeEq(int32(len(stored)), sha256.Size)
	expectedLengthMatches := subtle.ConstantTimeEq(int32(len(expected)), sha256.Size)
	copy(storedHash[:], stored)
	copy(expectedHash[:], expected)
	return storedLengthMatches&expectedLengthMatches&subtle.ConstantTimeCompare(storedHash[:], expectedHash[:]) == 1
}

func lockProviderSnapshotRestoreCleanup(tx *gorm.DB, objectKey string) (*providerSnapshotRestoreObjectCleanupRecord, error) {
	var record providerSnapshotRestoreObjectCleanupRecord
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("object_key = ?", objectKey).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	if err != nil {
		return nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	return &record, nil
}

func providerSnapshotCleanupBackoff(attempt uint32) time.Duration {
	delay := providerSnapshotRestoreCleanupBaseBackoff
	for current := uint32(1); current < attempt && delay < providerSnapshotRestoreCleanupMaxBackoff; current++ {
		delay *= 2
		if delay > providerSnapshotRestoreCleanupMaxBackoff {
			return providerSnapshotRestoreCleanupMaxBackoff
		}
	}
	return delay
}

func (s *PersistentStore) claimProviderSnapshotRestoreCleanup(
	ctx context.Context,
	objectKey string,
) (*providerSnapshotCleanupClaim, bool, error) {
	var claim *providerSnapshotCleanupClaim
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, lockErr := lockProviderSnapshotRestoreCleanup(tx, objectKey)
		if lockErr != nil {
			if errors.Is(lockErr, domainappdev.ErrProviderSnapshotRestoreConflict) {
				return nil
			}
			return lockErr
		}
		now, clockErr := providerSnapshotRestoreDBNow(ctx, tx)
		if clockErr != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		switch record.CleanupState {
		case providerSnapshotCleanupStatePending:
			if !record.CleanupAfter.Valid || record.CleanupAfter.Time.After(now) {
				return nil
			}
		case providerSnapshotCleanupStateClaimed:
			if !record.ClaimExpiresAt.Valid || record.ClaimExpiresAt.Time.After(now) {
				return nil
			}
		case providerSnapshotCleanupStateDeleted:
			if record.TombstoneExpiresAt.Valid && !record.TombstoneExpiresAt.Time.After(now) {
				result := tx.Where("object_key = ? AND cleanup_state = ?", objectKey, providerSnapshotCleanupStateDeleted).
					Delete(&providerSnapshotRestoreObjectCleanupRecord{})
				if result.Error != nil {
					return domainappdev.ErrProviderSnapshotRestoreUnavailable
				}
			}
			return nil
		default:
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}

		project, projectErr := lockProviderSnapshotRestoreProject(tx, record.SpaceID, record.ProjectID)
		if projectErr == nil {
			if project.SourceObjectKey == record.ObjectKey {
				result := tx.Where("object_key = ? AND cleanup_state <> ?", objectKey, providerSnapshotCleanupStateDeleted).
					Delete(&providerSnapshotRestoreObjectCleanupRecord{})
				if result.Error != nil {
					return domainappdev.ErrProviderSnapshotRestoreUnavailable
				}
				return nil
			}
		} else if !errors.Is(projectErr, domainappdev.ErrNotFound) {
			return projectErr
		}

		var token [32]byte
		if _, randomErr := io.ReadFull(cryptorand.Reader, token[:]); randomErr != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		tokenHash := sha256.Sum256(token[:])
		expiresAt := now.Add(providerSnapshotRestoreCleanupClaimLease)
		result := tx.Model(&providerSnapshotRestoreObjectCleanupRecord{}).
			Where("object_key = ? AND cleanup_state = ?", objectKey, record.CleanupState).
			Updates(map[string]any{
				"cleanup_state":    providerSnapshotCleanupStateClaimed,
				"claim_token_hash": tokenHash[:],
				"claim_expires_at": expiresAt,
				"attempt_count":    gorm.Expr("attempt_count + 1"),
				"last_attempt_at":  now,
				"updated_at":       now,
			})
		if result.Error != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		record.CleanupState = providerSnapshotCleanupStateClaimed
		record.ClaimTokenHash = append([]byte(nil), tokenHash[:]...)
		record.ClaimExpiresAt = providerSnapshotRestoreTime{Time: expiresAt, Valid: true}
		record.AttemptCount++
		record.LastAttemptAt = providerSnapshotRestoreTime{Time: now, Valid: true}
		record.UpdatedAt = providerSnapshotRestoreTime{Time: now, Valid: true}
		claim = &providerSnapshotCleanupClaim{record: *record, tokenHash: tokenHash}
		claimed = true
		return nil
	})
	if err != nil {
		return nil, false, providerSnapshotRestoreError(ctx, err)
	}
	return claim, claimed, nil
}

func (s *PersistentStore) validateProviderSnapshotCleanupClaim(ctx context.Context, claim *providerSnapshotCleanupClaim) error {
	if claim == nil {
		return domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := lockProviderSnapshotRestoreCleanup(tx, claim.record.ObjectKey)
		if err != nil {
			return err
		}
		now, err := providerSnapshotRestoreDBNow(ctx, tx)
		if err != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if record.CleanupState != providerSnapshotCleanupStateClaimed ||
			!record.ClaimExpiresAt.Valid || !record.ClaimExpiresAt.Time.After(now) ||
			!providerSnapshotCleanupHashEqual(record.ClaimTokenHash, claim.tokenHash[:]) {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		project, err := lockProviderSnapshotRestoreProject(tx, record.SpaceID, record.ProjectID)
		if err == nil && project.SourceObjectKey == record.ObjectKey {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		if err != nil && !errors.Is(err, domainappdev.ErrNotFound) {
			return err
		}
		return nil
	})
}

func (s *PersistentStore) releaseProviderSnapshotCleanupClaim(ctx context.Context, claim *providerSnapshotCleanupClaim) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := lockProviderSnapshotRestoreCleanup(tx, claim.record.ObjectKey)
		if err != nil {
			return err
		}
		if record.CleanupState != providerSnapshotCleanupStateClaimed ||
			!providerSnapshotCleanupHashEqual(record.ClaimTokenHash, claim.tokenHash[:]) {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		now, err := providerSnapshotRestoreDBNow(ctx, tx)
		if err != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		result := tx.Model(&providerSnapshotRestoreObjectCleanupRecord{}).
			Where("object_key = ? AND cleanup_state = ? AND claim_token_hash = ?",
				record.ObjectKey, providerSnapshotCleanupStateClaimed, claim.tokenHash[:]).
			Updates(map[string]any{
				"cleanup_state":        providerSnapshotCleanupStatePending,
				"claim_token_hash":     nil,
				"claim_expires_at":     nil,
				"cleanup_after":        now.Add(providerSnapshotCleanupBackoff(record.AttemptCount)),
				"tombstone_expires_at": nil,
				"updated_at":           now,
			})
		if result.Error != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		return nil
	})
}

func (s *PersistentStore) completeProviderSnapshotCleanupClaim(ctx context.Context, claim *providerSnapshotCleanupClaim) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := lockProviderSnapshotRestoreCleanup(tx, claim.record.ObjectKey)
		if err != nil {
			return err
		}
		if record.CleanupState != providerSnapshotCleanupStateClaimed ||
			!providerSnapshotCleanupHashEqual(record.ClaimTokenHash, claim.tokenHash[:]) {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		project, err := lockProviderSnapshotRestoreProject(tx, record.SpaceID, record.ProjectID)
		if err == nil && project.SourceObjectKey == record.ObjectKey {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		if err != nil && !errors.Is(err, domainappdev.ErrNotFound) {
			return err
		}
		now, err := providerSnapshotRestoreDBNow(ctx, tx)
		if err != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		result := tx.Model(&providerSnapshotRestoreObjectCleanupRecord{}).
			Where("object_key = ? AND cleanup_state = ? AND claim_token_hash = ?",
				record.ObjectKey, providerSnapshotCleanupStateClaimed, claim.tokenHash[:]).
			Updates(map[string]any{
				"cleanup_state":        providerSnapshotCleanupStateDeleted,
				"claim_token_hash":     nil,
				"claim_expires_at":     nil,
				"deleted_at":           now,
				"tombstone_expires_at": now.Add(providerSnapshotRestoreTombstoneRetention),
				"cleanup_after":        now.Add(providerSnapshotRestoreTombstoneRetention),
				"updated_at":           now,
			})
		if result.Error != nil {
			return domainappdev.ErrProviderSnapshotRestoreUnavailable
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProviderSnapshotRestoreConflict
		}
		return nil
	})
}

func (s *PersistentStore) processProviderSnapshotRestoreCleanup(ctx context.Context, objectKey string) (bool, error) {
	claim, claimed, err := s.claimProviderSnapshotRestoreCleanup(ctx, objectKey)
	if err != nil || !claimed {
		return false, err
	}
	if s.snapshotCleanupAfterClaim != nil {
		s.snapshotCleanupAfterClaim(&claim.record)
	}
	if err = s.validateProviderSnapshotCleanupClaim(ctx, claim); err != nil {
		return false, providerSnapshotRestoreError(ctx, err)
	}
	deleteObject := s.snapshotCleanupDeleteObject
	if deleteObject == nil {
		return false, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	if err = deleteObject(ctx, claim.record.ObjectKey); err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.releaseProviderSnapshotCleanupClaim(releaseCtx, claim)
		return false, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = s.completeProviderSnapshotCleanupClaim(finalizeCtx, claim); err != nil {
		return false, providerSnapshotRestoreError(finalizeCtx, err)
	}
	return true, nil
}

func (s *PersistentStore) cleanupProviderSnapshotRestoreAttempt(ctx context.Context, cleanup *providerSnapshotRestoreObjectCleanupRecord) {
	if s == nil || cleanup == nil || s.db == nil || s.objects == nil {
		return
	}
	now, err := providerSnapshotRestoreDBNow(ctx, s.db)
	if err != nil {
		return
	}
	result := s.db.WithContext(ctx).Model(&providerSnapshotRestoreObjectCleanupRecord{}).
		Where("object_key = ? AND space_id = ? AND project_id = ? AND restore_operation_hash = ? AND source_version = ? AND cleanup_state = ?",
			cleanup.ObjectKey, cleanup.SpaceID, cleanup.ProjectID, cleanup.RestoreOperationHash, cleanup.SourceVersion, providerSnapshotCleanupStatePending).
		Updates(map[string]any{"cleanup_after": now, "updated_at": now})
	if result.Error != nil {
		return
	}
	_, _ = s.processProviderSnapshotRestoreCleanup(ctx, cleanup.ObjectKey)
}

// ProviderSnapshotCleanupCursor is an opaque stable ordering position for the
// durable cleanup scheduler. It contains no object key content in formatted
// output and is never exposed outside server-side wiring.
type ProviderSnapshotCleanupCursor struct {
	DueAt     time.Time
	ObjectKey string
}

func (ProviderSnapshotCleanupCursor) String() string {
	return "ProviderSnapshotCleanupCursor{<redacted>}"
}

func (ProviderSnapshotCleanupCursor) GoString() string {
	return "ProviderSnapshotCleanupCursor{<redacted>}"
}

// CleanupDeferredProviderSnapshotRestoreObjects is the compatibility entry
// point for callers that do not retain a cursor.
func (s *PersistentStore) CleanupDeferredProviderSnapshotRestoreObjects(ctx context.Context, limit int) (int, error) {
	cleaned, _, err := s.CleanupDeferredProviderSnapshotRestoreObjectsPage(ctx, nil, limit)
	return cleaned, err
}

// CleanupDeferredProviderSnapshotRestoreObjectsPage processes one bounded,
// ordered page. Claims and authoritative project rows share a database lock
// order, and poison entries never starve later rows or reset the caller cursor.
func (s *PersistentStore) CleanupDeferredProviderSnapshotRestoreObjectsPage(
	ctx context.Context,
	cursor *ProviderSnapshotCleanupCursor,
	limit int,
) (int, *ProviderSnapshotCleanupCursor, error) {
	if s == nil || s.db == nil || s.objects == nil || ctx == nil || ctx.Err() != nil || limit <= 0 || limit > 1000 {
		return 0, nil, domainappdev.ErrProviderSnapshotRestoreInvalid
	}
	now, err := providerSnapshotRestoreDBNow(ctx, s.db)
	if err != nil {
		return 0, nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	dueExpression := "CASE cleanup_state WHEN ? THEN cleanup_after WHEN ? THEN claim_expires_at ELSE tombstone_expires_at END"
	var candidates []providerSnapshotRestoreObjectCleanupRecord
	query := s.db.WithContext(ctx).
		Where("(cleanup_state = ? AND cleanup_after <= ?) OR (cleanup_state = ? AND claim_expires_at <= ?) OR (cleanup_state = ? AND tombstone_expires_at <= ?)",
			providerSnapshotCleanupStatePending, now,
			providerSnapshotCleanupStateClaimed, now,
			providerSnapshotCleanupStateDeleted, now)
	if cursor != nil {
		if cursor.DueAt.IsZero() || strings.TrimSpace(cursor.ObjectKey) == "" {
			return 0, nil, domainappdev.ErrProviderSnapshotRestoreInvalid
		}
		query = query.Where(
			"("+dueExpression+" > ?) OR ("+dueExpression+" = ? AND object_key > ?)",
			providerSnapshotCleanupStatePending, providerSnapshotCleanupStateClaimed, cursor.DueAt.UTC(),
			providerSnapshotCleanupStatePending, providerSnapshotCleanupStateClaimed, cursor.DueAt.UTC(), cursor.ObjectKey,
		)
	}
	err = query.
		Order(clause.Expr{
			SQL: dueExpression + " ASC, object_key ASC",
			Vars: []any{
				providerSnapshotCleanupStatePending,
				providerSnapshotCleanupStateClaimed,
			},
		}).
		Limit(limit + 1).
		Find(&candidates).Error
	if err != nil {
		return 0, nil, domainappdev.ErrProviderSnapshotRestoreUnavailable
	}
	hasMore := len(candidates) > limit
	if hasMore {
		candidates = candidates[:limit]
	}
	cleaned := 0
	var cleanupErrors []error
	for index := range candidates {
		if ctxErr := ctx.Err(); ctxErr != nil {
			cleanupErrors = append(cleanupErrors, ctxErr)
			break
		}
		itemCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		deleted, cleanupErr := s.processProviderSnapshotRestoreCleanup(itemCtx, candidates[index].ObjectKey)
		cancel()
		if cleanupErr != nil {
			cleanupErrors = append(cleanupErrors, cleanupErr)
			continue
		}
		if deleted {
			cleaned++
		}
	}
	var next *ProviderSnapshotCleanupCursor
	if hasMore && len(candidates) != 0 {
		last := candidates[len(candidates)-1]
		next = &ProviderSnapshotCleanupCursor{
			DueAt:     providerSnapshotCleanupDueAt(last),
			ObjectKey: last.ObjectKey,
		}
	}
	return cleaned, next, errors.Join(cleanupErrors...)
}

func providerSnapshotCleanupDueAt(record providerSnapshotRestoreObjectCleanupRecord) time.Time {
	switch record.CleanupState {
	case providerSnapshotCleanupStateClaimed:
		return record.ClaimExpiresAt.Time.UTC()
	case providerSnapshotCleanupStateDeleted:
		return record.TombstoneExpiresAt.Time.UTC()
	default:
		return record.CleanupAfter.Time.UTC()
	}
}

func newProviderSnapshotRestoreAttemptID() (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(cryptorand.Reader, random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}

func (s *PersistentStore) ProjectFilesDir(spaceID string, projectID string) (string, error) {
	unlock := s.lockProject(spaceID, projectID)
	defer unlock()
	if _, err := s.ensureMaterializedLocked(context.Background(), spaceID, projectID); err != nil {
		return "", err
	}
	return s.cache.ProjectFilesDir(spaceID, projectID)
}

func (s *PersistentStore) ProjectSourceURL(ctx context.Context, spaceID string, projectID string) (string, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return "", err
	}
	return s.objects.GetObjectUrl(ctx, record.SourceObjectKey)
}

// LoadProviderRuntimeSourceArtifact returns only the internal capability used
// by server-side orchestration. The digest and size are computed from the
// actual bounded object stream rather than a signed URL or untrusted metadata.
func (s *PersistentStore) LoadProviderRuntimeSourceArtifact(ctx context.Context, spaceID string, projectID string) (appdevapp.ProviderRuntimeSourceArtifact, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return appdevapp.ProviderRuntimeSourceArtifact{}, err
	}
	head, err := s.objects.HeadObject(ctx, record.SourceObjectKey)
	if err != nil || head == nil || head.Size <= 0 || head.Size > appDevMaxArchiveBytes {
		return appdevapp.ProviderRuntimeSourceArtifact{}, fmt.Errorf("load appdev source artifact metadata")
	}
	streaming, ok := s.objects.(storage.StreamingStorage)
	if !ok {
		return appdevapp.ProviderRuntimeSourceArtifact{}, fmt.Errorf("appdev source artifact streaming is unavailable")
	}
	reader, err := streaming.OpenObjectStream(ctx, record.SourceObjectKey)
	if err != nil || reader == nil {
		return appdevapp.ProviderRuntimeSourceArtifact{}, fmt.Errorf("load appdev source artifact stream")
	}
	hasher := sha256.New()
	limited := &io.LimitedReader{R: reader, N: appDevMaxArchiveBytes + 1}
	actualSize, readErr := io.Copy(hasher, limited)
	closeErr := reader.Close()
	if contextErr := ctx.Err(); contextErr != nil {
		return appdevapp.ProviderRuntimeSourceArtifact{}, contextErr
	}
	if readErr != nil || closeErr != nil || actualSize <= 0 || actualSize > appDevMaxArchiveBytes || actualSize != head.Size {
		return appdevapp.ProviderRuntimeSourceArtifact{}, fmt.Errorf("validate appdev source artifact")
	}
	artifact, err := appdevapp.NewProviderRuntimeSourceArtifact(
		record.SourceObjectKey,
		fmt.Sprintf("sha256:%x", hasher.Sum(nil)),
		actualSize,
	)
	if err != nil {
		return appdevapp.ProviderRuntimeSourceArtifact{}, fmt.Errorf("validate appdev source artifact")
	}
	return artifact, nil
}

func (s *PersistentStore) GetBuildArtifact(ctx context.Context, spaceID string, projectID string) ([]byte, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(record.LastBuildArtifact) == "" {
		return nil, fmt.Errorf("请先完成发布构建后再下载产物")
	}
	content, err := s.objects.GetObject(ctx, record.LastBuildArtifact)
	if err != nil {
		return nil, fmt.Errorf("load appdev build artifact: %w", err)
	}
	if len(content) > appDevMaxArchiveBytes {
		return nil, fmt.Errorf("build artifact cannot exceed 100MB")
	}
	return content, nil
}

func (s *PersistentStore) SaveBuildArtifact(ctx context.Context, spaceID string, projectID string, content []byte) (string, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", fmt.Errorf("build artifact cannot be empty")
	}
	if len(content) > appDevMaxArchiveBytes {
		return "", fmt.Errorf("build artifact cannot exceed 100MB")
	}
	objectKey := fmt.Sprintf(
		"appdev/spaces/%d/projects/%s/builds/%020d.zip",
		record.SpaceID,
		safeObjectSegment(projectID),
		time.Now().UTC().UnixNano(),
	)
	if err := s.objects.PutObject(ctx, objectKey, content); err != nil {
		return "", fmt.Errorf("store appdev build artifact: %w", err)
	}
	return objectKey, nil
}

func (s *PersistentStore) UpdateProjectRuntime(ctx context.Context, spaceID string, projectID string, status domainappdev.RuntimeStatus, previewURL string) (*domainappdev.Project, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&appDevProjectRecord{}).
			Where("id = ? AND space_id = ?", projectID, spaceInt).
			Updates(map[string]any{
				"runtime_status": string(status),
				"preview_url":    previewURL,
				"updated_at":     now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domainappdev.ErrNotFound
		}
		runtimeRecord := &appDevRuntimeRecord{
			SpaceID:       spaceInt,
			ProjectID:     projectID,
			Status:        string(status),
			PreviewURL:    previewURL,
			LastKeepAlive: &now,
			UpdatedAt:     now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "space_id"}, {Name: "project_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "preview_url", "last_keep_alive_at", "updated_at"}),
		}).Create(runtimeRecord).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetProject(ctx, spaceID, projectID)
}

func (s *PersistentStore) UpdateProjectBuild(ctx context.Context, spaceID string, projectID string, status string, publishType string, artifactPath string, message string, buildAt string) (*domainappdev.Project, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	buildTime, _ := time.Parse(time.RFC3339, buildAt)
	updates := map[string]any{
		"last_build_status":   status,
		"last_build_type":     publishType,
		"last_build_artifact": artifactPath,
		"last_build_message":  appdevapp.SanitizeAppDevOutput(message),
		"updated_at":          time.Now().UTC(),
	}
	if !buildTime.IsZero() {
		updates["last_build_at"] = buildTime
	}
	result := s.db.WithContext(ctx).Model(&appDevProjectRecord{}).
		Where("id = ? AND space_id = ?", projectID, spaceInt).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domainappdev.ErrNotFound
	}
	return s.GetProject(ctx, spaceID, projectID)
}

func (s *PersistentStore) ensureMaterializedLocked(ctx context.Context, spaceID string, projectID string) (*appDevProjectRecord, error) {
	record, err := s.getProjectRecord(ctx, spaceID, projectID)
	if err != nil {
		return nil, err
	}
	key := projectCacheKey(spaceID, projectID)
	if version, ok := s.materializedVersions.Load(key); ok && version == record.SourceVersion {
		if _, statErr := os.Stat(s.cache.filesDir(spaceID, projectID)); statErr == nil {
			return record, nil
		}
	}
	archive, err := s.objects.GetObject(ctx, record.SourceObjectKey)
	if err != nil {
		return nil, fmt.Errorf("load appdev source object: %w", err)
	}
	if len(archive) > appDevMaxArchiveBytes {
		return nil, fmt.Errorf("project archive cannot exceed 100MB")
	}
	if err := s.materializePayloadLocked(ctx, record, archive); err != nil {
		return nil, err
	}
	s.materializedVersions.Store(key, record.SourceVersion)
	return record, nil
}

func (s *PersistentStore) materializePayloadLocked(ctx context.Context, record *appDevProjectRecord, archive []byte) error {
	spaceID := strconv.FormatInt(record.SpaceID, 10)
	s.invalidateCache(spaceID, record.ID)
	_, err := s.cache.restoreProjectArchive(ctx, projectFromRecord(record), archive)
	return err
}

func (s *PersistentStore) commitLocalSourceLocked(ctx context.Context, spaceID string, projectID string, record *appDevProjectRecord) error {
	archive, _, err := s.cache.ExportProjectArchive(ctx, spaceID, projectID)
	if err != nil {
		s.invalidateCache(spaceID, projectID)
		return err
	}
	return s.commitSourcePayloadLocked(ctx, spaceID, projectID, record, archive)
}

func (s *PersistentStore) commitSourcePayloadLocked(ctx context.Context, spaceID string, projectID string, record *appDevProjectRecord, archive []byte) error {
	nextVersion := record.SourceVersion + 1
	nextObjectKey := sourceObjectKey(record.SpaceID, projectID, nextVersion)
	if err := s.objects.PutObject(ctx, nextObjectKey, archive); err != nil {
		s.invalidateCache(spaceID, projectID)
		return fmt.Errorf("store appdev source object: %w", err)
	}
	updateErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, lockErr := lockProviderSnapshotRestoreProject(tx, record.SpaceID, projectID)
		if lockErr != nil {
			return lockErr
		}
		if projectArchiveBlocksMutation(locked) ||
			locked.SourceVersion != record.SourceVersion {
			return domainappdev.ErrProjectArchiveConflict
		}
		now, nowErr := providerSnapshotRestoreDBNow(ctx, tx)
		if nowErr != nil {
			return domainappdev.ErrProjectArchiveUnavailable
		}
		result := tx.Model(&appDevProjectRecord{}).
			Where(
				"id = ? AND space_id = ? AND source_version = ? AND archive_state IN ?",
				projectID,
				record.SpaceID,
				record.SourceVersion,
				[]string{"", string(domainappdev.ProjectArchiveStateNone)},
			).
			Updates(map[string]any{
				"source_object_key": nextObjectKey,
				"source_version":    nextVersion,
				"source_updated_at": now,
				"updated_at":        now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainappdev.ErrProjectArchiveConflict
		}
		return nil
	})
	if updateErr != nil {
		_ = s.objects.DeleteObject(ctx, nextObjectKey)
		s.invalidateCache(spaceID, projectID)
		if errors.Is(updateErr, domainappdev.ErrProjectArchiveConflict) {
			return domainappdev.ErrProjectArchiveConflict
		}
		return updateErr
	}
	s.materializedVersions.Store(projectCacheKey(spaceID, projectID), nextVersion)
	return nil
}

func (s *PersistentStore) getProjectRecord(ctx context.Context, spaceID string, projectID string) (*appDevProjectRecord, error) {
	spaceInt, err := persistentSpaceID(spaceID)
	if err != nil {
		return nil, err
	}
	if err := s.available(); err != nil {
		return nil, err
	}
	var record appDevProjectRecord
	if err := s.db.WithContext(ctx).
		Where("id = ? AND space_id = ?", strings.TrimSpace(projectID), spaceInt).
		Take(&record).Error; err != nil {
		return nil, persistentNotFound(err)
	}
	return &record, nil
}

func (s *PersistentStore) available() error {
	if s == nil || s.db == nil || s.objects == nil || s.cache == nil {
		return fmt.Errorf("persistent appdev store is not configured")
	}
	return nil
}

func (s *PersistentStore) lockProject(spaceID string, projectID string) func() {
	key := projectCacheKey(spaceID, projectID)
	value, _ := s.projectLocks.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func (s *PersistentStore) invalidateCache(spaceID string, projectID string) {
	key := projectCacheKey(spaceID, projectID)
	s.materializedVersions.Delete(key)
	_ = os.RemoveAll(s.cache.projectDir(spaceID, projectID))
}

func projectRecordFromDomain(project *domainappdev.Project, spaceID int64, objectKey string, version int64) (*appDevProjectRecord, error) {
	creatorID, err := strconv.ParseInt(strings.TrimSpace(project.CreatorID), 10, 64)
	if err != nil || creatorID <= 0 {
		return nil, fmt.Errorf("invalid appdev project creator")
	}
	now := time.Now().UTC()
	if project.CreatedAt.IsZero() {
		project.CreatedAt = now
	}
	if project.UpdatedAt.IsZero() {
		project.UpdatedAt = now
	}
	if project.SourceUpdatedAt.IsZero() {
		project.SourceUpdatedAt = now
	}
	var buildAt *time.Time
	if !project.LastBuildAt.IsZero() {
		value := project.LastBuildAt
		buildAt = &value
	}
	return &appDevProjectRecord{
		ID:                project.ID,
		SpaceID:           spaceID,
		Name:              project.Name,
		Description:       project.Description,
		Prompt:            project.Prompt,
		Status:            string(project.Status),
		RuntimeStatus:     string(project.RuntimeStatus),
		PreviewURL:        project.PreviewURL,
		LastBuildStatus:   project.LastBuildStatus,
		LastBuildType:     project.LastBuildType,
		LastBuildArtifact: project.LastBuildArtifact,
		LastBuildMessage:  project.LastBuildMessage,
		LastBuildAt:       buildAt,
		SourceObjectKey:   objectKey,
		SourceVersion:     version,
		SourceUpdatedAt:   project.SourceUpdatedAt,
		ArchiveState:      string(domainappdev.ProjectArchiveStateNone),
		CreatorID:         creatorID,
		CreatorName:       project.CreatorName,
		CreatedAt:         project.CreatedAt,
		UpdatedAt:         project.UpdatedAt,
	}, nil
}

func projectFromRecord(record *appDevProjectRecord) *domainappdev.Project {
	if record == nil {
		return nil
	}
	project := &domainappdev.Project{
		ID:                       record.ID,
		SpaceID:                  strconv.FormatInt(record.SpaceID, 10),
		Name:                     record.Name,
		Description:              record.Description,
		Prompt:                   record.Prompt,
		Status:                   domainappdev.ProjectStatus(record.Status),
		RuntimeStatus:            domainappdev.RuntimeStatus(record.RuntimeStatus),
		PreviewURL:               record.PreviewURL,
		LastBuildStatus:          record.LastBuildStatus,
		LastBuildType:            record.LastBuildType,
		LastBuildArtifact:        record.LastBuildArtifact,
		LastBuildMessage:         record.LastBuildMessage,
		SourceVersion:            record.SourceVersion,
		SourceUpdatedAt:          record.SourceUpdatedAt,
		ArchiveState:             normalizedProjectArchiveState(record.ArchiveState),
		ArchiveIntentVersion:     record.ArchiveIntentVersion,
		ArchiveSourceVersion:     record.ArchiveSourceVersion,
		ArchiveRuntimeGeneration: record.ArchiveRuntimeGeneration,
		CreatorID:                strconv.FormatInt(record.CreatorID, 10),
		CreatorName:              record.CreatorName,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
	}
	if record.LastBuildAt != nil {
		project.LastBuildAt = *record.LastBuildAt
	}
	return project
}

func snapshotFromRecord(record *appDevSnapshotRecord) *domainappdev.ProjectSnapshot {
	if record == nil {
		return nil
	}
	return &domainappdev.ProjectSnapshot{
		ID:        record.ID,
		Label:     record.Label,
		CreatedAt: record.CreatedAt,
	}
}

func persistentSpaceID(spaceID string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(spaceID), 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid appdev space_id")
	}
	return value, nil
}

func persistentNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainappdev.ErrNotFound
	}
	return err
}

func sourceObjectKey(spaceID int64, projectID string, version int64) string {
	return fmt.Sprintf("appdev/spaces/%d/projects/%s/source/%020d.zip", spaceID, safeObjectSegment(projectID), version)
}

func snapshotRestoreSourceObjectKey(spaceID int64, projectID string, version int64, attemptID string) string {
	return fmt.Sprintf("appdev/spaces/%d/projects/%s/source/%020d/restore-%s.zip", spaceID, safeObjectSegment(projectID), version, safeObjectSegment(attemptID))
}

func snapshotObjectKey(spaceID int64, projectID string, snapshotID string) string {
	return fmt.Sprintf("appdev/spaces/%d/projects/%s/snapshots/%s.zip", spaceID, safeObjectSegment(projectID), safeObjectSegment(snapshotID))
}

func safeObjectSegment(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func projectCacheKey(spaceID string, projectID string) string {
	return strings.TrimSpace(spaceID) + ":" + strings.TrimSpace(projectID)
}

func envBool(name string) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
	return err == nil && value
}
