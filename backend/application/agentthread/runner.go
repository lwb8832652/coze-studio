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
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const defaultRunProcessorWorkerID = "agent-harness"
const defaultRunProcessorBatchSize int32 = 10
const defaultRunLeaseCleanupTimeout = 5 * time.Second

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
	WorkerID          string
	BatchSize         int32
	EventSink         RunEventSink
	TitleGenerator    RunTitleGenerator
	MetricsCollector  RuntimeMetricsCollector
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	LeaseClock        RunLeaseClock
}

type RunProcessor struct {
	app              *ApplicationService
	executor         RunExecutor
	eventSink        RunEventSink
	titleGenerator   RunTitleGenerator
	metricsCollector RuntimeMetricsCollector
	workerID         string
	batchSize        int32
	leaseConfig      runLeaseHeartbeatConfig
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
		leaseConfig:      normalizeRunLeaseHeartbeatConfig(opts.LeaseTTL, opts.HeartbeatInterval, opts.LeaseClock),
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
		WorkerID:       p.workerID,
		Limit:          p.batchSize,
		Now:            p.leaseConfig.Clock.Now().UnixMilli(),
		LeaseTTLMillis: p.leaseConfig.TTL.Milliseconds(),
	})
	if err != nil {
		return result, err
	}

	result.ClaimedRuns = len(claimed.Runs)
	p.recordRuntimeRunBacklog(ctx)
	var batchErr error
	for _, run := range claimed.Runs {
		p.recordRuntimeRunQueueDelay(ctx, run)
		runCtx, heartbeat := startRunLeaseHeartbeat(ctx, p.app, run, p.leaseConfig)
		outcome, err := p.processRun(runCtx, run, heartbeat)
		if heartbeatErr := heartbeat.Stop(); err == nil && heartbeatErr != nil &&
			!(outcome == runProcessCanceled && errors.Is(heartbeatErr, domainrepo.ErrRunLeaseLost)) {
			outcome = runProcessErrored
			err = heartbeatErr
		}
		heartbeat.Close()
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
			batchErr = errors.Join(batchErr, fmt.Errorf("process run %d: %w", runSummaryID(run), err))
			if releaseErr := releaseUnfinalizedRunLease(ctx, p.app, run, RunStatusPending); releaseErr != nil {
				batchErr = errors.Join(batchErr, fmt.Errorf("release run %d lease: %w", runSummaryID(run), releaseErr))
			}
		}
	}

	return result, batchErr
}

func releaseUnfinalizedRunLease(
	ctx context.Context,
	app *ApplicationService,
	run *RunSummary,
	toStatus RunStatus,
) error {
	if app == nil || run == nil || run.RunID <= 0 {
		return nil
	}
	cleanupCtx := context.Background()
	if ctx != nil {
		cleanupCtx = context.WithoutCancel(ctx)
	}
	cleanupCtx, cancel := context.WithTimeout(cleanupCtx, defaultRunLeaseCleanupTimeout)
	defer cancel()

	_, err := app.ReleaseRunLease(cleanupCtx, &ReleaseRunLeaseRequest{
		RunID:               run.RunID,
		LeaseOwner:          run.LeaseOwner,
		LeaseToken:          run.LeaseToken,
		ExecutionGeneration: run.ExecutionGeneration,
		ToStatus:            toStatus,
	})
	return err
}

func runSummaryID(run *RunSummary) int64 {
	if run == nil {
		return 0
	}
	return run.RunID
}

func (p *RunProcessor) processRun(
	ctx context.Context,
	run *RunSummary,
	heartbeat *runLeaseHeartbeat,
) (runProcessOutcome, error) {
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
				return p.finalizeFailedRun(ctx, run, heartbeat, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)
			}

			return p.finalizeRunExecution(ctx, run, heartbeat, result, err)
		}
		return p.finalizeFailedRun(ctx, run, heartbeat, subagentRetryNotSupportedCode, subagentRetryNotSupportedMessage)
	}

	result, err := p.executor.Execute(ctx, run)

	return p.finalizeRunExecution(ctx, run, heartbeat, result, err)
}

func (p *RunProcessor) finalizeRunExecution(
	ctx context.Context,
	run *RunSummary,
	heartbeat *runLeaseHeartbeat,
	result *RunExecutionResult,
	err error,
) (runProcessOutcome, error) {
	if supersededRun, superseded, lookupErr := durableMultitaskInterruptedRun(ctx, p.app, run, err); lookupErr != nil {
		_ = heartbeat.Stop()
		return runProcessErrored, lookupErr
	} else if superseded {
		_ = heartbeat.Stop()
		terminalRun, rollbackErr := finalizeMultitaskRollback(ctx, p.app, run, supersededRun)
		if rollbackErr != nil {
			return runProcessErrored, rollbackErr
		}
		p.recordRuntimeRunTerminal(ctx, run, terminalRun, runtimeMetricResultInterrupted, runtimeMetricErrorNone)
		return runProcessInterrupted, nil
	}
	if canceledRun, canceled, lookupErr := durableCanceledRunAfterLeaseLoss(ctx, p.app, run); lookupErr != nil {
		_ = heartbeat.Stop()
		return runProcessErrored, lookupErr
	} else if canceled {
		_ = heartbeat.Stop()
		p.recordRuntimeRunTerminal(ctx, run, canceledRun, runtimeMetricResultCanceled, runtimeMetricErrorNone)
		return runProcessCanceled, nil
	}
	if err != nil {
		var canceled *RunCanceledError
		if errors.As(err, &canceled) {
			if heartbeatErr := heartbeat.Stop(); heartbeatErr != nil {
				return runProcessErrored, heartbeatErr
			}
			terminalRun, cancelErr := requestDurableRunCancellation(
				ctx,
				p.app,
				run,
				p.leaseConfig.Clock.Now().UnixMilli(),
			)
			if cancelErr != nil {
				return runProcessErrored, cancelErr
			}
			p.recordRuntimeRunTerminal(ctx, run, terminalRun, runtimeMetricResultCanceled, runtimeMetricErrorNone)
			return runProcessCanceled, nil
		}
		var interrupted *RunInterruptedError
		if errors.As(err, &interrupted) {
			if abortErr := stopRunLeaseHeartbeat(ctx, heartbeat); abortErr != nil {
				return runProcessErrored, abortErr
			}
			interruptPayload := map[string]any{
				"status":    string(RunStatusInterrupted),
				"worker_id": p.workerID,
			}
			if interrupted.CheckpointKey != "" {
				interruptPayload["checkpoint_key"] = interrupted.CheckpointKey
			}
			if len(interrupted.Interrupts) > 0 {
				interruptPayload["interrupt_count"] = len(interrupted.Interrupts)
			}
			transitionResp, transitionErr := p.app.InterruptRun(ctx, &UpdateRunStatusRequest{
				RunID:                 run.RunID,
				From:                  RunStatusRunning,
				WorkerID:              p.workerID,
				LeaseOwner:            run.LeaseOwner,
				LeaseToken:            run.LeaseToken,
				ExecutionGeneration:   run.ExecutionGeneration,
				EventPayload:          encodeRunEventPayload(ctx, interruptPayload),
				EventAlreadyPersisted: interrupted.EventPersisted,
			})
			if transitionErr != nil {
				return runProcessErrored, transitionErr
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
		return p.finalizeFailedRun(ctx, run, heartbeat, "executor_error", err.Error())
	}
	if cause := context.Cause(ctx); cause != nil {
		_ = heartbeat.Stop()
		return runProcessErrored, cause
	}

	message := strings.TrimSpace(resultMessage(result))
	if message == "" {
		return p.finalizeFailedRun(ctx, run, heartbeat, "empty_executor_result", "executor returned empty assistant message")
	}

	expectedTitle, generatedTitle := p.prepareGeneratedThreadTitle(ctx, run, result)
	if abortErr := stopRunLeaseHeartbeat(ctx, heartbeat); abortErr != nil {
		return runProcessErrored, abortErr
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
		TitleEventPayload: encodeRunEventPayload(ctx, map[string]any{
			"thread_title": generatedTitle,
		}),
		CompletionEventPayload: encodeRunEventPayload(ctx, map[string]any{
			"status":    string(RunStatusSucceeded),
			"worker_id": p.workerID,
		}),
		ExpectedThreadTitle: expectedTitle,
		ThreadTitle:         generatedTitle,
	})
	if err != nil {
		if supersededRun, superseded, lookupErr := durableMultitaskInterruptedRun(
			ctx,
			p.app,
			run,
			err,
		); lookupErr != nil {
			return runProcessErrored, lookupErr
		} else if superseded {
			terminalRun, rollbackErr := finalizeMultitaskRollback(ctx, p.app, run, supersededRun)
			if rollbackErr != nil {
				return runProcessErrored, rollbackErr
			}
			p.recordRuntimeRunTerminal(ctx, run, terminalRun, runtimeMetricResultInterrupted, runtimeMetricErrorNone)
			return runProcessInterrupted, nil
		}
		if errors.Is(err, domainrepo.ErrRunCanceled) {
			p.recordRuntimeRunTerminal(ctx, run, nil, runtimeMetricResultCanceled, runtimeMetricErrorNone)
			return runProcessCanceled, nil
		}
		return runProcessErrored, err
	}
	p.recordRuntimeRunTerminal(
		ctx,
		run,
		finalized.Run,
		runtimeMetricResultSuccess,
		runtimeMetricErrorNone,
	)

	return runProcessSucceeded, nil
}

func durableMultitaskInterruptedRun(
	ctx context.Context,
	app *ApplicationService,
	run *RunSummary,
	executionErr error,
) (*RunSummary, bool, error) {
	if app == nil || app.ThreadSVC == nil || run == nil {
		return nil, false, nil
	}
	var canceled *RunCanceledError
	shouldCheck := errors.As(executionErr, &canceled) ||
		errors.Is(executionErr, domainrepo.ErrRunLeaseLost) ||
		errors.Is(context.Cause(ctx), domainrepo.ErrRunLeaseLost)
	if !shouldCheck {
		return nil, false, nil
	}

	lookupCtx := context.Background()
	if ctx != nil {
		lookupCtx = context.WithoutCancel(ctx)
	}
	lookupCtx, cancel := context.WithTimeout(lookupCtx, defaultRunLeaseCleanupTimeout)
	defer cancel()
	current, err := app.ThreadSVC.GetRun(lookupCtx, &domainservice.GetRunRequest{RunID: run.RunID})
	if err != nil {
		return nil, false, fmt.Errorf("load run %d after multitask interruption: %w", run.RunID, err)
	}
	if current == nil || current.Status != entity.RunStatusInterrupted ||
		!strings.HasPrefix(strings.TrimSpace(current.ErrorCode), "multitask_") {
		return nil, false, nil
	}
	return DomainRunToSummary(current), true, nil
}

func finalizeMultitaskRollback(
	ctx context.Context,
	app *ApplicationService,
	run *RunSummary,
	interrupted *RunSummary,
) (*RunSummary, error) {
	if app == nil || run == nil || interrupted == nil ||
		interrupted.ErrorCode != "multitask_rollback" {
		return interrupted, nil
	}
	mode, err := runtimeModeFromRun(run)
	if err != nil {
		return nil, fmt.Errorf("resolve rollback runtime for run %d: %w", run.RunID, err)
	}
	cleanupCtx := context.Background()
	if ctx != nil {
		cleanupCtx = context.WithoutCancel(ctx)
	}
	cleanupCtx, cancel := context.WithTimeout(cleanupCtx, defaultRunLeaseCleanupTimeout)
	defer cancel()
	var checkpointErr error
	if mode == RuntimeModeEinoADK {
		checkpointErr = app.DeleteRuntimeCheckpoint(cleanupCtx, &DeleteRuntimeCheckpointRequest{
			ThreadID:    run.ThreadID,
			RunID:       run.RunID,
			RuntimeType: string(RuntimeModeEinoADK),
			RuntimeKey:  adkCheckpointKeyForRun(run.RunID),
		})
	}
	failed, err := app.FailRun(cleanupCtx, &UpdateRunStatusRequest{
		RunID:        run.RunID,
		From:         RunStatusInterrupted,
		Now:          time.Now().UnixMilli(),
		ErrorCode:    "multitask_rollback",
		ErrorMessage: "run rolled back by a newer thread run",
	})
	if err != nil {
		return nil, errors.Join(checkpointErr, err)
	}
	if failed == nil || failed.Run == nil {
		return nil, errors.Join(
			checkpointErr,
			fmt.Errorf("finalize rollback run %d returned empty run", run.RunID),
		)
	}
	return failed.Run, checkpointErr
}

func (p *RunProcessor) finalizeFailedRun(
	ctx context.Context,
	run *RunSummary,
	heartbeat *runLeaseHeartbeat,
	code string,
	message string,
) (runProcessOutcome, error) {
	if abortErr := stopRunLeaseHeartbeat(ctx, heartbeat); abortErr != nil {
		return runProcessErrored, abortErr
	}
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
		EventPayload: encodeRunEventPayload(ctx, map[string]any{
			"status":     string(RunStatusFailed),
			"worker_id":  p.workerID,
			"error_code": code,
		}),
	})

	if err != nil {
		return nil, err
	}

	return updateRunStatusResponseRun(resp), nil
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

func (p *RunProcessor) prepareGeneratedThreadTitle(
	ctx context.Context,
	run *RunSummary,
	result *RunExecutionResult,
) (string, string) {
	if p == nil || p.app == nil || run == nil {
		return "", ""
	}
	if run.ParentRunID > 0 || run.RunKind == RunKindSubagent {
		return "", ""
	}

	userMessage, ok := runtimeLatestUserInputText(run.Input)
	if !ok {
		return "", ""
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", ""
	}
	thread, err := p.app.ThreadSVC.GetThread(ctx, run.ThreadID)
	if err != nil || thread == nil {
		return "", ""
	}
	currentTitle := strings.TrimSpace(thread.Title)
	initialTitle := taskThreadTitle("", userMessage)
	if currentTitle != "" && currentTitle != initialTitle {
		return "", ""
	}
	title := p.generatedThreadTitle(ctx, run, userMessage, result)
	if title == "" {
		return "", ""
	}
	if currentTitle == title {
		return "", ""
	}

	return currentTitle, title
}

func requestDurableRunCancellation(
	ctx context.Context,
	app *ApplicationService,
	run *RunSummary,
	now int64,
) (*RunSummary, error) {
	if app == nil || run == nil {
		return nil, fmt.Errorf("run cancellation context is required")
	}
	cleanupCtx := context.Background()
	if ctx != nil {
		cleanupCtx = context.WithoutCancel(ctx)
	}
	cleanupCtx, cancel := context.WithTimeout(cleanupCtx, defaultRunLeaseCleanupTimeout)
	defer cancel()

	result, err := app.requestRunCancellation(cleanupCtx, &domainservice.RequestRunCancellationRequest{
		RunID:        run.RunID,
		Now:          now,
		ErrorCode:    "run_canceled",
		ErrorMessage: "run canceled by request",
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Run == nil {
		return nil, fmt.Errorf("agent thread service returned empty canceled run")
	}

	return DomainRunToSummary(result.Run), nil
}

func durableCanceledRunAfterLeaseLoss(
	ctx context.Context,
	app *ApplicationService,
	run *RunSummary,
) (*RunSummary, bool, error) {
	if !errors.Is(context.Cause(ctx), domainrepo.ErrRunLeaseLost) {
		return nil, false, nil
	}
	if app == nil || app.ThreadSVC == nil || run == nil {
		return nil, false, fmt.Errorf("load canceled run after lease loss: run context is required")
	}
	lookupCtx := context.Background()
	if ctx != nil {
		lookupCtx = context.WithoutCancel(ctx)
	}
	lookupCtx, cancel := context.WithTimeout(lookupCtx, defaultRunLeaseCleanupTimeout)
	defer cancel()

	current, err := app.ThreadSVC.GetRun(lookupCtx, &domainservice.GetRunRequest{RunID: run.RunID})
	if err != nil {
		return nil, false, fmt.Errorf("load run %d after lease loss: %w", run.RunID, err)
	}
	if current == nil || current.Status != entity.RunStatusCanceled {
		return nil, false, nil
	}

	return DomainRunToSummary(current), true, nil
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
