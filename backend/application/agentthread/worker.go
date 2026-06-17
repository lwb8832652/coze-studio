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
)

const (
	defaultRunWorkerInterval       = 2 * time.Second
	defaultResumeRunWorkerInterval = 2 * time.Second
)

type RunWorkerOptions struct {
	Interval time.Duration
}

type RunWorker struct {
	processor *RunProcessor
	interval  time.Duration
}

func NewRunWorker(processor *RunProcessor, opts RunWorkerOptions) *RunWorker {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultRunWorkerInterval
	}

	return &RunWorker{
		processor: processor,
		interval:  interval,
	}
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

func (w *RunWorker) RunOnce(ctx context.Context) {
	if w == nil || w.processor == nil {
		return
	}

	if err := w.processor.ProcessPendingRuns(ctx); err != nil {
		logs.CtxErrorf(ctx, "[agent-run-worker] process pending runs failed, err=%v", err)
	}
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
