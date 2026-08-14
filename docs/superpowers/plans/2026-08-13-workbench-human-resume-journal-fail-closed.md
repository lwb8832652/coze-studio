# Workbench Human Resume Journal Fail-Closed Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent Journal-enrolled Human Resume from creating a queued Run without an active target Attempt while preserving exact replay and non-Journal Resume.

**Architecture:** Add one application-layer admission check in `ResumeHumanInteraction` after exact idempotent replay and before checkpoint reads. Reuse `JournalRecoveryRepository.GetActiveJournalAttempt`; only `ErrJournalNotEnrolled` continues, while active/terminal enrollment returns the existing resume-conflict taxonomy and unexpected dependency errors propagate.

**Tech Stack:** Go, application service/domain repository interfaces, testify, Workbench execution authority graph.

---

### Task 1: Lock the Human Resume Journal gate with RED tests

**Files:**
- Modify: `backend/application/agentthread/service_test.go`

- [x] **Step 1: Add a focused repository stub**

Add a test-only stub that embeds the existing interface so only the used method needs an implementation:

```go
type humanResumeJournalRepositoryStub struct {
    JournalRecoveryRepository
    attempt *entity.RunAttempt
    err     error
    calls   int
}

func (s *humanResumeJournalRepositoryStub) GetActiveJournalAttempt(
    _ context.Context,
    _ int64,
) (*entity.RunAttempt, error) {
    s.calls++
    return s.attempt, s.err
}
```

- [x] **Step 2: Make the existing non-Journal happy path explicit**

In `TestApplicationResumeHumanInteractionCreatesQueuedRun`, inject the stub with
`err: domainrepo.ErrJournalNotEnrolled`. Keep all existing bundle assertions and add
`require.Equal(t, 1, journalRepo.calls)`.

- [x] **Step 3: Add the fail-closed table test**

Add `TestApplicationResumeHumanInteractionFailsClosedForJournalEnrollment` with two rows:

```go
tests := []struct {
    name    string
    attempt *entity.RunAttempt
    err     error
}{
    {
        name: "active attempt",
        attempt: &entity.RunAttempt{
            JournalRunID: 20,
            ExecutionRunID: 20,
            AttemptID: "attempt-1",
            Status: entity.RunAttemptStatusRunning,
        },
    },
    {name: "terminal enrollment", err: domainrepo.ErrJournalAttemptTerminal},
}
```

Use the same valid interrupted source and request shape as the happy path, call
`ResumeHumanInteraction`, and assert `ErrHumanInteractionResumeConflict`, exact error
text `journal-enrolled human interaction resume requires attempt rollover`, one attempt
lookup, no checkpoint request, and no `CreateRunBundle` request.

- [x] **Step 4: Add dependency-error and replay-priority tests**

Add `TestApplicationResumeHumanInteractionPropagatesJournalAttemptLookupFailure` using
`errors.New("attempt lookup failed")`; assert the same error and zero checkpoint/bundle
mutation. Strengthen `TestApplicationResumeHumanInteractionReturnsExistingIdempotentRun`
with a stub whose lookup error would fail the request, then assert `calls == 0` so exact
replay demonstrably precedes the gate.

- [x] **Step 5: Run RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread \
  -run '^(TestApplicationResumeHumanInteractionCreatesQueuedRun|TestApplicationResumeHumanInteractionFailsClosedForJournalEnrollment|TestApplicationResumeHumanInteractionPropagatesJournalAttemptLookupFailure|TestApplicationResumeHumanInteractionReturnsExistingIdempotentRun)$' -count=1
```

Expected: the active and terminal enrollment rows fail because the old implementation
continues to checkpoint lookup or creates a bundle. The non-Journal and replay cases may
already pass; they are compatibility guards.

### Task 2: Add the minimal application gate

**Files:**
- Modify: `backend/application/agentthread/human_interaction_resume.go`
- Test: `backend/application/agentthread/service_test.go`

- [x] **Step 1: Add one private gate helper**

Add:

```go
const humanResumeJournalRolloverRequired =
    "journal-enrolled human interaction resume requires attempt rollover"

func (s *ApplicationService) requireHumanResumeJournalRollover(
    ctx context.Context,
    sourceRunID int64,
) error {
    if s == nil || s.JournalRecoveryRepository == nil {
        return errors.New("journal attempt reader is unavailable")
    }
    attempt, err := s.JournalRecoveryRepository.GetActiveJournalAttempt(ctx, sourceRunID)
    switch {
    case err == nil && attempt != nil:
        return newHumanInteractionResumeSemanticError(
            ErrHumanInteractionResumeConflict,
            errors.New(humanResumeJournalRolloverRequired),
        )
    case err == nil:
        return errors.New("journal attempt reader returned no result")
    case errors.Is(err, domainrepo.ErrJournalNotEnrolled):
        return nil
    case errors.Is(err, domainrepo.ErrJournalAttemptTerminal):
        return newHumanInteractionResumeSemanticError(
            ErrHumanInteractionResumeConflict,
            errors.New(humanResumeJournalRolloverRequired),
        )
    default:
        return err
    }
}
```

Import the repository package with the existing domain alias convention.

- [x] **Step 2: Call the gate at the frozen priority point**

Immediately after the existing idempotent replay branch and before
`latestActiveADKCheckpoint`, add:

```go
if err := s.requireHumanResumeJournalRollover(ctx, req.SourceRunID); err != nil {
    return nil, err
}
```

Do not change request validation, source authorization/status checks, replay validation,
checkpoint parsing, bundle construction, or canonical handler mapping.

- [x] **Step 3: Run GREEN and package regression**

Run the focused command from Task 1. Expected: PASS.

Then run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread -count=1
```

Expected: PASS. If existing Human Resume tests construct an application without the
reader and reach the post-replay gate, update only those fixtures to return
`ErrJournalNotEnrolled`; do not weaken the production fail-closed behavior.

### Task 3: Synchronize authority and verify the slice

**Files:**
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Modify: `scripts/workbench-execution-graph/contract.mjs`

- [x] **Step 1: Record only the implemented boundary**

Mark P1M-C3h1b complete and state: exact replay first; active/terminal enrollment returns
the existing 409 conflict before checkpoint/new writes; explicit not-enrolled continues;
unexpected errors propagate. Keep Human rollover, Resume adaptive facts, IDL/UI, MySQL
dual-connection verification, P1M PASS, P1L, and Thread DELETE deferred. Do not add a new
production execution edge.

- [x] **Step 2: Update graph evidence and digest**

Reuse the existing adaptive boundary node, add the gate helper and its focused tests as
anchors/evidence, update the exclusion text, and replace only the structural digest in
`contract.mjs`.

- [x] **Step 3: Verify source, authority, and derived graph**

Run:

```bash
gofmt -w backend/application/agentthread/human_interaction_resume.go \
  backend/application/agentthread/service_test.go
git diff --check
jq -e empty docs/superpowers/context/workbench-execution-graph.json
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Expected: all commands PASS. Graph build artifacts remain ignored.

- [x] **Step 4: Review and commit exact scope**

Require separate spec-compliance and code-quality reviews with P0/P1 equal to zero. Stage
only the two Go files, the five authority files, and this plan; never stage the two
user-owned untracked plans. Commit with:

```bash
git commit --no-verify -m 'fix: fail closed for journal human resume'
```
