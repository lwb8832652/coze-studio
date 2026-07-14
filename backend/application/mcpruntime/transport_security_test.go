// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	stdioSecurityHelperEnv     = "COZE_MCP_STDIO_SECURITY_HELPER"
	stdioSecurityHelperEnvFile = "COZE_MCP_STDIO_SECURITY_ENV_FILE"
)

func TestSafeStdioTransportUsesMinimalEnvironmentAndContinuouslyDrainsStderr(t *testing.T) {
	t.Setenv("HOST_MCP_SECRET", "must-not-reach-child")
	t.Setenv("DATABASE_URL", "mysql://must-not-reach-child")
	t.Setenv("MCP_AES_AUTH_SECRET", "must-not-reach-child")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-reach-child")

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	envFile := filepath.Join(t.TempDir(), "child-env.json")
	transport, err := NewSafeStdioTransport(StdioTransportOptions{
		Command: executable,
		Args: []string{
			"-test.run=^TestMCPRuntimeStdioSecurityHelperProcess$",
		},
		Env: []string{
			stdioSecurityHelperEnv + "=1",
			stdioSecurityHelperEnvFile + "=" + envFile,
		},
		WorkingDir:    t.TempDir(),
		ExecutionMode: NewStdioExecutionMode("debug", true),
	})
	if err != nil {
		t.Fatalf("new safe stdio transport: %v", err)
	}
	client := mcpclient.NewClient(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start stdio client: %v", err)
	}
	request := mcpsdk.InitializeRequest{}
	request.Params.ProtocolVersion = mcpsdk.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = mcpsdk.Implementation{Name: "security-test", Version: "1"}
	if _, err := client.Initialize(ctx, request); err != nil {
		t.Fatalf("initialize after large stderr output: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close stdio client: %v", err)
	}

	childEnv := readSecurityHelperEnvironment(t, envFile)
	assertMinimalSecurityHelperEnvironment(t, childEnv, executable)
}

func TestMCPRuntimeStdioSecurityHelperProcess(t *testing.T) {
	if os.Getenv(stdioSecurityHelperEnv) != "1" {
		return
	}
	envBytes, err := json.Marshal(os.Environ())
	if err != nil || os.WriteFile(os.Getenv(stdioSecurityHelperEnvFile), envBytes, 0o600) != nil {
		os.Exit(2)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.Method != "initialize" {
			continue
		}
		chunk := bytes.Repeat([]byte("stderr-must-be-drained\n"), 4096)
		for index := 0; index < 32; index++ {
			if _, err := os.Stderr.Write(chunk); err != nil {
				os.Exit(3)
			}
		}
		_, _ = fmt.Fprintf(
			os.Stdout,
			`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"%s","capabilities":{},"serverInfo":{"name":"security-helper","version":"1"}}}`+"\n",
			request.ID,
			mcpsdk.LATEST_PROTOCOL_VERSION,
		)
	}
	os.Exit(0)
}

func TestSafeSSETransportIgnoresDuplicateEndpointAndProjectsHeadersEverywhere(t *testing.T) {
	const authorization = "Bearer projected-auth-token"

	responses := make(chan string, 8)
	seenHeaders := make(map[string]string)
	var seenMu sync.Mutex
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			seenMu.Lock()
			seenHeaders["GET"] = request.Header.Get("Authorization")
			seenMu.Unlock()
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := writer.(http.Flusher)
			if !ok {
				http.Error(writer, "stream unsupported", http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprintf(writer, "event: endpoint\ndata: %s/rpc\n\n", server.URL)
			_, _ = fmt.Fprintf(writer, "event: endpoint\ndata: %s/rpc\n\n", server.URL)
			flusher.Flush()
			for {
				select {
				case response := <-responses:
					_, _ = fmt.Fprintf(writer, "event: message\ndata: %s\n\n", response)
					flusher.Flush()
				case <-request.Context().Done():
					return
				}
			}
		}

		var payload struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(writer, "invalid payload", http.StatusBadRequest)
			return
		}
		seenMu.Lock()
		seenHeaders[payload.Method] = request.Header.Get("Authorization")
		seenMu.Unlock()
		writer.WriteHeader(http.StatusAccepted)
		if len(payload.ID) > 0 && string(payload.ID) != "null" {
			responses <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{}}`, payload.ID)
		}
	}))
	defer server.Close()

	transport, err := NewSafeSSETransport(SSETransportOptions{
		URL:             server.URL,
		Headers:         map[string]string{"Authorization": authorization},
		HTTPClient:      server.Client(),
		EndpointTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new safe SSE transport: %v", err)
	}
	defer func() { _ = transport.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatalf("start safe SSE transport: %v", err)
	}

	for index, method := range []string{"initialize", "tools/list", "tools/call"} {
		_, err := transport.SendRequest(ctx, mcptransport.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      mcpsdk.NewRequestId(int64(index + 1)),
			Method:  method,
			Params:  map[string]any{},
		})
		if err != nil {
			t.Fatalf("send %s request: %v", method, err)
		}
	}
	if err := transport.SendNotification(ctx, mcpsdk.JSONRPCNotification{
		JSONRPC: "2.0",
		Notification: mcpsdk.Notification{
			Method: "notifications/initialized",
		},
	}); err != nil {
		t.Fatalf("send notification: %v", err)
	}

	seenMu.Lock()
	defer seenMu.Unlock()
	for _, operation := range []string{"GET", "initialize", "tools/list", "tools/call", "notifications/initialized"} {
		if got := seenHeaders[operation]; got != authorization {
			t.Fatalf("%s authorization header = %q", operation, got)
		}
	}
}

func TestSafeSSETransportRejectsPreEndpointStreamLimits(t *testing.T) {
	tests := map[string]func(io.Writer){
		"too many data lines without delimiter": func(writer io.Writer) {
			for index := 0; index <= maxSSEDataLines; index++ {
				_, _ = io.WriteString(writer, "data: x\n")
			}
		},
		"single line too large": func(writer io.Writer) {
			_, _ = io.WriteString(writer, strings.Repeat("x", maxSSELineBytes+1)+"\n")
		},
	}
	for name, writeAttack := range tests {
		name, writeAttack := name, writeAttack
		t.Run(name, func(t *testing.T) {
			streamClosed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				defer close(streamClosed)
				writer.Header().Set("Content-Type", "text/event-stream")
				writeAttack(writer)
				writer.(http.Flusher).Flush()
				<-request.Context().Done()
			}))
			defer server.Close()
			transport, err := NewSafeSSETransport(SSETransportOptions{
				URL:             server.URL,
				HTTPClient:      server.Client(),
				EndpointTimeout: 2 * time.Second,
			})
			if err != nil {
				t.Fatalf("new safe SSE transport: %v", err)
			}
			defer func() { _ = transport.Close() }()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = transport.Start(ctx)
			assertFixedSSELimitError(t, err, "attacker-controlled-data")
			select {
			case <-streamClosed:
			case <-time.After(time.Second):
				t.Fatal("SSE response was not closed after stream limit")
			}
		})
	}
}

func TestSafeSSETransportRejectsOversizedEventDuringRequest(t *testing.T) {
	const secret = "attacker-secret-must-not-enter-error"
	trigger := make(chan struct{}, 1)
	streamClosed := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusAccepted)
			trigger <- struct{}{}
			return
		}
		defer close(streamClosed)
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		_, _ = fmt.Fprintf(writer, "event: endpoint\ndata: %s/rpc\n\n", server.URL)
		flusher.Flush()
		<-trigger
		const eventLine = "event: message"
		_, _ = io.WriteString(writer, eventLine+"\n")
		writeSSECommentBytes(writer, maxSSEEventBytes-len(eventLine))
		_, _ = io.WriteString(writer, ":"+secret+"\n")
		flusher.Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	transport, err := NewSafeSSETransport(SSETransportOptions{
		URL:             server.URL,
		HTTPClient:      server.Client(),
		EndpointTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new safe SSE transport: %v", err)
	}
	defer func() { _ = transport.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatalf("start safe SSE transport: %v", err)
	}
	_, err = transport.SendRequest(ctx, mcptransport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcpsdk.NewRequestId(int64(1)),
		Method:  "tools/list",
		Params:  map[string]any{},
	})
	assertFixedSSELimitError(t, err, secret)
	select {
	case <-streamClosed:
	case <-time.After(time.Second):
		t.Fatal("SSE response was not closed after event byte limit")
	}
}

func TestSafeSSETransportAcceptsExactLimitsAndNormalMultilineEvent(t *testing.T) {
	trigger := make(chan struct{}, 1)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusAccepted)
			trigger <- struct{}{}
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		_, _ = fmt.Fprintf(writer, "event: endpoint\ndata: %s/rpc\n\n", server.URL)
		flusher.Flush()
		<-trigger
		const (
			eventLine = "event: message"
			dataPart1 = `data: {"jsonrpc":"2.0",`
			dataPart2 = `data: "id":1,"result":{}}`
		)
		baseBytes := len(eventLine) + len(dataPart1) + len(dataPart2) + (maxSSEDataLines-2)*len("data:")
		_, _ = io.WriteString(writer, eventLine+"\n")
		writeSSECommentBytes(writer, maxSSEEventBytes-baseBytes)
		for index := 0; index < maxSSEDataLines-2; index++ {
			_, _ = io.WriteString(writer, "data:\n")
		}
		_, _ = io.WriteString(writer, dataPart1+"\n")
		_, _ = io.WriteString(writer, dataPart2+"\n\n")
		flusher.Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	transport, err := NewSafeSSETransport(SSETransportOptions{
		URL:             server.URL,
		HTTPClient:      server.Client(),
		EndpointTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new safe SSE transport: %v", err)
	}
	defer func() { _ = transport.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatalf("start safe SSE transport: %v", err)
	}
	result, err := transport.SendRequest(ctx, mcptransport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcpsdk.NewRequestId(int64(1)),
		Method:  "tools/list",
		Params:  map[string]any{},
	})
	if err != nil {
		t.Fatalf("exact-limit multiline event: %v", err)
	}
	if result == nil || result.ID.String() != "int64:1" {
		t.Fatalf("unexpected multiline response: %#v", result)
	}
}

func writeSSECommentBytes(writer io.Writer, total int) {
	for total > 0 {
		lineBytes := total
		if lineBytes > maxSSELineBytes {
			lineBytes = maxSSELineBytes
		}
		_, _ = io.WriteString(writer, ":"+strings.Repeat("x", lineBytes-1)+"\n")
		total -= lineBytes
	}
}

func assertFixedSSELimitError(t *testing.T, err error, secret string) {
	t.Helper()
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("expected fixed unavailable error, got %v", err)
	}
	if err.Error() != ErrSessionUnavailable.Error() {
		t.Fatalf("unexpected error text %q", err.Error())
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("stream data leaked into error")
	}
}

func readSecurityHelperEnvironment(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read child environment: %v", err)
	}
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode child environment: %v", err)
	}
	return result
}

func assertMinimalSecurityHelperEnvironment(t *testing.T, environment []string, executable string) {
	t.Helper()
	realExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatalf("resolve real test executable: %v", err)
	}
	values := make(map[string]string, len(environment))
	for _, item := range environment {
		for index := 0; index < len(item); index++ {
			if item[index] == '=' {
				values[item[:index]] = item[index+1:]
				break
			}
		}
	}
	for _, forbidden := range []string{
		"HOST_MCP_SECRET",
		"DATABASE_URL",
		"MCP_AES_AUTH_SECRET",
		"AWS_SECRET_ACCESS_KEY",
	} {
		if _, ok := values[forbidden]; ok {
			t.Fatalf("child inherited forbidden environment key %s", forbidden)
		}
	}
	if got := values["PATH"]; got != filepath.Dir(realExecutable) {
		t.Fatalf("child PATH = %q, want %q", got, filepath.Dir(realExecutable))
	}
	for key := range values {
		switch key {
		case stdioSecurityHelperEnv, stdioSecurityHelperEnvFile, "PATH":
		default:
			t.Fatalf("child received unexpected environment key %s", key)
		}
	}
}
