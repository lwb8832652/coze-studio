# Eino ADK Callback Tracing And Usage Design

## Goal

Use Eino callbacks as raw runtime telemetry and normalize them into Coze-owned
trace, model metadata, and token usage records.

## Current Baseline

`ADKUsageBridge` already attaches through `adk.WithCallbacks`, records model
callback usage, deduplicates callback usage against message-event usage, and
persists raw provider token counts through `UsageCollector`.

## Scope For M2.16

- Add a stable Coze trace ID for each ADK run.
- Add deterministic span and parent span IDs to callback token-usage metadata.
- Add a bounded model metadata snapshot to callback usage records.
- Add the same trace ID to ADK executor result metadata when a callback bridge
  is active.
- Keep prompt text, model output text, tool arguments, checkpoint bytes, and
  object keys out of usage metadata.

## Trace Shape

The trace fields are deterministic hex strings:

- `trace_id`: 32 hex chars derived from space, thread, run, and runtime label.
- `span_id`: 16 hex chars derived from trace ID and model-call identity.
- `parent_span_id`: 16 hex chars derived from trace ID and agent name.

These IDs are not a full OpenTelemetry implementation yet. They are stable
Coze correlation fields that can be mapped into OpenTelemetry spans later.

## Model Metadata Snapshot

The `model_metadata` object may include:

- `model_name`
- `provider`
- `component`
- `component_name`
- `finish_reason`
- selected call config values such as `max_tokens`, `temperature`, `top_p`,
  and `stop_count`

It must not include prompt content, completion content, raw tool arguments,
credentials, or provider response bodies.

## Deferred

- OpenTelemetry span exporter.
- Provider-aware pricing snapshots and cost calculation.
- Full failover trace graph once the model-candidate provider exists.
- Tool callback usage and retry eligibility taxonomy.
