# Guardrail Audit Retention Legal Hold M8

## Objective

Add a safe legal-hold switch for Guardrail audit retention cleanup so operators
can pause destructive deletion without disabling the worker or losing cleanup
observability.

## Scope

- Add `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED=false`.
- Skip retention cleanup before calling the reaper or repository when legal
  hold is enabled.
- Emit a content-free skipped metrics observation with
  `skip_reason='legal_hold'`.
- Keep the worker and metrics collector active so operators can verify the
  hold.
- Update environment examples, roadmap context, and the Guardrail audit
  operations runbook.

## Safe Observation Fields

- success flag
- skipped flag
- skip reason
- configured retention days
- batch size
- elapsed milliseconds

## Excluded Fields

- space, thread, run, user, target, and audit row IDs
- reason codes and rule IDs
- decision messages
- prompts, model input/output
- tool arguments/results
- object URIs, raw URLs, filenames
- credentials
- checkpoint bytes
- scanner raw payloads and provider raw bodies
- hidden run config
- raw metadata JSON
- raw SQL
- raw repository errors
- archive object locations

## Follow-Up Work

- Scheduled archive/export jobs that honor legal hold
- Prometheus exporter
- OpenTelemetry metrics and trace linkage
- Policy UI

## Verification

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditRetentionWorker.*LegalHold|TestGuardrailAuditRetentionWorkerRunOnceSkips'
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditRetentionWorker|TestGuardrailAuditRetentionMetricsCollector'
go test -count=1 ./application/agentthread
cd ..
git diff --check
```
