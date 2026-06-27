# Eino ADK Semantic Loop And Budget M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Coze-owned semantic loop detection for Eino ADK task runs while keeping Eino `MaxIterations` as the hard execution budget.

**Architecture:** Implement a focused ADK handler that inspects `ChatModelAgentState` after model calls and returns a typed Coze error when repeated Assistant actions exceed configured limits. Event mapping turns that typed error into a bounded `run.semantic_loop_detected` event without raw content.

**Tech Stack:** Go, Eino ADK `v0.9.9`, `*schema.Message`, Coze AgentThread event mapper, Go unit tests with Mockey-compatible `-gcflags="all=-l -N"`.

---

## File Structure

- Create `backend/application/agentthread/adk_semantic_loop.go`: config parsing, signature hashing, middleware, and typed error.
- Create `backend/application/agentthread/adk_semantic_loop_test.go`: RED/GREEN tests for config and detector behavior.
- Modify `backend/application/agentthread/adk_middleware.go`: register the middleware in the ADK handler order.
- Modify `backend/application/agentthread/adk_middleware_test.go`: assert middleware placement.
- Modify `backend/application/agentthread/adk_event_mapper.go`: map the typed error to `run.semantic_loop_detected`.
- Modify `backend/application/agentthread/adk_event_mapper_test.go`: assert content-free event payload.
- Modify `AGENTS.md`: persist the semantic-loop runtime rule.
- Modify `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`: mark M2.14 progress and update remaining gaps.

## Task 1: Add Semantic Loop Tests

- [x] **Step 1: Write failing config tests**

Add tests to `backend/application/agentthread/adk_semantic_loop_test.go` for disabled default config, valid run config, and limit rejection above `20`.

- [x] **Step 2: Run config tests and verify RED**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKSemanticLoopConfig' -count=1
```

Expected: FAIL because semantic-loop config functions do not exist.

- [x] **Step 3: Write failing middleware tests**

Add tests to the same file for repeated tool calls, repeated Assistant text,
and a new user turn resetting the scan scope.

- [x] **Step 4: Run middleware tests and verify RED**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKSemanticLoopMiddleware' -count=1
```

Expected: FAIL because the middleware does not exist.

## Task 2: Implement Semantic Loop Middleware

- [x] **Step 1: Add production implementation**

Create `backend/application/agentthread/adk_semantic_loop.go` with:

- `ADKSemanticLoopConfig`
- `ADKSemanticLoopError`
- `NewADKSemanticLoopMiddleware`
- `adkSemanticLoopConfigFromRun`
- hashed tool-call and Assistant-text signature helpers

- [x] **Step 2: Run semantic-loop tests and verify GREEN**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKSemanticLoop' -count=1
```

Expected: PASS.

## Task 3: Wire Middleware And Event Mapping

- [x] **Step 1: Write failing order and mapper tests**

Extend existing tests so `semantic_loop` is ordered after
`tool_error_normalization` and before `policy`, and so `ADKSemanticLoopError`
maps to `run.semantic_loop_detected`.

- [x] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKMiddlewareSemanticLoop|TestMapADKEventMapsSemanticLoop' -count=1
```

Expected: FAIL because the middleware is not registered and mapper does not
handle the new error type.

- [x] **Step 3: Wire production code**

Add `ADKMiddlewareSemanticLoop` to `adk_middleware.go`, construct the middleware
from run config, and map the typed error in `adk_event_mapper.go`.

- [x] **Step 4: Run focused tests and verify GREEN**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKMiddlewareSemanticLoop|TestMapADKEventMapsSemanticLoop' -count=1
```

Expected: PASS.

## Task 4: Documentation And Final Verification

- [x] **Step 1: Update runtime docs**

Update `AGENTS.md` and the DeerFlow parity roadmap with the new config,
middleware order, event type, and remaining deferred work.

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
