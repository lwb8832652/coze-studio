# M3.34 Eino ADK Subagent Retry Result Join Design

## Goal

Show subagent retry attempts on the original failed or canceled subagent card
in canonical task detail, so retry requests and their latest status remain
attached to the child run that triggered them.

## Scope

This slice is frontend projection only. The backend already creates retry runs
as top-level task runs with metadata `source="subagent_retry"` and
`source_run_id`. The frontend does not expose retry command, input, config, or
context payloads and does not infer final child output from retry runs.

## Data Model

`TaskDetailSubagentRun` gains an optional `retryAttempts` array. Each attempt
contains the retry top-level run ID, normalized status, status text, bounded
error fields, requested timestamp, and updated timestamp. The source child run
is resolved only from retry run metadata fields:

- `source === "subagent_retry"`
- `source_run_id`
- optional `requested_at`

Runs that match this retry metadata are not treated as ordinary parent runs
when fetching child subagent rows or grouped token usage. This keeps retry run
projection from creating extra child-list requests or mixing retry run usage
into the original parent rollup.

## UI

The subagent card detail line displays a compact metadata-only retry summary:
`重试 N 次` and `最近重试：状态`. It does not render retry output, messages, tool
arguments, model text, checkpoint bytes, object URIs, URLs, filenames, raw
provider payloads, or retry command/config/input/context.

## Testing

Task detail tests cover a parent run plus a top-level retry run whose metadata
points back to a failed child run. The expected behavior is that the failed
child card shows the retry summary and the loader does not query child runs for
the retry top-level run.
