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

	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
)

type ADKSubagentReference struct {
	Name                   string
	Description            string
	AgentID                int64
	Version                string
	IsDraft                bool
	FullChatHistoryAsInput bool
	AllowedTools           []string
	AllowedDynamicTools    []string
}

type ADKSubagentReferenceProvider interface {
	ResolveADKSubagentReferences(
		ctx context.Context,
		run *RunSummary,
	) ([]ADKSubagentReference, error)
}

type ADKSubagentReferenceProviderFunc func(
	ctx context.Context,
	run *RunSummary,
) ([]ADKSubagentReference, error)

func (f ADKSubagentReferenceProviderFunc) ResolveADKSubagentReferences(
	ctx context.Context,
	run *RunSummary,
) ([]ADKSubagentReference, error) {
	if f == nil {
		return nil, nil
	}
	return f(ctx, run)
}

type ADKRunConfigSubagentReferenceProvider struct{}

func NewADKRunConfigSubagentReferenceProvider() *ADKRunConfigSubagentReferenceProvider {
	return &ADKRunConfigSubagentReferenceProvider{}
}

func (p *ADKRunConfigSubagentReferenceProvider) ResolveADKSubagentReferences(
	_ context.Context,
	run *RunSummary,
) ([]ADKSubagentReference, error) {
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return nil, fmt.Errorf("parse subagent refs config: %w", err)
	}
	rawItems, ok := firstADKConfigArray(payload, "subagent_refs", "subagentRefs")
	if !ok || len(rawItems) == 0 {
		return nil, nil
	}
	refs := make([]ADKSubagentReference, 0, len(rawItems))
	seen := map[string]struct{}{}
	for _, rawItem := range rawItems {
		item, ok := configObject(rawItem)
		if !ok {
			return nil, fmt.Errorf("subagent reference must be an object")
		}
		ref := normalizeADKSubagentReference(ADKSubagentReference{
			Name:        firstConfigString(item, "name"),
			Description: firstConfigString(item, "description", "desc"),
			AgentID:     firstConfigInt64(item, "agent_id", "agentId"),
			Version:     firstConfigString(item, "version"),
			IsDraft:     firstADKConfigBool(item, "is_draft", "isDraft"),
			AllowedTools: firstConfigStringSlice(
				item,
				"allowed_tools",
				"allowedTools",
			),
			AllowedDynamicTools: firstConfigStringSlice(
				item,
				"allowed_dynamic_tools",
				"allowedDynamicTools",
			),
			FullChatHistoryAsInput: firstADKConfigBool(
				item,
				"full_chat_history",
				"fullChatHistory",
			),
		})
		if err := validateADKSubagentReference(ref); err != nil {
			return nil, err
		}
		if ref.Name != "" {
			if _, ok := seen[ref.Name]; ok {
				return nil, fmt.Errorf(
					"duplicate subagent tool name: %s",
					ref.Name,
				)
			}
			seen[ref.Name] = struct{}{}
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

type ADKSingleAgentSubagentService interface {
	GetSingleAgentDraft(
		ctx context.Context,
		agentID int64,
	) (*saEntity.SingleAgent, error)
	GetSingleAgent(
		ctx context.Context,
		agentID int64,
		version string,
	) (*saEntity.SingleAgent, error)
}

type ADKSubagentToolGrant struct {
	AllowedTools        []string
	AllowedDynamicTools []string
}

type ADKSubagentToolGrantRequest struct {
	Parent     *RunSummary
	Reference  ADKSubagentReference
	Definition ADKSubagentDefinition
	Agent      *saEntity.SingleAgent
}

type ADKSubagentToolGrantProvider interface {
	ResolveADKSubagentToolGrant(
		ctx context.Context,
		request ADKSubagentToolGrantRequest,
	) (ADKSubagentToolGrant, error)
}

type ADKSubagentToolGrantProviderFunc func(
	ctx context.Context,
	request ADKSubagentToolGrantRequest,
) (ADKSubagentToolGrant, error)

func (f ADKSubagentToolGrantProviderFunc) ResolveADKSubagentToolGrant(
	ctx context.Context,
	request ADKSubagentToolGrantRequest,
) (ADKSubagentToolGrant, error) {
	if f == nil {
		return ADKSubagentToolGrant{}, nil
	}
	return f(ctx, request)
}

type ADKSingleAgentSubagentDefinitionProvider struct {
	source     ADKSingleAgentSubagentService
	references ADKSubagentReferenceProvider
	grants     ADKSubagentToolGrantProvider
}

func NewADKSingleAgentSubagentDefinitionProvider(
	source ADKSingleAgentSubagentService,
	references ADKSubagentReferenceProvider,
) *ADKSingleAgentSubagentDefinitionProvider {
	return &ADKSingleAgentSubagentDefinitionProvider{
		source:     source,
		references: references,
	}
}

func NewADKSingleAgentSubagentDefinitionProviderWithToolGrants(
	source ADKSingleAgentSubagentService,
	references ADKSubagentReferenceProvider,
	grants ADKSubagentToolGrantProvider,
) *ADKSingleAgentSubagentDefinitionProvider {
	return &ADKSingleAgentSubagentDefinitionProvider{
		source:     source,
		references: references,
		grants:     grants,
	}
}

func (p *ADKSingleAgentSubagentDefinitionProvider) ResolveADKSubagents(
	ctx context.Context,
	run *RunSummary,
) ([]ADKSubagentDefinition, error) {
	if p == nil || p.source == nil {
		return nil, fmt.Errorf("single agent subagent source is required")
	}
	if p.references == nil {
		return nil, nil
	}
	refs, err := p.references.ResolveADKSubagentReferences(ctx, run)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}

	definitions := make([]ADKSubagentDefinition, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		ref = normalizeADKSubagentReference(ref)
		if err := validateADKSubagentReference(ref); err != nil {
			return nil, err
		}
		agent, err := p.loadSingleAgent(ctx, ref)
		if err != nil {
			return nil, err
		}
		if agent == nil || agent.SingleAgent == nil {
			return nil, fmt.Errorf(
				"subagent not found: agent_id=%d",
				ref.AgentID,
			)
		}

		definition := normalizeADKSubagentDefinition(ADKSubagentDefinition{
			Name:                   subagentToolNameFromReference(ref, agent),
			Description:            subagentDescriptionFromReference(ref, agent),
			AgentID:                ref.AgentID,
			Version:                subagentVersionFromReference(ref, agent),
			IsDraft:                ref.IsDraft,
			FullChatHistoryAsInput: ref.FullChatHistoryAsInput,
			AllowedTools:           ref.AllowedTools,
			AllowedDynamicTools:    ref.AllowedDynamicTools,
		})
		if p.grants != nil {
			grant, err := p.grants.ResolveADKSubagentToolGrant(
				ctx,
				ADKSubagentToolGrantRequest{
					Parent:     run,
					Reference:  ref,
					Definition: definition,
					Agent:      agent,
				},
			)
			if err != nil {
				return nil, fmt.Errorf(
					"resolve subagent tool grant %s: %w",
					definition.Name,
					err,
				)
			}
			definition = applyADKSubagentToolGrant(definition, grant)
		}
		if err := validateADKSubagentDefinition(definition); err != nil {
			return nil, err
		}
		if _, ok := seen[definition.Name]; ok {
			return nil, fmt.Errorf(
				"duplicate subagent tool name: %s",
				definition.Name,
			)
		}
		seen[definition.Name] = struct{}{}
		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func (p *ADKSingleAgentSubagentDefinitionProvider) loadSingleAgent(
	ctx context.Context,
	ref ADKSubagentReference,
) (*saEntity.SingleAgent, error) {
	return loadADKSingleAgentSubagentSnapshot(ctx, p.source, ref)
}

func normalizeADKSubagentReference(ref ADKSubagentReference) ADKSubagentReference {
	ref.Name = strings.TrimSpace(ref.Name)
	ref.Description = strings.TrimSpace(ref.Description)
	ref.Version = strings.TrimSpace(ref.Version)
	ref.AllowedTools = normalizeConfigStringSlice(ref.AllowedTools)
	ref.AllowedDynamicTools = normalizeConfigStringSlice(ref.AllowedDynamicTools)
	return ref
}

func validateADKSubagentReference(ref ADKSubagentReference) error {
	if ref.AgentID <= 0 {
		return fmt.Errorf("subagent agent_id is required")
	}
	if ref.Name != "" && !isADKSubagentToolName(ref.Name) {
		return fmt.Errorf("invalid subagent tool name: %s", ref.Name)
	}
	return nil
}

func applyADKSubagentToolGrant(
	definition ADKSubagentDefinition,
	grant ADKSubagentToolGrant,
) ADKSubagentDefinition {
	grant.AllowedTools = normalizeConfigStringSlice(grant.AllowedTools)
	grant.AllowedDynamicTools = normalizeConfigStringSlice(
		grant.AllowedDynamicTools,
	)
	definition.AllowedTools = mergeRequestedAndGrantedToolNames(
		definition.AllowedTools,
		grant.AllowedTools,
	)
	definition.AllowedDynamicTools = mergeRequestedAndGrantedToolNames(
		definition.AllowedDynamicTools,
		grant.AllowedDynamicTools,
	)
	return normalizeADKSubagentDefinition(definition)
}

func mergeRequestedAndGrantedToolNames(
	requested []string,
	granted []string,
) []string {
	requested = normalizeConfigStringSlice(requested)
	granted = normalizeConfigStringSlice(granted)
	if len(requested) == 0 {
		return granted
	}
	if len(granted) == 0 {
		return []string{}
	}
	grantSet := make(map[string]struct{}, len(granted))
	for _, name := range granted {
		grantSet[name] = struct{}{}
	}
	merged := make([]string, 0, len(requested))
	for _, name := range requested {
		if _, ok := grantSet[name]; ok {
			merged = append(merged, name)
		}
	}
	return merged
}

func subagentToolNameFromReference(
	ref ADKSubagentReference,
	agent *saEntity.SingleAgent,
) string {
	if ref.Name != "" {
		return ref.Name
	}
	if agent != nil && agent.SingleAgent != nil &&
		isADKSubagentToolName(agent.Name) {
		return agent.Name
	}
	if ref.AgentID > 0 {
		return fmt.Sprintf("agent_%d", ref.AgentID)
	}
	return ""
}

func subagentDescriptionFromReference(
	ref ADKSubagentReference,
	agent *saEntity.SingleAgent,
) string {
	if ref.Description != "" {
		return ref.Description
	}
	if agent == nil || agent.SingleAgent == nil {
		return ""
	}
	return agent.Desc
}

func subagentVersionFromReference(
	ref ADKSubagentReference,
	agent *saEntity.SingleAgent,
) string {
	if ref.Version != "" {
		return ref.Version
	}
	if agent == nil || agent.SingleAgent == nil {
		return ""
	}
	return agent.Version
}

func firstADKConfigArray(payload map[string]any, keys ...string) ([]any, bool) {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if typed, ok := value.([]any); ok {
			return typed, true
		}
	}
	return nil, false
}
