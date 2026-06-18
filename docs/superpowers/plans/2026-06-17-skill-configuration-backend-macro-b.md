# Macro Stage B - Skill Configuration Backend

## Frozen Scope

Macro Stage B replicates Deer-flow style skill configuration on top of the existing Coze Studio skill compatibility layer. This document tracks the backend slices completed so far. It does not add frontend pages, new menu behavior, MCP tool permissions, IM channels, or unrelated security scanning.

Per the latest delivery decision, MCP and IM are moved out of the current first-stage scope and into Phase 2. Current Macro Stage B must not start MCP server configuration, MCP runtime transport, MCP permission hardening, IM inbound/outbound messages, IM channel binding, or IM channel UI.

## Phase 2 Boundary

The following work is explicitly Phase 2:

- MCP tool configuration backend and frontend.
- MCP stdio/SSE/HTTP transports, OAuth, secret masking, health checks, and session pool.
- MCP runtime invocation through Tool Registry.
- IM channel configuration, inbound/outbound routing, channel-thread binding, and channel-specific run policies.
- IM-related task detail badges, filters, and message handoff behavior.

The current first-stage work can keep neutral data fields such as `source`, `enable_mcp`, or future extension points when they already exist, but it must not implement or expand MCP/IM behavior.

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
- Persist `.skill` archive resources as immutable version attachments in `skill_resources`:
  - `skill_id`
  - `version_id`
  - relative path
  - file content
  - byte size
  - SHA-256 hash
- Expose version resource reads through the Workbench skill backend:
  - domain-level `ListVersionResources`
  - application-level `ListSkillVersionResources`
  - IDL contract and HTTP handler for `GET /api/workbench/skills/:skill_id/versions/:version_id/resources`
  - route registration under the existing Workbench skill path without redirects
  - base64 encoded content for Markdown, scripts, assets, and binary-safe future export/edit flows
- Expose version `.skill` archive export through the Workbench skill backend:
  - application-level `ExportSkillVersion`
  - IDL contract and HTTP handler for `GET /api/workbench/skills/:skill_id/versions/:version_id/export`
  - route registration under the existing Workbench skill path without redirects
  - base64 encoded zip content with `Content-Type` metadata
  - preserved `SKILL.md` entrypoint plus version-scoped resource files
- Expose skill version rollback through the Workbench skill backend:
  - domain-level `RollbackVersion`
  - application-level `RollbackSkillVersion`
  - IDL contract and HTTP handler for `POST /api/workbench/skills/:skill_id/versions/:version_id/rollback`
  - route registration under the existing Workbench skill path without redirects
  - restore live skill metadata and execution configuration from the selected immutable version snapshot
  - create a new current snapshot and copy selected version resources so latest history reflects the rollback result
- Expose version resource editing through the Workbench skill backend:
  - domain-level `UpdateVersionResource`
  - application-level `UpdateSkillVersionResource`
  - IDL contract and HTTP handler for `PUT /api/workbench/skills/:skill_id/versions/:version_id/resources`
  - route registration under the existing Workbench skill path without redirects
  - JSON body with resource `path` and base64 encoded content to avoid path wildcard ambiguity
  - safe resource path validation, `SKILL.md` conflict rejection, per-file size limit reuse, and SHA-256 refresh
  - create a new current snapshot from the selected version and attach the edited resource set without mutating historical resources
- Expose `SKILL.md` entrypoint editing through the Workbench skill backend:
  - domain-level `UpdateVersionContent`
  - application-level `UpdateSkillVersionContent`
  - IDL contract and HTTP handler for `PUT /api/workbench/skills/:skill_id/versions/:version_id/content`
  - route registration under the existing Workbench skill path without redirects
  - JSON body with raw `skill_md` content for the future custom skill editor
  - reuse the existing `SKILL.md` parser, declaration validation, and entrypoint size limit
  - update live skill metadata from edited frontmatter while preserving the selected snapshot's schemas, executor, and permissions
  - create a new immutable current snapshot and copy selected version resources without mutating historical versions
- Change `skill_versions` from unique semantic versions to append-only snapshots:
  - drop `uk_skill_versions_skill_version`
  - add non-unique `idx_skill_versions_skill_version`
  - allow update and rollback flows to preserve history even when the semantic version string repeats
- Add a production migration for the resource table and the current skill-version snapshot columns.
- Expose Deer-flow compatible skill types through the Workbench API enum extension:
  - `DeerSkill`
  - `PublicSkill`
  - `CustomSkill`
- Keep existing Workbench skill API methods compatible.

## Current Semantics

The existing `skills` table remains the live skill compatibility table. The new `skill_versions` repository model stores immutable snapshots for history, future rollback, and export workflows.

For create/update flows, `SkillMD` is generated from the existing skill entity fields. For `SKILL.md` and `.skill` archive imports, the original Markdown entrypoint content is stored in the version snapshot so the future editor, rollback, and export flows can preserve Deer-flow skill instructions.

`.skill` archive resources are validated, fingerprinted, stored as version-scoped attachments, readable through the Workbench skill version resource API, and editable through a version-scoped resource update API. Resource content is returned and accepted as base64 so text and binary assets share one transport shape. Resource edits create a new current snapshot and do not mutate historical resources. Resources are not executed through the runtime yet.

Version snapshots can now be exported as `.skill` zip archives from the version export endpoint. The archive is assembled from the immutable version `SKILL.md` snapshot and stored version resources, then returned as base64 JSON payload for frontend download flows. Resource content and the `SKILL.md` entrypoint can now be edited through backend APIs. Both operations append a new snapshot so the frontend editor can preserve history while working from any selected version.

Version rollback now restores the live skill row from an immutable snapshot, preserving the current skill ID and space binding. Rollback also writes a new latest snapshot and clones the selected snapshot's resources, so subsequent history reads, exports, and future runtime lookup can treat the rollback result as the current skill state. Resource and entrypoint edits use the same latest-snapshot pattern. Entrypoint edits take name, description, type, semantic version, enabled state, and Markdown instructions from the submitted `SKILL.md`; explicit execution configuration remains version-scoped and is copied from the selected snapshot. The version table is snapshot-oriented, so the same semantic version string may appear multiple times.

Until the next full thriftgo/hz generation pass, the new Workbench skill version DTOs are kept in a small extension file next to the generated skill model. The IDL remains the source contract.

## Deferred Within Macro Stage B

- Skill enablement injection into the Go Agent Harness runtime.
- Frontend skill list/detail/editor/test-run replication.

## Explicitly Deferred To Phase 2

- MCP production permissions, server config, transports, runtime invocation, and tool page replication.
- IM channels, channel-thread binding, inbound/outbound delivery, and channel UI integration.

## Explicitly Deferred To Later Macro Stages

- Complex security scanning.
- Final production acceptance.
