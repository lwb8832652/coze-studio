# Guardrail Audit Scheduled Archive Worker M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a disabled-by-default scheduled worker that archives expired Guardrail audit rows through the existing archive exporter and object-storage writer without deleting rows.

**Architecture:** Keep the archive worker separate from retention cleanup. The worker computes the cutoff from configured retention days, calls `GuardrailAuditArchiveExporter`, records content-free archive metrics, and honors the shared legal-hold switch before any archive attempt.

**Tech Stack:** Go, existing `agentthread` worker/env patterns, Coze `infra/storage`, existing Guardrail audit repository and archive exporter.

---

### Task 1: Archive Worker Contract Tests

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_archive_worker_test.go`

- [x] **Step 1: Write `RunOnce` delegation test**

```go
func TestGuardrailAuditArchiveWorkerRunOnceDelegates(t *testing.T) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        2,
			Total:           5,
			ArchiveID:       "archive_1",
		},
	}
	worker := NewGuardrailAuditArchiveWorker(archiver, GuardrailAuditArchiveWorkerOptions{
		RetentionDays: 1,
		NowMillis:     func() int64 { return 86401000 },
	})

	result := worker.RunOnce(context.Background())

	require.Equal(t, archiver.result, result)
	require.Equal(t, int64(1000), archiver.cutoffCreatedAt)
}
```

- [x] **Step 2: Write metrics, failure, and legal-hold tests**

Assert success metrics include cutoff, archived count, total, archive ID, retention days, batch size, and elapsed milliseconds. Assert failure metrics use only `guardrail_audit_archive_failed`. Assert legal hold skips archive calls with `skip_reason=legal_hold`.

### Task 2: Env Wiring Tests

**Files:**
- Modify: `backend/application/agentthread/guardrail_audit_archive_worker_test.go`

- [x] **Step 1: Test disabled-by-default env startup**

```go
worker, status := StartGuardrailAuditArchiveWorkerFromEnvWithStatus(
	context.Background(),
	&recordingGuardrailAuditRepository{},
	&recordingGuardrailAuditArchiveStorage{},
)
require.Nil(t, worker)
require.False(t, status.Enabled)
```

- [x] **Step 2: Test required repository and storage**

When enabled, missing repository returns reason `guardrail audit repository is not configured`; missing storage returns reason `guardrail audit archive storage is not configured`.

- [x] **Step 3: Test configured worker**

Set enabled, retention days, batch size, interval, object prefix, and metrics log env vars. Assert the worker starts with a `GuardrailAuditArchiveExporter`, configured writer, metrics collector, interval, retention days, and batch size.

### Task 3: Implementation

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_archive_worker.go`
- Create: `backend/application/agentthread/guardrail_audit_archive_metrics.go`
- Modify: `backend/application/application.go`

- [x] **Step 1: Add worker types**

Implement `GuardrailAuditArchiver`, `GuardrailAuditArchiveWorkerOptions`, `GuardrailAuditArchiveWorker`, `GuardrailAuditArchiveWorkerEnvStatus`, `NewGuardrailAuditArchiveWorker`, `Start`, and `RunOnce`.

- [x] **Step 2: Add metrics**

Implement `GuardrailAuditArchiveMetricsCollector`, `GuardrailAuditArchiveMetricsObservation`, and logging collector behind `AGENT_GUARDRAIL_AUDIT_ARCHIVE_METRICS_LOG_ENABLED`.

- [x] **Step 3: Add env startup**

Read `AGENT_GUARDRAIL_AUDIT_ARCHIVE_WORKER_ENABLED`, `AGENT_GUARDRAIL_AUDIT_ARCHIVE_RETENTION_DAYS`, `AGENT_GUARDRAIL_AUDIT_ARCHIVE_BATCH_SIZE`, `AGENT_GUARDRAIL_AUDIT_ARCHIVE_INTERVAL_MS`, and `AGENT_GUARDRAIL_AUDIT_ARCHIVE_OBJECT_PREFIX`. Reuse `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED` for legal-hold skip behavior.

- [x] **Step 4: Wire application startup**

Call `StartGuardrailAuditArchiveWorkerFromEnv` with the Guardrail audit repository and application object storage before retention cleanup startup.

### Task 4: Documentation And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/runbooks/guardrail-audit-operations.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Document M8.22**

Record that scheduled archive is disabled by default, content-free, legal-hold-aware, and non-destructive.

- [x] **Step 2: Run verification**

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditArchiveWorker|TestGuardrailAuditArchiveMetrics|TestGuardrailAuditArchiveExporter|TestGuardrailAuditObjectStorageArchiveWriter'
go test -count=1 ./application/agentthread
cd ..
git diff --check
```
