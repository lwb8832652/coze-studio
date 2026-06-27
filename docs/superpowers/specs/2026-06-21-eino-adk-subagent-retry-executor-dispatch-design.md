# Eino ADK Subagent Retry Executor Dispatch Design

## Goal

M3.29 introduces the executor capability boundary required before implementing
real subagent retry replay. `subagent_retry` runs must not be processed by
ordinary task execution, but replay-capable executors need a way to take over
without duplicating run completion and failure handling.

## Contract

`RunProcessor` recognizes `command.subagent_retry` after emitting
`run.started`. It then checks whether the selected executor implements:

```go
type SubagentRetryRunExecutor interface {
    ExecuteSubagentRetry(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
}
```

If the executor implements the interface, the processor calls
`ExecuteSubagentRetry` and feeds the result into the same finalization path used
by ordinary execution:

- append assistant message when a non-empty message is returned;
- complete the run when append succeeds;
- map `RunCanceledError` to canceled;
- map `RunInterruptedError` to interrupted;
- map ordinary errors to failed;
- preserve existing `run.completed`, `run.failed`, `run.canceled`, and
  `run.interrupted` event behavior.

If the executor does not implement the interface, the M3.27 fail-closed
behavior remains: the run fails with
`error_code='subagent_retry_not_supported'`.

## Safety

The dispatch layer does not parse source child run payloads or invoke Eino
directly. It only decides whether the selected executor explicitly supports the
retry capability. This keeps legacy and unsupported runtimes from executing a
retry placeholder as a normal empty-message task.

## Follow-Up

The next backend slice should implement `ExecuteSubagentRetry` on ADKExecutor
behind injected source-run resolution and child-agent replay dependencies. That
implementation should load the source child run, parse
`coze.subagent_tool_call.v1`, rebuild the child agent under policy, invoke it
with stored arguments, and emit content-free retry lifecycle events.
