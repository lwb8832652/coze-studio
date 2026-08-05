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
	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
)

type ADKSingleAgentSubagentAgentFactory struct {
	source  ADKSingleAgentSubagentService
	factory ADKAgentFactory
}

func NewADKSingleAgentSubagentAgentFactory(
	source ADKSingleAgentSubagentService,
	factory ADKAgentFactory,
) *ADKSingleAgentSubagentAgentFactory {
	return &ADKSingleAgentSubagentAgentFactory{
		source:  source,
		factory: factory,
	}
}

func (f *ADKSingleAgentSubagentAgentFactory) BuildADKSubagent(
	ctx context.Context,
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (adk.Agent, error) {
	if f == nil {
		return nil, fmt.Errorf("single agent subagent factory is required")
	}
	if f.source == nil {
		return nil, fmt.Errorf("single agent subagent source is required")
	}

	ref := ADKSubagentReference{
		Name:                   definition.Name,
		Description:            definition.Description,
		AgentID:                definition.AgentID,
		Version:                definition.Version,
		IsDraft:                definition.IsDraft,
		FullChatHistoryAsInput: definition.FullChatHistoryAsInput,
		AllowedTools:           definition.AllowedTools,
		AllowedDynamicTools:    definition.AllowedDynamicTools,
	}
	ref = normalizeADKSubagentReference(ref)
	if err := validateADKSubagentReference(ref); err != nil {
		return nil, err
	}
	snapshot, err := loadADKSingleAgentSubagentSnapshot(ctx, f.source, ref)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || snapshot.SingleAgent == nil {
		return nil, fmt.Errorf("subagent not found: agent_id=%d", ref.AgentID)
	}

	definition = normalizeADKSubagentDefinition(ADKSubagentDefinition{
		Name:                   subagentToolNameFromReference(ref, snapshot),
		Description:            subagentDescriptionFromReference(ref, snapshot),
		AgentID:                ref.AgentID,
		Version:                subagentVersionFromReference(ref, snapshot),
		IsDraft:                ref.IsDraft,
		FullChatHistoryAsInput: ref.FullChatHistoryAsInput,
		AllowedTools:           ref.AllowedTools,
		AllowedDynamicTools:    ref.AllowedDynamicTools,
	})
	if err := validateADKSubagentDefinition(definition); err != nil {
		return nil, err
	}

	childRun, err := buildADKSingleAgentSubagentRunSummary(
		parent,
		definition,
		snapshot,
	)
	if err != nil {
		return nil, err
	}

	factory := f.factory
	if factory == nil {
		factory = NewApplicationADKAgentFactory(DefaultChatModelProvider, nil, nil)
	}
	agent, err := factory.Build(ctx, childRun)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("single agent subagent factory returned empty agent")
	}

	return agent, nil
}

func buildADKSingleAgentSubagentRunSummary(
	parent *RunSummary,
	definition ADKSubagentDefinition,
	snapshot *saEntity.SingleAgent,
) (*RunSummary, error) {
	if snapshot == nil || snapshot.SingleAgent == nil {
		return nil, fmt.Errorf("single agent subagent snapshot is required")
	}
	definition = normalizeADKSubagentDefinition(definition)
	if err := validateADKSubagentDefinition(definition); err != nil {
		return nil, err
	}

	child := &RunSummary{}
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
	child.AssistantID = fmt.Sprintf("singleagent:%d", definition.AgentID)
	child.RunKind = RunKindSubagent

	configPayload := map[string]any{
		"runtime":           string(RuntimeModeEinoADK),
		"requested_policy":  string(DeerFlowRequestedPolicyPro),
		"mode":              string(DeerFlowModePro),
		"thinking_enabled":  false,
		"is_plan_mode":      false,
		"subagent_enabled":  false,
		"agent_name":        definition.Name,
		"agent_description": definition.Description,
		"single_agent": map[string]any{
			"agent_id": definition.AgentID,
			"version":  definition.Version,
			"is_draft": definition.IsDraft,
		},
		"tool_policy": map[string]any{
			"allowed_tools":         normalizeConfigStringSlice(definition.AllowedTools),
			"allowed_dynamic_tools": normalizeConfigStringSlice(definition.AllowedDynamicTools),
		},
	}
	if snapshot.Prompt != nil {
		if prompt := strings.TrimSpace(snapshot.Prompt.GetPrompt()); prompt != "" {
			configPayload["system_prompt"] = prompt
		}
	}
	applyADKSingleAgentModelInfo(configPayload, snapshot)

	configBytes, err := json.Marshal(configPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal single agent subagent config: %w", err)
	}
	child.Config = string(configBytes)

	metadataPayload := map[string]any{
		"source":   "single_agent_subagent",
		"agent_id": definition.AgentID,
		"is_draft": definition.IsDraft,
	}
	if subagent := adkChildSubagentIdentity(parent, definition.Name); subagent != nil {
		metadataPayload["subagent"] = subagent
	}
	if definition.Version != "" {
		metadataPayload["version"] = definition.Version
	}
	metadataBytes, err := json.Marshal(metadataPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal single agent subagent metadata: %w", err)
	}
	child.Metadata = string(metadataBytes)

	return child, nil
}

func applyADKSingleAgentModelInfo(
	payload map[string]any,
	snapshot *saEntity.SingleAgent,
) {
	if payload == nil ||
		snapshot == nil ||
		snapshot.SingleAgent == nil ||
		snapshot.ModelInfo == nil {
		return
	}
	modelInfo := snapshot.ModelInfo
	if modelInfo.IsSetModelId() {
		payload["model_id"] = modelInfo.GetModelId()
	}
	if modelInfo.IsSetTemperature() {
		payload["temperature"] = modelInfo.GetTemperature()
	}
	if modelInfo.IsSetMaxTokens() {
		payload["max_tokens"] = modelInfo.GetMaxTokens()
	}
	if modelInfo.IsSetTopP() {
		payload["top_p"] = modelInfo.GetTopP()
	}
}

func loadADKSingleAgentSubagentSnapshot(
	ctx context.Context,
	source ADKSingleAgentSubagentService,
	ref ADKSubagentReference,
) (*saEntity.SingleAgent, error) {
	if source == nil {
		return nil, fmt.Errorf("single agent subagent source is required")
	}
	if ref.IsDraft {
		agent, err := source.GetSingleAgentDraft(ctx, ref.AgentID)
		if err != nil {
			return nil, fmt.Errorf("load draft subagent: %w", err)
		}
		return agent, nil
	}
	agent, err := source.GetSingleAgent(ctx, ref.AgentID, ref.Version)
	if err != nil {
		return nil, fmt.Errorf("load versioned subagent: %w", err)
	}
	return agent, nil
}
