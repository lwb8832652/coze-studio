# DeerFlow Agent Runtime Slice 1 Security And Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox syntax for tracking.

**Goal:** Close production-blocking authorization, redaction, run ownership,
recovery, cancellation, retry scheduling, transaction and SSE gaps before
changing Agent semantics.

**Architecture:** Keep Eino ADK as the execution kernel and Coze/MySQL as the
durable control plane. Add one authenticated thread-access boundary, leased run
ownership with fencing, cursor-based event delivery and transactionally
consistent commands. Public Workbench and LangGraph projections remain
separate from internal runtime records.

**Tech Stack:** Go, Hertz, GORM/MySQL, Eino ADK, Atlas, Testify, Mockey.

---

## File Map

- backend/application/agentthread/thread_authorization.go: public thread/run
  access policy.
- backend/application/agentthread/public_projection.go: bounded public records.
- backend/domain/agentthread/entity/thread.go: lease and cancellation fields.
- backend/domain/agentthread/repository/repository.go: lease, recovery and
  cursor contracts.
- backend/domain/agentthread/repository/mysql.go: atomic claims, heartbeat,
  stale reconciliation and cursor queries.
- backend/domain/agentthread/service/service_impl.go: fenced transitions and
  transactional commands.
- backend/application/agentthread/runner.go: batch isolation and finalization.
- backend/application/agentthread/resume_runner.go: leased resume processing.
- backend/application/agentthread/worker.go: heartbeat and recovery workers.
- backend/api/handler/coze/workbench_thread_service.go: authorized, redacted
  Workbench API and cursor SSE.
- backend/api/handler/coze/langgraph_*_service.go: authorized compatibility API.
- docker/atlas/migrations/20260711000100_agent_run_leases.sql: run lease schema.

### Task 1: Enforce Thread And Run Authorization

**Files:**

- Create: backend/application/agentthread/thread_authorization.go
- Create: backend/application/agentthread/thread_authorization_test.go
- Modify: backend/application/agentthread/service.go
- Modify: backend/application/agentthread/init.go
- Modify: backend/application/application.go
- Modify: backend/application/workbench/chat_gateway.go
- Modify: backend/domain/user/internal/dal/space_user.go
- Modify: backend/domain/user/repository/repository.go
- Modify: backend/domain/user/service/user.go
- Modify: backend/domain/user/service/user_impl.go
- Modify: backend/api/handler/coze/workbench_thread_service.go
- Modify: backend/api/handler/coze/workbench_chat_service.go
- Modify: backend/api/handler/coze/langgraph_thread_service.go
- Modify: backend/api/handler/coze/langgraph_run_service.go
- Test: backend/application/workbench/chat_gateway_test.go
- Test: backend/domain/user/service/space_test.go
- Test: backend/api/handler/coze/workbench_thread_service_test.go
- Test: backend/api/handler/coze/langgraph_thread_service_test.go
- Test: backend/api/handler/coze/langgraph_run_service_test.go

- [x] **Step 1: Write failing application authorization tests**

Add owner, different-user and different-space cases for thread, message, run,
checkpoint, event and token operations. Use this contract:

~~~go
type ThreadAccessRequest struct {
    ViewerID int64
    SpaceID  int64
    ThreadID int64
    RunID    int64
}

type ThreadAuthorizer interface {
    AuthorizeThreadAccess(context.Context, ThreadAccessRequest) error
}
~~~

- [x] **Step 2: Verify the tests fail for the missing boundary**

Run:

~~~bash
cd backend
go test ./application/agentthread -run 'TestThreadOwnerAuthorizer|TestApplication.*AuthorizesThreadAccess' -count=1
~~~

Expected: FAIL because the authorizer and access calls do not exist.

- [x] **Step 3: Implement fail-closed owner and space authorization**

Load the thread through the domain service, require authenticated viewer
identity, compare creator and space, and verify an optional run belongs to the
thread. Wire the authorizer in InitService. A missing authorizer on a public
call is an error, not allow.

- [x] **Step 4: Add handler IDOR tests and pass server identity**

Use two session user IDs. Cover a read and mutation in every Workbench and
LangGraph resource family. Assert HTTP 403 and no mutation. Client owner, user
and space fields cannot grant access.

- [x] **Step 5: Verify and commit**

~~~bash
cd backend
go test ./application/agentthread -run 'Authoriz|AccessDenied' -count=1
go test -gcflags="all=-N -l" ./api/handler/coze -run 'Forbidden|AccessDenied' -count=1
git add backend/application/agentthread backend/api/handler/coze
git commit -m "fix: enforce agent thread access boundaries"
~~~

Completed 2026-07-11. The boundary now uses authenticated session/API-key
identity, current workspace membership, thread ownership and optional run
ownership. It also masks checkpoint existence, authorizes Workbench SSE before
headers, protects the legacy Workbench Chat path before persistence, and
caches successful request scopes without caching denials. Verification passed
for `domain/user`, `application/agentthread`, `application/workbench`, Coze
handlers and routes. Independent spec review returned `SPEC COMPLIANT`; the
follow-up quality review returned `CODE QUALITY APPROVED` after replacing the
full workspace-list lookup with a targeted membership query and removing
duplicate request/SSE authorization queries. Final backend verification passed
with `go test -gcflags="all=-N -l" ./... -count=1`; `gofmt -l` and
`git diff --check` returned no findings.

### Task 2: Separate Internal Records From Public Projections

Reference evidence:
`docs/superpowers/evidence/2026-07-11-deerflow-agent-runtime-public-projection.md`.

**Files:**

- Create: backend/application/agentthread/public_projection.go
- Create: backend/application/agentthread/public_projection_test.go
- Modify: backend/api/handler/coze/workbench_thread_service.go
- Modify: backend/api/handler/coze/langgraph_run_service.go
- Modify: backend/api/handler/coze/langgraph_thread_service.go
- Test: backend/api/handler/coze/workbench_thread_service_test.go
- Test: backend/api/handler/coze/langgraph_run_service_test.go

- [x] **Step 1: Write failing leak-fixture tests**

Build fixtures containing prompt/completion/reasoning text, tool
arguments/results, credentials, object URIs, raw config/context/command,
checkpoint bytes and provider errors. Assert public records keep only IDs,
status, safe labels, timestamps, bounded error codes, token counts and approved
event metadata.

- [x] **Step 2: Verify redaction tests fail**

~~~bash
cd backend
go test ./application/agentthread -run 'TestPublic.*Redacts' -count=1
~~~

- [x] **Step 3: Implement allow-list projections**

Define PublicRun, PublicRunEvent, PublicMessageMetadata and PublicRuntimeError.
Unknown event types expose only ID, type and timestamp. Raw errors are logged
internally and mapped to bounded product errors.

- [x] **Step 4: Make REST and SSE use the same projection**

Replace handler-specific partial redaction. Never return top-level command,
input, config, context or raw metadata based on run kind.

- [x] **Step 5: Verify and commit**

~~~bash
cd backend
go test ./application/agentthread -run 'TestPublic.*Redacts' -count=1
go test -gcflags="all=-N -l" ./api/handler/coze -run 'Redacts|DoesNotExpose|UnsafePayload' -count=1
git add backend/application/agentthread/public_projection* backend/api/handler/coze
git commit -m "fix: harden public agent runtime projections"
~~~

Completed 2026-07-11. One application-level allow-list now projects public
runs, user-visible messages, run events, journal rows, checkpoints, token
usage, artifacts and bounded runtime errors. Workbench REST/SSE and
LangGraph-compatible REST/SSE consume the same projection. Approved
user/assistant content and stream tokens remain visible; hidden reasoning,
tool arguments/results, raw provider payloads/errors, command/input/config/
context, worker/idempotency fields, checkpoint bytes and unsafe artifact paths
do not cross the API boundary. Checkpoint update/resume internals retain their
raw state behind the projection, so redaction does not corrupt recovery.

Verification passed for application and handler leak fixtures, complete
`application/agentthread` and Coze handler packages, targeted `go vet`, task
detail frontend tests (61/61), frontend TypeScript, `gofmt -l`, and
`git diff --check`. The full backend passed serially with
`go test -p 1 -gcflags="all=-N -l" ./... -count=1`; the first parallel run hit
an unrelated Mockey/Go 1.25 `SIGBUS` in the unchanged workflow compose test,
whose package passed immediately when rerun alone with the required gcflags.

### Task 3: Add Leased Run Ownership And Fencing

Reference evidence:
`docs/superpowers/evidence/2026-07-11-deerflow-agent-run-lease-ownership.md`.

**Files:**

- Create: docker/atlas/migrations/20260711000100_agent_run_leases.sql
- Modify: docker/atlas/migrations/atlas.sum
- Modify: backend/domain/agentthread/entity/thread.go
- Modify: backend/domain/agentthread/repository/repository.go
- Modify: backend/domain/agentthread/repository/mysql.go
- Test: backend/domain/agentthread/repository/mysql_test.go
- Modify: backend/domain/agentthread/service/service.go
- Modify: backend/domain/agentthread/service/service_impl.go
- Test: backend/domain/agentthread/service/service_impl_test.go
- Modify: backend/application/agentthread/dto.go
- Modify: backend/application/agentthread/thread_app.go
- Modify: backend/application/agentthread/service.go
- Modify: backend/application/agentthread/runner.go
- Modify: backend/application/agentthread/resume_runner.go
- Test: backend/application/agentthread/service_test.go
- Test: backend/application/agentthread/runner_test.go
- Test: backend/application/agentthread/resume_runner_test.go

- [x] **Step 1: Write failing lease tests**

Cover atomic claim, lease owner/token/expiry, heartbeat renewal, wrong-token
rejection, expired lease discovery and terminal lease clearing. Two claimers
must never own the same run.

- [x] **Step 2: Verify tests fail**

~~~bash
cd backend
go test ./domain/agentthread/repository -run 'Lease|Claim|Heartbeat' -count=1
~~~

- [x] **Step 3: Add the migration and domain contract**

Add nullable lease_owner, lease_token, lease_expires_at, heartbeat_at,
cancel_requested_at and monotonically increasing execution_generation. Add
indexes for status/expiry and worker heartbeat.

- [x] **Step 4: Implement atomic claim and fenced transitions**

Use a transaction and row locks. Increment generation on claim and require
owner, token and generation on every running-to-terminal transition.

- [x] **Step 5: Validate Atlas, test and commit**

~~~bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
cd backend
go test ./domain/agentthread/repository -run 'Lease|Claim|Heartbeat|Transition' -count=1
git add docker/atlas/migrations backend/domain/agentthread
git commit -m "feat: add leased agent run ownership"
~~~

Completed 2026-07-11. Both top-level claim paths now install a durable random
lease and increment execution generation in the same transaction. Heartbeats,
release and worker finalization use database compare-and-update predicates over
owner, token, generation and expiry. Successful, failed and interrupted runs
clear active ownership, while cancellation remains intentionally compatible
until Task 5. Release to `queued` is restricted to protected checkpoint resume
runs. Domain and application contracts carry the lease through ordinary and
resume processors, and public projections explicitly omit it.

Verification passed for repository RED/GREEN lease fixtures, all affected
domain/application/handler/router packages, targeted `go vet`, Atlas v0.35.0
hash and validation, public projection redaction, `gofmt`, `git diff --check`,
and the serial full backend with the required Mockey compiler flags.

### Task 4: Isolate Batches And Recover Stale Runs

**Files:**

- Modify: backend/application/agentthread/dto.go
- Modify: backend/application/agentthread/runner.go
- Modify: backend/application/agentthread/resume_runner.go
- Modify: backend/application/agentthread/worker.go
- Modify: backend/application/agentthread/service.go
- Test: backend/application/agentthread/runner_test.go
- Test: backend/application/agentthread/resume_runner_test.go
- Test: backend/application/agentthread/worker_test.go

- [ ] **Step 1: Write failing batch isolation tests**

Claim three runs, make the first fail at infrastructure level, and assert the
other two are processed or explicitly released. Repeat for resume runs.

- [ ] **Step 2: Write failing heartbeat and stale recovery tests**

With a fake clock, prove heartbeat renewal, shutdown cleanup, checkpoint-based
recovery and bounded run_abandoned failure when no checkpoint exists.

- [ ] **Step 3: Implement per-run isolation, heartbeat and reconciliation**

Continue the batch after a single-run error. Explicitly release an unfinalized
lease. Heartbeat active executions and periodically reconcile expired leases
with an idempotency key based on source run and generation.

- [ ] **Step 4: Verify and commit**

~~~bash
cd backend
go test ./application/agentthread -run 'Batch|Heartbeat|Stale|Abandoned|Recovery' -count=1
git add backend/application/agentthread
git commit -m "fix: recover and isolate leased agent runs"
~~~

### Task 5: Fence Cancellation And Transactional Finalization

**Files:**

- Modify: backend/application/agentthread/adk_cancel.go
- Modify: backend/application/agentthread/adk_executor.go
- Modify: backend/application/agentthread/service.go
- Modify: backend/application/agentthread/runner.go
- Modify: backend/application/agentthread/resume_runner.go
- Modify: backend/domain/agentthread/service/state_machine.go
- Modify: backend/domain/agentthread/service/service_impl.go
- Test: backend/application/agentthread/adk_cancel_integration_test.go
- Test: backend/application/agentthread/runner_test.go
- Test: backend/domain/agentthread/service/state_machine_test.go

- [ ] **Step 1: Write failing cancellation race tests**

Cover cancel before handle registration, during stream, after tool completion
but before assistant append, duplicate cancel and late executor success. Assert
no post-cancel assistant message, title update, success event or success state.

- [ ] **Step 2: Persist cancel intent and invalidate the fence first**

Record cancel_requested_at and invalidate execution_generation before
notifying an in-memory ADK handle. The durable fence is authoritative.

- [ ] **Step 3: Finalize message, title and success under one fence**

Check lease token, generation and cancel intent, then persist assistant message,
title and terminal success transactionally. A late result is discarded and
produces one canceled terminal event.

- [ ] **Step 4: Verify and commit**

~~~bash
cd backend
go test ./application/agentthread -run 'Cancel|LateResult|FinalizeFence' -count=1
go test ./domain/agentthread/service -run 'Cancel|Generation|Fence' -count=1
git add backend/application/agentthread backend/domain/agentthread/service
git commit -m "fix: fence agent run cancellation"
~~~

### Task 6: Make Create And Resume Commands Transactional

**Files:**

- Modify: backend/domain/agentthread/repository/repository.go
- Modify: backend/domain/agentthread/repository/mysql.go
- Modify: backend/domain/agentthread/service/service.go
- Modify: backend/domain/agentthread/service/service_impl.go
- Modify: backend/application/agentthread/service.go
- Modify: backend/application/agentthread/human_interaction_resume.go
- Test: backend/domain/agentthread/repository/mysql_test.go
- Test: backend/application/agentthread/service_test.go
- Test: backend/application/agentthread/adk_human_interaction_test.go

- [ ] **Step 1: Write failure-boundary rollback tests**

Inject failure after each thread, run, initial message, resume run and resume
message write. Assert no executable orphan remains and the idempotency key can
be safely retried.

- [ ] **Step 2: Add transactional command methods**

Implement CreateThreadRunMessage and CreateResumeRunMessage as repository
transactions. Validate policy and authorization before opening the transaction.
Write events from a committed outbox or reconstruct them idempotently.

- [ ] **Step 3: Verify and commit**

~~~bash
cd backend
go test ./domain/agentthread/repository -run 'Create.*Transaction|Rollback' -count=1
go test ./application/agentthread -run 'CreateTaskThread.*Rollback|Resume.*Rollback|Idempot' -count=1
git add backend/domain/agentthread backend/application/agentthread
git commit -m "fix: make agent run commands transactional"
~~~

### Task 7: Schedule Subagent Retry And Stream By Event Cursor

**Files:**

- Modify: backend/application/agentthread/subagent_retry.go
- Modify: backend/application/agentthread/runner.go
- Modify: backend/application/agentthread/dto.go
- Modify: backend/application/agentthread/service.go
- Modify: backend/domain/agentthread/repository/repository.go
- Modify: backend/domain/agentthread/repository/mysql.go
- Modify: backend/domain/agentthread/service/service_impl.go
- Modify: backend/api/handler/coze/workbench_thread_service.go
- Test: backend/application/agentthread/subagent_retry_test.go
- Test: backend/domain/agentthread/repository/mysql_test.go
- Test: backend/api/handler/coze/workbench_thread_service_test.go

- [ ] **Step 1: Write a failing retry scheduling test**

Create retry through the public method, run the production worker once, and
assert ExecuteSubagentRetry is called and the row reaches a terminal state.
Duplicate retry returns the existing row.

- [ ] **Step 2: Write a failing 450-event reconnect test**

Seed 450 events, stream after event 190, reconnect after 320, and assert every
event 191 through 450 is delivered exactly once before done.

- [ ] **Step 3: Implement executable retry command kind**

Persist source child, attempt and idempotency key. Include eligible retry rows
in leased claim and route them through the same cancel/finalize fence.

- [ ] **Step 4: Implement event-id cursor reads**

Query event_id greater than the cursor in ascending bounded pages. Drain all
pages per poll. Normalize query cursor and Last-Event-ID to one source.

- [ ] **Step 5: Verify and commit**

~~~bash
cd backend
go test ./application/agentthread -run 'SubagentRetry' -count=1
go test ./domain/agentthread/repository -run 'Claim.*Retry|RunEvents.*Cursor' -count=1
go test -gcflags="all=-N -l" ./api/handler/coze -run 'StreamTaskThreadRunEvents.*(Pagination|Reconnect|450)' -count=1
git add backend/application/agentthread backend/domain/agentthread backend/api/handler/coze
git commit -m "fix: schedule retries and stream events by cursor"
~~~

### Task 8: Slice Acceptance, Review And Evidence

**Files:**

- Modify: docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md
- Create: docs/superpowers/evidence/2026-07-11-deerflow-agent-runtime-parity/slice1-security-lifecycle.md

- [ ] **Step 1: Run the complete affected suite**

~~~bash
cd backend
go test ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository -count=1
go test -gcflags="all=-N -l" ./api/handler/coze ./api/router/coze -run 'Agent|Thread|Run|LangGraph|Workbench' -count=1
~~~

- [ ] **Step 2: Validate migrations and build**

~~~bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
make build_server
git diff --check
~~~

- [ ] **Step 3: Run authenticated API smoke**

With two test users, verify owner success and cross-user 403 for thread detail,
messages, run events, cancel and checkpoint history. Produce more than 200
events and confirm reconnect reaches terminal without gaps. Evidence must omit
credentials and raw runtime payloads.

- [ ] **Step 4: Review and close the slice**

Record exact commands and results, request code review from base to head, fix
all critical and important findings, then mark only this slice complete in the
tracker.

- [ ] **Step 5: Commit evidence**

~~~bash
git add docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md
git add docs/superpowers/evidence/2026-07-11-deerflow-agent-runtime-parity
git commit -m "docs: record agent runtime security lifecycle closure"
~~~

## Slice Exit Criteria

- Every public thread/run operation is authenticated and object/space
  authorized.
- Public REST and SSE pass raw-data leak fixtures.
- Every running run has an active lease/fence or is reconciled.
- One failed claimed run cannot strand the rest of a batch.
- Canceled runs cannot append a late assistant message or succeed.
- Task creation and resume cannot leave executable orphans.
- Subagent retry rows are consumed by a production worker.
- Workbench SSE reconnects beyond 200 events without gaps or duplicates.
