# Eino ADK Subagent Timeline Design

## Goal

M3.20 adds an expandable child lifecycle timeline to the canonical task detail
subagent cards. This improves DeerFlow-style execution visibility while keeping
the child run row as the durable source of identity and terminal status.

## Data Source

The timeline uses parent-run `subagent.run.*` events already returned by the
task-thread run event API. The frontend groups those events by
`payload.child_run_id` and attaches the ordered list to the matching durable
`run_kind="subagent"` child row.

The latest lifecycle event still provides lightweight card metadata such as
elapsed time and terminal classification. The timeline is only an expanded
view of the same content-free event stream.

## Display Fields

Each timeline row may display:

- event title derived from the event type;
- normalized lifecycle status;
- event timestamp;
- `elapsed_ms`;
- `terminal_classification`;
- sanitized lifecycle `error_message`.

The UI must not display child final output, model input/output, tool
arguments, tool results, checkpoint bytes, object URIs, URLs, filenames,
credentials, or provider raw payloads.

## Deferred Work

Open follow-up work:

- browser visual QA against live multi-agent task fixtures;
- retry/resume controls after backend child-run semantics are implemented;
- parent/child token and cost rollup APIs;
- per-child provider/model attribution.
