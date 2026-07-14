// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
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
	if err := registry.Register(adapter); err != nil {
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
	if err := registry.Register(hook); err != nil {
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
	if err := permanent.Register(blocked); err != nil {
		t.Fatalf("register blocked hook: %v", err)
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer shortCancel()
	err := shutdownRegistryWithRetry(shortCtx, permanent, 5*time.Millisecond, time.Millisecond)
	if !errors.Is(err, errApplicationShutdownFailed) || permanent.Pending() != 1 {
		t.Fatalf("permanent shutdown failure was lost: err=%v pending=%d", err, permanent.Pending())
	}
}

type retryableApplicationShutdownHook struct {
	calls    atomic.Int32
	failures int32
}

func (h *retryableApplicationShutdownHook) Shutdown(context.Context) error {
	call := h.calls.Add(1)
	if call <= h.failures {
		return errors.New("orphan remains")
	}
	return nil
}
