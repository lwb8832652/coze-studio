// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestMySQLSessionSettingsCASIsIndependentFromSchedulerCAS(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	ctx := context.Background()
	legacy, err := repository.GetSchedulerSettings(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerSettings() error = %v", err)
	}
	defaults := domainsandbox.DefaultSessionRuntimeSettings()
	encoded, err := marshalSessionRuntimeSettings(defaults)
	if err != nil {
		t.Fatalf("marshal session defaults: %v", err)
	}
	if err := db.Model(&schedulerSettingsPO{}).Where("id = ?", 1).Updates(map[string]any{
		"session_settings_json": encoded, "session_settings_version": 1,
		"session_settings_updated_by": 0, "aio_runtime_generation": 0,
	}).Error; err != nil {
		t.Fatalf("seed session settings: %v", err)
	}

	current, err := repository.GetSessionSettings(ctx)
	if err != nil {
		t.Fatalf("GetSessionSettings() error = %v", err)
	}
	next := current
	next.CoreEnabled = true
	updated, err := repository.UpdateSessionSettingsCAS(ctx, domainsandbox.UpdateSessionSettingsInput{
		ExpectedVersion: current.Version, Settings: next, UpdatedBy: 9,
	})
	if err != nil {
		t.Fatalf("UpdateSessionSettingsCAS() error = %v", err)
	}
	if updated.Version != 2 || !updated.CoreEnabled {
		t.Fatalf("session update = %#v", updated)
	}
	afterSession, err := repository.GetSchedulerSettings(ctx)
	if err != nil || afterSession.Version != legacy.Version || afterSession.MaxOutstanding != legacy.MaxOutstanding {
		t.Fatalf("session CAS changed scheduler snapshot: %#v / %v", afterSession, err)
	}

	legacy.MaxOutstanding--
	legacyUpdated, err := repository.UpdateSchedulerSettingsCAS(ctx, domainsandbox.UpdateSchedulerSettingsInput{
		ExpectedVersion: legacy.Version, Settings: legacy, UpdatedBy: 11,
	})
	if err != nil {
		t.Fatalf("UpdateSchedulerSettingsCAS() error = %v", err)
	}
	afterLegacy, err := repository.GetSessionSettings(ctx)
	if err != nil || afterLegacy.Version != updated.Version || !afterLegacy.CoreEnabled || legacyUpdated.Version != legacy.Version+1 {
		t.Fatalf("scheduler CAS changed session snapshot: %#v / %v", afterLegacy, err)
	}
	if _, err := repository.UpdateSessionSettingsCAS(ctx, domainsandbox.UpdateSessionSettingsInput{
		ExpectedVersion: current.Version, Settings: next, UpdatedBy: 12,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale session CAS error = %v", err)
	}
}

func TestMySQLSessionSettingsMissingColumnsFailClosed(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	if err := db.Exec(`CREATE TABLE sandbox_scheduler_settings (id integer primary key, settings_json text not null, version integer not null, updated_by integer not null, created_at datetime not null, updated_at datetime not null)`).Error; err != nil {
		t.Fatalf("create legacy scheduler table: %v", err)
	}
	if _, err := repository.GetSessionSettings(context.Background()); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("GetSessionSettings() error = %v, want ErrConfigurationInvalid", err)
	}
}

func TestMySQLSchedulerLegacyProjectionIgnoresSessionColumns(t *testing.T) {
	repository, mock := newProviderCreateTransactionRepository(t)
	settingsJSON, err := marshalSchedulerSettings(domainsandbox.DefaultSchedulerSettings())
	if err != nil {
		t.Fatalf("marshal scheduler settings: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`settings_json`,`version`,`updated_by`,`created_at`,`updated_at` FROM `sandbox_scheduler_settings` WHERE id = ? ORDER BY `sandbox_scheduler_settings`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settings_json", "version", "updated_by", "created_at", "updated_at"}).
			AddRow(1, settingsJSON, 4, 5, persistenceNow(), persistenceNow()))
	mock.ExpectCommit()
	got, err := repository.GetSchedulerSettings(context.Background())
	if err != nil || got.Version != 4 || got.UpdatedBy != 5 {
		t.Fatalf("GetSchedulerSettings() = %#v / %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("legacy scheduler projection SQL = %v", err)
	}
}

func TestSessionAndSchedulerCASUpdateDisjointColumns(t *testing.T) {
	repository, db := newSQLiteRepositoryWithLogger(t, newRecordingGORMLogger())
	logger := db.Logger.(*recordingGORMLogger)
	if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	ctx := context.Background()
	legacy, err := repository.GetSchedulerSettings(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerSettings() error = %v", err)
	}
	raw, _ := marshalSessionRuntimeSettings(domainsandbox.DefaultSessionRuntimeSettings())
	if err := db.Model(&schedulerSettingsPO{}).Where("id = ?", 1).Updates(map[string]any{"session_settings_json": raw, "session_settings_version": 1}).Error; err != nil {
		t.Fatal(err)
	}
	current, err := repository.GetSessionSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	logger.reset()
	next := current
	next.CoreEnabled = true
	if _, err := repository.UpdateSessionSettingsCAS(ctx, domainsandbox.UpdateSessionSettingsInput{ExpectedVersion: 1, Settings: next, UpdatedBy: 7}); err != nil {
		t.Fatal(err)
	}
	sessionSQL := logger.String()
	if !strings.Contains(sessionSQL, "session_settings_version") || strings.Contains(sessionSQL, "`settings_json`=") || strings.Contains(sessionSQL, "`version`=") || strings.Contains(sessionSQL, "`updated_at`=") {
		t.Fatalf("session CAS touched legacy columns: %s", sessionSQL)
	}
	logger.reset()
	legacy.MaxOutstanding--
	if _, err := repository.UpdateSchedulerSettingsCAS(ctx, domainsandbox.UpdateSchedulerSettingsInput{ExpectedVersion: legacy.Version, Settings: legacy, UpdatedBy: 8}); err != nil {
		t.Fatal(err)
	}
	schedulerSQL := logger.String()
	if strings.Contains(schedulerSQL, "session_settings_") {
		t.Fatalf("scheduler CAS touched session columns: %s", schedulerSQL)
	}
}
