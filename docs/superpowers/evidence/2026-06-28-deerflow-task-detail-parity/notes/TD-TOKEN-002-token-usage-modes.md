# TD-TOKEN-002 Token Usage Modes Evidence

Date: 2026-06-28

DeerFlow reference:

- User-provided screenshot:
  `/var/folders/g2/6t16q1y15bg_bf0zqhbnhj780000gp/T/codex-clipboard-ef89b6b4-0bdb-4a51-a5f3-ecc62fde5c62.png`
- Token indicator source:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/token-usage-indicator.tsx`
- Per-message usage source:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-token-usage.tsx`
- Chinese labels source:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/i18n/locales/zh-CN.ts`

Observed DeerFlow shape:

- Header button renders `Tokens <total>` with a compact pill.
- Click popover title is `Token 用量`.
- Summary rows are `输入`, `输出`, and `总计`.
- Display modes are `关闭`, `总览`, `每轮`, and `调试`, with `每轮`
  selected by default in the captured screenshot.
- The explanatory note clarifies that top totals prefer persisted thread
  usage, while per-turn/debug usage is derived from visible messages.

Coze implementation:

- `TaskTokenUsageIndicator` now uses the same Chinese summary rows and display
  mode labels/descriptions.
- The selected display mode is persisted in
  `coze.task-detail.token-usage-view-mode`.
- Default mode is `per_turn`, matching DeerFlow's visible per-turn behavior.
- `TaskMessageTokenUsage` displays assistant turn summaries as
  `Tokens / 输入 / 输出 / 总计`.
- `task-detail-loader.ts` groups `/token_usage` rows by `run_id`, and
  task detail maps the latest assistant message `run_id` to the per-turn
  summary.
- Provider names, raw usage payloads, step names, prompts, tool arguments, and
  hidden metadata are not rendered in the top popover or per-turn summary.

Verification:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token usage"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Result:

- Token-focused tests passed: `2 passed`.
- Full task detail tests passed: `36 passed`.
- TypeScript check passed.
- `TSESTREE_SINGLE_RUN=true npx eslint --fix --cache ...` exited with no
  errors and four existing complexity/no-magic-number warnings in touched task
  files.
- Real Coze browser screenshot remains a manual验收 item because the user will
  perform visual confirmation after local rebuild/reload.
