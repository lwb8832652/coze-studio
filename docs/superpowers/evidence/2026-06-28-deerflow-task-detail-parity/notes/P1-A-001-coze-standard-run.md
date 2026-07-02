# P1-A-001 Coze Standard Run Evidence

Captured: 2026-07-01

## Scope

Fresh Coze task run for the same standard Mermaid prompt used in DeerFlow.
This validates the first P1 paired browser baseline for task creation and
completed task detail rendering.

## Result

- Coze task detail URL:
  `http://localhost:8080/space/7656275718757679104/tasks/7657390468782620672`
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-001-standard-mermaid-run.png`
- Safe browser summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/network/coze/P1-A-001-standard-mermaid-run-summary.json`

## Visible Evidence

- Top bar shows compact `Tokens` and `导出`.
- Task detail shows completed content, not a running/stop state.
- Execution steps are visible with bounded step metadata.
- Generated Markdown artifact card is rendered in the message stream.
- Right-side artifact preview is open and renders Mermaid diagrams.
- Bottom composer remains available for follow-up.

## Closure Decision

This case is accepted for P1-A with an explicit authenticated API capture N/A
reason recorded in:

`docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-001-003-authenticated-api-capture-decision.md`

The browser summary already redacts prompt text, full assistant output, tool
arguments/results, credentials, object URIs, signed URLs, raw provider bodies,
and checkpoint bytes.

Unauthenticated curl checks against task, run, and event endpoints returned
`401 missing session_key in cookie`, which is recorded as an auth-boundary
check, not as a replacement for authenticated API payload evidence:

`docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/network/coze/P1-A-001-api-unauthenticated-summary.json`
