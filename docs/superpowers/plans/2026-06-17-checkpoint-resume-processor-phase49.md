# Phase 49 - Checkpoint Resume Processor Execution

## Goal

Connect queued checkpoint resume runs to the Go-native harness resume executor. This replaces the earlier safe `checkpoint_replay_not_implemented` failure with actual pending-step replay while keeping resume processing behind the dedicated queued-resume claim path.

## Scope

- Add a resume executor boundary to `ResumeRunProcessor`.
- Default the processor executor to `NewApplicationHarnessExecutor(app)`.
- Load checkpoint state through the existing `HarnessResumeInput` conversion.
- Call `Resume(ctx, run, input)` after checkpoint validation and `run.resume.loaded`.
- Append the resumed assistant message to the resume run.
- Complete the resume run from `running` to `succeeded`.
- Emit `run.completed` with checkpoint resume context.
- Mark the run failed when the resume executor errors, returns an empty assistant message, or message append fails.

## Out Of Scope

- Adding a background resume worker.
- Adding resume worker environment flags.
- Changing LangGraph HTTP APIs.
- Changing checkpoint readiness or run creation guards.
- Changing `HarnessExecutor.Execute` or planner behavior.
- Adding Eino orchestration changes.

## Execution Semantics

Queued resume processing now follows:

- claim protected queued resume run
- parse resume command
- load checkpoint
- validate checkpoint ownership and pending sends
- convert checkpoint values to `HarnessResumeInput`
- call Go harness resume executor
- append assistant response
- complete the resume run

The normal run processor still owns fresh runs. The resume processor only claims runs marked with checkpoint resume metadata by the domain claim path.

## Verification

- `go test -count=1 ./application/agentthread -run 'TestResumeRunProcessor|TestLoadHarnessResumeInput'`
- `go test -count=1 ./application/agentthread -run 'TestHarnessExecutorResume'`
- `go test -count=1 ./application/agentthread`
- `go test -count=1 ./domain/agentthread/...`
- `git diff --check`

## Next Phase

Phase 50 should add the background resume worker boundary and startup configuration so protected queued resume runs are processed automatically without manual processor invocation.
