# Skill Metadata Edit Design

## Goal

M5.3 closes the basic management loop for Skills by allowing users to edit
name, description, and version from the existing version management drawer.

## Scope

- Add a `基础信息` editor at the top of the Skill version management drawer.
- Allow editing `技能名称`, `技能描述`, and `技能版本`.
- Save through the existing Workbench `UpdateSkill` API.
- Send a complete current Skill snapshot with only name, description, and
  version replaced by trimmed form values.
- Reuse the drawer's existing loading, notice, and error surfaces.
- Call the parent refresh callback after success so the list row can reload.

## Non-Goals

- No delete API or soft-delete behavior.
- No category/public marketplace workflow.
- No permissions editor, allowed-tool grants, resource mount policy, scanner,
  quarantine, or Eino runtime behavior change.
- No direct mutation of historical `SkillVersion` rows from this form.

## UI Behavior

The drawer shows `基础信息` before the `SKILL.md` and resource tabs. The save
button is disabled while another drawer action is saving, or when name,
description, or version is blank after trimming. On success, the drawer shows
`基础信息已保存` and triggers the parent `onSkillChanged` refresh.

## Consistency

The implementation follows the same full-snapshot update rule as M5.1. It
does not introduce a partial update endpoint and does not alter enabled state,
type, schemas, executor, or permissions. The backend remains the authority for
validation, authorization, version snapshot creation, and audit.

## Test Coverage

Frontend coverage verifies that the version management drawer:

- renders the metadata fields;
- accepts controlled name, description, and version edits;
- calls `UpdateSkill` with the complete expected payload;
- calls `onSkillChanged` after success;
- shows the existing success notice surface.
