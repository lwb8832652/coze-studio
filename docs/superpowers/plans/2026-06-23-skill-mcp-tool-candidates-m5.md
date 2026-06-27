# Skill MCP Tool Candidates M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the Skill tool candidate API from static built-ins to a
Coze-owned MCP provider while preserving the metadata-only grant contract.

**Architecture:** `application/skill` owns the public candidate API and final
normalization. It depends on a narrow `ToolCandidateProvider` interface. The
current `application/mcptool` service implements that interface by mapping
enabled MCP server tool definitions into safe grant names.

**Tech Stack:** Go, Hertz handler/router, Thrift/API schema, React,
TypeScript, Coze Design Button, Vitest.

---

## Result

M5.8 wires configured MCP tools into
`GET /api/workbench/skills/tool_candidates?space_id=...` as metadata-only
candidate rows with source information. The Skill permission drawer shows the
MCP server source on candidate buttons and still saves only safe grant names in
`permissions.allowed_tools`.

## Completed

- Added optional `source`, `source_id`, and `source_name` fields to
  `SkillToolCandidate` in backend API models, Thrift IDL, and frontend schema.
- Added `ToolCandidateProvider` to the Skill application service.
- Added Skill-side candidate normalization for safe names, non-empty
  descriptions, default category/visibility, metadata bounds, and duplicate
  filtering.
- Kept built-in ADK candidates first and marked them as `source=builtin`.
- Implemented the Workbench MCP tool service as a candidate provider.
- Generated MCP grant names as `mcp_{server_id}_{sanitized_tool_name}`, with
  hash truncation when needed to stay inside Eino's 64-character limit.
- Filtered disabled MCP servers and incomplete MCP tool definitions.
- Injected the MCP provider into Skill service initialization in the
  application bootstrap path.
- Updated the Skill permission drawer to show MCP server source metadata while
  toggling the safe grant name.
- Added backend and frontend tests for provider filtering and MCP button
  behavior.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./application/skill ./application/mcptool -count=1`
- `go test ./api/handler/coze -run TestListSkillToolCandidatesHandlerReturnsSafeMetadata -count=1`
- `go test ./api/router/coze -run TestRegisterIncludesWorkbenchSkillVersionRoutes -count=1`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-permission-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`

## Remaining

Durable MCP persistence, secret masking, health/status metadata, Tool Registry
authorization, runtime Eino MCP adapter execution, audit events, per-user
policy, scanner/quarantine, resource mounts, and browser E2E remain separate
M6 or production acceptance work.
