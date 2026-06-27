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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ADKSubagentDefinition struct {
	Name                   string
	Description            string
	AgentID                int64
	Version                string
	IsDraft                bool
	FullChatHistoryAsInput bool
	AllowedTools           []string
	AllowedDynamicTools    []string
}

const (
	defaultADKSubagentMaxSubagents = 16
	defaultADKSubagentMaxDepth     = 2
	defaultADKSubagentTimeout      = 5 * time.Minute
)

type ADKSubagentPolicy struct {
	MaxSubagents int
	MaxDepth     int
	Timeout      time.Duration
}

type ADKSubagentDefinitionProvider interface {
	ResolveADKSubagents(
		ctx context.Context,
		run *RunSummary,
	) ([]ADKSubagentDefinition, error)
}

type ADKSubagentDefinitionProviderFunc func(
	ctx context.Context,
	run *RunSummary,
) ([]ADKSubagentDefinition, error)

func (f ADKSubagentDefinitionProviderFunc) ResolveADKSubagents(
	ctx context.Context,
	run *RunSummary,
) ([]ADKSubagentDefinition, error) {
	if f == nil {
		return nil, nil
	}
	return f(ctx, run)
}

type ADKSubagentAgentFactory interface {
	BuildADKSubagent(
		ctx context.Context,
		parent *RunSummary,
		definition ADKSubagentDefinition,
	) (adk.Agent, error)
}

type ADKSubagentAgentFactoryFunc func(
	ctx context.Context,
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (adk.Agent, error)

func (f ADKSubagentAgentFactoryFunc) BuildADKSubagent(
	ctx context.Context,
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (adk.Agent, error) {
	if f == nil {
		return nil, fmt.Errorf("subagent factory is required")
	}
	return f(ctx, parent, definition)
}

type ADKSubagentToolProvider struct {
	base              ADKToolProvider
	definition        ADKSubagentDefinitionProvider
	factory           ADKSubagentAgentFactory
	eventSink         RunEventSink
	recorder          ADKSubagentRunRecorder
	guardrailEnforcer ADKGuardrailEnforcer
}

type ADKSubagentToolProviderOption func(*ADKSubagentToolProvider)

func WithADKSubagentToolProviderEventSink(
	eventSink RunEventSink,
) ADKSubagentToolProviderOption {
	return func(p *ADKSubagentToolProvider) {
		p.eventSink = eventSink
	}
}

func WithADKSubagentToolProviderRunRecorder(
	recorder ADKSubagentRunRecorder,
) ADKSubagentToolProviderOption {
	return func(p *ADKSubagentToolProvider) {
		p.recorder = recorder
	}
}

func WithADKSubagentToolProviderGuardrailEnforcer(
	enforcer ADKGuardrailEnforcer,
) ADKSubagentToolProviderOption {
	return func(p *ADKSubagentToolProvider) {
		p.guardrailEnforcer = enforcer
	}
}

func NewADKSubagentToolProvider(
	base ADKToolProvider,
	definition ADKSubagentDefinitionProvider,
	factory ADKSubagentAgentFactory,
	options ...ADKSubagentToolProviderOption,
) *ADKSubagentToolProvider {
	provider := &ADKSubagentToolProvider{
		base:       base,
		definition: definition,
		factory:    factory,
	}
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}

	return provider
}

func (p *ADKSubagentToolProvider) ResolveToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	if p == nil {
		return ADKToolSet{}, fmt.Errorf("subagent tool provider is required")
	}
	set, err := p.resolveBaseToolSet(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}
	if p.definition == nil {
		return set, nil
	}
	definitions, err := p.definition.ResolveADKSubagents(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}
	if len(definitions) == 0 {
		return set, nil
	}
	policy, err := adkSubagentPolicyFromRun(run)
	if err != nil {
		return ADKToolSet{}, err
	}
	if policy.MaxSubagents > 0 && len(definitions) > policy.MaxSubagents {
		return ADKToolSet{}, fmt.Errorf(
			"subagent count limit exceeded: %d > %d",
			len(definitions),
			policy.MaxSubagents,
		)
	}
	if policy.MaxDepth > 0 && adkSubagentDepthFromRun(run) >= policy.MaxDepth {
		return ADKToolSet{}, fmt.Errorf(
			"subagent depth limit exceeded: depth=%d max_depth=%d",
			adkSubagentDepthFromRun(run),
			policy.MaxDepth,
		)
	}
	if p.factory == nil {
		return ADKToolSet{}, fmt.Errorf("subagent factory is required")
	}

	names, err := adkToolNameSet(ctx, set.StaticTools, set.DynamicTools)
	if err != nil {
		return ADKToolSet{}, err
	}
	for _, definition := range definitions {
		definition = normalizeADKSubagentDefinition(definition)
		if err := validateADKSubagentDefinition(definition); err != nil {
			return ADKToolSet{}, err
		}
		if _, ok := names[definition.Name]; ok {
			return ADKToolSet{}, fmt.Errorf(
				"duplicate eino adk tool name: %s",
				definition.Name,
			)
		}
		names[definition.Name] = struct{}{}

		agent, err := p.factory.BuildADKSubagent(ctx, run, definition)
		if err != nil {
			return ADKToolSet{}, fmt.Errorf(
				"build subagent %s: %w",
				definition.Name,
				err,
			)
		}
		if agent == nil {
			return ADKToolSet{}, fmt.Errorf(
				"subagent factory returned empty agent: %s",
				definition.Name,
			)
		}
		options := []adk.AgentToolOption{}
		if definition.FullChatHistoryAsInput {
			options = append(options, adk.WithFullChatHistoryAsInput())
		}
		parent := (*RunSummary)(nil)
		if run != nil {
			parentSnapshot := *run
			parent = &parentSnapshot
		}
		agentTool := adk.NewAgentTool(ctx, agent, options...)
		if policy.Timeout > 0 {
			agentTool = &adkSubagentPolicyTool{
				base:    agentTool,
				timeout: policy.Timeout,
			}
		}
		if p.eventSink != nil || p.recorder != nil {
			agentTool = &adkSubagentLifecycleTool{
				base:       agentTool,
				parent:     parent,
				definition: definition,
				eventSink:  p.eventSink,
				recorder:   p.recorder,
			}
		}
		if p.guardrailEnforcer != nil {
			agentTool = &adkSubagentGuardrailTool{
				base:       agentTool,
				parent:     parent,
				definition: definition,
				enforcer:   p.guardrailEnforcer,
			}
		}
		set.StaticTools = append(set.StaticTools, agentTool)
	}

	return set, nil
}

func (p *ADKSubagentToolProvider) ResolveTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}
	return set.StaticTools, nil
}

func (p *ADKSubagentToolProvider) ResolveDynamicTools(
	ctx context.Context,
	run *RunSummary,
) ([]tool.BaseTool, error) {
	set, err := p.ResolveToolSet(ctx, run)
	if err != nil {
		return nil, err
	}
	return set.DynamicTools, nil
}

func (p *ADKSubagentToolProvider) resolveBaseToolSet(
	ctx context.Context,
	run *RunSummary,
) (ADKToolSet, error) {
	if p.base == nil {
		return ADKToolSet{}, nil
	}
	if toolSetProvider, ok := p.base.(ADKToolSetProvider); ok {
		return toolSetProvider.ResolveToolSet(ctx, run)
	}
	staticTools, err := p.base.ResolveTools(ctx, run)
	if err != nil {
		return ADKToolSet{}, err
	}
	set := ADKToolSet{StaticTools: staticTools}
	if dynamicProvider, ok := p.base.(ADKDynamicToolProvider); ok {
		set.DynamicTools, err = dynamicProvider.ResolveDynamicTools(ctx, run)
		if err != nil {
			return ADKToolSet{}, err
		}
	}
	return set, nil
}

type ADKRunConfigSubagentDefinitionProvider struct{}

func NewADKRunConfigSubagentDefinitionProvider() *ADKRunConfigSubagentDefinitionProvider {
	return &ADKRunConfigSubagentDefinitionProvider{}
}

func (p *ADKRunConfigSubagentDefinitionProvider) ResolveADKSubagents(
	_ context.Context,
	run *RunSummary,
) ([]ADKSubagentDefinition, error) {
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return nil, fmt.Errorf("parse subagent config: %w", err)
	}
	rawItems, ok := payload["subagents"].([]any)
	if !ok || len(rawItems) == 0 {
		return nil, nil
	}
	definitions := make([]ADKSubagentDefinition, 0, len(rawItems))
	seen := map[string]struct{}{}
	for _, rawItem := range rawItems {
		item, ok := configObject(rawItem)
		if !ok {
			return nil, fmt.Errorf("subagent definition must be an object")
		}
		definition := normalizeADKSubagentDefinition(ADKSubagentDefinition{
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

func adkSubagentPolicyFromRun(run *RunSummary) (ADKSubagentPolicy, error) {
	policy := ADKSubagentPolicy{
		MaxSubagents: defaultADKSubagentMaxSubagents,
		MaxDepth:     defaultADKSubagentMaxDepth,
		Timeout:      defaultADKSubagentTimeout,
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return policy, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return policy, fmt.Errorf("parse subagent policy config: %w", err)
	}
	rawPolicy := payload
	for _, key := range []string{"subagent_policy", "subagentPolicy"} {
		if nested, ok := configObject(payload[key]); ok {
			rawPolicy = nested
			break
		}
	}
	if max := firstConfigInt64(
		rawPolicy,
		"max_subagents",
		"maxSubagents",
	); max > 0 {
		policy.MaxSubagents = int(max)
	}
	if maxDepth := firstConfigInt64(
		rawPolicy,
		"max_depth",
		"maxDepth",
	); maxDepth > 0 {
		policy.MaxDepth = int(maxDepth)
	}
	if timeoutMS := firstConfigInt64(
		rawPolicy,
		"timeout_ms",
		"timeoutMs",
	); timeoutMS > 0 {
		policy.Timeout = time.Duration(timeoutMS) * time.Millisecond
	}

	return policy, nil
}

func adkSubagentDepthFromRun(run *RunSummary) int {
	identity := adkSubagentIdentityFromRunMetadata(run)
	if identity == nil {
		return 0
	}
	return identity.Depth
}

type adkSubagentPolicyTool struct {
	base    tool.BaseTool
	timeout time.Duration
}

func (t *adkSubagentPolicyTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	if t == nil || t.base == nil {
		return nil, fmt.Errorf("subagent policy tool base is required")
	}
	return t.base.Info(ctx)
}

func (t *adkSubagentPolicyTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	if t == nil || t.base == nil {
		return "", fmt.Errorf("subagent policy tool base is required")
	}
	invokable, ok := t.base.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("subagent tool is not invokable")
	}
	if t.timeout <= 0 {
		return invokable.InvokableRun(ctx, argumentsInJSON, opts...)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	return invokable.InvokableRun(timeoutCtx, argumentsInJSON, opts...)
}

type adkSubagentGuardrailTool struct {
	base       tool.BaseTool
	parent     *RunSummary
	definition ADKSubagentDefinition
	enforcer   ADKGuardrailEnforcer
}

func (t *adkSubagentGuardrailTool) Info(
	ctx context.Context,
) (*schema.ToolInfo, error) {
	if t == nil || t.base == nil {
		return nil, fmt.Errorf("subagent guardrail tool base is required")
	}
	return t.base.Info(ctx)
}

func (t *adkSubagentGuardrailTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	if t == nil || t.base == nil {
		return "", fmt.Errorf("subagent guardrail tool base is required")
	}
	invokable, ok := t.base.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("subagent tool is not invokable")
	}
	if handled, err := adkGuardrailHandleConfirmationResume(ctx); handled {
		if err != nil {
			return "", err
		}
		resumeCtx := compose.AppendAddressSegment(
			ctx,
			compose.AddressSegmentTool,
			"guardrail_approved_"+normalizeADKSubagentDefinition(t.definition).Name,
		)
		return invokable.InvokableRun(resumeCtx, argumentsInJSON, opts...)
	}
	if t.enforcer != nil {
		request := adkSubagentGuardrailRequest(t.parent, t.definition)
		result, err := t.enforcer.Evaluate(ctx, request)
		if err != nil {
			var confirmation *GuardrailConfirmationRequiredError
			if errors.As(err, &confirmation) || result.RequiresConfirmation {
				decision := result.Decision
				if confirmation != nil &&
					strings.TrimSpace(decision.ReasonCode) == "" {
					decision.ReasonCode = confirmation.ReasonCode
				}
				return "", adkSubagentGuardrailConfirmationInterrupt(
					ctx,
					request,
					decision,
				)
			}
			return "", err
		}
		if result.RequiresConfirmation {
			return "", adkSubagentGuardrailConfirmationInterrupt(
				ctx,
				request,
				result.Decision,
			)
		}
		if !result.Allowed {
			return "", &GuardrailDeniedError{
				ReasonCode: result.Decision.ReasonCode,
			}
		}
	}

	return invokable.InvokableRun(ctx, argumentsInJSON, opts...)
}

func adkSubagentGuardrailRequest(
	parent *RunSummary,
	definition ADKSubagentDefinition,
) GuardrailRequest {
	definition = normalizeADKSubagentDefinition(definition)
	request := GuardrailRequest{
		TargetType: GuardrailTargetToolCall,
		TargetID:   definition.Name,
		Operation:  "invoke",
		Source:     "adk_subagent_tool",
		FailMode:   GuardrailFailClosed,
	}
	if parent != nil {
		request.SpaceID = parent.SpaceID
		request.ThreadID = parent.ThreadID
		request.RunID = parent.RunID
		request.UserID = parent.CreatorID
	}

	return request
}

func adkSubagentGuardrailConfirmationInterrupt(
	ctx context.Context,
	request GuardrailRequest,
	decision GuardrailDecision,
) error {
	return adkGuardrailConfirmationInterrupt(
		ctx,
		request,
		decision,
		adkGuardrailConfirmationPromptOptions{
			Title:          "Review subagent invocation",
			TargetLabel:    "subagent tool",
			ResourcePrefix: "subagent_tool",
			Consequence:    "The subagent tool will not run unless this request is approved.",
		},
	)
}

type adkSubagentLifecycleTool struct {
	base       tool.BaseTool
	parent     *RunSummary
	definition ADKSubagentDefinition
	eventSink  RunEventSink
	recorder   ADKSubagentRunRecorder
}

func (t *adkSubagentLifecycleTool) Info(
	ctx context.Context,
) (*schema.ToolInfo, error) {
	if t == nil || t.base == nil {
		return nil, fmt.Errorf("subagent lifecycle tool base is required")
	}
	return t.base.Info(ctx)
}

func (t *adkSubagentLifecycleTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	if t == nil || t.base == nil {
		return "", fmt.Errorf("subagent lifecycle tool base is required")
	}
	invokable, ok := t.base.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("subagent tool is not invokable")
	}
	if t.eventSink == nil && t.recorder == nil {
		return invokable.InvokableRun(ctx, argumentsInJSON, opts...)
	}

	var childRun *RunSummary
	if t.recorder != nil {
		started, err := t.recorder.StartADKSubagentRun(ctx, ADKSubagentRunStartRequest{
			Parent:          t.parent,
			Definition:      t.definition,
			ArgumentsInJSON: argumentsInJSON,
		})
		if err != nil {
			t.emitLifecycleEvent(ctx, "subagent.run.failed", nil, "failed", 0, err)
			return "", err
		}
		childRun = started
	}

	t.emitLifecycleEvent(ctx, "subagent.run.started", childRun, "running", 0, nil)

	startedAt := time.Now()
	result, err := invokable.InvokableRun(ctx, argumentsInJSON, opts...)
	elapsed := time.Since(startedAt)
	if err != nil {
		terminal := classifyADKSubagentTerminalError(err)
		errorMessage := sanitizeADKSubagentLifecycleErrorMessage(err.Error())
		if t.recorder != nil && childRun != nil {
			_, finishErr := t.recorder.FinishADKSubagentRun(
				ctx,
				ADKSubagentRunFinishRequest{
					Parent:       t.parent,
					Child:        childRun,
					Definition:   t.definition,
					Status:       terminal.Status,
					ErrorCode:    terminal.ErrorCode,
					ErrorMessage: errorMessage,
				},
			)
			if finishErr != nil {
				return "", finishErr
			}
		}
		t.emitLifecycleEvent(
			ctx,
			terminal.EventType,
			childRun,
			terminal.Status,
			elapsed,
			err,
		)
		return "", err
	}

	if t.recorder != nil && childRun != nil {
		if _, err := t.recorder.FinishADKSubagentRun(
			ctx,
			ADKSubagentRunFinishRequest{
				Parent:     t.parent,
				Child:      childRun,
				Definition: t.definition,
				Status:     RunStatusSucceeded,
			},
		); err != nil {
			return "", err
		}
	}
	t.emitLifecycleEvent(ctx, "subagent.run.completed", childRun, "succeeded", elapsed, nil)

	return result, nil
}

func (t *adkSubagentLifecycleTool) emitLifecycleEvent(
	ctx context.Context,
	eventType string,
	child *RunSummary,
	status RunStatus,
	elapsed time.Duration,
	runErr error,
) {
	if t == nil || t.eventSink == nil {
		return
	}
	emitRunEvent(ctx, t.eventSink, RunEvent{
		ThreadID:  runSummaryThreadID(t.parent),
		RunID:     runSummaryRunID(t.parent),
		EventType: eventType,
		Payload: encodeRunEventPayload(ctx, adkSubagentLifecyclePayload(
			t.parent,
			child,
			t.definition,
			status,
			elapsed,
			runErr,
		)),
	})
}

func adkSubagentLifecyclePayload(
	parent *RunSummary,
	child *RunSummary,
	definition ADKSubagentDefinition,
	status RunStatus,
	elapsed time.Duration,
	runErr error,
) map[string]any {
	definition = normalizeADKSubagentDefinition(definition)
	terminal := classifyADKSubagentTerminalError(runErr)
	if runErr == nil {
		terminal = adkSubagentTerminalClassification{Status: status}
	}
	payload := map[string]any{
		"source":        "eino_adk",
		"status":        status,
		"parent_run_id": runSummaryRunID(parent),
		"subagent":      adkChildSubagentIdentity(parent, definition.Name),
		"agent_id":      definition.AgentID,
		"is_draft":      definition.IsDraft,
	}
	if definition.Version != "" {
		payload["version"] = definition.Version
	}
	if child != nil && child.RunID > 0 {
		payload["child_run_id"] = child.RunID
	}
	if elapsed > 0 {
		payload["elapsed_ms"] = elapsed.Milliseconds()
	}
	if runErr != nil {
		payload["error_code"] = terminal.ErrorCode
		payload["terminal_classification"] = terminal.Classification
		payload["error_message"] = sanitizeADKSubagentLifecycleErrorMessage(
			runErr.Error(),
		)
	}

	return payload
}

type adkSubagentTerminalClassification struct {
	Status         RunStatus
	EventType      string
	ErrorCode      string
	Classification string
}

func classifyADKSubagentTerminalError(
	err error,
) adkSubagentTerminalClassification {
	if err == nil {
		return adkSubagentTerminalClassification{
			Status:    RunStatusSucceeded,
			EventType: "subagent.run.completed",
		}
	}
	if errors.Is(err, context.Canceled) {
		return adkSubagentTerminalClassification{
			Status:         RunStatusCanceled,
			EventType:      "subagent.run.canceled",
			ErrorCode:      "subagent_canceled",
			Classification: "canceled",
		}
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "context deadline exceeded") {
		return adkSubagentTerminalClassification{
			Status:         RunStatusFailed,
			EventType:      "subagent.run.failed",
			ErrorCode:      "subagent_timeout",
			Classification: "timeout",
		}
	}
	return adkSubagentTerminalClassification{
		Status:         RunStatusFailed,
		EventType:      "subagent.run.failed",
		ErrorCode:      "subagent_failed",
		Classification: "failed",
	}
}

func sanitizeADKSubagentLifecycleErrorMessage(message string) string {
	message = sanitizeADKToolErrorMessage(message)
	if index := strings.IndexAny(message, "\r\n"); index >= 0 {
		message = message[:index]
	}
	message = strings.TrimSpace(strings.TrimPrefix(message, "[NodeRunError]"))
	return sanitizeADKToolErrorMessage(message)
}

func runSummaryRunID(run *RunSummary) int64 {
	if run == nil {
		return 0
	}
	return run.RunID
}

func runSummaryThreadID(run *RunSummary) int64 {
	if run == nil {
		return 0
	}
	return run.ThreadID
}

func normalizeADKSubagentDefinition(
	definition ADKSubagentDefinition,
) ADKSubagentDefinition {
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Description = strings.TrimSpace(definition.Description)
	definition.Version = strings.TrimSpace(definition.Version)
	definition.AllowedTools = normalizeConfigStringSlice(definition.AllowedTools)
	definition.AllowedDynamicTools = normalizeConfigStringSlice(
		definition.AllowedDynamicTools,
	)
	return definition
}

func validateADKSubagentDefinition(definition ADKSubagentDefinition) error {
	if !isADKSubagentToolName(definition.Name) {
		return fmt.Errorf("invalid subagent tool name: %s", definition.Name)
	}
	if definition.Description == "" {
		return fmt.Errorf("subagent description is required: %s", definition.Name)
	}
	return nil
}

func isADKSubagentToolName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for index, char := range name {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char == '_':
		case index > 0 && char >= '0' && char <= '9':
		default:
			return false
		}
		if index == 0 && char >= '0' && char <= '9' {
			return false
		}
	}
	return true
}

func adkToolNameSet(
	ctx context.Context,
	toolGroups ...[]tool.BaseTool,
) (map[string]struct{}, error) {
	names := map[string]struct{}{}
	for _, group := range toolGroups {
		for _, item := range group {
			name, err := adkToolName(ctx, item)
			if err != nil {
				return nil, err
			}
			if name == "" {
				continue
			}
			if _, ok := names[name]; ok {
				return nil, fmt.Errorf("duplicate eino adk tool name: %s", name)
			}
			names[name] = struct{}{}
		}
	}
	return names, nil
}
