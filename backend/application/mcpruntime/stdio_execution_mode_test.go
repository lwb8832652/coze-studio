// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeStdioTransportRejectsProductionHostExecution(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	for _, test := range []struct {
		name    string
		appEnv  string
		enabled bool
	}{
		{name: "production even with switch", appEnv: "production", enabled: true},
		{name: "debug without switch", appEnv: "debug", enabled: false},
		{name: "empty environment", appEnv: "", enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewSafeStdioTransport(StdioTransportOptions{
				Command:       executable,
				Args:          []string{"-test.run=^$"},
				WorkingDir:    t.TempDir(),
				ExecutionMode: NewStdioExecutionMode(test.appEnv, test.enabled),
			})
			if !errors.Is(err, ErrStdioHostExecutionDenied) {
				t.Fatalf("host execution must fail closed, got %v", err)
			}
		})
	}
}

func TestSafeStdioTransportDebugUsesCanonicalExecutableAndNodeScript(t *testing.T) {
	root := t.TempDir()
	nodeTarget := writeCommandPolicyExecutable(t, root, "node-real", "node-v1")
	nodeLink := filepath.Join(root, "node")
	if err := os.Symlink(nodeTarget, nodeLink); err != nil {
		t.Fatalf("link node: %v", err)
	}
	scriptRoot := filepath.Join(root, "scripts")
	if err := os.Mkdir(scriptRoot, 0o700); err != nil {
		t.Fatalf("create scripts: %v", err)
	}
	scriptTarget := filepath.Join(scriptRoot, "server-real.js")
	if err := os.WriteFile(scriptTarget, []byte("server"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	scriptLink := filepath.Join(scriptRoot, "server.js")
	if err := os.Symlink(scriptTarget, scriptLink); err != nil {
		t.Fatalf("link script: %v", err)
	}
	policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{nodeLink},
		NodeScriptRoots: []string{scriptRoot},
	})
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	transport, err := NewSafeStdioTransport(StdioTransportOptions{
		Command:       nodeLink,
		Args:          []string{scriptLink, "--stdio"},
		WorkingDir:    root,
		CommandPolicy: policy,
		ExecutionMode: NewStdioExecutionMode("debug", true),
	})
	if err != nil {
		t.Fatalf("new debug transport: %v", err)
	}
	stdio := transport.(*safeStdioTransport)
	realNode, _ := filepath.EvalSymlinks(nodeLink)
	realScript, _ := filepath.EvalSymlinks(scriptLink)
	if stdio.command != realNode || len(stdio.args) == 0 || stdio.args[0] != realScript {
		t.Fatalf("transport did not retain canonical invocation: command=%q args=%#v", stdio.command, stdio.args)
	}
}

func TestTrustedProductionExecutableRejectsServiceOwnedOrWritablePath(t *testing.T) {
	serviceOwned := writeCommandPolicyExecutable(t, t.TempDir(), "runner", "runner")
	if _, err := validateTrustedProductionExecutable(serviceOwned); !errors.Is(err, ErrStdioHostExecutionDenied) {
		t.Fatalf("service-owned runner path must fail closed, got %v", err)
	}

	if os.Geteuid() != 0 {
		canonical, err := filepath.EvalSymlinks("/bin/echo")
		if err != nil {
			t.Fatalf("canonicalize trusted runner fixture: %v", err)
		}
		validated, err := validateTrustedProductionExecutable(canonical)
		if err != nil {
			t.Fatalf("immutable root-owned runner fixture was denied: %v", err)
		}
		if validated != canonical {
			t.Fatalf("validated runner = %q, want %q", validated, canonical)
		}
	}
}

func TestTrustedProductionExecutableRejectsReplaceableSymlink(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("production runner execution is intentionally unavailable to root")
	}
	trusted, err := filepath.EvalSymlinks("/bin/echo")
	if err != nil {
		t.Fatalf("canonicalize trusted target: %v", err)
	}
	root := t.TempDir()
	configured := filepath.Join(root, "production-runner")
	if err := os.Symlink(trusted, configured); err != nil {
		t.Fatalf("link trusted runner: %v", err)
	}
	if _, err := validateTrustedProductionExecutable(configured); !errors.Is(err, ErrStdioHostExecutionDenied) {
		t.Fatalf("symlink to trusted runner must be rejected before use, got %v", err)
	}
	if err := os.Remove(configured); err != nil {
		t.Fatalf("remove trusted runner link: %v", err)
	}
	attacker := writeCommandPolicyExecutable(t, root, "attacker-runner", "attacker")
	if err := os.Symlink(attacker, configured); err != nil {
		t.Fatalf("replace runner link: %v", err)
	}
	if _, err := validateTrustedProductionExecutable(configured); !errors.Is(err, ErrStdioHostExecutionDenied) {
		t.Fatalf("replaced runner symlink must remain rejected, got %v", err)
	}
}

func TestSafeStdioTransportRevalidatesExecutableAndNodeScriptBeforeStart(t *testing.T) {
	t.Run("executable replacement", func(t *testing.T) {
		root := t.TempDir()
		server := writeCommandPolicyExecutable(t, root, "server", "server-v1")
		policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
			AllowedCommands: []string{server},
			CommandRules: []StdioCommandRule{{
				Command:      server,
				AllowAnyArgs: true,
			}},
		})
		if err != nil {
			t.Fatalf("compile executable policy: %v", err)
		}
		transport, err := NewSafeStdioTransport(StdioTransportOptions{
			Command:       server,
			WorkingDir:    root,
			CommandPolicy: policy,
			ExecutionMode: NewStdioExecutionMode("debug", true),
		})
		if err != nil {
			t.Fatalf("new executable transport: %v", err)
		}
		if err := os.Remove(server); err != nil {
			t.Fatalf("remove executable: %v", err)
		}
		writeCommandPolicyExecutable(t, root, "server", "server-v2-replacement")
		if err := transport.Start(context.Background()); !errors.Is(err, ErrStdioCommandDenied) {
			t.Fatalf("replaced executable reached exec: %v", err)
		}
	})

	t.Run("node script replacement", func(t *testing.T) {
		root := t.TempDir()
		node := writeCommandPolicyExecutable(t, root, "node", "node-v1")
		scriptRoot := filepath.Join(root, "scripts")
		if err := os.Mkdir(scriptRoot, 0o700); err != nil {
			t.Fatalf("create script root: %v", err)
		}
		script := filepath.Join(scriptRoot, "server.js")
		if err := os.WriteFile(script, []byte("server-v1"), 0o600); err != nil {
			t.Fatalf("write script: %v", err)
		}
		policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
			AllowedCommands: []string{node},
			NodeScriptRoots: []string{scriptRoot},
		})
		if err != nil {
			t.Fatalf("compile node policy: %v", err)
		}
		transport, err := NewSafeStdioTransport(StdioTransportOptions{
			Command:       node,
			Args:          []string{script},
			WorkingDir:    root,
			CommandPolicy: policy,
			ExecutionMode: NewStdioExecutionMode("debug", true),
		})
		if err != nil {
			t.Fatalf("new node transport: %v", err)
		}
		if err := os.Remove(script); err != nil {
			t.Fatalf("remove script: %v", err)
		}
		if err := os.WriteFile(script, []byte("server-v2-replacement"), 0o600); err != nil {
			t.Fatalf("replace script: %v", err)
		}
		if err := transport.Start(context.Background()); !errors.Is(err, ErrStdioCommandDenied) {
			t.Fatalf("replaced node script reached exec: %v", err)
		}
	})
}
