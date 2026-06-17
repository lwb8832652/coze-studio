# Phase 39 - Agent Checkpoint Persistence Skeleton

## Goal

Add the durable checkpoint persistence skeleton required for the later LangGraph state/history cutover, while keeping Phase 38 state/history behavior unchanged.

## Scope

- Add `agent_checkpoints` migration and latest Atlas schema entry.
- Add domain checkpoint entity with thread/run ownership, parent checkpoint, namespace, channel values, channel versions, pending sends, metadata, and created timestamp.
- Add repository create/list/latest checkpoint methods with JSON validation and newest-first ordering.
- Add domain service methods that derive `thread_id` from the run, reject mismatched thread IDs, and default missing checkpoint JSON fields.
- Add application service DTOs and mapping methods for create/list/latest checkpoint operations.
- Add focused repository, domain service, and application service tests.

## Out Of Scope

- Wiring Phase 38 `/api/threads/:thread_id/state` or `/history` to `agent_checkpoints`.
- Eino runtime checkpoint writes.
- Resume, rollback, or update-state APIs.
- Frontend checkpoint/state/history views.
- IM channels, memory injection changes, artifacts, token usage changes, MCP, skills, or security scanning.

## Verification

- Run focused repository tests for checkpoint create/list/latest and invalid JSON.
- Run focused domain service tests for checkpoint defaults, thread/run ownership, list limit normalization, and latest lookup.
- Run focused application service mapping tests.
- Run `go test -count=1 ./domain/agentthread/... ./application/agentthread`.
- Run `git diff --check` and staged diff checks before committing.
