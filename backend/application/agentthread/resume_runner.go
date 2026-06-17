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
