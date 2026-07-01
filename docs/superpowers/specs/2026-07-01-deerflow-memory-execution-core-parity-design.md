# DeerFlow Memory And Execution Core Parity Design

## Goal

Make Coze's Go/Eino Agent runtime match DeerFlow's core runtime behavior for
memory and execution flow, not just the visible task-detail shell. P0 proved
that tasks can run and render, but P1 must align the model-visible context,
memory update lifecycle, planning loop, message contract, and frontend step
projection.

## Non-Goals

- Do not add Python sidecars.
- Do not redesign Skills/MCP UI unless required by the runtime contract.
- Do not treat checkpoint history as the visible-step source of truth.
- Do not expose prompt text, raw model completions, tool arguments/results,
  checkpoint bytes, credentials, object URIs, or raw provider payloads.

## DeerFlow Memory Contract

Verified source anchors:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/middlewares/dynamic_context_middleware.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/middlewares/memory_middleware.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/memory/message_processing.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/memory/prompt.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/memory/updater.py`

Behavior to reproduce:

- `DynamicContextMiddleware` injects memory and current date before the agent
  runs. It creates a hidden `HumanMessage` with `<system-reminder>`, optional
  `<memory>`, and `<current_date>`.
- First-turn injection rewrites the first user message into a hidden reminder
  message plus the original user message with a derived id. This preserves
  prefix-cache behavior and prevents the reminder from showing in the UI.
- Same-day follow-ups do not re-inject the reminder. Cross-day follow-ups inject
  a lightweight date-update reminder on the current user turn.
- Memory injection has bounded timeout/degrade behavior. A slow memory load
  must skip memory/date injection rather than hang the request.
- Memory formatting includes user context, history, and facts sorted by
  confidence, with a token budget.
- `MemoryMiddleware.after_agent` queues memory update only after successful
  agent completion.
- Memory update input keeps user messages and final assistant responses only.
  It excludes tool-call AI messages and strips `<uploaded_files>` blocks.
- Correction and reinforcement signals are detected from recent user messages
  and passed into the memory update prompt.
- Memory update is asynchronous and retry-safe. It loads current memory,
  prompts a memory model, applies structured updates, strips upload mentions,
  and saves through storage.

## Coze Memory Gap

Current source anchors:

- `backend/application/agentthread/adk_memory_middleware.go`
- `backend/application/agentthread/memory_provider.go`
- `backend/application/agentthread/adk_transcript.go`
- `backend/application/agentthread/memory_flush_processor.go`
- `backend/application/agentthread/memory_model_extractor.go`
- `backend/application/agentthread/worker.go`

Initial observed gaps:

- Before P1-I-003, Coze appended `<memory_context>` JSON lines to
  `ChatModelAgentContext.Instruction`. This is not DeerFlow's hidden
  `HumanMessage` / `<system-reminder>` contract.
- Coze's memory formatter is flat `scope/content` JSON. DeerFlow formats user
  context, history, and facts, with confidence-aware ranking and correction
  metadata.
- Coze has recall filtering and memory CRUD, but runtime injection timing is
  not aligned with first-turn, same-day, and cross-day behavior.
- Coze has transcript snapshots and memory flush jobs, but extraction is tied
  to summarization/transcript persistence and the worker is disabled by
  default. This is not yet equivalent to DeerFlow's after-agent queue.
- Coze extraction prompt is safety-oriented and bounded, but it does not yet
  mirror DeerFlow's correction/reinforcement hints and "user + final assistant"
  filtering contract.

2026-07-01 implementation note:

- `backend/application/agentthread/adk_memory_middleware.go` now injects memory
  through Eino `BeforeModelRewriteState` as a hidden `schema.UserMessage`, with
  `Extra.hide_from_ui=true` and `Extra.dynamic_context_reminder=true`.
- The injected content follows DeerFlow's outer contract:
  `<system-reminder>`, optional `<memory>`, and `<current_date>`.
- Same-day turns do not duplicate full reminders. Cross-day turns inject a
  date-only reminder before the current user message.
- Memory recall is bounded by a 5 second timeout and degrades by skipping the
  reminder injection for that model turn.
- Coze summarization now uses a custom finalizer that keeps dynamic context
  reminders before the generated summary, matching DeerFlow's reminder-preserve
  behavior.
- Coze summarization model input filters out dynamic context reminders before
  generating summaries, then restores the reminders ahead of the summary message.
  This matches DeerFlow's `_preserve_dynamic_context_reminders` behavior and
  avoids folding hidden memory/date reminders into summary text.
- Memory injection formatting now matches DeerFlow's visible prompt contract
  when safe section metadata is available: `User Context:` supports `Work`,
  `Personal`, and `Current Focus`; `History:` supports `Recent`, `Earlier`, and
  `Background`; `Facts:` remains confidence sorted and supports correction
  `(avoid: ...)` metadata.
- `ModelMemoryExtractor` now asks for DeerFlow's `user/history/newFacts`
  update shape and still accepts the legacy Coze `facts` shape. Parsed
  `user/history` summaries are bridged into normalized `AgentMemory` records
  using controlled `deerflow_section` metadata, so later injection can render
  DeerFlow-style memory sections without leaking raw model metadata.
- Memory flush processing now recalls current memories before model extraction
  and sends a safe DeerFlow-like `user/history/facts` JSON projection to the
  extractor. The projection omits source ids, raw metadata, private paths,
  credentials, object URIs, and provider payloads.
- Remaining gap: Coze still stores normalized records rather than a single
  DeerFlow memory document, and it does not yet apply `factsToRemove` as
  delete/update operations.

## Eino Landing Point

Verified Eino `v0.9.9` anchors:

- `/Users/liuwenbo/go/pkg/mod/github.com/cloudwego/eino@v0.9.9/adk/handler.go`
- `/Users/liuwenbo/go/pkg/mod/github.com/cloudwego/eino@v0.9.9/adk/chatmodel.go`

Implementation approach:

- Use `ChatModelAgentMiddleware.BeforeModelRewriteState` for DeerFlow-style
  hidden message injection because it can modify `TypedChatModelAgentState.Messages`
  and persist the change into agent state.
- Use `ChatModelAgentMiddleware.AfterAgent` for DeerFlow-style post-run memory
  queueing because it receives final successful conversation state.
- Keep `BeforeAgent` only for tool/instruction setup that DeerFlow also treats
  as static or pre-run configuration.
- Preserve existing Go storage, tenant isolation, and memory APIs. Do not add a
  parallel memory store.

## DeerFlow Execution Contract

Verified source anchors:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/lead_agent/agent.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/middlewares/todo_middleware.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/runtime/journal.py`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/messages/utils.ts`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-group.tsx`
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/threads/hooks.ts`

Behavior to reproduce:

- Lead-agent middleware order starts with shared runtime middlewares, then
  dynamic context, skill activation, summarization, To-dos, token usage, title,
  memory, vision, deferred tools, subagent limits, loop detection, safety finish,
  and clarification last.
- `TodoMiddleware` uses `write_todos`, injects hidden todo reminders when
  todos fall out of context, and prevents premature final answers when active
  todos remain. It caps completion reminders to avoid infinite loops.
- `RunJournal` captures user-visible message events from LangChain callbacks:
  `llm.human.input`, `llm.ai.response`, and `llm.tool.result` with
  `category=message`.
- `RunJournal` also accumulates token usage by caller bucket: lead agent,
  subagent, and middleware.
- Frontend history reads `GET /api/threads/:thread_id/runs/:run_id/messages`.
  Checkpoint history is not the visible step source.
- Frontend `getMessageGroups` filters hidden control messages, groups human,
  assistant processing, assistant final responses, present-files, clarification,
  and subagent messages.
- `convertToSteps` derives visible steps from reasoning and tool calls, joining
  tool results by tool call id.

## Coze Execution Gap

Current source anchors:

- `backend/application/agentthread/adk_middleware.go`
- `backend/application/agentthread/adk_turn_loop.go`
- `backend/application/agentthread/adk_event_mapper.go`
- `backend/application/agentthread/adk_plan_backend.go`
- `frontend/apps/coze-studio/src/pages/tasks/task-event-projection.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-event-display.ts`
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`

Observed gaps:

- Coze has an Eino ADK middleware chain, but ordering and semantics are not yet
  a DeerFlow-equivalent chain. Memory currently runs too early as instruction
  decoration, while DeerFlow places dynamic context before model calls and
  memory update after title generation.
- Coze has `plantask` and durable plan backend. It does not yet implement
  DeerFlow's incomplete-todo loop guard that forces continued model work while
  active tasks remain.
- Coze persists generic run events such as `message.completed`,
  `tool.completed`, `plan.task.*`, `memory.update_*`, and lifecycle events.
  DeerFlow's visible contract is message-category events, not generic lifecycle
  steps.
- Coze frontend projects assistant tool calls into DeerFlow-style steps, but
  this is a projection over Coze events. It needs a backend contract that is
  structurally equivalent to DeerFlow message groups, otherwise UI parity will
  keep drifting.
- Coze currently hides some internal lifecycle events in the flow, but the
  event stream still carries concepts that should be bounded metadata only.

## First Implementation Slices

### Slice 1: Runtime Memory Injection

Expected behavior:

- First model turn includes one hidden system-reminder-style message followed by
  the original user message.
- The reminder contains `<system-reminder>`, optional `<memory>`, and
  `<current_date>`.
- The reminder never renders in task detail or task history.
- A second same-day follow-up does not duplicate the full reminder.

Tests first:

- Backend unit test for `ADKMemoryMiddleware.BeforeModelRewriteState`.
- Backend unit test for bounded memory formatting and confidence/correction
  ordering.
- Frontend projection test that hidden control messages stay out of visible
  execution/detail surfaces if they are surfaced as events.

Implementation evidence:

- `go test ./application/agentthread -run 'TestADKMemoryMiddleware|TestADKExecutorPersistsSummarizationEventsWithBoundedMemory|TestADKSummarizedCheckpointRestartsWithoutOriginalHistoryOrDuplicateMemory|TestADKMiddleware' -count=1`
- `go test ./application/agentthread -count=1`

Residual follow-up:

- Browser-visible evidence is still needed after P1-I-004/P1-J-004 because the
  hidden reminder must remain invisible in task detail while still improving
  follow-up behavior.

### Slice 2: After-Agent Memory Queue

Expected behavior:

- Successful agent completion filters state to user messages plus final
  assistant response.
- Tool-call-only AI messages and uploaded-file-only user messages are not sent
  to memory extraction.
- Correction and reinforcement flags are recorded on the queued request.
- Existing memory flush processing remains tenant-scoped and retry-safe.

Tests first:

- Backend unit test for memory update filtering.
- Backend unit test for correction/reinforcement detection.
- Backend service test proving queue payload does not leak uploads, tool
  arguments, or raw provider payloads.

Implementation evidence:

- `backend/application/agentthread/adk_transcript.go` now queues memory only from
  terminal successful ADK state. `summary_input` snapshots remain transcript
  evidence and no longer enqueue memory updates.
- Terminal runs keep the full transcript snapshot, then build a sanitized
  memory-flush input snapshot when the memory input differs from the full
  transcript. The queued job points to the sanitized snapshot.
- The memory input keeps only real user text and final assistant messages. It
  skips hidden dynamic context reminders, Eino summarization control messages,
  assistant tool-call messages, tool results, and uploaded-file blocks; if a
  user turn is upload-only, the immediate assistant response is skipped.
- Correction/reinforcement signals mirror DeerFlow's recent-user-message regex
  behavior and are stored as bounded metadata under `memory_flush`. The metadata
  contains booleans and schema/purpose only, not user text.
- `backend/application/agentthread/memory_model_extractor.go` now instructs the
  memory extraction model to honor correction/reinforcement metadata.

Verification:

- `go test ./application/agentthread -run 'TestADKTranscript|TestModelMemoryExtractor' -count=1`
- `go test ./application/agentthread -count=1`

Residual follow-up:

- If Eino summarization has already collapsed all real user turns into a
  synthetic summary message, Coze skips that synthetic message for memory safety.
  This avoids writing summary boilerplate as user memory; browser/API follow-up
  recall evidence still belongs to `P1-I-005`.

### Slice 2.5: Follow-Up Recall Query

Verified Coze follow-up behavior:

- The canonical task-detail follow-up path appends the latest user message, then
  creates a new run whose `input.messages` contains prior real user/assistant
  turns plus the latest user message.
- `ThreadMemoryProvider` extracts the last user message from `run.Input` and
  uses it as the memory retrieval query.

Implementation evidence:

- `RecallMemoriesRequest` now carries `Query` from application service to
  domain service.
- `ThreadMemoryProvider` forwards the latest user query into
  `ApplicationService.RecallMemories`.
- `threadService.RecallMemories` trims and forwards the query into repository
  `ListMemories`, so candidate recall is query-aware before Coze's local
  relevance sorting.

Verification:

- `go test ./application/agentthread -run 'TestThreadMemoryProvider|TestADKMemoryMiddleware' -count=1`
- `go test ./domain/agentthread/service -run 'TestRecallMemoriesNormalizesLimitAndUsesRunContext|TestManageMemoriesNormalizesAndDelegates' -count=1`

Residual follow-up:

- Browser/API evidence still needs to prove a real follow-up answer uses the
  recalled memory and that memory update jobs remain hidden/safe.

### Slice 3: Execution Message Contract

Expected behavior:

- Backend emits or exposes a DeerFlow-equivalent visible message stream for a
  run: human input, AI reasoning/tool calls, tool results, present-files, final
  assistant answer.
- Frontend task-detail steps are derived from that stream rather than
  lifecycle/stub events.
- Existing `ListTaskThreadRunEvents` remains available for Coze audit/runtime
  metadata, but UI-visible execution flow uses the DeerFlow-equivalent contract.

Tests first:

- Backend mapper test for Eino ADK message output to DeerFlow-style run message.
- Backend API test for listing visible run messages without hidden control
  messages.
- Frontend unit test mirroring DeerFlow `getMessageGroups` cases.

2026-07-01 implementation note:

- `backend/application/agentthread/run_journal_messages.go` adds the internal
  DeerFlow-style journal projection:
  `ProjectRunJournalMessages(run, persistedMessages, events)`.
- The projection emits `human`, `ai`, and `tool` messages with the fields
  DeerFlow's frontend depends on: AI `additional_kwargs.reasoning_content`,
  `tool_calls` with parsed arguments, tool `name`, `tool_call_id`, and final
  assistant content.
- Current-run user text comes from the persisted user message for that run. If
  it is missing, Coze falls back to the latest user/human message in
  `run.input.messages`; this avoids replaying the whole follow-up history as
  the current run's visible input.
- Pure final-answer `message.completed` events are de-duplicated against
  persisted assistant messages. The visible final answer remains the transcript
  assistant message, while reasoning/tool-call AI events remain available for
  steps.
- `ProjectThreadRunJournalMessages` groups multiple runs for the task-detail
  thread endpoint and sorts by created time with user -> event -> final assistant
  tie-breaking for same-millisecond writes.
- `ListTaskThreadRunEvents` now returns optional `journal_messages` in addition
  to existing `events`. The handler builds journal messages from the same
  already-sanitized event payloads returned to the browser, so raw tool results,
  credentials, object URLs, provider raw bodies, and token raw usage are not
  newly exposed.
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts` prefers
  `journal_messages` for message/tool-backed steps and keeps non-message events
  such as plan, todo, run lifecycle, and safe metadata events from the original
  event list.

Verification:

- `go test ./application/agentthread -run TestProjectRunJournalMessages -count=1`
- `go test ./api/handler/coze -run 'TestListTaskThreadRunEventsHandlerReturnsJournalMessages|TestTaskThreadRunEventPayloadKeepsSafe|TestListTaskThreadRunEventsHandlerRedactsUnsafePayload' -count=1 -gcflags="all=-N -l"`
- `npm run test -- src/pages/tasks/__tests__/task-detail-loader.test.ts`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`
- `npx tsc --noEmit --project tsconfig.json`

Residual follow-up:

- Browser/API evidence still needs to compare Coze `journal_messages` with
  DeerFlow `GET /api/threads/:thread_id/runs/:run_id/messages` for the canonical
  document, Mermaid, search, and follow-up tasks.
- Coze UI still needs the final ChainOfThought visual/style alignment recorded
  under `P1-J-008`; this slice only moves the data source away from frontend
  guesswork.

### Slice 4: To-dos Loop Control

Verified DeerFlow behavior:

- `TodoMiddleware.after_model` detects a clean final assistant answer while
  active To-dos still exist, queues a hidden `<system_reminder>`, and jumps back
  to the model instead of letting the run finish.
- `TodoMiddleware.wrap_model_call` injects the queued hidden reminder into the
  next model call as a `HumanMessage` with `hide_from_ui=True`.
- The completion reminder is capped at 2 attempts to avoid infinite loops.

Eino/Coze implementation:

- Eino `plantask` supplies create/get/update/list tools and state, but it does
  not provide DeerFlow's incomplete-To-dos final-answer guard.
- Coze composes `plantask` with
  `backend/application/agentthread/adk_plan_completion_guard.go`.
- The guard adds a reserved internal tool,
  `coze_plan_completion_guard`, only to the executable tool set. It is removed
  from model-visible `ToolInfos` and `DeferredToolInfos` before each model call.
- `WrapModel` inspects model `Generate` and `Stream` outputs before Eino emits
  response events. When active plan tasks remain and the assistant has no tool
  calls, it rewrites the message to call the reserved internal tool with a
  DeerFlow-style hidden `<system_reminder>`.
- The reserved tool returns that reminder as a tool result, causing the next
  model turn to continue work. After 2 reminders, Coze allows the final answer.
- `adk_event_mapper.go` maps reserved guard assistant tool calls and tool
  results to hidden `agent.control` payloads:
  `{"hidden":true,"schema":"coze.plan_completion_guard.v1"}`. The UI must not
  render these as visible steps or chat messages.

Completed tests:

- `TestADKPlanCompletionGuardContinuesWhenTasksAreIncomplete`
- `TestADKPlanCompletionGuardAllowsFinalWhenTasksAreComplete`
- `TestADKPlanCompletionGuardAllowsFinalAfterReminderCap`
- `TestMapADKEventHidesPlanCompletionGuardMessages`
- `TestADKMiddlewareExposesPlanToolsOnlyWithCozeBackend`

Remaining browser parity work:

- The To-dos dock still needs paired DeerFlow/Coze browser evidence for exact
  visible timing and collapsed/expanded states. Track this under
  `P1-J-007`/`P1-J-008`, not under the backend loop guard.

## Verification

For each slice:

1. Run targeted Go tests under `backend/application/agentthread`.
2. Run targeted task-detail Vitest cases.
3. Run one paired browser task in DeerFlow and Coze using the same prompt.
4. Record evidence in `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/`.
5. Update `docs/superpowers/plans/2026-07-01-deerflow-p1-stabilization-tracker.md`.

## Acceptance

This core work is complete only when Coze can run the same multi-turn tasks as
DeerFlow and match these user-visible outcomes:

- Memory-aware follow-up answers use prior thread/long-term context.
- Execution steps show real reasoning/tool/present-files flow, not fixed
  lifecycle placeholders.
- To-dos prevent premature completion in complex tasks.
- Artifacts appear in the same relative order and side preview flow.
- Token rows and top-bar token display remain consistent with the message
  stream.
