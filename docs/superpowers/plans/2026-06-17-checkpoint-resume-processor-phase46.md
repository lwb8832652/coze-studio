# Phase 46 - Checkpoint Resume Processor Skeleton

## Goal

Add a dedicated resume processor skeleton that consumes queued checkpoint-resume runs through the Phase 45 handoff boundary and fails safely until graph replay is implemented.

## Scope

- Add `ResumeRunProcessor`.
- Claim work through `ClaimQueuedResumeRuns`.
- Keep the normal `RunProcessor` and `ClaimPendingRuns` path unchanged.
- Parse `command.resume.checkpoint_id`.
- Load the referenced checkpoint through the application service.
- Validate that the checkpoint belongs to the resume run thread.
- Validate that checkpoint channel values and pending sends are available.
- Emit `run.resume.started`.
- Emit `run.failed` with resume checkpoint context.
- Fail valid resume runs with:
  - `error_code = checkpoint_replay_not_implemented`
  - `error_message = checkpoint replay is not implemented yet`
- Fail invalid resume payloads with:
  - `error_code = checkpoint_resume_payload_invalid`

## Out Of Scope

- Rebuilding Go Agent Harness state from checkpoint values.
- Executing from `pending_sends`.
- Connecting the resume processor to a background worker loop.
- Adding HTTP APIs for internal resume processing.
- Changing normal task execution.
- New database migrations.

## Processor Semantics

The processor is intentionally conservative:

- it only consumes runs already claimed by the dedicated queued-resume claim path;
- it validates the resume command and checkpoint payload before any replay attempt;
- it always fails with an explicit replay-not-implemented error after successful validation.

This gives later replay work a concrete and test-covered entry point without allowing a queued resume run to silently execute as a fresh task.

## Verification

- `go test -count=1 ./application/agentthread -run 'TestResumeRunProcessor'`
- `go test -count=1 ./application/agentthread -run 'Test(RunProcessor|ResumeRunProcessor)'`
- `go test -count=1 ./application/agentthread ./domain/agentthread/...`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`

## Next Phase

Phase 47 should introduce the replay state loader boundary: convert checkpoint channel values and pending sends into an internal harness resume input without invoking the planner yet.
