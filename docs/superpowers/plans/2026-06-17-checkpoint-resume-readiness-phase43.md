# Phase 43 - Checkpoint Resume Readiness

## Goal

Expose checkpoint-backed resume-readiness semantics without pretending the Go Agent Harness can replay a graph from a checkpoint yet. This phase gives API callers a precise answer for whether a checkpoint is a valid resume candidate and blocks explicit resume run creation until full replay is implemented.

## Scope

- Add checkpoint lookup by `checkpoint_id` across repository, domain service, and application service.
- Add `GET /api/threads/:thread_id/checkpoints/:checkpoint_id/resume`.
- Return a compact readiness response with:
  - `resumable`
  - `resume_from`
  - `reason`
  - `status`
  - `error_type`
  - `pending_sends`
  - checkpoint config and metadata
- Treat only Go Harness terminal failed/canceled checkpoints with pending sends as resume-ready.
- Reject explicit run creation from a checkpoint until graph replay exists.
- Detect explicit checkpoint selection from:
  - `command.checkpoint_id`
  - `command.resume.checkpoint_id`
  - `config.checkpoint_id`
  - `config.configurable.checkpoint_id`

## Out Of Scope

- Full graph replay from checkpoint.
- Worker execution from pending sends.
- Mutating checkpoint state.
- Forking thread state.
- Frontend resume controls.
- New database migrations.

## Readiness Rules

- `pending_sends_available`: resume-ready. The checkpoint is from `agent_harness`, terminal, failed/canceled, and has pending sends.
- `checkpoint_already_succeeded`: not resume-ready. The run already reached a successful terminal checkpoint.
- `no_pending_sends`: not resume-ready. There is no explicit next node to start from.
- `checkpoint_not_terminal`: not resume-ready. Running/intermediate checkpoints remain observable but are not replay candidates.
- `unsupported_checkpoint_source`: not resume-ready. Only Go Harness checkpoints are accepted for this phase.
- `unsupported_checkpoint_status`: not resume-ready. Unknown statuses are rejected conservatively.

## Execution Guard

Thread run creation remains unchanged for normal requests. If a request explicitly selects a checkpoint, the API evaluates readiness and returns `400`:

- not ready: `checkpoint is not resumable: <reason>`
- ready: `checkpoint resume execution is not enabled yet: pending_sends_available`

This prevents clients from accidentally creating a fresh run that looks like a resume but does not actually replay from checkpoint state.

## Verification

- `go test -count=1 ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/handler/coze ./api/router/coze -run 'Test(ThreadRepositoryCreateListAndGetLatestCheckpoints|GetCheckpointReturnsCheckpointByID|ApplicationCheckpointMethodsMapDomainCheckpoints|LangGraphCheckpointResumeReadinessHandlerReturnsPendingSends|LangGraphCheckpointResumeReadinessHandlerReturnsNotResumableForSucceededCheckpoint|RegisterIncludesLangGraphThreadRoutes|LangGraphRunCreateRejectsReadyCheckpointResumeUntilReplayIsImplemented|LangGraphRunCreateRejectsNotResumableCheckpoint)'`
- `go test -count=1 ./application/agentthread ./domain/agentthread/...`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`

## Next Phase

Phase 44 should add the first real resume command model for Go Harness. The safe next step is to create a new pending run that records the selected checkpoint and remains guarded from worker execution until the harness can initialize state from checkpoint values and pending sends.
