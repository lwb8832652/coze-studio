# Eino ADK Subagent Lifecycle Events Design

## Goal

M3.12 adds a Coze-owned lifecycle event protocol around Eino `AgentTool`
subagent invocation. This gives the task detail UI, audit pipeline, and future
child-run persistence a stable event stream before the `agent_runs` schema is
expanded for parent/child rows.

This slice does not create child run rows. Writing child rows into the current
top-level `agent_runs` list would pollute task listings because there is no
`parent_run_id` or run-kind column yet.

## Events

`ADKSubagentToolProvider` can be configured with
`WithADKSubagentToolProviderEventSink`. When present, each subagent `AgentTool`
is wrapped by a lifecycle tool that emits:

- `subagent.run.started`
- `subagent.run.completed`
- `subagent.run.failed`

Events are attached to the parent run's `thread_id` and `run_id`.

## Payload

Payloads are content-free and include:

- `source: "eino_adk"`
- `status`
- `parent_run_id`
- standardized `subagent` identity payload;
- `agent_id`
- `version`, when available;
- `is_draft`
- `elapsed_ms` for terminal events;
- sanitized `error_message` for failures.

The payload must not include tool arguments, model input, model output,
subagent final text, tool result content, checkpoint bytes, or object URIs.

## Provider Wiring

The production default provider accepts
`WithDefaultADKToolProviderEventSink(eventSink)` and passes it into the
subagent provider. `application.Init` now passes the existing ADK event sink
used by the executor, so subagent lifecycle events are persisted through the
same `agent_run_events` path as lead run events.

## Deferred Work

Open follow-up work:

- add schema support for durable child run rows without polluting top-level
  task run lists;
- connect child run row IDs to lifecycle events;
- roll up child status, usage, and artifacts into frontend state cards;
- add denied-grant lifecycle/audit events;
- propagate cancellation and timeout terminal classification into structured
  payload fields.
