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

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

const (
	adkBuiltinGeneralPurposeName        = "general_purpose"
	adkBuiltinGeneralPurposeDescription = "Delegate an independent task to a general-purpose child agent for focused reasoning and execution."
	adkBuiltinGeneralPurposePrompt      = `You are a general-purpose subagent working on one delegated task.

Complete the delegated task autonomously and return a concise, actionable result.
Focus only on the assigned task, use available tools when needed, do not ask for clarification, and do not delegate to another subagent.`
)

type adkBuiltinSubagentDefinitionProvider struct {
	configured ADKSubagentDefinitionProvider
}

func newADKBuiltinSubagentDefinitionProvider(
	configured ADKSubagentDefinitionProvider,
) *adkBuiltinSubagentDefinitionProvider {
	return &adkBuiltinSubagentDefinitionProvider{configured: configured}
}

func (p *adkBuiltinSubagentDefinitionProvider) ResolveADKSubagents(
	ctx context.Context,
	run *RunSummary,
) ([]ADKSubagentDefinition, error) {
	definitions := make([]ADKSubagentDefinition, 0, 1)
	runtimeConfig, err := ParseDeerFlowRuntimeConfig("")
	if run != nil {
		runtimeConfig, err = ParseDeerFlowRuntimeConfig(run.Config)
	}
	if err != nil {
		return nil, err
	}
	if (runtimeConfig.ModeExplicit || runtimeConfig.SubagentExplicit) &&
		runtimeConfig.SubagentEnabled {
		definitions = append(definitions, builtinADKGeneralPurposeDefinition())
	}
	if p == nil || p.configured == nil {
		return definitions, nil
	}
	configured, err := p.configured.ResolveADKSubagents(ctx, run)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		seen[definition.Name] = struct{}{}
	}
	for _, definition := range configured {
		definition = normalizeADKSubagentDefinition(definition)
		if definition.Name == adkBuiltinGeneralPurposeName {
			return nil, fmt.Errorf("subagent name is reserved by a builtin: %s", definition.Name)
		}
		if _, exists := seen[definition.Name]; exists {
			return nil, fmt.Errorf("subagent name is reserved by a builtin: %s", definition.Name)
		}
		seen[definition.Name] = struct{}{}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func builtinADKGeneralPurposeDefinition() ADKSubagentDefinition {
	return ADKSubagentDefinition{
		Name:        adkBuiltinGeneralPurposeName,
		Description: adkBuiltinGeneralPurposeDescription,
	}
}

type adkBuiltinSubagentAgentFactory struct {
	factory ADKAgentFactory
}

type adkBuiltinSubagentToolProvider struct {
	base ADKToolProvider
}

func newADKBuiltinSubagentToolProvider(
	base ADKToolProvider,
) *adkBuiltinSubagentToolProvider {
	return &adkBuiltinSubagentToolProvider{base: base}
}

func (p *adkBuiltinSubagentToolProvider) ResolveToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	var base ADKToolProvider
	if p != nil {
		base = p.base
	}
	set, err := NewADKToolPolicyProvider(base).ResolveToolSet(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}
	set.StaticTools, err = filterADKBuiltinSubagentTools(ctx, set.StaticTools)
	if err != nil {
		return ADKToolSet{}, err
	}
	set.DynamicTools, err = filterADKBuiltinSubagentTools(ctx, set.DynamicTools)
	if err != nil {
		return ADKToolSet{}, err
	}
	activeStaticNames, err := adkToolNameSet(ctx, set.StaticTools)
	if err != nil {
		return ADKToolSet{}, err
	}
	set.SubagentToolNames = filterADKSubagentToolNames(
		set.SubagentToolNames,
		activeStaticNames,
	)
	return set, nil
}

func (p *adkBuiltinSubagentToolProvider) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}
	return set.StaticTools, nil
}

func (p *adkBuiltinSubagentToolProvider) ResolveDynamicTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}
	return set.DynamicTools, nil
}

func filterADKBuiltinSubagentTools(
	ctx context.Context,
	candidates []tool.BaseTool,
) ([]tool.BaseTool, error) {
	denied := map[string]struct{}{
		adkClarificationToolName:         {},
		adkDeerFlowClarificationToolName: {},
		adkConfirmationToolName:          {},
		adkPresentFilesToolName:          {},
	}
	filtered := make([]tool.BaseTool, 0, len(candidates))
	for _, candidate := range candidates {
		name, err := adkToolName(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if _, blocked := denied[name]; blocked {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered, nil
}

func newADKBuiltinSubagentAgentFactory(
	factory ADKAgentFactory,
) *adkBuiltinSubagentAgentFactory {
	return &adkBuiltinSubagentAgentFactory{factory: factory}
}

func (f *adkBuiltinSubagentAgentFactory) BuildADKSubagent(
	ctx context.Context,
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (adk.Agent, error) {
	if f == nil || f.factory == nil {
		return nil, fmt.Errorf("builtin subagent factory is required")
	}
	if !isADKBuiltinSubagent(definition) {
		return nil, fmt.Errorf("unsupported builtin subagent: %s", definition.Name)
	}
	child, err := buildADKBuiltinSubagentRunSummary(parent, definition)
	if err != nil {
		return nil, err
	}
	agent, err := f.factory.Build(ctx, child)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("builtin subagent factory returned empty agent")
	}
	return agent, nil
}

type adkConfiguredOrBuiltinSubagentAgentFactory struct {
	builtin    ADKSubagentAgentFactory
	configured ADKSubagentAgentFactory
}

func (f *adkConfiguredOrBuiltinSubagentAgentFactory) BuildADKSubagent(
	ctx context.Context,
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (adk.Agent, error) {
	if isADKBuiltinSubagent(definition) {
		if f == nil || f.builtin == nil {
			return nil, fmt.Errorf("builtin subagent factory is required")
		}
		return f.builtin.BuildADKSubagent(ctx, parent, definition)
	}
	if f == nil || f.configured == nil {
		return nil, fmt.Errorf("configured subagent factory is required")
	}
	return f.configured.BuildADKSubagent(ctx, parent, definition)
}

func isADKBuiltinSubagent(definition ADKSubagentDefinition) bool {
	return definition.AgentID == 0 && strings.TrimSpace(definition.Name) == adkBuiltinGeneralPurposeName
}

func buildADKBuiltinSubagentRunSummary(
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (*RunSummary, error) {
	definition = normalizeADKSubagentDefinition(definition)
	if !isADKBuiltinSubagent(definition) {
		return nil, fmt.Errorf("unsupported builtin subagent: %s", definition.Name)
	}
	if err := validateADKSubagentDefinition(definition); err != nil {
		return nil, err
	}

	child := &RunSummary{AssistantID: "builtin:" + definition.Name, RunKind: RunKindSubagent}
	if parent != nil {
		child.RunID = parent.RunID
		child.ParentRunID = parent.RunID
		child.PlanScopeRunID = parent.PlanScopeRunID
		child.ThreadID = parent.ThreadID
		child.SpaceID = parent.SpaceID
		child.CreatorID = parent.CreatorID
		child.StreamMode = parent.StreamMode
		child.MultitaskStrategy = parent.MultitaskStrategy
		child.OnDisconnect = parent.OnDisconnect
		child.Durability = parent.Durability
	}

	configPayload := map[string]any{}
	if parent != nil && strings.TrimSpace(parent.Config) != "" {
		var parentConfig map[string]any
		if err := json.Unmarshal([]byte(parent.Config), &parentConfig); err != nil {
			return nil, fmt.Errorf("parse parent config for builtin subagent: %w", err)
		}
		copyADKBuiltinSubagentModelConfig(configPayload, parentConfig)
		copyADKBuiltinSubagentToolConfig(configPayload, parentConfig)
	}
	configPayload["runtime"] = string(RuntimeModeEinoADK)
	configPayload["requested_policy"] = string(DeerFlowRequestedPolicyPro)
	configPayload["mode"] = string(DeerFlowModePro)
	configPayload["thinking_enabled"] = false
	configPayload["is_plan_mode"] = false
	configPayload["subagent_enabled"] = false
	configPayload["agent_name"] = definition.Name
	configPayload["agent_description"] = definition.Description
	configPayload["system_prompt"] = adkBuiltinGeneralPurposePrompt
	configBytes, err := json.Marshal(configPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal builtin subagent config: %w", err)
	}
	child.Config = string(configBytes)

	metadataPayload := map[string]any{"source": "builtin_subagent"}
	if subagent := adkChildSubagentIdentity(parent, definition.Name); subagent != nil {
		metadataPayload["subagent"] = subagent
	}
	metadataBytes, err := json.Marshal(metadataPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal builtin subagent metadata: %w", err)
	}
	child.Metadata = string(metadataBytes)
	return child, nil
}

func copyADKBuiltinSubagentModelConfig(target, source map[string]any) {
	for _, key := range []string{
		"model_id",
		"model_type",
		"model_name",
		"temperature",
		"max_tokens",
		"top_p",
	} {
		if value, exists := source[key]; exists {
			target[key] = value
		}
	}
}

func copyADKBuiltinSubagentToolConfig(target, source map[string]any) {
	for _, key := range []string{
		"web_tools",
		"mcp_tools",
		"tool_policy",
		"enable_mcp",
		"enable_skills",
		"skills",
	} {
		if value, exists := source[key]; exists {
			target[key] = value
		}
	}
}
