# Agent Memory Retrieval Budget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Add the M7.3 context-aware memory retrieval boundary while keeping
ADK memory injection as the final token-budget control point.

**Architecture:** Extend `ThreadMemoryProvider` instead of introducing a
parallel retrieval system. The provider reads optional run config
`memory_retrieval`, requests a bounded candidate set, filters unsafe or stale
facts, and returns a smaller deterministic memory set for the existing ADK
memory middleware to inject under its token budget.

**Tech Stack:** Go, existing AgentThread memory provider, existing ADK memory
middleware.

---

### Task 1: Retrieval Contract

**Files:**
- Modify: `backend/application/agentthread/memory_provider.go`
- Modify: `backend/application/agentthread/memory_provider_test.go`

- [x] **Step 1: Write failing test**

Add a provider test proving `memory_retrieval.limit`,
`memory_retrieval.candidate_limit`, `memory_retrieval.scopes`, and
`memory_retrieval.min_confidence` are applied, and that corrected facts are
hidden in favor of the correction.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestThreadMemoryProviderAppliesContextAwareRetrievalConfig|TestThreadMemoryProviderRecallsApplicationMemories' -count=1
```

Expected: failure because the provider still used the constructor limit and
did not apply retrieval config.

- [x] **Step 3: Implement config-driven retrieval**

Parse `memory_retrieval` from `RunSummary.Config`, derive a query from the
latest user message when no explicit query is provided, request candidates
with `candidate_limit`, validate scopes and confidence thresholds, remove
corrected and low-confidence memories, rank by lexical query overlap and
confidence-weighted score, then return `limit` memories.

- [x] **Step 4: Verify GREEN**

Run the focused command again and expect PASS.

### Task 2: Context And Guardrails

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Document runtime boundary**

Record that M7.3 is a deterministic provider-side retrieval boundary. ADK
memory middleware remains the final token-budget injection layer.

- [x] **Step 2: Preserve open scope**

Keep vector retrieval, embeddings, semantic ranking, extraction/upsert workers,
UI memory management, audit views, and browser E2E explicitly outside this
slice so they can be implemented as later M7 work.
