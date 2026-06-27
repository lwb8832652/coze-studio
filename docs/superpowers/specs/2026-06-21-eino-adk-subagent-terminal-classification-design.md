# Eino ADK Subagent Terminal Classification Design

## Goal

M3.15 makes subagent terminal outcomes explicit and durable. Child
`AgentTool` errors are now classified before lifecycle events and child run
status transitions are emitted, so timeout, cancellation, and generic failure
do not collapse into the same persisted state.

## Classification Contract

`ADKSubagentToolProvider` classifies child invocation errors with
`classifyADKSubagentTerminalError`:

- `context.Canceled` maps to `RunStatusCanceled`,
  `subagent.run.canceled`, `subagent_canceled`, and
  `terminal_classification: "canceled"`.
- `context.DeadlineExceeded` maps to `RunStatusFailed`,
  `subagent.run.failed`, `subagent_timeout`, and
  `terminal_classification: "timeout"`.
- all other errors map to `RunStatusFailed`, `subagent.run.failed`,
  `subagent_failed`, and `terminal_classification: "failed"`.

Timeout remains a failed run because it is an execution budget breach, while
explicit cancellation is a user or parent-runtime stop signal.

## Recorder Behavior

`ADKSubagentRunFinishRequest` carries the normalized `ErrorCode`.
`ApplicationADKSubagentRunRecorder` now supports:

- `CompleteRun` for `RunStatusSucceeded`;
- `FailRun` for `RunStatusFailed`;
- `CancelRun` for `RunStatusCanceled`.

The recorder still sanitizes error messages and does not persist tool
arguments, model input, model output, final text, checkpoint bytes, object
URIs, or provider raw content.

## Event Payloads

Lifecycle events remain emitted on the parent run. Failed and canceled events
include content-free terminal metadata:

- `status`;
- `error_code`;
- `terminal_classification`;
- sanitized `error_message`;
- `child_run_id` when a durable child row was created.

## Deferred Work

Open follow-up work:

- parent-to-child cancellation propagation across active model/tool/sandbox
  calls;
- child retry and resume semantics;
- terminal race protection when timeout, cancellation, and provider errors
  arrive close together;
- frontend child-run state cards and usage rollups.
