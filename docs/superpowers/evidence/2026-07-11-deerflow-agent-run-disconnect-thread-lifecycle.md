# DeerFlow Agent Run Disconnect And Thread Lifecycle Evidence

**Date:** 2026-07-11

**Task:** `AR-PARITY-001.11`

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

## Result

NewX now owns disconnect policy and thread lifecycle on the server. New runs
default to `on_disconnect=cancel`; only `cancel` and `continue` can be
persisted. Workbench and LangGraph SSE paths detect canceled requests, failed
writes and failed heartbeats, then apply the persisted mode through the
durable cancellation transaction. Thread reads derive status from durable
top-level task runs instead of trusting a stale thread row.

## DeerFlow Reference Verified

- `backend/app/gateway/routers/thread_runs.py:37-54` constrains
  `on_disconnect` to `cancel | continue` and defaults it to `cancel`.
- `backend/app/gateway/services.py:313-392` persists the selected mode and
  marks thread metadata running when a run starts.
- `backend/app/gateway/services.py:436-467` checks connection state while
  consuming SSE, emits heartbeat frames and cancels only active `cancel` runs.
- `backend/packages/harness/deerflow/runtime/stream_bridge/base.py:54-59`
  defines the stream heartbeat interval as 15 seconds.
- `backend/app/gateway/services.py:470-515` applies the same policy to wait
  consumers while treating an observed terminal sentinel as completion.
- `backend/packages/harness/deerflow/runtime/runs/manager.py:509-538` owns
  idempotent task interruption and durable cancellation state.
- `backend/packages/harness/deerflow/runtime/runs/worker.py:400-436` flushes
  completion data and projects terminal thread status before stream end.
- `backend/tests/test_wait_disconnect_handling.py:61-176` covers completion,
  cancel, continue and already-terminal disconnect behavior.

## NewX Contract Implemented

### Persisted disconnect mode

- Every domain run creation path normalizes the mode before persistence.
- Missing or whitespace-only mode becomes `cancel`; explicit `continue` is
  preserved; any other new value is rejected as a client error before a run or
  aggregate is written.
- Disconnect handling reloads the authorized run and trusts only its persisted
  mode. Only explicit `continue` survives; blank or legacy invalid values fail
  closed to cancellation.

### Server-owned stream lifecycle

- Workbench and LangGraph share a disconnect-tracking writer contract covering
  event, metadata, terminal and heartbeat writes.
- Request cancellation, failed writes and failed keepalives invoke
  `CancelRunOnDisconnect`; ordinary server timeout and successful terminal
  `done` / `end` do not.
- The cancellation call uses `context.WithoutCancel` plus a five-second bound,
  preserving authentication and authorization values while no longer inheriting
  the dead socket context.
- Cancellation reuses the existing transaction that fences execution,
  persists `run.canceled`, records bounded `client_disconnected` metadata and
  notifies the Eino ADK cancellation registry after commit.
- Streams emit DeerFlow's fixed 15-second keepalive. NewX's bounded server
  timeout performs one final keepalive probe before returning, so a short
  stream can still distinguish a dead connection from a normal timeout.

### Durable thread status projection

- `GetThread`, `ListThreads`, projected-status filters and totals use one SQL
  expression over `agent_runs`.
- Any top-level task run in `pending / queued / running` projects the thread as
  `running`, even if a newer terminal row exists.
- Otherwise the newest top-level task run maps `succeeded -> completed`,
  `failed -> failed`, `canceled -> canceled`, and `interrupted -> idle`.
- A thread with no top-level task run keeps its stored status. Child subagent
  runs are ignored.
- The expression uses SQL shared by MySQL and SQLite (`CASE`, `EXISTS`,
  `COALESCE`, ordered scalar subquery and `LIMIT`) and keeps filtered totals
  consistent with paginated rows.
- Domain, application, Workbench and LangGraph adapters pass the projected
  status through without client-side mutation.

## Tests Added Or Strengthened

- Domain tests cover the `cancel` default, explicit `continue`, and rejection
  before all three creation transaction paths.
- Application tests cover active cancel, explicit continue, terminal no-op and
  fail-closed legacy invalid mode.
- Workbench tests cover event-write failure, canceled request context, fixed
  15-second heartbeat parity, idle probe failure, explicit continue and normal
  timeout.
- LangGraph tests cover write-failure cancellation; its reconnect fixture now
  explicitly requests `continue`, keeping cursor replay separate from default
  cancellation semantics.
- Repository tests cover stale stored statuses, all terminal mappings, active
  precedence, child-run exclusion, no-run fallback, status-filtered totals and
  stable two-page results.

## Verification

All commands passed:

```bash
cd backend
go test -gcflags='all=-N -l' ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread ./api/handler/coze ./api/router/coze -count=1
go test -race -gcflags='all=-N -l' ./domain/agentthread/repository -run 'TestThreadRepository(ProjectsDurableTopLevelRunStatus|ListFiltersAndPaginatesProjectedStatus)$' -count=1
go test -race -gcflags='all=-N -l' ./application/agentthread ./api/handler/coze -run 'Test(ApplicationCancelRunOnDisconnectHonorsPersistedMode|RunEventStreamKeepAliveIntervalMatchesDeerFlow|StreamTaskThreadRunEventsCancelsPersistedCancelModeOnWriteFailure|StreamTaskThreadRunEventsCancelsPersistedCancelModeOnRequestCancellation|StreamTaskThreadRunEventsCancelsPersistedCancelModeOnHeartbeatFailure|StreamTaskThreadRunEventsDoesNotCancelContinueModeOrTimeout|LangGraphRunStreamCancelsPersistedCancelModeOnWriteFailure)$' -count=1
go vet ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread ./api/handler/coze ./api/router/coze
go test -p 1 -gcflags='all=-N -l' ./... -count=1
```

```bash
git diff --check
APP_ENV=debug make build_server
```

The macOS race link step emitted the existing malformed `LC_DYSYMTAB` warning;
all race binaries completed successfully with exit code zero.

## Security And Compatibility Boundary

- No client-supplied stream mode is consulted after run creation.
- No lease token, cancellation-registry state, prompt, provider body, tool
  argument/result or raw audit payload is added to public APIs.
- This change adds no schema migration and does not change the existing NewX
  public thread status vocabulary; it changes the authoritative source of that
  status to durable top-level runs.
