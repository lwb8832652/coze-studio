# Baseline Findings

Date: 2026-06-28

## Evidence

- DeerFlow screenshot:
  `screenshots/deerflow/BASELINE-task-detail.png`
- Coze screenshot:
  `screenshots/coze/BASELINE-task-detail.png`
- DeerFlow DOM summary:
  `notes/BASELINE-deerflow-dom-summary.json`
- Coze DOM summary:
  `notes/BASELINE-coze-dom-summary.json`

## Key Findings

- DeerFlow task detail is primarily a chat-like thread surface with header
  token/export actions, inline thinking, rendered Mermaid, per-turn token
  summary, and a bottom composer with upload, mode, and model controls.
- Coze task detail keeps task naming and correctly shows completed terminal
  status. It renders two Mermaid diagrams as ready SVGs and hides raw
  Mermaid fences.
- Coze currently places Runtime Doctor, security audit, and task memory panels
  directly in the main content flow. This should be treated as a P0 layout
  gap: those panels are inspector/management surfaces and should not appear as
  chat transcript records.
- Coze lacks a visible DeerFlow-equivalent thread/task export action in the
  header. Current visible export actions are memory/audit-scoped.
- Coze does not yet show a DeerFlow-equivalent inline thinking block in this
  baseline.
- Coze needs per-turn/per-message token display parity, not only header-level
  aggregate usage.
- Document generation and preview must be validated through `TD-DOC-*` cases:
  generation, artifact registration, markdown/plain text preview, table
  preview, PDF/Office fallback, image preview, active-content safety, download,
  failure states, lifecycle, and redaction.

## API Sampling Status

Direct curl against both local apps returned 401, and the browser read-only
evaluation sandbox did not expose `fetch` or `XMLHttpRequest`. API evidence
therefore remains pending for this baseline and must be collected later with
browser-login-aware network capture or a controlled test-auth path.
