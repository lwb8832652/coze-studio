/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioWorkdirLeaseReaperWorkerRunOnceDelegates(
	t *testing.T,
) {
	reaper := &recordingADKMCPRuntimeStdioWorkdirLeaseCleaner{
		result: ADKMCPRuntimeStdioWorkdirLeaseReaperResult{
			Listed:   2,
			Cleaned:  1,
			Finished: 1,
			Failed:   1,
		},
	}
	worker := NewADKMCPRuntimeStdioWorkdirLeaseReaperWorker(
		reaper,
		ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerOptions{
			Interval: 1500 * time.Millisecond,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Equal(t, reaper.result, result)
	require.Equal(t, 1, reaper.calls)
	require.Equal(t, 1500*time.Millisecond, worker.interval)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperWorkerRunOnceSanitizesErrors(
	t *testing.T,
) {
	reaper := &recordingADKMCPRuntimeStdioWorkdirLeaseCleaner{
		err: errors.New("remove /mnt/coze/mcp with stdio-secret-token"),
	}
	worker := NewADKMCPRuntimeStdioWorkdirLeaseReaperWorker(
		reaper,
		ADKMCPRuntimeStdioWorkdirLeaseReaperWorkerOptions{},
	)

	result := worker.RunOnce(context.Background())

	require.Empty(t, result)
	require.Equal(t, 1, reaper.calls)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvDisabledByDefault(
	t *testing.T,
) {
	clearADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnv(t)

	worker, status := StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
		context.Background(),
		&recordingMCPRuntimeWorkdirLeaseRepository{},
	)

	require.Nil(t, worker)
	require.False(t, status.Enabled)
	require.False(t, status.Started)
	require.Empty(t, status.Reason)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvRequiresRepository(
	t *testing.T,
) {
	clearADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnv(t)
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperRootEnv, t.TempDir())

	worker, status := StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
		context.Background(),
		nil,
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "mcp stdio workdir lease repository is not configured", status.Reason)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvRequiresAbsoluteRoot(
	t *testing.T,
) {
	clearADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnv(t)
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperRootEnv, "../secret-root")

	worker, status := StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
		context.Background(),
		&recordingMCPRuntimeWorkdirLeaseRepository{},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "mcp stdio workdir reaper root is invalid", status.Reason)
	require.NotContains(t, status.Reason, "../secret-root")
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvBuildsConfiguredWorker(
	t *testing.T,
) {
	clearADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnv(t)
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperRootEnv, t.TempDir())
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperBatchSizeEnv, "7")
	t.Setenv(agentThreadMCPStdioWorkdirLeaseReaperIntervalMsEnv, "2500")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus(
		ctx,
		&recordingMCPRuntimeWorkdirLeaseRepository{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Enabled)
	require.True(t, status.Started)
	require.Empty(t, status.Reason)
	require.Equal(t, 2500*time.Millisecond, worker.interval)
	reaper, ok := worker.reaper.(*ADKMCPRuntimeStdioWorkdirLeaseReaper)
	require.True(t, ok)
	require.Equal(t, int32(7), reaper.batchSize)
}

type recordingADKMCPRuntimeStdioWorkdirLeaseCleaner struct {
	result ADKMCPRuntimeStdioWorkdirLeaseReaperResult
	err    error
	calls  int
}

func (r *recordingADKMCPRuntimeStdioWorkdirLeaseCleaner) CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
	ctx context.Context,
) (ADKMCPRuntimeStdioWorkdirLeaseReaperResult, error) {
	r.calls++
	if r.err != nil {
		return ADKMCPRuntimeStdioWorkdirLeaseReaperResult{}, r.err
	}

	return r.result, nil
}

func clearADKMCPRuntimeStdioWorkdirLeaseReaperWorkerEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		agentThreadMCPStdioWorkdirLeaseReaperEnabledEnv,
		agentThreadMCPStdioWorkdirLeaseReaperRootEnv,
		agentThreadMCPStdioWorkdirLeaseReaperBatchSizeEnv,
		agentThreadMCPStdioWorkdirLeaseReaperIntervalMsEnv,
	} {
		t.Setenv(key, "")
	}
}
