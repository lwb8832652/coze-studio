# Agent Run Processor Phase 14 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first Go-native Agent Harness execution loop that can process claimed task-thread runs through a pluggable executor.

**Architecture:** Keep this as an application-layer coordinator over the Phase 13 run lifecycle APIs. The processor claims pending runs, calls an injected executor, appends an assistant message, and marks the run succeeded or failed. It does not start a background goroutine, call a real LLM, execute tools, stream events, or add HTTP APIs.

**Tech Stack:** Go, existing `backend/application/agentthread` package, existing domain/application tests.

---

## Scope

This phase implements:

- `RunExecutor` interface for future Go-native Agent Harness execution.
- `RunExecutorFunc` adapter for tests and lightweight wiring.
- `RunProcessor` with `ProcessPendingRuns(ctx)` and configurable worker ID / batch size.
- Successful execution path: claim run -> executor -> append assistant message -> complete run.
- Failed execution path: claim run -> executor error -> fail run with `executor_error`.

This phase does not implement:

- Eino/LLM integration.
- Tool calls, MCP calls, skills, memory, token usage, or safety scanning.
- Scheduler startup from `application.Init`.
- SSE/WebSocket event streaming.

## Files

- Create `backend/application/agentthread/runner.go`: processor, executor interface, options, and execution flow.
- Create `backend/application/agentthread/runner_test.go`: TDD tests for success and failure paths.
- Add `docs/superpowers/plans/2026-06-14-agent-run-processor-phase14.md`.

## Task 1: Run Processor Tests

**Files:**
- Create: `backend/application/agentthread/runner_test.go`

- [ ] **Step 1: Write failing success-path test**

The test should set up an `ApplicationService` with the existing `recordingThreadService`, a claimed running run, an executor that returns an assistant answer, and assert:

- `ClaimPendingRuns` receives `worker-a` and limit `1`.
- executor receives the claimed run.
- `AppendMessage` receives role `assistant`, the run ID, and the executor answer.
- `CompleteRun` receives `running -> succeeded` for the same worker.

- [ ] **Step 2: Write failing failure-path test**

The test should make the executor return an error and assert:

- no assistant message is appended;
- `FailRun` receives `executor_error` and the original error message;
- the processor returns nil when failure marking succeeds.

- [ ] **Step 3: Run tests to verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestRunProcessor'
```

Expected: FAIL because `RunProcessor`, `RunExecutorFunc`, and related types do not exist.

## Task 2: Implement Minimal Processor

**Files:**
- Create: `backend/application/agentthread/runner.go`

- [ ] **Step 1: Implement executor interface**

Add:

```go
type RunExecutor interface {
	Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
}

type RunExecutorFunc func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)

type RunExecutionResult struct {
	Message  string
	Metadata string
}
```

- [ ] **Step 2: Implement processor**

Add:

```go
type RunProcessor struct {
	app       *ApplicationService
	executor  RunExecutor
	workerID  string
	batchSize int32
}
```

`ProcessPendingRuns` should:

1. claim pending runs with worker ID and batch size;
2. execute each claimed run;
3. on executor error, call `FailRun`;
4. on success, append an assistant message and call `CompleteRun`.

- [ ] **Step 3: Run tests to verify GREEN**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestRunProcessor'
```

Expected: PASS.

## Task 3: Final Verification and Commit

**Files:**
- All files above.

- [ ] **Step 1: Run focused verification**

Run:

```bash
cd backend
go test -count=1 ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

Run:

```bash
git add docs/superpowers/plans/2026-06-14-agent-run-processor-phase14.md \
  backend/application/agentthread/runner.go \
  backend/application/agentthread/runner_test.go
git commit -m "feat: add agent run processor"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan covers the first Go-native execution loop while deliberately avoiding LLM/tool/SSE scope.
- Placeholder scan: No placeholders remain.
- Type consistency: `RunProcessor`, `RunExecutor`, `RunExecutionResult`, and `ProcessPendingRuns` names are consistent across tests and implementation.
