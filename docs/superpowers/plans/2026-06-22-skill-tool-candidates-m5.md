# Skill Tool Candidates M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a backend-owned tool candidate list and wire it into Skill
permission editing.

**Architecture:** The Workbench Skill application exposes a metadata-only
candidate API seeded by current ADK built-in tool names. The frontend consumes
that API and toggles candidates into `permissions.allowed_tools`, while the
existing permission serializer remains the single save path.

**Tech Stack:** Go, Hertz handler/router, Thrift/API schema, React,
TypeScript, Coze Design Button, Vitest.

---

## Result

M5.7 adds `GET /api/workbench/skills/tool_candidates?space_id=...` and a
candidate-button UI in the Skill permission editor.

## Completed

- Added backend API model extension structs for tool candidates.
- Added `ApplicationService.ListSkillToolCandidates` with a sanitized static
  candidate list for current ADK built-ins.
- Added handler and route before `/:skill_id` so `tool_candidates` is not
  parsed as a Skill ID.
- Added IDL and frontend API schema/service wiring.
- Added candidate loading to the permission hook.
- Split permission hook code into `skill-permission-hooks.ts` to keep the
  version hook file under lint limits.
- Added candidate buttons that toggle allowed tool names through the same
  validation/serialization path as manual text editing.
- Added backend, router, handler, and frontend tests.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./application/skill -count=1`
- `go test ./api/handler/coze -run 'TestListSkillToolCandidatesHandlerReturnsSafeMetadata|TestDeleteSkillHandlerSoftDeletesSkill' -count=1`
- `go test ./api/router/coze -run TestRegisterIncludesWorkbenchSkillVersionRoutes -count=1`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-permission-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`

## Remaining

Registry-backed candidates, MCP grants, per-space authorization, health/status
badges, run-specific availability, backend policy audit, runtime Skill
middleware enforcement, resource virtual mounts, scanner/quarantine, and
browser E2E remain separate M5/M6 or production acceptance work.
