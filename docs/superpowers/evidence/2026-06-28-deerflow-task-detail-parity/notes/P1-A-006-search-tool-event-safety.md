# P1-A-006 Search / Tool Event Safety

## Scope

This evidence verifies that a web-search task shows useful visible step labels
while hiding unsafe raw tool/provider data. Skills and MCP deep validation are
not repeated here because P0 accepted those workflows for launch and deeper
policy/history work remains under P1-F/P2.

## DeerFlow Comparator

- Reference task:
  `http://localhost:2026/workspace/chats/72b6d7d7-ac4e-4899-80e3-86ece96e3ce7`
- P1-A screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-A-006-deerflow-search-tool-safe-steps.jpg`

DeerFlow visible behavior: web-search/page-view style steps are visible as
bounded step labels, followed by a normal final answer and token row.

## Coze Runtime Evidence

- Task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Runtime screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-006-coze-search-tool-safe-steps.png`
- Search-answer reference screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-006-coze-search-answer-reference.jpg`
- Safe DOM scan summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-006-search-tool-event-safety-summary.json`

Safe scan result:

- Search/reasoning step is visible.
- Token text is visible.
- Unsafe visible-pattern hits: none.

The scan stores only booleans, counts, URL, and regex hit names. It does not
retain page text, prompt text, model completions, tool arguments, tool results,
provider payloads, credentials, object URIs, signed URLs, or checkpoint bytes.

## Source Contract

- Frontend safe display:
  - `frontend/apps/coze-studio/src/pages/tasks/task-tool-event-safety.ts`
  - `frontend/apps/coze-studio/src/pages/tasks/task-event-tool-display.ts`
  - `frontend/apps/coze-studio/src/pages/tasks/task-event-projection.ts`
- Backend safe event projection:
  - `backend/api/handler/coze/workbench_thread_service.go`
  - `backend/application/agentthread/run_journal_messages.go`

## Automated Verification

Commands:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx -t "formats agent tool events|hides unsafe tool event details"

cd backend
go test ./api/handler/coze -run 'TestListTaskThreadRunEventsHandlerRedactsUnsafePayload|TestTaskThreadRunEventPayloadKeepsSafeWebSearchQuery|TestTaskThreadRunEventPayloadKeepsSafeAssistantToolCallSummary' -count=1 -gcflags="all=-N -l"
```

Result: both commands passed.

Covered behavior:

- Tool events become bounded execution-step labels.
- Unsafe tool event details are hidden from visible task cards.
- Backend run event payloads redact unsafe raw fields.
- `web_search` may keep a safe query summary while omitting URLs,
  credentials, raw results, and provider payloads.

## Remaining Gap

No open P1-A-006 gap remains for web-search/tool event visible safety. MCP
provider policy/history hardening remains under P1-F and P2.
