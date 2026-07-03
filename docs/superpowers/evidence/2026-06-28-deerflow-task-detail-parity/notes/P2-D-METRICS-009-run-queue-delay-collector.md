# P2-D-METRICS-009 Run Queue Delay Collector

## Scope

- Implement the safe `coze_agent_thread_run_queue_delay_ms` histogram from the
  runtime metrics contract.
- Use only metadata already present on claimed runs: runtime config, run kind,
  `CreatedAt`, and `StartedAt`.
- Do not approximate `coze_agent_thread_runs_backlog` from claim counts.

## Source Verification

- `threadRepository.ClaimPendingRuns` updates pending task runs to running and
  sets `started_at` when a worker claims them.
- `RunProcessor.ProcessPendingRunsWithResult` receives those claimed runs as
  `RunSummary` values before executing the runtime.
- `ApplicationService.ListRuns` and domain `ListRuns` require a `ThreadID`.
  No global status aggregation exists yet, so a backlog gauge would need a new
  repository/domain contract instead of reusing per-thread list totals.

## Implementation

- Added `RuntimeRunQueueDelayMetricsObservation`.
- Added `RecordRuntimeRunQueueDelay` to the runtime metrics collector contract.
- Added Prometheus histogram `coze_agent_thread_run_queue_delay_ms` with
  bounded `runtime` and `source` labels.
- `RunProcessor` records queue delay immediately after claim and before
  executing each run.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollectorRecordsRunQueueDelaySafely|TestRunProcessorRecordsRuntimeRunTerminalMetrics' -count=1
go test ./application/agentthread -count=1
```

The targeted test passed locally. The full package test is part of the final
P2-D-METRICS-009 verification run.

## Residual Work

- `P2-D-METRICS-010` added the real run status aggregation contract and
  `coze_agent_thread_runs_backlog` gauge.
- `P2-D-METRICS-011`: continue MCP health state aggregation, model latency and
  failure metrics, web search, artifact-domain, and memory-domain collectors.
