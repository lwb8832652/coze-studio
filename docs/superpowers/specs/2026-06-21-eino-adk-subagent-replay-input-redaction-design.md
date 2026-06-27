# Eino ADK Subagent Replay Input And API Redaction Design

## Goal

M3.28 prepares subagent retry replay without exposing child tool-call payloads
through Workbench APIs. Subagent child rows need enough internal data to replay
the original `AgentTool` invocation, while task detail clients should continue
to receive only safe child-run metadata.

## Internal Replay Input

`adkSubagentLifecycleTool` receives the original `argumentsInJSON` passed to
the Eino `AgentTool`. When a child run recorder is configured, it forwards that
JSON to `ApplicationADKSubagentRunRecorder.StartADKSubagentRun`.

The recorder stores child run input as:

```json
{
  "schema": "coze.subagent_tool_call.v1",
  "tool_name": "researcher",
  "arguments": {"request": "find context"}
}
```

The arguments must be valid JSON. Empty arguments are normalized to `{}`. The
payload is an internal execution contract for a future retry replay executor;
it is not a public task detail contract.

## API Redaction

Workbench `TaskThreadRun` responses for `run_kind='subagent'` redact:

- `command`;
- `input`;
- `config`;
- `context`.

The mapper preserves fields required by child cards: run IDs, parent run ID,
assistant ID, run kind, status, metadata, worker/error fields, stream settings,
idempotency key, and timestamps. Top-level `run_kind='task'` responses keep the
existing command/input/config/context behavior.

## Safety

Subagent lifecycle events still must not include tool arguments, model
input/output, final child text, checkpoint bytes, object URIs, URLs, filenames,
or provider payloads. Persisted replay input remains available only inside the
backend execution path and must be consumed through a future policy-checked
retry executor.

## Follow-Up

The next backend slice can replace the M3.27 fail-closed branch with a replay
executor that loads the source child run, reads `coze.subagent_tool_call.v1`,
rebuilds the child agent/tool with the recorded definition, and invokes it with
the stored arguments under the same policy and lifecycle recorder boundaries.
