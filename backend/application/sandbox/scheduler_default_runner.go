// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"sort"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

// DefaultProviderSchedulerRunner resolves default Providers at every apply so
// an administrator's default/health change cannot leave a stale Runner target
// in the configuration distribution path.
type DefaultProviderSchedulerRunner struct {
	signer    *infrasandbox.SchedulerConfigSigner
	defaults  domainsandbox.ProviderDefaultRepository
	providers interface {
		GetProvider(context.Context, int64) (*domainsandbox.Provider, error)
	}
	factory RuntimeProviderFactory
	now     func() time.Time

	mu      sync.RWMutex
	lastSet domainsandbox.SchedulerSettings
}

type DefaultProviderSchedulerRunnerOptions struct {
	Signer    *infrasandbox.SchedulerConfigSigner
	Defaults  domainsandbox.ProviderDefaultRepository
	Providers interface {
		GetProvider(context.Context, int64) (*domainsandbox.Provider, error)
	}
	Factory RuntimeProviderFactory
	Now     func() time.Time
}

func NewDefaultProviderSchedulerRunner(options DefaultProviderSchedulerRunnerOptions) (*DefaultProviderSchedulerRunner, error) {
	if options.Signer == nil || options.Defaults == nil || options.Providers == nil || options.Factory == nil || options.Now == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &DefaultProviderSchedulerRunner{signer: options.Signer, defaults: options.Defaults, providers: options.Providers, factory: options.Factory, now: options.Now}, nil
}

func (runner *DefaultProviderSchedulerRunner) ApplySchedulerSettings(ctx context.Context, settings domainsandbox.SchedulerSettings) error {
	if runner == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSchedulerSettings(settings)
	if err != nil || normalized.Version == 0 {
		return domainsandbox.ErrInvalidInput
	}
	configuration, err := runner.signer.Sign(normalized)
	if err != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	remotes, err := runner.nativeDefaults(ctx)
	if err != nil {
		return err
	}
	if len(remotes) == 0 {
		return domainsandbox.ErrUnavailable
	}
	defer closeSchedulerRemotes(ctx, remotes)
	for _, remote := range remotes {
		if err := remote.remote.ApplySchedulerConfiguration(ctx, configuration); err != nil {
			return domainsandbox.ErrUnavailable
		}
	}
	runner.mu.Lock()
	runner.lastSet = normalized
	runner.mu.Unlock()
	return nil
}

func (runner *DefaultProviderSchedulerRunner) RuntimeStatus(ctx context.Context) (NativeRunnerStatus, error) {
	if runner == nil || ctx == nil {
		return NativeRunnerStatus{}, domainsandbox.ErrUnavailable
	}
	remotes, err := runner.nativeDefaults(ctx)
	if err != nil || len(remotes) == 0 {
		return NativeRunnerStatus{}, domainsandbox.ErrUnavailable
	}
	defer closeSchedulerRemotes(ctx, remotes)
	status, statusErr := remotes[0].remote.RuntimeStatus(ctx)
	if statusErr != nil {
		return NativeRunnerStatus{}, domainsandbox.ErrUnavailable
	}
	return nativeRunnerStatusFromRemote(status), nil
}

func (runner *DefaultProviderSchedulerRunner) nativeDefaults(ctx context.Context) ([]managedSchedulerRemote, error) {
	defaults, err := runner.defaults.ListProviderDefaults(ctx)
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	providerIDs := make(map[int64]struct{}, len(defaults))
	for _, item := range defaults {
		if item != nil && item.ProviderID > 0 {
			providerIDs[item.ProviderID] = struct{}{}
		}
	}
	orderedIDs := make([]int64, 0, len(providerIDs))
	for providerID := range providerIDs {
		orderedIDs = append(orderedIDs, providerID)
	}
	sort.Slice(orderedIDs, func(left, right int) bool { return orderedIDs[left] < orderedIDs[right] })
	remotes := make([]managedSchedulerRemote, 0, len(orderedIDs))
	for _, providerID := range orderedIDs {
		provider, err := runner.providers.GetProvider(ctx, providerID)
		if err != nil || !isHealthyNativeDefault(provider, runner.now()) {
			continue
		}
		runtime, err := runner.factory.Build(ctx, *provider)
		if err != nil || runtime == nil {
			return nil, domainsandbox.ErrUnavailable
		}
		remote, ok := runtime.(SchedulerConfigurationRemote)
		if !ok {
			_ = runtime.CloseContext(ctx)
			return nil, domainsandbox.ErrUnavailable
		}
		remotes = append(remotes, managedSchedulerRemote{remote: remote, runtime: runtime})
	}
	return remotes, nil
}

type managedSchedulerRemote struct {
	remote  SchedulerConfigurationRemote
	runtime infrasandbox.RuntimeProvider
}

func closeSchedulerRemotes(ctx context.Context, remotes []managedSchedulerRemote) {
	for _, remote := range remotes {
		if remote.runtime != nil {
			_ = remote.runtime.CloseContext(ctx)
		}
	}
}

func isHealthyNativeDefault(provider *domainsandbox.Provider, now time.Time) bool {
	if provider == nil || provider.Type != domainsandbox.ProviderTypeRemoteHTTP || provider.Status != domainsandbox.ProviderStatusEnabled ||
		provider.Health.Status != domainsandbox.HealthStatusHealthy || !freshHealthSnapshot(provider.Health.CheckedAt, now) {
		return false
	}
	hasQueue, hasIdentity := false, false
	for _, feature := range provider.Health.Features {
		hasQueue = hasQueue || feature == domainsandbox.ProviderFeatureQueueStatusV1
		hasIdentity = hasIdentity || feature == domainsandbox.ProviderFeatureSignedExecutionContext
	}
	return hasQueue && hasIdentity
}

var _ NativeSchedulerRunner = (*DefaultProviderSchedulerRunner)(nil)
