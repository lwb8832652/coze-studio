# M3.24 Eino ADK Subagent Provider Model Summary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render a bounded multi-provider/model attribution summary on child
subagent state cards.

**Architecture:** Reuse the existing parent `include_child_runs=true` token
usage response. The task detail frontend groups attribution labels by child run
ID, de-duplicates them in response order, and renders only a compact card label.

**Tech Stack:** React 18, TypeScript, Vitest, Coze task-thread service schema.

---

### Task 1: Add The Red Test

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [x] **Step 1: Extend the subagent card fixture**

Add two distinct usage rows for `run-child-1`, for example
`openai / gpt-4.1` and `volcengine / doubao-pro`, while keeping the existing
per-run aggregate total unchanged.

- [x] **Step 2: Assert the compact summary**

Expect the card text to contain:

```text
openai / gpt-4.1 +1
```

Keep the second child assertion as a single-label control:

```text
volcengine / doubao-lite
```

- [x] **Step 3: Verify the test fails before implementation**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "subagent run cards"
```

Expected red result: the test fails because the card still displays only
`openai / gpt-4.1`.

### Task 2: Implement The Summary

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-subagents.ts`

- [x] **Step 1: Collect distinct labels per run**

Replace the single attribution map with `Map<string, string[]>`. For each
usage row, derive the existing attribution label and append it only when the
label is non-empty and not already present for that run.

- [x] **Step 2: Format the card label**

Add a helper that returns an empty string for no labels, the first label for a
single label, and `${first} +${rest.length}` for multiple labels.

- [x] **Step 3: Keep card data bounded**

Pass only the formatted string into `TaskDetailSubagentRun.modelAttribution`.
Do not pass raw usage rows or a provider/model list to the card component.

### Task 3: Update Context And Verify

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-provider-model-summary-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-provider-model-summary-m3.md`

- [x] **Step 1: Document the metadata-only summary rule**

Record that subagent cards may show `provider / model_name +N` for additional
distinct labels and must not expose raw usage, prompts, completions, tool data,
provider payloads, credentials, checkpoint bytes, or object identifiers.

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
