# P2-D-TOKEN-019 Live Token Snapshot Event

## Scope

Close the active task-detail token evidence gap without fabricating token
counts. Coze still uses persisted `/token_usage` rows as the source of truth,
but now emits a bounded `token_usage.snapshot` run event immediately after a
token usage row is recorded successfully.

## DeerFlow Reference

DeerFlow task-detail token UI combines backend usage and message
`usage_metadata` in:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/usage.ts`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/usage-model.ts`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/token-usage-indicator.tsx`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-token-usage.tsx`

The Coze implementation keeps the existing DeerFlow-style top-bar and per-turn
token UI, but supplies a safe runtime snapshot before the next page refresh or
polling cycle.

## Implemented

- `ThreadUsageCollector` accepts an optional `RunEventSink`.
- `NewApplicationHarnessExecutor` injects the same application run-event sink
  used by normal execution events.
- After `ApplicationService.RecordTokenUsage` succeeds, the collector emits
  `token_usage.snapshot`.
- The frontend SSE hook consumes `token_usage.snapshot`, updates aggregate and
  run-scoped token state, and does not render the event as a visible execution
  step.
- The frontend de-duplicates streamed snapshots by `usage_id` or event id.

## Safety Boundary

The snapshot payload includes only bounded metadata:

- `usage_id`, `run_id`
- `source`, `step_id`, `step_index`, `step_name`
- `model_name`, `provider`
- `input_tokens`, `output_tokens`, `total_tokens`
- `cost_micros`, `currency`, `estimated`, `created_at`

The event does not include prompt text, completion text, tool arguments, tool
results, checkpoint bytes, credentials, raw provider bodies, `raw_usage`, or
metadata JSON.

## Verification

Passed:

```bash
cd backend
go test ./application/agentthread -run TestThreadUsageCollectorEmitsSafeTokenUsageSnapshotRunEvent -count=1
go test ./application/agentthread -run 'TestThreadUsageCollector|TestADKUsage' -count=1
go test ./application/agentthread -count=1

cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "streaming token usage snapshots"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token usage|streaming token usage snapshots"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx

git diff --check
```

Additional check:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail-loader.test.ts
```

## Closed Boundary

This slice intentionally stays at the same visible-product layer as the current
DeerFlow task-detail token UI: aggregate and per-turn usage are rendered from
safe usage metadata snapshots. It does not fabricate provider-level
token-by-token deltas. If a provider later exposes safe partial usage callbacks,
that can be added as a separate runtime contract without changing the Workbench
UI data shape.
