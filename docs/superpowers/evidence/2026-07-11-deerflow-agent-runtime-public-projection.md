# DeerFlow Agent Runtime Public Projection Evidence

**Date:** 2026-07-11

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX AI baseline:** `2c201bf66`

## Scope

This note records the evidence required before changing Agent runtime public
projections. It covers runs, visible messages, run events, checkpoints, token
usage and artifacts across Workbench REST/SSE and LangGraph-compatible APIs.

The target is observable DeerFlow parity with a stronger security boundary.
DeerFlow fields that expose raw runtime state are documented as upstream
defects and are not parity targets.

## 1. DeerFlow Source Chain

Locked source paths:

- Run request/response and REST list/get:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py`
  lines 37-77, 112-131 and 202-221.
- Thread/run message and event endpoints:
  the same file, lines 339-422.
- SSE serialization and bridge forwarding:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/services.py`
  lines 49-62 and 436-462.
- Thread state/history projection:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py`
  lines 445-494 and 599-664.

Verified behavior:

1. `RunResponse` contains identifiers, status, timestamps and token counters,
   but the locked baseline also copies `record.metadata` and `record.kwargs`
   without an allow-list.
2. `GET /api/threads/:thread_id/runs/:run_id/events` returns rows from the run
   event store without a public projection.
3. SSE calls `format_sse(entry.event, entry.data, ...)`, so bridge payloads are
   forwarded as-is.
4. Message endpoints return the run journal's displayable message rows. This
   visible user/assistant content is a product contract, while provider bodies,
   hidden reasoning, tool arguments/results and internal message metadata are
   not.
5. Thread state/history serializes selected checkpoint channel values. It does
   not intentionally expose serialized checkpoint implementation bytes, but it
   may include user metadata and latest display messages.

## 2. DeerFlow API And Runtime Evidence

Relevant public routes:

- `GET /api/threads/:thread_id/runs`
- `GET /api/threads/:thread_id/runs/:run_id`
- `GET /api/threads/:thread_id/messages`
- `GET /api/threads/:thread_id/runs/:run_id/messages`
- `GET /api/threads/:thread_id/runs/:run_id/events`
- `GET|POST /api/threads/:thread_id/runs/:run_id/stream`
- `GET /api/threads/:thread_id/state`
- `POST /api/threads/:thread_id/history`

Runtime checks performed against the locked source:

- A local Python invocation of `_record_to_response` with synthetic sentinel
  values confirmed that arbitrary nested `metadata` and `kwargs` survive the
  response model unchanged.
- Source-level execution of `sse_consumer` confirms that event payload data is
  passed directly into `format_sse`.
- The previously used DeerFlow browser URL was unavailable during this audit:
  `http://localhost:2026` returned connection refused. No live-page or live
  network claim is made for this run.

The raw `metadata/kwargs/event` behavior is an upstream security defect. The
approved parity design explicitly excludes copying it.

## 3. NewX AI Current Difference

Current internal conversion in
`backend/application/agentthread/thread_app.go` copies complete domain rows into
application summaries. This is acceptable for internal execution, but those
summaries are currently reused directly by public handlers.

Verified public gaps:

1. `taskThreadRunToAPI` returns top-level run `command`, `input`, `config`,
   `context`, raw `metadata`, `idempotency_key`, `worker_id` and raw
   `error_message`. Only subagent runs receive partial field clearing.
2. `langGraphRunToAPI` returns raw run metadata, input, command, config, context
   and error for every run kind.
3. Workbench run-event redaction is a handler-local deny-list. Event types not
   named by the helper return raw payload. The helper also preserves reasoning
   and selected tool argument fields that the backend security boundary forbids.
4. LangGraph REST and SSE independently deserialize and return event payloads.
   Unknown events, update/value/message modes and stream errors can expose raw
   runtime data or raw Go/provider errors.
5. LangGraph checkpoint history fallback can put a raw event payload into
   `values.event`; checkpoint metadata includes raw metadata and channel
   versions.
6. Workbench message rows expose raw message metadata. Visible user/assistant
   `content` is required by the product and must remain; metadata must be an
   allow-list projection.
7. Workbench run-journal messages currently expose `additional_kwargs`, usage
   objects and tool-call arguments. These are internal records, not a public
   debugging contract.
8. Token usage already clears `raw_usage` and metadata in the Workbench mapper,
   but the policy lives in the handler instead of one application projection.
9. Artifact responses expose `virtual_path` and raw metadata. The current task
   UI uses a bounded `/mnt/user-data/outputs/...` virtual path to identify and
   label presented files; object URIs, signed URLs, credentials and arbitrary
   metadata must be removed without breaking that bounded path contract.

## 4. Required Public Contract

The application layer will define explicit public projection types and
functions. Handlers may adapt field names for Workbench or LangGraph, but may
not read raw internal summaries after projection.

Allowed run data:

- run/thread/parent identifiers;
- assistant identifier and safe run kind/status labels;
- safe, allow-listed product metadata needed for subagent name/source/retry
  lineage;
- stream mode and bounded lifecycle policy labels where compatibility requires
  them;
- bounded product error code and public error message;
- timestamps.

Disallowed run data:

- command, input, config, context and raw metadata;
- idempotency key and worker identity;
- raw provider/runtime error text.

Allowed message data:

- message/thread/run identifiers, role, approved visible user/assistant
  content and timestamp;
- only explicitly approved message metadata.

Allowed event data:

- identifiers, type and timestamp for every event;
- a per-event-type allow-list of bounded display metadata;
- unknown event types receive no payload fields.

Allowed checkpoint data:

- checkpoint identifiers, parent, namespace, timestamp, safe next/interrupt
  labels and approved user-visible state such as title, Todo and visible
  messages;
- never checkpoint bytes, runtime keys, channel versions, pending sends or raw
  checkpoint metadata.

Allowed token and artifact data:

- bounded token counts, cost/currency when already safe, source/model labels and
  timestamps; never raw provider usage or metadata;
- artifact identifiers, title/type/content type/size/preview mode/timestamps
  and a validated user-data output virtual path; never object URI/key, signed
  URL, credential or raw metadata.

## 5. Implementation And Acceptance

1. Add failing application leak fixtures first.
2. Introduce one allow-list projection in
   `backend/application/agentthread/public_projection.go`.
3. Make Workbench REST/SSE and LangGraph REST/SSE consume the same projected
   run/event/error/message/checkpoint/token/artifact contracts.
4. Preserve visible conversation text and the bounded artifact output path;
   do not preserve hidden reasoning or raw tool/provider payloads.
5. Verify unknown event types reduce to ID/type/timestamp and raw errors map to
   bounded product errors while full details remain server-side logs only.

## 6. Implemented Result And Verification

Implemented public projections:

- `PublicRun`, `PublicMessage`, `PublicMessageMetadata`, `PublicRunEvent`,
  `PublicRunJournalMessage`, `PublicCheckpoint`, `PublicTokenUsage`,
  `PublicArtifact`, and `PublicRuntimeError`;
- one per-event-type allow-list shared by Workbench REST/SSE and LangGraph
  REST/SSE;
- approved visible user/assistant text, `llm.token` chunks, bounded node
  updates and step labels remain available;
- hidden reasoning, raw tool input/output, provider bodies/errors, runtime
  command/input/config/context, worker/idempotency fields, checkpoint bytes,
  channel versions and unsafe artifact paths are excluded;
- checkpoint recovery and update continue to consume internal state, while
  public state/history and resume readiness expose only projected values and
  safe next/interrupt labels.

Verification commands and results:

```bash
cd backend
go test ./application/agentthread -run 'TestPublic.*Redacts' -count=1
go test -gcflags="all=-N -l" ./api/handler/coze -run 'Redacts|DoesNotExpose|UnsafePayload' -count=1
go test ./application/agentthread -count=1
go test -gcflags="all=-N -l" ./api/handler/coze -count=1
go vet ./application/agentthread ./api/handler/coze
go test -p 1 -gcflags="all=-N -l" ./... -count=1

cd ../frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

All commands above passed. The task-detail suite reported 61/61 tests. A first
parallel `go test -gcflags="all=-N -l" ./...` attempt hit a `SIGBUS` in the
unchanged workflow compose Mockey test under Go 1.25; the failing package
passed immediately in isolation, and the serial full-backend command above
passed. `gofmt -l` and `git diff --check` returned no findings.
