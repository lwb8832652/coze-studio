# DeerFlow Task Detail Parity Evidence

This directory stores browser, API, and code-comparison evidence for
`docs/superpowers/plans/2026-06-28-deerflow-task-detail-parity-validation.md`.

## Directory Layout

```text
screenshots/
  deerflow/
  coze/
network/
  deerflow/
  coze/
notes/
```

## Naming

Use the validation case id as the prefix:

- `screenshots/deerflow/TD-MD-001-sequence.png`
- `screenshots/coze/TD-MD-001-sequence.png`
- `network/deerflow/TD-RUN-002-runs-stream.json`
- `network/coze/TD-RUN-002-run-events-stream.json`
- `screenshots/coze/TD-DOC-003-markdown-preview.png`
- `network/coze/TD-DOC-003-artifact-content-preview.json`
- `notes/TD-LAYOUT-001-findings.md`

## Redaction

Never store credentials, tokens, API keys, prompt/completion full text, tool
arguments, tool results, object URIs, provider raw bodies, cookies, or local
absolute upload filenames in evidence files.

For API samples, keep only:

- method, path, status, duration when useful;
- safe query/body summary;
- response top-level fields, ids, statuses, usage totals, event types;
- explicit note that unsafe fields were redacted.

## Browser Baseline Targets

- DeerFlow:
  `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`
- Coze:
  `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`

Both local projects use:

- account: `840582614@qq.com`
- password: `z8832652`

## Document Generation Prompt

Use this prompt for `TD-DOC-*` validation unless a case says otherwise:

```text
请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项，并生成一个可在产物面板预览和下载的 Markdown 或 PDF 文档。
```
