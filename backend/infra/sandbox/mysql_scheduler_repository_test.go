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

func TestMySQLSchedulerCreatesSingletonDefaultAndReturnsDetachedValues(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
		t.Fatalf("migrate scheduler settings: %v", err)
	}
	first, err := repository.GetSchedulerSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSchedulerSettings() error = %v", err)
	}
	if first.Version != 1 || first.UpdatedBy != 0 || first.MaxOutstanding != 32 {
		t.Fatalf("singleton default metadata mismatch: version=%d max_outstanding=%d", first.Version, first.MaxOutstanding)
	}
	first.Workloads[domainsandbox.ScopeAgent] = domainsandbox.SchedulerWorkload{}
	second, err := repository.GetSchedulerSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSchedulerSettings(second) error = %v", err)
	}
	if second.Workloads[domainsandbox.ScopeAgent].Weight != 2 {
		t.Fatal("GetSchedulerSettings() returned shared workload map")
	}
	var rows int64
	if err := db.Model(&schedulerSettingsPO{}).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("scheduler singleton rows/error = %d/%v", rows, err)
	}
}

func TestMySQLSchedulerGetLocksBeforeInitializingSingleton(t *testing.T) {
	repository, mock := newProviderCreateTransactionRepository(t)
	settingsJSON, err := marshalSchedulerSettings(domainsandbox.DefaultSchedulerSettings())
	if err != nil {
		t.Fatalf("marshal default settings: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`settings_json`,`version`,`updated_by`,`created_at`,`updated_at` FROM `sandbox_scheduler_settings` WHERE id = ? ORDER BY `sandbox_scheduler_settings`.`id` LIMIT ? FOR UPDATE")).
		WithArgs(1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settings_json", "version", "updated_by", "created_at", "updated_at"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `sandbox_scheduler_settings` (`settings_json`,`version`,`updated_by`,`created_at`,`updated_at`,`id`) VALUES (?,?,?,?,?,?)")).
		WithArgs(settingsJSON, domainsandbox.InitialVersion, 0, sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	settings, err := repository.GetSchedulerSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSchedulerSettings() error = %v", err)
	}
	if settings.Version != domainsandbox.InitialVersion || settings.UpdatedBy != 0 || settings.Workloads[domainsandbox.ScopeAgent].CPULimit != 1250 {
		t.Fatal("GetSchedulerSettings() did not initialize the locked default singleton")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("GetSchedulerSettings() lock/initialization SQL = %v", err)
	}
}

func TestMySQLSchedulerCASPersistsOnlySettingsJSONAndRejectsStaleVersion(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(&schedulerSettingsPO{}); err != nil {
		t.Fatalf("migrate scheduler settings: %v", err)
	}
	ctx := context.Background()
	current, err := repository.GetSchedulerSettings(ctx)
	if err != nil {
		t.Fatalf("GetSchedulerSettings() error = %v", err)
	}
	next := current
	next.MaxOutstanding = 31
	updated, err := repository.UpdateSchedulerSettingsCAS(ctx, domainsandbox.UpdateSchedulerSettingsInput{
		ExpectedVersion: current.Version,
		Settings:        next,
		UpdatedBy:       77,
	})
	if err != nil {
		t.Fatalf("UpdateSchedulerSettingsCAS() error = %v", err)
	}
	if updated.Version != current.Version+1 || updated.UpdatedBy != 77 || updated.MaxOutstanding != 31 {
		t.Fatalf("CAS result mismatch: version=%d max_outstanding=%d", updated.Version, updated.MaxOutstanding)
	}
	if _, err := repository.UpdateSchedulerSettingsCAS(ctx, domainsandbox.UpdateSchedulerSettingsInput{
		ExpectedVersion: current.Version,
		Settings:        next,
		UpdatedBy:       78,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale UpdateSchedulerSettingsCAS() error = %v", err)
	}
	var raw schedulerSettingsPO
	if err := db.First(&raw, "id = ?", 1).Error; err != nil {
		t.Fatalf("read scheduler row: %v", err)
	}
	if strings.Contains(raw.SettingsJSON, "credential") || strings.Contains(raw.SettingsJSON, "encrypted") {
		t.Fatal("settings_json contains a credential-like value")
	}
}

func TestProviderFeaturesRoundTripWithoutChangingMaxConcurrency(t *testing.T) {
	repository, _ := newSQLiteRepository(t)
	provider, err := repository.CreateProvider(context.Background(), validCreateProviderInput("feature-round-trip"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	nextVersion, err := repository.UpdateProviderHealth(context.Background(), domainsandbox.UpdateProviderHealthInput{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, ActorUserID: 99,
		Health: domainsandbox.HealthSnapshot{
			Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
			Features: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1}, CheckedAt: persistenceNow(),
		},
	})
	if err != nil {
		t.Fatalf("UpdateProviderHealth() error = %v", err)
	}
	got, err := repository.GetProvider(context.Background(), provider.ID)
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if nextVersion != provider.Version+1 || len(got.Health.Features) != 1 || got.Health.Features[0] != domainsandbox.ProviderFeatureQueueStatusV1 || got.Policy.MaxConcurrency != provider.Policy.MaxConcurrency {
		t.Fatalf("provider feature round trip/version/max concurrency failed")
	}
}
