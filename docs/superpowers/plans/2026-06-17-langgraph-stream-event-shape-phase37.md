# Phase 37 - LangGraph Stream Event Shape

## Goal

Make persisted run events emit LangGraph-compatible SSE event names when callers request a `stream_mode`, while preserving the existing generic `events` stream by default.

## Scope

- Keep default replay behavior as `event: events` when no explicit `stream_mode` is requested.
- Support mode-specific output for:
  - `updates` from `node.*`, `step.*`, or explicit `updates` persisted events.
  - `messages` from `llm.*`, `message.*`, or explicit `messages` persisted events.
  - `values` from `state.*`, `checkpoint.*`, or explicit `values` persisted events.
  - `debug` and `custom` from matching persisted event prefixes.
  - `events` as the generic raw event fallback.
- Pass create-and-stream request `stream_mode` through to the actual SSE replay.
- Preserve metadata, event ids, terminal `end`, and `Last-Event-ID` replay behavior.

## Out Of Scope

- Real model token streaming from Eino.
- Checkpoint/state storage APIs.
- Frontend execution flow redesign.
- IM channels, memory, MCP tools, skills, token accounting, or security scanning.
- Full SDK black-box suite; this phase only adds focused handler coverage.

## Verification

- Add focused handler tests for `updates`, `messages`, and create-and-stream mode propagation.
- Re-run the focused LangGraph handler regression suite.
- Re-run LangGraph router registration tests.
- Run `git diff --check` and staged diff checks before committing.
