# Eino ADK Subagent Retry Executor Replay Design

## Goal

M3.31 gives ADKExecutor a real subagent retry replay path. Earlier slices
created the retry run, persisted the child invocation input and definition
snapshot, and added a run-processor dispatch boundary. This slice makes the ADK
executor consume those internal contracts.

## Replay Flow

`ADKExecutor.ExecuteSubagentRetry`:

1. validates the retry run;
2. parses `command.subagent_retry` with schema `coze.subagent_retry.v1`;
3. loads the source child run through injected
   `ADKSubagentRetrySourceResolver`;
4. validates source run ID, thread ID, parent run ID, and `run_kind='subagent'`;
5. parses source child `input` schema `coze.subagent_tool_call.v1`;
6. parses source child config for `agent_name` and `full_chat_history`;
7. rebuilds the child agent through the existing ADK agent factory;
8. invokes the child through Eino `adk.NewAgentTool`.

The executor returns the AgentTool text result as the retry run assistant
message. Metadata includes only `source='eino_adk_subagent_retry'`,
`source_run_id`, and `parent_run_id`.

## Safety

The retry executor must not hand-roll child argument-to-message conversion.
Eino AgentTool owns that behavior. The executor must not expose tool
arguments, prompts, model input/output, checkpoint bytes, object URIs, URLs,
filenames, or provider payloads in events or metadata. Invalid schema, mismatched
source identity, non-subagent source rows, or malformed arguments fail before
child agent invocation.

## Follow-Up

RuntimeSelector still needs to route `ExecuteSubagentRetry` to the ADK executor
when the retry run is configured for `eino_adk`; unsupported runtimes must
continue to fail closed. Later slices should add retry-result joins, frontend
retry controls, recursive descendant policy, and browser visual QA.
