# Frontend Task Thread List Phase 5 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Switch the visible task list surfaces to the canonical `task_threads` API while keeping task-style menu names and preserving legacy detail fallback.

**Architecture:** The frontend keeps the current task-first UI copy. The all-tasks page and workspace recent-task sidebar read `TaskThread` summaries through `listTaskThreads`, render thread status/message fields, and navigate to `legacy_task_id` when present so the existing detail page can still use legacy task details until run/message/event persistence is added.

**Tech Stack:** React 18, TypeScript, Vitest, existing Coze Studio API schema clients, existing prototype task styles.

---

### Task 1: All Tasks Page Source

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`

- [ ] **Step 1: Write the failing test**

Update the page service mock to expose `listTaskThreads`, return a thread summary with `thread_id`, `legacy_task_id`, `last_user_message`, and string status, then assert:
- `listTaskThreads` is called with `{ space_id: 'space-1' }`
- The rendered list shows the thread title and last user message
- Opening the task navigates to `/space/space-1/chats/task-legacy-1`

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: FAIL because the page still calls `listTasks`.

- [ ] **Step 3: Write minimal implementation**

Change `frontend/apps/coze-studio/src/pages/tasks/index.tsx` to:
- Import `listTaskThreads` from `./service`
- Use `workbenchTask.TaskThread` as the list item type
- Render `last_user_message || last_agent_message || title` as the row description
- Map thread status strings to existing task tones/text
- Use `thread.thread_id` for keys and favorites
- Navigate to `thread.legacy_task_id || thread.thread_id`

- [ ] **Step 4: Run test to verify it passes**

Run the same Vitest command. Expected: PASS.

### Task 2: Workspace Recent Task Source

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-status.ts`

- [ ] **Step 1: Write the failing test**

Add a render test for `WorkspaceTaskList` with mocked router, space store, and `listTaskThreads`. Assert:
- `listTaskThreads` is called with `{ space_id: 'space-1', page_size: 8 }`
- The sidebar renders the thread title
- Clicking it navigates to `/space/space-1/chats/task-legacy-1`

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: FAIL because the sidebar still calls `listTasks`.

- [ ] **Step 3: Write minimal implementation**

Change `WorkspaceTaskList` to call `listTaskThreads`, store `TaskThread[]`, and use `legacy_task_id || thread_id` for navigation. Extend `getWorkspaceTaskStatusMeta` to accept canonical thread statuses: `idle`, `running`, `completed`, `failed`, and `canceled`.

- [ ] **Step 4: Run test to verify it passes**

Run the same Vitest command. Expected: PASS.

### Task 3: Verification and Commit

**Files:**
- Verify changed frontend tests
- Verify formatting whitespace with Git

- [ ] **Step 1: Run focused frontend tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run diff whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/plans/2026-06-13-frontend-task-thread-list-phase5.md frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx frontend/apps/coze-studio/src/pages/tasks/index.tsx frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-status.ts
git commit -m "feat: use task thread list source"
```
