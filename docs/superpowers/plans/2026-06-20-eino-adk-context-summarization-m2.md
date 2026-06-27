# Eino ADK Context And Summarization M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add deterministic context budgets, bounded memory injection, and observable configuration-driven Eino summarization to the Go-native ADK runtime.

**Architecture:** Keep Eino's summarization middleware as the state-rewrite engine. Coze owns budget parsing, deterministic fallback token counting, memory selection, public event translation, and runtime wiring. This slice does not enable tool-result offloading because that requires the production sandbox/artifact backend planned in M4; reduction remains disabled until that backend exists.

**Tech Stack:** Go 1.24, Eino `v0.9.9` ADK middleware, Coze agentthread application services, Go testing, Testify.

---

## File Structure

- Create `backend/application/agentthread/adk_context_budget.go` for config parsing, validation, and deterministic token estimates.
- Create `backend/application/agentthread/adk_context_budget_test.go` for budget and counter contracts.
- Create `backend/application/agentthread/adk_memory_middleware.go` for bounded memory selection and instruction injection.
- Create `backend/application/agentthread/adk_memory_middleware_test.go` for deterministic selection and error behavior.
- Modify `backend/application/agentthread/adk_middleware.go` to build memory and summarization middleware from the parsed budget.
- Modify `backend/application/agentthread/adk_middleware_test.go` for runtime-configured summarization thresholds.
- Modify `backend/application/agentthread/adk_event_mapper.go` to translate Eino summarization actions into Coze events.
- Modify `backend/application/agentthread/adk_event_mapper_test.go` for before/generate/after summarization events.
- Modify `backend/application/application.go` to pass the existing thread memory provider into the ADK middleware assembler.

### Task 1: Deterministic Context Budget

**Files:**
- Create: `backend/application/agentthread/adk_context_budget.go`
- Create: `backend/application/agentthread/adk_context_budget_test.go`

- [x] **Step 1: Write the failing default and override tests**

```go
func TestADKContextBudgetDefaults(t *testing.T) {
	budget, err := adkContextBudgetFromRun(&RunSummary{})
	require.NoError(t, err)
	require.Equal(t, 120000, budget.SummarizationTokens)
	require.Equal(t, 200, budget.SummarizationMessages)
	require.Equal(t, 4000, budget.MemoryTokens)
}

func TestADKContextBudgetReadsRuntimeOverrides(t *testing.T) {
	budget, err := adkContextBudgetFromRun(&RunSummary{Config: `{
		"context_budget":{
			"context_window_tokens":64000,
			"summarization_tokens":48000,
			"summarization_messages":80,
			"memory_tokens":1200
		}
	}`})
	require.NoError(t, err)
	require.Equal(t, 64000, budget.ContextWindowTokens)
	require.Equal(t, 48000, budget.SummarizationTokens)
	require.Equal(t, 80, budget.SummarizationMessages)
	require.Equal(t, 1200, budget.MemoryTokens)
}

func TestADKContextBudgetRejectsInvalidRelationships(t *testing.T) {
	_, err := adkContextBudgetFromRun(&RunSummary{Config: `{
		"context_budget":{
			"context_window_tokens":32000,
			"summarization_tokens":40000
		}
	}`})
	require.ErrorContains(t, err, "summarization tokens")
}
```

- [x] **Step 2: Run the tests and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run TestADKContextBudget
```

Expected: FAIL because `adkContextBudgetFromRun` does not exist.

- [x] **Step 3: Implement config parsing and validation**

```go
type ADKContextBudget struct {
	ContextWindowTokens  int
	SummarizationTokens  int
	SummarizationMessages int
	MemoryTokens         int
}

func adkContextBudgetFromRun(run *RunSummary) (ADKContextBudget, error) {
	budget := ADKContextBudget{
		ContextWindowTokens:   128000,
		SummarizationTokens:   120000,
		SummarizationMessages: 200,
		MemoryTokens:          4000,
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return budget, nil
	}
	var payload struct {
		ContextBudget struct {
			ContextWindowTokens   int `json:"context_window_tokens"`
			SummarizationTokens   int `json:"summarization_tokens"`
			SummarizationMessages int `json:"summarization_messages"`
			MemoryTokens          int `json:"memory_tokens"`
		} `json:"context_budget"`
	}
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return ADKContextBudget{}, fmt.Errorf("decode adk context budget: %w", err)
	}
	// Apply positive overrides, then validate each limit and relationship.
	return budget, validateADKContextBudget(budget)
}
```

Validation must reject non-positive effective limits, summarization tokens at or above the context window, and a memory budget above the summarization threshold.

- [x] **Step 4: Add deterministic fallback token counting**

```go
func estimateADKTextTokens(value string) int {
	if value == "" {
		return 0
	}
	return (utf8.RuneCountInString(value) + 3) / 4
}

func estimateADKMessagesTokens(messages []*schema.Message, tools []*schema.ToolInfo) int {
	// Count role/content/reasoning/tool arguments deterministically and add
	// fixed per-message/per-tool overhead. Do not call a network tokenizer.
}
```

Add tests for ASCII, Chinese text, tool arguments, and repeatability.

- [x] **Step 5: Run tests and verify GREEN**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run 'TestADKContextBudget|TestEstimateADK'
```

Expected: PASS.

### Task 2: Bounded Memory Middleware

**Files:**
- Create: `backend/application/agentthread/adk_memory_middleware.go`
- Create: `backend/application/agentthread/adk_memory_middleware_test.go`
- Modify: `backend/application/agentthread/adk_middleware.go`

- [x] **Step 1: Write failing memory-selection tests**

```go
func TestADKMemoryMiddlewareInjectsWithinBudget(t *testing.T) {
	provider := &recordingMemoryProvider{memories: []AgentMemory{
		{ID: "1", Scope: "long_term", Content: "preferred fact", Score: 0.9},
		{ID: "2", Scope: "thread", Content: strings.Repeat("x", 200), Score: 0.5},
	}}
	mw := NewADKMemoryMiddleware(provider, ADKContextBudget{MemoryTokens: 8})
	runCtx := &adk.ChatModelAgentContext{Instruction: "base"}

	_, got, err := mw.BeforeAgent(context.Background(), runCtx)
	require.NoError(t, err)
	require.Contains(t, got.Instruction, "preferred fact")
	require.NotContains(t, got.Instruction, strings.Repeat("x", 200))
}
```

Also cover empty memory, provider failure, duplicate content, stable ordering, and nil run context.

- [x] **Step 2: Run and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run TestADKMemoryMiddleware
```

Expected: FAIL because the middleware does not exist.

- [x] **Step 3: Implement the middleware**

```go
type ADKMemoryMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run      *RunSummary
	provider MemoryProvider
	budget   ADKContextBudget
}
```

`BeforeAgent` must load memory once per run, normalize it with the existing `normalizeMemoryContext`, deduplicate by normalized content, consume the token budget in provider order, and append a deterministic `<memory_context>` block to `Instruction`. It must not put raw metadata, user IDs, or storage paths into the prompt.

- [x] **Step 4: Wire the memory builder**

Add `MemoryProvider MemoryProvider` to `ADKMiddlewareAssemblerOptions`. The default memory builder returns a reserved no-op only when the provider is nil; otherwise it builds `ADKMemoryMiddleware` using `input.Run` and `adkContextBudgetFromRun`.

- [x] **Step 5: Run tests and verify GREEN**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run 'TestADKMemoryMiddleware|TestADKMiddleware'
```

Expected: PASS.

### Task 3: Configuration-Driven Eino Summarization

**Files:**
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/adk_middleware_test.go`

- [x] **Step 1: Write the failing threshold test**

Build the assembler with a run config containing a five-message threshold, execute the summarization handler against six messages, and assert the summary model is called. Repeat with five messages and assert it is not called.

- [x] **Step 2: Run and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run TestADKMiddlewareUsesRunContextBudgetForSummarization
```

Expected: FAIL because the builder still hard-codes `120000/200`.

- [x] **Step 3: Build Eino summarization from the parsed budget**

```go
budget, err := adkContextBudgetFromRun(input.Run)
if err != nil {
	return nil, err
}
return summarization.New(ctx, &summarization.Config{
	Model: input.Model,
	TokenCounter: func(_ context.Context, input *summarization.TokenCounterInput) (int, error) {
		return estimateADKMessagesTokens(input.Messages, input.Tools), nil
	},
	Trigger: &summarization.TriggerCondition{
		ContextTokens:   budget.SummarizationTokens,
		ContextMessages: budget.SummarizationMessages,
	},
	EmitInternalEvents: true,
})
```

- [x] **Step 4: Keep reduction fail-closed**

Retain `SkipTruncation: true` and `SkipClear: true`. Add a test that the default production assembler does not offload to `/tmp` until a Coze filesystem backend is explicitly configured.

- [x] **Step 5: Run tests and verify GREEN**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run TestADKMiddleware
```

Expected: PASS.

### Task 4: Summarization Event Translation

**Files:**
- Modify: `backend/application/agentthread/adk_event_mapper.go`
- Modify: `backend/application/agentthread/adk_event_mapper_test.go`

- [x] **Step 1: Write failing event mapping tests**

Cover Eino `summarization.CustomizedAction` values:

```go
func TestMapADKEventMapsSummarizationActions(t *testing.T) {
	event := &adk.AgentEvent{Action: &adk.AgentAction{
		CustomizedAction: &summarization.CustomizedAction{
			Type: summarization.ActionTypeBeforeSummarize,
			Before: &summarization.BeforeSummarizeAction{
				Messages: []*schema.Message{schema.UserMessage("hello")},
			},
		},
	}}
	mapped, err := MapADKEvent(context.Background(), 10, 20, event)
	require.NoError(t, err)
	require.Equal(t, "context.summarizing", mapped.EventType)
	require.Contains(t, mapped.Payload, `"message_count":1`)
}
```

Map generate attempts to `context.summary_model_call` with phase, attempt, and normalized error; map completion to `context.summarized` with before/after message counts and summary content digest. Do not expose full transcript content in public event payloads.

- [x] **Step 2: Run and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run TestMapADKEventMapsSummarization
```

Expected: FAIL because customized actions currently fall through to `agent.event`.

- [x] **Step 3: Implement typed action mapping**

Add a type switch before generic action handling:

```go
if action, ok := event.Action.CustomizedAction.(*summarization.CustomizedAction); ok {
	return mapADKSummarizationAction(threadID, runID, base, action)
}
```

Use SHA-256 digests for transcript/summary correlation. Payloads contain counts, phase, attempt, digest, agent name, and run path only.

- [x] **Step 4: Run tests and verify GREEN**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread -run 'TestMapADKEvent|TestADKParity'
```

Expected: PASS.

### Task 5: Production Wiring And Verification

**Files:**
- Modify: `backend/application/application.go`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Wire the existing thread memory provider**

```go
agentthread.NewADKMiddlewareAssembler(agentthread.ADKMiddlewareAssemblerOptions{
	MemoryProvider: agentthread.NewThreadMemoryProvider(primaryServices.agentThreadSVC, 32),
})
```

The provider fetch limit is deliberately above the prompt budget so middleware budget selection, not database row count, determines prompt inclusion.

- [x] **Step 2: Add restart and replay integration coverage**

Execute an ADK run that triggers summarization, persist its checkpoint and events, construct a fresh executor, resume, and assert:

- context events remain ordered;
- summary callback usage is attributed to middleware;
- resumed execution does not re-inject duplicate memory blocks;
- final assistant output remains unchanged.

- [x] **Step 3: Run focused and full verification**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' ./application/agentthread ./domain/agentthread/... ./api/handler/coze
GOCACHE=/private/tmp/coze-go-build go test -race -gcflags='all=-l -N' ./application/agentthread -run 'TestADK(Context|Memory|Middleware|Parity)' -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./...
go mod verify
git diff --check
```

Expected: all commands succeed. The existing macOS `LC_DYSYMTAB` linker warning may appear during race linking but must not accompany a test failure.

- [x] **Step 4: Update roadmap evidence**

Mark only deterministic context budgeting and observable Eino summarization as complete. Keep reduction/offload and artifact registration open until the M4 filesystem and artifact backend is implemented.

## Exit Gate

1. Memory prompt injection is deterministic, token-bounded, and free of raw metadata.
2. Summarization thresholds come from validated run configuration.
3. Eino remains the summarization engine.
4. Summarization internal actions become stable Coze events without transcript leakage.
5. Summary model usage is attributed as middleware usage.
6. Checkpoint/resume preserves summarized state and does not duplicate memory injection.
7. Reduction/offload remains disabled without an explicit Coze filesystem backend.
8. Full backend tests pass with Mockey-compatible compiler flags.
