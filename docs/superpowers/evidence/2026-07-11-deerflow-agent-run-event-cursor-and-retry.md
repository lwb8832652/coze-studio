# DeerFlow Agent Run Event Cursor And Retry Evidence

**Date:** 2026-07-11

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX AI baseline:** `a12fbfa65`

## Scope

This slice closes the last executable-retry and event-reconnect gaps in the
Agent runtime security/lifecycle plan. A queued subagent retry is now eligible
for the production ordinary worker without exposing checkpoint resume or
arbitrary queued runs. Workbench and LangGraph-compatible SSE now read events
through a durable event-ID cursor and drain every bounded page before emitting
the terminal marker.

## Verified DeerFlow Contract

Locked source:

- `backend/app/gateway/services.py` lines 438-458;
- `backend/packages/harness/deerflow/runtime/stream_bridge/memory.py` lines
  25-123;
- `backend/tests/test_stream_bridge.py` lines 142-213;
- `frontend/src/core/threads/hooks.ts` lines 944-968.

The DeerFlow frontend requests resumable streaming. Its gateway reads the
standard `Last-Event-ID` header and passes it to `StreamBridge.subscribe`.
The in-memory bridge resolves the first event after that ID, emits retained
events in order and yields the end sentinel only after the buffered event log
is exhausted. Tests prove replay after the cursor and correct continuation
when a bounded buffer has trimmed earlier events.

DeerFlow's community memory bridge retains at most 256 events per run by
default. NewX AI keeps the same visible reconnect semantics over its durable
MySQL event journal, so a process restart or a transcript longer than the
in-memory window does not lose the reconnect position.

## NewX AI Contract

### Durable event cursor

`ListRunEventsRequest` carries `AfterEventID` through application, domain and
repository layers. The repository applies `id > after_event_id` together with
the authorized thread/run filter, orders by ID ascending and then applies the
bounded limit. `Total` describes only rows after the cursor.

Workbench SSE accepts `after_event_id`; when it is absent, it uses
`Last-Event-ID`. A positive query cursor takes precedence, while negative or
malformed cursors are rejected before SSE headers are written. Each poll reads
page 1 after the latest emitted ID in batches of 200 until the returned batch
is short. A progress guard prevents an invalid page from spinning forever.
Only after the current cursor is drained does the handler check terminal run
state and emit one `done` event.

The LangGraph-compatible stream uses the same repository cursor. Its existing
stream-mode filtering still advances the durable cursor for intentionally
unrequested event modes, preventing those events from reappearing after a
reconnect.

### Executable subagent retry

The public retry command persists source child ID, parent ID, fixed first
attempt, requested time and an idempotency key. It remains a queued top-level
run. Ordinary worker claim now accepts either a normal pending top-level run or
a queued run with the bounded `subagent_retry` metadata contract. Queued
checkpoint resume rows and arbitrary queued rows remain ineligible and are not
stolen from the resume worker.

After claim, the existing command dispatcher invokes `ExecuteSubagentRetry`
instead of the ordinary executor and uses the same leased cancellation and
transactional finalization fence. An end-to-end SQLite test creates a failed
child through the public application path, creates the retry, runs the
production processor once, verifies success and confirms that a duplicate
retry returns the original run.

## Verification

RED/GREEN coverage includes:

- repository `id > cursor` filtering, ascending order and remaining total;
- negative cursor rejection and cross-layer propagation;
- 450 persisted events replayed after event 190 and event 320 with exact IDs,
  no gaps, no duplicates and one terminal `done`;
- Workbench query/header cursor normalization;
- LangGraph cursor reconnect regression;
- queued subagent retry claim without checkpoint-resume/manual-queue theft;
- public retry -> production worker -> retry executor -> succeeded state ->
  idempotent replay;
- persisted retry attempt in command, metadata and requested event.

Commands completed successfully:

```bash
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
```

All affected packages, targeted race runs, `go vet` and the serial full backend
suite exited `0`. Formatting and diff checks also passed.
