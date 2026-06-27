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
	"bytes"
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

func TestGuardrailPrometheusMetricsCollectorRecordsDecisionMetrics(
	t *testing.T,
) {
	registry := prometheus.NewRegistry()
	collector, err := NewGuardrailPrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordGuardrailEvaluation(
		context.Background(),
		GuardrailEvaluationMetricsObservation{
			TargetType:           "tool_call",
			Operation:            "invoke",
			Source:               "adk_tool_wrapper",
			FailMode:             "fail_closed",
			Action:               "deny",
			Provider:             "scanner /mnt/raw",
			ErrorCode:            "guardrail denied",
			Allowed:              false,
			Warning:              false,
			RequiresConfirmation: false,
			AuditRecorded:        true,
			ElapsedMs:            17,
		},
	)

	text := gatherGuardrailMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_guardrail_evaluations_total")
	require.Contains(t, text, "coze_guardrail_evaluation_latency_ms")
	require.Contains(t, text, `provider="scanner_mnt_raw"`)
	require.Contains(t, text, `error_code="guardrail_denied"`)
	require.NotContains(t, text, "runtime_tool:search_docs")
	require.NotContains(t, text, "secret prompt")
	require.NotContains(t, text, "sk-secret")
	require.NotContains(t, text, "/mnt/raw")
}

func TestGuardrailPrometheusMetricsCollectorRecordsArchiveAndRetentionMetricsSafely(
	t *testing.T,
) {
	registry := prometheus.NewRegistry()
	collector, err := NewGuardrailPrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordGuardrailAuditArchive(
		context.Background(),
		GuardrailAuditArchiveMetricsObservation{
			Success:         true,
			CutoffCreatedAt: 1000,
			Archived:        2,
			Total:           5,
			ArchiveID:       "archive_secret_s3://bucket/raw",
			RetentionDays:   30,
			BatchSize:       12,
			ElapsedMs:       25,
		},
	)
	collector.RecordGuardrailAuditRetention(
		context.Background(),
		GuardrailAuditRetentionMetricsObservation{
			Success:         true,
			CutoffCreatedAt: 1000,
			Archived:        2,
			ArchiveID:       "archive_secret_s3://bucket/raw",
			Deleted:         2,
			RetentionDays:   30,
			BatchSize:       12,
			ElapsedMs:       30,
		},
	)

	text := gatherGuardrailMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_guardrail_audit_archive_attempts_total")
	require.Contains(t, text, "coze_guardrail_audit_archive_rows_total")
	require.Contains(t, text, "coze_guardrail_audit_archive_latency_ms")
	require.Contains(t, text, "coze_guardrail_audit_retention_attempts_total")
	require.Contains(t, text, "coze_guardrail_audit_retention_rows_total")
	require.Contains(t, text, "coze_guardrail_audit_retention_latency_ms")
	require.Contains(t, text, `row_kind="archived"`)
	require.Contains(t, text, `row_kind="deleted"`)
	require.NotContains(t, text, "archive_secret")
	require.NotContains(t, text, "s3://")
	require.NotContains(t, text, "bucket/raw")
	require.NotContains(t, text, "event_id")
	require.NotContains(t, text, "9101")
	require.NotContains(t, text, "secret prompt")
	require.NotContains(t, text, "sk-secret")
	require.NotContains(t, text, "checkpoint")
	require.NotContains(t, text, "provider_raw")
}

func TestGuardrailMetricsCollectorFromEnvBuildsPrometheusMux(t *testing.T) {
	t.Setenv(agentGuardrailMetricsLogEnabledEnv, "true")
	t.Setenv(agentGuardrailPrometheusMetricsEnabledEnv, "true")

	collector := NewGuardrailMetricsCollectorFromEnv()

	require.NotNil(t, collector)
	_, ok := collector.(*guardrailMetricsCollectorMux)
	require.True(t, ok)
}

func TestGuardrailAuditArchiveMetricsCollectorFromEnvBuildsPrometheusMux(
	t *testing.T,
) {
	t.Setenv(agentGuardrailAuditArchiveMetricsLogEnabledEnv, "true")
	t.Setenv(agentGuardrailPrometheusMetricsEnabledEnv, "true")

	collector := NewGuardrailAuditArchiveMetricsCollectorFromEnv()

	require.NotNil(t, collector)
	_, ok := collector.(*guardrailAuditArchiveMetricsCollectorMux)
	require.True(t, ok)
}

func TestGuardrailAuditRetentionMetricsCollectorFromEnvBuildsPrometheusMux(
	t *testing.T,
) {
	t.Setenv(agentGuardrailAuditRetentionMetricsLogEnabledEnv, "true")
	t.Setenv(agentGuardrailPrometheusMetricsEnabledEnv, "true")

	collector := NewGuardrailAuditRetentionMetricsCollectorFromEnv()

	require.NotNil(t, collector)
	_, ok := collector.(*guardrailAuditRetentionMetricsCollectorMux)
	require.True(t, ok)
}

func gatherGuardrailMetricFamiliesText(
	t *testing.T,
	registry *prometheus.Registry,
) string {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	var buffer bytes.Buffer
	for _, family := range families {
		_, err := expfmt.MetricFamilyToText(&buffer, family)
		require.NoError(t, err)
	}
	return buffer.String()
}

var _ *dto.MetricFamily
