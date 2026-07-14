// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"testing"

	toolmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

type recordingCapabilityDiscoverer struct {
	connection MCPServerConnection
	result     *DiscoveredCapabilities
	err        error
}

func (r *recordingCapabilityDiscoverer) Discover(
	_ context.Context,
	connection MCPServerConnection,
) (*DiscoveredCapabilities, error) {
	r.connection = connection
	return r.result, r.err
}

func TestDiscoverCapabilitiesDelegatesAndPreservesCapabilityKinds(t *testing.T) {
	discoverer := &recordingCapabilityDiscoverer{
		result: &DiscoveredCapabilities{
			Tools:     []*toolmodel.MCPToolDefinition{{Name: "forecast", InputSchema: `{}`}},
			Resources: []*toolmodel.MCPResource{{URI: "resource://guide"}},
			Prompts:   []*toolmodel.MCPPrompt{{Name: "summarize"}},
		},
	}
	connection := MCPServerConnection{
		ServerType: "streamable_http",
		Config:     `{"url":"https://mcp.example.com"}`,
		Auth:       `{"token":"secret"}`,
	}

	result, err := discoverCapabilities(context.Background(), discoverer, connection)
	if err != nil {
		t.Fatalf("discover capabilities: %v", err)
	}
	if discoverer.connection != connection {
		t.Fatalf("connection mismatch: got %#v want %#v", discoverer.connection, connection)
	}
	if len(result.Tools) != 1 || len(result.Resources) != 1 || len(result.Prompts) != 1 {
		t.Fatalf("unexpected discovery result: %#v", result)
	}
}

func TestDiscoverCapabilitiesFailsClosedWithoutDiscoverer(t *testing.T) {
	_, err := discoverCapabilities(context.Background(), nil, MCPServerConnection{})
	if !errors.Is(err, ErrCapabilityDiscoveryUnavailable) {
		t.Fatalf("expected ErrCapabilityDiscoveryUnavailable, got %v", err)
	}
}

func TestDiscoverCapabilitiesRejectsUnboundedResults(t *testing.T) {
	tools := make([]*toolmodel.MCPToolDefinition, maxDiscoveredCapabilities+1)
	for i := range tools {
		tools[i] = &toolmodel.MCPToolDefinition{Name: "tool"}
	}
	discoverer := &recordingCapabilityDiscoverer{
		result: &DiscoveredCapabilities{Tools: tools},
	}

	_, err := discoverCapabilities(context.Background(), discoverer, MCPServerConnection{})
	if !errors.Is(err, ErrCapabilityDiscoveryTooLarge) {
		t.Fatalf("expected ErrCapabilityDiscoveryTooLarge, got %v", err)
	}
}
