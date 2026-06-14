# Agent Run Worker Phase 15 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a controlled worker entrypoint for the Go-native Agent run processor without enabling run consumption by default.

**Architecture:** Keep worker orchestration in `backend/application/agentthread`, next to the Phase 14 `RunProcessor`. The worker wraps `ProcessPendingRuns(ctx)` with `RunOnce` and optional ticker startup. Application startup gets a default-disabled env hook; if the hook is enabled without a real executor, it logs and does not consume pending runs.

**Tech Stack:** Go, existing `envkey` helper, existing application package initialization, existing agentthread tests.

---

## Scope

This phase implements:

- `RunWorker` with `RunOnce(ctx)` and `Start(ctx)`.
- Env config:
  - `AGENT_THREAD_WORKER_ENABLED`, default `false`.
  - `AGENT_THREAD_WORKER_ID`, default `agent-harness`.
  - `AGENT_THREAD_WORKER_BATCH_SIZE`, default `10`.
  - `AGENT_THREAD_WORKER_INTERVAL_MS`, default `2000`.
- `StartRunWorkerFromEnv(ctx, app, executor)` helper.
- Safe application startup hook with nil executor, so the worker remains inactive until a real executor is wired.

This phase does not implement:

- Real LLM executor.
- Tool/MCP execution.
- SSE events.
- Distributed lease/heartbeat.
- Retry/backoff/stale running recovery.

## Files

- Create `backend/application/agentthread/worker.go`: worker, env config, startup helper.
- Create `backend/application/agentthread/worker_test.go`: TDD tests.
- Modify `backend/application/application.go`: call the safe startup hook after `agentThreadSVC` initialization.
- Add `docs/superpowers/plans/2026-06-14-agent-run-worker-phase15.md`.

## Task 1: Worker Tests

**Files:**
- Create: `backend/application/agentthread/worker_test.go`

- [ ] **Step 1: Write failing RunOnce test**

Test should build a `RunWorker` with an actual `RunProcessor` and recording domain service, then call `RunOnce(ctx)` and assert the processor claimed a run with the configured worker ID and batch size.

- [ ] **Step 2: Write failing env tests**

Tests should assert:

- default env does not start a worker;
- enabled env with nil executor does not start a worker;
- enabled env with executor returns a worker configured with env worker ID, batch size, and interval.

- [ ] **Step 3: Run tests to verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestRunWorker'
```

Expected: FAIL because `RunWorker`, `RunWorkerOptions`, and `StartRunWorkerFromEnv` do not exist.

## Task 2: Implement Worker

**Files:**
- Create: `backend/application/agentthread/worker.go`

- [ ] **Step 1: Implement `RunWorker`**

Add:

```go
type RunWorkerOptions struct {
	Interval time.Duration
}

type RunWorker struct {
	processor *RunProcessor
	interval  time.Duration
}

func NewRunWorker(processor *RunProcessor, opts RunWorkerOptions) *RunWorker
func (w *RunWorker) RunOnce(ctx context.Context)
func (w *RunWorker) Start(ctx context.Context)
```

`RunOnce` should log processor errors and return.

- [ ] **Step 2: Implement env startup helper**

Add:

```go
func StartRunWorkerFromEnv(ctx context.Context, app *ApplicationService, executor RunExecutor) *RunWorker
```

Behavior:

- return nil when `AGENT_THREAD_WORKER_ENABLED` is false or unset;
- return nil when enabled but `executor` is nil;
- create a `RunProcessor` and `RunWorker`, start it, and return the worker when enabled and executor is present.

- [ ] **Step 3: Run tests to verify GREEN**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestRunWorker'
```

Expected: PASS.

## Task 3: Application Startup Hook

**Files:**
- Modify: `backend/application/application.go`

- [ ] **Step 1: Add safe startup hook**

After `task.NewWorker(primaryServices.taskSVC).Start(ctx)`, call:

```go
agentthread.StartRunWorkerFromEnv(ctx, primaryServices.agentThreadSVC, nil)
```

Nil executor is intentional in this phase. The helper will not consume runs.

- [ ] **Step 2: Run package compile check**

Run:

```bash
cd backend
go test ./application/agentthread ./application
```

Expected: PASS or, if `./application` has unrelated environment-heavy tests, use `go test ./application -run TestNonExistent` as a compile check.

## Task 4: Final Verification and Commit

**Files:**
- All files above.

- [ ] **Step 1: Run focused verification**

Run:

```bash
cd backend
go test -count=1 ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository
go test ./application -run TestNonExistent
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
git add docs/superpowers/plans/2026-06-14-agent-run-worker-phase15.md \
  backend/application/agentthread/worker.go \
  backend/application/agentthread/worker_test.go \
  backend/application/application.go
git commit -m "feat: add controlled agent run worker"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan covers controlled startup and worker wrapping without expanding into LLM execution.
- Placeholder scan: No placeholders remain.
- Type consistency: `RunWorker`, `RunWorkerOptions`, and `StartRunWorkerFromEnv` are used consistently across tests and implementation.
