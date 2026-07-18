// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestMCPStdioInvokeEnvelopeNormalizesStrictBoundedContract(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizeMCPStdioInvokeEnvelope(MCPStdioInvokeEnvelope{
		Schema:        MCPStdioInvokeSchemaV1,
		OperationID:   "operation_123",
		Command:       "npx",
		Args:          []string{"-y", "@example/provider-mcp"},
		ToolName:      "search-docs",
		Arguments:     json.RawMessage(`{"query":"docs"}`),
		WorkingDir:    "workspace/mcp_stdio/run-20",
		EnvNames:      []string{"MCP_TOKEN", "MCP_TOKEN"},
		ClientVersion: "1",
	})
	if err != nil {
		t.Fatalf("normalize invoke envelope: %v", err)
	}
	if normalized.Command != "npx" || len(normalized.EnvNames) != 1 ||
		normalized.EnvNames[0] != "MCP_TOKEN" {
		t.Fatalf("normalized envelope = %#v", normalized)
	}
	wire, err := MarshalMCPStdioInvokeEnvelope(normalized)
	if err != nil {
		t.Fatalf("marshal invoke envelope: %v", err)
	}
	if bytes.Contains(wire, []byte("projected-secret")) {
		t.Fatal("invoke stdin contains an environment secret")
	}
}

func TestMCPStdioInvokeEnvelopeRejectsUnknownMalformedAndOversizedInput(t *testing.T) {
	t.Parallel()

	valid := MCPStdioInvokeEnvelope{
		Schema:        MCPStdioInvokeSchemaV1,
		OperationID:   "operation_123",
		Command:       "npx",
		Args:          []string{"-y", "@example/provider-mcp"},
		ToolName:      "search-docs",
		Arguments:     json.RawMessage(`{"query":"docs"}`),
		WorkingDir:    "workspace/mcp_stdio/run-20",
		ClientVersion: "1",
	}
	for name, mutate := range map[string]func(*MCPStdioInvokeEnvelope){
		"schema":       func(value *MCPStdioInvokeEnvelope) { value.Schema = "unknown" },
		"command path": func(value *MCPStdioInvokeEnvelope) { value.Command = "/bin/npx" },
		"workdir":      func(value *MCPStdioInvokeEnvelope) { value.WorkingDir = "../host" },
		"arguments":    func(value *MCPStdioInvokeEnvelope) { value.Arguments = json.RawMessage(`[]`) },
		"oversized": func(value *MCPStdioInvokeEnvelope) {
			value.Arguments = json.RawMessage(`{"value":"` + strings.Repeat("x", MaxStdinBytes) + `"}`)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := NormalizeMCPStdioInvokeEnvelope(input); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("normalize error = %v", err)
			}
		})
	}
}

func TestParseMCPStdioResultEnvelopeReturnsOnlyReviewedToolSemantics(t *testing.T) {
	t.Parallel()

	result, err := ParseMCPStdioResultEnvelope([]byte(`{
		"schema":"coze.sandbox.mcp_stdio.result.v1",
		"content":[{"type":"text","text":"safe result"}],
		"is_error":true
	}`), 1024)
	if err != nil {
		t.Fatalf("parse result envelope: %v", err)
	}
	if !result.IsError || len(result.Content) != 1 || result.Content[0].Text != "safe result" {
		t.Fatalf("result projection = %#v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	for _, prohibited := range []string{"schema", "execution_id", "stdout", "stderr", "endpoint"} {
		if bytes.Contains(encoded, []byte(prohibited)) {
			t.Fatalf("projection contains prohibited field %q: %s", prohibited, encoded)
		}
	}
}

func TestParseMCPStdioResultEnvelopeFailsClosedOnMalformedUnknownAndOversized(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"malformed":       `{"schema":`,
		"unknown schema":  `{"schema":"v2","content":[],"is_error":false}`,
		"unknown field":   `{"schema":"coze.sandbox.mcp_stdio.result.v1","content":[],"is_error":false,"execution_id":"secret"}`,
		"duplicate field": `{"schema":"coze.sandbox.mcp_stdio.result.v1","content":[],"content":[],"is_error":false}`,
		"unknown content": `{"schema":"coze.sandbox.mcp_stdio.result.v1","content":[{"type":"image","text":"raw"}],"is_error":false}`,
		"oversized":       `{"schema":"coze.sandbox.mcp_stdio.result.v1","content":[{"type":"text","text":"` + strings.Repeat("x", 1024) + `"}],"is_error":false}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			if _, err := ParseMCPStdioResultEnvelope([]byte(body), 256); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("parse error = %v", err)
			}
		})
	}
}
