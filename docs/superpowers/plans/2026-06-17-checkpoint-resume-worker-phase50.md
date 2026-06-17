# Phase 50 - Checkpoint Resume Worker Boundary

## Goal

Add a background worker boundary for protected queued checkpoint resume runs. The worker should be opt-in through dedicated environment variables and should process only the queued resume claim path introduced in earlier phases.

## Scope

- Add `ResumeRunWorker` beside the existing `RunWorker`.
- Add `RunOnce` and ticker-based `Start` behavior for resume runs.
- Add `StartResumeRunWorkerFromEnv`.
- Use dedicated `AGENT_THREAD_RESUME_WORKER_*` environment variables.
- Keep the resume worker disabled by default.
- Require an explicit resume executor when starting from environment.
- Wire application startup to initialize the resume worker with the Go-native harness executor.

## Out Of Scope

- Changing normal pending run worker behavior.
- Changing LangGraph run creation, resume readiness, or checkpoint APIs.
- Adding worker metrics, distributed rate limiting, or lease extension.
- Adding frontend changes.
- Changing Eino orchestration or tool execution semantics.

## Environment Variables

- `AGENT_THREAD_RESUME_WORKER_ENABLED`
- `AGENT_THREAD_RESUME_WORKER_ID`
- `AGENT_THREAD_RESUME_WORKER_BATCH_SIZE`
- `AGENT_THREAD_RESUME_WORKER_INTERVAL_MS`

## Execution Semantics

The normal worker still claims ordinary pending runs. The resume worker claims only protected queued resume runs through `ClaimQueuedResumeRuns`, then delegates to `ResumeRunProcessor`, which loads the checkpoint and calls the Go harness resume executor.

The worker is intentionally disabled by default. Production deployment can enable it separately from the normal run worker after checkpoint resume behavior is verified in the target environment.

## Verification

- `go test -count=1 ./application/agentthread -run 'TestResumeRunWorker|TestRunWorker'`
- `go test -count=1 ./application/agentthread`
- `go test -count=1 ./application -run Test`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`
- `git diff --check`

## Next Phase

Phase 51 should add resume worker production controls and observability, including explicit logs/events for claim counts, process errors, and safe operator-facing configuration guidance.
