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
	"errors"
	"fmt"
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
}

type appDevProjectRecord struct {
	ID                string     `gorm:"column:id;primaryKey;size:64"`
	SpaceID           int64      `gorm:"column:space_id;not null;index:idx_appdev_projects_space_updated,priority:1"`
	Name              string     `gorm:"column:name;size:128;not null"`
	Description       string     `gorm:"column:description;size:1024"`
	Prompt            string     `gorm:"column:prompt;type:text"`
	Status            string     `gorm:"column:status;size:32;not null;index"`
	RuntimeStatus     string     `gorm:"column:runtime_status;size:32;not null"`
	PreviewURL        string     `gorm:"column:preview_url;size:2048"`
	LastBuildStatus   string     `gorm:"column:last_build_status;size:32"`
	LastBuildType     string     `gorm:"column:last_build_type;size:32"`
	LastBuildArtifact string     `gorm:"column:last_build_artifact;size:512"`
	LastBuildMessage  string     `gorm:"column:last_build_message;size:2048"`
	LastBuildAt       *time.Time `gorm:"column:last_build_at"`
	SourceObjectKey   string     `gorm:"column:source_object_key;size:512;not null"`
	SourceVersion     int64      `gorm:"column:source_version;not null"`
	SourceUpdatedAt   time.Time  `gorm:"column:source_updated_at;not null"`
	CreatorID         int64      `gorm:"column:creator_id;not null"`
	CreatorName       string     `gorm:"column:creator_name;size:128"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;index:idx_appdev_projects_space_updated,priority:2"`
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
	return &PersistentStore{
		db:      db,
		objects: objects,
		cache:   NewLocalStoreForTest(cacheRoot),
	}
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
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&appDevProjectRecord{}).
		Where("id = ? AND space_id = ? AND source_version = ?", projectID, record.SpaceID, record.SourceVersion).
		Updates(map[string]any{
			"source_object_key": nextObjectKey,
			"source_version":    nextVersion,
			"source_updated_at": now,
			"updated_at":        now,
		})
	if result.Error != nil || result.RowsAffected != 1 {
		_ = s.objects.DeleteObject(ctx, nextObjectKey)
		s.invalidateCache(spaceID, projectID)
		if result.Error != nil {
			return result.Error
		}
		return fmt.Errorf("cannot update stale appdev project version")
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
		ID:                record.ID,
		SpaceID:           strconv.FormatInt(record.SpaceID, 10),
		Name:              record.Name,
		Description:       record.Description,
		Prompt:            record.Prompt,
		Status:            domainappdev.ProjectStatus(record.Status),
		RuntimeStatus:     domainappdev.RuntimeStatus(record.RuntimeStatus),
		PreviewURL:        record.PreviewURL,
		LastBuildStatus:   record.LastBuildStatus,
		LastBuildType:     record.LastBuildType,
		LastBuildArtifact: record.LastBuildArtifact,
		LastBuildMessage:  record.LastBuildMessage,
		SourceUpdatedAt:   record.SourceUpdatedAt,
		CreatorID:         strconv.FormatInt(record.CreatorID, 10),
		CreatorName:       record.CreatorName,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
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
