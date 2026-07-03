# P2-A TD-MSG-006 Assistant Reply Actions

## Scope

Close the small DeerFlow-visible assistant message action gap that was deferred
from P1. This slice adds the reply-level copy / positive feedback / negative
feedback affordances to Coze task detail without changing Workbench API
contracts.

## DeerFlow Reference

- Source: `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-list-item.tsx`
- `MessageListItem` renders a hover `MessageToolbar` for non-loading messages.
- The toolbar includes `CopyButton`.
- When `feedback`, `runId`, and `threadId` are available, it also renders
  `ThumbsUpIcon` and `ThumbsDownIcon` actions.
- DeerFlow persists feedback through `upsertFeedback` / `deleteFeedback`.

## Coze Implementation

- `frontend/apps/coze-studio/src/pages/tasks/task-assistant-message-actions.tsx`
  adds a shared DeerFlow-style action strip.
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx` renders it for
  canonical task-thread assistant replies.
- `frontend/apps/coze-studio/src/pages/tasks/task-result-section.tsx` renders
  it for legacy-compatible answer results.
- `frontend/apps/coze-studio/src/components/workspace-prototype.less` styles
  the actions as a low-emphasis hover/focus toolbar.
- Copy uses the already-reviewed `copyTextToClipboard` helper and receives only
  visible answer text after Coze's existing `<think>` stripping path.
- Feedback buttons keep local UI selection state only. Durable feedback API
  parity remains a P2-B policy/history/API task, because adding a new feedback
  contract is outside this visible-detail slice.

## Verification

- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "assistant reply actions"`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`
- `npx tsc --noEmit --project tsconfig.json`

## Redaction Check

The regression test asserts the copied assistant text excludes hidden
`<think>` reasoning. No prompt, raw model payload, tool arguments/results,
checkpoint bytes, credentials, object URIs, or signed URLs are introduced.
