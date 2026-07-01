# P1-J-006 Artifact Split Width Evidence

Date: 2026-07-01

## Scope

This note covers the artifact side-preview width behavior. It does not close the
full artifact/present_files acceptance row.

## DeerFlow Reference

Verified source anchor:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/chats/chat-box.tsx`

Relevant DeerFlow behavior:

```ts
const CLOSE_MODE = { chat: 100, artifacts: 0 };
const OPEN_MODE = { chat: 60, artifacts: 40 };
```

Artifacts use a 60/40 split when the side panel is open, rather than a fixed
maximum-width drawer.

## Coze Change

- Added `data-width-mode="deerflow-60-40"` to the task artifact side preview.
- Changed `--coze-prototype-artifact-side-preview-width` from
  `clamp(360px, 40%, 640px)` to `40%`, keeping the existing mobile/min-width
  guard through the side-preview `min-width` rule.

## Browser Evidence

Page:

`http://localhost:8080/space/7656275718757679104/tasks/7657390468782620672`

Read-only DOM check:

```json
{
  "layout": "deerflow-split",
  "widthMode": "deerflow-60-40",
  "pageWidth": 900,
  "width": 360,
  "widthRatio": 0.4,
  "cssWidth": "360px",
  "minWidth": "360px"
}
```

## Automated Verification

Commands:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders generated document artifacts"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Result:

- Targeted test failed first on missing `data-width-mode`, then passed after the
  Coze change.
- Full task-detail suite passed: 49 tests.
- TypeScript check passed.

Known test noise:

- Existing `localhost:3000` `ECONNREFUSED` logger noise appeared in one
  unrelated follow-up composer test; the test suite passed.

## Remaining Gap

Full P1-J-006 still needs paired DeerFlow/Coze screenshot evidence for multiple
artifact MIME types, final-answer ordering, and preview/download/copy behavior.
