# DeerFlow P1 Stabilization Tracker

> P1 starts after the DeerFlow P0 launch cutline is merged into `dev`.
> Keep this tracker focused on parity stabilization and evidence, not broad
> platform enhancements. P2 items remain parked unless they fix a direct P1
> regression.

## Goal

P1 turns the P0 DeerFlow parity implementation into a durable verification and
hardening baseline. The priority is to preserve the shipped task-detail,
Skill, MCP, memory, token, artifact, and web-search behavior while adding the
missing evidence, automated browser coverage, and narrow hardening that P0
explicitly deferred.

2026-07-01 scope correction: the memory system and Agent execution flow are
core DeerFlow capabilities, not cosmetic polish. P0 completed the management
surface and a runnable Go/Eino path, but it does **not** yet mean Coze fully
matches DeerFlow's runtime memory injection/update semantics, middleware chain,
To-dos loop control, RunJournal/message contract, or visible execution
quality. P1 must prioritize these two capabilities before broad hardening.

## Scope Rules

- Mainline remains DeerFlow-visible behavior first. If a user cannot observe
  the difference in the DeerFlow-equivalent workflow, record it as P2.
- Task vocabulary stays `新建任务`、`全部任务`、`我的任务`、`任务详情`、
  `任务记忆`.
- No IM Channels.
- No Python sidecar.
- No complex security scanner/quarantine UI in P1 unless it is required to
  keep an existing P0 feature safe.
- Every task-detail change must reference a case id from
  `docs/superpowers/plans/2026-06-28-deerflow-task-detail-parity-validation.md`.
- Do not treat task-memory CRUD or safe event projection as full DeerFlow
  parity. Runtime memory and execution-flow parity require source/runtime/API
  evidence from DeerFlow plus matching Coze behavior.

## Status Rules

| Status | Meaning |
| --- | --- |
| 待开始 | Accepted P1 work, not started. |
| 进行中 | Current active slice; document expected verification before code changes. |
| 待验收 | Code/doc changes exist, but evidence or tests still need rerun. |
| 已完成 | Implementation, tests, and evidence are recorded. |
| 延后(P2) | Useful hardening, not required for P1 parity stabilization. |

## P1 Workstreams

| Workstream | Status | Acceptance | Notes |
| --- | --- | --- | --- |
| P1-I 记忆系统运行时对齐 | 已完成 | Coze matches DeerFlow memory injection/update behavior for normal task runs, follow-ups, and long-term recall without exposing unsafe payloads. | 2026-07-01 completed source/runtime/UI/API-effect evidence for hidden memory injection, terminal async update, follow-up recall, durable memory flush, and memory management smoke. Direct JSON API capture was blocked by browser security, so P1-I-006 records real UI API effects plus DB audit summaries. |
| P1-J Agent 执行流程对齐 | 已完成 | Coze task runs follow DeerFlow's agent harness semantics: planning/To-dos, step grouping, tool/artifact messages, follow-up context, streaming status, stop/retry, and visible execution quality. | Backend/runtime and visible task-detail parity slices are complete for P1. Remaining browser evidence and regression hardening continue under P1-A/P1-B. |
| P1-A 浏览器回归与证据基线 | 已完成 | DeerFlow and Coze have paired desktop evidence for the canonical P0 workflows, plus safe Network/API summaries where applicable. | 2026-07-01 completed P1-A-000~008. Authenticated Network/API capture is explicitly N/A for P1-A-001~003 inside the current in-app browser read-only scope; no inferred payloads were recorded. Any future black-box API packet capture moves to P1-D or external QA evidence. |
| P1-B 任务详情视觉补证 | 进行中 | Task detail screenshots/API summaries cover layout, execution steps, inline thinking, Mermaid, Artifacts, export, token modes, memory entry, loading/error states, and narrow/mobile risk cases. | 2026-07-01 evidence map created at `notes/P1-B-task-detail-visual-evidence-map.md`; existing desktop evidence covers most modules. First `TD-LAYOUT-002` responsive capture attempt was blocked by browser tab binding timeout, so next target remains 390px/768px screenshots and any overlap fix. |
| P1-C 文档产物 MIME 样本库 | 已完成 | Markdown/TXT/CSV/PDF/image/HTML-SVG blocked/error/delete-restore cases each have UI evidence, API summaries, and unit/API tests. | 2026-07-01 automated source/test coverage map recorded in `notes/P1-C-artifact-mime-sample-map.md`; helper, task-detail, API handler, and application scan-policy tests passed. Fixture task `7657556998191316992` covers Markdown/TXT/JSON/CSV/PDF/PNG/HTML/SVG cards; Go ADK `write_file` supports binary `content_base64`; Markdown/TXT/JSON/CSV/PNG/HTML-SVG browser screenshots are recorded. PDF preview is aligned to DeerFlow's artifact-detail iframe by removing Coze's empty PDF iframe `sandbox`; focused task-detail test verified red/green, browser DOM verified `sandbox=null`, and the freshly fetched iframe PDF rendered visible page pixels in `screenshots/coze/P1-C-coze-pdf-page-pixels-rendered-7657556998191316992.png`. SVG delete/restore lifecycle, SVG scan review, scan retry, bounded blocked preview error, copy/download affordances, API summaries, and stale-preview cleanup regressions are all recorded with targeted tests. No raw object URI, signed URL, provider payload, or scanner raw body in evidence. Already-open preview URL renewal after signed URL TTL expiry is tracked as P2 hardening, not a P1-C blocker. |
| P1-D LangGraph 兼容套件 | 待开始 | Black-box tests cover thread/run/history/state/stream/cancel/join behavior beyond DeerFlow-visible P0 paths. | Do not treat LangGraph checkpoint history as the only visible step source. Visible steps still follow the DeerFlow messages/history chain verified in P0. |
| P1-E Runtime Doctor 深诊断 | 待开始 | Adds live model connectivity, provider capability matrix, sandbox diagnostics, and richer Skill/MCP probes with bounded metadata. | Must fail closed and avoid prompt, credential, tool arg/result, checkpoint, raw provider, and object URI leaks. |
| P1-F Skills/MCP/Tools 策略与历史 | 待开始 | Adds policy controls, invocation history, output budgets, and deeper transport management where DeerFlow-visible workflows need it. | User has accepted P0 Skills/MCP as good enough; this is hardening, not re-litigating P0 UI. |
| P1-G Token 成本与调试视图 | 待开始 | Adds pricing-ready cost snapshots only when backend has safe positive cost and a single currency, plus provider/model breakdowns. | Keep per-turn and top-bar behavior stable. |
| P1-H Web Search Provider Hardening | 待开始 | Documents and tests provider selection/fallback for DuckDuckGo v2, Wikipedia, Brave HTML, and configured HTTP search. | Search is disabled by default unless policy/env enables it. |

## P1-I Checklist: 记忆系统运行时对齐

| Case | Status | Required Evidence |
| --- | --- | --- |
| P1-I-001 DeerFlow memory source map | 已完成 | Source anchors and behavior contract recorded in `docs/superpowers/specs/2026-07-01-deerflow-memory-execution-core-parity-design.md`. Verified `DynamicContextMiddleware`, `MemoryMiddleware`, `message_processing.py`, `agents/memory/prompt.py`, and `agents/memory/updater.py`. |
| P1-I-002 Coze memory runtime gap map | 已完成 | Coze anchors and initial gaps recorded in `docs/superpowers/specs/2026-07-01-deerflow-memory-execution-core-parity-design.md`: before P1-I-003, `ADKMemoryMiddleware` injected `<memory_context>` into instruction; extraction remains transcript/flush-worker based and is not yet DeerFlow after-agent queue parity. |
| P1-I-003 Prompt injection contract | 已完成 | Implemented in `backend/application/agentthread/adk_memory_middleware.go`: Eino `BeforeModelRewriteState` injects a hidden user `<system-reminder>` with optional `<memory>` and `<current_date>`, skips same-day duplicates, adds date-only cross-day reminders, degrades on recall timeout, and preserves reminders through summarization finalization. 2026-07-01: memory formatter now matches DeerFlow `User Context` / `History` / `Facts` sections from safe metadata, preserves confidence-sorted facts, and supports correction `(avoid: ...)` without leaking raw metadata. Tests: `go test ./application/agentthread -run 'TestADKMemoryMiddleware|TestADKExecutorPersistsSummarizationEventsWithBoundedMemory|TestADKSummarizedCheckpointRestartsWithoutOriginalHistoryOrDuplicateMemory|TestADKMiddleware' -count=1`; `go test ./application/agentthread -count=1`. |
| P1-I-004 Async memory update contract | 已完成 | Implemented DeerFlow-style terminal-only memory queueing in `backend/application/agentthread/adk_transcript.go`: `summary_input` no longer queues memory; terminal runs build a sanitized memory input from real user messages plus final assistant responses only; hidden dynamic reminders, Eino summarization control messages, tool calls/results, and `<uploaded_files>` blocks are excluded; upload-only turns skip the immediate assistant response; correction/reinforcement signals are captured in safe snapshot metadata. 2026-07-01: `ModelMemoryExtractor` now prompts/parses DeerFlow `user/history/newFacts` output, receives a safe current-memory JSON projection, maps section summaries to safe `deerflow_section` memory metadata, and applies `factsToRemove` via safe current-memory ids before writing replacement memories while preserving legacy `facts` compatibility. Tests: `go test ./application/agentthread -run 'TestApplicationProcessMemoryFlushJobs|TestModelMemoryExtractor|TestADKMemoryMiddleware|TestThreadMemoryProvider|TestADKTranscript' -count=1`; `go test ./application/agentthread -count=1`. |
| P1-I-005 Follow-up recall behavior | 已完成 | Backend recall query propagation completed: canonical follow-up run input already carries prior user/assistant turns plus latest user message; `ThreadMemoryProvider` forwards the latest user query through application/domain `RecallMemoriesRequest` into repository memory search before local relevance sorting. 2026-07-01 browser evidence: Coze task `7657470660221861888` preserved prior turns and answered the follow-up with `青鸢计划`; fresh task `7657481311426183168` verified async memory flush from application startup wrote 1 `long_term` memory and emitted `memory.update_completed`. Evidence: `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-I-005-follow-up-memory-runtime.md`; screenshots: `screenshots/coze/P1-I-005-follow-up-recall.png`, `screenshots/coze/P1-I-005-memory-flush-success.png`. Tests: `go test ./application/agentthread -run 'TestMemoryFlushWorker|TestApplicationProcessMemoryFlushJobs|TestModelMemoryExtractor|TestThreadMemoryProvider|TestADKMemoryMiddleware' -count=1`; `go test ./domain/agentthread/service -run 'TestRecallMemoriesNormalizesLimitAndUsesRunContext|TestManageMemoriesNormalizesAndDelegates' -count=1`. |
| P1-I-006 Memory UI/API smoke | 已完成 | Browser and backend evidence show memory management and runtime recall/update agree: task `7657481311426183168` listed the runtime-written memory, edited it, deleted it, viewed it under `已删除`, and restored it; DB audit recorded `memory.updated`, `memory.deleted`, and `memory.restored`; active memory metadata keys stayed bounded to `category`, `unsafe_memory_key_hits=0`. Evidence: `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-I-006-memory-ui-api-smoke.md`; screenshots: `screenshots/coze/P1-I-006-memory-panel-list.png`, `screenshots/coze/P1-I-006-memory-deleted.png`, `screenshots/coze/P1-I-006-memory-restored.png`. Tests: `go test ./api/handler/coze -run 'TestExportTaskThreadMemoriesHandlerReturnsSchemaPayload|TestImportTaskThreadMemoriesHandlerCreatesMemoriesAndAuditsActor|TestListTaskThreadMemoriesHandlerPassesSessionViewerID|TestListTaskThreadMemoriesHandlerMapsAuthorizationDeniedToForbidden' -count=1 -gcflags="all=-N -l"`; `npm run test -- src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/tasks/__tests__/task-memory-section-icons.test.tsx`. |

## P1-J Checklist: Agent 执行流程对齐

| Case | Status | Required Evidence |
| --- | --- | --- |
| P1-J-001 DeerFlow execution source map | 已完成 | Source anchors and behavior contract recorded in `docs/superpowers/specs/2026-07-01-deerflow-memory-execution-core-parity-design.md`. Verified `build_middlewares`, `TodoMiddleware`, `RunJournal`, `GET /api/threads/:thread_id/runs/:run_id/messages`, `getMessageGroups`, and `convertToSteps`. |
| P1-J-002 Coze execution gap map | 已完成 | Coze anchors and gaps recorded in `docs/superpowers/specs/2026-07-01-deerflow-memory-execution-core-parity-design.md`: current ADK event projection exists but is not yet a DeerFlow-equivalent visible message contract. |
| P1-J-003 Middleware parity | 已完成 | Dynamic context + summarization reminder preservation completed via P1-I-003. 2026-07-01: DeerFlow title middleware source chain was verified; Coze now generates first-exchange short task titles with the task model, strips `<think>` blocks, wrapper quotes, and trailing punctuation before updating task metadata, and falls back to the existing prompt-derived title if title generation fails. 2026-07-01: DeerFlow `SafetyFinishReasonMiddleware` and detector source were verified; Coze now suppresses unsafe assistant tool calls on provider safety termination before semantic-loop/policy handling, stamps bounded `safety_termination` metadata, persists a `middleware:safety_termination` audit event without tool arguments, and expands safety finish classification for Anthropic/Gemini signals. 2026-07-01: DeerFlow `ToolErrorHandlingMiddleware` source was verified; Coze tool-error payloads now include the DeerFlow-style model-visible recovery message and cap details at 500 chars while keeping structured `coze.tool_error.v1` decoding. 2026-07-01: DeerFlow `LoopDetectionMiddleware` source was verified; Coze now defaults to DeerFlow-style repeated tool-call warn/hard thresholds, queues a `loop_warning` user message before the next model call, and strips repeated assistant tool calls at hard stop so the model can finalize instead of surfacing a raw loop error. 2026-07-01: DeerFlow `ClarificationMiddleware` and `ask_clarification` tool source were verified; Coze keeps `ask_user_clarification` and adds DeerFlow-compatible `ask_clarification` with `clarification_type/context/options` parameters mapped into the existing interrupt/resume prompt contract. Tests cover title generation, safe-finish tool suppression, detector coverage, middleware order, tool-error recovery payloads, loop warning/hard-stop behavior, and DeerFlow clarification alias/tool-provider wiring. Verification: `go test ./application/agentthread -count=1`. |
| P1-J-004 To-dos and incomplete-work loop | 已完成 | Implemented in `backend/application/agentthread/adk_plan_completion_guard.go`: Coze composes Eino `plantask` with a DeerFlow-style incomplete-work guard. If active plan items remain and the model attempts a clean final answer, `WrapModel` rewrites it into a hidden internal completion-reminder tool call before the ADK event sender observes it, forcing another model turn; reminders are capped at 2. The reserved tool is removed from model-visible `ToolInfos` and hidden guard events are mapped to bounded `agent.control` metadata. Tests: `go test ./application/agentthread -run 'TestADKPlanCompletionGuard|TestMapADKEvent|TestADKMiddleware' -count=1`; `go test ./application/agentthread -count=1`. Frontend To-dos dock parity remains tracked under P1-J-007/P1-J-008 browser acceptance. |
| P1-J-005 RunJournal/message contract | 已完成 | Backend/API/loader baseline implemented and validated: `ProjectRunJournalMessages` projects persisted user/final assistant messages plus sanitized `message.completed`/`tool.*` run events into DeerFlow-style `human/ai/tool` journal messages; `ListTaskThreadRunEvents` returns optional `journal_messages`; task-detail loader prefers journal-backed message/tool steps while preserving non-message runtime events. 2026-07-01 source/runtime evidence recorded in `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-J-005-run-journal-message-contract.md`; screenshots stored under `screenshots/deerflow/P1-J-005-deerflow-message-thinking.png` and `screenshots/coze/P1-J-005-coze-journal-web-search.png`. Direct browser API navigation was blocked by local browser extension, so API evidence is covered by handler tests. Tests passed: `go test ./application/agentthread -run TestProjectRunJournalMessages -count=1`; `go test ./api/handler/coze -run 'TestListTaskThreadRunEventsHandlerReturnsJournalMessages|TestTaskThreadRunEventPayloadKeepsSafe|TestListTaskThreadRunEventsHandlerRedactsUnsafePayload' -count=1 -gcflags="all=-N -l"`; `npm run test -- src/pages/tasks/__tests__/task-detail-loader.test.ts`; `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`; `npx tsc --noEmit --project tsconfig.json`. |
| P1-J-006 Artifacts and present_files semantics | 已完成 | File creation, output presentation, artifact card position, side preview, download/copy, MIME handling, and final answer ordering match DeerFlow for document artifacts and `.skill` action semantics. 2026-07-01: side-preview width follows DeerFlow's 60/40 split; paired DeerFlow/Coze runtime screenshots, source anchors, DOM summaries, no-review-card evidence, and final-answer ordering tests are recorded in `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-J-006-artifact-present-files.md` and `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-J-006-artifact-split-width.md`. Tests passed: `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "generated document artifacts\|document artifact cards\|presented document cards"`; `go test ./application/agentthread -run 'TestApplication(WriteOutputFileStoresObjectAndRegistersOutputFile\|CreateSkillPackageWritesInstallableSkillArchive\|PresentOutputFilesRegistersArtifactsAndEmitsSafeEvent)' -count=1`. |
| P1-J-007 Streaming/stop/retry/follow-up | 已完成 | 2026-07-01: completed source-first parity pass for DeerFlow submit/stop-square, loading dots, canonical follow-up input/history, previous-turn step preservation, and retry/cancel affordance coverage. Evidence recorded in `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-J-007-stream-stop-followup.md`; screenshots: `screenshots/deerflow/P1-J-007-deerflow-input-ready.jpg`, `screenshots/coze/P1-J-007-coze-followup-ready.jpg`. Tests passed: `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "stops the latest running canonical thread run from the DeerFlow composer\|shows an optimistic waiting turn immediately after canonical follow-up submit\|sends canonical thread follow-up messages through the message API\|keeps execution steps for previous assistant turns after a follow-up\|renders streaming answer events before the final result is persisted"`. Exact To-dos/ChainOfThought visual spacing remains under P1-J-008. |
| P1-J-008 Quality acceptance suite | 已完成 | Same prompts run on DeerFlow and Coze for document artifacts, search/answer, and multi-turn revision; compare visible output, steps, artifacts, tokens, and safety redaction. | 2026-07-01: task-detail ChainOfThought structure and spacing were tightened against DeerFlow `ChainOfThought` / `message-group.tsx`; frontend test locks the DeerFlow-style chain container, collapsed last-step behavior, expanded step list, tool-specific labels, icons, and path pill. Browser DOM evidence, paired expanded-step screenshots, same-prompt document-artifact smoke, same-prompt search/answer smoke, and same-prompt multi-turn revision smoke are recorded in `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-J-008-chain-of-thought-style.md`; screenshot paths include `screenshots/deerflow/P1-J-008-deerflow-expanded-steps.jpg`, `screenshots/coze/P1-J-008-coze-expanded-steps.jpg`, `screenshots/coze/P1-J-008-coze-same-prompt-document.jpg`, `screenshots/deerflow/P1-J-008-deerflow-search-answer.jpg`, `screenshots/coze/P1-J-008-coze-search-answer.jpg`, `screenshots/deerflow/P1-J-008-deerflow-revision-followup.jpg`, and `screenshots/coze/P1-J-008-coze-revision-followup.jpg`. Skills/MCP detailed quality acceptance was explicitly deferred by user to P2, so it no longer blocks this quality suite. |

## P1-A Checklist

| Case | Status | Required Evidence |
| --- | --- | --- |
| P1-A-000 new-task environment baseline | 已完成 | DeerFlow and Coze new-task pages are reachable; safe screenshots and redacted DOM summaries stored. DeerFlow old reference task redirects to `workspace/chats/new`, so P1 uses fresh paired runs. |
| P1-A-001 canonical new task run | 已完成 | Coze task created at `http://localhost:8080/space/7656275718757679104/tasks/7657390468782620672`; screenshot and safe browser summary captured; unauthenticated curl correctly returns `401 missing session_key in cookie`. Authenticated API/event capture is explicitly N/A in the current in-app browser scope and documented in `notes/P1-A-001-003-authenticated-api-capture-decision.md`; no fake payloads recorded. |
| P1-A-002 DeerFlow comparison run | 已完成 | Fresh DeerFlow run created at `http://localhost:2026/workspace/chats/165d8335-5aa2-48ec-8295-54c4a917fce7`; visual screenshot and safe summary captured after an initial transient browser timeout. Authenticated Network capture is explicitly N/A for this P1-A browser pass and documented in `notes/P1-A-001-003-authenticated-api-capture-decision.md`. |
| P1-A-003 follow-up preserves history | 已完成 | Coze search/revision follow-up evidence captured for task `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`: prior turns, prior run steps, follow-up run steps, and token row remain visible. Direct browser-authenticated API read is blocked by read-only browser scope; source/handler-test anchors and the explicit N/A decision are recorded. Artifact continuity is handled by P1-A-005 because this sample has no artifacts. |
| P1-A-004 sidebar recent tasks pagination | 已完成 | Browser evidence shows `我的任务` renders 20 tasks, scrolls independently, and loads the next page to 40 rows. Source/test evidence covers `加载更多任务...`, `page_size=20`, and `coze:workspace-task-thread-upsert` immediate insertion/title patch. Evidence: `notes/P1-A-004-sidebar-recent-tasks.md`; test: `npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx -t "loads more recent tasks\|prepends a newly created task thread\|patches an existing recent task title"`. |
| P1-A-005 artifact side preview | 已完成 | Paired DeerFlow/Coze Markdown artifact card and side-preview evidence is recorded using the P1-J-006 verified runs, with P1-A screenshot aliases and source/test anchors. Evidence: `notes/P1-A-005-artifact-side-preview.md`; screenshots: `screenshots/deerflow/P1-A-005-deerflow-artifact-side-preview.png`, `screenshots/coze/P1-A-005-coze-artifact-side-preview.png`. Broader MIME samples remain P1-C. |
| P1-A-006 search/tool event safety | 已完成 | Web-search task evidence shows visible search/reasoning steps and token text with zero unsafe visible-pattern hits. Source/test anchors cover tool display sanitization and backend run-event payload redaction. Evidence: `notes/P1-A-006-search-tool-event-safety.md`; tests: frontend tool event display + backend redaction/web_search safe query targeted suites passed. |
| P1-A-007 token popover modes | 已完成 | Browser evidence shows the top-bar Token popover with `输入`/`输出`/`总计`, modes `关闭`/`总览`/`每轮`/`调试`, explanatory note, and zero unsafe visible-pattern hits. Evidence: `notes/P1-A-007-token-popover-modes.md`; test: `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token usage"`. |
| P1-A-008 loading/error states | 已完成 | Browser evidence covers bounded task not-found/cannot-view state with no raw error leak. Targeted frontend/backend tests cover DeerFlow-style loading skeleton, artifact preview failure redaction, safe artifact headers, scan-blocked conflict mapping, and signed URL object-URI hiding. Evidence: `notes/P1-A-008-loading-error-states.md`. Live throttled loading screenshot is N/A for this turn and replaced by deterministic component tests. |

## P1-C Checklist

| Case | Status | Required Evidence |
| --- | --- | --- |
| P1-C-001 automated coverage map | 已完成 | Source/test coverage matrix recorded in `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-C-artifact-mime-sample-map.md`. Tests passed: `npm run test -- src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`; `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t 'canonical thread artifacts\|generated document artifacts\|deletes a canonical thread artifact\|restores the last removed\|lists deleted thread artifacts\|reviews a blocked artifact\|renders artifact scan jobs'`; `go test ./api/handler/coze -run 'Test(GetTaskThreadArtifactContentHandler\|GetTaskThreadArtifactSignedURLHandler\|ReviewTaskThreadArtifactScanHandler\|ListTaskThreadArtifactsHandler\|DeleteTaskThreadArtifactHandler\|RetryTaskThreadArtifactScanJobHandler\|ListTaskThreadArtifactScanJobsHandler)' -count=1 -gcflags='all=-N -l'`; `go test ./application/agentthread -run 'Test(ApplicationReadArtifactContent\|ArtifactScanReadPolicy\|ApplicationRecordArtifactScanResult\|ApplicationProcessArtifactScanJobs)' -count=1`. |
| P1-C-002 real MIME fixture task | 已完成 | Coze task `http://localhost:8080/space/7656275718757679104/tasks/7657556998191316992` contains Markdown, TXT, JSON, CSV, PDF, PNG, HTML, and SVG artifact cards. Go ADK `write_file` now accepts binary `content_base64` and rejects ambiguous/missing content. Evidence: `notes/P1-C-artifact-mime-sample-map.md`; screenshot: `screenshots/coze/P1-C-coze-mime-fixture-task-7657556998191316992.png`. |
| P1-C-003 UI screenshots per MIME family | 已完成 | Browser screenshots recorded for full card placement, Markdown preview, TXT/JSON/CSV preview, PNG preview, HTML/SVG download-only active-content safety, PDF iframe mount after review release, copy/download affordances, and rendered PDF page pixels. Scan-pending safe blocked-state copy is covered by service/task-detail tests. Evidence: `notes/P1-C-artifact-mime-sample-map.md`; screenshots under `screenshots/coze/P1-C-coze-*-7657556998191316992.png`, including `P1-C-coze-pdf-page-pixels-rendered-7657556998191316992.png`. |
| P1-C-004 bounded API summaries | 已完成 | Source/test-based summaries recorded in `notes/P1-C-artifact-mime-sample-map.md` for artifact list, content, signed URL, scan jobs, retry, scan review, delete, and restore. The current in-app browser read-only scope cannot provide authenticated `fetch` payload capture, so live browser API capture is explicitly N/A. Redaction tests verify no raw object URI, object path, original filename, raw signed URL evidence, provider body, scanner raw body, prompt, completion, or checkpoint bytes are recorded in evidence. |
| P1-C-005 lifecycle and scan state UI | 已完成 | PDF scan review-release path has real browser evidence from task `7657556998191316992`: `pending` -> `clean`, review buttons removed, and iframe preview mounted without object URI exposure. SVG delete/restore lifecycle evidence is recorded: remove confirmation, removed-list restore row, and restored active row with actions; stale undo notice cleanup after deleted-list restore is fixed, covered by a unit regression, and recaptured in browser after reload. SVG scan review evidence is recorded: `blocked` state shows bounded review actions, then `clean` state removes review actions with no unsafe visible payload markers. Scan retry evidence is recorded: a single fixture scan job was temporarily changed to `failed`, UI showed `重试`, clicking the real retry action returned the job to `pending` with `manual retry requested`, and visible unsafe-pattern checks stayed empty. Bounded scan-blocked error evidence is recorded: `p1c-fixture.png` in `blocked` status shows `产物安全扫描未通过，暂不能预览`, no image preview is mounted, and stale signed-preview cleanup after review is fixed with regression coverage. |

## Evidence Locations

Use the existing evidence root:

```text
docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/
```

Naming convention:

- `screenshots/deerflow/P1-A-XXX-short-name.png`
- `screenshots/coze/P1-A-XXX-short-name.png`
- `network/deerflow/P1-A-XXX-endpoint.json`
- `network/coze/P1-A-XXX-endpoint.json`
- `notes/P1-A-XXX-findings.md`

Network summaries must include only bounded metadata: method/path/status,
safe query/body shape, top-level response fields, run/thread ids, event types,
usage totals, and explicit redaction notes.

## Verification Gate

Before marking a P1 slice `已完成`, record:

1. DeerFlow source/runtime/API evidence or an explicit N/A reason.
2. Coze source/runtime/API evidence.
3. Targeted tests or browser checks.
4. Security redaction check for prompts, completions, tool arguments/results,
   credentials, object URIs, signed URLs, raw provider bodies, and checkpoint
   bytes.
5. Any remaining gap moved to a later P1 row or `延后(P2)`.

## Current Next Step

Continue P1-B task-detail visual evidence:

- Capture the remaining `TD-LAYOUT-002` 390px/768px responsive screenshots and
  fix only real overlap or clipping regressions found there.
- Keep P1-C closed. Any automatic renewal for already-open signed artifact
  preview URLs after TTL expiry is a P2 hardening item unless it becomes a
  current visible P1 regression.
