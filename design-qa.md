# AppDev Precision Glass Design QA

Date: 2026-07-10

## Scope

- Selected reference: option 2, Precision Glass.
- Product surface: webpage application list, create modal, AI development studio preview mode, and code mode.
- Browser: Codex in-app browser.
- Verified frontend URL: `http://localhost:8080/space/7658729310143905792/app-dev`.
- Verified project: `appdev_657118cb57aaf1e9` in workspace `7658729310143905792`.
- QA viewport: 1208 x 956 CSS pixels. The selected 1440 x 1024 reference was evaluated responsively because the integrated Coze workspace sidebar remains visible.

## Visual comparison

- Reference: `/Users/liuwenbo/.codex/generated_images/019f2888-2a0e-7a82-b93c-b399a4020e72/exec-9c2c74fd-c2c1-48bf-8cdb-e6bc30449ffe.png`
- Preview state: `.superpowers/audit/app-dev-redesign/11-precision-glass-preview-final.png`
- Code state: `.superpowers/audit/app-dev-redesign/15-precision-glass-code-fixed.png`
- List state: `.superpowers/audit/app-dev-redesign/16-precision-glass-list-stable.png`
- Create state: `.superpowers/audit/app-dev-redesign/17-precision-glass-modal-stable.png`

The implementation retains the existing Coze workspace shell and business controls while matching the selected reference's cool-white surfaces, cobalt primary actions, teal success states, fine borders, restrained elevation, compact segmented controls, technical canvas texture, and typography hierarchy.

## Blocking findings

- P0: none.
- P1: none.
- P2: none.

Resolved during QA:

- Removed the editor's fixed 1000px minimum width so the right panel no longer clips behind the workspace shell.
- Reflowed the code file-tree header and controls so Chinese labels remain horizontal and usable at the integrated viewport width.
- Waited for project history, runtime preview, and file contents before capturing evidence to avoid judging loading states as final UI.

## Interaction checks

- Project list loads the real project card and remains within the viewport.
- Create modal opens, remains fully inside the viewport, and preserves the existing form and actions.
- Preview and code tabs switch successfully.
- Runtime preview reaches the running state and loads the project iframe.
- `App.tsx` can be selected and its editor content loads successfully.
- No page-level horizontal overflow was observed in the verified states.
- No new application console errors were observed. Third-party Statsig telemetry timeouts were excluded because they are outside the product runtime and do not affect the flow.

## Accepted P3 differences

- The production page includes the existing Coze workspace sidebar; the selected visual reference was intentionally standalone.
- The website shown inside the preview iframe is project-generated content, so its hero artwork is not hard-coded into the AppDev product chrome.
- Toolbar controls wrap responsively at the narrower integrated viewport instead of reproducing the 1440px reference in a clipped single row.

final result: passed

---

# Chat Detail Experience Design QA

Date: 2026-07-22

## Scope

- Selected reference: approved focused-conversation and bottom-right token popover target.
- Reference image: `.superpowers/audits/chat-detail-redesign-2026-07-20/03-approved-token-popover-target.png`.
- Product surface: task detail header, conversation turns, execution summary, shared composer, token usage, artifact preview, detail drawer, notification and account actions.
- Intended browser: Codex in-app browser only.
- Accepted task URL: `http://localhost:8080/space/7645565700475453440/tasks/7663677877128265728`.

## Automated evidence

- Task detail regression: 3 test files passed, 73 tests passed.
- Approved visual contract: 4 tests passed after the final spacing adjustment.
- TypeScript: `npx tsc --noEmit --project tsconfig.json` passed.
- Changed-file ESLint: no errors; existing complexity and catch-handling warnings remain unchanged.
- Frontend development build: repeated hot rebuilds completed successfully.
- Backend: service remained available on port 8888 and authenticated workspace APIs returned HTTP 200.
- The task-detail source now has a static visual contract for content width, title metadata, real icon usage, execution-summary copy, floating To-dos and the bottom-right usage control.

## In-app browser evidence

- Browser: Codex in-app browser, authenticated personal workspace, viewport `1208 x 926`.
- Reference and implementation screenshots were inspected together after the same no-drawer task state was restored.
- Header height is `72px`; conversation and composer share the `843.7px` content width at this viewport.
- Composer height is `105.9px`, reduced from `148.8px`, while retaining the shared home/detail composer behavior.
- To-dos and token usage keep an `11.8px` horizontal gap at the real `138.8K` usage state.
- Expanding To-dos did not move or resize the composer; the execution list expanded and collapsed normally.
- Token usage opened with bounded input, output, session and current-reply totals plus the usage-detail action.
- The detail drawer preserved runtime, security, tool and memory sections; document cards still opened the source/preview panel and closed cleanly.
- The assistant mark now reuses the same NewX workspace brand component as the sidebar.
- No render crash or task-detail error state occurred. Existing Markdown child-key warnings and optional flow-infra diagnostic logs remain console-only legacy noise.

## Product findings

- P0: none.
- P1: none.
- P2: none.
- P3: existing Markdown child-key warning and optional flow-infra diagnostic logging are outside this visual alignment change.

final result: passed
