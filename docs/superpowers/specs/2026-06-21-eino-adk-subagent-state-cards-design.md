# Eino ADK Subagent State Cards Design

## Goal

M3.17 shows durable subagent execution state in the task detail page. The UI
uses child `agent_runs` rows created by the ADK subagent recorder instead of
trying to infer state only from lifecycle events.

## Data Flow

For canonical task-thread details, the frontend loader now:

1. fetches thread messages, run events, token usage, and top-level runs;
2. requests child runs for each top-level run with `parent_run_id`;
3. maps `run_kind="subagent"` rows into `TaskDetailSubagentRun`;
4. passes the mapped rows through `useTaskDetailData` into the detail page.

Legacy task details keep the existing task/event path and do not request
thread runs.

## UI Behavior

The detail page renders a compact `子智能体执行` section when subagent rows are
available. Each row shows:

- subagent name from content-free `metadata.subagent.name`;
- durable status text;
- sanitized error message or assistant identity;
- latest update time.

Status mapping is intentionally simple:

- `running` -> `运行中`;
- `succeeded` / `completed` -> `已完成`;
- `failed` with `subagent_timeout` -> `超时`;
- other `failed` -> `失败`;
- `canceling` / `canceled` -> `已取消`;
- `queued` / `pending` / `created` -> pending labels.

## Safety Boundaries

Cards must not render tool arguments, model input, model output, final text,
checkpoint bytes, object URIs, or raw provider payloads. They consume only
durable run metadata fields already intended for task detail presentation.

## Deferred Work

Open follow-up work:

- join child run rows with child lifecycle event streams;
- expose per-child token and cost rollups;
- add child retry/resume controls after backend semantics are implemented;
- add browser visual QA once the task detail dev fixture is stable.
