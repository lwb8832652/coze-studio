# Skill Soft Delete Design

## Goal

M5.5 closes the visible Skill deletion gap by adding a production-oriented
soft-delete lifecycle for Workbench Skills while preserving historical
versions and resource snapshots for future restore, audit, and retention
work.

## Scope

- Add `skills.deleted_at BIGINT NOT NULL DEFAULT 0`.
- Add `DELETE /api/workbench/skills/:skill_id`.
- Treat `deleted_at = 0` as the active Skill set for default product APIs.
- Soft-delete by setting `deleted_at`, updating `updated_at`, and disabling
  the Skill.
- Preserve `skill_versions` and `skill_resources` rows. They remain durable
  snapshots and must not be physically removed by the delete action.
- Add a frontend row action named `删除` with confirmation and per-row loading.
- Reload the Skill list after successful delete and clear stale test-run state
  for the deleted Skill.

## Non-Goals

- No restore API in this slice.
- No physical cleanup, retention scheduler, or archive export for deleted-only
  records.
- No marketplace uninstall behavior.
- No scanner/quarantine/audit review UI.
- No runtime Skill middleware change beyond active-only repository reads.

## Backend Behavior

`SkillRepository.Delete` is a compare-and-set style soft delete over active
rows only. It updates only `id = ? AND deleted_at = 0`; already-deleted or
missing rows return not found behavior. The update sets:

- `deleted_at` to the current millisecond timestamp.
- `updated_at` to the same timestamp.
- `enabled` to false.

Default repository reads must exclude deleted rows:

- `Get` requires `deleted_at = 0`.
- `List` requires `deleted_at = 0`.
- `Update` requires `deleted_at = 0`.

The domain service adds an active Skill gate before version and resource read
APIs. This means deleted Skill snapshots are still preserved in storage but
are not exposed through normal version list, resource list, export, edit,
rollback, or test-run flows. A future restore/audit endpoint can read deleted
history through an explicit lifecycle API with authorization and audit.

## Frontend Behavior

The Skill list row shows `删除` next to the existing management actions. The
action is disabled while the row is running, updating, or already deleting.
On click, the UI asks for confirmation with the current Skill name. If
confirmed, it calls the Workbench DELETE API with only `skill_id`, clears any
stale test result for that Skill, reloads the list, and reports bounded errors
through the existing page error surface.

## Security And Consistency

Deletion is metadata-only and content-safe:

- The frontend never sends Skill content, versions, resources, tool arguments,
  object keys, or runtime payloads to delete a Skill.
- Normal APIs do not expose deleted Skill bodies or resources by ID after
  deletion.
- Historical rows stay available for a future authorized restore/audit path.
- Hard delete must remain a separate lifecycle decision with retention,
  audit, and cleanup guarantees.

## Migration

`docker/atlas/migrations/20260622000100_skills_deleted_at.sql` adds
`deleted_at` and `idx_skills_space_deleted`. The latest Atlas schema mirrors
the new column and index, and `atlas.sum` must be regenerated with Atlas
Community `v0.35.0`.
