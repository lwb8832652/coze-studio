# Phase 41 - Agent Harness Checkpoint Writes

## Goal

Make the Go native Agent Harness write durable checkpoints while a run executes, so the LangGraph-compatible state and history APIs can read real harness snapshots instead of only reconstructed message or event fallback data.

## Scope

- Add a harness-level `CheckpointSink` abstraction.
- Add an application adapter that persists checkpoints through the existing agent thread application service.
- Wire the production run worker through a single application-level harness constructor.
- Write checkpoints at run start, after each model/tool step, and after successful terminal output.
- Store LangGraph-style `channel_values`, `channel_versions`, `pending_sends`, and checkpoint metadata.
- Preserve existing run event, memory recall, and token usage behavior.

## Out Of Scope

- Resume, rollback, or update-state execution from checkpoint.
- Failure-terminal checkpoints for runner/planner/max-step errors.
- Frontend execution-flow UI changes.
- IM channel integration.
- MCP tool configuration changes.
- Eino graph checkpoint integration beyond the current harness state snapshots.
- Security scanning or governance changes.

## Checkpoint Shape

- `harness.initial` captures input messages, recalled memory, empty artifacts, empty todos, no executed steps, and status `running`.
- `harness.step` captures model step output as assistant messages, step records, pending remaining steps, and status `running`.
- `harness.tool` captures tool output as tool messages and `tool_results`, pending remaining steps, and status `running`.
- `harness.terminal` captures the final successful state with status `succeeded`.

## Application Wiring

`NewApplicationHarnessExecutor` is the production constructor for run workers. It configures the default planner/runner plus durable event, memory, usage, and checkpoint sinks. `application.Init` now uses this constructor so worker-executed runs write checkpoints automatically.

## Verification

- `go test -count=1 ./application/agentthread -run 'TestNewApplicationHarnessExecutorConfiguresDurableSinks'`
- `go test -count=1 ./application/agentthread`

## Next Phase

Phase 42 should focus on failure checkpoints and resume-readiness semantics, including deterministic checkpoint metadata for planner errors, runner errors, cancellation, and max-step termination.
