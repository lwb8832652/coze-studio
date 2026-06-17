# Phase 45 - Checkpoint Resume Handoff Boundary

## Goal

Add the first internal handoff boundary for queued checkpoint-resume runs. The boundary lets a future replay executor claim only protected resume runs, without changing the normal pending-run worker path.

## Scope

- Add `ClaimQueuedResumeRuns` at repository, domain service, and application service layers.
- Claim only runs with:
  - `status = queued`
  - `metadata.checkpoint_resume` present
- Transition claimed resume runs to `running`.
- Set `worker_id`, `started_at`, and `updated_at` during claim.
- Preserve `ClaimPendingRuns` behavior exactly as-is.
- Add tests proving:
  - normal queued runs are not claimed by the resume path
  - pending resume-marked runs are not claimed by the resume path
  - queued resume runs are not claimed by the normal pending path
  - worker ID validation and limit normalization remain in the domain layer

## Out Of Scope

- Running graph replay.
- Loading checkpoint values into the Go Agent Harness.
- Starting execution from `pending_sends`.
- Adding a background resume worker loop.
- Exposing resume claim APIs over HTTP.
- New database migrations.

## Handoff Semantics

The dedicated claim path is intentionally separate from `ClaimPendingRuns`:

- normal worker: `pending -> running`
- future resume worker: `queued + checkpoint_resume -> running`

This separation keeps queued resume records durable and visible after Phase 44, while preventing the existing worker from executing them as fresh work. A replay executor can later call the dedicated application method after it is ready to rebuild state from checkpoint values and pending sends.

## Verification

- `go test -count=1 ./domain/agentthread/repository -run 'TestThreadRepositoryClaim(QueuedResumeRunsMarksOldestResumeRunsRunning|PendingRunsSkipsQueuedResumeRuns)'`
- `go test -count=1 ./domain/agentthread/service -run 'TestClaimQueuedResumeRuns(RequiresWorkerID|NormalizesLimit)'`
- `go test -count=1 ./application/agentthread -run TestApplicationClaimQueuedResumeRunsMapsDomainRuns`
- `go test -count=1 ./application/agentthread ./domain/agentthread/...`
- `go test -count=1 ./api/handler/coze -run 'TestLangGraph(Thread|Run|Checkpoint)'`
- `go test -count=1 ./api/router/coze -run 'TestRegisterIncludesLangGraph'`

## Next Phase

Phase 46 should add a resume processor skeleton that consumes `ClaimQueuedResumeRuns`, validates checkpoint payload availability, emits a clear run event, and fails safely with `checkpoint_replay_not_implemented` until graph state replay is implemented.
