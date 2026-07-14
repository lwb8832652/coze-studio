// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProductionSessionFactoryShutdownWaitsForActiveSessionAndCanRetry(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	client := &fakeProtocolClient{initializeResult: initializeResultWithCapabilities(t, true, false, false)}
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
	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err = factory.Shutdown(shortCtx)
	cancel()
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("shutdown with active session = %v", err)
	}
	if _, err := factory.Open(context.Background(), Connection{ServerType: ServerTypeStdio}); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("new open after shutdown start = %v", err)
	}
	factory.workdirManager.mu.RLock()
	closedEarly := factory.workdirManager.closed
	factory.workdirManager.mu.RUnlock()
	if closedEarly {
		t.Fatal("manager closed before active session Close")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close active session: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("retry shutdown: %v", err)
	}
}

func TestProductionSessionFactoryShutdownWaitsForOpeningSession(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	client := &fakeProtocolClient{initializeResult: initializeResultWithCapabilities(t, true, false, false)}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(context.Context, ResolvedConnection) (ProtocolClient, error) {
			close(entered)
			<-release
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	type openResult struct {
		session Session
		err     error
	}
	resultCh := make(chan openResult, 1)
	go func() {
		session, err := factory.Open(context.Background(), Connection{
			ServerType: ServerTypeStdio,
			Config:     `{"command":"/bin/echo"}`,
			Auth:       `{}`,
		})
		resultCh <- openResult{session: session, err: err}
	}()
	<-entered
	shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err = factory.Shutdown(shortCtx)
	cancel()
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("shutdown with opening session = %v", err)
	}
	close(release)
	result := <-resultCh
	if result.err != nil || result.session == nil {
		t.Fatalf("opening result: session=%v err=%v", result.session, result.err)
	}
	if err := result.session.Close(); err != nil {
		t.Fatalf("close opened session: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("retry opening shutdown: %v", err)
	}
}
