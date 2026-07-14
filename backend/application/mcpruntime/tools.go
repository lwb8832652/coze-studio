// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
	"strings"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

type ToolPager interface {
	ListToolsByPage(ctx context.Context, cursor string) (ToolPage, error)
}

type MCPToolPageClient interface {
	ListToolsByPage(
		ctx context.Context,
		request mcpsdk.ListToolsRequest,
	) (*mcpsdk.ListToolsResult, error)
}

type mcpToolPager struct {
	client MCPToolPageClient
}

func NewMCPToolPager(client MCPToolPageClient) (ToolPager, error) {
	if client == nil {
		return nil, ErrSessionUnavailable
	}
	return &mcpToolPager{client: client}, nil
}

func (p *mcpToolPager) ListToolsByPage(ctx context.Context, cursor string) (ToolPage, error) {
	if p == nil || p.client == nil {
		return ToolPage{}, ErrSessionUnavailable
	}
	request := mcpsdk.ListToolsRequest{}
	request.Params.Cursor = mcpsdk.Cursor(cursor)
	result, err := p.client.ListToolsByPage(ctx, request)
	if err != nil || result == nil {
		return ToolPage{}, ErrSessionUnavailable
	}
	items := make([]Tool, 0, len(result.Tools))
	for _, item := range result.Tools {
		schema := append(json.RawMessage(nil), item.RawInputSchema...)
		if len(schema) == 0 {
			schema, err = json.Marshal(item.InputSchema)
			if err != nil {
				return ToolPage{}, ErrSessionUnavailable
			}
		}
		items = append(items, Tool{
			Name:        item.Name,
			Description: item.Description,
			InputSchema: schema,
		})
	}
	return ToolPage{Items: items, NextCursor: string(result.NextCursor)}, nil
}

func ListTools(ctx context.Context, pager ToolPager, limits DiscoveryLimits) ([]Tool, error) {
	if pager == nil {
		return nil, ErrSessionUnavailable
	}
	budget := &discoveryBudget{limits: discoveryLimitsWithDefaults(limits)}
	return listToolsWithBudget(ctx, pager, budget, "")
}

func FindTool(
	ctx context.Context,
	pager ToolPager,
	name string,
	limits DiscoveryLimits,
) (Tool, bool, error) {
	name = strings.TrimSpace(name)
	if pager == nil || name == "" {
		return Tool{}, false, ErrInvalidConnection
	}
	budget := &discoveryBudget{limits: discoveryLimitsWithDefaults(limits)}
	items, err := listToolsWithBudget(ctx, pager, budget, name)
	if err != nil {
		return Tool{}, false, err
	}
	if len(items) == 0 {
		return Tool{}, false, nil
	}
	return items[0], true, nil
}

func listToolsWithBudget(
	ctx context.Context,
	pager ToolPager,
	budget *discoveryBudget,
	targetName string,
) ([]Tool, error) {
	items := []Tool{}
	cursor := ""
	seen := map[string]struct{}{}
	for pageCount := 0; ; pageCount++ {
		if pageCount >= budget.limits.MaxPages {
			return nil, ErrDiscoveryLimitExceeded
		}
		page, err := pager.ListToolsByPage(ctx, cursor)
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		if err := validateDiscoveryPage(budget, page.Items, page.NextCursor, seen); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			item.InputSchema = append(json.RawMessage(nil), item.InputSchema...)
			if targetName != "" && strings.TrimSpace(item.Name) == targetName {
				return []Tool{item}, nil
			}
			if targetName == "" {
				items = append(items, item)
			}
		}
		if page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}
