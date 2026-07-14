// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPServerManagementContract(t *testing.T) {
	server := MCPToolServer{
		CreatorID:  42,
		SourceType: MCPServerSourceTypeCustom,
		Resources: []*MCPResource{
			{
				URI:         "resource://guide",
				ResourceID:  "mcp_resource_safe",
				Name:        "guide",
				Description: "Project guide",
				MIMEType:    "text/markdown",
			},
		},
		Prompts: []*MCPPrompt{
			{
				Name:        "summarize",
				Description: "Summarize a document",
				Arguments: []*MCPPromptArgument{
					{Name: "document", Required: true},
				},
			},
		},
	}

	encoded, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("marshal MCP server: %v", err)
	}
	payload := string(encoded)
	for _, field := range []string{
		`"creator_id":"42"`,
		`"source_type":"custom"`,
		`"resources"`,
		`"prompts"`,
		`"mime_type":"text/markdown"`,
		`"resource_id":"mcp_resource_safe"`,
	} {
		if !strings.Contains(payload, field) {
			t.Fatalf("management contract is missing %s: %s", field, payload)
		}
	}
	if strings.Contains(payload, "resource://guide") || strings.Contains(payload, `"uri"`) {
		t.Fatalf("management contract leaked a raw resource URI: %s", payload)
	}
}

func TestMCPServerSourceTypeValidation(t *testing.T) {
	for _, sourceType := range []MCPServerSourceType{
		MCPServerSourceTypeCustom,
		MCPServerSourceTypeOfficial,
	} {
		if !sourceType.Valid() {
			t.Fatalf("expected %q to be valid", sourceType)
		}
	}

	if MCPServerSourceType("browser-controlled").Valid() {
		t.Fatal("unknown source type must not be accepted")
	}
}
