# Eino ADK Subagent Child Run API Design

## Goal

M3.16 exposes durable child run rows through the workbench task-thread API so
the task detail page can load subagent execution state without mixing child
runs into the default top-level task run list.

## API Contract

`GET /api/workbench/task_threads/:thread_id/runs` keeps its existing default:
when no child-specific query is present, it returns top-level task runs only.

The endpoint now accepts `parent_run_id`. When present and positive, the
handler forwards it to `ApplicationService.ListRuns`, which uses the existing
domain/repository child-run query path.

`TaskThreadRun` responses now include:

- `parent_run_id`;
- `run_kind`.

This allows frontend state cards to distinguish top-level task runs from
`run_kind="subagent"` child rows and group them under the parent run.

## Boundaries

This slice intentionally does not expose a mixed child/top-level admin view
and does not create child rows directly from HTTP. Child rows remain created
by the ADK subagent recorder when an Eino `AgentTool` starts.

The API also does not enrich rows with token rollups, event summaries, or
subagent card presentation fields yet. Those remain separate UI and usage
aggregation follow-ups.

## Testing

Handler tests create a parent run and a durable `run_kind="subagent"` child
run, then assert that querying with `parent_run_id` returns only the child and
includes `parent_run_id`, `run_kind`, and status fields.
