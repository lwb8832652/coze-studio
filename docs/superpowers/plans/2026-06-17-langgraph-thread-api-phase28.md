# LangGraph Thread API Phase 28 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first LangGraph-compatible `/api/threads` facade for thread create/get/search on top of the Go-native agent thread service.

**Architecture:** Keep `agent_threads` as the source of truth and add a thin API adapter that maps LangGraph-style JSON to existing `application/agentthread` DTOs. This phase only exposes non-streaming thread lifecycle endpoints and returns an empty `values.messages` array until state/history checkpoints land later.

**Tech Stack:** Go, Hertz router/handler, existing `agentthread` application service, sqlite-backed handler tests.

---

### Task 1: Backend API Contract and RED Tests

**Files:**
- Create: `backend/api/model/agent/langgraph/thread.go`
- Create: `backend/api/handler/coze/langgraph_thread_service_test.go`
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`

- [ ] Write failing tests for:
  - `POST /api/threads` creates a thread from metadata.
  - `GET /api/threads/:thread_id` returns LangGraph-style thread JSON.
  - `POST /api/threads/search` returns a list filtered by `metadata.space_id`.
  - Router registration includes all three paths.
- [ ] Run focused Go tests and confirm failure from missing handlers/routes.

### Task 2: Handler and Router Implementation

**Files:**
- Create: `backend/api/handler/coze/langgraph_thread_service.go`
- Modify: `backend/api/router/coze/api.go`

- [ ] Add request/response structs for `CreateThread`, `GetThread`, and `SearchThreads`.
- [ ] Add metadata parsing helpers for `space_id`, `user_id`, `creator_id`, `title`, and `source`.
- [ ] Map application thread summaries to LangGraph thread JSON:
  - `thread_id` as a string.
  - `created_at` and `updated_at` as UTC RFC3339 timestamps.
  - `metadata` from persisted thread metadata.
  - `status` from agent thread status.
  - `values.messages` as an empty array for this phase.
- [ ] Register:
  - `POST /api/threads`
  - `GET /api/threads/:thread_id`
  - `POST /api/threads/search`

### Task 3: Verification and Commit

**Files:**
- All Phase 28 files.

- [ ] Run:
  - `go test ./api/handler/coze -run 'TestLangGraph(ThreadCreateGetAndSearchHandlers|ThreadRoutes)'`
  - `go test ./api/router/coze -run TestRegisterIncludesLangGraphThreadRoutes`
  - `git diff --check`
- [ ] Stage only Phase 28 files.
- [ ] Commit with `feat: add langgraph thread api`.
