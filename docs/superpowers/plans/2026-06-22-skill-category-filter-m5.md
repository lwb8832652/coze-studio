# Skill Category Filter M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace temporary Skill filter labels with explicit category filters.

**Architecture:** Keep category filtering in `getVisibleSkills` using a single
map from UI filter keys to `workbenchSkill.SkillType` values.

**Tech Stack:** React, TypeScript, Coze Design ButtonGroup/Button, Vitest,
Workbench Skill API schema.

---

## Result

M5.4 adds explicit category filters for all current Skill types.

## Completed

- Added frontend coverage for custom, public, bootstrap, and legacy category
  filtering.
- Expanded `SkillTypeFilter` to include `custom`, `public`, and `bootstrap`.
- Mapped `bootstrap` to `DeerSkill`.
- Updated toolbar labels to `全部技能`, `自定义`, `公共`, `内置`, `脚本`, and
  `工作流`.
- Preserved keyword search and existing script/workflow filtering.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/skill/__tests__/skill.test.tsx --testNamePattern "filters skills"`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`
- `git diff --check`

## Remaining

Delete, server-side category queries, marketplace behavior, permissions and
allowed-tool editing, runtime Skill middleware integration, resource mount
policy, scanner/quarantine, and browser E2E remain separate M5 work.
