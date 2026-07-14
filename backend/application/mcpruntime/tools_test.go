// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

func TestListToolsRejectsCursorLoopsAndPageLimit(t *testing.T) {
	t.Run("cursor loop", func(t *testing.T) {
		session := &discoverySession{
			toolPages: map[string]ToolPage{
				"":       {Items: []Tool{{Name: "one", InputSchema: json.RawMessage(`{}`)}}, NextCursor: "repeat"},
				"repeat": {Items: []Tool{{Name: "two", InputSchema: json.RawMessage(`{}`)}}, NextCursor: "repeat"},
			},
		}
		_, err := ListTools(context.Background(), session, DiscoveryLimits{
			MaxItems: 10, MaxBytes: 4096, MaxPageBytes: 2048, MaxPages: 10,
		})
		if !errors.Is(err, ErrDiscoveryLimitExceeded) {
			t.Fatalf("expected cursor loop denial, got %v", err)
		}
	})

	t.Run("page limit", func(t *testing.T) {
		session := &discoverySession{
			toolPages: map[string]ToolPage{
				"":               {Items: []Tool{{Name: "one", InputSchema: json.RawMessage(`{}`)}}, NextCursor: "second"},
				"second":         {Items: []Tool{{Name: "two", InputSchema: json.RawMessage(`{}`)}}, NextCursor: "must-not-fetch"},
				"must-not-fetch": {Items: []Tool{{Name: "three", InputSchema: json.RawMessage(`{}`)}}},
			},
		}
		_, err := ListTools(context.Background(), session, DiscoveryLimits{
			MaxItems: 10, MaxBytes: 4096, MaxPageBytes: 2048, MaxPages: 2,
		})
		if !errors.Is(err, ErrDiscoveryLimitExceeded) {
			t.Fatalf("expected page limit denial, got %v", err)
		}
		if got := strings.Join(session.toolCursors, ","); got != ",second" {
			t.Fatalf("unexpected page calls %q", got)
		}
	})
}

func TestListToolsRejectsOversizedSinglePage(t *testing.T) {
	session := &discoverySession{
		toolPages: map[string]ToolPage{
			"": {Items: []Tool{{
				Name:        "large",
				Description: strings.Repeat("x", 512),
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}}},
		},
	}
	_, err := ListTools(context.Background(), session, DiscoveryLimits{
		MaxItems: 10, MaxBytes: 4096, MaxPageBytes: 128, MaxPages: 2,
	})
	if !errors.Is(err, ErrDiscoveryLimitExceeded) {
		t.Fatalf("expected single-page byte denial, got %v", err)
	}
}

func TestFindToolValidatesEntireDecodedPageBeforeReturningTarget(t *testing.T) {
	session := &discoverySession{
		toolPages: map[string]ToolPage{
			"": {
				Items: []Tool{
					{Name: "target", Description: "bounded", InputSchema: json.RawMessage(`{"type":"object"}`)},
					{Name: "oversized-after-target", Description: strings.Repeat("x", 2048)},
				},
				NextCursor: "must-not-fetch",
			},
			"must-not-fetch": {Items: []Tool{{Name: "late"}}},
		},
	}
	_, found, err := FindTool(context.Background(), session, "target", DiscoveryLimits{
		MaxItems: 2, MaxBytes: 256, MaxPageBytes: 256, MaxPages: 2,
	})
	if !errors.Is(err, ErrDiscoveryLimitExceeded) || found {
		t.Fatalf("target page must be fully validated: found=%v err=%v", found, err)
	}
	if got := strings.Join(session.toolCursors, ","); got != "" {
		t.Fatalf("lookup fetched after target: %q", got)
	}
}

func TestMCPToolPagerMapsRawInputSchema(t *testing.T) {
	result := &mcpsdk.ListToolsResult{
		Tools: []mcpsdk.Tool{{
			Name:           "search-docs",
			Description:    "Search documentation",
			RawInputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		}},
	}
	result.NextCursor = mcpsdk.Cursor("next")
	client := &recordingMCPToolPageClient{result: result}
	pager, err := NewMCPToolPager(client)
	if err != nil {
		t.Fatalf("new MCP pager: %v", err)
	}
	page, err := pager.ListToolsByPage(context.Background(), "cursor")
	if err != nil {
		t.Fatalf("list MCP page: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "search-docs" ||
		!strings.Contains(string(page.Items[0].InputSchema), `"query"`) || page.NextCursor != "next" {
		t.Fatalf("unexpected mapped page: %#v", page)
	}
	if len(client.requests) != 1 || string(client.requests[0].Params.Cursor) != "cursor" {
		t.Fatalf("unexpected requests: %#v", client.requests)
	}
}

type recordingMCPToolPageClient struct {
	result   *mcpsdk.ListToolsResult
	err      error
	requests []mcpsdk.ListToolsRequest
}

func (c *recordingMCPToolPageClient) ListToolsByPage(
	_ context.Context,
	request mcpsdk.ListToolsRequest,
) (*mcpsdk.ListToolsResult, error) {
	c.requests = append(c.requests, request)
	return c.result, c.err
}
