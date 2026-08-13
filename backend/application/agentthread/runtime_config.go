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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidRuntimeConfig = errors.New("invalid agent runtime config")

type DeerFlowMode string

type DeerFlowRequestedPolicy string

const (
	DeerFlowModePro   DeerFlowMode = "pro"
	DeerFlowModeUltra DeerFlowMode = "ultra"

	DeerFlowRequestedPolicyAuto  DeerFlowRequestedPolicy = "auto"
	DeerFlowRequestedPolicyPro   DeerFlowRequestedPolicy = "pro"
	DeerFlowRequestedPolicyUltra DeerFlowRequestedPolicy = "ultra"

	defaultDeerFlowMaxConcurrentSubagents = 3
	minDeerFlowMaxConcurrentSubagents     = 2
	maxDeerFlowMaxConcurrentSubagents     = 4
)

var deerFlowRuntimeContextKeys = []string{
	"model_name",
	"requested_policy",
	"mode",
	"thinking_enabled",
	"reasoning_effort",
	"is_plan_mode",
	"subagent_enabled",
	"max_concurrent_subagents",
	"agent_name",
	"is_bootstrap",
}

type DeerFlowRuntimeConfig struct {
	Runtime                 RuntimeMode
	RequestedPolicy         DeerFlowRequestedPolicy
	Mode                    DeerFlowMode
	ThinkingEnabled         bool
	ReasoningEffort         string
	IsPlanMode              bool
	SubagentEnabled         bool
	MaxConcurrentSubagents  int
	RequestedPolicyExplicit bool
	ModeExplicit            bool
	ThinkingExplicit        bool
	ReasoningEffortExplicit bool
	PlanModeExplicit        bool
	SubagentExplicit        bool
	SubagentMaximumExplicit bool
	resolved                bool
}

func ParseDeerFlowRuntimeConfig(rawConfig string) (DeerFlowRuntimeConfig, error) {
	payload, err := parseDeerFlowRuntimePayload(rawConfig)
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}

	config := defaultDeerFlowRuntimeConfig(DeerFlowModeUltra)
	config.RequestedPolicy = DeerFlowRequestedPolicyAuto
	config.resolved = true

	runtimeValue, runtimeSet, err := optionalDeerFlowConfigString(payload, "runtime")
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}
	if runtimeSet && runtimeValue != "" {
		config.Runtime = RuntimeMode(strings.ToLower(runtimeValue))
		switch config.Runtime {
		case RuntimeModeLegacy, RuntimeModeEinoADK:
		default:
			return DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
				"unsupported agent runtime: %s",
				config.Runtime,
			)
		}
	}

	requestedPolicyValue, requestedPolicySet, err := optionalDeerFlowConfigString(
		payload,
		"requested_policy",
	)
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}
	if requestedPolicySet && requestedPolicyValue != "" {
		requestedPolicy, resolvedMode, normalizeErr := normalizeDeerFlowRequestedPolicy(
			requestedPolicyValue,
		)
		if normalizeErr != nil {
			return DeerFlowRuntimeConfig{}, normalizeErr
		}
		config = defaultDeerFlowRuntimeConfig(resolvedMode)
		config.Runtime = RuntimeMode(strings.ToLower(runtimeValue))
		config.RequestedPolicy = requestedPolicy
		config.RequestedPolicyExplicit = true
		config.ModeExplicit = true
		config.resolved = true
	}

	modeValue, modeSet, err := optionalDeerFlowConfigString(payload, "mode")
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}
	if modeSet && modeValue != "" {
		mode, normalizeErr := normalizeDeerFlowMode(modeValue)
		if normalizeErr != nil {
			return DeerFlowRuntimeConfig{}, normalizeErr
		}
		if config.RequestedPolicyExplicit && mode != config.Mode {
			return DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
				"mode %s conflicts with requested_policy %s",
				mode,
				config.RequestedPolicy,
			)
		}
		if !config.RequestedPolicyExplicit {
			config = defaultDeerFlowRuntimeConfig(mode)
			config.Runtime = RuntimeMode(strings.ToLower(runtimeValue))
			config.RequestedPolicy = requestedPolicyForMode(mode)
		}
		config.ModeExplicit = true
		config.resolved = true
	}

	if value, set, boolErr := optionalDeerFlowConfigBool(payload, "thinking_enabled"); boolErr != nil {
		return DeerFlowRuntimeConfig{}, boolErr
	} else if set {
		config.ThinkingEnabled = value
		config.ThinkingExplicit = true
	}
	if value, set, boolErr := optionalDeerFlowConfigBool(payload, "is_plan_mode"); boolErr != nil {
		return DeerFlowRuntimeConfig{}, boolErr
	} else if set {
		config.IsPlanMode = value
		config.PlanModeExplicit = true
	}
	if value, set, boolErr := optionalDeerFlowConfigBool(payload, "subagent_enabled"); boolErr != nil {
		return DeerFlowRuntimeConfig{}, boolErr
	} else if set {
		config.SubagentEnabled = value
		config.SubagentExplicit = true
	}

	effort, effortSet, err := optionalDeerFlowConfigString(payload, "reasoning_effort")
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}
	if effortSet {
		effort = strings.ToLower(effort)
		if effort != "" && !validADKReasoningEffort(effort) {
			return DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
				"unsupported reasoning_effort: %s",
				effort,
			)
		}
		config.ReasoningEffort = effort
		config.ReasoningEffortExplicit = true
	}

	maximum, maximumSet, err := optionalDeerFlowConfigInt(payload, "max_concurrent_subagents")
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}
	if maximumSet {
		if maximum < minDeerFlowMaxConcurrentSubagents || maximum > maxDeerFlowMaxConcurrentSubagents {
			return DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
				"max_concurrent_subagents must be between %d and %d",
				minDeerFlowMaxConcurrentSubagents,
				maxDeerFlowMaxConcurrentSubagents,
			)
		}
		config.MaxConcurrentSubagents = maximum
		config.SubagentMaximumExplicit = true
	}
	if config.SubagentEnabled && config.MaxConcurrentSubagents == 0 {
		config.MaxConcurrentSubagents = defaultDeerFlowMaxConcurrentSubagents
	}

	return config, nil
}

func parseADKRuntimeConfig(rawConfig string) (DeerFlowRuntimeConfig, error) {
	payload, err := parseDeerFlowRuntimePayload(rawConfig)
	if err != nil {
		return DeerFlowRuntimeConfig{}, err
	}
	delete(payload, "requested_policy")
	delete(payload, "mode")

	projected, err := json.Marshal(payload)
	if err != nil {
		return DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
			"encode mode-free runtime config: %v",
			err,
		)
	}
	return ParseDeerFlowRuntimeConfig(string(projected))
}

func (c DeerFlowRuntimeConfig) PlanCapabilityEnabled() bool {
	if c.ModeExplicit || c.PlanModeExplicit {
		return c.IsPlanMode
	}
	return true
}

func (c DeerFlowRuntimeConfig) SubagentCapabilityEnabled() bool {
	if c.ModeExplicit || c.SubagentExplicit {
		return c.SubagentEnabled
	}
	return true
}

func (c DeerFlowRuntimeConfig) ExecutionReasoningRequest() ADKReasoningRequest {
	if !c.ModeExplicit && !c.ThinkingExplicit && !c.ReasoningEffortExplicit {
		return ADKReasoningRequest{}
	}
	effort := c.ReasoningEffort
	if !c.ModeExplicit && !c.ReasoningEffortExplicit {
		effort = ""
	}
	thinkingEnabled := c.ThinkingEnabled
	if !c.ModeExplicit && !c.ThinkingExplicit {
		thinkingEnabled = false
	}
	return ADKReasoningRequest{
		ReasoningEffort: effort,
		ThinkingEnabled: thinkingEnabled,
	}
}

func (c DeerFlowRuntimeConfig) ExecutionReasoningRequestOr(
	fallback ADKReasoningRequest,
) ADKReasoningRequest {
	if !c.ModeExplicit && !c.ThinkingExplicit && !c.ReasoningEffortExplicit {
		return fallback
	}
	return c.ExecutionReasoningRequest()
}

func normalizeNewDeerFlowRunConfig(
	rawConfig string,
	policy RuntimePolicy,
	rawContexts ...string,
) (string, DeerFlowRuntimeConfig, error) {
	payload, err := parseDeerFlowRuntimePayload(rawConfig)
	if err != nil {
		return "", DeerFlowRuntimeConfig{}, err
	}
	configurable, err := optionalDeerFlowConfigObject(payload, "configurable")
	if err != nil {
		return "", DeerFlowRuntimeConfig{}, err
	}
	mergeDeerFlowRuntimeContext(payload, configurable)
	for _, rawContext := range rawContexts {
		contextPayload, parseErr := parseDeerFlowRuntimePayload(rawContext)
		if parseErr != nil {
			return "", DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
				"parse run context: %v",
				parseErr,
			)
		}
		mergeDeerFlowRuntimeContext(payload, contextPayload)
	}
	nestedContext, err := optionalDeerFlowConfigObject(payload, "context")
	if err != nil {
		return "", DeerFlowRuntimeConfig{}, err
	}
	mergeDeerFlowRuntimeContext(payload, nestedContext)
	projected, err := json.Marshal(payload)
	if err != nil {
		return "", DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
			"encode projected run config: %v",
			err,
		)
	}
	config, err := ParseDeerFlowRuntimeConfig(string(projected))
	if err != nil {
		return "", DeerFlowRuntimeConfig{}, err
	}
	if policy.DefaultMode != "" && policy.DefaultMode != RuntimeModeEinoADK {
		return "", DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
			"legacy runtime cannot be the production default",
		)
	}
	if config.Runtime == RuntimeModeLegacy {
		return "", DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
			"legacy runtime is not selectable for new runs",
		)
	}
	if config.Runtime != "" && config.Runtime != RuntimeModeEinoADK {
		return "", DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
			"unsupported agent runtime: %s",
			config.Runtime,
		)
	}
	if err := policy.authorize(RuntimeModeEinoADK); err != nil {
		return "", DeerFlowRuntimeConfig{}, fmt.Errorf("%w: %v", ErrInvalidRuntimeConfig, err)
	}

	config.Runtime = RuntimeModeEinoADK
	config.ModeExplicit = true
	config.resolved = true
	payload["runtime"] = string(RuntimeModeEinoADK)
	payload["requested_policy"] = string(config.RequestedPolicy)
	payload["mode"] = string(config.Mode)
	payload["thinking_enabled"] = config.ThinkingEnabled
	payload["is_plan_mode"] = config.IsPlanMode
	payload["subagent_enabled"] = config.SubagentEnabled
	if config.ReasoningEffort == "" {
		delete(payload, "reasoning_effort")
	} else {
		payload["reasoning_effort"] = config.ReasoningEffort
	}
	if config.SubagentEnabled {
		payload["max_concurrent_subagents"] = config.MaxConcurrentSubagents
	} else if !config.SubagentMaximumExplicit {
		delete(payload, "max_concurrent_subagents")
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", DeerFlowRuntimeConfig{}, invalidRuntimeConfigf(
			"encode normalized run config: %v",
			err,
		)
	}
	return string(encoded), config, nil
}

func mergeDeerFlowRuntimeContext(target, source map[string]any) {
	if target == nil || source == nil {
		return
	}
	for _, key := range deerFlowRuntimeContextKeys {
		if value, ok := source[key]; ok {
			target[key] = value
		}
	}
}

func defaultDeerFlowRuntimeConfig(mode DeerFlowMode) DeerFlowRuntimeConfig {
	config := DeerFlowRuntimeConfig{Mode: mode}
	switch mode {
	case DeerFlowModePro:
		config.ThinkingEnabled = true
		config.ReasoningEffort = "medium"
		config.IsPlanMode = true
	case DeerFlowModeUltra:
		config.ThinkingEnabled = true
		config.ReasoningEffort = "high"
		config.IsPlanMode = true
		config.SubagentEnabled = true
		config.MaxConcurrentSubagents = defaultDeerFlowMaxConcurrentSubagents
	}
	return config
}

func normalizeDeerFlowMode(value string) (DeerFlowMode, error) {
	switch strings.TrimSpace(value) {
	case string(DeerFlowModePro):
		return DeerFlowModePro, nil
	case string(DeerFlowModeUltra):
		return DeerFlowModeUltra, nil
	default:
		return "", invalidRuntimeConfigf("unsupported DeerFlow mode: %s", value)
	}
}

func normalizeDeerFlowRequestedPolicy(
	value string,
) (DeerFlowRequestedPolicy, DeerFlowMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(DeerFlowRequestedPolicyAuto):
		return DeerFlowRequestedPolicyAuto, DeerFlowModeUltra, nil
	case string(DeerFlowRequestedPolicyPro):
		return DeerFlowRequestedPolicyPro, DeerFlowModePro, nil
	case string(DeerFlowRequestedPolicyUltra):
		return DeerFlowRequestedPolicyUltra, DeerFlowModeUltra, nil
	default:
		return "", "", invalidRuntimeConfigf("unsupported requested_policy: %s", value)
	}
}

func requestedPolicyForMode(mode DeerFlowMode) DeerFlowRequestedPolicy {
	switch mode {
	case DeerFlowModePro:
		return DeerFlowRequestedPolicyPro
	case DeerFlowModeUltra:
		return DeerFlowRequestedPolicyUltra
	default:
		return ""
	}
}

func parseDeerFlowRuntimePayload(rawConfig string) (map[string]any, error) {
	payload := map[string]any{}
	rawConfig = strings.TrimSpace(rawConfig)
	if rawConfig == "" {
		return payload, nil
	}
	if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
		return nil, invalidRuntimeConfigf("parse run config: %v", err)
	}
	if payload == nil {
		return nil, invalidRuntimeConfigf("run config must be a JSON object")
	}
	return payload, nil
}

func optionalDeerFlowConfigString(
	payload map[string]any,
	key string,
) (string, bool, error) {
	value, exists := payload[key]
	if !exists || value == nil {
		return "", false, nil
	}
	parsed, ok := value.(string)
	if !ok {
		return "", false, invalidRuntimeConfigf("%s must be a string", key)
	}
	return strings.TrimSpace(parsed), true, nil
}

func optionalDeerFlowConfigBool(
	payload map[string]any,
	key string,
) (bool, bool, error) {
	value, exists := payload[key]
	if !exists || value == nil {
		return false, false, nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, false, invalidRuntimeConfigf("%s must be a boolean", key)
	}
	return parsed, true, nil
}

func optionalDeerFlowConfigInt(
	payload map[string]any,
	key string,
) (int, bool, error) {
	value, exists := payload[key]
	if !exists || value == nil {
		return 0, false, nil
	}
	number, ok := value.(float64)
	if !ok || number != float64(int(number)) {
		return 0, false, invalidRuntimeConfigf("%s must be an integer", key)
	}
	return int(number), true, nil
}

func optionalDeerFlowConfigObject(
	payload map[string]any,
	key string,
) (map[string]any, error) {
	value, exists := payload[key]
	if !exists || value == nil {
		return nil, nil
	}
	parsed, ok := value.(map[string]any)
	if !ok {
		return nil, invalidRuntimeConfigf("%s must be a JSON object", key)
	}
	return parsed, nil
}

func invalidRuntimeConfigf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRuntimeConfig, fmt.Sprintf(format, args...))
}
