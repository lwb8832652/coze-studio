# M3.29 Eino ADK Subagent Retry Executor Dispatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let replay-capable executors handle `subagent_retry` runs while
keeping unsupported executors fail-closed.

**Architecture:** `RunProcessor` keeps the `subagent_retry` command guard, but
checks for an explicit `SubagentRetryRunExecutor` capability before failing.
When present, it calls `ExecuteSubagentRetry` and reuses the same result
finalization path as ordinary execution.

**Tech Stack:** Go, Coze agentthread run processor tests.

---

### Task 1: Add Red Test

**Files:**
- Modify: `backend/application/agentthread/runner_test.go`

- [x] **Step 1: Add retry-capable fake executor**

Add `recordingSubagentRetryRunExecutor` with both `Execute` and
`ExecuteSubagentRetry`. Track whether each method is called and store the run
passed to each method.

- [x] **Step 2: Assert dispatch**

Add `TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor` with a
claimed running run containing:

```json
{"subagent_retry":{"schema":"coze.subagent_retry.v1","source_run_id":20,"parent_run_id":10}}
```

Assert that normal `Execute` is not called, `ExecuteSubagentRetry` is called
with the claimed run, the returned assistant message is appended, and the run
is completed.

- [x] **Step 3: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor' -count=1 -gcflags="all=-l -N"
```

Expected red result: current fail-closed behavior prevents the retry-capable
executor from running.

### Task 2: Implement Dispatch Boundary

**Files:**
- Modify: `backend/application/agentthread/runner.go`

- [x] **Step 1: Add capability interface**

Add:

```go
type SubagentRetryRunExecutor interface {
    ExecuteSubagentRetry(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
}
```

- [x] **Step 2: Reuse finalization**

Extract the existing post-executor result handling into
`finalizeRunExecution`. It should preserve the existing behavior for canceled,
interrupted, failed, empty-result, append-message failure, and successful runs.

- [x] **Step 3: Dispatch retry-capable executors**

When `isSubagentRetryCommand(run.Command)` is true, call
`ExecuteSubagentRetry` if the selected executor implements
`SubagentRetryRunExecutor`. Otherwise retain the fixed
`subagent_retry_not_supported` fail-closed path.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-executor-dispatch-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-executor-dispatch-m3.md`

- [x] **Step 1: Document executor capability boundary**

State that unsupported executors must fail closed and replay-capable executors
must implement `SubagentRetryRunExecutor`.

- [x] **Step 2: Update roadmap**

Add M3.29 completion status and reduce the remaining task/person-month
estimate by one backend dispatch slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused run processor tests**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor|TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestRunProcessorCompletesClaimedRunWithAssistantMessage|TestRunProcessorMarksRunFailedWhenExecutorErrors' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run broader affected backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestADKSubagentToolProvider|TestApplicationADKSubagentRunRecorder|TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor|TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Run frontend task tests and lint**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
cd frontend/apps/coze-studio && npx eslint src/pages/tasks/service.ts --cache --quiet
```

- [x] **Step 4: Check formatting and doc placeholders**

Run:

```bash
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-executor-dispatch-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-executor-dispatch-m3.md
```
