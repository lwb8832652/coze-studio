# Eino ADK Multimodal History Budget M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve complete file and multimodal messages in Coze-owned state
while projecting bounded, deterministic model inputs for both normal Eino ADK
calls and summarization calls.

**Architecture:** Parse complete `schema.Message` values from run input and
retain them unchanged in checkpoints and transcript snapshots. Add independent
file-history and image/audio/video-history allocations to `ADKContextBudget`.
A Coze middleware projects a cloned message view immediately before each main
model call, while summarization uses the same projector through
`GenModelInput`. Selection is newest-first, the latest user turn is protected,
and an oversized latest turn fails before provider invocation. This slice does
not fetch files, write a filesystem, or register artifacts; those remain behind
the M2.9/M4 filesystem boundary.

**Tech Stack:** Go 1.24, Eino `v0.9.9` ADK handlers, Eino `schema.Message`,
Go testing, Testify.

---

## File Structure

- Modify `backend/application/agentthread/model_executor.go` to decode complete
  Eino message fields and accept attachment-only user messages.
- Modify `backend/application/agentthread/model_executor_test.go` for run-input
  compatibility contracts.
- Modify `backend/application/agentthread/adk_context_budget.go` for separate
  file and multimodal allocations plus deterministic modality estimates.
- Modify `backend/application/agentthread/adk_context_budget_test.go` for
  defaults, overrides, validation, and representation-independent estimates.
- Create `backend/application/agentthread/adk_multimodal_budget.go` for
  non-mutating message projection, the ADK model wrapper, protected-current-turn
  validation, and safe observability.
- Create `backend/application/agentthread/adk_multimodal_budget_test.go` for
  projection and wrapper contracts.
- Modify `backend/application/agentthread/adk_middleware.go` to install the
  middleware and project summarization model input.
- Modify `backend/application/agentthread/adk_middleware_test.go` and
  `backend/application/agentthread/adk_transcript_test.go` for full ADK
  integration, transcript preservation, and handler ordering.
- Modify
  `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md` after
  verification.

## Task 1: Complete Run Input Message Decoding

- [x] Add failing tests proving `parseModelExecutorMessages` preserves:
  - `UserInputMultiContent` image, audio, video, file, and text parts;
  - deprecated `MultiContent` for compatibility;
  - assistant output parts, reasoning, tool calls, and tool results;
  - attachment-only user messages without top-level text.
- [x] Replace the reduced input message DTO with complete
  `[]*schema.Message` decoding.
- [x] Normalize role and whitespace without deleting multimodal fields.
- [x] Keep rejecting inputs with no non-system text, media, file, tool call, or
  tool result content.
- [x] Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' \
  ./application/agentthread \
  -run 'TestParseModelExecutorMessages|TestModelExecutor.*Multimodal'
```

Expected: PASS.

## Task 2: File And Multimodal Budget Configuration

- [x] Add failing tests for defaults:

```go
MultimodalHistoryTokens: 16000
FileHistoryTokens:       8000
```

- [x] Add JSON overrides:

```json
{
  "context_budget": {
    "multimodal_history_tokens": 12000,
    "file_history_tokens": 6000
  }
}
```

- [x] Clamp unspecified defaults below the context window and reject explicit
  zero, negative, or context-window-sized allocations.
- [x] Introduce an `ADKMessageTokenEstimate` breakdown:

```go
type ADKMessageTokenEstimate struct {
	TextTokens       int
	MultimodalTokens int
	FileTokens       int
}
```

- [x] Count text and metadata deterministically without treating base64
  representation length as prompt text. Use conservative provider-neutral
  modality units:
  - image low: 256;
  - image auto/default: 768;
  - image high: 1536;
  - audio: 4096;
  - video: 8192;
  - file: 2048.
- [x] Continue including content, reasoning, tool calls, tool results, MIME
  type, filename, URL/URI, and JSON-compatible `Extra` metadata in the total
  estimate.
- [x] Run the context-budget package tests and verify the existing
  summarization trigger tests still pass.

## Task 3: Non-Mutating History Projection

- [x] Add failing tests for:
  - newest-first retention of historical media;
  - independent file and non-file budgets;
  - unconditional retention of every media/file part in the latest user turn;
  - explicit error when the protected latest turn alone exceeds either limit;
  - stable textual placeholders for omitted historical parts;
  - unchanged source messages, nested parts, URLs, and base64 payloads.
- [x] Implement:

```go
type ADKMultimodalProjection struct {
	Messages                  []*schema.Message
	KeptMultimodalParts       int
	OmittedMultimodalParts    int
	KeptFileParts             int
	OmittedFileParts          int
	EstimatedMultimodalTokens int
	EstimatedFileTokens       int
}

func projectADKMultimodalHistory(
	messages []*schema.Message,
	budget ADKContextBudget,
) (*ADKMultimodalProjection, error)
```

- [x] Clone message structs and part slices before replacement. Do not mutate
  source state or nested retained payloads.
- [x] Replace omitted parts with deterministic text markers that expose only
  the part type and budget reason, never URLs, filenames, base64, or `Extra`.
- [x] Add a typed protected-turn budget error containing category, estimate,
  limit, and part count without content.

## Task 4: Main Model And Summarization Integration

- [x] Add failing tests proving the middleware:
  - leaves `ChatModelAgentState.Messages` unchanged;
  - sends only the projected clone to `Generate` and `Stream`;
  - emits one `context.multimodal_pruned` event per logical model call, outside
    retry wrappers;
  - emits `context.multimodal_budget_exceeded` and stops before provider
    invocation for an oversized latest turn.
- [x] Implement `ADKMultimodalBudgetMiddleware`:
  - `BeforeModelRewriteState` analyzes final post-summary state, emits events,
    and returns the original state;
  - `WrapModel` applies projection to the actual provider input without
    emitting duplicate retry events.
- [x] Register `ADKMiddlewareMultimodalBudget` after summarization and reduction
  so normal calls inspect the final model-bound state.
- [x] Configure summarization `GenModelInput` to:
  - remove leading original system messages as Eino's default path does;
  - project original context through the same budget;
  - prepend Eino's summarization system instruction;
  - append Eino's summarization user instruction;
  - emit safe prune/exceeded events with phase `summarization`.
- [x] Keep the transcript callback on the complete pre-summary state.

## Task 5: Replay, Persistence, And Verification

- [x] Add ADK integration coverage with old media history, summarization, and
  interrupt/resume. Assert:
  - the normal and summarization models do not receive omitted payloads;
  - the current attachment remains;
  - the pre-summary snapshot retains original media for summarized calls;
  - the terminal snapshot retains original media when projection occurs
    without summarization;
  - checkpoint/resume preserves original source state and produces the same
    projection.
- [x] Run focused projection and replay tests 20 times.
- [x] Run the agentthread race suite.
- [x] Run:

```bash
cd backend
go mod verify
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./...
```

- [x] Run `git diff --check`.
- [x] Update the master roadmap: mark M2.7 file/multimodal history allocation
  complete, while keeping file fetch, provider capability negotiation,
  filesystem offload, artifact registration, scanning, and retention in their
  later milestones.
