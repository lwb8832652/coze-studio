/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type ToolCall struct {
	Name      string
	Arguments string
	Run       *RunSummary
	State     AgentHarnessState
}

type ToolResult struct {
	Content  string
	Metadata string
}

type AgentTool interface {
	Call(ctx context.Context, call ToolCall) (*ToolResult, error)
}

type AgentToolFunc func(ctx context.Context, call ToolCall) (*ToolResult, error)

func (f AgentToolFunc) Call(ctx context.Context, call ToolCall) (*ToolResult, error) {
	if f == nil {
		return nil, fmt.Errorf("tool handler is required")
	}

	return f(ctx, call)
}

type ToolRegistry interface {
	Register(name string, tool AgentTool) error
	Lookup(name string) (AgentTool, bool)
}

type InMemoryToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]AgentTool
}

func NewInMemoryToolRegistry() *InMemoryToolRegistry {
	return &InMemoryToolRegistry{
		tools: make(map[string]AgentTool),
	}
}

func (r *InMemoryToolRegistry) Register(name string, tool AgentTool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if tool == nil {
		return fmt.Errorf("tool handler is required")
	}
	if r == nil {
		return fmt.Errorf("tool registry is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tools == nil {
		r.tools = make(map[string]AgentTool)
	}
	r.tools[name] = tool

	return nil
}

func (r *InMemoryToolRegistry) Lookup(name string) (AgentTool, bool) {
	name = strings.TrimSpace(name)
	if r == nil || name == "" {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]

	return tool, ok
}

type ToolStepRunner struct {
	registry ToolRegistry
}

func NewToolStepRunner(registry ToolRegistry) *ToolStepRunner {
	return &ToolStepRunner{registry: registry}
}

func (r *ToolStepRunner) RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error) {
	if r == nil || r.registry == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	if step.Type != AgentStepTypeTool {
		return nil, fmt.Errorf("unsupported agent step type: %s", step.Type)
	}

	toolName := agentStepToolName(step)
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	tool, ok := r.registry.Lookup(toolName)
	if !ok || tool == nil {
		return nil, fmt.Errorf("unsupported agent tool: %s", toolName)
	}

	result, err := tool.Call(ctx, ToolCall{
		Name:      toolName,
		Arguments: strings.TrimSpace(step.ToolArguments),
		Run:       run,
		State:     cloneHarnessState(state),
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("agent tool returned empty result: %s", toolName)
	}

	return &AgentStepResult{
		Message:  strings.TrimSpace(result.Content),
		Metadata: toolResultMetadata(toolName, result.Metadata),
		Final:    step.Final,
	}, nil
}

func toolResultMetadata(toolName, rawMetadata string) string {
	payload := map[string]any{
		"source":    "tool_runner",
		"tool_name": toolName,
	}
	if strings.TrimSpace(rawMetadata) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(rawMetadata), &metadata); err == nil {
			payload["tool_metadata"] = metadata
		} else {
			payload["tool_metadata"] = rawMetadata
		}
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return `{"source":"tool_runner"}`
	}

	return string(bytes)
}
