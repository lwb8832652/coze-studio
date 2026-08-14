// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestTask9AServiceProjectsAllDefaultsVersionsAndGlobalSummary(t *testing.T) {
	harness := newControlPlaneHarness(t)
	provider := testProvider(17, 7)
	provider.Name = "Primary"
	harness.providers.providers[provider.ID] = provider
	harness.defaults.defaults[domainsandbox.ScopeAgent] = &domainsandbox.ProviderDefault{
		Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID, Version: 3,
		UpdatedAt: time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC),
	}
	management := &task9AManagementRepository{
		defaults: harness.defaults,
		summary:  domainsandbox.ProviderSummary{Total: 12, Enabled: 8, Unhealthy: 2},
	}
	harness.service.management = management

	defaults, err := harness.service.ListDefaults(context.Background(), testActor())
	if err != nil {
		t.Fatalf("ListDefaults() error = %v", err)
	}
	if len(defaults.Items) != 3 || defaults.Items[0].Scope != domainsandbox.ScopeAgent ||
		defaults.Items[1].Scope != domainsandbox.ScopeMCPStdio || defaults.Items[2].Scope != domainsandbox.ScopeAppDev {
		t.Fatalf("default ordering = %#v", defaults.Items)
	}
	if !defaults.Items[0].Configured || defaults.Items[0].ProviderID == nil || *defaults.Items[0].ProviderID != 17 ||
		defaults.Items[0].Version != 3 || defaults.Items[0].ProviderVersion != 7 || defaults.Items[0].ProviderName != "Primary" {
		t.Fatalf("configured default = %#v", defaults.Items[0])
	}
	if defaults.Items[1].Configured || defaults.Items[1].ProviderID != nil || defaults.Items[1].Version != 0 {
		t.Fatalf("missing default = %#v", defaults.Items[1])
	}

	summary, err := harness.service.GetSummary(context.Background(), testActor())
	if err != nil || *summary != (ProviderSummaryDTO{Total: 12, Enabled: 8, Unhealthy: 2}) {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
	if len(management.authorizedScopes) != 3 {
		t.Fatalf("summary authorized scopes = %#v", management.authorizedScopes)
	}

	denied := testActor()
	denied.SystemAdmin = false
	if _, err := harness.service.ListDefaults(context.Background(), denied); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ListDefaults denied error = %v", err)
	}
	if management.listCalls != 1 || management.summaryCalls != 1 {
		t.Fatalf("denied request reached repository: %#v", management)
	}
}

func TestTask9ACapabilitiesUseExactSafeGates(t *testing.T) {
	cases := []struct {
		name         string
		env          map[string]string
		controlPlane bool
		localDebug   bool
		localReason  string
		hostShell    bool
		hostReason   string
	}{
		{"legacy exact", map[string]string{"SANDBOX_CONTROL_PLANE_ENABLED": "true", "APP_ENV": "debug", "APP_DEV_HOST_RUNTIME_ENABLED": "true"}, true, true, CapabilityReasonAvailable, false, CapabilityReasonHostShellUnavailable},
		{"host exact independent of legacy", map[string]string{"SANDBOX_CONTROL_PLANE_ENABLED": "true", "APP_ENV": "debug", "APP_DEV_HOST_RUNTIME_ENABLED": "false", "SANDBOX_HOST_SHELL_SESSION_ENABLED": "true", "SANDBOX_HOST_SHELL_GATEWAY_ADDR": "127.0.0.1:8099"}, true, false, CapabilityReasonLocalDebugUnavailable, true, CapabilityReasonAvailable},
		{"control plane case mismatch", map[string]string{"SANDBOX_CONTROL_PLANE_ENABLED": "TRUE", "APP_ENV": "debug", "APP_DEV_HOST_RUNTIME_ENABLED": "true", "SANDBOX_HOST_SHELL_SESSION_ENABLED": "true", "SANDBOX_HOST_SHELL_GATEWAY_ADDR": "127.0.0.1:8099"}, false, false, CapabilityReasonControlPlaneDisabled, false, CapabilityReasonControlPlaneDisabled},
		{"app env case mismatch", map[string]string{"SANDBOX_CONTROL_PLANE_ENABLED": "true", "APP_ENV": "DEBUG", "APP_DEV_HOST_RUNTIME_ENABLED": "true", "SANDBOX_HOST_SHELL_SESSION_ENABLED": "true", "SANDBOX_HOST_SHELL_GATEWAY_ADDR": "127.0.0.1:8099"}, true, false, CapabilityReasonLocalDebugUnavailable, false, CapabilityReasonHostShellUnavailable},
		{"host gate case mismatch", map[string]string{"SANDBOX_CONTROL_PLANE_ENABLED": "true", "APP_ENV": "debug", "APP_DEV_HOST_RUNTIME_ENABLED": "true", "SANDBOX_HOST_SHELL_SESSION_ENABLED": "TRUE", "SANDBOX_HOST_SHELL_GATEWAY_ADDR": "localhost:8099"}, true, true, CapabilityReasonAvailable, false, CapabilityReasonHostShellUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			test.env["APP_DEV_RUNNER_TOKEN"] = "secret-token-must-not-leak"
			projection := ProjectCapabilities(func(key string) string { return test.env[key] })
			if projection.ControlPlane.Available != test.controlPlane || projection.LocalDebug.Available != test.localDebug || projection.LocalDebug.ReasonCode != test.localReason ||
				projection.HostShell.Available != test.hostShell || projection.HostShell.ReasonCode != test.hostReason {
				t.Fatalf("projection = %#v", projection)
			}
			encoded, err := json.Marshal(projection)
			if err != nil {
				t.Fatalf("marshal capabilities: %v", err)
			}
			text := string(encoded)
			for _, prohibited := range []string{"secret-token-must-not-leak", "APP_ENV", "APP_DEV_HOST_RUNTIME_ENABLED", "SANDBOX_HOST_SHELL_SESSION_ENABLED", "SANDBOX_HOST_SHELL_GATEWAY_ADDR", "SANDBOX_CONTROL_PLANE_ENABLED", "endpoint", "credential"} {
				if strings.Contains(text, prohibited) {
					t.Fatalf("capability projection leaked %q: %s", prohibited, text)
				}
			}
		})
	}
}

func TestHostShellCapabilityAdmitsLocalProviderWithoutOpeningLegacyOneShot(t *testing.T) {
	environment := map[string]string{
		"SANDBOX_CONTROL_PLANE_ENABLED":      "true",
		"APP_ENV":                            "debug",
		"APP_DEV_HOST_RUNTIME_ENABLED":       "false",
		"SANDBOX_HOST_SHELL_SESSION_ENABLED": "true",
		"SANDBOX_HOST_SHELL_GATEWAY_ADDR":    "[::1]:8099",
	}
	service := &Service{capabilities: NewEnvironmentCapabilityPolicy(func(key string) string {
		return environment[key]
	})}
	if err := service.requireLocalDebugCapability(domainsandbox.ProviderTypeLocalDebug); err != nil {
		t.Fatalf("Host Shell-only local provider admission error = %v", err)
	}
	if ProjectCapabilities(func(key string) string { return environment[key] }).LocalDebug.Available {
		t.Fatal("Host Shell gate unexpectedly opened legacy one-shot local debug")
	}
}

type task9AManagementRepository struct {
	defaults         *testDefaultRepository
	summary          domainsandbox.ProviderSummary
	authorizedScopes []domainsandbox.Scope
	listCalls        int
	summaryCalls     int
}

func (r *task9AManagementRepository) ListProviderDefaults(ctx context.Context) ([]*domainsandbox.ProviderDefault, error) {
	r.listCalls++
	return r.defaults.ListProviderDefaults(ctx)
}

func (r *task9AManagementRepository) SummarizeProviders(_ context.Context, scopes []domainsandbox.Scope) (domainsandbox.ProviderSummary, error) {
	r.summaryCalls++
	r.authorizedScopes = append([]domainsandbox.Scope(nil), scopes...)
	return r.summary, nil
}
