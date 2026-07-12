# DeerFlow Agent Runtime Slice 7 Durable Parity State Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` and `superpowers:test-driven-development` task by
> task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `AR-PARITY-002.4` with a versioned, durable, tenant-scoped Eino
thread-state contract for messages, Todo, workspace/uploads, presented
Artifacts, promoted tools, active Skills, interrupts and completion state.

**Architecture:** Keep Eino's opaque gob checkpoint bytes internal and add a
bounded `ADKParityState` projection to checkpoint envelope v2. A run-scoped,
mutex-protected tracker is seeded from the latest compatible thread checkpoint
and updated by concrete Eino middleware/tool owners. Interrupt snapshots are
written by the checkpoint store; successful terminal snapshots are committed
atomically with the assistant message, generated title and run completion in
the existing repository transaction. Public LangGraph and Workbench APIs read
only the typed projection and retain legacy top-level parsing as a compatibility
fallback.

**Tech Stack:** Go, Eino ADK `v0.9.9`, Hertz application/domain/repository
layers, GORM/MySQL, Testify.

---

## Locked Evidence

- DeerFlow baseline:
  `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- DeerFlow `thread_state.py` defines the persisted reducers: Todo replaces on
  every provided list including empty, Artifacts deduplicate in insertion
  order, uploads merge by path, promoted tools replace on catalog-hash change
  and union on the same hash, and conflicting sandbox identities fail closed.
- DeerFlow `tool_search.py` hashes the sorted full tool schemas and stores only
  `catalog_hash + names` in thread state.
- DeerFlow `uploads_middleware.py`, `todo_middleware.py` and
  `present_file_tool.py` update graph state at the behavior owner instead of
  reconstructing user-visible state from display events.
- DeerFlow `threads.py` projects latest checkpoint `channel_values` through
  `/api/threads/:thread_id/state`; checkpoint history is distinct from visible
  execution-step messages.
- Eino ADK `v0.9.9` persists opaque runner checkpoint bytes on
  interruption/cancellation, but does not create an ordinary successful
  terminal checkpoint. NewX therefore needs an explicit terminal snapshot.

## Confirmed NewX Gaps

- `ADKCheckpointEnvelope` v1 contains only opaque Eino bytes and interrupt
  targets. It cannot project Todo, uploads, Artifacts, Skills or promoted tools.
- `taskThreadTodosFromLatestCheckpoint` tries to parse top-level `todos`, but
  Eino checkpoint rows contain the envelope object; refresh parity is therefore
  accidental and falls back to thread metadata.
- Plan tasks and Artifacts are already durable in their owning services, but
  their current state is not represented in one thread checkpoint contract.
- Skills are version-resolved and tool search is deferred correctly, but the
  selected version snapshots and promoted dynamic tools live only in the
  current process/Eino message state.
- `FinalizeRunSuccess` atomically writes assistant message, title event and
  terminal run status. A separate post-finalization checkpoint would permit a
  succeeded run with stale thread state.
- There is no production viewed-image tool in the current runtime. The state
  contract will preserve bounded viewed-image references when supplied, but it
  will not fabricate a producer.

## File Structure

- Create `backend/application/agentthread/adk_parity_state.go`: public-safe
  state types, reducer semantics, bounds, cloning and context tracker.
- Create `backend/application/agentthread/adk_parity_state_test.go`: reducer,
  ownership, bounds and concurrency tests.
- Create `backend/application/agentthread/adk_parity_middleware.go`: Eino state
  observer for active messages, summary boundary and promoted dynamic tools.
- Create `backend/application/agentthread/adk_parity_middleware_test.go`:
  middleware ordering and state observation tests.
- Modify `backend/application/agentthread/adk_checkpoint.go`: envelope v2
  dual-read/write, thread-state seed loading and interrupt snapshots.
- Modify `backend/application/agentthread/adk_executor.go`: create/inject the
  run tracker and return a terminal snapshot.
- Modify concrete owners: `adk_uploads_middleware.go`, `adk_plan_backend.go`,
  `adk_artifact_tools.go`, `adk_middleware.go` and their focused tests.
- Modify finalization DTO/service/repository files to atomically persist the
  terminal checkpoint without adding a database migration.
- Modify `public_projection.go`, `langgraph_thread_service.go` and
  `workbench_thread_service.go` to use the typed state projection first.
- Add evidence under `docs/superpowers/evidence/` and close `.4` in the P0
  tracker only after all verification passes.

## Task 1: Typed State And Reducers

**Files:**

- Create: `backend/application/agentthread/adk_parity_state.go`
- Create: `backend/application/agentthread/adk_parity_state_test.go`

- [x] **Step 1: Write RED reducer tests.** Cover Todo replace-on-write including
  empty lists; ordered Artifact/upload dedupe; catalog-hash replacement and
  same-hash promotion union; Skill version replacement by stable ID; interrupt
  replacement; explicit viewed-image clear; and workspace identity conflict.
- [x] **Step 2: Run the focused test and verify RED.**

  Run:
  `cd backend && go test ./application/agentthread -run 'TestADKParityState' -count=1`

  Expected: compile/test failure because the parity state contract does not
  exist.
- [x] **Step 3: Implement the bounded contract.** Add schema version, revision,
  safe user/assistant messages, summary boundary, title, Todo items, logical
  workspace identity, upload refs, Artifact refs, viewed-image refs, promoted
  tools, active Skill refs, interrupt refs and completion reason. Reject invalid
  ownership, control characters, unknown statuses, oversized collections and
  path escapes. Never store prompts, reasoning, tool arguments/results,
  credentials, object URIs or raw provider payloads.
- [x] **Step 4: Implement a mutex-protected tracker.** `Snapshot` must deep-copy;
  reducer calls must be race-safe; seeding a different space/thread/workspace
  identity must fail closed.
- [x] **Step 5: Run focused tests and race tests.**

  Run:
  `cd backend && go test ./application/agentthread -run 'TestADKParityState' -count=1`

  Run:
  `cd backend && go test -race ./application/agentthread -run 'TestADKParityStateTracker' -count=1`

  Expected: PASS.

## Task 2: Envelope V2 And Migration

**Files:**

- Modify: `backend/application/agentthread/adk_checkpoint.go`
- Modify: `backend/application/agentthread/adk_checkpoint_test.go`
- Modify: `backend/application/agentthread/adk_contract_test.go`

- [x] **Step 1: Write RED compatibility tests.** Prove valid v1 checkpoints
  still resume, v2 carries typed parity state, unknown future versions fail
  closed, indexed/envelope versions must agree, and interrupt writes preserve
  parity state and parent lineage.
- [x] **Step 2: Add envelope v2 dual-read/write.** Keep v1 decoding unchanged;
  write v2 for all new snapshots. Runtime/interrupt envelopes require opaque
  checkpoint bytes; terminal snapshots may omit them and must never be returned
  by `CheckPointStore.Get` as resumable bytes.
- [x] **Step 3: Seed from the latest compatible thread checkpoint.** Verify
  thread/space/workspace ownership before accepting the projection. Legacy v1
  or legacy top-level checkpoints seed only fields that can be reconstructed
  safely; the next write upgrades them to v2.
- [x] **Step 4: Persist parity state in `Set` and `RecordInterrupts`.** Preserve
  opaque Eino bytes and runtime-version checks. Mark the snapshot phase and
  completion reason without exposing bytes through public APIs.
- [x] **Step 5: Run checkpoint suites.**

  Run:
  `cd backend && go test ./application/agentthread -run 'TestADKCheckpoint|TestADKParityEnvelope|TestADKContract' -count=1`

  Expected: PASS.

## Task 3: Runtime State Producers

**Files:**

- Create: `backend/application/agentthread/adk_parity_middleware.go`
- Create: `backend/application/agentthread/adk_parity_middleware_test.go`
- Modify: `backend/application/agentthread/adk_executor.go`
- Modify: `backend/application/agentthread/adk_executor_test.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/adk_middleware_test.go`
- Modify: `backend/application/agentthread/adk_uploads_middleware.go`
- Modify: `backend/application/agentthread/adk_uploads_middleware_test.go`
- Modify: `backend/application/agentthread/adk_plan_backend.go`
- Modify: `backend/application/agentthread/adk_plan_backend_test.go`
- Modify: `backend/application/agentthread/adk_artifact_tools.go`
- Modify: `backend/application/agentthread/adk_artifact_tools_test.go`

- [x] **Step 1: Write RED producer tests.** Verify run input seeds safe full
  history and current uploads; loaded Skills store stable ID/name/version only;
  Plan mutations replace Todo state; successful `present_files` appends ordered
  Artifact refs; and the parity middleware observes Eino summaries and only
  currently promoted dynamic tool names.
- [x] **Step 2: Inject the tracker before Agent construction.** Build the
  checkpoint store first, load the base state, create the tracker, attach it to
  the execution context, then build middleware/tools. Resume must seed from the
  source envelope but write only to the current run.
- [x] **Step 3: Add one real parity middleware.** Register it after
  summarization/Plan/tool-search state rewrites and before final semantic
  accounting. It records only safe user/assistant active messages, a bounded
  summary boundary and promoted names intersected with the policy-filtered
  dynamic catalog. Compute the catalog hash from sorted canonical tool schemas,
  matching DeerFlow's invalidation semantics.
- [x] **Step 4: Update concrete behavior owners.** Upload, Skill, Plan and
  Artifact owners update the shared tracker only after their own operation
  succeeds. Failed tool/service operations must not mutate parity state.
- [x] **Step 5: Return the deep-copied terminal snapshot.** The executor adds
  the final assistant message and `succeeded` completion reason to
  `RunExecutionResult`; interrupt paths record their targets/reason in the
  checkpoint store before returning. Canceled/failed lifecycle remains
  authoritative in the run table and terminal events; Eino cancellation bytes
  are not reclassified as an atomic terminal parity snapshot.
- [x] **Step 6: Run focused producer tests and race tests.**

  Run:
  `cd backend && go test ./application/agentthread -run 'TestADKParityMiddleware|TestADKExecutor|TestADKUploadedFiles|TestADKPlanBackend|TestADKArtifact' -count=1`

  Run:
  `cd backend && go test -race ./application/agentthread -run 'TestADKParity|TestADKExecutor' -count=1`

  Expected: PASS.

## Task 4: Atomic Terminal Persistence

**Files:**

- Modify: `backend/application/agentthread/runner.go`
- Modify: `backend/application/agentthread/resume_runner.go`
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [x] **Step 1: Write RED transaction tests.** A successful finalization must
  atomically write assistant message, optional generated title, completion
  event, succeeded run status and one terminal parity checkpoint. Duplicate
  checkpoint/message IDs, lost lease or cancellation must roll back every
  write.
- [x] **Step 2: Add a terminal-checkpoint request DTO.** The application layer
  validates the v2 envelope, runtime identity, thread/run ownership, phase and
  parent before passing a domain checkpoint entity. The domain service
  allocates its ID in the same ID batch as message/events.
- [x] **Step 3: Extend the repository transaction.** Create the checkpoint only
  after lease fencing and ownership checks but before commit. No schema or
  Atlas migration is required because `agent_checkpoints` already contains all
  fields.
- [x] **Step 4: Wire ordinary and resumed success.** Generated title is applied
  to the terminal parity snapshot before finalization. Resumed runs preserve
  source lineage and active Todo/Artifact/Skill state.
- [x] **Step 5: Run domain/application finalization suites.**

  Run:
  `cd backend && go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread -run 'Test.*FinalizeRunSuccess|Test.*TerminalParity' -count=1`

  Expected: PASS.

## Task 5: Public Projection And Workbench State

**Files:**

- Modify: `backend/application/agentthread/public_projection.go`
- Modify: `backend/application/agentthread/public_projection_test.go`
- Modify: `backend/api/handler/coze/langgraph_thread_service.go`
- Modify: `backend/api/handler/coze/langgraph_thread_service_test.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] **Step 1: Write RED API projection tests.** Eino v2 latest state/history
  must expose bounded messages, title, Todo, uploads, Artifact refs, promoted
  names, Skill refs, interrupts and completion metadata without opaque bytes or
  hidden payloads. History omits repeated messages after the latest entry.
- [x] **Step 2: Project typed parity state.** `ProjectPublicCheckpoint` decodes
  v2 and emits only the approved projection. v1 keeps `runtime + interrupts`.
  Unknown/malformed envelopes fail closed to an empty safe projection.
- [x] **Step 3: Make Workbench Todo parity-first.** Read `parity_state.todos`
  from the latest compatible Eino checkpoint; use legacy top-level checkpoint
  and metadata parsing only when no typed state exists. An explicitly empty
  Todo list must remain empty and must not trigger stale fallback data.
- [x] **Step 4: Preserve `/state` and `/history` contracts.** POST state remains
  blocked for opaque Eino runtime rows. GET state/history never expose raw
  checkpoint bytes, prompts, reasoning, tool arguments/results, paths outside
  approved virtual roots or credentials.
- [x] **Step 5: Run API/application suites.**

  Run:
  `cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'Test.*Checkpoint|Test.*ThreadState|Test.*Todos' -count=1`

  Expected: PASS.

## Task 6: Verification, Evidence And Commit

- [x] **Step 1: Run package verification.**

  Run:
  `cd backend && go test ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository ./api/handler/coze ./api/router/coze -count=1`

  Run:
  `cd backend && go test -race ./application/agentthread ./domain/agentthread/service -run 'TestADKParity|Test.*FinalizeRunSuccess' -count=1`

  Run:
  `cd backend && go vet ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository ./api/handler/coze ./api/router/coze`

  Expected: PASS.
- [x] **Step 2: Run broad backend verification.**

  Run: `cd backend && go test -p 1 -gcflags='all=-N -l' ./...`

  Run: `APP_ENV=debug make build_server`

  Expected: PASS. The repository's Mockey suites require disabled compiler
  optimizations; a plain `go test -p 1 ./...` is not an authoritative full-suite
  command for this repository.
- [x] **Step 3: Record evidence.** Add
  `docs/superpowers/evidence/2026-07-11-deerflow-agent-durable-parity-state.md`
  with DeerFlow source references, envelope migration matrix, reducer table,
  producer ownership, API examples, verification commands and known absence of
  a viewed-image producer.
- [x] **Step 4: Self-review and close tracker.** Check tenant isolation,
  rollback behavior, collection bounds, v1 resume, v2 projection and secret
  redaction. Mark only `AR-PARITY-002.4` complete; do not claim the entire
  semantic core complete if a separate tracked gap remains.
- [x] **Step 5: Commit the major mainline slice.** Stage only related files,
  commit once, and do not push or merge `dev` before user review.

## Exit Criteria

- Refresh and process restart preserve Todo, uploads, Artifacts, promoted tools,
  active Skill snapshots, interrupt targets and successful terminal state;
  canceled/failed lifecycle remains durable in the run aggregate.
- Eino v1 checkpoints remain resumable; all new writes use v2; unknown future
  versions fail closed.
- Successful run finalization cannot commit without its terminal parity
  checkpoint, and checkpoint failure rolls back message/title/run completion.
- Workspace ownership conflicts and cross-thread/cross-space state are rejected.
- Public APIs never expose Eino checkpoint bytes or hidden model/tool/provider
  payloads.
- No database migration, Python sidecar, IM channel work or fabricated
  viewed-image state is introduced.
