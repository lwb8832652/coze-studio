# Phase 38 - LangGraph Thread State And History API

## Goal

Expose the first LangGraph-compatible thread state and history endpoints on the fully migrated `/api/threads` path.

## Scope

- Add `GET /api/threads/:thread_id/state`.
- Add `GET /api/threads/:thread_id/history`.
- Build current state from existing task-thread messages, with LangGraph-style `values`, `next`, `config`, `metadata`, and timestamps.
- Build history from existing durable run events as checkpoint-like snapshots until the full `agent_checkpoints` table is implemented.
- Preserve thread metadata `source` and store the derived state source as `checkpoint_source`.
- Keep routes direct, with no redirect fallback.

## Out Of Scope

- New checkpoint persistence tables or migrations.
- Eino runtime checkpoint writes.
- Resume, rollback, or update-state commands.
- Memory, artifacts, token usage, IM channels, MCP, skills, or security scanning.
- Frontend state/history views.

## Verification

- Add handler tests for state messages/config and history event snapshots.
- Add router registration coverage for `/state` and `/history`.
- Re-run LangGraph thread/run handler regressions.
- Re-run LangGraph router registration tests.
- Run `git diff --check` and staged diff checks before committing.
