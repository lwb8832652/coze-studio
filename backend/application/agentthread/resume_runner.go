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
	"strconv"
	"strings"
	"time"

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const defaultResumeRunProcessorWorkerID = "agent-harness-resume"

const (
	resumeRunPayloadInvalidCode    = "checkpoint_resume_payload_invalid"
	resumeRunCheckpointInvalidCode = "checkpoint_resume_checkpoint_invalid"
	resumeRunExecutorErrorCode     = "checkpoint_resume_executor_error"
	resumeRunEmptyResultCode       = "checkpoint_resume_empty_executor_result"
)

type ResumeRunProcessorOptions struct {
	WorkerID          string
	BatchSize         int32
	EventSink         RunEventSink
	Executor          ResumeRunExecutor
	MetricsCollector  RuntimeMetricsCollector
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	LeaseClock        RunLeaseClock
}

type ResumeRunProcessor struct {
	app              *ApplicationService
	executor         ResumeRunExecutor
	eventSink        RunEventSink
	metricsCollector RuntimeMetricsCollector
	workerID         string
	batchSize        int32
	leaseConfig      runLeaseHeartbeatConfig
}

type ResumeRunProcessResult struct {
	ClaimedRuns     int
	ProcessedRuns   int
	InterruptedRuns int
	CanceledRuns    int
	SucceededRuns   int
	FailedRuns      int
	ErroredRuns     int
}

type ResumeRunExecutor interface {
	Resume(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error)
}

type ResumeRunExecutorFunc func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error)

func (f ResumeRunExecutorFunc) Resume(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
	if f == nil {
		return nil, fmt.Errorf("resume run executor function is required")
	}

	return f(ctx, run, input)
}

type resumeRunPayload struct {
	CheckpointID int64
	CheckpointNS string
	ResumeFrom   string
	Targets      map[string]any
}

type resumeRunProcessOutcome string

const (
	resumeRunProcessSkipped     resumeRunProcessOutcome = "skipped"
	resumeRunProcessInterrupted resumeRunProcessOutcome = "interrupted"
	resumeRunProcessCanceled    resumeRunProcessOutcome = "canceled"
	resumeRunProcessSucceeded   resumeRunProcessOutcome = "succeeded"
	resumeRunProcessFailed      resumeRunProcessOutcome = "failed"
	resumeRunProcessErrored     resumeRunProcessOutcome = "errored"
)

type HarnessResumeInput struct {
	Runtime          RuntimeMode
	RuntimeKey       string
	ADKCheckpoint    *ADKCheckpointEnvelope
	ADKResumeTargets map[string]any
	ThreadID         int64
	RunID            int64
	SourceRunID      int64
	CheckpointID     int64
	CheckpointNS     string
	ResumeFrom       string
	ChannelValues    map[string]any
	ChannelVersions  map[string]any
	Metadata         map[string]any
	Messages         []map[string]any
	PendingSteps     []AgentStep
	State            AgentHarnessState
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
	executor := opts.Executor
	if executor == nil && app != nil {
		executor = NewApplicationHarnessExecutor(app)
	}

	return &ResumeRunProcessor{
		app:              app,
		executor:         executor,
		eventSink:        eventSink,
		metricsCollector: opts.MetricsCollector,
		workerID:         workerID,
		batchSize:        batchSize,
		leaseConfig:      normalizeRunLeaseHeartbeatConfig(opts.LeaseTTL, opts.HeartbeatInterval, opts.LeaseClock),
	}
}

func (p *ResumeRunProcessor) ProcessQueuedResumeRuns(ctx context.Context) error {
	_, err := p.ProcessQueuedResumeRunsWithResult(ctx)

	return err
}

func (p *ResumeRunProcessor) ProcessQueuedResumeRunsWithResult(ctx context.Context) (ResumeRunProcessResult, error) {
	result := ResumeRunProcessResult{}
	if p == nil || p.app == nil {
		return result, fmt.Errorf("agent resume run processor application service is required")
	}
	if p.executor == nil {
		return result, fmt.Errorf("agent resume run executor is required")
	}

	claimed, err := p.app.ClaimQueuedResumeRuns(ctx, &ClaimQueuedResumeRunsRequest{
		WorkerID:       p.workerID,
		Limit:          p.batchSize,
		Now:            p.leaseConfig.Clock.Now().UnixMilli(),
		LeaseTTLMillis: p.leaseConfig.TTL.Milliseconds(),
	})
	if err != nil {
		return result, err
	}

	result.ClaimedRuns = len(claimed.Runs)
	var batchErr error
	for _, run := range claimed.Runs {
		runCtx, heartbeat := startRunLeaseHeartbeat(ctx, p.app, run, p.leaseConfig)
		outcome, err := p.processResumeRun(runCtx, run, heartbeat)
		if heartbeatErr := heartbeat.Stop(); err == nil && heartbeatErr != nil &&
			!(outcome == resumeRunProcessCanceled && errors.Is(heartbeatErr, domainrepo.ErrRunLeaseLost)) {
			outcome = resumeRunProcessErrored
			err = heartbeatErr
		}
		heartbeat.Close()
		switch outcome {
		case resumeRunProcessInterrupted:
			result.ProcessedRuns++
			result.InterruptedRuns++
		case resumeRunProcessCanceled:
			result.ProcessedRuns++
			result.CanceledRuns++
		case resumeRunProcessSucceeded:
			result.ProcessedRuns++
			result.SucceededRuns++
		case resumeRunProcessFailed:
			result.ProcessedRuns++
			result.FailedRuns++
		case resumeRunProcessErrored:
			result.ProcessedRuns++
		}
		if err != nil {
			result.ErroredRuns++
			batchErr = errors.Join(batchErr, fmt.Errorf("process resume run %d: %w", runSummaryID(run), err))
			if releaseErr := releaseUnfinalizedRunLease(ctx, p.app, run, RunStatusQueued); releaseErr != nil {
				batchErr = errors.Join(batchErr, fmt.Errorf("release resume run %d lease: %w", runSummaryID(run), releaseErr))
			}
		}
	}

	return result, batchErr
}

func (p *ResumeRunProcessor) processResumeRun(
	ctx context.Context,
	run *RunSummary,
	heartbeat *runLeaseHeartbeat,
) (resumeRunProcessOutcome, error) {
	if run == nil {
		return resumeRunProcessSkipped, nil
	}

	resume, err := parseResumeRunPayload(run.Command)
	p.emitResumeRunStarted(ctx, run, resume)
	if err != nil {
		return p.finalizeFailedResumeRun(ctx, run, heartbeat, resume, resumeRunPayloadInvalidCode, err.Error())
	}

	checkpointResp, err := p.app.GetCheckpoint(ctx, &GetCheckpointRequest{CheckpointID: resume.CheckpointID})
	if err != nil {
		return p.finalizeFailedResumeRun(ctx, run, heartbeat, resume, resumeRunCheckpointInvalidCode, err.Error())
	}
	if checkpointResp == nil || checkpointResp.Checkpoint == nil {
		return p.finalizeFailedResumeRun(ctx, run, heartbeat, resume, resumeRunCheckpointInvalidCode, "checkpoint is missing")
	}

	checkpoint := checkpointResp.Checkpoint
	if strings.TrimSpace(resume.CheckpointNS) == "" {
		resume.CheckpointNS = checkpoint.CheckpointNS
	}
	resumeInput, err := loadResumeInput(run, resume, checkpoint)
	if err != nil {
		return p.finalizeFailedResumeRun(ctx, run, heartbeat, resume, resumeRunCheckpointInvalidCode, err.Error())
	}
	p.emitResumeRunLoaded(ctx, run, resumeInput)

	result, err := p.executor.Resume(ctx, run, resumeInput)
	if canceledRun, canceled, lookupErr := durableCanceledRunAfterLeaseLoss(ctx, p.app, run); lookupErr != nil {
		_ = heartbeat.Stop()
		return resumeRunProcessErrored, lookupErr
	} else if canceled {
		_ = heartbeat.Stop()
		p.recordRuntimeRunTerminal(ctx, run, canceledRun, runtimeMetricResultCanceled, runtimeMetricErrorNone)
		return resumeRunProcessCanceled, nil
	}
	if err != nil {
		var canceled *RunCanceledError
		if errors.As(err, &canceled) {
			if heartbeatErr := heartbeat.Stop(); heartbeatErr != nil {
				return resumeRunProcessErrored, heartbeatErr
			}
			terminalRun, cancelErr := requestDurableRunCancellation(
				ctx,
				p.app,
				run,
				p.leaseConfig.Clock.Now().UnixMilli(),
			)
			if cancelErr != nil {
				return resumeRunProcessErrored, cancelErr
			}
			p.recordRuntimeRunTerminal(ctx, run, terminalRun, runtimeMetricResultCanceled, runtimeMetricErrorNone)
			return resumeRunProcessCanceled, nil
		}
		var interrupted *RunInterruptedError
		if errors.As(err, &interrupted) {
			if abortErr := stopRunLeaseHeartbeat(ctx, heartbeat); abortErr != nil {
				return resumeRunProcessErrored, abortErr
			}
			transitionResp, transitionErr := p.app.InterruptRun(ctx, &UpdateRunStatusRequest{
				RunID:               run.RunID,
				From:                RunStatusRunning,
				WorkerID:            p.workerID,
				LeaseOwner:          run.LeaseOwner,
				LeaseToken:          run.LeaseToken,
				ExecutionGeneration: run.ExecutionGeneration,
			})
			if transitionErr != nil {
				return resumeRunProcessErrored, transitionErr
			}
			if !interrupted.EventPersisted {
				p.emitResumeRunInterrupted(ctx, run, resume, interrupted)
			}
			p.recordRuntimeRunTerminal(
				ctx,
				run,
				updateRunStatusResponseRun(transitionResp),
				runtimeMetricResultInterrupted,
				runtimeMetricErrorNone,
			)

			return resumeRunProcessInterrupted, nil
		}
		return p.finalizeFailedResumeRun(ctx, run, heartbeat, resume, resumeRunExecutorErrorCode, err.Error())
	}
	if cause := context.Cause(ctx); cause != nil {
		_ = heartbeat.Stop()
		return resumeRunProcessErrored, cause
	}

	message := strings.TrimSpace(resultMessage(result))
	if message == "" {
		return p.finalizeFailedResumeRun(ctx, run, heartbeat, resume, resumeRunEmptyResultCode, "resume executor returned empty assistant message")
	}

	if abortErr := stopRunLeaseHeartbeat(ctx, heartbeat); abortErr != nil {
		return resumeRunProcessErrored, abortErr
	}

	finalized, err := p.app.FinalizeRunSuccess(ctx, &FinalizeRunSuccessRequest{
		RunID:               run.RunID,
		ThreadID:            run.ThreadID,
		LeaseOwner:          run.LeaseOwner,
		LeaseToken:          run.LeaseToken,
		ExecutionGeneration: run.ExecutionGeneration,
		Now:                 p.leaseConfig.Clock.Now().UnixMilli(),
		Message:             message,
		MessageMetadata:     resultMetadata(result),
	})
	if err != nil {
		if errors.Is(err, domainrepo.ErrRunCanceled) {
			p.recordRuntimeRunTerminal(ctx, run, nil, runtimeMetricResultCanceled, runtimeMetricErrorNone)
			return resumeRunProcessCanceled, nil
		}
		return resumeRunProcessErrored, err
	}

	p.emitResumeRunCompleted(ctx, run, resume)
	p.recordRuntimeRunTerminal(
		ctx,
		run,
		finalized.Run,
		runtimeMetricResultSuccess,
		runtimeMetricErrorNone,
	)

	return resumeRunProcessSucceeded, nil
}

func (p *ResumeRunProcessor) emitResumeRunInterrupted(
	ctx context.Context,
	run *RunSummary,
	resume resumeRunPayload,
	interrupted *RunInterruptedError,
) {
	payload := p.resumeRunEventPayload(resume, map[string]any{
		"status":    string(RunStatusInterrupted),
		"worker_id": p.workerID,
	})
	if interrupted != nil {
		payload["checkpoint_key"] = interrupted.CheckpointKey
		payload["interrupts"] = interrupted.Interrupts
	}
	p.emitRunEvent(ctx, run, "run.interrupted", payload)
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
	targets, err := resumePayloadTargets(resume["targets"])
	if err != nil {
		return payload, err
	}
	payload.Targets = targets
	if strings.TrimSpace(payload.ResumeFrom) == "" {
		payload.ResumeFrom = "pending_sends"
	}
	if payload.CheckpointID <= 0 {
		return payload, fmt.Errorf("resume run is missing command.resume.checkpoint_id")
	}

	return payload, nil
}

func resumePayloadTargets(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	targets, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resume run command.resume.targets must be an object")
	}
	if len(targets) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(targets))
	for key, target := range targets {
		targetID := strings.TrimSpace(key)
		if targetID == "" {
			return nil, fmt.Errorf("resume run command.resume.targets contains empty target id")
		}
		out[targetID] = copyResumeValue(target)
	}

	return out, nil
}

func validateResumeCheckpoint(run *RunSummary, checkpoint *CheckpointSummary) error {
	if run == nil {
		return fmt.Errorf("resume run is required")
	}
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is missing")
	}
	if checkpoint.ThreadID != run.ThreadID {
		return fmt.Errorf("checkpoint does not belong to resume run thread")
	}
	if strings.TrimSpace(checkpoint.ChannelValues) == "" || strings.TrimSpace(checkpoint.ChannelValues) == "{}" {
		return fmt.Errorf("checkpoint channel values are missing")
	}

	mode, err := runtimeModeFromCheckpoint(checkpoint)
	if err != nil {
		return err
	}
	if mode == RuntimeModeEinoADK {
		if checkpoint.RuntimeDeletedAt > 0 {
			return fmt.Errorf("eino checkpoint is no longer active")
		}
		envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
		if err != nil {
			return fmt.Errorf("decode eino checkpoint: %w", err)
		}
		if checkpoint.RuntimeKey != "" && checkpoint.RuntimeKey != envelope.RuntimeKey {
			return fmt.Errorf("checkpoint runtime key does not match envelope")
		}
		if checkpoint.EnvelopeVersion != 0 &&
			int(checkpoint.EnvelopeVersion) != envelope.EnvelopeVersion {
			return fmt.Errorf("checkpoint envelope version does not match indexed version")
		}

		return nil
	}

	if strings.TrimSpace(checkpoint.PendingSends) == "" ||
		strings.TrimSpace(checkpoint.PendingSends) == "[]" {
		return fmt.Errorf("checkpoint pending sends are missing")
	}

	return nil
}

func loadResumeInput(
	run *RunSummary,
	resume resumeRunPayload,
	checkpoint *CheckpointSummary,
) (*HarnessResumeInput, error) {
	mode, err := runtimeModeFromCheckpoint(checkpoint)
	if err != nil {
		return nil, err
	}
	if mode == RuntimeModeEinoADK {
		return loadADKResumeInput(run, resume, checkpoint)
	}

	return loadHarnessResumeInput(run, resume, checkpoint)
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
	mode, err := runtimeModeFromCheckpoint(checkpoint)
	if err != nil {
		return nil, err
	}
	if mode != RuntimeModeLegacy {
		return nil, fmt.Errorf("checkpoint is not a legacy harness checkpoint")
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
		Skills: resumeSkillContext(channelValues["skills"]),
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
		Runtime:         RuntimeModeLegacy,
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

func loadADKResumeInput(
	run *RunSummary,
	resume resumeRunPayload,
	checkpoint *CheckpointSummary,
) (*HarnessResumeInput, error) {
	if err := validateResumeCheckpoint(run, checkpoint); err != nil {
		return nil, err
	}
	mode, err := runtimeModeFromCheckpoint(checkpoint)
	if err != nil {
		return nil, err
	}
	if mode != RuntimeModeEinoADK {
		return nil, fmt.Errorf("checkpoint is not an eino adk checkpoint")
	}

	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
	if err != nil {
		return nil, fmt.Errorf("decode eino checkpoint: %w", err)
	}
	if envelope.RuntimeVersion != adkCheckpointRuntimeVersion {
		return nil, fmt.Errorf(
			"checkpoint runtime version %s is incompatible with %s",
			envelope.RuntimeVersion,
			adkCheckpointRuntimeVersion,
		)
	}
	metadata, err := decodeResumeJSONMap(checkpoint.Metadata, "checkpoint metadata")
	if err != nil {
		return nil, err
	}

	checkpointNS := strings.TrimSpace(resume.CheckpointNS)
	if checkpointNS == "" {
		checkpointNS = checkpoint.CheckpointNS
	}

	targets, err := adkResumeTargetsFromPayload(envelope.Interrupts, resume.Targets)
	if err != nil {
		return nil, err
	}

	return &HarnessResumeInput{
		Runtime:          RuntimeModeEinoADK,
		RuntimeKey:       envelope.RuntimeKey,
		ADKCheckpoint:    &envelope,
		ADKResumeTargets: targets,
		ThreadID:         run.ThreadID,
		RunID:            run.RunID,
		SourceRunID:      checkpoint.RunID,
		CheckpointID:     checkpoint.CheckpointID,
		CheckpointNS:     checkpointNS,
		ResumeFrom:       resumeDefaultString(resume.ResumeFrom, "interrupt"),
		Metadata:         metadata,
	}, nil
}

func adkResumeTargetsFromPayload(
	interrupts map[string]ADKInterruptItem,
	targets map[string]any,
) (map[string]any, error) {
	if len(targets) == 0 {
		return adkResumeTargets(interrupts), nil
	}

	out := make(map[string]any, len(targets))
	for targetID, data := range targets {
		if _, ok := interrupts[targetID]; !ok {
			return nil, fmt.Errorf("resume target %s is not present in checkpoint interrupts", targetID)
		}
		out[targetID] = copyResumeValue(data)
	}

	return out, nil
}

func adkResumeTargets(interrupts map[string]ADKInterruptItem) map[string]any {
	if len(interrupts) == 0 {
		return nil
	}

	targets := make(map[string]any, len(interrupts))
	for key, item := range interrupts {
		targetID := strings.TrimSpace(item.ID)
		if targetID == "" {
			targetID = strings.TrimSpace(key)
		}
		if targetID != "" {
			targets[targetID] = nil
		}
	}

	return targets
}

func runtimeModeFromCheckpoint(checkpoint *CheckpointSummary) (RuntimeMode, error) {
	if checkpoint == nil {
		return "", fmt.Errorf("checkpoint is missing")
	}

	indexedMode := RuntimeMode(strings.TrimSpace(checkpoint.RuntimeType))
	if indexedMode == "" {
		indexedMode = RuntimeModeLegacy
	}

	metadataMode := RuntimeMode("")
	if strings.TrimSpace(checkpoint.Metadata) != "" {
		var metadata struct {
			Runtime string `json:"runtime"`
		}
		if err := json.Unmarshal([]byte(checkpoint.Metadata), &metadata); err != nil {
			return "", fmt.Errorf("checkpoint metadata is invalid")
		}
		metadataMode = RuntimeMode(strings.TrimSpace(metadata.Runtime))
	}
	switch metadataMode {
	case "", RuntimeModeLegacy, RuntimeModeEinoADK:
	default:
		if indexedMode == RuntimeModeLegacy {
			metadataMode = ""
		} else {
			return "", fmt.Errorf("unsupported checkpoint metadata runtime: %s", metadataMode)
		}
	}

	if metadataMode != "" && metadataMode != indexedMode {
		if indexedMode == RuntimeModeLegacy && strings.TrimSpace(checkpoint.RuntimeKey) == "" {
			indexedMode = metadataMode
		} else {
			return "", fmt.Errorf(
				"checkpoint runtime metadata %s does not match indexed runtime %s",
				metadataMode,
				indexedMode,
			)
		}
	}

	switch indexedMode {
	case RuntimeModeLegacy, RuntimeModeEinoADK:
		return indexedMode, nil
	default:
		return "", fmt.Errorf("unsupported checkpoint runtime: %s", indexedMode)
	}
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
			ID:                   resumePayloadString(memoryPayload["id"]),
			Scope:                resumePayloadString(memoryPayload["scope"]),
			Content:              resumePayloadString(memoryPayload["content"]),
			Metadata:             resumePayloadString(memoryPayload["metadata"]),
			Score:                resumePayloadFloat64(memoryPayload["score"]),
			Confidence:           resumePayloadFloat64(memoryPayload["confidence"]),
			SourceType:           resumePayloadString(memoryPayload["source_type"]),
			SourceID:             resumePayloadString(memoryPayload["source_id"]),
			CorrectionOfMemoryID: int64(resumePayloadFloat64(memoryPayload["correction_of_memory_id"])),
			CorrectedAt:          int64(resumePayloadFloat64(memoryPayload["corrected_at"])),
		}
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		memories = append(memories, memory)
	}

	return AgentMemoryContext{Items: memories}
}

func resumeSkillContext(value any) AgentSkillContext {
	payload, ok := value.(map[string]any)
	if !ok {
		return AgentSkillContext{}
	}
	items, ok := payload["items"].([]any)
	if !ok {
		return AgentSkillContext{}
	}

	skills := make([]AgentSkill, 0, len(items))
	for _, item := range items {
		skillPayload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		skill := AgentSkill{
			ID:          resumePayloadInt64(skillPayload["id"]),
			Name:        strings.TrimSpace(resumePayloadString(skillPayload["name"])),
			Description: strings.TrimSpace(resumePayloadString(skillPayload["description"])),
			Type:        strings.TrimSpace(resumePayloadString(skillPayload["type"])),
			Version:     strings.TrimSpace(resumePayloadString(skillPayload["version"])),
			Context:     strings.TrimSpace(resumePayloadString(skillPayload["context"])),
			Agent:       strings.TrimSpace(resumePayloadString(skillPayload["agent"])),
			Model:       strings.TrimSpace(resumePayloadString(skillPayload["model"])),
			Body:        strings.TrimSpace(resumePayloadString(skillPayload["body"])),
		}
		if skill.ID <= 0 || skill.Name == "" || skill.Body == "" {
			continue
		}
		skills = append(skills, skill)
	}

	return AgentSkillContext{Items: skills}
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
		result[key] = copyResumeValue(value)
	}

	return result
}

func copyResumeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyResumeMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = copyResumeValue(item)
		}
		return out
	default:
		return typed
	}
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
) (*RunSummary, error) {
	p.emitResumeRunFailed(ctx, run, resume, code, message)

	resp, err := p.app.FailRun(ctx, &UpdateRunStatusRequest{
		RunID:               run.RunID,
		From:                RunStatusRunning,
		WorkerID:            p.workerID,
		LeaseOwner:          run.LeaseOwner,
		LeaseToken:          run.LeaseToken,
		ExecutionGeneration: run.ExecutionGeneration,
		ErrorCode:           code,
		ErrorMessage:        message,
	})

	if err != nil {
		return nil, err
	}

	return updateRunStatusResponseRun(resp), nil
}

func (p *ResumeRunProcessor) finalizeFailedResumeRun(
	ctx context.Context,
	run *RunSummary,
	heartbeat *runLeaseHeartbeat,
	resume resumeRunPayload,
	code string,
	message string,
) (resumeRunProcessOutcome, error) {
	if abortErr := stopRunLeaseHeartbeat(ctx, heartbeat); abortErr != nil {
		return resumeRunProcessErrored, abortErr
	}
	terminalRun, err := p.failResumeRun(ctx, run, resume, code, message)
	if err != nil {
		return resumeRunProcessFailed, err
	}
	p.recordRuntimeRunTerminal(ctx, run, terminalRun, runtimeMetricResultFailed, code)

	return resumeRunProcessFailed, nil
}

func (p *ResumeRunProcessor) recordRuntimeRunTerminal(
	ctx context.Context,
	run *RunSummary,
	terminalRun *RunSummary,
	result string,
	errorCode string,
) {
	if p == nil {
		return
	}
	recordRuntimeRunTerminal(ctx, p.metricsCollector, run, terminalRun, "resume", result, errorCode)
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

func (p *ResumeRunProcessor) emitResumeRunCompleted(ctx context.Context, run *RunSummary, resume resumeRunPayload) {
	payload := p.resumeRunEventPayload(resume, map[string]any{
		"status":    string(RunStatusSucceeded),
		"worker_id": p.workerID,
	})
	p.emitRunEvent(ctx, run, "run.completed", payload)
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
