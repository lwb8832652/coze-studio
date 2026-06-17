# Phase 40 - LangGraph Checkpoint State And History Cutover

## Goal

Make LangGraph-compatible `/api/threads/:thread_id/state` and `/api/threads/:thread_id/history` prefer durable `agent_checkpoints`, while preserving Phase 38 message/event fallback behavior for threads without checkpoints.

## Scope

- Read the latest checkpoint for `GET /api/threads/:thread_id/state`.
- Return checkpoint `channel_values` as LangGraph `values`.
- Derive `next` from checkpoint `pending_sends`, with `values.next` fallback.
- Populate `config.configurable.thread_id`, `checkpoint_id`, and `checkpoint_ns` from the checkpoint.
- Merge thread metadata with checkpoint metadata and add checkpoint identifiers, channel versions, source, and parent checkpoint metadata.
- Read checkpoint history for `GET /api/threads/:thread_id/history`.
- Keep event snapshot history as the fallback when no checkpoints exist.
- Keep message snapshot state as the fallback when no checkpoints exist.
- Preserve direct `/api/threads` routes with no redirects.

## Out Of Scope

- Runtime/Eino checkpoint writes.
- Resume, rollback, update-state, or checkpoint mutation APIs.
- Frontend checkpoint/history views.
- IM channels, MCP, skills, memory injection changes, artifacts storage, token usage changes, or security scanning.

## Verification

- Add handler tests proving state prefers the latest checkpoint over message snapshots.
- Add handler tests proving history prefers checkpoints over event snapshots.
- Re-run existing state/history fallback tests.
- Re-run LangGraph thread/run handler regressions.
- Re-run LangGraph router registration tests.
- Run `git diff --check` and staged diff checks before committing.
