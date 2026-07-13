# DeerFlow Agent Conditional Middleware Pipeline Evidence

**Task:** `AR-PARITY-002.3`

**DeerFlow baseline:** `5851f8250eb150ca23134c79b11ebc5073ac2789`

**NewX branch:** `codex/deerflow-parity-mainline`

## 1. Verified Source Behavior

This slice was derived from the locked DeerFlow and Eino sources rather than
from inferred UI behavior.

- DeerFlow
  `backend/packages/harness/deerflow/agents/middlewares/tool_error_handling_middleware.py:129-187`
  constructs only enabled middleware. Optional guardrail and upload behavior is
  omitted when unavailable; no placeholder enters the runtime chain.
- DeerFlow
  `backend/packages/harness/deerflow/agents/lead_agent/agent.py:260-377`
  conditionally installs dynamic context, Skill activation, summarization,
  Todo, vision, deferred tools, subagent limiting and loop/safety behavior.
- DeerFlow
  `backend/packages/harness/deerflow/agents/middlewares/subagent_limit_middleware.py:20-76`
  clamps the per-model-response subagent limit to two through four and removes
  only excess subagent calls.
- DeerFlow
  `backend/packages/harness/deerflow/agents/middlewares/safety_finish_reason_middleware.py:21-32`
  observes raw model output before later accounting can alter tool calls.
- Eino ADK `v0.9.9` `adk/chatmodel.go:320-360` and
  `adk/wrappers.go:1229-1281` execute before/after rewrite hooks in registration
  order while the first registered model/tool wrapper is outermost.

The last point is important: NewX cannot copy the visual Python list order and
assume reverse after-model execution. The registered order must explicitly put
safety before subagent limiting and semantic loop accounting.

## 2. Explicit Conditional Eino Chain

The canonical declared order is now:

```text
reduction? -> filesystem? -> uploaded_files -> patch_tools ->
tool_error_normalization -> memory? -> skill? -> transcript? ->
summarization -> plan_task? -> provider_capability -> multimodalbudget ->
tool_search? -> contextbudget -> safety_finish -> subagent_limit? ->
semantic_loop
```

Question marks identify conditional handlers. Missing offload, Memory, Skill,
transcript, Plan, dynamic-tool or subagent capability now returns one explicit
not-applicable result and is omitted from both `Handlers` and `HandlerNames`.
A builder returning a nil handler without that result remains an assembly
error.

The former `agentsmd`, `policy`, `audit` and `usage` declarations were removed
because they were no-op placeholders rather than Eino middleware. Their actual
owners remain unchanged:

- stable Agent instructions: versioned Lead prompt composer;
- tool policy and guardrails: tool-provider/catalog wrappers;
- runtime audit events: concrete middleware and tool lifecycle sinks;
- usage attribution: model callbacks and run processor.

Tests now locate active middleware by `HandlerNames`; disabled capabilities can
no longer pass tests merely because a reserved handler occupied the old index.

## 3. Per-Response Subagent Limit

`ADKToolSet` carries internal-only normalized subagent tool names from
`ADKSubagentToolProvider` to `ApplicationADKAgentFactory`. Tool-policy filtering
removes metadata for filtered static tools, while the human-interaction wrapper
preserves the resolved tool-set metadata.

The limiter is installed only when subagents are enabled and at least one
allowed subagent tool exists. For the last assistant message it:

- counts only known subagent calls;
- retains the first configured two through four calls;
- preserves all ordinary tool calls and their original relative order;
- deep-clones returned messages/tool metadata and does not mutate input state;
- emits `subagent.tool_calls_truncated` with schema
  `coze.subagent_tool_limit.v1`, limit and requested/retained/dropped counts,
  plus at most 16 unique dropped tool names;
- never writes tool arguments or results to the event.

Safety suppression runs before this limiter. Semantic loop accounting receives
the already safe, bounded call set afterward.

## 4. Provider Capability Downgrade

Provider capability middleware now retains both requested and effective
reasoning options. If the selected model lacks requested thinking or reasoning
support, model execution continues with the effective downgraded options and
the middleware emits exactly one `model.capability_downgraded` event per Agent
instance using `sync.Once`.

The `coze.provider_capability_downgrade.v1` payload contains only:

- downgraded capability names;
- requested/effective `thinking_enabled` booleans;
- requested/effective reasoning-effort labels (`none`, `low`, `medium`,
  `high`).

The public run-event projector has an explicit case for this event and copies
only those fields. Prompt text, model bodies, provider responses, tool data and
credentials remain excluded. Other `model.*` events continue through the
existing redacted projection.

## 5. Verification

Passed:

```text
go test -gcflags='all=-N -l' ./application/agentthread -count=1
go test -gcflags='all=-N -l' ./application/... -count=1
go test -race -gcflags='all=-N -l' ./application/agentthread -run '^(TestADKProviderCapabilityMiddlewareEmitsDowngradeOnceConcurrently|TestADKSubagentLimitDropsOnlyExcessSubagentCalls|TestADKMiddlewareAssemblyPreservesHookAndWrapperOrder)$' -count=1
go vet ./application/...
go test -p 1 -gcflags='all=-N -l' ./... -count=1
APP_ENV=debug make build_server
git diff --check
```

The targeted race run emitted the known macOS malformed `LC_DYSYMTAB` linker
warning and returned exit code zero. `make build_server` also reported that
`goimports` was not installed; all touched Go files were formatted with
`gofmt`, and the build returned exit code zero.

## 6. Scope Boundary

This evidence closes `AR-PARITY-002.3`, not the complete Agent semantic core.
`AR-PARITY-002.4` still owns durable parity state across Agent rebuilds and
resume paths, including Todo, promoted tools, Skill activation, uploads,
Artifacts and interrupts. The capability-downgrade once guard is intentionally
per Agent instance in this slice; durable event deduplication belongs to that
next state boundary.
