// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordingRuntimeExecutor struct {
	call   RuntimeToolCall
	result *RuntimeToolResult
	err    error
}

func (r *recordingRuntimeExecutor) ExecuteMCPTool(_ context.Context, call RuntimeToolCall) (*RuntimeToolResult, error) {
	r.call = call
	return r.result, r.err
}

func TestExecuteRuntimeToolCallDelegatesToConfiguredExecutor(t *testing.T) {
	executor := &recordingRuntimeExecutor{
		result: &RuntimeToolResult{Status: "success", Output: `{"temperature":21}`, LatencyMs: 17},
	}
	call := RuntimeToolCall{
		SpaceID:   12,
		ServerID:  34,
		ToolName:  "forecast",
		Arguments: `{"city":"Wuhan"}`,
	}

	result, err := executeRuntimeToolCall(context.Background(), executor, call)
	if err != nil {
		t.Fatalf("execute runtime tool call: %v", err)
	}
	if executor.call != call {
		t.Fatalf("runtime call mismatch: got %#v want %#v", executor.call, call)
	}
	if result.Status != "success" || result.Output != `{"temperature":21}` || result.LatencyMs != 17 {
		t.Fatalf("unexpected bounded result: %#v", result)
	}
}

func TestExecuteRuntimeToolCallFailsClosedWithoutExecutor(t *testing.T) {
	_, err := executeRuntimeToolCall(context.Background(), nil, RuntimeToolCall{})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable, got %v", err)
	}
}

func TestExecuteRuntimeToolCallRejectsOversizedOutput(t *testing.T) {
	executor := &recordingRuntimeExecutor{
		result: &RuntimeToolResult{
			Status: "success",
			Output: strings.Repeat("x", maxRuntimeToolOutputBytes+1),
		},
	}

	_, err := executeRuntimeToolCall(context.Background(), executor, RuntimeToolCall{})
	if !errors.Is(err, ErrRuntimeOutputTooLarge) {
		t.Fatalf("expected ErrRuntimeOutputTooLarge, got %v", err)
	}
}

func TestExecuteRuntimeToolCallPropagatesRuntimeFailure(t *testing.T) {
	runtimeErr := errors.New("runtime unavailable")
	executor := &recordingRuntimeExecutor{err: runtimeErr}

	_, err := executeRuntimeToolCall(context.Background(), executor, RuntimeToolCall{})
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("expected runtime error, got %v", err)
	}
}
