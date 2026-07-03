# P2-D-METRICS-010 Run Backlog Collector

## Scope

- Implement the safe `coze_agent_thread_runs_backlog` gauge from the runtime
  metrics contract.
- Use a real global run aggregation, not per-thread list totals or current
  worker claimed counts.
- Keep labels bounded to `runtime` and `status`; do not expose thread IDs, run
  IDs, raw config payloads, prompts, tool arguments, or other sensitive data.

## Source Verification

- `ListRuns` is thread-scoped, so it cannot represent system backlog.
- `ClaimPendingRuns` moves pending top-level runs to running and sets
  `started_at` when a worker claims them.
- Active backlog statuses are `pending`, `queued`, `running`, and
  `interrupted`. Terminal statuses (`succeeded`, `failed`, `canceled`) are not
  backlog.

## Implementation

- Added `entity.RunBacklogAggregate`.
- Added repository/domain service `AggregateRunBacklog` contracts.
- MySQL repository groups `agent_runs` by `status, config` for active statuses.
- Domain service normalizes the default active-status list.
- Runtime metrics collector now exposes disabled-by-default Prometheus gauge
  `coze_agent_thread_runs_backlog` with bounded `runtime/status` labels.
- Gauge recording resets the collector snapshot before writing current values,
  preventing stale labels after backlog state changes.
- `RunProcessor` records the backlog snapshot after claiming pending runs and
  before executing claimed runs. Metrics are fail-open and skipped when the
  collector is disabled.

## Verification

```bash
cd backend
go test ./domain/agentthread/repository ./application/agentthread -run 'TestThreadRepositoryAggregateRunBacklogGroupsActiveStatusesByConfig|TestRuntimePrometheusMetricsCollectorRecordsRunBacklogSnapshotSafely|TestRunProcessorRecordsRuntimeRunBacklogMetrics' -count=1
go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread -count=1
```

Both commands passed locally.

## Residual Work

- `P2-D-METRICS-011` added memory flush job counters and terminal latency.
- `P2-D-METRICS-012` added artifact scan job counters, terminal latency, and
  queue delay.
- `P2-D-METRICS-013` added artifact scan backlog aggregation.
- `P2-D-METRICS-014`: MCP health state aggregation, memory facts/backlog, model
  call latency/failure, and web search metrics.
