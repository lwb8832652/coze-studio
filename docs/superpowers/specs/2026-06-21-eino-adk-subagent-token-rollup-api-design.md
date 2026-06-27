# Eino ADK Subagent Token Rollup API Design

## Goal

M3.21 adds a backend rollup option for parent and child subagent run usage.
The goal is to support DeerFlow-style multi-agent observability without
changing the meaning of existing run-scoped token usage queries.

## API Contract

`GET /api/workbench/task_threads/:thread_id/token_usage` accepts:

```text
run_id=:parent_run_id
include_child_runs=true
```

When the flag is present with a valid `run_id`, the response includes usage
rows and aggregate totals for:

- the requested parent run;
- direct child runs whose `parent_run_id` equals the requested run.

The default remains unchanged: `run_id` without `include_child_runs=true`
returns only that run.

## Boundaries

The rollup is direct-child only. Recursive descendant rollups require a
separate product and API decision because they change attribution scope and
can surprise cost accounting.

The endpoint still validates that `run_id` belongs to `thread_id` before
returning usage. The flag must not include sibling runs, other thread runs,
or child output content.

## Deferred Work

Open follow-up work:

- grouped per-child aggregate response shape;
- cost and pricing snapshot presentation;
- provider/model attribution rollups;
- frontend request reduction once grouped child aggregates exist;
- recursive descendant rollups if nested subagent UI becomes productized.
