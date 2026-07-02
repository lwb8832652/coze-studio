# P1-A-002 DeerFlow Standard Run Evidence

Captured: 2026-07-01

## Scope

Fresh DeerFlow reference run for the standard Mermaid prompt. This note records
the browser/runtime state and visual evidence status.

## Result

- DeerFlow new-task page accepted the standard Mermaid prompt.
- The in-app browser navigated to a fresh task detail URL:
  `http://localhost:2026/workspace/chats/165d8335-5aa2-48ec-8295-54c4a917fce7`
- Browser title after navigation:
  `Mermaid Diagrams Request and Example - DeerFlow`
- Screenshot captured after the page stabilized:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-A-002-standard-mermaid-run.png`
- Safe browser summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/network/deerflow/P1-A-002-standard-mermaid-run-summary.json`

## Transient Capture Issue

- In-app browser screenshot capture for the DeerFlow task detail page timed out
  at `Page.captureScreenshot` immediately after navigation to the fresh run.
- Pulling a full DOM snapshot for DeerFlow also timed out in the in-app browser
  while the page was still settling.
- A later screenshot retry succeeded without changing the task URL.
- External unauthenticated HTTP check against the same URL returns
  `307 Temporary Redirect` to `/login`, so curl-only capture cannot replace the
  browser-authenticated page evidence.
- macOS `screencapture` is unavailable in this environment
  (`could not create image from display`), so system-level fallback screenshots
  are not available.

## Decision

Accept `P1-A-002` for P1-A with an explicit authenticated Network capture N/A
reason recorded in:

`docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-001-003-authenticated-api-capture-decision.md`

The DeerFlow visual/runtime evidence exists and the unauthenticated redirect
confirms the auth boundary. A future authenticated Network capture can be added
as supplemental evidence, but it no longer blocks the P1-A browser baseline.
