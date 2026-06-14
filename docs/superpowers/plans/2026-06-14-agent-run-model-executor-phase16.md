# Agent Run Model Executor Phase 16 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Continue the original Phase 14 Go-native Agent Harness work by adding the first real model execution boundary for pending task runs.

**Architecture:** Keep the executor in `backend/application/agentthread` so it plugs into the existing `RunProcessor` and `RunWorker`. The executor parses run input/config, resolves an Eino chat model through Coze Studio model configuration, calls `Generate`, and returns a `RunExecutionResult` that the processor already persists as an assistant message.

**Tech Stack:** Go, Eino `model.BaseChatModel`, existing `modelbuilder` configuration, existing agentthread tests.

---

## Scope

This phase implements:

- `ModelExecutor` as a `RunExecutor` implementation.
- Run input parsing for `{"messages":[{"role":"user","content":"..."}]}` and compatibility with simple `{"message":"..."}` payloads.
- Run config parsing for model selection and generation options:
  - `model_id`, `modelId`, `model_type`, `modelType`;
  - `model_name`, `modelName`;
  - `temperature`, `max_tokens`, `maxTokens`, `top_p`, `topP`;
  - `system_prompt`, `systemPrompt`.
- A default model provider using `modelbuilder.BuildModelByID` when a model ID exists, otherwise `modelbuilder.GetBuiltinChatModel`.
- Worker startup wiring with the real executor while retaining the existing default-disabled env gate.

This phase does not implement:

- Planner/step runner loop.
- Tool or MCP invocation.
- Skills execution.
- Memory injection.
- SSE/WebSocket event streaming.
- LangGraph-compatible API.
- IM channel routing.
- Token usage persistence.

## Files

- Create `backend/application/agentthread/model_executor.go`: executor, input/config parsing, model provider.
- Create `backend/application/agentthread/model_executor_test.go`: RED/GREEN tests.
- Modify `backend/application/application.go`: pass the model executor to the controlled worker startup.
- Add `docs/superpowers/plans/2026-06-14-agent-run-model-executor-phase16.md`.

## Task 1: Model Executor Tests

**Files:**
- Create: `backend/application/agentthread/model_executor_test.go`

- [ ] **Step 1: Write failing Generate test**

Create a fake Eino chat model and provider. Assert that `ModelExecutor.Execute`:

- parses `run.Input` messages in order;
- prepends `system_prompt` from `run.Config`;
- resolves `model_id`;
- passes `model_name`, `temperature`, `max_tokens`, and `top_p` as Eino model options;
- returns the model response as `RunExecutionResult.Message`.

- [ ] **Step 2: Write failing compatibility and validation tests**

Assert:

- `{"message":"..."}` input becomes one user message;
- blank/no messages returns a clear error;
- provider/model errors propagate to the processor failure path.

- [ ] **Step 3: Run tests to verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestModelExecutor'
```

Expected: FAIL because `ModelExecutor` does not exist.

## Task 2: Implement Model Executor

**Files:**
- Create: `backend/application/agentthread/model_executor.go`

- [ ] **Step 1: Define provider and executor**

Add:

```go
type ChatModelProvider func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error)

func DefaultChatModelProvider(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error)
func NewModelExecutor(provider ChatModelProvider) *ModelExecutor
func (e *ModelExecutor) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error)
```

- [ ] **Step 2: Parse input and config**

Support message roles `system`, `user`, `assistant`, and `tool`. Ignore blank messages. Reject unsupported roles and empty final message lists.

- [ ] **Step 3: Call Eino Generate**

Resolve the model with the configured model ID, pass generation options, call `Generate`, trim the response, and return metadata with `source`, `model_id`, and `model_name` when present.

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestModelExecutor'
```

Expected: PASS.

## Task 3: Wire Controlled Worker

**Files:**
- Modify: `backend/application/application.go`

- [ ] **Step 1: Replace nil executor startup**

Change the existing safe startup hook to:

```go
agentthread.StartRunWorkerFromEnv(ctx, primaryServices.agentThreadSVC, agentthread.NewModelExecutor(nil))
```

The worker remains default-disabled. When explicitly enabled, it consumes runs through the model executor.

- [ ] **Step 2: Compile check**

Run:

```bash
cd backend
go test ./application/agentthread ./application -run 'TestModelExecutor|TestNonExistent'
```

Expected: PASS.

## Task 4: Final Verification and Commit

**Files:**
- All files above.

- [ ] **Step 1: Run focused verification**

Run:

```bash
cd backend
go test -count=1 ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository
go test ./application -run TestNonExistent
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

Run:

```bash
git add docs/superpowers/plans/2026-06-14-agent-run-model-executor-phase16.md \
  backend/application/agentthread/model_executor.go \
  backend/application/agentthread/model_executor_test.go \
  backend/application/application.go
git commit -m "feat: add agent run model executor"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan stays on the original Go-native Agent Harness model execution path.
- Scope control: No MCP, skills, IM channel, frontend, SSE, or LangGraph compatibility work is included.
- Type consistency: The executor implements the existing `RunExecutor` contract and reuses Coze Studio Eino model infrastructure.
