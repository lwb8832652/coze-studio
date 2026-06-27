# Skill Category Filter Design

## Goal

M5.4 replaces the temporary skill list filter labels with explicit Skill
categories that match the current Workbench Skill type model.

## Scope

- Keep filtering client-side against the already loaded `ListSkills` result.
- Support these categories:
  - `全部技能` for all records.
  - `自定义` for `CustomSkill`.
  - `公共` for `PublicSkill`.
  - `内置` for `DeerSkill` bootstrap skills.
  - `脚本` for legacy `Script`.
  - `工作流` for legacy `Workflow`.
- Preserve existing keyword search behavior and combine keyword + category
  filters.

## Non-Goals

- No new backend list API, category migration, or persistence field.
- No marketplace discovery workflow.
- No delete behavior.
- No runtime skill selection policy change.

## UI Behavior

The toolbar filter group shows the explicit Skill categories. Selecting a
category narrows the visible list to that Skill type; `全部技能` shows all
loaded Skills.

## Consistency

The filter uses the existing `workbenchSkill.SkillType` enum. `内置` maps to
`DeerSkill` because those are the bootstrap DeerFlow-style Skills in the
current schema. This keeps the UI aligned with API data without inventing a
second category model.

## Test Coverage

Frontend coverage verifies that `getVisibleSkills` returns the expected
CustomSkill, PublicSkill, DeerSkill, and legacy Workflow records for each
category.
