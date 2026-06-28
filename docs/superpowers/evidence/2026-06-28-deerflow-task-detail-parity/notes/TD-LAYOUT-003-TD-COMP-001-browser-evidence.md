# TD-LAYOUT-003 / TD-COMP-001 Browser Evidence

Date: 2026-06-28

Reference pages:

- DeerFlow:
  `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`
- Coze:
  `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`

## DeerFlow Baseline

- `document.scrollHeight` equals viewport height in the reference detail page.
- The chat history scrolls inside an internal overflow container.
- The composer `FORM` stays near the viewport bottom while the message history
  is scrolled.

## Coze Verification

- `task-detail-scroll` exists and is the scrollable message/history container.
- `coze-prototype-followup` is no longer contained by
  `task-chat-transcript`.
- Browser wheel verification changed only `task-detail-scroll.scrollTop`.
  The follow-up composer bounds stayed fixed:
  - before scroll: `followTop=822`, `followBottom=988`
  - after scroll up: `followTop=822`, `followBottom=988`
  - after scroll down: `followTop=822`, `followBottom=988`
- Header `详情` opens the secondary inspector that contains `运行诊断`,
  `安全审计`, and `任务记忆`; those sections no longer render in the chat
  transcript by default.

## Code Evidence

- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-inspector.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-top-bar.tsx`
- `frontend/apps/coze-studio/src/components/workspace-prototype.less`
- `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

Verification commands:

```bash
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
git diff --check
```
