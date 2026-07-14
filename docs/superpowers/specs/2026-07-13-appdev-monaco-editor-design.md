# AppDev Monaco Editor Design

## Goal

Replace the AppDev plain textarea with the repository's Monaco editor package
without changing the existing file, workspace, permission, or persistence
contracts.

## Scope

- Render the selected project file with `@coze-arch/bot-monaco-editor`.
- Preserve the current `CodeEditor` component API so the IDE orchestrator does
  not need to change.
- Keep explicit save behavior and the existing version-based optimistic
  concurrency contract in `useAppDevFiles`.
- Provide language selection, view-state retention, editor loading and failure
  states, and a real `Ctrl/Cmd + S` Monaco action.
- Keep the editor read-only while a save request is in flight so an accepted
  response cannot overwrite newer local input.

## Architecture

`CodeEditor` remains an AppDev adapter. It maps `path`, `value`, and save state
to the shared Monaco package, while file loading and persistence remain owned by
`useAppDevFiles`. The adapter does not call HTTP services and does not maintain
a second copy of file state.

The Monaco model uses the project-relative path as its identity. Language mode
comes from the existing `getLanguageFromPath` utility. The shared package owns
Monaco worker loading, so AppDev does not add Nuwax's `/vs` asset convention or
global promise rejection handlers.

## Error and state behavior

- File loading continues to use the existing AppDev loading state.
- Monaco initialization displays a bounded loading state.
- A lazy-load or render failure is contained inside the editor surface and does
  not crash the AppDev page.
- Dirty, saved, saving, line-count, and character-count states stay visible in
  the existing header and footer.
- The save command reads current props through a ref, preventing stale Monaco
  command closures from issuing duplicate saves.

## Non-goals

- Git source control APIs and UI.
- Interactive terminal or WebSocket shell support.
- Replacing Coze's version-aware save contract with Nuwax auto-save.
- Copying Nuwax's Ant Design, Umi, static Monaco worker, or global error code.

## Acceptance criteria

- A selected TypeScript file opens in Monaco with TypeScript language mode.
- Editor changes continue to update the existing AppDev draft.
- The save button and `Ctrl/Cmd + S` invoke the same save callback.
- Save commands are disabled while loading or saving.
- Monaco loading and failure remain contained inside the editor panel.
- Existing file APIs and backend code are unchanged.
