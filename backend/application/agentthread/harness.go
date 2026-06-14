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

type AgentHarnessState struct {
	StepIndex int
	Results   []AgentStepResult
}

type AgentPlanner interface {
	Plan(ctx context.Context, run *RunSummary, state AgentHarnessState) (*AgentPlan, error)
}

type AgentStepRunner interface {
	RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error)
}

type HarnessExecutorOptions struct {
	MaxSteps      int
	ModelProvider ChatModelProvider
	ToolRegistry  ToolRegistry
	EventSink     RunEventSink
}

type HarnessExecutor struct {
	planner   AgentPlanner
	runner    AgentStepRunner
	eventSink RunEventSink
	maxSteps  int
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
		planner:   planner,
		runner:    runner,
		eventSink: opts.EventSink,
		maxSteps:  maxSteps,
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

	state := AgentHarnessState{}
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
			if stepResult.Final {
				message := strings.TrimSpace(stepResult.Message)

				return &RunExecutionResult{
					Message:  message,
					Metadata: harnessMetadata(stepResult.Metadata, executedSteps),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("agent harness exceeded max steps: %d", maxSteps)
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

	return clone
}

func harnessMetadata(stepMetadata string, steps int) string {
	payload := map[string]any{
		"source": "agent_harness",
		"steps":  steps,
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
