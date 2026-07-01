# P1-A-008 Loading And Error States

## Scope

This evidence covers the P1-A loading/error baseline for task detail:
DeerFlow-style loading skeleton, bounded not-found/forbidden-style page error,
and safe artifact preview/download failure handling.

## Browser Evidence

- Not-found / cannot-view URL:
  `http://localhost:8080/space/7656275718757679104/tasks/1`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-008-coze-task-not-found.png`
- Safe summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-008-error-state-summary.json`

Safe browser result:

- The page renders a bounded cannot-view / return-or-retry style state.
- No raw stack trace, SQL text, token, authorization string, checkpoint marker,
  or other raw internal error pattern was visible.

## Source Contract

- Loading skeleton:
  - `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
  - `data-testid="task-detail-loading-skeleton"`
- Artifact preview:
  - `frontend/apps/coze-studio/src/pages/tasks/task-artifact-inline-preview.tsx`
  - `frontend/apps/coze-studio/src/pages/tasks/task-artifact-actions.ts`
- Artifact API:
  - `backend/api/handler/coze/workbench_thread_service.go`

## Automated Verification

Commands:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders a DeerFlow-style message skeleton|generated document artifacts"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders canonical thread artifacts, previews safe text inline, and downloads via signed URL"

cd backend
go test ./api/handler/coze -run 'TestGetTaskThreadArtifactContentHandlerReturnsBytesWithSafeHeaders|TestGetTaskThreadArtifactContentHandlerMapsScanBlockedToConflict|TestGetTaskThreadArtifactSignedURLHandlerCapsTTLAndHidesObjectURI' -count=1 -gcflags="all=-N -l"
```

Result: all commands passed.

Covered behavior:

- Task detail renders a DeerFlow-style message skeleton while loading and does
  not show a generic `加载中...` placeholder.
- Artifact preview failures show the bounded user message
  `读取任务产物失败，请稍后重试`.
- Artifact preview failure messages do not expose secret tokens or long raw
  exception text.
- Artifact content responses include safe headers such as `nosniff`.
- Blocked/unsafe artifact content maps to a bounded conflict response.
- Signed URL responses cap TTL and do not expose object URI.

## Remaining Gap

No open P1-A-008 blocker remains. A live throttled-network loading screenshot
was not captured because the browser evidence mechanism does not provide safe
network throttling in this turn; the loading skeleton is locked by targeted
frontend tests instead.
