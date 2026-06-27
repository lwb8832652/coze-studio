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

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

type ADKMCPToolRegistry interface {
	ListMCPToolRegistryEntries(
		ctx context.Context,
		spaceID int64,
	) ([]*toolapi.MCPToolRegistryEntry, error)
}

type ADKMCPRuntimeToolCall struct {
	Run       *RunSummary
	Name      string
	ServerID  int64
	ToolName  string
	Arguments string
}

type ADKMCPRuntimeToolExecutor interface {
	InvokeADKMCPRuntimeTool(
		ctx context.Context,
		call ADKMCPRuntimeToolCall,
	) (string, error)
}

type ADKMCPRuntimeToolCatalog struct {
	registry ADKMCPToolRegistry
	executor ADKMCPRuntimeToolExecutor
}

type ADKMCPRuntimeToolCatalogOption func(*ADKMCPRuntimeToolCatalog)

func WithADKMCPRuntimeToolExecutor(
	executor ADKMCPRuntimeToolExecutor,
) ADKMCPRuntimeToolCatalogOption {
	return func(catalog *ADKMCPRuntimeToolCatalog) {
		catalog.executor = executor
	}
}

func NewADKMCPRuntimeToolCatalog(
	registry ADKMCPToolRegistry,
	options ...ADKMCPRuntimeToolCatalogOption,
) *ADKMCPRuntimeToolCatalog {
	catalog := &ADKMCPRuntimeToolCatalog{registry: registry}
	for _, option := range options {
		if option != nil {
			option(catalog)
		}
	}

	return catalog
}

func (c *ADKMCPRuntimeToolCatalog) LoadADKRuntimeTools(
	ctx context.Context,
	run *RunSummary,
) ([]ADKRuntimeToolDefinition, error) {
	config, err := adkMCPToolConfigFromRun(run)
	if err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}
	if c == nil || c.registry == nil {
		return nil, fmt.Errorf("mcp tool registry is required")
	}
	if run == nil || run.SpaceID <= 0 {
		return nil, fmt.Errorf("space_id is required")
	}

	entries, err := c.registry.ListMCPToolRegistryEntries(ctx, run.SpaceID)
	if err != nil {
		return nil, err
	}

	definitions := make([]ADKRuntimeToolDefinition, 0, len(entries))
	for _, entry := range entries {
		definition := adkMCPRuntimeToolDefinitionFromEntry(
			entry,
			config,
			c.executor,
		)
		if definition != nil {
			definitions = append(definitions, *definition)
		}
	}

	return definitions, nil
}

type adkMCPToolConfig struct {
	Enabled      bool
	Visibility   ADKRuntimeToolVisibility
	AllowedTools map[string]struct{}
}

func adkMCPToolConfigFromRun(run *RunSummary) (adkMCPToolConfig, error) {
	config := adkMCPToolConfig{
		Visibility: ADKRuntimeToolVisibilityDeferred,
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return config, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return config, fmt.Errorf("parse mcp tools config: %w", err)
	}
	raw, ok := firstADKConfigObject(payload, "mcp_tools", "mcpTools")
	if !ok {
		return config, nil
	}
	config.Enabled = firstADKConfigBool(raw, "enabled")
	if !config.Enabled {
		return config, nil
	}
	if visibility := firstConfigString(raw, "visibility"); visibility != "" {
		switch ADKRuntimeToolVisibility(visibility) {
		case ADKRuntimeToolVisibilityStatic, ADKRuntimeToolVisibilityDeferred:
			config.Visibility = ADKRuntimeToolVisibility(visibility)
		default:
			return config, fmt.Errorf("unsupported mcp tool visibility: %s", visibility)
		}
	}
	allowed, allowedSet := configStringSetWithPresence(
		raw,
		"allowed_tools",
		"allowedTools",
	)
	if allowedSet {
		config.AllowedTools = allowed
	}

	return config, nil
}

func adkMCPRuntimeToolDefinitionFromEntry(
	entry *toolapi.MCPToolRegistryEntry,
	config adkMCPToolConfig,
	executor ADKMCPRuntimeToolExecutor,
) *ADKRuntimeToolDefinition {
	if entry == nil || !entry.Enabled {
		return nil
	}
	if strings.TrimSpace(entry.Source) != "mcp" {
		return nil
	}

	name := strings.TrimSpace(entry.Name)
	description := strings.TrimSpace(entry.Description)
	toolName := strings.TrimSpace(entry.ToolName)
	if !isADKSubagentToolName(name) || description == "" {
		return nil
	}
	if entry.ServerID <= 0 || toolName == "" {
		return nil
	}
	if config.AllowedTools != nil {
		if _, ok := config.AllowedTools[name]; !ok {
			return nil
		}
	}

	return &ADKRuntimeToolDefinition{
		Name:        name,
		Description: description,
		Visibility:  config.Visibility,
		Invoker: adkMCPRuntimeToolInvoker{
			name:     name,
			serverID: entry.ServerID,
			toolName: toolName,
			executor: executor,
		},
	}
}

type adkMCPRuntimeToolInvoker struct {
	name     string
	serverID int64
	toolName string
	executor ADKMCPRuntimeToolExecutor
}

func (i adkMCPRuntimeToolInvoker) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = strings.TrimSpace(i.name)
	}
	if name == "" {
		name = "mcp_tool"
	}
	if i.executor == nil {
		return "", fmt.Errorf("mcp runtime tool execution is not enabled: %s", name)
	}

	result, err := i.executor.InvokeADKMCPRuntimeTool(ctx, ADKMCPRuntimeToolCall{
		Run:       call.Run,
		Name:      name,
		ServerID:  i.serverID,
		ToolName:  i.toolName,
		Arguments: strings.TrimSpace(call.Arguments),
	})
	if err != nil {
		return "", fmt.Errorf("mcp runtime tool execution failed: %s", name)
	}

	return result, nil
}
