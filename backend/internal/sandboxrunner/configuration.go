// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"sync"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

// ConfigurationStore verifies complete snapshots first, then swaps the active
// projection under one lock. Running executions keep their captured version.
type ConfigurationStore struct {
	mu      sync.RWMutex
	signer  *infrasandbox.SchedulerConfigSigner
	current domainsandbox.SchedulerSettings
	applier SchedulerSettingsApplier
}

// SchedulerSettingsApplier updates the runtime components that consume a
// scheduler snapshot. It receives only already verified, non-secret settings.
type SchedulerSettingsApplier interface {
	ApplySchedulerSettings(context.Context, domainsandbox.SchedulerSettings) error
}

func NewConfigurationStore(signer *infrasandbox.SchedulerConfigSigner, initial domainsandbox.SchedulerSettings) (*ConfigurationStore, error) {
	return NewConfigurationStoreWithApplier(signer, initial, nil)
}

func NewConfigurationStoreWithApplier(signer *infrasandbox.SchedulerConfigSigner, initial domainsandbox.SchedulerSettings, applier SchedulerSettingsApplier) (*ConfigurationStore, error) {
	if signer == nil {
		return nil, ErrConfiguration
	}
	normalized, err := domainsandbox.NormalizeSchedulerSettings(initial)
	if err != nil || normalized.Version == 0 {
		return nil, ErrConfiguration
	}
	return &ConfigurationStore{signer: signer, current: normalized, applier: applier}, nil
}

func (store *ConfigurationStore) ApplyConfiguration(ctx context.Context, value infrasandbox.SchedulerConfiguration) error {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return ErrConfiguration
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	settings, err := store.signer.Verify(value, store.current.Version)
	if err != nil {
		return ErrConfiguration
	}
	if settings.Version == store.current.Version {
		return nil
	}
	if store.applier != nil && store.applier.ApplySchedulerSettings(ctx, settings) != nil {
		return ErrConfiguration
	}
	store.current = settings
	return nil
}

func (store *ConfigurationStore) Configuration(context.Context) (ConfigurationProjection, error) {
	if store == nil {
		return ConfigurationProjection{}, ErrUnavailable
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return ConfigurationProjection{Schema: infrasandbox.SchedulerConfigurationSchemaV1, Version: store.current.Version}, nil
}

func (store *ConfigurationStore) Settings() domainsandbox.SchedulerSettings {
	if store == nil {
		return domainsandbox.SchedulerSettings{}
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	settings, err := domainsandbox.NormalizeSchedulerSettings(store.current)
	if err != nil {
		return domainsandbox.SchedulerSettings{}
	}
	return settings
}

var _ ConfigurationSource = (*ConfigurationStore)(nil)
var _ ConfigurationApplier = (*ConfigurationStore)(nil)
