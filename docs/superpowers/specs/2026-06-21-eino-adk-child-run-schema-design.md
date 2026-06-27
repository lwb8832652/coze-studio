# Eino ADK Child Run Schema Design

## Goal

M3.13 adds the database and domain model foundation for durable subagent child
run rows without polluting the top-level task run list.

This slice does not yet create child rows from `AgentTool` invocation. It
prepares the schema, repository filters, service validation, and application
DTO mapping needed for that follow-up.

## Schema

`agent_runs` now has:

- `parent_run_id bigint not null default 0`
- `run_kind varchar(32) not null default 'task'`

Indexes:

- `idx_agent_runs_parent_created(parent_run_id, created_at)`
- `idx_agent_runs_thread_kind(thread_id, run_kind, created_at)`

Top-level task runs use `parent_run_id = 0` and `run_kind = 'task'`.
Subagent child runs use `parent_run_id = <parent run id>` and
`run_kind = 'subagent'`.

## Repository Semantics

`ListRuns` defaults to top-level runs only by adding `parent_run_id = 0` when
no explicit child-run query is requested. Callers may:

- pass `ParentRunID` to list children of one parent run;
- pass `IncludeChildRuns` for diagnostic or admin use cases that intentionally
  need mixed rows.

Worker claim queries also filter `parent_run_id = 0`, so pending child rows
cannot be claimed as independent top-level task work.

## Service Semantics

`CreateRun` defaults `run_kind` from the parent:

- no parent means `task`;
- positive `parent_run_id` means `subagent`.

The service rejects invalid combinations:

- task run with a parent;
- subagent run without a parent;
- child run whose parent belongs to a different thread.

## Deferred Work

Open follow-up work:

- create and update child rows from `ADKSubagentToolProvider` lifecycle
  events;
- attach child run IDs to lifecycle event payloads;
- roll up child status and token usage to parent task details;
- expose explicit child-run listing for the task detail UI;
- migrate any frontend run list assumptions that still expect all rows to be
  top-level task runs.
