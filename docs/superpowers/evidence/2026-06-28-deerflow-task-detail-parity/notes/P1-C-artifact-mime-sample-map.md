# P1-C Artifact MIME Sample Map

Captured: 2026-07-01

## Scope

P1-C builds the richer artifact MIME sample library after P0/P1-J proved the
DeerFlow-style document artifact path. This note maps existing source and test
coverage first, then keeps the remaining real-browser sample work explicit.

This is not a claim that the full sample library is complete. The current
covered state is an automated safety and rendering baseline plus one real
browser fixture task covering Markdown, TXT, JSON, CSV, PDF, PNG, HTML, and
SVG artifact cards. Visible PDF page-pixel rendering remains open.

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
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts
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
- `tasks-service.test.ts`: 14 passed.
- `task-detail.test.tsx` focused artifact subset: 7 passed, 42 skipped.
- `backend/api/handler/coze`: passed.
- `backend/application/agentthread`: passed.
- `backend/application/agentthread` ADK artifact binary/write-file subset:
  passed.
- 2026-07-02 copy fallback regression:
  `npm run test -- src/pages/tasks/__tests__/task-clipboard.test.ts`
  passed with 3 tests.
- 2026-07-02 focused copy/download regression:
  `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t 'renders Mermaid answer markdown through the markdown viewer|renders generated document artifacts in a DeerFlow-style side preview without mixing them with thread export'`
  passed with 2 tests, 48 skipped.
- 2026-07-02 stale artifact preview regression:
  `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t 'clears the active artifact preview after reviewing the previewed artifact'`
  passed with 1 test, 50 skipped.

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
- PDF clean iframe mount after review release:
  `screenshots/coze/P1-C-coze-pdf-preview-clean-7657556998191316992.png`
- SVG delete/restore removed-list state:
  `screenshots/coze/P1-C-coze-delete-restore-deleted-7657556998191316992.png`
- SVG delete/restore restored active-list state:
  `screenshots/coze/P1-C-coze-delete-restore-restored-7657556998191316992.png`
- SVG scan review blocked state:
  `screenshots/coze/P1-C-coze-scan-review-blocked-7657556998191316992.png`
- SVG scan review released state:
  `screenshots/coze/P1-C-coze-scan-review-released-7657556998191316992.png`
- Scan retry failed state:
  `screenshots/coze/P1-C-coze-scan-retry-failed-7657556998191316992.png`
- Scan retry after state:
  `screenshots/coze/P1-C-coze-scan-retry-after-7657556998191316992.png`
- Copy/download affordance state:
  `screenshots/coze/P1-C-coze-copy-download-affordance-7657556998191316992.png`
- Bounded scan-blocked preview error state:
  `screenshots/coze/P1-C-coze-bounded-error-scan-blocked-message-7657556998191316992.png`

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
- PDF artifact metadata is present with `content_type=application/pdf` and
  `preview_mode=pdf`, but the fixture task's artifact scan rows and scan jobs
  are still `pending`. The backend therefore correctly rejects preview signed
  URL generation by scan policy. The frontend now maps bounded
  `reason=scan_pending` to the safe user-facing message
  `产物安全扫描中，暂不能预览`, without exposing object URI, signed URL,
  scanner raw payload, provider payload, prompt, completion, or checkpoint
  bytes.
- The existing artifact management panel was then opened through task `详情` ->
  `产物 8`; it showed `扫描队列 8`, pending scan tags, and per-artifact
  review actions. Clicking the single test PDF's `放行产物 p1c-fixture.pdf`
  action changed that row to `clean` and removed the review actions for the PDF.
- After release, clicking `预览 p1c-fixture.pdf` mounted
  `iframe[data-testid="task-artifact-inline-preview-pdf"]` with title
  `预览 p1c-fixture.pdf`, a signed HTTP preview URL, and no visible
  `读取任务产物失败，请稍后重试` or `产物安全扫描中` message. Browser script
  checks confirmed the iframe `src` did not contain object URI patterns such as
  `s3://`, `tos://`, or `file_id`.
- The screenshot records the management drawer iframe container after release.
  It does not claim PDF page pixels rendered in the browser capture; the current
  in-app browser screenshot still showed a blank iframe surface even though the
  iframe was mounted with a signed preview URL.
- The same artifact management panel was used to validate the delete/restore
  UI lifecycle on the visible `p1c-fixture.svg` sample. Clicking
  `删除 p1c-fixture.svg` opened the bounded confirmation copy
  `移除任务产物？只会从任务详情隐藏该产物，不会删除底层文件。`; confirming with
  `移除` moved the row out of the active list. The removed-list capture shows
  the `已移除` tab with `已移除 1`, `p1c-fixture.svg`, and
  `恢复 p1c-fixture.svg`. The restored-state capture verifies that `恢复`
  returned the SVG row to the active list with
  the expected `预览` / `放行` / `隔离` / `阻断` / `下载` / `删除` actions.
  This browser pass also exposed a stale undo notice after restoring from the
  `已移除` list; the frontend now clears that notice through the deleted-list
  restore callback, and `task-detail.test.tsx` locks the regression with
  `clears the undo notice when restoring a removed artifact from the deleted list`.
  The restored-state screenshot was recaptured after the fix and shows no
  `撤销移除` notice while the SVG row is back in the active list.
  No raw object URI, signed URL, scanner raw body, provider payload, prompt,
  completion, or checkpoint bytes were recorded in these lifecycle screenshots.
- The management panel was also used to validate manual scan review for the
  same SVG sample. Clicking `阻断产物 p1c-fixture.svg` moved the row to
  `blocked` and left only bounded review/actions visible: `放行`, `隔离`,
  `下载`, and `删除`. Clicking `放行产物 p1c-fixture.svg` then moved the row to
  `clean`, removed review actions, and left `下载` / `删除` only. Browser script
  checks for both states found zero visible hits for `s3://`, `tos://`,
  `file_id`, `checkpoint`, tool-argument labels, or provider payload markers.
- Scan retry UI evidence was captured against the same fixture task. Because
  the local debug artifact scan worker is disabled, all fixture scan jobs were
  initially `pending`. For UI evidence only, one fixture scan job
  `7657557151346327552` was changed from `pending` to `failed` with bounded
  metadata (`worker_id=codex-ui-evidence`, `attempt_count=1`,
  `last_error=artifact scan failed`). The management panel then showed the
  `failed` tag and `重试` button for that job. Clicking the page's real
  `重试扫描任务 7657557151346327552` action called the frontend retry path and
  returned the job to `pending` with bounded `last_error=manual retry
  requested`; the retry button disappeared. Browser visible-text checks after
  retry found zero hits for `s3://`, `tos://`, `file_id`, `checkpoint`,
  `provider_payload`, `scanner_raw`, or `agent-runtime/`.
- Bounded error-state evidence was captured from the same fixture task. The
  local debug policy currently uses an outage mode that may allow non-executable
  `pending` image/text previews, so the browser pass used a real manual review
  transition instead of treating `pending` as fail-closed evidence. Clicking
  `阻断产物 p1c-fixture.png` changed the PNG row to `blocked`; reloading the
  page confirmed no stale `img[data-testid="task-artifact-inline-preview-image"]`
  remained. Clicking `预览 p1c-fixture.png` then showed the bounded user-facing
  error `产物安全扫描未通过，暂不能预览`, did not mount an image preview, and
  visible text checks found zero hits for `s3://`, `tos://`, `file_id`,
  `object_uri`, `agent-runtime/`, `checkpoint`, `provider_payload`,
  `scanner_raw`, `signed_url`, or `raw_body`. This browser pass exposed a stale
  preview bug: the task transcript and the artifact drawer keep separate
  preview action state, so reviewing an already-previewed artifact could leave
  a previously signed preview mounted after the artifact became blocked. The
  frontend now clears the affected inline preview after successful review and
  also clears transcript-side previews when refreshed artifact metadata moves
  the previewed artifact into a blocking scan status. The regression is locked
  by `clears the active artifact preview after reviewing the previewed artifact`.
- The visible page text included `content_base64` because the fixture task
  prompt itself contained base64 inputs for PDF/PNG/HTML/SVG generation. This
  was user-visible test input text, not a provider/tool payload, object URI, or
  signed URL leak.
- Copy/download affordance checks were run after reloading the task page.
  Browser DOM evidence showed all 8 artifact message cards expose visible
  download buttons; the right-side Markdown artifact preview exposes the
  DeerFlow-style code/preview toggle and a header copy button; the Mermaid block
  exposes download-SVG and copy-source icon buttons. Clicking
  `复制文档 p1c-fixture.md` initially left the browser clipboard empty, because
  the frontend relied only on `navigator.clipboard?.writeText` and silently did
  nothing when the API was blocked or unavailable. The frontend now routes
  document-preview copy and Mermaid-source copy through `copyTextToClipboard`,
  which falls back to a temporary textarea and `document.execCommand('copy')`.
  The same browser action was recaptured after the fix: clipboard length was
  `590`, it contained `# P1-C Fixture: Markdown`, and unsafe visible-pattern
  checks found zero hits for `s3://`, `tos://`, `file_id`, `checkpoint`,
  `provider_payload`, or `scanner_raw`.

## 2026-07-02 Bounded API Summary

Browser API capture note:

- The in-app browser read-only page scope could not provide an authenticated
  `fetch` summary for artifact APIs in this pass. No inferred live
  request/response payloads are recorded.
- The bounded API summary below is source/test based. Evidence anchors:
  `backend/api/router/coze/api.go` routes,
  `backend/api/model/workbench/thread/thread.go` request/response structs,
  `backend/api/handler/coze/workbench_thread_service.go` handlers, and
  `backend/api/handler/coze/workbench_thread_service_test.go` redaction tests.
- `task.thrift` defines the generated artifact list and signed URL shape for
  `/artifacts` and `/signed_url`; the scan/content/delete/restore routes are
  currently modeled directly in Go router/model structs.

Routes and bounded response contracts:

| Area | Route | Request shape | Safe response shape | Redaction evidence |
| --- | --- | --- | --- | --- |
| List artifacts | `GET /api/workbench/task_threads/:thread_id/artifacts` | `thread_id`, optional `run_id`, `deleted_only`, `space_id`, `page`, `page_size` | `artifacts[]` with ids, title, artifact type, virtual path, content type, size, preview mode, metadata, timestamps, plus `total` | Handler test asserts list response includes safe metadata but not `agent-runtime/` object URI. |
| List scan jobs | `GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs` | `thread_id`, optional `run_id`, `artifact_id`, `space_id`, `status`, `scanner`, `page`, `page_size` | `jobs[]` with job/thread/run/artifact/file ids, scanner, status, worker id, attempt count, bounded last error, timestamps, plus `total` | Handler test asserts no `agent-runtime/`, `/mnt/user-data`, or original `secret.txt`. |
| Retry scan job | `POST /api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry` | `thread_id`, `job_id`, optional `space_id` | `job` safe projection plus `retried` | Handler tests cover failed -> pending retry and non-failed -> conflict, both without object URI or `/mnt/user-data`. |
| Review scan | `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review` | `thread_id`, `artifact_id`, optional `space_id`, JSON `decision`, `reason` | `artifact_id`, `decision`, `scan_status`, `reviewed` | Handler test asserts response and emitted event omit object URI, `/mnt/user-data`, original filename, artifact bytes, and reviewer reason. |
| Read content | `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content` | `thread_id`, `artifact_id`, optional `space_id`, `mode=preview|download` | raw bytes only after scan policy allows; headers include content type, bounded content-disposition filename, `X-Content-Type-Options: nosniff` | Handler tests assert safe headers, no object URI in filename, and scan-blocked conflict returns bounded `reason=scan_blocked` without bytes/object path/original filename. |
| Signed URL | `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url` | `thread_id`, `artifact_id`, optional `space_id`, `mode`, `ttl_seconds` | `artifact_id`, `url`, bounded `expires_in_seconds`, `content_type`, `preview_mode` | Handler tests assert the service signs the internal object key but response omits `object_uri` and the raw object URI; TTL is clamped to safe bounds. Actual signed URLs are not recorded in this evidence file. |
| Delete artifact | `DELETE /api/workbench/task_threads/:thread_id/artifacts/:artifact_id` | `thread_id`, `artifact_id`, optional `space_id` | `code=0`, `msg=success` only | Handler/list tests verify deleted artifact disappears from active list. |
| Restore artifact | `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore` | `thread_id`, `artifact_id`, optional `space_id` | `artifact_id`, `restored` | Handler test asserts restore response and event payload omit object URI, `/mnt/user-data`, and filename. |

Security boundary recorded for P1-C-004:

- Safe to expose: ids, title, artifact type, virtual path, content type,
  preview mode, size, timestamps, scanner/status/attempt count, bounded scan
  error text, and bounded lifecycle decisions/status.
- Not recorded and not exposed by tests: raw object URI, raw signed URL
  evidence, scanner raw body, provider body, prompt, completion, checkpoint
  bytes, artifact bytes in JSON error responses, or reviewer free-form reason
  in emitted lifecycle event payloads.

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
- The current fixture records PDF card placement and clean iframe mounting after
  review release, while the scan-pending safe message is covered by service and
  task-detail component tests. Full browser lifecycle evidence for retry,
  delete, restore, and visible PDF page pixels remains under P1-C-005.

## Remaining P1-C Gaps

1. Capture a browser image that proves PDF page pixels render rather than only
   iframe mount state.
2. Replace the prompt-embedded base64 fixture with a cleaner seeded fixture or
   dedicated skill/tool-driven fixture if future evidence needs zero
   `content_base64` visible-text hits.

## Status

P1-C is in progress. The automated coverage baseline, Go binary write support,
one real MIME fixture task, scan-pending safe message mapping, PDF review
release, browser iframe-mount evidence for the current PDF artifact,
delete/restore UI lifecycle evidence, manual scan review evidence, and
copy/download affordance checks, bounded API summaries, and scan retry UI
evidence, bounded error-state browser evidence, and stale-preview regression
coverage are complete. PDF pixel-render evidence is still open.
