# DeerFlow Agent Runtime Slice 1 Security And Lifecycle Acceptance

**Date:** 2026-07-11

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX AI accepted range:** `2c201bf66..83cfc14ce`

## Result

Slice 1 is accepted. The durable control plane now satisfies every exit
criterion in
`docs/superpowers/plans/2026-07-11-deerflow-agent-runtime-slice1-security-lifecycle.md`.
This acceptance is intentionally narrower than complete DeerFlow Agent
semantic parity; the next P0 gaps are listed below.

## Accepted Mainline Commits

1. `2c201bf66` `fix: enforce agent thread access boundaries`
2. `c9a26fca1` `fix: harden public agent runtime projections`
3. `bca1ace81` `feat: add leased agent run ownership`
4. `e929b710f` `fix: recover and isolate leased agent runs`
5. `8997e4874` `fix: fence agent run cancellation`
6. `a12fbfa65` `feat: make agent run creation transactional`
7. `83cfc14ce` `fix: schedule retries and stream events by cursor`

## Exit Criteria

- **Authorization:** every public thread/run operation uses authenticated
  workspace, owner and optional run ownership checks. Cross-user mutations are
  rejected before persistence, and SSE authorization completes before stream
  headers.
- **Public projection:** Workbench and LangGraph REST/SSE share bounded
  allow-list projections. Prompt, hidden reasoning, tool arguments/results,
  credentials, checkpoint bytes, raw provider errors and lease internals remain
  private.
- **Leased ownership:** a running run has one owner/token/generation/expiry
  fence. Heartbeats renew ownership, terminal transitions clear it and stale
  workers cannot publish.
- **Batch isolation and recovery:** one failed run does not strand a batch.
  Expired leases reconcile through a compatible checkpoint or bounded
  `run_abandoned` failure.
- **Cancellation:** durable cancellation invalidates the generation before
  notifying the local Eino registry. A late worker result cannot append an
  assistant message, update title or succeed.
- **Transactional commands:** initial task, follow-up, human resume and
  subagent retry aggregates commit or roll back together and replay by
  idempotency key.
- **Executable retry:** queued subagent retry commands are selectively claimed
  by the ordinary worker, dispatched to `ExecuteSubagentRetry`, fenced and
  finalized. Resume and arbitrary queued rows are not stolen.
- **Resumable events:** Workbench and LangGraph SSE use durable event-ID cursors,
  normalize query/header reconnect positions and drain all bounded pages before
  terminal output. A 450-event fixture passes reconnect after events 190 and
  320 without a gap or duplicate.

## Verification Evidence

Detailed evidence:

- `docs/superpowers/evidence/2026-07-11-deerflow-agent-runtime-public-projection.md`
- `docs/superpowers/evidence/2026-07-11-deerflow-agent-run-lease-ownership.md`
- `docs/superpowers/evidence/2026-07-11-deerflow-agent-run-cancellation-fence.md`
- `docs/superpowers/evidence/2026-07-11-deerflow-agent-run-transactional-creation.md`
- `docs/superpowers/evidence/2026-07-11-deerflow-agent-run-event-cursor-and-retry.md`

Final commands completed successfully:

```bash
atlas version
atlas migrate validate --dir file://docker/atlas/migrations
(cd docker/atlas && atlas migrate hash)

cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service \
  ./application/agentthread -count=1
go test -gcflags='all=-N -l' ./api/handler/coze ./api/router/coze -count=1
go test -race ./domain/agentthread/repository ./application/agentthread -run \
  'Test(ThreadRepositoryListRunEventsFiltersAfterCursor|ThreadRepositoryClaimPendingRunsClaimsOnlyQueuedSubagentRetries|SubagentRetryPublicCommandRunsThroughProductionWorker)$' -count=1
go test -race ./api/handler/coze -run \
  'Test(StreamTaskThreadRunEventsDrains450EventsAcrossReconnects|ResolveTaskThreadRunEventCursorUsesHeaderAndQueryPrecedence|LangGraphRunStreamReconnectReplaysEventsExactlyOnceInIDOrder)$' -count=1
go vet ./domain/agentthread/repository ./domain/agentthread/service \
  ./application/agentthread ./api/handler/coze ./api/router/coze
go test -p 1 -gcflags='all=-N -l' ./... -count=1

cd ..
APP_ENV=debug make build_server
```

The Atlas CLI was local Community `v0.35.0`. Hash and validation produced no
tracked migration drift. The ordinary `make build_server` compiled the Go
binary but could not copy absent local `docker/.env`; the documented debug
profile command used the existing ignored `docker/.env.debug` and completed
with exit code `0`. No credentials are included in this evidence.

The final authenticated handler smoke covered owner/cross-user thread read,
message mutation, resume/retry mutation, run mismatch/cancel protection, SSE
authorization-before-headers, 450-event reconnect and LangGraph reconnect. It
completed with exit code `0`.

## Remaining DeerFlow P0 Semantics

These items are outside Slice 1 and prevent a claim of complete backend Agent
parity:

1. **Atomic multitask admission.** DeerFlow `RunManager.create_or_reject`
   defaults to `reject` and performs active-run detection plus
   `reject / interrupt / rollback` under one lock. NewX AI currently defaults
   new runs to `enqueue` and does not arbitrate same-thread active runs in the
   creation transaction. Multiple top-level runs can therefore be admitted
   before workers claim them.
2. **Durable stream-end ordering.** DeerFlow publishes its end sentinel only
   after its stream bridge has published prior events. NewX AI drains every
   event already committed before checking terminal run state, but terminal
   status and all terminal event types are not yet one durable journal
   transaction. The next slice must define a durable end marker or atomically
   persist terminal events before claiming exact end-order parity.
3. **Thread lifecycle aggregate.** DeerFlow updates thread metadata/status when
   a run starts. NewX AI's canonical thread row is created as `idle`, while
   current UI status is primarily inferred from runs. A server-owned aggregate
   status projection is still required for consistent filtered task lists and
   non-UI consumers.

The recommended next slice order is: atomic multitask admission, terminal
event/end ordering, then thread lifecycle projection. UI or platform
enhancements remain secondary until these runtime semantics close.
