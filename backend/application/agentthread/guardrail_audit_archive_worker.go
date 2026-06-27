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

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	agentGuardrailAuditArchiveWorkerEnabledEnv = "AGENT_GUARDRAIL_AUDIT_ARCHIVE_WORKER_ENABLED"
	agentGuardrailAuditArchiveRetentionDaysEnv = "AGENT_GUARDRAIL_AUDIT_ARCHIVE_RETENTION_DAYS"
	agentGuardrailAuditArchiveBatchSizeEnv     = "AGENT_GUARDRAIL_AUDIT_ARCHIVE_BATCH_SIZE"
	agentGuardrailAuditArchiveIntervalMsEnv    = "AGENT_GUARDRAIL_AUDIT_ARCHIVE_INTERVAL_MS"
	agentGuardrailAuditArchiveObjectPrefixEnv  = "AGENT_GUARDRAIL_AUDIT_ARCHIVE_OBJECT_PREFIX"
	defaultGuardrailAuditArchiveRetentionDays  = defaultGuardrailAuditRetentionDays
	defaultGuardrailAuditArchiveInterval       = defaultGuardrailAuditRetentionInterval
)

type GuardrailAuditArchiver interface {
	ArchiveExpiredGuardrailAuditEvents(
		ctx context.Context,
		cutoffCreatedAt int64,
	) (GuardrailAuditArchiveExportResult, error)
}

type GuardrailAuditArchiveWorkerOptions struct {
	Interval         time.Duration
	MetricsCollector GuardrailAuditArchiveMetricsCollector
	NowMillis        func() int64
	RetentionDays    int
	BatchSize        int32
	LegalHoldEnabled bool
}

type GuardrailAuditArchiveWorker struct {
	archiver         GuardrailAuditArchiver
	metricsCollector GuardrailAuditArchiveMetricsCollector
	interval         time.Duration
	nowMillis        func() int64
	retentionDays    int
	batchSize        int32
	legalHoldEnabled bool
}

type GuardrailAuditArchiveWorkerEnvStatus struct {
	Enabled bool
	Started bool
	Reason  string
}

func NewGuardrailAuditArchiveWorker(
	archiver GuardrailAuditArchiver,
	options GuardrailAuditArchiveWorkerOptions,
) *GuardrailAuditArchiveWorker {
	interval := options.Interval
	if interval <= 0 {
		interval = defaultGuardrailAuditArchiveInterval
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &GuardrailAuditArchiveWorker{
		archiver:         archiver,
		metricsCollector: options.MetricsCollector,
		interval:         interval,
		nowMillis:        nowMillis,
		retentionDays:    options.RetentionDays,
		batchSize:        options.BatchSize,
		legalHoldEnabled: options.LegalHoldEnabled,
	}
}

func (w *GuardrailAuditArchiveWorker) Start(ctx context.Context) {
	if w == nil || w.archiver == nil {
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

func (w *GuardrailAuditArchiveWorker) RunOnce(
	ctx context.Context,
) GuardrailAuditArchiveExportResult {
	if w == nil || w.archiver == nil || w.retentionDays <= 0 {
		return GuardrailAuditArchiveExportResult{}
	}
	startedAt := w.now()
	if w.legalHoldEnabled {
		w.recordMetrics(ctx, GuardrailAuditArchiveMetricsObservation{
			Success:       true,
			Skipped:       true,
			SkipReason:    guardrailAuditRetentionLegalHoldSkipReason,
			RetentionDays: w.retentionDays,
			BatchSize:     w.batchSize,
			ElapsedMs:     w.elapsedSince(startedAt),
		})

		return GuardrailAuditArchiveExportResult{}
	}

	cutoffCreatedAt := startedAt - int64(w.retentionDays)*int64(24*time.Hour/time.Millisecond)
	result, err := w.archiver.ArchiveExpiredGuardrailAuditEvents(ctx, cutoffCreatedAt)
	if err != nil {
		logs.CtxErrorf(ctx, "[guardrail-audit-archive] archive failed")
		w.recordMetrics(ctx, GuardrailAuditArchiveMetricsObservation{
			Success:       false,
			ErrorCode:     guardrailAuditArchiveFailedErrorCode,
			RetentionDays: w.retentionDays,
			BatchSize:     w.batchSize,
			ElapsedMs:     w.elapsedSince(startedAt),
		})

		return GuardrailAuditArchiveExportResult{}
	}
	if result.Archived > 0 {
		logs.CtxInfof(
			ctx,
			"[guardrail-audit-archive] archived expired audit events, cutoff_created_at=%d archived=%d total=%d archive_id=%s",
			result.CutoffCreatedAt,
			result.Archived,
			result.Total,
			result.ArchiveID,
		)
	}
	w.recordMetrics(ctx, GuardrailAuditArchiveMetricsObservation{
		Success:         true,
		CutoffCreatedAt: result.CutoffCreatedAt,
		Archived:        result.Archived,
		Total:           result.Total,
		ArchiveID:       result.ArchiveID,
		RetentionDays:   w.retentionDays,
		BatchSize:       w.batchSize,
		ElapsedMs:       w.elapsedSince(startedAt),
	})

	return result
}

func (w *GuardrailAuditArchiveWorker) recordMetrics(
	ctx context.Context,
	observation GuardrailAuditArchiveMetricsObservation,
) {
	if w == nil || w.metricsCollector == nil {
		return
	}
	if observation.ElapsedMs < 0 {
		observation.ElapsedMs = 0
	}
	w.metricsCollector.RecordGuardrailAuditArchive(ctx, observation)
}

func (w *GuardrailAuditArchiveWorker) elapsedSince(startedAt int64) int64 {
	elapsed := w.now() - startedAt
	if elapsed < 0 {
		return 0
	}

	return elapsed
}

func (w *GuardrailAuditArchiveWorker) now() int64 {
	if w == nil || w.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return w.nowMillis()
}

func StartGuardrailAuditArchiveWorkerFromEnv(
	ctx context.Context,
	repository domainrepo.GuardrailAuditRepository,
	objectStorage GuardrailAuditArchiveObjectStorage,
) *GuardrailAuditArchiveWorker {
	worker, _ := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
		ctx,
		repository,
		objectStorage,
	)

	return worker
}

func StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
	ctx context.Context,
	repository domainrepo.GuardrailAuditRepository,
	objectStorage GuardrailAuditArchiveObjectStorage,
) (*GuardrailAuditArchiveWorker, GuardrailAuditArchiveWorkerEnvStatus) {
	status := GuardrailAuditArchiveWorkerEnvStatus{
		Enabled: envkey.GetBoolD(
			agentGuardrailAuditArchiveWorkerEnabledEnv,
			false,
		),
	}
	if !status.Enabled {
		return nil, status
	}
	if repository == nil {
		status.Reason = "guardrail audit repository is not configured"
		logs.CtxWarnf(ctx, "[guardrail-audit-archive] enabled but repository is not configured")

		return nil, status
	}
	if objectStorage == nil {
		status.Reason = "guardrail audit archive storage is not configured"
		logs.CtxWarnf(ctx, "[guardrail-audit-archive] enabled but storage is not configured")

		return nil, status
	}

	retentionDays := envkey.GetIntD(
		agentGuardrailAuditArchiveRetentionDaysEnv,
		defaultGuardrailAuditArchiveRetentionDays,
	)
	if retentionDays <= 0 {
		status.Reason = "guardrail audit archive retention days is invalid"
		logs.CtxWarnf(ctx, "[guardrail-audit-archive] enabled but retention days is invalid")

		return nil, status
	}
	batchSize := envkey.GetI32D(
		agentGuardrailAuditArchiveBatchSizeEnv,
		defaultGuardrailAuditRetentionBatchSize,
	)
	writer := NewGuardrailAuditObjectStorageArchiveWriter(
		GuardrailAuditObjectStorageArchiveWriterOptions{
			Storage: objectStorage,
			Prefix:  envkey.GetStringD(agentGuardrailAuditArchiveObjectPrefixEnv, ""),
		},
	)
	exporter := NewGuardrailAuditArchiveExporter(
		GuardrailAuditArchiveExporterOptions{
			Repository: repository,
			Writer:     writer,
			BatchSize:  batchSize,
		},
	)
	worker := NewGuardrailAuditArchiveWorker(
		exporter,
		GuardrailAuditArchiveWorkerOptions{
			Interval: time.Duration(envkey.GetIntD(
				agentGuardrailAuditArchiveIntervalMsEnv,
				int(defaultGuardrailAuditArchiveInterval/time.Millisecond),
			)) * time.Millisecond,
			MetricsCollector: NewGuardrailAuditArchiveMetricsCollectorFromEnv(),
			RetentionDays:    retentionDays,
			BatchSize:        batchSize,
			LegalHoldEnabled: envkey.GetBoolD(
				agentGuardrailAuditRetentionLegalHoldEnabledEnv,
				false,
			),
		},
	)
	worker.Start(ctx)
	status.Started = true

	return worker, status
}
