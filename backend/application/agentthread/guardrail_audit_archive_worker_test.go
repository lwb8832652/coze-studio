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

func TestGuardrailAuditArchiveWorkerRunOnceDelegates(t *testing.T) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        2,
			Total:           5,
			ArchiveID:       "archive_1",
		},
	}
	worker := NewGuardrailAuditArchiveWorker(
		archiver,
		GuardrailAuditArchiveWorkerOptions{
			Interval:      1500 * time.Millisecond,
			RetentionDays: 1,
			NowMillis:     func() int64 { return 86401000 },
		},
	)

	result := worker.RunOnce(context.Background())

	require.Equal(t, archiver.result, result)
	require.Equal(t, 1, archiver.calls)
	require.Equal(t, int64(1000), archiver.cutoffCreatedAt)
	require.Equal(t, 1500*time.Millisecond, worker.interval)
}

func TestGuardrailAuditArchiveWorkerRunOnceRecordsMetrics(t *testing.T) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        2,
			Total:           5,
			ArchiveID:       "archive_1",
		},
	}
	metrics := &recordingGuardrailAuditArchiveMetricsCollector{}
	worker := NewGuardrailAuditArchiveWorker(
		archiver,
		GuardrailAuditArchiveWorkerOptions{
			MetricsCollector: metrics,
			NowMillis:        newSequenceMillis(86401000, 86401025),
			RetentionDays:    1,
			BatchSize:        7,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Equal(t, archiver.result, result)
	require.Len(t, metrics.observations, 1)
	require.Equal(t, GuardrailAuditArchiveMetricsObservation{
		Success:         true,
		CutoffCreatedAt: 1000,
		Archived:        2,
		Total:           5,
		ArchiveID:       "archive_1",
		RetentionDays:   1,
		BatchSize:       7,
		ElapsedMs:       25,
	}, metrics.observations[0])
}

func TestGuardrailAuditArchiveWorkerRunOnceRecordsFailureMetrics(t *testing.T) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		err: errors.New("write s3://bucket/raw with sk-secret prompt"),
	}
	metrics := &recordingGuardrailAuditArchiveMetricsCollector{}
	worker := NewGuardrailAuditArchiveWorker(
		archiver,
		GuardrailAuditArchiveWorkerOptions{
			MetricsCollector: metrics,
			NowMillis:        newSequenceMillis(86401000, 86401011),
			RetentionDays:    1,
			BatchSize:        100,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Empty(t, result)
	require.Equal(t, 1, archiver.calls)
	require.Len(t, metrics.observations, 1)
	got := metrics.observations[0]
	require.False(t, got.Success)
	require.Equal(t, "guardrail_audit_archive_failed", got.ErrorCode)
	require.Equal(t, int64(11), got.ElapsedMs)
	require.Equal(t, 1, got.RetentionDays)
	require.Equal(t, int32(100), got.BatchSize)
	require.NotContains(t, got.ErrorCode, "s3://")
	require.NotContains(t, got.ErrorCode, "sk-secret")
	require.NotContains(t, got.ErrorCode, "prompt")
}

func TestGuardrailAuditArchiveWorkerRunOnceSkipsWhenLegalHoldEnabled(t *testing.T) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        99,
			Total:           99,
			ArchiveID:       "archive_1",
		},
	}
	metrics := &recordingGuardrailAuditArchiveMetricsCollector{}
	worker := NewGuardrailAuditArchiveWorker(
		archiver,
		GuardrailAuditArchiveWorkerOptions{
			LegalHoldEnabled: true,
			MetricsCollector: metrics,
			NowMillis:        newSequenceMillis(86401000, 86401009),
			RetentionDays:    1,
			BatchSize:        100,
		},
	)

	result := worker.RunOnce(context.Background())

	require.Empty(t, result)
	require.Equal(t, 0, archiver.calls)
	require.Len(t, metrics.observations, 1)
	require.Equal(t, GuardrailAuditArchiveMetricsObservation{
		Success:       true,
		Skipped:       true,
		SkipReason:    "legal_hold",
		RetentionDays: 1,
		BatchSize:     100,
		ElapsedMs:     9,
	}, metrics.observations[0])
}

func TestGuardrailAuditArchiveWorkerFromEnvDisabledByDefault(t *testing.T) {
	clearGuardrailAuditArchiveWorkerEnv(t)

	worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		context.Background(),
		&recordingGuardrailAuditRepository{},
		&recordingGuardrailAuditArchiveStorage{},
	)

	require.Nil(t, worker)
	require.False(t, status.Enabled)
	require.False(t, status.Started)
	require.Empty(t, status.Reason)
}

func TestGuardrailAuditArchiveWorkerFromEnvRequiresRepository(t *testing.T) {
	clearGuardrailAuditArchiveWorkerEnv(t)
	t.Setenv(agentGuardrailAuditArchiveWorkerEnabledEnv, "true")

	worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		context.Background(),
		nil,
		&recordingGuardrailAuditArchiveStorage{},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "guardrail audit repository is not configured", status.Reason)
}

func TestGuardrailAuditArchiveWorkerFromEnvRequiresStorage(t *testing.T) {
	clearGuardrailAuditArchiveWorkerEnv(t)
	t.Setenv(agentGuardrailAuditArchiveWorkerEnabledEnv, "true")

	worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		context.Background(),
		&recordingGuardrailAuditRepository{},
		nil,
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "guardrail audit archive storage is not configured", status.Reason)
}

func TestGuardrailAuditArchiveWorkerFromEnvRejectsInvalidRetention(t *testing.T) {
	clearGuardrailAuditArchiveWorkerEnv(t)
	t.Setenv(agentGuardrailAuditArchiveWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditArchiveRetentionDaysEnv, "0")

	worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		context.Background(),
		&recordingGuardrailAuditRepository{},
		&recordingGuardrailAuditArchiveStorage{},
	)

	require.Nil(t, worker)
	require.True(t, status.Enabled)
	require.False(t, status.Started)
	require.Equal(t, "guardrail audit archive retention days is invalid", status.Reason)
}

func TestGuardrailAuditArchiveWorkerFromEnvBuildsConfiguredWorker(t *testing.T) {
	clearGuardrailAuditArchiveWorkerEnv(t)
	t.Setenv(agentGuardrailAuditArchiveWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditArchiveRetentionDaysEnv, "30")
	t.Setenv(agentGuardrailAuditArchiveBatchSizeEnv, "12")
	t.Setenv(agentGuardrailAuditArchiveIntervalMsEnv, "2500")
	t.Setenv(agentGuardrailAuditArchiveObjectPrefixEnv, "custom/archive")
	t.Setenv(agentGuardrailAuditArchiveMetricsLogEnabledEnv, "true")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		ctx,
		&recordingGuardrailAuditRepository{},
		&recordingGuardrailAuditArchiveStorage{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Enabled)
	require.True(t, status.Started)
	require.Empty(t, status.Reason)
	require.Equal(t, 2500*time.Millisecond, worker.interval)
	require.Equal(t, 30, worker.retentionDays)
	require.Equal(t, int32(12), worker.batchSize)
	_, ok := worker.metricsCollector.(*GuardrailAuditArchiveLoggingMetricsCollector)
	require.True(t, ok)
	exporter, ok := worker.archiver.(*GuardrailAuditArchiveExporter)
	require.True(t, ok)
	require.Equal(t, int32(12), exporter.batchSize)
	writer, ok := exporter.writer.(*GuardrailAuditObjectStorageArchiveWriter)
	require.True(t, ok)
	require.Equal(t, "custom/archive", writer.prefix)
}

func TestGuardrailAuditArchiveWorkerFromEnvBuildsLegalHoldWorker(t *testing.T) {
	clearGuardrailAuditArchiveWorkerEnv(t)
	t.Setenv(agentGuardrailAuditArchiveWorkerEnabledEnv, "true")
	t.Setenv(agentGuardrailAuditRetentionLegalHoldEnabledEnv, "true")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		ctx,
		&recordingGuardrailAuditRepository{},
		&recordingGuardrailAuditArchiveStorage{},
	)

	require.NotNil(t, worker)
	require.True(t, status.Started)
	require.True(t, worker.legalHoldEnabled)
}

func TestGuardrailAuditArchiveMetricsCollectorFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentGuardrailAuditArchiveMetricsLogEnabledEnv, "")

	collector := NewGuardrailAuditArchiveMetricsCollectorFromEnv()

	require.Nil(t, collector)
}

func TestGuardrailAuditArchiveMetricsCollectorFromEnvBuildsLoggingCollector(t *testing.T) {
	t.Setenv(agentGuardrailAuditArchiveMetricsLogEnabledEnv, "true")

	collector := NewGuardrailAuditArchiveMetricsCollectorFromEnv()

	require.NotNil(t, collector)
	_, ok := collector.(*GuardrailAuditArchiveLoggingMetricsCollector)
	require.True(t, ok)
}

type recordingGuardrailAuditArchiveRunner struct {
	result          GuardrailAuditArchiveExportResult
	err             error
	cutoffCreatedAt int64
	calls           int
}

func (r *recordingGuardrailAuditArchiveRunner) ArchiveExpiredGuardrailAuditEvents(
	ctx context.Context,
	cutoffCreatedAt int64,
) (GuardrailAuditArchiveExportResult, error) {
	r.calls++
	r.cutoffCreatedAt = cutoffCreatedAt
	if r.err != nil {
		return GuardrailAuditArchiveExportResult{}, r.err
	}

	return r.result, nil
}

type recordingGuardrailAuditArchiveMetricsCollector struct {
	observations []GuardrailAuditArchiveMetricsObservation
}

func (c *recordingGuardrailAuditArchiveMetricsCollector) RecordGuardrailAuditArchive(
	ctx context.Context,
	observation GuardrailAuditArchiveMetricsObservation,
) {
	c.observations = append(c.observations, observation)
}

func clearGuardrailAuditArchiveWorkerEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		agentGuardrailAuditArchiveWorkerEnabledEnv,
		agentGuardrailAuditArchiveRetentionDaysEnv,
		agentGuardrailAuditArchiveBatchSizeEnv,
		agentGuardrailAuditArchiveIntervalMsEnv,
		agentGuardrailAuditArchiveObjectPrefixEnv,
		agentGuardrailAuditArchiveMetricsLogEnabledEnv,
		agentGuardrailAuditRetentionLegalHoldEnabledEnv,
	} {
		t.Setenv(key, "")
	}
}
