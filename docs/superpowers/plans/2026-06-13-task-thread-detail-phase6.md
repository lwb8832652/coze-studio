# Task Thread Detail Phase 6 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/space/:space_id/chats/:thread_id` resolve canonical task threads before loading detail content, while preserving the existing task-style UI and legacy task detail path.

**Architecture:** Legacy `/tasks/:task_id` routes keep using `getTask` and `listTaskEvents` directly. Canonical `/chats/:thread_id` routes call `getTaskThread` first; if the thread has `legacy_task_id`, the detail page loads the legacy task and events with that id. If the thread has no legacy task, the page maps the thread summary into a read-only `ChatTask`-shaped view so users still see title, prompt, answer preview, status, and follow-up context.

**Tech Stack:** React 18, TypeScript, Vitest, existing Coze Studio API schema clients, existing task detail UI components.

---

### Task 1: Canonical Thread Route With Legacy Fallback

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`

- [ ] **Step 1: Write the failing test**

Update the service mock to expose `getTaskThread`, then add a test for route params `{ space_id: 'space-1', thread_id: 'thread-1' }`. Mock `getTaskThread` to return `legacy_task_id: 'task-legacy-1'`, assert:
- `getTaskThread` is called with `{ thread_id: 'thread-1' }`
- `getTask` is called with `{ task_id: 'task-legacy-1' }`
- `listTaskEvents` is called with `{ task_id: 'task-legacy-1' }`

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: FAIL because the detail page still calls `getTask({ task_id: thread_id })` directly.

- [ ] **Step 3: Write minimal implementation**

In `detail.tsx`, import `getTaskThread`, split legacy loading into `fetchLegacyTaskDetail(taskId)`, and add `fetchTaskDetail({ id, source })` where `source` is `task` or `thread`. For `source: 'thread'`, call `getTaskThread`; when `legacy_task_id` is non-empty, delegate to `fetchLegacyTaskDetail(legacy_task_id)`.

- [ ] **Step 4: Run test to verify it passes**

Run the same Vitest command. Expected: PASS.

### Task 2: Thread Summary Without Legacy Task

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`

- [ ] **Step 1: Write the failing test**

Add a route test where `getTaskThread` returns a thread with empty `legacy_task_id`. Assert:
- `getTask` is not called
- `listTaskEvents` is not called
- The page renders the thread title, `last_user_message`, and `last_agent_message`

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: FAIL because the detail page has no thread-summary fallback.

- [ ] **Step 3: Write minimal implementation**

Add `mapTaskThreadToTask(thread)` in `detail.tsx`:
- `id`: `legacy_task_id || thread_id`
- `status`: map `completed/succeeded` to `TaskStatus.Succeeded`, `running` to `TaskStatus.Running`, `failed` to `TaskStatus.Failed`, `canceled` to `TaskStatus.Canceled`, and waiting states to `TaskStatus.Created`
- `input`: JSON with `message: last_user_message`
- `result`: JSON with `message: last_agent_message`, `result_type: 'answer'`, `execution_type: 'Agent'`
- `events`: empty list

- [ ] **Step 4: Run test to verify it passes**

Run the same Vitest command. Expected: PASS.

### Task 3: Verification and Commit

**Files:**
- Verify detail tests and the list tests touched in Phase 5
- Verify focused ESLint
- Verify Git whitespace

- [ ] **Step 1: Run focused tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts
```

Expected: PASS.

- [ ] **Step 2: Run focused ESLint**

```bash
cd frontend/apps/coze-studio
npx eslint --cache --quiet src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/detail.tsx
```

Expected: no output and exit code 0.

- [ ] **Step 3: Run diff whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/2026-06-13-task-thread-detail-phase6.md frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx frontend/apps/coze-studio/src/pages/tasks/detail.tsx
git commit -m "feat: resolve task thread detail routes"
```
