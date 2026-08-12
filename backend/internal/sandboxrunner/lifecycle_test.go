// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestLifecycleDerivesTenantScopedReuseKeysAndIdleTTLs(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	tests := []struct {
		name    string
		scope   domainsandbox.Scope
		request LifecycleRequest
		wantKey string
		wantTTL time.Duration
	}{
		{name: "agent", scope: domainsandbox.ScopeAgent, request: lifecycleRequest(domainsandbox.ScopeAgent, "exec-agent", 10, 20, "project", "session"), wantKey: "agent:10:20", wantTTL: 5 * time.Minute},
		{name: "appdev", scope: domainsandbox.ScopeAppDev, request: lifecycleRequest(domainsandbox.ScopeAppDev, "exec-appdev", 10, 20, "project", "session"), wantKey: "appdev:10:project", wantTTL: 10 * time.Minute},
		{name: "mcp", scope: domainsandbox.ScopeMCPStdio, request: lifecycleRequest(domainsandbox.ScopeMCPStdio, "exec-mcp", 10, 20, "project", "session"), wantKey: "mcp_stdio:10:session", wantTTL: 3 * time.Minute},
		{name: "plugin", scope: domainsandbox.ScopePlugin, request: lifecycleRequest(domainsandbox.ScopePlugin, "exec-plugin", 10, 20, "project", "session"), wantKey: "plugin:10:exec-plugin", wantTTL: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, ttl, err := manager.ReuseKey(test.request)
			if err != nil || key != test.wantKey || ttl != test.wantTTL {
				t.Fatalf("ReuseKey() = %q, %v, %v; want %q, %v", key, ttl, err, test.wantKey, test.wantTTL)
			}
			lease, err := manager.Acquire(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			if label := driver.created[len(driver.created)-1].Spec.ReuseKeyHash; label == "" || label == key {
				t.Fatalf("runtime label leaked raw reuse key: %q", label)
			}
			if err := manager.Release(context.Background(), lease.ExecutionID); err != nil {
				t.Fatal(err)
			}
		})
	}
	if got := driver.destroyed; len(got) != 1 || got[0] != "container-4" {
		t.Fatalf("plugin must destroy on release, destroyed=%v", got)
	}
}

func TestLifecycleReusesOnlyMatchingCleanHealthyContainers(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	request := lifecycleRequest(domainsandbox.ScopeAgent, "exec-first", 10, 20, "project", "session")
	first, err := manager.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(context.Background(), first.ExecutionID); err != nil {
		t.Fatal(err)
	}
	request.ExecutionID = "exec-reuse"
	request.Identity.ExecutionID = request.ExecutionID
	reused, err := manager.Acquire(context.Background(), request)
	if err != nil || reused.ContainerID != first.ContainerID || driver.prepareCalls != 1 || driver.healthCalls != 1 {
		t.Fatalf("reused/error/prepare/health = %#v/%v/%d/%d", reused, err, driver.prepareCalls, driver.healthCalls)
	}
	if err := manager.Release(context.Background(), reused.ExecutionID); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*LifecycleRequest){
		func(value *LifecycleRequest) { value.PolicyVersion = "policy-v2" },
		func(value *LifecycleRequest) {
			value.ImageDigest = "registry.example/newx/runtime@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		func(value *LifecycleRequest) { value.SchedulerVersion++ },
		func(value *LifecycleRequest) { value.CredentialGeneration = "credential-v2" },
	} {
		mutate(&request)
		request.ExecutionID = "exec-new-generation-" + strconv.Itoa(len(driver.created))
		request.Identity.ExecutionID = request.ExecutionID
		fresh, err := manager.Acquire(context.Background(), request)
		if err != nil || fresh.ContainerID == first.ContainerID {
			t.Fatalf("fresh/error = %#v/%v", fresh, err)
		}
		if err := manager.Release(context.Background(), fresh.ExecutionID); err != nil {
			t.Fatal(err)
		}
	}
	if len(driver.destroyed) != 4 {
		t.Fatalf("generation mismatch must drain each old idle container, destroyed=%v", driver.destroyed)
	}
}

func TestLifecycleQuarantinesFailedCleanupAndNeverReusesIt(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	request := lifecycleRequest(domainsandbox.ScopeAgent, "exec-first", 10, 20, "project", "session")
	first, err := manager.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(context.Background(), first.ExecutionID); err != nil {
		t.Fatal(err)
	}
	driver.prepareErr = errors.New("cleanup failed")
	request.ExecutionID = "exec-second"
	request.Identity.ExecutionID = request.ExecutionID
	second, err := manager.Acquire(context.Background(), request)
	if err != nil || second.ContainerID == first.ContainerID || manager.Snapshot().Quarantined != 1 {
		t.Fatalf("second/error/snapshot = %#v/%v/%#v", second, err, manager.Snapshot())
	}
}

func TestLifecycleCancellationHonorsGraceThenForceKillsAndDestroysPlugin(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	driver.stopped = false
	lease, err := manager.Acquire(context.Background(), lifecycleRequest(domainsandbox.ScopePlugin, "exec-plugin", 10, 20, "project", "session"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Cancel(context.Background(), lease.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if driver.terminateCalls != 1 || driver.waitGrace != 5*time.Second || driver.killCalls != 1 || len(driver.destroyed) != 1 {
		t.Fatalf("terminate/grace/kill/destroy = %d/%v/%d/%v", driver.terminateCalls, driver.waitGrace, driver.killCalls, driver.destroyed)
	}
}

func TestLifecycleRecoveryKeepsOnlyIdleCompatibleContainers(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	request := lifecycleRequest(domainsandbox.ScopeAgent, "exec-recovered", 10, 20, "project", "session")
	key, _, err := manager.ReuseKey(request)
	if err != nil {
		t.Fatal(err)
	}
	driver.listed = []sandboxruntime.Container{
		{ID: "idle-container", Spec: sandboxruntime.Specification{ReuseKeyHash: lifecycleHash(key), Scope: string(request.Scope), ImageDigest: request.ImageDigest, PolicyVersion: request.PolicyVersion, SchedulerVersion: request.SchedulerVersion, CredentialGeneration: request.CredentialGeneration, DeploymentID: "runner-test"}, State: sandboxruntime.ContainerStateIdle},
		{ID: "running-container", Spec: sandboxruntime.Specification{ReuseKeyHash: lifecycleHash(key), Scope: string(request.Scope), ImageDigest: request.ImageDigest, PolicyVersion: request.PolicyVersion, SchedulerVersion: request.SchedulerVersion, CredentialGeneration: request.CredentialGeneration, DeploymentID: "runner-test"}, State: sandboxruntime.ContainerStateRunning},
	}
	if err := manager.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	request.ExecutionID = "exec-reuse-recovered"
	request.Identity.ExecutionID = request.ExecutionID
	lease, err := manager.Acquire(context.Background(), request)
	if err != nil || lease.ContainerID != "idle-container" {
		t.Fatalf("recovered lease/error = %#v/%v", lease, err)
	}
	if len(driver.destroyed) != 1 || driver.destroyed[0] != "running-container" {
		t.Fatalf("unknown running container must be destroyed, got %v", driver.destroyed)
	}
}

func TestLifecycleDestroysExpiredIdleContainerBeforeCreatingFreshLease(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	current := now
	driver := &lifecycleDriverFake{stopped: true}
	manager, err := NewLifecycle(LifecycleConfig{Driver: driver, Settings: schedulerSettingsForTest(), DeploymentID: "runner-test", Now: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	request := lifecycleRequest(domainsandbox.ScopeMCPStdio, "exec-first", 10, 20, "project", "session")
	first, err := manager.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(context.Background(), first.ExecutionID); err != nil {
		t.Fatal(err)
	}
	current = current.Add(181 * time.Second)
	request.ExecutionID, request.Identity.ExecutionID = "exec-after-expiry", "exec-after-expiry"
	fresh, err := manager.Acquire(context.Background(), request)
	if err != nil || fresh.ContainerID == first.ContainerID || len(driver.destroyed) != 1 || driver.destroyed[0] != first.ContainerID {
		t.Fatalf("fresh/error/destroyed = %#v/%v/%v", fresh, err, driver.destroyed)
	}
}

func TestLifecycleQuarantinesHealthFailureWithoutReusingContainer(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	request := lifecycleRequest(domainsandbox.ScopeAgent, "exec-first", 10, 20, "project", "session")
	first, err := manager.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(context.Background(), first.ExecutionID); err != nil {
		t.Fatal(err)
	}
	driver.healthErr = errors.New("unhealthy")
	request.ExecutionID, request.Identity.ExecutionID = "exec-second", "exec-second"
	second, err := manager.Acquire(context.Background(), request)
	if err != nil || second.ContainerID == first.ContainerID || manager.Snapshot().Quarantined != 1 {
		t.Fatalf("second/error/snapshot = %#v/%v/%#v", second, err, manager.Snapshot())
	}
}

func TestLifecycleQuarantinesWhenPluginDestroyFails(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, driver := newLifecycleFixture(t, now)
	driver.destroyErr = errors.New("destroy failed")
	lease, err := manager.Acquire(context.Background(), lifecycleRequest(domainsandbox.ScopePlugin, "exec-plugin", 10, 20, "project", "session"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(context.Background(), lease.ExecutionID); !errors.Is(err, ErrUnavailable) || manager.Snapshot().Quarantined != 1 {
		t.Fatalf("Release/snapshot = %v/%#v", err, manager.Snapshot())
	}
}

func TestLifecycleConcurrentAcquireDoesNotLeaseOneContainerTwice(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	manager, _ := newLifecycleFixture(t, now)
	request := lifecycleRequest(domainsandbox.ScopeAgent, "exec-first", 10, 20, "project", "session")
	first, err := manager.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(context.Background(), first.ExecutionID); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	leases := make(chan LifecycleLease, 8)
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			candidate := request
			candidate.ExecutionID = "exec-concurrent-" + strconv.Itoa(index)
			candidate.Identity.ExecutionID = candidate.ExecutionID
			lease, err := manager.Acquire(context.Background(), candidate)
			if err != nil {
				t.Errorf("Acquire(): %v", err)
				return
			}
			leases <- lease
		}(index)
	}
	group.Wait()
	close(leases)
	seen := map[string]struct{}{}
	for lease := range leases {
		if _, exists := seen[lease.ContainerID]; exists {
			t.Fatalf("container leased twice: %q", lease.ContainerID)
		}
		seen[lease.ContainerID] = struct{}{}
	}
}

func lifecycleRequest(scope domainsandbox.Scope, executionID string, spaceID, userID int64, projectID, sessionID string) LifecycleRequest {
	return LifecycleRequest{ExecutionID: executionID, Scope: scope, Identity: sandboxidentity.Request{SpaceID: spaceID, UserID: userID, ProjectID: projectID, SessionID: sessionID, ExecutionID: executionID}, ImageDigest: "registry.example/newx/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PolicyVersion: "policy-v1", SchedulerVersion: 7, CredentialGeneration: "credential-v1"}
}

func newLifecycleFixture(t *testing.T, now time.Time) (*Lifecycle, *lifecycleDriverFake) {
	t.Helper()
	driver := &lifecycleDriverFake{stopped: true}
	manager, err := NewLifecycle(LifecycleConfig{Driver: driver, Settings: schedulerSettingsForTest(), DeploymentID: "runner-test", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return manager, driver
}

type lifecycleDriverFake struct {
	mu             sync.Mutex
	created        []sandboxruntime.Container
	listed         []sandboxruntime.Container
	destroyed      []string
	prepareCalls   int
	healthCalls    int
	terminateCalls int
	killCalls      int
	waitGrace      time.Duration
	stopped        bool
	prepareErr     error
	healthErr      error
	destroyErr     error
}

func (driver *lifecycleDriverFake) Create(_ context.Context, specification sandboxruntime.Specification) (sandboxruntime.Container, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	container := sandboxruntime.Container{ID: "container-" + string(rune('1'+len(driver.created))), Spec: specification, State: sandboxruntime.ContainerStateRunning}
	driver.created = append(driver.created, container)
	return container, nil
}
func (driver *lifecycleDriverFake) PrepareForReuse(context.Context, string) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.prepareCalls++
	return driver.prepareErr
}
func (driver *lifecycleDriverFake) Health(context.Context, string) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.healthCalls++
	return driver.healthErr
}
func (driver *lifecycleDriverFake) Terminate(context.Context, string) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.terminateCalls++
	return nil
}
func (driver *lifecycleDriverFake) WaitStopped(_ context.Context, _ string, grace time.Duration) (bool, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.waitGrace = grace
	return driver.stopped, nil
}
func (driver *lifecycleDriverFake) ForceKill(context.Context, string) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.killCalls++
	return nil
}
func (driver *lifecycleDriverFake) Destroy(_ context.Context, id string) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.destroyed = append(driver.destroyed, id)
	return driver.destroyErr
}
func (driver *lifecycleDriverFake) List(context.Context) ([]sandboxruntime.Container, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	return append([]sandboxruntime.Container(nil), driver.listed...), nil
}
