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
display as the production baseline.

This gap is now closed by `P2-D-TOKEN-019`: Coze emits a safe
`token_usage.snapshot` run event only after usage is persisted, then the UI
merges that snapshot into the existing DeerFlow-style token display.

## Follow-Up Contract

P2-D added a bounded `token_usage.snapshot` runtime event with:

- thread id and run id;
- source category;
- input/output/total deltas or current snapshot;
- no raw provider body, prompt, completion, tool args/results, or credentials.

The UI merges the active run usage with persisted usage while keeping
`token_usage.snapshot` out of the visible execution-step stream.
