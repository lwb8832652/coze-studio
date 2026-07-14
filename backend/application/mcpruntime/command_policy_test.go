// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStdioCommandPolicyAllowsExactOfficialNpxPackage(t *testing.T) {
	root := t.TempDir()
	npx := writeCommandPolicyExecutable(t, root, "npx", "npx-v1")
	policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{npx},
		NpxPackages:     []string{"@modelcontextprotocol/server-github"},
	})
	if err != nil {
		t.Fatalf("compile command policy: %v", err)
	}

	resolved, err := policy.Resolve(
		npx,
		[]string{"-y", "@modelcontextprotocol/server-github", "--readonly"},
		root,
	)
	if err != nil {
		t.Fatalf("resolve official package: %v", err)
	}
	realPath, err := filepath.EvalSymlinks(npx)
	if err != nil {
		t.Fatalf("evaluate executable: %v", err)
	}
	if resolved != realPath || !filepath.IsAbs(resolved) {
		t.Fatalf("resolved command = %q, want real path %q", resolved, realPath)
	}
}

func TestStdioCommandPolicyRejectsNpxAndUVXTargetOverrides(t *testing.T) {
	root := t.TempDir()
	npx := writeCommandPolicyExecutable(t, root, "npx", "npx-v1")
	uvx := writeCommandPolicyExecutable(t, root, "uvx", "uvx-v1")
	policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{npx, uvx},
		NpxPackages:     []string{"@modelcontextprotocol/server-github"},
		UvxPackages:     []string{"official-uv-tool"},
	})
	if err != nil {
		t.Fatalf("compile command policy: %v", err)
	}

	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "arbitrary npx package", command: npx, args: []string{"-y", "attacker-package"}},
		{name: "npx package flag", command: npx, args: []string{"--package", "attacker-package", "@modelcontextprotocol/server-github"}},
		{name: "npx short package flag", command: npx, args: []string{"-p", "attacker-package", "@modelcontextprotocol/server-github"}},
		{name: "npx call", command: npx, args: []string{"-c", "attacker-package"}},
		{name: "npx shell", command: npx, args: []string{"--shell", "sh", "@modelcontextprotocol/server-github"}},
		{name: "npx eval", command: npx, args: []string{"--eval", "code", "@modelcontextprotocol/server-github"}},
		{name: "uvx from", command: uvx, args: []string{"--from", "attacker-package", "official-uv-tool"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, resolveErr := policy.Resolve(test.command, test.args, root)
			if !errors.Is(resolveErr, ErrStdioCommandDenied) {
				t.Fatalf("expected command denial, got %v", resolveErr)
			}
		})
	}

	withoutUVXPackages, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{uvx},
	})
	if err != nil {
		t.Fatalf("compile empty uvx policy: %v", err)
	}
	if _, err := withoutUVXPackages.Resolve(uvx, []string{"official-uv-tool"}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("uvx without package allowlist must fail closed, got %v", err)
	}
}

func TestStdioCommandPolicyRestrictsNodeToRealScriptRoots(t *testing.T) {
	root := t.TempDir()
	node := writeCommandPolicyExecutable(t, root, "node", "node-v1")
	scriptRoot := filepath.Join(root, "scripts")
	if err := os.Mkdir(scriptRoot, 0o700); err != nil {
		t.Fatalf("create script root: %v", err)
	}
	script := filepath.Join(scriptRoot, "server.js")
	if err := os.WriteFile(script, []byte("server"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outside := filepath.Join(root, "outside.js")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside script: %v", err)
	}
	policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{node},
		NodeScriptRoots: []string{scriptRoot},
	})
	if err != nil {
		t.Fatalf("compile node policy: %v", err)
	}

	if _, err := policy.Resolve(node, []string{script, "--port", "3000"}, root); err != nil {
		t.Fatalf("resolve allowed node script: %v", err)
	}
	for _, args := range [][]string{
		{"--eval", "process.exit()"},
		{"-e", "process.exit()"},
		{"--print", "process.env"},
		{"-p", "process.env"},
		{"--require", outside, script},
		{"-r", outside, script},
	} {
		if _, err := policy.Resolve(node, args, root); !errors.Is(err, ErrStdioCommandDenied) {
			t.Fatalf("node injection args %#v were not denied: %v", args, err)
		}
	}

	link := filepath.Join(scriptRoot, "linked.js")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create script symlink: %v", err)
	}
	if _, err := policy.Resolve(node, []string{link}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("node script escaping root must fail closed, got %v", err)
	}
}

func TestStdioCommandPolicyRequiresArgvPrefixForOtherExecutables(t *testing.T) {
	root := t.TempDir()
	server := writeCommandPolicyExecutable(t, root, "custom-server", "server-v1")
	withoutRule, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{server},
	})
	if err != nil {
		t.Fatalf("compile command-only policy: %v", err)
	}
	if _, err := withoutRule.Resolve(server, []string{"serve", "--stdio"}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("command without argv rule must fail closed, got %v", err)
	}

	withRule, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{server},
		CommandRules: []StdioCommandRule{{
			Command:    server,
			ArgvPrefix: []string{"serve", "--stdio"},
		}},
	})
	if err != nil {
		t.Fatalf("compile argv policy: %v", err)
	}
	if _, err := withRule.Resolve(server, []string{"serve", "--stdio", "--readonly"}, root); err != nil {
		t.Fatalf("resolve fixed argv prefix: %v", err)
	}
	if _, err := withRule.Resolve(server, []string{"shell", "--stdio"}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("argv prefix bypass must fail closed, got %v", err)
	}
}

func TestStdioCommandPolicyRejectsEmptyArgvPrefixAndRequiresExplicitArgumentMode(t *testing.T) {
	root := t.TempDir()
	server := writeCommandPolicyExecutable(t, root, "custom-server", "server-v1")

	for _, rule := range []StdioCommandRule{
		{Command: server},
		{Command: server, ArgvPrefix: []string{}},
	} {
		_, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
			AllowedCommands: []string{server},
			CommandRules:    []StdioCommandRule{rule},
		})
		if !errors.Is(err, ErrStdioCommandDenied) {
			t.Fatalf("empty or omitted argv prefix must fail closed, got %v", err)
		}
	}

	allowAny, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{server},
		CommandRules: []StdioCommandRule{{
			Command:      server,
			AllowAnyArgs: true,
		}},
	})
	if err != nil {
		t.Fatalf("compile explicit allow-any-args rule: %v", err)
	}
	if _, err := allowAny.Resolve(server, []string{"arbitrary", "arguments"}, root); err != nil {
		t.Fatalf("explicit allow-any-args rule was denied: %v", err)
	}

	exact, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{server},
		CommandRules: []StdioCommandRule{{
			Command:   server,
			ExactArgs: []string{"serve", "--stdio"},
		}},
	})
	if err != nil {
		t.Fatalf("compile exact args rule: %v", err)
	}
	if _, err := exact.Resolve(server, []string{"serve", "--stdio"}, root); err != nil {
		t.Fatalf("exact args were denied: %v", err)
	}
	if _, err := exact.Resolve(server, []string{"serve", "--stdio", "--extra"}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("exact args accepted an extra argument: %v", err)
	}
}

func TestStdioCommandPolicyCanonicalizesNodeScriptInvocation(t *testing.T) {
	root := t.TempDir()
	nodeTarget := writeCommandPolicyExecutable(t, root, "node-real", "node-v1")
	nodeLink := filepath.Join(root, "node")
	if err := os.Symlink(nodeTarget, nodeLink); err != nil {
		t.Fatalf("link node executable: %v", err)
	}
	scriptRoot := filepath.Join(root, "scripts")
	if err := os.Mkdir(scriptRoot, 0o700); err != nil {
		t.Fatalf("create script root: %v", err)
	}
	scriptTarget := filepath.Join(scriptRoot, "server-real.js")
	if err := os.WriteFile(scriptTarget, []byte("server"), 0o600); err != nil {
		t.Fatalf("write node script: %v", err)
	}
	scriptLink := filepath.Join(scriptRoot, "server.js")
	if err := os.Symlink(scriptTarget, scriptLink); err != nil {
		t.Fatalf("link node script: %v", err)
	}
	policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{nodeLink},
		NodeScriptRoots: []string{scriptRoot},
	})
	if err != nil {
		t.Fatalf("compile node policy: %v", err)
	}

	invocation, err := policy.ResolveInvocation(nodeLink, []string{scriptLink, "--stdio"}, root)
	if err != nil {
		t.Fatalf("resolve canonical invocation: %v", err)
	}
	realNode, _ := filepath.EvalSymlinks(nodeLink)
	realScript, _ := filepath.EvalSymlinks(scriptLink)
	if invocation.Command != realNode {
		t.Fatalf("canonical command = %q, want %q", invocation.Command, realNode)
	}
	if len(invocation.Args) != 2 || invocation.Args[0] != realScript || invocation.Args[1] != "--stdio" {
		t.Fatalf("canonical args = %#v", invocation.Args)
	}
}

func TestStdioCommandPolicyRejectsExecutableSymlinkAndBinaryReplacement(t *testing.T) {
	root := t.TempDir()
	targetOne := writeCommandPolicyExecutable(t, root, "npx-v1", "version-one")
	targetTwo := writeCommandPolicyExecutable(t, root, "npx-v2", "version-two")
	link := filepath.Join(root, "npx")
	if err := os.Symlink(targetOne, link); err != nil {
		t.Fatalf("create executable symlink: %v", err)
	}
	policy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{link},
		NpxPackages:     []string{"official-package"},
	})
	if err != nil {
		t.Fatalf("compile symlink policy: %v", err)
	}
	if _, err := policy.Resolve(link, []string{"-y", "official-package"}, root); err != nil {
		t.Fatalf("resolve original symlink: %v", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatalf("remove executable symlink: %v", err)
	}
	if err := os.Symlink(targetTwo, link); err != nil {
		t.Fatalf("replace executable symlink: %v", err)
	}
	if _, err := policy.Resolve(link, []string{"-y", "official-package"}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("changed executable symlink must fail closed, got %v", err)
	}

	direct := writeCommandPolicyExecutable(t, root, "uvx", "uvx-v1")
	directPolicy, err := NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: []string{direct},
		UvxPackages:     []string{"official-uv-tool"},
	})
	if err != nil {
		t.Fatalf("compile direct executable policy: %v", err)
	}
	if err := os.Remove(direct); err != nil {
		t.Fatalf("remove direct executable: %v", err)
	}
	writeCommandPolicyExecutable(t, root, "uvx", "uvx-v2-replacement")
	if _, err := directPolicy.Resolve(direct, []string{"official-uv-tool"}, root); !errors.Is(err, ErrStdioCommandDenied) {
		t.Fatalf("binary replacement must fail closed, got %v", err)
	}
}

func writeCommandPolicyExecutable(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}
