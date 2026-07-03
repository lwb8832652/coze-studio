# P2-D-METRICS-015 Memory Facts Counter

## Scope

- Implement the safe `coze_agent_thread_memory_facts_total` counter from the
  runtime metrics contract.
- Count fact-level effects produced by memory flush jobs: upserts, removals,
  and skips.
- Keep labels bounded to `operation` and `result`; do not expose fact text,
  memory scopes, source IDs, transcript content, raw errors, prompts, model
  payloads, thread IDs, run IDs, user IDs, or snapshot IDs.

## Source Verification

- `ProcessMemoryFlushJobs` already computes successful memory write, skipped
  write, removed fact, and skipped remove counts before completing a job.
- The existing memory flush completed run event stores only counts and already
  avoids memory fact text.
- `MemoryFlushWorker.RunOnce` records memory flush job metrics and backlog
  snapshots, so it is the narrow place to add fact counters without adding
  exporter dependencies to domain services.

## Implementation

- Added `MemoryFactMetricsSummary` to memory flush job responses.
- `ProcessMemoryFlushJobs` now emits metadata-only fact summaries after a job
  completes successfully:
  - `operation=upsert,result=success`
  - `operation=remove,result=success`
  - `operation=skip,result=skipped`
- Runtime metrics collector exposes disabled-by-default Prometheus counter
  `coze_agent_thread_memory_facts_total` with bounded `operation/result`
  labels.
- Counter recording ignores non-positive counts and sanitizes invalid labels to
  reviewed fallbacks.
- `MemoryFlushWorker` records fact summaries after per-job metrics and before
  backlog/tick metrics.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollectorRecordsMemoryFactsSafely|TestApplicationProcessMemoryFlushJobsAppliesFactsToRemove|TestMemoryFlushWorkerRecordsMemoryFlushJobMetrics' -count=1
go test ./application/agentthread -count=1
```

Targeted tests passed locally; broader package tests are run as the final
verification for this slice.

## Residual Work

- `P2-D-METRICS-016` has since implemented web search metrics.
- `P2-D-METRICS-017`: MCP health state aggregation and model call
  latency/failure remain open.
