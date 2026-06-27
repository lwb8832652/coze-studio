# Eino ADK Subagent Token Rollup Design

## Goal

M3.19 adds per-subagent token visibility to task detail cards. The frontend
uses the existing run-scoped token usage API, so this slice improves
observability without introducing a second accounting path.

## Data Flow

After canonical task detail loads child `run_kind="subagent"` rows, the loader
requests:

```text
GET /api/workbench/task_threads/:thread_id/token_usage?run_id=:child_run_id
```

for each child run. The response aggregate is mapped with the same
`TaskDetailTokenUsage` shape used by the top-bar thread usage indicator.

Cards render only:

- total token count;
- call count.

Input/output breakdown, provider/model attribution, price snapshots, and cost
rollups remain outside this slice.

## Boundaries

The child run row remains the primary identity and status source. Token rows
are display metadata only and must not change child terminal status.

The frontend must not infer missing usage as zero-cost execution. When the
aggregate has no total tokens, the card simply omits token metadata.

## Deferred Work

Open follow-up work:

- backend-side parent/child usage rollup APIs;
- cost and pricing snapshot presentation;
- per-child provider/model attribution;
- retry/resume controls after backend semantics are implemented;
- browser visual QA against live task fixtures.
