# Eino ADK Skill And Tool Budget M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace eager Skill prompt concatenation on the ADK path with the
Eino Skill middleware and enforce deterministic context budgets after dynamic
tool search.

**Architecture:** Coze remains the Skill catalog and version system of record.
The runtime adapter resolves the run-selected, enabled Skill snapshots once,
then exposes them through Eino's `skill.Backend`. Eino owns progressive
discovery and invocation. Coze owns tenant filtering, budget validation,
frontmatter compatibility, and fail-closed tool-definition limits. This slice
supports production inline Skill execution. Forked Skill execution remains
blocked until the M3 AgentHub, tool-policy intersection, and durable child-run
contracts are available.

**Tech Stack:** Go 1.24, Eino `v0.9.9` ADK Skill and dynamic-tool middleware,
Coze Skill domain service, Go testing, Testify.

---

## Task 1: Preserve Eino Skill Frontmatter

**Files:**
- Modify: `backend/domain/skill/service/declaration.go`
- Modify: `backend/domain/skill/service/declaration_test.go`
- Modify: `backend/application/agentthread/harness.go`
- Modify: `backend/application/agentthread/skill_provider.go`
- Modify: `backend/application/agentthread/skill_provider_test.go`

- [x] Add failing parser tests for `context`, `agent`, and `model`.
- [x] Extend the declaration and runtime snapshot without changing the legacy
  Harness prompt behavior.
- [x] Validate context values as `inline`, `fork`, or `fork_with_context`.
- [x] Verify selected Skill snapshots preserve the execution metadata.

## Task 2: Coze-Backed Eino Skill Middleware

**Files:**
- Create: `backend/application/agentthread/adk_skill_backend.go`
- Create: `backend/application/agentthread/adk_skill_backend_test.go`
- Modify: `backend/application/agentthread/adk_context_budget.go`
- Modify: `backend/application/agentthread/adk_context_budget_test.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/application.go`

- [x] Add `skill_catalog_tokens` and `skill_content_tokens` run-config limits.
- [x] Add failing tests for stable ordering, duplicate names, catalog overflow,
  body overflow, unknown Skill lookup, and exact run selection.
- [x] Build an immutable Eino `skill.Backend` from the run-selected snapshots.
- [x] Use Eino `skill.NewMiddleware` with a bounded catalog description and
  Coze-owned inline content formatting.
- [x] Keep empty selections as a no-op and fail explicitly when a selected
  forked Skill is invoked before AgentHub is configured.
- [x] Wire the existing `RuntimeSkillProvider` into the production ADK
  assembler.
- [x] Add a real Eino agent test proving the first model request contains only
  Skill metadata, the Skill tool loads the body, and the next request contains
  the loaded instructions.

## Task 3: Tool Definition Budget After Dynamic Search

**Files:**
- Create: `backend/application/agentthread/adk_tool_budget_middleware.go`
- Create: `backend/application/agentthread/adk_tool_budget_middleware_test.go`
- Modify: `backend/application/agentthread/adk_context_budget.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/adk_middleware_test.go`

- [x] Add the `tool_definition_tokens` run-config limit.
- [x] Add a context-budget middleware after Eino tool search and before
  summarization.
- [x] Count both `ToolInfos` and provider-native `DeferredToolInfos`.
- [x] Fail closed when initial static definitions, promoted client-search
  definitions, or provider-native deferred definitions exceed the limit.
- [x] Reorder state-rewrite handlers so summarization counts the post-search
  model-visible tool set instead of the entire unresolved catalog.
- [x] Verify client-side deferred tools remain hidden until promoted.

## Task 4: Verification And Roadmap

- [x] Run focused parser, Skill backend, middleware, and end-to-end tests.
- [x] Repeat checkpoint/summary/Skill tests to detect state leakage.
- [x] Run the agentthread race suite.
- [x] Run `go mod verify`, `git diff --check`, and the full backend suite.
- [x] Update the master roadmap with completed and intentionally deferred M2.7
  scope.
