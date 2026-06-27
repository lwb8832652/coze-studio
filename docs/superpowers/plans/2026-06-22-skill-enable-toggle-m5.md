# Skill Enable Toggle M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add enable and disable management to the skill configuration list.

**Architecture:** The skill row reuses the existing Workbench `UpdateSkill`
API and sends a complete skill snapshot with only `enabled` inverted. The page
refreshes from `ListSkills` after success so UI state follows server state.

**Tech Stack:** React, TypeScript, Coze Design Button, Vitest, Workbench Skill
API schema.

---

## Result

M5.1 adds a row-level `启用` / `停用` action backed by the existing
`UpdateSkill` endpoint.

## Completed

- Added frontend coverage for toggling a skill from enabled to disabled.
- Added `updateSkill` wiring to the skill page.
- Sent the existing skill fields unchanged while flipping only `enabled`.
- Refreshed the list after update success.
- Kept errors in the existing page error surface.
- Split the skill page and version panel into smaller component, hook, and
  utility files so the M5 frontend slice satisfies the repository lint
  function/file-size rules.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/skill/__tests__/skill.test.tsx --testNamePattern "toggles enabled state"`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`
- `git diff --check`

## Remaining

Skill create form, base metadata editing, delete, category management,
runtime Skill middleware integration, resource mounts, allowed-tool policy,
scanner/quarantine, and browser E2E remain separate M5 work.
