// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

func TestProductionSessionFactoryStartsInitializesAndCleansOwnedWorkdir(t *testing.T) {
	t.Parallel()

	root := safeWorkdirTestRoot(t)
	client := &fakeProtocolClient{initializeResult: initializeResultWithCapabilities(t, true, true, true)}
	var built ResolvedConnection
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			built = connection
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new session factory: %v", err)
	}

	session, err := factory.Open(context.Background(), Connection{
		ServerType: "stdio",
		Config:     `{"command":"/bin/echo","args":["hello"]}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if client.startCalls.Load() != 1 || client.initializeCalls.Load() != 1 {
		t.Fatalf("start=%d initialize=%d", client.startCalls.Load(), client.initializeCalls.Load())
	}
	if built.Stdio == nil || built.Stdio.WorkingDir == "" {
		t.Fatalf("missing owned stdio workdir: %#v", built)
	}
	if filepath.Dir(built.Stdio.WorkingDir) != filepath.Clean(root) {
		t.Fatalf("workdir escaped root: %q", built.Stdio.WorkingDir)
	}
	if _, statErr := os.Stat(built.Stdio.WorkingDir); statErr != nil {
		t.Fatalf("owned workdir does not exist: %v", statErr)
	}
	if got := session.Capabilities(); !got.Tools || !got.Resources || !got.Prompts {
		t.Fatalf("capability flags = %#v", got)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	if client.closeCalls.Load() != 1 {
		t.Fatalf("client close calls = %d", client.closeCalls.Load())
	}
	if _, statErr := os.Stat(built.Stdio.WorkingDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("owned workdir was not removed: %v", statErr)
	}
}

func TestProductionSessionFactoryInitializeFailureClosesAndCleans(t *testing.T) {
	t.Parallel()

	root := safeWorkdirTestRoot(t)
	client := &fakeProtocolClient{initializeErr: errors.New("provider returned raw-secret")}
	var workingDir string
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:        testPolicy(root),
		ExecutionMode: NewStdioExecutionMode("debug", true),
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			workingDir = connection.Stdio.WorkingDir
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new session factory: %v", err)
	}

	_, err = factory.Open(context.Background(), Connection{
		ServerType: "stdio",
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("expected sanitized initialize failure, got %v", err)
	}
	if err != nil && containsSecret(err.Error()) {
		t.Fatalf("initialize error leaked secret: %v", err)
	}
	if client.closeCalls.Load() != 1 {
		t.Fatalf("client close calls = %d", client.closeCalls.Load())
	}
	if _, statErr := os.Stat(workingDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("workdir survived initialize failure: %v", statErr)
	}
}

func TestProductionSessionMapsSDKDefinitionsAndDiscardsRawCallResult(t *testing.T) {
	t.Parallel()

	client := &fakeProtocolClient{
		initializeResult: initializeResultWithCapabilities(t, true, true, true),
		toolsResult: &mcpsdk.ListToolsResult{
			Tools: []mcpsdk.Tool{{
				Name:           "search",
				Description:    "Search documents",
				RawInputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			}},
		},
		resourcesResult: &mcpsdk.ListResourcesResult{
			Resources: []mcpsdk.Resource{{URI: "resource://guide", Name: "guide", Description: "Guide", MIMEType: "text/plain"}},
		},
		promptsResult: &mcpsdk.ListPromptsResult{
			Prompts: []mcpsdk.Prompt{{Name: "summarize", Description: "Summarize", Arguments: []mcpsdk.PromptArgument{{Name: "topic", Required: true}}}},
		},
		callToolResult: &mcpsdk.CallToolResult{
			Content:           []mcpsdk.Content{mcpsdk.TextContent{Type: "text", Text: "raw-secret"}},
			StructuredContent: map[string]any{"token": "provider-secret"},
		},
	}
	policy := testPolicy(t.TempDir())
	policy.StdioEnabled = false
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy: policy,
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, _ ResolvedConnection) (ProtocolClient, error) {
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new session factory: %v", err)
	}
	session, err := factory.Open(context.Background(), Connection{
		ServerType: "sse",
		Config:     `{"url":"https://mcp.example.com/events"}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	defer session.Close()

	tools, err := session.ListToolsByPage(context.Background(), "")
	if err != nil || len(tools.Items) != 1 {
		t.Fatalf("list tools: %#v, %v", tools, err)
	}
	if string(tools.Items[0].InputSchema) != `{"type":"object","properties":{"query":{"type":"string"}}}` {
		t.Fatalf("input schema = %s", tools.Items[0].InputSchema)
	}
	resources, err := session.ListResourcesByPage(context.Background(), "")
	if err != nil || len(resources.Items) != 1 || resources.Items[0].MIMEType != "text/plain" {
		t.Fatalf("list resources: %#v, %v", resources, err)
	}
	prompts, err := session.ListPromptsByPage(context.Background(), "")
	if err != nil || len(prompts.Items) != 1 || !prompts.Items[0].Arguments[0].Required {
		t.Fatalf("list prompts: %#v, %v", prompts, err)
	}
	result, err := session.CallTool(context.Background(), "search", map[string]any{"query": "coze"})
	if err != nil || result.IsError {
		t.Fatalf("call tool: %#v, %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal bounded result: %v", err)
	}
	if containsSecret(string(encoded)) {
		t.Fatalf("bounded result leaked provider content: %s", encoded)
	}
}

func TestProductionSessionFactoryRetriesBoundedSafeWorkdirCleanup(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{
		Root: root,
		DeleteLimits: SafeWorkdirDeleteLimits{
			MaxEntries: 2,
			MaxBytes:   512,
			MaxDepth:   8,
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	client := &fakeProtocolClient{initializeResult: initializeResultWithCapabilities(t, true, false, false)}
	var workingDir string
	factory, err := NewProductionSessionFactory(FactoryOptions{
		Policy:                testPolicy(root),
		ExecutionMode:         NewStdioExecutionMode("debug", true),
		WorkdirManager:        manager,
		WorkdirCleanupTimeout: time.Second,
		ClientBuilder: ProtocolClientBuilderFunc(func(_ context.Context, connection ResolvedConnection) (ProtocolClient, error) {
			workingDir = connection.Stdio.WorkingDir
			return client, nil
		}),
	})
	if err != nil {
		t.Fatalf("new factory: %v", err)
	}
	session, err := factory.Open(context.Background(), Connection{
		ServerType: "stdio",
		Config:     `{"command":"/bin/echo"}`,
		Auth:       `{}`,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for index := 0; index < 7; index++ {
		name := filepath.Join(workingDir, "entry-"+string(rune('a'+index)))
		if err := os.WriteFile(name, []byte("bounded"), 0o600); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := session.Close(); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("first bounded close = %v", err)
	}
	for attempt := 0; attempt < 10; attempt++ {
		if err = session.Close(); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("cleanup retry did not converge: %v", err)
	}
	if _, err := os.Stat(workingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("management workdir remains: %v", err)
	}
	if client.closeCalls.Load() != 1 {
		t.Fatalf("client close calls = %d", client.closeCalls.Load())
	}
}

type fakeProtocolClient struct {
	startCalls       atomic.Int32
	initializeCalls  atomic.Int32
	closeCalls       atomic.Int32
	startErr         error
	initializeErr    error
	initializeResult *mcpsdk.InitializeResult
	toolsResult      *mcpsdk.ListToolsResult
	resourcesResult  *mcpsdk.ListResourcesResult
	promptsResult    *mcpsdk.ListPromptsResult
	callToolResult   *mcpsdk.CallToolResult
	callToolErr      error
}

func (c *fakeProtocolClient) Start(context.Context) error {
	c.startCalls.Add(1)
	return c.startErr
}

func (c *fakeProtocolClient) Initialize(context.Context, mcpsdk.InitializeRequest) (*mcpsdk.InitializeResult, error) {
	c.initializeCalls.Add(1)
	return c.initializeResult, c.initializeErr
}

func (c *fakeProtocolClient) ListToolsByPage(context.Context, mcpsdk.ListToolsRequest) (*mcpsdk.ListToolsResult, error) {
	return c.toolsResult, nil
}

func (c *fakeProtocolClient) ListResourcesByPage(context.Context, mcpsdk.ListResourcesRequest) (*mcpsdk.ListResourcesResult, error) {
	return c.resourcesResult, nil
}

func (c *fakeProtocolClient) ListPromptsByPage(context.Context, mcpsdk.ListPromptsRequest) (*mcpsdk.ListPromptsResult, error) {
	return c.promptsResult, nil
}

func (c *fakeProtocolClient) CallTool(context.Context, mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return c.callToolResult, c.callToolErr
}

func (c *fakeProtocolClient) Close() error {
	c.closeCalls.Add(1)
	return nil
}

func initializeResultWithCapabilities(t *testing.T, tools, resources, prompts bool) *mcpsdk.InitializeResult {
	t.Helper()
	payload := map[string]any{"protocolVersion": mcpsdk.LATEST_PROTOCOL_VERSION, "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
	capabilities := payload["capabilities"].(map[string]any)
	if tools {
		capabilities["tools"] = map[string]any{}
	}
	if resources {
		capabilities["resources"] = map[string]any{}
	}
	if prompts {
		capabilities["prompts"] = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal initialize result: %v", err)
	}
	var result mcpsdk.InitializeResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	return &result
}

func containsSecret(value string) bool {
	return strings.Contains(value, "raw-secret") || strings.Contains(value, "provider-secret")
}
