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

## Paired Screenshot Evidence

2026-07-01 paired visual smoke:

- DeerFlow expanded-step reference:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-008-deerflow-expanded-steps.jpg`
- Coze expanded-step reference:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-008-coze-expanded-steps.jpg`

DeerFlow DOM summary:

```json
{
  "url": "http://localhost:2026/workspace/chats/fb3b753d-7c88-44e9-949e-5e0a7f860286",
  "hasMoreStepsBeforeExpand": true,
  "hasHideStepsAfterExpand": true,
  "visibleStepLabels": [
    "Create P1-J-006 artifact verification document",
    "Copy to outputs directory"
  ],
  "hasArtifactCard": true,
  "hasDownload": true
}
```

Coze DOM summary:

```json
{
  "url": "http://localhost:8080/space/7656275718757679104/tasks/7657390468782620672",
  "hasMoreStepsBeforeExpand": true,
  "hasHideStepsAfterExpand": true,
  "visibleStepLabels": [
    "可用技能目录 22 个",
    "创建mermaid-diagrams Markdown 文档",
    "展示文件"
  ],
  "hasArtifactCard": true,
  "hasDownload": true,
  "hasRightPreview": true
}
```

Runtime limitation:

- The earliest DeerFlow Mermaid reference task
  `c155a732-f475-4cf9-aa49-13fd26b29888` redirected to
  `/workspace/chats/new` during this pass, so it cannot be used as same-prompt
  visual evidence.
- The paired screenshots above prove expanded-step structure, spacing,
  collapse affordance, file path pills, and artifact adjacency, but they are not
  a same-prompt output-quality comparison.

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
closer to DeerFlow and now has paired expanded-step screenshots. Full P1-J-008
still needs same-prompt DeerFlow/Coze visual evidence across the canonical
quality prompts before the tracker can move to `已完成`.
