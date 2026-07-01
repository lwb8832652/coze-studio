# P1-J-007 Streaming / Stop / Retry / Follow-up Evidence

Date: 2026-07-01

## Scope

This note closes the P1-J-007 acceptance row for the task-detail streaming
control path: DeerFlow submit button state, stop-square behavior, loading dots,
canonical follow-up submission, follow-up history preservation, previous-turn
step preservation, and retry/cancel affordance coverage.

Broader visual polish for the step/reasoning block and To-dos collapsed timing
remains tracked under P1-J-008.

## DeerFlow Reference

Source anchors verified from local DeerFlow:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/app/workspace/chats/[thread_id]/page.tsx:148`
  submits the prompt through `sendMessage(threadId, message)`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/app/workspace/chats/[thread_id]/page.tsx:158`
  calls `thread.stop()` for stop.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/app/workspace/chats/[thread_id]/page.tsx:256`
  maps `thread.isLoading` to input-box `status="streaming"`.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx:306`
  treats submit while streaming as `onStop()` and does not send another
  message.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/ai-elements/prompt-input.tsx:1093`
  maps `submitted` to spinner, `streaming` to square, `error` to X, and default
  to arrow-up.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/threads/hooks.ts:802`
  guards duplicate in-flight sends, appends optimistic user/upload messages,
  submits with `streamSubgraphs: true` and `streamResumable: true`, and
  invalidates thread list/history queries after submit.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx:508`
  generates follow-up suggestions after streaming ends from the latest
  user/assistant turns.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/streaming-indicator.tsx:3`
  renders the three animated loading dots used while an assistant turn is
  streaming.

Runtime note:

- The local DeerFlow reference detail URL redirected to `/workspace/chats/new`
  during this pass, so running-task detail behavior is not claimed from that
  stale page. Runtime smoke only verifies the current DeerFlow input submit
  affordance in ready state; streaming semantics are covered by the source
  anchors above.

Screenshot:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-007-deerflow-input-ready.jpg`

## Coze Current Behavior

Source anchors verified:

- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx:427`
  enables the composer stop state only for canonical thread details with a
  latest run id and cancelable task status.
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx:1142`
  passes `stopMode`, `stopLoading`, `onStop`, and `onSubmit` into
  `TaskFollowUpComposer`.
- `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx:509`
  routes submit while `stopMode` to `onStop()` instead of creating a new run.
- `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer-controls.tsx:370`
  renders the DeerFlow-style icon button with `aria-label="停止任务"` and
  `data-status="streaming"` when stop mode is active, otherwise
  `aria-label="发送任务"`.
- `frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts:89`
  calls `cancelTaskThreadRun` and refreshes the thread detail after stop.
- `frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts:120`
  creates a retry run with safe retry metadata and refreshes detail after
  retry.
- `frontend/apps/coze-studio/src/pages/tasks/task-result-section.tsx:38`
  renders the three-dot `TaskStreamingIndicator` while the task is nonterminal.
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts:436`
  sends canonical follow-ups, builds an optimistic detail immediately, clears
  the composer value, and refreshes the canonical detail.
- `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts:115`
  reloads recent thread messages before appending the latest user message, so
  follow-up run input includes previous user/assistant turns plus the latest
  message.

Runtime smoke:

- Current Coze task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`
- DOM check on completed state found one `button[aria-label="发送任务"]` with
  `data-status="ready"`, no stop button, and no loading dots. This is expected
  for a completed task.
- No extra model run was started during this smoke pass; running stop behavior
  is covered by targeted tests below.

Screenshot:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-007-coze-followup-ready.jpg`

## Automated Verification

Command:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "stops the latest running canonical thread run from the DeerFlow composer|shows an optimistic waiting turn immediately after canonical follow-up submit|sends canonical thread follow-up messages through the message API|keeps execution steps for previous assistant turns after a follow-up|renders streaming answer events before the final result is persisted"
```

Result:

- Frontend targeted suite passed: 5 tests, 44 skipped.

Covered assertions:

- Running canonical thread details render the answer loading dots and the
  composer stop button calls `cancelTaskThreadRun` with the latest run id.
- Canonical follow-up submit appends the user message, starts a new run, and
  immediately shows an optimistic waiting turn while the refresh is pending.
- Follow-up run input includes previous real user/assistant messages and the
  latest appended user message, while orphan failed follow-up messages are not
  replayed.
- Previous assistant turns keep their execution steps after a follow-up.
- Streaming answer events render before the final result is persisted.

## Remaining P1 Notes

P1-J-007 is complete for the functional streaming, stop, retry, and follow-up
contracts. Paired browser evidence for exact ChainOfThought/To-dos visual
spacing, expanded/collapsed state, and step-row polish stays under P1-J-008.
