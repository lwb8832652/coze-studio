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
	"strings"
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

	result := worker.RunOnce(context.Background())

	require.Equal(t, "worker-a", domainSVC.claimRunsReq.WorkerID)
	require.Equal(t, int32(3), domainSVC.claimRunsReq.Limit)
	require.Equal(t, int64(200), domainSVC.completeRunReq.RunID)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
}

func TestRunWorkerOwnsConfiguredTurnLoopRegistry(t *testing.T) {
	registry := NewADKTurnLoopRegistry()

	worker := NewRunWorker(nil, RunWorkerOptions{
		Interval:         time.Second,
		TurnLoopRegistry: registry,
	})

	require.Same(t, registry, worker.TurnLoopRegistry())
}

func TestRunWorkerCreatesTurnLoopRegistryByDefault(t *testing.T) {
	worker := NewRunWorker(nil, RunWorkerOptions{Interval: time.Second})

	require.NotNil(t, worker.TurnLoopRegistry())
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
	t.Setenv(agentThreadWorkerLeaseTTLMsEnv, "9000")
	t.Setenv(agentThreadWorkerHeartbeatIntervalMsEnv, "3000")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartRunWorkerFromEnv(ctx, &ApplicationService{}, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}))

	require.NotNil(t, worker)
	require.Equal(t, 1500*time.Millisecond, worker.interval)
	require.Equal(t, "worker-env", worker.processor.workerID)
	require.Equal(t, int32(7), worker.processor.batchSize)
	require.Equal(t, 9*time.Second, worker.processor.leaseConfig.TTL)
	require.Equal(t, 3*time.Second, worker.processor.leaseConfig.Interval)
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
	t.Setenv(agentThreadResumeWorkerLeaseTTLMsEnv, "12000")
	t.Setenv(agentThreadResumeWorkerHeartbeatIntervalMsEnv, "4000")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartResumeRunWorkerFromEnv(ctx, &ApplicationService{}, ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}))

	require.NotNil(t, worker)
	require.Equal(t, 1750*time.Millisecond, worker.interval)
	require.Equal(t, "resume-worker-env", worker.processor.workerID)
	require.Equal(t, int32(5), worker.processor.batchSize)
	require.Equal(t, 12*time.Second, worker.processor.leaseConfig.TTL)
	require.Equal(t, 4*time.Second, worker.processor.leaseConfig.Interval)
}

func TestRunLeaseRecoveryWorkerRunOnceDelegatesToProcessor(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(3_000))
	source := expiredRecoveryTestRun(200)
	service := newRunLeaseRecoveryTestService(source)
	processor := NewRunLeaseRecoveryProcessor(
		&ApplicationService{ThreadSVC: service},
		RunLeaseRecoveryProcessorOptions{Limit: 4, Clock: clock},
	)
	worker := NewRunLeaseRecoveryWorker(processor, RunLeaseRecoveryWorkerOptions{Interval: time.Second})

	result := worker.RunOnce(context.Background())

	require.Equal(t, RunLeaseRecoveryResult{ExpiredRuns: 1, AbandonedRuns: 1}, result)
	require.Equal(t, time.Second, worker.interval)
	require.Equal(t, entity.RunStatusFailed, source.Status)
}

func TestRunLeaseRecoveryWorkerFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentThreadLeaseRecoveryWorkerEnabledEnv, "")

	worker := StartRunLeaseRecoveryWorkerFromEnv(context.Background(), &ApplicationService{})

	require.Nil(t, worker)
}

func TestRunLeaseRecoveryWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	t.Setenv(agentThreadLeaseRecoveryWorkerEnabledEnv, "true")
	t.Setenv(agentThreadLeaseRecoveryWorkerBatchSizeEnv, "17")
	t.Setenv(agentThreadLeaseRecoveryWorkerIntervalMsEnv, "4500")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartRunLeaseRecoveryWorkerFromEnv(ctx, &ApplicationService{ThreadSVC: &recordingThreadService{}})

	require.NotNil(t, worker)
	require.Equal(t, 4500*time.Millisecond, worker.interval)
	require.Equal(t, int32(17), worker.processor.limit)
}

func TestArtifactScanWorkerRunOnceDelegatesToApplication(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:         800,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    30,
				UserID:     40,
				ArtifactID: 100,
				FileID:     90,
				Scanner:    "clamav",
				Status:     entity.ArtifactScanJobStatusProcessing,
				WorkerID:   "artifact-scan-worker-a",
			},
		},
		got: &entity.AgentArtifact{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			ArtifactType: "report",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain",
			SizeBytes:    13,
			Metadata:     `{"scan_status":"pending"}`,
		},
		completeScanJob:   &entity.ArtifactScanJob{ID: 800, Status: entity.ArtifactScanJobStatusSucceeded},
		completeScanJobOK: true,
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": []byte("artifact body"),
		},
	}
	scanner := &recordingArtifactContentScanner{
		result: &ArtifactScanResult{ScanStatus: "clean", ScannerVersion: "1.4.0"},
	}
	app := &ApplicationService{
		ThreadSVC:             &recordingThreadService{},
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactScanner:       scanner,
	}
	worker := NewArtifactScanWorker(app, ArtifactScanWorkerOptions{
		Scanner:   "clamav",
		WorkerID:  "artifact-scan-worker-a",
		BatchSize: 4,
		LeaseTTL:  90 * time.Second,
		Interval:  time.Second,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, "clamav", artifactSVC.claimScanJobsReq.Scanner)
	require.Equal(t, "artifact-scan-worker-a", artifactSVC.claimScanJobsReq.WorkerID)
	require.Equal(t, int32(4), artifactSVC.claimScanJobsReq.Limit)
	require.Equal(t, int64((90 * time.Second).Milliseconds()), artifactSVC.claimScanJobsReq.LeaseTTLMillis)
	require.Equal(t, "agent-runtime/30/10/runs/20/outputs/report.txt", storage.key)
	require.Equal(t, "clamav", scanner.req.Scanner)
	require.Equal(t, ArtifactScanWorkerResult{
		ClaimedJobs:   1,
		SucceededJobs: 1,
	}, result)
}

func TestMemoryFlushWorkerRunOnceDelegatesToApplication(t *testing.T) {
	digest := strings.Repeat("c", 64)
	domainSVC := &recordingThreadService{
		claimedMemoryFlushJobs: []*entity.MemoryFlushJob{
			{
				ID:                   810,
				ThreadID:             10,
				RunID:                20,
				SpaceID:              30,
				TranscriptSnapshotID: 510,
				Status:               entity.MemoryFlushJobStatusProcessing,
				WorkerID:             "memory-worker-a",
			},
		},
		gotTranscriptSnapshot: &entity.TranscriptSnapshot{
			ID:             510,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			Kind:           entity.TranscriptKindTerminal,
			Digest:         digest,
			IdempotencyKey: "terminal:" + digest,
			MessageCount:   1,
			Messages:       `[{"role":"user","content":"记住我的区域是 APAC"}]`,
			Metadata:       `{"runtime":"eino_adk"}`,
		},
		rememberedMemories: []*entity.Memory{
			{
				ID:         311,
				ThreadID:   10,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "用户区域是 APAC",
				SourceType: "transcript_summary",
				SourceID:   "snapshot:510:region",
			},
		},
		completedMemoryFlushJob: &entity.MemoryFlushJob{
			ID:     810,
			Status: entity.MemoryFlushJobStatusSucceeded,
		},
		memoryFlushUpdated: true,
	}
	app := &ApplicationService{
		ThreadSVC: domainSVC,
		MemoryExtractor: &recordingMemoryExtractor{
			facts: []MemoryExtractionFact{
				{
					Key:        "region",
					Content:    "用户区域是 APAC",
					Confidence: 0.9,
				},
			},
		},
	}
	worker := NewMemoryFlushWorker(app, MemoryFlushWorkerOptions{
		WorkerID:     "memory-worker-a",
		BatchSize:    4,
		LeaseTTL:     90 * time.Second,
		Interval:     time.Second,
		MaxAttempts:  3,
		RetryBackoff: 2 * time.Minute,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, "memory-worker-a", domainSVC.claimMemoryFlushReq.WorkerID)
	require.Equal(t, int32(4), domainSVC.claimMemoryFlushReq.Limit)
	require.Equal(t, int64((90 * time.Second).Milliseconds()), domainSVC.claimMemoryFlushReq.LeaseTTLMillis)
	require.Equal(t, int64(810), domainSVC.completeMemoryFlushReq.JobID)
	require.Equal(t, MemoryFlushWorkerResult{
		ClaimedJobs:   1,
		SucceededJobs: 1,
	}, result)
}

func TestMemoryFlushWorkerFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentMemoryFlushWorkerEnabledEnv, "")

	worker := StartMemoryFlushWorkerFromEnv(
		context.Background(),
		&ApplicationService{MemoryExtractor: &recordingMemoryExtractor{}},
	)

	require.Nil(t, worker)
}

func TestMemoryFlushWorkerFromEnvRequiresExtractorWhenEnabled(t *testing.T) {
	t.Setenv(agentMemoryFlushWorkerEnabledEnv, "true")

	worker := StartMemoryFlushWorkerFromEnv(
		context.Background(),
		&ApplicationService{ThreadSVC: &recordingThreadService{}},
	)

	require.Nil(t, worker)
}

func TestMemoryFlushWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	t.Setenv(agentMemoryFlushWorkerEnabledEnv, "true")
	t.Setenv(agentMemoryFlushWorkerIDEnv, "memory-worker-env")
	t.Setenv(agentMemoryFlushWorkerBatchSizeEnv, "6")
	t.Setenv(agentMemoryFlushWorkerIntervalMsEnv, "2500")
	t.Setenv(agentMemoryFlushWorkerLeaseTTLMsEnv, "120000")
	t.Setenv(agentMemoryFlushWorkerMaxAttemptsEnv, "4")
	t.Setenv(agentMemoryFlushWorkerRetryBackoffMsEnv, "30000")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartMemoryFlushWorkerFromEnv(
		ctx,
		&ApplicationService{
			ThreadSVC:       &recordingThreadService{},
			MemoryExtractor: &recordingMemoryExtractor{},
		},
	)

	require.NotNil(t, worker)
	require.Equal(t, 2500*time.Millisecond, worker.interval)
	require.Equal(t, "memory-worker-env", worker.workerID)
	require.Equal(t, int32(6), worker.batchSize)
	require.Equal(t, 120*time.Second, worker.leaseTTL)
	require.Equal(t, int32(4), worker.maxAttempts)
	require.Equal(t, 30*time.Second, worker.retryBackoff)
}

func TestArtifactScanWorkerFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentArtifactScanWorkerEnabledEnv, "")

	worker := StartArtifactScanWorkerFromEnv(
		context.Background(),
		&ApplicationService{ArtifactScanner: &recordingArtifactContentScanner{}},
	)

	require.Nil(t, worker)
}

func TestArtifactScanWorkerFromEnvRequiresScannerWhenEnabled(t *testing.T) {
	t.Setenv(agentArtifactScanWorkerEnabledEnv, "true")

	worker := StartArtifactScanWorkerFromEnv(
		context.Background(),
		&ApplicationService{ArtifactObjectStorage: &recordingArtifactObjectReader{}},
	)

	require.Nil(t, worker)
}

func TestArtifactScanWorkerFromEnvReportsScannerConfigError(t *testing.T) {
	t.Setenv(agentArtifactScanWorkerEnabledEnv, "true")

	worker, status := StartArtifactScanWorkerFromEnvWithStatus(
		context.Background(),
		&ApplicationService{
			ArtifactSVC:           &recordingArtifactService{},
			ArtifactObjectStorage: &recordingArtifactObjectReader{},
			ArtifactScannerStatus: ArtifactScannerEnvStatus{
				Enabled: true,
				Type:    "http",
				Error:   "artifact scanner endpoint is required",
			},
		},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "artifact scanner is not configured", status.Reason)
	require.Equal(t, "artifact scanner endpoint is required", status.ScannerStatus.Error)
	require.NotContains(t, status.ScannerStatus.Error, "agent-runtime")
}

func TestArtifactScanWorkerFromEnvRequiresStreamingStorage(t *testing.T) {
	t.Setenv(agentArtifactScanWorkerEnabledEnv, "true")

	worker, status := StartArtifactScanWorkerFromEnvWithStatus(
		context.Background(),
		&ApplicationService{
			ArtifactSVC:           &recordingArtifactService{},
			ArtifactObjectStorage: &nonStreamingArtifactObjectReader{},
			ArtifactScanner:       &recordingArtifactContentScanner{},
		},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "artifact object storage streaming is not configured", status.Reason)
}

func TestArtifactScanWorkerFromEnvRequiresBoundedScanner(t *testing.T) {
	t.Setenv(agentArtifactScanWorkerEnabledEnv, "true")

	worker, status := StartArtifactScanWorkerFromEnvWithStatus(
		context.Background(),
		&ApplicationService{
			ArtifactSVC:           &recordingArtifactService{},
			ArtifactObjectStorage: &recordingArtifactObjectReader{},
			ArtifactScanner:       &unboundedArtifactContentScanner{},
		},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "artifact scanner limits are not configured", status.Reason)
}

func TestArtifactScanWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	t.Setenv(agentArtifactScanWorkerEnabledEnv, "true")
	t.Setenv(agentArtifactScanWorkerIDEnv, "artifact-worker-env")
	t.Setenv(agentArtifactScanWorkerScannerEnv, "clamav")
	t.Setenv(agentArtifactScanWorkerBatchSizeEnv, "6")
	t.Setenv(agentArtifactScanWorkerIntervalMsEnv, "2500")
	t.Setenv(agentArtifactScanWorkerLeaseTTLMsEnv, "120000")
	t.Setenv(agentArtifactScanWorkerMaxAttemptsEnv, "4")
	t.Setenv(agentArtifactScanWorkerRetryBackoffMsEnv, "30000")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := StartArtifactScanWorkerFromEnv(
		ctx,
		&ApplicationService{
			ArtifactSVC:           &recordingArtifactService{},
			ArtifactObjectStorage: &recordingArtifactObjectReader{},
			ArtifactScanner:       &recordingArtifactContentScanner{},
		},
	)

	require.NotNil(t, worker)
	require.Equal(t, 2500*time.Millisecond, worker.interval)
	require.Equal(t, "artifact-worker-env", worker.workerID)
	require.Equal(t, "clamav", worker.scanner)
	require.Equal(t, int32(6), worker.batchSize)
	require.Equal(t, 120000*time.Millisecond, worker.leaseTTL)
	require.Equal(t, int32(4), worker.maxAttempts)
	require.Equal(t, 30000*time.Millisecond, worker.retryBackoff)
}

type nonStreamingArtifactObjectReader struct{}

func (*nonStreamingArtifactObjectReader) GetObject(context.Context, string) ([]byte, error) {
	return nil, nil
}

type unboundedArtifactContentScanner struct{}

func (*unboundedArtifactContentScanner) ScanArtifact(
	context.Context,
	ArtifactScanRequest,
) (*ArtifactScanResult, error) {
	return &ArtifactScanResult{ScanStatus: "clean"}, nil
}
