# Agent Memory Extractor Wiring And Usage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Wire the model-backed memory extractor into deployment configuration
and persist extractor model token usage as middleware usage metadata.

**Architecture:** Keep the memory extractor disabled by default. When
`AGENT_MEMORY_EXTRACTOR_ENABLED=true`, `InitService` creates a
`ModelMemoryExtractor` using the existing `DefaultChatModelProvider` and a
`ThreadUsageCollector`. The extractor records token usage from
`schema.ResponseMeta.Usage` as `TokenUsageSourceMiddleware` with metadata-only
attribution. Transcript text, extracted facts, and model output must not appear
in usage metadata.

**Tech Stack:** Go, Eino `schema.ResponseMeta.Usage`, existing token usage
collector, env-based service wiring.

---

### Task 1: Extractor Usage Attribution

**Files:**
- Modify: `backend/application/agentthread/memory_model_extractor.go`
- Modify: `backend/application/agentthread/memory_flush_processor_test.go`

- [x] **Step 1: Write failing test**

Add a test proving `ModelMemoryExtractor` records `middleware` token usage from
model response metadata and does not leak transcript/model output into usage
metadata.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestModelMemoryExtractorRecordsTokenUsageWithoutContentLeak' -count=1
```

Expected: build failure because `UsageCollector` is not part of extractor
options.

- [x] **Step 3: Implement attribution**

Add `UsageCollector` to extractor options and record prompt/completion/total,
cached, and reasoning token counts with bounded metadata.

### Task 2: Env Wiring

**Files:**
- Modify: `backend/application/agentthread/memory_model_extractor.go`
- Modify: `backend/application/agentthread/init.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `docker/.env.example`
- Modify: `docker/.env.debug.example`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Write failing tests**

Add tests for disabled-by-default env behavior, configured extractor options,
and `InitService` attaching the extractor when enabled.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestModelMemoryExtractorFromEnv|TestInitServiceConfiguresMemoryExtractorFromEnv' -count=1
```

Expected: build failure because env constants and constructor do not exist.

- [x] **Step 3: Implement env wiring**

Add `AGENT_MEMORY_EXTRACTOR_*` env parsing, attach the extractor in
`InitService`, and document disabled-by-default defaults.
