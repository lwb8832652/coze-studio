# Workbench Human Resume Journal Attempt Rollover Design

**Status:** Frozen design, implementation pending
**Date:** 2026-08-13
**Delivery rule:** Close the Journal-enrolled Human Resume attempt gap only. Do not
add ordinary non-Journal enrollment, gate-on production, a legacy decoder, a new
route, or unrelated UI capability.

## 1. Goal

When an enrolled Eino ADK Run pauses for Human input, the source Run is already
`interrupted`, but its Journal Attempt intentionally remains
`running + active_slot=1`. The current resume path can safely create a non-Journal
queued Run, but it cannot create a Journal successor because the logical Journal
still has an active Attempt.

C3h2b replaces the temporary fail-closed gate with one atomic rollover that:

1. appends the accepted Human response to the active source Attempt;
2. terminalizes that source Attempt as the non-error status `interrupted`;
3. releases its active slot;
4. creates the queued resume Run, User Message, and a pending active target
   Attempt with exact source Attempt/checkpoint lineage; and
5. makes retries deterministic and concurrent requests single-winner.

The source Run remains `interrupted`. The target Run remains a top-level queued
task and is consumed by the already-delivered C3h2a `ADKExecutor.Resume` typed
bootstrap path.

## 2. Non-goals

This slice does not:

- enroll ordinary non-Journal Resume or lease-recovery targets;
- change initial Human interrupt behavior or terminalize the Attempt while the
  user is still deciding;
- add `RunStatusInterrupted` to the global terminal Attempt mapper;
- enable gate-on adaptive admission or add a legacy Config decoder;
- add a route, request field, page, feature flag, table column, or index;
- reinterpret `confirmation.resolved` as a terminal lifecycle event;
- mark P1M complete or treat a skipped MySQL gate as PASS.

## 3. Attempt status contract

Add the persisted/public status `interrupted` to `RunAttemptStatus` and
`JournalExecutionStatus`.

| Property | `interrupted` contract |
| --- | --- |
| Active | no |
| Terminal | yes |
| Error/failure | no |
| `active_slot` | `NULL` |
| `started_at` | required |
| `ended_at` | required |
| `terminal_event_id` | required |
| UI label | neutral `已中断` |

`RunAttemptStatus.IsTerminal()` includes `interrupted`; `IsActive()` does not.
The existing aliases and the six prior values remain unchanged.

The following global mappings deliberately remain unchanged:

- `terminalJournalAttemptStatus(RunStatusInterrupted, ...)` still returns no
  terminal status;
- `journalAttemptStatusForRecoveryReplay(RunStatusInterrupted)` still returns
  `running` for the pre-rollover interrupted source Run.

Only the Human Resume rollover transaction may change an active source Attempt
to `interrupted`. This prevents the original Human interrupt event from closing
the Attempt before the response is accepted.

`IsTerminal()` remains the persisted lifecycle/SSE predicate and therefore
includes `interrupted`. It is not an admission predicate for arbitrary terminal
mutations. Add one named closed predicate for legacy-finalizable Attempt statuses
whose exact members are `completed`, `failed`, `cancelled`, and `timed_out`.
Both the public service and repository implementations of
`FinalizeJournalAttempt` use that predicate and reject `interrupted`; only the
transaction-internal Human rollover helper accepts it. Internal generic terminal
projectors continue receiving only the legacy-finalizable values.

Generic Journal recovery must not select an `interrupted` Attempt as a recovery
source. `selectJournalRecoverySourceAttempt` must stop using `IsTerminal()` as
its admission predicate and instead use the exact closed source allowlist
`completed`, `failed`, `cancelled`, and `timed_out`. Both the requested-Attempt
path and the implicit latest-source scan use that predicate. After taking the
source locks, the repository first-write validator repeats the same exact
allowlist as defense in depth; an explicitly requested or implicitly discovered
`interrupted` Attempt is rejected at both layers. Human successor lineage is
consumed only by the Human rollover and C3h2a Resume bootstrap contracts.

## 4. Persistence and wire compatibility

### 4.1 Forward migration

Create a new forward Atlas migration. Do not edit
`20260730000100_agent_run_attempts.sql`.

The migration drops and recreates all three affected named CHECK constraints in
one `ALTER TABLE agent_run_attempts` statement:

- `chk_agent_run_attempts_status` adds `interrupted`;
- `chk_agent_run_attempts_active_slot` treats `interrupted` as terminal and
  requires `active_slot IS NULL`;
- `chk_agent_run_attempts_lifecycle` requires `started_at`, `ended_at`, and
  `terminal_event_id` for `interrupted`.

No column or index changes are needed. The existing unique indexes on
`(journal_run_id, active_slot)` and
`(journal_run_id, recovery_idempotency_key)` remain the concurrency authority.
Regenerate `docker/atlas/migrations/atlas.sum`. The historical Journal migrations
do not project `agent_run_attempts` into `opencoze_latest_schema.hcl`, so this
slice does not introduce an unrelated HCL baseline rewrite.

### 4.2 IDL and versions

Append `JournalExecutionStatus_Interrupted = "interrupted"` to
`idl/workbench/journal.thrift` and regenerate the Go and TypeScript clients.

Keep:

- `JOURNAL_SCHEMA_VERSION = "1.1"`;
- `JOURNAL_PAYLOAD_VERSION = "1.0"`;
- `JOURNAL_PROTOCOL_VERSION = "1.1"`.

This is an additive literal in the existing string status contract. It changes
neither field IDs nor event, payload, or SSE frame shapes. Bumping the protocol
would instead require an unrelated negotiation path.

The rollout is nevertheless ordered because old strict clients reject an
unknown status literal. C3h2b is delivered in two production-compatible phases,
without adding a permanent feature flag:

1. deploy the migration and compatible server readers/contracts while retaining
   the C3h1b enrolled-Human fail-closed gate, then deploy the compatible frontend
   consumers;
2. verify `new server + old frontend` with the producer gate closed,
   `old server + new frontend`, and `new server + new frontend`; only after all
   three combinations pass may the final C3h2b activation change remove the gate
   and allow the server to produce `interrupted`.

The activation change and the atomic rollover implementation are one reviewed
unit. A release process must not cherry-pick or deploy producer activation before
the compatible frontend is live. The preparatory phase may read the new literal
but cannot persist or stream it from the Human Resume path.

After producer activation has persisted the first `interrupted` Attempt, rollback
has a compatibility floor: the server may roll back only to the phase-1 build
that understands `interrupted` and restores the C3h1b fail-closed producer gate,
and the frontend may roll back only to a compatible consumer build. Rolling back
to an older server or strict client that does not recognize the literal is
forbidden. Already committed Attempts are neither deleted nor rewritten. The
release gate includes a rollback rehearsal that restores the phase-1 gate build,
reads existing `interrupted` attempts, terminates SSE correctly, and performs no
new Human rollover.

## 5. Terminal event contract

The original `run.interrupted` event represents
`confirmation.requested`. The response event
`human.interaction.resolved` represents `confirmation.resolved`. Neither is a
terminal lifecycle event and neither may become `terminal_event_id`.

The rollover allocates one additional source event ID and writes one dedicated
dual-view terminal row. The physical/base view is narrowly typed for RunEvents
consumers, while the Journal view keeps the already-frozen terminal shape:

| Field | Value |
| --- | --- |
| physical `run_id` | source execution Run ID |
| physical event type | `journal.attempt.interrupted` |
| physical payload | `{"schema":"coze.journal_attempt_interrupted.v1","status":"interrupted","resume_run_id":<id>}` |
| Journal event type | `run.lifecycle` |
| Journal status | `interrupted` |
| visibility | `user` |
| Journal payload | `{"type":"terminal","data":{"status":"interrupted"}}` |
| idempotency key | `journal:run:<source_run_id>:terminal:interrupted` |
| parent | the newly appended `confirmation.resolved` event |

Both views share the same row and ID; that ID becomes the source Attempt
`terminal_event_id`. The physical payload is entirely server-owned and contains
only the authorized target resume Run link. The Journal columns, rather than the
physical payload, bind the row to the locked logical root and source Attempt.
This follows the existing terminal dual-view storage pattern, avoids
manufacturing a second public `run.interrupted` fact, and prevents the new row
from replacing the source's latest `run.*` lifecycle summary. The physical
`journal.attempt.interrupted` view is a persistence helper, not a new public
RunEvent. The generic repository `ListRunEvents` and `ListRunEventsByCursor`
queries exclude this exact physical type, while Journal-by-Attempt queries retain
its Journal view. This storage-bound filter makes totals, cursors, and `has_more`
visible-only and prevents an empty or repeating tail page. As defense in depth,
`ProjectPublicRunEvent` also returns nil for the type. Canonical list, page,
replay-stream, and live-stream paths therefore never expose it. The user-visible
fact is the Journal
`run.lifecycle/interrupted` view and the Attempt status. No public projection
exposes the physical payload, Journal envelope, source Attempt/checkpoint
lineage, or hidden configuration. The generic runtime projector is not taught
that the original `run.interrupted` is terminal.

The transaction uses a strict internal terminal helper that reuses
`validateTerminalJournalEvent`, `journalEventToPOWithBase`, parent resolution,
sequence allocation, and Attempt CAS rules. It must not call the public
`FinalizeJournalAttempt` method, which starts a separate transaction, and it
must not use the fail-soft branch of `persistTerminalRunEventWithJournal`, which
can degrade projection while still terminalizing the Attempt.

## 6. Closed service and repository request

Extend the existing enrollment option with a Human-only mutually exclusive
variant:

```go
type JournalHumanResumeEnrollmentOptions struct {
    JournalRunID       int64
    SourceRunID        int64
    SourceAttemptID    string
    SourceCheckpointID int64
    IdempotencyKey     string
}
```

`JournalEnrollmentOptions.HumanResume` and `.Recovery` cannot both be set.
Human Resume requires `EnrollJournal=true`, a queued top-level task Run, a
pending target Attempt, one User Message, one
`human.interaction.resolved` base event, and a healthy
`confirmation.resolved` Journal projection. The Run idempotency key and Human
re-entry key must be identical.

The repository receives an explicit closed mode rather than inferring Human
semantics from `RecoveryIdempotencyKey`:

```go
type HumanResumeRolloverRequest struct {
    SourceRunID     int64
    TerminalBase    *entity.RunEvent
    TerminalJournal *entity.JournalEvent
}
```

The target Attempt remains the single persistence source for
`JournalRunID`, `SourceAttemptID`, `SourceCheckpointID`, and
`RecoveryIdempotencyKey`. The service validates the Human option and copies it
into the target Attempt and resolved event source; the option is not forwarded
as a second repository lineage authority. The service preallocates one shared
ID for `TerminalBase` and `TerminalJournal`. Both must have the exact source
Thread/Run identity; the base type/payload and Journal type/status/payload/key
must match §5, and the Journal root/Attempt fields are empty until the repository
binds them or already equal the locked identity. The repository cross-checks the
target Attempt against the locked logical root, source Attempt, source
checkpoint, `HumanResumeRollover.SourceRunID`, and
`EventJournalSourceRunID`. The base `resume_run_id` must equal the target Run;
the Journal columns must bind the locked root/source Attempt. Neither terminal
payload duplicates source Attempt/checkpoint lineage. `RecoverySourceLease` must
be nil.

The existing recovery-named column and unique index are reused as the generic
successor/re-entry key for delivery priority. No schema rename or duplicate key
is introduced.

## 7. First-write validation and lock order

Application full Human-rollover replay remains before mutable Journal state or
checkpoint reads; a Run-only lookup is candidate discovery, never replay
success.
For a new enrolled request the repository transaction uses this lock order:

1. Thread `FOR UPDATE`;
2. logical Journal root Run `FOR UPDATE`;
3. source execution Run `FOR UPDATE` (reuse the root lock when IDs match);
4. source Attempt `FOR UPDATE`;
5. source checkpoint `FOR UPDATE`;
6. existing active top-level Run admission query.

From transaction begin until the Thread `FOR UPDATE`, the Human branch performs
no plain/consistent read of any table. In particular, it skips the generic
pre-lock bundle replay. After the Thread lock it performs its first full replay
lookup, so a request that waited for a same-key winner sees the winner's commit
under MySQL repeatable-read. The transaction-exit fallback uses the same full
validator. SQL-order tests and the real MySQL race assert that the Thread lock is
the first statement after transaction begin for this branch.

Before any write, validate:

- target Run ownership matches the locked Thread and it is queued, top-level,
  task-kind, `multitask=reject`;
- logical root is a top-level task with the same Thread, Space, and creator;
- source Run matches the request and source Attempt, has the same ownership,
  is top-level task-kind, and is exactly `interrupted`;
- source Attempt matches root, source Run, Thread, and public Attempt ID; is
  exactly `running + active_slot=1`; has `started_at`; has no `ended_at` or
  `terminal_event_id`; and has healthy projection;
- source checkpoint belongs to the same Thread/source Run, is not runtime
  deleted, and has `checkpoint_ns=eino.adk` and `runtime_type=eino_adk`;
- the source Attempt is the latest ordinal and target ordinal is exactly
  `source.ordinal + 1`;
- the resolved projection request is present, healthy, user-visible,
  `confirmation.resolved/completed`, and carries no caller-selected root or
  Attempt identity; after locking, the repository assigns the exact source
  Journal root/Attempt identity (or accepts only an already equal internal value);
- terminal event identity, schema, status, payload, visibility, deterministic
  key, and parent are exact.

Any drift is fail closed before persistence.

## 8. Atomic write order

After all locks and validation, the Human branch writes in this exact order in
the same transaction:

1. insert the target queued Run;
2. insert its User Message;
3. strictly append the physical target `human.interaction.resolved` event and
   its Journal `confirmation.resolved` view to the still-active source Attempt,
   consuming sequence `N`;
4. strictly append the dedicated source physical
   `journal.attempt.interrupted` / Journal `run.lifecycle/interrupted` dual-view row
   with the resolved row as parent, consume sequence `N+1`, and CAS the source
   Attempt from
   `running + active_slot=1 + next_sequence=N+1` to
   `interrupted + active_slot=NULL + next_sequence=N+2`, setting
   `ended_at`, `updated_at`, and `terminal_event_id`;
5. insert the target pending Attempt with `active_slot=1`, source lineage,
   ordinal `source+1`, inherited enrollment version, snapshot setting, and
   trace ID.

The target Attempt is last because inserting it before releasing the source
slot violates the unique active-slot index. The resolved event precedes the
terminal row because Journal events cannot be appended to a terminal Attempt.
Any failure rolls back the target Run/Message, both events, source sequence and
lifecycle changes, and the target Attempt.

## 9. Idempotency and concurrency

### 9.1 Exact replay

Add one internal repository/service full-aggregate query, for example
`GetHumanResumeRolloverReplay`, whose request carries expected Space/Thread/source
Run, Run operation/fingerprint, re-entry key, expected resolved Journal key
(computed from the source Run ID, interrupt ID, and existing confirmation
projection convention), whether the Message reference was requested, and the
normalized stable Human-response fields needed to derive the User Message,
Resume command, and resolved correlation. It locates the committed target by the
existing `(space_id, idempotency_key)` authority. An application-discovered
candidate target ID may be supplied only as a lookup consistency hint; a newly
allocated request target ID is never compared. This is essential because a
same-key concurrent loser allocates a different unused ID before observing the
winner. The stable response is the complete normalized
`HumanInteractionResponse` except for the server-owned `submitted_at`: schema,
interaction ID, kind, decision, answer, choice ID, comment, submitted-by, and
source are all compared. Generated `submitted_at`, target Run ID, Message ID,
and event IDs are not caller authorities: the query requires their persisted
values to be positive and internally consistent, then compares every stable
request field.
It returns the validated Run/Message/resolved/terminal/target-Attempt aggregate
or one of three outcomes: not found, `ErrJournalNotEnrolled`, or error. A Run row
alone is never a replay success condition.

The application may use `GetRunByIdempotencyKey` only to discover a candidate
target ID. Before returning it, it calls the full aggregate query. A candidate
without an Attempt returns `ErrJournalNotEnrolled` and preserves existing
non-Journal replay; an enrolled candidate must pass the complete Human aggregate
validation below. If the candidate has no target Attempt, the validator checks
the source enrollment: no source Attempt means true `ErrJournalNotEnrolled`;
any source Attempt means a partial enrolled aggregate and therefore
`ErrRunIdempotencyConflict`. The Thread-lock in-transaction replay and the transaction-exit
fallback invoke the same repository validator rather than maintaining weaker
copies. The validator reads no source checkpoint row and does not require current
checkpoint availability.

An exact enrolled retry returns the committed target bundle before checking
mutable source/target lifecycle or checkpoint deletion. It validates only
immutable creation facts:

- target Run Thread/parent/kind, ownership, and existing operation/fingerprint;
- target Run Command decodes as the canonical checkpoint Resume command; its
  source checkpoint facts and interrupt target are internally consistent, its
  embedded Human response matches every stable normalized field above, and its
  positive server `submitted_at` equals the one persisted in Run/Message/event
  metadata. This comparison therefore catches clarification answer and approved
  or rejected confirmation comment drift even when those fields do not appear
  in the resolved projection;
- target Attempt Thread, `ExecutionRunID == target Run ID`, logical root, source
  Attempt/checkpoint, re-entry key, ordinal, enrollment version, snapshot
  setting, and inherited trace identity;
- exactly one target User Message belonging to the target Run with role `user`
  and exact server-normalized content. Its `human_interaction` metadata matches
  the stable source Run, interrupt, interaction, kind, decision, optional choice,
  submitted-by/source fields and the one positive server timestamp. Answer and
  rejection-comment semantics are compared through the normalized Message
  content rather than invented metadata fields. Target Run metadata contains
  the matching stable Human fields plus the authorized Thread and checkpoint
  resume facts. The resolved base payload contains the same applicable stable
  fields plus the committed target `resume_run_id`; its Journal view must equal
  the canonical projection derived by the existing confirmation projector.
  When requested, the target Run `_message` reference points to this exact
  Message;
- source Attempt status `interrupted`, cleared active slot, ended timestamp,
  and `TerminalEventID == committed terminal row ID`;
- the resolved row's target physical Run, source Journal root/Attempt,
  deterministic existing confirmation key derived from the source Run,
  base and Journal event types, exact normalized response/correlation fields at
  their persisted locations, status/visibility, and positive sequence;
- the terminal row's source physical Run, same root/Attempt, deterministic
  terminal key, physical `journal.attempt.interrupted` payload, Journal
  `run.lifecycle/interrupted/terminal`, `ParentEventID == resolved row ID`, and
  `sequence = resolved.sequence + 1`;
- source and target Attempt ordinals satisfy `target = source + 1`.

Replay does not require the target Attempt to remain pending/active and does not
require the source checkpoint to remain undeleted. Missing or drifted aggregate
facts return `ErrRunIdempotencyConflict`; replay never repairs a partial bundle.
Freshly allocated retry event IDs are not compared with committed IDs.

### 9.2 Races

- Same key and fingerprint: one request creates; the other replays the exact
  target, with one Message, resolved row, terminal row, and target Attempt.
- Same key with changed fingerprint or lineage: one create and one deterministic
  idempotency conflict; no second mutation.
- Different keys for the same source: one rollover wins; the loser observes the
  terminal source/active successor and returns Human resume conflict with no
  orphan target data.

Add the domain sentinel `ErrHumanResumeRolloverConflict` for first-write stale
source lifecycle, different-key same-source losers, and active successor/admission
conflicts. The application maps only that sentinel and `ErrActiveRunExists` to
the existing `ErrHumanInteractionResumeConflict` (`409 run_not_resumable`).
`ErrRunIdempotencyConflict` remains the existing `409 idempotency_conflict`.
Projection, parent, sequence, checkpoint-integrity, SQL, and unknown repository
errors are not relabeled; they propagate with their original error chain.

## 10. Application re-entry

`ResumeHumanInteraction` keeps this order:

1. authorize and validate the source Run's immutable identity, Thread, Space,
   creator, and task kind; normalize the response, discard any caller-supplied
   `submitted_at`, and derive the re-entry key from stable response semantics;
2. discover an exact target Run candidate and return only after the full
   rollover aggregate query validates it;
3. on replay miss, require the source Run's current status to be exactly
   `interrupted` and read the active Journal Attempt;
4. read/decode the latest active Eino ADK checkpoint and validate the Human
   interrupt/response, then allocate the one server `submitted_at` used by the
   Resume command, Message metadata, Run metadata, and resolved event;
5. sanitize the copied target Config;
6. create the bundle.

The canonical API performs the same separation: authorization and immutable
source identity may precede the application call, but mutable
`sourceRun.Status == interrupted` cannot reject an otherwise exact committed
replay. The API's pre-application `sourceRun.Status` gate and any duplicate
Run-only idempotency lookup are removed; after authorization it delegates to the
application, which performs the one full replay decision. Mutable source status
is checked only after that replay misses.

`ErrJournalNotEnrolled` continues the existing non-Journal bundle path. An
enrolled source must return one exact running Attempt matching the source Run and
Thread, then use `JournalEnrollment.HumanResume`. A terminal/missing/drifted
enrolled Attempt remains fail closed. Projection construction for an enrolled
Human Resume is also fail closed: a projection error is returned before the
repository call, and `EventJournalProjectionFailed` remains false. The final
C3h2b activation removes the C3h1b gate only after the compatibility rollout in
§4.2; producer activation and atomic target-Attempt creation remain the same
reviewed unit, so there is no interval in which an enrolled Resume creates a
target without a target Attempt.

## 11. Public clients and presentation

Regenerate the Journal Go/TypeScript contracts and update the handwritten
Workbench Journal status union and strict adapter. Add `interrupted` to both
reducer and stream terminal sets so SSE ends rather than reconnecting forever.

The internal physical `journal.attempt.interrupted` helper is rejected by the
generic repository RunEvent list/cursor queries and again by the public RunEvent
projector. Canonical list pagination and replay/live streams therefore skip it
without exposing a payload or producing an empty/repeating cursor page.
TaskDetail receives no extra structured-event step from this row; refresh and
SSE remain observationally equivalent. Journal bootstrap and Journal SSE
continue to expose the user-visible `run.lifecycle/interrupted` view.

The Attempt selector/timeline label uses neutral text `已中断`. The
`run.lifecycle` terminal fact remains an execution-state event and does not
become a new displayable timeline/milestone/action item; therefore this slice
does not invent a separate `已中断执行` card. `journal-conversation-flow.tsx`
gives `interrupted` an explicit neutral status icon and `data-status` path rather
than falling through to an unknown/null presentation. Existing failed/timed-out
error presentation remains unchanged; `interrupted` is not added to their red
error selectors. No new control, page, interaction, or recovery action is added.

## 12. Required tests and gates

### Contract and migration

- entity active/terminal truth table;
- public service and repository `FinalizeJournalAttempt` both reject
  `interrupted`, while the Human transaction-internal helper accepts it;
- forward migration contains all three rebuilt CHECKs;
- generated Go/TS status parity and existing codegen verifier;
- strict adapter accepts `interrupted`, reducer/stream terminate, the Attempt
  selector label and conversation-flow status are neutral, and lifecycle facts
  remain non-displayable;
- public RunEvent list/page/replay/live-stream filters the physical
  `journal.attempt.interrupted` helper at the repository query and public
  projector boundaries, reports visible-only totals/cursors/`has_more`, and
  TaskDetail renders no extra step after refresh;
- generic Journal recovery rejects `interrupted` for both explicit requested-ID
  and implicit latest-source selection, and the locked repository validation
  rejects it again;
- the three rolling compatibility combinations in §4.2 pass before producer
  activation.

### Repository and service

- happy rollover durable shape and adjacent sequences;
- exact replay after the source is terminal and after target lifecycle changes;
- corrupt/missing target Message, resolved payload/correlation, terminal parent,
  terminal link, or inherited lineage conflicts before replay returns;
- changed answer, choice, approved/rejected comment, or any other stable Human
  response field conflicts by comparison with the committed Resume command;
- changed fingerprint/source Run/source Attempt/checkpoint conflicts;
- wrong source status/ownership, Attempt state/projection, missing/cross-run/
  deleted/non-ADK checkpoint, missing/failed/mismatched projections: zero writes;
- rollback at target Run, Message, resolved append, terminal append/CAS, and
  target Attempt insert restores the complete source state;
- appending another Journal event after rollover is rejected without changing
  `next_sequence` or `terminal_event_id`;
- same-key, changed-key, and different-key concurrent single-winner behavior.

### Application/API/runtime

- non-Journal Resume remains unchanged;
- enrolled Resume creates a target Attempt with exact lineage;
- exact replay remains before Attempt/checkpoint reads;
- source conflicts map to `run_not_resumable`, fingerprint drift to
  `idempotency_conflict`;
- `ADKExecutor.Resume` observes the target Attempt and C3h2a typed inherited
  facts before runtime construction;
- Journal bootstrap/SSE exposes the source terminal status and selects the
  target successor.

### External gate

Run the real MySQL dual-connection matrix against a disposable database with the
new migration and all three CHECKs. Missing
`COZE_AGENTTHREAD_TEST_MYSQL_DSN` or DDL opt-in is reported as
`NOT_VERIFIED`; the test must not be an empty PASS. Local implementation may
continue when that external environment is unavailable, but P1M exit cannot be
declared until the gate passes.

Finally update the Workbench execution authority and run graph verify, build,
and verify-derived. C3h2b completion closes only the Human Attempt rollover
blocker; P1M remains open for the explicitly listed remaining delivery slices
and external gates.
