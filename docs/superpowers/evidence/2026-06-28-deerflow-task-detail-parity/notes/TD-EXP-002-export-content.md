# TD-EXP-002 Export Content Evidence

Date: 2026-06-28

DeerFlow user-provided export samples:

- Markdown: `/Users/liuwenbo/Downloads/Mermaid 绘制时序图与架构图 (1).md`
- JSON: `/Users/liuwenbo/Downloads/Mermaid 绘制时序图与架构图 (1).json`

Observed DeerFlow Markdown shape:

```markdown
# Mermaid 绘制时序图与架构图

*Exported on 2026/6/28 16:29:18 · Created Unknown*

---

## 🧑 User

...

---

## 🤖 Assistant

...
```

Observed DeerFlow JSON shape:

```json
{
  "title": "Mermaid 绘制时序图与架构图",
  "thread_id": "c155a732-f475-4cf9-aa49-13fd26b29888",
  "exported_at": "2026-06-28T08:29:35.630Z",
  "messages": [
    {
      "type": "human",
      "id": "46f02f19-a2d3-439b-b46e-f0b2e57e8b19__user",
      "content": "你可以绘制时许图或架构图吗"
    }
  ]
}
```

Coze implementation:

- Header `导出` now opens a DeerFlow-style format menu with `Markdown` and
  `JSON` items.
- Markdown export uses the DeerFlow heading, metadata, separator, and
  `## 🧑 User` / `## 🤖 Assistant` turn structure.
- JSON export uses top-level `title`, `thread_id`, `exported_at`, and
  `messages` with `type`, `id`, and `content`.
- Default export filters tool messages, `<think>...</think>`,
  `<uploaded_files>...</uploaded_files>`, and `<system-reminder>...</system-reminder>`.
- Export intentionally does not add Coze-only `schema`, `token_usage`, status,
  run config, tool payloads, provider raw payloads, or hidden runtime metadata.

Verification:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "exports task detail as safe visible"
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
TSESTREE_SINGLE_RUN=true npx eslint --fix --cache src/pages/tasks/task-export-action.tsx src/pages/tasks/task-detail-header.tsx src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Result:

- Export-focused tests passed: Markdown and JSON downloads are generated from
  the header menu and keep only safe visible transcript content.
- Combined task page tests passed: `49 passed`.
- ESLint and TypeScript checks passed.
- Real browser download sample remains a manual验收 item because the current
  running frontend may need rebuild/restart before the updated bundle is visible.
