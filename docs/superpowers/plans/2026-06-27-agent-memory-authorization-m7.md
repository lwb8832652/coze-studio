# Agent Memory Authorization M7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add production backend authorization for Workbench task-thread memory
management before adding user-facing permission controls.

**Architecture:** Memory management gets the same application-layer owner
authorization boundary as artifacts. Handlers pass the session viewer ID into
application DTOs. The application service authorizes every memory read/write,
export/import, restore, clear, and audit request before touching `ThreadSVC`.
`ActorID` remains audit identity and does not replace authorization.

**Tech Stack:** Go, Hertz handlers, existing agentthread application service,
domain `ThreadService`, focused Go tests.

---

### Task 1: Failing Authorization Coverage

**Files:**
- Modify: `backend/application/agentthread/service_test.go`

- [x] Add a failing table test covering unauthorized list, export, import,
  update, delete, clear, restore, and audit requests.
- [x] Assert each denied request records the expected operation, thread ID,
  memory ID when applicable, viewer ID, and does not call the underlying
  `ThreadSVC` repository path.
- [x] Add owner-authorizer coverage for creator versus non-creator access.

### Task 2: Application Authorization Boundary

**Files:**
- Create: `backend/application/agentthread/memory_authorization.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/init.go`

- [x] Add `ErrMemoryAccessDenied`, `MemoryAccessOperation`,
  `MemoryAccessRequest`, `MemoryAuthorizer`, and
  `ThreadOwnerMemoryAuthorizer`.
- [x] Add `MemoryAuthorizer` to `ApplicationService` and default wiring in
  `InitService`.
- [x] Add `ViewerID` to memory management DTOs separately from audit `ActorID`.
- [x] Authorize all memory management application methods before calling
  `ThreadSVC`.

### Task 3: Handler Viewer And Error Mapping

**Files:**
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] Pass `workbenchViewerIDFromCtx(ctx)` into list, export, import, update,
  delete, clear, restore, and audit memory application requests.
- [x] Map `ErrMemoryAccessDenied` to HTTP 403 with a bounded
  `memory access denied` response.
- [x] Add handler tests for session viewer propagation and forbidden mapping.
- [x] Keep denied responses free of memory content, metadata JSON, prompts,
  model text, tool arguments/results, checkpoint bytes, object URIs, raw
  provider payloads, credentials, URLs, filenames, and hidden run config.

### Task 4: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M7.11 backend memory authorization and remaining frontend
  permission affordance/browser E2E work.
- [x] Run focused application and handler Go tests.
- [x] Run `git diff --check`.
