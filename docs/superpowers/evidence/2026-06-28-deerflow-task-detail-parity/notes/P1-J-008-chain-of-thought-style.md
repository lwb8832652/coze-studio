# P1-J-008 ChainOfThought Style Evidence

Date: 2026-07-01

## Scope

This note covers the task-detail execution-step / thinking-chain visual
alignment slice. It does not close the full P1-J-008 acceptance suite.

## DeerFlow Reference

Verified source anchors:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/ai-elements/chain-of-thought.tsx`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-group.tsx`

Relevant DeerFlow structure:

- `ChainOfThought`: `w-full gap-2 rounded-lg border p-0.5`
- `ChainOfThoughtContent`: `mt-2 space-y-3`, often `px-4 pb-2`
- `ChainOfThoughtStep`: `flex gap-2 text-sm`, left icon plus vertical rail
- Message group collapse affordance uses `查看其他 n 个步骤` / `隐藏步骤`
  semantics and renders the latest actionable step when collapsed.

## Coze Change

Updated task-detail execution feed to expose DeerFlow-like structural classes:

- `coze-prototype-chain-of-thought`
- `coze-prototype-chain-content`

Updated spacing and visual token mapping for:

- Chain container border/padding/overflow.
- Collapse button text size and padding.
- Step row font size, gap, content gap, and icon rail.
- Path pill spacing and line height.

## Browser Evidence

Page:

`http://localhost:8080/space/7656275718757679104/tasks/7657390468782620672`

Read-only DOM check after local code update:

```json
{
  "className": "coze-prototype-execution-feed coze-prototype-reasoning-panel coze-prototype-chain-of-thought",
  "contentClassName": "coze-prototype-execution-feed-list coze-prototype-chain-content",
  "moreText": "查看其他 4 个步骤",
  "stepCount": 2,
  "borderRadius": "10px",
  "borderColor": "rgb(232, 225, 211)",
  "padding": "2px",
  "fontSize": "14px",
  "lineHeight": "22px"
}
```

After clicking `查看其他 4 个步骤`:

```json
{
  "chevronOpen": "true",
  "moreText": "隐藏步骤",
  "stepCount": 6
}
```

Observed expanded step texts included skill catalog, reasoning, file creation,
and file presentation steps. No raw tool argument/result payloads were exposed.

## Automated Verification

Commands:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders DeerFlow-style reasoning steps"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Result:

- Targeted test passed after first watching it fail on missing
  `coze-prototype-chain-of-thought`.
- Full task-detail test suite passed: 49 tests.
- TypeScript check passed.

Known test noise:

- Existing test emitted a `localhost:3000` `ECONNREFUSED` logger message in
  `keeps the follow-up composer docked outside the scrollable chat transcript`;
  the suite still passed and the noise was unrelated to this style slice.

## Remaining Gap

This slice confirms Coze's current execution feed structure and spacing are
closer to DeerFlow. Full P1-J-008 still needs paired DeerFlow/Coze visual
screenshots across the canonical prompts before the tracker can move to
`已完成`.
