# M3.27 Eino ADK Subagent Retry Executor Guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion.

**Goal:** Prevent queued `subagent_retry` placeholder runs from being processed
as ordinary empty-message task runs before the replay executor exists.

**Architecture:** `RunProcessor.processRun` performs a small command-shape
check after `run.started` and before `RunExecutor.Execute`. A non-null
top-level `subagent_retry` command fails closed with a fixed error code and
content-free failure event.

**Tech Stack:** Go, Coze agentthread application service tests.

---

### Task 1: Add Red Test

**Files:**
- Modify: `backend/application/agentthread/runner_test.go`

- [x] **Step 1: Add guarded retry fixture**

Create a claimed running task run with
`command={"subagent_retry":{"schema":"coze.subagent_retry.v1","source_run_id":20,"parent_run_id":10}}`.

- [x] **Step 2: Assert fail-closed behavior**

Assert that the executor is not called, no assistant message is appended, the
run is not completed, and `FailRun` receives
`error_code='subagent_retry_not_supported'`.

- [x] **Step 3: Assert content-free event**

Assert that emitted events are `run.started` then `run.failed`, and that the
failure payload does not include source or parent run IDs from the command.

- [x] **Step 4: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport' -count=1 -gcflags="all=-l -N"
```

Expected red result: the executor is still called before the guard exists.

### Task 2: Implement Guard

**Files:**
- Modify: `backend/application/agentthread/runner.go`

- [x] **Step 1: Add fixed error constants**

Add `subagent_retry_not_supported` and the short fixed message
`subagent retry executor is not implemented`.

- [x] **Step 2: Add command detector**

Parse the run command as JSON and return true only when the top-level
`subagent_retry` field is present and non-null.

- [x] **Step 3: Fail before executor**

After `run.started`, fail matching runs through the existing failure path and
return `runProcessFailed`.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-executor-guard-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-executor-guard-m3.md`

- [x] **Step 1: Add AGENTS rule**

Document that queued `subagent_retry` runs must fail closed until replay
execution is implemented.

- [x] **Step 2: Update roadmap**

Add M3.27 completion status and reduce remaining task/person-month estimates
by one small backend contract slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run frontend task tests and lint**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
cd frontend/apps/coze-studio && npx eslint src/pages/tasks/service.ts --cache --quiet
```

- [x] **Step 3: Check formatting and doc placeholders**

Run:

```bash
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-executor-guard-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-executor-guard-m3.md
```
