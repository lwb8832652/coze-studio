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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestJournalMetricsDeduplicatesVisibleEventsAndUsesOnlyBoundedLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewJournalPrometheusMetricsCollector(registry)
	require.NoError(t, err)
	labels := JournalMetricLabels{
		Version: "1.1", RolloutCohort: "treatment", TaskType: "general",
		ClientVersion: "1.1", Result: "success", ErrorCode: "none",
	}
	observation := JournalEventVisibleMetricObservation{
		Labels: labels, EventID: 501, RunID: 101, TraceID: "trace-1",
		SubmitAt: time.UnixMilli(1_000), VisibleAt: time.UnixMilli(1_125),
	}

	collector.RecordEventVisible(context.Background(), observation)
	collector.RecordEventVisible(context.Background(), observation)
	require.Equal(t, float64(1), testutil.ToFloat64(
		collector.eventVisibleTotal.With(prometheus.Labels(labels.prometheusLabels())),
	))

	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				require.Contains(t, journalPrometheusLabelNames, label.GetName())
				require.NotContains(t, []string{"run_id", "attempt_id", "trace_id", "event_id"}, label.GetName())
			}
		}
	}
}

func TestJournalMetricsCompletenessDenominatorExcludesNonEnrolledAndNonJournalModes(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewJournalPrometheusMetricsCollector(registry)
	require.NoError(t, err)
	base := JournalCompletedTaskMetricObservation{
		Labels: JournalMetricLabels{
			Version: "1.1", RolloutCohort: "treatment", TaskType: "complex",
			ClientVersion: "1.1", ErrorCode: "none",
		},
		Enrolled: true, Mode: DeerFlowModePro, Completed: true,
		FinalSequenceContinuous: true,
		ProjectionState:         domainentity.JournalProjectionStateHealthy,
	}

	collector.RecordCompletedTask(context.Background(), base)
	incomplete := base
	incomplete.FinalSequenceContinuous = false
	collector.RecordCompletedTask(context.Background(), incomplete)
	flash := base
	flash.Mode = DeerFlowModeFlash
	collector.RecordCompletedTask(context.Background(), flash)
	thinking := base
	thinking.Mode = DeerFlowModeThinking
	collector.RecordCompletedTask(context.Background(), thinking)
	unEnrolled := base
	unEnrolled.Enrolled = false
	collector.RecordCompletedTask(context.Background(), unEnrolled)

	completeLabels := base.Labels
	completeLabels.Result = "complete"
	incompleteLabels := base.Labels
	incompleteLabels.Result = "incomplete"
	require.Equal(t, float64(1), testutil.ToFloat64(
		collector.completedTasksTotal.With(prometheus.Labels(completeLabels.prometheusLabels())),
	))
	require.Equal(t, float64(1), testutil.ToFloat64(
		collector.completedTasksTotal.With(prometheus.Labels(incompleteLabels.prometheusLabels())),
	))
}

func TestJournalMetricsRecordsSSESnapshotRecoveryAndRetentionSignals(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewJournalPrometheusMetricsCollector(registry)
	require.NoError(t, err)
	labels := JournalMetricLabels{
		Version: "1.1", RolloutCohort: "treatment", TaskType: "general",
		ClientVersion: "1.1", Result: "success", ErrorCode: "none",
	}

	collector.SetSSEActive(context.Background(), labels, 7)
	collector.RecordSSESignal(context.Background(), JournalSSEMetricObservation{Labels: labels, Signal: "slow_consumer"})
	collector.RecordSSESignal(context.Background(), JournalSSEMetricObservation{Labels: labels, Signal: "backfill"})
	collector.RecordSnapshot(context.Background(), JournalSnapshotMetricObservation{
		Labels: labels, Latency: 25 * time.Millisecond,
	})
	collector.RecordRecovery(context.Background(), JournalRecoveryMetricObservation{Labels: labels})
	collector.SetRetentionBacklog(context.Background(), labels, 11)

	metricNames := map[string]bool{}
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		metricNames[family.GetName()] = true
	}
	for _, name := range []string{
		"coze_journal_sse_active",
		"coze_journal_sse_signals_total",
		"coze_journal_snapshot_requests_total",
		"coze_journal_snapshot_latency_ms",
		"coze_journal_recovery_results_total",
		"coze_journal_retention_backlog",
	} {
		require.True(t, metricNames[name], "metric %s is missing", name)
	}
}

func TestJournalAlertEvaluationUsesFrozenThresholdsAndSampleRules(t *testing.T) {
	require.Empty(t, EvaluateJournalAlerts(JournalSLISample{
		CompletedEnrolledTasks: 99, CompleteJournalTasks: 0,
		SnapshotEligibleRequests: 99, SnapshotSuccessfulRequests: 0,
	}))

	warnings := EvaluateJournalAlerts(JournalSLISample{
		CompletedEnrolledTasks: 100_000, CompleteJournalTasks: 99_993,
		SnapshotEligibleRequests: 100_000, SnapshotSuccessfulRequests: 99_940,
		SSEActive: 71, SSECapacity: 100,
	})
	require.Contains(t, warnings, JournalAlert{Signal: "event_completeness", Severity: JournalAlertSeverityWarning})
	require.Contains(t, warnings, JournalAlert{Signal: "snapshot_success", Severity: JournalAlertSeverityWarning})
	require.Contains(t, warnings, JournalAlert{Signal: "sse_capacity", Severity: JournalAlertSeverityWarning})

	critical := EvaluateJournalAlerts(JournalSLISample{
		CompletedEnrolledTasks: 100_000, CompleteJournalTasks: 99_989,
		SnapshotEligibleRequests: 100_000, SnapshotSuccessfulRequests: 99_899,
		SSEActive: 86, SSECapacity: 100,
	})
	require.Contains(t, critical, JournalAlert{Signal: "event_completeness", Severity: JournalAlertSeverityCritical})
	require.Contains(t, critical, JournalAlert{Signal: "snapshot_success", Severity: JournalAlertSeverityCritical})
	require.Contains(t, critical, JournalAlert{Signal: "sse_capacity", Severity: JournalAlertSeverityCritical})

	security := EvaluateJournalAlerts(JournalSLISample{DuplicateSideEffects: 1})
	require.Equal(t, []JournalAlert{{
		Signal: "duplicate_side_effect", Severity: JournalAlertSeverityP0,
		KillFeature: JournalFeatureRecovery,
	}}, security)
	security = EvaluateJournalAlerts(JournalSLISample{UnauthorizedOrSensitiveLeaks: 1})
	require.Equal(t, []JournalAlert{{
		Signal: "unauthorized_or_sensitive_leak", Severity: JournalAlertSeverityP0,
		KillFeature: JournalFeatureProjection,
	}}, security)
}

func TestJournalSnapshotSLIExcludesNoPermissionFromDenominator(t *testing.T) {
	eligible, successful := JournalSnapshotSLI([]JournalSnapshotSLIResult{
		JournalSnapshotSLISuccess,
		JournalSnapshotSLIFailed,
		JournalSnapshotSLINoPermission,
	})
	require.Equal(t, int64(2), eligible)
	require.Equal(t, int64(1), successful)
}
