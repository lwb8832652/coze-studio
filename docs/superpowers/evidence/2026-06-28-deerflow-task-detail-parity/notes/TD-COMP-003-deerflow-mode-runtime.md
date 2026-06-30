# TD-COMP-003 DeerFlow Mode Runtime

Date: 2026-06-30

## DeerFlow reference

- Source verified:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx`
  defines native modes `flash`, `thinking`, `pro`, `ultra`.
- Default behavior:
  `getResolvedMode(mode, supportsThinking)` returns `pro` when the selected
  model supports thinking and no explicit mode is selected.
- DeerFlow model metadata exposes `supports_thinking` and
  `supports_reasoning_effort`; mode controls the run context, while provider
  reasoning effort is shown/sent only when the selected model supports it.
- Submit context verified in
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/threads/hooks.ts`:
  `thinking_enabled = mode !== "flash"`,
  `is_plan_mode = mode === "pro" || mode === "ultra"`,
  `subagent_enabled = mode === "ultra"`.

## Coze implementation

- Workbench mode type now uses native DeerFlow values:
  `flash`, `thinking`, `pro`, `ultra`.
- The composer mode trigger is controlled by parent state, not local-only UI
  state, so the displayed selection and submitted run config stay aligned.
- Default mode is `pro` for homepage, task detail follow-up, and retry payloads.
- `createWorkbenchRunConfig` serializes:
  `mode`, `thinking_enabled`, `is_plan_mode`, and `subagent_enabled`.
- `reasoning_effort` is serialized only when Workbench runtime reasoning is
  explicitly enabled. Native DeerFlow mode selection no longer forces provider
  reasoning effort for models such as DeepSeek that reject unsupported
  reasoning parameters.
- Legacy `sendWorkbenchChat` enum mapping remains only for compatibility:
  `flash -> Auto`, `thinking -> Ask`, `pro/ultra -> Agent`.

## Verification

- `cd frontend/apps/coze-studio && npx vitest run src/pages/workbench/__tests__/workbench.test.tsx`
  - Covers all four mode runtime contexts.
  - Covers default mode configs omitting `reasoning_effort`.
  - Covers explicit runtime reasoning adding `reasoning_effort`.
  - Covers clicking `Ultra` in the composer and sending `subagent_enabled=true`.
- `cd frontend/apps/coze-studio && npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx`
  - Covers task detail follow-up metadata/config and retry config using `pro`.
- `cd frontend/apps/coze-studio && npx eslint src/pages/workbench/components/types.ts src/pages/workbench/components/workbench-composer-controls.tsx src/pages/workbench/index.tsx src/pages/tasks/task-detail-hooks.ts src/pages/tasks/task-run-actions-hook.ts src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx --quiet`
  - Lint passed for touched frontend files.
- `cd frontend/apps/coze-studio && npx vitest run src/pages/workbench/__tests__/workbench.test.tsx -t 'mode|Ultra|reasoning|default mode'`
  - Result: passed, 8 tests selected.

## Runtime Smoke

- Post-fix task run `7656986537476751360` persisted:
  - `mode=pro`
  - `thinking_enabled=true`
  - `is_plan_mode=true`
  - `subagent_enabled=false`
  - `model_name=deepseek-v4-pro`
  - no `reasoning_effort` key
- The same run succeeded through the real Eino ADK path, confirming that the
  DeerFlow mode fields no longer force unsupported provider reasoning
  parameters for DeepSeek.

## Browser note

In-app browser verification on 2026-06-30 reloaded
`http://localhost:8080/space/7656275718757679104/chats/new` and confirmed:

- composer presentation is `deerflow`;
- the visible mode is `Pro`;
- no visible `reasoning_effort` text or old `Auto/Ask/Agent` segmented control
  is present on the homepage composer.

Visual screenshot capture for the mode menu is still manual/next-run evidence.
