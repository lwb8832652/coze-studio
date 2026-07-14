// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestProductionSessionFactoryFailedOpenKeepsRetryableOrphanUntilCleanup(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	client := &retryableCloseProtocolClient{
		fakeProtocolClient: &fakeProtocolClient{initializeErr: errors.New("initialize failed")},
	}
	var workdir string
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			workdir = connection.Stdio.WorkingDir
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("failed open = %v", err)
	}
	shortCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = factory.Shutdown(shortCtx)
	cancel()
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("shutdown discarded live orphan: %v", err)
	}
	factory.workdirManager.mu.RLock()
	closedEarly := factory.workdirManager.closed
	factory.workdirManager.mu.RUnlock()
	if closedEarly {
		t.Fatal("manager closed while orphan termination remained unconfirmed")
	}
	if _, err := os.Stat(workdir); err != nil {
		t.Fatalf("orphan workdir was deleted before termination: %v", err)
	}
	client.allowClose.Store(true)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("retry shutdown did not drain orphan: %v", err)
	}
	if _, err := os.Stat(workdir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("drained orphan workdir remains: %v", err)
	}
}

func TestProtocolSessionCloseRemainsRetryableAndActiveUntilCleanup(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	client := &retryableCloseProtocolClient{
		fakeProtocolClient: &fakeProtocolClient{
			initializeResult: initializeResultWithCapabilities(t, true, false, false),
		},
	}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(context.Context, ResolvedConnection) (ProtocolClient, error) {
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	session, err := factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := session.Close(); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("first close must stay retryable: %v", err)
	}
	factory.lifecycleMu.Lock()
	active := factory.active
	factory.lifecycleMu.Unlock()
	if active != 1 {
		t.Fatalf("failed close released active session: %d", active)
	}
	client.allowClose.Store(true)
	if err := session.Close(); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	factory.lifecycleMu.Lock()
	active = factory.active
	factory.lifecycleMu.Unlock()
	if active != 0 {
		t.Fatalf("successful retry did not release session: %d", active)
	}
	if err := factory.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

type retryableCloseProtocolClient struct {
	*fakeProtocolClient
	allowClose atomic.Bool
}

func (c *retryableCloseProtocolClient) Close() error {
	c.closeCalls.Add(1)
	if !c.allowClose.Load() {
		return errors.New("termination not yet confirmed")
	}
	return nil
}
