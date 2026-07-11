# DeerFlow Agent Run Cancellation Fence Evidence

**Date:** 2026-07-11

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX AI baseline:** `e929b710f`

## Scope

This slice closes the race between user cancellation and a worker publishing a
late assistant result. It also makes assistant message, optional first-turn
title and successful run status one database transaction. The visible target
is DeerFlow's idempotent cancellation behavior; the generation and lease fence
is the stronger control required by NewX AI's database-backed multi-worker
runtime.

## Verified DeerFlow Contract

Locked source:

- `backend/packages/harness/deerflow/runtime/runs/manager.py` lines 509-537;
- `backend/packages/harness/deerflow/persistence/run/model.py` lines 13-49.

`RunManager.cancel` executes under the process-local run lock. It:

1. returns success when the local run is already `interrupted`;
2. accepts only `pending` or `running` local runs;
3. stores the abort action and sets the abort event;
4. cancels the active asyncio task when present;
5. changes status to `interrupted` before releasing the lock;
6. persists the interrupted status after the locked mutation.

The persisted DeerFlow run row contains status and timestamps but no durable
owner, cancellation generation or lease token. Its cancellation guarantee is
therefore process-local. NewX AI must preserve the same idempotent visible
terminal behavior without copying that single-process limitation.

## NewX AI Contract

### Durable cancellation first

`RequestRunCancellation` is one repository transaction. It locks the run row,
accepts `pending`, `queued`, `running` or `interrupted`, and atomically:

- writes terminal `canceled`;
- retains `cancel_requested_at` for the finalization fence;
- increments `execution_generation`;
- clears owner, token, expiry, heartbeat and worker ownership;
- writes exactly one bounded `run.canceled` event.

A second cancellation returns the existing canceled row with `changed=false`.
It does not allocate an event ID, modify the timestamp/generation or write a
second event. An old worker's owner/token/generation can no longer complete the
run.

The public cancel application path authorizes the run before mutation, commits
the database cancellation first, and only then notifies the process-local Eino
ADK registry. Failure or timeout of that in-memory notification does not undo
the authoritative database terminal state.

### Cancel before execution registration

The ADK cancellation registry retains a bounded, one-minute pending request
when the database cancellation wins before the Eino execution handle is
registered. Registration consumes that request as `CancelImmediate` and
recursively cancels nested ADK work. Direct best-effort cancellation of an
unknown run still returns `ErrADKRunNotActive`; only a caller that already
committed durable cancellation may queue the pending request.

An inner execution-context cancellation is normalized to `RunCanceledError`
only when the parent worker context remains active. Parent context cancellation
continues to mean worker shutdown or lease loss and is not mislabeled as a user
cancel.

When durable cancellation clears ownership immediately before a heartbeat, the
renewal can correctly return typed `ErrRunLeaseLost`. Ordinary and resume batch
processors re-read the internal run row and suppress that lease error only when
the persisted terminal status is already `canceled`; a stale worker that lost
ownership to any other state remains an error and cannot publish or release the
new owner's lease.

### Transactional successful finalization

Ordinary and checkpoint-resume workers no longer append an assistant message,
update a title and then separately mark the run successful. They stop the
heartbeat and call `FinalizeRunSuccess`, which in one transaction:

- requires a live `running` lease with matching owner, token and generation;
- requires `cancel_requested_at IS NULL`;
- transitions the run to `succeeded` and clears active lease fields;
- inserts the assistant message;
- conditionally updates the first-turn title only when its expected old value
  still matches.

Any message or title write error rolls back the status transition. A title
changed concurrently by the user is preserved while message and success still
commit. If cancellation already won, the repository returns typed
`ErrRunCanceled`; the worker records a canceled outcome and emits neither a
title update nor `run.completed`. Terminal `run.canceled` remains unique because
only the cancellation transaction writes it.

Subagent lifecycle rows that do not own an assistant transcript continue to use
their status-only terminal transition. Their running-status compare still
prevents a success transition after durable cancellation.

## Verification

RED/GREEN coverage includes:

- pending and running cancellation;
- duplicate cancellation and event uniqueness;
- duplicate cancellation while the ID generator is unavailable;
- mismatched cancellation-event thread rejection with full rollback;
- active ADK cancellation and cancel-before-register delivery;
- user cancel versus worker shutdown error normalization;
- heartbeat lease loss caused by durable cancellation for ordinary and resume
  workers;
- cancel winning immediately before ordinary and resume success finalization;
- assistant-message write rollback;
- title compare-and-set preservation;
- old generation/lease rejection.

Commands completed successfully:

```bash
go test ./application/agentthread -count=1
go test ./domain/agentthread/repository ./domain/agentthread/service \
  ./application/workbench -count=1
go test -race ./application/agentthread -run \
  'Test(ADKCancelRegistry|ApplicationCancelRun|RunProcessorTreatsLateSuccess|ResumeRunProcessorTreatsLateSuccess|RunProcessorTreatsLeaseLossFromDurableCancellation|ResumeRunProcessorTreatsLeaseLossFromDurableCancellation|NormalizeADKExecutionError)' -count=1
go vet ./application/agentthread ./application/workbench \
  ./domain/agentthread/repository ./domain/agentthread/service
go test -p 1 -gcflags="all=-N -l" ./... -count=1
```

The targeted race run, static checks, formatting and diff checks passed. The
serial full-backend run was repeated after the final repository boundary change
and exited `0`.
