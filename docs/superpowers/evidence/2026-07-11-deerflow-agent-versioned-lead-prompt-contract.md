# DeerFlow Agent Versioned Lead Prompt Contract Evidence

**Task:** `AR-PARITY-002.2`

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX branch:** `codex/deerflow-parity-mainline`

## 1. Verified DeerFlow Source Chain

This slice was derived from the locked DeerFlow source, not from UI text or
assumed Agent behavior.

- `backend/packages/harness/deerflow/agents/lead_agent/prompt.py:364-560`
  owns the stable Lead role, thinking/clarification order, workspace/output,
  citation, response-style and critical-reminder contract.
- `prompt.py:608-675` places bounded Skill metadata in the base prompt and
  requires progressive loading of the full Skill instructions.
- `prompt.py:184-362,757-822` builds the subagent section only when enabled and
  projects the same maximum-concurrency value into decomposition guidance.
- `backend/packages/harness/deerflow/agents/lead_agent/agent.py:415-538`
  resolves mode, model, custom Agent, tools and deferred tools before composing
  the prompt. The same `subagent_enabled` decision controls both tools and
  prompt sections.
- `agent.py:300-365` keeps date, memory, selected Skill content, Todo and other
  turn-specific behavior in middleware. Those values are not copied into the
  static system prompt.
- `backend/tests/test_lead_agent_prompt.py` verifies workspace paths, custom
  Agent behavior, subagent content and template-placeholder integrity.

## 2. NewX Versioned Prompt Contract

Every Eino ADK Agent constructed through `ApplicationADKAgentFactory` now
receives a system-owned prompt with contract version `newx.lead_prompt.v1`.
An empty request prompt no longer leaves the Agent without stable operating
instructions.

The stable contract contains:

- NewX AI Lead identity and instruction hierarchy;
- concise thinking and visible-final-response requirements;
- clarification before ambiguous, missing or high-risk work;
- Skill progressive loading and explicit activation precedence;
- verified upload, workspace and output paths;
- artifact presentation, citation and response-style requirements;
- tenant, workspace, tool and completion boundaries.

The static prompt deliberately excludes dates, recalled memory, full Skill
bodies, raw tool catalogs, tool arguments/results, credentials, object URIs,
provider payloads and runtime configuration. Existing Eino middleware remains
the owner of those dynamic values.

## 3. Mode And Tool Conditional Sections

The already canonical `DeerFlowRuntimeConfig` drives prompt sections as well as
runtime capabilities.

| Condition | Prompt projection |
| --- | --- |
| deferred tools exist | tool-search discovery and promotion guidance |
| Plan capability enabled | Todo/Plan lifecycle guidance |
| subagent capability enabled | decomposition, synthesis and bounded concurrency guidance |
| child subagent run | explicit `flash`; thinking, Plan and recursive subagent capability disabled |

This prevents historical compatibility defaults from installing top-level
orchestration instructions in a child Agent. Both the in-memory child snapshot
and the persisted child-run config carry the same explicit mode fields, so a
retry cannot change semantics.

## 4. Bounded Custom Agent Overlays

Request `system_prompt` text no longer replaces the stable Lead contract. It is
trimmed, UTF-8/NUL validated, limited to 16 KiB, XML-escaped, source-labelled
and appended as a request overlay after system-owned sections.

Top-level `assistant_id=singleagent:<id>` may add a durable Agent overlay through
the existing SingleAgent domain service. The provider:

- accepts only a matching positive Agent ID and a coherent draft/version
  reference;
- loads the requested draft or immutable version;
- fails closed for a missing snapshot, ID/version mismatch or cross-space
  access;
- returns only bounded name, description, prompt and model defaults;
- never returns plugins, knowledge, workflows, credentials or raw snapshot
  metadata to the prompt composer.

Model precedence is explicit run configuration, then missing values from the
durable SingleAgent snapshot, then the existing global model fallback. Durable
defaults cannot overwrite request model options.

## 5. Public API Security Boundary

The composed prompt is passed only to Eino `ChatModelAgentConfig.Instruction`.
It is not written to run events or journal messages by this slice.
`ProjectPublicRun` continues to expose a bounded run projection without
`Config`, `Context`, prompt or model-input fields. Existing public event and
message projectors remain the only API output path, so request/durable prompt
text and internal mode configuration are not newly exposed.

No IDL, API response shape or database migration changed.

## 6. Verification

Passed:

```text
go test -gcflags='all=-N -l' ./application/agentthread -run '^TestADKLeadPrompt' -count=1
go test -gcflags='all=-N -l' ./application/agentthread -run '^TestADK(SingleAgentLeadPromptOverlay|ApplyLeadPromptModelDefaults)' -count=1
go test -gcflags='all=-N -l' ./application/agentthread -run '^TestADKAgentFactory.*(LeadPrompt|PromptOverlay|ModePrompt)' -count=1
go test -gcflags='all=-N -l' ./application/agentthread -run '^(TestADKSingleAgentSubagentAgentFactoryBuildsRunnableChildAgent|TestADKSingleAgentSubagentRunSummaryMapsStableSnapshotFields|TestApplicationADKSubagentRunRecorderStartsChildRun)$' -count=1
go test -gcflags='all=-N -l' ./application/agentthread -count=1
go test -p 1 -gcflags='all=-N -l' ./application/... -count=1
go test -race -gcflags='all=-N -l' ./application/agentthread -run '^(TestADK(AgentFactory(BuildsVersionedLeadPromptWithoutClientPrompt|ProjectsModePromptSections|AppliesDurableLeadPromptOverlay)|SingleAgentLeadPromptOverlay.*|ApplyLeadPromptModelDefaults.*|LeadPrompt.*|SingleAgentSubagentAgentFactoryBuildsRunnableChildAgent|SingleAgentSubagentRunSummaryMapsStableSnapshotFields)|TestApplicationADKSubagentRunRecorderStartsChildRun)$' -count=1
go vet ./application/...
go test -p 1 -gcflags='all=-N -l' ./... -count=1
APP_ENV=debug make build_server
```

The targeted race run emitted the known macOS malformed `LC_DYSYMTAB` linker
warning and returned exit code zero.

## 7. Scope Boundary

This evidence closes `AR-PARITY-002.2`, not the full Agent semantic core.

- `AR-PARITY-002.3` still owns conditional middleware phase cleanup, canonical
  concurrency enforcement and bounded capability-downgrade events.
- `AR-PARITY-002.4` still owns durable Todo, promoted-tool, Skill, upload,
  artifact and interrupt parity state.
- Full DeerFlow workspace/file tools and default-on memory remain in their
  separately approved later slices.
