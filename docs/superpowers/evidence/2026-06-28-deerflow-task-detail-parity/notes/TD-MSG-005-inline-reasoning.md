# TD-MSG-005 Inline Reasoning Evidence

## Scope

- Case: `TD-MSG-005`
- Goal: keep DeerFlow-style inline `思考` content in the task detail chat flow.
- Current slice: frontend rendering and safety behavior.

## Implementation Evidence

- `task-reasoning.ts` extracts reasoning text from:
  - `reasoning_content`
  - `reasoning`
  - `thinking`
  - `thought`
  - `reasoning_parts[].text`
  - inline `<think>...</think>` markers
- `task-inline-reasoning.tsx` renders a lightweight collapsible `思考` block before the assistant answer.
- `detail.tsx` prefers latest ADK run-event reasoning over fallback task-result reasoning.
- `task-event-display.ts` summarizes `message.completed` as safe execution metadata, preventing raw event JSON and provider signatures from appearing in the execution flow.

## Safety Checks

- Provider reasoning signatures are not rendered.
- Raw `message.completed` payload is not rendered in `执行流程`.
- Assistant answer body strips raw `<think>` tags.
- Markdown export remains filtered through existing internal marker stripping.

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders ADK reasoning content"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
TSESTREE_SINGLE_RUN=true npx eslint --fix --cache src/pages/tasks/detail.tsx src/pages/tasks/helpers.ts src/pages/tasks/task-event-display.ts src/pages/tasks/task-inline-reasoning.tsx src/pages/tasks/task-reasoning.ts src/pages/tasks/__tests__/task-detail.test.tsx
```

## Remaining Evidence

- Need a real local DeerFlow/Coze browser sample where the selected model emits reasoning content.
- Capture expanded/collapsed `思考` screenshot before marking `TD-MSG-005` as fully complete.
