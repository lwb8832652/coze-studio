# Frontend Task Thread Follow-Up Phase 10 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route follow-up messages from canonical task-thread detail pages into the new thread message API.

**Architecture:** Keep legacy task details on `WorkbenchChat` so existing task execution behavior is unchanged. Detect canonical-only thread detail pages from the loaded task id matching the route `thread_id`, call `AppendTaskThreadMessage` for user follow-ups, preserve composer mode/resource selections in message metadata, clear the composer, and refresh detail through the existing loader.

**Tech Stack:** React, TypeScript, Vitest, Coze Studio API schema clients, existing task detail page.

---

### Task 1: Canonical Thread Follow-Up Test

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: Add the mocked message append API**

Add `mockAppendTaskThreadMessage` to the service mock:

```ts
const mockAppendTaskThreadMessage = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  appendTaskThreadMessage: mockAppendTaskThreadMessage,
}));
```

Reset it in `beforeEach` and give it a successful default response.

- [ ] **Step 2: Write the failing canonical follow-up test**

Add a test that loads `/chats/thread-only-1`, submits `请追加行动建议`, and asserts:

```ts
expect(mockAppendTaskThreadMessage).toHaveBeenCalledWith({
  thread_id: 'thread-only-1',
  role: 'user',
  content: '请追加行动建议',
  metadata: expect.any(String),
});
expect(mockSendWorkbenchChat).not.toHaveBeenCalled();
```

Parse the metadata string and assert it contains the composer mode plus resource selections:

```ts
expect(JSON.parse(call.metadata)).toMatchObject({
  mode: 'Auto',
  enable_skills: expect.arrayContaining(['meego-guidelines']),
  enable_mcp: [],
  enable_kbs: [],
  enable_databases: [],
});
```

Assert the page refreshes and renders the newly appended user message from the second `listTaskThreadMessages` response.

- [ ] **Step 3: Run the detail test to verify it fails**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: FAIL because canonical thread follow-ups still call `sendWorkbenchChat`.

### Task 2: Follow-Up Routing Implementation

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`

- [ ] **Step 1: Import the append client**

```ts
import { appendTaskThreadMessage, sendWorkbenchChat } from './service';
```

- [ ] **Step 2: Add metadata helper**

Add a local helper:

```ts
const getThreadFollowUpMetadata = (
  payload: WorkbenchComposerSubmitPayload,
) =>
  JSON.stringify({
    mode: payload.mode,
    model_type: payload.modelType,
    model_name: payload.modelName,
    enable_skills: payload.enable_skills,
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
  });
```

- [ ] **Step 3: Branch canonical-only thread submissions**

In `handleFollowUpSubmit`, compute:

```ts
const isCanonicalThreadDetail =
  taskDetailSource === 'thread' && task?.id === taskDetailId;
```

When true, call:

```ts
await appendTaskThreadMessage({
  thread_id: taskDetailId,
  role: 'user',
  content: payload.message,
  metadata: getThreadFollowUpMetadata(payload),
});
```

When false, keep the existing `sendWorkbenchChat` call.

- [ ] **Step 4: Run the detail test to verify it passes**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

### Task 3: Verification and Commit

**Files:**
- Verify changed frontend detail behavior
- Verify lint and whitespace

- [ ] **Step 1: Run focused frontend tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts
```

Expected: PASS.

- [ ] **Step 2: Run focused lint**

```bash
cd frontend/apps/coze-studio
npx eslint --cache --quiet src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/detail.tsx src/pages/tasks/service.ts
```

Expected: no lint errors.

- [ ] **Step 3: Run whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/2026-06-14-frontend-task-thread-followup-phase10.md frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx frontend/apps/coze-studio/src/pages/tasks/detail.tsx
git commit -m "feat: route thread followups to message api"
```
