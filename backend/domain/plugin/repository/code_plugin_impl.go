// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

const (
	codePluginWriteMaxAttempts = 3
	codePluginRetryDelay       = 5 * time.Millisecond
)

type codePluginRepositoryImpl struct {
	db *gorm.DB
}

func NewCodePluginRepository(db *gorm.DB) CodePluginRepository {
	return &codePluginRepositoryImpl{db: db}
}

// pluginDraftLockPO is the stable aggregate parent used to serialize every
// code-plugin write, including the first code-draft creation.
type pluginDraftLockPO struct {
	ID      int64 `gorm:"column:id;primaryKey"`
	SpaceID int64 `gorm:"column:space_id"`
}

func (pluginDraftLockPO) TableName() string { return "plugin_draft" }

type codeDraftPO struct {
	PluginID             int64     `gorm:"column:plugin_id;primaryKey"`
	SpaceID              int64     `gorm:"column:space_id;index:idx_plugin_code_drafts_space,priority:1"`
	Runtime              string    `gorm:"column:runtime"`
	EntryFile            string    `gorm:"column:entry_file"`
	SourceBundleRef      string    `gorm:"column:source_bundle_ref"`
	InputSchemaJSON      string    `gorm:"column:input_schema_json"`
	OutputSchemaJSON     string    `gorm:"column:output_schema_json"`
	Revision             int64     `gorm:"column:revision"`
	LastDebuggedRevision int64     `gorm:"column:last_debugged_revision"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at;index:idx_plugin_code_drafts_space,priority:2"`
}

func (codeDraftPO) TableName() string { return "plugin_code_drafts" }

type codeDraftFilePO struct {
	PluginID  int64     `gorm:"column:plugin_id;primaryKey"`
	Path      string    `gorm:"column:path;primaryKey"`
	Content   []byte    `gorm:"column:content"`
	Size      int64     `gorm:"column:size"`
	SHA256    string    `gorm:"column:sha256"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (codeDraftFilePO) TableName() string { return "plugin_code_draft_files" }

type codeVersionPO struct {
	PluginID         int64     `gorm:"column:plugin_id;primaryKey"`
	Version          string    `gorm:"column:version;primaryKey"`
	SpaceID          int64     `gorm:"column:space_id;index:idx_plugin_code_versions_space,priority:1"`
	Runtime          string    `gorm:"column:runtime"`
	EntryFile        string    `gorm:"column:entry_file"`
	SourceBundleRef  string    `gorm:"column:source_bundle_ref"`
	InputSchemaJSON  string    `gorm:"column:input_schema_json"`
	OutputSchemaJSON string    `gorm:"column:output_schema_json"`
	SourceRevision   int64     `gorm:"column:source_revision"`
	CreatedBy        int64     `gorm:"column:created_by"`
	CreatedAt        time.Time `gorm:"column:created_at;index:idx_plugin_code_versions_space,priority:2"`
}

func (codeVersionPO) TableName() string { return "plugin_code_versions" }

type publishedPluginVersionPO struct {
	PluginID int64  `gorm:"column:plugin_id;primaryKey"`
	Version  string `gorm:"column:version;primaryKey"`
}

func (publishedPluginVersionPO) TableName() string { return "plugin_version" }

type codeVersionFilePO struct {
	PluginID  int64     `gorm:"column:plugin_id;primaryKey"`
	Version   string    `gorm:"column:version;primaryKey"`
	Path      string    `gorm:"column:path;primaryKey"`
	Content   []byte    `gorm:"column:content"`
	Size      int64     `gorm:"column:size"`
	SHA256    string    `gorm:"column:sha256"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (codeVersionFilePO) TableName() string { return "plugin_code_version_files" }

func (r *codePluginRepositoryImpl) GetDraft(ctx context.Context, pluginID int64) (*entity.CodeDraft, bool, error) {
	if pluginID <= 0 {
		return nil, false, fmt.Errorf("plugin id is required")
	}
	var result *entity.CodeDraft
	var exists bool
	err := r.consistentRead(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := readPluginDraftParentSpaces(tx, pluginID)
		if err != nil {
			return err
		}
		var draft codeDraftPO
		err = tx.Where("plugin_id = ?", pluginID).Take(&draft).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if draft.SpaceID != parentSpaces[pluginID] {
			return ErrCodeSpaceMismatch
		}
		files, err := getDraftFiles(tx, pluginID)
		if err != nil {
			return err
		}
		result, err = entity.PrepareCodeDraft(draft.toEntity(files))
		if err != nil {
			return fmt.Errorf("invalid persisted code draft: %w", err)
		}
		exists = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, exists, nil
}

func (r *codePluginRepositoryImpl) SaveDraftCAS(ctx context.Context, draft *entity.CodeDraft, expectedRevision int64) (*entity.CodeDraft, error) {
	prepared, err := entity.PrepareCodeDraft(draft)
	if err != nil {
		return nil, err
	}
	if expectedRevision < 0 {
		return nil, fmt.Errorf("expected revision cannot be negative")
	}

	var saved *entity.CodeDraft
	err = r.writeTransaction(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := lockPluginDraftParents(tx, prepared.PluginID)
		if err != nil {
			return err
		}
		if prepared.SpaceID != parentSpaces[prepared.PluginID] {
			return ErrCodeSpaceMismatch
		}
		existing, exists, err := lockCodeDraft(tx, prepared.PluginID)
		if err != nil {
			return err
		}
		if !exists {
			if expectedRevision != 0 {
				return ErrCodeDraftConflict
			}
			now := time.Now().UTC()
			po := draftToPO(prepared)
			po.Revision = 1
			po.LastDebuggedRevision = 0
			po.CreatedAt = now
			po.UpdatedAt = now
			if err := tx.Create(po).Error; err != nil {
				return mapCodeDraftCreateError(err)
			}
			if err := replaceDraftFiles(tx, prepared.PluginID, prepared.Files, now); err != nil {
				return err
			}
			saved = po.toEntity(entity.CloneCodeFiles(prepared.Files))
			return nil
		}
		if expectedRevision != existing.Revision {
			return ErrCodeDraftConflict
		}
		if existing.SpaceID != parentSpaces[prepared.PluginID] {
			return ErrCodeSpaceMismatch
		}

		nextRevision := existing.Revision + 1
		now := time.Now().UTC()
		result := tx.Model(&codeDraftPO{}).
			Where("plugin_id = ?", prepared.PluginID).
			Updates(map[string]any{
				"space_id":               prepared.SpaceID,
				"runtime":                string(prepared.Runtime),
				"entry_file":             prepared.EntryFile,
				"source_bundle_ref":      prepared.SourceBundleRef,
				"input_schema_json":      prepared.InputSchemaJSON,
				"output_schema_json":     prepared.OutputSchemaJSON,
				"revision":               nextRevision,
				"last_debugged_revision": existing.LastDebuggedRevision,
				"updated_at":             now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCodeDraftConflict
		}
		if err := replaceDraftFiles(tx, prepared.PluginID, prepared.Files, now); err != nil {
			return err
		}
		prepared.Revision = nextRevision
		prepared.LastDebuggedRevision = existing.LastDebuggedRevision
		saved = cloneDraft(prepared)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func (r *codePluginRepositoryImpl) MarkDebuggedCAS(ctx context.Context, pluginID, revision int64) error {
	if pluginID <= 0 || revision <= 0 {
		return fmt.Errorf("plugin id and revision are required")
	}
	return r.writeTransaction(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := lockPluginDraftParents(tx, pluginID)
		if err != nil {
			return err
		}
		draft, exists, err := lockCodeDraft(tx, pluginID)
		if err != nil {
			return err
		}
		if !exists || draft.Revision != revision {
			return ErrCodeDraftConflict
		}
		if draft.SpaceID != parentSpaces[pluginID] {
			return ErrCodeSpaceMismatch
		}
		if draft.LastDebuggedRevision == revision {
			return nil
		}
		result := tx.Model(&codeDraftPO{}).
			Where("plugin_id = ?", pluginID).
			Updates(map[string]any{
				"last_debugged_revision": revision,
				"updated_at":             time.Now().UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCodeDraftConflict
		}
		return nil
	})
}

func (r *codePluginRepositoryImpl) PublishDebuggedVersion(ctx context.Context, pluginID int64, version string, operatorID int64) error {
	_, err := r.PrepareDebuggedVersion(ctx, pluginID, version, operatorID)
	return err
}

func (r *codePluginRepositoryImpl) PrepareDebuggedVersion(
	ctx context.Context,
	pluginID int64,
	version string,
	operatorID int64,
) (*PreparedCodeVersion, error) {
	version = strings.TrimSpace(version)
	if pluginID <= 0 || version == "" || operatorID <= 0 {
		return nil, fmt.Errorf("plugin id, version and operator id are required")
	}
	if len(version) > entity.MaxCodeVersionLength {
		return nil, fmt.Errorf("version exceeds %d characters", entity.MaxCodeVersionLength)
	}
	var prepared *PreparedCodeVersion
	err := r.writeTransaction(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := lockPluginDraftParents(tx, pluginID)
		if err != nil {
			return err
		}
		parentSpace := parentSpaces[pluginID]
		published, err := mainPluginVersionExists(tx, pluginID, version)
		if err != nil {
			return err
		}
		if published {
			var publishedVersion codeVersionPO
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("plugin_id = ? AND version = ?", pluginID, version).
				Take(&publishedVersion).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCodeVersionExists
			}
			if err != nil {
				return err
			}
			if publishedVersion.SpaceID != parentSpace {
				return ErrCodeSpaceMismatch
			}
			prepared = &PreparedCodeVersion{
				Version: publishedVersion.toEntity(nil),
				Created: false,
				State:   CodeVersionAlreadyPublished,
			}
			return nil
		}

		draft, exists, err := lockCodeDraft(tx, pluginID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrCodeDraftNotFound
		}
		if draft.SpaceID != parentSpace {
			return ErrCodeSpaceMismatch
		}

		files, err := getDraftFiles(tx, pluginID)
		if err != nil {
			return err
		}
		snapshot, err := entity.PrepareCodeDraft(draft.toEntity(files))
		if err != nil {
			return fmt.Errorf("invalid locked code draft: %w", err)
		}
		if draft.Revision <= 0 ||
			snapshot.SourceBundleRef == "" ||
			draft.LastDebuggedRevision != draft.Revision {
			return ErrCodeDraftNotDebugged
		}
		published, err = mainPluginVersionExists(tx, pluginID, version)
		if err != nil {
			return err
		}
		target := &entity.CodeVersion{
			PluginID:         snapshot.PluginID,
			Version:          version,
			SpaceID:          snapshot.SpaceID,
			Runtime:          snapshot.Runtime,
			EntryFile:        snapshot.EntryFile,
			SourceBundleRef:  snapshot.SourceBundleRef,
			InputSchemaJSON:  snapshot.InputSchemaJSON,
			OutputSchemaJSON: snapshot.OutputSchemaJSON,
			SourceRevision:   draft.Revision,
			CreatedBy:        operatorID,
			Files:            entity.CloneCodeFiles(snapshot.Files),
		}

		var existingVersion codeVersionPO
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("plugin_id = ? AND version = ?", pluginID, version).
			Take(&existingVersion).Error
		if err == nil {
			if existingVersion.SpaceID != parentSpace {
				return ErrCodeSpaceMismatch
			}
			if existingVersion.SourceRevision == draft.Revision &&
				existingVersion.SourceBundleRef == snapshot.SourceBundleRef {
				state := CodeVersionRecovered
				if published {
					state = CodeVersionAlreadyPublished
				}
				prepared = &PreparedCodeVersion{Version: target, Created: false, State: state}
				return nil
			}
			if published {
				return ErrCodeVersionExists
			}
			if err := tx.Where("plugin_id = ? AND version = ?", pluginID, version).
				Delete(&codeVersionPO{}).Error; err != nil {
				return err
			}
			err = gorm.ErrRecordNotFound
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if published {
			return ErrCodeVersionExists
		}
		if err := createCodeVersion(tx, target); err != nil {
			return err
		}
		prepared = &PreparedCodeVersion{
			Version: cloneCodeVersion(target),
			Created: true,
			State:   CodeVersionPrepared,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func (r *codePluginRepositoryImpl) CompensatePreparedVersion(
	ctx context.Context,
	prepared *PreparedCodeVersion,
) (bool, error) {
	if prepared == nil || prepared.Version == nil {
		return false, nil
	}
	version := prepared.Version
	var published bool
	err := r.writeTransaction(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := lockPluginDraftParents(tx, version.PluginID)
		if err != nil {
			return err
		}
		if version.SpaceID != parentSpaces[version.PluginID] {
			return ErrCodeSpaceMismatch
		}
		published, err = mainPluginVersionExists(tx, version.PluginID, version.Version)
		if err != nil || published {
			return err
		}
		if !prepared.Created {
			return nil
		}
		var existing codeVersionPO
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("plugin_id = ? AND version = ?", version.PluginID, version.Version).
			Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if existing.SpaceID != parentSpaces[version.PluginID] {
			return ErrCodeSpaceMismatch
		}
		if existing.SourceRevision != version.SourceRevision ||
			existing.SourceBundleRef != version.SourceBundleRef {
			return nil
		}
		return tx.Where("plugin_id = ? AND version = ?", version.PluginID, version.Version).
			Delete(&codeVersionPO{}).Error
	})
	return published, err
}

func (r *codePluginRepositoryImpl) EnsurePublishedVersion(
	ctx context.Context,
	prepared *PreparedCodeVersion,
) error {
	if prepared == nil || prepared.Version == nil {
		return fmt.Errorf("prepared code version is required")
	}
	version := prepared.Version
	return r.writeTransaction(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := lockPluginDraftParents(tx, version.PluginID)
		if err != nil {
			return err
		}
		if version.SpaceID != parentSpaces[version.PluginID] {
			return ErrCodeSpaceMismatch
		}
		published, err := mainPluginVersionExists(tx, version.PluginID, version.Version)
		if err != nil {
			return err
		}
		if !published {
			return ErrCodeMainVersionMissing
		}
		var existing codeVersionPO
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("plugin_id = ? AND version = ?", version.PluginID, version.Version).
			Take(&existing).Error
		if err == nil {
			if existing.SpaceID != parentSpaces[version.PluginID] {
				return ErrCodeSpaceMismatch
			}
			if existing.SourceRevision == version.SourceRevision &&
				existing.SourceBundleRef == version.SourceBundleRef {
				return nil
			}
			return ErrCodeVersionExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return createCodeVersion(tx, version)
	})
}

func mainPluginVersionExists(tx *gorm.DB, pluginID int64, version string) (bool, error) {
	var marker publishedPluginVersionPO
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("plugin_id = ? AND version = ?", pluginID, version).
		Take(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func createCodeVersion(tx *gorm.DB, version *entity.CodeVersion) error {
	now := time.Now().UTC()
	po := &codeVersionPO{
		PluginID:         version.PluginID,
		Version:          version.Version,
		SpaceID:          version.SpaceID,
		Runtime:          string(version.Runtime),
		EntryFile:        version.EntryFile,
		SourceBundleRef:  version.SourceBundleRef,
		InputSchemaJSON:  version.InputSchemaJSON,
		OutputSchemaJSON: version.OutputSchemaJSON,
		SourceRevision:   version.SourceRevision,
		CreatedBy:        version.CreatedBy,
		CreatedAt:        now,
	}
	if err := tx.Create(po).Error; err != nil {
		if isUniqueConstraintError(err) {
			return ErrCodeVersionExists
		}
		return err
	}
	files := make([]*codeVersionFilePO, 0, len(version.Files))
	for _, file := range version.Files {
		files = append(files, &codeVersionFilePO{
			PluginID:  version.PluginID,
			Version:   version.Version,
			Path:      file.Path,
			Content:   append([]byte(nil), file.Content...),
			Size:      file.Size,
			SHA256:    file.SHA256,
			CreatedAt: now,
		})
	}
	return tx.Create(&files).Error
}

func cloneCodeVersion(version *entity.CodeVersion) *entity.CodeVersion {
	if version == nil {
		return nil
	}
	cloned := *version
	cloned.Files = entity.CloneCodeFiles(version.Files)
	return &cloned
}

func (r *codePluginRepositoryImpl) GetVersion(ctx context.Context, pluginID int64, version string) (*entity.CodeVersion, bool, error) {
	version = strings.TrimSpace(version)
	if pluginID <= 0 || version == "" {
		return nil, false, fmt.Errorf("plugin id and version are required")
	}
	var result *entity.CodeVersion
	var exists bool
	err := r.consistentRead(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := readPluginDraftParentSpaces(tx, pluginID)
		if err != nil {
			return err
		}
		var versionPO codeVersionPO
		err = tx.Where("plugin_id = ? AND version = ?", pluginID, version).
			Take(&versionPO).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if versionPO.SpaceID != parentSpaces[pluginID] {
			return ErrCodeSpaceMismatch
		}
		files, err := getVersionFiles(tx, pluginID, versionPO.Version)
		if err != nil {
			return err
		}
		result, err = hydrateCodeVersion(&versionPO, files)
		if err != nil {
			return fmt.Errorf("invalid persisted code version: %w", err)
		}
		exists = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, exists, nil
}

func (r *codePluginRepositoryImpl) CopyDraft(ctx context.Context, sourcePluginID, targetPluginID, targetSpaceID int64) error {
	if sourcePluginID <= 0 || targetPluginID <= 0 || targetSpaceID <= 0 || sourcePluginID == targetPluginID {
		return fmt.Errorf("valid source, target and target space ids are required")
	}
	return r.writeTransaction(ctx, func(tx *gorm.DB) error {
		pluginIDs := []int64{sourcePluginID, targetPluginID}
		sort.Slice(pluginIDs, func(i, j int) bool { return pluginIDs[i] < pluginIDs[j] })
		parentSpaces, err := lockPluginDraftParents(tx, pluginIDs...)
		if err != nil {
			return err
		}

		lockedDrafts := make(map[int64]*codeDraftPO, len(pluginIDs))
		for _, pluginID := range pluginIDs {
			draft, exists, err := lockCodeDraft(tx, pluginID)
			if err != nil {
				return err
			}
			if exists {
				lockedDrafts[pluginID] = draft
			}
		}
		source, sourceExists := lockedDrafts[sourcePluginID]
		if !sourceExists {
			return ErrCodeDraftNotFound
		}
		if source.SpaceID != parentSpaces[sourcePluginID] ||
			targetSpaceID != parentSpaces[targetPluginID] {
			return ErrCodeSpaceMismatch
		}
		if _, targetExists := lockedDrafts[targetPluginID]; targetExists {
			return ErrCodeDraftConflict
		}

		files, err := getDraftFiles(tx, sourcePluginID)
		if err != nil {
			return err
		}
		prepared, err := entity.PrepareCodeDraft(&entity.CodeDraft{
			PluginID:         targetPluginID,
			SpaceID:          targetSpaceID,
			Runtime:          entity.CodeRuntime(source.Runtime),
			EntryFile:        source.EntryFile,
			InputSchemaJSON:  source.InputSchemaJSON,
			OutputSchemaJSON: source.OutputSchemaJSON,
			Files:            files,
		})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		target := draftToPO(prepared)
		target.Revision = 1
		target.LastDebuggedRevision = 0
		target.CreatedAt = now
		target.UpdatedAt = now
		if err := tx.Create(target).Error; err != nil {
			return mapCodeDraftCreateError(err)
		}
		return replaceDraftFiles(tx, targetPluginID, prepared.Files, now)
	})
}

func (r *codePluginRepositoryImpl) DeletePluginData(ctx context.Context, pluginID int64) error {
	if pluginID <= 0 {
		return fmt.Errorf("plugin id is required")
	}
	return r.writeTransaction(ctx, func(tx *gorm.DB) error {
		parentSpaces, err := lockPluginDraftParents(tx, pluginID)
		if err != nil {
			if errors.Is(err, ErrCodeDraftNotFound) {
				return nil
			}
			return err
		}
		draft, exists, err := lockCodeDraft(tx, pluginID)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if draft.SpaceID != parentSpaces[pluginID] {
			return ErrCodeSpaceMismatch
		}
		result := tx.Where("plugin_id = ?", pluginID).Delete(&codeDraftPO{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCodeDraftConflict
		}
		return nil
	})
}

func (r *codePluginRepositoryImpl) consistentRead(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
}

func (r *codePluginRepositoryImpl) writeTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return retryCodePluginWrite(ctx, func() error {
		return r.db.WithContext(ctx).Transaction(fn)
	})
}

func retryCodePluginWrite(ctx context.Context, operation func() error) error {
	var err error
	for attempt := 0; attempt < codePluginWriteMaxAttempts; attempt++ {
		err = operation()
		if err == nil || !isRetryableCodeWriteError(err) || attempt == codePluginWriteMaxAttempts-1 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * codePluginRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func readPluginDraftParentSpaces(tx *gorm.DB, pluginIDs ...int64) (map[int64]int64, error) {
	return pluginDraftParentSpaces(tx, false, pluginIDs...)
}

func lockPluginDraftParents(tx *gorm.DB, pluginIDs ...int64) (map[int64]int64, error) {
	return pluginDraftParentSpaces(tx, true, pluginIDs...)
}

func pluginDraftParentSpaces(tx *gorm.DB, lock bool, pluginIDs ...int64) (map[int64]int64, error) {
	ordered := append([]int64(nil), pluginIDs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	spaces := make(map[int64]int64, len(ordered))
	for index, pluginID := range ordered {
		if pluginID <= 0 {
			return nil, fmt.Errorf("plugin id is required")
		}
		if index > 0 && pluginID == ordered[index-1] {
			continue
		}
		var parent pluginDraftLockPO
		query := tx
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		err := query.Where("id = ?", pluginID).Take(&parent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCodeDraftNotFound
		}
		if err != nil {
			return nil, err
		}
		spaces[pluginID] = parent.SpaceID
	}
	return spaces, nil
}

func lockCodeDraft(tx *gorm.DB, pluginID int64) (*codeDraftPO, bool, error) {
	var draft codeDraftPO
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("plugin_id = ?", pluginID).
		Take(&draft).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &draft, true, nil
}

func replaceDraftFiles(tx *gorm.DB, pluginID int64, files []*entity.CodeFile, now time.Time) error {
	if err := tx.Where("plugin_id = ?", pluginID).Delete(&codeDraftFilePO{}).Error; err != nil {
		return err
	}
	filePOs := make([]*codeDraftFilePO, 0, len(files))
	for _, file := range files {
		filePOs = append(filePOs, &codeDraftFilePO{
			PluginID:  pluginID,
			Path:      file.Path,
			Content:   append([]byte(nil), file.Content...),
			Size:      file.Size,
			SHA256:    file.SHA256,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return tx.Create(&filePOs).Error
}

func getDraftFiles(db *gorm.DB, pluginID int64) ([]*entity.CodeFile, error) {
	var filePOs []*codeDraftFilePO
	if err := db.Where("plugin_id = ?", pluginID).Order("path ASC").Find(&filePOs).Error; err != nil {
		return nil, err
	}
	files := make([]*entity.CodeFile, 0, len(filePOs))
	for _, file := range filePOs {
		files = append(files, file.toEntity())
	}
	return files, nil
}

func getVersionFiles(db *gorm.DB, pluginID int64, version string) ([]*entity.CodeFile, error) {
	var filePOs []*codeVersionFilePO
	if err := db.Where("plugin_id = ? AND version = ?", pluginID, version).
		Order("path ASC").
		Find(&filePOs).Error; err != nil {
		return nil, err
	}
	files := make([]*entity.CodeFile, 0, len(filePOs))
	for _, file := range filePOs {
		files = append(files, file.toEntity())
	}
	return files, nil
}

func mapCodeDraftCreateError(err error) error {
	if isUniqueConstraintError(err) {
		return ErrCodeDraftConflict
	}
	return err
}

func isUniqueConstraintError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) {
		return mysqlError.Number == 1062
	}
	var stateError interface{ SQLState() string }
	if errors.As(err, &stateError) {
		return stateError.SQLState() == "23505"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate entry")
}

func isRetryableCodeWriteError(err error) bool {
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) {
		return mysqlError.Number == 1205 || mysqlError.Number == 1213
	}
	var stateError interface{ SQLState() string }
	return errors.As(err, &stateError) && stateError.SQLState() == "40001"
}

func draftToPO(draft *entity.CodeDraft) *codeDraftPO {
	return &codeDraftPO{
		PluginID:             draft.PluginID,
		SpaceID:              draft.SpaceID,
		Runtime:              string(draft.Runtime),
		EntryFile:            draft.EntryFile,
		SourceBundleRef:      draft.SourceBundleRef,
		InputSchemaJSON:      draft.InputSchemaJSON,
		OutputSchemaJSON:     draft.OutputSchemaJSON,
		Revision:             draft.Revision,
		LastDebuggedRevision: draft.LastDebuggedRevision,
	}
}

func (po *codeDraftPO) toEntity(files []*entity.CodeFile) *entity.CodeDraft {
	return &entity.CodeDraft{
		PluginID:             po.PluginID,
		SpaceID:              po.SpaceID,
		Runtime:              entity.CodeRuntime(po.Runtime),
		EntryFile:            po.EntryFile,
		SourceBundleRef:      po.SourceBundleRef,
		InputSchemaJSON:      po.InputSchemaJSON,
		OutputSchemaJSON:     po.OutputSchemaJSON,
		Revision:             po.Revision,
		LastDebuggedRevision: po.LastDebuggedRevision,
		Files:                entity.CloneCodeFiles(files),
	}
}

func (po *codeVersionPO) toEntity(files []*entity.CodeFile) *entity.CodeVersion {
	return &entity.CodeVersion{
		PluginID:         po.PluginID,
		SpaceID:          po.SpaceID,
		Version:          po.Version,
		Runtime:          entity.CodeRuntime(po.Runtime),
		EntryFile:        po.EntryFile,
		SourceBundleRef:  po.SourceBundleRef,
		InputSchemaJSON:  po.InputSchemaJSON,
		OutputSchemaJSON: po.OutputSchemaJSON,
		SourceRevision:   po.SourceRevision,
		CreatedBy:        po.CreatedBy,
		Files:            entity.CloneCodeFiles(files),
	}
}

func hydrateCodeVersion(po *codeVersionPO, files []*entity.CodeFile) (*entity.CodeVersion, error) {
	prepared, err := entity.PrepareCodeDraft(&entity.CodeDraft{
		PluginID:         po.PluginID,
		SpaceID:          po.SpaceID,
		Runtime:          entity.CodeRuntime(po.Runtime),
		EntryFile:        po.EntryFile,
		InputSchemaJSON:  po.InputSchemaJSON,
		OutputSchemaJSON: po.OutputSchemaJSON,
		Files:            files,
	})
	if err != nil {
		return nil, err
	}
	version := po.toEntity(prepared.Files)
	version.Runtime = prepared.Runtime
	version.EntryFile = prepared.EntryFile
	version.SourceBundleRef = prepared.SourceBundleRef
	version.InputSchemaJSON = prepared.InputSchemaJSON
	version.OutputSchemaJSON = prepared.OutputSchemaJSON
	return version, nil
}

func (po *codeDraftFilePO) toEntity() *entity.CodeFile {
	return &entity.CodeFile{
		Path:    po.Path,
		Content: append([]byte(nil), po.Content...),
		Size:    po.Size,
		SHA256:  po.SHA256,
	}
}

func (po *codeVersionFilePO) toEntity() *entity.CodeFile {
	return &entity.CodeFile{
		Path:    po.Path,
		Content: append([]byte(nil), po.Content...),
		Size:    po.Size,
		SHA256:  po.SHA256,
	}
}

func cloneDraft(draft *entity.CodeDraft) *entity.CodeDraft {
	if draft == nil {
		return nil
	}
	cloned := *draft
	cloned.Files = entity.CloneCodeFiles(draft.Files)
	return &cloned
}
