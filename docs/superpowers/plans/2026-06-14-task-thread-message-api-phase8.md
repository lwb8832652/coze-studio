# Task Thread Message API Phase 8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose canonical task-thread message history through backend workbench HTTP APIs.

**Architecture:** Build on Phase 7's application message boundary. Add manual transitional API DTOs under `backend/api/model/workbench/thread`, implement list/append handlers in `workbench_thread_service.go`, and register nested routes under `/api/workbench/task_threads/:thread_id/messages`. Frontend schema/client wiring remains a later slice.

**Tech Stack:** Go, Hertz, existing Coze handler conventions, agentthread application service, sqlite-backed handler tests.

---

### Task 1: Handler Contract

**Files:**
- Modify: `backend/api/model/workbench/thread/thread.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [ ] **Step 1: Write failing handler tests**

Add tests:
- `TestListTaskThreadMessagesHandlerReturnsMessages`
- `TestAppendTaskThreadMessageHandlerCreatesMessage`

The tests should:
- Register local routes directly on a Hertz test server
- Use `installAgentThreadTestService`
- Append seed messages through `appagentthread.SVC.AppendMessage`
- Assert `GET /api/workbench/task_threads/1/messages` returns message JSON
- Assert `POST /api/workbench/task_threads/1/messages` with JSON body creates a user message

- [ ] **Step 2: Run handler tests to verify they fail**

```bash
cd backend
go test ./api/handler/coze -run 'Test(ListTaskThreadMessages|AppendTaskThreadMessage)'
```

Expected: FAIL because DTOs and handlers are not implemented.

- [ ] **Step 3: Implement DTOs and handlers**

Add API model types:
- `TaskThreadMessage`
- `ListTaskThreadMessagesRequest/Data/Response`
- `AppendTaskThreadMessageRequest/Response`

Add handlers:
- `ListTaskThreadMessages`
- `AppendTaskThreadMessage`

Map application `MessageSummary` to API `TaskThreadMessage`. Responses keep existing `code/msg/data` shape.

- [ ] **Step 4: Run handler tests to verify they pass**

```bash
cd backend
go test ./api/handler/coze -run 'Test(ListTaskThreadMessages|AppendTaskThreadMessage)'
```

Expected: PASS.

### Task 2: Router Registration

**Files:**
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`
- Modify: `backend/api/router/coze/api.go`

- [ ] **Step 1: Write failing router test**

Extend `TestRegisterIncludesWorkbenchTaskThreadRoutes` to request:
- `GET /api/workbench/task_threads/1/messages`
- `POST /api/workbench/task_threads/1/messages`

Assert both are not 404.

- [ ] **Step 2: Run router test to verify it fails**

```bash
cd backend
go test ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
```

Expected: FAIL because nested message routes are not registered.

- [ ] **Step 3: Register routes**

In `backend/api/router/coze/api.go`, under `_task_threads`, add:
- `GET "/:thread_id/messages", coze.ListTaskThreadMessages`
- `POST "/:thread_id/messages", coze.AppendTaskThreadMessage`

- [ ] **Step 4: Run router test to verify it passes**

```bash
cd backend
go test ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
```

Expected: PASS.

### Task 3: Verification and Commit

**Files:**
- Verify focused handler/router/application/domain packages
- Verify diff whitespace

- [ ] **Step 1: Run focused Go tests**

```bash
cd backend
go test -count=1 ./api/handler/coze -run 'Test(ListTaskThreads|GetTaskThread|ListTaskThreadMessages|AppendTaskThreadMessage)'
go test -count=1 ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
go test -count=1 ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/plans/2026-06-14-task-thread-message-api-phase8.md backend/api/model/workbench/thread/thread.go backend/api/handler/coze/workbench_thread_service.go backend/api/handler/coze/workbench_thread_service_test.go backend/api/router/coze/workbench_thread_route_test.go backend/api/router/coze/api.go
git commit -m "feat: add task thread message api"
```
