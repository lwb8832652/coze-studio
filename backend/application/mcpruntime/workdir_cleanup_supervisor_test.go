// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProductionSessionFactorySupervisesInitializeFailureCleanup(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager := newSupervisorTestWorkdirManager(t, root)
	defer manager.Close()
	client := &fakeProtocolClient{initializeErr: errors.New("provider secret")}
	var workingDir string
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:            testPolicy(root),
		ExecutionMode:     NewStdioExecutionMode("debug", true),
		WorkdirManager:    manager,
		CleanupSupervisor: supervisorTestOptions(32),
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			workingDir = connection.Stdio.WorkingDir
			for index := 0; index < 12; index++ {
				if err := os.WriteFile(filepath.Join(workingDir, "entry-"+string(rune('a'+index))), []byte("bounded"), 0o600); err != nil {
					return nil, err
				}
			}
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
		t.Fatalf("open = %v, want unavailable", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("drain cleanup supervisor: %v", err)
	}
	assertSafeWorkdirCleanupComplete(t, root, workingDir)
}

func TestProductionSessionFactoryStartupScanRecoversExhaustedQuarantine(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager := newSupervisorTestWorkdirManager(t, root)
	defer manager.Close()
	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: "coze-mcp-management-"})
	if err != nil {
		t.Fatalf("create leftover: %v", err)
	}
	for index := 0; index < 12; index++ {
		if err := os.WriteFile(filepath.Join(workdir.Path, "entry-"+string(rune('a'+index))), []byte("bounded"), 0o600); err != nil {
			t.Fatalf("populate leftover: %v", err)
		}
	}
	if err := manager.Delete(context.Background(), workdir.RelativePath); !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
		t.Fatalf("seed bounded leftover = %v", err)
	}

	first, err := newSupervisorOnlyFactory(root, manager, supervisorTestOptions(1))
	if err != nil {
		t.Fatalf("new first factory: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := first.Shutdown(shutdownCtx); err != nil {
		cancel()
		t.Fatalf("shutdown exhausted supervisor: %v", err)
	}
	cancel()
	entries, err := os.ReadDir(filepath.Join(root, safeWorkdirTrashDirectory))
	if err != nil || len(entries) == 0 {
		t.Fatalf("exhausted supervisor did not preserve quarantine: entries=%d err=%v", len(entries), err)
	}

	second, err := newSupervisorOnlyFactory(root, manager, supervisorTestOptions(32))
	if err != nil {
		t.Fatalf("new recovery factory: %v", err)
	}
	shutdownCtx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := second.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown recovery supervisor: %v", err)
	}
	assertSafeWorkdirCleanupComplete(t, root, workdir.Path)
}

func TestProductionSessionFactoryRecoversCrashBeforeDeleteFromIdentityMarker(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	crashedManager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new crashed manager: %v", err)
	}
	crashed, err := crashedManager.Create(context.Background(), SafeWorkdirCreateRequest{
		Prefix: managementSafeWorkdirPrefix,
	})
	if err != nil {
		t.Fatalf("create crash-before-delete directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(crashed.Path, "crash-data"), []byte("bounded"), 0o600); err != nil {
		t.Fatalf("write crash data: %v", err)
	}
	if err := crashedManager.Close(); err != nil {
		t.Fatalf("simulate crashed manager close: %v", err)
	}

	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(context.Context, ResolvedConnection) (ProtocolClient, error) {
			return &fakeProtocolClient{}, nil
		}),
		CleanupSupervisor: supervisorTestOptions(32),
	})
	if err != nil {
		t.Fatalf("new recovering factory: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown recovering factory: %v", err)
	}
	if _, err := os.Stat(crashed.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("crash-before-delete directory remains: %v", err)
	}
}

func TestProductionSessionFactoryIsolatesInvalidRecoveryWithoutBlockingOpen(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	first, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new first manager: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first manager: %v", err)
	}
	damaged := filepath.Join(root, managementSafeWorkdirPrefix+"invalid-marker")
	if err := os.Mkdir(damaged, 0o700); err != nil {
		t.Fatalf("create damaged workdir: %v", err)
	}
	client := &fakeProtocolClient{initializeResult: initializeResultWithCapabilities(t, true, false, false)}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(context.Context, ResolvedConnection) (ProtocolClient, error) {
			return client, nil
		}),
		CleanupSupervisor: supervisorTestOptions(4),
	})
	if err != nil {
		t.Fatalf("new recovery factory: %v", err)
	}
	session, err := factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("isolated invalid marker blocked all opens: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	if _, err := os.Stat(damaged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid directory was not moved beneath rejected control area: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func newSupervisorTestWorkdirManager(t *testing.T, root string) *SafeWorkdirManager {
	t.Helper()
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{
		Root: root,
		DeleteLimits: SafeWorkdirDeleteLimits{
			MaxEntries: 1,
			MaxBytes:   512,
			MaxDepth:   64,
		},
	})
	if err != nil {
		t.Fatalf("new workdir manager: %v", err)
	}
	return manager
}

func supervisorTestOptions(maxAttempts int) WorkdirCleanupSupervisorOptions {
	return WorkdirCleanupSupervisorOptions{
		QueueSize:      8,
		MaxConcurrent:  2,
		MaxAttempts:    maxAttempts,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
	}
}

func newSupervisorOnlyFactory(
	root string,
	manager *SafeWorkdirManager,
	options WorkdirCleanupSupervisorOptions,
) (*ProductionSessionFactory, error) {
	return NewProductionSessionFactory(FactoryOptions{
		Policy:            testPolicy(root),
		ExecutionMode:     NewStdioExecutionMode("debug", true),
		WorkdirManager:    manager,
		CleanupSupervisor: options,
		ClientBuilder: ProtocolClientBuilderFunc(func(context.Context, ResolvedConnection) (ProtocolClient, error) {
			return &fakeProtocolClient{}, nil
		}),
	})
}
