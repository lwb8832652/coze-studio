# Phase 19: Agent Run Event Emission

## Goal

Wire the Phase 18 `agent_run_events` persistence into the Go-native run execution path so task detail pages can query a durable execution timeline after a worker processes a run.

This phase stays backend-only. It does not add SSE/WebSocket streaming, frontend rendering, MCP tool execution, skill management, memory retrieval, IM channels, or LangGraph compatibility APIs.

## Scope

- Add an application-layer run event sink that persists events through `ApplicationService.AppendRunEvent`.
- Emit run lifecycle events from `RunProcessor`:
  - `run.started` before executor invocation.
  - `run.completed` after the run is marked succeeded.
  - `run.failed` before the run is marked failed for executor, empty result, or append-message errors.
- Emit harness step events from `HarnessExecutor`:
  - `step.started` before each planned step runs.
  - `step.completed` after a step returns a non-empty result.
  - `step.failed` when the step runner returns an error or empty result.
- Wire the production worker startup so the default harness and processor share an event sink backed by the agent thread application service.

## Event Semantics

Event emission is best-effort in this phase. A persistence failure is logged but does not make an otherwise valid run fail. This keeps the worker resilient when the event table or database is temporarily degraded, while the event sink remains injectable for tests and future stricter policies.

Run event payloads are JSON objects with stable fields:

- `run.started`: `status`, `worker_id`.
- `run.completed`: `status`, `worker_id`.
- `run.failed`: `status`, `worker_id`, `error_code`, `error_message`.

Step event payloads are JSON objects with stable fields:

- `step_id`, `step_type`, `step_name`, `step_index`.
- `final` and `message_present` on `step.completed`.
- `error_message` on `step.failed`.

## Acceptance Criteria

- `RunProcessor` emits started/completed events on successful claimed runs.
- `RunProcessor` emits started/failed events when execution fails.
- `HarnessExecutor` emits step started/completed events on successful final steps.
- `HarnessExecutor` emits step started/failed events when a step runner fails.
- Existing worker env behavior remains unchanged.
- Targeted Go tests pass for `backend/application/agentthread`, `backend/domain/agentthread/service`, and `backend/domain/agentthread/repository`.
