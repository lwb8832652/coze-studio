# Guardrail Audit Frontend M8

## Objective

Expose Guardrail audit records on canonical task-thread detail pages without
expanding the public contract beyond metadata-safe audit fields.

## Scope

- Mount a `安全审计` panel only on task-thread detail pages.
- Reuse the generated `ListTaskThreadGuardrailAuditEvents` client exported by
  `frontend/apps/coze-studio/src/pages/tasks/service.ts`.
- Load the latest 20 audit events and provide manual refresh.
- Render loading, empty, and bounded error states.
- Display only event/action/target/operation/source/fail-mode/provider/reason,
  sanitized rule IDs, run ID, and timestamp.
- Do not render decision messages, prompts, model text, tool
  arguments/results, object URIs, raw URLs, filenames, credentials,
  checkpoint bytes, scanner raw payloads, provider raw bodies, hidden run
  config, or raw metadata JSON.

## Implemented

- `TaskGuardrailAuditSection`
- Task detail page mounting after subagent execution and before task memory.
- Task-detail integration test for metadata-only rendering.
- Task-service generated-client contract assertions.
- Task detail panel styling in `workspace-prototype.less`.

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- task-detail.test.tsx -t 'renders canonical thread guardrail audit records'
npm run test -- tasks-service.test.ts task-detail.test.tsx
cd ../../..
git diff --check
```
