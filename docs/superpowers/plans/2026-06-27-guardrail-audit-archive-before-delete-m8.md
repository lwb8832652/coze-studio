# Guardrail Audit Archive Before Delete M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Require a successful metadata-only Guardrail audit archive before retention cleanup deletes expired audit rows.

**Architecture:** Add a small cleaner wrapper around `GuardrailAuditArchiveExporter` and `GuardrailAuditRetentionReaper`. The wrapper computes one cutoff, archives one bounded batch, and only then deletes the exact archived event IDs through the retention reaper; env wiring keeps the existing deletion path unless the new require-archive switch is enabled.

**Tech Stack:** Go, existing `agentthread` retention worker, Guardrail archive exporter, object-storage writer, repository batch delete.

---

### Task 1: Archive-Before-Delete Contract Tests

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_archive_before_delete_test.go`

- [x] **Step 1: Write same-cutoff archive then exact-ID delete test**

```go
func TestGuardrailAuditArchiveBeforeDeleteCleanerArchivesBeforeDeletingSameCutoff(t *testing.T) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{CutoffCreatedAt: 1000, Archived: 2, ArchivedEventIDs: []int64{101, 102}, Total: 5, ArchiveID: "archive_1"},
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{
		result: GuardrailAuditRetentionReaperResult{CutoffCreatedAt: 1000, Deleted: 2},
	}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(GuardrailAuditArchiveBeforeDeleteCleanerOptions{
		Archiver:      archiver,
		Deleter:       deleter,
		Retention:     24 * time.Hour,
		BatchSize:     2,
		NowMillis:     func() int64 { return 86401000 },
	})

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(1000), archiver.cutoffCreatedAt)
	require.Equal(t, int64(1000), deleter.cutoffCreatedAt)
	require.Equal(t, []int64{101, 102}, deleter.eventIDs)
	require.Equal(t, int64(2), result.Archived)
	require.Equal(t, int64(2), result.Deleted)
	require.Equal(t, "archive_1", result.ArchiveID)
}
```

- [x] **Step 2: Write fail-closed tests**

Assert delete is not called when archive fails, when no rows were archived, when archived event IDs are missing, or when the archiver returns a mismatched cutoff. Errors must collapse to `guardrail audit retention cleanup failed` and must not include object keys, raw URLs, credentials, prompts, archive IDs, or provider errors.

### Task 2: Env Wiring Tests

**Files:**
- Modify: `backend/application/agentthread/guardrail_audit_retention_worker_test.go`

- [x] **Step 1: Test require-archive needs storage**

Set `AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED=true` and `AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED=true`; call env startup without storage and assert it refuses to start with reason `guardrail audit archive storage is not configured`.

- [x] **Step 2: Test require-archive builds wrapper**

Pass recording storage, assert `worker.cleaner` is `*GuardrailAuditArchiveBeforeDeleteCleaner`, and assert the wrapped exporter and reaper both use the configured retention batch size.

### Task 3: Implementation

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_archive_before_delete.go`
- Modify: `backend/application/agentthread/guardrail_audit_retention_reaper.go`
- Modify: `backend/application/agentthread/guardrail_audit_retention_worker.go`
- Modify: `backend/application/agentthread/guardrail_audit_retention_metrics.go`
- Modify: `backend/application/application.go`
- Modify: `backend/domain/agentthread/repository/guardrail_audit.go`

- [x] **Step 1: Add exact archived-ID delete method**

Add `CleanupGuardrailAuditEventsByIDs(ctx, cutoffCreatedAt, eventIDs)` to `GuardrailAuditRetentionReaper` and `DeleteGuardrailAuditEventsByIDs` to the repository so archive-before-delete deletes exactly the rows that were archived, not a fresh cutoff query that could race with inserts.

- [x] **Step 2: Add wrapper cleaner**

Implement `GuardrailAuditArchiveBeforeDeleteCleaner`, `GuardrailAuditRetentionDeleter`, and options with retention, batch size, and `NowMillis`.

- [x] **Step 3: Add env switch**

Add `AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED`. When enabled, retention worker startup requires object storage and builds exporter + object-storage writer + exact-cutoff reaper wrapper. When disabled, keep the existing direct reaper.

- [x] **Step 4: Extend retention result and safe metrics**

Add `Archived` and `ArchiveID` to retention cleanup result and metrics. Metrics/logs may include only stable archive IDs, not object keys or storage locations.

### Task 4: Documentation And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/runbooks/guardrail-audit-operations.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Document M8.23**

Record that archive-before-delete is opt-in, fail-closed, legal-hold-aware through the existing retention worker, and metadata-only.

- [x] **Step 2: Run verification**

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditArchiveBeforeDelete|TestGuardrailAuditRetentionWorker|TestGuardrailAuditRetentionReaper|TestGuardrailAuditArchiveExporter|TestGuardrailAuditObjectStorageArchiveWriter'
go test -count=1 ./application/agentthread
go test -count=1 ./application
cd ..
git diff --check
```
