# P1-A-000 New Task Baseline

Captured: 2026-07-01

## Scope

This is a read-only environment baseline before starting the paired P1 browser
regression run. It confirms that the local DeerFlow and Coze new-task entry
pages are reachable and records safe screenshot/DOM evidence.

## Evidence

- DeerFlow screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-A-000-new-task-baseline.png`
- DeerFlow safe DOM summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/network/deerflow/P1-A-000-new-task-summary.json`
- Coze screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-000-new-task-baseline.png`
- Coze safe DOM summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/network/coze/P1-A-000-new-task-summary.json`

## Findings

- DeerFlow reference task URL
  `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`
  currently redirects to `http://localhost:2026/workspace/chats/new` in the
  in-app browser session. Treat the old reference thread as unavailable for
  this local environment and create a fresh DeerFlow reference run for P1-A.
- Coze new-task page is reachable at
  `http://localhost:8080/space/7656275718757679104/chats/new`.
- Coze evidence screenshot is cropped to the main workbench area so recent task
  titles are not stored in the baseline screenshot.
- DOM summaries are redacted: no full body text, recent task titles, prompt
  text, message content, tool arguments/results, signed URLs, object URIs,
  credentials, raw provider body, or checkpoint bytes are stored.

## Next Step

Continue with `P1-A-001` and `P1-A-002`: run the same standard Mermaid prompt
in a fresh DeerFlow task and a fresh Coze task, then capture paired screenshots
and safe Network/API summaries.
