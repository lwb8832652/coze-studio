# Eino ADK Subagent Retry Executor Guard Design

## Goal

M3.27 adds an execution-time safety guard for the M3.26 subagent retry
contract. The retry API can now create a queued top-level task run carrying
`command.subagent_retry`, but the real replay executor is not implemented yet.
The worker must therefore fail that placeholder run closed instead of letting
the ordinary task executor process it as an empty-message run.

## Runtime Behavior

`RunProcessor.processRun` emits the existing `run.started` event, inspects the
claimed run command, and treats a non-null top-level `subagent_retry` field as
a guarded retry command. When detected, the processor:

- does not invoke the normal `RunExecutor`;
- does not append an assistant message;
- does not mark the run succeeded;
- marks the run failed from `running` with
  `error_code='subagent_retry_not_supported'`;
- emits the existing content-free `run.failed` event with the fixed error code
  and short fixed message.

Invalid JSON, empty commands, missing `subagent_retry`, and explicit
`"subagent_retry":null` remain ordinary commands so legacy and resume behavior
does not change.

## Safety

The guard must not copy command fields such as `source_run_id`,
`parent_run_id`, prompts, child output, tool arguments, checkpoint bytes, URLs,
object keys, or provider payloads into failure events. The persisted command
still remains on the retry run for the future replay executor, but event
payloads stay metadata-only.

## Future Replay Hook

Future work should replace the fail-closed branch with a dedicated replay
executor path that reconstructs the child/subagent invocation from the durable
retry command, policy snapshot, parent run, and source child run. That replay
executor must preserve top-level worker claim semantics and continue to keep
retry lifecycle events content-free.
