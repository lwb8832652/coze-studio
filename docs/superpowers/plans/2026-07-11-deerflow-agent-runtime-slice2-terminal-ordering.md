# DeerFlow Agent Runtime Slice 2 Terminal Ordering Plan

**Date:** 2026-07-11

**Status:** Complete

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

## Goal

Guarantee one durable order for every top-level run terminal outcome so
Workbench and LangGraph streams cannot emit `done`/`end` before the final
visible run event exists. Success, failure, interrupt, rollback, cancellation
and stale-lease recovery must expose a terminal state and its bounded terminal
event atomically.

This slice does not yet change `on_disconnect` or thread aggregate status;
those remain `AR-PARITY-001.11`.

## Verified DeerFlow Contract

- `backend/packages/harness/deerflow/runtime/runs/worker.py:339-398`
  persists the final run status before entering final cleanup; errors publish a
  bounded error frame after status persistence.
- `backend/packages/harness/deerflow/runtime/runs/worker.py:400-437` flushes the
  RunJournal, persists completion data, syncs title and thread status, then
  calls `bridge.publish_end` last.
- `backend/packages/harness/deerflow/runtime/stream_bridge/memory.py:68-123`
  stores regular events before setting `ended`; subscribers drain all retained
  events before receiving `END_SENTINEL`.
- `backend/app/gateway/services.py:455-467` translates `END_SENTINEL` to SSE
  `end` and returns immediately afterward.

## Confirmed NewX Gap

- ordinary and resume success commit assistant message and succeeded status,
  then append `run.completed` separately;
- failure appends `run.failed` before attempting the fenced failed transition,
  so a stale worker can leave a false failure event;
- non-ADK interrupt transitions status and appends `run.interrupted` separately;
- multitask interruption and expired-lease reconciliation persist terminal
  state without a terminal event in the same transaction;
- Workbench and LangGraph streams drain current events, inspect run status and
  immediately emit `done`/`end`, so any event written after terminal status can
  be skipped permanently.

Cancellation already writes `run.canceled` in the same transaction and is the
reference NewX pattern.

## Design

1. Extend terminal repository requests with bounded `RunEvent` entities.
2. Allocate event IDs in the domain service, never in the browser or handler.
3. Make success finalization one transaction containing status, assistant
   message, optional title event, completion event and title CAS.
4. Make failed/interrupted transitions and expired-lease reconciliation update
   status and insert their terminal event in one transaction.
5. For ADK interrupts whose mapped `run.interrupted` event is already durable,
   verify that event in the repository and allow the transition to reuse it
   rather than duplicate it. A false durability claim fails closed.
6. During multitask admission, allocate one terminal event ID per interrupted
   old run through a server-only allocator callback while the thread lock is
   held, then insert all events in the same admission transaction.
7. Remove best-effort terminal event writes from ordinary/resume processors.
8. Keep both SSE implementations event-first: once every terminal state has a
   durable preceding event, the existing drain-then-status check becomes safe.

## Tasks

- [x] **Task 1: RED repository and domain tests**
  - success event/message/status/title rollback as one unit;
  - failed/interrupted transition event atomicity and stale-fence suppression;
  - expired-lease terminal event atomicity;
  - multitask interrupted events and allocator failure rollback.
- [x] **Task 2: Implement terminal transactions**
  - contracts, ID allocation and bounded payload builders;
  - repository transactions for every terminal entry point;
  - idempotent cancellation remains unchanged.
- [x] **Task 3: Simplify processors**
  - pass ordinary/resume terminal payloads into domain transitions;
  - remove pre/post transition best-effort terminal writes;
  - preserve already-persisted ADK interrupt behavior.
- [x] **Task 4: Stream acceptance**
  - Workbench event precedes `done` for all outcomes;
  - LangGraph event/error precedes `end`;
  - reconnect from the previous cursor receives terminal event exactly once.
- [x] **Task 5: Verify, document and commit**
  - affected suites and targeted race;
  - `go vet`, serial full backend, debug build, formatting and diff checks;
  - evidence and tracker update.

## Exit Criteria

- A terminal status cannot commit without its required terminal event, except
  when an equivalent ADK interrupt event was durably written first and is
  verified in the repository transaction.
- A terminal event cannot claim success/failure/interruption when the fenced
  status transition did not commit.
- Workbench `done` and LangGraph `end` are always observed after the terminal
  event for fresh and reconnecting subscribers.
- No raw provider error, tool argument/result, checkpoint payload or lease
  metadata is added to terminal events.

## Completion Evidence

See
`docs/superpowers/evidence/2026-07-11-deerflow-agent-run-terminal-ordering.md`.
