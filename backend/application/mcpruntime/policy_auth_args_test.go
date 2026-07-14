// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"reflect"
	"testing"
)

func TestParseStdioConnectionProjectsOrderedAuthArgsBeforePolicy(t *testing.T) {
	connection := Connection{
		ServerType: "stdio",
		Config: `{
			"command":"npx",
			"auth_args":{"0":"args.0","1":"args.1","2":"args.2"},
			"auth_env":{"TOKEN":"env.TOKEN"}
		}`,
		Auth: `{
			"args":{"0":"-y","1":"private-package","2":"--api-key=secret"},
			"env":{"TOKEN":"env-secret"}
		}`,
	}

	resolved, err := ParseStdioConnection(connection, 64*1024)
	if err != nil {
		t.Fatalf("parse stdio auth references: %v", err)
	}
	if !reflect.DeepEqual(resolved.Args, []string{"-y", "private-package", "--api-key=secret"}) {
		t.Fatalf("resolved args = %#v", resolved.Args)
	}
	if resolved.Env["TOKEN"] != "env-secret" {
		t.Fatalf("resolved env = %#v", resolved.Env)
	}
}

func TestParseStdioConnectionRejectsUnknownAndAmbiguousArgMappings(t *testing.T) {
	tests := []struct {
		name   string
		config string
		auth   string
	}{
		{name: "unknown field", config: `{"command":"npx","custom":"secret"}`, auth: `{}`},
		{name: "reference hole", config: `{"command":"npx","auth_args":{"1":"args.1"}}`, auth: `{"args":{"1":"secret"}}`},
		{name: "wrong reference", config: `{"command":"npx","auth_args":{"0":"args.1"}}`, auth: `{"args":{"0":"secret"}}`},
		{name: "array references", config: `{"command":"npx","auth_args":["args.0"]}`, auth: `{"args":{"0":"secret"}}`},
		{name: "raw and auth args", config: `{"command":"npx","args":["secret"],"auth_args":{"0":"args.0"}}`, auth: `{"args":{"0":"secret"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseStdioConnection(Connection{ServerType: "stdio", Config: tt.config, Auth: tt.auth}, 64*1024)
			if err == nil {
				t.Fatal("expected fail-closed stdio config")
			}
			if err.Error() == "secret" {
				t.Fatalf("error leaked secret: %v", err)
			}
		})
	}
}

func TestPolicyParseStdioAppliesCommandPolicyAfterAuthArgProjection(t *testing.T) {
	policy := testPolicy(t.TempDir())
	policy.StdioAllowedCommands = append(policy.StdioAllowedCommands, "npx")
	policy.StdioAllowedNpxPackages = []string{"private-package"}

	resolved, err := policy.ParseStdio(Connection{
		ServerType: "stdio",
		Config:     `{"command":"npx","auth_args":{"0":"args.0"}}`,
		Auth:       `{"args":{"0":"private-package"}}`,
	})
	if err != nil {
		t.Fatalf("parse projected command args: %v", err)
	}
	if !reflect.DeepEqual(resolved.Args, []string{"private-package"}) {
		t.Fatalf("resolved args = %#v", resolved.Args)
	}
}

func TestPolicyParseRemoteRejectsURLCredentialsAndUnknownFields(t *testing.T) {
	policy := testPolicy(t.TempDir())
	for name, config := range map[string]string{
		"userinfo":      `{"url":"https://user:password@mcp.example.com/events"}`,
		"query":         `{"url":"https://mcp.example.com/events?token=secret"}`,
		"fragment":      `{"url":"https://mcp.example.com/events#secret"}`,
		"unknown field": `{"url":"https://mcp.example.com/events","oauth":{"token":"secret"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := policy.ParseRemote(Connection{ServerType: "sse", Config: config, Auth: `{}`})
			if err == nil {
				t.Fatal("expected fail-closed remote config")
			}
			if err.Error() == "secret" || err.Error() == "password" {
				t.Fatalf("error leaked URL credential: %v", err)
			}
		})
	}
}
