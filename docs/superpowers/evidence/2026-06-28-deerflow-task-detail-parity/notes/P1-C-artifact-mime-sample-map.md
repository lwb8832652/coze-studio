# P1-C Artifact MIME Sample Map

Captured: 2026-07-01

## Scope

P1-C builds the richer artifact MIME sample library after P0/P1-J proved the
DeerFlow-style document artifact path. This note maps existing source and test
coverage first, then keeps the remaining real-browser sample work explicit.

This is not a claim that the full sample library is complete. The current
covered state is an automated safety and rendering baseline for Markdown,
plain/text-like previews, CSV, PDF, PNG/image, active-content rejection,
blocked/error states, and delete/restore lifecycle. Browser screenshots and
API summaries for one consolidated fixture pack still remain open.

## DeerFlow Reference

Existing DeerFlow source and runtime references for generated files are already
recorded in:

- `notes/P1-J-006-artifact-present-files.md`
- `notes/P1-J-006-artifact-split-width.md`
- `notes/P1-A-005-artifact-side-preview.md`

Relevant DeerFlow source anchors from those notes:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-list.tsx`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-detail.tsx`

The parity target stays the same: generated file cards open the artifact panel,
safe previewable documents render in the panel, and download/copy actions stay
separate from thread export.

## Existing Coze Coverage Matrix

| Area | Current coverage | Evidence |
| --- | --- | --- |
| Markdown inline/side preview | Covered by component tests and P1-J/P1-A screenshots | `task-detail.test.tsx` generated document artifact cases; `task-artifacts-helpers.test.ts`; `notes/TD-DOC-003-008-markdown-preview-download.md`; `notes/P1-A-005-artifact-side-preview.md` |
| Plain text / JSON preview | Covered by helper and canonical artifact tests | `task-artifacts-helpers.test.ts`; `task-detail.test.tsx` canonical artifact case |
| CSV/table preview | Covered by helper and canonical artifact tests | `task-artifacts-helpers.test.ts`; `task-detail.test.tsx` canonical artifact case; `notes/TD-DOC-004-006-010-table-image-lifecycle.md` |
| PNG/image preview | Covered by preview-family helper and signed-url UI test | `task-artifacts-helpers.test.ts`; `task-detail.test.tsx` canonical artifact case |
| PDF preview/open | Covered by preview-family helper and signed-url UI test | `task-artifacts-helpers.test.ts`; `task-detail.test.tsx` canonical artifact case |
| HTML/XHTML/SVG active content | Covered as unsafe inline preview rejection and download-only path | `task-artifacts-helpers.test.ts`; `task-detail.test.tsx` canonical artifact case; backend sniff/header tests |
| Safe content headers | Covered by backend API tests | `workbench_thread_service_test.go` content and signed URL handler cases |
| Scanner blocked/error/review | Covered by frontend drawer tests and backend policy/API tests | `task-detail.test.tsx` blocked review and scan jobs cases; `service_test.go` scan policy cases |
| Delete/restore lifecycle | Covered by frontend UI tests and backend handler tests | `task-detail.test.tsx` delete/undo/deleted-list cases; handler list/delete/restore tests |
| Redaction boundary | Covered by frontend helper, backend API, and application audit tests | internal object URI hidden, signed URL response bounded, no raw scanner/provider/object payloads |

## Verification Run

Commands passed on 2026-07-01:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-artifacts-helpers.test.ts
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t 'canonical thread artifacts|generated document artifacts|deletes a canonical thread artifact|restores the last removed|lists deleted thread artifacts|reviews a blocked artifact|renders artifact scan jobs'
```

```bash
cd backend
go test ./api/handler/coze -run 'Test(GetTaskThreadArtifactContentHandler|GetTaskThreadArtifactSignedURLHandler|ReviewTaskThreadArtifactScanHandler|ListTaskThreadArtifactsHandler|DeleteTaskThreadArtifactHandler|RetryTaskThreadArtifactScanJobHandler|ListTaskThreadArtifactScanJobsHandler)' -count=1 -gcflags='all=-N -l'
go test ./application/agentthread -run 'Test(ApplicationReadArtifactContent|ArtifactScanReadPolicy|ApplicationRecordArtifactScanResult|ApplicationProcessArtifactScanJobs)' -count=1
```

Results:

- `task-artifacts-helpers.test.ts`: 3 passed.
- `task-detail.test.tsx` focused artifact subset: 7 passed, 42 skipped.
- `backend/api/handler/coze`: passed.
- `backend/application/agentthread`: passed.

## Remaining P1-C Gaps

1. Create one consolidated browser fixture task with generated or seeded
   artifacts for Markdown, TXT, JSON, CSV, PDF, PNG, HTML, and SVG.
2. Capture Coze UI evidence for each artifact family: card placement, side
   preview/open behavior, copy/download affordances, bounded errors, and no
   object URI or signed URL leakage.
3. Record bounded API summaries for artifact list, content, signed URL,
   scan-blocked conflict, review release, delete, and restore. Do not record
   raw object URIs, signed URLs, scanner raw bodies, provider payloads, prompt,
   completion, or checkpoint bytes.
4. Add browser evidence for active-content safety: HTML/SVG are not rendered as
   inline executable content and remain download-only or blocked by policy.
5. Add real UI evidence for deleted artifact list, undo restore, blocked scan
   review, and scan retry state.

## Status

P1-C is in progress. The automated coverage baseline is complete, but the real
MIME sample fixture pack and paired browser/API evidence are still open.
