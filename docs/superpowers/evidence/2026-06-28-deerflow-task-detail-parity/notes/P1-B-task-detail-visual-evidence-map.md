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

## Remaining P1-B Visual Gaps

These are the remaining task-detail visual items that still need either fresh
browser evidence or an explicit product decision:

1. `TD-LAYOUT-002` responsive screenshots: capture 390px and 768px Coze task
   detail screenshots, preferably with an artifact side preview closed/open
   where the layout supports it. DeerFlow comparison screenshots are useful
   but should not block Coze overlap/overflow verification.
2. `TD-MSG-006` assistant copy/feedback affordance: Coze has copy coverage, but
   feedback parity remains a P1 decision. If not building feedback now, record
   the reason and keep it out of the launch blocker list.
3. `TD-MD-003` Mermaid error visual state: component fallback exists in tests,
   but a real invalid Mermaid screenshot is still useful.
4. `TD-FLOW-003` subagent deep card: safe metadata projection exists as a
   backend/frontend boundary, but no dedicated browser sample is recorded.
5. `TD-COMP-006` file upload visual path: composer shows upload affordance, but
   upload-to-task evidence is not part of the DeerFlow P0 closure.
6. `TD-TOKEN-003` active streaming token state: current policy avoids showing
   misleading zero values; live running-token evidence remains a P1 hardening
   item.
7. Rich MIME artifact samples remain under `P1-C`, not P1-B, so P1-B should
   not expand into CSV/PDF/image/HTML-SVG matrix work.

## Next Action

Start with `TD-LAYOUT-002` because it is the only broad visual gap likely to
affect ordinary task-detail usage. Capture Coze 390px and 768px screenshots
for a completed task with visible messages, execution steps, token row, and
composer. If overlap is found, fix layout before moving to smaller edge cases.

## 2026-07-01 Responsive Capture Attempt

Attempted to use the in-app browser viewport capability to capture Coze
`TD-LAYOUT-002` at 390px and 768px on:

`http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`

The capture did not produce screenshots. The browser control surface reset
during tab binding; after reconnect, the only visible tab was `about:blank`,
and rebinding that tab timed out. No responsive evidence was written and
`TD-LAYOUT-002` remains open.

Do not mark P1-B complete until the responsive screenshots and bounded layout
summary are captured successfully.
