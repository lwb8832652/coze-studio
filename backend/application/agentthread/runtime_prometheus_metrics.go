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
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const agentThreadRuntimePrometheusMetricsEnabledEnv = "AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED"

const (
	runtimeWorkerTypeRun          = "run"
	runtimeWorkerTypeResume       = "resume"
	runtimeWorkerTypeMemoryFlush  = "memory_flush"
	runtimeWorkerTypeArtifactScan = "artifact_scan"

	runtimeMetricResultSuccess     = "success"
	runtimeMetricResultFailed      = "failed"
	runtimeMetricResultCanceled    = "canceled"
	runtimeMetricResultInterrupted = "interrupted"

	runtimeMetricErrorNone          = "none"
	runtimeMetricErrorUnknown       = "unknown"
	runtimeMetricErrorProcessFailed = "process_failed"
)

type RuntimeWorkerTickMetricsObservation struct {
	WorkerType string
	Result     string
	ErrorCode  string
	Claimed    int64
	ElapsedMs  int64
}

type RuntimeRunMetricsObservation struct {
	Runtime   string
	Source    string
	Result    string
	ErrorCode string
	LatencyMs int64
}

type RuntimeRunQueueDelayMetricsObservation struct {
	Runtime string
	Source  string
	DelayMs int64
}

type RuntimeRunBacklogMetricsObservation struct {
	Runtime string
	Status  string
	Count   int64
}

type RuntimeTokenMetricsObservation struct {
	Runtime     string
	ModelFamily string
	Source      string
	Direction   string
	Tokens      int64
}

type RuntimeModelCallMetricsObservation struct {
	Runtime     string
	ModelFamily string
	Result      string
	ErrorCode   string
	LatencyMs   int64
}

type RuntimeMemoryFlushJobMetricsObservation struct {
	Result         string
	ErrorCode      string
	LatencyMs      int64
	ObserveLatency bool
}

type RuntimeMemoryFlushBacklogMetricsObservation struct {
	Status string
	Count  int64
}

type RuntimeMemoryFactMetricsObservation struct {
	Operation string
	Result    string
	Count     int64
}

type RuntimeArtifactScanJobMetricsObservation struct {
	Scanner           string
	ContentFamily     string
	Result            string
	ErrorCode         string
	LatencyMs         int64
	QueueDelayMs      int64
	ObserveLatency    bool
	ObserveQueueDelay bool
}

type RuntimeArtifactScanBacklogMetricsObservation struct {
	Scanner string
	Status  string
	Count   int64
}

type RuntimeMCPInvocationMetricsObservation struct {
	Transport   string
	Result      string
	ErrorCode   string
	LatencyMs   int64
	OutputBytes int64
}

type RuntimeMCPHealthMetricsObservation struct {
	ServerID  int64
	Transport string
	Status    string
}

type RuntimeWebSearchMetricsObservation struct {
	Provider     string
	Result       string
	ErrorCode    string
	LatencyMs    int64
	ResultCount  int64
	ObserveCount bool
}

type RuntimeMetricsCollector interface {
	RecordRuntimeWorkerTick(ctx context.Context, observation RuntimeWorkerTickMetricsObservation)
	RecordRuntimeRunTerminal(ctx context.Context, observation RuntimeRunMetricsObservation)
	RecordRuntimeRunQueueDelay(ctx context.Context, observation RuntimeRunQueueDelayMetricsObservation)
	RecordRuntimeRunBacklog(ctx context.Context, observations []RuntimeRunBacklogMetricsObservation)
	RecordRuntimeTokenUsage(ctx context.Context, observation RuntimeTokenMetricsObservation)
	RecordRuntimeModelCall(ctx context.Context, observation RuntimeModelCallMetricsObservation)
	RecordRuntimeMemoryFlushJob(ctx context.Context, observation RuntimeMemoryFlushJobMetricsObservation)
	RecordRuntimeMemoryFlushBacklog(ctx context.Context, observations []RuntimeMemoryFlushBacklogMetricsObservation)
	RecordRuntimeMemoryFacts(ctx context.Context, observations []RuntimeMemoryFactMetricsObservation)
	RecordRuntimeArtifactScanJob(ctx context.Context, observation RuntimeArtifactScanJobMetricsObservation)
	RecordRuntimeArtifactScanBacklog(ctx context.Context, observations []RuntimeArtifactScanBacklogMetricsObservation)
	RecordRuntimeMCPInvocation(ctx context.Context, observation RuntimeMCPInvocationMetricsObservation)
	RecordRuntimeMCPHealth(ctx context.Context, observation RuntimeMCPHealthMetricsObservation)
	RecordRuntimeWebSearch(ctx context.Context, observation RuntimeWebSearchMetricsObservation)
}

type RuntimePrometheusMetricsCollector struct {
	workerTicksTotal       *prometheus.CounterVec
	workerClaimedTotal     *prometheus.CounterVec
	workerTickLatency      *prometheus.HistogramVec
	runsTotal              *prometheus.CounterVec
	runLatency             *prometheus.HistogramVec
	runQueueDelay          *prometheus.HistogramVec
	runsBacklog            *prometheus.GaugeVec
	tokensTotal            *prometheus.CounterVec
	modelCallsTotal        *prometheus.CounterVec
	modelCallLatency       *prometheus.HistogramVec
	memoryFlushJobs        *prometheus.CounterVec
	memoryFlushLatency     *prometheus.HistogramVec
	memoryFlushBacklog     *prometheus.GaugeVec
	memoryFacts            *prometheus.CounterVec
	artifactScanJobs       *prometheus.CounterVec
	artifactScanLatency    *prometheus.HistogramVec
	artifactScanQueueDelay *prometheus.HistogramVec
	artifactScanBacklog    *prometheus.GaugeVec
	mcpInvocations         *prometheus.CounterVec
	mcpLatency             *prometheus.HistogramVec
	mcpOutputBytes         *prometheus.HistogramVec
	mcpHealth              *prometheus.GaugeVec
	mcpHealthMu            sync.Mutex
	mcpHealthState         map[int64]runtimeMCPHealthState
	webSearchCalls         *prometheus.CounterVec
	webSearchLatency       *prometheus.HistogramVec
	webSearchResults       *prometheus.HistogramVec
}

type runtimeMCPHealthState struct {
	Transport string
	Status    string
}

var defaultRuntimePrometheusMetrics struct {
	once      sync.Once
	collector *RuntimePrometheusMetricsCollector
	err       error
}

func NewRuntimePrometheusMetricsCollector(
	registerer prometheus.Registerer,
) (*RuntimePrometheusMetricsCollector, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	collector := &RuntimePrometheusMetricsCollector{}
	var err error
	collector.workerTicksTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_worker_ticks_total",
				Help:      "Total number of DeerFlow-parity runtime worker ticks.",
			},
			[]string{"worker_type", "result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.workerClaimedTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_worker_claimed_total",
				Help:      "Total number of rows or jobs claimed by DeerFlow-parity runtime workers.",
			},
			[]string{"worker_type", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.workerTickLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_worker_tick_latency_ms",
				Help:      "DeerFlow-parity runtime worker tick latency in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000},
			},
			[]string{"worker_type", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.runsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_runs_total",
				Help:      "Total number of DeerFlow-parity runtime terminal runs.",
			},
			[]string{"runtime", "source", "result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.runLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_run_latency_ms",
				Help:      "DeerFlow-parity runtime terminal run latency in milliseconds.",
				Buckets:   []float64{100, 500, 1000, 5000, 15000, 30000, 60000, 300000, 900000, 1800000},
			},
			[]string{"runtime", "source", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.runQueueDelay, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_run_queue_delay_ms",
				Help:      "DeerFlow-parity runtime run queue delay from creation to worker claim in milliseconds.",
				Buckets:   []float64{0, 100, 500, 1000, 5000, 15000, 30000, 60000, 300000, 900000, 1800000},
			},
			[]string{"runtime", "source"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.runsBacklog, err = registerGaugeVec(
		registerer,
		prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "coze",
				Name:      "agent_thread_runs_backlog",
				Help:      "Current DeerFlow-parity runtime active run backlog by runtime and status.",
			},
			[]string{"runtime", "status"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.tokensTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_tokens_total",
				Help:      "Total persisted token usage for DeerFlow-parity runtime model calls.",
			},
			[]string{"runtime", "model_family", "source", "direction"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.modelCallsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_model_calls_total",
				Help:      "Total number of DeerFlow-parity runtime model calls.",
			},
			[]string{"runtime", "model_family", "result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.modelCallLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_model_latency_ms",
				Help:      "DeerFlow-parity runtime model call latency in milliseconds.",
				Buckets:   []float64{10, 50, 100, 250, 500, 1000, 5000, 15000, 30000, 120000, 600000},
			},
			[]string{"runtime", "model_family", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.memoryFlushJobs, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_memory_flush_jobs_total",
				Help:      "Total number of DeerFlow-parity memory flush jobs.",
			},
			[]string{"result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.memoryFlushLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_memory_flush_latency_ms",
				Help:      "DeerFlow-parity memory flush terminal job latency in milliseconds.",
				Buckets:   []float64{10, 50, 100, 250, 500, 1000, 5000, 15000, 30000, 120000, 600000},
			},
			[]string{"result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.memoryFlushBacklog, err = registerGaugeVec(
		registerer,
		prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "coze",
				Name:      "agent_thread_memory_flush_backlog",
				Help:      "Current DeerFlow-parity memory flush backlog by status.",
			},
			[]string{"status"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.memoryFacts, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_memory_facts_total",
				Help:      "Total number of DeerFlow-parity memory facts by operation and result.",
			},
			[]string{"operation", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.artifactScanJobs, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_artifact_scan_jobs_total",
				Help:      "Total number of DeerFlow-parity artifact scan jobs.",
			},
			[]string{"scanner", "content_family", "result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.artifactScanLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_artifact_scan_latency_ms",
				Help:      "DeerFlow-parity artifact scan terminal job latency in milliseconds.",
				Buckets:   []float64{10, 50, 100, 250, 500, 1000, 5000, 15000, 30000, 120000, 600000},
			},
			[]string{"scanner", "content_family", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.artifactScanQueueDelay, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_artifact_scan_queue_delay_ms",
				Help:      "DeerFlow-parity artifact scan queue delay from creation to worker claim in milliseconds.",
				Buckets:   []float64{0, 100, 500, 1000, 5000, 15000, 30000, 60000, 300000, 900000, 1800000},
			},
			[]string{"scanner", "content_family"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.artifactScanBacklog, err = registerGaugeVec(
		registerer,
		prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "coze",
				Name:      "agent_thread_artifact_scan_backlog",
				Help:      "Current DeerFlow-parity artifact scan backlog by scanner and status.",
			},
			[]string{"scanner", "status"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.mcpInvocations, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_mcp_invocations_total",
				Help:      "Total number of DeerFlow-parity MCP runtime invocations.",
			},
			[]string{"transport", "result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.mcpLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_mcp_latency_ms",
				Help:      "DeerFlow-parity MCP runtime invocation latency in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000},
			},
			[]string{"transport", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.mcpOutputBytes, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_mcp_output_bytes",
				Help:      "DeerFlow-parity MCP runtime invocation output bytes.",
				Buckets:   []float64{0, 128, 512, 1024, 4096, 16384, 65536, 262144, 1048576},
			},
			[]string{"transport", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.mcpHealth, err = registerGaugeVec(
		registerer,
		prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "coze",
				Name:      "agent_thread_mcp_health",
				Help:      "Current DeerFlow-parity MCP runtime server health by transport and status.",
			},
			[]string{"transport", "status"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.webSearchCalls, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "agent_thread_web_search_calls_total",
				Help:      "Total number of DeerFlow-parity web search calls.",
			},
			[]string{"provider", "result", "error_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.webSearchLatency, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_web_search_latency_ms",
				Help:      "DeerFlow-parity web search call latency in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 10000, 30000},
			},
			[]string{"provider", "result"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.webSearchResults, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "agent_thread_web_search_results",
				Help:      "DeerFlow-parity web search result count.",
				Buckets:   []float64{0, 1, 2, 3, 5, 8, 10, 20},
			},
			[]string{"provider", "result"},
		),
	)
	if err != nil {
		return nil, err
	}

	return collector, nil
}

func NewRuntimePrometheusMetricsCollectorFromEnv() *RuntimePrometheusMetricsCollector {
	if !envkey.GetBoolD(agentThreadRuntimePrometheusMetricsEnabledEnv, false) {
		return nil
	}
	defaultRuntimePrometheusMetrics.once.Do(func() {
		defaultRuntimePrometheusMetrics.collector, defaultRuntimePrometheusMetrics.err =
			NewRuntimePrometheusMetricsCollector(prometheus.DefaultRegisterer)
	})
	if defaultRuntimePrometheusMetrics.err != nil {
		logs.CtxWarnf(
			context.Background(),
			"[runtime-prometheus-metrics] collector init failed: %v",
			defaultRuntimePrometheusMetrics.err,
		)
		return nil
	}

	return defaultRuntimePrometheusMetrics.collector
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeWorkerTick(
	ctx context.Context,
	observation RuntimeWorkerTickMetricsObservation,
) {
	if c == nil {
		return
	}
	workerType := runtimeMetricsEnumLabel(observation.WorkerType, "unknown", 32)
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	tickLabels := prometheus.Labels{
		"worker_type": workerType,
		"result":      result,
		"error_code":  errorCode,
	}
	c.workerTicksTotal.With(tickLabels).Inc()
	if observation.Claimed > 0 {
		c.workerClaimedTotal.With(prometheus.Labels{
			"worker_type": workerType,
			"result":      result,
		}).Add(float64(observation.Claimed))
	}
	c.workerTickLatency.With(prometheus.Labels{
		"worker_type": workerType,
		"result":      result,
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.ElapsedMs)))
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeRunTerminal(
	ctx context.Context,
	observation RuntimeRunMetricsObservation,
) {
	if c == nil {
		return
	}
	runtime := runtimeMetricsEnumLabel(observation.Runtime, "unknown", 32)
	source := runtimeMetricsEnumLabel(observation.Source, "unknown", 32)
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	c.runsTotal.With(prometheus.Labels{
		"runtime":    runtime,
		"source":     source,
		"result":     result,
		"error_code": errorCode,
	}).Inc()
	c.runLatency.With(prometheus.Labels{
		"runtime": runtime,
		"source":  source,
		"result":  result,
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.LatencyMs)))
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeRunQueueDelay(
	ctx context.Context,
	observation RuntimeRunQueueDelayMetricsObservation,
) {
	if c == nil {
		return
	}
	c.runQueueDelay.With(prometheus.Labels{
		"runtime": runtimeMetricsEnumLabel(observation.Runtime, "unknown", 32),
		"source":  runtimeMetricsEnumLabel(observation.Source, "unknown", 32),
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.DelayMs)))
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeRunBacklog(
	ctx context.Context,
	observations []RuntimeRunBacklogMetricsObservation,
) {
	if c == nil {
		return
	}
	c.runsBacklog.Reset()
	for _, observation := range observations {
		c.runsBacklog.With(prometheus.Labels{
			"runtime": runtimeMetricsEnumLabel(observation.Runtime, "unknown", 32),
			"status":  runtimeMetricsEnumLabel(observation.Status, "unknown", 32),
		}).Set(float64(nonNegativeRuntimeMetricCount(observation.Count)))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeTokenUsage(
	ctx context.Context,
	observation RuntimeTokenMetricsObservation,
) {
	if c == nil || observation.Tokens <= 0 {
		return
	}
	c.tokensTotal.With(prometheus.Labels{
		"runtime":      runtimeMetricsEnumLabel(observation.Runtime, "unknown", 32),
		"model_family": runtimeMetricsModelFamilyLabel(observation.ModelFamily),
		"source":       runtimeMetricsEnumLabel(observation.Source, "unknown", 32),
		"direction":    runtimeMetricsEnumLabel(observation.Direction, "unknown", 32),
	}).Add(float64(observation.Tokens))
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeModelCall(
	ctx context.Context,
	observation RuntimeModelCallMetricsObservation,
) {
	if c == nil {
		return
	}
	runtime := runtimeMetricsEnumLabel(observation.Runtime, "unknown", 32)
	modelFamily := runtimeMetricsModelFamilyLabel(observation.ModelFamily)
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	c.modelCallsTotal.With(prometheus.Labels{
		"runtime":      runtime,
		"model_family": modelFamily,
		"result":       result,
		"error_code":   errorCode,
	}).Inc()
	c.modelCallLatency.With(prometheus.Labels{
		"runtime":      runtime,
		"model_family": modelFamily,
		"result":       result,
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.LatencyMs)))
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeMemoryFlushJob(
	ctx context.Context,
	observation RuntimeMemoryFlushJobMetricsObservation,
) {
	if c == nil {
		return
	}
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	c.memoryFlushJobs.With(prometheus.Labels{
		"result":     result,
		"error_code": errorCode,
	}).Inc()
	if observation.ObserveLatency {
		c.memoryFlushLatency.With(prometheus.Labels{
			"result": result,
		}).Observe(float64(nonNegativeRuntimeMetricMs(observation.LatencyMs)))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeMemoryFlushBacklog(
	ctx context.Context,
	observations []RuntimeMemoryFlushBacklogMetricsObservation,
) {
	if c == nil {
		return
	}
	c.memoryFlushBacklog.Reset()
	for _, observation := range observations {
		c.memoryFlushBacklog.With(prometheus.Labels{
			"status": runtimeMetricsEnumLabel(observation.Status, "unknown", 32),
		}).Set(float64(nonNegativeRuntimeMetricCount(observation.Count)))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeMemoryFacts(
	ctx context.Context,
	observations []RuntimeMemoryFactMetricsObservation,
) {
	if c == nil {
		return
	}
	for _, observation := range observations {
		count := nonNegativeRuntimeMetricCount(observation.Count)
		if count == 0 {
			continue
		}
		c.memoryFacts.With(prometheus.Labels{
			"operation": runtimeMetricsEnumLabel(observation.Operation, "unknown", 32),
			"result":    runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32),
		}).Add(float64(count))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeArtifactScanJob(
	ctx context.Context,
	observation RuntimeArtifactScanJobMetricsObservation,
) {
	if c == nil {
		return
	}
	scanner := runtimeMetricsEnumLabel(observation.Scanner, "unknown", 32)
	contentFamily := runtimeMetricsEnumLabel(observation.ContentFamily, "unknown", 32)
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	c.artifactScanJobs.With(prometheus.Labels{
		"scanner":        scanner,
		"content_family": contentFamily,
		"result":         result,
		"error_code":     errorCode,
	}).Inc()
	if observation.ObserveLatency {
		c.artifactScanLatency.With(prometheus.Labels{
			"scanner":        scanner,
			"content_family": contentFamily,
			"result":         result,
		}).Observe(float64(nonNegativeRuntimeMetricMs(observation.LatencyMs)))
	}
	if observation.ObserveQueueDelay {
		c.artifactScanQueueDelay.With(prometheus.Labels{
			"scanner":        scanner,
			"content_family": contentFamily,
		}).Observe(float64(nonNegativeRuntimeMetricMs(observation.QueueDelayMs)))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeArtifactScanBacklog(
	ctx context.Context,
	observations []RuntimeArtifactScanBacklogMetricsObservation,
) {
	if c == nil {
		return
	}
	c.artifactScanBacklog.Reset()
	for _, observation := range observations {
		c.artifactScanBacklog.With(prometheus.Labels{
			"scanner": runtimeMetricsEnumLabel(observation.Scanner, "unknown", 32),
			"status":  runtimeMetricsEnumLabel(observation.Status, "unknown", 32),
		}).Set(float64(nonNegativeRuntimeMetricCount(observation.Count)))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeMCPInvocation(
	ctx context.Context,
	observation RuntimeMCPInvocationMetricsObservation,
) {
	if c == nil {
		return
	}
	transport := runtimeMetricMCPTransport(observation.Transport)
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	c.mcpInvocations.With(prometheus.Labels{
		"transport":  transport,
		"result":     result,
		"error_code": errorCode,
	}).Inc()
	c.mcpLatency.With(prometheus.Labels{
		"transport": transport,
		"result":    result,
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.LatencyMs)))
	c.mcpOutputBytes.With(prometheus.Labels{
		"transport": transport,
		"result":    result,
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.OutputBytes)))
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeMCPHealth(
	ctx context.Context,
	observation RuntimeMCPHealthMetricsObservation,
) {
	if c == nil || observation.ServerID <= 0 {
		return
	}
	transport := runtimeMetricMCPTransport(observation.Transport)
	status := runtimeMetricMCPHealthStatus(observation.Status)

	c.mcpHealthMu.Lock()
	defer c.mcpHealthMu.Unlock()
	if c.mcpHealthState == nil {
		c.mcpHealthState = make(map[int64]runtimeMCPHealthState)
	}
	c.mcpHealthState[observation.ServerID] = runtimeMCPHealthState{
		Transport: transport,
		Status:    status,
	}

	counts := map[runtimeMCPHealthState]int64{}
	for _, state := range c.mcpHealthState {
		counts[state]++
	}
	c.mcpHealth.Reset()
	for state, count := range counts {
		c.mcpHealth.With(prometheus.Labels{
			"transport": state.Transport,
			"status":    state.Status,
		}).Set(float64(count))
	}
}

func (c *RuntimePrometheusMetricsCollector) RecordRuntimeWebSearch(
	ctx context.Context,
	observation RuntimeWebSearchMetricsObservation,
) {
	if c == nil {
		return
	}
	provider := runtimeMetricsEnumLabel(observation.Provider, "unknown", 32)
	result := runtimeMetricsEnumLabel(observation.Result, runtimeMetricResultFailed, 32)
	errorCode := runtimeMetricsEnumLabel(observation.ErrorCode, runtimeMetricErrorUnknown, 64)
	c.webSearchCalls.With(prometheus.Labels{
		"provider":   provider,
		"result":     result,
		"error_code": errorCode,
	}).Inc()
	c.webSearchLatency.With(prometheus.Labels{
		"provider": provider,
		"result":   result,
	}).Observe(float64(nonNegativeRuntimeMetricMs(observation.LatencyMs)))
	if observation.ObserveCount {
		c.webSearchResults.With(prometheus.Labels{
			"provider": provider,
			"result":   result,
		}).Observe(float64(nonNegativeRuntimeMetricCount(observation.ResultCount)))
	}
}

func runtimeMetricsEnumLabel(value, fallback string, maxLen int) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || len(normalized) > maxLen {
		return fallback
	}
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return fallback
	}
	return normalized
}

func runtimeMetricsModelFamilyLabel(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || len(normalized) > 64 {
		return "unknown"
	}
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return "unknown"
	}
	return normalized
}

func nonNegativeRuntimeMetricMs(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeRuntimeMetricCount(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func runtimeRunObservationLatencyMs(startedAt int64, endedAt int64) int64 {
	if startedAt <= 0 {
		return 0
	}
	if endedAt <= 0 {
		endedAt = time.Now().UnixMilli()
	}
	return nonNegativeRuntimeMetricMs(endedAt - startedAt)
}

func recordRuntimeRunTerminal(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	run *RunSummary,
	terminalRun *RunSummary,
	source string,
	result string,
	errorCode string,
) {
	if collector == nil || run == nil {
		return
	}
	if terminalRun == nil {
		terminalRun = run
	}
	if strings.TrimSpace(errorCode) == "" {
		errorCode = runtimeMetricErrorNone
	}
	collector.RecordRuntimeRunTerminal(ctx, RuntimeRunMetricsObservation{
		Runtime:   runtimeMetricsRunRuntime(run),
		Source:    runtimeMetricsRunSource(run, source),
		Result:    result,
		ErrorCode: errorCode,
		LatencyMs: runtimeRunObservationLatencyMs(run.StartedAt, terminalRun.EndedAt),
	})
}

func recordRuntimeRunQueueDelay(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	run *RunSummary,
) {
	if collector == nil || run == nil || run.CreatedAt <= 0 {
		return
	}
	claimedAt := run.StartedAt
	if claimedAt <= 0 {
		claimedAt = time.Now().UnixMilli()
	}
	collector.RecordRuntimeRunQueueDelay(ctx, RuntimeRunQueueDelayMetricsObservation{
		Runtime: runtimeMetricsRunRuntime(run),
		Source:  runtimeMetricsRunSource(run, ""),
		DelayMs: nonNegativeRuntimeMetricMs(claimedAt - run.CreatedAt),
	})
}

func recordRuntimeRunBacklog(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	aggregates []*entity.RunBacklogAggregate,
) {
	if collector == nil {
		return
	}
	observations := make([]RuntimeRunBacklogMetricsObservation, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if aggregate == nil {
			continue
		}
		observations = append(observations, RuntimeRunBacklogMetricsObservation{
			Runtime: runtimeMetricsRunRuntime(&RunSummary{Config: aggregate.Config}),
			Status:  string(aggregate.Status),
			Count:   aggregate.Count,
		})
	}
	collector.RecordRuntimeRunBacklog(ctx, observations)
}

func runtimeMetricsRunRuntime(run *RunSummary) string {
	mode, err := runtimeModeFromRun(run)
	if err != nil {
		return "unknown"
	}
	return string(mode)
}

func runtimeMetricsRunSource(run *RunSummary, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if run == nil {
		return "unknown"
	}
	if run.RunKind == RunKindSubagent || run.ParentRunID > 0 || isSubagentRetryCommand(run.Command) {
		return "subagent"
	}
	return "task"
}

func recordRuntimeTokenUsage(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	run *RunSummary,
	source TokenUsageSource,
	modelName string,
	inputTokens int64,
	outputTokens int64,
) {
	if collector == nil || run == nil {
		return
	}
	runtime := runtimeMetricsRunRuntime(run)
	tokenSource := strings.TrimSpace(string(source))
	if tokenSource == "" {
		tokenSource = string(TokenUsageSourceLeadAgent)
	}
	if inputTokens > 0 {
		collector.RecordRuntimeTokenUsage(ctx, RuntimeTokenMetricsObservation{
			Runtime:     runtime,
			ModelFamily: modelName,
			Source:      tokenSource,
			Direction:   "input",
			Tokens:      inputTokens,
		})
	}
	if outputTokens > 0 {
		collector.RecordRuntimeTokenUsage(ctx, RuntimeTokenMetricsObservation{
			Runtime:     runtime,
			ModelFamily: modelName,
			Source:      tokenSource,
			Direction:   "output",
			Tokens:      outputTokens,
		})
	}
}

func recordRuntimeMemoryFlushJob(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	metric *MemoryFlushJobMetricsSummary,
) {
	if collector == nil || metric == nil {
		return
	}
	collector.RecordRuntimeMemoryFlushJob(ctx, RuntimeMemoryFlushJobMetricsObservation{
		Result:         metric.Result,
		ErrorCode:      metric.ErrorCode,
		LatencyMs:      nonNegativeRuntimeMetricMs(metric.EndedAt - metric.StartedAt),
		ObserveLatency: metric.ObserveLatency,
	})
}

func recordRuntimeMemoryFlushBacklog(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	aggregates []*entity.MemoryFlushBacklogAggregate,
) {
	if collector == nil {
		return
	}
	observations := make([]RuntimeMemoryFlushBacklogMetricsObservation, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if aggregate == nil {
			continue
		}
		observations = append(observations, RuntimeMemoryFlushBacklogMetricsObservation{
			Status: string(aggregate.Status),
			Count:  aggregate.Count,
		})
	}
	collector.RecordRuntimeMemoryFlushBacklog(ctx, observations)
}

func recordRuntimeMemoryFacts(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	metrics []*MemoryFactMetricsSummary,
) {
	if collector == nil {
		return
	}
	observations := make([]RuntimeMemoryFactMetricsObservation, 0, len(metrics))
	for _, metric := range metrics {
		if metric == nil {
			continue
		}
		observations = append(observations, RuntimeMemoryFactMetricsObservation{
			Operation: metric.Operation,
			Result:    metric.Result,
			Count:     metric.Count,
		})
	}
	collector.RecordRuntimeMemoryFacts(ctx, observations)
}

func recordRuntimeArtifactScanJob(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	metric *ArtifactScanJobMetricsSummary,
) {
	if collector == nil || metric == nil {
		return
	}
	collector.RecordRuntimeArtifactScanJob(ctx, RuntimeArtifactScanJobMetricsObservation{
		Scanner:           metric.Scanner,
		ContentFamily:     metric.ContentFamily,
		Result:            metric.Result,
		ErrorCode:         metric.ErrorCode,
		LatencyMs:         nonNegativeRuntimeMetricMs(metric.EndedAt - metric.StartedAt),
		QueueDelayMs:      nonNegativeRuntimeMetricMs(metric.StartedAt - metric.CreatedAt),
		ObserveLatency:    metric.ObserveLatency,
		ObserveQueueDelay: metric.ObserveQueueDelay,
	})
}

func recordRuntimeArtifactScanBacklog(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	aggregates []*entity.ArtifactScanBacklogAggregate,
) {
	if collector == nil {
		return
	}
	observations := make([]RuntimeArtifactScanBacklogMetricsObservation, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if aggregate == nil {
			continue
		}
		observations = append(observations, RuntimeArtifactScanBacklogMetricsObservation{
			Scanner: aggregate.Scanner,
			Status:  string(aggregate.Status),
			Count:   aggregate.Count,
		})
	}
	collector.RecordRuntimeArtifactScanBacklog(ctx, observations)
}

func recordRuntimeMCPInvocation(
	ctx context.Context,
	collector RuntimeMetricsCollector,
	record ADKMCPRuntimeAuditRecord,
) {
	if collector == nil {
		return
	}
	result, terminal := runtimeMetricMCPResult(record.EventType)
	if !terminal {
		return
	}
	collector.RecordRuntimeMCPInvocation(ctx, RuntimeMCPInvocationMetricsObservation{
		Transport:   record.Transport,
		Result:      result,
		ErrorCode:   runtimeMetricErrorCode(record.ErrorCode),
		LatencyMs:   record.ElapsedMillis,
		OutputBytes: record.OutputBytes,
	})
}

func runtimeMetricMCPResult(eventType string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(eventType))
	switch {
	case strings.Contains(normalized, "completed"), strings.Contains(normalized, "success"):
		return runtimeMetricResultSuccess, true
	case strings.Contains(normalized, "failed"), strings.Contains(normalized, "error"):
		return runtimeMetricResultFailed, true
	default:
		return "", false
	}
}

func runtimeMetricMCPTransport(transport string) string {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case adkMCPRuntimeTransportStdio:
		return adkMCPRuntimeTransportStdio
	case adkMCPRuntimeTransportSSE:
		return adkMCPRuntimeTransportSSE
	case "http", "streamable-http", adkMCPRuntimeTransportStreamableHTTP:
		return adkMCPRuntimeTransportStreamableHTTP
	default:
		return "unknown"
	}
}

func runtimeMetricMCPHealthStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "healthy":
		return "healthy"
	case "unhealthy":
		return "unhealthy"
	default:
		return "unhealthy"
	}
}

func runtimeMetricErrorCode(code string) string {
	if strings.TrimSpace(code) == "" {
		return runtimeMetricErrorNone
	}
	return code
}

func newADKMCPRuntimeHealthReporterWithMetrics(
	base ADKMCPRuntimeHealthReporter,
	collector RuntimeMetricsCollector,
) ADKMCPRuntimeHealthReporter {
	metricsReporter := newADKMCPRuntimePrometheusHealthReporter(collector)
	if metricsReporter == nil {
		return base
	}
	if base == nil {
		return metricsReporter
	}
	return ADKMCPRuntimeHealthReporterFunc(func(
		ctx context.Context,
		report ADKMCPRuntimeHealthReport,
	) error {
		baseErr := base.ReportADKMCPRuntimeHealth(ctx, report)
		metricErr := metricsReporter.ReportADKMCPRuntimeHealth(ctx, report)
		if baseErr != nil {
			return baseErr
		}
		return metricErr
	})
}

func newADKMCPRuntimePrometheusHealthReporter(
	collector RuntimeMetricsCollector,
) ADKMCPRuntimeHealthReporter {
	if collector == nil {
		return nil
	}
	return ADKMCPRuntimeHealthReporterFunc(func(
		ctx context.Context,
		report ADKMCPRuntimeHealthReport,
	) error {
		status := "unhealthy"
		if report.Success {
			status = "healthy"
		}
		collector.RecordRuntimeMCPHealth(ctx, RuntimeMCPHealthMetricsObservation{
			ServerID:  report.ServerID,
			Transport: report.Transport,
			Status:    status,
		})
		return nil
	})
}
