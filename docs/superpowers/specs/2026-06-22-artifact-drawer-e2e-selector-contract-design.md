# Artifact Drawer E2E Selector Contract Design

## Goal

Prepare the task artifact drawer for browser E2E coverage without introducing
a new browser test framework in M4.37.

The current `@coze-studio/app` package has Vitest tests and no Playwright or
Cypress runner. The existing `@coze-data/e2e` package is a selector constant
package, not a runnable E2E harness. M4.37 therefore adds stable selectors and
unit-level assertions so a future browser suite can target the same DOM
contract.

## Selector Contract

The drawer exposes only stable, content-free selectors:

- `task-artifacts-open` on the task artifact drawer entry button;
- `task-artifact-item` on each visible artifact row, with safe
  `data-artifact-id`;
- `task-artifact-inline-preview` on the inline preview surface;
- `task-artifact-inline-preview-text` on source-text previews;
- `task-artifact-inline-preview-table` on CSV/TSV table previews.

Selectors must not contain object URI, virtual path, object key, signed URL,
scanner raw response, prompt text, model output, tool arguments, credentials,
checkpoint bytes, or provider raw bodies.

## Non-Goals

M4.37 does not add Playwright, Cypress, browser visual snapshots, image/PDF
pixel checks, or a CI browser job. Those remain production acceptance work.
