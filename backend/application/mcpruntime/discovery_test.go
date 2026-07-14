// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiscoverUsesBoundedPageLoopAndStopsAtItemLimit(t *testing.T) {
	t.Parallel()

	session := &discoverySession{
		capabilities: CapabilityFlags{Tools: true},
		toolPages: map[string]ToolPage{
			"":               {Items: []Tool{{Name: "one", InputSchema: []byte(`{}`)}, {Name: "two", InputSchema: []byte(`{}`)}}, NextCursor: "next"},
			"next":           {Items: []Tool{{Name: "three", InputSchema: []byte(`{}`)}, {Name: "four", InputSchema: []byte(`{}`)}}, NextCursor: "must-not-fetch"},
			"must-not-fetch": {Items: []Tool{{Name: "five", InputSchema: []byte(`{}`)}}},
		},
	}

	_, err := Discover(context.Background(), session, DiscoveryLimits{MaxItems: 3, MaxBytes: 4096, MaxPages: 10})
	if !errors.Is(err, ErrDiscoveryLimitExceeded) {
		t.Fatalf("expected item limit failure, got %v", err)
	}
	if got := strings.Join(session.toolCursors, ","); got != ",next" {
		t.Fatalf("unexpected page calls %q", got)
	}
}

func TestDiscoverSkipsUnsupportedCapabilities(t *testing.T) {
	t.Parallel()

	session := &discoverySession{}
	result, err := Discover(context.Background(), session, DiscoveryLimits{MaxItems: 10, MaxBytes: 4096, MaxPages: 10})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Tools) != 0 || len(result.Resources) != 0 || len(result.Prompts) != 0 {
		t.Fatalf("unexpected definitions: %#v", result)
	}
	if len(session.toolCursors)+len(session.resourceCursors)+len(session.promptCursors) != 0 {
		t.Fatalf("unsupported list method was called: %#v", session)
	}
}

func TestDiscoverStopsAtByteLimitAndRejectsCursorCycles(t *testing.T) {
	t.Parallel()

	t.Run("byte limit", func(t *testing.T) {
		session := &discoverySession{
			capabilities: CapabilityFlags{Resources: true},
			resourcePages: map[string]ResourcePage{
				"": {Items: []Resource{{URI: "resource://large", Description: strings.Repeat("x", 128)}}},
			},
		}
		_, err := Discover(context.Background(), session, DiscoveryLimits{MaxItems: 10, MaxBytes: 64, MaxPages: 10})
		if !errors.Is(err, ErrDiscoveryLimitExceeded) {
			t.Fatalf("expected byte limit failure, got %v", err)
		}
	})

	t.Run("cursor cycle", func(t *testing.T) {
		session := &discoverySession{
			capabilities: CapabilityFlags{Prompts: true},
			promptPages: map[string]PromptPage{
				"":       {Items: []Prompt{{Name: "one"}}, NextCursor: "repeat"},
				"repeat": {Items: []Prompt{{Name: "two"}}, NextCursor: "repeat"},
			},
		}
		_, err := Discover(context.Background(), session, DiscoveryLimits{MaxItems: 10, MaxBytes: 4096, MaxPages: 10})
		if !errors.Is(err, ErrDiscoveryLimitExceeded) {
			t.Fatalf("expected cursor cycle failure, got %v", err)
		}
	})
}

type discoverySession struct {
	capabilities    CapabilityFlags
	toolPages       map[string]ToolPage
	resourcePages   map[string]ResourcePage
	promptPages     map[string]PromptPage
	toolCursors     []string
	resourceCursors []string
	promptCursors   []string
}

func (s *discoverySession) Capabilities() CapabilityFlags { return s.capabilities }

func (s *discoverySession) ListToolsByPage(_ context.Context, cursor string) (ToolPage, error) {
	s.toolCursors = append(s.toolCursors, cursor)
	return s.toolPages[cursor], nil
}

func (s *discoverySession) ListResourcesByPage(_ context.Context, cursor string) (ResourcePage, error) {
	s.resourceCursors = append(s.resourceCursors, cursor)
	return s.resourcePages[cursor], nil
}

func (s *discoverySession) ListPromptsByPage(_ context.Context, cursor string) (PromptPage, error) {
	s.promptCursors = append(s.promptCursors, cursor)
	return s.promptPages[cursor], nil
}

func (s *discoverySession) CallTool(context.Context, string, map[string]any) (ToolCallResult, error) {
	return ToolCallResult{}, nil
}

func (s *discoverySession) Close() error { return nil }
