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
	"regexp"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const defaultRunProcessorWorkerID = "agent-harness"
const defaultRunProcessorBatchSize int32 = 10

const (
	subagentRetryNotSupportedCode    = "subagent_retry_not_supported"
	subagentRetryNotSupportedMessage = "subagent retry executor is not implemented"
)

var generatedThreadTitleThinkTagRE = regexp.MustCompile(`(?is)<think>.*?</think>`)

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
	Title    string
}

type RunTitleGenerationInput struct {
	Run              *RunSummary
	UserMessage      string
	AssistantMessage string
}

type RunTitleGenerator interface {
	GenerateTitle(ctx context.Context, input RunTitleGenerationInput) (string, error)
}

type RunProcessorOptions struct {
	WorkerID         string
	BatchSize        int32
	EventSink        RunEventSink
	TitleGenerator   RunTitleGenerator
	MetricsCollector RuntimeMetricsCollector
}

type RunProcessor struct {
	app              *ApplicationService
	executor         RunExecutor
	eventSink        RunEventSink
	titleGenerator   RunTitleGenerator
	metricsCollector RuntimeMetricsCollector
	workerID         string
	batchSize        int32
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
		app:              app,
		executor:         executor,
		eventSink:        eventSink,
		titleGenerator:   opts.TitleGenerator,
		metricsCollector: opts.MetricsCollector,
		workerID:         workerID,
		batchSize:        batchSize,
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
	p.recordRuntimeRunBacklog(ctx)
	for _, run := range claimed.Runs {
		p.recordRuntimeRunQueueDelay(ctx, run)
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

				return p.finalizeFailedRun(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)
			}

			return p.finalizeRunExecution(ctx, run, result, err)
		}
		p.emitRunFailedEvent(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)

		return p.finalizeFailedRun(ctx, run, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)
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
			p.recordRuntimeRunTerminal(ctx, run, nil, runtimeMetricResultCanceled, runtimeMetricErrorNone)
			return runProcessCanceled, nil
		}
		var interrupted *RunInterruptedError
		if errors.As(err, &interrupted) {
			transitionResp, transitionErr := p.app.InterruptRun(ctx, &UpdateRunStatusRequest{
				RunID:               run.RunID,
				From:                RunStatusRunning,
				WorkerID:            p.workerID,
				LeaseOwner:          run.LeaseOwner,
				LeaseToken:          run.LeaseToken,
				ExecutionGeneration: run.ExecutionGeneration,
			})
			if transitionErr != nil {
				return runProcessErrored, transitionErr
			}
			if !interrupted.EventPersisted {
				p.emitRunInterruptedEvent(ctx, run, interrupted)
			}
			p.recordRuntimeRunTerminal(
				ctx,
				run,
				updateRunStatusResponseRun(transitionResp),
				runtimeMetricResultInterrupted,
				runtimeMetricErrorNone,
			)

			return runProcessInterrupted, nil
		}
		p.emitRunFailedEvent(ctx, run, "executor_error", err.Error())

		return p.finalizeFailedRun(ctx, run, "executor_error", err.Error())
	}

	message := strings.TrimSpace(resultMessage(result))
	if message == "" {
		p.emitRunFailedEvent(ctx, run, "empty_executor_result", "executor returned empty assistant message")

		return p.finalizeFailedRun(ctx, run, "empty_executor_result", "executor returned empty assistant message")
	}

	if _, err := p.app.AppendMessage(ctx, &AppendMessageRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Role:     MessageRoleAssistant,
		Content:  message,
		Metadata: resultMetadata(result),
	}); err != nil {
		p.emitRunFailedEvent(ctx, run, "append_message_failed", err.Error())

		return p.finalizeFailedRun(ctx, run, "append_message_failed", err.Error())
	}

	p.syncGeneratedThreadTitle(ctx, run, result)

	completeResp, err := p.app.CompleteRun(ctx, &UpdateRunStatusRequest{
		RunID:               run.RunID,
		From:                RunStatusRunning,
		WorkerID:            p.workerID,
		LeaseOwner:          run.LeaseOwner,
		LeaseToken:          run.LeaseToken,
		ExecutionGeneration: run.ExecutionGeneration,
	})
	if err != nil {
		return runProcessErrored, err
	}

	p.emitRunEvent(ctx, run, "run.completed", map[string]any{
		"status":    string(RunStatusSucceeded),
		"worker_id": p.workerID,
	})
	p.recordRuntimeRunTerminal(
		ctx,
		run,
		updateRunStatusResponseRun(completeResp),
		runtimeMetricResultSuccess,
		runtimeMetricErrorNone,
	)

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

func (p *RunProcessor) finalizeFailedRun(
	ctx context.Context,
	run *RunSummary,
	code string,
	message string,
) (runProcessOutcome, error) {
	terminalRun, err := p.failRun(ctx, run, code, message)
	if err != nil {
		return runProcessFailed, err
	}
	p.recordRuntimeRunTerminal(ctx, run, terminalRun, runtimeMetricResultFailed, code)

	return runProcessFailed, nil
}

func (p *RunProcessor) failRun(ctx context.Context, run *RunSummary, code, message string) (*RunSummary, error) {
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

func (p *RunProcessor) recordRuntimeRunTerminal(
	ctx context.Context,
	run *RunSummary,
	terminalRun *RunSummary,
	result string,
	errorCode string,
) {
	if p == nil {
		return
	}
	recordRuntimeRunTerminal(ctx, p.metricsCollector, run, terminalRun, "", result, errorCode)
}

func (p *RunProcessor) recordRuntimeRunQueueDelay(
	ctx context.Context,
	run *RunSummary,
) {
	if p == nil {
		return
	}
	recordRuntimeRunQueueDelay(ctx, p.metricsCollector, run)
}

func (p *RunProcessor) recordRuntimeRunBacklog(ctx context.Context) {
	if p == nil || p.metricsCollector == nil || p.app == nil || p.app.ThreadSVC == nil {
		return
	}
	aggregates, err := p.app.ThreadSVC.AggregateRunBacklog(ctx, &domainservice.AggregateRunBacklogRequest{
		Statuses: []entity.RunStatus{
			entity.RunStatusPending,
			entity.RunStatusQueued,
			entity.RunStatusRunning,
			entity.RunStatusInterrupted,
		},
	})
	if err != nil {
		return
	}
	recordRuntimeRunBacklog(ctx, p.metricsCollector, aggregates)
}

func updateRunStatusResponseRun(resp *UpdateRunStatusResponse) *RunSummary {
	if resp == nil {
		return nil
	}

	return resp.Run
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

func resultTitle(result *RunExecutionResult) string {
	if result == nil {
		return ""
	}

	return result.Title
}

func (p *RunProcessor) syncGeneratedThreadTitle(
	ctx context.Context,
	run *RunSummary,
	result *RunExecutionResult,
) {
	if p == nil || p.app == nil || run == nil {
		return
	}
	if run.ParentRunID > 0 || run.RunKind == RunKindSubagent {
		return
	}

	userMessage, ok := runtimeLatestUserInputText(run.Input)
	if !ok {
		return
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return
	}
	threadResp, err := p.app.GetThread(ctx, &GetThreadRequest{ThreadID: run.ThreadID})
	if err != nil || threadResp == nil || threadResp.Thread == nil {
		return
	}
	currentTitle := strings.TrimSpace(threadResp.Thread.Title)
	initialTitle := taskThreadTitle("", userMessage)
	if currentTitle != "" && currentTitle != initialTitle {
		return
	}
	title := p.generatedThreadTitle(ctx, run, userMessage, result)
	if title == "" {
		return
	}
	if currentTitle == title {
		return
	}

	resp, err := p.app.UpdateThreadTitle(ctx, &UpdateThreadTitleRequest{
		ThreadID: run.ThreadID,
		Title:    title,
	})
	if err != nil || resp == nil || !resp.Updated {
		return
	}
	p.emitRunEvent(ctx, run, "context.thread_title_updated", map[string]any{
		"thread_title": title,
	})
}

func (p *RunProcessor) generatedThreadTitle(
	ctx context.Context,
	run *RunSummary,
	userMessage string,
	result *RunExecutionResult,
) string {
	if title := normalizeGeneratedThreadTitle(resultTitle(result)); title != "" {
		return title
	}
	if title := extractQuotedGeneratedThreadTitle(userMessage); title != "" {
		return title
	}
	if p != nil && p.titleGenerator != nil {
		title, err := p.titleGenerator.GenerateTitle(ctx, RunTitleGenerationInput{
			Run:              run,
			UserMessage:      userMessage,
			AssistantMessage: strings.TrimSpace(resultMessage(result)),
		})
		if err == nil {
			if title := normalizeGeneratedThreadTitle(title); title != "" {
				return title
			}
		}
	}
	return fallbackGeneratedThreadTitle(userMessage)
}

func generatedThreadTitle(userMessage, explicitTitle string) string {
	if title := normalizeGeneratedThreadTitle(explicitTitle); title != "" {
		return title
	}
	if title := extractQuotedGeneratedThreadTitle(userMessage); title != "" {
		return title
	}
	return fallbackGeneratedThreadTitle(userMessage)
}

func fallbackGeneratedThreadTitle(userMessage string) string {
	title := stripGeneratedThreadTitlePrefix(userMessage)
	title = firstGeneratedThreadTitleSentence(title)
	title = normalizeGeneratedThreadTitle(title)
	if title == "" {
		return ""
	}

	runes := []rune(title)
	if len(runes) > 50 {
		return string(runes[:50]) + "..."
	}
	return title
}

func extractQuotedGeneratedThreadTitle(text string) string {
	for _, pair := range [][2]string{
		{"《", "》"},
		{"\"", "\""},
		{"'", "'"},
	} {
		start := strings.Index(text, pair[0])
		if start < 0 {
			continue
		}
		remaining := text[start+len(pair[0]):]
		end := strings.Index(remaining, pair[1])
		if end <= 0 {
			continue
		}
		if title := normalizeGeneratedThreadTitle(remaining[:end]); title != "" {
			return title
		}
	}
	return ""
}

func stripGeneratedThreadTitlePrefix(text string) string {
	title := strings.TrimSpace(text)
	for _, prefix := range []string{
		"请帮我生成一份",
		"请帮我制定一份",
		"请帮我创建一个",
		"帮我生成一份",
		"帮我制定一份",
		"帮我创建一个",
		"请生成一份",
		"请制定一份",
		"请创建一个",
		"生成一份",
		"制定一份",
		"创建一个",
		"帮我",
		"请",
	} {
		if strings.HasPrefix(title, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	return title
}

func firstGeneratedThreadTitleSentence(text string) string {
	cut := len(text)
	for _, sep := range []string{"，", "。", "；", "\n", ",", ".", ";", "并", "包含"} {
		if idx := strings.Index(text, sep); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	return text[:cut]
}

func normalizeGeneratedThreadTitle(title string) string {
	return normalizeGeneratedThreadTitleWithLimit(title, defaultRunTitleMaxChars)
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
