# Agent Memory Model Extractor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Add a production-shaped model-backed `MemoryExtractor` that reuses the
existing Eino `model.BaseChatModel` provider path and returns structured facts
for the memory flush worker.

**Architecture:** Keep extraction and persistence separate. The model extractor
receives transcript snapshot metadata, calls a configured chat model, requires
JSON output in a narrow `facts` schema, normalizes bounded fact fields, and
returns `MemoryExtractionFact` values. The worker remains responsible for
idempotency checks, persistence, lease transitions, and content-free events.

**Tech Stack:** Go, Eino `model.BaseChatModel`, existing `ChatModelProvider`.

---

### Task 1: Model Extractor Contract

**Files:**
- Add: `backend/application/agentthread/memory_model_extractor.go`
- Add: `backend/application/agentthread/memory_flush_processor_test.go`

- [x] **Step 1: Write failing tests**

Add tests proving a fake Eino chat model can return structured JSON facts that
become `MemoryExtractionFact` values, and invalid model JSON is rejected.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestModelMemoryExtractor' -count=1
```

Expected: build failure because the model extractor constructor and options do
not exist yet.

- [x] **Step 3: Implement model extractor**

Add `ModelMemoryExtractor`, `ModelMemoryExtractorOptions`, prompt construction,
model option wiring, JSON/fenced-JSON parsing, fact count bounds, scope
normalization, metadata normalization, and score/confidence clamping.

- [x] **Step 4: Verify GREEN**

Run the focused command again and expect PASS.

### Remaining Work

- Wire the extractor into deployment configuration when the memory flush worker
  is intentionally enabled and model settings are available.
- Add callback usage/token attribution for extractor model calls.
- Add extraction prompt evaluation fixtures and browser/admin observability.
