# M4.6 Artifact List API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only workbench API for task artifact metadata.

**Architecture:** Reuse the M4.4 repository and M4.5 domain service list path,
map artifacts through the application DTO layer, and expose a workbench handler
that returns metadata only.

**Tech Stack:** Go, Hertz handler tests.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] **Step 1: Application mapping red test**

Add a test proving `ApplicationService.ListArtifacts` maps thread/run/page
parameters and does not expose object URI.

- [x] **Step 2: Handler red test**

Add a handler test that creates a runtime file, registers an artifact, calls
`GET /api/workbench/task_threads/:thread_id/artifacts`, and verifies metadata
without object URI leakage.

- [x] **Step 3: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationListArtifactsMapsDomainArtifacts' -count=1 -gcflags="all=-l -N"
cd backend && go test ./api/handler/coze -run 'TestListTaskThreadArtifactsHandlerReturnsArtifacts' -count=1 -gcflags="all=-l -N"
```

Expected red result: application DTOs, handler, and route are missing.

### Task 2: Implement Domain And Application List Mapping

**Files:**
- Modify: `backend/domain/agentthread/service/artifact.go`
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/init.go`

- [x] **Step 1: Add domain list method**

Expose `ListArtifacts` on the artifact service and delegate to the repository.

- [x] **Step 2: Add application DTOs**

Add `ArtifactSummary`, preview mode constants, list request, and list response.
Omit object URI from application DTOs.

- [x] **Step 3: Wire application service**

Add `ArtifactSVC` to `ApplicationService`, initialize it, and expose
`ListArtifacts`.

### Task 3: Implement Workbench API

**Files:**
- Modify: `backend/api/model/workbench/thread/thread.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] **Step 1: Add API model**

Add artifact request, response, data, and item DTOs without object URI.

- [x] **Step 2: Add handler**

Add `ListTaskThreadArtifacts` with optional `run_id` filtering.

- [x] **Step 3: Register route**

Register `GET /api/workbench/task_threads/:thread_id/artifacts`.

- [x] **Step 4: Extend handler test schema**

Add `agent_files` and `agent_artifacts` test tables.

### Task 4: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-artifact-list-api-design.md`
- Create: `docs/superpowers/plans/2026-06-21-artifact-list-api-m4.md`

- [x] **Step 1: Document list-only API**

State that the endpoint returns metadata only and does not provide content,
download links, or object URIs.

- [x] **Step 2: Update roadmap**

Add M4.6 completion status and reduce the remaining estimate by one artifact
list API slice.

### Task 5: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused tests**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationListArtifactsMapsDomainArtifacts' -count=1 -gcflags="all=-l -N"
cd backend && go test ./api/handler/coze -run 'TestListTaskThreadArtifactsHandlerReturnsArtifacts' -count=1 -gcflags="all=-l -N"
cd backend && go test ./api/router/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run affected backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestApplicationListArtifacts|TestListTaskThreadArtifacts|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Check formatting and doc placeholders**

Run:

```bash
gofmt -w backend/domain/agentthread/service/artifact.go backend/application/agentthread/dto.go backend/application/agentthread/thread_app.go backend/application/agentthread/service.go backend/application/agentthread/init.go backend/application/agentthread/service_test.go backend/api/model/workbench/thread/thread.go backend/api/handler/coze/workbench_thread_service.go backend/api/handler/coze/workbench_thread_service_test.go backend/api/router/coze/api.go backend/api/router/coze/workbench_thread_route_test.go
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-artifact-list-api-design.md docs/superpowers/plans/2026-06-21-artifact-list-api-m4.md
```
