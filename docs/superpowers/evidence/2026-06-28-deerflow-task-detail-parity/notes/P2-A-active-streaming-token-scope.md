# P2-A Active Streaming Token Scope

## DeerFlow Reference

- DeerFlow token display combines backend usage and message `usage_metadata`.
- Source paths:
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/usage.ts`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/usage-model.ts`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/token-usage-indicator.tsx`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-token-usage.tsx`

## Coze Current State

- Coze task detail token display is backed by persisted
  `GetTaskThreadTokenUsage` rows and run-scoped aggregation.
- Existing tests cover:
  - top-bar token popover;
  - every-turn assistant token summaries;
  - debug summary without raw provider payloads;
  - subagent token summaries.
- No live streaming token usage event is currently available in the Workbench
  run event stream, so the frontend cannot honestly show changing in-flight
  token totals during a running response.

## Decision

Do not fabricate live token usage in the UI. Keep persisted/per-run token
display as the production baseline and defer active streaming token evidence to
P2-D observability/runtime event work.

## Follow-Up Contract

P2-D should add a bounded runtime event such as `token_usage.delta` or
`token_usage.snapshot` with:

- thread id and run id;
- source category;
- input/output/total deltas or current snapshot;
- no raw provider body, prompt, completion, tool args/results, or credentials.

The UI can then merge the active run usage with persisted usage and capture
browser evidence while a run is still streaming.
