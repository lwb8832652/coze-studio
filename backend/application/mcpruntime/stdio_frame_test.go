// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const stdioFrameHelperEnv = "COZE_MCP_STDIO_FRAME_HELPER"

func TestSafeStdioTransportRejectsOversizedJSONRPCFrameBeforeDecode(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	transport, err := NewSafeStdioTransport(StdioTransportOptions{
		Command: executable,
		Args: []string{
			"-test.run=^TestMCPRuntimeStdioFrameHelperProcess$",
		},
		Env:           []string{stdioFrameHelperEnv + "=oversized"},
		WorkingDir:    t.TempDir(),
		MaxFrameBytes: 1024,
		ExecutionMode: NewStdioExecutionMode("debug", true),
	})
	if err != nil {
		t.Fatalf("new safe stdio transport: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatalf("start safe stdio transport: %v", err)
	}
	request := mcptransport.JSONRPCRequest{
		JSONRPC: mcpsdk.JSONRPC_VERSION,
		ID:      mcpsdk.NewRequestId(1),
		Method:  "initialize",
		Params:  map[string]any{},
	}
	_, err = transport.SendRequest(ctx, request)
	if !errors.Is(err, ErrStdioFrameLimitExceeded) {
		t.Fatalf("expected fixed frame limit error, got %v", err)
	}
	if err.Error() != ErrStdioFrameLimitExceeded.Error() || strings.Contains(err.Error(), "frame-secret") {
		t.Fatalf("frame error was not fixed and sanitized: %v", err)
	}
	_ = transport.Close()
}

func TestMCPRuntimeStdioFrameHelperProcess(t *testing.T) {
	if os.Getenv(stdioFrameHelperEnv) != "oversized" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(scanner.Bytes(), &request) != nil {
		os.Exit(3)
	}
	_, _ = fmt.Fprintf(
		os.Stdout,
		`{"jsonrpc":"2.0","id":%s,"result":{"payload":"%s-frame-secret"}}`+"\n",
		request.ID,
		strings.Repeat("x", 4096),
	)
	os.Exit(0)
}
