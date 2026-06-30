# TD-SKILL-005 Runtime Skill Loading

## DeerFlow Source Confirmed

- `README_zh.md`
  - States that Skills use progressive on-demand loading and are not all put
    into the context at once.
- `backend/packages/harness/deerflow/agents/lead_agent/prompt.py`
  - The system prompt lists enabled skills as metadata only: name,
    description, and container file path.
  - The prompt tells the model to read the matching skill file only when a
    skill applies.
- `backend/packages/harness/deerflow/agents/middlewares/skill_activation_middleware.py`
  - Finds the latest visible user message.
  - Resolves strict `/skill-name` activation against installed, enabled, and
    agent-visible skills.
  - Injects model-only context that states the user explicitly activated the
    skill and provides the remaining task text inside `<user_request>`.
- `backend/packages/harness/deerflow/skills/slash.py`
  - Slash skill names use lower-kebab syntax.
  - Reserved control commands are ignored.

## Coze Changes

- `backend/application/agentthread/skill_provider.go`
  - Runtime provider still loads only enabled prompt skills.
  - Explicit `enable_skills` continues to support both skill name and skill ID.
  - Slash activation narrows the runtime skill set to the slash-selected skill.
  - `runWithSkillPrompt` now appends a model-only `## Slash Skill Activation`
    section when the selected loaded skill matches the latest user slash
    command.
  - The section preserves the DeerFlow intent:
    - `The user explicitly activated the \`skill-name\` skill for this turn.`
    - `<user_request>` contains the slash command's remaining task text.
    - The model is told to follow the skill before choosing a general workflow.
- `backend/application/agentthread/adk_middleware.go`
  - ADK selected-skill middleware now emits a metadata-only
    `skills.loaded` run event after the Skill middleware is assembled.
  - Payload is bounded to `skill_count`, `skill_ids`, and `skill_names`.
  - Skill body, resource files, prompts, tool arguments/results, provider raw
    payloads, object URIs, URLs, filenames, and credentials are not emitted.
- `backend/application/agentthread/adk_skill_preload_middleware.go`
  - Single inline skills explicitly activated by `/skill-name` now preload
    the full Skill instructions before the first ADK model call.
  - This mirrors DeerFlow's explicit slash activation semantics and avoids
    relying on the model to first call the generic Skill loading tool.
  - Explicit `enable_skills` single-skill selection keeps the same preload
    behavior; multi-skill progressive loading still uses the Eino Skill
    middleware.
- `frontend/apps/coze-studio/src/pages/tasks/task-event-display.ts`
  - `skills.loaded` is rendered as DeerFlow-style available catalog state, for
    example `可用技能目录 22 个` or `可用技能 “skill-creator”`, instead of
    implying all skill contents were loaded.
  - Actual skill execution remains represented by Skill tool-call events, for
    example `使用 “skill-creator” 技能` or `“skill-creator” 技能加载失败`.
- `frontend/apps/coze-studio/src/pages/tasks/task-event-projection.ts` and
  `task-event-tool-display.ts`
  - Assistant `tool_calls` that are matched by a `tool.failed` result now stay
    failed in the projected execution flow.
  - Skill tool failures render the selected Skill name, for example
    `“skill-creator” 技能加载失败`, while keeping the raw error detail hidden.

## Verification

- Backend RED/GREEN:
  - Added `TestModelStepRunnerMarksSlashActivatedSkillInSystemPrompt`.
  - RED failed because the prior prompt only contained generic
    `## Enabled Skills`.
  - GREEN passed after adding slash activation prompt semantics.
- Command:
  - `cd backend && go test ./application/agentthread -run 'TestModelStepRunner(MarksSlashActivatedSkillInSystemPrompt|InjectsSkillInstructionsIntoSystemPrompt)|TestRuntimeSkillProvider' -count=1`
  - Result: pass.
- Backend selected-skill event RED/GREEN:
  - Added `TestADKSkillMiddlewareEmitsLoadedEventWithoutSkillContent`.
  - RED failed because ADK selected Skills loaded through middleware but did
    not emit a `skills.loaded` event.
  - GREEN passed after adding the shared metadata-only event helper.
- Backend ADK slash preload RED/GREEN:
  - Added `TestADKSkillMiddlewarePreloadsSlashActivatedInlineSkill`.
  - RED failed because `/skill-creator ...` reached the first ADK model call
    without the skill body preloaded.
  - GREEN passed after letting the selected-skill middleware detect
    `runtimeMatchedSlashSkillActivation`.
- Frontend event display RED/GREEN:
  - Added `renders skill catalog events as available-skill directory steps`.
  - RED showed `加载 22 个技能`, which incorrectly implied all skill contents
    were loaded.
  - GREEN renders `可用技能目录 22 个`.
  - Added `renders single skill catalog events without implying full content
    preload`.
  - RED showed `加载 “skill-creator” 技能`.
  - GREEN renders `可用技能 “skill-creator”`.
  - Added `keeps skill tool-call failures visible with selected skill names`.
  - RED showed a failed Skill tool call as successful `使用 “skill-creator” 技能`.
  - GREEN keeps failed status and renders `“skill-creator” 技能加载失败`.
- Live local HTTP/API smoke:
  - Login succeeded through local passport API and created a new Workbench task
    using selected Skill `skill-creator`.
  - New thread/run:
    - `thread_id=7656724464578592768`
    - `run_id=7656724465300013056`
    - `status=succeeded`
  - `agent_run_events` contains:
    - `run.started`
    - `skills.loaded` with `skill_count=1`,
      `skill_ids=["7656639981540081664"]`,
      `skill_names=["skill-creator"]`
    - `message.completed`
    - `context.transcript_persisted`
    - `memory.update_queued`
    - `run.completed`
- Browser visual verification on 2026-06-29:
  - Opened
    `http://localhost:8080/space/7656275718757679104/tasks/7656685105288577024`.
  - The recorded run is a guardrail-blocked Skill load scenario. Task detail
    renders failed Skill tool calls as `“skill-creator” 技能加载失败`, keeps
    the failure state visible, and shows `工具调用失败，详情已隐藏`.
  - The page text check confirmed no raw `skills.loaded` or `skill_count`
    payload is visible.
  - Screenshot:
    `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-SKILL-005-runtime-skill-failure-step.png`.
- Commands:
  - `cd backend && go test -gcflags="all=-N -l" ./application/agentthread -run TestADKSkillMiddlewareEmitsLoadedEventWithoutSkillContent -count=1`
  - `cd backend && go test -gcflags="all=-N -l" ./application/agentthread -run 'TestADKSkill|TestRuntimeSkillProvider|TestModelStepRunner(MarksSlashActivatedSkillInSystemPrompt|InjectsSkillInstructionsIntoSystemPrompt)' -count=1`
  - `cd backend && go test ./application/agentthread -run 'TestRuntimeSkillProvider|TestADKSkillMiddleware|TestModelStepRunner.*Skill|TestHarnessExecutorLoadsEnabledSkillsBeforePlanning' -count=1`
  - `cd backend && go test -gcflags="all=-N -l" ./application/agentthread -run 'TestHarnessExecutor(LoadsSkills|RunsPlan|RecordsStepUsage|LoadsMemory)|TestNewApplicationHarnessExecutorConfiguresDurableSinks' -count=1`
  - `cd backend && go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestTaskThreadRunEventPayloadKeepsSafeSkillToolCallName|TestListTaskThreadRunEventsHandlerRedactsUnsafePayload' -count=1`
  - `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx`
  - Result: pass.
- Current 2026-06-30 update:
  - `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx -t "skill catalog"`
  - `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx`
  - Result: pass.

## Remaining Under TD-SKILL-005

- Slash-triggered browser/network verification remains pending separately
  under TD-SKILL-004.
- In-app browser control timed out while selecting the current tab after a
  reload attempt, so this update is verified by source comparison and frontend
  unit tests only. Re-run browser DOM/screenshot verification when the browser
  control channel is healthy.
- Coze still uses the existing system-prompt injection path rather than
  DeerFlow's exact hidden `HumanMessage` middleware object shape. This keeps
  the implementation Eino/Go-native while matching the model-visible runtime
  semantics needed for P0.
