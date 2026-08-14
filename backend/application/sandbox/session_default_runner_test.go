// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestDefaultProviderSessionRunnerDeduplicatesRemoteDefaultsAndAppliesExactSettings(t *testing.T) {
	now := time.Unix(2_000_009_000, 0).UTC()
	provider := sessionConfigurationProvider(17, now)
	// Configuration distribution must work before Session features are
	// advertised by health; enabling Core is what makes those features ready.
	provider.Health.Features = nil
	store := &sessionDefaultProviderStoreFake{
		defaults: []*domainsandbox.ProviderDefault{
			{Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID},
			{Scope: domainsandbox.ScopePlugin, ProviderID: provider.ID},
		},
		providers: map[int64]*domainsandbox.Provider{provider.ID: provider},
	}
	remote := &sessionConfigurationRemoteFake{}
	factory := &sessionDefaultFactoryFake{runtime: remote}
	runner, err := NewDefaultProviderSessionRunner(DefaultProviderSessionRunnerOptions{
		Defaults: store, Providers: store, Factory: factory,
	})
	if err != nil {
		t.Fatalf("NewDefaultProviderSessionRunner() error = %v", err)
	}
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.CoreEnabled = true
	settings.Version = 3
	settings.UpdatedBy = 41

	applied, err := runner.ApplySessionSettings(context.Background(), settings)
	if err != nil || applied != settings.Version {
		t.Fatalf("ApplySessionSettings() = %d, %v", applied, err)
	}
	if len(remote.applied) != 1 || !reflect.DeepEqual(remote.applied[0], settings) {
		t.Fatalf("remote applied = %#v, want exact persisted snapshot %#v", remote.applied, settings)
	}
	if factory.buildCalls != 1 || factory.descriptors[0].Scope != domainsandbox.ScopeAgent {
		t.Fatalf("BuildSession calls/scope = %d/%q", factory.buildCalls, factory.descriptors[0].Scope)
	}
	if factory.validateCalls != 0 {
		t.Fatalf("configuration path required advertised Session features via ValidateSessionConfig: %d", factory.validateCalls)
	}
	if remote.closeCalls != 1 {
		t.Fatalf("remote CloseContext() calls = %d, want 1", remote.closeCalls)
	}
}

func TestDefaultProviderSessionRunnerFailsClosedForAmbiguousOrMissingRemoteTarget(t *testing.T) {
	now := time.Unix(2_000_009_100, 0).UTC()
	remoteOne := sessionConfigurationProvider(17, now)
	remoteTwo := sessionConfigurationProvider(18, now)
	local := sessionConfigurationProvider(19, now)
	local.Type = domainsandbox.ProviderTypeLocalDebug
	disabled := sessionConfigurationProvider(20, now)
	disabled.Status = domainsandbox.ProviderStatusDisabled
	tests := []struct {
		name     string
		defaults []*domainsandbox.ProviderDefault
		values   map[int64]*domainsandbox.Provider
		storeErr error
	}{
		{name: "no defaults"},
		{name: "only local default", defaults: []*domainsandbox.ProviderDefault{{Scope: domainsandbox.ScopeAgent, ProviderID: local.ID}}, values: map[int64]*domainsandbox.Provider{local.ID: local}},
		{name: "disabled remote default", defaults: []*domainsandbox.ProviderDefault{{Scope: domainsandbox.ScopeAgent, ProviderID: disabled.ID}}, values: map[int64]*domainsandbox.Provider{disabled.ID: disabled}},
		{name: "two different remote defaults", defaults: []*domainsandbox.ProviderDefault{{Scope: domainsandbox.ScopeAgent, ProviderID: remoteOne.ID}, {Scope: domainsandbox.ScopePlugin, ProviderID: remoteTwo.ID}}, values: map[int64]*domainsandbox.Provider{remoteOne.ID: remoteOne, remoteTwo.ID: remoteTwo}},
		{name: "defaults repository unavailable", storeErr: errors.New("database unavailable")},
		{name: "referenced provider missing", defaults: []*domainsandbox.ProviderDefault{{Scope: domainsandbox.ScopeAgent, ProviderID: 99}}, values: map[int64]*domainsandbox.Provider{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &sessionDefaultProviderStoreFake{defaults: test.defaults, providers: test.values, err: test.storeErr}
			factory := &sessionDefaultFactoryFake{runtime: &sessionConfigurationRemoteFake{}}
			runner, err := NewDefaultProviderSessionRunner(DefaultProviderSessionRunnerOptions{Defaults: store, Providers: store, Factory: factory})
			if err != nil {
				t.Fatalf("NewDefaultProviderSessionRunner() error = %v", err)
			}
			settings := domainsandbox.DefaultSessionRuntimeSettings()
			settings.Version = 2
			if applied, applyErr := runner.ApplySessionSettings(context.Background(), settings); !errors.Is(applyErr, domainsandbox.ErrUnavailable) || applied != 0 {
				t.Fatalf("ApplySessionSettings() = %d, %v; want fail closed", applied, applyErr)
			}
			if factory.buildCalls != 0 {
				t.Fatalf("ambiguous/missing target reached factory %d times", factory.buildCalls)
			}
		})
	}
}

func TestDefaultProviderSessionRunnerProjectsOnlyStrictRuntimeAggregate(t *testing.T) {
	now := time.Unix(2_000_009_200, 0).UTC()
	provider := sessionConfigurationProvider(17, now)
	store := &sessionDefaultProviderStoreFake{
		defaults:  []*domainsandbox.ProviderDefault{{Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID}},
		providers: map[int64]*domainsandbox.Provider{provider.ID: provider},
	}
	remoteStatus := infrasandbox.SessionRuntimeStatus{
		Available: true, AppliedConfigVersion: 7, RuntimeGeneration: 9,
		CoreEnabled: true, RawAIOReady: true, GenerationState: "ready",
		QueueDepth: 2, Running: 1, UsedWeight: 1, TotalWeight: 2,
		ActiveSessions: 1, IdleSessions: 3, ActiveShells: 1, IdleShells: 2,
		TransportEncrypted: true,
	}
	remote := &sessionConfigurationRemoteFake{status: remoteStatus}
	runner, err := NewDefaultProviderSessionRunner(DefaultProviderSessionRunnerOptions{
		Defaults: store, Providers: store, Factory: &sessionDefaultFactoryFake{runtime: remote},
	})
	if err != nil {
		t.Fatalf("NewDefaultProviderSessionRunner() error = %v", err)
	}

	status, err := runner.SessionRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("SessionRuntimeStatus() error = %v", err)
	}
	want := NativeSessionRuntimeStatus{
		Available: true, AppliedConfigVersion: 7, RuntimeGeneration: 9,
		CoreEnabled: true, RawAIOReady: true, GenerationState: "ready",
		QueueDepth: 2, Running: 1, UsedWeight: 1, TotalWeight: 2,
		ActiveSessions: 1, IdleSessions: 3, ActiveShells: 1, IdleShells: 2,
		TransportKnown: true, TransportEncrypted: true,
	}
	if !reflect.DeepEqual(status, want) {
		t.Fatalf("SessionRuntimeStatus() = %#v, want %#v", status, want)
	}
	if remote.closeCalls != 1 {
		t.Fatalf("runtime status transport close calls = %d", remote.closeCalls)
	}
}

func TestDefaultProviderSessionRunnerRejectsRuntimeWithoutConfigurationCapability(t *testing.T) {
	now := time.Unix(2_000_009_300, 0).UTC()
	provider := sessionConfigurationProvider(17, now)
	store := &sessionDefaultProviderStoreFake{
		defaults:  []*domainsandbox.ProviderDefault{{Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID}},
		providers: map[int64]*domainsandbox.Provider{provider.ID: provider},
	}
	runtime := &sessionRuntimeProviderStub{}
	runner, err := NewDefaultProviderSessionRunner(DefaultProviderSessionRunnerOptions{
		Defaults: store, Providers: store, Factory: &sessionDefaultFactoryFake{runtime: runtime},
	})
	if err != nil {
		t.Fatalf("NewDefaultProviderSessionRunner() error = %v", err)
	}
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version = 2
	if _, err := runner.ApplySessionSettings(context.Background(), settings); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("ApplySessionSettings() error = %v", err)
	}
	if runtime.closeCalls != 1 {
		t.Fatalf("incompatible runtime CloseContext() calls = %d", runtime.closeCalls)
	}
}

type sessionDefaultProviderStoreFake struct {
	defaults  []*domainsandbox.ProviderDefault
	providers map[int64]*domainsandbox.Provider
	err       error
}

func (f *sessionDefaultProviderStoreFake) ListProviderDefaults(context.Context) ([]*domainsandbox.ProviderDefault, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.defaults, nil
}

func (*sessionDefaultProviderStoreFake) GetProviderDefault(context.Context, domainsandbox.Scope) (*domainsandbox.ProviderDefault, error) {
	return nil, domainsandbox.ErrDefaultMissing
}

func (*sessionDefaultProviderStoreFake) SetProviderDefault(context.Context, domainsandbox.SetProviderDefaultInput) (*domainsandbox.ProviderDefault, error) {
	return nil, domainsandbox.ErrInvalidInput
}

func (f *sessionDefaultProviderStoreFake) GetProvider(_ context.Context, id int64) (*domainsandbox.Provider, error) {
	if f.err != nil {
		return nil, f.err
	}
	provider := f.providers[id]
	if provider == nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *provider
	return &copy, nil
}

type sessionDefaultFactoryFake struct {
	runtime       infrasandbox.SessionRuntimeProvider
	validateCalls int
	buildCalls    int
	descriptors   []ProviderDescriptor
}

func (*sessionDefaultFactoryFake) ValidateConfig(context.Context, ProviderDescriptor) error {
	return nil
}

func (*sessionDefaultFactoryFake) Build(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return nil, domainsandbox.ErrExecutionForbidden
}

func (f *sessionDefaultFactoryFake) ValidateSessionConfig(context.Context, ProviderDescriptor) error {
	f.validateCalls++
	return domainsandbox.ErrScopeUnsupported
}

func (f *sessionDefaultFactoryFake) BuildSession(_ context.Context, _ domainsandbox.Provider, descriptor ProviderDescriptor) (infrasandbox.SessionRuntimeProvider, error) {
	f.buildCalls++
	f.descriptors = append(f.descriptors, descriptor)
	return f.runtime, nil
}

type sessionConfigurationRemoteFake struct {
	sessionRuntimeProviderStub
	applied []domainsandbox.SessionRuntimeSettings
	status  infrasandbox.SessionRuntimeStatus
}

func (f *sessionConfigurationRemoteFake) ApplySessionSettings(_ context.Context, settings domainsandbox.SessionRuntimeSettings) (uint64, error) {
	f.applied = append(f.applied, settings)
	return settings.Version, nil
}

func (f *sessionConfigurationRemoteFake) SessionRuntimeStatus(context.Context) (infrasandbox.SessionRuntimeStatus, error) {
	return f.status, nil
}

func sessionConfigurationProvider(id int64, now time.Time) *domainsandbox.Provider {
	return &domainsandbox.Provider{
		ID: id, ProviderKey: "session-runner-provider-" + string(rune('a'+id%26)),
		Type: domainsandbox.ProviderTypeRemoteHTTP, Status: domainsandbox.ProviderStatusEnabled,
		Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopePlugin},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30, MemoryLimitMB: 128, CPULimit: 1,
			MaxOutputBytes: 4096, MaxConcurrency: 2,
		},
		Health: domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusUnknown, CheckedAt: now},
	}
}
