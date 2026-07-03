# P2-A TD-MD-003 Mermaid Error Fallback

## DeerFlow Reference

- Source path:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/markdown-content.tsx`
- Markdown rendering flows through `MarkdownContent` ->
  `MessageResponse` / `ClipboardSafeStreamdown`.
- `ClipboardSafeStreamdown` is protected by
  `StreamdownFallbackBoundary` in
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/ai-elements/streamdown.tsx`.
  If message rendering throws, DeerFlow falls back to readable plain text rather
  than replacing the whole route with an error page.
- DeerFlow code blocks expose copy affordances through
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/ai-elements/code-block.tsx`.

## Coze Difference

- Coze already split Mermaid fenced blocks out of markdown and rendered them
  through `mermaid.render`.
- Invalid Mermaid diagrams entered `data-status="error"` and preserved source,
  but the error fallback had no source copy action. That made the fallback less
  useful than DeerFlow's code/text fallback path.

## Implementation

- File:
  `frontend/apps/coze-studio/src/pages/tasks/task-markdown-content.tsx`
- Error-state Mermaid blocks now keep the `复制 Mermaid 源码` action.
- SVG download remains available only for `ready` diagrams; failed diagrams do
  not generate or expose a stale/invalid SVG download action.
- No backend/API contract changed. Mermaid rendering and fallback are pure
  front-end presentation behavior.

## Verification

Red/green targeted test:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "invalid Mermaid"
```

Red result before implementation:

- Failed at `expect(copyButton).toBeTruthy()`.

Green result after implementation:

- `1 passed | 56 skipped`.
