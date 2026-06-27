# Eino ADK Tool Repair And Error Normalization Design

## Goal

Bring the Eino ADK runtime closer to production DeerFlow 2.x task execution by making tool-call gaps and tool failures recoverable, observable, and stable across backend events.

## Scope

- Keep Eino `patchtoolcalls` as the source algorithm for repairing dangling tool calls.
- Add Coze-owned tool result schemas so patched and failed tool results are stable public contracts.
- Normalize sync, streaming, enhanced sync, and enhanced streaming tool endpoint errors into model-visible tool results.
- Map normalized tool failures from ADK events to Coze `tool.failed` run events.
- Keep product naming as “任务”; these are execution events inside a run, not top-level task records.

## Non-Goals

- Do not implement retry/failover in this slice.
- Do not add IM Channels.
- Do not expose host filesystem or shell tools.
- Do not change Eino internals or vendor code.

## Runtime Contract

### Dangling Tool-Call Repair

Use `patchtoolcalls.New` with `PatchedContentGenerator`. The generator returns JSON:

```json
{
  "schema": "coze.tool_repair.v1",
  "status": "patched",
  "tool_name": "search_docs",
  "tool_call_id": "call-1",
  "reason": "missing_tool_result",
  "message": "Tool result was missing and has been patched by Coze runtime. Continue with available context or retry the tool if needed."
}
```

The same generator emits a content-free `tool.repaired` run event through the Coze `RunEventSink` when a run is available. The event payload includes only schema, tool name, tool call id, and reason.

### Tool Error Normalization

Tool endpoint errors are converted to a normal tool result:

```json
{
  "schema": "coze.tool_error.v1",
  "status": "failed",
  "tool_name": "search_docs",
  "tool_call_id": "call-1",
  "error_message": "upstream timeout",
  "recoverable": true,
  "normalized": true
}
```

The JSON is bounded and sanitized:

- Empty errors become `tool call failed`.
- Error text is trimmed.
- Error text is truncated before entering tool output.
- Stack traces, object storage keys, and internal payloads must not be added.

### Event Mapping

`MapADKEvent` continues to map normal tool messages to `tool.completed`. If the tool message content, or enhanced tool text part, decodes as `coze.tool_error.v1`, it maps to `tool.failed` and includes the decoded payload under `tool_error`.

Enhanced tool events from Eino use `UserInputMultiContent`. Coze event mapping must extract text parts from `UserInputMultiContent` before schema detection, otherwise enhanced tool failures would be invisible to the frontend.

## Middleware Order

Add `tool_error_normalization` after Eino reduction and before Coze policy/audit/usage reserved middleware in `adkMiddlewareOrder`.

This gives the normalizer an inner position relative to reduction, so reduction still sees a normal successful result after an endpoint error is converted. Eino's event sender wrapper remains outermost and emits the normalized result event.

## Test Gates

- Schema helper tests for stable JSON, truncation, and decode.
- Middleware tests for repair content and `tool.repaired` event.
- Middleware tests for invokable, streamable, enhanced invokable, and enhanced streamable error normalization.
- Event mapper tests for `tool.failed` and enhanced text extraction.
- Focused package test:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
