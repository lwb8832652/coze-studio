# Skill Permission Edit Design

## Goal

M5.6 adds a basic Skill permission editor so the Workbench can manage the
durable permission JSON needed by DeerFlow-style Skills and future Eino Skill
allowed-tool policy.

## Scope

- Add a `权限配置` section to the Skill version management drawer.
- Edit `permissions.network` through a controlled checkbox.
- Edit `permissions.allowed_tools` through a newline/comma separated text area.
- Save through the existing `UpdateSkill` Workbench API with a complete Skill
  snapshot.
- Preserve unknown permission keys so future backend policy fields do not get
  dropped by the UI.
- Validate tool names before save using the Eino-safe name shape
  `[A-Za-z_][A-Za-z0-9_]{0,63}`.

## Non-Goals

- No Tool Registry picker in this slice.
- No MCP server/tool grant UI.
- No durable grant audit table change.
- No runtime allowed-tool enforcement change.
- No backend partial permission API.

## Frontend Behavior

The drawer initializes from `skill.permissions`. Invalid or non-object JSON is
treated as an empty permission object for the editor. `network` defaults to
false, and `allowed_tools` defaults to an empty list unless the stored value is
an array of non-empty strings.

Saving:

- Parses allowed tool names from newlines or commas.
- Trims blank entries.
- Deduplicates names while preserving first-seen order.
- Rejects unsafe names before calling the API.
- Serializes the permission object by preserving all existing keys and
  replacing only `network` and `allowed_tools`.
- Calls `UpdateSkill` with the current Skill fields unchanged except
  `permissions`.
- Refreshes the parent Skill list and shows the existing drawer notice/error
  banners.

## Consistency

This is an editing surface for Coze-owned Skill metadata, not an authorization
decision by itself. Runtime enforcement remains behind the existing
Coze/Eino tool policy layer. A future registry picker should feed the same
`allowed_tools` field or migrate it through a backend-owned compatibility
adapter rather than creating a parallel grant shape.
