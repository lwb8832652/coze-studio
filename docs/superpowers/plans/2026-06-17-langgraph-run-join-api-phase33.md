# Phase 33 - LangGraph Run Join API

## Goal

Add the thread-bound LangGraph run join endpoints:

- `POST /api/threads/:thread_id/runs/:run_id/join`
- `GET /api/threads/:thread_id/runs/:run_id/join`

This phase lets clients wait for an existing run or subscribe to its events through the already migrated `/api/threads` path.

## Scope

- Bind `thread_id` and `run_id` from the path.
- Reject cross-thread access before waiting or opening SSE.
- `POST join` polls the run until it reaches `succeeded`, `failed`, or `canceled`, then returns the LangGraph run JSON.
- If `POST join` reaches `timeout_ms`, return the latest run JSON instead of holding the request indefinitely.
- `GET join` reuses the Phase 31 SSE stream shape for the same run.
- Support `interval_ms`, `timeout_ms`, `after_event_id`, and `Last-Event-ID` where applicable.
- Register both routes without redirects.

## Out of Scope

- Stateless `/api/runs/:run_id/join`.
- Worker scheduling or interrupt propagation.
- Eino graph execution changes.
- Token chunk mapping for `messages`.
- Frontend execution-flow consumption.

## Verification

- Red: focused handler tests fail before implementation because join request/handlers/helpers are missing.
- Green: focused handler and router tests pass after implementation.
- Diff hygiene: `git diff --check`.
