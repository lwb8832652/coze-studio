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

package sandbox

import (
	"context"
	"reflect"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func setTask9CapabilityEnvironment(t *testing.T, available bool) {
	t.Helper()
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "debug")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	if !available {
		t.Setenv("APP_ENV", "production")
	}
}

func task9LocalProvider(id int64, version uint64) *domainsandbox.Provider {
	provider := testProvider(id, version)
	provider.Type = domainsandbox.ProviderTypeLocalDebug
	provider.EndpointSecret = ""
	provider.EndpointHint = ""
	provider.CredentialSecret = ""
	provider.CredentialFingerprint = ""
	return provider
}

func TestTask9QualityLocalDebugCapabilityGatesCreateEnableAndHealth(t *testing.T) {
	t.Run("create fails closed", func(t *testing.T) {
		setTask9CapabilityEnvironment(t, false)
		h := newControlPlaneHarness(t)
		_, err := h.service.Create(context.Background(), testActor(), CreateProviderRequest{
			Name: "Local", Type: domainsandbox.ProviderTypeLocalDebug,
			Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent}, Policy: testRuntimePolicy(),
		})
		if domainsandbox.ErrorCodeOf(err) != domainsandbox.ErrCodeLocalDebugUnavailable {
			t.Fatalf("Create(local_debug) error = %v, code = %q", err, domainsandbox.ErrorCodeOf(err))
		}
		if h.uow.calls != 0 {
			t.Fatalf("Create(local_debug) reached transaction: %d", h.uow.calls)
		}
	})

	t.Run("create passes exact gate", func(t *testing.T) {
		setTask9CapabilityEnvironment(t, true)
		h := newControlPlaneHarness(t)
		if _, err := h.service.Create(context.Background(), testActor(), CreateProviderRequest{
			Name: "Local", Type: domainsandbox.ProviderTypeLocalDebug,
			Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent}, Policy: testRuntimePolicy(),
		}); err != nil {
			t.Fatalf("Create(local_debug) error = %v", err)
		}
	})

	t.Run("enable fails closed but disable remains available", func(t *testing.T) {
		setTask9CapabilityEnvironment(t, false)
		h := newControlPlaneHarness(t)
		provider := task9LocalProvider(901, 1)
		h.providers.providers[provider.ID] = provider
		_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
		})
		if domainsandbox.ErrorCodeOf(err) != domainsandbox.ErrCodeLocalDebugUnavailable {
			t.Fatalf("SetStatus(enable local_debug) error = %v, code = %q", err, domainsandbox.ErrorCodeOf(err))
		}
		if h.uow.calls != 0 {
			t.Fatalf("enable local_debug reached transaction: %d", h.uow.calls)
		}

		enabled := task9LocalProvider(902, 1)
		enabled.Status = domainsandbox.ProviderStatusEnabled
		h.providers.providers[enabled.ID] = enabled
		if _, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: enabled.ID, ExpectedVersion: enabled.Version, Status: domainsandbox.ProviderStatusDisabled,
		}); err != nil {
			t.Fatalf("SetStatus(disable existing local_debug) error = %v", err)
		}
	})

	t.Run("enable passes exact gate", func(t *testing.T) {
		setTask9CapabilityEnvironment(t, true)
		h := newControlPlaneHarness(t)
		provider := task9LocalProvider(903, 1)
		h.providers.providers[provider.ID] = provider
		if _, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
		}); err != nil {
			t.Fatalf("SetStatus(enable local_debug) error = %v", err)
		}
	})

	t.Run("health fails closed", func(t *testing.T) {
		setTask9CapabilityEnvironment(t, false)
		h := newControlPlaneHarness(t)
		provider := task9LocalProvider(904, 1)
		h.providers.providers[provider.ID] = provider
		_, err := h.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version,
		})
		if domainsandbox.ErrorCodeOf(err) != domainsandbox.ErrCodeLocalDebugUnavailable {
			t.Fatalf("HealthCheck(local_debug) error = %v, code = %q", err, domainsandbox.ErrorCodeOf(err))
		}
		if h.factory.calls != 0 {
			t.Fatalf("HealthCheck(local_debug) reached factory: %d", h.factory.calls)
		}
	})

	t.Run("health passes exact gate", func(t *testing.T) {
		setTask9CapabilityEnvironment(t, true)
		h := newControlPlaneHarness(t)
		provider := task9LocalProvider(905, 1)
		h.providers.providers[provider.ID] = provider
		h.factory.provider = &testHealthProvider{result: infrasandbox.HealthResult{
			ProtocolVersion: infrasandbox.HealthProtocolV1,
			Status:          domainsandbox.HealthStatusHealthy,
			Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAgent},
		}}
		if _, err := h.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version,
		}); err != nil {
			t.Fatalf("HealthCheck(local_debug) error = %v", err)
		}
	})
}

func TestTask9QualityUpdatePreservesLegacyPolicyAndIgnoresNilEmptyDifferences(t *testing.T) {
	t.Run("default legacy provider name update", func(t *testing.T) {
		h := newControlPlaneHarness(t)
		provider := testProvider(920, 4)
		provider.Status = domainsandbox.ProviderStatusEnabled
		provider.Health = task9Healthy()
		provider.Policy.AllowEnv = []string{"PATH"}
		provider.Policy.AllowRead = []string{"workspace"}
		provider.Policy.AllowWrite = []string{"outputs"}
		provider.Policy.AllowRun = []string{"node"}
		provider.Policy.AllowFFI = []string{"ffi-safe"}
		provider.Policy.NodeModulesDir = "node-modules-v1"
		h.providers.providers[provider.ID] = provider
		h.defaults.defaults[domainsandbox.ScopeAgent] = &domainsandbox.ProviderDefault{
			Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID, Version: 1,
		}
		beforeHealth := provider.Health
		publicPolicy := provider.Policy
		publicPolicy.AllowEnv = nil
		publicPolicy.AllowRead = nil
		publicPolicy.AllowWrite = nil
		publicPolicy.AllowRun = nil
		publicPolicy.AllowFFI = nil
		publicPolicy.NodeModulesDir = ""

		updated, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Name: "Renamed",
			Scopes: append([]domainsandbox.Scope(nil), provider.Scopes...), Policy: publicPolicy,
		})
		if err != nil {
			t.Fatalf("Update(default legacy name) error = %v", err)
		}
		stored := h.providers.providers[provider.ID]
		if updated.Name != "Renamed" || !reflect.DeepEqual(stored.Health, beforeHealth) {
			t.Fatalf("name update reset health: updated=%#v health=%#v", updated, stored.Health)
		}
		if !reflect.DeepEqual(stored.Policy.AllowEnv, []string{"PATH"}) ||
			!reflect.DeepEqual(stored.Policy.AllowRead, []string{"workspace"}) ||
			!reflect.DeepEqual(stored.Policy.AllowWrite, []string{"outputs"}) ||
			!reflect.DeepEqual(stored.Policy.AllowRun, []string{"node"}) ||
			!reflect.DeepEqual(stored.Policy.AllowFFI, []string{"ffi-safe"}) ||
			stored.Policy.NodeModulesDir != "node-modules-v1" {
			t.Fatalf("legacy policy was not preserved: %#v", stored.Policy)
		}
	})

	t.Run("nil and empty public lists are equivalent", func(t *testing.T) {
		h := newControlPlaneHarness(t)
		provider := testProvider(921, 2)
		provider.Status = domainsandbox.ProviderStatusEnabled
		provider.Health = task9Healthy()
		h.providers.providers[provider.ID] = provider
		h.defaults.defaults[domainsandbox.ScopeAgent] = &domainsandbox.ProviderDefault{
			Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID, Version: 1,
		}
		publicPolicy := provider.Policy
		publicPolicy.NetworkAllowlist = []string{}
		publicPolicy.AllowedEnvNames = []string{}
		publicPolicy.VirtualReadPrefixes = []string{}
		publicPolicy.VirtualWritePrefixes = []string{}
		publicPolicy.AllowedExecutables = []string{}
		_, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Name: "Renamed",
			Scopes: provider.Scopes, Policy: publicPolicy,
		})
		if err != nil {
			t.Fatalf("Update(nil-empty default name) error = %v", err)
		}
		if h.providers.providers[provider.ID].Health.Status != domainsandbox.HealthStatusHealthy {
			t.Fatal("nil-empty policy difference reset health")
		}
	})
}

func task9Healthy() domainsandbox.HealthSnapshot {
	return domainsandbox.HealthSnapshot{
		Status:       domainsandbox.HealthStatusHealthy,
		Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		ReasonCode:   "HEALTHY",
		CheckedAt:    time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
	}
}
