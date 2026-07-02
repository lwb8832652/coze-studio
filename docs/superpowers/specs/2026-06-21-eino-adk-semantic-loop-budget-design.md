# Eino ADK Semantic Loop And Budget Design

> 2026-07-01 P1 DeerFlow parity update: the original M2 design made semantic
> loop detection opt-in and terminal-error oriented. P1-J-003 supersedes that
> default for repeated tool calls: Coze now enables DeerFlow-style warn/hard
> thresholds by default, injects a `loop_warning` user message before the next
> model call, and hard-stops by clearing repeated assistant tool calls so the
> model can finalize. The content-free `run.semantic_loop_detected` path remains
> only for explicit legacy limits and assistant-text loop detection.

## Goal

Add Coze-owned semantic loop detection around Eino ADK task execution without
duplicating Eino's native agent loop or `MaxIterations` budget.

## Scope For M2.14

- Keep Eino `ChatModelAgentConfig.MaxIterations` as the hard model-call budget.
- Add a Coze ADK handler that detects repeated Assistant actions after each
  model response.
- Detect repeated tool-call plans even when tool-result messages sit between
  Assistant messages.
- Detect repeated final Assistant text responses.
- Emit a content-free Coze runtime event when a semantic loop is detected.
- Keep raw tool arguments, model text, transcript content, and object paths out
  of loop events.

## Configuration

Semantic loop detection is disabled unless a run config contains
`semantic_loop` or `semanticLoop`.

```json
{
  "semantic_loop": {
    "max_repeated_tool_calls": 2,
    "max_repeated_assistant_messages": 2
  }
}
```

Supported fields:

- `max_repeated_tool_calls` / `maxRepeatedToolCalls`
- `max_repeated_assistant_messages` / `maxRepeatedAssistantMessages`

The value means how many consecutive identical Assistant actions may be
allowed. The middleware stops the run when the observed count is greater than
the configured limit. A zero or missing value disables that detector. Each
limit is bounded to `0-20`.

`max_iterations` remains the hard Eino execution budget and is still passed
directly to `ChatModelAgentConfig.MaxIterations`. The semantic-loop middleware
is a softer production guard for repeated behavior before the hard budget is
exhausted.

## Tool-Call Signature

The tool-call detector hashes a normalized action signature made from:

- the ordered tool-call list;
- each tool name;
- canonicalized JSON arguments when valid JSON is provided;
- trimmed raw arguments only when JSON parsing fails.

It intentionally excludes tool call IDs, indexes, and raw Eino runtime fields
because those can vary between retries and should not determine semantic
identity.

The reverse scan skips `tool` messages so the common pattern:

1. Assistant calls tool A;
2. tool A returns;
3. Assistant calls tool A again;

is counted as repeated Assistant tool-call behavior. The scan stops at a
`user` or `system` message, so a new turn starts a new semantic scope.

## Assistant-Text Signature

The text detector hashes normalized Assistant text output from:

- `Message.Content`;
- text parts in `AssistantGenMultiContent`.

Whitespace is collapsed. Assistant messages with tool calls are left to the
tool-call detector. Empty Assistant messages are ignored and may still be
handled by the model retry policy when explicitly configured.

## Error And Event Mapping

When a detector trips, the middleware returns `ADKSemanticLoopError`.
`MapADKEvent` maps that error to:

```json
{
  "agent_name": "lead",
  "kind": "tool_calls",
  "signature": "sha256:...",
  "count": 3,
  "limit": 2,
  "error": "semantic loop detected: tool_calls repeated 3 times (limit 2)"
}
```

The event type is `run.semantic_loop_detected`.

The payload is intentionally content-free. It contains no tool arguments, no
model text, no transcript content, no object keys, and no checkpoint bytes.

## Middleware Placement

Place `semantic_loop` after tool-result repair/error normalization and before
policy, audit, and usage middleware. This lets the detector see the model state
that will be persisted, while still allowing Coze policy/audit layers to handle
the resulting terminal error event through the normal event mapper.

## Deferred

- Provider-specific loop reasons and model vendor finish metadata.
- Cross-run loop analytics and automatic prompt repair.
- Failover candidate selection. M2.13a intentionally left real failover open
  until the Coze model-candidate provider exists.
- UI controls for semantic-loop limits.
