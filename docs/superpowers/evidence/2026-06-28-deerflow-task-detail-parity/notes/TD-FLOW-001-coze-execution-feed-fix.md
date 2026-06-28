# TD-FLOW-001 Coze Execution Feed Fix Evidence

Date: 2026-06-28

Target:

- Coze: `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`

Screenshot:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/TD-FLOW-001-coze-execution-feed.png`

Browser DOM verification after reload:

```json
{
  "executionTitle": "执行流程",
  "hasExecutionFeed": true,
  "hasReasoningPanel": true,
  "rowCount": 2,
  "rows": [
    {
      "kind": "step",
      "status": "completed",
      "text": "任务开始执行 Agent Worker: agent-harness 2026/6/28 12:09:17"
    },
    {
      "kind": "step",
      "status": "completed",
      "text": "任务执行完成 Agent Worker: agent-harness 2026/6/28 12:09:33"
    }
  ],
  "hasResultEyebrow": false,
  "hasOrdinaryAnswerAgentText": false,
  "hasMermaid": 6
}
```

Unit and type verification:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
TSESTREE_SINGLE_RUN=true npx eslint --fix --cache src/pages/tasks/detail.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Result:

- `执行流程` remains a visible title.
- Each execution step remains visible with status, title, runtime, detail or
  thought text, and timestamp.
- The former heavy execution card is replaced by a lighter inline feed.
- Assistant answer turns no longer render the Coze-specific
  `普通回答 · Agent` / `普通回答 · Ark` result eyebrow.
