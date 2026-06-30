# TD-SKILL-004 Composer Skill Activation

## DeerFlow Source Confirmed

- `frontend/src/components/workspace/input-box.tsx`
  - Leading `/` opens `aria-label="Skill suggestions"`.
  - Suggestions are enabled skills only, filtered by skill name, sorted with
    prefix matches first, capped at 6.
  - Selecting a skill writes `/${skill.name} ` into the composer.
  - `ArrowUp`, `ArrowDown`, `Enter`, `Tab`, and `Escape` control the menu.
- `backend/packages/harness/deerflow/skills/slash.py`
  - Slash activation uses strict lower-kebab syntax:
    `^/([a-z0-9]+(?:-[a-z0-9]+)*)(?:\s+|$)`.
  - Reserved control commands are ignored: `bootstrap`, `help`, `memory`,
    `models`, `new`, and `status`.
- `backend/packages/harness/deerflow/agents/middlewares/skill_activation_middleware.py`
  - Runtime activation resolves the slash skill against installed, enabled, and
    agent-visible skills.
  - DeerFlow injects hidden model context for the selected skill and keeps the
    visible user message unchanged.

## Coze Changes

- `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
  - Adds DeerFlow-style slash skill suggestions for enabled Workbench skills.
  - Uses the same leading-slash query semantics and keyboard behavior.
  - Selecting a suggestion inserts `/${skill.name} ` into the composer.
- `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer-controls.tsx`
  - Renders the slash suggestion listbox above the composer textarea.
  - Suggestion placement now follows the same `variant` rule as DeerFlow-style
    mode/resource popovers: home composers open downward, task-detail
    follow-up composers open upward.
- `frontend/apps/coze-studio/src/pages/workbench/index.less`
  - Adds scoped suggestion styles for the Workbench composer.
- `backend/application/agentthread/skill_provider.go`
  - Parses the latest user message from the run input for DeerFlow-compatible
    slash skill syntax.
  - Treats slash activation as a turn-scoped selector when no explicit
    `enable_skills` allow-list is present.
  - When an explicit allow-list is present, slash activation narrows the run to
    the selected skill but cannot broaden allowed skills. Existing Coze
    selector support for skill ID remains preserved.

## Verification

- Frontend RED/GREEN:
  - `cd frontend/apps/coze-studio && rushx test -- src/pages/workbench/__tests__/workbench.test.tsx`
  - Result after implementation and payload assertion: 13 files passed, 122
    tests passed.
  - Covered behavior:
    - `/wee` opens the DeerFlow-style skill suggestion list.
    - Selecting `/weekly-research` writes `/weekly-research ` into the
      composer.
    - Sending `/weekly-research collect market changes` preserves the slash
      command in `CreateTaskThread.message`.
    - Slash selection does not secretly populate `enable_skills`; runtime
      activation remains message-driven, matching DeerFlow's slash activation
      model.
- Backend RED/GREEN:
  - `cd backend && go test ./application/agentthread -run TestRuntimeSkillProvider -count=1`
  - Result after implementation: pass.
- Dev build check:
  - Restarted the local frontend dev server on `http://localhost:8080/`.
  - Verified the in-memory Workbench route chunk
    `/static/js/async/src_pages_workbench_index_tsx.js` contains
    `Skill suggestions` and `.chat-workbench-skill-suggestions`.
- Browser verification on 2026-06-29:
  - New task page:
    `http://localhost:8080/space/7656275718757679104/chats/new`.
    Typing `/skill` opens the Skill suggestions list with
    `data-placement="bottom"`, rect `y=237`, `bottom=403` in a `994px` high
    viewport, so the menu is no longer clipped above the page.
    Screenshot:
    `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-SKILL-004-home-slash-suggestions.png`.
  - Task-detail follow-up:
    `http://localhost:8080/space/7656275718757679104/tasks/7656685105288577024`.
    Typing `/skill` opens the Skill suggestions list with
    `data-placement="top"`, rect `y=655`, `bottom=821` in the same viewport.
    Screenshot:
    `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-SKILL-004-detail-slash-suggestions.png`.
- Task-detail coverage:
  - `frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx`
    reuses the same `WorkbenchComposer` with `variant="detail"`.
  - Canonical follow-up run input includes the latest user message through
    `getThreadFollowUpRunInput(...)`, so `/skill-name ...` is parsed by the
    same backend runtime provider path.

## Remaining Under TD-SKILL-004

- Capture create/follow-up run request payloads to confirm selected skill names
  flow into `enable_skills` or slash text consistently.
- Coze currently injects skill instructions through the existing system-prompt
  skill provider path. DeerFlow's exact hidden HumanMessage middleware shape is
  not yet duplicated; keep that as a separate follow-up only if visible
  behavior or runtime correctness requires it.
