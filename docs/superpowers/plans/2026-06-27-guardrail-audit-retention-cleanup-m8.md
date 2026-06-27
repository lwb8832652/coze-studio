# Guardrail Audit Retention Cleanup M8

## Objective

Add a backend-owned retention cleanup boundary for Guardrail audit events so
operators can prune old metadata-only audit rows without changing task-detail
query or export semantics.

## Scope

- Add bounded repository deletion for `agent_guardrail_audit_events` by
  `created_at` cutoff.
- Add `GuardrailAuditRetentionReaper` to compute cutoff from configured
  retention and delete one ordered batch per run.
- Add `GuardrailAuditRetentionWorker`, disabled by default and controlled by:
  - `AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED`
  - `AGENT_GUARDRAIL_AUDIT_RETENTION_DAYS`
  - `AGENT_GUARDRAIL_AUDIT_RETENTION_BATCH_SIZE`
  - `AGENT_GUARDRAIL_AUDIT_RETENTION_INTERVAL_MS`
- Wire the worker into application startup without changing default runtime
  behavior.
- Document default-off environment examples.

## Safety Boundary

- Cleanup hard-deletes only audit rows older than the cutoff.
- Cleanup runs in bounded batches and does not loop until the table is empty.
- Worker errors and repository errors stay sanitized.
- Logs may include cutoff timestamps and deletion counts only.

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
- raw repository error details

## Follow-Up Work

- Scheduled archive/export jobs
- Legal-hold policy
- Metrics
- OpenTelemetry linkage
- Policy UI
- Operator runbooks

## Verification

```bash
cd backend
go test -count=1 ./domain/agentthread/repository -run GuardrailAudit
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditRetention|TestApplication.*GuardrailAudit'
go test -count=1 ./application
cd ..
git diff --check
```
