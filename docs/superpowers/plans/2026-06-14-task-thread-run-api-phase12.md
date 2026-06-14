# Task Thread Run API Phase 12 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a pending canonical run when a user follows up in a task-thread detail page.

**Architecture:** Add transitional workbench HTTP APIs under `/api/workbench/task_threads/:thread_id/runs` backed by the Phase 11 `agent_runs` application boundary. Then expose the APIs in the frontend schema/service layer and update canonical-only task detail follow-ups to append the user message and create a pending run with composer mode/resource context. This slice does not schedule workers, stream events, or execute tools.

**Tech Stack:** Go, Hertz, GORM/sqlite tests, React, TypeScript, Vitest, existing workbench task-thread API schema.

---

### Task 1: Backend Run API Contract

**Files:**
- Modify: `backend/api/model/workbench/thread/thread.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [ ] **Step 1: Write failing handler tests**

Add:
- `TestCreateTaskThreadRunHandlerCreatesPendingRun`
- `TestListTaskThreadRunsHandlerReturnsRuns`

The create test should:
- register `POST /api/workbench/task_threads/:thread_id/runs`
- call `installAgentThreadTestService`
- post JSON with `input`, `config`, `metadata`, and `idempotency_key`
- assert response contains `"status":"pending"`, `"thread_id":"1"`, and the input JSON
- verify `appagentthread.SVC.ListRuns` returns one run

The list test should:
- create two runs with `appagentthread.SVC.CreateRun`
- register `GET /api/workbench/task_threads/:thread_id/runs`
- assert response contains `total:2` and the newest run first

- [ ] **Step 2: Run handler tests to verify they fail**

```bash
cd backend
go test ./api/handler/coze -run 'Test(CreateTaskThreadRun|ListTaskThreadRuns)'
```

Expected: FAIL because API models and handlers do not exist.

- [ ] **Step 3: Implement API models and handlers**

Add model types:
- `TaskThreadRun`
- `CreateTaskThreadRunRequest/Response`
- `ListTaskThreadRunsRequest/Data/Response`

Add handlers:
- `CreateTaskThreadRun`
- `ListTaskThreadRuns`

Map all production-shaped `RunSummary` fields into API JSON.

- [ ] **Step 4: Run handler tests to verify they pass**

```bash
cd backend
go test ./api/handler/coze -run 'Test(CreateTaskThreadRun|ListTaskThreadRuns)'
```

Expected: PASS.

### Task 2: Backend Route Registration

**Files:**
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`
- Modify: `backend/api/router/coze/api.go`

- [ ] **Step 1: Write failing router assertions**

Extend the route test to check:
- `GET /api/workbench/task_threads/1/runs`
- `POST /api/workbench/task_threads/1/runs`

- [ ] **Step 2: Run router test to verify it fails**

```bash
cd backend
go test ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
```

Expected: FAIL because run routes are not registered.

- [ ] **Step 3: Register routes**

Under `_task_threads`, add:

```go
_task_threads.GET("/:thread_id/runs", append(_listtaskthreadrunsMw(), coze.ListTaskThreadRuns)...)
_task_threads.POST("/:thread_id/runs", append(_createtaskthreadrunMw(), coze.CreateTaskThreadRun)...)
```

- [ ] **Step 4: Run router test to verify it passes**

```bash
cd backend
go test ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
```

Expected: PASS.

### Task 3: Frontend Run API Client

**Files:**
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts`

- [ ] **Step 1: Write failing service export test**

Assert:

```ts
expect(typeof createTaskThreadRun).toBe('function');
expect(typeof listTaskThreadRuns).toBe('function');
```

- [ ] **Step 2: Run service test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts
```

Expected: FAIL because run clients are not exported.

- [ ] **Step 3: Add schema types and API clients**

Add:
- `TaskThreadRun`
- `CreateTaskThreadRunRequest/Response`
- `ListTaskThreadRunsRequest/Data/Response`
- `CreateTaskThreadRun`
- `ListTaskThreadRuns`

Use `/api/workbench/task_threads/:thread_id/runs`.

- [ ] **Step 4: Re-export clients from task service**

```ts
export const createTaskThreadRun = workbenchTask.CreateTaskThreadRun;
export const listTaskThreadRuns = workbenchTask.ListTaskThreadRuns;
```

- [ ] **Step 5: Run service test to verify it passes**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts
```

Expected: PASS.

### Task 4: Frontend Follow-Up Creates Pending Run

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: Write failing detail test**

Extend the canonical follow-up test to mock and assert `createTaskThreadRun` is called after `appendTaskThreadMessage`:

```ts
expect(mockCreateTaskThreadRun).toHaveBeenCalledWith({
  thread_id: 'thread-only-1',
  input: expect.any(String),
  config: expect.any(String),
  metadata: expect.any(String),
  idempotency_key: expect.any(String),
});
```

Parse the request:
- `input.messages[0].role === 'user'`
- `input.messages[0].content === '请追加行动建议'`
- `config.mode === 'Auto'`
- `metadata.source === 'workbench_detail_followup'`

- [ ] **Step 2: Run detail test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: FAIL because canonical follow-up currently only appends a message.

- [ ] **Step 3: Create pending run after appending user message**

In the canonical branch:
- append the user message
- call `createTaskThreadRun`
- include message id, mode, model, selected resources, and source metadata
- keep legacy branch on `sendWorkbenchChat`

- [ ] **Step 4: Run detail test to verify it passes**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

### Task 5: Verification and Commit

**Files:**
- Verify backend handler/router and frontend task tests
- Verify lint and whitespace

- [ ] **Step 1: Run backend focused tests**

```bash
cd backend
go test -count=1 ./api/handler/coze -run 'Test(ListTaskThreads|GetTaskThread|ListTaskThreadMessages|AppendTaskThreadMessage|CreateTaskThreadRun|ListTaskThreadRuns)'
go test -count=1 ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
go test -count=1 ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Run focused frontend lint**

```bash
cd frontend/apps/coze-studio
npx eslint --cache --quiet src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/detail.tsx src/pages/tasks/service.ts ../../packages/arch/api-schema/src/idl/workbench/task.ts
```

Expected: no lint errors.

- [ ] **Step 4: Run whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/2026-06-14-task-thread-run-api-phase12.md backend/api/model/workbench/thread/thread.go backend/api/handler/coze/workbench_thread_service.go backend/api/handler/coze/workbench_thread_service_test.go backend/api/router/coze/workbench_thread_route_test.go backend/api/router/coze/api.go frontend/packages/arch/api-schema/src/idl/workbench/task.ts frontend/apps/coze-studio/src/pages/tasks/service.ts frontend/apps/coze-studio/src/pages/tasks/detail.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx
git commit -m "feat: create pending runs for thread followups"
```
