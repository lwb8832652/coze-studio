# Phase 34 - LangGraph Stateless Existing Run API

## Goal

Add LangGraph stateless-path operations for existing runs:

- `GET /api/runs/:run_id`
- `POST /api/runs/:run_id/cancel`
- `GET /api/runs/:run_id/stream`
- `POST /api/runs/:run_id/join`
- `GET /api/runs/:run_id/join`

This phase exposes run operations without requiring clients to know the backing `thread_id`.

## Scope

- Bind `run_id` from the stateless path.
- Reuse the existing persisted run as the source of truth.
- Return the same LangGraph run JSON shape as thread-bound routes.
- Cancel pending/queued/running runs by reading the current run status and using the existing Go `agentthread` application service.
- Reuse the Phase 31 SSE shape for `/api/runs/:run_id/stream`.
- Reuse the Phase 33 wait behavior for `/api/runs/:run_id/join`.
- Register stateless routes without redirects.

## Out of Scope

- `POST /api/runs`.
- `POST /api/runs/stream`.
- Auto-creating backing threads for stateless run creation.
- Eino graph execution changes.
- Token chunk mapping for `messages`.

## Verification

- Red: focused handler tests fail before implementation because stateless request models, handlers, and stream helper are missing.
- Green: focused handler and router tests pass after implementation.
- Diff hygiene: `git diff --check`.
