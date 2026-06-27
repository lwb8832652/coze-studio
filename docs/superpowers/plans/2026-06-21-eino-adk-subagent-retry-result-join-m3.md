# M3.34 Eino ADK Subagent Retry Result Join Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Attach top-level subagent retry runs back to their source child cards
in canonical task detail.

**Architecture:** Parse safe retry metadata from top-level runs inside
`task-detail-subagents.ts`, exclude retry runs from parent child-fetch and
usage-rollup queries, and render a compact retry summary from
`TaskSubagentRunsSection`.

**Tech Stack:** React, TypeScript, Vitest.

---

### Task 1: Add Red Test

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [x] **Step 1: Add retry top-level run fixture**

Extend the canonical subagent-card test so the top-level run list includes:

```ts
{
  run_id: 'run-retry-1',
  run_kind: 'task',
  status: 'queued',
  metadata: JSON.stringify({
    source: 'subagent_retry',
    source_run_id: 'run-child-2',
    parent_run_id: 'run-parent-1',
    requested_at: 1717000350000,
  }),
}
```

- [x] **Step 2: Assert retry join and parent filtering**

Assert the child card contains `重试 1 次` and `最近重试：排队中`, and that
`listTaskThreadRuns` is not called with `parent_run_id: 'run-retry-1'`.

- [x] **Step 3: Verify red**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders canonical thread subagent run cards grouped under parent runs"
```

Expected red result: retry summary is missing and the retry top-level run may
be treated as a parent run.

### Task 2: Implement Retry Attempt Projection

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-subagents.ts`

- [x] **Step 1: Add retry attempt type**

Add `TaskDetailSubagentRetryAttempt` with retry run ID, normalized status,
status text, error fields, requested timestamp, and updated timestamp.

- [x] **Step 2: Parse retry metadata**

Add helpers that parse top-level run metadata and return a source child run ID
only when `source === "subagent_retry"` and `source_run_id` is present.

- [x] **Step 3: Split parent and retry runs**

In `fetchTaskThreadSubagentRuns`, split top-level non-subagent runs into
ordinary parent runs and retry runs. Fetch child rows and grouped token usage
only for ordinary parent runs.

- [x] **Step 4: Attach attempts to source child**

Group retry attempts by source child run ID, newest first, and attach the group
to the mapped `TaskDetailSubagentRun`.

### Task 3: Render Retry Summary

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-subagent-runs-section.tsx`

- [x] **Step 1: Format retry summary**

Add a helper that returns `重试 N 次` and `最近重试：状态` for runs with retry
attempts.

- [x] **Step 2: Include summary in detail line**

Append the retry summary to `getSubagentRunDetail` after terminal
classification and before usage metadata.

### Task 4: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-result-join-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-result-join-m3.md`

- [x] **Step 1: Document retry-result join**

State that task detail may join retry top-level runs back to child cards only
through safe metadata and must not expose retry command/input/config/context.

- [x] **Step 2: Update roadmap**

Add M3.34 completion status and reduce the remaining estimate by one frontend
projection slice.

### Task 5: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused frontend test**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders canonical thread subagent run cards grouped under parent runs"
```

- [x] **Step 2: Run full task tests**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
```

- [x] **Step 3: Run frontend lint**

Run:

```bash
cd frontend/apps/coze-studio && npx eslint src/pages/tasks/task-detail-subagents.ts src/pages/tasks/task-subagent-runs-section.tsx src/pages/tasks/__tests__/task-detail.test.tsx --cache --quiet
```

- [x] **Step 4: Check formatting and doc placeholders**

Run:

```bash
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-result-join-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-result-join-m3.md
```
