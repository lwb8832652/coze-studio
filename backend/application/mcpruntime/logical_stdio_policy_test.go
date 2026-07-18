// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"errors"
	"testing"
)

func TestLogicalStdioPolicyValidatesProviderExecutableWithoutHostResolution(t *testing.T) {
	t.Parallel()

	policy, err := NewLogicalStdioPolicy(LogicalStdioPolicyOptions{
		AllowedCommands: []string{"provider-mcp"},
		CommandRules: []StdioCommandRule{{
			Command:   "provider-mcp",
			ExactArgs: []string{"serve", "--stdio"},
		}},
		AllowedEnvKeys:   []string{"MCP_TOKEN"},
		MaxArgs:          4,
		MaxArgBytes:      128,
		MaxEnvVars:       1,
		MaxEnvValueBytes: 128,
	})
	if err != nil {
		t.Fatalf("compile logical policy: %v", err)
	}

	err = policy.Validate(StdioConfig{
		Command: "provider-mcp",
		Args:    []string{"serve", "--stdio"},
		Env:     map[string]string{"MCP_TOKEN": "projected-secret"},
	})
	if err != nil {
		t.Fatalf("validate nonexistent provider executable: %v", err)
	}
}

func TestLogicalStdioPolicyRejectsPathsShellWhitespaceAndControls(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		"/usr/bin/provider-mcp",
		`bin/provider-mcp`,
		`bin\provider-mcp`,
		"sh",
		"bash",
		"provider mcp",
		"provider\tmcp",
		"provider\nmcp",
		"provider\x00mcp",
	} {
		if err := ValidateLogicalExecutable(command); !errors.Is(err, ErrStdioCommandDenied) {
			t.Fatalf("logical executable %q error = %v", command, err)
		}
	}
}

func TestLogicalStdioPolicyPreservesPackageArgAndEnvAllowlists(t *testing.T) {
	t.Parallel()

	policy, err := NewLogicalStdioPolicy(LogicalStdioPolicyOptions{
		AllowedCommands:  []string{"npx"},
		NpxPackages:      []string{"@example/provider-mcp"},
		AllowedEnvKeys:   []string{"MCP_TOKEN"},
		MaxArgs:          4,
		MaxArgBytes:      128,
		MaxEnvVars:       1,
		MaxEnvValueBytes: 32,
	})
	if err != nil {
		t.Fatalf("compile logical policy: %v", err)
	}
	if err := policy.Validate(StdioConfig{
		Command: "npx",
		Args:    []string{"-y", "@example/provider-mcp"},
		Env:     map[string]string{"MCP_TOKEN": "secret"},
	}); err != nil {
		t.Fatalf("validate allowed npx invocation: %v", err)
	}

	denied := []StdioConfig{
		{Command: "npx", Args: []string{"-y", "attacker-package"}},
		{Command: "npx", Args: []string{"--package", "attacker-package", "@example/provider-mcp"}},
		{Command: "npx", Args: []string{"-y", "@example/provider-mcp"}, Env: map[string]string{"PATH": "/tmp"}},
		{Command: "npx", Args: []string{"1", "2", "3", "4", "5"}},
	}
	for index, config := range denied {
		if err := policy.Validate(config); err == nil {
			t.Fatalf("denied logical config %d unexpectedly passed", index)
		}
	}
}
