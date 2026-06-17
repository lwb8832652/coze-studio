# Macro Stage B - Skill Configuration Backend

## Frozen Scope

Macro Stage B replicates Deer-flow style skill configuration on top of the existing Coze Studio skill compatibility layer. This document covers the first backend slice only. It does not add frontend pages, new menu behavior, MCP tool permissions, IM channels, or unrelated security scanning.

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
- Expose application-level `ListSkillVersions` with a stable summary DTO.
- Keep existing Workbench skill API methods compatible.

## Current Semantics

The existing `skills` table remains the live skill compatibility table. The new `skill_versions` repository model stores immutable snapshots for history, future rollback, and export workflows.

For this slice, `SkillMD` is generated from the existing skill entity fields. Later Deer-flow `.skill` package support will replace this generated text with the actual `SKILL.md` entrypoint content and frontmatter.

## Deferred Within Macro Stage B

- HTTP handler and IDL exposure for listing versions.
- Deer-flow `SKILL.md` frontmatter parser.
- `.skill` archive import and safe extraction.
- Custom skill content editing and rollback.
- Skill enablement injection into the Go Agent Harness runtime.
- Frontend skill list/detail/editor/test-run replication.

## Explicitly Deferred To Later Macro Stages

- MCP production permissions and server config.
- IM channels.
- Complex security scanning.
- Final production acceptance.
