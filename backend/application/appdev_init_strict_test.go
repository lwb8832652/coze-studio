// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppDevApplicationInitializationPropagatesExplicitEnablementErrors(t *testing.T) {
	t.Run("disabled remains fail closed without failing startup", func(t *testing.T) {
		t.Setenv(sandboxControlPlaneEnabledEnv, "false")
		runtime, err := initAppDevProviderRuntimeForApplication(
			context.Background(),
			appDevProviderWiringDependencies{},
		)
		require.NoError(t, err)
		require.Nil(t, runtime)
	})

	t.Run("enabled incomplete provider wiring is fatal", func(t *testing.T) {
		t.Setenv(sandboxControlPlaneEnabledEnv, "true")
		runtime, err := initAppDevProviderRuntimeForApplication(
			context.Background(),
			appDevProviderWiringDependencies{},
		)
		require.ErrorIs(t, err, errAppDevProviderWiringUnavailable)
		require.Nil(t, runtime)
	})

	t.Run("management enabled with runtime routing disabled stays fail closed", func(t *testing.T) {
		t.Setenv(sandboxControlPlaneEnabledEnv, "true")
		t.Setenv(sandboxRuntimeRoutingEnabledEnv, "false")
		runtime, err := initAppDevProviderRuntimeForApplication(
			context.Background(),
			appDevProviderWiringDependencies{},
		)
		require.NoError(t, err)
		require.Nil(t, runtime)
	})

	t.Run("enabled sandbox initialization error is propagated safely", func(t *testing.T) {
		t.Setenv(sandboxControlPlaneEnabledEnv, "invalid")
		err := initSandboxControlPlaneForApplication(nil)
		require.ErrorIs(t, err, errSandboxControlPlaneInitialization)
		require.NotContains(t, err.Error(), "invalid")
	})
}
