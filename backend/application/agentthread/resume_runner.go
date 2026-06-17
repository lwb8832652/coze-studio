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
	"strconv"
	"strings"
)

const defaultResumeRunProcessorWorkerID = "agent-harness-resume"

const (
	resumeRunPayloadInvalidCode       = "checkpoint_resume_payload_invalid"
	resumeRunCheckpointInvalidCode    = "checkpoint_resume_checkpoint_invalid"
	resumeRunReplayNotImplementedCode = "checkpoint_replay_not_implemented"
)

type ResumeRunProcessorOptions struct {
	WorkerID  string
	BatchSize int32
	EventSink RunEventSink
}

type ResumeRunProcessor struct {
	app       *ApplicationService
	eventSink RunEventSink
	workerID  string
	batchSize int32
}

type resumeRunPayload struct {
	CheckpointID int64
	CheckpointNS string
	ResumeFrom   string
}

type HarnessResumeInput struct {
	ThreadID        int64
	RunID           int64
	SourceRunID     int64
	CheckpointID    int64
	CheckpointNS    string
	ResumeFrom      string
	ChannelValues   map[string]any
	ChannelVersions map[string]any
	Metadata        map[string]any
	Messages        []map[string]any
	PendingSteps    []AgentStep
	State           AgentHarnessState
}

func NewResumeRunProcessor(app *ApplicationService, opts ResumeRunProcessorOptions) *ResumeRunProcessor {
	workerID := strings.TrimSpace(opts.WorkerID)
	if workerID == "" {
		workerID = defaultResumeRunProcessorWorkerID
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultRunProcessorBatchSize
	}
	eventSink := opts.EventSink
	if eventSink == nil {
		eventSink = NewApplicationRunEventSink(app)
	}

	return &ResumeRunProcessor{
		app:       app,
		eventSink: eventSink,
		workerID:  workerID,
		batchSize: batchSize,
	}
}

func (p *ResumeRunProcessor) ProcessQueuedResumeRuns(ctx context.Context) error {
	if p == nil || p.app == nil {
		return fmt.Errorf("agent resume run processor application service is required")
	}

	claimed, err := p.app.ClaimQueuedResumeRuns(ctx, &ClaimQueuedResumeRunsRequest{
		WorkerID: p.workerID,
		Limit:    p.batchSize,
	})
	if err != nil {
		return err
	}

	for _, run := range claimed.Runs {
		if err := p.processResumeRun(ctx, run); err != nil {
			return err
		}
	}

	return nil
}

func (p *ResumeRunProcessor) processResumeRun(ctx context.Context, run *RunSummary) error {
	if run == nil {
		return nil
	}

	resume, err := parseResumeRunPayload(run.Command)
	p.emitResumeRunStarted(ctx, run, resume)
	if err != nil {
		return p.failResumeRun(ctx, run, resume, resumeRunPayloadInvalidCode, err.Error())
	}

	checkpointResp, err := p.app.GetCheckpoint(ctx, &GetCheckpointRequest{CheckpointID: resume.CheckpointID})
	if err != nil {
		return p.failResumeRun(ctx, run, resume, resumeRunCheckpointInvalidCode, err.Error())
	}
	if checkpointResp == nil || checkpointResp.Checkpoint == nil {
		return p.failResumeRun(ctx, run, resume, resumeRunCheckpointInvalidCode, "checkpoint is missing")
	}

	checkpoint := checkpointResp.Checkpoint
	if strings.TrimSpace(resume.CheckpointNS) == "" {
		resume.CheckpointNS = checkpoint.CheckpointNS
	}
	if err := validateResumeCheckpoint(run, checkpoint); err != nil {
		return p.failResumeRun(ctx, run, resume, resumeRunCheckpointInvalidCode, err.Error())
	}

	resumeInput, err := loadHarnessResumeInput(run, resume, checkpoint)
	if err != nil {
		return p.failResumeRun(ctx, run, resume, resumeRunCheckpointInvalidCode, err.Error())
	}
	p.emitResumeRunLoaded(ctx, run, resumeInput)

	return p.failResumeRun(ctx, run, resume, resumeRunReplayNotImplementedCode, "checkpoint replay is not implemented yet")
}

func parseResumeRunPayload(command string) (resumeRunPayload, error) {
	payload := resumeRunPayload{}
	if strings.TrimSpace(command) == "" {
		return payload, fmt.Errorf("resume run is missing command.resume.checkpoint_id")
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(command), &root); err != nil {
		return payload, fmt.Errorf("resume run command is invalid json")
	}

	resume, ok := root["resume"].(map[string]any)
	if !ok {
		return payload, fmt.Errorf("resume run is missing command.resume.checkpoint_id")
	}

	payload.CheckpointID = resumePayloadInt64(resume["checkpoint_id"])
	payload.CheckpointNS = resumePayloadString(resume["checkpoint_ns"])
	payload.ResumeFrom = resumePayloadString(resume["resume_from"])
	if strings.TrimSpace(payload.ResumeFrom) == "" {
		payload.ResumeFrom = "pending_sends"
	}
	if payload.CheckpointID <= 0 {
		return payload, fmt.Errorf("resume run is missing command.resume.checkpoint_id")
	}

	return payload, nil
}

func validateResumeCheckpoint(run *RunSummary, checkpoint *CheckpointSummary) error {
	if checkpoint.ThreadID != run.ThreadID {
		return fmt.Errorf("checkpoint does not belong to resume run thread")
	}
	if strings.TrimSpace(checkpoint.ChannelValues) == "" || strings.TrimSpace(checkpoint.ChannelValues) == "{}" {
		return fmt.Errorf("checkpoint channel values are missing")
	}
	if strings.TrimSpace(checkpoint.PendingSends) == "" || strings.TrimSpace(checkpoint.PendingSends) == "[]" {
		return fmt.Errorf("checkpoint pending sends are missing")
	}

	return nil
}

func loadHarnessResumeInput(run *RunSummary, resume resumeRunPayload, checkpoint *CheckpointSummary) (*HarnessResumeInput, error) {
	if run == nil {
		return nil, fmt.Errorf("resume run is required")
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("checkpoint is missing")
	}
	if err := validateResumeCheckpoint(run, checkpoint); err != nil {
		return nil, err
	}

	channelValues, err := decodeResumeJSONMap(checkpoint.ChannelValues, "checkpoint channel values")
	if err != nil {
		return nil, err
	}
	channelVersions, err := decodeResumeJSONMap(checkpoint.ChannelVersions, "checkpoint channel versions")
	if err != nil {
		return nil, err
	}
	metadata, err := decodeResumeJSONMap(checkpoint.Metadata, "checkpoint metadata")
	if err != nil {
		return nil, err
	}
	pendingValues, err := decodeResumeJSONArray(checkpoint.PendingSends, "checkpoint pending sends")
	if err != nil {
		return nil, err
	}

	messages := resumeMessages(channelValues["messages"])
	state := AgentHarnessState{
		Steps:  resumeExecutedSteps(channelValues["steps"], messages),
		Memory: resumeMemoryContext(channelValues["memory"]),
	}
	state.StepIndex = len(state.Steps)
	state.Results = resumeStepResults(state.Steps)
	pendingSteps := resumePendingSteps(pendingValues)
	if len(pendingSteps) == 0 {
		return nil, fmt.Errorf("checkpoint pending sends are missing")
	}

	checkpointNS := strings.TrimSpace(resume.CheckpointNS)
	if checkpointNS == "" {
		checkpointNS = checkpoint.CheckpointNS
	}

	return &HarnessResumeInput{
		ThreadID:        run.ThreadID,
		RunID:           run.RunID,
		SourceRunID:     checkpoint.RunID,
		CheckpointID:    checkpoint.CheckpointID,
		CheckpointNS:    checkpointNS,
		ResumeFrom:      resumeDefaultString(resume.ResumeFrom, "pending_sends"),
		ChannelValues:   channelValues,
		ChannelVersions: channelVersions,
		Metadata:        metadata,
		Messages:        messages,
		PendingSteps:    pendingSteps,
		State:           state,
	}, nil
}

func decodeResumeJSONMap(raw string, field string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err != nil || result == nil {
		return nil, fmt.Errorf("%s is invalid", field)
	}

	return result, nil
}

func decodeResumeJSONArray(raw string, field string) ([]any, error) {
	var result []any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err != nil || result == nil {
		return nil, fmt.Errorf("%s is invalid", field)
	}

	return result, nil
}

func resumeMessages(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return []map[string]any{}
	}

	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		messages = append(messages, copyResumeMap(payload))
	}

	return messages
}

func resumeExecutedSteps(value any, messages []map[string]any) []AgentExecutedStep {
	items, ok := value.([]any)
	if !ok {
		return []AgentExecutedStep{}
	}

	messagesByStepID := resumeMessagesByStepID(messages)
	steps := make([]AgentExecutedStep, 0, len(items))
	for _, item := range items {
		payload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		stepID := resumePayloadString(payload["step_id"])
		if stepID == "" {
			continue
		}
		step := AgentExecutedStep{
			StepID:    stepID,
			StepType:  resumeStepType(payload["step_type"]),
			StepName:  resumeDefaultString(resumePayloadString(payload["step_name"]), resumePayloadString(payload["node"])),
			StepIndex: int(resumePayloadInt64(payload["step_index"])),
			Final:     resumePayloadBool(payload["final"]),
			Message:   messagesByStepID[stepID],
		}
		steps = append(steps, step)
	}

	return steps
}

func resumeStepResults(steps []AgentExecutedStep) []AgentStepResult {
	results := make([]AgentStepResult, 0, len(steps))
	for _, step := range steps {
		results = append(results, AgentStepResult{
			Message: step.Message,
			Final:   step.Final,
		})
	}

	return results
}

func resumeMemoryContext(value any) AgentMemoryContext {
	payload, ok := value.(map[string]any)
	if !ok {
		return AgentMemoryContext{}
	}
	items, ok := payload["items"].([]any)
	if !ok {
		return AgentMemoryContext{}
	}

	memories := make([]AgentMemory, 0, len(items))
	for _, item := range items {
		memoryPayload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		memory := AgentMemory{
			ID:       resumePayloadString(memoryPayload["id"]),
			Scope:    resumePayloadString(memoryPayload["scope"]),
			Content:  resumePayloadString(memoryPayload["content"]),
			Metadata: resumePayloadString(memoryPayload["metadata"]),
			Score:    resumePayloadFloat64(memoryPayload["score"]),
		}
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		memories = append(memories, memory)
	}

	return AgentMemoryContext{Items: memories}
}

func resumePendingSteps(items []any) []AgentStep {
	steps := make([]AgentStep, 0, len(items))
	for _, item := range items {
		payload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := resumeDefaultString(resumePayloadString(payload["step_name"]), resumePayloadString(payload["node"]))
		step := AgentStep{
			ID:            resumePayloadString(payload["step_id"]),
			Type:          resumeStepType(payload["step_type"]),
			Name:          name,
			ToolName:      resumePayloadString(payload["tool_name"]),
			ToolArguments: resumePayloadString(payload["tool_arguments"]),
			Final:         resumePayloadBool(payload["final"]),
		}
		if strings.TrimSpace(step.ID) == "" {
			step.ID = name
		}
		if strings.TrimSpace(step.Name) == "" {
			step.Name = step.ID
		}
		if strings.TrimSpace(step.ID) == "" && strings.TrimSpace(step.Name) == "" {
			continue
		}
		steps = append(steps, step)
	}

	return steps
}

func resumeMessagesByStepID(messages []map[string]any) map[string]string {
	result := make(map[string]string, len(messages))
	for _, message := range messages {
		stepID := resumePayloadString(message["step_id"])
		content := resumePayloadString(message["content"])
		if stepID == "" || content == "" {
			continue
		}
		result[stepID] = content
	}

	return result
}

func copyResumeMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}

	return result
}

func resumeStepType(value any) AgentStepType {
	stepType := AgentStepType(resumePayloadString(value))
	switch stepType {
	case AgentStepTypeModel, AgentStepTypeTool:
		return stepType
	default:
		return AgentStepTypeModel
	}
}

func (p *ResumeRunProcessor) failResumeRun(
	ctx context.Context,
	run *RunSummary,
	resume resumeRunPayload,
	code string,
	message string,
) error {
	p.emitResumeRunFailed(ctx, run, resume, code, message)

	_, err := p.app.FailRun(ctx, &UpdateRunStatusRequest{
		RunID:        run.RunID,
		From:         RunStatusRunning,
		WorkerID:     p.workerID,
		ErrorCode:    code,
		ErrorMessage: message,
	})

	return err
}

func (p *ResumeRunProcessor) emitResumeRunStarted(ctx context.Context, run *RunSummary, resume resumeRunPayload) {
	payload := p.resumeRunEventPayload(resume, map[string]any{
		"status":    string(RunStatusRunning),
		"worker_id": p.workerID,
	})
	p.emitRunEvent(ctx, run, "run.resume.started", payload)
}

func (p *ResumeRunProcessor) emitResumeRunLoaded(ctx context.Context, run *RunSummary, input *HarnessResumeInput) {
	if input == nil {
		return
	}

	payload := p.resumeRunEventPayload(resumeRunPayload{
		CheckpointID: input.CheckpointID,
		CheckpointNS: input.CheckpointNS,
		ResumeFrom:   input.ResumeFrom,
	}, map[string]any{
		"status":                string(RunStatusRunning),
		"worker_id":             p.workerID,
		"checkpoint_step_count": len(input.State.Steps),
		"pending_step_count":    len(input.PendingSteps),
		"message_count":         len(input.Messages),
	})
	p.emitRunEvent(ctx, run, "run.resume.loaded", payload)
}

func (p *ResumeRunProcessor) emitResumeRunFailed(
	ctx context.Context,
	run *RunSummary,
	resume resumeRunPayload,
	code string,
	message string,
) {
	payload := p.resumeRunEventPayload(resume, map[string]any{
		"status":        string(RunStatusFailed),
		"worker_id":     p.workerID,
		"error_code":    code,
		"error_message": message,
	})
	p.emitRunEvent(ctx, run, "run.failed", payload)
}

func (p *ResumeRunProcessor) resumeRunEventPayload(resume resumeRunPayload, payload map[string]any) map[string]any {
	if resume.CheckpointID > 0 {
		payload["checkpoint_id"] = strconv.FormatInt(resume.CheckpointID, 10)
	}
	if strings.TrimSpace(resume.CheckpointNS) != "" {
		payload["checkpoint_ns"] = resume.CheckpointNS
	}
	if strings.TrimSpace(resume.ResumeFrom) != "" {
		payload["resume_from"] = resume.ResumeFrom
	}

	return payload
}

func (p *ResumeRunProcessor) emitRunEvent(ctx context.Context, run *RunSummary, eventType string, payload map[string]any) {
	if p == nil || run == nil {
		return
	}

	emitRunEvent(ctx, p.eventSink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: eventType,
		Payload:   encodeRunEventPayload(ctx, payload),
	})
}

func resumePayloadString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func resumeDefaultString(value, fallback string) string {
	normalized := strings.TrimSpace(value)
	if normalized != "" {
		return normalized
	}

	return strings.TrimSpace(fallback)
}

func resumePayloadInt64(value any) int64 {
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func resumePayloadBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func resumePayloadFloat64(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
