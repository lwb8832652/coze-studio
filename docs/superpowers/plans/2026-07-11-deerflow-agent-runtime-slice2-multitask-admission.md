# DeerFlow Agent Runtime Slice 2 Multitask Admission Plan

**Date:** 2026-07-11

**Status:** Complete

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

## Goal

Align NewX AI's top-level run admission with DeerFlow's verified
`RunManager.create_or_reject` contract. Same-thread concurrency must be decided
atomically before a new run becomes runnable. The default is `reject`; the only
executable strategies are `reject`, `interrupt` and `rollback`.

This slice covers the server execution contract, not a new UI strategy picker.

## Verified DeerFlow Contract

- `RunCreateRequest.multitask_strategy` defaults to `reject`.
- `RunManager.create_or_reject` holds one manager lock across active-run lookup,
  new-run registration and persistence.
- Active means `pending` or `running` on the same thread.
- `reject` returns HTTP 409 when an active run exists.
- `interrupt` and `rollback` persist the new run before changing old runs. A
  failed or canceled new-run write leaves old runs untouched.
- The manager supports only `reject`, `interrupt` and `rollback`; the retained
  request-model value `enqueue` reaches HTTP 501.
- An interrupted local worker receives an immediate abort signal. `rollback`
  additionally removes the interrupted run's resumable runtime state before it
  can influence subsequent execution.

Locked references:

- `backend/app/gateway/routers/thread_runs.py` lines 37-68;
- `backend/app/gateway/services.py` lines 356-394;
- `backend/packages/harness/deerflow/runtime/runs/manager.py` lines 354-397 and
  540-626;
- `backend/packages/harness/deerflow/runtime/runs/worker.py` lines 182-201,
  339-385 and 456-545;
- `backend/tests/test_run_manager.py` lines 620-697 and 940-969.

## NewX AI Design

1. Normalize top-level strategy to `reject` in the domain service and reject
   unsupported values with a typed error.
2. Reuse the existing run aggregate transaction for both message-backed and
   message-less public run creation.
3. Lock the canonical thread row, re-check idempotency under that lock, and
   inspect active top-level runs in the same transaction.
4. On `reject`, create nothing and return a typed active-run conflict.
5. On `interrupt` or `rollback`, insert the complete new aggregate first, then
   fence active old runs by incrementing execution generation, clearing leases
   and persisting the interruption action. Any failure rolls back all changes.
6. Return interrupted run identities to the application and notify registered
   Eino executions only after the transaction commits.
7. Treat a worker observing a durable supersession as a normal interrupted
   outcome, not a batch infrastructure failure. For `rollback`, remove the
   superseded run's Eino checkpoint before acknowledging the outcome.
8. Map active-run conflict to HTTP 409 and unsupported strategy to HTTP 501 for
   Workbench and LangGraph endpoints before SSE headers are opened.

Queued top-level resume/retry commands count as active because they are NewX's
durable equivalent of DeerFlow's in-memory pending run. Child subagent rows do
not participate in top-level admission.

## Tasks

- [x] **Task 1: RED repository and domain tests**
  - default strategy is `reject`;
  - unsupported `enqueue` fails closed;
  - same-thread `reject` conflict and different-thread independence;
  - pending, queued and running top-level rows participate;
  - child subagent rows do not block admission;
  - interrupt/rollback commit ordering, idempotent replay and rollback on write
    failure;
  - two concurrent same-thread requests admit at most one `reject` run.
- [x] **Task 2: Implement transactional admission**
  - extend aggregate result with interrupted runs;
  - lock thread and active rows;
  - fence old execution generations and clear leases;
  - preserve aggregate idempotency.
- [x] **Task 3: Wire execution cancellation and rollback cleanup**
  - notify Eino cancel registry after commit;
  - classify lease-loss supersession as interrupted;
  - delete superseded Eino checkpoint for rollback;
  - prevent a canceled executor from overwriting durable interrupted state.
- [x] **Task 4: Align HTTP contracts**
  - Workbench and LangGraph 409/501 mappings;
  - stream endpoints fail before SSE headers;
  - bounded public errors contain no hidden run metadata.
- [x] **Task 5: Verify, document and commit**
  - affected unit/integration and concurrency tests;
  - targeted race and `go vet`;
  - serial full backend suite;
  - `APP_ENV=debug make build_server`;
  - evidence and tracker update.

## Exit Criteria

- Two concurrent default submissions on one thread cannot both become active.
- A failed new aggregate never interrupts an existing run.
- A committed `interrupt` or `rollback` cannot be defeated by a late worker
  result or stale lease.
- The exact public default and HTTP error behavior matches the locked DeerFlow
  baseline.
- No browser-provided owner, status or history field participates in admission.

## Evidence

Acceptance evidence and exact commands:

`docs/superpowers/evidence/2026-07-11-deerflow-agent-run-multitask-admission.md`
