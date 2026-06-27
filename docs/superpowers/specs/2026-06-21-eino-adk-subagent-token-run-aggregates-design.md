# Eino ADK Subagent Token Run Aggregates Design

## Goal

M3.22 adds per-run token aggregates to the task-thread token usage response.
This lets task detail render per-subagent token cards from one parent rollup
request instead of issuing a separate token usage request for every child run.

## API Shape

`GET /api/workbench/task_threads/:thread_id/token_usage` may return:

```json
{
  "run_aggregates": [
    {
      "run_id": "1002",
      "aggregate": {
        "total_tokens": 300,
        "call_count": 2
      }
    }
  ]
}
```

The field is optional. It is populated when the backend has grouped run
aggregates, especially for `run_id=:parent&include_child_runs=true`.

## Frontend Use

Canonical task detail still lists top-level runs and direct subagent children.
For each top-level parent run, it requests token usage once with
`include_child_runs=true`, then maps `run_aggregates[].run_id` to each child
card.

If a child run has no matching aggregate or a zero-token aggregate, the UI
omits token metadata instead of showing a zero-cost claim.

## Safety Boundary

Grouped aggregates are metadata only: token counts, cost micros, call counts,
and source bucket totals. They must not include prompt text, completion text,
tool arguments/results, object URIs, provider raw bodies, credentials, or
checkpoint bytes.

## Deferred Work

Open follow-up work:

- cost and pricing snapshot presentation;
- provider/model attribution display;
- recursive descendant rollups if nested subagents become productized;
- browser visual QA against live multi-agent task fixtures.
