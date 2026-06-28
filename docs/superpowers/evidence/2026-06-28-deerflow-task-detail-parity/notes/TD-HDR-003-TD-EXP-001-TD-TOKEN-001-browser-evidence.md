# TD-HDR-003 / TD-EXP-001 / TD-TOKEN-001 Browser Evidence

Date: 2026-06-28

Target:

- Coze: `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`

Browser DOM verification after reload:

```json
{
  "actionText": "Tokens8,032导出详情☆ 收藏分享wb",
  "topbarRect": { "height": 56, "width": 951 },
  "actionsRect": { "height": 30, "width": 412 },
  "hasTaskBreadcrumb": false,
  "hasArtifactInTopbar": false,
  "hasExportButton": true,
  "hasInspectorButton": true,
  "tokenText": "Tokens8,032",
  "tokenTitle": "Input 5,556 · Output 2,476 · Total 8,032"
}
```

Token click verification:

```json
{
  "hasTokenButton": true,
  "hasTokenPopover": true,
  "tokenButtonAriaExpanded": "true",
  "tokenPopoverText": "Token 用量Input5,556Output2,476Total8,032"
}
```

Mermaid block verification:

```json
[
  {
    "status": "ready",
    "buttons": ["下载 Mermaid SVG", "复制 Mermaid 源码"]
  },
  {
    "status": "ready",
    "buttons": ["下载 Mermaid SVG", "复制 Mermaid 源码"]
  }
]
```

Unit and type verification:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Result:

- Header no longer renders `任务详情 › 已完成`.
- Header no longer renders top-level `产物 0`.
- Header primary actions render compact `Tokens`, `导出`, and `详情`.
- Header `Tokens` is clickable and opens Input/Output/Total usage details.
- Task export downloads a visible Markdown transcript and filters internal
  markers, reasoning tags, and tool messages in unit coverage.
- Mermaid diagrams render as ready SVGs and expose SVG download plus source
  copy controls.
