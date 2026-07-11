# DeerFlow Agent Run Terminal Ordering Evidence

**Date:** 2026-07-11

**Task:** `AR-PARITY-001.10`

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

## Result

NewX now exposes one durable order for run termination: the bounded terminal
event is committed with the terminal state, or, for an Eino ADK interrupt, is
known to have been committed before the state transition. Workbench `done` and
LangGraph `end` therefore cannot overtake the visible terminal event.

## DeerFlow Reference Verified

- `backend/packages/harness/deerflow/runtime/runs/worker.py:339-398` persists
  terminal run status and publishes bounded failures.
- `backend/packages/harness/deerflow/runtime/runs/worker.py:400-437` flushes the
  journal, writes completion data, synchronizes title/thread state and only then
  calls `bridge.publish_end`.
- `backend/packages/harness/deerflow/runtime/stream_bridge/memory.py:68-123`
  retains regular events before setting the end flag; subscribers drain those
  events before `END_SENTINEL`.
- `backend/app/gateway/services.py:455-467` maps the sentinel to SSE `end`.

## NewX Contract Implemented

### Atomic terminal paths

- Success commits the fenced succeeded status, assistant message, optional
  title CAS + title event and `run.completed` in one transaction. The title
  event ID must sort before the completion event ID.
- Failed and non-ADK interrupted transitions commit status and `run.failed` or
  `run.interrupted` in one transaction. A lost lease writes neither.
- Expired lease reconciliation commits failed/interrupted state and its event
  together.
- Same-thread `interrupt` and `rollback` admission allocates one server event ID
  per superseded run while holding the thread lock, then persists each
  `run.interrupted` in the admission transaction.
- Cancellation keeps the existing status + `run.canceled` transaction.
- An Eino ADK interrupt carries `EventAlreadyPersisted=true`; the status update
  reuses the mapped `run.interrupted` and cannot create a duplicate. The
  repository verifies that a matching event exists for the same thread/run and
  current run start window before committing the status transition; a missing
  claimed event rolls the transition back.

### Processor and stream behavior

- Ordinary and resume processors pass bounded terminal payloads into domain
  transitions and no longer write terminal events through the best-effort event
  sink.
- Failure events retain status, worker and error code only. Provider error text,
  raw provider bodies, tool arguments/results and unknown fields are removed.
- Workbench and LangGraph still drain event pages before checking terminal
  status. Atomic persistence makes this ordering authoritative.
- Reconnect from an earlier cursor drains all remaining events including the
  final `run.completed` exactly once before `done`.

## Tests Added Or Strengthened

- Repository transaction and rollback coverage for success, failure,
  interruption, multitask admission and expired leases, including event-ID
  allocator failure, event insert failure and a falsely claimed pre-persisted
  interrupt event.
- Domain service ID allocation, event type, payload bounding, title/completion
  ordering and pre-persisted ADK interrupt coverage.
- Runner and resume-runner tests assert terminal payload handoff and absence of
  best-effort duplicate events.
- Workbench and LangGraph stream tests assert `run.completed` occurs before
  `done`/`end`.
- The Workbench 450-event reconnect fixture now includes the terminal event.

## Verification

All commands passed:

```bash
cd backend
go test -gcflags='all=-N -l' ./domain/agentthread/repository -run 'TestThreadRepository(CreateRunBundleRollsBackWhenInterruptedEventCannotPersist|UpdateRunStatusRollsBackWhenTerminalEventWriteFails|InterruptedTransitionRequiresClaimedDurableEvent|FinalizeRunSuccessCommitsMessageTitleAndStatusTogether|ReconcileExpiredRunLeaseUsesStaleFenceAndClearsLease)$' -count=1
go test -gcflags='all=-N -l' ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/handler/coze -count=1
go test -race -gcflags='all=-N -l' ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/handler/coze -count=1
go vet -gcflags='all=-N -l' ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/handler/coze
go test -p 1 -gcflags='all=-N -l' ./... -count=1
```

```bash
git diff --check
APP_ENV=debug make build_server
```

The macOS race link step emitted the existing malformed `LC_DYSYMTAB` warning;
all race binaries completed successfully with exit code zero.

## Remaining Boundary

This slice does not change server-owned disconnect behavior or thread aggregate
lifecycle projection. Those remain isolated in `AR-PARITY-001.11`.
