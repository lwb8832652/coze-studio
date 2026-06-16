# Phase 30 - LangGraph Run Cancel API

## Goal

Add the thread-bound LangGraph compatible run cancellation endpoint:

- `POST /api/threads/:thread_id/runs/:run_id/cancel`

This continues the native Go API facade work from Phase 28-29 and keeps the external path fully migrated under `/api/threads`, without redirect fallback.

## Scope

- Add request binding for `thread_id` and `run_id`.
- Resolve the run by `run_id`, then verify it belongs to the requested `thread_id`.
- Cancel pending/queued/running runs through the existing Go `agentthread` application service.
- Return the LangGraph run shape directly, without the Coze `code/msg` wrapper.
- Cover route registration, successful cancellation, and cross-thread rejection with tests.

## Out of Scope

- Streaming/join/stateless run APIs.
- Eino planner or graph execution integration.
- Worker interrupt propagation beyond persisted run status.
- Frontend execution-flow changes.

## Verification

- Red: focused handler tests fail before implementation because `CancelLangGraphRun` is missing.
- Green: focused handler and router tests pass after implementation.
- Diff hygiene: `git diff --check`.
