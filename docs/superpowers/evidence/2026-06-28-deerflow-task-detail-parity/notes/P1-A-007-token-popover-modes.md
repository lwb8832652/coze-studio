# P1-A-007 Token Popover Modes

## Scope

This evidence verifies the task-detail top-bar token usage popover: summary
rows, DeerFlow display modes, explanatory note, and safe metadata boundaries.

## DeerFlow Reference

Existing reference evidence:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/TD-TOKEN-002-token-usage-modes.md`

Verified DeerFlow source anchors:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/token-usage-indicator.tsx`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-token-usage.tsx`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/i18n/locales/zh-CN.ts`

DeerFlow shape: `Token 用量`, `输入`, `输出`, `总计`, and display modes
`关闭` / `总览` / `每轮` / `调试`, with an explanatory note about persisted
thread usage and visible per-turn/debug usage.

## Coze Runtime Evidence

- Task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-007-coze-token-popover.png`
- Safe DOM summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-007-token-popover-summary.json`

Safe browser result:

- Token popover opens from the top-bar token pill.
- Popover includes `输入`, `输出`, and `总计`.
- Popover includes display modes `关闭`, `总览`, `每轮`, and `调试`.
- Popover includes the explanatory note.
- Unsafe visible-pattern hits: none.

The summary stores only booleans and unsafe hit names. It does not retain raw
prompt text, model completions, provider payloads, tool arguments/results, raw
usage payloads, credentials, or checkpoint bytes.

## Source Contract

- `frontend/apps/coze-studio/src/pages/tasks/task-token-usage-indicator.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-message-token-usage.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-token-usage.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`

## Automated Verification

Command:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token usage"
```

Result: passed, 2 tests.

Covered behavior:

- The top-bar token popover renders the DeerFlow-style summary rows and display
  mode controls.
- The selected display mode is persisted through
  `coze.task-detail.token-usage-view-mode`.
- Per-turn token summaries are gated by the selected mode and do not expose raw
  provider metadata.

## Remaining Gap

No open P1-A-007 gap remains. Pricing/cost and provider/model breakdowns remain
under P1-G.
