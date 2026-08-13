// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strconv"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestMySQLSessionSettingsCASWithAuditRollsBackOnAuditFailure(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	defaults := domainsandbox.DefaultSessionRuntimeSettings()
	encoded, _ := marshalSessionRuntimeSettings(defaults)
	now := persistenceNow()
	if err := db.Create(&schedulerSettingsPO{ID: 1, SettingsJSON: mustSchedulerSettingsJSON(t), Version: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed scheduler row: %v", err)
	}
	if err := db.Model(&schedulerSettingsPO{}).Where("id = ?", 1).Updates(map[string]any{"session_settings_json": encoded, "session_settings_version": 1}).Error; err != nil {
		t.Fatalf("seed session settings: %v", err)
	}
	current, err := repository.GetSessionSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSessionSettings() error = %v", err)
	}
	next := current
	next.CoreEnabled = true
	if err := db.Exec(`CREATE TRIGGER fail_session_settings_audit BEFORE INSERT ON sandbox_scheduler_audit_events BEGIN SELECT RAISE(ABORT, 'audit failed'); END`).Error; err != nil {
		t.Fatalf("create audit failure trigger: %v", err)
	}
	goodAudit := sessionSettingsAuditInputForTest(domainsandbox.SchedulerAuditActionUpdate, 1, 2)
	if _, err := repository.UpdateSessionSettingsCASWithAudit(context.Background(), domainsandbox.UpdateSessionSettingsInput{ExpectedVersion: 1, Settings: next, UpdatedBy: 7}, goodAudit); err == nil {
		t.Fatal("audited update unexpectedly succeeded when audit insert failed")
	}
	after, err := repository.GetSessionSettings(context.Background())
	if err != nil || after.Version != 1 || after.CoreEnabled {
		t.Fatalf("audit insertion failure changed session settings: %#v / %v", after, err)
	}
	if err := db.Exec(`DROP TRIGGER fail_session_settings_audit`).Error; err != nil {
		t.Fatalf("drop audit failure trigger: %v", err)
	}
	updated, err := repository.UpdateSessionSettingsCASWithAudit(context.Background(), domainsandbox.UpdateSessionSettingsInput{ExpectedVersion: 1, Settings: next, UpdatedBy: 7}, goodAudit)
	if err != nil || updated.Version != 2 {
		t.Fatalf("audited update = %#v / %v", updated, err)
	}
	var rows []schedulerAuditEventPO
	if err := db.Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].Action != string(domainsandbox.SchedulerAuditActionUpdate) {
		t.Fatalf("session audit rows = %#v / %v", rows, err)
	}
}

func TestMySQLSessionSettingsAuditRejectsUnsafeMetadataBeforeWriting(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	invalid := sessionSettingsAuditInputForTest(domainsandbox.SchedulerAuditActionUpdateFailed, 1, 2)
	invalid.Metadata[domainsandbox.SchedulerAuditMetadataChangedFields] = "settings"
	if _, err := repository.AppendSessionSettingsAuditEvent(context.Background(), invalid); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("invalid session audit error = %v", err)
	}
	var count int64
	if err := db.Model(&schedulerAuditEventPO{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid session audit rows = %d / %v", count, err)
	}
}

func TestMySQLSessionSettingsCASWithAuditRejectsAuditThatDoesNotDescribeUpdate(t *testing.T) {
	tests := map[string]func(*domainsandbox.AppendSchedulerAuditEventInput){
		"action": func(input *domainsandbox.AppendSchedulerAuditEventInput) {
			input.Action = domainsandbox.SchedulerAuditActionUpdateFailed
		},
		"actor": func(input *domainsandbox.AppendSchedulerAuditEventInput) {
			input.ActorUserID++
		},
		"versions": func(input *domainsandbox.AppendSchedulerAuditEventInput) {
			input.Metadata[domainsandbox.SchedulerAuditMetadataPreviousVersion] = "2"
			input.Metadata[domainsandbox.SchedulerAuditMetadataNewVersion] = "3"
		},
		"changed fields": func(input *domainsandbox.AppendSchedulerAuditEventInput) {
			input.Metadata[domainsandbox.SchedulerAuditMetadataChangedFields] = "host_shell_enabled"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			repository, db := newSQLiteRepository(t)
			if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
				t.Fatalf("migrate settings: %v", err)
			}
			defaults := domainsandbox.DefaultSessionRuntimeSettings()
			encoded, err := marshalSessionRuntimeSettings(defaults)
			if err != nil {
				t.Fatalf("marshal defaults: %v", err)
			}
			now := persistenceNow()
			if err := db.Create(&schedulerSettingsPO{ID: 1, SettingsJSON: mustSchedulerSettingsJSON(t), Version: 1, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				t.Fatalf("seed scheduler row: %v", err)
			}
			if err := db.Model(&schedulerSettingsPO{}).Where("id = ?", 1).Updates(map[string]any{
				"session_settings_json": encoded, "session_settings_version": 1,
			}).Error; err != nil {
				t.Fatalf("seed session settings: %v", err)
			}
			next := defaults
			next.CoreEnabled = true
			audit := sessionSettingsAuditInputForTest(domainsandbox.SchedulerAuditActionUpdate, 1, 2)
			mutate(&audit)
			if _, err := repository.UpdateSessionSettingsCASWithAudit(context.Background(), domainsandbox.UpdateSessionSettingsInput{
				ExpectedVersion: 1, Settings: next, UpdatedBy: 7,
			}, audit); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("mismatched audit error = %v, want ErrInvalidInput", err)
			}
			after, err := repository.GetSessionSettings(context.Background())
			if err != nil || after.Version != 1 || after.CoreEnabled {
				t.Fatalf("mismatched audit changed settings: %#v / %v", after, err)
			}
			var count int64
			if err := db.Model(&schedulerAuditEventPO{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("mismatched audit rows = %d / %v", count, err)
			}
		})
	}
}

func sessionSettingsAuditInputForTest(action domainsandbox.SchedulerAuditAction, previousVersion, newVersion uint64) domainsandbox.AppendSchedulerAuditEventInput {
	return domainsandbox.AppendSchedulerAuditEventInput{ActorUserID: 7, Action: action, Metadata: map[string]string{
		domainsandbox.SchedulerAuditMetadataPreviousVersion: strconv.FormatUint(previousVersion, 10),
		domainsandbox.SchedulerAuditMetadataNewVersion:      strconv.FormatUint(newVersion, 10),
		domainsandbox.SchedulerAuditMetadataChangedFields:   "core_enabled",
		domainsandbox.SessionSettingsAuditMetadataDomain:    domainsandbox.SessionSettingsAuditDomain,
	}}
}

func mustSchedulerSettingsJSON(t *testing.T) string {
	t.Helper()
	raw, err := marshalSchedulerSettings(domainsandbox.DefaultSchedulerSettings())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
