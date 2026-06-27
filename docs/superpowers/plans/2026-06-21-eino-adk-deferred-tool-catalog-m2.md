# Eino ADK Deferred Tool Catalog M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Feed Coze-owned, policy-filtered runtime tool catalogs into Eino ADK static and deferred tool search through a single tool-set resolution path.

**Architecture:** Add `ADKToolSetProvider` so factory code resolves static and dynamic tools from one snapshot. Add a catalog adapter that wraps runtime catalog entries as Eino `tool.BaseTool` values and lets existing Eino `toolsearch` handle discovery and promotion.

**Tech Stack:** Go, Eino ADK `v0.9.9`, Eino JSON Schema tool definitions, Coze AgentThread factory and middleware tests.

---

## File Structure

- Modify `backend/application/agentthread/adk_agent_factory.go`: add `ADKToolSet` and `ADKToolSetProvider`, then prefer single-call resolution.
- Create `backend/application/agentthread/adk_tool_catalog_provider.go`: catalog DTOs, provider, Eino tool adapter, JSON Schema conversion.
- Create `backend/application/agentthread/adk_tool_catalog_provider_test.go`: provider partition, invocation, validation, and factory single-call tests.
- Modify `AGENTS.md`: document single tool-set boundary and deferred tool catalog rules.
- Modify `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`: mark M2.15 progress.

## Task 1: Add RED Tests

- [x] **Step 1: Write catalog provider tests**

Add tests proving that static and deferred catalog entries become separate
Eino tool slices, JSON Schema is preserved, deferred tools invoke the same
catalog invoker, and duplicate names across partitions are rejected.

- [x] **Step 2: Write factory single-call test**

Add a test proving `ApplicationADKAgentFactory` calls `ResolveToolSet` once
when the provider supports it, instead of independently calling static and
dynamic resolution.

- [x] **Step 3: Run focused tests and verify RED**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKRuntimeToolCatalogProvider|TestADKAgentFactoryUsesToolSetProvider' -count=1
```

Expected: FAIL because the catalog provider and tool-set provider do not exist.

## Task 2: Implement Provider And Factory Wiring

- [x] **Step 1: Add provider implementation**

Create `adk_tool_catalog_provider.go` with catalog interfaces, DTOs,
validation, JSON Schema parsing, and `tool.InvokableTool` adapter.

- [x] **Step 2: Wire factory single-call resolution**

Modify `ApplicationADKAgentFactory.Build` to prefer `ADKToolSetProvider` when
available and keep the existing legacy provider path for current tests.

- [x] **Step 3: Run focused tests and verify GREEN**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKRuntimeToolCatalogProvider|TestADKAgentFactoryUsesToolSetProvider' -count=1
```

Expected: PASS.

## Task 3: Regression Verification

- [x] **Step 1: Run dynamic tool search regressions**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKMiddlewareUsesClientToolSearch|TestADKMiddlewareUsesNativeToolSearch|TestADKToolBudget' -count=1
```

Expected: PASS.

- [x] **Step 2: Update docs**

Update `AGENTS.md` and the master roadmap with the M2.15 runtime boundary.

- [x] **Step 3: Run full package verification**

Run:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```

Expected: PASS.

- [x] **Step 4: Run diff whitespace check**

Run:

```bash
git diff --check
```

Expected: no output.
