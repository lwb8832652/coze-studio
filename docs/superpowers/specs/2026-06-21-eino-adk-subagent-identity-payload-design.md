# Eino ADK Subagent Identity Payload Design

## Goal

M3.4 adds a stable subagent identity payload to the Go-native Eino ADK event
mapper. This gives backend events, token usage metadata, and future frontend
state cards the same grouping key before durable child run records are added.

This is not the final parent-child run ledger. It is the compatibility layer
that lets later migrations and UI work reuse one event contract.

## Event Contract

For every ADK event whose `RunPath` has more than one entry, Coze adds:

```json
{
  "subagent": {
    "name": "researcher",
    "root_name": "lead",
    "parent_name": "lead",
    "step_id": "lead/researcher",
    "run_path": ["lead", "researcher"],
    "depth": 1
  }
}
```

The same base fields are generated for message, tool, interrupt,
summarization, customized action, retry, cancellation, and runtime error
events because they all pass through `adkEventBasePayload`.

## Usage Metadata

`newAgentTokenUsage` now persists event-side metadata with:

- `source: "eino_event"`
- `agent_name`
- `run_path`
- `step_id`
- `step_index`
- `subagent` when the event is nested

This metadata intentionally contains no message content, tool result content,
URLs, file names, or provider raw payloads.

Callback-based usage already attributes non-lead model calls as `subagent`.
The event-side metadata closes the remaining gap for model providers that
surface usage through `schema.ResponseMeta.Usage`.

## Deferred Work

Durable parent-child run records remain open:

- child run IDs and checkpoint namespaces;
- parent-child lifecycle transitions;
- cancellation and timeout propagation;
- token cost rollups by child run;
- state-card APIs and frontend rendering.
