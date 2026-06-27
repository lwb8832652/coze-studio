# Guardrail Audit Backend Export Frontend M8

## Objective

Switch the task-detail `安全审计` export action from frontend current-list
serialization to the backend Guardrail audit export endpoint.

## Scope

- Reuse the generated `ExportTaskThreadGuardrailAuditEvents` client from the
  task page service.
- Keep the visible audit list on the existing latest-20-row query.
- Export with `page_size=1000`.
- Continue requesting export pages until accumulated event count reaches
  backend `total` or the backend returns an empty page.
- Show export progress through the Semi/Coze Design `Button.loading` prop.
- Disable export when the audit total is zero.
- Preserve the existing safe JSON export shape, including parsed `rule_ids`
  string arrays and normalized `page` / `page_size` metadata.

## Excluded Fields

- decision messages
- prompts, model input/output
- tool arguments/results
- object URIs, raw URLs, filenames
- credentials
- checkpoint bytes
- scanner raw payloads and provider raw bodies
- hidden run config
- raw metadata JSON
- client-controlled identity
- future API fields not explicitly reviewed for this export allow-list

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- task-detail.test.tsx -t 'renders canonical thread guardrail audit records'
npm run test -- tasks-service.test.ts task-detail.test.tsx
cd ../../..
git diff --check
```
