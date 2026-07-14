// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPServerExportContractNeverContainsAuth(t *testing.T) {
	exported := ExportMCPToolServerData{
		Name:        "weather",
		Description: "Weather tools",
		ServerType:  "streamable_http",
		Config:      `{"url":"https://mcp.example.com"}`,
		Tools: []*MCPToolDefinition{
			{Name: "forecast", Description: "Get forecast", InputSchema: `{}`},
		},
	}

	encoded, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	payload := strings.ToLower(string(encoded))
	if strings.Contains(payload, "auth") || strings.Contains(payload, "token") || strings.Contains(payload, "secret") {
		t.Fatalf("safe export leaked an authentication field: %s", payload)
	}
}

func TestMCPDiscoveryAndAuditContractsAreBounded(t *testing.T) {
	discovery := DiscoverMCPToolServerData{
		Tools:     []*MCPToolDefinition{{Name: "forecast"}},
		Resources: []*MCPResource{{URI: "resource://guide", ResourceID: "mcp_resource_safe"}},
		Prompts:   []*MCPPrompt{{Name: "summarize"}},
	}
	if len(discovery.Tools) != 1 || len(discovery.Resources) != 1 || len(discovery.Prompts) != 1 {
		t.Fatal("discovery contract must preserve each MCP capability kind")
	}
	encodedDiscovery, err := json.Marshal(discovery)
	if err != nil {
		t.Fatalf("marshal discovery: %v", err)
	}
	if strings.Contains(string(encodedDiscovery), "resource://guide") || strings.Contains(string(encodedDiscovery), `"uri"`) {
		t.Fatalf("discovery leaked raw resource URI: %s", encodedDiscovery)
	}

	event := MCPRuntimeAuditEvent{
		EventID:   "evt-1",
		ToolName:  "forecast",
		Status:    "success",
		LatencyMs: 12,
		CreatedAt: 100,
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	for _, forbidden := range []string{"arguments", "result", "config", "auth", "provider"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("audit event exposed forbidden runtime field %q: %s", forbidden, encoded)
		}
	}
}
