# Agent Harness Core Loop Phase 17 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Continue the original Phase 14 Go-native Agent Harness work by adding a planner/step-runner core loop around the Phase 16 model executor.

**Architecture:** Keep the core loop in `backend/application/agentthread` as another `RunExecutor` implementation. `HarnessExecutor` owns bounded step execution, delegates planning through an `AgentPlanner`, delegates step execution through an `AgentStepRunner`, and defaults to a single model step backed by `ModelExecutor`.

**Tech Stack:** Go, existing `RunExecutor`, existing `ModelExecutor`, existing agentthread tests.

---

## Scope

This phase implements:

- `HarnessExecutor` as the default Go-native Agent Harness executor.
- `AgentPlanner`, `AgentPlan`, `AgentStep`, and `AgentStepRunner` interfaces/types.
- A default single-model-step planner.
- `ModelStepRunner` that wraps the Phase 16 `ModelExecutor`.
- Maximum step guard to prevent runaway loops.
- Context cancellation checks before planning and before every step.
- Worker startup wiring to use `HarnessExecutor`.

This phase does not implement:

- Tool or MCP step execution.
- Skills execution.
- Memory injection.
- Token usage persistence.
- SSE/WebSocket event streaming.
- LangGraph-compatible API.
- IM channel routing.
- Frontend execution-flow UI.

## Files

- Create `backend/application/agentthread/harness.go`: planner/step-runner interfaces, default planner, model step runner, harness executor.
- Create `backend/application/agentthread/harness_test.go`: RED/GREEN tests.
- Modify `backend/application/application.go`: pass the harness executor to controlled worker startup.
- Add `docs/superpowers/plans/2026-06-14-agent-harness-core-loop-phase17.md`.

## Task 1: Harness Executor Tests

**Files:**
- Create: `backend/application/agentthread/harness_test.go`

- [ ] **Step 1: Write failing final-step test**

Assert that a custom planner and custom step runner can execute one planned model step and return a final assistant message.

- [ ] **Step 2: Write failing bounded-loop test**

Assert that non-final step results are retried through planning until `MaxSteps` is reached, then return `agent harness exceeded max steps`.

- [ ] **Step 3: Write failing cancel test**

Assert that a canceled context returns `context canceled` before the planner or step runner is called.

- [ ] **Step 4: Write failing default-model-step test**

Assert that `NewHarnessExecutor(nil, nil, HarnessExecutorOptions{ModelProvider: fakeProvider})` uses the default single-model-step planner and returns the fake model response.

- [ ] **Step 5: Run tests to verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestHarnessExecutor'
```

Expected: FAIL because `NewHarnessExecutor`, `AgentPlanner`, `AgentStepRunner`, and related types do not exist.

## Task 2: Implement Core Loop

**Files:**
- Create: `backend/application/agentthread/harness.go`

- [ ] **Step 1: Define public contracts**

Add:

```go
type AgentStepType string

const AgentStepTypeModel AgentStepType = "model"

type AgentStep struct {
	ID   string
	Type AgentStepType
	Name string
}

type AgentPlan struct {
	Steps []AgentStep
}

type AgentStepResult struct {
	Message  string
	Metadata string
	Final    bool
}

type AgentHarnessState struct {
	StepIndex int
	Results   []AgentStepResult
}

type AgentPlanner interface {
	Plan(ctx context.Context, run *RunSummary, state AgentHarnessState) (*AgentPlan, error)
}

type AgentStepRunner interface {
	RunStep(ctx context.Context, run *RunSummary, step AgentStep, state AgentHarnessState) (*AgentStepResult, error)
}
```

- [ ] **Step 2: Add constructor and defaults**

Add:

```go
type HarnessExecutorOptions struct {
	MaxSteps     int
	ModelProvider ChatModelProvider
}

func NewHarnessExecutor(planner AgentPlanner, runner AgentStepRunner, opts HarnessExecutorOptions) *HarnessExecutor
```

Default `MaxSteps` to `4`, default planner to one model step, and default runner to `NewModelStepRunner(NewModelExecutor(opts.ModelProvider))`.

- [ ] **Step 3: Implement `Execute`**

Loop until a step result is final. Before planning and before every step, check `ctx.Err()`. Return a clear error when:

- planner returns no steps;
- step runner returns nil;
- final step returns an empty message;
- max steps is reached without a final result.

- [ ] **Step 4: Implement `ModelStepRunner`**

Wrap any `RunExecutor`, reject non-model steps, delegate to `Execute`, and convert `RunExecutionResult` to a final `AgentStepResult`.

- [ ] **Step 5: Run tests to verify GREEN**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestHarnessExecutor'
```

Expected: PASS.

## Task 3: Wire Controlled Worker

**Files:**
- Modify: `backend/application/application.go`

- [ ] **Step 1: Replace worker executor**

Change the existing startup hook to:

```go
agentthread.StartRunWorkerFromEnv(ctx, primaryServices.agentThreadSVC, agentthread.NewHarnessExecutor(nil, nil, agentthread.HarnessExecutorOptions{}))
```

The worker remains default-disabled. When enabled, pending runs now go through the harness core loop.

- [ ] **Step 2: Compile check**

Run:

```bash
cd backend
go test ./application/agentthread ./application -run 'TestHarnessExecutor|TestNonExistent'
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
git add docs/superpowers/plans/2026-06-14-agent-harness-core-loop-phase17.md \
  backend/application/agentthread/harness.go \
  backend/application/agentthread/harness_test.go \
  backend/application/application.go
git commit -m "feat: add agent harness core loop"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan continues the original Go-native Agent Harness core loop work.
- Scope control: It does not introduce MCP, skills, IM channels, frontend work, event streaming, or LangGraph compatibility.
- Type consistency: The harness remains a `RunExecutor`, so the existing `RunProcessor` and `RunWorker` contracts are unchanged.
