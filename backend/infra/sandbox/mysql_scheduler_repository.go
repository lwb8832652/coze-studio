// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

func (r *MySQLRepository) GetSchedulerSettings(ctx context.Context) (domainsandbox.SchedulerSettings, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	var settings domainsandbox.SchedulerSettings
	err = db.Transaction(func(tx *gorm.DB) error {
		po, err := findOrCreateSchedulerSettings(tx, true)
		if err != nil {
			return err
		}
		settings, err = po.toDomain()
		return err
	})
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	return settings, nil
}

func (r *MySQLRepository) UpdateSchedulerSettingsCAS(
	ctx context.Context,
	input domainsandbox.UpdateSchedulerSettingsInput,
) (domainsandbox.SchedulerSettings, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeUpdateSchedulerSettingsInput(input)
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	updatedBy, err := positiveDomainInt64ToUint64(normalized.UpdatedBy)
	if err != nil {
		return domainsandbox.SchedulerSettings{}, domainsandbox.ErrInvalidInput
	}
	settingsJSON, err := marshalSchedulerSettings(normalized.Settings)
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	var result domainsandbox.SchedulerSettings
	err = db.Transaction(func(tx *gorm.DB) error {
		po, err := findOrCreateSchedulerSettings(tx, true)
		if err != nil {
			return err
		}
		if po.Version != normalized.ExpectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		nextVersion, err := domainsandbox.NextVersion(normalized.ExpectedVersion)
		if err != nil {
			return err
		}
		now := persistenceNow()
		update := tx.Model(&schedulerSettingsPO{}).Where("id = ? AND version = ?", 1, normalized.ExpectedVersion).Updates(map[string]any{
			"settings_json": settingsJSON,
			"version":       nextVersion,
			"updated_by":    updatedBy,
			"updated_at":    now,
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return domainsandbox.ErrVersionConflict
		}
		result = normalized.Settings
		result.Version = nextVersion
		result.UpdatedBy = normalized.UpdatedBy
		return nil
	})
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	return result, nil
}

func findOrCreateSchedulerSettings(db *gorm.DB, lock bool) (*schedulerSettingsPO, error) {
	var po schedulerSettingsPO
	query := db.Where("id = ?", 1)
	if lock {
		query = withUpdateLock(query)
	}
	err := query.First(&po).Error
	if err == nil {
		return &po, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	defaults := domainsandbox.DefaultSchedulerSettings()
	settingsJSON, err := marshalSchedulerSettings(defaults)
	if err != nil {
		return nil, err
	}
	now := persistenceNow()
	po = schedulerSettingsPO{
		ID:           1,
		SettingsJSON: settingsJSON,
		Version:      domainsandbox.InitialVersion,
		UpdatedBy:    0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(&po).Error; err != nil {
		return nil, err
	}
	return &po, nil
}
