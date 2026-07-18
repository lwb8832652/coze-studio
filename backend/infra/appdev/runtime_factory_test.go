/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package appdev

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type configuredRuntimeDefaultLookup struct{}

func (configuredRuntimeDefaultLookup) GetProviderDefault(
	context.Context,
	domainsandbox.Scope,
) (*domainsandbox.ProviderDefault, error) {
	return &domainsandbox.ProviderDefault{Scope: domainsandbox.ScopeAppDev, ProviderID: 1, Version: 1}, nil
}

type configuredRuntimeProviderLookup struct{}

func (configuredRuntimeProviderLookup) GetProvider(context.Context, int64) (*domainsandbox.Provider, error) {
	return &domainsandbox.Provider{ID: 1, ProviderKey: appDevTestProviderKey}, nil
}

func TestConfiguredRuntimeManagerFailsClosedInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	t.Setenv("APP_DEV_RUNNER_ENDPOINT", "https://legacy-runner.example.com")
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")

	manager := NewConfiguredRuntimeManager()
	_, err := manager.Start(context.Background(), &appdevapp.RuntimeManagerRequest{
		SpaceID:   "space",
		ProjectID: "project",
	})

	require.ErrorContains(t, err, "isolated appdev runner is not configured")
}

func TestConfiguredRuntimeManagerNeverReturnsHostOrLegacyRemoteManager(t *testing.T) {
	t.Setenv("APP_ENV", "debug")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	t.Setenv("APP_DEV_RUNNER_ENDPOINT", "http://127.0.0.1:9999")
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")

	manager := NewConfiguredRuntimeManager()
	require.IsType(t, &disabledRuntimeManager{}, manager)
}

func TestConfiguredRuntimeManagerRequiresExactControlPlaneFlagAndDependencies(t *testing.T) {
	options := ConfiguredRuntimeManagerOptions{
		Router:    &appsandbox.ProviderRouter{},
		Defaults:  configuredRuntimeDefaultLookup{},
		Providers: configuredRuntimeProviderLookup{},
	}
	t.Setenv("APP_DEV_PREVIEW_GATEWAY_BASE_URL", "https://preview.example.com/apps")
	t.Setenv("APP_DEV_RUNNER_ENDPOINT", "https://legacy-runner.example.com")
	t.Setenv("APP_DEV_RUNNER_TOKEN", "legacy-token")

	for _, value := range []string{"", "false", "TRUE", " true ", "1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", value)
			manager := NewConfiguredRuntimeManager(options)
			require.IsType(t, &disabledRuntimeManager{}, manager)
		})
	}

	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	manager := NewConfiguredRuntimeManager(options)
	require.IsType(t, &SandboxRuntimeManager{}, manager)

	missingDependencies := NewConfiguredRuntimeManager()
	require.IsType(t, &disabledRuntimeManager{}, missingDependencies)
}

func TestValidateRemotePreviewURL(t *testing.T) {
	require.NoError(t, validateRemotePreviewURL(
		"https://preview.example.com",
		"https://preview.example.com/apps/runtime-1/",
	))
	require.Error(t, validateRemotePreviewURL(
		"https://preview.example.com",
		"http://127.0.0.1:3000/",
	))
	require.Error(t, validateRemotePreviewURL(
		"https://preview.example.com",
		"https://attacker.example.net/apps/runtime-1/",
	))
}

func TestRemoteRuntimeManagerRequiresProductionTLSAndToken(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	_, err := newRemoteRuntimeManager(
		"http://runner.internal",
		"https://preview.example.com",
		"runner-token",
	)
	require.ErrorContains(t, err, "must use HTTPS")

	_, err = newRemoteRuntimeManager(
		"https://runner.internal",
		"https://preview.example.com",
		"",
	)
	require.ErrorContains(t, err, "token is required")
}
