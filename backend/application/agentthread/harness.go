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
	Steps     []AgentExecutedStep
	Memory    AgentMemoryContext
}

type AgentExecutedStep struct {
	StepID    string
	StepType  AgentStepType
	StepName  string
	StepIndex int
	Message   string
	Metadata  string
	Final     bool
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
	CheckpointSink CheckpointSink
}

type HarnessExecutor struct {
	planner        AgentPlanner
	runner         AgentStepRunner
	eventSink      RunEventSink
	memoryProvider MemoryProvider
	usageCollector UsageCollector
	checkpointSink CheckpointSink
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
		checkpointSink: opts.CheckpointSink,
		maxSteps:       maxSteps,
	}
}

func NewApplicationHarnessExecutor(app *ApplicationService) *HarnessExecutor {
	return NewHarnessExecutor(nil, nil, HarnessExecutorOptions{
		EventSink:      NewApplicationRunEventSink(app),
		MemoryProvider: NewThreadMemoryProvider(app, 8),
		UsageCollector: NewThreadUsageCollector(app),
		CheckpointSink: NewThreadCheckpointSink(app),
	})
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
	parentCheckpointID, err := e.saveCheckpoint(ctx, run, 0, "harness.initial", "initial", "running", state, nil, nil)
	if err != nil {
		return nil, err
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

		for stepPlanIndex, step := range plan.Steps {
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
			state.Steps = append(state.Steps, agentExecutedStep(step, executedSteps-1, stepResult))
			e.emitStepCompletedEvent(ctx, run, step, executedSteps-1, stepResult)
			e.recordStepUsage(ctx, run, step, executedSteps-1, stepResult)
			parentCheckpointID, err = e.saveCheckpoint(
				ctx,
				run,
				parentCheckpointID,
				harnessStepCheckpointNS(step),
				harnessStepCheckpointPhase(step),
				"running",
				state,
				&step,
				plan.Steps[stepPlanIndex+1:],
			)
			if err != nil {
				return nil, err
			}

			if stepResult.Final {
				message := strings.TrimSpace(stepResult.Message)
				if _, err := e.saveCheckpoint(ctx, run, parentCheckpointID, "harness.terminal", "terminal", "succeeded", state, nil, nil); err != nil {
					return nil, err
				}

				return &RunExecutionResult{
					Message:  message,
					Metadata: harnessMetadata(stepResult.Metadata, executedSteps, state.Memory),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("agent harness exceeded max steps: %d", maxSteps)
}

func (e *HarnessExecutor) saveCheckpoint(
	ctx context.Context,
	run *RunSummary,
	parentCheckpointID int64,
	checkpointNS string,
	phase string,
	status string,
	state AgentHarnessState,
	step *AgentStep,
	pendingSteps []AgentStep,
) (int64, error) {
	if e == nil || e.checkpointSink == nil {
		return parentCheckpointID, nil
	}

	checkpoint := AgentCheckpoint{
		ParentCheckpointID: parentCheckpointID,
		CheckpointNS:       checkpointNS,
		ChannelValues:      encodeHarnessJSON(harnessCheckpointValues(run, state), "{}"),
		ChannelVersions:    encodeHarnessJSON(harnessCheckpointVersions(state), "{}"),
		PendingSends:       encodeHarnessJSON(harnessPendingSends(pendingSteps), "[]"),
		Metadata:           encodeHarnessJSON(harnessCheckpointMetadata(run, phase, status, state, step), "{}"),
	}
	saved, err := e.checkpointSink.SaveCheckpoint(ctx, run, checkpoint)
	if err != nil {
		return parentCheckpointID, err
	}
	if saved == nil || saved.CheckpointID <= 0 {
		return parentCheckpointID, fmt.Errorf("agent harness checkpoint sink returned empty checkpoint")
	}

	return saved.CheckpointID, nil
}

func harnessCheckpointValues(run *RunSummary, state AgentHarnessState) map[string]any {
	messages := harnessInputMessages(runInput(run))
	for _, step := range state.Steps {
		message := strings.TrimSpace(step.Message)
		if message == "" {
			continue
		}
		role := string(MessageRoleAssistant)
		payload := map[string]any{
			"role":    role,
			"content": message,
			"step_id": step.StepID,
		}
		if step.StepType == AgentStepTypeTool {
			payload["role"] = string(MessageRoleTool)
			payload["tool_name"] = step.StepName
		}
		messages = append(messages, payload)
	}

	return map[string]any{
		"messages":     messages,
		"artifacts":    map[string]any{},
		"todos":        []any{},
		"memory":       harnessMemoryValues(state.Memory),
		"tool_results": harnessToolResults(state.Steps),
		"steps":        harnessStepValues(state.Steps),
	}
}

func harnessCheckpointVersions(state AgentHarnessState) map[string]any {
	return map[string]any{
		"messages":     len(state.Steps),
		"memory":       len(state.Memory.Items),
		"steps":        len(state.Steps),
		"tool_results": len(harnessToolResults(state.Steps)),
	}
}

func harnessCheckpointMetadata(
	run *RunSummary,
	phase string,
	status string,
	state AgentHarnessState,
	step *AgentStep,
) map[string]any {
	metadata := map[string]any{
		"source":           "agent_harness",
		"checkpoint_phase": phase,
		"status":           status,
		"step_count":       len(state.Steps),
	}
	if run != nil {
		metadata["thread_id"] = run.ThreadID
		metadata["run_id"] = run.RunID
	}
	if step != nil {
		metadata["step_id"] = step.ID
		metadata["step_type"] = string(step.Type)
		metadata["step_name"] = step.Name
	}

	return metadata
}

func harnessInputMessages(rawInput string) []map[string]any {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return []map[string]any{}
	}

	var input modelExecutorRunInput
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
		return []map[string]any{}
	}

	messages := make([]map[string]any, 0, len(input.Messages)+1)
	for _, item := range input.Messages {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(item.Role))
		if role == "" {
			role = string(MessageRoleUser)
		}
		message := map[string]any{
			"role":    role,
			"content": content,
		}
		if toolCallID := strings.TrimSpace(item.ToolCallID); toolCallID != "" {
			message["tool_call_id"] = toolCallID
		}
		if toolName := strings.TrimSpace(item.ToolName); toolName != "" {
			message["tool_name"] = toolName
		}
		messages = append(messages, message)
	}
	if len(messages) == 0 {
		if message := strings.TrimSpace(input.Message); message != "" {
			messages = append(messages, map[string]any{
				"role":    string(MessageRoleUser),
				"content": message,
			})
		}
	}

	return messages
}

func harnessMemoryValues(memory AgentMemoryContext) map[string]any {
	items := make([]map[string]any, 0, len(memory.Items))
	for _, item := range memory.Items {
		payload := map[string]any{
			"id":      item.ID,
			"scope":   item.Scope,
			"content": item.Content,
			"score":   item.Score,
		}
		if metadata := strings.TrimSpace(item.Metadata); metadata != "" {
			payload["metadata"] = metadata
		}
		items = append(items, payload)
	}

	return map[string]any{"items": items}
}

func harnessToolResults(steps []AgentExecutedStep) map[string]any {
	results := map[string]any{}
	for _, step := range steps {
		if step.StepType != AgentStepTypeTool || strings.TrimSpace(step.Message) == "" {
			continue
		}
		key := strings.TrimSpace(step.StepName)
		if key == "" {
			key = step.StepID
		}
		results[key] = map[string]any{
			"content": step.Message,
			"step_id": step.StepID,
		}
	}

	return results
}

func harnessStepValues(steps []AgentExecutedStep) []map[string]any {
	values := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		values = append(values, map[string]any{
			"step_id":         step.StepID,
			"step_type":       string(step.StepType),
			"step_name":       step.StepName,
			"step_index":      step.StepIndex,
			"final":           step.Final,
			"message_present": strings.TrimSpace(step.Message) != "",
		})
	}

	return values
}

func harnessPendingSends(steps []AgentStep) []map[string]any {
	pending := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		node := harnessStepNodeName(step)
		if node == "" {
			continue
		}
		pending = append(pending, map[string]any{
			"node":      node,
			"step_id":   step.ID,
			"step_type": string(step.Type),
		})
	}

	return pending
}

func agentExecutedStep(step AgentStep, stepIndex int, result *AgentStepResult) AgentExecutedStep {
	executed := AgentExecutedStep{
		StepID:    strings.TrimSpace(step.ID),
		StepType:  step.Type,
		StepName:  harnessStepNodeName(step),
		StepIndex: stepIndex,
		Final:     result != nil && result.Final,
	}
	if result != nil {
		executed.Message = strings.TrimSpace(result.Message)
		executed.Metadata = strings.TrimSpace(result.Metadata)
	}

	return executed
}

func harnessStepCheckpointNS(step AgentStep) string {
	if step.Type == AgentStepTypeTool {
		return "harness.tool"
	}

	return "harness.step"
}

func harnessStepCheckpointPhase(step AgentStep) string {
	if step.Type == AgentStepTypeTool {
		return "tool"
	}

	return "step"
}

func harnessStepNodeName(step AgentStep) string {
	if step.Type == AgentStepTypeTool {
		if toolName := agentStepToolName(step); toolName != "" {
			return toolName
		}
	}
	if name := strings.TrimSpace(step.Name); name != "" {
		return name
	}

	return strings.TrimSpace(step.ID)
}

func runInput(run *RunSummary) string {
	if run == nil {
		return ""
	}

	return run.Input
}

func encodeHarnessJSON(value any, fallback string) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return fallback
	}

	return string(bytes)
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
	if len(state.Steps) > 0 {
		clone.Steps = append([]AgentExecutedStep(nil), state.Steps...)
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
