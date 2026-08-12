// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestControlPlaneRunnerBuildsCanonicalAgentRequest(t *testing.T) {
	harness := newControlPlaneRunnerHarness(t)
	runner := newControlPlaneRunnerForTest(t, harness.source, 4096)

	response, err := runner.Run(context.Background(), &coderunner.RunRequest{
		Language: coderunner.Python,
		Code:     "async def main(args):\n    return {'ok': True}",
		Params:   map[string]any{"query": "safe input"},
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response == nil || response.Result["ok"] != true {
		t.Fatalf("response = %#v", response)
	}
	if harness.router.scope != domainsandbox.ScopeAgent ||
		harness.router.providerKey != "agent-provider" ||
		harness.defaults.scope != domainsandbox.ScopeAgent {
		t.Fatalf("selection = %q %q", harness.router.scope, harness.router.providerKey)
	}
	if len(harness.selection.requests) != 1 {
		t.Fatalf("execute requests = %d", len(harness.selection.requests))
	}
	request := harness.selection.requests[0]
	if request.Scope != domainsandbox.ScopeAgent ||
		request.WorkloadKind != infrasandbox.WorkloadAgent ||
		request.Entrypoint != "agent/code/run" ||
		request.IdempotencyKey != "code_runner_operation_test" {
		t.Fatalf("request identity = %#v", request)
	}
	if len(request.Env) != 0 || len(request.Files) != 0 ||
		len(request.ArtifactReferences) != 0 {
		t.Fatalf("request carried unrelated context: %#v", request)
	}
	envelope, err := infrasandbox.ParseCodeRunnerInvokeEnvelope(
		request.Stdin,
		infrasandbox.MaxCodeRunnerEnvelopeBytes,
	)
	if err != nil {
		t.Fatalf("ParseCodeRunnerInvokeEnvelope() error = %v", err)
	}
	if envelope.Language != string(coderunner.Python) ||
		!strings.Contains(envelope.Code, "return") ||
		!strings.Contains(string(envelope.Params), "safe input") {
		t.Fatalf("envelope = %#v", envelope)
	}
	if harness.selection.releaseCalls != 1 {
		t.Fatalf("release calls = %d", harness.selection.releaseCalls)
	}
}

func TestControlPlaneRunnerBuildsCanonicalPluginRequest(t *testing.T) {
	harness := newControlPlaneRunnerHarness(t)
	configurePluginHarness(harness)
	runner := newControlPlaneRunnerForTest(t, harness.source, 4096)

	response, err := runner.Run(context.Background(), &coderunner.RunRequest{
		Purpose:  coderunner.PurposePlugin,
		Language: coderunner.Python,
		Code:     "async def main(args):\n    return {'ok': True}",
		Params:   map[string]any{"query": "safe input"},
	})

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response == nil || response.Result["ok"] != true {
		t.Fatalf("response = %#v", response)
	}
	if harness.defaults.scope != domainsandbox.ScopePlugin ||
		harness.router.scope != domainsandbox.ScopePlugin ||
		harness.router.providerKey != "plugin-provider" {
		t.Fatalf(
			"selection = default:%q router:%q provider:%q",
			harness.defaults.scope,
			harness.router.scope,
			harness.router.providerKey,
		)
	}
	if len(harness.selection.requests) != 1 {
		t.Fatalf("execute requests = %d", len(harness.selection.requests))
	}
	request := harness.selection.requests[0]
	if request.Scope != domainsandbox.ScopePlugin ||
		request.WorkloadKind != infrasandbox.WorkloadPlugin ||
		request.Entrypoint != "plugin/code/run" {
		t.Fatalf("request identity = %#v", request)
	}
}

func TestControlPlaneRunnerProjectsOnlyServerOwnedSandboxIdentity(t *testing.T) {
	harness := newControlPlaneRunnerHarness(t)
	runner := newControlPlaneRunnerForTest(t, harness.source, 4096)
	ctx := sandboxidentity.WithRequest(context.Background(), sandboxidentity.Request{
		Scope:       sandboxidentity.ScopeAgent,
		SpaceID:     11,
		UserID:      22,
		ExecutionID: "run_33",
	})

	response, err := runner.Run(ctx, &coderunner.RunRequest{
		Language: coderunner.Python,
		Code:     "async def main(args): return {}",
		Params:   map[string]any{},
	})
	if err != nil || response == nil {
		t.Fatalf("Run() = %#v, %v", response, err)
	}
	if got := harness.selection.requests[0].Identity; got != (infrasandbox.ExecutionIdentity{SpaceID: 11, UserID: 22, ExecutionID: "run_33"}) {
		t.Fatalf("request identity = %#v", got)
	}
}

func TestControlPlaneRunnerPluginFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*controlPlaneRunnerHarness)
	}{
		{
			name: "runtime routing disabled",
			configure: func(h *controlPlaneRunnerHarness) {
				h.source.ok = false
			},
		},
		{
			name: "provider missing",
			configure: func(h *controlPlaneRunnerHarness) {
				h.providers.value = nil
			},
		},
		{
			name: "scope unsupported",
			configure: func(h *controlPlaneRunnerHarness) {
				h.router.err = domainsandbox.ErrScopeUnsupported
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newControlPlaneRunnerHarness(t)
			configurePluginHarness(harness)
			test.configure(harness)
			runner := newControlPlaneRunnerForTest(t, harness.source, 4096)

			response, err := runner.Run(context.Background(), &coderunner.RunRequest{
				Purpose:  coderunner.PurposePlugin,
				Language: coderunner.Python,
				Code:     "async def main(args): return {}",
				Params:   map[string]any{},
			})

			if !errors.Is(err, coderunner.ErrCodeRunnerUnavailable) {
				t.Fatalf("Run() error = %v, want %v", err, coderunner.ErrCodeRunnerUnavailable)
			}
			if response != nil {
				t.Fatalf("response = %#v", response)
			}
			if len(harness.selection.requests) != 0 {
				t.Fatalf("fail-closed path executed %d requests", len(harness.selection.requests))
			}
		})
	}
}

func TestControlPlaneRunnerFailsClosedAndMapsSafeErrors(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*controlPlaneRunnerHarness)
		want      error
	}{
		{
			name: "binding unavailable",
			configure: func(h *controlPlaneRunnerHarness) {
				h.source.ok = false
			},
			want: coderunner.ErrCodeRunnerUnavailable,
		},
		{
			name: "capacity exhausted",
			configure: func(h *controlPlaneRunnerHarness) {
				h.router.err = domainsandbox.ErrCapacityExhausted
			},
			want: coderunner.ErrCodeRunnerCapacityExhausted,
		},
		{
			name: "provider timeout",
			configure: func(h *controlPlaneRunnerHarness) {
				h.selection.results = []infrasandbox.ExecuteResult{{
					ExecutionID: "execution_timeout",
					Status:      infrasandbox.ExecutionStatusTimedOut,
				}}
			},
			want: coderunner.ErrCodeRunnerTimeout,
		},
		{
			name: "provider canceled",
			configure: func(h *controlPlaneRunnerHarness) {
				h.selection.results = []infrasandbox.ExecuteResult{{
					ExecutionID: "execution_canceled",
					Status:      infrasandbox.ExecutionStatusCanceled,
				}}
			},
			want: coderunner.ErrCodeRunnerCanceled,
		},
		{
			name: "output limit",
			configure: func(h *controlPlaneRunnerHarness) {
				h.selection.results = []infrasandbox.ExecuteResult{{
					ExecutionID: "execution_large",
					Status:      infrasandbox.ExecutionStatusSucceeded,
					ExitCode:    codeRunnerIntPointer(0),
					Stdout:      strings.Repeat("x", 4097),
				}}
			},
			want: coderunner.ErrCodeRunnerOutputLimit,
		},
		{
			name: "async result canceled",
			configure: func(h *controlPlaneRunnerHarness) {
				h.selection.results = []infrasandbox.ExecuteResult{{
					ExecutionID: "execution_running",
					Status:      infrasandbox.ExecutionStatusRunning,
				}}
			},
			want: coderunner.ErrCodeRunnerUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newControlPlaneRunnerHarness(t)
			test.configure(harness)
			runner := newControlPlaneRunnerForTest(t, harness.source, 4096)

			response, err := runner.Run(context.Background(), &coderunner.RunRequest{
				Language: coderunner.Python,
				Code:     "async def main(args): return {}",
				Params:   map[string]any{},
			})

			if !errors.Is(err, test.want) {
				t.Fatalf("Run() error = %v, want %v", err, test.want)
			}
			if response != nil {
				t.Fatalf("response = %#v", response)
			}
			if err != nil && strings.Contains(err.Error(), "provider-secret") {
				t.Fatalf("safe error leaked provider output: %v", err)
			}
			if len(harness.selection.requests) > 0 &&
				harness.selection.releaseCalls != 1 {
				t.Fatalf("release calls = %d", harness.selection.releaseCalls)
			}
			if test.name == "async result canceled" &&
				harness.selection.cancelCalls != 1 {
				t.Fatalf("cancel calls = %d", harness.selection.cancelCalls)
			}
		})
	}
}

func TestControlPlaneRunnerReconcilesUncertainSubmissionWithSameRequest(t *testing.T) {
	harness := newControlPlaneRunnerHarness(t)
	harness.selection.errs = []error{
		errors.New("provider-secret"),
		nil,
	}
	runner := newControlPlaneRunnerForTest(t, harness.source, 4096)

	response, err := runner.Run(context.Background(), &coderunner.RunRequest{
		Language: coderunner.Python,
		Code:     "async def main(args): return {'ok': True}",
		Params:   map[string]any{},
	})

	if err != nil || response == nil {
		t.Fatalf("Run() = %#v, %v", response, err)
	}
	if len(harness.selection.requests) != 2 {
		t.Fatalf("execute requests = %d", len(harness.selection.requests))
	}
	first, _ := json.Marshal(harness.selection.requests[0])
	second, _ := json.Marshal(harness.selection.requests[1])
	if string(first) != string(second) {
		t.Fatalf("reconcile request changed")
	}
}

func newControlPlaneRunnerForTest(
	t *testing.T,
	source BindingSource,
	maxOutputBytes int,
) coderunner.Runner {
	t.Helper()
	runner, err := NewRunner(Options{
		BindingSource:  source,
		MaxInputBytes:  infrasandbox.MaxCodeRunnerEnvelopeBytes,
		MaxOutputBytes: maxOutputBytes,
		CancelTimeout:  50 * time.Millisecond,
		CleanupTimeout: 50 * time.Millisecond,
		OperationID: func(context.Context) (string, error) {
			return "operation_test", nil
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	return runner
}

type controlPlaneRunnerHarness struct {
	defaults  *codeRunnerDefaultRepository
	providers *codeRunnerProviderRepository
	router    *codeRunnerRouter
	selection *codeRunnerSelection
	source    *codeRunnerBindingSource
}

func newControlPlaneRunnerHarness(t *testing.T) *controlPlaneRunnerHarness {
	t.Helper()
	result, err := infrasandbox.MarshalCodeRunnerResultEnvelope(map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("MarshalCodeRunnerResultEnvelope() error = %v", err)
	}
	selection := &codeRunnerSelection{
		policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds:       30,
			MemoryLimitMB:        128,
			CPULimit:             1,
			MaxOutputBytes:       4096,
			MaxConcurrency:       2,
			VirtualReadPrefixes:  []string{"workspace"},
			VirtualWritePrefixes: []string{"workspace"},
			AllowedExecutables:   []string{"python3", "node"},
		},
		results: []infrasandbox.ExecuteResult{{
			ExecutionID: "execution_success",
			Status:      infrasandbox.ExecutionStatusSucceeded,
			ExitCode:    codeRunnerIntPointer(0),
			Stdout:      string(result),
		}},
	}
	defaults := &codeRunnerDefaultRepository{value: &domainsandbox.ProviderDefault{
		Scope: domainsandbox.ScopeAgent, ProviderID: 42, Version: 1,
	}}
	providers := &codeRunnerProviderRepository{value: &domainsandbox.Provider{
		ID: 42, ProviderKey: "agent-provider",
	}}
	router := &codeRunnerRouter{selection: selection}
	source := &codeRunnerBindingSource{
		ok: true,
		binding: Binding{
			ProviderDefaults: defaults,
			Providers:        providers,
			Router:           router,
		},
	}
	return &controlPlaneRunnerHarness{
		defaults: defaults, providers: providers, router: router,
		selection: selection, source: source,
	}
}

func configurePluginHarness(harness *controlPlaneRunnerHarness) {
	harness.defaults.value.Scope = domainsandbox.ScopePlugin
	harness.providers.value.ProviderKey = "plugin-provider"
}

type codeRunnerBindingSource struct {
	binding Binding
	ok      bool
}

func (s *codeRunnerBindingSource) LoadCodeRunnerSandboxBinding() (Binding, bool) {
	return s.binding, s.ok
}

type codeRunnerDefaultRepository struct {
	value *domainsandbox.ProviderDefault
	err   error
	scope domainsandbox.Scope
}

func (r *codeRunnerDefaultRepository) GetProviderDefault(
	_ context.Context,
	scope domainsandbox.Scope,
) (*domainsandbox.ProviderDefault, error) {
	r.scope = scope
	if r.err != nil {
		return nil, r.err
	}
	if r.value == nil {
		return nil, domainsandbox.ErrDefaultMissing
	}
	copy := *r.value
	return &copy, nil
}

type codeRunnerProviderRepository struct {
	value *domainsandbox.Provider
	err   error
}

func (r *codeRunnerProviderRepository) GetProvider(context.Context, int64) (*domainsandbox.Provider, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.value == nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *r.value
	return &copy, nil
}

type codeRunnerRouter struct {
	providerKey string
	scope       domainsandbox.Scope
	selection   Selection
	err         error
}

func (r *codeRunnerRouter) ResolveCodeRunnerSandbox(
	_ context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (Selection, error) {
	r.providerKey = providerKey
	r.scope = scope
	if r.err != nil {
		return nil, r.err
	}
	return r.selection, nil
}

type codeRunnerSelection struct {
	mu           sync.Mutex
	policy       domainsandbox.RuntimePolicy
	results      []infrasandbox.ExecuteResult
	errs         []error
	requests     []infrasandbox.ExecuteRequest
	cancelCalls  int
	releaseCalls int
}

func (s *codeRunnerSelection) CodeRunnerSandboxPolicy() domainsandbox.RuntimePolicy {
	return s.policy
}

func (s *codeRunnerSelection) Status(context.Context) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{
		ExecutionID: "execution_running",
		Status:      infrasandbox.ExecutionStatusCanceled,
	}, nil
}

func (s *codeRunnerSelection) Execute(
	_ context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := len(s.requests)
	s.requests = append(s.requests, request)
	var result infrasandbox.ExecuteResult
	if len(s.results) > 0 {
		result = s.results[min(index, len(s.results)-1)]
	}
	var err error
	if index < len(s.errs) {
		err = s.errs[index]
	}
	return result, err
}

func (s *codeRunnerSelection) Cancel(context.Context, string) error {
	s.cancelCalls++
	return nil
}

func (s *codeRunnerSelection) Release(context.Context) error {
	s.releaseCalls++
	return nil
}

func codeRunnerIntPointer(value int) *int {
	return &value
}
