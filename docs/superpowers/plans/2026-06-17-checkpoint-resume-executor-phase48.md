# Phase 48 - Checkpoint Resume Executor Boundary

## Goal

Add a Go Harness resume execution boundary that consumes `HarnessResumeInput` and runs only the pending steps from a checkpoint. This keeps replay execution behind existing planner/runner abstractions without changing the queued resume worker flow yet.

## Scope

- Add `HarnessExecutor.Resume`.
- Require the resume input to belong to the current run.
- Start execution from the loaded `AgentHarnessState`.
- Run only `HarnessResumeInput.PendingSteps`.
- Do not call the planner during resume execution.
- Emit the same step events used by normal harness execution.
- Record usage through the existing usage collector path.
- Write step checkpoints with the selected checkpoint as the parent.
- Write terminal success or failure checkpoints through the existing checkpoint sink.
- Preserve pending sends on failure.

## Out Of Scope

- Connecting `ResumeRunProcessor` to `HarnessExecutor.Resume`.
- Starting a background resume worker.
- Adding HTTP APIs.
- Changing normal `HarnessExecutor.Execute`.
- Changing model or tool execution semantics.
- Adding Eino orchestration changes.

## Execution Semantics

Normal execution still follows:

- planner -> step runner -> checkpoints

Resume execution follows:

- loaded checkpoint state -> pending step runner -> checkpoints

The planner is intentionally skipped. The pending step list is the replay plan for this phase. If a pending step fails, the executor writes a terminal failed checkpoint whose pending sends include the failed step and remaining steps.

## Verification

- `go test -count=1 ./application/agentthread -run 'TestHarnessExecutorResume'`
- `go test -count=1 ./application/agentthread -run 'TestHarnessExecutor'`
- `go test -count=1 ./application/agentthread ./domain/agentthread/...`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`

## Next Phase

Phase 49 should wire `ResumeRunProcessor` to `HarnessExecutor.Resume`, replacing the safe replay-not-implemented failure with actual pending-step replay while keeping the dedicated queued-resume claim path.
