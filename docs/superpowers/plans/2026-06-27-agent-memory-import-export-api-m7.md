# Agent Memory Import Export API M7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add production-shaped task-thread memory import/export backend and
generated API contracts for the existing `任务记忆` management surface.

**Architecture:** Export is a read-only Workbench API backed by the existing
memory list path and returns a bounded `coze.task_thread_memories.export.v1`
payload. Import is a domain-owned write path that validates the same fields as
normal memory creation, preserves source-based idempotency, and records only a
metadata-safe aggregate audit event. Frontend controls are a later slice.

**Tech Stack:** Go, Hertz handlers, existing agentthread application/domain
services, Thrift IDL, `@coze-studio/api-schema`, Vitest, Go tests.

---

### Task 1: Domain Import Boundary

**Files:**
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Test: `backend/domain/agentthread/service/service_impl_test.go`

- [x] Add `ImportMemoryItem`, `ImportMemoriesRequest`, and
  `ImportMemoriesResult` to the domain service contract.
- [x] Add a failing test that imports two valid memory rows, skips a duplicate
  source row, trims fields, preserves run scope behavior, and writes one
  `memory.imported` audit event with `affected_count` equal to the created row
  count.
- [x] Implement `ImportMemories` by reusing the same validation and
  source-idempotent creation rules as `RememberMemory`.
- [x] Reject empty imports, imports above the bounded batch limit, invalid
  confidence, invalid scope, run-scope rows without `run_id`, negative
  correction IDs, negative timestamps, and negative expiry values.

### Task 2: Application Import/Export DTOs

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Test: `backend/application/agentthread/service_test.go`

- [x] Add export/import request and response DTOs.
- [x] Add a failing application test that export delegates to `ListMemories`
  with filters and returns schema, thread ID, total, exported timestamp, and
  memory summaries.
- [x] Add a failing application test that import forwards actor ID and item
  fields to the domain import boundary and maps imported/skipped counts.
- [x] Implement export with the existing list path and a bounded default page
  size.
- [x] Implement import by calling the domain `ImportMemories` method.

### Task 3: Workbench Handler And Routes

**Files:**
- Modify: `backend/api/model/workbench/thread/thread.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/router/coze/api.go`
- Test: `backend/api/handler/coze/workbench_thread_service_test.go`
- Test: `backend/api/router/coze/workbench_thread_route_test.go`

- [x] Add request/response models for
  `GET /api/workbench/task_threads/:thread_id/memories/export` and
  `POST /api/workbench/task_threads/:thread_id/memories/import`.
- [x] Add failing handler tests for export/import mapping and content-free
  import audit assumptions.
- [x] Add route coverage so export/import are not 404.
- [x] Implement handlers with authenticated actor ID for import and the same
  filter semantics as memory list/export.

### Task 4: IDL And API Schema

**Files:**
- Modify: `idl/workbench/task.thrift`
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Test: `frontend/packages/arch/api-schema/__tests__/workbench-task-memory.test.ts`

- [x] Add failing schema tests for `ExportTaskThreadMemories` and
  `ImportTaskThreadMemories` generated clients and metadata.
- [x] Add matching Thrift structs and service methods.
- [x] Add TypeScript request/response interfaces and `createAPI` clients.
- [x] Keep export/import contracts free of prompts, model text, tool
  arguments/results, checkpoint bytes, object URIs, raw provider payloads,
  credentials, URLs, filenames, hidden run config, and audit content.

### Task 5: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M7.9 import/export boundaries and remaining M7 work.
- [x] Run focused Go tests for domain, application, handler, and router.
- [x] Run focused api-schema tests and lint.
- [x] Run `git diff --check`.
