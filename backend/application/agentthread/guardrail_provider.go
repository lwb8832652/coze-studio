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
	"strings"
	"unicode"
)

type GuardrailAction string

const (
	GuardrailActionAllow   GuardrailAction = "allow"
	GuardrailActionWarn    GuardrailAction = "warn"
	GuardrailActionConfirm GuardrailAction = "confirm"
	GuardrailActionDeny    GuardrailAction = "deny"
)

func (a GuardrailAction) String() string {
	return string(a)
}

type GuardrailFailMode string

const (
	GuardrailFailOpen   GuardrailFailMode = "fail_open"
	GuardrailFailClosed GuardrailFailMode = "fail_closed"
)

type GuardrailTargetType string

const (
	GuardrailTargetToolCall GuardrailTargetType = "tool_call"
	GuardrailTargetSkill    GuardrailTargetType = "skill"
	GuardrailTargetMCPTool  GuardrailTargetType = "mcp_tool"
	GuardrailTargetFile     GuardrailTargetType = "file"
	GuardrailTargetNetwork  GuardrailTargetType = "network"
	GuardrailTargetCommand  GuardrailTargetType = "command"
	GuardrailTargetArtifact GuardrailTargetType = "artifact"
)

type GuardrailRequest struct {
	SpaceID    int64
	ThreadID   int64
	RunID      int64
	UserID     int64
	TargetType GuardrailTargetType
	TargetID   string
	Operation  string
	Source     string
	FailMode   GuardrailFailMode
	Metadata   map[string]string
}

type GuardrailDecision struct {
	Action     GuardrailAction
	Provider   string
	ReasonCode string
	Message    string
	RuleIDs    []string
	Metadata   map[string]string
}

type GuardrailProvider interface {
	EvaluateGuardrail(
		ctx context.Context,
		request GuardrailRequest,
	) (GuardrailDecision, error)
}

type GuardrailProviderFunc func(
	ctx context.Context,
	request GuardrailRequest,
) (GuardrailDecision, error)

func (f GuardrailProviderFunc) EvaluateGuardrail(
	ctx context.Context,
	request GuardrailRequest,
) (GuardrailDecision, error) {
	if f == nil {
		return GuardrailDecision{Action: GuardrailActionAllow}, nil
	}
	return f(ctx, request)
}

type ChainGuardrailProvider struct {
	providers []GuardrailProvider
}

func NewChainGuardrailProvider(
	providers ...GuardrailProvider,
) *ChainGuardrailProvider {
	filtered := make([]GuardrailProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &ChainGuardrailProvider{providers: filtered}
}

func (p *ChainGuardrailProvider) EvaluateGuardrail(
	ctx context.Context,
	request GuardrailRequest,
) (GuardrailDecision, error) {
	if p == nil || len(p.providers) == 0 {
		return normalizeGuardrailDecision(GuardrailDecision{
			Action:     GuardrailActionAllow,
			Provider:   "guardrail",
			ReasonCode: "no_provider",
		}), nil
	}

	merged := normalizeGuardrailDecision(GuardrailDecision{
		Action:     GuardrailActionAllow,
		Provider:   "guardrail",
		ReasonCode: "default_allow",
	})
	for _, provider := range p.providers {
		decision, err := provider.EvaluateGuardrail(ctx, request)
		if err != nil {
			return guardrailProviderErrorDecision(request.FailMode), nil
		}
		decision = normalizeGuardrailDecision(decision)
		if guardrailActionRank(decision.Action) >= guardrailActionRank(merged.Action) {
			merged = decision
		}
	}

	return merged, nil
}

func guardrailProviderErrorDecision(
	failMode GuardrailFailMode,
) GuardrailDecision {
	if failMode == GuardrailFailOpen {
		return GuardrailDecision{
			Action:     GuardrailActionAllow,
			Provider:   "guardrail",
			ReasonCode: "guardrail_provider_error_fail_open",
			Message:    "guardrail provider failed open",
		}
	}

	return GuardrailDecision{
		Action:     GuardrailActionDeny,
		Provider:   "guardrail",
		ReasonCode: "guardrail_provider_error_fail_closed",
		Message:    "guardrail provider failed closed",
	}
}

func guardrailActionRank(action GuardrailAction) int {
	switch action {
	case GuardrailActionDeny:
		return 4
	case GuardrailActionConfirm:
		return 3
	case GuardrailActionWarn:
		return 2
	case GuardrailActionAllow:
		return 1
	default:
		return 0
	}
}

func normalizeGuardrailDecision(
	decision GuardrailDecision,
) GuardrailDecision {
	if guardrailActionRank(decision.Action) == 0 {
		decision.Action = GuardrailActionDeny
	}
	decision.Provider = sanitizeGuardrailIdentifier(decision.Provider, 64)
	if decision.Provider == "" {
		decision.Provider = "guardrail"
	}
	decision.ReasonCode = sanitizeGuardrailIdentifier(decision.ReasonCode, 64)
	if decision.ReasonCode == "" {
		decision.ReasonCode = string(decision.Action)
	}
	decision.Message = sanitizeGuardrailMessage(decision.Message)
	decision.RuleIDs = sanitizeGuardrailRuleIDs(decision.RuleIDs)
	decision.Metadata = sanitizeGuardrailMetadata(decision.Metadata)

	return decision
}

func sanitizeGuardrailIdentifier(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		if builder.Len() >= limit {
			break
		}
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == ':' || r == '-':
			builder.WriteRune(r)
		case r == '_' || unicode.IsSpace(r) || r == '/':
			builder.WriteRune('_')
		}
	}
	return collapseGuardrailUnderscores(strings.Trim(builder.String(), "_"))
}

func collapseGuardrailUnderscores(value string) string {
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return value
}

func sanitizeGuardrailMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	if containsUnsafeGuardrailValue(message) {
		return "guardrail decision requires review"
	}
	return boundString(message, 160)
}

func sanitizeGuardrailRuleIDs(ruleIDs []string) []string {
	if len(ruleIDs) == 0 {
		return nil
	}
	sanitized := make([]string, 0, len(ruleIDs))
	seen := map[string]struct{}{}
	for _, ruleID := range ruleIDs {
		item := sanitizeGuardrailIdentifier(ruleID, 64)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		sanitized = append(sanitized, item)
	}
	return sanitized
}

func sanitizeGuardrailMetadata(
	metadata map[string]string,
) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = sanitizeGuardrailIdentifier(key, 64)
		if key == "" || unsafeGuardrailMetadataKey(key) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || containsUnsafeGuardrailValue(value) {
			continue
		}
		sanitized[key] = boundString(value, 128)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func unsafeGuardrailMetadataKey(key string) bool {
	key = strings.ToLower(key)
	unsafeParts := []string{
		"argument",
		"checkpoint",
		"completion",
		"credential",
		"filename",
		"object",
		"prompt",
		"provider_raw",
		"raw",
		"secret",
		"tool_result",
		"uri",
		"url",
	}
	for _, part := range unsafeParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func containsUnsafeGuardrailValue(value string) bool {
	value = strings.ToLower(value)
	unsafeParts := []string{
		"://",
		"/mnt/",
		"checkpoint",
		"credential",
		"object_uri",
		"prompt",
		"provider raw",
		"secret",
		"sk-",
		"tool result",
	}
	for _, part := range unsafeParts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}

func boundString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
