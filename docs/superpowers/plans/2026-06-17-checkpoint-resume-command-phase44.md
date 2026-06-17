# Phase 44 - Checkpoint Resume Command Model

## Goal

Turn a resume-ready checkpoint into an explicit run record without letting the normal worker execute it as a fresh pending run. This phase creates the command and metadata contract the Go Agent Harness can consume in a later replay phase.

## Scope

- Allow `CreateRunRequest` to carry an internal initial run status.
- Keep normal run creation defaulting to `pending`.
- Allow only `pending` and `queued` as create-time statuses.
- When a LangGraph run creation request selects a resume-ready checkpoint, create the run as `queued`.
- Normalize resume intent into `command.resume`.
- Store checkpoint resume metadata under `metadata.checkpoint_resume`.
- Keep not-resumable checkpoint requests rejected with the existing readiness reason.
- Cover both non-streaming and streaming LangGraph run creation.
- Add repository coverage proving `ClaimPendingRuns` does not claim queued resume runs.

## Out Of Scope

- Graph replay from checkpoint state.
- Worker startup from `pending_sends`.
- Checkpoint mutation or checkpoint forking.
- UI resume controls.
- New database migrations.
- Changing normal pending run scheduling.

## Resume Run Contract

Ready checkpoint resume creation persists a run with:

- `status = queued`
- `command.resume.checkpoint_id`
- `command.resume.checkpoint_ns`
- `command.resume.thread_id`
- `command.resume.run_id`
- `command.resume.resume_from = pending_sends`
- `command.resume.reason = pending_sends_available`
- `command.resume.pending_sends`
- `command.resume.guard = worker_replay_not_enabled`
- `metadata.checkpoint_resume.protected_from_worker_claim = true`
- `metadata.checkpoint_resume.guard = worker_replay_not_enabled`

The important production invariant is that queued resume runs are durable and visible, but the existing `ClaimPendingRuns` path continues to select only `pending` runs. A later phase can introduce a dedicated resume/replay worker path that understands checkpoint values and pending sends.

## Verification

- `go test -count=1 ./api/handler/coze -run 'TestLangGraphRunCreate(Stream)?CreatesProtectedResumeRunFromReadyCheckpoint|TestLangGraphRunCreateRejectsNotResumableCheckpoint'`
- `go test -count=1 ./domain/agentthread/... -run 'TestCreateRun(AcceptsQueuedInitialStatus|RejectsUnsupportedInitialStatus)|TestThreadRepositoryClaimPendingRunsSkipsQueuedResumeRuns'`
- `go test -count=1 ./application/agentthread -run TestApplicationCreateRunMapsDomainRun`
- `go test -count=1 ./application/agentthread ./domain/agentthread/...`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`

## Next Phase

Phase 45 should add the replay handoff boundary: a dedicated resume executor path can claim or transition queued resume runs only after it loads checkpoint values, rebuilds the harness state, and starts from pending sends.
