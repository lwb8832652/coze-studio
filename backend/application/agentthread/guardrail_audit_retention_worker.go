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
	agentGuardrailAuditRetentionWorkerEnabledEnv         = "AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED"
	agentGuardrailAuditRetentionDaysEnv                  = "AGENT_GUARDRAIL_AUDIT_RETENTION_DAYS"
	agentGuardrailAuditRetentionBatchSizeEnv             = "AGENT_GUARDRAIL_AUDIT_RETENTION_BATCH_SIZE"
	agentGuardrailAuditRetentionIntervalMsEnv            = "AGENT_GUARDRAIL_AUDIT_RETENTION_INTERVAL_MS"
	agentGuardrailAuditRetentionLegalHoldEnabledEnv      = "AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED"
	agentGuardrailAuditRetentionRequireArchiveEnabledEnv = "AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED"
)

const (
	defaultGuardrailAuditRetentionDays     = 90
	defaultGuardrailAuditRetentionInterval = time.Hour
)

type GuardrailAuditRetentionCleaner interface {
	CleanupExpiredGuardrailAuditEvents(
		ctx context.Context,
	) (GuardrailAuditRetentionReaperResult, error)
}

type GuardrailAuditRetentionWorkerOptions struct {
	Interval         time.Duration
	MetricsCollector GuardrailAuditRetentionMetricsCollector
	NowMillis        func() int64
	RetentionDays    int
	BatchSize        int32
	LegalHoldEnabled bool
}

type GuardrailAuditRetentionWorker struct {
	cleaner          GuardrailAuditRetentionCleaner
	metricsCollector GuardrailAuditRetentionMetricsCollector
	interval         time.Duration
	nowMillis        func() int64
	retentionDays    int
	batchSize        int32
	legalHoldEnabled bool
}

type GuardrailAuditRetentionWorkerEnvStatus struct {
	Enabled bool
	Started bool
	Reason  string
}

func NewGuardrailAuditRetentionWorker(
	cleaner GuardrailAuditRetentionCleaner,
	options GuardrailAuditRetentionWorkerOptions,
) *GuardrailAuditRetentionWorker {
	interval := options.Interval
	if interval <= 0 {
		interval = defaultGuardrailAuditRetentionInterval
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &GuardrailAuditRetentionWorker{
		cleaner:          cleaner,
		metricsCollector: options.MetricsCollector,
		interval:         interval,
		nowMillis:        nowMillis,
		retentionDays:    options.RetentionDays,
		batchSize:        options.BatchSize,
		legalHoldEnabled: options.LegalHoldEnabled,
	}
}

func (w *GuardrailAuditRetentionWorker) Start(ctx context.Context) {
	if w == nil || w.cleaner == nil {
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

func (w *GuardrailAuditRetentionWorker) RunOnce(
	ctx context.Context,
) GuardrailAuditRetentionReaperResult {
	if w == nil || w.cleaner == nil {
		return GuardrailAuditRetentionReaperResult{}
	}
	startedAt := w.now()
	if w.legalHoldEnabled {
		w.recordMetrics(ctx, GuardrailAuditRetentionMetricsObservation{
			Success:       true,
			Skipped:       true,
			SkipReason:    guardrailAuditRetentionLegalHoldSkipReason,
			RetentionDays: w.retentionDays,
			BatchSize:     w.batchSize,
			ElapsedMs:     w.elapsedSince(startedAt),
		})

		return GuardrailAuditRetentionReaperResult{}
	}

	result, err := w.cleaner.CleanupExpiredGuardrailAuditEvents(ctx)
	if err != nil {
		logs.CtxErrorf(ctx, "[guardrail-audit-retention] cleanup failed: %v", err)
		w.recordMetrics(ctx, GuardrailAuditRetentionMetricsObservation{
			Success:       false,
			ErrorCode:     guardrailAuditRetentionCleanupFailedErrorCode,
			RetentionDays: w.retentionDays,
			BatchSize:     w.batchSize,
			ElapsedMs:     w.elapsedSince(startedAt),
		})

		return GuardrailAuditRetentionReaperResult{}
	}
	if result.Deleted > 0 {
		logs.CtxInfof(
			ctx,
			"[guardrail-audit-retention] deleted expired audit events, cutoff_created_at=%d deleted=%d",
			result.CutoffCreatedAt,
			result.Deleted,
		)
	}
	w.recordMetrics(ctx, GuardrailAuditRetentionMetricsObservation{
		Success:         true,
		CutoffCreatedAt: result.CutoffCreatedAt,
		Archived:        result.Archived,
		ArchiveID:       result.ArchiveID,
		Deleted:         result.Deleted,
		RetentionDays:   w.retentionDays,
		BatchSize:       w.batchSize,
		ElapsedMs:       w.elapsedSince(startedAt),
	})

	return result
}

func (w *GuardrailAuditRetentionWorker) recordMetrics(
	ctx context.Context,
	observation GuardrailAuditRetentionMetricsObservation,
) {
	if w == nil || w.metricsCollector == nil {
		return
	}
	if observation.ElapsedMs < 0 {
		observation.ElapsedMs = 0
	}
	w.metricsCollector.RecordGuardrailAuditRetention(ctx, observation)
}

func (w *GuardrailAuditRetentionWorker) elapsedSince(startedAt int64) int64 {
	elapsed := w.now() - startedAt
	if elapsed < 0 {
		return 0
	}

	return elapsed
}

func (w *GuardrailAuditRetentionWorker) now() int64 {
	if w == nil || w.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return w.nowMillis()
}

func StartGuardrailAuditRetentionWorkerFromEnv(
	ctx context.Context,
	repository domainrepo.GuardrailAuditRepository,
	objectStorage ...GuardrailAuditArchiveObjectStorage,
) *GuardrailAuditRetentionWorker {
	worker, _ := StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
		ctx,
		repository,
		objectStorage...,
	)

	return worker
}

func StartGuardrailAuditRetentionWorkerFromEnvWithStatus(
	ctx context.Context,
	repository domainrepo.GuardrailAuditRepository,
	objectStorage ...GuardrailAuditArchiveObjectStorage,
) (*GuardrailAuditRetentionWorker, GuardrailAuditRetentionWorkerEnvStatus) {
	status := GuardrailAuditRetentionWorkerEnvStatus{
		Enabled: envkey.GetBoolD(
			agentGuardrailAuditRetentionWorkerEnabledEnv,
			false,
		),
	}
	if !status.Enabled {
		return nil, status
	}
	if repository == nil {
		status.Reason = "guardrail audit repository is not configured"
		logs.CtxWarnf(ctx, "[guardrail-audit-retention] enabled but repository is not configured")

		return nil, status
	}

	retentionDays := envkey.GetIntD(
		agentGuardrailAuditRetentionDaysEnv,
		defaultGuardrailAuditRetentionDays,
	)
	if retentionDays <= 0 {
		status.Reason = "guardrail audit retention days is invalid"
		logs.CtxWarnf(ctx, "[guardrail-audit-retention] enabled but retention days is invalid")

		return nil, status
	}
	batchSize := envkey.GetI32D(
		agentGuardrailAuditRetentionBatchSizeEnv,
		defaultGuardrailAuditRetentionBatchSize,
	)
	requireArchive := envkey.GetBoolD(
		agentGuardrailAuditRetentionRequireArchiveEnabledEnv,
		false,
	)
	var archiveStorage GuardrailAuditArchiveObjectStorage
	if len(objectStorage) > 0 {
		archiveStorage = objectStorage[0]
	}
	if requireArchive && archiveStorage == nil {
		status.Reason = "guardrail audit archive storage is not configured"
		logs.CtxWarnf(ctx, "[guardrail-audit-retention] enabled but archive storage is not configured")

		return nil, status
	}

	reaper := NewGuardrailAuditRetentionReaper(
		GuardrailAuditRetentionReaperOptions{
			Repository: repository,
			Retention:  time.Duration(retentionDays) * 24 * time.Hour,
			BatchSize:  batchSize,
		},
	)
	var cleaner GuardrailAuditRetentionCleaner = reaper
	if requireArchive {
		writer := NewGuardrailAuditObjectStorageArchiveWriter(
			GuardrailAuditObjectStorageArchiveWriterOptions{
				Storage: archiveStorage,
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
		cleaner = NewGuardrailAuditArchiveBeforeDeleteCleaner(
			GuardrailAuditArchiveBeforeDeleteCleanerOptions{
				Archiver:  exporter,
				Deleter:   reaper,
				Retention: time.Duration(retentionDays) * 24 * time.Hour,
				BatchSize: batchSize,
			},
		)
	}
	worker := NewGuardrailAuditRetentionWorker(
		cleaner,
		GuardrailAuditRetentionWorkerOptions{
			Interval: time.Duration(envkey.GetIntD(
				agentGuardrailAuditRetentionIntervalMsEnv,
				int(defaultGuardrailAuditRetentionInterval/time.Millisecond),
			)) * time.Millisecond,
			MetricsCollector: NewGuardrailAuditRetentionMetricsCollectorFromEnv(),
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
