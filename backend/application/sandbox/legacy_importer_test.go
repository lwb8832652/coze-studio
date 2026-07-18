// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestLegacyImporterNoConfigAndExistingProviderAreNoOps(t *testing.T) {
	t.Run("missing legacy config", func(t *testing.T) {
		store := newFakeLegacyImportStore()
		importer, err := NewLegacyImporter(func(context.Context) (*adminconfig.SandboxConfig, error) {
			return nil, nil
		}, store)
		if err != nil {
			t.Fatalf("NewLegacyImporter() error = %v", err)
		}
		if err := importer.Import(context.Background()); err != nil {
			t.Fatalf("Import() error = %v", err)
		}
		if len(store.providers) != 0 || len(store.audits) != 0 {
			t.Fatalf("missing config imported provider/audit = %d/%d", len(store.providers), len(store.audits))
		}
	})

	t.Run("any new provider prevents reading legacy source", func(t *testing.T) {
		store := newFakeLegacyImportStore()
		store.providers = append(store.providers, &domainsandbox.Provider{ID: 9, ProviderKey: "remote", Version: 1})
		loadCalls := 0
		importer, err := NewLegacyImporter(func(context.Context) (*adminconfig.SandboxConfig, error) {
			loadCalls++
			return nil, errors.New("legacy source must not be read")
		}, store)
		if err != nil {
			t.Fatalf("NewLegacyImporter() error = %v", err)
		}
		if err := importer.Import(context.Background()); err != nil {
			t.Fatalf("Import() error = %v", err)
		}
		if loadCalls != 0 || len(store.providers) != 1 {
			t.Fatalf("existing table load/providers = %d/%d", loadCalls, len(store.providers))
		}
	})
}

func TestLegacyImporterMapsCanonicalPolicyAndHashesOnlyCanonicalFields(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	t.Setenv("LEGACY_SECRET", "first-secret-value")
	legacy := validLegacySandboxConfig()
	store := newFakeLegacyImportStore()
	importer, err := NewLegacyImporter(staticLegacyLoader(legacy), store)
	if err != nil {
		t.Fatalf("NewLegacyImporter() error = %v", err)
	}
	if err := importer.Import(context.Background()); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(store.providers) != 1 {
		t.Fatalf("provider count = %d", len(store.providers))
	}
	provider := store.providers[0]
	if provider.Name != LegacyProviderName || provider.Type != domainsandbox.ProviderTypeLocalDebug {
		t.Fatalf("provider identity = %q/%q", provider.Name, provider.Type)
	}
	if provider.Status != domainsandbox.ProviderStatusDisabled || store.defaultCalls != 0 {
		t.Fatalf("production status/default calls = %q/%d", provider.Status, store.defaultCalls)
	}
	if len(provider.LegacySourceHash) != 64 || strings.Trim(provider.LegacySourceHash, "0123456789abcdef") != "" {
		t.Fatalf("legacy source hash is not lowercase SHA-256: %q", provider.LegacySourceHash)
	}
	wantLists := map[string][]string{
		"env":   {"HOME", "LEGACY_SECRET", "PATH"},
		"read":  {"/tmp", "/workspace"},
		"write": {"/workspace/output"},
		"run":   {"node", "python3"},
		"ffi":   {"ctypes"},
	}
	gotLists := map[string][]string{
		"env": provider.Policy.AllowEnv, "read": provider.Policy.AllowRead,
		"write": provider.Policy.AllowWrite, "run": provider.Policy.AllowRun, "ffi": provider.Policy.AllowFFI,
	}
	for key, want := range wantLists {
		got, encodedWant := strings.Join(gotLists[key], ","), strings.Join(want, ",")
		if got != encodedWant {
			t.Errorf("canonical %s = %q, want %q", key, got, encodedWant)
		}
	}
	if !provider.Policy.AllowNetwork || strings.Join(provider.Policy.NetworkAllowlist, ",") != "api.example.test,registry.example.test" {
		t.Fatalf("network policy = %t/%v", provider.Policy.AllowNetwork, provider.Policy.NetworkAllowlist)
	}
	if provider.Policy.NodeModulesDir != "/opt/coze/node_modules" || provider.Policy.TimeoutSeconds != 60 || provider.Policy.MemoryLimitMB != 512 {
		t.Fatalf("scalar policy = %#v", provider.Policy)
	}
	auditJSON, err := json.Marshal(store.audits)
	if err != nil {
		t.Fatalf("marshal audits: %v", err)
	}
	for _, prohibited := range []string{"first-secret-value", "/opt/coze/node_modules", "/workspace/output", "registry.example.test"} {
		if strings.Contains(string(auditJSON), prohibited) {
			t.Fatalf("audit leaked %q: %s", prohibited, auditJSON)
		}
	}

	secondStore := newFakeLegacyImportStore()
	t.Setenv("LEGACY_SECRET", "different-secret-value")
	secondImporter, err := NewLegacyImporter(staticLegacyLoader(legacy), secondStore)
	if err != nil {
		t.Fatalf("NewLegacyImporter(second) error = %v", err)
	}
	if err := secondImporter.Import(context.Background()); err != nil {
		t.Fatalf("Import(second) error = %v", err)
	}
	if secondStore.providers[0].LegacySourceHash != provider.LegacySourceHash {
		t.Fatalf("source hash depended on environment value: %q != %q", secondStore.providers[0].LegacySourceHash, provider.LegacySourceHash)
	}
}

func TestLegacySourceHashExcludesEnvironmentAndPathMaterial(t *testing.T) {
	base := validLegacySandboxConfig()
	_, baseHash, err := canonicalLegacyRuntimePolicy(base)
	if err != nil {
		t.Fatalf("canonicalLegacyRuntimePolicy(base) error = %v", err)
	}
	variations := []struct {
		name   string
		mutate func(*adminconfig.SandboxConfig)
	}{
		{name: "environment names or resolved values", mutate: func(value *adminconfig.SandboxConfig) {
			value.AllowEnv = "RESOLVED_SECRET_VALUE,OTHER_ENV"
		}},
		{name: "read paths", mutate: func(value *adminconfig.SandboxConfig) {
			value.AllowRead = "/etc,/private/host/workdir"
		}},
		{name: "write paths", mutate: func(value *adminconfig.SandboxConfig) {
			value.AllowWrite = "/var/run/private-output"
		}},
		{name: "node modules host path", mutate: func(value *adminconfig.SandboxConfig) {
			value.NodeModulesDir = "/srv/private/node_modules"
		}},
	}
	for _, variation := range variations {
		t.Run(variation.name, func(t *testing.T) {
			changed := *base
			variation.mutate(&changed)
			_, gotHash, err := canonicalLegacyRuntimePolicy(&changed)
			if err != nil {
				t.Fatalf("canonicalLegacyRuntimePolicy() error = %v", err)
			}
			if gotHash != baseHash {
				t.Fatalf("path/env material changed hash: %q != %q", gotHash, baseHash)
			}
		})
	}
	if len(baseHash) != 64 || strings.Trim(baseHash, "0123456789abcdef") != "" {
		t.Fatalf("source hash is not lowercase SHA-256: %q", baseHash)
	}
}

func TestLegacySourceHashChangesForCanonicalResourceSemantics(t *testing.T) {
	policy, _, err := canonicalLegacyRuntimePolicy(validLegacySandboxConfig())
	if err != nil {
		t.Fatalf("canonicalLegacyRuntimePolicy() error = %v", err)
	}
	baseHash, err := legacySourceHash(policy)
	if err != nil {
		t.Fatalf("legacySourceHash(base) error = %v", err)
	}
	variations := []struct {
		name   string
		mutate func(*domainsandbox.RuntimePolicy)
	}{
		{name: "cpu", mutate: func(value *domainsandbox.RuntimePolicy) { value.CPULimit = 2 }},
		{name: "memory", mutate: func(value *domainsandbox.RuntimePolicy) { value.MemoryLimitMB = 1024 }},
		{name: "timeout", mutate: func(value *domainsandbox.RuntimePolicy) { value.TimeoutSeconds = 120 }},
		{name: "output", mutate: func(value *domainsandbox.RuntimePolicy) { value.MaxOutputBytes *= 2 }},
		{name: "concurrency", mutate: func(value *domainsandbox.RuntimePolicy) { value.MaxConcurrency = 2 }},
		{name: "network", mutate: func(value *domainsandbox.RuntimePolicy) {
			value.NetworkAllowlist = []string{"different.example.test"}
			value.AllowNetwork = true
		}},
	}
	for _, variation := range variations {
		t.Run(variation.name, func(t *testing.T) {
			changed := policy
			changed.NetworkAllowlist = append([]string(nil), policy.NetworkAllowlist...)
			variation.mutate(&changed)
			gotHash, err := legacySourceHash(changed)
			if err != nil {
				t.Fatalf("legacySourceHash() error = %v", err)
			}
			if gotHash == baseHash {
				t.Fatalf("semantic change did not change hash: %q", gotHash)
			}
		})
	}
}

func TestLegacyImporterIsIdempotentAcrossRetryAndConcurrentStartup(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	store := newFakeLegacyImportStore()
	importer, err := NewLegacyImporter(staticLegacyLoader(validLegacySandboxConfig()), store)
	if err != nil {
		t.Fatalf("NewLegacyImporter() error = %v", err)
	}

	const workers = 2
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByWorker <- importer.Import(testCtx)
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Errorf("concurrent Import() error = %v", err)
		}
	}
	if err := importer.Import(context.Background()); err != nil {
		t.Fatalf("retry Import() error = %v", err)
	}
	if len(store.providers) != 1 || len(store.audits) != 1 {
		t.Fatalf("idempotent providers/audits = %d/%d", len(store.providers), len(store.audits))
	}
	different := validLegacySandboxConfig()
	different.TimeoutSeconds = 120
	differentStore := newFakeLegacyImportStore()
	differentImporter, err := NewLegacyImporter(staticLegacyLoader(different), differentStore)
	if err != nil {
		t.Fatalf("NewLegacyImporter(different) error = %v", err)
	}
	if err := differentImporter.Import(context.Background()); err != nil {
		t.Fatalf("Import(different) error = %v", err)
	}
	if differentStore.providers[0].LegacySourceHash == store.providers[0].LegacySourceHash {
		t.Fatal("different canonical legacy policies produced the same hash")
	}
}

func TestLegacyImporterFeatureDisabledDoesNotReadOrWrite(t *testing.T) {
	for _, value := range []string{"", "false", "TRUE", "1"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", value)
			store := newFakeLegacyImportStore()
			loaderCalls := 0
			err := ImportLegacySandboxConfigIfEnabled(context.Background(), func(context.Context) (*adminconfig.SandboxConfig, error) {
				loaderCalls++
				return &adminconfig.SandboxConfig{AllowNet: "invalid-but-must-not-be-read", TimeoutSeconds: -1}, nil
			}, store)
			if err != nil {
				t.Fatalf("disabled import error = %v", err)
			}
			if loaderCalls != 0 || store.transactionCalls != 0 || len(store.providers) != 0 || len(store.audits) != 0 {
				t.Fatalf("disabled import loader/tx/providers/audits = %d/%d/%d/%d", loaderCalls, store.transactionCalls, len(store.providers), len(store.audits))
			}
		})
	}
}

func TestLegacyImporterFeatureEnabledFailsClosedOnInvalidSource(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	store := newFakeLegacyImportStore()
	loaderCalls := 0
	err := ImportLegacySandboxConfigIfEnabled(context.Background(), func(context.Context) (*adminconfig.SandboxConfig, error) {
		loaderCalls++
		return &adminconfig.SandboxConfig{AllowNet: "invalid", TimeoutSeconds: -1}, nil
	}, store)
	if err == nil {
		t.Fatal("enabled invalid import error = nil")
	}
	if loaderCalls != 1 || store.transactionCalls != 1 || len(store.providers) != 0 {
		t.Fatalf("enabled invalid loader/tx/providers = %d/%d/%d", loaderCalls, store.transactionCalls, len(store.providers))
	}
}

func TestLegacyNetworkListNormalizesBeforeAllowNetwork(t *testing.T) {
	tests := []struct {
		name      string
		allowNet  string
		wantItems string
		wantAllow bool
	}{
		{name: "trailing repeated comma", allowNet: " api.example.test,,api.example.test, ", wantItems: "api.example.test", wantAllow: true},
		{name: "all empty", allowNet: " , , ", wantItems: "", wantAllow: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := validLegacySandboxConfig()
			legacy.AllowNet = test.allowNet
			policy, _, err := canonicalLegacyRuntimePolicy(legacy)
			if err != nil {
				t.Fatalf("canonicalLegacyRuntimePolicy() error = %v", err)
			}
			if got := strings.Join(policy.NetworkAllowlist, ","); got != test.wantItems {
				t.Fatalf("network allowlist = %q, want %q", got, test.wantItems)
			}
			if policy.AllowNetwork != test.wantAllow {
				t.Fatalf("allow network = %t, want %t", policy.AllowNetwork, test.wantAllow)
			}
		})
	}

	legacy := validLegacySandboxConfig()
	legacy.AllowNet = strings.Repeat("x", domainsandbox.MaxLegacyPolicyItemBytes+1)
	if _, _, err := canonicalLegacyRuntimePolicy(legacy); !errors.Is(err, ErrInvalidLegacySandboxConfig) {
		t.Fatalf("invalid non-empty network entry error = %v", err)
	}
}

func TestLegacyImporterDatabaseGatePreventsCrossInstanceEmptyTableRace(t *testing.T) {
	store := newFakeLegacyImportStore()
	store.providerCreateAttempts = make(chan struct{}, 2)
	normalPrepared := make(chan struct{})
	allowNormalCommit := make(chan struct{})
	normalDone := make(chan error, 1)
	go func() {
		normalDone <- store.WithinProviderCreateTransaction(context.Background(), func(
			ctx context.Context,
			repositories LegacyImportRepositories,
		) error {
			_, err := repositories.Providers.CreateProvider(ctx, domainsandbox.CreateProviderInput{
				ProviderKey: "different-instance-provider", Name: "Normal provider",
				Type: domainsandbox.ProviderTypeLocalDebug, Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent},
				Policy: validLegacyRuntimePolicy(), ActorUserID: 42,
			})
			close(normalPrepared)
			<-allowNormalCommit
			return err
		})
	}()
	<-store.providerCreateAttempts
	<-normalPrepared

	loaderCalls := 0
	importer, err := NewLegacyImporter(func(context.Context) (*adminconfig.SandboxConfig, error) {
		loaderCalls++
		return validLegacySandboxConfig(), nil
	}, store)
	if err != nil {
		t.Fatalf("NewLegacyImporter() error = %v", err)
	}
	importDone := make(chan error, 1)
	go func() { importDone <- importer.Import(context.Background()) }()
	importFinishedBeforeNormalCommit := false
	select {
	case <-store.providerCreateAttempts:
		// The fixed importer is waiting on the normal create's shared gate.
	case err = <-importDone:
		// The old importer bypasses the shared gate and commits from a stale
		// empty-table observation before the normal transaction commits.
		importFinishedBeforeNormalCommit = true
		if err != nil {
			t.Fatalf("Import() error = %v", err)
		}
	}
	close(allowNormalCommit)
	if err = <-normalDone; err != nil {
		t.Fatalf("normal create error = %v", err)
	}
	if !importFinishedBeforeNormalCommit {
		if err = <-importDone; err != nil {
			t.Fatalf("Import() error = %v", err)
		}
	}
	if len(store.providers) != 1 || store.providers[0].ProviderKey != "different-instance-provider" || len(store.audits) != 0 {
		t.Fatalf("provider create/import race committed providers=%v audits=%d", providerKeys(store.providers), len(store.audits))
	}
	if loaderCalls != 0 {
		t.Fatalf("legacy loader calls = %d, want 0 after normal create commits", loaderCalls)
	}
}

func TestLegacyImporterUsesProviderCreateTransaction(t *testing.T) {
	store := newFakeLegacyImportStore()
	importer, err := NewLegacyImporter(staticLegacyLoader(validLegacySandboxConfig()), store)
	if err != nil {
		t.Fatalf("NewLegacyImporter() error = %v", err)
	}
	if err = importer.Import(context.Background()); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if store.providerCreateTransactionCalls != 1 {
		t.Fatalf("provider create transaction calls = %d, want 1", store.providerCreateTransactionCalls)
	}
}

func TestLegacyImporterLocalDebugRequiresExactDualGateAndNeverDefaults(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		enabled string
		want    domainsandbox.ProviderStatus
	}{
		{name: "debug dual gate", appEnv: "debug", enabled: "true", want: domainsandbox.ProviderStatusEnabled},
		{name: "production", appEnv: "production", enabled: "true", want: domainsandbox.ProviderStatusDisabled},
		{name: "missing feature gate", appEnv: "debug", enabled: "false", want: domainsandbox.ProviderStatusDisabled},
		{name: "non canonical values", appEnv: "DEBUG", enabled: "TRUE", want: domainsandbox.ProviderStatusDisabled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("APP_ENV", test.appEnv)
			t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", test.enabled)
			store := newFakeLegacyImportStore()
			importer, err := NewLegacyImporter(staticLegacyLoader(validLegacySandboxConfig()), store)
			if err != nil {
				t.Fatalf("NewLegacyImporter() error = %v", err)
			}
			if err := importer.Import(context.Background()); err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if got := store.providers[0].Status; got != test.want {
				t.Fatalf("provider status = %q, want %q", got, test.want)
			}
			if store.defaultCalls != 0 {
				t.Fatalf("default calls = %d", store.defaultCalls)
			}
		})
	}
}

func TestLegacyImporterInvalidOrUnavailableSourceFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		loader LegacySandboxConfigLoader
	}{
		{name: "source error", loader: func(context.Context) (*adminconfig.SandboxConfig, error) {
			return nil, errors.New("database password=must-not-leak")
		}},
		{name: "invalid bounds", loader: staticLegacyLoader(&adminconfig.SandboxConfig{
			AllowEnv: "SECRET_VALUE_MUST_NOT_LEAK", TimeoutSeconds: 0, MemoryLimitMb: 512,
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeLegacyImportStore()
			importer, err := NewLegacyImporter(test.loader, store)
			if err != nil {
				t.Fatalf("NewLegacyImporter() error = %v", err)
			}
			err = importer.Import(context.Background())
			if err == nil {
				t.Fatal("Import() error = nil")
			}
			for _, prohibited := range []string{"password", "must-not-leak", "SECRET_VALUE_MUST_NOT_LEAK"} {
				if strings.Contains(err.Error(), prohibited) {
					t.Fatalf("safe error leaked %q: %v", prohibited, err)
				}
			}
			if len(store.providers) != 0 || len(store.audits) != 0 {
				t.Fatalf("failed import persisted providers/audits = %d/%d", len(store.providers), len(store.audits))
			}
		})
	}
}

func validLegacySandboxConfig() *adminconfig.SandboxConfig {
	return &adminconfig.SandboxConfig{
		AllowEnv: " PATH,LEGACY_SECRET,HOME,PATH ", AllowRead: " /workspace,/tmp,/workspace ",
		AllowWrite: "/workspace/output", AllowRun: "python3,node,python3",
		AllowNet: "registry.example.test,api.example.test", AllowFfi: "ctypes",
		NodeModulesDir: "/opt/coze/node_modules", TimeoutSeconds: 60, MemoryLimitMb: 512,
	}
}

func staticLegacyLoader(value *adminconfig.SandboxConfig) LegacySandboxConfigLoader {
	return func(context.Context) (*adminconfig.SandboxConfig, error) {
		copy := *value
		return &copy, nil
	}
}

type fakeLegacyImportStore struct {
	mu                             sync.Mutex
	providers                      []*domainsandbox.Provider
	audits                         []*domainsandbox.ProviderAuditEvent
	defaultCalls                   int
	transactionCalls               int
	providerCreateGate             chan struct{}
	providerCreateAttempts         chan struct{}
	providerCreateTransactionCalls int
}

func newFakeLegacyImportStore() *fakeLegacyImportStore {
	providerCreateGate := make(chan struct{}, 1)
	providerCreateGate <- struct{}{}
	return &fakeLegacyImportStore{providerCreateGate: providerCreateGate}
}

func (s *fakeLegacyImportStore) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, LegacyImportRepositories) error,
) error {
	if s.providerCreateAttempts != nil {
		s.providerCreateAttempts <- struct{}{}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.providerCreateGate:
	}
	defer func() { s.providerCreateGate <- struct{}{} }()

	s.mu.Lock()
	s.transactionCalls++
	s.providerCreateTransactionCalls++
	baselineProviderKeys := make(map[string]struct{}, len(s.providers))
	transaction := newFakeLegacyImportStore()
	transaction.providers = make([]*domainsandbox.Provider, 0, len(s.providers))
	for _, provider := range s.providers {
		baselineProviderKeys[provider.ProviderKey] = struct{}{}
		copy := *provider
		transaction.providers = append(transaction.providers, &copy)
	}
	baselineAuditCount := len(s.audits)
	transaction.audits = append([]*domainsandbox.ProviderAuditEvent(nil), s.audits...)
	s.mu.Unlock()

	err := callback(ctx, LegacyImportRepositories{Providers: transaction, Defaults: transaction, Audits: transaction})
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, provider := range transaction.providers {
		if _, existed := baselineProviderKeys[provider.ProviderKey]; existed || containsProviderKey(s.providers, provider.ProviderKey) {
			continue
		}
		copy := *provider
		copy.ID = int64(len(s.providers) + 1)
		s.providers = append(s.providers, &copy)
	}
	for _, event := range transaction.audits[baselineAuditCount:] {
		copy := *event
		copy.ID = int64(len(s.audits) + 1)
		s.audits = append(s.audits, &copy)
	}
	return nil
}

func (s *fakeLegacyImportStore) ListProviders(
	ctx context.Context,
	_ domainsandbox.ProviderListRequest,
) ([]*domainsandbox.Provider, int64, error) {
	s.mu.Lock()
	items := append([]*domainsandbox.Provider(nil), s.providers...)
	total := int64(len(items))
	s.mu.Unlock()
	return items, total, nil
}

func (s *fakeLegacyImportStore) CreateProvider(
	_ context.Context,
	input domainsandbox.CreateProviderInput,
) (*domainsandbox.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, provider := range s.providers {
		if provider.ProviderKey == input.ProviderKey || provider.LegacySourceHash == input.LegacySourceHash {
			return nil, domainsandbox.ErrProviderAlreadyExists
		}
	}
	provider := &domainsandbox.Provider{
		ID: int64(len(s.providers) + 1), ProviderKey: input.ProviderKey, Name: input.Name, Type: input.Type,
		Scopes: append([]domainsandbox.Scope(nil), input.Scopes...), Policy: input.Policy,
		Status: domainsandbox.ProviderStatusDisabled, Health: domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusUnknown},
		LegacySourceHash: input.LegacySourceHash, Version: domainsandbox.InitialVersion,
		CreatedBy: input.ActorUserID, UpdatedBy: input.ActorUserID,
	}
	s.providers = append(s.providers, provider)
	return provider, nil
}

func (s *fakeLegacyImportStore) UpdateProviderStatus(
	_ context.Context,
	input domainsandbox.UpdateProviderStatusInput,
) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, provider := range s.providers {
		if provider.ID == input.ProviderID {
			if provider.Version != input.ExpectedVersion {
				return 0, domainsandbox.ErrVersionConflict
			}
			provider.Status = input.Status
			provider.Version++
			return provider.Version, nil
		}
	}
	return 0, domainsandbox.ErrProviderNotFound
}

func (s *fakeLegacyImportStore) SetProviderDefault(
	context.Context,
	domainsandbox.SetProviderDefaultInput,
) (*domainsandbox.ProviderDefault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultCalls++
	return nil, errors.New("legacy importer must not set defaults")
}

func (s *fakeLegacyImportStore) AppendProviderAuditEvent(
	_ context.Context,
	input domainsandbox.AppendProviderAuditEventInput,
) (*domainsandbox.ProviderAuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := &domainsandbox.ProviderAuditEvent{
		ID: int64(len(s.audits) + 1), ProviderID: input.ProviderID, ActorUserID: input.ActorUserID,
		Action: input.Action, Result: input.Result, RequestID: input.RequestID,
		Metadata: input.Metadata,
	}
	s.audits = append(s.audits, event)
	return event, nil
}

func validLegacyRuntimePolicy() domainsandbox.RuntimePolicy {
	policy, _, _ := canonicalLegacyRuntimePolicy(validLegacySandboxConfig())
	return policy
}

func containsProviderKey(providers []*domainsandbox.Provider, providerKey string) bool {
	for _, provider := range providers {
		if provider.ProviderKey == providerKey {
			return true
		}
	}
	return false
}

func providerKeys(providers []*domainsandbox.Provider) []string {
	keys := make([]string, 0, len(providers))
	for _, provider := range providers {
		keys = append(keys, provider.ProviderKey)
	}
	return keys
}
