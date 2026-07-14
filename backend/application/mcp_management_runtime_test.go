// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	toolmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
	"github.com/coze-dev/coze-studio/backend/application/mcptool"
)

func TestMCPManagementRuntimeExecuteValidatesPersistedServerBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*toolmodel.MCPToolServer, *mcptool.RuntimeToolCall)
	}{
		{name: "cross space", mutate: func(server *toolmodel.MCPToolServer, call *mcptool.RuntimeToolCall) {
			server.SpaceID = call.SpaceID + 1
		}},
		{name: "disabled", mutate: func(server *toolmodel.MCPToolServer, _ *mcptool.RuntimeToolCall) { server.Enabled = false }},
		{name: "unknown tool", mutate: func(_ *toolmodel.MCPToolServer, call *mcptool.RuntimeToolCall) { call.ToolName = "missing" }},
		{name: "arguments must be object", mutate: func(_ *toolmodel.MCPToolServer, call *mcptool.RuntimeToolCall) { call.Arguments = `[]` }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := managementRuntimeServer()
			call := mcptool.RuntimeToolCall{SpaceID: 10, ServerID: 41, ToolName: "search", Arguments: `{}`}
			tt.mutate(server, &call)
			factory := &managementSessionFactory{session: &managementSession{capabilities: mcpruntime.CapabilityFlags{Tools: true}}}
			runtime := newMCPManagementRuntime(&managementResolver{server: server}, factory, mcpruntime.DefaultDiscoveryLimits())

			_, err := runtime.ExecuteMCPTool(context.Background(), call)
			if !errors.Is(err, ErrMCPManagementRuntimeFailed) {
				t.Fatalf("expected fail-closed validation, got %v", err)
			}
			if factory.openCalls != 0 {
				t.Fatalf("session opened before validation: %d", factory.openCalls)
			}
		})
	}
}

func TestMCPManagementRuntimeExecuteReturnsOnlyFixedSummary(t *testing.T) {
	t.Parallel()

	session := &managementSession{
		capabilities: mcpruntime.CapabilityFlags{Tools: true},
		callResult:   mcpruntime.ToolCallResult{IsError: false},
		rawSecret:    "provider-raw-secret",
	}
	factory := &managementSessionFactory{session: session}
	runtime := newMCPManagementRuntime(&managementResolver{server: managementRuntimeServer()}, factory, mcpruntime.DefaultDiscoveryLimits())

	result, err := runtime.ExecuteMCPTool(context.Background(), mcptool.RuntimeToolCall{
		SpaceID: 10, ServerID: 41, ToolName: "search", Arguments: `{"query":"coze"}`,
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if result.Status != "success" || result.Output != `{"result":"completed"}` || result.LatencyMs < 0 {
		t.Fatalf("unexpected safe result: %#v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(encoded), session.rawSecret) {
		t.Fatalf("raw provider result leaked: %s", encoded)
	}
	if session.callName != "search" || session.callArguments["query"] != "coze" {
		t.Fatalf("unexpected call: %q %#v", session.callName, session.callArguments)
	}
	if session.closeCalls != 1 {
		t.Fatalf("session close calls = %d", session.closeCalls)
	}
}

func TestMCPManagementRuntimeSanitizesSessionErrors(t *testing.T) {
	t.Parallel()

	session := &managementSession{callErr: errors.New("transport token raw-secret")}
	runtime := newMCPManagementRuntime(
		&managementResolver{server: managementRuntimeServer()},
		&managementSessionFactory{session: session},
		mcpruntime.DefaultDiscoveryLimits(),
	)
	_, err := runtime.ExecuteMCPTool(context.Background(), mcptool.RuntimeToolCall{
		SpaceID: 10, ServerID: 41, ToolName: "search", Arguments: `{}`,
	})
	if !errors.Is(err, ErrMCPManagementRuntimeFailed) {
		t.Fatalf("expected sanitized error, got %v", err)
	}
	if strings.Contains(err.Error(), "raw-secret") {
		t.Fatalf("adapter leaked runtime error: %v", err)
	}
}

func TestMCPManagementRuntimeDiscoverMapsOnlyBoundedDefinitions(t *testing.T) {
	t.Parallel()

	session := &managementSession{
		capabilities: mcpruntime.CapabilityFlags{Tools: true, Resources: true, Prompts: true},
		toolPage:     mcpruntime.ToolPage{Items: []mcpruntime.Tool{{Name: "search", Description: "Search", InputSchema: []byte(`{"type":"object"}`)}}},
		resourcePage: mcpruntime.ResourcePage{Items: []mcpruntime.Resource{{URI: "resource://guide", Name: "guide", MIMEType: "text/plain"}}},
		promptPage:   mcpruntime.PromptPage{Items: []mcpruntime.Prompt{{Name: "summarize", Arguments: []mcpruntime.PromptArgument{{Name: "topic", Required: true}}}}},
	}
	runtime := newMCPManagementRuntime(
		&managementResolver{server: managementRuntimeServer()},
		&managementSessionFactory{session: session},
		mcpruntime.DiscoveryLimits{MaxItems: 10, MaxBytes: 4096, MaxPages: 10},
	)

	result, err := runtime.Discover(context.Background(), mcptool.MCPServerConnection{
		ServerType: "sse",
		Config:     `{"url":"https://mcp.example.com/events"}`,
		Auth:       `{"token":"secret"}`,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].InputSchema != `{"type":"object"}` {
		t.Fatalf("tool mapping = %#v", result.Tools)
	}
	if len(result.Resources) != 1 || result.Resources[0].MIMEType != "text/plain" {
		t.Fatalf("resource mapping = %#v", result.Resources)
	}
	if len(result.Prompts) != 1 || !result.Prompts[0].Arguments[0].Required {
		t.Fatalf("prompt mapping = %#v", result.Prompts)
	}
	if session.closeCalls != 1 {
		t.Fatalf("session close calls = %d", session.closeCalls)
	}
}

func TestMCPManagementRuntimeProductionBinding(t *testing.T) {
	t.Parallel()

	t.Run("valid config binds same adapter once", func(t *testing.T) {
		target := &managementBindingTarget{managementResolver: managementResolver{server: managementRuntimeServer()}}
		adapter, err := bindMCPManagementRuntime(target, agentthread.ADKMCPRuntimeBootstrapConfig{
			Enabled:                true,
			RemoteEinoEnabled:      true,
			RemoteAllowedHosts:     []string{"mcp.example.com"},
			RemoteMaxConfigBytes:   4096,
			RemoteMaxHeaders:       8,
			RemoteMaxHeaderBytes:   2048,
			ExecutorTimeout:        time.Second,
			ExecutorMaxOutputBytes: 4096,
		})
		if err != nil {
			t.Fatalf("bind management runtime: %v", err)
		}
		if adapter == nil || adapter.managementFactory == nil {
			t.Fatal("binding did not retain concrete closable management factory")
		}
		if target.bindCalls != 1 || target.executor == nil || target.discoverer == nil {
			t.Fatalf("binding not installed: %#v", target)
		}
		if reflect.ValueOf(target.executor).Pointer() != reflect.ValueOf(target.discoverer).Pointer() {
			t.Fatalf("executor and discoverer must be the same adapter")
		}
	})

	t.Run("missing config remains unbound", func(t *testing.T) {
		target := &managementBindingTarget{managementResolver: managementResolver{server: managementRuntimeServer()}}
		adapter, err := bindMCPManagementRuntime(target, agentthread.ADKMCPRuntimeBootstrapConfig{})
		if err != nil {
			t.Fatalf("missing config: %v", err)
		}
		if adapter != nil {
			t.Fatal("missing config returned management runtime")
		}
		if target.bindCalls != 0 {
			t.Fatalf("unsafe missing config was bound")
		}
	})

	t.Run("production host stdio is rejected", func(t *testing.T) {
		target := &managementBindingTarget{managementResolver: managementResolver{server: managementRuntimeServer()}}
		adapter, err := bindMCPManagementRuntime(target, agentthread.ADKMCPRuntimeBootstrapConfig{
			Enabled:                        true,
			AppEnv:                         "production",
			StdioEinoEnabled:               true,
			StdioDebugHostExecutionEnabled: true,
			StdioWorkdirRoot:               t.TempDir(),
			StdioAllowedCommands:           []string{"/bin/echo"},
			StdioCommandRules: []mcpruntime.StdioCommandRule{{
				Command:      "/bin/echo",
				AllowAnyArgs: true,
			}},
			ExecutorTimeout: time.Second,
		})
		if err == nil || !strings.Contains(err.Error(), "host execution") {
			t.Fatalf("production stdio must fail closed, got %v", err)
		}
		if adapter != nil {
			t.Fatal("rejected production stdio returned management runtime")
		}
		if target.bindCalls != 0 {
			t.Fatalf("production host stdio was bound")
		}
	})

	t.Run("binding failure blocks startup", func(t *testing.T) {
		target := &managementBindingTarget{
			managementResolver: managementResolver{server: managementRuntimeServer()},
			bindErr:            errors.New("already bound"),
		}
		adapter, err := bindMCPManagementRuntime(target, agentthread.ADKMCPRuntimeBootstrapConfig{
			Enabled:                true,
			RemoteEinoEnabled:      true,
			RemoteAllowedHosts:     []string{"mcp.example.com"},
			RemoteMaxConfigBytes:   4096,
			RemoteMaxHeaders:       8,
			RemoteMaxHeaderBytes:   2048,
			ExecutorTimeout:        time.Second,
			ExecutorMaxOutputBytes: 4096,
		})
		if !errors.Is(err, target.bindErr) {
			t.Fatalf("binding failure must propagate, got %v", err)
		}
		if adapter != nil {
			t.Fatal("failed binding returned management runtime")
		}
	})
}

type managementResolver struct {
	server *toolmodel.MCPToolServer
	err    error
}

func (r *managementResolver) ResolveADKMCPRuntimeServer(context.Context, int64) (*toolmodel.MCPToolServer, error) {
	return r.server, r.err
}

type managementSessionFactory struct {
	session    mcpruntime.Session
	err        error
	openCalls  int
	connection mcpruntime.Connection
}

func (f *managementSessionFactory) Open(_ context.Context, connection mcpruntime.Connection) (mcpruntime.Session, error) {
	f.openCalls++
	f.connection = connection
	return f.session, f.err
}

type managementSession struct {
	capabilities  mcpruntime.CapabilityFlags
	toolPage      mcpruntime.ToolPage
	resourcePage  mcpruntime.ResourcePage
	promptPage    mcpruntime.PromptPage
	callResult    mcpruntime.ToolCallResult
	callErr       error
	callName      string
	callArguments map[string]any
	closeCalls    int
	rawSecret     string
}

func (s *managementSession) Capabilities() mcpruntime.CapabilityFlags { return s.capabilities }
func (s *managementSession) ListToolsByPage(context.Context, string) (mcpruntime.ToolPage, error) {
	return s.toolPage, nil
}
func (s *managementSession) ListResourcesByPage(context.Context, string) (mcpruntime.ResourcePage, error) {
	return s.resourcePage, nil
}
func (s *managementSession) ListPromptsByPage(context.Context, string) (mcpruntime.PromptPage, error) {
	return s.promptPage, nil
}
func (s *managementSession) CallTool(_ context.Context, name string, arguments map[string]any) (mcpruntime.ToolCallResult, error) {
	s.callName = name
	s.callArguments = arguments
	return s.callResult, s.callErr
}
func (s *managementSession) Close() error { s.closeCalls++; return nil }

type managementBindingTarget struct {
	managementResolver
	bindCalls  int
	executor   mcptool.RuntimeExecutor
	discoverer mcptool.CapabilityDiscoverer
	bindErr    error
}

func (t *managementBindingTarget) BindManagementRuntime(executor mcptool.RuntimeExecutor, discoverer mcptool.CapabilityDiscoverer) error {
	t.bindCalls++
	t.executor = executor
	t.discoverer = discoverer
	return t.bindErr
}

func managementRuntimeServer() *toolmodel.MCPToolServer {
	return &toolmodel.MCPToolServer{
		ServerID:   41,
		SpaceID:    10,
		Enabled:    true,
		ServerType: "sse",
		Config:     `{"url":"https://mcp.example.com/events"}`,
		Auth:       `{}`,
		Tools:      []*toolmodel.MCPToolDefinition{{Name: "search", InputSchema: `{"type":"object"}`}},
	}
}
