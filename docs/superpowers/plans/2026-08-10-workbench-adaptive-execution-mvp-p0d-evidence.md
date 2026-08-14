# Workbench Adaptive Execution MVP P0D Final Race / Evidence Packet

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development`
> or `superpowers:executing-plans`. Use `superpowers:test-driven-development` for every behavior
> change and `superpowers:verification-before-completion` before every PASS claim. Do not load P1D,
> P2, ADK, UI, IDL, or migration implementation work.

**Goal:** close P0 with two consecutive machine-checked disposable-MySQL rounds proving concurrent
PlanItem mutation, lease-takeover cross-API serialization, cancel versus verified success, and
crash-after-commit replay, while keeping the adaptive repository gate private and unwired.

**Architecture:** P0D does not add another execution path. It reuses the P0A/P0B boundary and the
sole P0C `FinalizeRunSuccess`. The only production corrections allowed by this packet are the three
prefix changes proved by its saved lock-cycle REDs: gate-on finalization becomes
`Thread -> logical Journal root -> distinct Execution Run -> Attempt`; gate-off finalization becomes
`Thread -> Execution Run -> optional Attempt`; and active-source recovery becomes
`Thread -> logical root -> source Execution Run -> source Attempt`. All race orchestration remains
test-only. The final proof uses real MySQL connections, observable row-lock waits, a helper subprocess
for crash visibility, and commit-bound evidence.

This packet closes the exact Workbench API matrix named in P0C plus the newly exposed
recovery-versus-cancellation inversion. It does **not** claim a repository-wide deadlock-free order.
Section 0.4 normatively narrows and supersedes the phrase “global aggregate lock order” in the P0C
handoff: P0D PASS means the enumerated Workbench repository paths are closed; historical hard-delete,
Journal-attempt, and generic boundary combinations remain a separate cross-packet P1 and cannot be
silently admitted into P1M/P1D/P2 wiring.

---

## 0. Frozen authority and non-negotiable contracts

### 0.1 Entry lineage and docs-only handoff

- The only valid P0C implementation parent is
  `6ff0eb075fe470431e0e7b475f3068d1489bbfab` with subject
  `feat: gate adaptive verified success atomically`.
- `/private/tmp/workbench-adaptive-mvp-p0c/state.env` must contain `STATUS=PASS`, `P0C_HEAD` must
  equal that commit, and `/private/tmp/workbench-adaptive-mvp-p0c/evidence.sha256` must verify from
  its own directory before this packet is committed.
- This file is the only file in the P0D docs commit. Its path is exactly
  `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md`; its subject is
  exactly `docs: add adaptive execution P0D packet`; its parent is exactly the P0C implementation.
- The implementation base is the resulting P0D docs commit, not the P0C implementation commit.
- P0A2, P0A3, P0C, and P0D packet subjects use the dispatcher form. P0B is an explicitly
  grandfathered exception with subject `docs: add adaptive replay recovery packet`; P0D audits the
  frozen historical subject instead of rewriting lineage.

### 0.2 P0D implementation allowlist

The P0D implementation diff from its docs commit is exact-seven:

1. `backend/domain/agentthread/repository/mysql.go`
2. `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
3. `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`
4. `backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go`
5. `docs/superpowers/context/workbench-execution-chain.md`
6. `docs/superpowers/context/workbench-execution-graph.json`
7. `scripts/workbench-execution-graph/contract.mjs`

No other file may change. In particular this packet must not change `repository.go`,
`adaptive_execution.go`, `mysql_journal.go`, any migration, application/domain service,
runner/ADK, API/IDL, frontend, mode/config codec, notification production repository, or cancellation
implementation contract. If a real RED cannot be fixed inside the exact-seven without weakening an assertion,
P0D is FAIL and the worker stops for a design decision.

### 0.3 Fixture closure is test-only

Before interpreting a MySQL result, `mysql_adaptive_execution_integration_test.go` must make its
disposable schema physically capable of running the already-approved P0C finalizer:

- preserve the existing `mysqldriver.ParseDSN` gate before any DDL, require the parsed `DBName` (not an
  arbitrary DSN substring) to contain `agentthread_disposable`, and require `SELECT DATABASE()` on every
  participant/observer connection to equal that parsed database name;
- load `docker/atlas/migrations/20260614000100_agent_thread_messages.sql` before the Run migration;
- seed logical Journal root Run `30` with the same Thread/Space/Creator identity as Execution Run
  `20`; it is a production-valid top-level task root with `status=succeeded`, `parent_run_id=0`, no
  lease, required JSON columns set to canonical `{}` and `stream_mode=[]`, so it cannot satisfy the
  active-Run query;
- drop `agent_thread_messages` before `agent_runs` during cleanup;
- create test-owned `p0d_adaptive_outbox_probe` with one explicit `CREATE TABLE ... ENGINE=InnoDB`:
  primary key `idempotency_key VARCHAR(191)`, required `event_id VARCHAR(128)`, `fingerprint CHAR(64)`, canonical
  payload `LONGBLOB`, and `created_at BIGINT`; drop it in the same fixture. Its `AppendWithResult` uses
  raw MySQL `INSERT IGNORE`, requires `RowsAffected=1` for first write and `0` for replay even when the
  DSN enables `clientFoundRows`, then SELECTs and
  exact-compares all immutable columns. Its legacy `Append` callback delegates to the same probe and
  requires a first insert, so gate-off winners cannot bypass outbox verification; gate-on and crash
  retry use `AppendWithResult` and assert the returned inserted bit. `AutoMigrate` and production
  migration changes are forbidden;
- use two distinct participant MySQL connections plus a distinct observer connection and assert all
  three `CONNECTION_ID()` values differ;
- every final leaf owns a complete schema lifecycle and runs sequentially: each of the ten
  LeaseTakeover interleavings, both cancel interleavings, ConcurrentPlanItemMutation, and
  CrashAfterCommitRetry reset independently. Parent/mode nodes hold no shared mutable fixture and no
  test uses `t.Parallel`.

Missing root/message tables, unknown columns on the exercised active-rejection path, a shared
connection, or fixture panic are fixture failures, never accepted as behavior RED. The delete race must use its own
independent fixture and retain exactly one additional active top-level sentinel Run `40` whose complete
row never changes. Before the race the active set is exactly `{20,40}`; after the verified finalizer
succeeds it is exactly `{40}`. `DeleteThreadIfIdle` therefore locks Run `20` while evaluating the set,
then deterministically returns `ErrActiveRunExists` because of Run `40` and never enters an incompletely
migrated cascade path. The recovery and cancel fixtures do not contain this sentinel.

### 0.4 P0D target lock order

Gate-on `FinalizeRunSuccess` is frozen to:

```text
plain logical-root identity discovery (no row lock)
Thread
  -> logical Journal root Run
  -> distinct Execution Run (reuse root when IDs are equal)
  -> exact Attempt
  -> exact Verification tuple
  -> Decision / Evidence authority
  -> Plan scope Run / Plan / all current PlanItems(TaskID ASC)
  -> Message / optional TitleEvent / Completion / selected Checkpoint / outbox
```

The plain root discovery is used only to obtain the server-owned Thread ID. The implementation must
never lock `gate.Evidence.ThreadID`, Message.ThreadID, or another caller-selected Thread before durable
discovery. It locks the discovered Thread, then locks root/execution and revalidates the complete
root/Thread/tenant identity. The Thread row is held through title selection and terminal commit.
Tuple hit still bypasses only transient lease/status fences;
it must revalidate the same durable authority and post-image as P0C.

Gate-off may first make one ordinary, non-locking exact Run identity read to discover the authoritative
Thread ID. It then locks that Thread, performs the existing Execution Run lease-fence `UPDATE`, reloads
the locked Run and revalidates the immutable Thread identity, then reaches any optional legacy Attempt.
It must not lock a caller-selected Thread, query Decision/Evidence/Plan tables, or fabricate adaptive
Verification. This packet changes
only the row-lock acquisition prefix of the existing two finalizer branches plus the active
`RecoverySourceLease` source pair. Recovery may plain-read the immutable source Attempt identity, but
after Thread/root it must lock source Execution Run before source Attempt, revalidate both, then read the
source checkpoint. This aligns with cancellation's existing Execution Run -> Attempt order without
changing cancellation. All write contracts and `DeleteThreadIfIdle` otherwise retain established
behavior. No retry loop, advisory lock, sleep, failpoint, or ignored MySQL error is introduced. A bounded
whole-transaction retry is permitted by the dispatcher only as a fallback design after a new review;
it is not the default fix.

This section is the successor authority for interpreting the P0C handoff's overly broad “global
aggregate lock order” phrase. The exact proof is limited to `FinalizeRunSuccess` gate-on/off,
`CreateRunBundle` active-source recovery, `RequestRunCancellation`, and the active-rejection branch of
`DeleteThreadIfIdle` exercised below. It is not a claim that every historical repository method has one
global order.

Both public delete entry points share `deleteThreadCascade` when deletion is actually allowed. That
cascade is `Thread -> dependent Attempt delete -> Run delete`, while several terminal writers are
`Run -> Attempt`. Moreover, trying to repair deletion by locking all Runs in numeric order would invert
`CreateJournalAttempt` and `CommitAdaptiveExecutionBoundary`, which lock logical root before execution
Run even when their numeric IDs are reversed. P0D therefore neither calls the idle cascade nor changes
`DeleteThread`, `CreateJournalAttempt`, or generic boundary locking. Record these historical combinations
as one cross-packet P1. Because the idle cascade can race a terminal exact boundary replay, P1M/P1D/P2 wiring
remains blocked until a later reviewed packet defines and proves a repository-wide order; P0D/P0 PASS
must never be described as globally deadlock-free.

### 0.5 Final MySQL test names and outcomes

One top-level test owns the final matrix:

```text
TestAdaptiveExecutionP0DMySQL/ConcurrentPlanItemMutation
TestAdaptiveExecutionP0DMySQL/LeaseTakeover
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/competitor_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/finalizer_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/competitor_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/finalizer_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/recovery_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/cancellation_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/competitor_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/finalizer_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/competitor_first
TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/finalizer_first
TestAdaptiveExecutionP0DMySQL/CancelVsVerifiedSuccess
TestAdaptiveExecutionP0DMySQL/CancelVsVerifiedSuccess/cancel_wins
TestAdaptiveExecutionP0DMySQL/CancelVsVerifiedSuccess/success_wins
TestAdaptiveExecutionP0DMySQL/CrashAfterCommitRetry
```

The four direct children must each emit exactly one `Action=pass` in each official JSON round. The three
LeaseTakeover children, their four finalizer-mode nodes, all ten frozen lease interleavings, and the two cancel
interleavings must also each pass exactly once. The set of direct PASS children must equal the four
frozen names, not merely contain them. No target entry may emit skip/fail, no package-level fail is
allowed, and target output must contain none of the forbidden MySQL/fixture markers below.

All race calls use bounded contexts and channels; `time.Sleep` is forbidden. Public errors are checked
with the existing domain sentinels where the API exposes one. For every participant, reject MySQL 1213,
1205, 1062, context deadline/cancel, and strings containing deadlock/lock timeout/duplicate entry,
panic, unknown column, missing table, or fixture failure.

The direct-child invariants are:

- **ConcurrentPlanItemMutation:** reuse the P0A3 public boundary race. Exactly one complete adaptive
  writer wins; the loser is a typed revision/version conflict; event/checkpoint/Plan/item state contains
  no mixed post-image.
- **LeaseTakeover:** both named cross-API cases use real public repository calls, dedicated participant
  connections, a third observer connection, and channel barriers. The finalizer's first acquired
  `FOR UPDATE` row must be the Thread in both gate modes. Each recovery fixture first commits one public
  P0B boundary and uses that single authority as the legal closed endpoint `Decision == Evidence`.
  `CreateRunBundle` uses the same source Run/Attempt/checkpoint plus a non-nil, exact
  `RecoverySourceLease`. Its logical `Now` is after expiry while the finalizer's logical `Now` remains
  before expiry. In `competitor_first`, recovery creates Run `21`/Attempt `2` and reconciles source Run
  `20`; gate-on finalizer returns `ErrAdaptiveExecutionVerifiedSuccessConflict` plus `ErrRunLeaseLost`,
  while gate-off returns `ErrRunLeaseLost`. In `finalizer_first`, finalizer succeeds with
  `Replayed=false`; recovery returns `ErrJournalInvalidStateTransition`, and Run `21`/Attempt `2` do not
  exist. Every branch checks the complete Run/Attempt/event/checkpoint/message/Plan post-state.
  Every gate-on and gate-off finalizer request also freezes a real title change: current title equals
  nonempty `ExpectedThreadTitle`, a different nonempty `ThreadTitle` is requested, and TitleEvent plus
  primary/fallback checkpoint identities are valid. This makes the old gate-off branch actually acquire
  Run before its Thread title CAS. A winning finalizer must report `TitleUpdated=true` and persist exact
  title/event/selected-checkpoint/outbox rows; a losing finalizer must roll the entire title path back and
  leave the original title unchanged.
  Delete uses the exact active-set fixture from 0.3. In both directions and both gate modes, finalizer
  succeeds with `Replayed=false`, delete returns `deleted=false` plus `ErrActiveRunExists`, Run `20` is
  succeeded, only sentinel Run `40` remains active and unchanged, and the success terminal set is exact.
  The recovery-vs-cancellation case proves the source lock order too. In `recovery_first`, recovery
  acquires source Execution Run before source Attempt, creates Run `21`/Attempt `2`, and cancellation
  later returns a non-nil stable rejection containing `cannot be canceled from status failed` (the
  existing API exposes no sentinel for that branch). In `cancellation_first`,
  cancellation succeeds, recovery returns `ErrJournalInvalidStateTransition`, and Run `21`/Attempt `2`
  remain absent. The cancellation request in both directions carries a valid `run.canceled` JournalEvent
  for source Attempt `1`, so the old production path demonstrably locks Run `20` and then Attempt `1`;
  omitting that JournalEvent is an invalid fixture. Both branches reject 1213/1205/deadline and assert
  one coherent terminal source state.
- **CancelVsVerifiedSuccess:** run both deterministic interleavings, `cancel_wins` and `success_wins`.
  Exactly one terminal writer succeeds. In `cancel_wins`, cancellation succeeds and gate-on finalizer
  returns `ErrAdaptiveExecutionVerifiedSuccessConflict` plus `ErrRunCanceled`. In `success_wins`,
  finalizer succeeds with `Replayed=false`; cancellation returns a non-nil stable rejection containing
  `cannot be canceled from status succeeded` because that existing API exposes no sentinel for this
  branch. The cancellation request again carries the exact Attempt `1` JournalEvent; no new cancellation
  error is introduced. There is exactly one terminal Attempt/event outcome, and success-only
  Message/Verification/Checkpoint/outbox rows exist iff success won, while the cancellation event/outbox
  identity exists iff cancellation won.
- **CrashAfterCommitRetry:** a helper subprocess calls the public finalizer, observes a successful
  first-write result internally with `Replayed=false`, then exits with frozen code `86` before writing a
  response marker. The parent first proves all new terminal IDs/tuples and the test outbox identity are
  absent. It then sees only that exit, opens a new GORM connection and new repository instance, retries
  the exact request, receives `Replayed=true`, and proves every durable row/outbox identity is unchanged
  and unique.

### 0.6 Barrier requirements

GORM callbacks are test-only, registered on a single test connection, removed with `t.Cleanup`, and
activated by a private context marker. They may inspect the current statement/table/locking clause but
must never mutate production SQL. Each participant records the real MySQL `CONNECTION_ID()` from the
same transaction connection. A third connection must be able to read
`performance_schema.data_lock_waits`, `performance_schema.data_locks`, and
`performance_schema.threads`. A bounded, no-sleep observer loop joins requester/blocker thread IDs to
the two recorded process-list IDs and requires the requested object to equal the row frozen for that
interleaving: `agent_threads` for Thread-prefix waits, `agent_runs` for Execution-Run waits, or
`agent_run_attempts` for source-Attempt waits.
Missing performance-schema visibility is a fixture/entry failure, not a skip or weaker barrier.

The two finalizer cross-API LeaseTakeover cases run two independent fixtures:

1. **`competitor_first`:** the recovery/delete callback signals after it has acquired Thread and pauses;
   the finalizer starts. The observer must first see the finalizer connection waiting on that exact
   Thread row, blocked by the competitor connection. Only then is the competitor released. Under the old
   order finalizer already holds the target Run, so competitor's subsequent Run lock creates the real
   Thread<->Run cycle and a raw MySQL 1213 RED. Under the new order finalizer owns no Run yet, waits only
   on Thread, and both calls finish in the frozen legal serial result.
2. **`finalizer_first`:** independently record the first locking table, but pause only after finalizer has
   acquired the actually contested Execution Run `20` (gate-on's distinct Run `SELECT FOR UPDATE`, or
   gate-off's lease-fence `UPDATE`). With the new order it already holds Thread, so the observer must see
   competitor waiting on Thread before release. With the old order competitor instead acquires Thread.
   Recovery then waits on logical root `30` for gate-on and Execution Run `20` for gate-off; Delete waits
   on Execution Run `20` in both gate modes. Only after that variant-specific server wait is observed may
   the finalizer be released so its subsequent Thread request closes the real cycle and yields the
   accepted 1213 RED marker. Pausing the finalizer merely after root `30` is invalid because the Delete
   branch never contends on that terminal row and the common barrier must prove Run `20` was acquired.
3. Every callback/barrier is removed before its fixture closes. Every channel wait shares one context
   deadline and failure reports the exact stage.

Together the two modes, two APIs, and two directions reproduce the old cycles and prove both serial
directions without relying on scheduler luck. A before-query callback alone, a simple simultaneous-start
stress loop, or releasing the holder before the observer sees the server-side wait is invalid evidence.

`RecoveryAdmissionVsCancellation` uses two analogous fixtures. In `recovery_first`, pause after the first
locked source member: old order holds Attempt while new order holds Execution Run. Start cancellation and
observe its exact server wait before release. In `cancellation_first`, pause cancellation after Execution
Run, start recovery, and observe recovery waiting on that Run. Old Attempt->Run versus Run->Attempt must
produce a real 1213 RED in both directions; new Run->Attempt order must serialize to the exact outcomes in
0.5. The observer records whether the requested row was `agent_runs` or `agent_run_attempts` so a missing
source-order change cannot pass as generic contention.

For cancel-first/success-first, pause the chosen winner after it holds Execution Run, start the loser,
and use the observer to prove the loser connection is waiting on exact Execution Run `20` before the
winner is released. Do not use callback ordering or a merely simultaneous start as a substitute for
that server wait and the durable post-state assertions.

### 0.7 Crash helper contract

`TestAdaptiveExecutionP0DCrashHelper` is a test-binary helper, not one of the four matrix children. With
no `COZE_AGENTTHREAD_P0D_CRASH_HELPER=1` it returns normally and never skips. With the marker it:

1. opens the already-seeded disposable schema without resetting it;
2. reconstructs the frozen exact finalizer request from deterministic IDs/constants;
3. calls public `FinalizeRunSuccess` through a new repository;
4. validates `Replayed=false`, the returned Run/Message/Verification/Completion/Title/selected
   Checkpoint identities, and that its tracked outbox callback reported `inserted=true`;
5. calls `os.Exit(86)` without printing the parent's success-response marker.

The parent uses `os.Executable()` and `exec.CommandContext` with exact
`-test.run=^TestAdaptiveExecutionP0DCrashHelper$`, inherits only the necessary gated MySQL environment,
captures output, requires exit code 86, and requires the success-response marker to be absent. Before
launch it proves the new Message/Verification/Completion/selected checkpoint tuples and outbox identity
do not exist. After exit it snapshots the committed durable state, then retries through a new connection
and repository; retry must report outbox `inserted=false`, return a result deep-equal to the first frozen
IDs/post-image except `Replayed=true`, and leave the full snapshot byte/field exact with every terminal
tuple count equal to one. That snapshot includes Thread, logical root and execution Runs, Attempt, all
events, Message, all checkpoints, Plan and every PlanItem, plus the complete outbox probe row. Killing the
process before Commit, pre-seeding a committed result, exiting after
an IPC response, invoking a private transaction helper, or doing the retry in the same repository
instance is not valid evidence.

### 0.8 Deferred cross-packet P1s

A recovery Attempt whose immutable source checkpoint freezes Plan revision `N` can commit its first
Plan-bearing boundary `N -> N+1`, but a second Plan-bearing boundary in the same Attempt currently fails
`ErrAdaptiveExecutionLineageConflict` because the source still says `N`. P0D does not weaken or redesign
that durable lineage contract. P0D/P0 may pass using the already-approved closed endpoint
`Decision == Evidence`; P1D/P2 wiring is forbidden from emitting Decision followed by later Evidence in
the same recovery Attempt until a later packet designs and tests rolling exact authority. No scan-latest,
source checkpoint rewrite, source-pointer mutation, or current-Plan trust is allowed as a shortcut.

The historical lock combinations described in 0.4 remain a second cross-packet P1. A later packet must
inventory all public Run/Attempt/delete transactions, define one repository-wide order, and save real
MySQL RED/GREEN before claiming global closure. P1M/P1D/P2 production wiring remains blocked until that
packet passes; P0D proves only the currently unwired repository primitive and named API matrix.

### 0.9 Three-bucket P0 aggregate audit

The dispatcher product-code allowlist and the already-approved authority maintenance must not be mixed.
P0D uses `70c18605f3b6469968584a3289bca17ab1a34a1f` as `P0_BASE_SHA` and audits
`P0_BASE_SHA..P0_HEAD_SHA` into three exact buckets:

**Product/repository bucket (six paths):**

```text
backend/domain/agentthread/repository/adaptive_execution.go
backend/domain/agentthread/repository/mysql.go
backend/domain/agentthread/repository/mysql_adaptive_execution.go
backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go
backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
backend/domain/agentthread/repository/repository.go
```

**Approved authority-maintenance bucket (five paths):**

```text
docs/superpowers/context/project-context.md
docs/superpowers/context/workbench-execution-chain.md
docs/superpowers/context/workbench-execution-graph.json
scripts/workbench-execution-graph.test.mjs
scripts/workbench-execution-graph/contract.mjs
```

**Packet-doc bucket (five paths after P0D docs commit):**

```text
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a2-sqlite-atomic.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a3-mysql-race.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0b-recovery.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md
```

Each packet-doc commit is also verified independently for exact parent, subject, and single-file diff.
This explicit split implements the P0B handoff requirement and supersedes any naive aggregate command
that filters only packet docs and then misclassifies the five authority files as product scope.

### 0.10 P0 timebox evidence

The dispatcher names a two-engineer-day hard limit, but the frozen P0A1 manifest contains no timestamp
or labor-hour field. P0D must not invent one. `timebox.txt` records that absence, uses the P0 base commit
epoch as the earliest reproducible lower-bound timestamp, records the final P0D commit epoch and use of
parallel reviewers, and computes their wall-clock difference. If that measurable interval exceeds
48 hours, or other available evidence shows more than two attributable engineer-days, set
`P0_RESULT=FAIL` unless the user explicitly extends the timebox. Otherwise state the measurable result
and the limitation of converting agent parallelism into engineer-days; do not claim unsupported labor
precision.

---

## 1. Task 1: freeze and commit this P0D packet

- [ ] **Step 1: verify P0C evidence and the only untracked file.**

  ```bash
  set -euo pipefail
  p0c=/private/tmp/workbench-adaptive-mvp-p0c
  cd /private/tmp/workbench-adaptive-mvp-p0c
  grep -Fx 'STATUS=PASS' state.env
  grep -Fx 'P0C_HEAD=6ff0eb075fe470431e0e7b475f3068d1489bbfab' state.env
  test "$(shasum -a 256 evidence.sha256 | awk '{print $1}')" = \
    d6883c88f4d25abfb9d155e2a567e37cc53c267e79fbddc99eb0910f1d03974f
  shasum -a 256 -c evidence.sha256
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git rev-parse HEAD)" = 6ff0eb075fe470431e0e7b475f3068d1489bbfab
  test "$(git status --porcelain)" = \
    '?? docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md'
  ```

- [ ] **Step 2: syntax/whitespace and independent plan review.**

  Extract every fenced `bash` block and run both shells. Because the plan is still untracked, use a
  no-index whitespace check rather than the ineffective ordinary `git diff --check`. Two read-only
  reviewers independently audit the four direct MySQL names, all gate-mode/interleaving names, fixture
  closure, helper-process ordering, exact-seven, aggregate buckets, evidence list, and every shell
  assertion. They write the two frozen reports named below. Any P0/P1 is fixed in this single file and
  both SHA-bound reviews restart.

  ```bash
  set -euo pipefail
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  plan=docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md
  fences=/private/tmp/workbench-adaptive-mvp-p0d-plan-fences.sh
  check_log=/private/tmp/workbench-adaptive-mvp-p0d-plan-diff-check.log
  syntax_log=/private/tmp/workbench-adaptive-mvp-p0d-plan-syntax.log
  reviewed=/private/tmp/workbench-adaptive-mvp-p0d-plan-reviewed.sha256
  cd "$repo"
  awk 'BEGIN { inside=0 } /^[[:space:]]*```bash[[:space:]]*$/ { inside=1; next } /^[[:space:]]*```[[:space:]]*$/ { if (inside) { print ""; inside=0 }; next } inside { sub(/^  /, ""); print }' \
    "$plan" > "$fences"
  : > "$syntax_log"
  bash -n "$fences" >> "$syntax_log" 2>&1
  zsh -n "$fences" >> "$syntax_log" 2>&1
  test ! -s "$syntax_log"
  if git diff --no-index --check /dev/null "$plan" > "$check_log" 2>&1; then
    check_status=0
  else
    check_status=$?
  fi
  test "$check_status" -eq 1
  test ! -s "$check_log"
  shasum -a 256 "$plan" > "$reviewed"
  ```

  Stop here and give the exact reviewed hash to both read-only reviewers. After both reports exist, run
  this separate validation block; if either reviewer finds P0/P1, fix the plan, rerun the first block,
  replace both reports, and validate only the new hash. `P0=0`/`P1=0` are plan-scope counts; both
  reports must separately acknowledge the two frozen cross-packet P1s and blocked wiring.

  ```bash
  set -euo pipefail
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  plan=docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md
  reviewed=/private/tmp/workbench-adaptive-mvp-p0d-plan-reviewed.sha256
  review_manifest=/private/tmp/workbench-adaptive-mvp-p0d-plan-review-reports.sha256
  cd "$repo"
  shasum -a 256 -c "$reviewed"
  reviewed_hash="$(awk '{print $1}' "$reviewed")"
  reports=(
    /private/tmp/workbench-adaptive-mvp-p0d-plan-spec-review.txt
    /private/tmp/workbench-adaptive-mvp-p0d-plan-quality-review.txt
  )
  for report in "${reports[@]}"; do
    test -s "$report"
    grep -Fx "PLAN_SHA256=$reviewed_hash" "$report"
    grep -Fx 'P0=0' "$report"
    grep -Fx 'P1=0' "$report"
    grep -Fx 'CROSS_PACKET_P1=2' "$report"
    grep -Fx 'P1M_P1D_P2_WIRING=BLOCKED' "$report"
  done
  shasum -a 256 "${reports[@]}" > "$review_manifest"
  ```

- [ ] **Step 3: create the docs-only commit.**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  p0c_head=6ff0eb075fe470431e0e7b475f3068d1489bbfab
  plan=docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md
  reviewed=/private/tmp/workbench-adaptive-mvp-p0d-plan-reviewed.sha256
  review_manifest=/private/tmp/workbench-adaptive-mvp-p0d-plan-review-reports.sha256
  test "$(git rev-parse HEAD)" = "$p0c_head"
  shasum -a 256 -c "$reviewed"
  shasum -a 256 -c "$review_manifest"
  git add "$plan"
  test "$(git diff --cached --name-status)" = $'A\t'"$plan"
  git diff --cached --check
  reviewed_hash="$(awk '{print $1}' "$reviewed")"
  staged_hash="$(git show ":$plan" | shasum -a 256 | awk '{print $1}')"
  test "$staged_hash" = "$reviewed_hash"
  git commit -m 'docs: add adaptive execution P0D packet'
  test "$(git show -s --format=%P HEAD)" = "$p0c_head"
  test "$(git show -s --format=%s HEAD)" = 'docs: add adaptive execution P0D packet'
  test "$(git diff-tree --no-commit-id --name-status -r HEAD)" = $'A\t'"$plan"
  committed_hash="$(git show "HEAD:$plan" | shasum -a 256 | awk '{print $1}')"
  test "$committed_hash" = "$reviewed_hash"
  test -z "$(git status --porcelain)"
  ```

- [ ] **Step 4: initialize P0D evidence from the docs commit.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  test ! -e "$evidence"
  mkdir "$evidence"
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git rev-parse HEAD > "$evidence/base.sha"
  git rev-parse origin/dev > "$evidence/origin-dev.sha"
  shasum -a 256 \
    docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md \
    > "$evidence/plan.sha256"
  cp /private/tmp/workbench-adaptive-mvp-p0d-plan-reviewed.sha256 \
    "$evidence/plan-reviewed.sha256"
  shasum -a 256 -c /private/tmp/workbench-adaptive-mvp-p0d-plan-review-reports.sha256
  cp /private/tmp/workbench-adaptive-mvp-p0d-plan-spec-review.txt \
    "$evidence/plan-spec-review.txt"
  cp /private/tmp/workbench-adaptive-mvp-p0d-plan-quality-review.txt \
    "$evidence/plan-quality-review.txt"
  shasum -a 256 "$evidence/plan-spec-review.txt" "$evidence/plan-quality-review.txt" \
    > "$evidence/plan-review-reports.sha256"
  cp /private/tmp/workbench-adaptive-mvp-p0d-plan-diff-check.log \
    "$evidence/plan-diff-check.log"
  cp /private/tmp/workbench-adaptive-mvp-p0d-plan-syntax.log \
    "$evidence/plan-syntax.log"
  printf '%s\n' 'PACKET=P0D' 'STATUS=active' > "$evidence/state.env"
  ```

---

## 2. Task 2: disposable MySQL entry and fixture closure

- [ ] **Step 1: fail closed when the external gate is unavailable.**

  ```bash
  set -euo pipefail
  test -n "${COZE_AGENTTHREAD_TEST_MYSQL_DSN:-}"
  test "${COZE_AGENTTHREAD_TEST_ALLOW_DDL:-}" = I_UNDERSTAND_DISPOSABLE_DB
  case "$COZE_AGENTTHREAD_TEST_MYSQL_DSN" in
    *agentthread_disposable*) ;;
    *) exit 1 ;;
  esac
  ```

  If this fails, write `STATUS=BLOCKED` and stop without modifying implementation files. SQLite, a
  skipped test, a database with another name, or an implicit local credential is not a substitute.

- [ ] **Step 2: prove the existing disposable entry with JSON output.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  cd "$repo"
  test "$(git rev-parse HEAD)" = "$(cat "$evidence/base.sha")"
  test -z "$(git status --porcelain)"
  cd "$repo/backend"
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-entry-cache \
    go test -json -p 1 ./domain/agentthread/repository \
      -run '^TestJournalMySQLIntegrationGaplessSequenceAcrossConnections$' \
      -count=1 -timeout=60s > "$evidence/mysql-entry.json"
  jq -e -s '
    ([.[] | select(.Action == "pass" and .Test == "TestJournalMySQLIntegrationGaplessSequenceAcrossConnections")] | length) == 1 and
    ([.[] | select((.Action == "skip" or .Action == "fail") and .Test == "TestJournalMySQLIntegrationGaplessSequenceAcrossConnections")] | length) == 0 and
    ([.[] | select(.Action == "fail" and .Test == null)] | length) == 0
  ' "$evidence/mysql-entry.json"
  ```

- [ ] **Step 3: add the test-only physical closure and compile.**

  Modify only `mysql_adaptive_execution_integration_test.go` as specified in 0.3, add the P0D fixture
  constructor/probes, and do not yet change production lock order. Add the frozen fixture top-level
  `TestAdaptiveExecutionP0DMySQLFixtureClosure` with exact children `VerifiedSuccessSmoke` and
  `LockWaitObserverCapabilities`. The first calls public verified success and reads back the terminal
  post-state. The second uses two participant connections and a third observer to create, observe, and
  release a real Thread-row wait, proving the performance-schema join and connection-ID mapping before
  any race RED is interpreted.

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  cd "$repo"
  test "$(git rev-parse HEAD)" = "$(cat "$evidence/base.sha")"
  git diff --quiet -- \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go
  test "$(git diff --name-only)" = \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go
  cd "$repo/backend"
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-fixture-compile-cache \
    go test -p 1 ./domain/agentthread/repository -run '^$' -count=1 \
    2>&1 | tee "$evidence/fixture-compile.log"
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-fixture-smoke-cache \
    go test -json -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionP0DMySQLFixtureClosure$' -count=1 -timeout=60s \
      > "$evidence/fixture-smoke.json"
  jq -e -s '
    def once($name; $action):
      ([.[] | select(.Action == $action and .Test == $name)] | length) == 1;
    ([.[] | select(.Action == "pass" and
      (((.Test // "") == "TestAdaptiveExecutionP0DMySQLFixtureClosure") or
       ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQLFixtureClosure/")))) | .Test] | sort) ==
      (["TestAdaptiveExecutionP0DMySQLFixtureClosure",
        "TestAdaptiveExecutionP0DMySQLFixtureClosure/LockWaitObserverCapabilities",
        "TestAdaptiveExecutionP0DMySQLFixtureClosure/VerifiedSuccessSmoke"] | sort) and
    once("TestAdaptiveExecutionP0DMySQLFixtureClosure"; "pass") and
    once("TestAdaptiveExecutionP0DMySQLFixtureClosure/VerifiedSuccessSmoke"; "pass") and
    once("TestAdaptiveExecutionP0DMySQLFixtureClosure/LockWaitObserverCapabilities"; "pass") and
    ([.[] | select((.Action == "skip" or .Action == "fail") and
      ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQLFixtureClosure")))] | length) == 0 and
    ([.[] | select(.Action == "fail" and .Test == null)] | length) == 0 and
    ([.[] | select(.Action == "output" and
      ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQLFixtureClosure"))) | (.Output // "")]
      | join("")
      | test("panic:|unknown column|doesn.t exist|no such table|context deadline|fixture failure"; "i")
      | not)
  ' "$evidence/fixture-smoke.json"
  ```

---

## 3. Task 3: lock-cycle RED and minimal Thread-first GREEN

- [ ] **Step 1: write all final lock-order tests before production changes.**

  In `mysql_adaptive_execution_test.go`, preserve the frozen P0C top-level test name
  `TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt`,
  while replacing only its internal Run-first sqlmock expectations. Keep the public
  `FinalizeRunSuccess` test's `DistinctJournalAndExecutionRuns` and `SharedJournalAndExecutionRun`
  subtests. Both now expect one plain logical-root identity discovery, Thread `FOR UPDATE`, locked
  logical root, then only a distinct Execution lock when needed, then Attempt and exact tuple. Add
  `CallerThreadSelectorCannotPrelockUnrelatedThread`: seed the plain root discovery with durable Thread
  `10` while the otherwise internally-consistent caller selector says `11`; prove the first row lock is
  Thread `10`, never `11`, and the request fails before any write after locked authority is revalidated.
  Add
  `TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun` with two subtests.
  `AuthoritativeThreadPrecedesExecutionRun` expects one plain Run identity discovery, Thread
  `FOR UPDATE`, then the existing Run fence `UPDATE`. `CallerThreadSelectorCannotPrelockUnrelatedThread`
  returns durable Run.ThreadID `10` while every caller selector is internally consistent on `11`; it
  proves the first row lock is Thread `10`, never `11`, and the request rolls back without a durable
  write. A sentinel error after the required prefix proves no terminal write ran first. Each old-order
  assertion emits the exact `P0D_LOCK_ORDER_RED` marker used below; Query and Update statements are both
  observed.

  Add `TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt`. Through public
  `CreateRunBundle`, it expects plain source-Attempt discovery followed by source Execution Run
  `FOR UPDATE`, source Attempt `FOR UPDATE`, and the exact checkpoint. The old Attempt-first order emits
  the same frozen marker before any recovery write.

  In the integration file, add both LeaseTakeover barrier cases from 0.5/0.6 in both gate modes. Their
  observers cover GORM Query and Update callbacks: gate-on old order reports
  `agent_runs_select_for_update`, gate-off old order reports `agent_runs_update`, and new order requires
  `agent_threads_select_for_update`. The first-lock observation is retained in failure detail, but every
  integration RED emits `P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213` only after the server observer has proved
  the frozen wait and the returned error is a real MySQL 1213.

- [ ] **Step 2: save fresh RED without accepting fixture noise.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  cd "$repo"
  test "$(git rev-parse HEAD)" = "$(cat "$evidence/base.sha")"
  production_files=(
    backend/domain/agentthread/repository/mysql.go
    backend/domain/agentthread/repository/mysql_adaptive_execution.go
  )
  red_test_files=(
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
  )
  git diff --quiet -- "${production_files[@]}"
  test "$(git diff --name-only | LC_ALL=C sort)" = \
    "$(printf '%s\n' "${red_test_files[@]}" | LC_ALL=C sort)"
  : > "$evidence/lock-red-production-base.sha256"
  for production_file in "${production_files[@]}"; do
    worktree_hash="$(shasum -a 256 "$production_file" | awk '{print $1}')"
    base_hash="$(git show "HEAD:$production_file" | shasum -a 256 | awk '{print $1}')"
    test "$worktree_hash" = "$base_hash"
    printf '%s  %s\n' "$base_hash" "$production_file" \
      >> "$evidence/lock-red-production-base.sha256"
  done
  shasum -a 256 "${red_test_files[@]}" > "$evidence/lock-red-tests.sha256"
  git diff --binary -- "${red_test_files[@]}" > "$evidence/lock-red-tests.patch"
  test -s "$evidence/lock-red-tests.patch"
  git diff --name-status > "$evidence/lock-red-scope.txt"
  cd "$repo/backend"
  unit_regex='^(TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun|TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt)$'
  if GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-lock-unit-red \
    go test -json -p 1 ./domain/agentthread/repository -run "$unit_regex" -count=1 \
      > "$evidence/lock-unit-red.json"; then
    unit_status=0
  else
    unit_status=$?
  fi
  test "$unit_status" -ne 0
  jq -e -s '
    def once($name; $action):
      ([.[] | select(.Action == $action and .Test == $name)] | length) == 1;
    def marked($name):
      any(.[]; .Action == "output" and .Test == $name and
        ((.Output // "") | contains("P0D_LOCK_ORDER_RED")));
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/DistinctJournalAndExecutionRuns"; "run") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/DistinctJournalAndExecutionRuns"; "fail") and
    marked("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/DistinctJournalAndExecutionRuns") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/SharedJournalAndExecutionRun"; "run") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/SharedJournalAndExecutionRun"; "fail") and
    marked("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/SharedJournalAndExecutionRun") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/CallerThreadSelectorCannotPrelockUnrelatedThread"; "run") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/CallerThreadSelectorCannotPrelockUnrelatedThread"; "fail") and
    marked("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/CallerThreadSelectorCannotPrelockUnrelatedThread") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/AuthoritativeThreadPrecedesExecutionRun"; "run") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/AuthoritativeThreadPrecedesExecutionRun"; "fail") and
    marked("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/AuthoritativeThreadPrecedesExecutionRun") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/CallerThreadSelectorCannotPrelockUnrelatedThread"; "run") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/CallerThreadSelectorCannotPrelockUnrelatedThread"; "fail") and
    marked("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/CallerThreadSelectorCannotPrelockUnrelatedThread") and
    once("TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt"; "run") and
    once("TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt"; "fail") and
    marked("TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt") and
    ([.[] | select(.Action == "skip" and ((.Test // "") | startswith("TestThreadRepositoryFinalizeRunSuccess")))] | length) == 0 and
    ([.[] | select(.Action == "output") | (.Output // "")] | join("")
      | test("panic:|undefined:|build failed|no tests to run"; "i") | not)
  ' "$evidence/lock-unit-red.json"

  if GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-lock-mysql-red \
    go test -json -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionP0DMySQL/LeaseTakeover/(RecoveryAdmissionVsVerifiedFinalizer|RecoveryAdmissionVsCancellation|DeleteThreadIfIdleVsVerifiedFinalizer)$' \
      -count=1 -timeout=180s > "$evidence/lock-mysql-red.json"; then
    mysql_status=0
  else
    mysql_status=$?
  fi
  test "$mysql_status" -ne 0
  jq -e -s '
    def once($name; $action):
      ([.[] | select(.Action == $action and .Test == $name)] | length) == 1;
    def marked($name; $marker):
      any(.[]; .Action == "output" and .Test == $name and
        ((.Output // "") | contains($marker)));
    def red($name; $marker): once($name; "run") and once($name; "fail") and marked($name; $marker);
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/competitor_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/finalizer_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/competitor_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/finalizer_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/recovery_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/cancellation_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/competitor_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/finalizer_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/competitor_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    red("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/finalizer_first"; "P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213") and
    ([.[] | select(.Action == "skip" and ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQL/LeaseTakeover")))] | length) == 0 and
    ([.[] | select(.Action == "output") | (.Output // "")] | join("")
      | test("panic:|unknown column|doesn.t exist|no such table|context deadline|context canceled|lock wait timeout|1205|1062|duplicate entry|fixture failure|build failed|no tests to run"; "i") | not)
  ' "$evidence/lock-mysql-red.json"
  cd "$repo"
  test "$(git rev-parse HEAD)" = "$(cat "$evidence/base.sha")"
  git diff --quiet -- "${production_files[@]}"
  shasum -a 256 -c "$evidence/lock-red-tests.sha256"
  test "$(git diff --name-only | LC_ALL=C sort)" = \
    "$(printf '%s\n' "${red_test_files[@]}" | LC_ALL=C sort)"
  ```

- [ ] **Step 3: move both finalizer branches to Thread-first row locking.**

  In `finalizeAdaptiveVerifiedRunSuccess`, plain-read the exact logical-root identity only to discover
  its durable Thread ID. Lock and validate that Thread, then lock and revalidate Journal root, optionally
  distinct Execution, Attempt, and tuple. Never use Evidence/Message caller identity to select the first
  row lock.
  Preserve all P0C identity checks, replay placement, transient fences, Plan/item order, write order, and
  recognized missing-row causes.

  In gate-off `FinalizeRunSuccess`, make a plain exact Run identity read, lock that authoritative Thread,
  then perform the existing Run fence `UPDATE` and revalidate `Run.ThreadID` from the locked post-image.
  Keep nil-gate adaptive-table access at zero and preserve legacy Journal Attempt semantics. Do not edit
  cancellation, `DeleteThreadIfIdle`, or any other terminal production behavior.

  In active-lease recovery, plain-read source Attempt only to discover immutable Execution Run identity
  and match it to `RecoverySourceLease.RunID`; after the existing Thread/root prefix lock source Execution
  Run before exact source Attempt, then revalidate identity/status and read checkpoint. Do not change the
  terminal-source/no-lease branch or any recovery post-state.

- [ ] **Step 4: run lock-order unit and real-MySQL GREEN.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  cd "$repo"
  shasum -a 256 -c "$evidence/lock-red-tests.sha256"
  test "$(git diff --name-only | LC_ALL=C sort)" = "$(printf '%s\n' \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    | LC_ALL=C sort)"
  cd "$repo/backend"
  unit_regex='^(TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun|TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt)$'
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-lock-unit-green \
    go test -json -p 1 ./domain/agentthread/repository -run "$unit_regex" -count=1 \
      > "$evidence/lock-unit-green.json"
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-lock-mysql-green \
    go test -json -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionP0DMySQL/LeaseTakeover/(RecoveryAdmissionVsVerifiedFinalizer|RecoveryAdmissionVsCancellation|DeleteThreadIfIdleVsVerifiedFinalizer)$' \
      -count=1 -timeout=180s > "$evidence/lock-mysql-green.json"
  jq -e -s '
    def once($name): ([.[] | select(.Action == "pass" and .Test == $name)] | length) == 1;
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/DistinctJournalAndExecutionRuns") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/SharedJournalAndExecutionRun") and
    once("TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/CallerThreadSelectorCannotPrelockUnrelatedThread") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/AuthoritativeThreadPrecedesExecutionRun") and
    once("TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/CallerThreadSelectorCannotPrelockUnrelatedThread") and
    once("TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt") and
    ([.[] | select((.Action == "fail" or .Action == "skip") and ((.Test // "") | startswith("TestThreadRepositoryFinalizeRunSuccess")))] | length) == 0
  ' "$evidence/lock-unit-green.json"
  jq -e -s '
    def once($name): ([.[] | select(.Action == "pass" and .Test == $name)] | length) == 1;
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/competitor_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/finalizer_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/competitor_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/finalizer_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/recovery_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/cancellation_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/competitor_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/finalizer_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/competitor_first") and
    once("TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/finalizer_first") and
    ([.[] | select((.Action == "fail" or .Action == "skip") and ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQL/LeaseTakeover")))] | length) == 0 and
    ([.[] | select(.Action == "output") | (.Output // "")] | join("")
      | test("panic:|deadlock|lock wait timeout|context deadline|context canceled|1213|1205|1062|duplicate entry|unknown column|doesn.t exist|no such table|fixture failure"; "i") | not)
  ' "$evidence/lock-mysql-green.json"
  ```

---

## 4. Task 4: complete the four-child MySQL matrix

- [ ] **Step 1: finish the public integration matrix.**

  Add/refactor test helpers so the P0D root invokes only public repository methods. Reuse P0A3's
  PlanItem race assertions rather than weakening them. Build P0B Decision/Evidence and P0C finalizer
  requests with deterministic IDs. Add both cancel interleavings, tracked outbox identity, and the
  helper-process crash test. Test helpers may be shared across `_test.go` files in package `repository`;
  no production export is added.

- [ ] **Step 2: freeze the exact JSON assertion and run the complete matrix.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  printf '%s\n' \
    '. as $events |' \
    'def once($name): ([$events[] | select(.Action == "pass" and .Test == $name)] | length) == 1;' \
    'def target_passes: ([$events[] | select(.Action == "pass" and (((.Test // "") == "TestAdaptiveExecutionP0DMySQL") or ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQL/")))) | .Test] | sort);' \
    '[' \
    '  "TestAdaptiveExecutionP0DMySQL",' \
    '  "TestAdaptiveExecutionP0DMySQL/ConcurrentPlanItemMutation",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/competitor_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_on/finalizer_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/competitor_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsVerifiedFinalizer/gate_off/finalizer_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/recovery_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/RecoveryAdmissionVsCancellation/cancellation_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/competitor_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_on/finalizer_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/competitor_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/LeaseTakeover/DeleteThreadIfIdleVsVerifiedFinalizer/gate_off/finalizer_first",' \
    '  "TestAdaptiveExecutionP0DMySQL/CancelVsVerifiedSuccess",' \
    '  "TestAdaptiveExecutionP0DMySQL/CancelVsVerifiedSuccess/cancel_wins",' \
    '  "TestAdaptiveExecutionP0DMySQL/CancelVsVerifiedSuccess/success_wins",' \
    '  "TestAdaptiveExecutionP0DMySQL/CrashAfterCommitRetry"' \
    '] as $expected |' \
    '(target_passes == ($expected | sort)) and' \
    '(reduce $expected[] as $name (true; . and once($name))) and' \
    '([$events[] | select((.Action == "skip" or .Action == "fail") and (((.Test // "") == "TestAdaptiveExecutionP0DMySQL") or ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQL/"))))] | length) == 0 and' \
    '([$events[] | select(.Action == "fail" and .Test == null)] | length) == 0 and' \
    '([$events[] | select(.Action == "output" and (((.Test // "") == "TestAdaptiveExecutionP0DMySQL") or ((.Test // "") | startswith("TestAdaptiveExecutionP0DMySQL/")))) | (.Output // "")] | join("") | test("panic:|deadlock|lock wait timeout|context deadline|context canceled|1213|1205|1062|duplicate entry|unknown column|doesn.t exist|no such table|fixture failure"; "i") | not)' \
    > "$evidence/mysql-matrix-assert.jq"
  shasum -a 256 "$evidence/mysql-matrix-assert.jq" > "$evidence/mysql-matrix-assert.sha256"
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-matrix-savepoint \
    go test -json -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionP0DMySQL$' -count=1 -timeout=180s \
      > "$evidence/mysql-matrix-savepoint.json"
  jq -e -s -f "$evidence/mysql-matrix-assert.jq" "$evidence/mysql-matrix-savepoint.json"
  ```

  Existing P0C behavior may make the non-lock children pass without a new RED. Do not manufacture one by
  deleting code. The saved lock-cycle/source-order RED is the only production behavior RED authorized by
  this packet. A fixture/orchestration failure may be fixed in tests; any other public behavior failure
  makes P0D FAIL and stops for a new design review.

- [ ] **Step 3: preserve the frozen production scope.**

  Production changes are limited to Task 3's two finalizer prefixes and active recovery source lock
  order in `mysql.go`/`mysql_adaptive_execution.go`. Fix only fixture/orchestration defects in tests.
  Never convert 1213/1205 into success, skip a child, lower concurrency, remove post-state assertions,
  or use SQLite as final evidence. Any other required production fix is P0D FAIL even when its file is in
  the exact-seven.

- [ ] **Step 4: run one pre-review MySQL GREEN.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-matrix-green \
    go test -json -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionP0DMySQL$' -count=1 -timeout=180s \
      > "$evidence/mysql-matrix-green.json"
  jq -e -s -f "$evidence/mysql-matrix-assert.jq" "$evidence/mysql-matrix-green.json"
  ```

---

## 5. Task 5: authority chain and graph

- [ ] **Step 1: update only the current authority facts.**

  Update the chain and only the existing unwired exclusion object in graph JSON. State precisely:

  - P0D proves real MySQL races and crash replay;
  - adaptive gate-on order is Thread -> root -> distinct execution -> Attempt;
  - gate-off row locks are Thread -> execution -> optional Attempt while its durable behavior and
    production callers remain unchanged;
  - active recovery locks source execution before source Attempt;
  - hard `DeleteThread` and `DeleteThreadIfIdle` idle cascade versus historical Journal/boundary/replay
    transactions remain an explicit cross-packet P1 outside the named P0D target set;
  - no production edge constructs `AdaptiveGate`;
  - P1D/P2 remain blocked by same-recovery-Attempt progression; P1M/P1D/P2 are all blocked by the
    repository-wide delete/Journal/boundary lock-order P1 even after P0D's named matrix passes.

  Do not add a production node/edge or change exclusion status away from `implemented_not_wired`.

- [ ] **Step 2: capture contract digest RED, update only the digest, then run graph gates.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git rev-parse origin/dev)" = "$(cat "$evidence/origin-dev.sha")"
  if node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    > "$evidence/graph-digest-red.log" 2>&1; then
    digest_status=0
  else
    digest_status=$?
  fi
  test "$digest_status" -ne 0
  test "$(rg -c '^- ' "$evidence/graph-digest-red.log" || true)" = 1
  test "$(rg -c '^- canonical_profile_structure_mismatch: expected [0-9a-f]{64} actual [0-9a-f]{64}$' \
    "$evidence/graph-digest-red.log" || true)" = 1
  if rg -n 'panic:|build failed|ENOENT|SyntaxError' "$evidence/graph-digest-red.log"; then
    exit 1
  else
    graph_noise_status=$?
  fi
  test "$graph_noise_status" -eq 1
  node -e '
    const fs = require("node:fs");
    const crypto = require("node:crypto");
    const c = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
    const stable = v => Array.isArray(v) ? v.map(stable) :
      (v && typeof v === "object" ? Object.fromEntries(Object.keys(v).sort().map(k => [k, stable(v[k])])) : v);
    const byID = (a, b) => String(a?.id ?? "").trim().localeCompare(String(b?.id ?? "").trim());
    const projection = {
      schema_version: c.schema_version,
      profile: c.profile,
      authority_rule: c.authority?.rule,
      scope: c.scope,
      nodes: [...(c.nodes ?? [])].sort(byID),
      edges: [...(c.edges ?? [])].sort(byID),
      chains: [...(c.chains ?? [])].sort(byID),
      exclusions: [...(c.exclusions ?? [])].sort(byID),
      required_queries: [...(c.required_queries ?? [])].sort(byID),
    };
    process.stdout.write(crypto.createHash("sha256").update(JSON.stringify(stable(projection))).digest("hex") + "\n");
  ' docs/superpowers/context/workbench-execution-graph.json \
    > "$evidence/graph-digest-actual.txt"
  red_actual="$(sed -nE 's/.* actual ([0-9a-f]{64}).*/\1/p' "$evidence/graph-digest-red.log" | tail -n 1)"
  test "$(wc -l < "$evidence/graph-digest-actual.txt" | tr -d ' ')" = 1
  grep -Fx "$red_actual" "$evidence/graph-digest-actual.txt"
  ```

  Update only the digest constant in `contract.mjs` to `graph-digest-actual.txt`, and then run strictly
  serially:

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git rev-parse origin/dev)" = "$(cat "$evidence/origin-dev.sha")"
  node --test scripts/workbench-execution-graph.test.mjs \
    2>&1 | tee "$evidence/graph-contract.log"
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    2>&1 | tee "$evidence/graph-verify.log"
  node scripts/workbench-execution-graph.mjs build \
    2>&1 | tee "$evidence/graph-build.log"
  node scripts/workbench-execution-graph.mjs verify-derived \
    2>&1 | tee "$evidence/graph-derived.log"
  grep -Fx '# tests 53' "$evidence/graph-contract.log"
  grep -Fx '# pass 53' "$evidence/graph-contract.log"
  grep -Fx '# fail 0' "$evidence/graph-contract.log"
  grep -Fx '# skipped 0' "$evidence/graph-contract.log"
  grep -Fx 'Workbench execution graph verification passed (115 nodes, 132 edges, 27 chains).' \
    "$evidence/graph-verify.log"
  grep -Fx 'Workbench derived execution graph verification passed.' "$evidence/graph-derived.log"
  ```

  If and only if sandbox build returns `graphify_ast_empty`, stop all graph commands and rerun that same
  build once in a single escalated session, overwrite the failed build log with the successful output,
  then run fresh `verify-derived`. Do not create a substitute graph.

---

## 6. Task 6: focused regressions, reviews, and implementation commit

- [ ] **Step 1: fresh non-MySQL regressions.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-package-cache \
    go test -p 1 ./domain/agentthread/repository -count=1 -timeout=240s \
    2>&1 | tee "$evidence/package-green.log"
  GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-compile-cache \
    go test -p 1 ./domain/agentthread/repository -run '^$' -count=1 \
    2>&1 | tee "$evidence/compile.log"

  lock_order_regex='^(TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun|TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt)$'
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-lock-unit-final-cache \
    go test -p 1 ./domain/agentthread/repository -run "$lock_order_regex" -count=1 -v \
      2>&1 | tee "$evidence/lock-unit-final.log"
  lock_order_tests=(
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/DistinctJournalAndExecutionRuns
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/SharedJournalAndExecutionRun
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/CallerThreadSelectorCannotPrelockUnrelatedThread
    TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun
    TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/AuthoritativeThreadPrecedesExecutionRun
    TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun/CallerThreadSelectorCannotPrelockUnrelatedThread
    TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt
  )
  for test_name in "${lock_order_tests[@]}"; do
    test "$(rg -c -F -- "--- PASS: ${test_name} (" "$evidence/lock-unit-final.log" || true)" = 1
  done

  p0a_regex='^(TestValidateAdaptiveExecutionBoundaryRequest|TestValidateAdaptiveExecutionMutationRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt|TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically|TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops|TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage|TestAdaptiveExecutionBoundaryRejectsStalePlanRevision|TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion|TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict)$'
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-p0a-cache \
    go test -p 1 ./domain/agentthread/repository -run "$p0a_regex" -count=1 -timeout=180s -v \
      2>&1 | tee "$evidence/p0a-focused.log"
  p0a_tests=(
    TestValidateAdaptiveExecutionBoundaryRequest
    TestValidateAdaptiveExecutionMutationRequest
    TestLockAdaptiveExecutionRun
    TestLockAdaptiveExecutionAttempt
    TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON
    TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically
    TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops
    TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage
    TestAdaptiveExecutionBoundaryRejectsStalePlanRevision
    TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion
    TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict
  )
  for test_name in "${p0a_tests[@]}"; do
    test "$(rg -c -F -- "--- PASS: ${test_name} (" "$evidence/p0a-focused.log" || true)" = 1
  done

  p0c_regex='^(TestValidateAdaptiveVerifiedSuccessGate|TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites|TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables|TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites|TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome|TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites)$'
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-p0c-cache \
    go test -p 1 ./domain/agentthread/repository -run "$p0c_regex" -count=1 -timeout=180s -v \
      2>&1 | tee "$evidence/p0c-exact10.log"
  p0c_tests=(
    TestValidateAdaptiveVerifiedSuccessGate
    TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites
    TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables
    TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt
    TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites
    TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome
    TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites
  )
  for test_name in "${p0c_tests[@]}"; do
    test "$(rg -c -F -- "--- PASS: ${test_name} (" "$evidence/p0c-exact10.log" || true)" = 1
  done
  test "$(rg -c -F -- '--- PASS: TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites/' \
    "$evidence/p0c-exact10.log" || true)" = 39

  p0b_regex='^(TestAdaptiveExecutionBoundary(ReplaysLostResponseWithoutWrites|RejectsReplayPayloadAndMutationDrift|DuplicateDecisionConflicts|DuplicateVerificationReplaysAfterRepositoryReload|ReplaySurvivesDegradedProjection|ChecksReplayTupleAfterRunAndAttemptLocks|LocksSourceRunBeforeSourceAttempt)|TestAdaptiveExecutionRecoveryRead(UsesExactSourceCheckpoint|RecoversCanonicalPlanScopeAcrossMultipleHops|RejectsAuthorityDriftWithoutWrites|MatchesCompatibilityReaders|SelectsSafeTransactionOptions))$'
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-p0b-cache \
    go test -p 1 ./domain/agentthread/repository -run "$p0b_regex" -count=1 -timeout=180s -v \
      2>&1 | tee "$evidence/p0b-focused.log"
  p0b_tests=(
    TestAdaptiveExecutionBoundaryReplaysLostResponseWithoutWrites
    TestAdaptiveExecutionBoundaryRejectsReplayPayloadAndMutationDrift
    TestAdaptiveExecutionBoundaryDuplicateDecisionConflicts
    TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload
    TestAdaptiveExecutionBoundaryReplaySurvivesDegradedProjection
    TestAdaptiveExecutionBoundaryChecksReplayTupleAfterRunAndAttemptLocks
    TestAdaptiveExecutionBoundaryLocksSourceRunBeforeSourceAttempt
    TestAdaptiveExecutionRecoveryReadUsesExactSourceCheckpoint
    TestAdaptiveExecutionRecoveryReadRecoversCanonicalPlanScopeAcrossMultipleHops
    TestAdaptiveExecutionRecoveryReadRejectsAuthorityDriftWithoutWrites
    TestAdaptiveExecutionRecoveryReadMatchesCompatibilityReaders
    TestAdaptiveExecutionRecoveryReadSelectsSafeTransactionOptions
  )
  for test_name in "${p0b_tests[@]}"; do
    test "$(rg -c -F -- "--- PASS: ${test_name} (" "$evidence/p0b-focused.log" || true)" = 1
  done

  recovery_bundle_regex='^(TestJournalRecoveryRunBundle(CommitsRunAndAttemptAndReplays|RejectsDifferentKeyWithoutOrphans|ConcurrentKeysAdmitOneAttempt|RollsBackRunWhenAttemptInsertFails)|TestJournalLeaseRecoveryBundle(AtomicallyTerminatesExpiredSource|RollsBackSourceWhenNewAttemptFails|RollsBackSourceWhenNewRunFails))$'
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-recovery-bundle-cache \
    go test -p 1 ./domain/agentthread/repository -run "$recovery_bundle_regex" -count=1 -timeout=180s -v \
      2>&1 | tee "$evidence/recovery-bundle-focused.log"
  recovery_bundle_tests=(
    TestJournalRecoveryRunBundleCommitsRunAndAttemptAndReplays
    TestJournalRecoveryRunBundleRejectsDifferentKeyWithoutOrphans
    TestJournalRecoveryRunBundleConcurrentKeysAdmitOneAttempt
    TestJournalRecoveryRunBundleRollsBackRunWhenAttemptInsertFails
    TestJournalLeaseRecoveryBundleAtomicallyTerminatesExpiredSource
    TestJournalLeaseRecoveryBundleRollsBackSourceWhenNewAttemptFails
    TestJournalLeaseRecoveryBundleRollsBackSourceWhenNewRunFails
  )
  for test_name in "${recovery_bundle_tests[@]}"; do
    test "$(rg -c -F -- "--- PASS: ${test_name} (" "$evidence/recovery-bundle-focused.log" || true)" = 1
  done

  legacy_regex='^(TestThreadRepositoryFinalizeRunSuccessCommitsMessageTitleAndStatusTogether|TestThreadRepositoryFinalizeRunSuccessCommitsTerminalCheckpointTogether|TestThreadRepositoryFinalizeRunSuccessRollsBackWhenTerminalCheckpointFails|TestThreadRepositoryFinalizeRunSuccessPreservesConcurrentlyChangedTitle|TestThreadRepositoryFinalizeRunSuccessRejectsCancelAndRollsBackWriteFailure|TestThreadRepositoryRequestRunCancellationInvalidatesGenerationAndWritesOneEvent|TestThreadRepositoryRequestRunCancellationCancelsPendingRun|TestThreadRepositoryRequestRunCancellationRejectsMismatchedEventThread|TestThreadRepositoryRequestRunCancellationAppendsOutboxOnlyOnceOnReplay|TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload)$'
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    GOCACHE=/private/tmp/workbench-adaptive-mvp-p0d-legacy-cache \
    go test -p 1 ./domain/agentthread/repository -run "$legacy_regex" -count=1 -timeout=180s -v \
      2>&1 | tee "$evidence/legacy10.log"
  legacy_tests=(
    TestThreadRepositoryFinalizeRunSuccessCommitsMessageTitleAndStatusTogether
    TestThreadRepositoryFinalizeRunSuccessCommitsTerminalCheckpointTogether
    TestThreadRepositoryFinalizeRunSuccessRollsBackWhenTerminalCheckpointFails
    TestThreadRepositoryFinalizeRunSuccessPreservesConcurrentlyChangedTitle
    TestThreadRepositoryFinalizeRunSuccessRejectsCancelAndRollsBackWriteFailure
    TestThreadRepositoryRequestRunCancellationInvalidatesGenerationAndWritesOneEvent
    TestThreadRepositoryRequestRunCancellationCancelsPendingRun
    TestThreadRepositoryRequestRunCancellationRejectsMismatchedEventThread
    TestThreadRepositoryRequestRunCancellationAppendsOutboxOnlyOnceOnReplay
    TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload
  )
  for test_name in "${legacy_tests[@]}"; do
    test "$(rg -c -F -- "--- PASS: ${test_name} (" "$evidence/legacy10.log" || true)" = 1
  done
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed|no tests to run' \
    "$evidence/p0a-focused.log" "$evidence/p0c-exact10.log" "$evidence/p0b-focused.log" \
    "$evidence/recovery-bundle-focused.log" "$evidence/legacy10.log" \
    "$evidence/lock-unit-final.log"; then
    exit 1
  else
    regression_noise_status=$?
  fi
  test "$regression_noise_status" -eq 1
  ```

- [ ] **Step 2: format, exact-seven, wiring, and finalizer gates.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  gofmt -d \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    > "$evidence/gofmt.log"
  test ! -s "$evidence/gofmt.log"
  git diff --check
  test -z "$(git diff --cached --name-only)"
  test -z "$(git diff --name-only --diff-filter=D)"
  test "$(git diff --name-only | LC_ALL=C sort)" = "$(printf '%s\n' \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs | LC_ALL=C sort)"
  base="$(cat "$evidence/base.sha")"
  test "$(git diff --numstat "$base" -- scripts/workbench-execution-graph/contract.mjs)" = \
    $'1\t1\tscripts/workbench-execution-graph/contract.mjs'
  git show "$base:docs/superpowers/context/workbench-execution-graph.json" \
    > "$evidence/graph-base.json"
  node -e '
    const assert = require("node:assert/strict");
    const fs = require("node:fs");
    const before = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
    const after = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
    const id = "exclude.unwired_adaptive_boundary";
    const pick = value => value.exclusions.find(item => item.id === id);
    const oldTarget = pick(before);
    const newTarget = pick(after);
    assert.ok(oldTarget && newTarget);
    const without = value => ({...value, exclusions: value.exclusions.filter(item => item.id !== id)});
    assert.deepEqual(without(after), without(before));
    for (const key of new Set([...Object.keys(oldTarget), ...Object.keys(newTarget)])) {
      if (!["term", "reason", "evidence"].includes(key)) assert.deepEqual(newTarget[key], oldTarget[key]);
    }
    assert.equal(newTarget.status, oldTarget.status);
    process.stdout.write("target-exclusion-only PASS\n");
  ' "$evidence/graph-base.json" docs/superpowers/context/workbench-execution-graph.json \
    > "$evidence/graph-scope-check.log"
  grep -Fx 'target-exclusion-only PASS' "$evidence/graph-scope-check.log"
  if rg -n 'AdaptiveGate|AdaptiveVerifiedSuccessGate' backend/application backend/api frontend; then
    exit 1
  else
    wiring_status=$?
  fi
  test "$wiring_status" -eq 1
  if rg -n '\.CommitAdaptiveExecutionBoundary\(' \
    backend/domain/agentthread/repository --glob '*.go' --glob '!*_test.go'; then
    exit 1
  else
    caller_status=$?
  fi
  test "$caller_status" -eq 1
  git diff -U0 -- \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    > "$evidence/test-added-lines.diff"
  if rg -n '^\+[^+].*(AutoMigrate|time\.Sleep|t\.Parallel)' \
    "$evidence/test-added-lines.diff"; then
    exit 1
  else
    forbidden_test_status=$?
  fi
  test "$forbidden_test_status" -eq 1
  p0c_head=6ff0eb075fe470431e0e7b475f3068d1489bbfab
  git grep -h -E '^func \(r \*threadRepository\) [A-Za-z0-9]*Finalize[A-Za-z0-9]*\(' \
    "$p0c_head" -- backend/domain/agentthread/repository \
    | LC_ALL=C sort > "$evidence/finalizers-base.txt"
  rg -I -N -g '*.go' '^func \(r \*threadRepository\) [A-Za-z0-9]*Finalize[A-Za-z0-9]*\(' \
    backend/domain/agentthread/repository \
    | LC_ALL=C sort > "$evidence/finalizers-head.txt"
  diff -u "$evidence/finalizers-base.txt" "$evidence/finalizers-head.txt" \
    > "$evidence/finalizers.diff"
  printf '%s\n' \
    'func (r *threadRepository) FinalizeJournalAttempt(' \
    'func (r *threadRepository) FinalizeRunSuccess(' \
    > "$evidence/finalizers-expected.txt"
  diff -u "$evidence/finalizers-expected.txt" "$evidence/finalizers-head.txt" \
    > "$evidence/finalizers-exact.diff"
  ```

- [ ] **Step 3: two independent read-only reviews.**

  Before review, write `implementation-files.sha256` for the exact seven paths in 0.2. Bind reviewers
  to that manifest and all RED/GREEN/evidence hashes. One reviewer focuses on
  transaction/lock/crash correctness; one focuses on scope, aggregate lineage, machine assertions, and
  test validity. Both rerun fresh exact/package/Node verify (no graph build). Any P0/P1 causes a new
  behavioral RED, smallest fix, full revalidation, new hashes, and both reviews restart. P2 risks are
  recorded; the deferred cross-packet P1 remains explicit and does not get mislabeled P2.

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  implementation_files=(
    backend/domain/agentthread/repository/mysql.go
    backend/domain/agentthread/repository/mysql_adaptive_execution.go
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
    docs/superpowers/context/workbench-execution-chain.md
    docs/superpowers/context/workbench-execution-graph.json
    scripts/workbench-execution-graph/contract.mjs
  )
  shasum -a 256 "${implementation_files[@]}" > "$evidence/implementation-files.sha256"
  printf '%s\n' \
    base.sha \
    compile.log \
    finalizers-base.txt \
    finalizers-exact.diff \
    finalizers-expected.txt \
    finalizers-head.txt \
    finalizers.diff \
    fixture-compile.log \
    fixture-smoke.json \
    gofmt.log \
    graph-base.json \
    graph-build.log \
    graph-contract.log \
    graph-derived.log \
    graph-digest-actual.txt \
    graph-digest-red.log \
    graph-scope-check.log \
    graph-verify.log \
    implementation-files.sha256 \
    legacy10.log \
    lock-mysql-green.json \
    lock-mysql-red.json \
    lock-red-production-base.sha256 \
    lock-red-scope.txt \
    lock-red-tests.patch \
    lock-red-tests.sha256 \
    lock-unit-final.log \
    lock-unit-green.json \
    lock-unit-red.json \
    mysql-entry.json \
    mysql-matrix-assert.jq \
    mysql-matrix-assert.sha256 \
    mysql-matrix-green.json \
    mysql-matrix-savepoint.json \
    origin-dev.sha \
    p0a-focused.log \
    p0b-focused.log \
    p0c-exact10.log \
    package-green.log \
    plan-diff-check.log \
    plan-quality-review.txt \
    plan-review-reports.sha256 \
    plan-reviewed.sha256 \
    plan-spec-review.txt \
    plan-syntax.log \
    plan.sha256 \
    recovery-bundle-focused.log \
    review-evidence-files.actual \
    review-evidence-files.expected \
    review-evidence-inventory.diff \
    test-added-lines.diff \
    | LC_ALL=C sort > "$evidence/review-evidence-files.expected"
  : > "$evidence/review-evidence-files.actual"
  : > "$evidence/review-evidence-inventory.diff"
  find "$evidence" -maxdepth 1 -type f \
    ! -name state.env \
    ! -name spec-review.txt \
    ! -name quality-review.txt \
    ! -name review-evidence.sha256 \
    ! -name implementation-review-reports.sha256 \
    -exec basename {} \; | LC_ALL=C sort > "$evidence/review-evidence-files.actual"
  diff -u "$evidence/review-evidence-files.expected" "$evidence/review-evidence-files.actual" \
    > "$evidence/review-evidence-inventory.diff"
  (
    cd "$evidence"
    while IFS= read -r review_file; do
      shasum -a 256 "$review_file"
    done < review-evidence-files.expected > review-evidence.sha256
    shasum -a 256 -c review-evidence.sha256
  )
  ```

  Stop here while the two reviewers independently read the frozen manifests and write
  `spec-review.txt` and `quality-review.txt`. Each report must contain the exact implementation and
  review-evidence manifest hashes plus `P0=0` and `P1=0`, where those two counts are explicitly scoped
  to P0D's named matrix. Each also records `CROSS_PACKET_P1=2` and
  `P1M_P1D_P2_WIRING=BLOCKED`; the clean in-scope count must not erase either deferred P1. Reviewers
  write no other file in the evidence root. If either review finds an in-scope P0/P1, apply the smallest
  TDD fix, rerun all affected gates, overwrite the two manifests from the first block, and replace both
  reports; stale reports cannot match the new hashes. After both clean reports exist, run:

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  implementation_manifest_hash="$(shasum -a 256 "$evidence/implementation-files.sha256" | awk '{print $1}')"
  review_evidence_manifest_hash="$(shasum -a 256 "$evidence/review-evidence.sha256" | awk '{print $1}')"
  for review in "$evidence/spec-review.txt" "$evidence/quality-review.txt"; do
    test -s "$review"
    grep -Fx "IMPLEMENTATION_MANIFEST_SHA256=$implementation_manifest_hash" "$review"
    grep -Fx "REVIEW_EVIDENCE_MANIFEST_SHA256=$review_evidence_manifest_hash" "$review"
    grep -Fx 'P0=0' "$review"
    grep -Fx 'P1=0' "$review"
    grep -Fx 'CROSS_PACKET_P1=2' "$review"
    grep -Fx 'P1M_P1D_P2_WIRING=BLOCKED' "$review"
  done
  shasum -a 256 "$evidence/spec-review.txt" "$evidence/quality-review.txt" \
    > "$evidence/implementation-review-reports.sha256"
  shasum -a 256 -c "$evidence/implementation-files.sha256"
  (cd "$evidence" && shasum -a 256 -c review-evidence.sha256)
  ```

- [ ] **Step 4: commit exact-seven.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  base="$(cat "$evidence/base.sha")"
  test "$(git rev-parse HEAD)" = "$base"
  shasum -a 256 -c "$evidence/implementation-files.sha256"
  shasum -a 256 -c "$evidence/implementation-review-reports.sha256"
  shasum -a 256 -c "$evidence/plan-review-reports.sha256"
  shasum -a 256 -c "$evidence/plan.sha256"
  shasum -a 256 -c "$evidence/plan-reviewed.sha256"
  git add \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs
  expected="$(cut -d' ' -f3- "$evidence/implementation-files.sha256" | LC_ALL=C sort)"
  test "$(git diff --cached --name-only | LC_ALL=C sort)" = "$expected"
  git diff --cached --check
  git commit -m 'fix: close adaptive execution MySQL race gates'
  test "$(git show -s --format=%P HEAD)" = "$base"
  test "$(git show -s --format=%s HEAD)" = 'fix: close adaptive execution MySQL race gates'
  test "$(git diff-tree --no-commit-id --name-only -r HEAD | LC_ALL=C sort)" = "$expected"
  shasum -a 256 -c "$evidence/implementation-files.sha256"
  shasum -a 256 -c "$evidence/implementation-review-reports.sha256"
  test -z "$(git status --porcelain)"
  git rev-parse HEAD > "$evidence/head.sha"
  ```

---

## 7. Task 7: commit-bound two-round evidence and P0 aggregate

- [ ] **Step 1: run two consecutive official MySQL JSON rounds from the implementation commit.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  expected_head="$(cat "$evidence/head.sha")"
  cd "$repo"
  shasum -a 256 -c "$evidence/mysql-matrix-assert.sha256"
  for round in 1 2; do
    test "$(git rev-parse HEAD)" = "$expected_head"
    test -z "$(git status --porcelain)"
    shasum -a 256 -c "$evidence/implementation-files.sha256"
    cd "$repo/backend"
    GOCACHE="/private/tmp/workbench-adaptive-mvp-p0d-round-${round}-cache" \
      go test -json -p 1 ./domain/agentthread/repository \
        -run '^TestAdaptiveExecutionP0DMySQL$' -count=1 -timeout=180s \
        > "$evidence/mysql-round-${round}.json"
    jq -e -s -f "$evidence/mysql-matrix-assert.jq" "$evidence/mysql-round-${round}.json"
    cd "$repo"
    test "$(git rev-parse HEAD)" = "$expected_head"
    test -z "$(git status --porcelain)"
    shasum -a 256 -c "$evidence/implementation-files.sha256"
  done
  ```

- [ ] **Step 2: post-commit graph and clean-worktree gates.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  expected_head="$(cat "$evidence/head.sha")"
  cd "$repo"
  test "$(git rev-parse HEAD)" = "$expected_head"
  test -z "$(git status --porcelain)"
  test "$(git rev-parse origin/dev)" = "$(cat "$evidence/origin-dev.sha")"
  shasum -a 256 -c "$evidence/implementation-files.sha256"
  node --test scripts/workbench-execution-graph.test.mjs \
    2>&1 | tee "$evidence/graph-postcommit-contract.log"
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    2>&1 | tee "$evidence/graph-postcommit-verify.log"
  node scripts/workbench-execution-graph.mjs build \
    2>&1 | tee "$evidence/graph-postcommit-build.log"
  node scripts/workbench-execution-graph.mjs verify-derived \
    2>&1 | tee "$evidence/graph-postcommit-derived.log"
  grep -Fx '# tests 53' "$evidence/graph-postcommit-contract.log"
  grep -Fx '# pass 53' "$evidence/graph-postcommit-contract.log"
  grep -Fx '# fail 0' "$evidence/graph-postcommit-contract.log"
  grep -Fx '# skipped 0' "$evidence/graph-postcommit-contract.log"
  grep -Fx 'Workbench execution graph verification passed (115 nodes, 132 edges, 27 chains).' \
    "$evidence/graph-postcommit-verify.log"
  grep -Fx 'Workbench derived execution graph verification passed.' \
    "$evidence/graph-postcommit-derived.log"
  test "$(git rev-parse HEAD)" = "$expected_head"
  test -z "$(git status --porcelain)"
  shasum -a 256 -c "$evidence/implementation-files.sha256"
  ```

  Use the same single-build sandbox fallback rule from Task 5. No reviewer or later gate may run a
  second build after this post-commit build.

- [ ] **Step 3: audit packet lineage and the three aggregate buckets.**

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  p0_base=70c18605f3b6469968584a3289bca17ab1a34a1f
  p0d_base="$(cat "$evidence/base.sha")"
  p0d_head="$(cat "$evidence/head.sha")"
  cd "$repo"
  printf '%s\t%s\t%s\n' \
    ce638d88c9e79959f26051fbf4aa14bf6e6d9042 70c18605f3b6469968584a3289bca17ab1a34a1f 'feat: fence adaptive execution identity' \
    f144229c67790112da901d88a5b9e71413f7b4f4 ce638d88c9e79959f26051fbf4aa14bf6e6d9042 'docs: add adaptive execution P0A2 packet' \
    a796262aef605d5477c770b977f09ae065995ca0 f144229c67790112da901d88a5b9e71413f7b4f4 'feat: commit adaptive execution state atomically' \
    ff813f9a12242cc31227383a7b750a9bba465f09 a796262aef605d5477c770b977f09ae065995ca0 'docs: add adaptive execution P0A3 packet' \
    a1df1789b0b60c0916505711bcc1e5a1fa1410cd ff813f9a12242cc31227383a7b750a9bba465f09 'fix: unify plan mutation lock order' \
    2088e116d7094bca5e8a0d2b402052ef93e5f634 a1df1789b0b60c0916505711bcc1e5a1fa1410cd 'docs: add adaptive replay recovery packet' \
    54271fa1ae52d70e589482addbdc7f6a1b49d33b 2088e116d7094bca5e8a0d2b402052ef93e5f634 'feat: add adaptive boundary replay recovery' \
    937241b4285e4acf56a4f5b3127400133510ac42 54271fa1ae52d70e589482addbdc7f6a1b49d33b 'docs: add adaptive execution P0C packet' \
    6ff0eb075fe470431e0e7b475f3068d1489bbfab 937241b4285e4acf56a4f5b3127400133510ac42 'feat: gate adaptive verified success atomically' \
    "$p0d_base" 6ff0eb075fe470431e0e7b475f3068d1489bbfab 'docs: add adaptive execution P0D packet' \
    "$p0d_head" "$p0d_base" 'fix: close adaptive execution MySQL race gates' \
    > "$evidence/lineage-expected.tsv"
  git log --reverse --format='%H%x09%P%x09%s' "$p0_base..$p0d_head" \
    > "$evidence/lineage-actual.tsv"
  diff -u "$evidence/lineage-expected.tsv" "$evidence/lineage-actual.tsv" \
    > "$evidence/lineage.diff"

  printf '%s\t%s\n' \
    f144229c67790112da901d88a5b9e71413f7b4f4 docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a2-sqlite-atomic.md \
    ff813f9a12242cc31227383a7b750a9bba465f09 docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a3-mysql-race.md \
    2088e116d7094bca5e8a0d2b402052ef93e5f634 docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0b-recovery.md \
    937241b4285e4acf56a4f5b3127400133510ac42 docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md \
    "$p0d_base" docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md \
    > "$evidence/packet-docs.expected.tsv"
  while IFS=$'\t' read -r doc_commit doc_path; do
    test "$(git diff-tree --no-commit-id --name-status -r "$doc_commit")" = $'A\t'"$doc_path"
  done < "$evidence/packet-docs.expected.tsv"

  printf '%s\n' \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    backend/domain/agentthread/repository/repository.go \
    | LC_ALL=C sort > "$evidence/bucket-product.expected"
  printf '%s\n' \
    docs/superpowers/context/project-context.md \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph.test.mjs \
    scripts/workbench-execution-graph/contract.mjs \
    | LC_ALL=C sort > "$evidence/bucket-authority.expected"
  cut -f2 "$evidence/packet-docs.expected.tsv" | LC_ALL=C sort \
    > "$evidence/bucket-packets.expected"
  git log --format= --name-only "$p0_base..$p0d_head" | sed '/^$/d' | LC_ALL=C sort -u \
    > "$evidence/aggregate-actual.txt"
  LC_ALL=C sort "$evidence/bucket-product.expected" "$evidence/bucket-authority.expected" \
    "$evidence/bucket-packets.expected" > "$evidence/aggregate-expected.txt"
  diff -u "$evidence/aggregate-expected.txt" "$evidence/aggregate-actual.txt" \
    > "$evidence/aggregate.diff"
  if rg -n '^docker/atlas/migrations/' "$evidence/aggregate-actual.txt"; then
    exit 1
  else
    migration_scope_status=$?
  fi
  test "$migration_scope_status" -eq 1

  : > "$evidence/older-manifests.sha256"
  for packet in p0a1 p0a2 p0a3 p0b p0c; do
    older="/private/tmp/workbench-adaptive-mvp-${packet}"
    test -f "$older/evidence.sha256"
    case "$packet" in
      p0a1)
        packet_name=P0A1
        manifest_hash=95a5969e78f0ea7042d778de30b773bb98296b4895d78ffb233503a014a866f4
        base_field='BASE_SHA=70c18605f3b6469968584a3289bca17ab1a34a1f'
        head_field='HEAD_SHA=ce638d88c9e79959f26051fbf4aa14bf6e6d9042'
        ;;
      p0a2)
        packet_name=P0A2
        manifest_hash=cdb6b15e2796c4526ea523de517c3cbcd4ed38d6c36502583f94d7152a14e3dc
        base_field='BASE_SHA=f144229c67790112da901d88a5b9e71413f7b4f4'
        head_field='HEAD_SHA=a796262aef605d5477c770b977f09ae065995ca0'
        ;;
      p0a3)
        packet_name=P0A3
        manifest_hash=ea1a6e460d2662d7ecd890fbb695571d6f459b6fc154ba8bf99d84aeeb96f222
        base_field='BASE_SHA=ff813f9a12242cc31227383a7b750a9bba465f09'
        head_field='HEAD_SHA=a1df1789b0b60c0916505711bcc1e5a1fa1410cd'
        ;;
      p0b)
        packet_name=P0B
        manifest_hash=8d14b097140a9d01a567a8325b42fbfc2c9b9d7b1e48abed409b600c67b968d1
        base_field='P0B_BASE=2088e116d7094bca5e8a0d2b402052ef93e5f634'
        head_field='P0B_HEAD=54271fa1ae52d70e589482addbdc7f6a1b49d33b'
        ;;
      p0c)
        packet_name=P0C
        manifest_hash=d6883c88f4d25abfb9d155e2a567e37cc53c267e79fbddc99eb0910f1d03974f
        base_field='P0C_BASE=937241b4285e4acf56a4f5b3127400133510ac42'
        head_field='P0C_HEAD=6ff0eb075fe470431e0e7b475f3068d1489bbfab'
        ;;
    esac
    {
      actual_manifest_hash="$(shasum -a 256 "$older/evidence.sha256" | awk '{print $1}')"
      test "$actual_manifest_hash" = "$manifest_hash"
      printf 'EVIDENCE_MANIFEST_SHA256=%s\n' "$actual_manifest_hash"
      (cd "$older" && shasum -a 256 -c evidence.sha256)
      grep -Fx "PACKET=$packet_name" "$older/state.env"
      grep -Fx 'STATUS=PASS' "$older/state.env"
      grep -Fx "$base_field" "$older/state.env"
      grep -Fx "$head_field" "$older/state.env"
    } > "$evidence/${packet}-evidence-check.log"
    shasum -a 256 "$older/evidence.sha256" >> "$evidence/older-manifests.sha256"
  done
  ```

- [ ] **Step 4: freeze remaining risk and final evidence manifest.**

  `P0_RESULT=PASS` is forbidden until both official MySQL rounds, both reviews, all earlier evidence,
  exact lineage, three buckets, graph gates, checksums, and clean worktree pass. Write the final state
  **before** hashing it. The failure trap changes it to `STATUS=FAIL` if inventory or checksum validation
  fails; after the successful manifest check, perform no further write in the evidence directory.

  ```bash
  set -euo pipefail
  evidence=/private/tmp/workbench-adaptive-mvp-p0d
  repo=/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  p0_base=70c18605f3b6469968584a3289bca17ab1a34a1f
  p0d_base="$(cat "$evidence/base.sha")"
  p0d_head="$(cat "$evidence/head.sha")"
  finalized=0
  fail_state() {
    if test "$finalized" -ne 1; then
      printf '%s\n' 'PACKET=P0D' 'STATUS=FAIL' "P0D_BASE=$p0d_base" "P0D_HEAD=$p0d_head" \
        'P0_RESULT=FAIL' > "$evidence/state.env"
    fi
  }
  trap fail_state EXIT
  cd "$repo"
  test "$(git rev-parse HEAD)" = "$p0d_head"
  test -z "$(git status --porcelain)"
  test "$(git rev-parse origin/dev)" = "$(cat "$evidence/origin-dev.sha")"
  shasum -a 256 -c "$evidence/implementation-files.sha256"
  shasum -a 256 -c "$evidence/implementation-review-reports.sha256"
  shasum -a 256 -c "$evidence/plan-review-reports.sha256"
  shasum -a 256 -c "$evidence/plan.sha256"
  shasum -a 256 -c "$evidence/plan-reviewed.sha256"
  implementation_manifest_hash="$(shasum -a 256 "$evidence/implementation-files.sha256" | awk '{print $1}')"
  review_evidence_manifest_hash="$(shasum -a 256 "$evidence/review-evidence.sha256" | awk '{print $1}')"
  for review in "$evidence/spec-review.txt" "$evidence/quality-review.txt"; do
    grep -Fx "IMPLEMENTATION_MANIFEST_SHA256=$implementation_manifest_hash" "$review"
    grep -Fx "REVIEW_EVIDENCE_MANIFEST_SHA256=$review_evidence_manifest_hash" "$review"
    grep -Fx 'P0=0' "$review"
    grep -Fx 'P1=0' "$review"
    grep -Fx 'CROSS_PACKET_P1=2' "$review"
    grep -Fx 'P1M_P1D_P2_WIRING=BLOCKED' "$review"
  done
  (cd "$evidence" && shasum -a 256 -c review-evidence.sha256)

  start_epoch="$(git show -s --format=%ct "$p0_base")"
  end_epoch="$(git show -s --format=%ct "$p0d_head")"
  elapsed_seconds="$((end_epoch - start_epoch))"
  test "$elapsed_seconds" -ge 0
  test "$elapsed_seconds" -le 172800
  printf '%s\n' \
    'P0A1_TIMESTAMP_FIELD=absent' \
    "LOWER_BOUND_START_COMMIT=$p0_base" \
    "LOWER_BOUND_START_ISO=$(git show -s --format=%cI "$p0_base")" \
    "FINAL_COMMIT=$p0d_head" \
    "FINAL_COMMIT_ISO=$(git show -s --format=%cI "$p0d_head")" \
    "WALL_CLOCK_SECONDS=$elapsed_seconds" \
    'PARALLEL_REVIEWERS=2' \
    'ATTRIBUTABLE_ENGINEER_DAYS=not_recorded' \
    'RESULT=PASS_WITH_MEASUREMENT_LIMITATION' \
    > "$evidence/timebox.txt"

  printf '%s\n' \
    'Cross-packet P1: a recovery Attempt can commit one Plan-bearing boundary but cannot commit a second after the Plan revision advances; P1D/P2 remain blocked until rolling exact authority is designed and tested.' \
    'Cross-packet P1: hard DeleteThread and DeleteThreadIfIdle idle cascade can invert dependent-row/Run ordering against CreateJournalAttempt, CommitExecutionBoundary, and generic adaptive boundary/replay paths; P1M/P1D/P2 remain blocked until a repository-wide order has real MySQL proof.' \
    'P0D normatively narrows the P0C global-order phrase to the named repository matrix and must not be described as globally deadlock-free.' \
    'P0D proves repository/storage races only; the adaptive gate remains unwired and P1M/P1D/P2 retain their frozen ownership.' \
    'Gate-off FinalizeRunSuccess changes only its lock prefix; its durable/write behavior and child-run CompleteRun remain outside adaptive gate-on proof.' \
    'No historical Plan snapshot exists; later Plan/Item drift remains fail-closed.' \
    > "$evidence/remaining-risk.txt"
  printf '%s\n' \
    'PACKET=P0D' \
    'STATUS=PASS' \
    "P0_BASE_SHA=$p0_base" \
    "P0D_BASE=$p0d_base" \
    "P0D_HEAD=$p0d_head" \
    'P0D_SCOPE_RESULT=PASS' \
    'CROSS_PACKET_P1=2' \
    'P1M_P1D_P2_WIRING=BLOCKED' \
    'P0_RESULT=PASS' \
    > "$evidence/state.env"

  printf '%s\n' \
    aggregate-actual.txt \
    aggregate-expected.txt \
    aggregate.diff \
    base.sha \
    bucket-authority.expected \
    bucket-packets.expected \
    bucket-product.expected \
    compile.log \
    evidence-files.actual \
    evidence-files.expected \
    evidence-inventory.diff \
    evidence-nonfiles.actual \
    finalizers-base.txt \
    finalizers-exact.diff \
    finalizers-expected.txt \
    finalizers-head.txt \
    finalizers.diff \
    fixture-compile.log \
    fixture-smoke.json \
    gofmt.log \
    graph-base.json \
    graph-build.log \
    graph-contract.log \
    graph-derived.log \
    graph-digest-actual.txt \
    graph-digest-red.log \
    graph-postcommit-build.log \
    graph-postcommit-contract.log \
    graph-postcommit-derived.log \
    graph-postcommit-verify.log \
    graph-scope-check.log \
    graph-verify.log \
    head.sha \
    implementation-files.sha256 \
    implementation-review-reports.sha256 \
    legacy10.log \
    lineage-actual.tsv \
    lineage-expected.tsv \
    lineage.diff \
    lock-mysql-green.json \
    lock-mysql-red.json \
    lock-red-production-base.sha256 \
    lock-red-scope.txt \
    lock-red-tests.patch \
    lock-red-tests.sha256 \
    lock-unit-final.log \
    lock-unit-green.json \
    lock-unit-red.json \
    mysql-entry.json \
    mysql-matrix-assert.jq \
    mysql-matrix-assert.sha256 \
    mysql-matrix-green.json \
    mysql-matrix-savepoint.json \
    mysql-round-1.json \
    mysql-round-2.json \
    older-manifests.sha256 \
    origin-dev.sha \
    p0a-focused.log \
    p0a1-evidence-check.log \
    p0a2-evidence-check.log \
    p0a3-evidence-check.log \
    p0b-evidence-check.log \
    p0b-focused.log \
    p0c-evidence-check.log \
    p0c-exact10.log \
    package-green.log \
    packet-docs.expected.tsv \
    plan-diff-check.log \
    plan-quality-review.txt \
    plan-review-reports.sha256 \
    plan-reviewed.sha256 \
    plan-spec-review.txt \
    plan-syntax.log \
    plan.sha256 \
    quality-review.txt \
    recovery-bundle-focused.log \
    remaining-risk.txt \
    review-evidence-files.actual \
    review-evidence-files.expected \
    review-evidence-inventory.diff \
    review-evidence.sha256 \
    spec-review.txt \
    state.env \
    test-added-lines.diff \
    timebox.txt \
    | LC_ALL=C sort > "$evidence/evidence-files.expected"
  : > "$evidence/evidence-files.actual"
  : > "$evidence/evidence-inventory.diff"
  : > "$evidence/evidence-nonfiles.actual"
  find "$evidence" -mindepth 1 -maxdepth 1 ! -type f -print \
    > "$evidence/evidence-nonfiles.actual"
  test ! -s "$evidence/evidence-nonfiles.actual"
  find "$evidence" -maxdepth 1 -type f ! -name evidence.sha256 -exec basename {} \; \
    | LC_ALL=C sort > "$evidence/evidence-files.actual"
  diff -u "$evidence/evidence-files.expected" "$evidence/evidence-files.actual" \
    > "$evidence/evidence-inventory.diff"
  test ! -e "$evidence/evidence.sha256"
  (
    cd "$evidence"
    while IFS= read -r evidence_file; do
      shasum -a 256 "$evidence_file"
    done < evidence-files.expected > evidence.sha256
    shasum -a 256 -c evidence.sha256
  )
  finalized=1
  trap - EXIT
  ```

---

## 8. Exit gate and handoff

P0D/P0 PASS requires all of the following:

- disposable MySQL gate is explicit and database name contains `agentthread_disposable`;
- two consecutive JSON rounds each match the complete frozen PASS-name set: exactly four direct children,
  all three lease cases, all four gate-mode nodes, ten lease interleavings, and both cancel interleavings,
  with zero skip/fail/package fail or forbidden output;
- all three cross-API barrier groups observe the frozen row wait/order and return no
  1213/1205/deadline/raw duplicate;
- cancel and verified success have one terminal winner in both deterministic interleavings;
- helper process exits only after durable Commit and new-process/new-repository exact retry returns the
  original terminal result without repair or duplication;
- P0C/P0B/P0A and legacy repository regressions, compile, format, diff, graph contract/verify/build/
  derived all pass;
- no migration, public API, application/ADK wiring, mode/codec, second finalizer, or extra file exists;
- P0D docs and implementation form an exact linear parent/child; the implementation is exact-seven;
- P0 aggregate product, authority, and packet-doc buckets are exact and every evidence checksum passes;
- both the same-recovery-Attempt progression P1 and the repository-wide idle-delete/Journal/boundary
  lock-order P1 are explicit; the first blocks multi-boundary P1D/P2 wiring and the second blocks all
  P1M/P1D/P2 production wiring.

Only after this gate may the dispatcher generate the next real P1 packet from the committed P0D HEAD.
The next packet must not silently route adaptive production success or treat P0D repository proof as
application wiring.
