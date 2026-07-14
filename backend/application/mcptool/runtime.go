// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
)

const maxRuntimeToolOutputBytes = 64 * 1024

var (
	ErrRuntimeUnavailable                 = errors.New("MCP runtime is unavailable")
	ErrRuntimeInvalidResult               = errors.New("MCP runtime returned an invalid result")
	ErrRuntimeOutputTooLarge              = errors.New("MCP runtime output exceeds the response limit")
	ErrRuntimeCallFailed                  = errors.New("MCP runtime call failed")
	ErrManagementRuntimeDependencies      = errors.New("MCP management runtime dependencies are required")
	ErrManagementRuntimeAlreadyBound      = errors.New("MCP management runtime is already bound")
)

type RuntimeToolCall struct {
	SpaceID   int64
	ServerID  int64
	ToolName  string
	Arguments string
}

type RuntimeToolResult struct {
	Status    string
	Output    string
	LatencyMs int64
}

// RuntimeExecutor keeps the MCP control plane independent from the concrete
// Eino ADK runtime package. application.go owns the adapter between them.
type RuntimeExecutor interface {
	ExecuteMCPTool(ctx context.Context, call RuntimeToolCall) (*RuntimeToolResult, error)
}

func executeRuntimeToolCall(
	ctx context.Context,
	executor RuntimeExecutor,
	call RuntimeToolCall,
) (*RuntimeToolResult, error) {
	if executor == nil {
		return nil, ErrRuntimeUnavailable
	}

	result, err := executor.ExecuteMCPTool(ctx, call)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrRuntimeInvalidResult
	}
	if len(result.Output) > maxRuntimeToolOutputBytes {
		return nil, ErrRuntimeOutputTooLarge
	}

	return &RuntimeToolResult{
		Status:    result.Status,
		Output:    result.Output,
		LatencyMs: result.LatencyMs,
	}, nil
}
