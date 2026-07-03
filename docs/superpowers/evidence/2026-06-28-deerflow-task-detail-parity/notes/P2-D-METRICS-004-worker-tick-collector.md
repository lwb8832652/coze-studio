# P2-D-METRICS-004 Worker Tick Collector

## Scope

Implement the first code slice of the DeerFlow-parity runtime metrics
contract: worker tick metrics for the Go-native Agent Harness runtime.

This slice only covers aggregate worker tick observability. It does not
implement task terminal run metrics, model/token metrics, MCP invocation
metrics, web search metrics, artifact-domain metrics, or memory-domain
metrics.

## Implemented

- Added `RuntimeMetricsCollector` as a narrow application-layer interface.
- Added a disabled-by-default Prometheus collector gated by
  `AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED`.
- Added worker metrics:
  - `coze_agent_thread_worker_ticks_total`
  - `coze_agent_thread_worker_claimed_total`
  - `coze_agent_thread_worker_tick_latency_ms`
- Injected the collector into:
  - run worker
  - resume worker
  - memory flush worker
  - artifact scan worker
- Added safe-label tests that reject raw, user-controlled, path-like, or
  secret-like values as labels.

## Safety Boundary

The collector only emits bounded enum labels:

- `worker_type`
- `result`
- `error_code`

It does not emit user prompts, model completions, tool arguments, tool results,
checkpoint bytes, artifact object locations, memory text, file paths, secrets,
thread IDs, run IDs, or other high-cardinality identifiers.

Collector construction failures are logged and fail closed to `nil`; worker
execution continues without metrics.

## Verification

Passed:

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollector|TestRunWorkerRecordsRuntimeMetrics|TestResumeRunWorkerRecordsRuntimeMetrics|TestMemoryFlushWorkerRecordsRuntimeMetrics|TestArtifactScanWorkerRecordsRuntimeMetrics' -count=1
go test ./application/agentthread -run 'Test.*Worker.*|TestRuntimePrometheusMetricsCollector' -count=1
```

## Residual Work

- `P2-D-METRICS-005`: remaining task runtime, model/token, MCP, web search,
  artifact-domain, and memory-domain collectors.
