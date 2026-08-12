// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestConfigurationStoreAppliesOnlyNewerVerifiedSnapshots(t *testing.T) {
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	store, err := NewConfigurationStore(signer, schedulerConfigurationSettings(1))
	require.NoError(t, err)
	next := schedulerConfigurationSettings(2)
	envelope, err := signer.Sign(next)
	require.NoError(t, err)
	require.NoError(t, store.ApplyConfiguration(context.Background(), envelope))
	require.Equal(t, uint64(2), store.Settings().Version)
	require.NoError(t, store.ApplyConfiguration(context.Background(), envelope))
	tampered := envelope
	tampered.Settings.MaxOutstanding = 31
	require.ErrorIs(t, store.ApplyConfiguration(context.Background(), tampered), ErrConfiguration)
}

func TestConfigurationStoreUpdatesRuntimeBeforePublishingNewVersion(t *testing.T) {
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	applier := &recordingSettingsApplier{}
	store, err := NewConfigurationStoreWithApplier(signer, schedulerConfigurationSettings(1), applier)
	require.NoError(t, err)
	next := schedulerConfigurationSettings(2)
	next.MaxOutstanding = 16
	envelope, err := signer.Sign(next)
	require.NoError(t, err)
	require.NoError(t, store.ApplyConfiguration(context.Background(), envelope))
	require.Equal(t, uint64(2), applier.settings.Version)
	require.Equal(t, uint64(2), store.Settings().Version)
	require.Equal(t, 16, store.Settings().MaxOutstanding)
}

func TestConfigurationStoreDoesNotPublishSnapshotWhenRuntimeUpdateFails(t *testing.T) {
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	applier := &recordingSettingsApplier{err: ErrUnavailable}
	store, err := NewConfigurationStoreWithApplier(signer, schedulerConfigurationSettings(1), applier)
	require.NoError(t, err)
	envelope, err := signer.Sign(schedulerConfigurationSettings(2))
	require.NoError(t, err)
	require.ErrorIs(t, store.ApplyConfiguration(context.Background(), envelope), ErrConfiguration)
	require.Equal(t, uint64(1), store.Settings().Version)
}

type recordingSettingsApplier struct {
	settings domainsandbox.SchedulerSettings
	err      error
}

func (applier *recordingSettingsApplier) ApplySchedulerSettings(_ context.Context, settings domainsandbox.SchedulerSettings) error {
	if applier.err != nil {
		return applier.err
	}
	applier.settings = settings
	return nil
}

func schedulerConfigurationSettings(version uint64) domainsandbox.SchedulerSettings {
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = version
	return settings
}
