# Phase 31 - LangGraph Run Stream API

## Goal

Add the existing-run LangGraph stream endpoint:

- `GET /api/threads/:thread_id/runs/:run_id/stream`

This phase exposes persisted Go runtime run events through the LangGraph-style SSE wire shape while keeping the fully migrated `/api/threads` path.

## Scope

- Bind `thread_id` and `run_id` from the path.
- Reject cross-thread stream access before opening SSE.
- Emit a `metadata` SSE event for the current run.
- Replay persisted run events as `events` SSE events with numeric event ids.
- Emit `end` when the run is already terminal or becomes terminal while the stream is open.
- Support cursor replay through `after_event_id` and `Last-Event-ID`.
- Register the route in the Go router and cover it with route tests.

## Out of Scope

- `POST /api/threads/:thread_id/runs/stream` create-and-stream.
- Stateless `/api/runs/stream`.
- Token chunk mapping for `messages`.
- Eino graph execution changes.
- Frontend execution-flow consumption.

## Verification

- Red: focused handler tests fail before implementation because `StreamLangGraphRun` and `streamLangGraphRunEvents` are missing.
- Green: focused handler and router tests pass after implementation.
- Diff hygiene: `git diff --check`.
