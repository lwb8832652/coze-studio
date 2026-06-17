# Macro Stage B - Skill Configuration Backend

## Frozen Scope

Macro Stage B replicates Deer-flow style skill configuration on top of the existing Coze Studio skill compatibility layer. This document tracks the backend slices completed so far. It does not add frontend pages, new menu behavior, MCP tool permissions, IM channels, or unrelated security scanning.

## Completed In This Slice

- Extend the skill entity type vocabulary toward Deer-flow and compatibility sources:
  - `deer_skill`
  - `public_skill`
  - `custom_skill`
  - `coze_script`
  - `coze_workflow`
- Add `SkillVersion` as the backend version snapshot model.
- Add repository support for creating and listing skill versions.
- Record a version snapshot when a skill is imported, created, or updated.
- Expose domain-level `ListVersions`.
- Expose application-level `ListSkillVersions` with the Workbench API response shape.
- Add IDL contract and HTTP handler for `GET /api/workbench/skills/:skill_id/versions`.
- Register the version list route under the existing Workbench skill path without redirects.
- Parse Deer-flow style `SKILL.md` files with YAML frontmatter and Markdown body.
- Import `SKILL.md` text files as `deer_skill` records and preserve the original `SKILL.md` content in version snapshots.
- Parse `.skill` zip archives with safe path validation, file count and size limits, unique `SKILL.md` detection, and symlink rejection.
- Capture archive resource metadata for files under the same skill root:
  - relative path
  - byte size
  - SHA-256 hash
- Expose Deer-flow compatible skill types through the Workbench API enum extension:
  - `DeerSkill`
  - `PublicSkill`
  - `CustomSkill`
- Keep existing Workbench skill API methods compatible.

## Current Semantics

The existing `skills` table remains the live skill compatibility table. The new `skill_versions` repository model stores immutable snapshots for history, future rollback, and export workflows.

For create/update flows, `SkillMD` is generated from the existing skill entity fields. For `SKILL.md` and `.skill` archive imports, the original Markdown entrypoint content is stored in the version snapshot so the future editor, rollback, and export flows can preserve Deer-flow skill instructions.

`.skill` archive parsing is intentionally read-only in this backend slice. Resource files are validated and fingerprinted, but they are not persisted, executed, or injected into the runtime yet.

Until the next full thriftgo/hz generation pass, the new Workbench skill version DTOs are kept in a small extension file next to the generated skill model. The IDL remains the source contract.

## Deferred Within Macro Stage B

- Resource persistence for `.skill` archive assets, scripts, and references.
- Custom skill content editing and rollback.
- Skill enablement injection into the Go Agent Harness runtime.
- Frontend skill list/detail/editor/test-run replication.

## Explicitly Deferred To Later Macro Stages

- MCP production permissions and server config.
- IM channels.
- Complex security scanning.
- Final production acceptance.
