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
)

const defaultHarnessMaxSteps = 4

type AgentStepType string

const (
	AgentStepTypeModel AgentStepType = "model"
	AgentStepTypeTool  AgentStepType = "tool"
)

type AgentStep struct {
	ID            string
	Type          AgentStepType
	Name          string
	ToolName      string
	ToolArguments string
	Final         bool
}

type AgentPlan struct {
	Steps []AgentStep
}

type AgentStepResult struct {
	Message  string
	Metadata string
	Final    bool
}

type AgentMemory struct {
	ID       string
	Scope    string
	Content  string
	Metadata string
	Score    float64
}

type AgentMemoryContext struct {
	Items []AgentMemory
}

type AgentHarnessState struct {
	StepIndex int
	Results   []AgentStepResult
	Memory    AgentMemoryContext
}

type AgentPlanner interface {
	Plan(ctx context.Context, run *RunSummary, state AgentHarnessState) (*AgentPlan, error)
}

type AgentStepRunner interface {
	RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error)
}

type MemoryProvider interface {
	Recall(ctx context.Context, run *RunSummary) ([]AgentMemory, error)
}

type HarnessExecutorOptions struct {
	MaxSteps       int
	ModelProvider  ChatModelProvider
	ToolRegistry   ToolRegistry
	EventSink      RunEventSink
	MemoryProvider MemoryProvider
	UsageCollector UsageCollector
}

type HarnessExecutor struct {
	planner        AgentPlanner
	runner         AgentStepRunner
	eventSink      RunEventSink
	memoryProvider MemoryProvider
	usageCollector UsageCollector
	maxSteps       int
}

func NewHarnessExecutor(planner AgentPlanner, runner AgentStepRunner, opts HarnessExecutorOptions) *HarnessExecutor {
	if planner == nil {
		planner = singleModelPlanner{}
	}
	if runner == nil {
		runner = NewDefaultAgentStepRunner(opts.ModelProvider, opts.ToolRegistry)
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultHarnessMaxSteps
	}

	return &HarnessExecutor{
		planner:        planner,
		runner:         runner,
		eventSink:      opts.EventSink,
		memoryProvider: opts.MemoryProvider,
		usageCollector: opts.UsageCollector,
		maxSteps:       maxSteps,
	}
}

func (e *HarnessExecutor) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
	if e == nil {
		return nil, fmt.Errorf("agent harness executor is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	planner := e.planner
	if planner == nil {
		planner = singleModelPlanner{}
	}
	runner := e.runner
	if runner == nil {
		runner = NewModelStepRunner(NewModelExecutor(nil))
	}
	maxSteps := e.maxSteps
	if maxSteps <= 0 {
		maxSteps = defaultHarnessMaxSteps
	}

	memory, err := e.recallMemory(ctx, run)
	if err != nil {
		return nil, err
	}
	state := AgentHarnessState{
		Memory: memory,
	}
	if len(memory.Items) > 0 {
		e.emitMemoryRecalledEvent(ctx, run, memory)
	}
	executedSteps := 0
	for executedSteps < maxSteps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		plan, err := planner.Plan(ctx, run, cloneHarnessState(state))
		if err != nil {
			return nil, err
		}
		if plan == nil || len(plan.Steps) == 0 {
			return nil, fmt.Errorf("agent harness planner returned no steps")
		}

		for _, step := range plan.Steps {
			if executedSteps >= maxSteps {
				return nil, fmt.Errorf("agent harness exceeded max steps: %d", maxSteps)
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			state.StepIndex = executedSteps
			e.emitStepStartedEvent(ctx, run, step, executedSteps)
			stepResult, err := runner.RunStep(ctx, run, step, cloneHarnessState(state))
			if err != nil {
				e.emitStepFailedEvent(ctx, run, step, executedSteps, err.Error())

				return nil, err
			}
			if stepResult == nil {
				err := fmt.Errorf("agent harness step runner returned empty result")
				e.emitStepFailedEvent(ctx, run, step, executedSteps, err.Error())

				return nil, err
			}
			if stepResult.Final && strings.TrimSpace(stepResult.Message) == "" {
				err := fmt.Errorf("agent harness final step returned empty message")
				e.emitStepFailedEvent(ctx, run, step, executedSteps, err.Error())

				return nil, err
			}

			executedSteps++
			state.Results = append(state.Results, *stepResult)
			e.emitStepCompletedEvent(ctx, run, step, executedSteps-1, stepResult)
			e.recordStepUsage(ctx, run, step, executedSteps-1, stepResult)
			if stepResult.Final {
				message := strings.TrimSpace(stepResult.Message)

				return &RunExecutionResult{
					Message:  message,
					Metadata: harnessMetadata(stepResult.Metadata, executedSteps, state.Memory),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("agent harness exceeded max steps: %d", maxSteps)
}

func (e *HarnessExecutor) recordStepUsage(ctx context.Context, run *RunSummary, step AgentStep, stepIndex int, result *AgentStepResult) {
	if e == nil || e.usageCollector == nil || result == nil {
		return
	}

	usage, ok := tokenUsageFromStepResult(step, stepIndex, result)
	if !ok {
		return
	}
	if err := e.usageCollector.Record(ctx, run, usage); err != nil {
		return
	}

	emitRunEvent(ctx, e.eventSink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: "usage.recorded",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"step_id":      usage.StepID,
			"step_index":   usage.StepIndex,
			"step_name":    usage.StepName,
			"source":       usage.Source,
			"total_tokens": usage.TotalTokens,
		}),
	})
}

func (e *HarnessExecutor) recallMemory(ctx context.Context, run *RunSummary) (AgentMemoryContext, error) {
	if e == nil || e.memoryProvider == nil {
		return AgentMemoryContext{}, nil
	}

	memories, err := e.memoryProvider.Recall(ctx, run)
	if err != nil {
		return AgentMemoryContext{}, err
	}

	return normalizeMemoryContext(memories), nil
}

func normalizeMemoryContext(memories []AgentMemory) AgentMemoryContext {
	if len(memories) == 0 {
		return AgentMemoryContext{}
	}

	items := make([]AgentMemory, 0, len(memories))
	for _, memory := range memories {
		item := AgentMemory{
			ID:       strings.TrimSpace(memory.ID),
			Scope:    strings.TrimSpace(memory.Scope),
			Content:  strings.TrimSpace(memory.Content),
			Metadata: strings.TrimSpace(memory.Metadata),
			Score:    memory.Score,
		}
		if item.Content == "" {
			continue
		}
		items = append(items, item)
	}

	return AgentMemoryContext{Items: items}
}

type stepUsageMetadata struct {
	Usage *stepUsagePayload `json:"usage"`
}

type stepUsagePayload struct {
	Source       string          `json:"source"`
	ModelName    string          `json:"model_name"`
	Provider     string          `json:"provider"`
	InputTokens  int64           `json:"input_tokens"`
	OutputTokens int64           `json:"output_tokens"`
	TotalTokens  int64           `json:"total_tokens"`
	CostMicros   int64           `json:"cost_micros"`
	Currency     string          `json:"currency"`
	Estimated    bool            `json:"estimated"`
	RawUsage     json.RawMessage `json:"raw_usage"`
	Metadata     json.RawMessage `json:"metadata"`
}

func tokenUsageFromStepResult(step AgentStep, stepIndex int, result *AgentStepResult) (AgentTokenUsage, bool) {
	if result == nil {
		return AgentTokenUsage{}, false
	}
	rawMetadata := strings.TrimSpace(result.Metadata)
	if rawMetadata == "" {
		return AgentTokenUsage{}, false
	}

	var metadata stepUsageMetadata
	if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil || metadata.Usage == nil {
		return AgentTokenUsage{}, false
	}

	usagePayload := metadata.Usage
	totalTokens := usagePayload.TotalTokens
	if totalTokens == 0 {
		totalTokens = usagePayload.InputTokens + usagePayload.OutputTokens
	}
	rawUsage := normalizeRawJSON(usagePayload.RawUsage)
	usageMetadata := normalizeRawJSON(usagePayload.Metadata)
	if totalTokens == 0 && usagePayload.CostMicros == 0 && rawUsage == "" {
		return AgentTokenUsage{}, false
	}

	source := TokenUsageSource(strings.TrimSpace(usagePayload.Source))
	if source == "" {
		source = TokenUsageSourceLeadAgent
		if step.Type == AgentStepTypeTool {
			source = TokenUsageSourceTool
		}
	}
	stepName := strings.TrimSpace(step.Name)
	toolName := ""
	if step.Type == AgentStepTypeTool {
		toolName = agentStepToolName(step)
		if stepName == "" {
			stepName = toolName
		}
	}

	return AgentTokenUsage{
		Source:       source,
		StepID:       strings.TrimSpace(step.ID),
		StepIndex:    int32(stepIndex),
		StepName:     stepName,
		ToolName:     toolName,
		ModelName:    strings.TrimSpace(usagePayload.ModelName),
		Provider:     strings.TrimSpace(usagePayload.Provider),
		InputTokens:  usagePayload.InputTokens,
		OutputTokens: usagePayload.OutputTokens,
		TotalTokens:  totalTokens,
		CostMicros:   usagePayload.CostMicros,
		Currency:     strings.TrimSpace(usagePayload.Currency),
		Estimated:    usagePayload.Estimated,
		RawUsage:     rawUsage,
		Metadata:     usageMetadata,
	}, true
}

func normalizeRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}

	return string(bytes)
}

func (e *HarnessExecutor) emitMemoryRecalledEvent(ctx context.Context, run *RunSummary, memory AgentMemoryContext) {
	if len(memory.Items) == 0 {
		return
	}

	scopes := make([]string, 0, len(memory.Items))
	seen := make(map[string]struct{}, len(memory.Items))
	for _, item := range memory.Items {
		if item.Scope == "" {
			continue
		}
		if _, ok := seen[item.Scope]; ok {
			continue
		}
		seen[item.Scope] = struct{}{}
		scopes = append(scopes, item.Scope)
	}

	emitRunEvent(ctx, e.eventSink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: "memory.recalled",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"memory_count": len(memory.Items),
			"scopes":       scopes,
		}),
	})
}

func (e *HarnessExecutor) emitStepStartedEvent(ctx context.Context, run *RunSummary, step AgentStep, stepIndex int) {
	payload := map[string]any{
		"step_index": stepIndex,
	}
	eventType := "step.started"
	if step.Type == AgentStepTypeTool {
		eventType = "tool.started"
		payload["tool_name"] = agentStepToolName(step)
		payload["arguments_present"] = strings.TrimSpace(step.ToolArguments) != ""
	}

	e.emitStepEvent(ctx, run, step, eventType, payload)
}

func (e *HarnessExecutor) emitStepCompletedEvent(ctx context.Context, run *RunSummary, step AgentStep, stepIndex int, result *AgentStepResult) {
	payload := map[string]any{
		"step_index": stepIndex,
		"final":      result != nil && result.Final,
	}
	eventType := "step.completed"
	if step.Type == AgentStepTypeTool {
		eventType = "tool.completed"
		payload["tool_name"] = agentStepToolName(step)
		payload["result_present"] = result != nil && strings.TrimSpace(result.Message) != ""
	} else {
		payload["message_present"] = result != nil && strings.TrimSpace(result.Message) != ""
	}

	e.emitStepEvent(ctx, run, step, eventType, payload)
}

func (e *HarnessExecutor) emitStepFailedEvent(ctx context.Context, run *RunSummary, step AgentStep, stepIndex int, message string) {
	payload := map[string]any{
		"step_index":    stepIndex,
		"error_message": message,
	}
	eventType := "step.failed"
	if step.Type == AgentStepTypeTool {
		eventType = "tool.failed"
		payload["tool_name"] = agentStepToolName(step)
	}

	e.emitStepEvent(ctx, run, step, eventType, payload)
}

func (e *HarnessExecutor) emitStepEvent(ctx context.Context, run *RunSummary, step AgentStep, eventType string, payload map[string]any) {
	if e == nil || run == nil {
		return
	}

	eventPayload := map[string]any{
		"step_id":   step.ID,
		"step_type": string(step.Type),
		"step_name": step.Name,
	}
	for key, value := range payload {
		eventPayload[key] = value
	}

	emitRunEvent(ctx, e.eventSink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: eventType,
		Payload:   encodeRunEventPayload(ctx, eventPayload),
	})
}

type defaultAgentStepRunner struct {
	modelRunner AgentStepRunner
	toolRunner  AgentStepRunner
}

func NewDefaultAgentStepRunner(modelProvider ChatModelProvider, registry ToolRegistry) AgentStepRunner {
	return defaultAgentStepRunner{
		modelRunner: NewModelStepRunner(NewModelExecutor(modelProvider)),
		toolRunner:  NewToolStepRunner(registry),
	}
}

func (r defaultAgentStepRunner) RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error) {
	switch step.Type {
	case AgentStepTypeModel:
		return r.modelRunner.RunStep(ctx, run, step, state)
	case AgentStepTypeTool:
		return r.toolRunner.RunStep(ctx, run, step, state)
	default:
		return nil, fmt.Errorf("unsupported agent step type: %s", step.Type)
	}
}

type singleModelPlanner struct{}

func (singleModelPlanner) Plan(ctx context.Context, run *RunSummary, state AgentHarnessState) (*AgentPlan, error) {
	return &AgentPlan{
		Steps: []AgentStep{
			{ID: "model-1", Type: AgentStepTypeModel, Name: "generate_answer"},
		},
	}, nil
}

type ModelStepRunner struct {
	executor RunExecutor
}

func NewModelStepRunner(executor RunExecutor) *ModelStepRunner {
	if executor == nil {
		executor = NewModelExecutor(nil)
	}

	return &ModelStepRunner{executor: executor}
}

func (r *ModelStepRunner) RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error) {
	if r == nil || r.executor == nil {
		return nil, fmt.Errorf("model step runner executor is required")
	}
	if step.Type != AgentStepTypeModel {
		return nil, fmt.Errorf("unsupported agent step type: %s", step.Type)
	}

	result, err := r.executor.Execute(ctx, run)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("model step runner returned empty result")
	}

	return &AgentStepResult{
		Message:  result.Message,
		Metadata: result.Metadata,
		Final:    true,
	}, nil
}

func agentStepToolName(step AgentStep) string {
	if toolName := strings.TrimSpace(step.ToolName); toolName != "" {
		return toolName
	}

	return strings.TrimSpace(step.Name)
}

func cloneHarnessState(state AgentHarnessState) AgentHarnessState {
	clone := AgentHarnessState{
		StepIndex: state.StepIndex,
	}
	if len(state.Results) > 0 {
		clone.Results = append([]AgentStepResult(nil), state.Results...)
	}
	if len(state.Memory.Items) > 0 {
		clone.Memory.Items = append([]AgentMemory(nil), state.Memory.Items...)
	}

	return clone
}

func harnessMetadata(stepMetadata string, steps int, memory AgentMemoryContext) string {
	payload := map[string]any{
		"source": "agent_harness",
		"steps":  steps,
	}
	if len(memory.Items) > 0 {
		payload["memory_count"] = len(memory.Items)
	}
	if strings.TrimSpace(stepMetadata) != "" {
		var stepPayload map[string]any
		if err := json.Unmarshal([]byte(stepMetadata), &stepPayload); err == nil {
			payload["step_metadata"] = stepPayload
		} else {
			payload["step_metadata"] = stepMetadata
		}
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return `{"source":"agent_harness"}`
	}

	return string(bytes)
}
