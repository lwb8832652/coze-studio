# P2-D-METRICS-014 Memory Flush Backlog Collector

## Scope

- Implement the safe `coze_agent_thread_memory_flush_backlog` gauge from the
  runtime metrics contract.
- Use a real global memory flush job aggregation, not current worker claimed
  counts.
- Keep labels bounded to `status`; do not expose thread IDs, run IDs, user IDs,
  assistant IDs, transcript snapshot IDs, idempotency keys, worker IDs, memory
  fact text, memory scopes, transcript content, raw errors, prompts, or model
  payloads.

## Source Verification

- `MemoryFlushJob` active backlog states are `pending` and `processing`.
- Terminal statuses (`succeeded`, `failed`) are not backlog.
- `MemoryFlushWorker.RunOnce` already owns the runtime tick cadence, so it can
  record a safe global snapshot after each successful processing attempt.
- Repository aggregation is required because per-tick claimed jobs cannot
  represent total backlog across workers.

## Implementation

- Added `entity.MemoryFlushBacklogAggregate`.
- Added repository/domain service `AggregateMemoryFlushBacklog` contracts.
- MySQL repository groups `agent_memory_flush_jobs` by `status` for active
  statuses.
- Domain service normalizes the default active-status list to `pending` and
  `processing`.
- Runtime metrics collector now exposes disabled-by-default Prometheus gauge
  `coze_agent_thread_memory_flush_backlog` with a bounded `status` label.
- Gauge recording resets the collector snapshot before writing current values,
  preventing stale labels after backlog state changes.
- `MemoryFlushWorker` records the backlog snapshot after per-job metrics and
  before worker tick metrics. Metrics are fail-open and skipped when the
  collector is disabled.

## Verification

```bash
cd backend
go test ./domain/agentthread/repository ./application/agentthread -run 'TestThreadRepositoryAggregateMemoryFlushBacklogGroupsActiveStatuses|TestRuntimePrometheusMetricsCollectorRecordsMemoryFlushBacklogSafely|TestMemoryFlushWorkerRecordsRuntimeMemoryFlushBacklogMetrics' -count=1
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread -count=1
```

Targeted tests passed locally; broader package tests are run as the final
verification for this slice.

## Residual Work

- `P2-D-METRICS-015` added memory facts counters.
- `P2-D-METRICS-016`: MCP health state aggregation, model call
  latency/failure, and web search metrics remain open.
