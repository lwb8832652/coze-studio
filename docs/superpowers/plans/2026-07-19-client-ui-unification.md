# Client UI Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` or `superpowers:executing-plans`
> to implement this plan task-by-task.

**Goal:** Apply the approved NewX visual direction to the production client
without removing existing routes, controls, or business behavior.

**Architecture:** Keep Coze and Semi components as the interaction layer.
Centralize shared color, surface, border, radius, and motion decisions in the
existing NewX token/component files. Keep page-specific layout changes in the
Workbench stylesheet and shell-specific changes in the workspace prototype
stylesheet.

**Tech Stack:** React, TypeScript, Less, Coze Design, Semi Design, Vitest.

---

### Task 1: Record the visual baseline

**Files:**
- Reference:
  `.superpowers/brainstorm/55672-1784443270/content/direction-1-v3.png`
- Audit:
  `.superpowers/audits/ui-unification-2026-07-19/`

- [x] Capture the current Chat Workbench at `1208x926`.
- [x] Capture the primary product and system-management routes.
- [x] Build a side-by-side target/current comparison.
- [x] Confirm the old template grid is the primary visible mismatch.

### Task 2: Lock the Workbench visual contract

**Files:**
- Modify:
  `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [x] Require the concise welcome heading.
- [x] Require six templates and category-icon wrappers.
- [x] Require compact template actions outside the removed creation card.
- [ ] Run the focused Workbench test and confirm it passes.

### Task 3: Implement the Workbench list

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.less`

- [x] Render Coze Design icons for each template category.
- [x] Preserve create, import, search, sort, tab, and template-click controls.
- [x] Replace the card grid with responsive compact rows.
- [x] Align the title, composer, toolbar, tabs, statistics, and hover/focus
      states with the approved direction.

### Task 4: Unify the application shell

**Files:**
- Modify: `frontend/apps/coze-studio/src/styles/newx-tokens.less`
- Modify: `frontend/apps/coze-studio/src/styles/newx-components.less`
- Modify:
  `frontend/apps/coze-studio/src/components/workspace-prototype.less`

- [x] Use one neutral canvas/surface hierarchy across product pages.
- [x] Use the NewX green accent for primary actions and active states.
- [x] Normalize borders, radii, shadows, control heights, and focus behavior.
- [x] Preserve the existing sidebar information architecture and task list.

### Task 5: Verify and compare

**Files:**
- Update audit images in:
  `.superpowers/audits/ui-unification-2026-07-19/`

- [ ] Run the Workbench Vitest suite.
- [ ] Run ESLint and TypeScript checks.
- [ ] Reload every audited route in the in-app browser.
- [ ] Capture the final Chat Workbench and compare it beside the approved
      reference at the same viewport.
- [ ] Confirm no fatal text, horizontal overflow, or console errors.
