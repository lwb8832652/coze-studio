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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGuardrailAuditRetentionWorkerRunOnceDelegates(t *testing.T) {
	cleaner := &recordingGuardrailAuditRetentionCleaner{
		result: GuardrailAuditRetentionReaperResult{
			CutoffCreatedAt: 1000,
			Deleted:         2,
		},
	}
	worker := NewGuardrailAuditRetentionWorker(
		cleaner,
		GuardrailAuditRetentionWorkerOptions{
			Interval: 1500 * time.Millisecond,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Equal(t, cleaner.result, result)
	require.Equal(t, 1, cleaner.calls)
	require.Equal(t, 1500*time.Millisecond, worker.interval)
}

func TestGuardrailAuditRetentionWorkerRunOnceRecordsMetrics(t *testing.T) {
	cleaner := &recordingGuardrailAuditRetentionCleaner{
		result: GuardrailAuditRetentionReaperResult{
			CutoffCreatedAt: 1000,
			Deleted:         2,
		},
	}
	metrics := &recordingGuardrailAuditRetentionMetricsCollector{}
	worker := NewGuardrailAuditRetentionWorker(
		cleaner,
		GuardrailAuditRetentionWorkerOptions{
			MetricsCollector: metrics,
			NowMillis:        newSequenceMillis(2000, 2025),
			RetentionDays:    30,
			BatchSize:        12,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Equal(t, cleaner.result, result)
	require.Len(t, metrics.observations, 1)
	require.Equal(t, GuardrailAuditRetentionMetricsObservation{
		Success:         true,
		CutoffCreatedAt: 1000,
		Deleted:         2,
		RetentionDays:   30,
		BatchSize:       12,
		ElapsedMs:       25,
	}, metrics.observations[0])
}

func TestGuardrailAuditRetentionWorkerRunOnceSanitizesErrors(t *testing.T) {
	cleaner := &recordingGuardrailAuditRetentionCleaner{
		err: errors.New("delete audit row with sk-secret and prompt"),
	}
	worker := NewGuardrailAuditRetentionWorker(
		cleaner,
		GuardrailAuditRetentionWorkerOptions{},
	)

	result := worker.RunOnce(context.Background())

	require.Empty(t, result)
	require.Equal(t, 1, cleaner.calls)
}

func TestGuardrailAuditRetentionWorkerRunOnceRecordsFailureMetrics(t *testing.T) {
	cleaner := &recordingGuardrailAuditRetentionCleaner{
		err: errors.New("delete audit row with sk-secret and s3://raw"),
	}
	metrics := &recordingGuardrailAuditRetentionMetricsCollector{}
	worker := NewGuardrailAuditRetentionWorker(
		cleaner,
		GuardrailAuditRetentionWorkerOptions{
			MetricsCollector: metrics,
			NowMillis:        newSequenceMillis(3000, 3011),
			RetentionDays:    90,
			BatchSize:        1000,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Empty(t, result)
	require.Len(t, metrics.observations, 1)
	got := metrics.observations[0]
	require.False(t, got.Success)
	require.Equal(t, "guardrail_audit_retention_cleanup_failed", got.ErrorCode)
	require.Equal(t, int64(0), got.Deleted)
	require.Equal(t, int64(11), got.ElapsedMs)
	require.Equal(t, 90, got.RetentionDays)
	require.Equal(t, int32(1000), got.BatchSize)
	require.NotContains(t, got.ErrorCode, "sk-secret")
	require.NotContains(t, got.ErrorCode, "s3://")
}

func TestGuardrailAuditRetentionWorkerRunOnceSkipsWhenLegalHoldEnabled(t *testing.T) {
	cleaner := &recordingGuardrailAuditRetentionCleaner{
		result: GuardrailAuditRetentionReaperResult{
			CutoffCreatedAt: 1000,
			Deleted:         99,
		},
	}
	metrics := &recordingGuardrailAuditRetentionMetricsCollector{}
	worker := NewGuardrailAuditRetentionWorker(
		cleaner,
		GuardrailAuditRetentionWorkerOptions{
			LegalHoldEnabled: true,
			MetricsCollector: metrics,
			NowMillis:        newSequenceMillis(4000, 4009),
			RetentionDays:    90,
			BatchSize:        1000,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Empty(t, result)
	require.Equal(t, 0, cleaner.calls)
	require.Len(t, metrics.observations, 1)
	require.Equal(t, GuardrailAuditRetentionMetricsObservation{
		Success:       true,
		Skipped:       true,
		SkipReason:    "legal_hold",
		RetentionDays: 90,
		BatchSize:     1000,
		ElapsedMs:     9,
	}, metrics.observations[0])
}

func TestGuardrailAuditRetentionWorkerFromEnvDisabledByDefault(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		context.Background(),
		&recordingGuardrailAuditRepository{},
	)

	require.Nil(t, worker)
	require.False(t, status.Enabled)
	require.False(t, status.Started)
	require.Empty(t, status.Reason)
}

func TestGuardrailAuditRetentionWorkerFromEnvRequiresRepository(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		context.Background(),
		nil,
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "guardrail audit repository is not configured", status.Reason)
}

func TestGuardrailAuditRetentionWorkerFromEnvRejectsInvalidRetention(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionDaysEnv, "0")

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		context.Background(),
		&recordingGuardrailAuditRepository{},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "guardrail audit retention days is invalid", status.Reason)
}

func TestGuardrailAuditRetentionWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionDaysEnv, "30")
	t.Setenv(agentGuardrailAuditRetentionBatchSizeEnv, "12")
	t.Setenv(agentGuardrailAuditRetentionIntervalMsEnv, "2500")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		ctx,
		&recordingGuardrailAuditRepository{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Enabled)
	require.True(t, status.Started)
	require.Empty(t, status.Reason)
	require.Equal(t, 2500*time.Millisecond, worker.interval)
	reaper, ok := worker.cleaner.(*GuardrailAuditRetentionReaper)
	require.True(t, ok)
	require.Equal(t, 30*24*time.Hour, reaper.retention)
	require.Equal(t, int32(12), reaper.batchSize)
}

func TestGuardrailAuditRetentionMetricsCollectorFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentGuardrailAuditRetentionMetricsLogEnabledEnv, "")

	collector := NewGuardrailAuditRetentionMetricsCollectorFromEnv()

	require.Nil(t, collector)
}

func TestGuardrailAuditRetentionMetricsCollectorFromEnvBuildsLoggingCollector(t *testing.T) {
	t.Setenv(agentGuardrailAuditRetentionMetricsLogEnabledEnv, "true")

	collector := NewGuardrailAuditRetentionMetricsCollectorFromEnv()

	require.NotNil(t, collector)
	_, ok := collector.(*GuardrailAuditRetentionLoggingMetricsCollector)
	require.True(t, ok)
}

func TestGuardrailAuditRetentionWorkerFromEnvBuildsMetricsCollector(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionMetricsLogEnabledEnv, "true")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		ctx,
		&recordingGuardrailAuditRepository{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Started)
	_, ok := worker.metricsCollector.(*GuardrailAuditRetentionLoggingMetricsCollector)
	require.True(t, ok)
	require.Equal(t, defaultGuardrailAuditRetentionDays, worker.retentionDays)
	require.Equal(t, defaultGuardrailAuditRetentionBatchSize, worker.batchSize)
}

func TestGuardrailAuditRetentionWorkerFromEnvBuildsLegalHoldWorker(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionLegalHoldEnabledEnv, "true")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		ctx,
		&recordingGuardrailAuditRepository{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Started)
	require.True(t, worker.legalHoldEnabled)
}

func TestGuardrailAuditRetentionWorkerFromEnvRequireArchiveNeedsStorage(t *testing.T) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionRequireArchiveEnabledEnv, "true")

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		context.Background(),
		&recordingGuardrailAuditRepository{},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "guardrail audit archive storage is not configured", status.Reason)
}

func TestGuardrailAuditRetentionWorkerFromEnvRequireArchiveBuildsCleaner(
	t *testing.T,
) {
	clearGuardrailAuditRetentionWorkerEnv(t)
	t.Setenv(agentGuardrailAuditRetentionWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionRequireArchiveEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionDaysEnv, "30")
	t.Setenv(agentGuardrailAuditRetentionBatchSizeEnv, "12")
	t.Setenv(agentGuardrailAuditArchiveObjectPrefixEnv, "retention/archive")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		ctx,
		&recordingGuardrailAuditRepository{},
		&recordingGuardrailAuditArchiveStorage{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Started)
	cleaner, ok := worker.cleaner.(*GuardrailAuditArchiveBeforeDeleteCleaner)
	require.True(t, ok)
	require.Equal(t, 30*24*time.Hour, cleaner.retention)
	require.Equal(t, int32(12), cleaner.batchSize)
	exporter, ok := cleaner.archiver.(*GuardrailAuditArchiveExporter)
	require.True(t, ok)
	require.Equal(t, int32(12), exporter.batchSize)
	writer, ok := exporter.writer.(*GuardrailAuditObjectStorageArchiveWriter)
	require.True(t, ok)
	require.Equal(t, "retention/archive", writer.prefix)
	reaper, ok := cleaner.deleter.(*GuardrailAuditRetentionReaper)
	require.True(t, ok)
	require.Equal(t, int32(12), reaper.batchSize)
}

type recordingGuardrailAuditRetentionCleaner struct {
	result GuardrailAuditRetentionReaperResult
	err    error
	calls  int
}

func (r *recordingGuardrailAuditRetentionCleaner) CleanupExpiredGuardrailAuditEvents(
	ctx context.Context,
) (GuardrailAuditRetentionReaperResult, error) {
	r.calls++
	if r.err != nil {
		return GuardrailAuditRetentionReaperResult{}, r.err
	}

	return r.result, nil
}

func clearGuardrailAuditRetentionWorkerEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		agentGuardrailAuditRetentionWorkerEnabledEnv,
		agentGuardrailAuditRetentionDaysEnv,
		agentGuardrailAuditRetentionBatchSizeEnv,
		agentGuardrailAuditRetentionIntervalMsEnv,
		agentGuardrailAuditRetentionMetricsLogEnabledEnv,
		agentGuardrailAuditRetentionLegalHoldEnabledEnv,
		agentGuardrailAuditRetentionRequireArchiveEnabledEnv,
		agentGuardrailAuditArchiveObjectPrefixEnv,
	} {
		t.Setenv(key, "")
	}
}

type recordingGuardrailAuditRetentionMetricsCollector struct {
	observations []GuardrailAuditRetentionMetricsObservation
}

func (c *recordingGuardrailAuditRetentionMetricsCollector) RecordGuardrailAuditRetention(
	ctx context.Context,
	observation GuardrailAuditRetentionMetricsObservation,
) {
	c.observations = append(c.observations, observation)
}
