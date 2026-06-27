# M3.26 Eino ADK Subagent Retry Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a backend API contract that turns a failed or canceled child
subagent run into a new top-level queued retry run.

**Architecture:** The application service validates the source child run,
loads its parent run, creates a top-level queued run with a content-free
`subagent_retry` command, and appends a metadata-only retry-requested event.
The HTTP handler exposes this through the Workbench task-thread API and the
frontend schema exports the callable endpoint.

**Tech Stack:** Go, Hertz, Coze agentthread application service, TypeScript API
schema.

---

### Task 1: Add Application Red Tests

**Files:**
- Modify: `backend/application/agentthread/service_test.go`

- [x] **Step 1: Add success fixture**

Create a failed child `run_kind='subagent'` run with `parent_run_id=10`, a
parent top-level run, and a fake created retry run.

- [x] **Step 2: Assert the retry contract**

Assert that `RetrySubagentRun` creates a queued top-level task run, copies the
parent run execution settings, uses `{"messages":[]}` input, stores a
`subagent_retry` command with source and parent run IDs, and appends
`subagent.retry.requested`.

- [x] **Step 3: Add rejection fixture**

Assert that a running child run is rejected with
`source subagent run must be failed or canceled`.

- [x] **Step 4: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationRetrySubagentRun' -gcflags="all=-l -N"
```

Expected red result: compile failure because `RetrySubagentRun` and request
types do not exist.

### Task 2: Implement Application Contract

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Create: `backend/application/agentthread/subagent_retry.go`

- [x] **Step 1: Add DTOs**

Add `RetrySubagentRunRequest` with `ThreadID`, `SourceRunID`, and
`IdempotencyKey`, plus `RetrySubagentRunResponse`.

- [x] **Step 2: Validate source run**

Require positive thread and source run IDs, source run ownership by thread,
`run_kind='subagent'`, non-zero parent run ID, and status `failed` or
`canceled`.

- [x] **Step 3: Create retry run**

Load the parent run, create a queued top-level task run with parent execution
settings, deterministic/default idempotency, metadata-only `subagent_retry`
command, and `{"messages":[]}` input.

- [x] **Step 4: Append retry event**

Append `subagent.retry.requested` on the new retry run with source child ID,
parent run ID, retry run ID, source status, and requested time only.

### Task 3: Expose HTTP And Schema

**Files:**
- Modify: `backend/api/model/workbench/thread/thread.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`

- [x] **Step 1: Add HTTP model and handler**

Add `RetryTaskThreadSubagentRunRequest` and
`RetryTaskThreadSubagentRunResponse`. The handler binds thread/run IDs and
optional `idempotency_key`, calls `RetrySubagentRun`, and returns
`TaskThreadRun`.

- [x] **Step 2: Add route**

Register:

```text
POST /api/workbench/task_threads/:thread_id/runs/:run_id/retry
```

- [x] **Step 3: Add frontend schema export**

Add `RetryTaskThreadSubagentRun` to the task API schema and export
`retryTaskThreadSubagentRun` from the task service.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run backend focused tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestRegisterIncludesWorkbenchTaskThreadRoutes' -gcflags="all=-l -N"
```

- [x] **Step 2: Run frontend lint for schema consumers**

Run:

```bash
cd frontend/apps/coze-studio && npx eslint src/pages/tasks/service.ts --cache --quiet
```

- [x] **Step 3: Check formatting**

Run:

```bash
git diff --check
```
