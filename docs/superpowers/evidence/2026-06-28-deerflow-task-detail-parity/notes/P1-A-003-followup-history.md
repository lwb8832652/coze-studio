# P1-A-003 Follow-up Preserves History

## Scope

This evidence records the Coze side of the follow-up history regression
baseline. It reuses the canonical search/revision task already paired with
DeerFlow under P1-J-008, then adds a P1-A specific screenshot and API/source
contract notes.

## Coze Runtime Evidence

- Task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-003-coze-followup-history.png`
- Safe visible summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-003-coze-followup-history-summary.json`
- API capture attempt:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/network/coze/P1-A-003-followup-history-api-summary.json`

Observed safe behavior:

- The task title remains a short generated title rather than the full user
  prompt.
- The first user turn, first assistant run, follow-up user turn, and follow-up
  assistant run are all visible in the same task-detail stream.
- The first run still exposes its search/reasoning step after the follow-up.
- The follow-up run exposes its own reasoning step and answer.
- Token usage remains visible after the follow-up.
- No artifact cards are expected for this sample because the task is a
  search/revision task, not a document-generation task.

## DeerFlow Comparator

Existing paired DeerFlow evidence for the same prompt family is recorded in:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-J-008-chain-of-thought-style.md`
- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-008-deerflow-revision-followup.jpg`

The parity point is the same for this P1-A case: the follow-up is grounded in
the previous turn, previous visible steps remain available, token usage remains
visible, and unsafe tool/provider payloads are not rendered.

## API And Source Contract

Coze task-detail loading uses:

- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
  - calls `listTaskThreadRuns`
  - calls `listTaskThreadRunEvents`
  - passes `runEventsResponse.data?.journal_messages` into
    `mergeJournalTaskEvents`
- `backend/api/handler/coze/workbench_thread_service.go`
  - handles `GET /api/workbench/task_threads/:thread_id/run_events`
  - returns bounded `events`, `total`, and `journal_messages`
- `backend/api/handler/coze/workbench_thread_service_test.go`
  - `TestListTaskThreadRunEventsHandlerReturnsJournalMessages`
  - `TestListTaskThreadRunEventsHandlerRedactsUnsafePayload`

The in-app browser's read-only page scope did not expose `fetch` or
`XMLHttpRequest`, so direct authenticated API capture was not available in this
turn. The recorded API summary therefore contains the failed capture metadata
only, while the response contract remains covered by source anchors and handler
tests.

## Acceptance Decision

P1-A-003 is accepted for search/revision follow-up history with an explicit
authenticated API capture N/A reason recorded in:

`docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-001-003-authenticated-api-capture-decision.md`

Artifact continuity is intentionally not closed here because this sample has
no artifacts; it remains under P1-A-005 artifact side-preview and the existing
P1-J-006 artifact semantics evidence.
