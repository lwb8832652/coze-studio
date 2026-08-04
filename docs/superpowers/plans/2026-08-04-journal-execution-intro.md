# Journal Execution Intro Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Pro/Ultra 的真实 Journal 任务中，把任务级执行策略摘要稳定展示在首个大步骤之前；简单直答不展示。

**Architecture:** 不创建额外聊天消息，不增加第二套状态机，也不修改模型原有执行提示词。后端可读取已有计划中的可选 `metadata.execution_intro`，但默认从完整计划标题生成安全摘要；现有 `plan.task.*` RunEvent 经过脱敏后投影为幂等 `journal.intro`。前端只消费公共 Journal 投影，并在 Milestone 列表之前渲染摘要。Journal 的父子步骤绑定只保存在当前进程的瞬时投影状态中并按 Agent 隔离；恢复后由 Journal 仓储为同一 action 继承已落库的 milestone，没有既有阶段时才降级为原子步骤。Skill Journal 观察异步且限时，绝不改写、阻塞或取消工具执行。

**Tech Stack:** Go、Eino ADK PlanTask middleware、JournalEvent v1、React、TypeScript、Vitest、Testing Library。

---

### Task 1: Freeze The Non-Interference Contract

**Files:**
- Test: `backend/application/agentthread/adk_lead_prompt_test.go`
- Test: `backend/application/agentthread/adk_plan_completion_guard_test.go`
- Test: `backend/application/agentthread/adk_parity_state_test.go`

- [x] **Step 1: Write the execution-preservation tests**

Assert that the existing planning prompt remains advisory, simple actions remain direct, execution tool calls are never rewritten because a plan is absent or incomplete, and Journal bindings do not change durable parity state.

- [x] **Step 2: Run the focused test and confirm RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./application/agentthread -run TestDefaultADKLeadPromptComposer -count=1
```

Expected: the new non-interference assertions fail while the experimental execution guard and durable Journal bindings are present.

- [x] **Step 3: Remove execution coupling**

Keep `<todo_system>` identical to the established runtime contract. Remove the Journal execution guard, keep tool calls untouched, and store parent bindings outside durable parity state.

- [x] **Step 4: Re-run the focused test and confirm GREEN**

Run the same command and expect `ok`.

### Task 2: Persist And Project The Intro

**Files:**
- Modify: `backend/application/agentthread/adk_plan_backend.go`
- Modify: `backend/application/agentthread/public_projection.go`
- Modify: `backend/application/agentthread/journal_projection.go`
- Test: `backend/application/agentthread/adk_plan_backend_test.go`
- Test: `backend/application/agentthread/public_projection_test.go`
- Test: `backend/application/agentthread/journal_projection_test.go`

- [x] **Step 1: Write failing backend tests**

Cover these exact behaviors:

```text
explicit model intro -> plan.task.created payload contains only sanitized execution_intro
missing model intro -> pending creation has no fallback; first visible step uses the complete plan fallback
sensitive or absolute-path intro -> omitted from the public payload
pending plan + intro -> one journal.intro projection with run-scoped idempotency
visible milestone + intro -> execution_intro remains available on the milestone projection
```

- [x] **Step 2: Run the focused tests and confirm RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./application/agentthread -run 'Test(ADKPlanBackend|ProjectPublicRunEventPayload|ProjectRunEventToJournal).*Intro' -count=1
```

Expected: tests fail because no intro is emitted or projected.

- [x] **Step 3: Implement the smallest backend change**

Add a bounded `execution_intro` extractor with a plan-title fallback. Copy only the approved label through `projectPublicRunEventPayload`. Project a pending explicit intro as:

```json
{"type":"intro","data":{"text":"收到。我会先..."}}
```

Use the stable run-scoped phase `intro` so retries replay the same Journal projection instead of adding duplicate visible text. For an in-progress or terminal milestone, retain `execution_intro` in milestone data so the fallback can render before that milestone.

- [x] **Step 4: Re-run focused and package tests**

Run the focused command, then:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./application/agentthread -count=1
```

Expected: both commands return `ok`.

### Task 3: Render The Intro Before Milestones

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/journal-types.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal-event-model.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal-conversation-flow.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal.less`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-event-display.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Test: `frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-ui.test.tsx`
- Test: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`

- [x] **Step 1: Write failing frontend tests**

Assert that `journal.intro` is rendered exactly once before the first `.journal-milestone`, that milestone fallback works, and that Journal without an intro remains unchanged.

- [x] **Step 2: Run the focused Vitest and confirm RED**

Run:

```bash
cd frontend/apps/coze-studio
rushx test src/pages/tasks/journal/__tests__/journal-ui.test.tsx --silent
```

Expected: the intro element is missing.

- [x] **Step 3: Implement event selection and faithful styling**

Select the earliest non-empty `journal.intro.data.text`, then fall back to the earliest milestone `execution_intro`. Render a plain paragraph before the milestone list using the accepted v26 typography and spacing; do not add a heading, icon, card, or `EXECUTION JOURNAL` label.

The real-browser direct-answer check also exposed a legacy fallback that counted
skill catalogs and opaque runtime metadata as visible steps. Keep those records
available as metadata, but mark them `visibleInFlow: false`; only structured,
user-facing execution events may enter the conversation flow.

- [x] **Step 4: Re-run the focused test and task-detail regression suite**

Run:

```bash
cd frontend/apps/coze-studio
rushx test src/pages/tasks/journal src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-detail-visual-contract.test.tsx --silent
```

Expected: all selected files pass.

### Task 4: Production-Style Verification

**Files:**
- Verify only; no new implementation files expected.

- [x] **Step 1: Run formatting and diff checks**

```bash
gofmt -w backend/application/agentthread/adk_lead_prompt.go backend/application/agentthread/adk_lead_prompt_test.go backend/application/agentthread/adk_plan_backend.go backend/application/agentthread/adk_plan_backend_test.go backend/application/agentthread/public_projection.go backend/application/agentthread/public_projection_test.go backend/application/agentthread/journal_projection.go backend/application/agentthread/journal_projection_test.go
git diff --check
```

- [x] **Step 2: Build the frontend and local images**

Use the repository's existing Rush build and the single local Compose validation environment. Preserve the current database and keep only one backend worker active.

- [x] **Step 3: Create a fresh real Pro task**

Verify in the Codex in-app browser that the visible order is:

```text
Agent heading -> execution intro -> current/completed major steps -> child actions -> final answer -> artifact
```

Also submit a simple direct question and verify there is no intro and no Journal ceremony.

- [x] **Step 4: Record evidence without committing**

Capture DOM order, visible copy, screenshot, console errors, backend projection tests, and test/build output. Do not commit, merge, or push until the repository's two-stage dev integration audit is requested and confirmed.
