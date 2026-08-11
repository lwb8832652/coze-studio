// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestMySQLSchedulerAuditPersistsOnlySafeGlobalMetadata(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
		t.Fatalf("migrate scheduler audit tables: %v", err)
	}
	ctx := context.Background()
	current, err := repository.GetSchedulerSettings(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerSettings() error = %v", err)
	}
	next := current
	next.MaxOutstanding = 31
	updated, err := repository.UpdateSchedulerSettingsCASWithAudit(ctx, domainsandbox.UpdateSchedulerSettingsInput{
		ExpectedVersion: current.Version, Settings: next, UpdatedBy: 7,
	}, schedulerAuditInputForTest(domainsandbox.SchedulerAuditActionUpdate, current.Version, current.Version+1))
	if err != nil {
		t.Fatalf("UpdateSchedulerSettingsCASWithAudit() error = %v", err)
	}
	if updated.Version != current.Version+1 || updated.MaxOutstanding != 31 {
		t.Fatalf("scheduler update result mismatch: version=%d max_outstanding=%d", updated.Version, updated.MaxOutstanding)
	}
	if _, err := repository.AppendSchedulerAuditEvent(ctx, schedulerAuditInputForTest(domainsandbox.SchedulerAuditActionUpdateFailed, current.Version, updated.Version)); err != nil {
		t.Fatalf("AppendSchedulerAuditEvent(update_failed) error = %v", err)
	}

	var rows []schedulerAuditEventPO
	if err := db.Order("event_id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read scheduler audit rows: %v", err)
	}
	if len(rows) != 2 || rows[0].Action != string(domainsandbox.SchedulerAuditActionUpdate) || rows[1].Action != string(domainsandbox.SchedulerAuditActionUpdateFailed) {
		t.Fatal("scheduler audit action rows were not persisted")
	}
	for _, row := range rows {
		metadata, err := unmarshalSchedulerAuditMetadata(row.MetadataJSON)
		if err != nil || len(metadata) != 3 || metadata[domainsandbox.SchedulerAuditMetadataPreviousVersion] == "" || metadata[domainsandbox.SchedulerAuditMetadataNewVersion] == "" || metadata[domainsandbox.SchedulerAuditMetadataChangedFields] != "max_outstanding" {
			t.Fatal("scheduler audit metadata was not the safe projection")
		}
		for _, forbidden := range []string{"settings", "updated_by", "user", "space", "project", "session", "credential", "token", "secret", "endpoint", "error"} {
			if strings.Contains(row.MetadataJSON, `"`+forbidden+`":`) {
				t.Fatal("scheduler audit metadata persisted a forbidden key")
			}
		}
	}

	before := len(rows)
	invalid := schedulerAuditInputForTest(domainsandbox.SchedulerAuditActionUpdateFailed, current.Version, updated.Version)
	invalid.Metadata["settings"] = "forbidden"
	if _, err := repository.AppendSchedulerAuditEvent(ctx, invalid); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatal("scheduler audit accepted a forbidden metadata key")
	}
	var count int64
	if err := db.Model(&schedulerAuditEventPO{}).Count(&count).Error; err != nil || int(count) != before {
		t.Fatal("invalid scheduler audit metadata was persisted")
	}
}

func schedulerAuditInputForTest(action domainsandbox.SchedulerAuditAction, previousVersion, newVersion uint64) domainsandbox.AppendSchedulerAuditEventInput {
	return domainsandbox.AppendSchedulerAuditEventInput{
		ActorUserID: 7,
		Action:      action,
		Metadata: map[string]string{
			domainsandbox.SchedulerAuditMetadataPreviousVersion: strconv.FormatUint(previousVersion, 10),
			domainsandbox.SchedulerAuditMetadataNewVersion:      strconv.FormatUint(newVersion, 10),
			domainsandbox.SchedulerAuditMetadataChangedFields:   "max_outstanding",
		},
	}
}
