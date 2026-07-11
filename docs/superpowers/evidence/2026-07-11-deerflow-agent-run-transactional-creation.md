# DeerFlow Agent Run Transactional Creation Evidence

**Date:** 2026-07-11

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX AI baseline:** `8997e4874`

## Scope

This slice removes partial run creation from the ordinary new-task, canonical
follow-up, human-interaction resume and subagent-retry paths. A run and its
required user message or lifecycle event are now one repository transaction.
It also moves follow-up history authority from the browser to persisted server
state.

File upload remains an external prerequisite because object storage and MySQL
cannot share a local transaction. Once upload succeeds, the subsequent run and
message commit or roll back together. Lease recovery keeps the compensation
contract already verified in `AR-PARITY-001.4`.

## Verified DeerFlow Contract

Locked source:

- `frontend/src/core/threads/hooks.ts` lines 892-968;
- `backend/app/gateway/services.py` lines 345-398;
- `backend/packages/harness/deerflow/runtime/runs/manager.py` lines 223-238,
  354-397 and 540-611.

The DeerFlow frontend uploads files first and submits one current human message
through `thread.submit`. It does not send a browser-composed copy of the full
visible transcript. The gateway passes `body.input` to the run manager and
uses `create_or_reject` before starting execution.

Run persistence is a visibility boundary. `RunManager.create` and
`create_or_reject` register the pending run under the manager lock, persist the
new row, propagate persistence failure and remove the in-memory registration in
`finally` when persistence did not complete. Therefore a failed creation is
not left visible as a runnable local record.

The active-run strategy arbitration in `create_or_reject` is a separate
runtime scheduling contract and remains tracked after this transactional
persistence slice.

## NewX AI Contract

### Aggregate transactions

`CreateThreadBundle` writes the initial thread, task run and user message in one
GORM transaction. `CreateRunBundle` writes a follow-up/resume/retry run plus its
required message and/or event in one transaction. Relationship and ownership
checks run before persistence, and any message or event failure rolls back the
run.

The domain service allocates all aggregate IDs before the first write and binds
the generated thread/run IDs server-side. Human resume creates its user answer
and `human.interaction.resolved` event atomically. Subagent retry creates its
run and `subagent.retry.requested` event atomically.

### Idempotent replay

Both repository aggregate methods check the run idempotency key inside the
transaction. A replay returns the original complete aggregate with
`created=false`; an incomplete legacy aggregate or a key reused for another
thread/run shape fails closed. A concurrent unique-key winner is re-read after
rollback and returned only when its complete aggregate matches.

### Authoritative follow-up history

The browser sends only the current user turn and uploaded-file metadata. The
application authorizes thread access, then reads persisted messages in ordered
pages of 200 until the reported total is exhausted. It accepts only non-empty
user/assistant messages tied to a persisted run and appends the current user
turn exactly once. Browser-supplied historical messages are ignored.

For data written by the pre-transaction frontend, a successful run can still
reference a `run_id=0` user message through its persisted
`appended_message_id` metadata. NewX AI pages the thread's runs and restores
only those explicitly referenced legacy messages. A message left behind when
old split run creation failed has no matching run and remains excluded.

### Workbench API

`POST /api/workbench/task_threads/:thread_id/runs` accepts bounded
`message_content` and `message_metadata` fields and returns the committed user
message beside the run. The generated client maps those fields only to run
creation. Task detail uses the returned message for immediate optimistic
rendering, then refreshes the committed transcript.

## Verification

RED/GREEN coverage includes:

- initial thread/run/message commit, replay and message-failure rollback;
- run/message and run/event commit, replay and event-failure rollback;
- generated aggregate IDs and server-bound ownership;
- canonical follow-up atomic API submission and optimistic rendering;
- server-authoritative history, more than 200 persisted messages and no
  browser-history trust;
- successful legacy `run_id=0` message restoration and failed-orphan filtering;
- atomic human resume and subagent retry;
- generated API request-field mapping.

Commands completed successfully:

```bash
go test -gcflags='all=-N -l' ./domain/agentthread/repository \
  ./domain/agentthread/service ./application/agentthread \
  ./application/workbench ./api/handler/coze ./api/router/coze -count=1
go test -race ./domain/agentthread/repository \
  ./domain/agentthread/service ./application/agentthread -run \
  'Test(ThreadRepositoryCreate(Thread|Run)Bundle|Create(ThreadRunMessage|RunBundle)|Application(CreateRunWithMessage|ResumeHumanInteractionCreatesQueuedRun|RetrySubagentRunCreatesQueuedTopLevelRun))' -count=1
go vet ./domain/agentthread/repository ./domain/agentthread/service \
  ./application/agentthread ./application/workbench \
  ./api/handler/coze ./api/router/coze
go test -p 1 -gcflags='all=-N -l' ./... -count=1
```

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-follow-up.test.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/workbench/__tests__/workbench.test.tsx
npx tsc --noEmit --project tsconfig.json

cd ../../packages/arch/api-schema
npm run test -- __tests__/workbench-task-contract.test.ts
```

The affected frontend suites passed 93 tests, the API contract suite passed 3
tests, the targeted race run and `go vet` exited `0`, and the serial full
backend suite exited `0`. Formatting and diff checks also passed.
