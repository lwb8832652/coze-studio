# Guardrail Audit Operations Runbook

## Purpose

Operate Guardrail decision auditing, export, retention cleanup, and metrics
without exposing prompts, model output, tool payloads, credentials, object
locations, or raw scanner/provider responses.

## Enablement Order

1. Enable a Guardrail provider in a non-production environment.
2. Verify `安全审计` rows appear on task-thread detail pages.
3. Test backend export on a bounded thread before broad export use.
4. Test archive export on a bounded cutoff before retention cleanup.
5. Decide the compliance retention window.
6. Enable the archive worker in non-production with a conservative batch size.
7. Confirm legal hold is not required before destructive cleanup.
8. Enable retention cleanup with a conservative batch size.
9. Enable metrics logs only after the log pipeline is ready for per-decision or
   periodic cleanup observations.

## Environment Flags

- `AGENT_GUARDRAIL_PROVIDER_TYPE`: empty, `pattern`, or `http`.
- `AGENT_GUARDRAIL_PATTERN_RULES_JSON`: metadata-only pattern rules.
- `AGENT_GUARDRAIL_HTTP_URL`: trusted scanner endpoint.
- `AGENT_GUARDRAIL_HTTP_TOKEN`: optional scanner bearer token.
- `AGENT_GUARDRAIL_HTTP_TIMEOUT_MS`: scanner timeout.
- `AGENT_GUARDRAIL_METRICS_LOG_ENABLED`: content-free decision metrics logs.
- `AGENT_GUARDRAIL_AUDIT_ARCHIVE_WORKER_ENABLED`: archive worker switch.
- `AGENT_GUARDRAIL_AUDIT_ARCHIVE_RETENTION_DAYS`: archive cutoff window.
- `AGENT_GUARDRAIL_AUDIT_ARCHIVE_BATCH_SIZE`: per-run archive limit.
- `AGENT_GUARDRAIL_AUDIT_ARCHIVE_INTERVAL_MS`: archive interval.
- `AGENT_GUARDRAIL_AUDIT_ARCHIVE_OBJECT_PREFIX`: internal object-storage
  prefix for archive JSON payloads.
- `AGENT_GUARDRAIL_AUDIT_ARCHIVE_METRICS_LOG_ENABLED`: content-free archive
  metrics logs.
- `AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED`: register Guardrail decision,
  archive, and retention metrics with the Prometheus default registry.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED`: retention worker switch.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED`: require
  successful archive-before-delete for retention cleanup.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_DAYS`: retention window.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_BATCH_SIZE`: per-run delete limit.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_INTERVAL_MS`: cleanup interval.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED`: skip destructive
  retention cleanup while leaving the worker and metrics active.
- `AGENT_GUARDRAIL_AUDIT_RETENTION_METRICS_LOG_ENABLED`: content-free cleanup
  metrics logs.

## Safe Signals

- Audit event count by thread or run.
- Export page count and total count.
- Archive payload schema, cutoff timestamp, exported timestamp, event count,
  stable archive ID, and safe audit row fields.
- Decision action, target type, operation, source, fail mode, provider, and
  sanitized error code.
- Archive worker cutoff timestamp, archived row count, total, stable archive
  ID, retention days, batch size, elapsed milliseconds, skip reason, and
  sanitized archive error code.
- Retention cutoff timestamp, deleted row count, retention days, batch size,
  archived row count, stable archive ID, elapsed milliseconds, and sanitized
  cleanup error code.
- Prometheus aggregate counters and latency histograms with low-cardinality
  metadata labels only: action, provider, error code, success, skipped, skip
  reason, row kind, and bounded decision dimensions.
- Legal-hold skipped cleanup observations with `skip_reason=legal_hold`.

## Forbidden Signals

- Prompt text, model input, or model output.
- Tool arguments or tool results.
- Decision messages.
- Target IDs in metric logs.
- Rule IDs and reason codes in metric logs.
- Object URIs, raw URLs, filenames, checkpoint bytes, and archive object
  locations.
- Archive IDs, archived event IDs, thread/run/space/user IDs, target IDs,
  object keys, prompts, model input/output, tool arguments/results, and raw
  provider payloads as Prometheus labels.
- Credentials, bearer tokens, raw scanner payloads, provider raw bodies, raw
  metadata JSON, raw SQL, hidden run config, and raw repository errors.

## Verification

Before enabling retention cleanup:

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditArchiveWorker|TestGuardrailAuditArchiveMetrics|TestGuardrailAuditObjectStorageArchiveWriter|TestGuardrailAuditArchiveExporter'
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditRetention'
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditArchiveBeforeDelete'
go test -count=1 ./application/agentthread -run 'TestGuardrailPrometheus'
go test -count=1 ./domain/agentthread/repository -run GuardrailAudit
```

After enabling archive worker:

- Confirm archive metric logs show `success=true`.
- Confirm `archived` stays within the configured batch size.
- Confirm `archive_id` is present only as a stable identifier, not an object
  key, object URI, raw URL, or filename.
- Confirm the source audit rows remain queryable until retention cleanup is
  explicitly enabled.
- Confirm no forbidden signal appears in log samples.
- If Prometheus metrics are enabled, confirm metric families include
  `coze_guardrail_evaluations_total`,
  `coze_guardrail_audit_archive_attempts_total`, and
  `coze_guardrail_audit_retention_attempts_total`, and confirm labels do not
  include archive IDs, event IDs, object locations, prompts, or raw provider
  payloads.

After enabling retention cleanup:

- Confirm cleanup metric logs show `success=true`.
- If `AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED=true`, confirm
  cleanup metrics include `archived > 0` and a stable `archive_id` whenever
  `deleted > 0`.
- Confirm retention cleanup deletes only exact archived event IDs; do not
  accept a path that deletes by a fresh cutoff query after archive.
- Confirm `deleted` stays within the configured batch size.
- Confirm audit list/export still work for recent task threads.
- Confirm no forbidden signal appears in log samples.

When legal hold is enabled:

- Confirm archive and cleanup metric logs show `skipped=true`.
- Confirm `skip_reason=legal_hold`.
- Confirm `archived=0` and `deleted=0`.
- Confirm the archive and retention workers are still running.

## Rollback

1. Set `AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED=false`.
2. Set `AGENT_GUARDRAIL_AUDIT_ARCHIVE_WORKER_ENABLED=false`.
3. Set `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED=true` if cleanup
   must pause while retaining worker visibility.
4. Set `AGENT_GUARDRAIL_AUDIT_ARCHIVE_METRICS_LOG_ENABLED=false` and
   `AGENT_GUARDRAIL_AUDIT_RETENTION_METRICS_LOG_ENABLED=false` if log
   volume is high.
5. Set `AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED=false` if Prometheus label
   review fails or scrape volume is high.
6. Keep Guardrail provider settings unchanged unless policy decisions are also
   failing.
7. Restart backend workers.
8. Verify no archive or destructive retention cleanup observations appear.

## Incident Triage

- If cleanup fails repeatedly, disable the retention worker and inspect backend
  database health using internal admin tools.
- If export totals drop unexpectedly, compare retention cutoff settings with
  the intended compliance window and confirm legal hold was not expected.
- If archive export fails, pause retention cleanup before retrying the archive
  path. Confirm the object-storage writer returns only archive IDs and that
  logs do not contain object keys, object URIs, raw URLs, filenames, or
  provider error bodies.
- If archive worker repeatedly reports `guardrail_audit_archive_failed`,
  disable the archive worker before enabling retention cleanup.
- If require-archive retention cleanup fails, leave retention cleanup disabled
  until archive storage, archive payload validation, and exact-ID deletion have
  been checked.
- If metric logs contain forbidden signals, disable the relevant metrics flag
  and treat the log sink as sensitive until reviewed.
- If Prometheus metrics contain forbidden labels, disable
  `AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED` and treat the scrape sink as
  sensitive until reviewed.
- If scanner outage causes broad deny decisions, check provider status and
  fail-mode policy before changing retention settings.
