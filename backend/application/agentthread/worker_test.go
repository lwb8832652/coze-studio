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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestRunWorkerRunOnceDelegatesToProcessor(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "ok",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 3,
	})
	worker := NewRunWorker(processor, RunWorkerOptions{Interval: time.Second})

	worker.RunOnce(context.Background())

	require.Equal(t, "worker-a", domainSVC.claimRunsReq.WorkerID)
	require.Equal(t, int32(3), domainSVC.claimRunsReq.Limit)
	require.Equal(t, int64(200), domainSVC.completeRunReq.RunID)
}

func TestRunWorkerFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentThreadWorkerEnabledEnv, "")

	worker := StartRunWorkerFromEnv(context.Background(), &ApplicationService{}, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}))

	require.Nil(t, worker)
}

func TestRunWorkerFromEnvRequiresExecutorWhenEnabled(t *testing.T) {
	t.Setenv(agentThreadWorkerEnabledEnv, "true")

	worker := StartRunWorkerFromEnv(context.Background(), &ApplicationService{}, nil)

	require.Nil(t, worker)
}

func TestRunWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	t.Setenv(agentThreadWorkerEnabledEnv, "true")
	t.Setenv(agentThreadWorkerIDEnv, "worker-env")
	t.Setenv(agentThreadWorkerBatchSizeEnv, "7")
	t.Setenv(agentThreadWorkerIntervalMsEnv, "1500")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartRunWorkerFromEnv(ctx, &ApplicationService{}, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}))

	require.NotNil(t, worker)
	require.Equal(t, 1500*time.Millisecond, worker.interval)
	require.Equal(t, "worker-env", worker.processor.workerID)
	require.Equal(t, int32(7), worker.processor.batchSize)
}

func TestResumeRunWorkerRunOnceDelegatesToProcessor(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{
				ID:       201,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				Command:  `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
				Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
				WorkerID: "resume-worker-a",
			},
		},
		checkpoint: &entity.Checkpoint{
			ID:              503,
			ThreadID:        10,
			RunID:           200,
			CheckpointNS:    "harness.terminal",
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		appended: &entity.Message{
			ID:       301,
			ThreadID: 10,
			RunID:    201,
			Role:     entity.MessageRoleAssistant,
			Content:  "resumed",
		},
		completedRun: &entity.Run{
			ID:       201,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "resume-worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 4,
		Executor: ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "resumed"}, nil
		}),
	})
	worker := NewResumeRunWorker(processor, ResumeRunWorkerOptions{Interval: time.Second})

	result := worker.RunOnce(context.Background())

	require.Equal(t, "resume-worker-a", domainSVC.claimQueuedResumeRunsReq.WorkerID)
	require.Equal(t, int32(4), domainSVC.claimQueuedResumeRunsReq.Limit)
	require.Equal(t, int64(201), domainSVC.completeRunReq.RunID)
	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
}

func TestResumeRunWorkerFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentThreadResumeWorkerEnabledEnv, "")

	worker := StartResumeRunWorkerFromEnv(context.Background(), &ApplicationService{}, ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}))

	require.Nil(t, worker)
}

func TestResumeRunWorkerFromEnvRequiresExecutorWhenEnabled(t *testing.T) {
	t.Setenv(agentThreadResumeWorkerEnabledEnv, "true")

	worker := StartResumeRunWorkerFromEnv(context.Background(), &ApplicationService{}, nil)

	require.Nil(t, worker)
}

func TestResumeRunWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	t.Setenv(agentThreadResumeWorkerEnabledEnv, "true")
	t.Setenv(agentThreadResumeWorkerIDEnv, "resume-worker-env")
	t.Setenv(agentThreadResumeWorkerBatchSizeEnv, "5")
	t.Setenv(agentThreadResumeWorkerIntervalMsEnv, "1750")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartResumeRunWorkerFromEnv(ctx, &ApplicationService{}, ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}))

	require.NotNil(t, worker)
	require.Equal(t, 1750*time.Millisecond, worker.interval)
	require.Equal(t, "resume-worker-env", worker.processor.workerID)
	require.Equal(t, int32(5), worker.processor.batchSize)
}
