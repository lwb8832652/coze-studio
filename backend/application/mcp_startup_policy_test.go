// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/application/mcptool"
)

func TestMCPStartupCatalogOptionsRequireExplicitAESKeyWhenEnabled(t *testing.T) {
	t.Run("disabled does not inspect key", func(t *testing.T) {
		t.Setenv(mcptool.MCPAESAuthSecretEnv, "invalid")

		options, err := mcpCatalogOptionsFromEnv(false)

		require.NoError(t, err)
		require.Empty(t, options)
	})

	for _, tt := range []struct {
		name   string
		secret string
	}{
		{name: "missing", secret: ""},
		{name: "invalid length", secret: "too-short"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(mcptool.MCPAESAuthSecretEnv, tt.secret)

			options, err := mcpCatalogOptionsFromEnv(true)

			require.Error(t, err)
			require.Empty(t, options)
			require.Contains(t, err.Error(), mcptool.MCPAESAuthSecretEnv)
			if tt.secret != "" {
				require.NotContains(t, err.Error(), tt.secret)
			}
		})
	}

	for _, tt := range []struct {
		name   string
		secret string
	}{
		{name: "valid 16 byte key", secret: "0123456789abcdef"},
		{name: "valid 24 byte key", secret: "0123456789abcdefghijklmn"},
		{name: "valid 32 byte key", secret: "0123456789abcdefghijklmnopqrstuv"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(mcptool.MCPAESAuthSecretEnv, tt.secret)

			options, err := mcpCatalogOptionsFromEnv(true)

			require.NoError(t, err)
			require.Len(t, options, 1)
		})
	}
}

func TestMCPManagementEnablementIsIndependentFromRuntime(t *testing.T) {
	t.Run("unset follows runtime enablement", func(t *testing.T) {
		t.Setenv(mcpManagementEnabledEnv, "")

		disabled, err := mcpManagementEnabledFromEnv(false)
		require.NoError(t, err)
		require.False(t, disabled)

		enabled, err := mcpManagementEnabledFromEnv(true)
		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("management can be enabled while runtime is disabled", func(t *testing.T) {
		t.Setenv(mcpManagementEnabledEnv, "true")

		enabled, err := mcpManagementEnabledFromEnv(false)

		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("management can be disabled while runtime is enabled", func(t *testing.T) {
		t.Setenv(mcpManagementEnabledEnv, "false")

		enabled, err := mcpManagementEnabledFromEnv(true)

		require.NoError(t, err)
		require.False(t, enabled)
	})

	t.Run("invalid explicit value fails closed", func(t *testing.T) {
		t.Setenv(mcpManagementEnabledEnv, "sometimes")

		_, err := mcpManagementEnabledFromEnv(false)

		require.Error(t, err)
		require.Contains(t, err.Error(), mcpManagementEnabledEnv)
	})
}
