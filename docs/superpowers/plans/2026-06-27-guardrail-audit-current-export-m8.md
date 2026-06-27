# Guardrail Audit Current Export M8

## Objective

Let operators export the currently visible Guardrail audit rows from canonical
task-thread detail pages without exposing raw execution content or implying a
full-history backend export contract.

## Scope

- Add `导出审计` to the task-detail `安全审计` panel.
- Export only rows already loaded in the panel.
- Use JSON schema `coze.task_thread_guardrail_audit.export.v1`.
- Rebuild each row from an explicit metadata-safe allow-list.
- Disable export when the current panel has no rows.
- Keep full-history export, pagination export, retention, and operator
  reporting as later backend-owned work.

## Safe Export Fields

- event_id, event_type, created_at
- thread_id, run_id, space_id, actor_id
- target_type, target_id, operation, source
- action, fail_mode, provider, reason_code
- rule_ids parsed into a string array

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
- future API fields not explicitly reviewed for this export allow-list

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- task-detail.test.tsx -t 'renders canonical thread guardrail audit records'
npm run test -- tasks-service.test.ts task-detail.test.tsx
cd ../../..
git diff --check
```
