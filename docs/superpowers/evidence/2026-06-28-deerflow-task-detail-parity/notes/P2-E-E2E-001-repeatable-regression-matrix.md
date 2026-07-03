# P2-E-E2E-001 Repeatable Regression Matrix

## Scope

This slice converts the high-risk DeerFlow parity surfaces into repeatable
frontend DOM regression coverage using the test harness that exists in this
repository today.

The repository currently has Vitest DOM/component tests for the Coze Studio
frontend, but no Playwright/Cypress browser E2E harness under
`frontend/apps/coze-studio`. Real browser CI and mobile screenshot capture are
therefore recorded as explicit deferred follow-up, not claimed as complete.

## Coverage Matrix

| Case | Automated coverage | Evidence |
| --- | --- | --- |
| P2-E-E2E-002 task detail lifecycle DOM smoke | `task-detail.test.tsx`, `tasks.test.tsx` | Canonical detail load, loading skeleton, streaming/waiting dots, follow-up submit, stop/retry/resume, stale status closure, historical execution preservation, task title/list updates, pagination/loading states. |
| P2-E-E2E-003 artifacts/token/memory DOM smoke | `task-detail.test.tsx`, `task-memory-section.test.tsx` | Generated document placement, artifact preview/download/actions, artifact scan review affordances, token topbar/per-turn usage, task memory inspector and read-only controls. |
| P2-E-E2E-004 Skills/MCP/tools DOM smoke | `skill.test.tsx`, `tools.test.tsx`, `task-detail.test.tsx` | Skill page smoke, tools page smoke, Skill artifact install affordance, metadata-only MCP runtime audit panel, task detail extension popover wiring. |
| P2-E-E2E-005 narrow/mobile layout regression | `task-detail.test.tsx` | Scoped responsive class cleanup and narrow task-detail layout guards. True device screenshots still require a browser E2E harness. |

## Regression Fix

While running the matrix, the task detail composer docking test passed but
emitted a real network error to `localhost:3000`. Root cause: the test clicked
the `拓展` popover, which loads MCP registry entries through
`../../tools/service`; `task-detail.test.tsx` mocked Skills and model loaders
but did not mock `listMCPToolRegistryEntries`.

The test now mocks the same extension MCP registry loader pattern used by the
Workbench tests and asserts it is called with the expected `space_id`. This
keeps the regression matrix deterministic and prevents hidden API calls during
DOM smoke tests.

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "keeps the follow-up composer docked outside the scrollable chat transcript"
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/skill/__tests__/skill.test.tsx src/pages/tools/__tests__/tools.test.tsx
```

Result:

- `task-detail.test.tsx` targeted composer docking test passed without
  `ECONNREFUSED` / logger network errors.
- Full matrix passed: 5 files, 102 tests.
- Remaining output is the existing Browserslist/caniuse-lite age warning and
  Tailwind content count; no runtime network error remained.

## Deferred Follow-up

- Add a true Playwright or Cypress browser E2E harness for desktop task detail,
  new task, follow-up, artifacts, token popover, Skills/MCP/tools, and resource
  mention flows.
- Add mobile viewport screenshots for task detail, artifacts panel, composer
  dock, and task list infinite scroll once the browser harness exists.
