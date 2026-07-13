# DeerFlow Agent Run Multitask Admission Evidence

**Date:** 2026-07-11

**NewX branch:** `codex/deerflow-parity-mainline`

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

## Result

`AR-PARITY-001.9` is complete. NewX AI now applies one atomic, server-owned
admission decision to every public top-level run creation path. The default is
`reject`; executable strategies are `reject`, `interrupt` and `rollback`.
Unsupported `enqueue` remains accepted by the request schema for compatibility
but fails with HTTP 501, matching DeerFlow.

This evidence closes multitask admission only. It does not close the remaining
P0 terminal-event ordering, `on_disconnect` default or server-owned thread
lifecycle work.

## Verified DeerFlow Contract

The implementation was derived from source and tests, not screenshots:

- `backend/app/gateway/routers/thread_runs.py:37-67`
  - `multitask_strategy` defaults to `reject`;
  - the compatibility enum still includes `enqueue`;
  - `on_disconnect` independently defaults to `cancel`.
- `backend/app/gateway/services.py:356-372`
  - all run creation enters `RunManager.create_or_reject`;
  - conflict maps to HTTP 409;
  - unsupported strategy maps to HTTP 501.
- `backend/packages/harness/deerflow/runtime/runs/manager.py:540-626`
  - one manager lock covers active lookup, new-run registration and persistence;
  - active is same-thread `pending` or `running`;
  - the new run is persisted before old runs are interrupted;
  - failed or canceled new-run persistence leaves old runs untouched.
- `backend/packages/harness/deerflow/runtime/runs/worker.py:182-201,339-385,456-545`
  - rollback captures the pre-run checkpoint;
  - the worker finishes rollback as an error terminal state;
  - checkpoint state is restored to the pre-run snapshot, or reset when none
    existed.
- `backend/tests/test_run_manager.py:620-697,940-969` and
  `backend/tests/test_run_worker_rollback.py:257-476`
  - persistence ordering, thread scope, rollback status and checkpoint restore
    are executable reference tests.

## NewX Implementation

### Atomic admission

`CreateRunBundle` now locks the canonical thread row and rechecks idempotency
inside the same transaction before reading active top-level runs. NewX includes
`queued` in the active set because durable queued resume/retry rows are the Go
runtime equivalent of DeerFlow's in-memory pending run.

| Strategy | Active run | Result |
| --- | --- | --- |
| `reject` | none | create the complete run aggregate |
| `reject` | pending/queued/running | create nothing; return typed conflict |
| `interrupt` | present | persist new aggregate, fence and interrupt old runs |
| `rollback` | present | persist filtered new aggregate, fence old runs, then rollback worker state |
| `enqueue` or unknown | any | typed unsupported strategy error |

Child subagent rows do not participate. Internal subagent retry uses a
server-only admission bypass because it is the continuation mechanism for its
already active parent; its persisted strategy is normalized to `reject` and
the bypass cannot be supplied by a client.

### Fencing and commit order

For `interrupt` and `rollback`, all active old runs are updated only after the
new run, optional user message and optional event have been inserted inside the
same transaction. The update increments `execution_generation`, clears lease
owner/token/expiry, persists `cancel_requested_at`, and records a bounded
`multitask_interrupt` or `multitask_rollback` code. Any aggregate write failure
rolls the entire transaction back, including old-run changes.

After commit, the application notifies registered Eino executions through a
bounded asynchronous wait, so slow tool shutdown cannot block creation of the
replacement run. A stale worker that returns cancellation, loses its lease, or
attempts a late success reloads durable state and exits as interrupted without
writing an assistant message or overwriting the newer generation.

### Rollback equivalence

NewX checkpoints are keyed per run rather than sharing DeerFlow's LangGraph
thread checkpoint. The equivalent restore therefore has two coordinated parts:

1. authoritative history messages carry an internal `_run_id` marker in the
   persisted run input; the repository removes messages owned by the active
   rollback target before the replacement aggregate commits;
2. future authoritative history reconstruction excludes every run whose
   durable error code is `multitask_rollback`, and the worker deletes the old
   Eino checkpoint before transitioning that run from `interrupted` to
   `failed`. If checkpoint cleanup fails, the failure is still returned to the
   worker for operations visibility, but the old run is first persisted as
   `failed` so it cannot remain in a non-terminal limbo.

The `_run_id` marker is storage-only. `parseModelExecutorMessages` drops it
while decoding `schema.Message`, and a regression test confirms it is not
forwarded to the model payload.

### HTTP behavior

Workbench and LangGraph share the bounded error mapper:

- active-run conflict: HTTP 409, `thread already has an active run`;
- unsupported strategy: HTTP 501, `multitask strategy is not supported`.

Streaming creation performs admission before constructing the SSE writer, so
409/501 responses do not carry `text/event-stream` headers. Public errors do
not expose run IDs, leases, generation, worker identity, input, configuration
or interrupted-run metadata.

## Verification

All commands passed:

```bash
cd backend
go test -gcflags='all=-N -l' \
  ./domain/agentthread/repository \
  ./domain/agentthread/service \
  ./application/agentthread \
  ./api/router/coze -count=1

go test -gcflags='all=-N -l' ./api/handler/coze -count=1

go test -race -gcflags='all=-N -l' \
  ./domain/agentthread/repository ./application/agentthread \
  -run 'TestThreadRepositoryCreateRunBundleConcurrentRejectAdmitsOneRun|TestApplicationCreateTopLevelRun(UsesAdmissionBundleAndCancelsSupersededADK|DoesNotWaitForSupersededADKShutdown)|TestFinalizeMultitaskRollbackPersistsFailedStatusWhenCheckpointCleanupFails|TestRunProcessor(DiscardsLateSuccessAfterDurableMultitaskInterruption|CleansEinoCheckpointForDurableMultitaskRollback)' \
  -count=1

go vet ./domain/agentthread/repository ./domain/agentthread/service \
  ./application/agentthread ./api/handler/coze ./api/router/coze

go test -p 1 -gcflags='all=-N -l' ./... -count=1

cd ..
APP_ENV=debug make build_server
git diff --check
```

Coverage includes:

- default `reject` and unsupported `enqueue`;
- same-thread pending, queued and running conflicts;
- different-thread independence and child-subagent exclusion;
- idempotent replay while the original run is active;
- concurrent default requests admitting exactly one run;
- new aggregate failure leaving the old lease and generation untouched;
- interrupt/rollback generation fencing and post-commit Eino cancellation;
- current and future rollback history exclusion;
- Eino checkpoint deletion and `interrupted -> failed` rollback finalization;
- executor-cancel and late-success races;
- non-blocking post-commit cancellation and checkpoint-cleanup failure paths;
- Workbench/LangGraph 409/501 and pre-SSE conflict handling;
- full serial backend regression and debug server build.

No database migration was required.

## Remaining P0

The following are deliberately not marked complete by this slice:

1. durable terminal event and SSE `done` ordering for every success, failure,
   interrupt, rollback and cancellation path;
2. DeerFlow's `on_disconnect=cancel` default and disconnect ownership behavior;
3. server-owned thread lifecycle/status projection after run state changes.
