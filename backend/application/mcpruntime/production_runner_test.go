// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProductionSessionFactoryTakesOwnershipOfPartialProcessTreeOnBuilderError(t *testing.T) {
	configured := trustedProductionRunnerForTest(t)
	root := safeWorkdirTestRoot(t)
	handle := &recordingProductionProcessTree{
		kind:               ProductionProcessTreeIsolationCgroup,
		controlRootAllowed: true,
	}
	builder := &capturingProductionStdioBuilder{
		configured:  configured,
		processTree: handle,
		buildErr:    errors.New("builder failed after starting process tree"),
	}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy: testPolicy(root), ProductionStdioBuilder: builder,
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}

	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio, Config: `{"command":"/bin/echo"}`, Auth: `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("partial builder error was accepted: %v", err)
	}
	if handle.terminateCalls != 1 {
		t.Fatalf("partial process tree ownership was lost: terminate calls=%d", handle.terminateCalls)
	}
	if _, statErr := os.Stat(builder.receivedWorkdir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("workdir was not cleaned after confirmed termination: %v", statErr)
	}
	if err := factory.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown retained terminated partial process tree: %v", err)
	}
}

func TestProductionSessionFactoryTakesOwnershipOfPartialClientOnBuilderError(t *testing.T) {
	configured := trustedProductionRunnerForTest(t)
	client := &fakeProtocolClient{}
	handle := &recordingProductionProcessTree{
		kind:               ProductionProcessTreeIsolationNamespace,
		controlRootAllowed: true,
	}
	builder := &capturingProductionStdioBuilder{
		configured: configured, client: client, processTree: handle,
		buildErr: errors.New("builder returned a partial client"),
	}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy: testPolicy(safeWorkdirTestRoot(t)), ProductionStdioBuilder: builder,
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}

	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio, Config: `{"command":"/bin/echo"}`, Auth: `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("partial client builder error was accepted: %v", err)
	}
	if client.closeCalls.Load() != 1 || handle.terminateCalls != 1 {
		t.Fatalf("partial client resources were not closed: client=%d tree=%d",
			client.closeCalls.Load(), handle.terminateCalls)
	}
	if err := factory.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown retained closed partial client: %v", err)
	}
}

func TestProductionSessionFactoryBuilderErrorWithoutResourcesCleansWorkdir(t *testing.T) {
	configured := trustedProductionRunnerForTest(t)
	builder := &capturingProductionStdioBuilder{
		configured: configured,
		buildErr:   errors.New("builder failed before starting resources"),
	}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy: testPolicy(safeWorkdirTestRoot(t)), ProductionStdioBuilder: builder,
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}

	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio, Config: `{"command":"/bin/echo"}`, Auth: `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("empty builder error was accepted: %v", err)
	}
	if _, statErr := os.Stat(builder.receivedWorkdir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("resource-free builder failure leaked workdir: %v", statErr)
	}
	if err := factory.Shutdown(context.Background()); err != nil {
		t.Fatalf("resource-free builder failure blocked shutdown: %v", err)
	}
}

func TestProductionSessionFactoryRetriesPartialBuilderTerminationDuringShutdown(t *testing.T) {
	configured := trustedProductionRunnerForTest(t)
	var allowTermination atomic.Bool
	handle := &recordingProductionProcessTree{
		kind:               ProductionProcessTreeIsolationDedicatedUID,
		controlRootAllowed: true,
		terminate: func(context.Context) error {
			if !allowTermination.Load() {
				return errors.New("termination not yet confirmed")
			}
			return nil
		},
	}
	builder := &capturingProductionStdioBuilder{
		configured: configured, processTree: handle,
		buildErr: errors.New("builder failed after process start"),
	}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy: testPolicy(safeWorkdirTestRoot(t)), ProductionStdioBuilder: builder,
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}
	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio, Config: `{"command":"/bin/echo"}`, Auth: `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("partial builder error was accepted: %v", err)
	}

	shortCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = factory.Shutdown(shortCtx)
	cancel()
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("shutdown discarded unterminated partial result: %v", err)
	}
	if _, statErr := os.Stat(builder.receivedWorkdir); statErr != nil {
		t.Fatalf("workdir cleaned before termination was safe: %v", statErr)
	}

	allowTermination.Store(true)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown retry did not drain partial result: %v", err)
	}
	if _, statErr := os.Stat(builder.receivedWorkdir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("terminated partial result workdir remains: %v", statErr)
	}
}

func TestProductionSessionBuilderReceivesOnlyValidatedCanonicalRunnerPath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("production runner execution is intentionally unavailable to root")
	}
	configured, err := filepath.EvalSymlinks("/bin/echo")
	if err != nil {
		t.Fatalf("canonicalize trusted runner: %v", err)
	}
	client := &fakeProtocolClient{
		initializeResult: initializeResultWithCapabilities(t, true, false, false),
	}
	builder := &capturingProductionStdioBuilder{
		configured: configured,
		client:     client,
		processTree: &recordingProductionProcessTree{
			kind:               ProductionProcessTreeIsolationCgroup,
			controlRootAllowed: true,
		},
	}
	root := safeWorkdirTestRoot(t)
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:                 testPolicy(root),
		ProductionStdioBuilder: builder,
	})
	if err != nil {
		t.Fatalf("new production session factory: %v", err)
	}
	session, err := factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo","args":["hello"]}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open production runner session: %v", err)
	}
	defer session.Close()

	if builder.received != configured {
		t.Fatalf("production builder received %q, want canonical %q", builder.received, configured)
	}
	if real, err := filepath.EvalSymlinks(builder.received); err != nil || real != builder.received {
		t.Fatalf("production builder received non-canonical path %q: real=%q err=%v", builder.received, real, err)
	}
}

func TestProductionSessionFactoryRejectsRunnerThatCanAccessControlRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("production runner execution is intentionally unavailable to root")
	}
	configured, err := filepath.EvalSymlinks("/bin/echo")
	if err != nil {
		t.Fatalf("canonicalize trusted runner: %v", err)
	}
	handle := &recordingProductionProcessTree{kind: ProductionProcessTreeIsolationNamespace}
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new external manager: %v", err)
	}
	defer manager.Close()
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:         testPolicy(root),
		WorkdirManager: manager,
		ProductionStdioBuilder: &capturingProductionStdioBuilder{
			configured: configured,
			client: &fakeProtocolClient{
				initializeResult: initializeResultWithCapabilities(t, true, false, false),
			},
			processTree: handle,
		},
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}
	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("control-root-accessible runner was accepted: %v", err)
	}
	if handle.controlRoot == "" || handle.terminateCalls == 0 {
		t.Fatalf("rejected runner was not terminated: root=%q calls=%d", handle.controlRoot, handle.terminateCalls)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("terminated rejected runner left orphan: %v", err)
	}
}

func TestProductionSessionFactoryRejectsRunnerWithoutIsolationHandle(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("production runner execution is intentionally unavailable to root")
	}
	configured, err := filepath.EvalSymlinks("/bin/echo")
	if err != nil {
		t.Fatalf("canonicalize trusted runner: %v", err)
	}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy: testPolicy(safeWorkdirTestRoot(t)),
		ProductionStdioBuilder: &capturingProductionStdioBuilder{
			configured: configured,
			client: &fakeProtocolClient{
				initializeResult: initializeResultWithCapabilities(t, true, false, false),
			},
		},
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}
	_, err = factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("missing isolation handle must fail closed: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := factory.Shutdown(shutdownCtx); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("missing handle orphan was silently discarded: %v", err)
	}
}

func TestProductionSessionCleanupWaitsForIsolatedProcessTree(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("production runner execution is intentionally unavailable to root")
	}
	configured, err := filepath.EvalSymlinks("/bin/echo")
	if err != nil {
		t.Fatalf("canonicalize trusted runner: %v", err)
	}
	root := safeWorkdirTestRoot(t)
	client := &fakeProtocolClient{initializeResult: initializeResultWithCapabilities(t, true, false, false)}
	builder := &rebuildingProductionStdioBuilder{configured: configured, client: client}
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:                 testPolicy(root),
		ProductionStdioBuilder: builder,
	})
	if err != nil {
		t.Fatalf("new production factory: %v", err)
	}
	defer factory.Close()
	session, err := factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open production session: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close production session: %v", err)
	}
	if !builder.processTree.sawWorkdirBeforeTerminate {
		t.Fatal("workdir cleanup ran before process-tree termination")
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(builder.workdir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminated process tree rebuilt cleaned workdir: %v", err)
	}
}

type capturingProductionStdioBuilder struct {
	configured      string
	received        string
	receivedWorkdir string
	client          ProtocolClient
	processTree     ProductionProcessTreeHandle
	buildErr        error
}

func (b *capturingProductionStdioBuilder) TrustedProductionRunnerExecutable() string {
	return b.configured
}

func (b *capturingProductionStdioBuilder) BuildWithTrustedProductionRunner(
	_ context.Context,
	canonicalRunnerExecutable string,
	connection ResolvedConnection,
) (*ProductionStdioBuildResult, error) {
	b.received = canonicalRunnerExecutable
	if connection.Stdio != nil {
		b.receivedWorkdir = connection.Stdio.WorkingDir
	}
	return &ProductionStdioBuildResult{Client: b.client, ProcessTree: b.processTree}, b.buildErr
}

func trustedProductionRunnerForTest(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("production runner execution is intentionally unavailable to root")
	}
	configured, err := filepath.EvalSymlinks("/bin/echo")
	if err != nil {
		t.Fatalf("canonicalize trusted runner: %v", err)
	}
	return configured
}

type recordingProductionProcessTree struct {
	kind                      ProductionProcessTreeIsolation
	terminate                 func(context.Context) error
	sawWorkdirBeforeTerminate bool
	controlRootAllowed        bool
	controlRoot               string
	terminateCalls            int
}

func (h *recordingProductionProcessTree) Isolation() ProductionProcessTreeIsolation {
	return h.kind
}

func (h *recordingProductionProcessTree) ControlRootInaccessible(canonicalControlRoot string) bool {
	h.controlRoot = canonicalControlRoot
	return h.controlRootAllowed
}

func (h *recordingProductionProcessTree) TerminateAndWait(ctx context.Context) error {
	h.terminateCalls++
	if h.terminate != nil {
		return h.terminate(ctx)
	}
	return nil
}

type rebuildingProductionStdioBuilder struct {
	configured  string
	client      ProtocolClient
	workdir     string
	processTree *recordingProductionProcessTree
}

func (b *rebuildingProductionStdioBuilder) TrustedProductionRunnerExecutable() string {
	return b.configured
}

func (b *rebuildingProductionStdioBuilder) BuildWithTrustedProductionRunner(
	_ context.Context,
	_ string,
	connection ResolvedConnection,
) (*ProductionStdioBuildResult, error) {
	b.workdir = connection.Stdio.WorkingDir
	stop := make(chan struct{})
	done := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = os.MkdirAll(b.workdir, 0o700)
				_ = os.WriteFile(filepath.Join(b.workdir, "process-tree-heartbeat"), []byte("active"), 0o600)
			}
		}
	}()
	b.processTree = &recordingProductionProcessTree{
		kind:               ProductionProcessTreeIsolationCgroup,
		controlRootAllowed: true,
		terminate: func(context.Context) error {
			if _, err := os.Stat(b.workdir); err == nil {
				b.processTree.sawWorkdirBeforeTerminate = true
			}
			stopOnce.Do(func() { close(stop) })
			<-done
			return nil
		},
	}
	return &ProductionStdioBuildResult{Client: b.client, ProcessTree: b.processTree}, nil
}
