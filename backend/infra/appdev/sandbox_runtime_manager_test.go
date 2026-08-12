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
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const appDevTestProviderKey = "018f0d2e-7b73-7e21-9a89-1a2b3c4d5e6f"

type sandboxRuntimeDefaultStub struct {
	providerKey string
	err         error
	calls       int
	scope       domainsandbox.Scope
}

func (s *sandboxRuntimeDefaultStub) ResolveDefaultProviderKey(
	_ context.Context,
	scope domainsandbox.Scope,
) (string, error) {
	s.calls++
	s.scope = scope
	return s.providerKey, s.err
}

type sandboxRuntimeRouterStub struct {
	mu          sync.Mutex
	providerKey string
	scope       domainsandbox.Scope
	calls       int
	selections  []*sandboxRuntimeSelectionStub
	err         error
}

func (s *sandboxRuntimeRouterStub) ResolveAppDev(
	_ context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (appDevSandboxSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.providerKey = providerKey
	s.scope = scope
	if s.err != nil {
		return nil, s.err
	}
	selection := newSandboxRuntimeSelectionStub()
	s.selections = append(s.selections, selection)
	return selection, nil
}

func (s *sandboxRuntimeRouterStub) latestSelection(t *testing.T) *sandboxRuntimeSelectionStub {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotEmpty(t, s.selections)
	return s.selections[len(s.selections)-1]
}

type sandboxRuntimeSelectionStub struct {
	mu           sync.Mutex
	request      infrasandbox.ExecuteRequest
	executeCall  int
	cancelIDs    []string
	renewCalls   int
	releaseCalls int
	started      chan struct{}
	cancelled    chan struct{}
	cancelOnce   sync.Once
}

func newSandboxRuntimeSelectionStub() *sandboxRuntimeSelectionStub {
	return &sandboxRuntimeSelectionStub{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
}

func (s *sandboxRuntimeSelectionStub) Policy() domainsandbox.RuntimePolicy {
	return domainsandbox.RuntimePolicy{
		TimeoutSeconds: 45,
		MemoryLimitMB:  512,
		CPULimit:       2,
		MaxOutputBytes: 64 * 1024,
		MaxConcurrency: 3,
	}
}

func (s *sandboxRuntimeSelectionStub) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	s.mu.Lock()
	s.executeCall++
	s.request = request
	s.mu.Unlock()
	close(s.started)
	select {
	case <-s.cancelled:
		return infrasandbox.ExecuteResult{
			ExecutionID: request.IdempotencyKey,
			Status:      infrasandbox.ExecutionStatusCanceled,
			Stdout:      "preview=https://attacker.example.net/steal",
		}, nil
	case <-ctx.Done():
		return infrasandbox.ExecuteResult{
			ExecutionID: request.IdempotencyKey,
			Status:      infrasandbox.ExecutionStatusCanceled,
		}, ctx.Err()
	}
}

func (s *sandboxRuntimeSelectionStub) Cancel(_ context.Context, executionID string) error {
	s.mu.Lock()
	s.cancelIDs = append(s.cancelIDs, executionID)
	s.mu.Unlock()
	s.cancelOnce.Do(func() { close(s.cancelled) })
	return nil
}

func (s *sandboxRuntimeSelectionStub) Renew(context.Context) error {
	s.mu.Lock()
	s.renewCalls++
	s.mu.Unlock()
	return nil
}

func (s *sandboxRuntimeSelectionStub) Release(context.Context) error {
	s.mu.Lock()
	s.releaseCalls++
	s.mu.Unlock()
	return nil
}

func (s *sandboxRuntimeSelectionStub) snapshot() (infrasandbox.ExecuteRequest, int, []string, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request, s.executeCall, append([]string(nil), s.cancelIDs...), s.renewCalls, s.releaseCalls
}

func sandboxRuntimeRequest() *appdevapp.RuntimeManagerRequest {
	return &appdevapp.RuntimeManagerRequest{
		SpaceID:     "1001",
		ActorUserID: 42,
		ProjectID:   "project-1",
		ProjectDir:  "/Users/tester/private/project",
		SourceURL:   "https://objects.example.com/source.zip?signature=must-not-log",
		Snapshot: &appdevapp.RuntimeSnapshotReference{
			ID:          "snapshot-0123456789abcdef",
			Path:        "source.zip",
			Digest:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Size:        4096,
			DownloadURL: "https://objects.example.com/source.zip?signature=must-not-log",
		},
	}
}

func newSandboxRuntimeManagerForTest(
	t *testing.T,
	router *sandboxRuntimeRouterStub,
	defaults *sandboxRuntimeDefaultStub,
	ids ...string,
) *SandboxRuntimeManager {
	t.Helper()
	index := 0
	manager, err := newSandboxRuntimeManager(sandboxRuntimeManagerOptions{
		router:         router,
		defaults:       defaults,
		previewBaseURL: "https://preview.example.com/apps",
		now: func() time.Time {
			return time.Unix(2_000_100_000, 0).UTC()
		},
		newRuntimeID: func() (string, error) {
			if index >= len(ids) {
				return "", errors.New("runtime id test fixture exhausted")
			}
			value := ids[index]
			index++
			return value, nil
		},
	})
	require.NoError(t, err)
	return manager
}

func TestSandboxRuntimeManagerUsesScopeAppDevAndBoundedSafeExecuteRequest(t *testing.T) {
	defaults := &sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey}
	router := &sandboxRuntimeRouterStub{}
	manager := newSandboxRuntimeManagerForTest(t, router, defaults, "appdev-runtime-1")

	info, err := manager.Start(context.Background(), sandboxRuntimeRequest())
	require.NoError(t, err)
	require.Equal(t, domainappdev.RuntimeStatusRunning, info.Status)
	require.Equal(t, "https://preview.example.com/apps/appdev-runtime-1/", info.PreviewURL)

	selection := router.latestSelection(t)
	select {
	case <-selection.started:
	case <-time.After(time.Second):
		t.Fatal("sandbox execution did not start")
	}
	request, executeCalls, _, _, _ := selection.snapshot()
	require.Equal(t, 1, defaults.calls)
	require.Equal(t, domainsandbox.ScopeAppDev, defaults.scope)
	require.Equal(t, appDevTestProviderKey, router.providerKey)
	require.Equal(t, domainsandbox.ScopeAppDev, router.scope)
	require.Equal(t, 1, executeCalls)
	require.Equal(t, domainsandbox.ScopeAppDev, request.Scope)
	require.Equal(t, infrasandbox.WorkloadAppDev, request.WorkloadKind)
	require.Equal(t, "appdev-runtime-1", request.IdempotencyKey)
	require.Equal(t, "appdev/runtime", request.Entrypoint)
	require.Equal(t, infrasandbox.ExecutionIdentity{
		SpaceID: 1001, UserID: 42, ProjectID: "project-1", ExecutionID: "appdev-runtime-1",
	}, request.Identity)
	require.Empty(t, request.Env)
	require.LessOrEqual(t, request.Deadline.Sub(time.Unix(2_000_100_000, 0).UTC()), 45*time.Second)
	require.Equal(t, 512, request.Policy.MemoryLimitMB)
	require.Equal(t, float64(2), request.Policy.CPULimit)
	require.Len(t, request.Files, 1)
	require.Equal(t, "snapshot-0123456789abcdef", request.Files[0].ID)
	require.Equal(t, "source.zip", request.Files[0].Path)

	var manifest map[string]any
	require.NoError(t, json.Unmarshal(request.Stdin, &manifest))
	require.Equal(t, "appdev-runtime-1", manifest["runtime_id"])
	require.Equal(t, "start", manifest["operation"])
	require.EqualValues(t, 4173, manifest["preview_port"])
	serialized := string(request.Stdin)
	require.NotContains(t, serialized, "/Users/tester")
	require.NotContains(t, serialized, "DB_PASSWORD")
	require.NotContains(t, serialized, "signature=must-not-log")

	stopped, err := manager.Stop(context.Background(), sandboxRuntimeRequest())
	require.NoError(t, err)
	require.Equal(t, domainappdev.RuntimeStatusStopped, stopped.Status)
	_, _, cancelIDs, _, releaseCalls := selection.snapshot()
	require.Equal(t, []string{"appdev-runtime-1"}, cancelIDs)
	require.Equal(t, 1, releaseCalls)
}

func TestSandboxRuntimeManagerFailsClosedWithoutDefaultOrOnOpaqueRouterError(t *testing.T) {
	request := sandboxRuntimeRequest()

	missing := newSandboxRuntimeManagerForTest(
		t,
		&sandboxRuntimeRouterStub{},
		&sandboxRuntimeDefaultStub{err: domainsandbox.ErrDefaultMissing},
		"unused-runtime",
	)
	_, err := missing.Start(context.Background(), request)
	require.ErrorIs(t, err, domainsandbox.ErrDefaultMissing)

	router := &sandboxRuntimeRouterStub{err: errors.New("token=supersecret /Users/operator/private")}
	manager := newSandboxRuntimeManagerForTest(
		t,
		router,
		&sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey},
		"unused-runtime",
	)
	_, err = manager.Start(context.Background(), request)
	require.ErrorIs(t, err, domainsandbox.ErrUnavailable)
	require.NotContains(t, err.Error(), "supersecret")
	require.NotContains(t, err.Error(), "/Users/")
}

func TestSandboxRuntimeManagerRetryReusesIdempotencyAndRestartUsesNewOperationIdentity(t *testing.T) {
	defaults := &sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey}
	router := &sandboxRuntimeRouterStub{}
	manager := newSandboxRuntimeManagerForTest(t, router, defaults, "appdev-runtime-1", "appdev-runtime-2")
	request := sandboxRuntimeRequest()

	first, err := manager.Start(context.Background(), request)
	require.NoError(t, err)
	retried, err := manager.Start(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, first.PreviewURL, retried.PreviewURL)
	require.Equal(t, 1, router.calls)

	restarted, err := manager.Restart(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "https://preview.example.com/apps/appdev-runtime-2/", restarted.PreviewURL)
	require.Equal(t, 2, router.calls)
	secondSelection := router.latestSelection(t)
	secondRequest, _, _, _, _ := secondSelection.snapshot()
	require.Equal(t, "appdev-runtime-2", secondRequest.IdempotencyKey)
	require.NotEqual(t, first.PreviewURL, restarted.PreviewURL)

	_, err = manager.Stop(context.Background(), request)
	require.NoError(t, err)
}

func TestSandboxRuntimeManagerRejectsUnsafeSnapshotAndPreviewConfiguration(t *testing.T) {
	_, err := newSandboxRuntimeManager(sandboxRuntimeManagerOptions{
		router:         &sandboxRuntimeRouterStub{},
		defaults:       &sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey},
		previewBaseURL: "http://preview.example.com/apps",
		now:            time.Now,
		newRuntimeID:   func() (string, error) { return "appdev-runtime-1", nil },
	})
	require.Error(t, err)

	manager := newSandboxRuntimeManagerForTest(
		t,
		&sandboxRuntimeRouterStub{},
		&sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey},
		"appdev-runtime-1",
	)
	request := sandboxRuntimeRequest()
	request.Snapshot.Path = "../../host-secret"
	_, err = manager.Start(context.Background(), request)
	require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
}

func TestSandboxRuntimeManagerKeepAliveRenewsRouterLease(t *testing.T) {
	router := &sandboxRuntimeRouterStub{}
	manager := newSandboxRuntimeManagerForTest(
		t,
		router,
		&sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey},
		"appdev-runtime-1",
	)
	request := sandboxRuntimeRequest()
	_, err := manager.Start(context.Background(), request)
	require.NoError(t, err)

	_, err = manager.KeepAlive(context.Background(), request)
	require.NoError(t, err)
	_, _, _, renewCalls, _ := router.latestSelection(t).snapshot()
	require.Equal(t, 1, renewCalls)

	_, err = manager.Stop(context.Background(), request)
	require.NoError(t, err)
}

func TestSandboxRuntimeManagerSanitizesExecutionFailure(t *testing.T) {
	router := &sandboxRuntimeRouterStub{err: errors.New(strings.Repeat("x", 32) + " API_TOKEN=must-not-leak")}
	manager := newSandboxRuntimeManagerForTest(
		t,
		router,
		&sandboxRuntimeDefaultStub{providerKey: appDevTestProviderKey},
		"appdev-runtime-1",
	)
	_, err := manager.Start(context.Background(), sandboxRuntimeRequest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "must-not-leak")
}
