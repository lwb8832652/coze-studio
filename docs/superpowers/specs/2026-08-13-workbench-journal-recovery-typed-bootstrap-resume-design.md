# Workbench Journal Recovery Typed Bootstrap Resume Design

## Status

Proposed delivery-first design for P1M-C3h2a, based on the approved typed-source-only
approach. Implementation has not started. This slice handles only an already-enrolled
Journal recovery target whose source has a valid durable adaptive bootstrap. It does
not complete P1M.

## Problem

Journal recovery already creates a queued target Run and its active target Attempt in
one transaction. The Attempt carries the source Attempt, source runtime checkpoint and
recovery idempotency key. However, `ADKExecutor.Resume` currently enters
`buildRuntime` without committing or loading adaptive facts for the target claim.

The existing C2 bootstrap writer only accepts fresh Attempts and `fresh` admissions.
Reusing it unchanged would reject the recovery target. Reading the source facts only in
the application would also be insufficient: it could mix a source tuple with a target
fence and would not atomically prove that the target lineage still names that source.

## Delivery-first boundary

This slice does exactly the following:

- accepts an existing Journal recovery target with a complete active Attempt lineage;
- requires the immediate source Attempt to have a strict, durable C2 bootstrap;
- creates a target `typed_inheritance` admission and a new revision-1 baseline
  decision bound to the target Run, Attempt, generation and lease;
- commits or exactly replays the target bootstrap before `buildRuntime`;
- injects only the target durable facts into the current Resume context.

It does not add a legacy decoder or fallback, Human Attempt rollover, ordinary
non-Journal enrollment, migration, IDL, generated client, UI, feature gate, adaptive
producer, Plan mutation, or new public API. Human Journal Resume stays fail closed under
C3h1b. Missing real-MySQL credentials remain `NOT_VERIFIED` and do not turn this slice
into P1M PASS evidence.

## Architecture

### Resume coordinator seam

The existing `AdaptiveBootstrapCoordinator` gains a Resume-specific method. The
production coordinator implements both fresh Execute and recovery Resume; the Execute
method and its ordering remain unchanged.

`ADKExecutor.Resume` calls the Resume method after the existing input, runtime key and
source-run validation, but before `buildRuntime`, checkpoint-store construction, agent
factory construction or runner creation. A coordinator error returns immediately. A nil
result means the Run is explicitly not Journal-enrolled and preserves the existing
non-Journal Resume path.

The Resume method:

1. reads the active Attempt for the target Run;
2. treats `ErrJournalNotEnrolled` as the only no-op result;
3. requires a complete recovery tuple: source Attempt ID, source checkpoint ID and
   recovery idempotency key, plus a current target lease and generation;
4. requires the Resume input checkpoint ID to equal the target Attempt's source
   checkpoint ID and the input source Run ID to be positive and distinct from the
   target;
5. reads the target bootstrap first and returns a valid exact replay without allocating
   IDs or rereading the source;
6. on target `NotFound`, reads the source bootstrap by
   `(thread, journal root, input source Run, source Attempt)`;
7. accepts only a source admission whose source is `fresh` or `typed_inheritance`, and
   rejects source `NotFound`, corruption, conflict, gate-on facts or identity drift; it
   never reads legacy Config as a fallback;
8. creates a gate-off target `typed_inheritance` admission by copying the validated
   source capabilities and limits. The target admission binds the immediate source Run
   ID and source execution generation; its config digest and decoder version stay
   empty, as required by C1;
9. produces a new gate-off baseline decision for the target physical identity, allocates
   three candidate IDs, and invokes the existing fenced bootstrap commit.

The source decision is not reused. Decision ID, revision, Plan scope, timestamps and
all physical identity fields belong to the target Attempt.

### Repository contract

The existing `CommitAdaptiveExecutionBootstrap` and `ReadAdaptiveExecutionBootstrap`
signatures remain unchanged. The bootstrap writer accepts two closed shapes:

- `fresh` admission with a fresh Attempt and all three Attempt lineage fields nil;
- `typed_inheritance` admission with all three recovery lineage fields present.

Legacy-decoder admission remains rejected in this slice.

For a typed target, the transaction retains the current lock and replay prefix:
logical Journal root, target execution Run, target Attempt, exact target event tuples,
and target control-checkpoint tuple. A complete matching target fact set returns replay
only after the loader has deep-validated the target pair, checkpoint metadata and the
metadata-to-target-Attempt lineage tuple. Exact replay then skips mutable target
lease/status checks and does not reread mutable source facts. Partial facts, target
lineage drift or any mismatched target authority conflict.

Before the first typed write, the repository then:

- validates the target Run lease, generation and cancel fence;
- validates the target Attempt active fence and complete lineage;
- discovers the source Attempt from the target's stored source Attempt ID under the same
  Journal root, requiring an older ordinal, the same Thread, and no self-reference;
- validates the stored source runtime checkpoint belongs to that source execution Run
  and is not deleted;
- locks or consistently reads and strictly validates the source bootstrap facts by
  their exact source tuple in the same transaction;
- requires the target admission's source Run/generation to equal the source bootstrap
  authority and requires target gate/capabilities/limits to equal the source admission;
- writes the two internal unsequenced events and the `workbench_control` checkpoint,
  then deep-reads the result in the same transaction.

The control-checkpoint metadata already has explicit source Attempt, source checkpoint
and recovery idempotency fields. Fresh rows keep them null; typed rows copy them from the
locked target Attempt. The reader requires metadata lineage to equal the current target
Attempt and requires admission shape to match that lineage. The target admission is an
immutable inherited snapshot, so a later target read validates its own closed metadata,
fingerprints and typed pair without depending on mutable source runtime state. Only the
first target write performs the source cross-row attestation.

This design does not add another table, checkpoint namespace, event type or digest
format.

## Error and replay priority

The stable ordering is:

1. Resume input/runtime/checkpoint validation;
2. explicit not-enrolled no-op;
3. target bootstrap deep read, immutable lineage validation and exact replay;
4. source tuple and source bootstrap validation;
5. target ID allocation and fenced commit;
6. runtime construction.

Only target `NotFound` starts source inheritance. Target/source conflicts, malformed
facts, incomplete lineage, target fence loss, repository failures, nil results and
gate-on facts fail closed. No such error is converted to not enrolled or legacy
fallback. At the repository boundary, an exact lost-response Commit retry returns the
original target facts and performs no new write even if the target lease/status later
changes; the Resume coordinator itself still begins from the currently active target
Attempt selected by the worker claim.

## Tests

Focused repository tests must prove:

1. fresh source bootstrap -> typed target commit -> strict target readback;
2. typed source bootstrap -> second typed target commit, proving multi-hop inheritance;
3. exact target replay validates stored target lineage, then precedes mutable target
   lease/status and source-fact checks and adds no rows;
4. missing/corrupt source bootstrap, partial lineage, source Run/generation drift,
   source checkpoint drift and non-older/self lineage all conflict with zero writes;
5. metadata lineage drift and admission-policy drift fail closed;
6. existing fresh bootstrap behavior and legacy-admission rejection do not regress.

Focused application tests must prove:

1. target replay returns before source read and ID allocation;
2. source facts produce the exact typed target admission and target decision identity;
3. source `NotFound` and every non-`NotFound` error do not fall back;
4. not-enrolled Resume is unchanged, while an enrolled Attempt without complete recovery
   lineage fails closed;
5. `ADKExecutor.Resume` receives target facts before factory/store creation, and any
   bootstrap error leaves both uncalled;
6. `ADKExecutor.Execute` remains unchanged.

The focused SQLite/application suites are required for this slice. The existing gated
real-MySQL two-connection bootstrap test is extended to the typed recovery target race
and remains the final concurrency gate; a skip is recorded as `NOT_VERIFIED`, not
success.

## Completion and deferred work

C3h2a is complete only when code, tests and Workbench execution authority describe the
same narrow Resume edge and the focused suites are green. It still does not satisfy the
overall P1M exit gate.

Deferred work remains: legacy source decoder/fallback, Human Attempt rollover,
ordinary non-Journal recovery enrollment, typed V2 IDL/writers, real-MySQL concurrency
evidence, and final P1M equivalence/structure gates. None may be silently folded into
this implementation package.
