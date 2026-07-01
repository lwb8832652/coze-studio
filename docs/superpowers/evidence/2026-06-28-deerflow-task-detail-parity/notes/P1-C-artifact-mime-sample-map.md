# P1-C Artifact MIME Sample Map

Captured: 2026-07-01

## Scope

P1-C builds the richer artifact MIME sample library after P0/P1-J proved the
DeerFlow-style document artifact path. This note maps existing source and test
coverage first, then keeps the remaining real-browser sample work explicit.

This is not a claim that the full sample library is complete. The current
covered state is an automated safety and rendering baseline plus one real
browser fixture task covering Markdown, TXT, JSON, CSV, PDF, PNG, HTML, and
SVG artifact cards. API summaries, every-family side-preview screenshots, and
the scan/delete/restore UI lifecycle evidence remain open.

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
go test ./application/agentthread -run 'TestADKArtifactToolCatalog|TestDefaultADKToolProviderCanWireArtifactTools|TestApplication(WriteOutputFileStoresObjectAndRegistersOutputFile|CreateSkillPackageWritesInstallableSkillArchive|PresentOutputFilesRegistersArtifactsAndEmitsSafeEvent)' -count=1
```

Results:

- `task-artifacts-helpers.test.ts`: 3 passed.
- `task-detail.test.tsx` focused artifact subset: 7 passed, 42 skipped.
- `backend/api/handler/coze`: passed.
- `backend/application/agentthread`: passed.
- `backend/application/agentthread` ADK artifact binary/write-file subset:
  passed.

## 2026-07-01 Browser Fixture Evidence

Fixture task:

- `http://localhost:8080/space/7656275718757679104/tasks/7657556998191316992`

Implementation note:

- Go ADK `write_file` now accepts either UTF-8 `content` or binary
  `content_base64`. This closes the fixture-generation gap where true PNG/PDF
  bytes could not be produced by the Go-native artifact tool path without a
  Python sidecar.
- The tool rejects calls that provide both fields, rejects calls that provide
  neither field, strips harmless base64 whitespace, and returns only bounded
  file metadata. It does not echo object URIs or base64 payloads.

Artifact card summary captured from the Coze task detail page:

```text
p1c-fixture.md    Markdown file  下载
p1c-fixture.txt   Text file      下载
p1c-fixture.json  JSON file      下载
p1c-fixture.csv   CSV file       下载
p1c-fixture.pdf   PDF file       下载
p1c-fixture.png   Image file     下载
p1c-fixture.html  HTML file      下载
p1c-fixture.svg   Image file     下载
```

Screenshots:

- Full fixture page:
  `screenshots/coze/P1-C-coze-mime-fixture-task-7657556998191316992.png`
- Markdown preview:
  `screenshots/coze/P1-C-coze-markdown-preview-7657556998191316992.png`
- PNG preview:
  `screenshots/coze/P1-C-coze-png-preview-7657556998191316992.png`
- TXT preview:
  `screenshots/coze/P1-C-coze-txt-preview-7657556998191316992.png`
- JSON preview:
  `screenshots/coze/P1-C-coze-json-preview-7657556998191316992.png`
- CSV preview:
  `screenshots/coze/P1-C-coze-csv-preview-7657556998191316992.png`
- HTML/SVG download-only state:
  `screenshots/coze/P1-C-coze-html-svg-download-only-7657556998191316992.png`

Observed behavior:

- Markdown opened in the side artifact panel and rendered the document content.
- TXT opened in the side artifact panel and rendered plain text.
- JSON opened in the side artifact panel and rendered formatted JSON text.
- CSV opened in the side artifact panel and rendered the delimited content.
- PNG opened in the side artifact panel and loaded as an image with natural size
  `1x1`; the underlying image URL is a signed preview URL and is intentionally
  not recorded here.
- HTML and SVG remained card/download entries; page-level script probes stayed
  false: `window.__p1c_should_not_run=false` and
  `window.__p1c_svg_should_not_run=false`.
- The visible page text included `content_base64` because the fixture task
  prompt itself contained base64 inputs for PDF/PNG/HTML/SVG generation. This
  was user-visible test input text, not a provider/tool payload, object URI, or
  signed URL leak.

Browser API capture note:

- The in-app browser read-only page scope could not provide an authenticated
  `fetch` summary for artifact APIs in this pass. No inferred request/response
  payloads are recorded. P1-C-004 stays open for bounded API summaries.

PDF parity note:

- DeerFlow source check: `artifact-file-detail.tsx` renders non-code artifacts
  inside the Artifact panel via iframe, while still exposing open-in-new-window
  and download actions.
- Coze previously opened PDF preview with `window.open`, so the right-side
  artifact panel did not switch to the PDF. The frontend contract was adjusted
  so `preview_mode=pdf` now sets a PDF preview state rendered by a right-side
  iframe. The existing download action is unchanged.
- Unit evidence: `task-detail.test.tsx` now asserts PDF preview uses
  `iframe[data-testid="task-artifact-inline-preview-pdf"]` and does not call
  `window.open`.
- Browser screenshot for PDF remains open because the in-app browser automation
  timed out twice while reloading the fixture page after the frontend change.

## Remaining P1-C Gaps

1. Capture the PDF right-side preview screenshot after the browser connection
   is stable again, plus copy/download affordance checks where the UI exposes
   copy.
2. Record bounded API summaries for artifact list, content, signed URL,
   scan-blocked conflict, review release, delete, and restore. Do not record
   raw object URIs, signed URLs, scanner raw bodies, provider payloads, prompt,
   completion, or checkpoint bytes.
3. Add real UI evidence for deleted artifact list, undo restore, blocked scan
   review, and scan retry state.
4. Replace the prompt-embedded base64 fixture with a cleaner seeded fixture or
   dedicated skill/tool-driven fixture if future evidence needs zero
   `content_base64` visible-text hits.

## Status

P1-C is in progress. The automated coverage baseline, Go binary write support,
and one real MIME fixture task are complete. API summaries and remaining
per-family/lifecycle browser evidence are still open.
