// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"math"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var _ domainsandbox.SessionSettingsRepository = (*MySQLRepository)(nil)

func marshalSessionRuntimeSettings(settings domainsandbox.SessionRuntimeSettings) (string, error) {
	normalized, err := domainsandbox.NormalizeSessionRuntimeSettings(settings)
	if err != nil {
		return "", err
	}
	return marshalCanonicalJSON(normalized)
}

func unmarshalSessionRuntimeSettings(raw string) (domainsandbox.SessionRuntimeSettings, error) {
	settings, err := domainsandbox.DecodeSessionRuntimeSettingsJSON([]byte(raw))
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.ErrConfigurationInvalid
	}
	return settings, nil
}

func (r *MySQLRepository) GetSessionSettings(ctx context.Context) (domainsandbox.SessionRuntimeSettings, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	po, err := findSessionSettings(db, false)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	return sessionSettingsDomain(po)
}

func (r *MySQLRepository) UpdateSessionSettingsCAS(ctx context.Context, input domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	var result domainsandbox.SessionRuntimeSettings
	err = db.Transaction(func(tx *gorm.DB) error {
		result, err = updateSessionSettingsCAS(tx, input)
		return err
	})
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	return result, nil
}

func updateSessionSettingsCAS(db *gorm.DB, input domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, error) {
	_, updated, err := updateSessionSettingsCASSnapshot(db, input)
	return updated, err
}

func updateSessionSettingsCASSnapshot(db *gorm.DB, input domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, domainsandbox.SessionRuntimeSettings, error) {
	po, err := findSessionSettings(db, true)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, err
	}
	current, err := sessionSettingsDomain(po)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, err
	}
	normalized, err := domainsandbox.NormalizeUpdateSessionSettingsInput(input, current)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, err
	}
	if po.SessionSettingsVersion != normalized.ExpectedVersion {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, domainsandbox.ErrVersionConflict
	}
	updatedBy, err := positiveDomainInt64ToUint64(normalized.UpdatedBy)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, domainsandbox.ErrInvalidInput
	}
	nextVersion, err := domainsandbox.NextVersion(normalized.ExpectedVersion)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, err
	}
	settingsJSON, err := marshalSessionRuntimeSettings(normalized.Settings)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, err
	}
	now := persistenceNow()
	update := db.Model(&schedulerSettingsPO{}).
		Where("id = ? AND session_settings_version = ?", 1, normalized.ExpectedVersion).
		UpdateColumns(map[string]any{
			"session_settings_json":       settingsJSON,
			"session_settings_version":    nextVersion,
			"session_settings_updated_by": updatedBy,
			"session_settings_updated_at": now,
		})
	if update.Error != nil {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, mapSessionSettingsSchemaError(update.Error)
	}
	if update.RowsAffected == 0 {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.SessionRuntimeSettings{}, domainsandbox.ErrVersionConflict
	}
	result := normalized.Settings
	result.Version = nextVersion
	result.UpdatedBy = normalized.UpdatedBy
	return current, result, nil
}

func findSessionSettings(db *gorm.DB, lock bool) (*schedulerSettingsPO, error) {
	var po schedulerSettingsPO
	query := db.Select(
		"id", "session_settings_json", "session_settings_version",
		"session_settings_updated_by", "session_settings_updated_at", "aio_runtime_generation",
	).Where("id = ?", 1)
	if lock {
		query = withUpdateLock(query)
	}
	if err := query.First(&po).Error; err != nil {
		return nil, mapSessionSettingsSchemaError(err)
	}
	return &po, nil
}

func sessionSettingsDomain(po *schedulerSettingsPO) (domainsandbox.SessionRuntimeSettings, error) {
	if po == nil || po.ID != 1 || po.SessionSettingsJSON == nil || po.SessionSettingsVersion < domainsandbox.InitialVersion || po.SessionSettingsUpdatedBy > math.MaxInt64 {
		return domainsandbox.SessionRuntimeSettings{}, domainsandbox.ErrConfigurationInvalid
	}
	settings, err := unmarshalSessionRuntimeSettings(*po.SessionSettingsJSON)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	settings.Version = po.SessionSettingsVersion
	settings.UpdatedBy = int64(po.SessionSettingsUpdatedBy)
	return settings, nil
}

func mapSessionSettingsSchemaError(err error) error {
	if err == nil {
		return nil
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1054 {
		return domainsandbox.ErrConfigurationInvalid
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such column") || errors.Is(err, gorm.ErrRecordNotFound) {
		return domainsandbox.ErrConfigurationInvalid
	}
	return err
}
