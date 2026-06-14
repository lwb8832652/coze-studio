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
	"fmt"
	"strings"
)

const defaultRunProcessorWorkerID = "agent-harness"
const defaultRunProcessorBatchSize int32 = 10

type RunExecutor interface {
	Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
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
	if p == nil || p.app == nil {
		return fmt.Errorf("agent run processor application service is required")
	}
	if p.executor == nil {
		return fmt.Errorf("agent run executor is required")
	}

	claimed, err := p.app.ClaimPendingRuns(ctx, &ClaimPendingRunsRequest{
		WorkerID: p.workerID,
		Limit:    p.batchSize,
	})
	if err != nil {
		return err
	}

	for _, run := range claimed.Runs {
		if err := p.processRun(ctx, run); err != nil {
			return err
		}
	}

	return nil
}

func (p *RunProcessor) processRun(ctx context.Context, run *RunSummary) error {
	if run == nil {
		return nil
	}

	p.emitRunEvent(ctx, run, "run.started", map[string]any{
		"status":    string(RunStatusRunning),
		"worker_id": p.workerID,
	})

	result, err := p.executor.Execute(ctx, run)
	if err != nil {
		p.emitRunFailedEvent(ctx, run, "executor_error", err.Error())

		return p.failRun(ctx, run, "executor_error", err.Error())
	}

	message := strings.TrimSpace(resultMessage(result))
	if message == "" {
		p.emitRunFailedEvent(ctx, run, "empty_executor_result", "executor returned empty assistant message")

		return p.failRun(ctx, run, "empty_executor_result", "executor returned empty assistant message")
	}

	if _, err := p.app.AppendMessage(ctx, &AppendMessageRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Role:     MessageRoleAssistant,
		Content:  message,
		Metadata: resultMetadata(result),
	}); err != nil {
		p.emitRunFailedEvent(ctx, run, "append_message_failed", err.Error())

		return p.failRun(ctx, run, "append_message_failed", err.Error())
	}

	if _, err := p.app.CompleteRun(ctx, &UpdateRunStatusRequest{
		RunID:    run.RunID,
		From:     RunStatusRunning,
		WorkerID: p.workerID,
	}); err != nil {
		return err
	}

	p.emitRunEvent(ctx, run, "run.completed", map[string]any{
		"status":    string(RunStatusSucceeded),
		"worker_id": p.workerID,
	})

	return nil
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
