# MCP Tool Config Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the Phase 22 MCP tool configuration backend to a new workbench Tool configuration page and fully migrate the old task-trigger route to the new tools route.

**Architecture:** Add a frontend API schema namespace for `workbenchTool`, expose a small page service, and replace the placeholder task-trigger page with a real MCP tool configuration surface. Move the workspace route to `/space/:space_id/tools`, point the sidebar menu at that route, and remove the old `/space/:space_id/task-trigger` route to avoid mixed route recognition.

**Tech Stack:** React 18, TypeScript, Vitest, existing `@coze-arch/bot-api` createAPI, existing prototype workspace styling.

---

## Scope

- Add `workbenchTool` exports in `frontend/packages/arch/api-schema`.
- Add frontend API calls for:
  - list MCP tool servers
  - upsert MCP tool server
  - get MCP tool server
  - test MCP tool call
- Add `frontend/apps/coze-studio/src/pages/tools/service.ts`.
- Add `frontend/apps/coze-studio/src/pages/tools/index.tsx`.
- Replace the old task-trigger route with the new tools route.
- Remove old task-trigger routing and page references instead of keeping a redirect.
- Build the tools page:
  - load and render MCP server cards
  - create a default stdio MCP server config
  - test the first configured tool
  - show loading, error, and test output states
- Change the workspace menu label and path from `任务触发器` / `task-trigger` to `工具` / `tools`.

## Non-Goals

- No real MCP transport execution in the browser.
- No secret editor/encryption UX.
- No advanced schema editor.
- No Agent Harness automatic tool loading.
- No LangGraph API.

## Testing

- API schema service tests verify every tool endpoint is exported.
- Tools page tests verify list rendering, create flow, and test-call flow.
- Workspace submenu tests verify the visible menu item is `工具`.
- Existing workbench/task tests remain unchanged.
