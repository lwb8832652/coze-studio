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
