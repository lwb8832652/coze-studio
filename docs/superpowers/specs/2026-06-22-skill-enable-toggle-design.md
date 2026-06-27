# Skill Enable Toggle Design

## Goal

M5.1 adds the first concrete Skills parity management action after version
history: a user can enable or disable a skill from the skill configuration
list without leaving the page.

## Scope

- Reuse the existing Workbench `UpdateSkill` API instead of adding a new
  partial update contract.
- Send the full current skill snapshot with only `enabled` inverted.
- Refresh the skill list after a successful update so the row reflects the
  server state.
- Keep errors bounded in the existing page-level error area.

## Non-Goals

- No delete API or hard deletion behavior in this slice.
- No new backend contract, migration, or generated API model change.
- No runtime Skill middleware behavior change.

## UI Behavior

Each skill row shows an `启用` or `停用` action beside `试运行` and version
management. While a skill is being updated, that row action shows loading and
concurrent updates are ignored. On success, the page reloads the list and the
status pill changes between `已发布` and `已停用`.

## Security And Consistency

The frontend must not invent a partial permission model. The existing backend
`UpdateSkill` path remains the authority for validation, authorization, and
snapshot persistence. This keeps M5.1 compatible with the Go Harness behavior
that injects only enabled skills into runtime selection.
