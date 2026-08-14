// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSessionSettingsServicePersistsDesiredBeforeApplyAndKeepsItOnRunnerFailure(t *testing.T) {
	store := &sessionSettingsStoreFake{settings: domainsandbox.DefaultSessionRuntimeSettings()}
	runner := &sessionRunnerFake{applyVersion: 1, err: errors.New("runner endpoint token must not leak")}
	service, err := NewSessionSettingsService(SessionSettingsServiceOptions{Store: store, Runner: runner})
	if err != nil {
		t.Fatalf("NewSessionSettingsService() error = %v", err)
	}
	next := domainsandbox.DefaultSessionRuntimeSettings()
	next.CoreEnabled = true
	result, err := service.Update(context.Background(), testActor(), UpdateSessionSettingsRequest{ExpectedVersion: 1, Settings: next})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Version != 2 || result.Applied || result.AppliedVersion != 1 || result.ReasonCode != SessionReasonRunnerUnavailable {
		t.Fatalf("Update() result = %#v", result)
	}
	if !store.persistedBeforeApply || runner.got.Version != 2 || !runner.got.CoreEnabled {
		t.Fatal("Update() did not apply the exact persisted desired snapshot")
	}
	if len(store.events) != 2 || store.events[0].Action != domainsandbox.SchedulerAuditActionUpdate || store.events[1].Action != domainsandbox.SchedulerAuditActionUpdateFailed {
		t.Fatalf("audit actions = %#v", store.events)
	}
	for _, event := range store.events {
		if event.Metadata[domainsandbox.SessionSettingsAuditMetadataDomain] != domainsandbox.SessionSettingsAuditDomain || len(event.Metadata) != 4 {
			t.Fatalf("unsafe Session audit metadata = %#v", event.Metadata)
		}
	}
	current, err := service.Get(context.Background(), testActor())
	if err != nil || current.Version != 2 || !current.Settings.CoreEnabled {
		t.Fatalf("Get() after failed apply = %#v, %v", current, err)
	}
}

func TestSessionSettingsServiceSuccessReportsExactAppliedVersion(t *testing.T) {
	store := &sessionSettingsStoreFake{settings: domainsandbox.DefaultSessionRuntimeSettings()}
	runner := &sessionRunnerFake{applyVersion: 2}
	service, _ := NewSessionSettingsService(SessionSettingsServiceOptions{Store: store, Runner: runner})
	next := domainsandbox.DefaultSessionRuntimeSettings()
	next.HostShellEnabled = true
	result, err := service.Update(context.Background(), testActor(), UpdateSessionSettingsRequest{ExpectedVersion: 1, Settings: next})
	if err != nil || !result.Applied || result.AppliedVersion != 2 || result.ReasonCode != "" {
		t.Fatalf("Update() = %#v, %v", result, err)
	}
}

func TestSessionSettingsServiceRejectsInteractiveAndStaleVersions(t *testing.T) {
	store := &sessionSettingsStoreFake{settings: domainsandbox.DefaultSessionRuntimeSettings()}
	service, _ := NewSessionSettingsService(SessionSettingsServiceOptions{Store: store})
	interactive := domainsandbox.DefaultSessionRuntimeSettings()
	interactive.InteractiveEnabled = true
	if _, err := service.Update(context.Background(), testActor(), UpdateSessionSettingsRequest{ExpectedVersion: 1, Settings: interactive}); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("interactive Update() error = %v", err)
	}
	if store.casCalls != 0 || len(store.events) != 0 {
		t.Fatal("interactive settings were persisted or audited")
	}

	current := domainsandbox.DefaultSessionRuntimeSettings()
	current.Version = 2
	store.settings = current
	next := domainsandbox.DefaultSessionRuntimeSettings()
	next.CoreEnabled = true
	if _, err := service.Update(context.Background(), testActor(), UpdateSessionSettingsRequest{ExpectedVersion: 1, Settings: next}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale Update() error = %v", err)
	}
	if store.casCalls != 0 || len(store.events) != 1 || store.events[0].Action != domainsandbox.SchedulerAuditActionUpdateFailed {
		t.Fatalf("stale update state: cas=%d events=%#v", store.casCalls, store.events)
	}
}

func TestSessionSettingsServiceDisableStillPersistsAndInvokesUnavailableRunner(t *testing.T) {
	current := domainsandbox.DefaultSessionRuntimeSettings()
	current.CoreEnabled = true
	current.Version = 3
	store := &sessionSettingsStoreFake{settings: current}
	runner := &sessionRunnerFake{err: domainsandbox.ErrUnavailable}
	service, _ := NewSessionSettingsService(SessionSettingsServiceOptions{Store: store, Runner: runner})
	next := domainsandbox.DefaultSessionRuntimeSettings()
	result, err := service.Update(context.Background(), testActor(), UpdateSessionSettingsRequest{ExpectedVersion: 3, Settings: next})
	if err != nil || result.Version != 4 || result.Applied || result.ReasonCode != SessionReasonRunnerUnavailable || runner.calls != 1 || store.settings.CoreEnabled {
		t.Fatalf("disable Update() = %#v, %v; calls=%d settings=%#v", result, err, runner.calls, store.settings)
	}
}

func TestSessionSettingsServiceRuntimeStatusProjectsOnlySafeAggregates(t *testing.T) {
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version = 4
	store := &sessionSettingsStoreFake{settings: settings}
	runner := &sessionRunnerFake{status: NativeSessionRuntimeStatus{
		Available: true, AppliedConfigVersion: 3, RuntimeGeneration: 7,
		CoreEnabled: true, InteractiveEnabled: false, HostShellEnabled: false, HostShellAvailable: false,
		RawAIOReady: true, GenerationState: "ready", QueueDepth: 2, Running: 1,
		UsedWeight: 1, TotalWeight: 2, ActiveSessions: 1, IdleSessions: 3,
		ActiveShells: 1, IdleShells: 2, TransportKnown: true, TransportEncrypted: true,
	}}
	service, _ := NewSessionSettingsService(SessionSettingsServiceOptions{Store: store, Runner: runner})
	result, err := service.RuntimeStatus(context.Background(), testActor())
	if err != nil || !result.Available || result.DesiredConfigVersion != 4 || result.AppliedConfigVersion != 3 || result.RuntimeGeneration != 7 ||
		result.GenerationState != "ready" || result.QueueDepth != 2 || result.ActiveSessions != 1 || result.IdleShells != 2 || !result.TransportKnown || !result.TransportEncrypted || result.ReasonCode != "" {
		t.Fatalf("RuntimeStatus() = %#v, %v", result, err)
	}

	service, _ = NewSessionSettingsService(SessionSettingsServiceOptions{Store: store})
	unavailable, err := service.RuntimeStatus(context.Background(), testActor())
	if err != nil || unavailable.Available || unavailable.DesiredConfigVersion != 4 || unavailable.TransportKnown || unavailable.TransportEncrypted || unavailable.ReasonCode != SessionReasonRunnerUnavailable {
		t.Fatalf("unavailable RuntimeStatus() = %#v, %v", unavailable, err)
	}
}

func TestSessionSettingsServiceRuntimeStatusAllowsInitialUnavailableZeroAppliedVersion(t *testing.T) {
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version = 1
	store := &sessionSettingsStoreFake{settings: settings}
	runner := &sessionRunnerFake{status: NativeSessionRuntimeStatus{
		Available: false, AppliedConfigVersion: 0, RuntimeGeneration: 0,
		GenerationState: "disabled", TotalWeight: 0, ReasonCode: "CONFIG_PENDING",
	}}
	service, _ := NewSessionSettingsService(SessionSettingsServiceOptions{Store: store, Runner: runner})
	result, err := service.RuntimeStatus(context.Background(), testActor())
	if err != nil || result.Available || result.AppliedConfigVersion != 0 || result.DesiredConfigVersion != 1 || result.GenerationState != "disabled" || result.ReasonCode != "CONFIG_PENDING" {
		t.Fatalf("RuntimeStatus() = %#v, %v", result, err)
	}
}

type sessionSettingsStoreFake struct {
	settings             domainsandbox.SessionRuntimeSettings
	updateErr            error
	events               []domainsandbox.SchedulerAuditEvent
	casCalls             int
	persistedBeforeApply bool
}

func (f *sessionSettingsStoreFake) GetSessionSettings(context.Context) (domainsandbox.SessionRuntimeSettings, error) {
	settings := f.settings
	if settings.Version == 0 {
		settings.Version = domainsandbox.InitialVersion
	}
	return settings, nil
}

func (*sessionSettingsStoreFake) UpdateSessionSettingsCAS(context.Context, domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, error) {
	return domainsandbox.SessionRuntimeSettings{}, errors.New("test must use atomic Session audit update")
}

func (f *sessionSettingsStoreFake) UpdateSessionSettingsCASWithAudit(_ context.Context, input domainsandbox.UpdateSessionSettingsInput, audit domainsandbox.AppendSchedulerAuditEventInput) (domainsandbox.SessionRuntimeSettings, error) {
	f.casCalls++
	if f.updateErr != nil {
		return domainsandbox.SessionRuntimeSettings{}, f.updateErr
	}
	f.settings = input.Settings
	f.settings.Version = input.ExpectedVersion + 1
	f.settings.UpdatedBy = input.UpdatedBy
	f.events = append(f.events, domainsandbox.SchedulerAuditEvent{Action: audit.Action, Metadata: audit.Metadata})
	f.persistedBeforeApply = true
	return f.settings, nil
}

func (f *sessionSettingsStoreFake) AppendSessionSettingsAuditEvent(_ context.Context, input domainsandbox.AppendSchedulerAuditEventInput) (*domainsandbox.SchedulerAuditEvent, error) {
	event := domainsandbox.SchedulerAuditEvent{Action: input.Action, Metadata: input.Metadata}
	f.events = append(f.events, event)
	return &event, nil
}

type sessionRunnerFake struct {
	applyVersion uint64
	err          error
	status       NativeSessionRuntimeStatus
	got          domainsandbox.SessionRuntimeSettings
	calls        int
}

func (f *sessionRunnerFake) ApplySessionSettings(_ context.Context, settings domainsandbox.SessionRuntimeSettings) (uint64, error) {
	f.calls++
	f.got = settings
	return f.applyVersion, f.err
}

func (f *sessionRunnerFake) SessionRuntimeStatus(context.Context) (NativeSessionRuntimeStatus, error) {
	return f.status, f.err
}
