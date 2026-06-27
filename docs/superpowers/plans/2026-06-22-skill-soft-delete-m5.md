# Skill Soft Delete M5 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Workbench Skill deletion with production soft-delete semantics.

**Architecture:** `skills.deleted_at` is the lifecycle marker. Normal Skill
APIs remain active-only, while version and resource snapshots stay preserved
for future restore/audit paths.

**Tech Stack:** Go, GORM, Atlas migrations, Hertz routing, Thrift/API schema,
React, TypeScript, Coze Design Button, Vitest.

---

## Result

M5.5 adds `DELETE /api/workbench/skills/:skill_id`, repository/domain/application
soft-delete behavior, Workbench Skill API wiring, and a confirmed frontend
`删除` row action.

## Completed

- Added `skills.deleted_at` migration, latest Atlas schema update, and Atlas
  checksum regeneration.
- Added `DeletedAt` to the Skill entity and MySQL mapping.
- Added active-only filters for repository `Get`, `List`, and `Update`.
- Added repository `Delete` that soft-deletes only active rows, disables the
  Skill, and preserves versions/resources.
- Added domain/application/handler/router support for `DeleteSkill`.
- Added active Skill gates before normal version and resource read APIs, so
  deleted Skill snapshots are preserved but not exposed through default APIs.
- Added Thrift and frontend API schema for `DeleteSkill`.
- Added frontend delete action with confirmation, row loading, stale test
  result cleanup, and list refresh.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./domain/skill/service -run 'TestServiceDeleteBlocksVersionAPIsButKeepsSnapshots|TestServiceDeleteHidesSkillAndKeepsVersions|TestServiceListVersions|TestServiceListVersionResources' -count=1`
- `go test ./domain/skill/repository ./domain/skill/service ./application/skill -count=1`
- `go test ./api/handler/coze -run TestDeleteSkillHandlerSoftDeletesSkill -count=1`
- `go test ./api/router/coze -run TestRegisterIncludesWorkbenchSkillVersionRoutes -count=1`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx --testNamePattern "soft deletes"`
- `npx vitest run src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx`
- `npx eslint src/pages/skill/index.tsx src/pages/skill/skill-page-components.tsx src/pages/skill/skill-page-hooks.ts src/pages/skill/skill-version-panel.tsx src/pages/skill/skill-version-panel-hooks.ts src/pages/skill/skill-version-panel-sections.tsx src/pages/skill/skill-version-panel-utils.ts src/pages/skill/service.ts src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx --quiet`
- `docker run --rm -v "$PWD":/work -w /work/docker/atlas arigaio/atlas:0.35.0-community-alpine migrate hash`
- `docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

Restore, deleted-only browsing, hard-delete retention cleanup, marketplace
uninstall semantics, audit/quarantine review, allowed-tool policy UI,
runtime Skill middleware integration, resource virtual mounts, and browser E2E
remain separate M5 or production acceptance work.
