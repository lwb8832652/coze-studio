// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSchedulerConfigSignerSignsCanonicalSnapshotAndRejectsTampering(t *testing.T) {
	signer, err := NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return now }
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 7
	value, err := signer.Sign(settings)
	require.NoError(t, err)
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	decoded, err := DecodeSchedulerConfiguration(encoded)
	require.NoError(t, err)
	require.Equal(t, value, decoded)
	verified, err := signer.Verify(value, 6)
	require.NoError(t, err)
	require.Equal(t, uint64(7), verified.Version)
	value.Settings.MaxOutstanding = 31
	_, err = signer.Verify(value, 6)
	require.ErrorIs(t, err, ErrSchedulerConfiguration)
}

func TestSchedulerConfigSignerRejectsStaleAndFutureConfigurations(t *testing.T) {
	signer, err := NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return now }
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 7
	value, err := signer.Sign(settings)
	require.NoError(t, err)
	_, err = signer.Verify(value, 8)
	require.ErrorIs(t, err, ErrSchedulerConfiguration)
	signer.now = func() time.Time { return now.Add(2 * time.Minute) }
	_, err = signer.Verify(value, 6)
	require.ErrorIs(t, err, ErrSchedulerConfiguration)
}

func TestDecodeSchedulerConfigurationRejectsDuplicateJSONFields(t *testing.T) {
	_, err := DecodeSchedulerConfiguration([]byte(`{
		"schema":"coze.sandbox.scheduler_config.v1",
		"schema":"coze.sandbox.scheduler_config.v1",
		"key_id":"key-1",
		"issued_at":"2026-08-12T00:00:00Z",
		"expires_at":"2026-08-12T00:01:00Z",
		"version":1,
		"settings":{},
		"signature":"AA"
	}`))
	require.ErrorIs(t, err, ErrSchedulerConfiguration)
}

func TestLoadSchedulerConfigSignerFromEnvRequiresCompleteKeyring(t *testing.T) {
	getenv := func(name string) string {
		switch name {
		case SandboxRunnerConfigSigningKeysJSONEnv:
			return `{"keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`
		case SandboxRunnerActiveConfigKeyIDEnv:
			return "key-1"
		default:
			return ""
		}
	}
	signer, configured, err := LoadSchedulerConfigSignerFromEnv(getenv, time.Minute)
	require.NoError(t, err)
	require.True(t, configured)
	require.NotNil(t, signer)

	_, configured, err = LoadSchedulerConfigSignerFromEnv(func(string) string { return "" }, time.Minute)
	require.NoError(t, err)
	require.False(t, configured)

	_, configured, err = LoadSchedulerConfigSignerFromEnv(func(name string) string {
		if name == SandboxRunnerConfigSigningKeysJSONEnv {
			return `{"keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`
		}
		return ""
	}, time.Minute)
	require.ErrorIs(t, err, ErrSchedulerConfiguration)
	require.False(t, configured)
}
