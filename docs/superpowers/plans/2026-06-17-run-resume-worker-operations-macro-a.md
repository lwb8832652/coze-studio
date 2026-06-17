# Macro Stage A - Run/Resume Worker Operations

## Purpose

This document is the operator-facing guide for the Go-native task run and checkpoint resume workers. It belongs to Macro Stage A and does not create a new phase.

## Worker Switches

Normal pending runs:

- `AGENT_THREAD_WORKER_ENABLED`: enables the normal run worker. Default is disabled.
- `AGENT_THREAD_WORKER_ID`: worker lease identity. Default is `agent-harness`.
- `AGENT_THREAD_WORKER_BATCH_SIZE`: maximum pending runs claimed per tick. Default is `10`.
- `AGENT_THREAD_WORKER_INTERVAL_MS`: worker tick interval. Default is `2000`.

Checkpoint resume runs:

- `AGENT_THREAD_RESUME_WORKER_ENABLED`: enables the checkpoint resume worker. Default is disabled.
- `AGENT_THREAD_RESUME_WORKER_ID`: worker lease identity. Default is `agent-harness-resume`.
- `AGENT_THREAD_RESUME_WORKER_BATCH_SIZE`: maximum queued resume runs claimed per tick. Default is `10`.
- `AGENT_THREAD_RESUME_WORKER_INTERVAL_MS`: worker tick interval. Default is `2000`.

## Deployment Guidance

- Enable the normal worker first.
- Enable the resume worker only after checkpoint writes and resume readiness are verified in the target environment.
- Use separate worker IDs for normal and resume workers.
- Start with a small resume batch size in production until retry behavior is observed.
- Keep both workers disabled in environments that only exercise API persistence without background execution.

## Per-Tick Signals

Both workers expose the same process result shape:

- `claimed`: runs claimed by the domain claim path.
- `processed`: runs that reached a terminal processor outcome.
- `succeeded`: runs completed successfully.
- `failed`: runs intentionally marked failed by the processor.
- `errored`: runs where processing returned an infrastructure or terminal update error.

The worker logs these counters when a tick processes work or returns an error.

## Retry And Idempotency Boundary

- Claiming is lease-based through the domain service. A run should only be processed by the worker that owns the current worker ID lease.
- Processor failures that successfully call `FailRun` are terminal failures. They are counted as `failed`, not retried by the same queued state.
- Infrastructure failures before terminal status is updated are counted as `errored`. Operators should inspect the run status before retrying.
- If a resume run appended an assistant message but `CompleteRun` failed, the result is counted as `processed=1` and `errored=1`. This indicates a terminal update risk rather than an executor failure.
- Resume execution is checkpoint-based. Retrying a resume run should use the persisted checkpoint state and pending sends, not rebuild a planner result.

## Current Safety Limits

- The workers do not automatically requeue errored runs.
- The workers do not extend leases mid-step.
- The workers do not expose external metrics yet; the process result and structured logs are the integration boundary for later metrics.
- The workers do not alter LangGraph API behavior or frontend task routes.
