# DeerFlow Agent Runtime Slice 5 Lead Prompt Composer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `executing-plans` and
> `test-driven-development` task by task. Keep the client prompt as an overlay;
> never restore replacement semantics.

**Goal:** Add one versioned NewX AI Lead Prompt Composer that preserves the
verified DeerFlow behavior contract, applies mode/tool sections
deterministically, and loads bounded custom SingleAgent overlays without
allowing client text to replace system-owned rules.

**Architecture:** `ApplicationADKAgentFactory` resolves the canonical
`DeerFlowRuntimeConfig`, an optional durable SingleAgent overlay, and the
available static/dynamic tool shape. A pure `ADKLeadPromptComposer` builds the
system instruction from versioned sections. Date/memory, selected Skill
content, Skill catalog tool metadata and deferred-tool catalog metadata remain
owned by their existing Eino middlewares. The factory applies durable model
defaults before model resolution and passes the composed prompt to Eino
`ChatModelAgent`.

**Tech Stack:** Go, Eino ADK, Coze SingleAgent domain service, Testify.

---

## Locked DeerFlow Evidence

- Baseline: `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- `backend/packages/harness/deerflow/agents/lead_agent/prompt.py:364-560`
  owns the stable role, clarification, workspace/output, response-style,
  citation and critical-reminder sections.
- `prompt.py:608-675` keeps Skill metadata in a bounded catalog and uses
  progressive loading rather than preloading every Skill body.
- `prompt.py:184-362,757-822` conditionally adds subagent orchestration and
  the hard per-turn concurrency guidance.
- `agent.py:415-538` resolves mode, model, Agent config and tool shape before
  calling `apply_prompt_template`; the same `subagent_enabled` value controls
  tools and prompt.
- `agent.py:300-365` injects date/memory and selected Skill content through
  middleware, not through the static prompt.
- `backend/tests/test_lead_agent_prompt.py` asserts custom-agent self-update,
  workspace paths, explicit config usage, subagent descriptions and template
  placeholder integrity.

## Confirmed Pre-Slice NewX Gaps

- `ApplicationADKAgentFactory` passes `cfg.SystemPrompt` directly as Eino
  `Instruction`; an empty client field leaves no stable Lead instruction.
- Client `system_prompt` therefore replaces the Agent identity and can remove
  clarification, output, citation and safety rules.
- Prompt text does not currently vary with canonical Plan/subagent mode.
- The top-level factory has no durable SingleAgent prompt/model overlay; only
  child subagent snapshots are loaded.
- Existing Eino memory, Skill, tool-search, uploads and artifact paths already
  provide dynamic data. The composer must reference those capabilities without
  duplicating private bodies/catalogs into static prompt text.

## Task 1: Pure Versioned Prompt Contract

**Files:**

- Add: `backend/application/agentthread/adk_lead_prompt.go`
- Add: `backend/application/agentthread/adk_lead_prompt_test.go`

- [x] Write RED tests for a default prompt containing the version marker,
  NewX AI role, clarification-before-action, Skill progressive-loading,
  workspace/output contract, citation rules, response style and system-owned
  hierarchy.
- [x] Write RED table tests proving Plan guidance appears only when
  `PlanCapabilityEnabled()` is true, deferred-tool guidance only when dynamic
  tools exist, and subagent decomposition/synthesis plus the canonical
  concurrency value only when `SubagentCapabilityEnabled()` is true.
- [x] Write RED tests proving client and durable Agent text is XML-escaped,
  bounded, source-labelled and appended after non-overridable system sections.
- [x] Implement:

```go
const adkLeadPromptContractVersion = "newx.lead_prompt.v1"

type ADKLeadPromptComposeInput struct {
    RuntimeConfig     DeerFlowRuntimeConfig
    HasDeferredTools  bool
    ClientOverlay     string
    DurableOverlay    ADKLeadPromptOverlay
}

type ADKLeadPrompt struct {
    Version          string
    Instruction      string
    AgentName        string
    AgentDescription string
}

type ADKLeadPromptComposer interface {
    Compose(ADKLeadPromptComposeInput) (ADKLeadPrompt, error)
}
```

- [x] Keep the static prompt free of dates, recalled memory, full Skill bodies,
  tool arguments/results, credentials, object URIs and raw provider metadata.
- [x] Run:

```bash
cd backend
go test -gcflags='all=-N -l' ./application/agentthread \
  -run '^TestADKLeadPrompt' -count=1
```

Expected: RED before implementation, then PASS.

## Task 2: Durable SingleAgent Overlay

**Files:**

- Add: `backend/application/agentthread/adk_lead_prompt_overlay.go`
- Add: `backend/application/agentthread/adk_lead_prompt_overlay_test.go`
- Reuse: `backend/application/agentthread/adk_singleagent_subagent_provider.go`

- [x] Write RED tests for parsing `assistant_id=singleagent:<id>`, loading a
  draft when version is empty, loading the explicit version when present, and
  returning no overlay for the default Lead Agent.
- [x] Write RED tests that fail closed when AssistantID/config Agent IDs differ,
  the durable Agent belongs to another space, the snapshot is missing, or the
  prompt exceeds the bounded overlay limit.
- [x] Implement an optional `ADKLeadPromptOverlayProvider` backed by the existing
  `ADKSingleAgentSubagentService`. Return only bounded name, description, prompt
  and model defaults; do not return plugins, knowledge, workflow credentials or
  other raw snapshot fields.
- [x] Merge model defaults with DeerFlow precedence: explicit run model options
  win; missing values may use the durable SingleAgent snapshot; global model
  fallback remains last.
- [x] Run:

```bash
cd backend
go test -gcflags='all=-N -l' ./application/agentthread \
  -run '^TestADK(SingleAgentLeadPromptOverlay|ApplyLeadPromptModelDefaults)' \
  -count=1
```

Expected: RED before implementation, then PASS.

## Task 3: ADK Factory Integration

**Files:**

- Modify: `backend/application/agentthread/adk_agent_factory.go`
- Modify: `backend/application/agentthread/adk_agent_factory_test.go`
- Modify: `backend/application/agentthread/adk_singleagent_subagent_agent_factory.go`
- Modify: `backend/application/agentthread/adk_singleagent_subagent_agent_factory_test.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder_test.go`
- Modify: `backend/application/application.go`

- [x] Write RED factory tests proving an empty client prompt still produces the
  versioned Lead prompt and a client prompt is present only inside the bounded
  overlay section rather than replacing the base contract.
- [x] Write RED tests proving Pro receives Plan guidance, Ultra receives
  subagent/concurrency guidance, and flash receives neither by default.
- [x] Write RED tests proving a durable SingleAgent overlay changes the bounded
  Agent name/description/prompt and supplies only missing model defaults.
- [x] Add backward-compatible factory options for the composer and overlay
  provider; keep existing constructor call sites valid.
- [x] Resolve the overlay before model selection, compose after static/dynamic
  tools are known, and pass only the composed instruction to
  `adk.NewChatModelAgent`.
- [x] Wire the production top-level factory to the existing durable
  SingleAgent domain service. Child factories without that provider continue to
  consume their already snapshotted config as a client overlay.
- [x] Canonicalize child runs as Eino `flash` with thinking, Plan and recursive
  subagent capability disabled so compatibility defaults cannot leak top-level
  orchestration into child prompts or persisted retries.
- [x] Run:

```bash
cd backend
go test -gcflags='all=-N -l' ./application/agentthread \
  -run '^TestADKAgentFactory.*(LeadPrompt|PromptOverlay|ModePrompt)' -count=1
```

Expected: RED before implementation, then PASS.

## Task 4: Acceptance, Evidence And Commit

- [x] Run the complete Agent package and affected application package suites.
- [x] Run targeted `-race`, `go vet`, serial full backend tests and
  `APP_ENV=debug make build_server`.
- [x] Record exact DeerFlow source chain, NewX runtime model-input evidence,
  conditional prompt assertions, API security boundary and known deferred
  items under `docs/superpowers/evidence/`.
- [x] Update the P0 tracker: close only `AR-PARITY-002.2`; keep middleware
  phase removal, durable state, full workspace tools and default-on memory in
  their approved later slices.
- [x] Perform a staged-diff self-review and commit this mainline slice without
  pushing or merging `dev`.

## Exit Criteria

- Every Eino ADK run has the same versioned, system-owned Lead contract even
  when the client sends no system prompt.
- Client text and durable SingleAgent text are bounded overlays and cannot
  replace role, clarification, tool/output, citation or safety sections.
- Canonical mode decisions condition the Plan and subagent prompt sections;
  the same runtime projection already controls tools/middleware.
- Dynamic date/memory, full Skill bodies and deferred tool catalogs remain in
  their existing middleware paths and are not duplicated in the static prompt.
- A durable custom Agent snapshot is tenant/space checked and contributes only
  bounded prompt identity and missing model defaults.
- Public Workbench/LangGraph projections still expose no prompt or raw runtime
  config.
