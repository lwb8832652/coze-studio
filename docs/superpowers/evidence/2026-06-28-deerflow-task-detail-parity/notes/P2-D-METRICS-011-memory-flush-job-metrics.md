# P2-D-METRICS-011 Memory Flush Job Metrics

## Scope

- Implement safe memory flush job counters and terminal latency from the runtime
  metrics contract.
- Keep labels bounded to `result` and `error_code`.
- Do not expose memory fact text, transcript content, scopes, snapshot IDs,
  thread/run IDs, raw errors, prompts, or model payloads.

## Source Verification

- `MemoryFlushWorker.RunOnce` delegates processing to
  `ApplicationService.ProcessMemoryFlushJobs`.
- `ProcessMemoryFlushJobs` already owns the per-job lifecycle outcome:
  succeeded, retried, failed, or skipped.
- `MemoryFlushJob` carries `StartedAt` and terminal `EndedAt`; retry paths are
  not terminal and should not emit latency samples.

## Implementation

- Added `RuntimeMemoryFlushJobMetricsObservation`.
- Added Prometheus counter `coze_agent_thread_memory_flush_jobs_total` with
  `result/error_code` labels.
- Added Prometheus histogram `coze_agent_thread_memory_flush_latency_ms` with a
  bounded `result` label.
- Extended the internal `ProcessMemoryFlushJobsResponse` with bounded
  `MemoryFlushJobMetricsSummary` values.
- `MemoryFlushWorker` records job metrics after processing and before worker
  tick metrics.
- Success and failed terminal jobs observe latency. Retried and skipped jobs are
  counted but do not write latency samples.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollectorRecordsMemoryFlushJobSafely|TestMemoryFlushWorkerRecordsMemoryFlushJobMetrics' -count=1
go test ./application/agentthread -count=1
```

Both commands passed locally.

## Residual Work

- `P2-D-METRICS-012` added artifact scan job counters, terminal latency, and
  queue delay.
- `P2-D-METRICS-013` added artifact scan backlog aggregation.
- `P2-D-METRICS-014` added memory flush backlog aggregation.
- `P2-D-METRICS-015`: memory facts counters, MCP health state, model
  latency/failure, and web search metrics.
