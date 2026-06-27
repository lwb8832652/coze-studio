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
)

const defaultRunProcessorWorkerID = "agent-harness"
const defaultRunProcessorBatchSize int32 = 10

const (
	subagentRetryNotSupportedCode    = "subagent_retry_not_supported"
	subagentRetryNotSupportedMessage = "subagent retry executor is not implemented"
)

type RunExecutor interface {
	Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
}

type SubagentRetryRunExecutor interface {
	ExecuteSubagentRetry(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
}

type SubagentRetryUnsupportedError struct {
	Message string
}

func (e *SubagentRetryUnsupportedError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return subagentRetryNotSupportedMessage
	}

	return e.Message
}

type RunExecutorFunc func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)

func (f RunExecutorFunc) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
	if f == nil {
		return nil, fmt.Errorf("run executor function is required")
	}

	return f(ctx, run)
}

type RunExecutionResult struct {
	Message  string
	Metadata string
}

type RunProcessorOptions struct {
	WorkerID  string
	BatchSize int32
	EventSink RunEventSink
}

type RunProcessor struct {
	app       *ApplicationService
	executor  RunExecutor
	eventSink RunEventSink
	workerID  string
	batchSize int32
}

type RunProcessResult struct {
	ClaimedRuns     int
	ProcessedRuns   int
	InterruptedRuns int
	CanceledRuns    int
	SucceededRuns   int
	FailedRuns      int
	ErroredRuns     int
}

type runProcessOutcome string

const (
	runProcessSkipped     runProcessOutcome = "skipped"
	runProcessInterrupted runProcessOutcome = "interrupted"
	runProcessCanceled    runProcessOutcome = "canceled"
	runProcessSucceeded   runProcessOutcome = "succeeded"
	runProcessFailed      runProcessOutcome = "failed"
	runProcessErrored     runProcessOutcome = "errored"
)

func NewRunProcessor(app *ApplicationService, executor RunExecutor, opts RunProcessorOptions) *RunProcessor {
	workerID := strings.TrimSpace(opts.WorkerID)
	if workerID == "" {
		workerID = defaultRunProcessorWorkerID
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultRunProcessorBatchSize
	}
	eventSink := opts.EventSink
	if eventSink == nil {
		eventSink = NewApplicationRunEventSink(app)
	}

	return &RunProcessor{
		app:       app,
		executor:  executor,
		eventSink: eventSink,
		workerID:  workerID,
		batchSize: batchSize,
	}
}

func (p *RunProcessor) ProcessPendingRuns(ctx context.Context) error {
	_, err := p.ProcessPendingRunsWithResult(ctx)

	return err
}

func (p *RunProcessor) ProcessPendingRunsWithResult(ctx context.Context) (RunProcessResult, error) {
	result := RunProcessResult{}
	if p == nil || p.app == nil {
		return result, fmt.Errorf("agent run processor application service is required")
	}
	if p.executor == nil {
		return result, fmt.Errorf("agent run executor is required")
	}

	claimed, err := p.app.ClaimPendingRuns(ctx, &ClaimPendingRunsRequest{
		WorkerID: p.workerID,
		Limit:    p.batchSize,
	})
	if err != nil {
		return result, err
	}

	result.ClaimedRuns = len(claimed.Runs)
	for _, run := range claimed.Runs {
		outcome, err := p.processRun(ctx, run)
		switch outcome {
		case runProcessInterrupted:
			result.ProcessedRuns++
			result.InterruptedRuns++
		case runProcessCanceled:
			result.ProcessedRuns++
			result.CanceledRuns++
		case runProcessSucceeded:
			result.ProcessedRuns++
			result.SucceededRuns++
		case runProcessFailed:
			result.ProcessedRuns++
			result.FailedRuns++
		case runProcessErrored:
			result.ProcessedRuns++
		}
		if err != nil {
			result.ErroredRuns++

			return result, err
		}
	}

	return result, nil
}

func (p *RunProcessor) processRun(ctx context.Context, run *RunSummary) (runProcessOutcome, error) {
	if run == nil {
		return runProcessSkipped, nil
	}

	p.emitRunEvent(ctx, run, "run.started", map[string]any{
		"status":    string(RunStatusRunning),
		"worker_id": p.workerID,
	})

	if isSubagentRetryCommand(run.Command) {
		retryExecutor, ok := p.executor.(SubagentRetryRunExecutor)
		if ok {
			result, err := retryExecutor.ExecuteSubagentRetry(ctx, run)
			if isSubagentRetryUnsupportedError(err) {
				p.emitRunFailedEvent(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)

				return runProcessFailed, p.failRun(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)
			}

			return p.finalizeRunExecution(ctx, run, result, err)
		}
		p.emitRunFailedEvent(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)

		return runProcessFailed, p.failRun(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)
	}

	result, err := p.executor.Execute(ctx, run)

	return p.finalizeRunExecution(ctx, run, result, err)
}

func (p *RunProcessor) finalizeRunExecution(
	ctx context.Context,
	run *RunSummary,
	result *RunExecutionResult,
	err error,
) (runProcessOutcome, error) {
	if err != nil {
		var canceled *RunCanceledError
		if errors.As(err, &canceled) {
			p.emitRunCanceledEvent(ctx, run)
			return runProcessCanceled, nil
		}
		var interrupted *RunInterruptedError
		if errors.As(err, &interrupted) {
			if _, transitionErr := p.app.InterruptRun(ctx, &UpdateRunStatusRequest{
				RunID:    run.RunID,
				From:     RunStatusRunning,
				WorkerID: p.workerID,
			}); transitionErr != nil {
				return runProcessErrored, transitionErr
			}
			if !interrupted.EventPersisted {
				p.emitRunInterruptedEvent(ctx, run, interrupted)
			}

			return runProcessInterrupted, nil
		}
		p.emitRunFailedEvent(ctx, run, "executor_error", err.Error())

		return runProcessFailed, p.failRun(ctx, run, "executor_error", err.Error())
	}

	message := strings.TrimSpace(resultMessage(result))
	if message == "" {
		p.emitRunFailedEvent(ctx, run, "empty_executor_result", "executor returned empty assistant message")

		return runProcessFailed, p.failRun(ctx, run, "empty_executor_result", "executor returned empty assistant message")
	}

	if _, err := p.app.AppendMessage(ctx, &AppendMessageRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Role:     MessageRoleAssistant,
		Content:  message,
		Metadata: resultMetadata(result),
	}); err != nil {
		p.emitRunFailedEvent(ctx, run, "append_message_failed", err.Error())

		return runProcessFailed, p.failRun(ctx, run, "append_message_failed", err.Error())
	}

	if _, err := p.app.CompleteRun(ctx, &UpdateRunStatusRequest{
		RunID:    run.RunID,
		From:     RunStatusRunning,
		WorkerID: p.workerID,
	}); err != nil {
		return runProcessErrored, err
	}

	p.emitRunEvent(ctx, run, "run.completed", map[string]any{
		"status":    string(RunStatusSucceeded),
		"worker_id": p.workerID,
	})

	return runProcessSucceeded, nil
}

func (p *RunProcessor) emitRunCanceledEvent(ctx context.Context, run *RunSummary) {
	p.emitRunEvent(ctx, run, "run.canceled", map[string]any{
		"status":    string(RunStatusCanceled),
		"worker_id": p.workerID,
	})
}

func (p *RunProcessor) emitRunInterruptedEvent(ctx context.Context, run *RunSummary, interrupted *RunInterruptedError) {
	payload := map[string]any{
		"status":    string(RunStatusInterrupted),
		"worker_id": p.workerID,
	}
	if interrupted != nil {
		payload["checkpoint_key"] = interrupted.CheckpointKey
		payload["interrupts"] = interrupted.Interrupts
	}
	p.emitRunEvent(ctx, run, "run.interrupted", payload)
}

func (p *RunProcessor) failRun(ctx context.Context, run *RunSummary, code, message string) error {
	_, err := p.app.FailRun(ctx, &UpdateRunStatusRequest{
		RunID:        run.RunID,
		From:         RunStatusRunning,
		WorkerID:     p.workerID,
		ErrorCode:    code,
		ErrorMessage: message,
	})

	return err
}

func (p *RunProcessor) emitRunFailedEvent(ctx context.Context, run *RunSummary, code, message string) {
	p.emitRunEvent(ctx, run, "run.failed", map[string]any{
		"status":        string(RunStatusFailed),
		"worker_id":     p.workerID,
		"error_code":    code,
		"error_message": message,
	})
}

func (p *RunProcessor) emitRunEvent(ctx context.Context, run *RunSummary, eventType string, payload map[string]any) {
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

func resultMessage(result *RunExecutionResult) string {
	if result == nil {
		return ""
	}

	return result.Message
}

func resultMetadata(result *RunExecutionResult) string {
	if result == nil || strings.TrimSpace(result.Metadata) == "" {
		return `{"source":"agent_harness"}`
	}

	return result.Metadata
}

func isSubagentRetryCommand(command string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}

	var payload struct {
		SubagentRetry json.RawMessage `json:"subagent_retry"`
	}
	if err := json.Unmarshal([]byte(command), &payload); err != nil {
		return false
	}
	if len(payload.SubagentRetry) == 0 {
		return false
	}

	return strings.TrimSpace(string(payload.SubagentRetry)) != "null"
}

func isSubagentRetryUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	var unsupported *SubagentRetryUnsupportedError

	return errors.As(err, &unsupported)
}
