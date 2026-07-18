// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestMCPManagementRuntimeApplicationShutdownInvokesConcreteFactory(t *testing.T) {
	target := &managementBindingTarget{managementResolver: managementResolver{server: managementRuntimeServer()}}
	adapter, err := bindMCPManagementRuntime(target, agentthread.ADKMCPRuntimeBootstrapConfig{
		Enabled:              true,
		RemoteEinoEnabled:    true,
		RemoteAllowedHosts:   []string{"mcp.example.com"},
		RemoteMaxConfigBytes: 4096,
		RemoteMaxHeaders:     8,
		RemoteMaxHeaderBytes: 2048,
	})
	if err != nil || adapter == nil || adapter.managementFactory == nil {
		t.Fatalf("bind concrete runtime: adapter=%v err=%v", adapter, err)
	}
	registry := newApplicationShutdownRegistry()
	if _, err := registry.Register(adapter); err != nil {
		t.Fatalf("register shutdown: %v", err)
	}
	if err := registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("application shutdown: %v", err)
	}
	_, err = adapter.managementFactory.Open(context.Background(), mcpruntime.Connection{
		ServerType: mcpruntime.ServerTypeSSE,
		Config:     `{"url":"https://mcp.example.com/events"}`,
		Auth:       `{}`,
	})
	if !errors.Is(err, mcpruntime.ErrSessionUnavailable) {
		t.Fatalf("management factory remained open after lifecycle shutdown: %v", err)
	}
}

func TestMCPManagementRuntimeApplicationShutdownRetriesAndReportsPendingHooks(t *testing.T) {
	registry := newApplicationShutdownRegistry()
	hook := &retryableApplicationShutdownHook{failures: 1}
	if _, err := registry.Register(hook); err != nil {
		t.Fatalf("register retryable hook: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdownRegistryWithRetry(ctx, registry, 20*time.Millisecond, time.Millisecond); err != nil {
		t.Fatalf("retry shutdown: %v", err)
	}
	if hook.calls.Load() != 2 || registry.Pending() != 0 {
		t.Fatalf("shutdown retry state: calls=%d pending=%d", hook.calls.Load(), registry.Pending())
	}

	permanent := newApplicationShutdownRegistry()
	blocked := &retryableApplicationShutdownHook{failures: 1 << 30}
	if _, err := permanent.Register(blocked); err != nil {
		t.Fatalf("register blocked hook: %v", err)
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer shortCancel()
	err := shutdownRegistryWithRetry(shortCtx, permanent, 5*time.Millisecond, time.Millisecond)
	if !errors.Is(err, errApplicationShutdownFailed) || permanent.Pending() != 1 {
		t.Fatalf("permanent shutdown failure was lost: err=%v pending=%d", err, permanent.Pending())
	}
}

func TestApplicationShutdownRegistryPreservesConcurrentRegistryIdentity(t *testing.T) {
	registry := newApplicationShutdownRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	oldHook := &barrierApplicationShutdownHook{
		name: "old-runtime", started: started, release: release, err: errors.New("old cleanup failed"),
	}
	oldRegistration, err := registry.Register(oldHook)
	if err != nil {
		t.Fatalf("register old hook: %v", err)
	}
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- registry.Shutdown(context.Background())
	}()
	<-started
	newHook := &barrierApplicationShutdownHook{name: "new-runtime"}
	newRegistration, err := registry.Register(newHook)
	if err != nil {
		t.Fatalf("register new hook: %v", err)
	}
	if !registry.Unregister(oldRegistration) {
		t.Fatal("old registration was not removed")
	}
	close(release)
	shutdownErr := <-shutdownDone
	if !errors.Is(shutdownErr, oldHook.err) || !strings.Contains(shutdownErr.Error(), "old-runtime") {
		t.Fatalf("shutdown error lost identity: %v", shutdownErr)
	}
	if registry.Pending() != 1 {
		t.Fatalf("concurrent registration was lost or old hook revived: %d", registry.Pending())
	}
	if !registry.Unregister(newRegistration) || registry.Pending() != 0 {
		t.Fatal("new registration identity could not be removed")
	}
}

func TestApplicationShutdownRegistryCommitsPartialResultsAndRetriesFailures(t *testing.T) {
	registry := newApplicationShutdownRegistry()
	success := &retryableApplicationShutdownHook{}
	failureErr := errors.New("owner release unavailable")
	failure := &barrierApplicationShutdownHook{name: "provider-runtime", err: failureErr}
	if _, err := registry.Register(success); err != nil {
		t.Fatal(err)
	}
	failureRegistration, err := registry.Register(failure)
	if err != nil {
		t.Fatal(err)
	}

	err = registry.Shutdown(context.Background())
	if !errors.Is(err, failureErr) || !strings.Contains(err.Error(), "provider-runtime") {
		t.Fatalf("partial error lost: %v", err)
	}
	if registry.Pending() != 1 || success.calls.Load() != 1 {
		t.Fatalf("partial commit failed: pending=%d success calls=%d", registry.Pending(), success.calls.Load())
	}
	failure.err = nil
	if err := registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if registry.Pending() != 0 || failure.calls.Load() != 2 {
		t.Fatalf("retry state invalid: pending=%d calls=%d", registry.Pending(), failure.calls.Load())
	}
	if registry.Unregister(failureRegistration) {
		t.Fatal("successful hook registration was resurrected")
	}
}

type retryableApplicationShutdownHook struct {
	calls    atomic.Int32
	failures int32
}

type barrierApplicationShutdownHook struct {
	name    string
	started chan struct{}
	release chan struct{}
	err     error
	once    sync.Once
	calls   atomic.Int32
}

func (hook *barrierApplicationShutdownHook) ShutdownName() string { return hook.name }

func (hook *barrierApplicationShutdownHook) Shutdown(context.Context) error {
	hook.calls.Add(1)
	if hook.started != nil {
		hook.once.Do(func() { close(hook.started) })
	}
	if hook.release != nil {
		<-hook.release
	}
	return hook.err
}

func (h *retryableApplicationShutdownHook) Shutdown(context.Context) error {
	call := h.calls.Add(1)
	if call <= h.failures {
		return errors.New("orphan remains")
	}
	return nil
}
