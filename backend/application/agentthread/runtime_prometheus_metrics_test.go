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
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestRuntimePrometheusMetricsCollectorDisabledByDefault(t *testing.T) {
	t.Setenv(agentThreadRuntimePrometheusMetricsEnabledEnv, "")

	collector := NewRuntimePrometheusMetricsCollectorFromEnv()

	require.Nil(t, collector)
}

func TestRuntimePrometheusMetricsCollectorRecordsWorkerTickSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeWorkerTick(
		context.Background(),
		RuntimeWorkerTickMetricsObservation{
			WorkerType: "run /mnt/raw",
			Result:     "success",
			ErrorCode:  "transport failed /mnt/raw sk-secret",
			Claimed:    2,
			ElapsedMs:  -5,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_worker_ticks_total")
	require.Contains(t, text, "coze_agent_thread_worker_claimed_total")
	require.Contains(t, text, "coze_agent_thread_worker_tick_latency_ms")
	require.Contains(t, text, `result="success"`)
	require.Contains(t, text, `worker_type="unknown"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "transport failed")
	require.NotContains(t, text, "sk-secret")
	require.NotContains(t, text, "sk_secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsRunTerminalSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeRunTerminal(
		context.Background(),
		RuntimeRunMetricsObservation{
			Runtime:   "eino_adk /mnt/raw",
			Source:    "task\nraw",
			Result:    "failed",
			ErrorCode: "executor_error sk-secret",
			LatencyMs: -10,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_runs_total")
	require.Contains(t, text, "coze_agent_thread_run_latency_ms")
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `runtime="unknown"`)
	require.Contains(t, text, `source="unknown"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "task\\nraw")
	require.NotContains(t, text, "executor_error sk-secret")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsRunQueueDelaySafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeRunQueueDelay(
		context.Background(),
		RuntimeRunQueueDelayMetricsObservation{
			Runtime: "eino_adk /mnt/raw",
			Source:  "task\nraw",
			DelayMs: -10,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_run_queue_delay_ms")
	require.Contains(t, text, `runtime="unknown"`)
	require.Contains(t, text, `source="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "task\\nraw")
}

func TestRuntimePrometheusMetricsCollectorRecordsRunBacklogSnapshotSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeRunBacklog(
		context.Background(),
		[]RuntimeRunBacklogMetricsObservation{
			{Runtime: "eino_adk", Status: "pending", Count: 3},
			{Runtime: "legacy /mnt/raw", Status: "running sk-secret", Count: -2},
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_runs_backlog")
	require.Contains(t, text, `runtime="eino_adk"`)
	require.Contains(t, text, `status="pending"`)
	require.Contains(t, text, `runtime="unknown"`)
	require.Contains(t, text, `status="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "sk-secret")

	collector.RecordRuntimeRunBacklog(
		context.Background(),
		[]RuntimeRunBacklogMetricsObservation{
			{Runtime: "eino_adk", Status: "running", Count: 1},
		},
	)

	text = gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, `runtime="eino_adk"`)
	require.Contains(t, text, `status="running"`)
	require.NotContains(t, text, `status="pending"`)
}

func TestRuntimePrometheusMetricsCollectorRecordsMemoryFlushJobSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeMemoryFlushJob(
		context.Background(),
		RuntimeMemoryFlushJobMetricsObservation{
			Result:         "success /mnt/raw",
			ErrorCode:      "memory failed sk-secret",
			LatencyMs:      -10,
			ObserveLatency: true,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_memory_flush_jobs_total")
	require.Contains(t, text, "coze_agent_thread_memory_flush_latency_ms")
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsMemoryFlushBacklogSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeMemoryFlushBacklog(
		context.Background(),
		[]RuntimeMemoryFlushBacklogMetricsObservation{
			{Status: "pending\nraw", Count: -10},
			{Status: "processing", Count: 3},
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_memory_flush_backlog")
	require.Contains(t, text, `status="unknown"`)
	require.Contains(t, text, `status="processing"`)
	require.NotContains(t, text, "pending\\nraw")

	collector.RecordRuntimeMemoryFlushBacklog(
		context.Background(),
		[]RuntimeMemoryFlushBacklogMetricsObservation{
			{Status: "pending", Count: 1},
		},
	)

	text = gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, `status="pending"`)
	require.NotContains(t, text, `status="processing"`)
}

func TestRuntimePrometheusMetricsCollectorRecordsMemoryFactsSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeMemoryFacts(
		context.Background(),
		[]RuntimeMemoryFactMetricsObservation{
			{Operation: "upsert /mnt/raw", Result: "success sk-secret", Count: 2},
			{Operation: "remove", Result: "success", Count: 1},
			{Operation: "skip", Result: "skipped", Count: 3},
			{Operation: "upsert", Result: "success", Count: -10},
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_memory_facts_total")
	require.Contains(t, text, `operation="unknown"`)
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `operation="remove"`)
	require.Contains(t, text, `operation="skip"`)
	require.Contains(t, text, `result="skipped"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsWebSearchSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeWebSearch(
		context.Background(),
		RuntimeWebSearchMetricsObservation{
			Provider:     "duckduckgo /mnt/raw",
			Result:       "success sk-secret",
			ErrorCode:    "timeout sk-secret",
			LatencyMs:    -10,
			ResultCount:  -5,
			ObserveCount: true,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_web_search_calls_total")
	require.Contains(t, text, "coze_agent_thread_web_search_latency_ms")
	require.Contains(t, text, "coze_agent_thread_web_search_results")
	require.Contains(t, text, `provider="unknown"`)
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsArtifactScanJobSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeArtifactScanJob(
		context.Background(),
		RuntimeArtifactScanJobMetricsObservation{
			Scanner:           "clamav /mnt/raw",
			ContentFamily:     "text\nplain",
			Result:            "success sk-secret",
			ErrorCode:         "scanner failed sk-secret",
			LatencyMs:         -10,
			QueueDelayMs:      -20,
			ObserveLatency:    true,
			ObserveQueueDelay: true,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_artifact_scan_jobs_total")
	require.Contains(t, text, "coze_agent_thread_artifact_scan_latency_ms")
	require.Contains(t, text, "coze_agent_thread_artifact_scan_queue_delay_ms")
	require.Contains(t, text, `scanner="unknown"`)
	require.Contains(t, text, `content_family="unknown"`)
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsArtifactScanBacklogSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeArtifactScanBacklog(
		context.Background(),
		[]RuntimeArtifactScanBacklogMetricsObservation{
			{
				Scanner: "clamav /mnt/raw",
				Status:  "pending\nraw",
				Count:   -10,
			},
			{
				Scanner: "clamav",
				Status:  "processing",
				Count:   3,
			},
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_artifact_scan_backlog")
	require.Contains(t, text, `scanner="unknown"`)
	require.Contains(t, text, `status="unknown"`)
	require.Contains(t, text, `scanner="clamav"`)
	require.Contains(t, text, `status="processing"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "pending\\nraw")
}

func TestThreadUsageCollectorRecordsRuntimeTokenMetrics(t *testing.T) {
	domainSVC := &recordingThreadService{
		recordedTokenUsage: &entity.TokenUsage{
			ID:           400,
			ThreadID:     10,
			RunID:        20,
			Source:       entity.TokenUsageSourceLeadAgent,
			Provider:     "openai-compatible",
			ModelName:    "gpt-test",
			InputTokens:  12,
			OutputTokens: 8,
			TotalTokens:  20,
		},
	}
	metrics := &recordingRuntimeMetricsCollector{}
	collector := NewThreadUsageCollectorWithOptions(
		&ApplicationService{ThreadSVC: domainSVC},
		ThreadUsageCollectorOptions{MetricsCollector: metrics},
	)

	err := collector.Record(context.Background(), &RunSummary{
		RunID:  20,
		Config: `{"runtime":"eino_adk"}`,
	}, AgentTokenUsage{
		Source:       TokenUsageSourceLeadAgent,
		ModelName:    "gpt-test",
		Provider:     "openai-compatible",
		InputTokens:  12,
		OutputTokens: 8,
		TotalTokens:  20,
		RawUsage:     `{"prompt_tokens":12,"completion_tokens":8,"secret":"sk-secret"}`,
		Metadata:     `{"trace_id":"private-trace"}`,
	})

	require.NoError(t, err)
	require.Equal(t, []RuntimeTokenMetricsObservation{
		{
			Runtime:     "eino_adk",
			ModelFamily: "gpt-test",
			Source:      "lead_agent",
			Direction:   "input",
			Tokens:      12,
		},
		{
			Runtime:     "eino_adk",
			ModelFamily: "gpt-test",
			Source:      "lead_agent",
			Direction:   "output",
			Tokens:      8,
		},
	}, metrics.tokens)
}

func TestRuntimePrometheusMetricsCollectorRecordsTokenUsageSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeTokenUsage(
		context.Background(),
		RuntimeTokenMetricsObservation{
			Runtime:     "eino_adk",
			ModelFamily: "deepseek-v4-pro/private",
			Source:      "lead_agent",
			Direction:   "input",
			Tokens:      42,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_tokens_total")
	require.Contains(t, text, `runtime="eino_adk"`)
	require.Contains(t, text, `model_family="unknown"`)
	require.Contains(t, text, `source="lead_agent"`)
	require.Contains(t, text, `direction="input"`)
	require.NotContains(t, text, "deepseek-v4-pro/private")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimePrometheusMetricsCollectorRecordsModelCallSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeModelCall(
		context.Background(),
		RuntimeModelCallMetricsObservation{
			Runtime:     "eino_adk /mnt/raw",
			ModelFamily: "deepseek-v4-pro/private",
			Result:      "failed sk-secret",
			ErrorCode:   "provider_error sk-secret",
			LatencyMs:   -10,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_model_calls_total")
	require.Contains(t, text, "coze_agent_thread_model_latency_ms")
	require.Contains(t, text, `runtime="unknown"`)
	require.Contains(t, text, `model_family="unknown"`)
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/mnt/raw")
	require.NotContains(t, text, "deepseek-v4-pro/private")
	require.NotContains(t, text, "provider_error sk-secret")
	require.NotContains(t, text, "sk-secret")
}

func TestRuntimeInstrumentedChatModelRecordsGenerateMetrics(t *testing.T) {
	metrics := &recordingRuntimeMetricsCollector{}
	base := &runtimeMetricChatModel{message: schema.AssistantMessage("done", nil)}
	chatModel := newRuntimeInstrumentedChatModel(
		base,
		metrics,
		RuntimeModelCallMetricsConfig{
			Runtime:     "eino_adk",
			ModelFamily: "deepseek-v4-pro",
		},
	)

	message, err := chatModel.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("hello"),
	})

	require.NoError(t, err)
	require.Equal(t, "done", message.Content)
	require.Len(t, metrics.modelCalls, 1)
	require.Equal(t, RuntimeModelCallMetricsObservation{
		Runtime:     "eino_adk",
		ModelFamily: "deepseek-v4-pro",
		Result:      "success",
		ErrorCode:   "none",
	}, metrics.modelCalls[0].withoutLatency())
	require.GreaterOrEqual(t, metrics.modelCalls[0].LatencyMs, int64(0))
}

func TestRuntimeInstrumentedChatModelRecordsGenerateFailureMetrics(t *testing.T) {
	metrics := &recordingRuntimeMetricsCollector{}
	chatModel := newRuntimeInstrumentedChatModel(
		&runtimeMetricChatModel{err: assertAnError("provider sk-secret failed")},
		metrics,
		RuntimeModelCallMetricsConfig{
			Runtime:     "eino_adk",
			ModelFamily: "deepseek-v4-pro",
		},
	)

	_, err := chatModel.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("hello"),
	})

	require.Error(t, err)
	require.Len(t, metrics.modelCalls, 1)
	require.Equal(t, RuntimeModelCallMetricsObservation{
		Runtime:     "eino_adk",
		ModelFamily: "deepseek-v4-pro",
		Result:      "failed",
		ErrorCode:   "model_failed",
	}, metrics.modelCalls[0].withoutLatency())
}

func TestRuntimeInstrumentedChatModelRecordsStreamMetrics(t *testing.T) {
	metrics := &recordingRuntimeMetricsCollector{}
	base := &runtimeMetricChatModel{message: schema.AssistantMessage("done", nil)}
	chatModel := newRuntimeInstrumentedChatModel(
		base,
		metrics,
		RuntimeModelCallMetricsConfig{
			Runtime:     "eino_adk",
			ModelFamily: "deepseek-v4-pro",
		},
	)

	stream, err := chatModel.Stream(context.Background(), []*schema.Message{
		schema.UserMessage("hello"),
	})
	require.NoError(t, err)
	defer stream.Close()

	require.Len(t, metrics.modelCalls, 1)
	require.Equal(t, RuntimeModelCallMetricsObservation{
		Runtime:     "eino_adk",
		ModelFamily: "deepseek-v4-pro",
		Result:      "success",
		ErrorCode:   "none",
	}, metrics.modelCalls[0].withoutLatency())
}

func TestRuntimeInstrumentedChatModelRecordsStreamFailureMetrics(t *testing.T) {
	metrics := &recordingRuntimeMetricsCollector{}
	chatModel := newRuntimeInstrumentedChatModel(
		&runtimeMetricChatModel{err: assertAnError("stream sk-secret failed")},
		metrics,
		RuntimeModelCallMetricsConfig{
			Runtime:     "eino_adk",
			ModelFamily: "deepseek-v4-pro",
		},
	)

	_, err := chatModel.Stream(context.Background(), []*schema.Message{
		schema.UserMessage("hello"),
	})

	require.Error(t, err)
	require.Len(t, metrics.modelCalls, 1)
	require.Equal(t, RuntimeModelCallMetricsObservation{
		Runtime:     "eino_adk",
		ModelFamily: "deepseek-v4-pro",
		Result:      "failed",
		ErrorCode:   "model_failed",
	}, metrics.modelCalls[0].withoutLatency())
}

func TestRuntimePrometheusMetricsCollectorRecordsMCPInvocationSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeMCPInvocation(
		context.Background(),
		RuntimeMCPInvocationMetricsObservation{
			Transport:   "stdio /tmp/raw",
			Result:      "failed",
			ErrorCode:   "transport_failed secret-token",
			LatencyMs:   30,
			OutputBytes: 256,
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_mcp_invocations_total")
	require.Contains(t, text, "coze_agent_thread_mcp_latency_ms")
	require.Contains(t, text, "coze_agent_thread_mcp_output_bytes")
	require.Contains(t, text, `transport="unknown"`)
	require.Contains(t, text, `result="failed"`)
	require.Contains(t, text, `error_code="unknown"`)
	require.NotContains(t, text, "/tmp/raw")
	require.NotContains(t, text, "secret-token")
}

func TestRuntimePrometheusMetricsCollectorRecordsMCPHealthSafely(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeMCPHealth(
		context.Background(),
		RuntimeMCPHealthMetricsObservation{
			ServerID:  100,
			Transport: "stdio /tmp/raw",
			Status:    "healthy sk-secret",
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, "coze_agent_thread_mcp_health")
	require.Contains(t, text, `transport="unknown"`)
	require.Contains(t, text, `status="unhealthy"`)
	require.NotContains(t, text, "/tmp/raw")
	require.NotContains(t, text, "sk-secret")
	require.NotContains(t, text, "100")
}

func TestRuntimePrometheusMetricsCollectorSnapshotsMCPHealthByCurrentState(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewRuntimePrometheusMetricsCollector(registry)
	require.NoError(t, err)

	collector.RecordRuntimeMCPHealth(
		context.Background(),
		RuntimeMCPHealthMetricsObservation{
			ServerID:  100,
			Transport: adkMCPRuntimeTransportStdio,
			Status:    "healthy",
		},
	)
	collector.RecordRuntimeMCPHealth(
		context.Background(),
		RuntimeMCPHealthMetricsObservation{
			ServerID:  101,
			Transport: adkMCPRuntimeTransportSSE,
			Status:    "unhealthy",
		},
	)
	collector.RecordRuntimeMCPHealth(
		context.Background(),
		RuntimeMCPHealthMetricsObservation{
			ServerID:  100,
			Transport: adkMCPRuntimeTransportStreamableHTTP,
			Status:    "unhealthy",
		},
	)

	text := gatherRuntimeMetricFamiliesText(t, registry)
	require.Contains(t, text, `transport="sse"`)
	require.Contains(t, text, `transport="streamable_http"`)
	require.Contains(t, text, `status="unhealthy"`)
	require.NotContains(t, text, `transport="stdio"`)
}

func TestADKMCPRuntimeHealthReporterWithMetricsFansOutSafely(t *testing.T) {
	existing := &recordingADKMCPRuntimeHealthReporter{}
	metrics := &recordingRuntimeMetricsCollector{}
	reporter := newADKMCPRuntimeHealthReporterWithMetrics(existing, metrics)

	err := reporter.ReportADKMCPRuntimeHealth(
		context.Background(),
		ADKMCPRuntimeHealthReport{
			ServerID:  100,
			Transport: adkMCPRuntimeTransportSSE,
			Success:   false,
			ErrorCode: "transport_failed secret-token",
			LatencyMs: 30,
		},
	)

	require.NoError(t, err)
	require.Len(t, existing.reports, 1)
	require.Len(t, metrics.mcpHealth, 1)
	require.Equal(t, RuntimeMCPHealthMetricsObservation{
		ServerID:  100,
		Transport: adkMCPRuntimeTransportSSE,
		Status:    "unhealthy",
	}, metrics.mcpHealth[0])
}

func TestRunWorkerRecordsRuntimeMetrics(t *testing.T) {
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
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewRunWorker(processor, RunWorkerOptions{
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
	require.Len(t, metrics.workerTicks, 1)
	require.Equal(t, RuntimeWorkerTickMetricsObservation{
		WorkerType: "run",
		Result:     "success",
		ErrorCode:  "none",
		Claimed:    1,
	}, metrics.workerTicks[0].withoutElapsed())
	require.GreaterOrEqual(t, metrics.workerTicks[0].ElapsedMs, int64(0))
}

func TestRunProcessorRecordsRuntimeRunTerminalMetrics(t *testing.T) {
	tests := []struct {
		name         string
		execute      RunExecutorFunc
		run          *entity.Run
		completedRun *entity.Run
		interrupted  *entity.Run
		failedRun    *entity.Run
		wantResult   string
		wantError    string
		wantOutcome  RunProcessResult
	}{
		{
			name: "succeeded",
			execute: func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
				return &RunExecutionResult{Message: "ok"}, nil
			},
			run: &entity.Run{
				ID:        200,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Config:    `{"runtime":"eino_adk"}`,
				Input:     `{"messages":[]}`,
				WorkerID:  "worker-a",
				StartedAt: 1000,
				CreatedAt: 500,
			},
			completedRun: &entity.Run{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusSucceeded,
				EndedAt:  4000,
			},
			wantResult: "success",
			wantError:  "none",
			wantOutcome: RunProcessResult{
				ClaimedRuns:   1,
				ProcessedRuns: 1,
				SucceededRuns: 1,
			},
		},
		{
			name: "failed",
			execute: func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
				return nil, assertAnError("model failed")
			},
			run: &entity.Run{
				ID:        201,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Config:    `{"runtime":"eino_adk"}`,
				Input:     `{"messages":[]}`,
				WorkerID:  "worker-a",
				StartedAt: 1000,
				CreatedAt: 500,
			},
			failedRun: &entity.Run{
				ID:        201,
				ThreadID:  10,
				Status:    entity.RunStatusFailed,
				ErrorCode: "executor_error",
				EndedAt:   5000,
			},
			wantResult: "failed",
			wantError:  "executor_error",
			wantOutcome: RunProcessResult{
				ClaimedRuns:   1,
				ProcessedRuns: 1,
				FailedRuns:    1,
			},
		},
		{
			name: "interrupted",
			execute: func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
				return nil, &RunInterruptedError{CheckpointKey: "coze-run-202"}
			},
			run: &entity.Run{
				ID:        202,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Config:    `{"runtime":"eino_adk"}`,
				Input:     `{"messages":[]}`,
				WorkerID:  "worker-a",
				StartedAt: 1000,
				CreatedAt: 500,
			},
			interrupted: &entity.Run{
				ID:       202,
				ThreadID: 10,
				Status:   entity.RunStatusInterrupted,
				EndedAt:  6000,
			},
			wantResult: "interrupted",
			wantError:  "none",
			wantOutcome: RunProcessResult{
				ClaimedRuns:     1,
				ProcessedRuns:   1,
				InterruptedRuns: 1,
			},
		},
		{
			name: "canceled",
			execute: func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
				return nil, &RunCanceledError{EventPersisted: true}
			},
			run: &entity.Run{
				ID:        203,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Config:    `{"runtime":"eino_adk"}`,
				Input:     `{"messages":[]}`,
				WorkerID:  "worker-a",
				StartedAt: 1000,
				CreatedAt: 500,
			},
			wantResult: "canceled",
			wantError:  "none",
			wantOutcome: RunProcessResult{
				ClaimedRuns:   1,
				ProcessedRuns: 1,
				CanceledRuns:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainSVC := &recordingThreadService{
				claimedRuns: []*entity.Run{tt.run},
				appended: &entity.Message{
					ID:       300,
					ThreadID: tt.run.ThreadID,
					RunID:    tt.run.ID,
					Role:     entity.MessageRoleAssistant,
					Content:  "ok",
				},
				completedRun:   tt.completedRun,
				interruptedRun: tt.interrupted,
				failedRun:      tt.failedRun,
			}
			app := &ApplicationService{ThreadSVC: domainSVC}
			metrics := &recordingRuntimeMetricsCollector{}
			processor := NewRunProcessor(app, tt.execute, RunProcessorOptions{
				WorkerID:         "worker-a",
				BatchSize:        1,
				MetricsCollector: metrics,
			})

			result, err := processor.ProcessPendingRunsWithResult(context.Background())

			require.NoError(t, err)
			require.Equal(t, tt.wantOutcome, result)
			require.Equal(t, []RuntimeRunQueueDelayMetricsObservation{
				{
					Runtime: "eino_adk",
					Source:  "task",
					DelayMs: 500,
				},
			}, metrics.runQueueDelays)
			require.Len(t, metrics.runTerminals, 1)
			require.Equal(t, RuntimeRunMetricsObservation{
				Runtime:   "eino_adk",
				Source:    "task",
				Result:    tt.wantResult,
				ErrorCode: tt.wantError,
			}, metrics.runTerminals[0].withoutLatency())
			require.GreaterOrEqual(t, metrics.runTerminals[0].LatencyMs, int64(0))
		})
	}
}

func TestRunProcessorRecordsRuntimeRunBacklogMetrics(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:        210,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Config:    `{"runtime":"eino_adk"}`,
				Input:     `{"messages":[]}`,
				WorkerID:  "worker-a",
				StartedAt: 1000,
				CreatedAt: 500,
			},
		},
		runBacklogAggregates: []*entity.RunBacklogAggregate{
			{Status: entity.RunStatusRunning, Config: `{"runtime":"eino_adk"}`, Count: 2},
			{Status: entity.RunStatusQueued, Config: `{"runtime":"legacy"}`, Count: 1},
		},
		appended: &entity.Message{
			ID:       310,
			ThreadID: 10,
			RunID:    210,
			Role:     entity.MessageRoleAssistant,
			Content:  "ok",
		},
		completedRun: &entity.Run{
			ID:       210,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			EndedAt:  2000,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	metrics := &recordingRuntimeMetricsCollector{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{
		WorkerID:         "worker-a",
		BatchSize:        1,
		MetricsCollector: metrics,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
	require.Equal(t, &domainservice.AggregateRunBacklogRequest{
		Statuses: []entity.RunStatus{
			entity.RunStatusPending,
			entity.RunStatusQueued,
			entity.RunStatusRunning,
			entity.RunStatusInterrupted,
		},
	}, domainSVC.aggregateRunBacklogReq)
	require.Equal(t, []RuntimeRunBacklogMetricsObservation{
		{Runtime: "eino_adk", Status: "running", Count: 2},
		{Runtime: "legacy", Status: "queued", Count: 1},
	}, metrics.runBacklogs)
}

func TestResumeRunWorkerRecordsRuntimeMetrics(t *testing.T) {
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
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewResumeRunWorker(processor, ResumeRunWorkerOptions{
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
	require.Len(t, metrics.workerTicks, 1)
	require.Equal(t, RuntimeWorkerTickMetricsObservation{
		WorkerType: "resume",
		Result:     "success",
		ErrorCode:  "none",
		Claimed:    1,
	}, metrics.workerTicks[0].withoutElapsed())
}

func TestResumeRunProcessorRecordsRuntimeRunTerminalMetrics(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{
				ID:        205,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Config:    `{"runtime":"eino_adk"}`,
				WorkerID:  "resume-worker-a",
				Command:   `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
				Metadata:  `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
				StartedAt: 1000,
			},
		},
		checkpoint: &entity.Checkpoint{
			ID:              503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "harness.terminal",
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		appended: &entity.Message{
			ID:       301,
			ThreadID: 10,
			RunID:    205,
			Role:     entity.MessageRoleAssistant,
			Content:  "resumed",
		},
		completedRun: &entity.Run{
			ID:       205,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			EndedAt:  4000,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	metrics := &recordingRuntimeMetricsCollector{}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:         "resume-worker-a",
		BatchSize:        1,
		MetricsCollector: metrics,
		Executor: ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "resumed"}, nil
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
	require.Len(t, metrics.runTerminals, 1)
	require.Equal(t, RuntimeRunMetricsObservation{
		Runtime:   "eino_adk",
		Source:    "resume",
		Result:    "success",
		ErrorCode: "none",
		LatencyMs: 3000,
	}, metrics.runTerminals[0])
}

func TestMemoryFlushWorkerRecordsRuntimeMetrics(t *testing.T) {
	domainSVC := &recordingThreadService{}
	app := &ApplicationService{
		ThreadSVC: domainSVC,
	}
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewMemoryFlushWorker(app, MemoryFlushWorkerOptions{
		WorkerID:         "memory-worker-a",
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, MemoryFlushWorkerResult{Errored: true}, result)
	require.Len(t, metrics.workerTicks, 1)
	require.Equal(t, RuntimeWorkerTickMetricsObservation{
		WorkerType: "memory_flush",
		Result:     "failed",
		ErrorCode:  "process_failed",
	}, metrics.workerTicks[0].withoutElapsed())
}

func TestMemoryFlushWorkerRecordsMemoryFlushJobMetrics(t *testing.T) {
	digest := strings.Repeat("d", 64)
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
				StartedAt:            1000,
				CreatedAt:            500,
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
			ID:        810,
			Status:    entity.MemoryFlushJobStatusSucceeded,
			StartedAt: 1000,
			EndedAt:   4000,
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
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewMemoryFlushWorker(app, MemoryFlushWorkerOptions{
		WorkerID:         "memory-worker-a",
		BatchSize:        4,
		LeaseTTL:         90 * time.Second,
		MaxAttempts:      3,
		RetryBackoff:     2 * time.Minute,
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, MemoryFlushWorkerResult{
		ClaimedJobs:   1,
		SucceededJobs: 1,
	}, result)
	require.Equal(t, []RuntimeMemoryFlushJobMetricsObservation{
		{
			Result:         "success",
			ErrorCode:      "none",
			LatencyMs:      3000,
			ObserveLatency: true,
		},
	}, metrics.memoryFlushJobs)
	require.Equal(t, []RuntimeMemoryFactMetricsObservation{
		{Operation: "upsert", Result: "success", Count: 1},
	}, metrics.memoryFacts)
}

func TestMemoryFlushWorkerRecordsRuntimeMemoryFlushBacklogMetrics(t *testing.T) {
	domainSVC := &recordingThreadService{
		memoryFlushBacklogAggregates: []*entity.MemoryFlushBacklogAggregate{
			{Status: entity.MemoryFlushJobStatusPending, Count: 2},
			{Status: entity.MemoryFlushJobStatusProcessing, Count: 1},
		},
	}
	app := &ApplicationService{
		ThreadSVC:       domainSVC,
		MemoryExtractor: &recordingMemoryExtractor{},
	}
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewMemoryFlushWorker(app, MemoryFlushWorkerOptions{
		WorkerID:         "memory-worker-a",
		BatchSize:        4,
		LeaseTTL:         90 * time.Second,
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, MemoryFlushWorkerResult{}, result)
	require.Equal(t, &domainservice.AggregateMemoryFlushBacklogRequest{
		Statuses: []entity.MemoryFlushJobStatus{
			entity.MemoryFlushJobStatusPending,
			entity.MemoryFlushJobStatusProcessing,
		},
	}, domainSVC.aggregateMemoryFlushBacklogReq)
	require.Equal(t, []RuntimeMemoryFlushBacklogMetricsObservation{
		{Status: "pending", Count: 2},
		{Status: "processing", Count: 1},
	}, metrics.memoryFlushBacklogs)
}

func TestArtifactScanWorkerRecordsArtifactScanJobMetrics(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:         820,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    30,
				UserID:     40,
				ArtifactID: 100,
				FileID:     90,
				Scanner:    "clamav",
				Status:     entity.ArtifactScanJobStatusProcessing,
				WorkerID:   "artifact-worker-a",
				CreatedAt:  500,
				StartedAt:  1000,
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
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    13,
			Metadata:     `{"scan_status":"pending"}`,
		},
		completeScanJob: &entity.ArtifactScanJob{
			ID:        820,
			Scanner:   "clamav",
			Status:    entity.ArtifactScanJobStatusSucceeded,
			StartedAt: 1000,
			EndedAt:   4500,
		},
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
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewArtifactScanWorker(app, ArtifactScanWorkerOptions{
		Scanner:          "clamav",
		WorkerID:         "artifact-worker-a",
		BatchSize:        4,
		LeaseTTL:         90 * time.Second,
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, ArtifactScanWorkerResult{
		ClaimedJobs:   1,
		SucceededJobs: 1,
	}, result)
	require.Equal(t, []RuntimeArtifactScanJobMetricsObservation{
		{
			Scanner:           "clamav",
			ContentFamily:     "text",
			Result:            "success",
			ErrorCode:         "none",
			LatencyMs:         3500,
			QueueDelayMs:      500,
			ObserveLatency:    true,
			ObserveQueueDelay: true,
		},
	}, metrics.artifactScanJobs)
}

func TestArtifactScanWorkerRecordsRuntimeArtifactScanBacklogMetrics(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		artifactScanBacklogAggregates: []*entity.ArtifactScanBacklogAggregate{
			{Scanner: "clamav", Status: entity.ArtifactScanJobStatusPending, Count: 2},
			{Scanner: "http", Status: entity.ArtifactScanJobStatusProcessing, Count: 1},
		},
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: &recordingArtifactObjectReader{},
		ArtifactScanner:       &recordingArtifactContentScanner{},
	}
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewArtifactScanWorker(app, ArtifactScanWorkerOptions{
		Scanner:          "clamav",
		WorkerID:         "artifact-worker-a",
		BatchSize:        4,
		LeaseTTL:         90 * time.Second,
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, ArtifactScanWorkerResult{}, result)
	require.Equal(t, &domainservice.AggregateArtifactScanBacklogRequest{
		Statuses: []entity.ArtifactScanJobStatus{
			entity.ArtifactScanJobStatusPending,
			entity.ArtifactScanJobStatusProcessing,
		},
	}, artifactSVC.aggregateArtifactScanBacklogReq)
	require.Equal(t, []RuntimeArtifactScanBacklogMetricsObservation{
		{Scanner: "clamav", Status: "pending", Count: 2},
		{Scanner: "http", Status: "processing", Count: 1},
	}, metrics.artifactScanBacklogs)
}

func TestArtifactScanWorkerRecordsRuntimeMetrics(t *testing.T) {
	app := &ApplicationService{
		ArtifactSVC: &recordingArtifactService{},
	}
	metrics := &recordingRuntimeMetricsCollector{}
	worker := NewArtifactScanWorker(app, ArtifactScanWorkerOptions{
		Scanner:          "clamav",
		WorkerID:         "artifact-worker-a",
		MetricsCollector: metrics,
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, ArtifactScanWorkerResult{Errored: true}, result)
	require.Len(t, metrics.workerTicks, 1)
	require.Equal(t, RuntimeWorkerTickMetricsObservation{
		WorkerType: "artifact_scan",
		Result:     "failed",
		ErrorCode:  "process_failed",
	}, metrics.workerTicks[0].withoutElapsed())
}

func gatherRuntimeMetricFamiliesText(
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

type recordingRuntimeMetricsCollector struct {
	workerTicks          []RuntimeWorkerTickMetricsObservation
	runTerminals         []RuntimeRunMetricsObservation
	runQueueDelays       []RuntimeRunQueueDelayMetricsObservation
	runBacklogs          []RuntimeRunBacklogMetricsObservation
	tokens               []RuntimeTokenMetricsObservation
	modelCalls           []RuntimeModelCallMetricsObservation
	memoryFlushJobs      []RuntimeMemoryFlushJobMetricsObservation
	memoryFlushBacklogs  []RuntimeMemoryFlushBacklogMetricsObservation
	memoryFacts          []RuntimeMemoryFactMetricsObservation
	artifactScanJobs     []RuntimeArtifactScanJobMetricsObservation
	artifactScanBacklogs []RuntimeArtifactScanBacklogMetricsObservation
	mcpInvocations       []RuntimeMCPInvocationMetricsObservation
	mcpHealth            []RuntimeMCPHealthMetricsObservation
	webSearches          []RuntimeWebSearchMetricsObservation
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeWorkerTick(
	ctx context.Context,
	observation RuntimeWorkerTickMetricsObservation,
) {
	c.workerTicks = append(c.workerTicks, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeRunTerminal(
	ctx context.Context,
	observation RuntimeRunMetricsObservation,
) {
	c.runTerminals = append(c.runTerminals, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeRunQueueDelay(
	ctx context.Context,
	observation RuntimeRunQueueDelayMetricsObservation,
) {
	c.runQueueDelays = append(c.runQueueDelays, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeRunBacklog(
	ctx context.Context,
	observations []RuntimeRunBacklogMetricsObservation,
) {
	c.runBacklogs = append(c.runBacklogs, observations...)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeTokenUsage(
	ctx context.Context,
	observation RuntimeTokenMetricsObservation,
) {
	c.tokens = append(c.tokens, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeModelCall(
	ctx context.Context,
	observation RuntimeModelCallMetricsObservation,
) {
	c.modelCalls = append(c.modelCalls, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeMemoryFlushJob(
	ctx context.Context,
	observation RuntimeMemoryFlushJobMetricsObservation,
) {
	c.memoryFlushJobs = append(c.memoryFlushJobs, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeMemoryFlushBacklog(
	ctx context.Context,
	observations []RuntimeMemoryFlushBacklogMetricsObservation,
) {
	c.memoryFlushBacklogs = append(c.memoryFlushBacklogs, observations...)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeMemoryFacts(
	ctx context.Context,
	observations []RuntimeMemoryFactMetricsObservation,
) {
	c.memoryFacts = append(c.memoryFacts, observations...)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeArtifactScanJob(
	ctx context.Context,
	observation RuntimeArtifactScanJobMetricsObservation,
) {
	c.artifactScanJobs = append(c.artifactScanJobs, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeArtifactScanBacklog(
	ctx context.Context,
	observations []RuntimeArtifactScanBacklogMetricsObservation,
) {
	c.artifactScanBacklogs = append(c.artifactScanBacklogs, observations...)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeMCPInvocation(
	ctx context.Context,
	observation RuntimeMCPInvocationMetricsObservation,
) {
	c.mcpInvocations = append(c.mcpInvocations, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeMCPHealth(
	ctx context.Context,
	observation RuntimeMCPHealthMetricsObservation,
) {
	c.mcpHealth = append(c.mcpHealth, observation)
}

func (c *recordingRuntimeMetricsCollector) RecordRuntimeWebSearch(
	ctx context.Context,
	observation RuntimeWebSearchMetricsObservation,
) {
	c.webSearches = append(c.webSearches, observation)
}

func (o RuntimeWorkerTickMetricsObservation) withoutElapsed() RuntimeWorkerTickMetricsObservation {
	o.ElapsedMs = 0
	return o
}

func (o RuntimeRunMetricsObservation) withoutLatency() RuntimeRunMetricsObservation {
	o.LatencyMs = 0
	return o
}

func (o RuntimeModelCallMetricsObservation) withoutLatency() RuntimeModelCallMetricsObservation {
	o.LatencyMs = 0
	return o
}

func (o RuntimeWebSearchMetricsObservation) withoutLatency() RuntimeWebSearchMetricsObservation {
	o.LatencyMs = 0
	return o
}

type runtimeMetricChatModel struct {
	message *schema.Message
	err     error
}

func (m *runtimeMetricChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.message, nil
}

func (m *runtimeMetricChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if m.err != nil {
		return nil, m.err
	}
	return schema.StreamReaderFromArray([]*schema.Message{m.message}), nil
}

type assertAnError string

func (e assertAnError) Error() string {
	return string(e)
}
