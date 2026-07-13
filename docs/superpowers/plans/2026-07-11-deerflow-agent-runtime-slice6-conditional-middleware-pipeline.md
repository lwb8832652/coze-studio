# DeerFlow Agent Runtime Slice 6 Conditional Middleware Pipeline Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `executing-plans` and
> `test-driven-development` task by task. Do not preserve reserved middleware
> placeholders for index compatibility.

**Goal:** Close `AR-PARITY-002.3` by replacing reserved no-op middleware with
an explicit conditional Eino handler pipeline, enforcing the canonical
per-model-turn subagent call limit, and recording one bounded provider
capability downgrade event per Agent instance.

**Architecture:** `ADKMiddlewareAssembler` keeps one ordered list of concrete
behavior owners. Optional builders return an explicit not-applicable sentinel
and are omitted from the Eino handler slice. `ADKToolSet` carries internal-only
subagent tool names into a dedicated after-model limiter. Provider capability
middleware compares requested and effective reasoning options and emits a
sanitized event through the existing run event sink. Public projection exposes
only the bounded downgrade summary.

**Tech Stack:** Go, Eino ADK `v0.9.9`, Testify.

---

## Locked Evidence

- DeerFlow baseline: `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- `backend/packages/harness/deerflow/agents/middlewares/tool_error_handling_middleware.py:129-187`
  assembles only enabled runtime middleware. Disabled guardrails and uploads
  do not create placeholders.
- `backend/packages/harness/deerflow/agents/lead_agent/agent.py:260-377`
  documents and implements the Lead chain, including conditional
  summarization, Todo, vision, deferred-tool filtering, subagent limiting,
  loop detection, safety finish and final clarification.
- `backend/packages/harness/deerflow/agents/middlewares/subagent_limit_middleware.py:20-76`
  clamps the limit to two through four and drops only excess task calls from
  one model response.
- `safety_finish_reason_middleware.py:21-32` requires safety suppression to
  observe raw model output before loop/subagent accounting.
- Eino `adk/chatmodel.go:320-360` and `adk/wrappers.go:1229-1281` prove handler
  before/after hooks run in registration order; WrapModel/WrapTool use the
  first registered handler as the outer wrapper.

## Confirmed NewX Gaps

- `adkMiddlewareOrder` declares `agentsmd`, `policy`, `audit` and `usage`
  entries with no implementation.
- Missing providers, empty Skills, no dynamic tools, disabled Plan, absent
  transcript storage and absent offload backend return `reservedADKMiddleware`
  and still enter the Eino chain.
- Tests depend on fixed handler indexes, hiding the difference between an
  enabled capability and a reserved placeholder.
- Ultra mode only describes a concurrency limit in the prompt. No after-model
  middleware truncates excess SingleAgent tool calls.
- Unsupported thinking/reasoning is silently downgraded before model option
  projection; no bounded runtime event records the downgrade.

## Task 1: Explicit Conditional Pipeline

**Files:**

- Modify: `backend/application/agentthread/adk_agent_factory.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/adk_middleware_test.go`
- Modify affected middleware tests that use fixed handler indexes.

- [x] Write RED tests proving absent Memory, Skill, deferred tools, Plan,
  transcript and offload handlers are omitted and no handler is a reserved
  placeholder.
- [x] Add internal handler names to `ADKMiddlewareBundle` so tests and runtime
  diagnostics can identify active handlers without fixed indexes.
- [x] Replace reserved middleware returns with an explicit not-applicable
  result handled only by the assembler. A custom builder returning nil without
  that result remains an error.
- [x] Remove unimplemented `agentsmd`, `policy`, `audit` and `usage` entries
  from the declared Eino chain; their existing behavior owners remain the
  prompt composer, tool policy/guardrail wrappers, callbacks and run processor.
- [x] Reorder concrete handlers by approved behavior phase. Because Eino after
  hooks run in registration order, safety suppression must precede subagent
  limiting and semantic loop accounting.
- [x] Convert fixed-index tests to name-based active-handler lookup and assert
  the concrete request/wrapper/unwind order.

## Task 2: Per-Turn Subagent Concurrency Enforcement

**Files:**

- Modify: `backend/application/agentthread/adk_agent_factory.go`
- Modify: `backend/application/agentthread/adk_subagent_tool_provider.go`
- Modify: `backend/application/agentthread/adk_tool_policy_provider.go`
- Add: `backend/application/agentthread/adk_subagent_limit.go`
- Add: `backend/application/agentthread/adk_subagent_limit_test.go`

- [x] Write RED tests with mixed ordinary/subagent calls proving only excess
  subagent calls are removed, original order is retained, the input state is
  not mutated and a bounded truncation event contains counts but no arguments.
- [x] Carry normalized subagent tool names as internal `ADKToolSet` metadata;
  preserve/filter it through human-interaction and tool-policy providers.
- [x] Add a conditional after-model middleware only when Ultra/subagent
  capability is enabled and at least one allowed subagent tool exists.
- [x] Enforce the canonical two-through-four limit from
  `DeerFlowRuntimeConfig.MaxConcurrentSubagents`; do not rely on prompt text.
- [x] Verify safety suppression runs before the limiter and semantic loop runs
  after it under Eino registration-order semantics.

## Task 3: Bounded Capability Downgrade Event

**Files:**

- Modify: `backend/application/agentthread/adk_provider_capability.go`
- Modify: `backend/application/agentthread/adk_provider_capability_test.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/public_projection.go`
- Modify: `backend/application/agentthread/public_projection_test.go`

- [x] Write RED tests for reasoning-only, thinking-only and combined downgrade,
  no event when supported, and concurrent/repeated hooks emitting once.
- [x] Preserve requested and effective reasoning values in provider capability
  middleware and emit `model.capability_downgraded` once per Agent instance via
  `sync.Once`.
- [x] Keep payload bounded to schema, capability names and requested/effective
  booleans/effort labels. Never include prompt, model body, provider response or
  tool data.
- [x] Add an explicit public projection case for the bounded fields; generic
  `model.*` redaction remains unchanged.

## Task 4: Verification, Evidence And Commit

- [x] Run focused RED/GREEN suites, complete `application/agentthread`, all
  application packages, targeted race, application vet, full serial backend
  tests and `APP_ENV=debug make build_server`.
- [x] Record DeerFlow source order, Eino order semantics, active NewX pipeline,
  concurrency enforcement, downgrade payload and deferred `.4` state work in
  `docs/superpowers/evidence/`.
- [x] Close only `AR-PARITY-002.3` in the tracker, self-review the staged diff
  and commit without pushing or merging `dev`.

## Exit Criteria

- No `reservedADKMiddleware` or other declared no-op enters an Eino Agent.
- Disabled optional capabilities are absent from `Handlers`, while required
  concrete handlers preserve tested order and wrapper unwind behavior.
- One model turn cannot execute more than the canonical number of allowed
  subagent tools, even if the model ignores the prompt limit.
- Unsupported reasoning/thinking succeeds with effective downgraded options and
  one bounded observable event per Agent instance.
- Public APIs expose no newly sensitive model, prompt, tool or provider data.
