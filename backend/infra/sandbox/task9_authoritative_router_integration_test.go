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

package sandbox_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const authoritativeIntegrationProviderKey = "remote-authority"

type authoritativeRouterDoer struct {
	mu      sync.Mutex
	calls   int
	bodies  [][]byte
	methods []string
	paths   []string
}

func (d *authoritativeRouterDoer) Do(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.calls++
	call := d.calls
	d.bodies = append(d.bodies, body)
	d.methods = append(d.methods, request.Method)
	d.paths = append(d.paths, request.URL.Path)
	d.mu.Unlock()
	if call == 1 {
		return nil, errors.New("original response lost")
	}
	if call == 2 {
		response := testJSONResponse(request, http.StatusNotFound, `{"code":"execution_not_recorded"}`)
		response.Header.Set("X-Coze-Sandbox-Reconciliation-Version", "1")
		return response, nil
	}
	return testJSONResponse(request, http.StatusOK, `{
		"schema":"coze.sandbox.execute.v1","execution_id":"exec-after-authority",
		"status":"succeeded","exit_code":0,"stdout":"","stderr":"","artifacts":[]
	}`), nil
}

func (d *authoritativeRouterDoer) snapshot() (int, [][]byte, []string, []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls, append([][]byte(nil), d.bodies...), append([]string(nil), d.methods...), append([]string(nil), d.paths...)
}

type authoritativeProviderLookup struct {
	provider *domainsandbox.Provider
}

func (l authoritativeProviderLookup) GetProviderByKey(context.Context, string) (*domainsandbox.Provider, error) {
	copy := *l.provider
	return &copy, nil
}

type authoritativeRuntimeFactory struct {
	runtime infrasandbox.RuntimeProvider
}

func (authoritativeRuntimeFactory) ValidateConfig(context.Context, appsandbox.ProviderDescriptor) error {
	return nil
}

func (f authoritativeRuntimeFactory) Build(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return f.runtime, nil
}

type authoritativeCapacityLimiter struct {
	mu       sync.Mutex
	releases int
}

func (*authoritativeCapacityLimiter) Acquire(context.Context, string, string, string, int, time.Duration) (int64, error) {
	return time.Now().Add(time.Minute).UnixMilli(), nil
}

func (*authoritativeCapacityLimiter) Renew(context.Context, string, string, string, string, time.Duration) (int64, error) {
	return time.Now().Add(time.Minute).UnixMilli(), nil
}

func (l *authoritativeCapacityLimiter) Release(context.Context, string, string, string) error {
	l.mu.Lock()
	l.releases++
	l.mu.Unlock()
	return nil
}

func TestRouterAcceptsAuthoritativeNotRecordedOnlyFromRemoteProtocol(t *testing.T) {
	doer := &authoritativeRouterDoer{}
	remote, err := infrasandbox.NewRemoteProviderWithTestDoer(infrasandbox.RemoteProviderConfig{
		Endpoint: "https://sandbox.example.test/", Credential: "synthetic-test-token",
		AllowedHosts: []string{"sandbox.example.test"}, Timeout: 2 * time.Second,
	}, doer)
	if err != nil {
		t.Fatalf("new RemoteProvider: %v", err)
	}
	now := time.Now().UTC()
	provider := &domainsandbox.Provider{
		ID: 1, ProviderKey: authoritativeIntegrationProviderKey, Name: "authority",
		Type: domainsandbox.ProviderTypeRemoteHTTP, Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30, MemoryLimitMB: 256, CPULimit: 1,
			MaxOutputBytes: 1024 * 1024, MaxConcurrency: 1,
		},
		Status: domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{
			Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent}, CheckedAt: now,
		},
	}
	limiter := &authoritativeCapacityLimiter{}
	router, err := appsandbox.NewProviderRouter(
		authoritativeProviderLookup{provider: provider}, authoritativeRuntimeFactory{runtime: remote}, limiter, 2*time.Second,
	)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	resolve, err := appsandbox.NewResolveProviderRequest(authoritativeIntegrationProviderKey, domainsandbox.ScopeAgent)
	if err != nil {
		t.Fatalf("resolve request: %v", err)
	}
	selected, err := router.Resolve(context.Background(), resolve)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	request := infrasandbox.ExecuteRequest{
		Scope: domainsandbox.ScopeAgent, WorkloadKind: infrasandbox.WorkloadAgent,
		IdempotencyKey: "idem-authority-e2e", Deadline: time.Now().Add(time.Minute),
		Policy: provider.Policy, Entrypoint: "bin/run",
	}
	if _, err := selected.Execute(context.Background(), request); err == nil {
		t.Fatal("ambiguous original Execute succeeded")
	}
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("authoritative reconciliation result = %v", err)
	}
	different := request
	different.IdempotencyKey = "idem-authority-new"
	different.Args = []string{"--different"}
	result, err := selected.Execute(context.Background(), different)
	if err != nil || result.ExecutionID != "exec-after-authority" {
		t.Fatalf("Execute after authority = %#v, %v", result, err)
	}
	calls, bodies, methods, paths := doer.snapshot()
	if calls != 3 || len(bodies) != 3 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("remote calls/body replay = %d/%d/%v", calls, len(bodies), len(bodies) >= 2 && bytes.Equal(bodies[0], bodies[1]))
	}
	for index := range methods {
		if methods[index] != http.MethodPost || paths[index] != "/v1/executions" {
			t.Fatalf("call %d = %s %s", index, methods[index], paths[index])
		}
	}
	if strings.Contains(string(bodies[1]), "synthetic-test-token") {
		t.Fatalf("credential leaked to wire: %s", bodies[1])
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func testJSONResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
	}
}
