# Guardrail Audit Object Storage Writer M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the concrete object-storage writer for non-destructive Guardrail audit archive payloads.

**Architecture:** Keep `GuardrailAuditArchiveExporter` storage-agnostic and add `GuardrailAuditObjectStorageArchiveWriter` as the only object-storage adapter. The writer converts the existing metadata-only archive payload into snake_case JSON, writes it under an internal prefix, and returns only an opaque archive ID.

**Tech Stack:** Go, `encoding/json`, SHA-256, Coze `infra/storage`, existing `agentthread` application tests.

---

### Task 1: Writer Contract Tests

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_archive_storage_test.go`

- [x] **Step 1: Write the failing JSON archive test**

```go
func TestGuardrailAuditObjectStorageArchiveWriterWritesJSONArchive(t *testing.T) {
	objectStorage := &recordingGuardrailAuditArchiveStorage{}
	writer := NewGuardrailAuditObjectStorageArchiveWriter(
		GuardrailAuditObjectStorageArchiveWriterOptions{
			Storage: objectStorage,
			Prefix:  "guardrail/audit/archive",
		},
	)

	result, err := writer.WriteGuardrailAuditArchive(
		context.Background(),
		validGuardrailAuditArchivePayloadForStorageTest(),
	)

	require.NoError(t, err)
	require.NotContains(t, result.ArchiveID, "/")
	require.Equal(t, objectStorage.key, "guardrail/audit/archive/1500/"+result.ArchiveID+".json")
	require.Contains(t, string(objectStorage.content), `"event_id"`)
	require.Contains(t, string(objectStorage.content), `"cutoff_created_at"`)
	require.NotContains(t, string(objectStorage.content), "s3://")
}
```

- [x] **Step 2: Verify the test fails before implementation**

Run:

```bash
cd backend
go test -count=1 ./application/agentthread -run TestGuardrailAuditObjectStorageArchiveWriter
```

Expected: build failure because `NewGuardrailAuditObjectStorageArchiveWriter` and `GuardrailAuditObjectStorageArchiveWriterOptions` are undefined.

### Task 2: Concrete Writer

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_archive_storage.go`

- [x] **Step 1: Add the object-storage writer**

```go
type GuardrailAuditArchiveObjectStorage interface {
	PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error
}

type GuardrailAuditObjectStorageArchiveWriterOptions struct {
	Storage GuardrailAuditArchiveObjectStorage
	Prefix  string
}
```

- [x] **Step 2: Serialize only reviewed archive fields**

Use a private JSON struct with explicit tags for `schema`, `exported_at`, `cutoff_created_at`, `total`, and each metadata-safe event summary field.

- [x] **Step 3: Return only a stable archive ID**

Compute `guardrail_archive_<exported_at>_<cutoff>_<sha256-prefix>` from the JSON content and write the object to `<prefix>/<cutoff>/<archive_id>.json`. Do not return the object key.

- [x] **Step 4: Sanitize failures**

Return only `guardrail audit archive write failed` for validation, marshal, and storage errors.

### Task 3: Documentation And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/runbooks/guardrail-audit-operations.md`
- Modify: `docs/superpowers/plans/2026-06-27-guardrail-audit-archive-export-boundary-m8.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Document M8.21**

Record that the writer exists, writes only allow-listed metadata JSON, returns archive IDs only, and does not schedule, delete, bypass legal hold, or expose archive objects.

- [x] **Step 2: Run verification**

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailAuditObjectStorageArchiveWriter|TestGuardrailAuditArchiveExporter'
go test -count=1 ./application/agentthread
cd ..
git diff --check
```
