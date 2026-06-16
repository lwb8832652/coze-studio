# LangGraph Run API Phase 29 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add non-streaming LangGraph-compatible thread-bound run endpoints on top of the Go-native agent run service.

**Architecture:** Reuse `application/agentthread` for all persistence and run state changes. Add a thin LangGraph adapter that translates JSON request bodies into the existing string-backed run fields and maps stored runs back into LangGraph-style JSON.

**Tech Stack:** Go, Hertz router/handler, existing `agentthread` application service, sqlite-backed handler tests.

---

### Task 1: Backend API Contract and RED Tests

**Files:**
- Create: `backend/api/model/agent/langgraph/run.go`
- Create: `backend/api/handler/coze/langgraph_run_service_test.go`
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`

- [ ] Write failing tests for:
  - `POST /api/threads/:thread_id/runs` creates a pending run from LangGraph JSON.
  - `GET /api/threads/:thread_id/runs` lists runs for the path thread only.
  - `GET /api/threads/:thread_id/runs/:run_id` returns one run and rejects cross-thread access.
  - Router registration includes all three paths.
- [ ] Run focused Go tests and confirm failure from missing handlers/routes.

### Task 2: Handler and Router Implementation

**Files:**
- Create: `backend/api/handler/coze/langgraph_run_service.go`
- Modify: `backend/api/router/coze/api.go`

- [ ] Add request/response structs for create/list/get run.
- [ ] Add JSON conversion helpers for `input`, `command`, `metadata`, `config`, `context`, and `stream_mode`.
- [ ] Map application run summaries to LangGraph run JSON:
  - `run_id`, `thread_id`, `assistant_id`, `status`
  - `created_at`, `updated_at`
  - `metadata`, `input`, `config`, `context`
  - `stream_mode`, `multitask_strategy`, `on_disconnect`, `durability`
- [ ] Register:
  - `POST /api/threads/:thread_id/runs`
  - `GET /api/threads/:thread_id/runs`
  - `GET /api/threads/:thread_id/runs/:run_id`

### Task 3: Verification and Commit

**Files:**
- All Phase 29 files.

- [ ] Run:
  - `go test -count=1 ./api/handler/coze -run 'TestLangGraphRun(CreateListAndGetHandlers|GetRejectsCrossThreadRun)'`
  - `go test -count=1 ./api/router/coze -run 'TestRegisterIncludes(WorkbenchTaskThreadRoutes|LangGraphThreadRoutes|LangGraphRunRoutes)'`
  - `git diff --check`
- [ ] Stage only Phase 29 files.
- [ ] Commit with `feat: add langgraph run api`.
