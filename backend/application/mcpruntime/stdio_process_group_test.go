// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	stdioProcessGroupHelperMode      = "COZE_MCP_PROCESS_GROUP_HELPER"
	stdioProcessGroupHelperHeartbeat = "COZE_MCP_PROCESS_GROUP_HEARTBEAT"
	stdioProcessGroupHelperPID       = "COZE_MCP_PROCESS_GROUP_CHILD_PID"
)

func TestSafeStdioDebugProcessGroupTerminatesChildBeforeCleanup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}
	root := safeWorkdirTestRoot(t)
	policy := testPolicy(root)
	policy.StdioAllowedCommands = []string{executable}
	policy.StdioCommandRules = []StdioCommandRule{{Command: executable, AllowAnyArgs: true}}
	commandPolicy, err := policy.commandPolicy()
	if err != nil {
		t.Fatalf("command policy: %v", err)
	}
	policy.StdioCommandPolicy = commandPolicy
	var workdir string
	var childPIDFile string
	var safeTransport *safeStdioTransport
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        policy,
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			workdir = connection.Stdio.WorkingDir
			childPIDFile = filepath.Join(root, "child.pid")
			transport, err := NewSafeStdioTransport(StdioTransportOptions{
				Command: executable,
				Args:    []string{"-test.run=^TestSafeStdioProcessGroupHelperProcess$"},
				Env: []string{
					stdioProcessGroupHelperMode + "=parent",
					stdioProcessGroupHelperHeartbeat + "=" + filepath.Join(workdir, "heartbeat"),
					stdioProcessGroupHelperPID + "=" + childPIDFile,
				},
				WorkingDir:            workdir,
				CommandPolicy:         commandPolicy,
				ExecutionMode:         NewStdioExecutionMode("debug", true),
				ProcessTerminateGrace: 50 * time.Millisecond,
				ProcessKillWait:       time.Second,
			})
			if err != nil {
				return nil, err
			}
			safeTransport = transport.(*safeStdioTransport)
			return &debugProcessGroupProtocolClient{
				transport:        transport,
				initializeResult: initializeResultWithCapabilities(t, true, false, false),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	t.Cleanup(func() {
		if payload, readErr := os.ReadFile(childPIDFile); readErr == nil {
			if pid, parseErr := strconv.Atoi(string(payload)); parseErr == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		_ = factory.Close()
	})
	config, _ := json.Marshal(map[string]any{
		"command": executable,
		"args":    []string{"-test.run=^TestSafeStdioProcessGroupHelperProcess$"},
	})
	session, err := factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio,
		Config:     string(config),
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open debug process group: %v", err)
	}
	waitForTestFile(t, childPIDFile, 2*time.Second)
	waitForTestFile(t, filepath.Join(workdir, "heartbeat"), 2*time.Second)
	if err := session.Close(); err != nil {
		safeTransport.terminateMu.Lock()
		inspectionFailure := safeTransport.inspectionFailure
		safeTransport.terminateMu.Unlock()
		t.Fatalf("close debug process group: %v inspection=%v", err, inspectionFailure)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(workdir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("child rebuilt workdir after successful cleanup: %v", err)
	}
}

func TestSafeStdioDebugProcessGroupDoesNotClaimSetsidChildTermination(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}
	root := safeWorkdirTestRoot(t)
	policy := testPolicy(root)
	policy.StdioAllowedCommands = []string{executable}
	policy.StdioCommandRules = []StdioCommandRule{{Command: executable, AllowAnyArgs: true}}
	commandPolicy, err := policy.commandPolicy()
	if err != nil {
		t.Fatalf("command policy: %v", err)
	}
	policy.StdioCommandPolicy = commandPolicy
	var workdir string
	var childPIDFile string
	var safeTransport *safeStdioTransport
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        policy,
		ExecutionMode: NewStdioExecutionMode("debug", true),
		CleanupSupervisor: WorkdirCleanupSupervisorOptions{
			QueueSize: 8, MaxConcurrent: 1, MaxAttempts: 1,
			InitialBackoff: 10 * time.Millisecond, MaxBackoff: 10 * time.Millisecond,
		},
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			workdir = connection.Stdio.WorkingDir
			childPIDFile = filepath.Join(root, "setsid-child.pid")
			transport, err := NewSafeStdioTransport(StdioTransportOptions{
				Command: executable,
				Args:    []string{"-test.run=^TestSafeStdioProcessGroupHelperProcess$"},
				Env: []string{
					stdioProcessGroupHelperMode + "=parent-setsid",
					stdioProcessGroupHelperHeartbeat + "=" + filepath.Join(workdir, "heartbeat"),
					stdioProcessGroupHelperPID + "=" + childPIDFile,
				},
				WorkingDir:            workdir,
				CommandPolicy:         commandPolicy,
				ExecutionMode:         NewStdioExecutionMode("debug", true),
				ProcessTerminateGrace: 50 * time.Millisecond,
				ProcessKillWait:       time.Second,
			})
			if err != nil {
				return nil, err
			}
			safeTransport = transport.(*safeStdioTransport)
			return &debugProcessGroupProtocolClient{
				transport: transport, initializeResult: initializeResultWithCapabilities(t, true, false, false),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	config, _ := json.Marshal(map[string]any{
		"command": executable,
		"args":    []string{"-test.run=^TestSafeStdioProcessGroupHelperProcess$"},
	})
	session, err := factory.Open(context.Background(), Connection{
		ServerType: ServerTypeStdio, Config: string(config), Auth: `{}`,
	})
	if err != nil {
		t.Fatalf("open setsid process: %v", err)
	}
	waitForTestFile(t, childPIDFile, 2*time.Second)
	waitForTestFile(t, filepath.Join(workdir, "heartbeat"), 2*time.Second)
	if err := session.Close(); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("setsid child was incorrectly reported fully terminated: %v", err)
	}
	safeTransport.terminateMu.Lock()
	terminationClaimed := safeTransport.terminated
	safeTransport.terminateMu.Unlock()
	if terminationClaimed {
		t.Fatal("best-effort debug process group claimed production-safe termination")
	}
	if _, err := os.Stat(workdir); err != nil {
		t.Fatalf("workdir cleaned while termination was unconfirmed: %v", err)
	}
	relative, err := factory.workdirManager.RelativePath(workdir)
	if err != nil {
		t.Fatalf("relative workdir: %v", err)
	}
	marker := filepath.Join(
		factory.workdirManager.ControlRoot(),
		safeWorkdirLeaseDirectory,
		safeWorkdirIdentityMarkerFilename(relative),
	)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("control lease released while termination was unconfirmed: %v", err)
	}
	factory.lifecycleMu.Lock()
	active := factory.active
	resources := make([]*managedSessionResource, 0, len(factory.resources))
	for _, resource := range factory.resources {
		resources = append(resources, resource)
	}
	factory.lifecycleMu.Unlock()
	if active != 1 || len(resources) != 1 || !resources[0].shouldRetry() {
		t.Fatalf("uncertain termination missing orphan retry state: active=%d resources=%d", active, len(resources))
	}
	if payload, readErr := os.ReadFile(childPIDFile); readErr == nil {
		if pid, parseErr := strconv.Atoi(string(payload)); parseErr == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := factory.resourceSupervisor.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("drain test orphan supervisor: %v", err)
	}
	if factory.cleanupSupervisor != nil {
		if err := factory.cleanupSupervisor.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("drain test cleanup supervisor: %v", err)
		}
	}
	if err := factory.workdirManager.Close(); err != nil {
		t.Fatalf("close test manager: %v", err)
	}
}

func TestSafeStdioProcessGroupHelperProcess(t *testing.T) {
	switch os.Getenv(stdioProcessGroupHelperMode) {
	case "parent", "parent-setsid":
		command := exec.Command(os.Args[0], "-test.run=^TestSafeStdioProcessGroupHelperProcess$")
		if os.Getenv(stdioProcessGroupHelperMode) == "parent-setsid" {
			command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		}
		command.Env = []string{
			stdioProcessGroupHelperMode + "=child",
			stdioProcessGroupHelperHeartbeat + "=" + os.Getenv(stdioProcessGroupHelperHeartbeat),
			stdioProcessGroupHelperPID + "=" + os.Getenv(stdioProcessGroupHelperPID),
		}
		if err := command.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Getenv(stdioProcessGroupHelperPID), []byte(strconv.Itoa(command.Process.Pid)), 0o600); err != nil {
			os.Exit(4)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "child":
		signal.Ignore(syscall.SIGTERM)
		heartbeat := os.Getenv(stdioProcessGroupHelperHeartbeat)
		for {
			_ = os.MkdirAll(filepath.Dir(heartbeat), 0o700)
			_ = os.WriteFile(heartbeat, []byte("alive"), 0o600)
			time.Sleep(5 * time.Millisecond)
		}
	}
}

type debugProcessGroupProtocolClient struct {
	transport        mcptransport.Interface
	initializeResult *mcpsdk.InitializeResult
}

func (c *debugProcessGroupProtocolClient) Start(ctx context.Context) error {
	return c.transport.Start(ctx)
}
func (c *debugProcessGroupProtocolClient) Initialize(context.Context, mcpsdk.InitializeRequest) (*mcpsdk.InitializeResult, error) {
	return c.initializeResult, nil
}
func (c *debugProcessGroupProtocolClient) ListToolsByPage(context.Context, mcpsdk.ListToolsRequest) (*mcpsdk.ListToolsResult, error) {
	return nil, nil
}
func (c *debugProcessGroupProtocolClient) ListResourcesByPage(context.Context, mcpsdk.ListResourcesRequest) (*mcpsdk.ListResourcesResult, error) {
	return nil, nil
}
func (c *debugProcessGroupProtocolClient) ListPromptsByPage(context.Context, mcpsdk.ListPromptsRequest) (*mcpsdk.ListPromptsResult, error) {
	return nil, nil
}
func (c *debugProcessGroupProtocolClient) CallTool(context.Context, mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return nil, nil
}
func (c *debugProcessGroupProtocolClient) Close() error { return c.transport.Close() }

func waitForTestFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", filepath.Base(path))
}
