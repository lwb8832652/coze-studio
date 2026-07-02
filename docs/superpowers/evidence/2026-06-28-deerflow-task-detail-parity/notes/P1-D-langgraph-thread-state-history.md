# P1-D LangGraph Thread State/History Compatibility Evidence

Date: 2026-07-02

## Scope

This note records the P1-D compatibility slice for DeerFlow's thread state,
checkpoint history, thread/run message API surface, run event debug API,
wait endpoints, stateless run read API, and existing-run stream route method
parity. It covers Coze's
`POST /api/threads/:thread_id/state`,
`POST /api/threads/:thread_id/history`, and
`GET /api/threads/:thread_id/runs/:run_id/messages` behavior, plus
`GET /api/threads/:thread_id/messages`,
`GET /api/threads/:thread_id/runs/:run_id/events`,
`POST /api/threads/:thread_id/runs/wait`, `POST /api/runs/wait`,
`GET /api/runs/:run_id/messages`, `GET /api/runs/:run_id/feedback`, and the
`POST /api/threads/:thread_id/runs/:run_id/stream` alias with
`action=interrupt|rollback` cancel support. The existing Coze
`GET /api/threads/:thread_id/history` compatibility endpoint is preserved.

Visible task-detail steps are not sourced from checkpoint history. They remain
projected through the DeerFlow-style run journal / per-run message contract
recorded in `P1-J-005-run-journal-message-contract.md`.

## DeerFlow Reference

Verified source anchors:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py:117`
  defines `ThreadStateResponse` with `values`, `next`, `metadata`,
  `checkpoint`, `checkpoint_id`, `parent_checkpoint_id`, `created_at`, and
  `tasks`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py:138`
  defines `ThreadStateUpdateRequest` with `values`, `checkpoint_id`,
  `checkpoint`, and `as_node`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py:498`
  exposes `POST /api/threads/{thread_id}/state`; it loads the existing
  checkpoint, shallow-merges `values` into `channel_values`, writes a fresh
  checkpoint, and syncs `title` through the thread store.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py:158`
  defines `ThreadHistoryRequest` with body `limit` and `before`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py:592`
  exposes `POST /api/threads/{thread_id}/history`; it returns checkpoint
  entries with top-level checkpoint identifiers and attaches `messages` only to
  the latest entry.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py:384`
  exposes `GET /api/threads/{thread_id}/runs/{run_id}/messages`, returns
  `{ data, has_more }`, and accepts `limit`, `before_seq`, and `after_seq`.
  This is the visible run-message source used by the UI for execution steps.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py:286`
  registers both `GET` and `POST`
  `/api/threads/{thread_id}/runs/{run_id}/stream`; the source notes that the
  LangGraph SDK uses `POST` for `joinStream` / `useStream` stop flows.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py:340`
  exposes `GET /api/threads/{thread_id}/messages`, returns a displayable
  message list across runs, and attaches `feedback` to the last AI message per
  run.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py:410`
  exposes `GET /api/threads/{thread_id}/runs/{run_id}/events` for debug/audit
  event reads.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py:162`
  exposes `POST /api/threads/{thread_id}/runs/wait`, which creates a run,
  waits for completion, and returns final channel values when available.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/runs.py:48`
  exposes stateless `POST /api/runs/wait`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/runs.py:90`
  exposes stateless `GET /api/runs/{run_id}/messages` and
  `GET /api/runs/{run_id}/feedback`.

## Coze Gap Before This Slice

- Coze had `GET /api/threads/:thread_id/state`, but no DeerFlow-compatible
  `POST /api/threads/:thread_id/state`.
- Coze had `GET /api/threads/:thread_id/history?limit&offset`, but no
  DeerFlow-compatible `POST /api/threads/:thread_id/history` body contract.
- ADK checkpoint envelopes could not be safely exposed as LangGraph
  `channel_values`; the compatibility projection needed to redact raw
  checkpoint bytes and runtime keys.
- Coze had the internal DeerFlow-style journal projection used by task detail,
  but no LangGraph route for
  `GET /api/threads/:thread_id/runs/:run_id/messages`.
- Coze did not expose the DeerFlow thread-level message list, run event debug
  list, thread/stateless wait endpoints, or stateless run messages/feedback
  routes.

## Coze Implementation

Source anchors:

- `backend/api/router/coze/api.go`
  registers `POST /api/threads/:thread_id/state`,
  `POST /api/threads/:thread_id/history`, and
  the added LangGraph thread/run/stateless compatibility routes.
- `backend/api/model/agent/langgraph/thread.go`
  adds `PostThreadStateRequest`, `PostThreadHistoryRequest`, and
  `ThreadHistoryEntry`.
- `backend/api/handler/coze/langgraph_thread_service.go`
  implements:
  - state update by loading a specified or latest checkpoint, shallow-merging
    request `values`, writing a new checkpoint, preserving the base checkpoint
    namespace/run/pending sends, and syncing `title` via
    `UpdateThreadTitle`;
  - checkpoint history body cursor support via `limit` and `before`;
  - DeerFlow-shaped `HistoryEntry` output with `parent_checkpoint_id:null`
    when absent;
  - latest-only `messages` projection for checkpoint history;
  - ADK checkpoint redaction: raw checkpoint bytes, runtime keys, and internal
    envelope fields are not emitted. Safe values only include `runtime` and
    interrupt IDs for `next`.
- `backend/api/handler/coze/langgraph_run_service.go`
  implements `GET /api/threads/:thread_id/runs/:run_id/messages` by projecting
  persisted messages and run events through `ProjectRunJournalMessages`, then
  returning a safe page of `human` / `ai` / `tool` messages with deterministic
  `seq`, `before_seq` / `after_seq` filtering, and `has_more`.
- `backend/api/handler/coze/langgraph_run_service.go`
  implements `GET /api/threads/:thread_id/messages` with
  `ProjectThreadRunJournalMessages`, stable `CreatedAt` / `RunID` ordering,
  DeerFlow-style array output, cursor filtering, and `feedback:null` for the
  current Coze feedback-less runtime.
- `backend/api/handler/coze/langgraph_run_service.go`
  implements `GET /api/threads/:thread_id/runs/:run_id/events` by reusing the
  Workbench safe run-event projection. It returns a DeerFlow-shaped array while
  preserving Coze redaction for tool results, credentials, raw object URLs,
  provider payloads, and token internals.
- `backend/api/handler/coze/langgraph_run_service.go`
  implements thread and stateless `wait` routes by creating a run, reusing the
  existing wait-terminal helpers, and returning final checkpoint values when
  present. If no final checkpoint exists before timeout, the response falls
  back to `{status,error}`, matching DeerFlow's no-final-state branch.
- `backend/api/handler/coze/langgraph_run_service.go`
  implements stateless run messages by reusing the bounded per-run journal
  projection. Stateless feedback returns `[]` for valid runs because Coze does
  not yet have a DeerFlow-equivalent feedback repository.
- `backend/api/router/coze/api.go`
  maps `POST /api/threads/:thread_id/runs/:run_id/stream` to the same bounded
  stream handler as `GET`, matching DeerFlow's method surface. The handler now
  processes `action=interrupt|rollback` before SSE writer creation; `wait=1`
  waits for terminal cancellation and returns `204`.

Safety note:

- `POST /state` is fail-closed for Eino ADK runtime checkpoints. Those rows
  contain an internal checkpoint envelope rather than LangGraph channel values,
  so treating them as mutable state would risk corrupting runtime recovery data.
- Run messages expose bounded journal fields only. Tool call ids, names, and
  types are retained for UI grouping; raw tool arguments are not emitted from
  this route.
- Run events reuse the existing Workbench redaction path and do not expose raw
  tool result content, credentials, signed URLs, object paths, or provider
  payloads.
- Stateless feedback is compatibility-only for now: it verifies run existence
  and returns an empty list instead of pretending that feedback persistence
  exists.

## Automated Verification

Commands:

```bash
cd backend
go test ./api/handler/coze -run TestLangGraphThreadStatePostHandlerMergesValuesAndSyncsTitle -count=1 -gcflags="all=-N -l"
go test ./api/router/coze -run TestRegisterIncludesLangGraphThreadRoutes -count=1
go test ./api/handler/coze -run TestLangGraphRunMessagesHandlerReturnsDeerFlowPage -count=1 -gcflags="all=-N -l"
go test ./api/handler/coze -run 'TestLangGraph(ThreadMessagesHandlerReturnsDeerFlowList|RunEventsHandlerReturnsSafeDeerFlowList)' -count=1 -gcflags="all=-N -l"
go test ./api/handler/coze -run 'TestLangGraph(RunWaitCreatesRunAndReturnsFallbackState|StatelessRunWaitCreatesRunAndReturnsFallbackState)' -count=1 -gcflags="all=-N -l"
go test ./api/handler/coze -run 'TestLangGraphStatelessRun(MessagesHandlerReturnsDeerFlowPage|FeedbackHandlerReturnsEmptyListForValidRun)' -count=1 -gcflags="all=-N -l"
go test ./api/handler/coze -run TestLangGraphRunStreamPostInterruptCancelsRunWhenWaitRequested -count=1 -gcflags="all=-N -l"
go test ./api/router/coze -run TestRegisterIncludesLangGraphRunRoutes -count=1
go test ./api/handler/coze -run 'TestLangGraphThread(Patch|Delete)' -count=1 -gcflags="all=-N -l"
go test ./domain/agentthread/repository -run 'TestThreadRepository(UpdateThreadMetadata|DeleteThreadRemovesThreadDomainRows)' -count=1
go test ./domain/agentthread/service ./application/agentthread ./api/handler/coze ./api/router/coze -run 'Test(Thread|LangGraph|RegisterIncludesLangGraph)' -count=1 -gcflags="all=-N -l"
go test ./domain/agentthread/repository ./api/handler/coze ./api/router/coze -run 'Test(ThreadRepository|LangGraph|RegisterIncludesLangGraph)' -count=1 -gcflags="all=-N -l"
go test ./api/handler/coze -run 'TestLangGraph' -count=1 -gcflags="all=-N -l"
go test ./api/router/coze -run 'TestRegisterIncludesLangGraph' -count=1
```

Result:

- All commands passed on 2026-07-02.
- `git diff --check` passed.

Covered assertions:

- `POST /api/threads/:thread_id/state` route is registered.
- `POST /state` merges new channel values while retaining prior messages.
- `POST /state` creates a new checkpoint with the base checkpoint as parent.
- `POST /state` records `source=update`, increments `step`, stores
  `writes[as_node]`, and syncs thread title.
- `POST /api/threads/:thread_id/history` route is registered.
- `POST /history` accepts DeerFlow body cursor fields `limit` and `before`.
- `POST /history` returns top-level `checkpoint_id`, `parent_checkpoint_id`,
  `checkpoint_ns`, `values`, `metadata`, `created_at`, and `next`.
- ADK history projection does not expose `checkpoint_bytes`,
  raw checkpoint payload, or `runtime_key`.
- `GET /api/threads/:thread_id/runs/:run_id/messages` is registered.
- `GET /api/threads/:thread_id/messages` is registered.
- `GET /api/threads/:thread_id/runs/:run_id/events` is registered and returns
  redacted event payloads.
- `POST /api/threads/:thread_id/runs/wait` and `POST /api/runs/wait` are
  registered and return the DeerFlow fallback state when no checkpoint is ready.
- `GET /api/runs/:run_id/messages` and `GET /api/runs/:run_id/feedback` are
  registered.
- `POST /api/threads/:thread_id/runs/:run_id/stream` is registered.
- `POST /api/threads/:thread_id/runs/:run_id/stream?action=interrupt&wait=1`
  cancels the run and returns `204`.
- `PATCH /api/threads/:thread_id` is registered and merges allowed metadata
  without allowing client-supplied identity, space, status, source, timestamp,
  values, or interrupt fields to override server-owned projection fields.
- `DELETE /api/threads/:thread_id` is registered and returns DeerFlow-style
  `{success,message}`. Coze deletion is implemented below the API layer and
  removes the thread plus thread-domain messages, runs, run events,
  checkpoints, memories, memory audit rows, transcript snapshots, memory flush
  jobs, token usage, runtime files, artifacts, artifact scan jobs, run plans,
  and run plan items in one repository transaction.
- Run messages return `{data, has_more}` with deterministic `seq`.
- Run message pagination supports `limit` and `after_seq`.
- Run message projection includes human input, AI reasoning, tool name, and
  tool call id while excluding unbounded raw event payloads.
- Thread messages return a DeerFlow-style array across runs with `feedback:null`
  and no raw tool arguments.
- Stateless feedback returns `[]` for a valid run until a real feedback store is
  added.

## P1-D Closure

- DeerFlow source verification showed `PATCH /threads/{thread_id}` merges
  metadata and re-reads the thread, while `DELETE /threads/{thread_id}` returns
  `{success,message}` after deleting local thread data/checkpoints/thread meta.
  Coze now exposes equivalent HTTP methods and keeps the data mutation below
  the API layer.
- Coze intentionally strips a broader set of server-owned patch metadata than
  DeerFlow's `owner_id/user_id` filter because Coze's task thread projection
  also carries `space_id`, `creator_id`, `source`, status, timestamps, values,
  and interrupts. This is a security boundary, not a missing parity feature.
- `POST /state` full `checkpoint` object replacement remains intentionally
  unsupported for P1 because verified DeerFlow-visible workflows use `values`
  merging, and Coze ADK checkpoints must remain protected from raw checkpoint
  replacement through Workbench/LangGraph compatibility APIs.
