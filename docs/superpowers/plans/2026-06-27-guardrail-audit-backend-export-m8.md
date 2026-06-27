# Guardrail Audit Backend Export M8

## Objective

Add a backend-owned, authorized, paginated export boundary for Guardrail audit
events so operators are not limited to the currently loaded task-detail rows.

## Scope

- Add application DTOs and `ExportGuardrailAuditEvents`.
- Use a distinct `GuardrailAuditAccessOperationExport` authorization operation.
- Reuse `GuardrailAuditRepository.ListGuardrailAuditEvents` with optional
  `run_id`, normalized `page`, and capped `page_size`.
- Return schema `coze.task_thread_guardrail_audit.export.v1`.
- Expose `GET
  /api/workbench/task_threads/:thread_id/guardrail_audit_events/export`.
- Update IDL, backend API models, generated frontend schema, and task page
  service re-export.

## Safe Export Fields

- schema, thread_id, exported_at, page, page_size, total
- event_id, event_type, created_at
- thread_id, run_id, space_id, actor_id
- target_type, target_id, operation, source
- action, fail_mode, provider, reason_code, rule_ids

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

## Verification

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestApplicationExportGuardrailAuditEvents|TestApplicationListGuardrailAuditEvents'
go test -count=1 ./api/handler/coze -run 'TestExportTaskThreadGuardrailAuditEvents|TestListTaskThreadGuardrailAuditEvents'
go test -count=1 ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes
cd ../frontend/apps/coze-studio
npm run test -- tasks-service.test.ts
cd ../../packages/arch/api-schema
npm run test -- workbench-task-memory.test.ts
cd ../../../..
git diff --check
```
