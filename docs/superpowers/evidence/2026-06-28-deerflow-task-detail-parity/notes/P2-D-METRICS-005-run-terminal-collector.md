# P2-D-METRICS-005 Run Terminal Collector

## Scope

Implement terminal run metrics for DeerFlow-parity task execution paths.

This slice covers run and resume processors only. It does not implement run
backlog gauges, queue delay histograms, model/token metrics, MCP invocation
metrics, web search metrics, artifact-domain metrics, or memory-domain metrics.

## Implemented

- Extended `RuntimeMetricsCollector` with terminal run observations.
- Added Prometheus metrics:
  - `coze_agent_thread_runs_total`
  - `coze_agent_thread_run_latency_ms`
- Instrumented `RunProcessor` terminal branches:
  - success
  - failed
  - interrupted
  - canceled
- Instrumented `ResumeRunProcessor` terminal branches with source `resume`.
- Reused the same disabled-by-default collector gate:
  `AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED`.

## Safety Boundary

Terminal run metrics emit only bounded metadata labels:

- `runtime`
- `source`
- `result`
- `error_code`

They do not emit prompts, model completions, tool payloads, checkpoint bytes,
thread IDs, run IDs, user IDs, space IDs, artifact object locations, file paths,
or raw error messages.

Invalid or path/secret-like label values are normalized to `unknown`.

## Verification

Passed:

```bash
cd backend
go test ./application/agentthread -run TestRunProcessorRecordsRuntimeRunTerminalMetrics -count=1
go test ./application/agentthread -run TestResumeRunProcessorRecordsRuntimeRunTerminalMetrics -count=1
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollector|TestRunProcessor|TestResumeRunProcessor|Test.*Worker.*' -count=1
go test ./application/agentthread -count=1
```

## Residual Work

- `P2-D-METRICS-006`: run backlog/queue delay, model/token, MCP, web search,
  artifact-domain, and memory-domain collectors.
