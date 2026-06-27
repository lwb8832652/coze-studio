# MCP Secret Masking And Delete M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent MCP auth secrets from leaking through Workbench API
responses and add a default delete lifecycle for MCP server configs.

**Architecture:** Keep the existing Workbench MCP API shape. Mask auth at the
application service response boundary, preserve masked fields during updates,
and add `Catalog.Delete` so the MySQL implementation can soft-delete rows.

**Tech Stack:** Go, GORM, Hertz, React, TypeScript, Vitest.

---

## Result

M6.2 masks MCP auth in create/list/get/delete responses, preserves stored
secrets when masked auth is submitted on update, and exposes
`DELETE /api/workbench/mcp_tools/:server_id` with frontend row-level deletion.

## Completed

- Added recursive MCP auth masking in `application/mcptool`.
- Added masked-auth merge logic so `********` values preserve stored secrets on
  update.
- Kept raw auth only inside the catalog/storage boundary.
- Added `Catalog.Delete`.
- Implemented in-memory delete for tests and MySQL soft-delete through
  `deleted_at`.
- Added `ApplicationService.DeleteServer`.
- Added Workbench `DeleteMCPToolServer` handler and router registration.
- Added frontend API client export and tools-page delete action.
- Added backend, router, handler, and frontend tests.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./application/mcptool -count=1`
- `go test ./api/handler/coze -run 'TestWorkbenchMCPToolHandlersMaskAuth|TestWorkbenchMCPToolHandlersDeleteServer|TestWorkbenchMCPToolHandlersManageServerAndTestCall' -count=1`
- `go test ./api/router/coze -run TestRegisterIncludesWorkbenchMCPToolRoutes -count=1`
- `npx vitest run src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts`
- `npx eslint src/pages/tools/index.tsx src/pages/tools/service.ts src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts --quiet`

## Remaining

Encrypted secret storage, restore API, authorization checks, audit events,
health/status metadata, normalized Tool Registry indexing, OAuth/session
lifecycle, stdio sandboxing, runtime Eino MCP adapter invocation, browser E2E,
and broader UI edit forms remain separate M6 or production acceptance work.
