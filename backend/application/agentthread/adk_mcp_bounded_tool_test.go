// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestADKMCPBoundedEinoToolPreservesSchemaAndCallResult(t *testing.T) {
	caller := &recordingBoundedMCPToolCaller{result: &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{mcpsdk.TextContent{Type: "text", Text: "ok"}},
	}}
	base, err := newADKMCPRuntimeEinoTool(caller, mcpruntime.Tool{
		Name:        "search-docs",
		Description: "Search documentation",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
	})
	require.NoError(t, err)

	info, err := base.Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "search-docs", info.Name)
	require.Equal(t, "Search documentation", info.Desc)
	require.NotNil(t, info.ParamsOneOf)
	inputSchema, err := info.ParamsOneOf.ToJSONSchema()
	require.NoError(t, err)
	encodedSchema, err := json.Marshal(inputSchema)
	require.NoError(t, err)
	require.Contains(t, string(encodedSchema), `"query"`)

	result, err := base.InvokableRun(context.Background(), `{"query":"coze"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"content":[{"type":"text","text":"ok"}]}`, result)
	require.Equal(t, "search-docs", caller.request.Params.Name)
	arguments, err := json.Marshal(caller.request.Params.Arguments)
	require.NoError(t, err)
	require.JSONEq(t, `{"query":"coze"}`, string(arguments))
}

func TestADKMCPBoundedEinoToolPreservesCallFailures(t *testing.T) {
	definition := mcpruntime.Tool{
		Name:        "search-docs",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}

	t.Run("transport error", func(t *testing.T) {
		base, err := newADKMCPRuntimeEinoTool(
			&recordingBoundedMCPToolCaller{err: errors.New("call failed")},
			definition,
		)
		require.NoError(t, err)
		_, err = base.InvokableRun(context.Background(), `{}`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to call mcp tool")
	})

	t.Run("MCP error result", func(t *testing.T) {
		base, err := newADKMCPRuntimeEinoTool(
			&recordingBoundedMCPToolCaller{result: &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{mcpsdk.TextContent{Type: "text", Text: "server-error"}},
				IsError: true,
			}},
			definition,
		)
		require.NoError(t, err)
		_, err = base.InvokableRun(context.Background(), `{}`)
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "mcp server return error"))
	})
}

type recordingBoundedMCPToolCaller struct {
	result  *mcpsdk.CallToolResult
	err     error
	request mcpsdk.CallToolRequest
}

func (c *recordingBoundedMCPToolCaller) CallTool(
	_ context.Context,
	request mcpsdk.CallToolRequest,
) (*mcpsdk.CallToolResult, error) {
	c.request = request
	return c.result, c.err
}
