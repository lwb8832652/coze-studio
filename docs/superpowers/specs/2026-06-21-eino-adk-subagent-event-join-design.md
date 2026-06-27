# Eino ADK Subagent Event Join Design

## Goal

M3.18 enriches task detail subagent state cards with lifecycle event metadata.
The card remains durable-run-first, but it can now show the latest lifecycle
timing and terminal classification from parent run events.

## Join Key

The frontend uses `child_run_id` from `subagent.run.*` event payloads to join
events to child `TaskThreadRun` rows. The event mapper accepts numeric and
string child IDs because backend event payloads may encode IDs as either JSON
numbers or strings.

When multiple lifecycle events exist for the same child, the newest
`created_at` event wins.

## Display Fields

The card may show:

- `elapsed_ms` as a human-readable duration;
- `terminal_classification`;
- sanitized lifecycle `error_message` only when the child run row does not
  already contain an error message.

The durable child run status remains the primary state source.

## Safety Boundaries

The join reads only content-free lifecycle metadata. It must not render tool
arguments, model input, model output, final child text, checkpoint bytes,
object URIs, provider raw payloads, or arbitrary nested event content.

## Deferred Work

Open follow-up work:

- expandable child event timeline;
- child token and cost rollups;
- retry/resume controls after backend semantics are implemented;
- browser visual QA against live task fixtures.
