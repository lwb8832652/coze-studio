# DeerFlow Agent Production Runtime And Mode Contract Evidence

**Task:** `AR-PARITY-002.1`

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX branch:** `codex/deerflow-parity-mainline`

## 1. Verified DeerFlow Behavior

The reference behavior was verified from source, not inferred from UI labels.

- `backend/app/gateway/services.py:121-165` copies only bounded Agent context
  keys into LangGraph `configurable` and `context`.
- `frontend/src/core/threads/hooks.ts:971-985` derives thinking, Plan,
  subagent and reasoning effort from `flash`, `thinking`, `pro` and `ultra`.
- When mode is absent, that frontend sends thinking enabled, Plan disabled,
  subagent disabled and no reasoning effort.
- `backend/packages/harness/deerflow/agents/lead_agent/agent.py:415-465`
  consumes those values while constructing the lead Agent and model.
- `agent.py:320-355` installs Todo only in plan mode and installs subagent
  limiting only when subagents are enabled.
- `agent.py:484-538` uses the same subagent decision for tools and prompt
  sections.

## 2. NewX Runtime Contract

### New public runs

- `RuntimePolicyFromEnv` now defaults to enabled `eino_adk`.
- A production environment cannot select `legacy` as its default and cannot
  disable the selected Eino runtime without failing startup configuration.
- Workbench and LangGraph creation normalize configuration before aggregate
  persistence. Missing runtime becomes `eino_adk`; explicit `legacy` or an
  unknown runtime returns bounded HTTP 400 and writes no thread/run/message.
- Raw runtime validation details are not returned to clients.

### Historical compatibility

- Existing rows without a runtime marker still resolve as `legacy` in worker
  selection.
- An explicit checkpoint runtime still takes precedence during resume.
- Historical camelCase `reasoningEffort` / `thinkingEnabled` configuration is
  retained when no canonical mode or snake_case reasoning field exists.
- Existing legacy checkpoints therefore remain readable/recoverable; this
  compatibility path is not selectable by a new public request.

## 3. Server-Owned Mode Projection

`DeerFlowRuntimeConfig` is the single mode projection used by ADK construction.

| Mode | Thinking | Plan/Todo | Subagent | Default effort |
| --- | --- | --- | --- | --- |
| `flash` | false | false | false | none |
| `thinking` | true | false | false | low |
| `pro` | true | true | false | medium |
| `ultra` | true | true | true | high |

Bounded explicit fields may override mode defaults. Subagent concurrency is
limited to the accepted range two through four, with three as the Ultra
default. Historical `Auto`, `Ask` and `Agent` values are canonicalized to
`flash`, `thinking` and `pro`.

For a request without mode, the canonical default matches DeerFlow's effective
context: thinking enabled, Plan/subagent disabled and no reasoning effort.

## 4. LangGraph Context Precedence

NewX accepts both its Workbench root config and DeerFlow/LangGraph context
shape. Only the verified context whitelist is projected:

`model_name`, `mode`, `thinking_enabled`, `reasoning_effort`,
`is_plan_mode`, `subagent_enabled`, `max_concurrent_subagents`, `agent_name`,
and `is_bootstrap`.

Precedence matches the observed DeerFlow merge behavior:

1. root NewX config;
2. `config.configurable`;
3. request `context`;
4. `config.context`.

Unknown request-context fields are not copied into canonical runtime config.
The original bounded run context remains separately persisted for internal
compatibility and is redacted by the existing public projection.

## 5. Conditional Agent Construction

- The ADK factory parses the mode projection once and passes it to model and
  middleware construction.
- Mode-derived thinking and reasoning effort reach provider options.
- Unsupported thinking/reasoning is downgraded rather than failing the whole
  run, matching DeerFlow's successful fallback behavior.
- Plan backend and Todo tools are not built outside a plan-enabled mode.
- Configured SingleAgent subagent definitions are not resolved when subagents
  are disabled. Unmarked historical rows keep the previous compatibility
  behavior until their checkpoints are retired.

## 6. Verification

Passed:

```text
go test -gcflags='all=-N -l' ./application/agentthread ./application/workbench ./api/handler/coze ./api/router/coze -count=1
go test -race -gcflags='all=-N -l' ./application/agentthread ./api/handler/coze -run '^(TestParseDeerFlowRuntimeConfig.*|TestNormalizeNewDeerFlowRunConfig.*|TestRuntimePolicyFromEnv.*|TestRuntimeSelector(KeepsUnmarkedHistoricalRunsOnLegacy|UsesCanonicalEinoRuntimeMarker)|TestApplicationCreate(TaskThreadCanonicalizesProductionRuntimeAndMode|RunRejectsLegacyRuntimeBeforePersistence|RunCanonicalizesLangGraphRuntimeContext)|TestADKAgentFactory(ProjectsModeDefaultReasoningOptions|DowngradesUnsupportedModeReasoning|PreservesHistoricalCamelCaseReasoningOptions)|TestADKMiddlewareDoesNotBuildPlanBackendOutsidePlanMode|TestADKSubagentToolProvider(SkipsDefinitionsWhenCapabilityDisabled|ResolvesDefinitionsWhenCapabilityEnabled)|TestCreateTaskThreadHandlerRejectsLegacyRuntimeAsBadRequest|TestLangGraphRunCreateCanonicalizesDeerFlowContextForEinoADK)$' -count=1
go vet ./application/agentthread ./application/workbench ./api/handler/coze ./api/router/coze
go test -p 1 -gcflags='all=-N -l' ./... -count=1
APP_ENV=debug make build_server
gofmt -l <touched-go-files>
git diff --check
```

The macOS race linker emitted the known malformed `LC_DYSYMTAB` warning; both
race packages returned exit code zero. No schema migration was required.

## 7. Remaining Agent Semantic Core Work

This evidence closes `AR-PARITY-002.1`, not all backend Agent parity.

- `AR-PARITY-002.2`: versioned DeerFlow lead prompt composer and bounded custom
  Agent overlays.
- `AR-PARITY-002.3`: conditional middleware phases and removal of reserved
  no-op entries, including enforcement of the canonical per-turn subagent
  concurrency limit and a bounded exactly-once capability downgrade event.
- `AR-PARITY-002.4`: durable parity state envelope for Todo, promoted tools,
  Skills, uploads, artifacts and interrupts.
- Workspace/file tools, DeerFlow task-style subagents and default-on memory
  remain in their approved later delivery slices.
