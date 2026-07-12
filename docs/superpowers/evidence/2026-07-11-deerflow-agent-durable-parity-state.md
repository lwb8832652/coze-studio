# DeerFlow Agent Durable Parity State Evidence

## Result

- Tracker item: `AR-PARITY-002.4`.
- Result: complete for the scoped durable Eino thread-state projection.
- DeerFlow source baseline:
  `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- NewX runtime baseline: Eino ADK `v0.9.9`.
- Storage: existing `agent_checkpoints`; no schema or Atlas migration.

This slice keeps Eino checkpoint bytes internal and persists a separate,
versioned, bounded `ADKParityState` projection. The projection is the durable
source for Workbench Todo and LangGraph-compatible public state. Existing Plan,
Artifact, Skill and upload owners update the projection only after their own
operation succeeds.

## DeerFlow Source Evidence

The implementation was derived from source and runtime contracts, not inferred
from screenshots:

- `backend/packages/harness/deerflow/agents/thread_state.py:21-39`:
  sandbox writes are idempotent and conflicting identities fail closed.
- `thread_state.py:42-49`: Artifacts merge in insertion order and deduplicate.
- `thread_state.py:52-66`: viewed images merge by key; an explicit empty map
  clears the state.
- `thread_state.py:69-79`: Todo preserves state on `None`, while every supplied
  list, including an empty list, replaces it.
- `thread_state.py:87-105`: promoted tools replace on catalog-hash change and
  union in stable order for the same hash.
- `thread_state.py:108-116`: the persisted thread contract contains messages,
  sandbox/thread paths, title, Artifacts, Todo, uploads, viewed images and
  promoted tools.
- `backend/packages/harness/deerflow/tools/builtins/tool_search.py:62-70`:
  deferred-tool catalog identity is the hash of sorted full tool schemas.
- `tool_search.py:130-158`: successful search writes
  `catalog_hash + promoted names` into graph state.
- `backend/packages/harness/deerflow/agents/middlewares/uploads_middleware.py:62-76`
  and `151-187`: uploads are state owned by the upload middleware and normalized
  to `/mnt/user-data/uploads/*`.
- `backend/packages/harness/deerflow/agents/middlewares/todo_middleware.py:104-151`:
  Todo is read from thread state even after the original tool call leaves the
  active model context.
- `backend/packages/harness/deerflow/tools/builtins/present_file_tool.py:33-80`
  and `83-120`: only files under the current thread outputs root can be
  presented, and successful presentation updates Artifact state.
- `backend/app/gateway/routers/threads.py:117-127` and `599-668`: thread state
  and checkpoint history are separate API projections over persisted channel
  values; history serializes state rather than exposing checkpointer internals.

Eino ADK `v0.9.9` was also inspected directly:

- `adk/runner.go:271-345`: interrupt and cancellation signals trigger an opaque
  checkpoint save before the event is returned.
- `adk/interrupt.go:285-310`: Eino serializes runner context and interrupt
  addresses into opaque bytes before calling `CheckPointStore.Set`.
- Ordinary successful completion has no equivalent terminal `Set` call, so
  NewX must create its successful terminal projection explicitly.

## State Contract

`ADKParityState` stores only bounded metadata required to reconstruct the
visible task state:

| Field | Reducer/owner | Durable behavior |
| --- | --- | --- |
| ownership/workspace | runtime bootstrap | fixed space/thread identity; sandbox identity is idempotent and conflicts fail closed |
| messages/summary | parity middleware | safe user/assistant messages only; hidden system/tool payloads excluded; summary stores digest and counts only |
| title | successful finalizer | generated title is written only when the title CAS succeeds; a conflict snapshot omits stale title state |
| Todo | Plan backend | supplied lists replace, including explicit empty; refresh does not revive stale metadata |
| uploads | upload middleware | ordered merge by normalized virtual path under `/mnt/user-data/uploads` |
| Artifacts | `present_files` owner | ordered merge only after successful persistence under `/mnt/user-data/outputs` |
| viewed images | contract only | merge/explicit-clear semantics are implemented; no producer is fabricated |
| promoted tools | parity middleware | hash-scoped replace/union after policy-filtered dynamic catalog observation |
| active Skills | Skill middleware | stable ID, name and resolved version only; replaced per run |
| interrupts | checkpoint store | stable IDs/addresses only; opaque interrupt state remains internal |
| completion | executor/finalizer | successful and interrupted projection metadata; run table remains authoritative for canceled/failed lifecycle |

Global bounds cover item counts, labels, message content and the complete
serialized projection. Prompt text, reasoning, tool arguments/results,
credentials, object URIs, provider bodies and opaque checkpoint bytes are not
members of the public contract.

## Envelope Migration Matrix

| Stored form | Resume | Seed parity state | Public projection | Next write |
| --- | --- | --- | --- | --- |
| legacy top-level checkpoint | existing legacy path | only safe legacy fallback | existing bounded legacy projection | unchanged by legacy runtime |
| Eino envelope v1 | yes, opaque bytes preserved | no typed state | runtime and interrupt IDs only | v2 |
| Eino envelope v2 runtime | yes | full validated typed state | approved typed fields only | v2 |
| Eino envelope v2 interrupt | yes when durable interrupt exists | full validated typed state | typed fields plus interrupt IDs | v2 |
| Eino envelope v2 terminal | no | full validated typed state | typed fields plus completion | terminal row |
| unknown future version | no | rejected | empty safe Eino projection | none |
| malformed/identity mismatch | no | rejected | empty safe Eino projection | none |

The indexed envelope version, runtime key, thread ID and run ID must match the
embedded envelope. Thread-state seeding queries the latest `eino_adk`
checkpoint explicitly, so a newer checkpoint from another runtime cannot hide
the durable Eino state.

## Producer And Persistence Flow

1. The checkpoint store is created before Agent construction.
2. It reads the latest compatible thread-level Eino state, validates ownership,
   creates a mutex-protected tracker and attaches it to the runtime context.
3. Input history and uploads seed the new run. A resumed run seeds from the
   selected source envelope but writes new checkpoints under the active run.
4. Skill, Plan, upload, Artifact and dynamic-tool owners update the tracker
   after successful operations. Failed operations do not advance the state.
5. Eino runtime/interrupt checkpoint writes include a deep-copied v2 state while
   retaining opaque runner bytes internally.
6. Successful execution returns a deep-copied terminal snapshot. The domain
   allocates one checkpoint ID and the repository commits assistant message,
   optional title event, completion event, run status and terminal checkpoint
   in one fenced transaction.

When generated-title CAS loses to another writer, the transaction selects the
same checkpoint identity with title omitted. The concurrent thread row remains
the title authority and no stale or guessed title is projected. Exactly one
terminal checkpoint is inserted.
Duplicate IDs, lease loss, late cancellation, ownership mismatch or checkpoint
insert failure roll back all finalization writes.

## Public API Projection

`ProjectPublicCheckpoint` is the sole typed projection boundary for Eino v2.
The public state can contain:

```json
{
  "runtime": "eino_adk",
  "messages": [{"role": "user", "content": "..."}],
  "title": "...",
  "todos": [],
  "uploaded_files": [],
  "artifacts": [],
  "viewed_images": {},
  "promoted": {"catalog_hash": "...", "names": []},
  "active_skills": [],
  "interrupts": [],
  "completion": {"status": "succeeded"}
}
```

Workbench Todo lookup order is fixed:

1. latest compatible Eino v2 typed state;
2. latest arbitrary legacy top-level checkpoint;
3. thread metadata fallback.

An explicitly present empty typed Todo list ends the lookup. LangGraph state and
history use the same safe projection; older history entries omit repeated full
message arrays. Eino POST state mutation remains blocked because raw Eino
checkpoint bytes are not a public write contract.

## Security Review

- Space/thread/workspace identity is checked while seeding and before terminal
  persistence.
- Runtime type filtering prevents mixed-runtime state confusion.
- Resume rejects terminal checkpoints and unknown versions.
- Virtual files are accepted only under bounded upload/output roots.
- Public projection strips injected `<uploaded_files>` model context.
- No raw model/tool/provider/checkpoint payload is returned by Workbench or
  LangGraph-compatible APIs.
- Collection and total serialized-size bounds apply both on mutation and when a
  stored snapshot is decoded.

## Verification

Passed on 2026-07-12:

```bash
cd backend
go test ./application/agentthread ./domain/agentthread/service \
  ./domain/agentthread/repository -count=1

go test -gcflags='all=-N -l' ./api/handler/coze ./api/router/coze -count=1

go test -race ./application/agentthread ./domain/agentthread/service \
  -run 'TestADKParity|Test.*FinalizeRunSuccess' -count=1

go vet ./application/agentthread ./domain/agentthread/service \
  ./domain/agentthread/repository ./api/handler/coze ./api/router/coze

go test -p 1 -gcflags='all=-N -l' ./...

cd ..
APP_ENV=debug make build_server
git diff --check
```

All commands exited with status 0. The race build emitted only the known macOS
linker `LC_DYSYMTAB` warning. A plain `go test -p 1 ./...` is not accepted as
the repository-wide verdict because Mockey-based workflow tests require
`-gcflags='all=-N -l'`; the canonical full command above passed.

## Deliberate Boundaries

- DeerFlow has a viewed-image producer; the current NewX runtime does not. This
  slice implements safe persistence/reducer/projection semantics but does not
  invent a tool that has no production caller.
- Canceled and failed lifecycle authority remains `agent_runs` plus terminal
  run events. Eino may save an opaque cancellation checkpoint, but NewX does
  not claim that as a separate atomic terminal parity snapshot. Successful
  state is atomic; interrupt state is durable before returning control.
- No Python sidecar, IM channel, database migration or frontend-only stub was
  introduced.
