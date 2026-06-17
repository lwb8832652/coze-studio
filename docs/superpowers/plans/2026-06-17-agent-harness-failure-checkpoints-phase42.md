# Phase 42 - Agent Harness Failure Checkpoints

## Goal

Make failed, canceled, and max-step-terminated Go Agent Harness runs write a terminal checkpoint. This prevents LangGraph-compatible state and history reads from being left at the last `running` checkpoint after execution exits with an error.

## Scope

- Write a `harness.terminal` checkpoint for planner errors.
- Write a `harness.terminal` checkpoint for step runner errors.
- Write a `harness.terminal` checkpoint for context cancellation during harness execution.
- Write a `harness.terminal` checkpoint when max steps are exceeded.
- Preserve the original execution error as the primary returned error.
- Include deterministic terminal metadata:
  - `checkpoint_phase`
  - `status`
  - `error_type`
  - `error_message`
  - step identity when a specific step failed or remains pending
- Preserve pending sends for failed or skipped planned steps to support future resume-readiness.

## Out Of Scope

- Actually resuming execution from a checkpoint.
- Rollback, fork, or update-state APIs.
- Retrying failed steps automatically.
- Adding frontend execution-flow states.
- Changing worker retry policy.
- Changing checkpoint schema or migration files.

## Failure Semantics

- Planner errors produce `status: failed`, `error_type: planner_error`, and no pending sends.
- Empty planner results produce `status: failed`, `error_type: planner_no_steps`, and no pending sends.
- Step runner errors produce `status: failed`, `error_type: step_error`, and pending sends containing the failed step plus remaining planned steps.
- Empty step results produce `status: failed`, `error_type: step_empty_result`, and pending sends containing the failed step plus remaining planned steps.
- Empty final messages produce `status: failed`, `error_type: step_empty_final_message`, and pending sends containing the failed step plus remaining planned steps.
- Context cancellation produces `status: canceled`, `error_type: context_canceled`.
- Max-step termination produces `status: failed`, `error_type: max_steps_exceeded`.

## Verification

- `go test -count=1 ./application/agentthread -run 'TestHarnessExecutorWrites(FailedCheckpointWhenPlannerErrors|FailedCheckpointWhenRunnerErrors|CanceledCheckpointWhenRunnerIsCanceled|FailedCheckpointWhenMaxStepsExceeded)'`
- `go test -count=1 ./application/agentthread`

## Next Phase

Phase 43 should introduce checkpoint-backed resume-readiness APIs or execution guards without yet attempting full graph replay. The key next decision is whether resume starts from pending sends only, from full channel values, or from an explicit run command that selects a checkpoint.
