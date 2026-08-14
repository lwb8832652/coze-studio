// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"sort"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const sessionRemoteCloseTimeout = 5 * time.Second

// SessionConfigurationRemote is the optional, aggregate-only management
// capability of a Session runtime. It is deliberately separate from Session
// execution so a provider cannot be selected for Core traffic merely because
// it exposes the configuration route.
type SessionConfigurationRemote interface {
	ApplySessionSettings(context.Context, domainsandbox.SessionRuntimeSettings) (uint64, error)
	SessionRuntimeStatus(context.Context) (infrasandbox.SessionRuntimeStatus, error)
}

// DefaultProviderSessionRunner resolves the current default set for every
// management operation. Exactly one distinct enabled remote provider must be
// present. Multiple scopes may point to that same provider and are deduplicated.
// Health feature advertisement is intentionally not an eligibility condition:
// the configuration route must be able to enable Core before Session features
// can become healthy.
type DefaultProviderSessionRunner struct {
	defaults  domainsandbox.ProviderDefaultRepository
	providers interface {
		GetProvider(context.Context, int64) (*domainsandbox.Provider, error)
	}
	factory SessionRuntimeProviderFactory
}

type DefaultProviderSessionRunnerOptions struct {
	Defaults  domainsandbox.ProviderDefaultRepository
	Providers interface {
		GetProvider(context.Context, int64) (*domainsandbox.Provider, error)
	}
	Factory SessionRuntimeProviderFactory
}

func NewDefaultProviderSessionRunner(options DefaultProviderSessionRunnerOptions) (*DefaultProviderSessionRunner, error) {
	if options.Defaults == nil || options.Providers == nil || options.Factory == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &DefaultProviderSessionRunner{
		defaults: options.Defaults, providers: options.Providers, factory: options.Factory,
	}, nil
}

func (runner *DefaultProviderSessionRunner) ApplySessionSettings(
	ctx context.Context,
	settings domainsandbox.SessionRuntimeSettings,
) (uint64, error) {
	if runner == nil || ctx == nil {
		return 0, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	normalized, err := domainsandbox.NormalizeSessionRuntimeSettings(settings)
	if err != nil || normalized.Version < domainsandbox.InitialVersion {
		return 0, domainsandbox.ErrInvalidInput
	}
	remote, runtime, err := runner.sessionRemote(ctx)
	if err != nil {
		return 0, err
	}
	defer closeManagedSessionRuntime(ctx, runtime)
	appliedVersion, err := remote.ApplySessionSettings(ctx, normalized)
	if err != nil || appliedVersion != normalized.Version {
		return 0, normalizeSessionRunnerError(ctx, err)
	}
	return appliedVersion, nil
}

func (runner *DefaultProviderSessionRunner) SessionRuntimeStatus(ctx context.Context) (NativeSessionRuntimeStatus, error) {
	if runner == nil || ctx == nil {
		return NativeSessionRuntimeStatus{}, domainsandbox.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return NativeSessionRuntimeStatus{}, err
	}
	remote, runtime, err := runner.sessionRemote(ctx)
	if err != nil {
		return NativeSessionRuntimeStatus{}, err
	}
	defer closeManagedSessionRuntime(ctx, runtime)
	status, err := remote.SessionRuntimeStatus(ctx)
	if err != nil {
		return NativeSessionRuntimeStatus{}, normalizeSessionRunnerError(ctx, err)
	}
	return NativeSessionRuntimeStatus{
		Available: status.Available, AppliedConfigVersion: status.AppliedConfigVersion,
		RuntimeGeneration: status.RuntimeGeneration, CoreEnabled: status.CoreEnabled,
		InteractiveEnabled: status.InteractiveEnabled, RawAIOReady: status.RawAIOReady,
		GenerationState: status.GenerationState, QueueDepth: status.QueueDepth,
		Running: status.Running, UsedWeight: status.UsedWeight, TotalWeight: status.TotalWeight,
		ActiveSessions: status.ActiveSessions, IdleSessions: status.IdleSessions,
		ActiveShells: status.ActiveShells, IdleShells: status.IdleShells,
		TransportKnown: true, TransportEncrypted: status.TransportEncrypted, ReasonCode: status.ReasonCode,
	}, nil
}

func (runner *DefaultProviderSessionRunner) sessionRemote(
	ctx context.Context,
) (SessionConfigurationRemote, infrasandbox.SessionRuntimeProvider, error) {
	if runner == nil || runner.defaults == nil || runner.providers == nil || runner.factory == nil {
		return nil, nil, domainsandbox.ErrUnavailable
	}
	defaults, err := runner.defaults.ListProviderDefaults(ctx)
	if err != nil {
		return nil, nil, normalizeSessionRunnerError(ctx, err)
	}
	targets := make(map[int64]domainsandbox.Scope, len(defaults))
	for _, item := range defaults {
		if item == nil || item.ProviderID <= 0 {
			return nil, nil, domainsandbox.ErrUnavailable
		}
		if current, exists := targets[item.ProviderID]; !exists || string(item.Scope) < string(current) {
			targets[item.ProviderID] = item.Scope
		}
	}
	providerIDs := make([]int64, 0, len(targets))
	for providerID := range targets {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Slice(providerIDs, func(left, right int) bool { return providerIDs[left] < providerIDs[right] })

	type candidate struct {
		provider   domainsandbox.Provider
		descriptor ProviderDescriptor
	}
	candidates := make([]candidate, 0, 1)
	for _, providerID := range providerIDs {
		provider, lookupErr := runner.providers.GetProvider(ctx, providerID)
		if lookupErr != nil || provider == nil {
			return nil, nil, normalizeSessionRunnerError(ctx, lookupErr)
		}
		if provider.Type != domainsandbox.ProviderTypeRemoteHTTP {
			continue
		}
		scope := targets[providerID]
		if provider.Status != domainsandbox.ProviderStatusEnabled || provider.DeletedAt != nil ||
			domainsandbox.ValidateProviderKey(provider.ProviderKey) != nil || !scopeIncluded(provider.Scopes, scope) ||
			domainsandbox.ValidateRuntimePolicy(provider.Policy) != nil {
			return nil, nil, domainsandbox.ErrUnavailable
		}
		descriptor := ProviderDescriptor{
			ProviderKey: provider.ProviderKey, ProviderType: provider.Type,
			Scope: scope, Policy: cloneRuntimePolicy(provider.Policy),
			features: append([]domainsandbox.ProviderFeature(nil), provider.Health.Features...),
		}
		candidates = append(candidates, candidate{provider: cloneProviderForAdapter(*provider, descriptor.Policy), descriptor: descriptor})
	}
	if len(candidates) != 1 {
		return nil, nil, domainsandbox.ErrUnavailable
	}
	runtime, buildErr := runner.factory.BuildSession(ctx, candidates[0].provider, candidates[0].descriptor)
	if buildErr != nil || runtime == nil {
		if runtime != nil {
			closeManagedSessionRuntime(ctx, runtime)
		}
		return nil, nil, normalizeSessionRunnerError(ctx, buildErr)
	}
	remote, ok := runtime.(SessionConfigurationRemote)
	if !ok || remote == nil {
		closeManagedSessionRuntime(ctx, runtime)
		return nil, nil, domainsandbox.ErrUnavailable
	}
	return remote, runtime, nil
}

func normalizeSessionRunnerError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && (err == context.Canceled || err == context.DeadlineExceeded) {
		return err
	}
	return domainsandbox.ErrUnavailable
}

func closeManagedSessionRuntime(ctx context.Context, runtime infrasandbox.SessionRuntimeProvider) {
	if runtime == nil {
		return
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	closeCtx, cancel := context.WithTimeout(base, sessionRemoteCloseTimeout)
	_ = closeSessionRuntimeProvider(closeCtx, runtime)
	cancel()
}

var _ NativeSessionRunner = (*DefaultProviderSessionRunner)(nil)
