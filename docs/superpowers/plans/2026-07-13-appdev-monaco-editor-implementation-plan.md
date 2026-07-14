# AppDev Monaco Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development or superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Replace the AppDev textarea with the shared Monaco editor while
preserving Coze's existing version-aware file save flow.

**Architecture:** Keep `CodeEditor` as a presentation adapter over
`@coze-arch/bot-monaco-editor`. Continue to source project state and mutations
from `useAppDevFiles`; do not introduce a second store or Nuwax service client.

**Tech Stack:** React 18, TypeScript, Vitest, Monaco Editor, Rush.js, Less.

---

### Task 1: Add the AppDev Monaco adapter contract

**Files:**

- Test: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/code-editor.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/code-editor.tsx`

- [ ] Test path, language, value changes, dirty state, and save command wiring.
- [ ] Replace the textarea with shared Monaco while retaining existing props.
- [ ] Contain Monaco initialization failures inside the editor surface.

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/code-editor.test.tsx
```

Expected: the Monaco adapter tests pass.

### Task 2: Wire dependency and visual states

**Files:**

- Modify: `frontend/apps/coze-studio/package.json`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/index.less`
- Generated: `common/config/rush/pnpm-lock.yaml`

- [ ] Add the explicit Monaco workspace dependency and run `rush update`.
- [ ] Style the Monaco surface, focus, loading, and failure states.
- [ ] Run the adapter and AppDev IDE tests.

### Task 3: Browser acceptance

**Files:** none.

- [ ] Open an AppDev project with the Codex in-app browser.
- [ ] Verify TypeScript highlighting, editing, dirty state, and save shortcut.
- [ ] Switch files and confirm model state does not leak.
- [ ] Confirm no new page-level or console errors appear.
