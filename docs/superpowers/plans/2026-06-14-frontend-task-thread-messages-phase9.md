# Frontend Task Thread Messages Phase 9 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the task detail page load canonical task-thread messages from the new backend message API.

**Architecture:** Extend the manual frontend workbench task schema with message list/append APIs, re-export those clients from the task page service boundary, and keep route compatibility in the detail loader. Legacy-backed threads continue to load legacy task/events; canonical-only threads use message history first and fall back to thread summary fields.

**Tech Stack:** React, TypeScript, Vitest, Coze Studio API schema createAPI, existing task detail loader.

---

### Task 1: Frontend Message API Client

**Files:**
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts`

- [ ] **Step 1: Write the failing service export test**

Add assertions that `listTaskThreadMessages` and `appendTaskThreadMessage` are exported functions:

```ts
expect(typeof listTaskThreadMessages).toBe('function');
expect(typeof appendTaskThreadMessage).toBe('function');
```

- [ ] **Step 2: Run the service test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts
```

Expected: FAIL because the new frontend clients are not exported yet.

- [ ] **Step 3: Add schema types and createAPI clients**

Add:
- `TaskThreadMessage`
- `ListTaskThreadMessagesRequest/Data/Response`
- `AppendTaskThreadMessageRequest/Response`
- `ListTaskThreadMessages`
- `AppendTaskThreadMessage`

Use `/api/workbench/task_threads/:thread_id/messages`, `thread_id` as path param, `page/page_size` as list query params, and `run_id/role/content/metadata` as append body params.

- [ ] **Step 4: Re-export the API clients from task service**

```ts
export const listTaskThreadMessages = workbenchTask.ListTaskThreadMessages;
export const appendTaskThreadMessage = workbenchTask.AppendTaskThreadMessage;
```

- [ ] **Step 5: Run the service test to verify it passes**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts
```

Expected: PASS.

### Task 2: Detail Page Message History

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: Write the failing detail test**

Extend the canonical-only thread test so the mocked message API returns newer user and assistant messages. Assert the loader calls:

```ts
expect(mockListTaskThreadMessages).toHaveBeenCalledWith({
  thread_id: 'thread-only-1',
  page: 1,
  page_size: 50,
});
```

Assert the rendered detail uses the message history content instead of stale thread summary fields.

- [ ] **Step 2: Run the detail test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: FAIL because canonical-only thread details do not call the message API yet.

- [ ] **Step 3: Load messages for canonical-only threads**

Import `listTaskThreadMessages` in `task-detail-loader.ts`. When a thread has no `legacy_task_id`, call:

```ts
listTaskThreadMessages({ thread_id: id, page: 1, page_size: 50 })
```

Pick the latest `user` and latest `assistant` message by iterating message history from the end. Use those contents for the mapped `ChatTask.input` and `ChatTask.result`; fall back to `thread.last_user_message`, `thread.last_agent_message`, and `thread.title`.

- [ ] **Step 4: Run the detail test to verify it passes**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

### Task 3: Verification and Commit

**Files:**
- Verify changed frontend tests
- Verify lint and whitespace

- [ ] **Step 1: Run focused frontend tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run focused lint**

```bash
cd frontend/apps/coze-studio
npx eslint --cache --quiet src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/task-detail-loader.ts src/pages/tasks/service.ts ../../packages/arch/api-schema/src/idl/workbench/task.ts
```

Expected: no lint errors.

- [ ] **Step 3: Run whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/2026-06-14-frontend-task-thread-messages-phase9.md frontend/packages/arch/api-schema/src/idl/workbench/task.ts frontend/apps/coze-studio/src/pages/tasks/service.ts frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts
git commit -m "feat: consume task thread messages in frontend"
```
