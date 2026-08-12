// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"math"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSchedulerServicePersistsDesiredBeforeUnavailableRunnerAndAuditsSafely(t *testing.T) {
	store := &schedulerSettingsStoreFake{settings: domainsandbox.DefaultSchedulerSettings()}
	runner := &schedulerRunnerFake{err: errors.New("runner endpoint token must not leak")}
	service, err := NewSchedulerService(SchedulerServiceOptions{Store: store, Runner: runner})
	if err != nil {
		t.Fatalf("NewSchedulerService() error = %v", err)
	}
	next := domainsandbox.DefaultSchedulerSettings()
	next.MaxOutstanding = 31
	result, err := service.Update(context.Background(), testActor(), UpdateSchedulerSettingsRequest{
		ExpectedVersion: 1,
		Settings:        next,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Version != 2 || result.Applied || result.ReasonCode != SchedulerReasonProviderUnavailable || !store.persistedBeforeApply {
		t.Fatal("Update() did not preserve the desired settings before unavailable runner application")
	}
	if len(store.events) != 2 || store.events[0].Action != domainsandbox.SchedulerAuditActionUpdate || store.events[1].Action != domainsandbox.SchedulerAuditActionUpdateFailed {
		t.Fatal("Update() did not write the required scheduler audit actions")
	}
	for _, event := range store.events {
		if len(event.Metadata) != 3 || event.Metadata["changed_fields"] == "" || event.Metadata["previous_version"] == "" || event.Metadata["new_version"] == "" {
			t.Fatal("scheduler audit metadata is not the strict safe projection")
		}
	}
}

func TestSchedulerServiceStaleVersionReturnsConflictAndRecordsFailure(t *testing.T) {
	store := &schedulerSettingsStoreFake{settings: domainsandbox.DefaultSchedulerSettings(), updateErr: domainsandbox.ErrVersionConflict}
	service, err := NewSchedulerService(SchedulerServiceOptions{Store: store})
	if err != nil {
		t.Fatalf("NewSchedulerService() error = %v", err)
	}
	next := domainsandbox.DefaultSchedulerSettings()
	next.MaxOutstanding = 31
	_, err = service.Update(context.Background(), testActor(), UpdateSchedulerSettingsRequest{
		ExpectedVersion: 1,
		Settings:        next,
	})
	if !errors.Is(err, domainsandbox.ErrVersionConflict) || len(store.events) != 1 || store.events[0].Action != domainsandbox.SchedulerAuditActionUpdateFailed {
		t.Fatal("stale scheduler update did not return conflict with safe failure audit")
	}
	if store.events[0].Metadata[domainsandbox.SchedulerAuditMetadataPreviousVersion] != "1" || store.events[0].Metadata[domainsandbox.SchedulerAuditMetadataNewVersion] != "2" {
		t.Fatal("stale scheduler audit did not record the attempted version transition")
	}
}

func TestSchedulerServicePreReadVersionConflictSkipsCASAndRecordsAttempt(t *testing.T) {
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 2
	store := &schedulerSettingsStoreFake{settings: settings}
	service, err := NewSchedulerService(SchedulerServiceOptions{Store: store})
	if err != nil {
		t.Fatalf("NewSchedulerService() error = %v", err)
	}
	next := domainsandbox.DefaultSchedulerSettings()
	next.MaxOutstanding = 31
	_, err = service.Update(context.Background(), testActor(), UpdateSchedulerSettingsRequest{
		ExpectedVersion: 1,
		Settings:        next,
	})
	if !errors.Is(err, domainsandbox.ErrVersionConflict) || store.casCalls != 0 || len(store.events) != 1 || store.events[0].Action != domainsandbox.SchedulerAuditActionUpdateFailed {
		t.Fatal("pre-read scheduler version conflict invoked CAS or wrote the wrong audit action")
	}
	if store.events[0].Metadata[domainsandbox.SchedulerAuditMetadataPreviousVersion] != "1" || store.events[0].Metadata[domainsandbox.SchedulerAuditMetadataNewVersion] != "2" {
		t.Fatal("pre-read scheduler version conflict did not record the attempted transition")
	}
}

func TestSchedulerServiceRejectsOverflowedAttemptedAuditVersion(t *testing.T) {
	store := &schedulerSettingsStoreFake{settings: domainsandbox.DefaultSchedulerSettings()}
	service, err := NewSchedulerService(SchedulerServiceOptions{Store: store})
	if err != nil {
		t.Fatalf("NewSchedulerService() error = %v", err)
	}
	next := domainsandbox.DefaultSchedulerSettings()
	next.MaxOutstanding = 31
	_, err = service.Update(context.Background(), testActor(), UpdateSchedulerSettingsRequest{
		ExpectedVersion: math.MaxUint64,
		Settings:        next,
	})
	if !errors.Is(err, domainsandbox.ErrInvalidInput) || len(store.events) != 0 || store.persistedBeforeApply {
		t.Fatal("overflowed scheduler transition was persisted or audited")
	}
}

func TestSchedulerServiceRuntimeStatusIsSafeWhenUnavailableAndAvailable(t *testing.T) {
	store := &schedulerSettingsStoreFake{settings: domainsandbox.DefaultSchedulerSettings()}
	service, err := NewSchedulerService(SchedulerServiceOptions{Store: store})
	if err != nil {
		t.Fatalf("NewSchedulerService() error = %v", err)
	}
	unavailable, err := service.RuntimeStatus(context.Background(), testActor())
	if err != nil || unavailable.Available || unavailable.ReasonCode != SchedulerReasonProviderUnavailable || unavailable.DesiredConfigVersion != 1 {
		t.Fatal("RuntimeStatus() did not return the safe unavailable projection")
	}
	runner := &schedulerRunnerFake{status: NativeRunnerStatus{
		Healthy: true, AppliedConfigVersion: 1, QueueDepth: 2, ActiveSlots: 1, SlotCapacity: 2,
		QueueHighWatermark: 3, DrainingCount: 1, QuarantinedCount: 0,
		QueueByScope:   map[domainsandbox.Scope]int{domainsandbox.ScopeAgent: 2},
		IdleContainers: 1, ActiveContainers: 1, MemoryReserveState: "available",
	}}
	service, err = NewSchedulerService(SchedulerServiceOptions{Store: store, Runner: runner})
	if err != nil {
		t.Fatalf("NewSchedulerService(available) error = %v", err)
	}
	available, err := service.RuntimeStatus(context.Background(), testActor())
	if err != nil || !available.Available || available.QueueDepth != 2 || available.ActiveSlots != 1 || available.QueueByScope[domainsandbox.ScopeAgent] != 2 || available.IdleContainers != 1 || available.MemoryReserveState != "available" || available.ReasonCode != "" {
		t.Fatal("RuntimeStatus() did not return the safe available aggregate projection")
	}
}

type schedulerSettingsStoreFake struct {
	settings             domainsandbox.SchedulerSettings
	updateErr            error
	events               []domainsandbox.SchedulerAuditEvent
	casCalls             int
	persistedBeforeApply bool
}

func (f *schedulerSettingsStoreFake) GetSchedulerSettings(context.Context) (domainsandbox.SchedulerSettings, error) {
	settings := f.settings
	if settings.Version == 0 {
		settings.Version = domainsandbox.InitialVersion
	}
	return settings, nil
}

func (f *schedulerSettingsStoreFake) UpdateSchedulerSettingsCAS(context.Context, domainsandbox.UpdateSchedulerSettingsInput) (domainsandbox.SchedulerSettings, error) {
	return domainsandbox.SchedulerSettings{}, errors.New("test must use atomic scheduler audit update")
}

func (f *schedulerSettingsStoreFake) UpdateSchedulerSettingsCASWithAudit(
	_ context.Context,
	input domainsandbox.UpdateSchedulerSettingsInput,
	audit domainsandbox.AppendSchedulerAuditEventInput,
) (domainsandbox.SchedulerSettings, error) {
	f.casCalls++
	if f.updateErr != nil {
		return domainsandbox.SchedulerSettings{}, f.updateErr
	}
	f.settings = input.Settings
	f.settings.Version = input.ExpectedVersion + 1
	f.settings.UpdatedBy = input.UpdatedBy
	f.events = append(f.events, domainsandbox.SchedulerAuditEvent{Action: audit.Action, Metadata: audit.Metadata})
	f.persistedBeforeApply = true
	return f.settings, nil
}

func (f *schedulerSettingsStoreFake) AppendSchedulerAuditEvent(_ context.Context, input domainsandbox.AppendSchedulerAuditEventInput) (*domainsandbox.SchedulerAuditEvent, error) {
	event := domainsandbox.SchedulerAuditEvent{Action: input.Action, Metadata: input.Metadata}
	f.events = append(f.events, event)
	return &event, nil
}

type schedulerRunnerFake struct {
	err    error
	status NativeRunnerStatus
}

func (f *schedulerRunnerFake) ApplySchedulerSettings(context.Context, domainsandbox.SchedulerSettings) error {
	return f.err
}
func (f *schedulerRunnerFake) RuntimeStatus(context.Context) (NativeRunnerStatus, error) {
	return f.status, f.err
}
