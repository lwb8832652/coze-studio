# Skill Basic Create M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first basic custom Skill creation form to the skill
configuration page.

**Architecture:** The skill page keeps Coze as the durable Skill control plane.
The new create hook calls the existing Workbench `CreateSkill` API with a
complete initial `CustomSkill` payload, then reloads `ListSkills`. The import
panel remains separate behind `导入技能`.

**Tech Stack:** React, TypeScript, Coze Design Button/Input/TextArea, Vitest,
Workbench Skill API schema.

---

## Result

M5.2 adds a `新建技能` panel backed by the existing `CreateSkill` endpoint.

## Completed

- Added frontend coverage for creating a custom skill from the basic form.
- Added `createSkill` wiring to the skill page through `useSkillCreateActions`.
- Split toolbar actions so `创建技能` opens the create panel and `导入技能`
  opens the import panel.
- Sent the complete initial `CustomSkill` payload with default version,
  schemas, executor, permissions, and enabled state.
- Refreshed the skill list after create success.
- Fixed the runtime enum import in the create hook so
  `workbenchSkill.SkillType.CustomSkill` is available during tests and runtime.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/skill/__tests__/skill.test.tsx --testNamePattern "creates a custom skill"`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`
- `git diff --check`

## Remaining

Base metadata editing after creation, delete, category management, runtime
Skill middleware integration, resource mounts, allowed-tool policy,
scanner/quarantine, and browser E2E remain separate M5 work.
