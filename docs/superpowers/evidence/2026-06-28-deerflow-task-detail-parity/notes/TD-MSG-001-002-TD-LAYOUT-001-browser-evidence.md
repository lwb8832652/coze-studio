# TD-MSG-001 / TD-MSG-002 / TD-LAYOUT-001 Browser Evidence

Date: 2026-06-28

Targets:

- Coze: `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`
- DeerFlow: `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`

Screenshots:

- Coze viewport:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/TD-MSG-001-002-coze-viewport.png`
- DeerFlow viewport:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/TD-MSG-001-002-deerflow-viewport.png`

Coze DOM verification:

```json
{
  "hasUserBubble": true,
  "userText": "你可以绘制时序图或架构图吗？请直接给出一个 Mermaid sequenceDiagram 和一个 flowchart 架构图，并说明如何继续修改。",
  "hasAssistantLine": true,
  "assistantLineText": "Aime · Agent 已为你启动工作流",
  "hasExecutionPanel": true,
  "executionText": "执行流程2/2 已完成 · 100% ... 任务开始执行 ... 任务执行完成",
  "resultType": "answer",
  "resultTextPrefix": "普通回答 · Agent Mermaid 时序图 (Sequence Diagram)",
  "mermaidReadyCount": 6,
  "rawMermaidFenceVisible": false,
  "composerDockedBelowScroll": true,
  "tokenButton": "Tokens8,032",
  "exportVisible": true,
  "detailVisible": true
}
```

DeerFlow DOM verification:

```json
{
  "hasPromptText": true,
  "hasThinkingButton": true,
  "hasTokenButton": true,
  "hasExportButton": true,
  "hasProMode": true,
  "tokenButtonText": "Tokens21.4K",
  "rawMermaidFenceVisible": false,
  "buttonTexts": [
    "Tokens21.4K",
    "导出",
    "思考",
    "Pro",
    "DeepSeek V4 Pro (Thinking)"
  ]
}
```

API evidence:

- Direct unauthenticated `curl` calls to Coze Workbench task-thread endpoints
  return `401 missing session_key in cookie`.
- Browser automation cannot read the session cookie because it is not exposed
  through `document.cookie`, so authenticated API shape capture remains a
  separate validation step.

Findings:

- `TD-MSG-001`: Coze displays the user prompt in a dedicated user bubble and
  does not execute Markdown from the user message. This is visually close to
  DeerFlow, pending exact spacing/avatar comparison.
- `TD-MSG-002`: Coze renders assistant Markdown and Mermaid diagrams, but the
  assistant response still carries the Coze-specific `普通回答 · Agent` eyebrow.
  DeerFlow renders the assistant turn as plain chat content without this result
  label. Treat as P0 visual parity gap.
- `TD-LAYOUT-001`: Coze now keeps the composer outside the scrollable transcript
  and fixed near the bottom. DeerFlow composer exposes mode and model controls
  inline (`Pro`, model picker). Coze exposes Auto/Ask/Agent plus `运行设置`, so
  the control semantics need a separate `TD-COMP-002/003` comparison.
- Execution progress differs: Coze shows a full `执行流程` card in the transcript,
  while DeerFlow exposes `思考` as an inline/collapsible turn-level control.
  This should be fixed under `TD-MSG-005` / `TD-FLOW-001` rather than by adding
  another diagnostics panel.
