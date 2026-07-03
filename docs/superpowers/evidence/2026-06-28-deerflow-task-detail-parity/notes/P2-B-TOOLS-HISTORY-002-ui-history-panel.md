# P2-B-TOOLS-HISTORY-002 MCP runtime history panel

## Scope

Surface the metadata-only MCP/runtime audit API in the task detail inspector.
This is tied to existing task-detail cases:

- `TD-LAYOUT-003`: diagnostics/audit/memory/tool history stay out of the chat
  transcript and live in the `详情` inspector.
- `TD-FLOW-002`: tool-related records are readable but do not expose sensitive
  payloads.
- `TD-COMP-005`: MCP runtime calls remain automatically injected by the enabled
  MCP catalog; this panel is observability, not a manual selection workflow.

## Coze implementation

- Frontend schema adds `TaskThreadMCPRuntimeAuditEvent`,
  `ListTaskThreadMCPRuntimeAuditEventsRequest`, and
  `ListTaskThreadMCPRuntimeAuditEventsResponse`.
- `frontend/apps/coze-studio/src/pages/tasks/service.ts` exports
  `listTaskThreadMCPRuntimeAuditEvents` through the existing `workbenchTask`
  generated-client path.
- `TaskDetailInspector` mounts `TaskMCPRuntimeAuditSection` after safety audit
  and before task memory.
- The panel title is `工具调用`, with refresh, loading, empty, and error states
  aligned to the existing inspector sections.

## Safety boundary

The UI renders only bounded metadata returned by the API:

- event type/status
- runtime tool name
- server ID
- run ID
- elapsed milliseconds
- output byte count
- error code
- created time

It does not render tool arguments, tool results, prompts, object URIs,
credentials, provider raw payloads, checkpoint bytes, or filesystem paths.

## Verification

RED:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders MCP runtime audit records"
```

The new test failed because `listTaskThreadMCPRuntimeAuditEvents` was never
called from the inspector.

GREEN:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders MCP runtime audit records"
```

The targeted test passed after adding the schema export, service export,
inspector section, and panel styles.

FULL:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

All 58 task-detail tests passed. `tsc --noEmit` passed. The test run still
prints the existing localhost:3000 `ECONNREFUSED` logger noise in one composer
layout test, but Vitest exits successfully.

Workspace check:

```bash
git diff --check
```

No whitespace errors.
