# Guardrail Audit Archive Export Boundary M8

## Objective

Add a non-destructive archive/export boundary for expired Guardrail audit rows
so retention cleanup can later archive before deleting.

## Scope

- Add repository paging by `created_at` cutoff.
- Add `GuardrailAuditArchiveExporter`.
- Add `GuardrailAuditArchiveWriter` as an injectable persistence boundary.
- Build schema `coze.guardrail_audit.archive.v1` payloads from one bounded
  expired batch.
- Keep this slice free of scheduling, deletion, legal-hold bypass, and
  object-storage implementation details.

## Safe Archive Fields

- schema, exported timestamp, cutoff timestamp, total
- event, thread, run, space, and actor IDs
- event type, target type, safe target ID
- operation, source, action, fail mode
- provider, reason code, sanitized rule IDs
- created timestamp

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
- raw SQL
- raw repository/provider errors
- client-controlled identity
- future API fields not explicitly reviewed for the archive allow-list

## Follow-Up Work

- Prometheus exporter
- OpenTelemetry metrics and trace linkage
- Policy UI

## Verification

```bash
cd backend
go test -count=1 ./domain/agentthread/repository -run 'TestGuardrailAuditRepositoryListsEventsBeforeCutoff'
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditArchiveExporter'
go test -count=1 ./domain/agentthread/repository -run GuardrailAudit
go test -count=1 ./application/agentthread -run 'TestGuardrailAudit'
go test -count=1 ./application/agentthread
cd ..
git diff --check
```
