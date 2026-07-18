// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestTask9APolicyJSONRoundTripPreservesCompleteSafeContract(t *testing.T) {
	policy := domainsandbox.RuntimePolicy{
		TimeoutSeconds: 60, MemoryLimitMB: 512, CPULimit: 1, MaxOutputBytes: 65536, MaxConcurrency: 8,
		AllowedEnvNames: []string{"HOME", "PATH"}, VirtualReadPrefixes: []string{"inputs", "workspace/src"},
		VirtualWritePrefixes: []string{"outputs"}, AllowedExecutables: []string{"node", "python3"},
		FFIEnabled: true, NodeModulesMode: domainsandbox.NodeModulesModeApprovedDirectory,
		NodeModulesDirectoryRef: "node-modules-v1",
	}
	normalized, err := domainsandbox.NormalizeRuntimePolicy(policy)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	raw, err := marshalRuntimePolicy(normalized)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"allowed_env_names", "virtual_read_prefixes", "virtual_write_prefixes", "allowed_executables", "ffi_enabled", "node_modules_mode", "node_modules_directory_ref"} {
		if !strings.Contains(raw, `"`+key+`"`) {
			t.Fatalf("persisted policy missing %s: %s", key, raw)
		}
	}
	decoded, err := unmarshalRuntimePolicy(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Legacy-only compatibility slices are intentionally outside the new
	// management contract; normalize nil/empty before comparing canonical data.
	decoded.AllowEnv, decoded.AllowRead, decoded.AllowWrite, decoded.AllowRun, decoded.AllowFFI = nil, nil, nil, nil, nil
	if !reflect.DeepEqual(decoded, normalized) {
		t.Fatalf("round trip = %#v, want %#v", decoded, normalized)
	}
}

func TestTask9ARepositoryHealthFilterAndGlobalSummaryExcludeSoftDeleted(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	create := func(key string, scopes []domainsandbox.Scope) *domainsandbox.Provider {
		input := validCreateProviderInput(key)
		input.Scopes = scopes
		provider, err := repository.CreateProvider(ctx, input)
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		return provider
	}
	healthy := create("task9a-healthy", []domainsandbox.Scope{domainsandbox.ScopeAgent})
	unhealthy := create("task9a-unhealthy", []domainsandbox.Scope{domainsandbox.ScopeAgent})
	degraded := create("task9a-degraded", []domainsandbox.Scope{domainsandbox.ScopeAgent})
	deleted := create("task9a-deleted", []domainsandbox.Scope{domainsandbox.ScopeAgent})
	appdev := create("task9a-appdev", []domainsandbox.Scope{domainsandbox.ScopeAppDev})
	checkedAt := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	setHealth := func(provider *domainsandbox.Provider, status domainsandbox.HealthStatus, enabled bool, deletedAt *time.Time) {
		code := strings.ToUpper(string(status))
		updates := map[string]any{
			"status": string(domainsandbox.ProviderStatusDisabled), "health_status": string(status),
			"last_health_capabilities_json": `["agent"]`, "last_health_code": code,
			"last_health_message": "bounded status", "last_health_at": checkedAt, "deleted_at": deletedAt,
		}
		if provider.ID == appdev.ID {
			updates["last_health_capabilities_json"] = `["appdev"]`
		}
		if enabled {
			updates["status"] = string(domainsandbox.ProviderStatusEnabled)
		}
		if err := db.Model(&providerPO{}).Where("id = ?", provider.ID).Updates(updates).Error; err != nil {
			t.Fatalf("set projection %d: %v", provider.ID, err)
		}
	}
	setHealth(healthy, domainsandbox.HealthStatusHealthy, true, nil)
	setHealth(unhealthy, domainsandbox.HealthStatusUnhealthy, true, nil)
	setHealth(degraded, domainsandbox.HealthStatusDegraded, false, nil)
	deletedAt := checkedAt.Add(time.Minute)
	setHealth(deleted, domainsandbox.HealthStatusUnhealthy, true, &deletedAt)
	setHealth(appdev, domainsandbox.HealthStatusHealthy, true, nil)

	providers, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		Health:           domainsandbox.HealthStatusUnhealthy,
		AuthorizedScopes: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAppDev},
	})
	if err != nil || total != 1 || len(providers) != 1 || providers[0].ID != unhealthy.ID {
		t.Fatalf("health list = %#v total=%d err=%v", providers, total, err)
	}
	summary, err := repository.SummarizeProviders(ctx, []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAppDev})
	if err != nil || summary != (domainsandbox.ProviderSummary{Total: 4, Enabled: 3, Unhealthy: 1}) {
		t.Fatalf("global summary = %#v, %v", summary, err)
	}
	scoped, err := repository.SummarizeProviders(ctx, []domainsandbox.Scope{domainsandbox.ScopeAgent})
	if err != nil || scoped != (domainsandbox.ProviderSummary{Total: 3, Enabled: 2, Unhealthy: 1}) {
		t.Fatalf("scoped summary = %#v, %v", scoped, err)
	}
}
