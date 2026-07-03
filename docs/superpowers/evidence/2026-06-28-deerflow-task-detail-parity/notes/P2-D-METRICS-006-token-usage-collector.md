# P2-D-METRICS-006 Token Usage Collector

## Scope

Implement token usage metrics for DeerFlow-parity model execution visibility.

This slice records input/output token counters after token usage has been
persisted. It does not implement model call latency/failure counters, cached or
reasoning token extraction, MCP invocation metrics, web search metrics,
artifact-domain metrics, memory-domain metrics, or run backlog/queue delay.

## Implemented

- Extended `RuntimeMetricsCollector` with token usage observations.
- Added Prometheus metric:
  - `coze_agent_thread_tokens_total`
- Added optional `ThreadUsageCollectorOptions` to inject runtime metrics.
- Recorded token metrics only after `ApplicationService.RecordTokenUsage`
  succeeds.
- Emitted positive input and output token counts as separate `direction`
  values.

## Safety Boundary

Token metrics emit only bounded metadata labels:

- `runtime`
- `model_family`
- `source`
- `direction`

The collector does not emit prompt text, completion text, raw usage JSON,
metadata JSON, trace IDs, step IDs, tool names, provider raw payloads, run IDs,
thread IDs, user IDs, or space IDs.

Model family labels only allow lowercase letters, digits, dots, underscores,
and hyphens. Values containing paths, spaces, URL-like content, or secret-like
payloads are normalized to `unknown`.

## Verification

Passed:

```bash
cd backend
go test ./application/agentthread -run 'TestThreadUsageCollectorRecordsRuntimeTokenMetrics|TestRuntimePrometheusMetricsCollectorRecordsTokenUsageSafely' -count=1
go test ./application/agentthread -run 'TestThreadUsageCollector|TestADKUsage|TestRuntimePrometheusMetricsCollector|TestRunProcessor|TestResumeRunProcessor|Test.*Worker.*' -count=1
go test ./application/agentthread -count=1
```

## Residual Work

- `P2-D-METRICS-007`: run backlog/queue delay, model call latency/failure,
  MCP, web search, artifact-domain, and memory-domain collectors.
