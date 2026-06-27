# MCP Health And Registry Boundary M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe MCP health metadata and a metadata-only MCP registry index
boundary for future Tool Registry/runtime policy work.

**Architecture:** Extend the existing Workbench MCP server model with health
metadata, persist it in the MySQL catalog, update it after successful test
calls, and expose a registry-entry endpoint that derives safe tool names from
enabled MCP server tool definitions.

**Tech Stack:** Go, GORM, Hertz, Atlas, TypeScript API schema, Vitest.

---

## Result

M6.3 adds health metadata to MCP server responses and exposes
`GET /api/workbench/mcp_tools/registry_entries?space_id=...`.

## Completed

- Added `health_status`, `health_checked_at`, `health_latency_ms`, and
  `health_error` to backend and frontend MCP server schemas.
- Persisted health metadata in `mcp_tool_servers`.
- Added `Catalog.UpdateHealth` for in-memory and MySQL catalogs.
- Updated successful Workbench MCP `TestCall` to mark the server `healthy` with
  checked time and latency.
- Added registry entry response models and `ListRegistryEntries`.
- Added `ListMCPToolRegistryEntries` handler and router registration before
  dynamic `/:server_id` routes.
- Added frontend API schema and tools service export for registry entries.
- Added backend, handler, router, and frontend service tests.
- Added Atlas migration `20260623000200_mcp_tool_health.sql`.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./application/mcptool -count=1`
- `go test ./api/handler/coze -run 'TestWorkbenchMCPToolHandlersManageServerAndTestCall|TestWorkbenchMCPToolHandlersListRegistryEntries' -count=1`
- `go test ./api/router/coze -run TestRegisterIncludesWorkbenchMCPToolRoutes -count=1`
- `npx vitest run src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts`
- `npx eslint src/pages/tools/index.tsx src/pages/tools/service.ts src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts --quiet`
- `atlas migrate validate --dir file://docker/atlas/migrations`

## Remaining

Real MCP health probes, health history, unified Tool Registry tables,
authorization policy, audit, OAuth/session lifecycle, stdio sandboxing, runtime
Eino MCP adapter invocation, output budgets, and browser E2E remain separate
M6 or production acceptance work.
