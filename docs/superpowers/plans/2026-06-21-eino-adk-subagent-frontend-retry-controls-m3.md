# M3.33 Eino ADK Subagent Frontend Retry Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add task detail retry controls for failed or canceled subagent child
runs.

**Architecture:** Keep retry side effects in `useTaskDetailActions` and keep
`TaskSubagentRunsSection` presentational through callback props. Reuse the
generated `retryTaskThreadSubagentRun` service wrapper and refresh task detail
with the existing loader after a successful retry request.

**Tech Stack:** React, TypeScript, Vitest, Coze Design/Semi Button semantics.

---

### Task 1: Add Red Test

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [x] **Step 1: Mock retry service**

Add a hoisted `mockRetryTaskThreadSubagentRun`, reset it in `beforeEach`, and
include it in the `../service` mock as `retryTaskThreadSubagentRun`.

- [x] **Step 2: Assert failed child retry**

Add a task detail test that renders a canonical thread with one failed child
run, clicks the row `重试` button, and asserts:

```ts
expect(mockRetryTaskThreadSubagentRun).toHaveBeenCalledWith({
  thread_id: 'thread-subagent-1',
  run_id: 'run-child-2',
});
expect(mockGetTaskThread).toHaveBeenCalledTimes(2);
```

- [x] **Step 3: Verify red**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "retries failed canonical thread subagent runs"
```

Expected red result: no retry button is rendered or no retry service is called.

### Task 2: Implement Retry Action

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-subagent-runs-section.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-prototype.less`

- [x] **Step 1: Add hook state and callback**

Import `retryTaskThreadSubagentRun`, add `retryingSubagentRunId` and
`subagentRetryError`, and implement `handleRetrySubagentRun(runId)` so it:

```ts
if (!taskDetailId || taskDetailSource !== 'thread') {
  setSubagentRetryError('缺少任务恢复上下文，请刷新后重试');
  return;
}
setRetryingSubagentRunId(runId);
setSubagentRetryError('');
try {
  await retryTaskThreadSubagentRun({ thread_id: taskDetailId, run_id: runId });
  const detail = await fetchTaskDetail({ id: taskDetailId, source: taskDetailSource });
  applyTaskDetail(detail);
} catch (err) {
  setSubagentRetryError(err instanceof Error ? err.message : '重试子智能体失败，请稍后再试');
} finally {
  setRetryingSubagentRunId('');
}
```

- [x] **Step 2: Pass action props**

Pass `handleRetrySubagentRun`, `retryingSubagentRunId`, and
`subagentRetryError` from `TaskDetailPage` into `TaskSubagentRunsSection`.

- [x] **Step 3: Render retry control**

Import `Button` from `@coze-arch/coze-design`. In
`TaskSubagentRunsSection`, render a small `重试` button only when status is
`failed` or `canceled` and `onRetrySubagentRun` exists. Set `loading` and
`disabled` when `retryingRunId === run.runId`.

- [x] **Step 4: Style row actions and error**

Add scoped styles for the row action area and inline retry error using existing
prototype spacing, border, and status color tokens.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-frontend-retry-controls-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-frontend-retry-controls-m3.md`

- [x] **Step 1: Document frontend retry rule**

Add the rule that task detail can expose retry only for failed/canceled child
subagent rows, and that the UI must call the retry API with only thread/run
IDs and refresh detail after success.

- [x] **Step 2: Update roadmap**

Add M3.33 completion status and reduce the remaining task/person-month
estimate by one frontend retry-control slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused frontend test**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "retries failed canonical thread subagent runs"
```

- [x] **Step 2: Run full task tests**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
```

- [x] **Step 3: Run frontend lint**

Run:

```bash
cd frontend/apps/coze-studio && npx eslint src/pages/tasks/task-subagent-runs-section.tsx src/pages/tasks/task-detail-hooks.ts src/pages/tasks/detail.tsx src/pages/tasks/__tests__/task-detail.test.tsx --cache --quiet
```

- [x] **Step 4: Run backend affected retry tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestADKExecutorReplaysSubagentRetryFromSourceChildRun|TestRuntimeSelectorRoutesSubagentRetryToConfiguredADKExecutor|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 5: Check formatting and doc placeholders**

Run:

```bash
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-frontend-retry-controls-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-frontend-retry-controls-m3.md
```
