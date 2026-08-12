// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestDefaultProviderSchedulerRunnerPushesOnceToEachHealthyNativeDefault(t *testing.T) {
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	provider := schedulerNativeProvider(7, now)
	store := &schedulerDefaultProviderStoreFake{
		defaults: []*domainsandbox.ProviderDefault{
			{Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID},
			{Scope: domainsandbox.ScopeAppDev, ProviderID: provider.ID},
			{Scope: domainsandbox.ScopePlugin, ProviderID: 8},
		},
		providers: map[int64]*domainsandbox.Provider{
			provider.ID: provider,
			8:           schedulerNonNativeProvider(8, now),
		},
	}
	remote := &schedulerRuntimeRemoteFake{schedulerConfigurationRemoteFake: &schedulerConfigurationRemoteFake{}}
	runner, err := NewDefaultProviderSchedulerRunner(DefaultProviderSchedulerRunnerOptions{
		Signer: signer, Defaults: store, Providers: store, Factory: schedulerRemoteFactoryFake{runtime: remote}, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 2

	require.NoError(t, runner.ApplySchedulerSettings(context.Background(), settings))
	require.Len(t, remote.applied, 1)
	require.Equal(t, 1, remote.closeCalls)

	_, err = runner.RuntimeStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, remote.closeCalls)
}

type schedulerDefaultProviderStoreFake struct {
	defaults  []*domainsandbox.ProviderDefault
	providers map[int64]*domainsandbox.Provider
}

func (f *schedulerDefaultProviderStoreFake) ListProviderDefaults(context.Context) ([]*domainsandbox.ProviderDefault, error) {
	return f.defaults, nil
}

func (f *schedulerDefaultProviderStoreFake) GetProviderDefault(_ context.Context, scope domainsandbox.Scope) (*domainsandbox.ProviderDefault, error) {
	for _, item := range f.defaults {
		if item != nil && item.Scope == scope {
			return item, nil
		}
	}
	return nil, domainsandbox.ErrDefaultMissing
}

func (*schedulerDefaultProviderStoreFake) SetProviderDefault(context.Context, domainsandbox.SetProviderDefaultInput) (*domainsandbox.ProviderDefault, error) {
	return nil, domainsandbox.ErrInvalidInput
}

func (f *schedulerDefaultProviderStoreFake) GetProvider(_ context.Context, id int64) (*domainsandbox.Provider, error) {
	return f.providers[id], nil
}

type schedulerRemoteFactoryFake struct{ runtime *schedulerRuntimeRemoteFake }

func (schedulerRemoteFactoryFake) ValidateConfig(context.Context, ProviderDescriptor) error {
	return nil
}

func (f schedulerRemoteFactoryFake) Build(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return f.runtime, nil
}

type schedulerRuntimeRemoteFake struct {
	*schedulerConfigurationRemoteFake
	closeCalls int
}

func (*schedulerRuntimeRemoteFake) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}
func (*schedulerRuntimeRemoteFake) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}
func (*schedulerRuntimeRemoteFake) Cancel(context.Context, string) error { return nil }
func (f *schedulerRuntimeRemoteFake) CloseContext(context.Context) error {
	f.closeCalls++
	return nil
}

func schedulerNativeProvider(id int64, now time.Time) *domainsandbox.Provider {
	return &domainsandbox.Provider{
		ID: id, Type: domainsandbox.ProviderTypeRemoteHTTP, Status: domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusHealthy, CheckedAt: now, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev}, Features: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1, domainsandbox.ProviderFeatureSignedExecutionContext}},
	}
}

func schedulerNonNativeProvider(id int64, now time.Time) *domainsandbox.Provider {
	provider := schedulerNativeProvider(id, now)
	provider.Health.Features = nil
	return provider
}
