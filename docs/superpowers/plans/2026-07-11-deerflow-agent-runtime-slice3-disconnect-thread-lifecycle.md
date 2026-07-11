# DeerFlow Agent Runtime Slice 3 Disconnect And Thread Lifecycle Plan

> **For agentic workers:** REQUIRED SUB-SKILL: execute this plan task by task
> with RED/GREEN tests and review each transaction or stream ownership change
> before proceeding.

**Goal:** Align DeerFlow's server-owned disconnect policy and make NewX thread
status a projection of durable top-level run state.

**Architecture:** Persist only validated `cancel` or `continue` disconnect modes,
defaulting to DeerFlow's `cancel`. SSE loops classify normal terminal/timeout
completion separately from a canceled request or failed socket write; only a
real disconnect invokes an application-owned, persisted-policy cancellation.
Thread reads use a SQL projection over top-level `agent_runs`, so legacy or
stale `agent_threads.status` values cannot override active/terminal run state.

**Tech Stack:** Go, Hertz SSE, GORM, MySQL/SQLite-compatible SQL, Eino ADK
cancellation registry, Testify.

---

## Verified DeerFlow Contract

- Baseline: `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- `backend/app/gateway/routers/thread_runs.py:37-67` declares
  `on_disconnect=cancel` as the request default.
- `backend/app/gateway/services.py:313-392` persists the mode and marks thread
  metadata `running` when a run starts.
- `backend/app/gateway/services.py:439-515` makes both SSE and wait consumers
  poll connection state and calls `run_mgr.cancel` only for active `cancel`
  runs; `continue` keeps executing.
- `backend/packages/harness/deerflow/runtime/runs/manager.py:509-538` owns
  idempotent cancellation and interrupts the registered task.
- `backend/packages/harness/deerflow/runtime/runs/worker.py:400-437` projects
  terminal thread status before publishing stream end.
- `backend/tests/test_wait_disconnect_handling.py:61-181` covers completed,
  cancel-on-disconnect, continue-on-disconnect and already-terminal behavior.

## Confirmed NewX Gaps

- `newRunEntity` defaults missing mode to `continue` and accepts arbitrary
  values.
- Workbench and LangGraph stream loops return on `ctx.Done()` or writer failure
  without consulting persisted `Run.OnDisconnect` or canceling Eino execution.
- `agent_threads.status` is initialized as `idle` and is not advanced by the
  canonical run lifecycle, so list/get APIs can report stale state.

## Task 1: Disconnect Mode Contract

**Files:**

- Modify: `backend/domain/agentthread/service/service_impl.go`
- Test: `backend/domain/agentthread/service/service_impl_test.go`

- [x] Change the existing default assertion from `continue` to `cancel` and run
  it to observe the expected failure.
- [x] Add RED cases proving explicit `continue` is preserved and unknown modes
  are rejected as client errors before persistence.
- [x] Add `normalizeOnDisconnectMode`, use it in every run creation path, and
  rerun the domain service package.

## Task 2: Server-Owned Stream Disconnect Cancellation

**Files:**

- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/handler/coze/langgraph_run_service.go`
- Test: `backend/application/agentthread/service_test.go`
- Test: `backend/api/handler/coze/workbench_thread_service_test.go`
- Test: `backend/api/handler/coze/langgraph_run_service_test.go`

- [x] Add RED application tests: an active persisted `cancel` run is canceled,
  an explicit `continue` run is unchanged, and terminal runs are no-ops.
- [x] Add RED stream tests using canceled contexts and failing writers for both
  Workbench and LangGraph paths.
- [x] Implement `CancelRunOnDisconnect` using the authenticated context and the
  persisted run mode. Reuse the existing durable cancellation transaction and
  ADK cancel registry; do not trust query/body mode fields.
- [x] Detach cancellation persistence from the canceled request with a bounded
  timeout while preserving identity/access values.
- [x] Keep timeout and normal terminal exits non-canceling. Treat request
  cancellation and failed SSE writes/heartbeats as disconnects.

## Task 3: Durable Thread Lifecycle Projection

**Files:**

- Modify: `backend/domain/agentthread/repository/mysql.go`
- Test: `backend/domain/agentthread/repository/mysql_test.go`

- [x] Add RED Get/List/filter tests with deliberately stale thread rows and
  durable top-level runs.
- [x] Implement one MySQL/SQLite-compatible projection expression:
  active `pending/queued/running` wins as `running`; otherwise the newest
  top-level task run maps `succeeded/failed/canceled/interrupted` to
  `completed/failed/canceled/idle`; no run preserves the stored status.
- [x] Ignore child subagent runs and preserve correct filtered totals and stable
  pagination.
- [x] Verify Workbench and LangGraph thread adapters consume the projected
  status without client-side mutation.

## Task 4: Acceptance And Commit

- [x] Run affected packages with Mockey flags.
- [x] Run targeted race tests for repository projection and disconnect paths.
- [x] Run targeted `go vet`, serial full backend tests and
  `APP_ENV=debug make build_server`.
- [x] Record source, API, transaction, stream and test evidence; update the P0
  tracker and complete self-review.
- [x] Commit this mainline slice without pushing or merging `dev`.

## Exit Criteria

- Missing `on_disconnect` persists as `cancel`; only `cancel` and `continue`
  are accepted.
- A real stream disconnect cancels an active `cancel` run durably and notifies
  Eino; the same disconnect never cancels `continue` or terminal runs.
- Server stream timeout and successful terminal `done`/`end` are not mistaken
  for client disconnects.
- Thread list/get/filter status is derived from durable top-level runs and is
  unaffected by stale stored status or child subagents.
- Public APIs reveal no new lease, raw error, credential, tool payload or
  cancellation-registry internals.
