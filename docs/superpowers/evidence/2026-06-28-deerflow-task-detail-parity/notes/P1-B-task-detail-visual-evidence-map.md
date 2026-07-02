# P1-B Task Detail Visual Evidence Map

Captured: 2026-07-01

## Scope

P1-B is the task-detail visual hardening pass after the P1-A browser evidence
baseline. This note maps the existing DeerFlow/Coze screenshots and evidence
notes to the visible task-detail modules, then lists only the remaining visual
gaps that still need fresh capture or product confirmation.

This is a mapping and scoping note. It does not fabricate missing browser
Network evidence, and it does not reopen Skills/MCP detailed verification that
the user deferred to P2.

## Covered By Existing Evidence

| Visual area | Status | Evidence |
| --- | --- | --- |
| Overall desktop layout, header, message stream, docked composer | Covered for desktop | `notes/TD-MSG-001-002-TD-LAYOUT-001-browser-evidence.md`, `screenshots/TD-MSG-001-002-coze-viewport.png`, `screenshots/TD-MSG-001-002-deerflow-viewport.png`, `screenshots/coze/TD-LAYOUT-HDR-COMP-current-7657061782099329024.png`, `screenshots/coze/P1-A-001-standard-mermaid-run.png`, `screenshots/deerflow/P1-A-002-standard-mermaid-run.png` |
| Header actions: Tokens, export, detail, no stale breadcrumb/status | Covered for desktop | `notes/TD-HDR-003-TD-EXP-001-TD-TOKEN-001-browser-evidence.md`, `screenshots/coze/TD-LAYOUT-HDR-COMP-current-7657061782099329024.png`, `screenshots/coze/P1-A-007-coze-token-popover.png` |
| User/assistant message rendering and Markdown | Covered for desktop | `notes/TD-MSG-001-002-TD-LAYOUT-001-browser-evidence.md`, `notes/P1-J-005-run-journal-message-contract.md`, `screenshots/coze/P1-J-005-coze-journal-web-search.png`, `screenshots/deerflow/P1-J-005-deerflow-message-thinking.png` |
| Inline thinking / execution-step visual structure | Covered for desktop | `notes/P1-J-008-chain-of-thought-style.md`, `notes/TD-FLOW-001-coze-execution-feed-fix.md`, `screenshots/coze/P1-J-008-coze-expanded-steps.jpg`, `screenshots/deerflow/P1-J-008-deerflow-expanded-steps.jpg`, `screenshots/TD-FLOW-001-coze-execution-feed.png` |
| Mermaid sequence/flowchart rendering and block actions | Covered for desktop | `notes/TD-HDR-003-TD-EXP-001-TD-TOKEN-001-browser-evidence.md`, `screenshots/coze/P1-A-001-standard-mermaid-run.png`, `screenshots/deerflow/P1-A-002-standard-mermaid-run.png` |
| Generated document artifact card placement | Covered for desktop | `notes/TD-DOC-012-artifact-message-cards.md`, `notes/P1-J-006-artifact-present-files.md`, `screenshots/coze/TD-DOC-012-artifact-message-card-7657061782099329024.png`, `screenshots/deerflow/P1-J-006-deerflow-artifact-document-preview.png` |
| Right-side artifact preview width and Markdown preview | Covered for desktop/wide viewport | `notes/TD-DOC-012-artifact-message-cards.md`, `notes/P1-J-006-artifact-split-width.md`, `notes/P1-A-005-artifact-side-preview.md`, `screenshots/coze/TD-DOC-012-artifact-side-preview-layout-7657061782099329024.png`, `screenshots/coze/P1-A-005-coze-artifact-side-preview.png`, `screenshots/deerflow/P1-A-005-deerflow-artifact-side-preview.png` |
| Export entry and export content boundary | Covered by UI evidence and tests | `notes/TD-HDR-003-TD-EXP-001-TD-TOKEN-001-browser-evidence.md`, `notes/TD-EXP-002-export-content.md` |
| Token popover and per-turn token rows | Covered for desktop | `notes/P1-A-007-token-popover-modes.md`, `notes/TD-TOKEN-002-token-usage-modes.md`, `screenshots/coze/P1-A-007-coze-token-popover.png` |
| Follow-up history and composer ready state | Covered for desktop | `notes/P1-A-003-followup-history.md`, `notes/P1-J-007-stream-stop-followup.md`, `screenshots/coze/P1-A-003-coze-followup-history.png`, `screenshots/coze/P1-J-007-coze-followup-ready.jpg`, `screenshots/deerflow/P1-J-007-deerflow-input-ready.jpg` |
| Memory entry and memory management UI | Covered by P1-I evidence | `notes/P1-I-005-follow-up-memory-runtime.md`, `notes/P1-I-006-memory-ui-api-smoke.md`, `screenshots/coze/P1-I-005-follow-up-recall.png`, `screenshots/coze/P1-I-006-memory-panel-list.png`, `screenshots/coze/P1-I-006-memory-deleted.png`, `screenshots/coze/P1-I-006-memory-restored.png` |
| Loading/error bounded states | Covered by P1-A evidence and component tests | `notes/P1-A-008-loading-error-states.md`, `screenshots/coze/P1-A-008-coze-task-not-found.png` |
| Search/tool event visual safety | Covered for desktop | `notes/P1-A-006-search-tool-event-safety.md`, `screenshots/coze/P1-A-006-coze-search-tool-safe-steps.png`, `screenshots/deerflow/P1-A-006-deerflow-search-tool-safe-steps.jpg` |
| Narrow responsive layout | Covered for 390px and 768px | `screenshots/coze/P1-B-TD-LAYOUT-002-coze-390-task-detail.png`, `screenshots/coze/P1-B-TD-LAYOUT-002-coze-768-task-detail.png` |

## P2 Visual Hardening Items

P1-B is closed for launch-critical task-detail parity. These smaller visual
items are useful hardening, but they do not block the P1-B closure because the
main task-detail layout, messages, steps, artifacts, token, memory entry,
loading/error, and narrow responsive cases already have evidence or tests:

1. `TD-MSG-006` assistant copy/feedback affordance: Coze has copy coverage, but
   feedback affordance parity can be designed as a P2 interaction pass.
2. `TD-MD-003` Mermaid error visual state: component fallback exists in tests,
   but a real invalid Mermaid screenshot remains useful for P2 QA evidence.
3. `TD-FLOW-003` subagent deep card: safe metadata projection exists as a
   backend/frontend boundary, but no dedicated browser sample is recorded.
4. `TD-COMP-006` file upload visual path: composer shows upload affordance, but
   upload-to-task evidence is not part of the DeerFlow P1 closure.
5. `TD-TOKEN-003` active streaming token state: current policy avoids showing
   misleading zero values; live running-token evidence remains a P2 hardening
   item.
6. Rich MIME artifact samples remain under `P1-C`, not P1-B, so P1-B should
   not expand into CSV/PDF/image/HTML-SVG matrix work.

## Next Action

P1-B is closed. Continue P1 stabilization with the next open workstream in the
tracker.

## 2026-07-01 Responsive Capture Attempt

Attempted to use the in-app browser viewport capability to capture Coze
`TD-LAYOUT-002` at 390px and 768px on:

`http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`

The capture did not produce screenshots. The browser control surface reset
during tab binding; after reconnect, the only visible tab was `about:blank`,
and rebinding that tab timed out. No responsive evidence was written and
`TD-LAYOUT-002` remains open.

The follow-up 2026-07-02 pass below captured the responsive screenshots and
bounded layout summary successfully.

## 2026-07-02 Responsive Evidence And Fix

The first in-app browser check on
`http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`
showed a real 768px overflow: document `scrollWidth=1200` with
`clientWidth=768`. The root cause was the global Coze shell `html, body`
minimum width, which is valid for the broader Studio shell but unsafe for the
DeerFlow-style task-detail responsive page.

The fix is scoped to the mounted task-detail page only:

- `TaskDetailPage` adds `coze-task-detail-responsive-page` to `html` and
  `body`, then removes it on unmount.
- At `width <= 1199px`, task-detail overrides the page/body/root/flex minimum
  widths to allow shrinking.
- At `width <= 720px`, the left workspace submenu is hidden for task detail so
  a 390px viewport can keep the task content and follow-up composer usable.
- The global `frontend/apps/coze-studio/src/global.less` `min-width: 1200px`
  rule was not changed.

Clean headless browser verification against the original 765627 workspace was
blocked by the frontend route guard returning the permission page, despite the
same authenticated API session being able to list task threads. That page was
therefore not used as final proof. Final evidence uses an authenticated task
detail page under:

`http://localhost:8080/space/7645565700475453440/tasks/7657075192350375936`

Final browser metrics:

- 768px viewport: task detail mounted, skeleton gone, no login/permission page,
  `scrollWidth=768`, `clientWidth=768`, `horizontalOverflow=false`, sidebar
  visible at 300px, task content at 468px, scroll area at 420px.
- 390px viewport: task detail mounted, skeleton gone, no login/permission page,
  `scrollWidth=390`, `clientWidth=390`, `horizontalOverflow=false`, sidebar
  hidden, task content at 390px, scroll area at 342px, composer at 342px.

Screenshots:

- `screenshots/coze/P1-B-TD-LAYOUT-002-coze-768-task-detail.png`
- `screenshots/coze/P1-B-TD-LAYOUT-002-coze-390-task-detail.png`

Safe Network/DOM notes: the final verification page loaded the task thread,
runs, messages, artifacts, token usage, run events, run-event stream, recent
task list, and bot type list without exposing raw cookie values, prompt
payloads, object URIs, signed URLs, tool arguments/results, provider bodies, or
checkpoint bytes in this evidence note.
