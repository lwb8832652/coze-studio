# Workbench Human Resume Journal Fail-Closed Design

## Status

Approved delivery-first safety slice for P1M-C3h1b. This document does not complete
Human Journal Attempt rollover or P1M.

## Problem

After an enrolled Eino ADK Run is interrupted for human input, the source Run is
`interrupted`, but its Journal Attempt remains active. The current Human Resume path
creates a new queued Run without creating a target Attempt. That Run cannot safely use
Journal checkpoint, side-effect, adaptive-bootstrap, or terminal-state contracts.

The durable fix needs an atomic Human Attempt rollover and a non-error `interrupted`
Attempt terminal contract. That requires persistence and public-contract work and is
outside this small slice.

## Decision

`ApplicationService.ResumeHumanInteraction` keeps its existing validation and exact
idempotent replay ordering. After an idempotency miss and before checkpoint reads or any
mutation, it asks the existing Journal attempt reader whether the source Run has an
active Attempt.

- An active Attempt returns the existing typed resume-conflict error. Canonical callers
  continue to receive HTTP 409, code `run_not_resumable`, `retryable=false`.
- `ErrJournalNotEnrolled` permits the existing non-Journal Human Resume path unchanged.
- `ErrJournalAttemptTerminal` is fail closed as a resume conflict: an enrolled source
  without the required atomic rollover cannot create a target Run.
- Dependency or repository errors propagate; they are never treated as not enrolled.
- An exact prior idempotent result still returns before the gate, including if the
  source Attempt remains active.

The gate creates no new public API, migration, IDL field, feature flag, runtime edge, or
database write. It does not disable non-Journal Human Resume.

## Tests

Application tests must prove:

1. active Journal Attempt -> conflict, no checkpoint read and no Run bundle mutation;
2. terminal/enrolled Attempt -> conflict, no mutation;
3. not enrolled -> existing Resume happy path still creates its bundle;
4. exact idempotent replay wins before the Journal gate;
5. unexpected reader error propagates with no mutation.

The existing canonical error-mapping test remains the wire-contract authority for the
409 response; no handler change is required.

## Deferred re-entry gate

C3h2 may remove this gate only in the same reviewed change that atomically terminates
the source Attempt, releases its active slot, creates the target Attempt with source
Attempt/checkpoint lineage, proves deterministic replay and concurrent single-winner
behavior, and defines the non-error `interrupted` Attempt terminal state across entity,
database CHECK, IDL/generated clients, and API projection.
