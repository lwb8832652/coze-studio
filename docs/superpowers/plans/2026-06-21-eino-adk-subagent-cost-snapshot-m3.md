# M3.25 Eino ADK Subagent Cost Snapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a safe cost snapshot on child subagent state cards when backend
token usage data provides a positive cost and one currency.

**Architecture:** Reuse the existing task-thread token usage response. The
frontend maps aggregate `cost_micros`, derives a single safe currency from
usage rows per child run, and formats a compact label in the subagent card.

**Tech Stack:** React 18, TypeScript, Vitest, Coze task-thread service schema.

---

### Task 1: Add The Red Test

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [x] **Step 1: Add cost data to the subagent fixture**

Set `run-child-1` usage rows to one currency, such as `USD`, and set the
`run_aggregates` entry to a positive `cost_micros` value.

- [x] **Step 2: Assert the formatted cost label**

Expect the rendered card text to contain:

```text
Cost USD 0.000250
```

- [x] **Step 3: Verify the test fails before implementation**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "subagent run cards"
```

Expected red result: the test fails because no cost label is rendered.

### Task 2: Implement Safe Cost Mapping

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-token-usage.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-subagents.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-subagent-runs-section.tsx`

- [x] **Step 1: Preserve aggregate cost**

Add `costMicros` and `currency` to `TaskDetailTokenUsage`, mapping
`aggregate.cost_micros` and defaulting currency to an empty string.

- [x] **Step 2: Attach one safe currency per child run**

While processing grouped usage rows, collect uppercased non-empty currencies by
`run_id`. Attach the currency to the child run token usage only when the
de-duplicated currency set has exactly one value.

- [x] **Step 3: Format the card detail**

Render `Cost <currency> <amount>` only when `costMicros > 0` and currency is
non-empty. Format the amount as `costMicros / 1_000_000` with six decimals.

### Task 3: Update Context And Verify

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-cost-snapshot-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-cost-snapshot-m3.md`

- [x] **Step 1: Document the backend-owned pricing boundary**

Record that the frontend may display a cost snapshot only from
backend-provided `cost_micros` plus a single currency, and that pricing rules
and cost accounting stay backend-owned.

- [x] **Step 2: Run focused and full frontend checks**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
```

Run:

```bash
cd frontend/apps/coze-studio && npx eslint \
  src/pages/tasks/task-detail-subagents.ts \
  src/pages/tasks/task-subagent-runs-section.tsx \
  src/pages/tasks/task-detail-loader.ts \
  src/pages/tasks/task-detail-hooks.ts \
  src/pages/tasks/detail.tsx \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  --cache --quiet
```

Run:

```bash
git diff --check
```
