# P1-A-001~003 Authenticated API Capture Decision

Captured: 2026-07-01

## Scope

This note closes the evidence decision for the remaining P1-A browser baseline
cases:

- `P1-A-001` Coze canonical new task run
- `P1-A-002` DeerFlow comparison run
- `P1-A-003` Coze follow-up preserves history

The original target asked for paired desktop browser evidence plus safe
Network/API summaries where applicable. During the browser pass, the visible
runtime evidence was captured, but authenticated Network export was not
available from the current in-app browser tooling.

## Evidence Already Captured

`P1-A-001` has Coze task-detail runtime evidence:

- Task URL:
  `http://localhost:8080/space/7656275718757679104/tasks/7657390468782620672`
- Screenshot:
  `screenshots/coze/P1-A-001-standard-mermaid-run.png`
- Safe browser summary:
  `network/coze/P1-A-001-standard-mermaid-run-summary.json`
- Auth-boundary check:
  `network/coze/P1-A-001-api-unauthenticated-summary.json`

`P1-A-002` has DeerFlow task-detail runtime evidence:

- Task URL:
  `http://localhost:2026/workspace/chats/165d8335-5aa2-48ec-8295-54c4a917fce7`
- Screenshot:
  `screenshots/deerflow/P1-A-002-standard-mermaid-run.png`
- Safe browser summary:
  `network/deerflow/P1-A-002-standard-mermaid-run-summary.json`

`P1-A-003` has Coze follow-up history runtime evidence:

- Task URL:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Screenshot:
  `screenshots/coze/P1-A-003-coze-followup-history.png`
- Safe browser summary:
  `notes/P1-A-003-coze-followup-history-summary.json`
- API capture attempt metadata:
  `network/coze/P1-A-003-followup-history-api-summary.json`

## Capture Limitation

Authenticated Network export is explicitly treated as not available for this
P1-A pass because:

- The in-app browser can navigate and capture visible authenticated pages, but
  does not expose a stable Network export primitive in this workflow.
- The read-only browser page scope did not expose `fetch` or
  `XMLHttpRequest`, so direct authenticated API replay from the page context
  failed.
- External unauthenticated HTTP checks only prove auth boundaries: Coze returns
  `401 missing session_key in cookie`; DeerFlow redirects to `/login`.
- Replacing authenticated Network evidence with unauthenticated curl output
  would be misleading.

## Acceptance Decision

For P1-A, these three cases are accepted with an explicit authenticated API
capture N/A reason. The browser baseline goal is satisfied by visible runtime
evidence, safe DOM/browser summaries, auth-boundary checks, source anchors, and
targeted handler/component tests already recorded in each case note.

No fake or inferred response payload was added. Any future authenticated
Network packet capture should be recorded as supplemental evidence, not as a
blocker for closing the P1-A browser baseline.

## Follow-up Placement

If a black-box authenticated API suite is still required, it belongs under
`P1-D LangGraph 兼容套件` or a dedicated external QA capture task, not under
P1-A. P1-A remains focused on paired browser-visible behavior and safe
evidence storage.

## Security Redaction

The stored P1-A summaries avoid prompt bodies, full model completions, tool
arguments, tool results, credentials, object URIs, signed URLs, raw provider
bodies, raw audit payloads, and checkpoint bytes.
