# P1-J-005 RunJournal Message Contract Evidence

Date: 2026-07-01

## Scope

This note closes the P1-J-005 source/runtime/test evidence for the visible
execution-message contract. It verifies that Coze task detail no longer treats
LangGraph checkpoint history or raw runtime events as the primary visible step
source. The visible message/tool-backed steps are projected through a
DeerFlow-style `human` / `ai` / `tool` journal message contract.

## DeerFlow Reference

Verified source anchors:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/threads/hooks.ts:188`
  builds `GET /api/threads/:thread_id/runs/:run_id/messages`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/threads/hooks.ts:1061`
  `useThreadHistory` loads run messages per run and merges them into the thread
  history.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py:384`
  exposes `GET /api/threads/:thread_id/runs/:run_id/messages` and returns
  `{ data, has_more }`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/runtime/journal.py:228`
  records `llm.human.input` with `category="message"`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/runtime/journal.py:294`
  records `llm.ai.response` with `category="message"`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/runtime/journal.py:349`
  records `llm.tool.result` with `category="message"`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/utils.ts:36`
  groups visible messages and filters hidden control messages.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-group.tsx:704`
  derives visible ChainOfThought steps from AI reasoning and tool calls.

Runtime page:

- `http://localhost:2026/workspace/chats/165d8335-5aa2-48ec-8295-54c4a917fce7`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-005-deerflow-message-thinking.png`

Observed safe DOM summary:

- Page title: `Mermaid Diagrams Request and Example - DeerFlow`.
- Visible content includes the user Mermaid prompt, a `思考` block, rendered
  Mermaid diagrams, and token usage.
- Direct browser navigation to DeerFlow API endpoints was blocked by the local
  browser extension with `ERR_BLOCKED_BY_CLIENT`; this evidence therefore uses
  source anchors plus visible runtime DOM, not a raw JSON dump.

## Coze Contract

Verified source anchors:

- `backend/application/agentthread/run_journal_messages.go:63`
  documents that `ProjectRunJournalMessages` normalizes persisted messages and
  ADK events into the DeerFlow message shape before frontend grouping.
- `backend/application/agentthread/run_journal_messages.go:287`
  maps only `message.completed`, `tool.completed`, and `tool.failed` into
  visible journal messages.
- `backend/application/agentthread/run_journal_messages.go:306`
  preserves AI `reasoning_content`, tool calls, and bounded usage metadata.
- `backend/api/handler/coze/workbench_thread_service.go:1334`
  builds `journal_messages` from persisted messages plus already-sanitized
  run events.
- `backend/api/handler/coze/workbench_thread_service.go:1378`
  maps journal messages to the Workbench API response.
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts:279`
  converts journal messages back into task events.
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts:286`
  prefers journal-backed events and filters old raw message/tool event
  projections when journal messages exist.
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts:402`
  uses `runEventsResponse.data?.journal_messages` in task-detail loading.

Runtime page:

- `http://localhost:8080/space/7656275718757679104/tasks/7657273152627539968`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-005-coze-journal-web-search.png`

Observed safe DOM summary:

```json
{
  "chainClass": "coze-prototype-execution-feed coze-prototype-reasoning-panel coze-prototype-chain-of-thought",
  "stepCount": 2,
  "steps": [
    "搜索网页：“青岛最佳旅游时间 几月份去最好”",
    "I got good search results. Let me summarize the key findings about the best time to visit Qingdao..."
  ],
  "visibleUnsafeHits": []
}
```

Direct browser navigation to the Coze `run_events` API was blocked by the local
browser extension with `ERR_BLOCKED_BY_CLIENT`. API contract evidence is covered
by the handler tests below; browser evidence covers the actual rendered page.

## Security Check

The Coze runtime DOM check scanned for these unsafe strings and found no
visible hits:

- `raw_usage`
- `checkpoint`
- `object_uri`
- `tool_result`
- `arguments`
- `credentials`
- `Authorization`

The API projection is built from already-sanitized run events and returns only
bounded journal-message fields. Raw tool results, credentials, signed/object
URIs, checkpoint bytes, raw provider bodies, and token raw usage are not part
of this visible contract.

## Automated Verification

Commands:

```bash
cd backend
go test ./application/agentthread -run TestProjectRunJournalMessages -count=1
go test ./api/handler/coze -run 'TestListTaskThreadRunEventsHandlerReturnsJournalMessages|TestTaskThreadRunEventPayloadKeepsSafe|TestListTaskThreadRunEventsHandlerRedactsUnsafePayload' -count=1 -gcflags="all=-N -l"

cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail-loader.test.ts
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Result:

- `application/agentthread`: passed.
- `api/handler/coze`: passed.
- `task-detail-loader.test.ts`: 1 test passed.
- `task-detail.test.tsx`: 49 tests passed. The known unrelated
  `localhost:3000` logger noise appeared, but the suite passed.
- `tsc --noEmit`: passed.

## Remaining Follow-Up

P1-J-005 is complete for the message contract. Remaining execution-flow work is
tracked separately:

- P1-J-006: paired artifact/final-answer ordering evidence.
- P1-J-007: streaming stop-square, loading dots, retry/cancel, and follow-up
  runtime affordance evidence.
- P1-J-008: broader paired ChainOfThought visual screenshots across canonical
  prompts.
