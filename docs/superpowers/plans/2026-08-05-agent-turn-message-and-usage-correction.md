# Agent Turn Message And Usage Correction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Guarantee one public assistant reply per user run and count each provider model invocation once without changing Agent execution.

**Architecture:** Correct the canonical backend projection instead of grouping duplicate replies in React. Add callback-lineage deduplication inside `ADKUsageBridge`, leaving the billing guard, provider calls, tools, events, and durable finalization untouched.

**Tech Stack:** Go, Hertz canonical handlers, Eino ADK callbacks, Testify, React/Vitest regression coverage.

---

### Task 1: Canonical assistant reply projection

**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Test: `backend/api/handler/coze/workbench_canonical_thread_service_test.go`

- [ ] Add a failing handler test with a tool-call assistant event whose content
  differs from the durable final reply; assert only the user message and durable
  final reply are returned.
- [ ] Add a failing handler test with two non-empty assistant events and no
  durable reply; assert only the latest event reply is returned.
- [ ] Add a failing handler test with two durable assistant messages for one
  Run; assert only the latest durable reply is returned.
- [ ] Run the two tests with `go test -gcflags="all=-l -N"` and confirm the
  current projection returns too many assistant messages.
- [ ] Track durable assistant run IDs and latest event-only assistant messages,
  then apply the two projection rules before constructing canonical messages.
- [ ] Re-run the focused tests and the canonical handler package tests.

### Task 2: Remove the display-only experiment

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] Remove the uncommitted positional grouping that merges every assistant
  message after the latest user message.
- [ ] Restore the test fixture to the real run identity contract.
- [ ] Run the focused task-detail regression suite to ensure the backend
  one-reply contract renders with the accepted Journal layout.

### Task 3: Nested callback token deduplication

**Files:**
- Modify: `backend/application/agentthread/adk_usage.go`
- Test: `backend/application/agentthread/adk_usage_test.go`

- [ ] Add a failing test that simulates billing-wrapper and provider callbacks
  for one model invocation followed by event usage; assert one usage record with
  provider metadata.
- [ ] Run the focused test and confirm it currently records two rows.
- [ ] Add parent callback identity to `adkUsageCall` and track the completed
  child fingerprint for each parent callback.
- [ ] Skip a parent callback only when its direct/nested child reports the same
  usage fingerprint, propagating the fingerprint through additional wrappers.
- [ ] Re-run focused and full `backend/application/agentthread` tests.

### Task 4: Documentation and end-to-end verification

**Files:**
- Modify: `docs/superpowers/context/workbench-chat.md`

- [ ] Document the one-public-reply projection and nested-wrapper usage
  accounting boundary.
- [ ] Run `git diff --check`, relevant Go tests, task-detail Vitest, and lint for
  touched frontend files.
- [ ] Inspect the final diff to confirm no execution, queue, tool, artifact,
  prompt, or billing-reservation behavior changed.
- [ ] Rebuild/restart one latest local environment; refresh an existing
  tool-producing task to verify one final reply with its artifact intact, then
  create one low-cost task to verify one reply and corrected token totals in
  the browser and database.
