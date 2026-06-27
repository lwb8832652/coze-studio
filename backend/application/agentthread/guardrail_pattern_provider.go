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
	"time"

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
)

const (
	agentGuardrailProviderTypeEnv     = "AGENT_GUARDRAIL_PROVIDER_TYPE"
	agentGuardrailPatternRulesJSONEnv = "AGENT_GUARDRAIL_PATTERN_RULES_JSON"
	agentGuardrailHTTPURLEnv          = "AGENT_GUARDRAIL_HTTP_URL"
	agentGuardrailHTTPTokenEnv        = "AGENT_GUARDRAIL_HTTP_TOKEN"
	agentGuardrailHTTPTimeoutMsEnv    = "AGENT_GUARDRAIL_HTTP_TIMEOUT_MS"
)

const (
	guardrailProviderTypePattern = "pattern"
	guardrailProviderTypeHTTP    = "http"
	defaultPatternProviderName   = "pattern_scanner"
)

type GuardrailProviderEnvStatus struct {
	Enabled    bool
	Type       string
	Configured bool
	Error      string
}

type GuardrailPatternProviderOptions struct {
	Provider string
	Rules    []GuardrailPatternRule
}

type GuardrailPatternRule struct {
	RuleID           string              `json:"rule_id"`
	Action           string              `json:"action"`
	TargetTypes      []string            `json:"target_types,omitempty"`
	Operations       []string            `json:"operations,omitempty"`
	TargetContains   []string            `json:"target_contains,omitempty"`
	SourceContains   []string            `json:"source_contains,omitempty"`
	MetadataContains map[string][]string `json:"metadata_contains,omitempty"`
	ReasonCode       string              `json:"reason_code,omitempty"`
	Message          string              `json:"message,omitempty"`
}

type guardrailPatternProvider struct {
	provider string
	rules    []normalizedGuardrailPatternRule
}

type normalizedGuardrailPatternRule struct {
	ruleID           string
	action           GuardrailAction
	targetTypes      []string
	operations       []string
	targetContains   []string
	sourceContains   []string
	metadataContains map[string][]string
	reasonCode       string
	message          string
}

func NewGuardrailPatternProvider(
	options GuardrailPatternProviderOptions,
) (GuardrailProvider, error) {
	if len(options.Rules) == 0 {
		return nil, fmt.Errorf("guardrail pattern rules are required")
	}
	provider := sanitizeGuardrailIdentifier(options.Provider, 64)
	if provider == "" {
		provider = defaultPatternProviderName
	}
	rules := make([]normalizedGuardrailPatternRule, 0, len(options.Rules))
	for index, rule := range options.Rules {
		normalized, err := normalizeGuardrailPatternRule(rule, index)
		if err != nil {
			return nil, err
		}
		rules = append(rules, normalized)
	}

	return &guardrailPatternProvider{
		provider: provider,
		rules:    rules,
	}, nil
}

func NewGuardrailProviderFromEnvWithStatus() (GuardrailProvider, GuardrailProviderEnvStatus) {
	providerType := strings.ToLower(strings.TrimSpace(envkey.GetStringD(agentGuardrailProviderTypeEnv, "")))
	rulesJSON := strings.TrimSpace(envkey.GetStringD(agentGuardrailPatternRulesJSONEnv, ""))
	if providerType == "" && rulesJSON == "" {
		return nil, GuardrailProviderEnvStatus{}
	}
	if providerType == "" {
		providerType = guardrailProviderTypePattern
	}
	status := GuardrailProviderEnvStatus{
		Enabled: true,
		Type:    providerType,
	}

	switch providerType {
	case guardrailProviderTypePattern:
		if rulesJSON == "" {
			status.Error = "guardrail pattern rules are required"
			return nil, status
		}
		var rules []GuardrailPatternRule
		if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
			status.Error = "guardrail pattern rules are invalid"
			return nil, status
		}
		provider, err := NewGuardrailPatternProvider(
			GuardrailPatternProviderOptions{Rules: rules},
		)
		if err != nil {
			status.Error = err.Error()
			return nil, status
		}
		status.Configured = true
		return provider, status
	case guardrailProviderTypeHTTP:
		provider, err := NewHTTPGuardrailProvider(
			HTTPGuardrailProviderOptions{
				Endpoint: envkey.GetStringD(agentGuardrailHTTPURLEnv, ""),
				Token:    envkey.GetStringD(agentGuardrailHTTPTokenEnv, ""),
				Timeout: time.Duration(
					envkey.GetIntD(
						agentGuardrailHTTPTimeoutMsEnv,
						int(defaultHTTPGuardrailTimeout/time.Millisecond),
					),
				) * time.Millisecond,
			},
		)
		if err != nil {
			status.Error = err.Error()
			return nil, status
		}
		status.Configured = true
		return provider, status
	default:
		status.Error = "unsupported guardrail provider type"
		return nil, status
	}
}

func NewGuardrailEnforcerFromEnv(
	repository domainrepo.GuardrailAuditRepository,
	idGen idgen.IDGenerator,
) (*GuardrailEnforcer, GuardrailProviderEnvStatus) {
	provider, status := NewGuardrailProviderFromEnvWithStatus()
	if !status.Enabled {
		return nil, status
	}
	if provider == nil {
		provider = failClosedGuardrailConfigProvider()
	}

	return NewGuardrailEnforcer(GuardrailEnforcerOptions{
		Provider:         provider,
		MetricsCollector: NewGuardrailMetricsCollectorFromEnv(),
		AuditRecorder: NewApplicationGuardrailAuditRecorder(
			ApplicationGuardrailAuditRecorderOptions{
				Repository: repository,
				IDGen:      idGen,
			},
		),
	}), status
}

func failClosedGuardrailConfigProvider() GuardrailProvider {
	return GuardrailProviderFunc(
		func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
			return GuardrailDecision{
				Action:     GuardrailActionDeny,
				Provider:   "guardrail",
				ReasonCode: "guardrail_config_invalid",
				Message:    "guardrail configuration is invalid",
			}, nil
		},
	)
}

func (p *guardrailPatternProvider) EvaluateGuardrail(
	ctx context.Context,
	request GuardrailRequest,
) (GuardrailDecision, error) {
	if p == nil || len(p.rules) == 0 {
		return normalizeGuardrailDecision(GuardrailDecision{
			Action:     GuardrailActionAllow,
			Provider:   defaultPatternProviderName,
			ReasonCode: "no_rules",
		}), nil
	}

	var merged GuardrailDecision
	matched := false
	for _, rule := range p.rules {
		if !rule.matches(request) {
			continue
		}
		decision := rule.decision(p.provider, request)
		if !matched ||
			guardrailActionRank(decision.Action) > guardrailActionRank(merged.Action) {
			merged = decision
			matched = true
			continue
		}
		if guardrailActionRank(decision.Action) == guardrailActionRank(merged.Action) {
			merged.RuleIDs = append(merged.RuleIDs, decision.RuleIDs...)
		}
	}
	if !matched {
		return normalizeGuardrailDecision(GuardrailDecision{
			Action:     GuardrailActionAllow,
			Provider:   p.provider,
			ReasonCode: "no_match",
		}), nil
	}

	return normalizeGuardrailDecision(merged), nil
}

func normalizeGuardrailPatternRule(
	rule GuardrailPatternRule,
	index int,
) (normalizedGuardrailPatternRule, error) {
	action := GuardrailAction(strings.ToLower(strings.TrimSpace(rule.Action)))
	if action != GuardrailActionWarn &&
		action != GuardrailActionConfirm &&
		action != GuardrailActionDeny {
		return normalizedGuardrailPatternRule{}, fmt.Errorf("guardrail pattern rule action is invalid")
	}
	ruleID := sanitizeGuardrailIdentifier(rule.RuleID, 64)
	if ruleID == "" {
		ruleID = fmt.Sprintf("pattern_rule_%d", index+1)
	}
	normalized := normalizedGuardrailPatternRule{
		ruleID:           ruleID,
		action:           action,
		targetTypes:      normalizeGuardrailPatternValues(rule.TargetTypes),
		operations:       normalizeGuardrailPatternValues(rule.Operations),
		targetContains:   normalizeGuardrailPatternValues(rule.TargetContains),
		sourceContains:   normalizeGuardrailPatternValues(rule.SourceContains),
		metadataContains: normalizeGuardrailPatternMap(rule.MetadataContains),
		reasonCode:       sanitizeGuardrailIdentifier(rule.ReasonCode, 64),
		message:          sanitizeGuardrailMessage(rule.Message),
	}
	if normalized.reasonCode == "" {
		normalized.reasonCode = ruleID
	}
	if normalized.message == "" {
		normalized.message = "guardrail pattern matched"
	}
	if !normalized.hasCondition() {
		return normalizedGuardrailPatternRule{}, fmt.Errorf("guardrail pattern rule condition is required")
	}

	return normalized, nil
}

func normalizeGuardrailPatternValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		item := strings.ToLower(strings.TrimSpace(value))
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeGuardrailPatternMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string][]string, len(values))
	for key, patterns := range values {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if normalizedPatterns := normalizeGuardrailPatternValues(patterns); len(normalizedPatterns) > 0 {
			normalized[trimmedKey] = normalizedPatterns
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (r normalizedGuardrailPatternRule) hasCondition() bool {
	return len(r.targetTypes) > 0 ||
		len(r.operations) > 0 ||
		len(r.targetContains) > 0 ||
		len(r.sourceContains) > 0 ||
		len(r.metadataContains) > 0
}

func (r normalizedGuardrailPatternRule) matches(request GuardrailRequest) bool {
	if !matchesGuardrailPatternSet(r.targetTypes, string(request.TargetType)) {
		return false
	}
	if !matchesGuardrailPatternSet(r.operations, request.Operation) {
		return false
	}
	if !containsAnyGuardrailPattern(r.targetContains, request.TargetID) {
		return false
	}
	if !containsAnyGuardrailPattern(r.sourceContains, request.Source) {
		return false
	}
	if !matchesGuardrailPatternMetadata(r.metadataContains, request.Metadata) {
		return false
	}
	return true
}

func (r normalizedGuardrailPatternRule) decision(
	provider string,
	request GuardrailRequest,
) GuardrailDecision {
	return GuardrailDecision{
		Action:     r.action,
		Provider:   provider,
		ReasonCode: r.reasonCode,
		Message:    r.message,
		RuleIDs:    []string{r.ruleID},
		Metadata: map[string]string{
			"operation":   sanitizeGuardrailIdentifier(request.Operation, 64),
			"source":      sanitizeGuardrailIdentifier(request.Source, 64),
			"target_type": sanitizeGuardrailIdentifier(string(request.TargetType), 32),
		},
	}
}

func matchesGuardrailPatternSet(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, pattern := range patterns {
		if normalized == pattern {
			return true
		}
	}
	return false
}

func containsAnyGuardrailPattern(patterns []string, value string) bool {
	if len(patterns) == 0 {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, pattern := range patterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}

func matchesGuardrailPatternMetadata(
	patterns map[string][]string,
	metadata map[string]string,
) bool {
	if len(patterns) == 0 {
		return true
	}
	for key, values := range patterns {
		metadataValue, ok := metadata[key]
		if !ok {
			return false
		}
		if !containsAnyGuardrailPattern(values, metadataValue) {
			return false
		}
	}
	return true
}
