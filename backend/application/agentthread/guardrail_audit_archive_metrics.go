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

	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	agentGuardrailAuditArchiveMetricsLogEnabledEnv = "AGENT_GUARDRAIL_AUDIT_ARCHIVE_METRICS_LOG_ENABLED"
	guardrailAuditArchiveFailedErrorCode           = "guardrail_audit_archive_failed"
)

type GuardrailAuditArchiveMetricsCollector interface {
	RecordGuardrailAuditArchive(
		ctx context.Context,
		observation GuardrailAuditArchiveMetricsObservation,
	)
}

type GuardrailAuditArchiveMetricsObservation struct {
	Success         bool
	ErrorCode       string
	Skipped         bool
	SkipReason      string
	CutoffCreatedAt int64
	Archived        int64
	Total           int64
	ArchiveID       string
	RetentionDays   int
	BatchSize       int32
	ElapsedMs       int64
}

type GuardrailAuditArchiveLoggingMetricsCollector struct{}

func NewGuardrailAuditArchiveMetricsCollectorFromEnv() GuardrailAuditArchiveMetricsCollector {
	var collectors []GuardrailAuditArchiveMetricsCollector
	if envkey.GetBoolD(agentGuardrailAuditArchiveMetricsLogEnabledEnv, false) {
		collectors = append(collectors, &GuardrailAuditArchiveLoggingMetricsCollector{})
	}
	if prometheusCollector := NewGuardrailPrometheusMetricsCollectorFromEnv(); prometheusCollector != nil {
		collectors = append(collectors, prometheusCollector)
	}

	return newGuardrailAuditArchiveMetricsCollectorMux(collectors...)
}

func (c *GuardrailAuditArchiveLoggingMetricsCollector) RecordGuardrailAuditArchive(
	ctx context.Context,
	observation GuardrailAuditArchiveMetricsObservation,
) {
	if c == nil {
		return
	}

	logs.CtxInfof(
		ctx,
		"[guardrail-audit-archive-metrics] archive success=%t error_code=%s skipped=%t skip_reason=%s cutoff_created_at=%d archived=%d total=%d archive_id=%s retention_days=%d batch_size=%d elapsed_ms=%d",
		observation.Success,
		observation.ErrorCode,
		observation.Skipped,
		observation.SkipReason,
		observation.CutoffCreatedAt,
		observation.Archived,
		observation.Total,
		observation.ArchiveID,
		observation.RetentionDays,
		observation.BatchSize,
		observation.ElapsedMs,
	)
}
