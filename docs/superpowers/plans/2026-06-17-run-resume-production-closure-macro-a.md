# Macro Stage A - Run/Resume Production Closure

## Frozen Scope

This macro stage closes the production gaps around Go-native run and checkpoint resume execution. It does not add new user-facing routes, frontend pages, LangGraph APIs, Eino orchestration, or unrelated security work.

## Completed In This Slice

- Add a `ResumeRunProcessResult` summary for queued checkpoint resume processing.
- Keep the existing `ProcessQueuedResumeRuns(ctx)` method for compatibility.
- Add `ProcessQueuedResumeRunsWithResult(ctx)` for worker observability and tests.
- Classify per-run outcomes as succeeded, failed, or errored.
- Return resume worker `RunOnce` processing counts to callers.
- Log resume worker claim/process/success/failure counts when work is processed or an error occurs.
- Add a matching `RunProcessResult` summary for normal pending run processing.
- Keep the existing `ProcessPendingRuns(ctx)` method for compatibility.
- Add `ProcessPendingRunsWithResult(ctx)` for normal worker observability and tests.
- Return normal worker `RunOnce` processing counts to callers.
- Classify terminal update failures as processed errors for both normal and resume runs.
- Add operator-facing worker environment, retry, and idempotency guidance.

## Operational Signals

The resume worker now exposes these per-tick counters in process results and logs:

- `claimed`
- `processed`
- `succeeded`
- `failed`
- `errored`

These counters are intentionally local to each worker tick. They can be wired into metrics later without changing processor semantics.

The normal run worker exposes the same counter shape through `RunProcessResult`.

## Deferred Beyond Macro Stage A

- Wire these per-tick process results into a metrics backend when the project selects one.
- Add lease extension if long-running steps start exceeding the domain lease window.
- Add an operator runbook for manual recovery of errored terminal updates.

## Explicitly Deferred To Later Macro Stages

- Skill configuration replication.
- MCP/tool production permissions.
- IM channels.
- Complex security scanning and governance.
- Frontend task experience changes.
