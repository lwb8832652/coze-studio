# Skill Metadata Edit M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add basic Skill metadata editing to the version management drawer.

**Architecture:** The drawer owns transient form state for name, description,
and version. Saving calls the existing `UpdateSkill` API with a complete
snapshot and then asks the parent skill list to refresh.

**Tech Stack:** React, TypeScript, Coze Design Input/TextArea/Button, Vitest,
Workbench Skill API schema.

---

## Result

M5.3 adds a `基础信息` editor in the Skill version management drawer.

## Completed

- Added frontend coverage for editing name, description, and version.
- Added `useSkillMetadataActions` with local form state synced from the
  current Skill.
- Added `SkillMetadataEditor` using Coze/Semi Input, TextArea, and Button.
- Saved through `UpdateSkill` with a complete current Skill snapshot.
- Reused existing drawer saving, notice, and error state.
- Triggered `onSkillChanged` after success so the outer list can refresh.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/skill/__tests__/skill-version-panel.test.tsx --testNamePattern "updates base skill metadata"`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`
- `git diff --check`

## Remaining

Delete, category management, permissions/allowed-tool editing, runtime Skill
middleware integration, resource mount policy, scanner/quarantine, and browser
E2E remain separate M5 work.
