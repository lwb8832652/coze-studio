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
	"time"

	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	agentThreadWorkerEnabledEnv    = "AGENT_THREAD_WORKER_ENABLED"
	agentThreadWorkerIDEnv         = "AGENT_THREAD_WORKER_ID"
	agentThreadWorkerBatchSizeEnv  = "AGENT_THREAD_WORKER_BATCH_SIZE"
	agentThreadWorkerIntervalMsEnv = "AGENT_THREAD_WORKER_INTERVAL_MS"

	agentThreadResumeWorkerEnabledEnv    = "AGENT_THREAD_RESUME_WORKER_ENABLED"
	agentThreadResumeWorkerIDEnv         = "AGENT_THREAD_RESUME_WORKER_ID"
	agentThreadResumeWorkerBatchSizeEnv  = "AGENT_THREAD_RESUME_WORKER_BATCH_SIZE"
	agentThreadResumeWorkerIntervalMsEnv = "AGENT_THREAD_RESUME_WORKER_INTERVAL_MS"

	agentMemoryFlushWorkerEnabledEnv        = "AGENT_MEMORY_FLUSH_WORKER_ENABLED"
	agentMemoryFlushWorkerIDEnv             = "AGENT_MEMORY_FLUSH_WORKER_ID"
	agentMemoryFlushWorkerBatchSizeEnv      = "AGENT_MEMORY_FLUSH_WORKER_BATCH_SIZE"
	agentMemoryFlushWorkerIntervalMsEnv     = "AGENT_MEMORY_FLUSH_WORKER_INTERVAL_MS"
	agentMemoryFlushWorkerLeaseTTLMsEnv     = "AGENT_MEMORY_FLUSH_WORKER_LEASE_TTL_MS"
	agentMemoryFlushWorkerMaxAttemptsEnv    = "AGENT_MEMORY_FLUSH_WORKER_MAX_ATTEMPTS"
	agentMemoryFlushWorkerRetryBackoffMsEnv = "AGENT_MEMORY_FLUSH_WORKER_RETRY_BACKOFF_MS"

	agentArtifactScanWorkerEnabledEnv        = "AGENT_ARTIFACT_SCAN_WORKER_ENABLED"
	agentArtifactScanWorkerIDEnv             = "AGENT_ARTIFACT_SCAN_WORKER_ID"
	agentArtifactScanWorkerScannerEnv        = "AGENT_ARTIFACT_SCAN_WORKER_SCANNER"
	agentArtifactScanWorkerBatchSizeEnv      = "AGENT_ARTIFACT_SCAN_WORKER_BATCH_SIZE"
	agentArtifactScanWorkerIntervalMsEnv     = "AGENT_ARTIFACT_SCAN_WORKER_INTERVAL_MS"
	agentArtifactScanWorkerLeaseTTLMsEnv     = "AGENT_ARTIFACT_SCAN_WORKER_LEASE_TTL_MS"
	agentArtifactScanWorkerMaxAttemptsEnv    = "AGENT_ARTIFACT_SCAN_WORKER_MAX_ATTEMPTS"
	agentArtifactScanWorkerRetryBackoffMsEnv = "AGENT_ARTIFACT_SCAN_WORKER_RETRY_BACKOFF_MS"
)

const (
	defaultRunWorkerInterval              = 2 * time.Second
	defaultResumeRunWorkerInterval        = 2 * time.Second
	defaultMemoryFlushWorkerID            = "memory-flush-worker"
	defaultMemoryFlushWorkerBatch         = int32(10)
	defaultMemoryFlushWorkerTTL           = 5 * time.Minute
	defaultMemoryFlushWorkerTick          = 5 * time.Second
	defaultMemoryFlushWorkerMaxAttempts   = int32(3)
	defaultMemoryFlushWorkerRetryBackoff  = time.Minute
	defaultArtifactScanWorkerID           = "artifact-scan-worker"
	defaultArtifactScanWorkerBatch        = int32(10)
	defaultArtifactScanWorkerTTL          = 5 * time.Minute
	defaultArtifactScanWorkerTick         = 5 * time.Second
	defaultArtifactScanWorkerMaxAttempts  = int32(1)
	defaultArtifactScanWorkerRetryBackoff = time.Minute
)

type RunWorkerOptions struct {
	Interval         time.Duration
	TurnLoopRegistry *ADKTurnLoopRegistry
}

type RunWorker struct {
	processor        *RunProcessor
	interval         time.Duration
	turnLoopRegistry *ADKTurnLoopRegistry
}

func NewRunWorker(processor *RunProcessor, opts RunWorkerOptions) *RunWorker {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultRunWorkerInterval
	}
	turnLoopRegistry := opts.TurnLoopRegistry
	if turnLoopRegistry == nil {
		turnLoopRegistry = NewADKTurnLoopRegistry()
	}

	return &RunWorker{
		processor:        processor,
		interval:         interval,
		turnLoopRegistry: turnLoopRegistry,
	}
}

func (w *RunWorker) TurnLoopRegistry() *ADKTurnLoopRegistry {
	if w == nil {
		return nil
	}

	return w.turnLoopRegistry
}

func (w *RunWorker) Start(ctx context.Context) {
	if w == nil || w.processor == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
}

func (w *RunWorker) RunOnce(ctx context.Context) RunProcessResult {
	if w == nil || w.processor == nil {
		return RunProcessResult{}
	}

	result, err := w.processor.ProcessPendingRunsWithResult(ctx)
	if err != nil {
		logs.CtxErrorf(
			ctx,
			"[agent-run-worker] process pending runs failed, claimed=%d processed=%d succeeded=%d failed=%d errored=%d err=%v",
			result.ClaimedRuns,
			result.ProcessedRuns,
			result.SucceededRuns,
			result.FailedRuns,
			result.ErroredRuns,
			err,
		)

		return result
	}
	if result.ClaimedRuns > 0 {
		logs.CtxInfof(
			ctx,
			"[agent-run-worker] processed pending runs, claimed=%d processed=%d succeeded=%d failed=%d",
			result.ClaimedRuns,
			result.ProcessedRuns,
			result.SucceededRuns,
			result.FailedRuns,
		)
	}

	return result
}

func StartRunWorkerFromEnv(ctx context.Context, app *ApplicationService, executor RunExecutor) *RunWorker {
	if !envkey.GetBoolD(agentThreadWorkerEnabledEnv, false) {
		return nil
	}
	if executor == nil {
		logs.CtxWarnf(ctx, "[agent-run-worker] enabled but executor is not configured")

		return nil
	}

	eventSink := NewApplicationRunEventSink(app)
	processor := NewRunProcessor(app, executor, RunProcessorOptions{
		WorkerID:  envkey.GetStringD(agentThreadWorkerIDEnv, defaultRunProcessorWorkerID),
		BatchSize: envkey.GetI32D(agentThreadWorkerBatchSizeEnv, defaultRunProcessorBatchSize),
		EventSink: eventSink,
	})
	worker := NewRunWorker(processor, RunWorkerOptions{
		Interval: time.Duration(envkey.GetIntD(agentThreadWorkerIntervalMsEnv, int(defaultRunWorkerInterval/time.Millisecond))) * time.Millisecond,
	})
	worker.Start(ctx)

	return worker
}

type ResumeRunWorkerOptions struct {
	Interval time.Duration
}

type ResumeRunWorker struct {
	processor *ResumeRunProcessor
	interval  time.Duration
}

func NewResumeRunWorker(processor *ResumeRunProcessor, opts ResumeRunWorkerOptions) *ResumeRunWorker {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultResumeRunWorkerInterval
	}

	return &ResumeRunWorker{
		processor: processor,
		interval:  interval,
	}
}

func (w *ResumeRunWorker) Start(ctx context.Context) {
	if w == nil || w.processor == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
}

func (w *ResumeRunWorker) RunOnce(ctx context.Context) ResumeRunProcessResult {
	if w == nil || w.processor == nil {
		return ResumeRunProcessResult{}
	}

	result, err := w.processor.ProcessQueuedResumeRunsWithResult(ctx)
	if err != nil {
		logs.CtxErrorf(
			ctx,
			"[agent-resume-run-worker] process queued resume runs failed, claimed=%d processed=%d succeeded=%d failed=%d errored=%d err=%v",
			result.ClaimedRuns,
			result.ProcessedRuns,
			result.SucceededRuns,
			result.FailedRuns,
			result.ErroredRuns,
			err,
		)

		return result
	}
	if result.ClaimedRuns > 0 {
		logs.CtxInfof(
			ctx,
			"[agent-resume-run-worker] processed queued resume runs, claimed=%d processed=%d succeeded=%d failed=%d",
			result.ClaimedRuns,
			result.ProcessedRuns,
			result.SucceededRuns,
			result.FailedRuns,
		)
	}

	return result
}

func StartResumeRunWorkerFromEnv(ctx context.Context, app *ApplicationService, executor ResumeRunExecutor) *ResumeRunWorker {
	if !envkey.GetBoolD(agentThreadResumeWorkerEnabledEnv, false) {
		return nil
	}
	if executor == nil {
		logs.CtxWarnf(ctx, "[agent-resume-run-worker] enabled but executor is not configured")

		return nil
	}

	eventSink := NewApplicationRunEventSink(app)
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  envkey.GetStringD(agentThreadResumeWorkerIDEnv, defaultResumeRunProcessorWorkerID),
		BatchSize: envkey.GetI32D(agentThreadResumeWorkerBatchSizeEnv, defaultRunProcessorBatchSize),
		EventSink: eventSink,
		Executor:  executor,
	})
	worker := NewResumeRunWorker(processor, ResumeRunWorkerOptions{
		Interval: time.Duration(envkey.GetIntD(agentThreadResumeWorkerIntervalMsEnv, int(defaultResumeRunWorkerInterval/time.Millisecond))) * time.Millisecond,
	})
	worker.Start(ctx)

	return worker
}

type MemoryFlushWorkerOptions struct {
	WorkerID     string
	BatchSize    int32
	LeaseTTL     time.Duration
	Interval     time.Duration
	MaxAttempts  int32
	RetryBackoff time.Duration
}

type MemoryFlushWorkerResult struct {
	ClaimedJobs   int32
	SucceededJobs int32
	RetriedJobs   int32
	FailedJobs    int32
	SkippedJobs   int32
	Errored       bool
}

type MemoryFlushWorker struct {
	app          *ApplicationService
	workerID     string
	batchSize    int32
	leaseTTL     time.Duration
	interval     time.Duration
	maxAttempts  int32
	retryBackoff time.Duration
}

func NewMemoryFlushWorker(app *ApplicationService, opts MemoryFlushWorkerOptions) *MemoryFlushWorker {
	workerID := opts.WorkerID
	if workerID == "" {
		workerID = defaultMemoryFlushWorkerID
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultMemoryFlushWorkerBatch
	}
	leaseTTL := opts.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultMemoryFlushWorkerTTL
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultMemoryFlushWorkerTick
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMemoryFlushWorkerMaxAttempts
	}
	retryBackoff := opts.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = defaultMemoryFlushWorkerRetryBackoff
	}

	return &MemoryFlushWorker{
		app:          app,
		workerID:     workerID,
		batchSize:    batchSize,
		leaseTTL:     leaseTTL,
		interval:     interval,
		maxAttempts:  maxAttempts,
		retryBackoff: retryBackoff,
	}
}

func (w *MemoryFlushWorker) Start(ctx context.Context) {
	if w == nil || w.app == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
}

func (w *MemoryFlushWorker) RunOnce(ctx context.Context) MemoryFlushWorkerResult {
	if w == nil || w.app == nil {
		return MemoryFlushWorkerResult{}
	}

	resp, err := w.app.ProcessMemoryFlushJobs(ctx, &ProcessMemoryFlushJobsRequest{
		WorkerID:           w.workerID,
		Limit:              w.batchSize,
		LeaseTTLMillis:     w.leaseTTL.Milliseconds(),
		MaxAttempts:        w.maxAttempts,
		RetryBackoffMillis: w.retryBackoff.Milliseconds(),
	})
	if err != nil {
		logs.CtxErrorf(
			ctx,
			"[memory-flush-worker] process memory flush jobs failed, worker=%s err=%v",
			w.workerID,
			err,
		)

		return MemoryFlushWorkerResult{Errored: true}
	}
	if resp == nil {
		return MemoryFlushWorkerResult{}
	}
	result := MemoryFlushWorkerResult{
		ClaimedJobs:   resp.Claimed,
		SucceededJobs: resp.Succeeded,
		RetriedJobs:   resp.Retried,
		FailedJobs:    resp.Failed,
		SkippedJobs:   resp.Skipped,
	}
	if result.ClaimedJobs > 0 {
		logs.CtxInfof(
			ctx,
			"[memory-flush-worker] processed memory flush jobs, claimed=%d succeeded=%d retried=%d failed=%d skipped=%d",
			result.ClaimedJobs,
			result.SucceededJobs,
			result.RetriedJobs,
			result.FailedJobs,
			result.SkippedJobs,
		)
	}

	return result
}

func StartMemoryFlushWorkerFromEnv(ctx context.Context, app *ApplicationService) *MemoryFlushWorker {
	if !envkey.GetBoolD(agentMemoryFlushWorkerEnabledEnv, false) {
		return nil
	}
	if app == nil || app.ThreadSVC == nil {
		logs.CtxWarnf(ctx, "[memory-flush-worker] enabled but thread service is not configured")

		return nil
	}
	if app.MemoryExtractor == nil {
		logs.CtxWarnf(ctx, "[memory-flush-worker] enabled but memory extractor is not configured")

		return nil
	}

	worker := NewMemoryFlushWorker(app, MemoryFlushWorkerOptions{
		WorkerID:     envkey.GetStringD(agentMemoryFlushWorkerIDEnv, defaultMemoryFlushWorkerID),
		BatchSize:    envkey.GetI32D(agentMemoryFlushWorkerBatchSizeEnv, defaultMemoryFlushWorkerBatch),
		LeaseTTL:     time.Duration(envkey.GetIntD(agentMemoryFlushWorkerLeaseTTLMsEnv, int(defaultMemoryFlushWorkerTTL/time.Millisecond))) * time.Millisecond,
		Interval:     time.Duration(envkey.GetIntD(agentMemoryFlushWorkerIntervalMsEnv, int(defaultMemoryFlushWorkerTick/time.Millisecond))) * time.Millisecond,
		MaxAttempts:  envkey.GetI32D(agentMemoryFlushWorkerMaxAttemptsEnv, defaultMemoryFlushWorkerMaxAttempts),
		RetryBackoff: time.Duration(envkey.GetIntD(agentMemoryFlushWorkerRetryBackoffMsEnv, int(defaultMemoryFlushWorkerRetryBackoff/time.Millisecond))) * time.Millisecond,
	})
	worker.Start(ctx)

	return worker
}

type ArtifactScanWorkerOptions struct {
	Scanner      string
	WorkerID     string
	BatchSize    int32
	LeaseTTL     time.Duration
	Interval     time.Duration
	MaxAttempts  int32
	RetryBackoff time.Duration
}

type ArtifactScanWorkerResult struct {
	ClaimedJobs   int32
	SucceededJobs int32
	RetriedJobs   int32
	FailedJobs    int32
	SkippedJobs   int32
	Errored       bool
}

type ArtifactScanWorkerEnvStatus struct {
	Enabled       bool
	Started       bool
	Reason        string
	ScannerStatus ArtifactScannerEnvStatus
}

type ArtifactScanWorker struct {
	app          *ApplicationService
	scanner      string
	workerID     string
	batchSize    int32
	leaseTTL     time.Duration
	interval     time.Duration
	maxAttempts  int32
	retryBackoff time.Duration
}

func NewArtifactScanWorker(app *ApplicationService, opts ArtifactScanWorkerOptions) *ArtifactScanWorker {
	workerID := opts.WorkerID
	if workerID == "" {
		workerID = defaultArtifactScanWorkerID
	}
	scanner := opts.Scanner
	if scanner == "" {
		scanner = defaultApplicationArtifactScanner
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultArtifactScanWorkerBatch
	}
	leaseTTL := opts.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultArtifactScanWorkerTTL
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultArtifactScanWorkerTick
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultArtifactScanWorkerMaxAttempts
	}
	retryBackoff := opts.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = defaultArtifactScanWorkerRetryBackoff
	}

	return &ArtifactScanWorker{
		app:          app,
		scanner:      scanner,
		workerID:     workerID,
		batchSize:    batchSize,
		leaseTTL:     leaseTTL,
		interval:     interval,
		maxAttempts:  maxAttempts,
		retryBackoff: retryBackoff,
	}
}

func (w *ArtifactScanWorker) Start(ctx context.Context) {
	if w == nil || w.app == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
}

func (w *ArtifactScanWorker) RunOnce(ctx context.Context) ArtifactScanWorkerResult {
	if w == nil || w.app == nil {
		return ArtifactScanWorkerResult{}
	}

	resp, err := w.app.ProcessArtifactScanJobs(ctx, &ProcessArtifactScanJobsRequest{
		Scanner:            w.scanner,
		WorkerID:           w.workerID,
		Limit:              w.batchSize,
		LeaseTTLMillis:     w.leaseTTL.Milliseconds(),
		MaxAttempts:        w.maxAttempts,
		RetryBackoffMillis: w.retryBackoff.Milliseconds(),
	})
	if err != nil {
		logs.CtxErrorf(
			ctx,
			"[artifact-scan-worker] process scan jobs failed, scanner=%s worker=%s err=%v",
			w.scanner,
			w.workerID,
			err,
		)

		return ArtifactScanWorkerResult{Errored: true}
	}
	if resp == nil {
		return ArtifactScanWorkerResult{}
	}
	result := ArtifactScanWorkerResult{
		ClaimedJobs:   resp.Claimed,
		SucceededJobs: resp.Succeeded,
		RetriedJobs:   resp.Retried,
		FailedJobs:    resp.Failed,
		SkippedJobs:   resp.Skipped,
	}
	if result.ClaimedJobs > 0 {
		logs.CtxInfof(
			ctx,
			"[artifact-scan-worker] processed scan jobs, claimed=%d succeeded=%d retried=%d failed=%d skipped=%d",
			result.ClaimedJobs,
			result.SucceededJobs,
			result.RetriedJobs,
			result.FailedJobs,
			result.SkippedJobs,
		)
	}

	return result
}

func StartArtifactScanWorkerFromEnv(ctx context.Context, app *ApplicationService) *ArtifactScanWorker {
	worker, _ := StartArtifactScanWorkerFromEnvWithStatus(ctx, app)

	return worker
}

func StartArtifactScanWorkerFromEnvWithStatus(
	ctx context.Context,
	app *ApplicationService,
) (*ArtifactScanWorker, ArtifactScanWorkerEnvStatus) {
	status := ArtifactScanWorkerEnvStatus{
		Enabled: envkey.GetBoolD(agentArtifactScanWorkerEnabledEnv, false),
	}
	if !envkey.GetBoolD(agentArtifactScanWorkerEnabledEnv, false) {
		return nil, status
	}
	if app == nil || app.ArtifactSVC == nil {
		status.Reason = "artifact service is not configured"
		logs.CtxWarnf(ctx, "[artifact-scan-worker] enabled but artifact service is not configured")

		return nil, status
	}
	if app.ArtifactObjectStorage == nil {
		status.Reason = "artifact object storage is not configured"
		logs.CtxWarnf(ctx, "[artifact-scan-worker] enabled but artifact object storage is not configured")

		return nil, status
	}
	status.ScannerStatus = app.ArtifactScannerStatus
	if app.ArtifactScanner == nil {
		status.Reason = "artifact scanner is not configured"
		if status.ScannerStatus.Error != "" {
			logs.CtxWarnf(
				ctx,
				"[artifact-scan-worker] enabled but artifact scanner is not configured: %s",
				status.ScannerStatus.Error,
			)
		} else {
			logs.CtxWarnf(ctx, "[artifact-scan-worker] enabled but artifact scanner is not configured")
		}

		return nil, status
	}

	worker := NewArtifactScanWorker(app, ArtifactScanWorkerOptions{
		Scanner:      envkey.GetStringD(agentArtifactScanWorkerScannerEnv, defaultApplicationArtifactScanner),
		WorkerID:     envkey.GetStringD(agentArtifactScanWorkerIDEnv, defaultArtifactScanWorkerID),
		BatchSize:    envkey.GetI32D(agentArtifactScanWorkerBatchSizeEnv, defaultArtifactScanWorkerBatch),
		LeaseTTL:     time.Duration(envkey.GetIntD(agentArtifactScanWorkerLeaseTTLMsEnv, int(defaultArtifactScanWorkerTTL/time.Millisecond))) * time.Millisecond,
		Interval:     time.Duration(envkey.GetIntD(agentArtifactScanWorkerIntervalMsEnv, int(defaultArtifactScanWorkerTick/time.Millisecond))) * time.Millisecond,
		MaxAttempts:  envkey.GetI32D(agentArtifactScanWorkerMaxAttemptsEnv, defaultArtifactScanWorkerMaxAttempts),
		RetryBackoff: time.Duration(envkey.GetIntD(agentArtifactScanWorkerRetryBackoffMsEnv, int(defaultArtifactScanWorkerRetryBackoff/time.Millisecond))) * time.Millisecond,
	})
	worker.Start(ctx)
	status.Started = true

	return worker, status
}
