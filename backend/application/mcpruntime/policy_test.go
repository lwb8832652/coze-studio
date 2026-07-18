// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeServerTypeAliasesAndFailsClosed(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"stdio":           ServerTypeStdio,
		"sse":             ServerTypeSSE,
		"streamable_http": ServerTypeStreamableHTTP,
		"streamable-http": ServerTypeStreamableHTTP,
		"http":            ServerTypeStreamableHTTP,
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeServerType(input)
			if err != nil {
				t.Fatalf("normalize server type: %v", err)
			}
			if got != want {
				t.Fatalf("normalized type = %q, want %q", got, want)
			}
		})
	}

	if _, err := NormalizeServerType("websocket"); !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("unknown server type must fail closed, got %v", err)
	}
}

func TestPolicyParseRemoteEnforcesNetworkAndHeaderBoundary(t *testing.T) {
	t.Parallel()

	policy := testPolicy(t.TempDir())
	tests := []struct {
		name       string
		serverType string
		config     string
	}{
		{
			name:       "ssrf host outside allowlist",
			serverType: "sse",
			config:     `{"url":"https://169.254.169.254/latest/meta-data"}`,
		},
		{
			name:       "insecure remote http",
			serverType: "http",
			config:     `{"url":"http://mcp.example.com/rpc"}`,
		},
		{
			name:       "userinfo",
			serverType: "sse",
			config:     `{"url":"https://user:pass@mcp.example.com/events"}`,
		},
		{
			name:       "fragment",
			serverType: "sse",
			config:     `{"url":"https://mcp.example.com/events#secret"}`,
		},
		{
			name:       "header crlf",
			serverType: "sse",
			config:     `{"url":"https://mcp.example.com/events","headers":{"X-Test":"ok\r\nX-Evil: yes"}}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := policy.ParseRemote(Connection{
				ServerType: tt.serverType,
				Config:     tt.config,
				Auth:       `{"token":"raw-auth-secret"}`,
			})
			if !errors.Is(err, ErrInvalidConnection) {
				t.Fatalf("expected invalid connection, got %v", err)
			}
			if strings.Contains(err.Error(), "raw-auth-secret") {
				t.Fatalf("error leaked raw auth: %v", err)
			}
		})
	}
}

func TestPolicyParseRemoteAllowsExplicitLocalHTTPAndProjectsSecrets(t *testing.T) {
	enableLocalMCPHTTP(t)

	policy := testPolicy(t.TempDir())
	policy.AllowInsecureHTTP = true
	resolved, err := policy.ParseRemote(Connection{
		ServerType: "streamable-http",
		Config: `{
			"url":"http://localhost:8080/mcp",
			"headers":{"X-Static":"coze"},
			"auth_headers":{"Authorization":"credentials.token"}
		}`,
		Auth: `{"credentials":{"token":"Bearer projected-secret"}}`,
	})
	if err != nil {
		t.Fatalf("parse remote connection: %v", err)
	}
	if resolved.ServerType != ServerTypeStreamableHTTP {
		t.Fatalf("transport = %q", resolved.ServerType)
	}
	if got := resolved.Headers["Authorization"]; got != "Bearer projected-secret" {
		t.Fatalf("projected auth header = %q", got)
	}
	if got := resolved.Headers["X-Static"]; got != "coze" {
		t.Fatalf("static header = %q", got)
	}
	if resolved.HTTPTimeout != 2*time.Second {
		t.Fatalf("http timeout = %s", resolved.HTTPTimeout)
	}
}

func TestPolicyParseRemoteNeverLeaksSecretOnProjectionFailure(t *testing.T) {
	t.Parallel()

	policy := testPolicy(t.TempDir())
	_, err := policy.ParseRemote(Connection{
		ServerType: "sse",
		Config:     `{"url":"https://mcp.example.com/events","auth_headers":{"Authorization":"missing.path"}}`,
		Auth:       `{"token":"must-never-appear"}`,
	})
	if !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("expected invalid connection, got %v", err)
	}
	if strings.Contains(err.Error(), "must-never-appear") {
		t.Fatalf("projection error leaked secret: %v", err)
	}
}

func TestPolicyParseStdioEnforcesExactCommandEnvAndOwnedWorkingDir(t *testing.T) {
	t.Parallel()

	policy := testPolicy(t.TempDir())
	tests := []struct {
		name   string
		config string
	}{
		{name: "command with args is not exact", config: `{"command":"/usr/bin/node --eval","args":[]}`},
		{name: "shell is denied", config: `{"command":"/bin/sh","args":["-c","id"]}`},
		{name: "npx needs explicit allowlist", config: `{"command":"npx","args":["server"]}`},
		{name: "uvx needs explicit allowlist", config: `{"command":"uvx","args":["server"]}`},
		{name: "env outside allowlist", config: `{"command":"/usr/bin/node","env":{"LD_PRELOAD":"evil"}}`},
		{name: "user cwd is rejected", config: `{"command":"/usr/bin/node","cwd":"/tmp/user-owned"}`},
		{name: "args exceed count", config: `{"command":"/usr/bin/node","args":["1","2","3"]}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := policy.ParseStdio(Connection{ServerType: "stdio", Config: tt.config, Auth: `{}`})
			if !errors.Is(err, ErrPolicyDenied) && !errors.Is(err, ErrInvalidConnection) {
				t.Fatalf("expected fail-closed stdio config, got %v", err)
			}
		})
	}
}

func TestPolicyParseStdioProjectsAuthAndAllowsExplicitNpx(t *testing.T) {
	t.Parallel()

	policy := testPolicy(t.TempDir())
	policy.StdioAllowedCommands = append(policy.StdioAllowedCommands, "npx")
	policy.StdioAllowedNpxPackages = []string{"server"}
	resolved, err := policy.ParseStdio(Connection{
		ServerType: "stdio",
		Config:     `{"command":"npx","args":["server"],"auth_env":{"MCP_TOKEN":"credentials.token"}}`,
		Auth:       `{"credentials":{"token":"projected-secret"}}`,
	})
	if err != nil {
		t.Fatalf("parse stdio connection: %v", err)
	}
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		t.Fatalf("resolve npx fixture: %v", err)
	}
	realNpxPath, err := filepath.EvalSymlinks(npxPath)
	if err != nil {
		t.Fatalf("resolve real npx fixture: %v", err)
	}
	if resolved.Command != realNpxPath || !reflect.DeepEqual(resolved.Args, []string{"server"}) {
		t.Fatalf("unexpected command: %#v", resolved)
	}
	if got := resolved.Env["MCP_TOKEN"]; got != "projected-secret" {
		t.Fatalf("projected env = %q", got)
	}
}

func TestNewCommandContextDoesNotInvokeShellOrInheritEnvironment(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	cmd, err := NewCommandContext(context.Background(), StdioConfig{
		Command:    "/bin/echo",
		Args:       []string{"hello; touch should-not-run"},
		Env:        map[string]string{"MCP_TOKEN": "secret"},
		WorkingDir: workingDir,
	})
	if err != nil {
		t.Fatalf("new command: %v", err)
	}
	if cmd.Path != "/bin/echo" {
		t.Fatalf("command path = %q", cmd.Path)
	}
	if !reflect.DeepEqual(cmd.Args, []string{"/bin/echo", "hello; touch should-not-run"}) {
		t.Fatalf("command args = %#v", cmd.Args)
	}
	if cmd.Dir != filepath.Clean(workingDir) {
		t.Fatalf("command dir = %q", cmd.Dir)
	}
	if !reflect.DeepEqual(cmd.Env, []string{"MCP_TOKEN=secret", "PATH=/bin"}) {
		t.Fatalf("command env = %#v", cmd.Env)
	}
}

func TestNewCommandContextRejectsConnectionProvidedPATH(t *testing.T) {
	t.Parallel()

	_, err := NewCommandContext(context.Background(), StdioConfig{
		Command:    "/bin/echo",
		Env:        map[string]string{"PATH": "/tmp/tenant-controlled"},
		WorkingDir: t.TempDir(),
	})
	if !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("connection-provided PATH must fail closed, got %v", err)
	}
}

func testPolicy(workdirRoot string) Policy {
	return Policy{
		RemoteEnabled:         true,
		RemoteAllowedHosts:    []string{"mcp.example.com", "localhost:8080"},
		RemoteMaxConfigBytes:  4096,
		RemoteMaxHeaders:      4,
		RemoteMaxHeaderBytes:  1024,
		HTTPTimeout:           2 * time.Second,
		StdioEnabled:          true,
		StdioWorkdirRoot:      workdirRoot,
		StdioAllowedCommands:  []string{"/bin/echo"},
		StdioCommandRules:     []StdioCommandRule{{Command: "/bin/echo", AllowAnyArgs: true}},
		StdioAllowedEnvKeys:   []string{"MCP_TOKEN"},
		StdioMaxConfigBytes:   4096,
		StdioMaxArgs:          2,
		StdioMaxArgBytes:      128,
		StdioMaxEnvVars:       2,
		StdioMaxEnvValueBytes: 128,
		RejectUserWorkingDir:  true,
	}
}
