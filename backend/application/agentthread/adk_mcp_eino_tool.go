// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

type adkMCPToolCaller interface {
	CallTool(
		ctx context.Context,
		request mcpsdk.CallToolRequest,
	) (*mcpsdk.CallToolResult, error)
}

type adkMCPRuntimeEinoTool struct {
	caller adkMCPToolCaller
	info   *schema.ToolInfo
}

func newADKMCPRuntimeEinoTool(
	caller adkMCPToolCaller,
	definition mcpruntime.Tool,
) (tool.InvokableTool, error) {
	name := strings.TrimSpace(definition.Name)
	if caller == nil || name == "" || len(definition.InputSchema) == 0 {
		return nil, mcpruntime.ErrSessionUnavailable
	}
	inputSchema := &jsonschema.Schema{}
	if err := sonic.Unmarshal(definition.InputSchema, inputSchema); err != nil {
		return nil, mcpruntime.ErrSessionUnavailable
	}
	return &adkMCPRuntimeEinoTool{
		caller: caller,
		info: &schema.ToolInfo{
			Name:        name,
			Desc:        definition.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(inputSchema),
		},
	}, nil
}

func (t *adkMCPRuntimeEinoTool) Info(context.Context) (*schema.ToolInfo, error) {
	if t == nil || t.info == nil {
		return nil, mcpruntime.ErrSessionUnavailable
	}
	return t.info, nil
}

func (t *adkMCPRuntimeEinoTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	if t == nil || t.caller == nil || t.info == nil {
		return "", mcpruntime.ErrSessionUnavailable
	}
	result, err := t.caller.CallTool(ctx, mcpsdk.CallToolRequest{
		Request: mcpsdk.Request{Method: "tools/call"},
		Params: mcpsdk.CallToolParams{
			Name:      t.info.Name,
			Arguments: json.RawMessage(argumentsInJSON),
		},
	})
	if err != nil || result == nil {
		return "", fmt.Errorf("failed to call mcp tool: %w", mcpruntime.ErrSessionUnavailable)
	}
	marshaledResult, err := sonic.MarshalString(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal mcp tool result: %w", mcpruntime.ErrSessionUnavailable)
	}
	if result.IsError {
		return "", fmt.Errorf("failed to call mcp tool, mcp server return error: %s", marshaledResult)
	}
	return marshaledResult, nil
}

func boundedADKMCPRuntimeEinoTools(
	ctx context.Context,
	client mcpclient.MCPClient,
	toolName string,
) ([]tool.BaseTool, error) {
	if client == nil {
		return nil, mcpruntime.ErrSessionUnavailable
	}
	pager, err := mcpruntime.NewMCPToolPager(client)
	if err != nil {
		return nil, err
	}
	definition, found, err := mcpruntime.FindTool(
		ctx,
		pager,
		strings.TrimSpace(toolName),
		mcpruntime.DefaultDiscoveryLimits(),
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	wrapped, err := newADKMCPRuntimeEinoTool(client, definition)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{wrapped}, nil
}
