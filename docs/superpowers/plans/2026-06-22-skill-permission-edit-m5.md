# Skill Permission Edit M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add basic permission and allowed-tool editing to the Skill version
management drawer.

**Architecture:** Reuse the existing full-snapshot `UpdateSkill` Workbench API.
The frontend parses and writes only the permission JSON fields it owns
(`network`, `allowed_tools`) while preserving unknown fields.

**Tech Stack:** React, TypeScript, Coze Design Button/Checkbox/TextArea,
Semi Design 2.72.3 guidance, Vitest.

---

## Result

M5.6 adds a `权限配置` editor for `network` and `allowed_tools` in the Skill
version management drawer.

## Completed

- Queried Semi MCP docs for Button and Checkbox on version `2.72.3`.
- Added controlled permission editor UI using Coze Design components.
- Added permission parsing that treats invalid/non-object JSON as empty.
- Added safe tool-name normalization with trim, comma/newline parsing,
  dedupe, and `[A-Za-z_][A-Za-z0-9_]{0,63}` validation.
- Preserved unknown permission keys when saving.
- Reused `UpdateSkill` with a complete Skill snapshot and only `permissions`
  changed.
- Added tests for preserving unknown keys, deduping allowed tools, and
  rejecting unsafe tool names before API submission.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/skill/__tests__/skill-version-panel.test.tsx --testNamePattern "permissions|tool names"`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`

## Remaining

Tool Registry picker, MCP grants, backend policy audit, durable scanner
review, runtime Skill middleware enforcement, resource virtual mounts, and
browser E2E remain separate M5 or production acceptance work.
