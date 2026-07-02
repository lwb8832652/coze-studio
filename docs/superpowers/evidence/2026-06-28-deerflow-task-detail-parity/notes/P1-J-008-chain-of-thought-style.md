# P1-J-008 ChainOfThought Style Evidence

Date: 2026-07-01

## Scope

This note covers the task-detail execution-step / thinking-chain visual
alignment slice, plus the same-prompt quality smoke evidence needed to close
P1-J-008 after the user deferred detailed Skills/MCP quality checks to P2.

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

## Same-Prompt Document Artifact Smoke

The DeerFlow expanded-step screenshot above uses this prompt:

```text
请生成一份《P1-J-006 产物验证》Markdown 简短文档，包含标题、要点列表和一个小表格。请保存为 Markdown 文件，并把文件作为右侧 Artifacts 产物展示，不要只在聊天里输出正文。
```

The same prompt was submitted through Coze:

- Coze task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657502884837195776`
- Coze screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-008-coze-same-prompt-document.jpg`

Coze DOM summary:

```json
{
  "hasMoreStepsBeforeExpand": true,
  "hasHideStepsAfterExpand": true,
  "hasMarkdownCard": true,
  "artifactName": "P1-J-006-产物验证.md",
  "hasDownload": true,
  "hasRightPreview": true,
  "tokenLine": "Tokens输入: 39.1K输出: 1,178总计: 40.3K",
  "unsafeHits": []
}
```

Expanded Coze step text included the safe skill catalog projection, reasoning,
file creation, and `present_files` display step. This matches the DeerFlow
same-prompt shape for this document-artifact smoke: reasoning/tool steps,
output path pill, artifact card, download, token row, and final summary are all
visible without raw tool arguments or object URIs.

## Same-Prompt Search / Answer Smoke

Prompt:

```text
请联网搜索青岛最佳旅游时间，并用 3 点回答，注明信息来源类型。
```

DeerFlow:

- Task:
  `http://localhost:2026/workspace/chats/72b6d7d7-ac4e-4899-80e3-86ece96e3ce7`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-008-deerflow-search-answer.jpg`
- Visible behavior: multiple web search / page view steps, expanded
  `隐藏步骤`, final 3-point answer, source-type notes, token row, and generated
  follow-up suggestions.

Coze:

- Task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-008-coze-search-answer.jpg`
- Visible behavior: automatic search steps (`搜索网页`), expanded `隐藏步骤`,
  final 3-point answer, source-type notes, token row, and safe step metadata.

Coze DOM summary:

```json
{
  "hasMoreStepsBeforeExpand": true,
  "hasHideStepsAfterExpand": true,
  "hasAnswer": true,
  "hasSearchStep": true,
  "hasTokens": true,
  "unsafeHits": []
}
```

DeerFlow and Coze differ in exact source selection and wording, but both satisfy
the parity acceptance for this search/answer smoke: intent-triggered web
search, visible search steps, final answer with source-type explanation, and no
unsafe raw tool payload on the page.

## Same-Prompt Multi-Turn Revision Smoke

Follow-up prompt:

```text
请把上面的回答压缩成更短版本，每点不超过 30 个字，并保留来源类型。
```

DeerFlow:

- Task:
  `http://localhost:2026/workspace/chats/72b6d7d7-ac4e-4899-80e3-86ece96e3ce7`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-008-deerflow-revision-followup.jpg`
- Visible behavior: the follow-up preserved the previous web-search answer
  context and returned three shortened lines with source types, a token row,
  and generated follow-up suggestions.

Coze:

- Task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-008-coze-revision-followup.jpg`
- Visible behavior: the canonical follow-up run preserved the previous Qingdao
  search answer context and returned three shortened lines with source types,
  a token row, and safe step metadata.

Coze DOM summary:

```json
{
  "hasOriginalSearchAnswer": true,
  "hasRevisionPrompt": true,
  "hasStopButton": false,
  "hasShortAnswer": true,
  "hasTokens": true,
  "hasMoreSteps": true,
  "unsafeHits": []
}
```

DeerFlow and Coze differ in exact wording and selected source labels, which is
expected for live model/search output. The parity point for this smoke is the
same: follow-up context is preserved, the new answer is grounded in the
previous turn, token usage remains visible, and internal tool payloads stay
hidden.

## Automated Verification

Commands:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders DeerFlow-style reasoning steps"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders DeerFlow-style reasoning steps|sends canonical thread follow-up messages through the message API|keeps execution steps for previous assistant turns after a follow-up"
git diff --check
file docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-008-deerflow-revision-followup.jpg docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-008-coze-revision-followup.jpg
```

Result:

- Targeted test passed after first watching it fail on missing
  `coze-prototype-chain-of-thought`.
- Full task-detail test suite passed: 49 tests.
- TypeScript check passed.
- 2026-07-01 multi-turn closure verification passed: 3 targeted tests,
  `git diff --check`, and JPEG validation for both revision screenshots.

Known test noise:

- Existing test emitted a `localhost:3000` `ECONNREFUSED` logger message in
  `keeps the follow-up composer docked outside the scrollable chat transcript`;
  the suite still passed and the noise was unrelated to this style slice.

## Closure

This slice confirms Coze's current execution feed structure and spacing are
closer to DeerFlow, now has paired expanded-step screenshots, and includes one
same-prompt document-artifact smoke, one same-prompt search/answer smoke, and
one same-prompt multi-turn revision smoke. Detailed Skills/MCP quality
acceptance was explicitly deferred by the user, so P1-J-008 can close without
reopening the Skills/MCP parity line in this slice.
