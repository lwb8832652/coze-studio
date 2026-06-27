# Skill Basic Create Design

## Goal

M5.2 adds the first custom Skill creation path to the skill configuration
page. A user can open a small creation panel, enter a name and description,
and create an enabled `CustomSkill` through the existing Workbench
`CreateSkill` API.

## Scope

- Keep the product surface named `技能配置` and keep the primary action labeled
  `创建技能`.
- Split creation from import: `创建技能` opens the basic create form, while
  `导入技能` keeps the existing file/content import flow.
- Create only `CustomSkill` records in this slice.
- Use the existing backend validation and version snapshot behavior by calling
  `CreateSkill` with a complete initial payload.
- Reload the skill list after successful creation so the row reflects server
  state.
- Keep errors bounded in the existing page-level error area.

## Create Payload

The basic create form sends:

- `space_id` from the current route.
- `name` and `description` from trimmed user input.
- `type = CustomSkill`.
- `version = 1.0.0`.
- `enabled = true`.
- `input_schema = {}` and `output_schema = {}`.
- `executor = {}`.
- `permissions = {"network":false,"allowed_tools":[]}`.

These defaults are intentionally minimal. Runtime execution policy, resource
mounts, allowed tools, scanners, quarantine, and rich `SKILL.md` editing remain
separate M5 work.

## UI Behavior

The toolbar exposes two explicit actions. `创建技能` toggles a `新建技能` panel
with `新技能名称`, `新技能描述`, and `保存技能`. The save action stays disabled
until both fields have non-empty trimmed values and a workspace `space_id`
exists. While saving, the button uses the component loading state and repeated
submissions are ignored by the hook guard.

`导入技能` continues to toggle the import panel and is not overloaded by the
creation flow.

## Security And Consistency

The frontend does not construct Skill runtime behavior, parse SKILL content,
or grant tools in M5.2. The backend remains the authority for validation,
authorization, audit, version snapshot creation, and eventual execution
policy. The runtime enum import must be a value import because the create
request needs `workbenchSkill.SkillType.CustomSkill` at runtime.

## Test Coverage

Frontend coverage verifies that the form:

- opens from `创建技能`;
- accepts controlled name and description input;
- enables `保存技能` only after required fields are present;
- calls `CreateSkill` with the complete default payload;
- refreshes the list and displays the newly created skill.
