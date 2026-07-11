# DeerFlow Agent Runtime Slice 4 Production Runtime And Mode Contract Plan

> **For agentic workers:** execute this plan task by task with RED/GREEN tests.
> Do not remove historical legacy checkpoint support while closing new-run
> selection.

**Goal:** Make Eino ADK the only runtime persisted by new public task/run
requests and establish one server-owned DeerFlow mode projection for Agent,
model, Plan/Todo and subagent decisions.

**Architecture:** Public creation normalizes raw run JSON into a canonical
`DeerFlowRuntimeConfig` before any aggregate is persisted. New runs accept only
`eino_adk`; missing runtime becomes `eino_adk`, while explicit `legacy` is a
bounded client error. Worker selection retains a separate historical fallback
for old rows/checkpoints without a runtime marker. The ADK factory parses mode
once and passes the projection to model capability and middleware assembly;
Plan and subagent capabilities are gated by that projection.

**Tech Stack:** Go, Eino ADK, Hertz, JSON contracts, Testify.

---

## Verified DeerFlow Contract

- Baseline: `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- `backend/app/gateway/services.py:121-165` forwards only the bounded runtime
  context keys to both LangGraph `configurable` and `context`.
- `frontend/src/core/threads/hooks.ts:971-985` deterministically projects:
  `flash` to no thinking/plan/subagent, `thinking` to thinking only, `pro` to
  thinking plus plan, and `ultra` to thinking plus plan and subagents. Default
  reasoning effort is respectively none, low, medium and high.
- `backend/packages/harness/deerflow/agents/lead_agent/agent.py:415-465`
  consumes the projected values once while constructing the Agent and model.
- `agent.py:320-355` installs Todo only for plan mode and subagent limiting only
  when subagents are enabled.
- `agent.py:484-538` filters tools and prompt sections with the same subagent
  decision.

## Confirmed Pre-Slice NewX Gaps

- `RuntimePolicyFromEnv` still defaults to `legacy` with Eino disabled, and a
  new request can explicitly persist `runtime=legacy`.
- Runtime validation does not normalize the JSON written to the run row, so a
  missing runtime remains ambiguous to workers and recovery.
- The frontend sends the correct mode fields, but backend middlewares parse raw
  JSON independently and have no authoritative mode contract.
- Production always creates a Plan backend, exposing Todo tools in flash and
  thinking modes.
- Configured subagent tools are not gated by `subagent_enabled`.
- Historical rows without a runtime marker must continue to resolve as legacy;
  changing the global default must not reinterpret their checkpoints as ADK.

## Task 1: Canonical Runtime And Mode Projection

**Files:**

- Add: `backend/application/agentthread/runtime_config.go`
- Test: `backend/application/agentthread/runtime_config_test.go`

- [x] Add RED table tests for `flash`, `thinking`, `pro` and `ultra`, including
  default effort and the Plan/subagent decisions.
- [x] Add RED tests for bounded explicit overrides, legacy mode aliases used by
  historical NewX clients, invalid mode/effort and subagent concurrency bounds.
- [x] Implement one parser and canonical JSON normalizer that preserves unknown
  bounded feature configuration while replacing runtime/mode-derived fields.

## Task 2: New Runs Use Only Eino ADK

**Files:**

- Modify: `backend/application/agentthread/runtime_selector.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Test: `backend/application/agentthread/runtime_selector_test.go`
- Test: `backend/application/agentthread/service_test.go`
- Test: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] Change production environment defaults to enabled `eino_adk` and reject
  an environment-selected legacy production default.
- [x] Normalize CreateTaskThread/CreateRun config before persistence; reject
  explicit legacy/unknown runtime as a bounded 400 without writing a thread,
  run or message.
- [x] Keep historical worker and checkpoint selection: old persisted rows with
  no marker still resolve as legacy, while explicit Eino checkpoints resume via
  ADK.

## Task 3: Apply The Mode Contract To ADK Construction

**Files:**

- Modify: `backend/application/agentthread/adk_agent_factory.go`
- Modify: `backend/application/agentthread/adk_provider_capability.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/adk_subagent_tool_provider.go`
- Test: corresponding `*_test.go` files.

- [x] Parse the runtime projection once in the ADK factory and pass it into
  middleware/model construction.
- [x] Project mode-derived thinking and reasoning defaults through provider
  capability handling; downstream code consumes the canonical projection after
  bounded explicit overrides have been validated.
- [x] Build or expose Plan/Todo tools only when the canonical plan capability is
  enabled; Pro/Ultra enable it by default.
- [x] Resolve configured subagent tools only when the canonical subagent
  capability is enabled; Ultra enables it by default. Retain historical
  unmarked-run compatibility until migration is complete.

## Task 4: Acceptance And Commit

- [x] Run focused RED/GREEN tests for runtime config, selector, service, Agent
  factory, provider capability, Plan middleware and subagent tool provider.
- [x] Run affected package suites with Mockey flags, targeted race/vet, serial
  full backend tests and `APP_ENV=debug make build_server`.
- [x] Record source/API/config/tool evidence, update the P0 tracker and perform
  self-review.
- [x] Commit this mainline slice without pushing or merging `dev`.

## Exit Criteria

- Every new public task/run persists canonical `runtime=eino_adk`; no request
  can select the legacy harness.
- Historical rows/checkpoints without an ADK marker retain the legacy recovery
  interpretation and are not silently reinterpreted.
- One backend projection owns mode, thinking, reasoning effort, plan and
  subagent semantics for all four modes.
- Flash/thinking disable Plan tools by default; Pro/Ultra enable them by
  default. Ultra alone enables subagent tools by default. Validated explicit
  overrides remain authoritative in the canonical projection.
- Invalid mode/runtime configuration fails before aggregate persistence with no
  raw config or provider details exposed.
