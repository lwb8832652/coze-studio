# TD-SKILL-002 Skill Settings Style

## DeerFlow Source Baseline

- Settings section:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/settings/settings-section.tsx`
- Skill settings page:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/settings/skill-settings-page.tsx`
- zh-CN copy:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/i18n/locales/zh-CN.ts`

DeerFlow renders Skills inside a settings section, not as a management table:

- title `技能`;
- description `管理 Agent Skill 配置和启用状态。`;
- only `公共` and `自定义` tabs;
- right-side `新建技能` action;
- item rows show name, description, and enabled switch;
- loading is muted text;
- empty state shows `还没有技能`, the `/skills/custom` hint, and
  `创建你的第一个技能`.

## Coze Implementation

- Removed the Coze workspace top guidance bar from Skill page content.
- Replaced centered page title with a DeerFlow-style left-aligned settings
  header.
- Reduced visible filters to Public/Custom.
- Removed visible search, type filters, status chips, date, run/delete actions,
  and refresh entry from the main list surface.
- Kept Coze enhanced actions (`版本管理`, `试运行`, `删除`) behind the row
  `更多` menu so the first viewport matches DeerFlow.
- Changed loading and empty states to match DeerFlow structure and copy.

## Verification

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx
```

Passed on 2026-06-29:

- 13 test files passed.
- 118 tests passed.

Browser validation on an accessible local workspace:

- URL:
  `http://localhost:8080/space/7645565700475453440/skill`
- Verified:
  - no `Aime 专属助理准备好` / `去聊天专属助理` top guidance text;
  - header text is `技能管理 Agent Skill 配置和启用状态。`;
  - toolbar text is `公共自定义新建技能`;
  - legacy `.coze-prototype-empty` is absent;
  - 22 imported DeerFlow builtin skills render as settings rows.
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-SKILL-002-skill-page-deerflow-empty-state.png`

Browser validation on the active workspace:

- URL:
  `http://localhost:8080/space/7656275718757679104/skill`
- Verified:
  - visible toolbar actions are `公共`, `自定义`, and `新建技能`;
  - no standalone `刷新` / refresh action is present;
  - row actions remain behind per-skill `更多` buttons.
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-SKILL-002-skill-page-current-no-refresh.png`

## Remaining Acceptance

- If a refresh control is observed again, capture whether it is inside the
  Skill version/resource panel or from a stale frontend build; it is not part
  of the DeerFlow Skill settings page baseline.
- Next Skill slices:
  - `TD-SKILL-003`: create/import/export and version-resource panel parity.
  - `TD-SKILL-004`: composer skill activation parity.
  - `TD-SKILL-005`: runtime skill loading parity.
