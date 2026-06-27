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
	agentGuardrailAuditRetentionMetricsLogEnabledEnv = "AGENT_GUARDRAIL_AUDIT_RETENTION_METRICS_LOG_ENABLED"
	guardrailAuditRetentionCleanupFailedErrorCode    = "guardrail_audit_retention_cleanup_failed"
	guardrailAuditRetentionLegalHoldSkipReason       = "legal_hold"
)

type GuardrailAuditRetentionMetricsCollector interface {
	RecordGuardrailAuditRetention(
		ctx context.Context,
		observation GuardrailAuditRetentionMetricsObservation,
	)
}

type GuardrailAuditRetentionMetricsObservation struct {
	Success         bool
	ErrorCode       string
	Skipped         bool
	SkipReason      string
	CutoffCreatedAt int64
	Archived        int64
	ArchiveID       string
	Deleted         int64
	RetentionDays   int
	BatchSize       int32
	ElapsedMs       int64
}

type GuardrailAuditRetentionLoggingMetricsCollector struct{}

func NewGuardrailAuditRetentionMetricsCollectorFromEnv() GuardrailAuditRetentionMetricsCollector {
	var collectors []GuardrailAuditRetentionMetricsCollector
	if envkey.GetBoolD(agentGuardrailAuditRetentionMetricsLogEnabledEnv, false) {
		collectors = append(collectors, &GuardrailAuditRetentionLoggingMetricsCollector{})
	}
	if prometheusCollector := NewGuardrailPrometheusMetricsCollectorFromEnv(); prometheusCollector != nil {
		collectors = append(collectors, prometheusCollector)
	}

	return newGuardrailAuditRetentionMetricsCollectorMux(collectors...)
}

func (c *GuardrailAuditRetentionLoggingMetricsCollector) RecordGuardrailAuditRetention(
	ctx context.Context,
	observation GuardrailAuditRetentionMetricsObservation,
) {
	if c == nil {
		return
	}

	logs.CtxInfof(
		ctx,
		"[guardrail-audit-retention-metrics] cleanup success=%t error_code=%s skipped=%t skip_reason=%s cutoff_created_at=%d archived=%d archive_id=%s deleted=%d retention_days=%d batch_size=%d elapsed_ms=%d",
		observation.Success,
		observation.ErrorCode,
		observation.Skipped,
		observation.SkipReason,
		observation.CutoffCreatedAt,
		observation.Archived,
		observation.ArchiveID,
		observation.Deleted,
		observation.RetentionDays,
		observation.BatchSize,
		observation.ElapsedMs,
	)
}
