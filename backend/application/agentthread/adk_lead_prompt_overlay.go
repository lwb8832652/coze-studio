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
	"fmt"
	"strconv"
	"strings"

	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
)

const adkSingleAgentAssistantIDPrefix = "singleagent:"

type ADKLeadPromptOverlayProvider interface {
	ResolveADKLeadPromptOverlay(
		ctx context.Context,
		run *RunSummary,
	) (ADKLeadPromptOverlay, bool, error)
}

type ADKLeadPromptOverlayProviderFunc func(
	ctx context.Context,
	run *RunSummary,
) (ADKLeadPromptOverlay, bool, error)

func (f ADKLeadPromptOverlayProviderFunc) ResolveADKLeadPromptOverlay(
	ctx context.Context,
	run *RunSummary,
) (ADKLeadPromptOverlay, bool, error) {
	return f(ctx, run)
}

type ADKSingleAgentLeadPromptOverlayProvider struct {
	source ADKSingleAgentSubagentService
}

func NewADKSingleAgentLeadPromptOverlayProvider(
	source ADKSingleAgentSubagentService,
) *ADKSingleAgentLeadPromptOverlayProvider {
	return &ADKSingleAgentLeadPromptOverlayProvider{source: source}
}

func (p *ADKSingleAgentLeadPromptOverlayProvider) ResolveADKLeadPromptOverlay(
	ctx context.Context,
	run *RunSummary,
) (ADKLeadPromptOverlay, bool, error) {
	ref, found, err := adkSingleAgentLeadPromptReference(run)
	if err != nil || !found {
		return ADKLeadPromptOverlay{}, found, err
	}
	if p == nil || p.source == nil {
		return ADKLeadPromptOverlay{}, true, fmt.Errorf(
			"single agent lead prompt source is required",
		)
	}

	var snapshot *saEntity.SingleAgent
	if ref.IsDraft {
		agent, loadErr := p.source.GetSingleAgentDraft(ctx, ref.AgentID)
		if loadErr != nil {
			return ADKLeadPromptOverlay{}, true, fmt.Errorf(
				"load draft single agent lead prompt: %w",
				loadErr,
			)
		}
		snapshot = agent
	} else {
		agent, loadErr := p.source.GetSingleAgent(ctx, ref.AgentID, ref.Version)
		if loadErr != nil {
			return ADKLeadPromptOverlay{}, true, fmt.Errorf(
				"load versioned single agent lead prompt: %w",
				loadErr,
			)
		}
		snapshot = agent
	}
	if snapshot == nil || snapshot.SingleAgent == nil {
		return ADKLeadPromptOverlay{}, true, fmt.Errorf(
			"single agent lead prompt snapshot not found",
		)
	}
	agent := snapshot.SingleAgent
	if agent.AgentID != ref.AgentID {
		return ADKLeadPromptOverlay{}, true, fmt.Errorf(
			"single agent lead prompt snapshot id mismatch",
		)
	}
	if run == nil || run.SpaceID <= 0 || agent.SpaceID != run.SpaceID {
		return ADKLeadPromptOverlay{}, true, fmt.Errorf(
			"single agent lead prompt access denied",
		)
	}
	if ref.Version != "" && strings.TrimSpace(agent.Version) != ref.Version {
		return ADKLeadPromptOverlay{}, true, fmt.Errorf(
			"single agent lead prompt version mismatch",
		)
	}

	instructions := ""
	if agent.Prompt != nil {
		instructions = strings.TrimSpace(agent.Prompt.GetPrompt())
	}
	if _, normalizeErr := normalizeADKLeadPromptOverlay(
		"durable agent prompt",
		instructions,
	); normalizeErr != nil {
		return ADKLeadPromptOverlay{}, true, normalizeErr
	}
	if _, normalizeErr := normalizeADKLeadPromptLabel(
		"durable agent name",
		agent.Name,
		adkLeadPromptAgentNameMaxRunes,
	); normalizeErr != nil {
		return ADKLeadPromptOverlay{}, true, normalizeErr
	}
	if _, normalizeErr := normalizeADKLeadPromptLabel(
		"durable agent description",
		agent.Desc,
		adkLeadPromptAgentDescMaxRunes,
	); normalizeErr != nil {
		return ADKLeadPromptOverlay{}, true, normalizeErr
	}

	return ADKLeadPromptOverlay{
		AgentName:        strings.TrimSpace(agent.Name),
		AgentDescription: strings.TrimSpace(agent.Desc),
		Instructions:     instructions,
		ModelDefaults:    adkLeadPromptModelDefaults(snapshot),
	}, true, nil
}

type adkSingleAgentLeadPromptRef struct {
	AgentID int64
	Version string
	IsDraft bool
}

func adkSingleAgentLeadPromptReference(
	run *RunSummary,
) (adkSingleAgentLeadPromptRef, bool, error) {
	if run == nil {
		return adkSingleAgentLeadPromptRef{}, false, nil
	}
	assistantID := strings.TrimSpace(run.AssistantID)
	if !strings.HasPrefix(assistantID, adkSingleAgentAssistantIDPrefix) {
		return adkSingleAgentLeadPromptRef{}, false, nil
	}
	rawAgentID := strings.TrimSpace(strings.TrimPrefix(
		assistantID,
		adkSingleAgentAssistantIDPrefix,
	))
	agentID, err := strconv.ParseInt(rawAgentID, 10, 64)
	if err != nil || agentID <= 0 {
		return adkSingleAgentLeadPromptRef{}, true, fmt.Errorf(
			"invalid single agent assistant_id",
		)
	}

	ref := adkSingleAgentLeadPromptRef{AgentID: agentID, IsDraft: true}
	payload, err := parseDeerFlowRuntimePayload(run.Config)
	if err != nil {
		return adkSingleAgentLeadPromptRef{}, true, err
	}
	config, err := optionalDeerFlowConfigObject(payload, "single_agent")
	if err != nil {
		return adkSingleAgentLeadPromptRef{}, true, err
	}
	if config == nil {
		return ref, true, nil
	}
	configuredID, configured, err := optionalDeerFlowConfigInt(config, "agent_id")
	if err != nil {
		return adkSingleAgentLeadPromptRef{}, true, err
	}
	if configured && int64(configuredID) != agentID {
		return adkSingleAgentLeadPromptRef{}, true, fmt.Errorf(
			"single agent config agent_id does not match assistant_id",
		)
	}
	version, versionSet, err := optionalDeerFlowConfigString(config, "version")
	if err != nil {
		return adkSingleAgentLeadPromptRef{}, true, err
	}
	if versionSet && version != "" {
		ref.Version = version
		ref.IsDraft = false
	}
	isDraft, draftSet, err := optionalDeerFlowConfigBool(config, "is_draft")
	if err != nil {
		return adkSingleAgentLeadPromptRef{}, true, err
	}
	if draftSet {
		if isDraft && ref.Version != "" {
			return adkSingleAgentLeadPromptRef{}, true, fmt.Errorf(
				"single agent draft reference cannot include version",
			)
		}
		ref.IsDraft = isDraft
	}
	if !ref.IsDraft && ref.Version == "" {
		return adkSingleAgentLeadPromptRef{}, true, fmt.Errorf(
			"versioned single agent lead prompt requires version",
		)
	}
	return ref, true, nil
}

func adkLeadPromptModelDefaults(agent *saEntity.SingleAgent) modelExecutorConfig {
	defaults := modelExecutorConfig{}
	if agent == nil || agent.SingleAgent == nil || agent.ModelInfo == nil {
		return defaults
	}
	modelInfo := agent.ModelInfo
	if modelInfo.IsSetModelId() {
		defaults.ModelID = modelInfo.GetModelId()
	}
	if modelInfo.IsSetTemperature() {
		value := float32(modelInfo.GetTemperature())
		defaults.Temperature = &value
	}
	if modelInfo.IsSetMaxTokens() {
		value := int(modelInfo.GetMaxTokens())
		defaults.MaxTokens = &value
	}
	if modelInfo.IsSetTopP() {
		value := float32(modelInfo.GetTopP())
		defaults.TopP = &value
	}
	return defaults
}

func applyADKLeadPromptModelDefaults(
	target *modelExecutorConfig,
	defaults modelExecutorConfig,
) {
	if target == nil {
		return
	}
	if target.ModelID == 0 {
		target.ModelID = defaults.ModelID
	}
	if target.ModelName == "" {
		target.ModelName = defaults.ModelName
	}
	if target.Temperature == nil {
		target.Temperature = cloneFloat32(defaults.Temperature)
	}
	if target.MaxTokens == nil {
		target.MaxTokens = cloneInt(defaults.MaxTokens)
	}
	if target.TopP == nil {
		target.TopP = cloneFloat32(defaults.TopP)
	}
}

func cloneFloat32(value *float32) *float32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
