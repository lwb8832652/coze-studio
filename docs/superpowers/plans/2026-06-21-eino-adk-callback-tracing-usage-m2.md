# Eino ADK Callback Tracing And Usage M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enrich Eino ADK callback usage records with Coze-owned trace IDs, span IDs, and bounded model metadata while preserving raw token usage.

**Architecture:** Extend `ADKUsageBridge` metadata generation; keep Eino callbacks as telemetry inputs and Coze as the durable attribution layer. Add trace ID to executor result metadata only when the callback bridge is active.

**Tech Stack:** Go, Eino callbacks, Eino ADK `v0.9.9`, Coze AgentThread usage collector.

---

## File Structure

- Modify `backend/application/agentthread/adk_usage.go`: trace/span helpers and model metadata snapshot.
- Modify `backend/application/agentthread/adk_usage_test.go`: callback metadata RED/GREEN coverage.
- Modify `backend/application/agentthread/adk_executor.go`: include trace ID in result metadata when usage bridge exists.
- Modify `backend/application/agentthread/adk_executor_test.go`: result metadata trace test.
- Modify `AGENTS.md`: document callback trace/metadata rules.
- Modify `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`: mark M2.16 progress.

## Task 1: Add RED Tests

- [x] **Step 1: Write usage metadata test**

Assert callback usage metadata includes `trace_id`, `span_id`,
`parent_span_id`, and bounded `model_metadata`, while excluding prompt text.

- [x] **Step 2: Write executor metadata test**

Assert `ADKExecutor` result metadata includes `trace_id` when callback usage is
enabled.

- [x] **Step 3: Run focused tests and verify RED**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKUsageMetadataIncludesTraceAndModelSnapshot|TestADKExecutorCollectsChatModelCallbackUsageOnce' -count=1
```

Expected: FAIL because trace/span/model metadata is not present yet.

## Task 2: Implement Trace And Metadata Enrichment

- [x] **Step 1: Extend usage bridge**

Add deterministic trace/span helpers and include the new fields in callback
usage metadata.

- [x] **Step 2: Extend executor result metadata**

Include callback bridge `trace_id` in final ADK execution metadata when the
bridge exists.

- [x] **Step 3: Run focused tests and verify GREEN**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKUsageMetadataIncludesTraceAndModelSnapshot|TestADKExecutorCollectsChatModelCallbackUsageOnce' -count=1
```

Expected: PASS.

## Task 3: Documentation And Verification

- [x] **Step 1: Update docs**

Update `AGENTS.md` and the master roadmap with the callback trace boundary.

- [x] **Step 2: Run full package verification**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```

Expected: PASS.

- [x] **Step 3: Run diff whitespace check**

Run:

```bash
git diff --check
```

Expected: no output.
