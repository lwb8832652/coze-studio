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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	humanInteractionSchema         = "coze.human_interaction.v1"
	humanInteractionResponseSchema = "coze.human_interaction_response.v1"
	humanInteractionResultSchema   = "coze.human_interaction_tool_result.v1"

	adkClarificationToolName = "ask_user_clarification"
	adkConfirmationToolName  = "request_human_confirmation"
)

const (
	maxHumanInteractionPromptBytes   = 32 << 10
	maxHumanInteractionResponseBytes = 16 << 10
	maxHumanInteractionShortText     = 2 << 10
	maxHumanInteractionMediumText    = 4 << 10
	maxHumanInteractionLongText      = 8 << 10
)

type HumanInteractionKind string

const (
	HumanInteractionKindClarification HumanInteractionKind = "clarification"
	HumanInteractionKindConfirmation  HumanInteractionKind = "confirmation"
)

type HumanInteractionDecision string

const (
	HumanInteractionDecisionAnswered HumanInteractionDecision = "answered"
	HumanInteractionDecisionApproved HumanInteractionDecision = "approved"
	HumanInteractionDecisionRejected HumanInteractionDecision = "rejected"
)

type HumanInteractionRiskLevel string

const (
	HumanInteractionRiskNone     HumanInteractionRiskLevel = "none"
	HumanInteractionRiskLow      HumanInteractionRiskLevel = "low"
	HumanInteractionRiskMedium   HumanInteractionRiskLevel = "medium"
	HumanInteractionRiskHigh     HumanInteractionRiskLevel = "high"
	HumanInteractionRiskCritical HumanInteractionRiskLevel = "critical"
)

type HumanInteractionChoice struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
}

type HumanInteractionPrompt struct {
	Schema            string                    `json:"schema"`
	InteractionID     string                    `json:"interaction_id"`
	Kind              HumanInteractionKind      `json:"kind"`
	Title             string                    `json:"title,omitempty"`
	Question          string                    `json:"question,omitempty"`
	Description       string                    `json:"description,omitempty"`
	Required          bool                      `json:"required"`
	AllowFreeText     bool                      `json:"allow_free_text"`
	Choices           []HumanInteractionChoice  `json:"choices,omitempty"`
	RiskLevel         HumanInteractionRiskLevel `json:"risk_level,omitempty"`
	ToolName          string                    `json:"tool_name,omitempty"`
	ToolCallID        string                    `json:"tool_call_id,omitempty"`
	PolicyRef         string                    `json:"policy_ref,omitempty"`
	Action            string                    `json:"action,omitempty"`
	Summary           string                    `json:"summary,omitempty"`
	Consequences      []string                  `json:"consequences,omitempty"`
	AffectedResources []string                  `json:"affected_resources,omitempty"`
	DefaultDecision   string                    `json:"default_decision,omitempty"`
	RejectionGuidance string                    `json:"rejection_guidance,omitempty"`
	CreatedAt         int64                     `json:"created_at,omitempty"`
}

type HumanInteractionResponse struct {
	Schema        string                   `json:"schema"`
	InteractionID string                   `json:"interaction_id"`
	Kind          HumanInteractionKind     `json:"kind"`
	Decision      HumanInteractionDecision `json:"decision"`
	Answer        string                   `json:"answer,omitempty"`
	ChoiceID      string                   `json:"choice_id,omitempty"`
	Comment       string                   `json:"comment,omitempty"`
	SubmittedBy   string                   `json:"submitted_by,omitempty"`
	SubmittedAt   int64                    `json:"submitted_at,omitempty"`
	Source        string                   `json:"source,omitempty"`
}

type humanInteractionToolState struct {
	Prompt HumanInteractionPrompt `json:"prompt"`
}

type clarificationToolInput struct {
	Question      string                   `json:"question"`
	Description   string                   `json:"description,omitempty"`
	Choices       []HumanInteractionChoice `json:"choices,omitempty"`
	AllowFreeText bool                     `json:"allow_free_text,omitempty"`
	Required      bool                     `json:"required,omitempty"`
}

type confirmationToolInput struct {
	Title             string                    `json:"title"`
	Summary           string                    `json:"summary"`
	Action            string                    `json:"action,omitempty"`
	RiskLevel         HumanInteractionRiskLevel `json:"risk_level,omitempty"`
	Consequences      []string                  `json:"consequences,omitempty"`
	AffectedResources []string                  `json:"affected_resources,omitempty"`
	RejectionGuidance string                    `json:"rejection_guidance,omitempty"`
}

func init() {
	schema.RegisterName[HumanInteractionPrompt]("coze_human_interaction_prompt_v1")
	schema.RegisterName[HumanInteractionResponse]("coze_human_interaction_response_v1")
	schema.RegisterName[humanInteractionToolState]("coze_human_interaction_tool_state_v1")
}

func NewADKClarificationTool() (tool.InvokableTool, error) {
	return &adkHumanInteractionTool{
		name:        adkClarificationToolName,
		description: "Ask the user for missing information required to continue the task.",
		kind:        HumanInteractionKindClarification,
	}, nil
}

func NewADKConfirmationTool() (tool.InvokableTool, error) {
	return &adkHumanInteractionTool{
		name:        adkConfirmationToolName,
		description: "Ask the user to approve or reject a sensitive action before continuing.",
		kind:        HumanInteractionKindConfirmation,
	}, nil
}

func NewADKHumanInteractionTools() ([]tool.BaseTool, error) {
	clarification, err := NewADKClarificationTool()
	if err != nil {
		return nil, err
	}
	confirmation, err := NewADKConfirmationTool()
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{clarification, confirmation}, nil
}

func NewDefaultADKToolProvider() ADKToolProvider {
	return NewADKToolPolicyProvider(
		newDefaultADKRuntimeToolProvider(defaultADKToolProviderOptions{}),
	)
}

type DefaultADKToolProviderOption func(*defaultADKToolProviderOptions)

type defaultADKToolProviderOptions struct {
	eventSink         RunEventSink
	recorder          ADKSubagentRunRecorder
	mcpRegistry       ADKMCPToolRegistry
	mcpExecutor       ADKMCPRuntimeToolExecutor
	guardrailEnforcer ADKGuardrailEnforcer
}

func WithDefaultADKToolProviderEventSink(
	eventSink RunEventSink,
) DefaultADKToolProviderOption {
	return func(options *defaultADKToolProviderOptions) {
		options.eventSink = eventSink
	}
}

func WithDefaultADKToolProviderSubagentRunRecorder(
	recorder ADKSubagentRunRecorder,
) DefaultADKToolProviderOption {
	return func(options *defaultADKToolProviderOptions) {
		options.recorder = recorder
	}
}

func WithDefaultADKToolProviderMCPRegistry(
	registry ADKMCPToolRegistry,
) DefaultADKToolProviderOption {
	return func(options *defaultADKToolProviderOptions) {
		options.mcpRegistry = registry
	}
}

func WithDefaultADKToolProviderMCPExecutor(
	executor ADKMCPRuntimeToolExecutor,
) DefaultADKToolProviderOption {
	return func(options *defaultADKToolProviderOptions) {
		options.mcpExecutor = executor
	}
}

func WithDefaultADKToolProviderGuardrailEnforcer(
	enforcer ADKGuardrailEnforcer,
) DefaultADKToolProviderOption {
	return func(options *defaultADKToolProviderOptions) {
		options.guardrailEnforcer = enforcer
	}
}

func NewDefaultADKToolProviderWithSingleAgentSubagents(
	source ADKSingleAgentSubagentService,
	options ...DefaultADKToolProviderOption,
) ADKToolProvider {
	parsedOptions := parseDefaultADKToolProviderOptions(options...)
	base := newDefaultADKRuntimeToolProvider(parsedOptions)
	if source == nil {
		return NewADKToolPolicyProvider(base)
	}

	childFactory := NewApplicationADKAgentFactory(
		DefaultChatModelProvider,
		NewDefaultADKToolProvider(),
		nil,
	)
	definitionProvider := NewADKSingleAgentSubagentDefinitionProviderWithToolGrants(
		source,
		NewADKRunConfigSubagentReferenceProvider(),
		NewADKSingleAgentSnapshotToolGrantProvider(),
	)

	return NewADKToolPolicyProvider(
		NewADKSubagentToolProvider(
			base,
			definitionProvider,
			NewADKSingleAgentSubagentAgentFactory(source, childFactory),
			WithADKSubagentToolProviderEventSink(parsedOptions.eventSink),
			WithADKSubagentToolProviderRunRecorder(parsedOptions.recorder),
			WithADKSubagentToolProviderGuardrailEnforcer(
				parsedOptions.guardrailEnforcer,
			),
		),
	)
}

func parseDefaultADKToolProviderOptions(
	options ...DefaultADKToolProviderOption,
) defaultADKToolProviderOptions {
	parsed := defaultADKToolProviderOptions{}
	for _, option := range options {
		if option != nil {
			option(&parsed)
		}
	}

	return parsed
}

func newDefaultADKRuntimeToolProvider(
	options defaultADKToolProviderOptions,
) ADKToolProvider {
	catalogs := []ADKRuntimeToolCatalog{
		NewADKWebToolCatalog(ADKWebToolCatalogOptions{}),
	}
	if options.mcpRegistry != nil {
		catalogs = append(
			catalogs,
			NewADKMCPRuntimeToolCatalog(
				options.mcpRegistry,
				WithADKMCPRuntimeToolExecutor(options.mcpExecutor),
			),
		)
	}
	catalog := ADKRuntimeToolCatalog(
		NewADKCompositeRuntimeToolCatalog(catalogs...),
	)
	if options.guardrailEnforcer != nil {
		catalog = NewADKGuardrailRuntimeToolCatalog(
			catalog,
			options.guardrailEnforcer,
		)
	}

	return NewADKHumanInteractionToolProvider(
		NewADKRuntimeToolCatalogProvider(
			catalog,
		),
	)
}

type adkHumanInteractionToolProvider struct {
	base ADKToolProvider
}

func NewADKHumanInteractionToolProvider(base ADKToolProvider) ADKToolProvider {
	return &adkHumanInteractionToolProvider{base: base}
}

func (p *adkHumanInteractionToolProvider) ResolveToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	set := ADKToolSet{}
	if p != nil && p.base != nil {
		if toolSetProvider, ok := p.base.(ADKToolSetProvider); ok {
			resolved, err := toolSetProvider.ResolveToolSet(ctx, run)
			if err != nil {
				return ADKToolSet{}, err
			}
			set = resolved
		} else {
			resolved, err := p.base.ResolveTools(ctx, run)
			if err != nil {
				return ADKToolSet{}, err
			}
			set.StaticTools = append(set.StaticTools, resolved...)
			if dynamicProvider, ok := p.base.(ADKDynamicToolProvider); ok {
				dynamicTools, err := dynamicProvider.ResolveDynamicTools(ctx, run)
				if err != nil {
					return ADKToolSet{}, err
				}
				set.DynamicTools = append(set.DynamicTools, dynamicTools...)
			}
		}
	}
	withBuiltins, err := p.appendBuiltins(ctx, set.StaticTools)
	if err != nil {
		return ADKToolSet{}, err
	}
	set.StaticTools = withBuiltins

	return set, nil
}

func (p *adkHumanInteractionToolProvider) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}

	return set.StaticTools, nil
}

func (p *adkHumanInteractionToolProvider) ResolveDynamicTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}

	return set.DynamicTools, nil
}

func (p *adkHumanInteractionToolProvider) appendBuiltins(
	ctx context.Context,
	tools []tool.BaseTool,
) ([]tool.BaseTool, error) {
	builtins, err := NewADKHumanInteractionTools()
	if err != nil {
		return nil, err
	}
	result := append([]tool.BaseTool{}, tools...)
	names := make(map[string]struct{}, len(tools)+len(builtins))
	for _, item := range tools {
		name, err := adkToolName(ctx, item)
		if err != nil {
			return nil, err
		}
		if name != "" {
			names[name] = struct{}{}
		}
	}
	for _, item := range builtins {
		name, err := adkToolName(ctx, item)
		if err != nil {
			return nil, err
		}
		if _, ok := names[name]; ok {
			return nil, fmt.Errorf("duplicate eino adk tool name: %s", name)
		}
		names[name] = struct{}{}
		result = append(result, item)
	}

	return result, nil
}

type adkHumanInteractionTool struct {
	name        string
	description string
	kind        HumanInteractionKind
}

func (t *adkHumanInteractionTool) Info(context.Context) (*schema.ToolInfo, error) {
	if t == nil {
		return nil, fmt.Errorf("human interaction tool is required")
	}
	params := map[string]*schema.ParameterInfo{}
	switch t.kind {
	case HumanInteractionKindClarification:
		params = map[string]*schema.ParameterInfo{
			"question":        {Type: schema.String, Desc: "Question to ask the user.", Required: true},
			"description":     {Type: schema.String, Desc: "Short context for the question."},
			"allow_free_text": {Type: schema.Boolean, Desc: "Whether a free-text answer is allowed."},
			"required":        {Type: schema.Boolean, Desc: "Whether the answer is required."},
			"choices": {
				Type: schema.Array,
				Desc: "Optional answer choices.",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"id":    {Type: schema.String, Desc: "Stable choice id."},
						"label": {Type: schema.String, Desc: "User-facing choice label."},
						"value": {Type: schema.String, Desc: "Choice value."},
					},
				},
			},
		}
	case HumanInteractionKindConfirmation:
		params = map[string]*schema.ParameterInfo{
			"title":              {Type: schema.String, Desc: "Short title of the action.", Required: true},
			"summary":            {Type: schema.String, Desc: "Concise action summary.", Required: true},
			"action":             {Type: schema.String, Desc: "Stable action name."},
			"risk_level":         {Type: schema.String, Desc: "Risk level.", Enum: []string{"none", "low", "medium", "high", "critical"}},
			"rejection_guidance": {Type: schema.String, Desc: "What to do if the user rejects the action."},
			"consequences": {
				Type:     schema.Array,
				Desc:     "Potential consequences.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"affected_resources": {
				Type:     schema.Array,
				Desc:     "Affected resources.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported human interaction tool kind: %s", t.kind)
	}

	return &schema.ToolInfo{
		Name:        t.name,
		Desc:        t.description,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}, nil
}

func (t *adkHumanInteractionTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	wasInterrupted, hasState, state := tool.GetInterruptState[humanInteractionToolState](ctx)
	if !wasInterrupted || !hasState {
		prompt, err := t.prompt(ctx, argumentsInJSON)
		if err != nil {
			return "", err
		}
		return "", tool.StatefulInterrupt(ctx, prompt, humanInteractionToolState{Prompt: prompt})
	}

	isResumeTarget, hasData, data := tool.GetResumeContext[any](ctx)
	if !isResumeTarget || !hasData {
		return "", tool.StatefulInterrupt(ctx, state.Prompt, state)
	}
	response, err := normalizeHumanInteractionResponse(data)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(response.InteractionID) == "" {
		response.InteractionID = state.Prompt.InteractionID
	}
	if response.Kind == "" {
		response.Kind = state.Prompt.Kind
	}
	if response.Schema == "" {
		response.Schema = humanInteractionResponseSchema
	}

	return humanInteractionToolResultFromResponse(response)
}

func (t *adkHumanInteractionTool) prompt(
	ctx context.Context,
	argumentsInJSON string,
) (HumanInteractionPrompt, error) {
	switch t.kind {
	case HumanInteractionKindClarification:
		var input clarificationToolInput
		if err := json.Unmarshal([]byte(strings.TrimSpace(argumentsInJSON)), &input); err != nil {
			return HumanInteractionPrompt{}, fmt.Errorf("clarification arguments are invalid: %w", err)
		}
		prompt := HumanInteractionPrompt{
			Schema:        humanInteractionSchema,
			InteractionID: newHumanInteractionID(t.kind, input.Question),
			Kind:          HumanInteractionKindClarification,
			Title:         "需要补充信息",
			Question:      strings.TrimSpace(input.Question),
			Description:   strings.TrimSpace(input.Description),
			Required:      input.Required,
			AllowFreeText: input.AllowFreeText,
			Choices:       normalizeHumanInteractionChoices(input.Choices),
			RiskLevel:     HumanInteractionRiskNone,
			ToolName:      t.name,
			ToolCallID:    compose.GetToolCallID(ctx),
			CreatedAt:     time.Now().UnixMilli(),
		}
		if !input.Required {
			prompt.Required = true
		}
		if !input.AllowFreeText && len(prompt.Choices) == 0 {
			prompt.AllowFreeText = true
		}
		if err := validateHumanInteractionPrompt(prompt); err != nil {
			return HumanInteractionPrompt{}, err
		}
		return prompt, nil
	case HumanInteractionKindConfirmation:
		var input confirmationToolInput
		if err := json.Unmarshal([]byte(strings.TrimSpace(argumentsInJSON)), &input); err != nil {
			return HumanInteractionPrompt{}, fmt.Errorf("confirmation arguments are invalid: %w", err)
		}
		risk := input.RiskLevel
		if risk == "" {
			risk = HumanInteractionRiskMedium
		}
		prompt := HumanInteractionPrompt{
			Schema:            humanInteractionSchema,
			InteractionID:     newHumanInteractionID(t.kind, input.Title+input.Summary),
			Kind:              HumanInteractionKindConfirmation,
			Title:             strings.TrimSpace(input.Title),
			Summary:           strings.TrimSpace(input.Summary),
			Required:          true,
			AllowFreeText:     true,
			RiskLevel:         risk,
			ToolName:          t.name,
			ToolCallID:        compose.GetToolCallID(ctx),
			Action:            strings.TrimSpace(input.Action),
			Consequences:      trimStringSlice(input.Consequences),
			AffectedResources: trimStringSlice(input.AffectedResources),
			DefaultDecision:   string(HumanInteractionDecisionRejected),
			RejectionGuidance: strings.TrimSpace(input.RejectionGuidance),
			CreatedAt:         time.Now().UnixMilli(),
		}
		if err := validateHumanInteractionPrompt(prompt); err != nil {
			return HumanInteractionPrompt{}, err
		}
		return prompt, nil
	default:
		return HumanInteractionPrompt{}, fmt.Errorf("unsupported human interaction kind: %s", t.kind)
	}
}

func validateHumanInteractionPrompt(prompt HumanInteractionPrompt) error {
	if prompt.Schema != humanInteractionSchema {
		return fmt.Errorf("human interaction prompt schema is invalid")
	}
	if strings.TrimSpace(prompt.InteractionID) == "" {
		return fmt.Errorf("human interaction prompt interaction_id is required")
	}
	switch prompt.Kind {
	case HumanInteractionKindClarification:
		if strings.TrimSpace(prompt.Question) == "" {
			return fmt.Errorf("clarification question is required")
		}
	case HumanInteractionKindConfirmation:
		if strings.TrimSpace(prompt.Title) == "" {
			return fmt.Errorf("confirmation title is required")
		}
		if strings.TrimSpace(prompt.Summary) == "" {
			return fmt.Errorf("confirmation summary is required")
		}
	default:
		return fmt.Errorf("human interaction prompt kind is invalid")
	}
	if err := validateHumanInteractionRisk(prompt.RiskLevel); err != nil {
		return err
	}
	if exceedsBytes(prompt.Title, maxHumanInteractionShortText) ||
		exceedsBytes(prompt.Question, maxHumanInteractionShortText) ||
		exceedsBytes(prompt.Summary, maxHumanInteractionShortText) {
		return fmt.Errorf("human interaction prompt short text exceeds maximum size")
	}
	if exceedsBytes(prompt.Description, maxHumanInteractionMediumText) ||
		exceedsBytes(prompt.RejectionGuidance, maxHumanInteractionMediumText) {
		return fmt.Errorf("human interaction prompt medium text exceeds maximum size")
	}
	raw, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Errorf("marshal human interaction prompt: %w", err)
	}
	if len(raw) > maxHumanInteractionPromptBytes {
		return fmt.Errorf("human interaction prompt exceeds maximum size")
	}

	return nil
}

func validateHumanInteractionResponse(response HumanInteractionResponse) error {
	if response.Schema != humanInteractionResponseSchema {
		return fmt.Errorf("human interaction response schema is invalid")
	}
	if strings.TrimSpace(response.InteractionID) == "" {
		return fmt.Errorf("human interaction response interaction_id is required")
	}
	switch response.Kind {
	case HumanInteractionKindClarification:
		if response.Decision != HumanInteractionDecisionAnswered {
			return fmt.Errorf("clarification decision must be answered")
		}
		if strings.TrimSpace(response.Answer) == "" && strings.TrimSpace(response.ChoiceID) == "" {
			return fmt.Errorf("clarification response requires answer or choice_id")
		}
	case HumanInteractionKindConfirmation:
		if response.Decision != HumanInteractionDecisionApproved &&
			response.Decision != HumanInteractionDecisionRejected {
			return fmt.Errorf("confirmation decision must be approved or rejected")
		}
	default:
		return fmt.Errorf("human interaction response kind is invalid")
	}
	if exceedsBytes(response.Answer, maxHumanInteractionLongText) ||
		exceedsBytes(response.Comment, maxHumanInteractionLongText) {
		return fmt.Errorf("human interaction response text exceeds maximum size")
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal human interaction response: %w", err)
	}
	if len(raw) > maxHumanInteractionResponseBytes {
		return fmt.Errorf("human interaction response exceeds maximum size")
	}

	return nil
}

func humanInteractionToolResultFromResponse(response HumanInteractionResponse) (string, error) {
	if err := validateHumanInteractionResponse(response); err != nil {
		return "", err
	}
	payload := map[string]any{
		"schema": humanInteractionResultSchema,
		"kind":   string(response.Kind),
	}
	switch response.Kind {
	case HumanInteractionKindClarification:
		payload["answered"] = true
		payload["answer"] = strings.TrimSpace(response.Answer)
		payload["choice_id"] = strings.TrimSpace(response.ChoiceID)
	case HumanInteractionKindConfirmation:
		approved := response.Decision == HumanInteractionDecisionApproved
		payload["approved"] = approved
		payload["decision"] = response.Decision
		payload["comment"] = strings.TrimSpace(response.Comment)
		if !approved {
			payload["guidance"] = "User rejected the action. Choose a safer alternative."
		}
	default:
		return "", fmt.Errorf("human interaction result kind is invalid")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal human interaction tool result: %w", err)
	}

	return string(raw), nil
}

func humanInteractionPromptFromInfo(info any) (*HumanInteractionPrompt, bool) {
	switch value := info.(type) {
	case HumanInteractionPrompt:
		if value.Schema == humanInteractionSchema {
			return &value, true
		}
	case *HumanInteractionPrompt:
		if value != nil && value.Schema == humanInteractionSchema {
			return value, true
		}
	case map[string]any:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		var prompt HumanInteractionPrompt
		if err := json.Unmarshal(raw, &prompt); err != nil {
			return nil, false
		}
		if prompt.Schema == humanInteractionSchema {
			return &prompt, true
		}
	}

	return nil, false
}

func humanInteractionPromptsFromInterrupts(items []ADKInterruptItem) []HumanInteractionPrompt {
	prompts := make([]HumanInteractionPrompt, 0, len(items))
	for _, item := range items {
		if !item.IsRootCause {
			continue
		}
		prompt, ok := humanInteractionPromptFromInfo(item.Info)
		if !ok || prompt == nil {
			continue
		}
		prompts = append(prompts, *prompt)
	}

	return prompts
}

func normalizeHumanInteractionResponse(value any) (HumanInteractionResponse, error) {
	switch response := value.(type) {
	case HumanInteractionResponse:
		return response, nil
	case *HumanInteractionResponse:
		if response == nil {
			return HumanInteractionResponse{}, fmt.Errorf("human interaction response is required")
		}
		return *response, nil
	case map[string]any:
		raw, err := json.Marshal(response)
		if err != nil {
			return HumanInteractionResponse{}, fmt.Errorf("marshal human interaction response map: %w", err)
		}
		var decoded HumanInteractionResponse
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return HumanInteractionResponse{}, fmt.Errorf("decode human interaction response map: %w", err)
		}
		return decoded, nil
	default:
		return HumanInteractionResponse{}, fmt.Errorf("unsupported human interaction response type %T", value)
	}
}

func adkToolName(ctx context.Context, item tool.BaseTool) (string, error) {
	if item == nil {
		return "", nil
	}
	info, err := item.Info(ctx)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", nil
	}

	return strings.TrimSpace(info.Name), nil
}

func validateHumanInteractionRisk(risk HumanInteractionRiskLevel) error {
	switch risk {
	case "", HumanInteractionRiskNone, HumanInteractionRiskLow, HumanInteractionRiskMedium,
		HumanInteractionRiskHigh, HumanInteractionRiskCritical:
		return nil
	default:
		return fmt.Errorf("human interaction risk level is invalid")
	}
}

func normalizeHumanInteractionChoices(choices []HumanInteractionChoice) []HumanInteractionChoice {
	out := make([]HumanInteractionChoice, 0, len(choices))
	for _, choice := range choices {
		normalized := HumanInteractionChoice{
			ID:    strings.TrimSpace(choice.ID),
			Label: strings.TrimSpace(choice.Label),
			Value: strings.TrimSpace(choice.Value),
		}
		if normalized.ID == "" && normalized.Value == "" && normalized.Label == "" {
			continue
		}
		out = append(out, normalized)
	}

	return out
}

func trimStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

func exceedsBytes(value string, maximum int) bool {
	return len([]byte(strings.TrimSpace(value))) > maximum
}

func newHumanInteractionID(kind HumanInteractionKind, seed string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s:%s:%d",
		kind,
		strings.TrimSpace(seed),
		time.Now().UnixNano(),
	)))

	return "hi_" + hex.EncodeToString(sum[:8])
}
