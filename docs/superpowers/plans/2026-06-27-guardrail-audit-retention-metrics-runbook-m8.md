# Guardrail Audit Retention Metrics Runbook M8

## Objective

Add content-free retention cleanup observations and the first operator runbook
for Guardrail audit operations.

## Scope

- Add `GuardrailAuditRetentionMetricsCollector`.
- Emit one `GuardrailAuditRetentionMetricsObservation` per cleanup attempt.
- Keep metric logs disabled by default through
  `AGENT_GUARDRAIL_AUDIT_RETENTION_METRICS_LOG_ENABLED=false`.
- Wire the env-built metrics collector into the retention worker.
- Document safe enablement, verification, rollback, and incident triage in
  `docs/superpowers/runbooks/guardrail-audit-operations.md`.

## Safe Observation Fields

- success flag
- sanitized error code
- cutoff timestamp
- deleted row count
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

- Scheduled archive/export jobs
- Legal-hold policy
- Prometheus exporter
- OpenTelemetry metrics and trace linkage
- Policy UI

## Verification

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditRetentionWorker|TestGuardrailAuditRetentionMetricsCollector'
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditRetention'
go test -count=1 ./application/agentthread
cd ..
git diff --check
```
