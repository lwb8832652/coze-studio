# Eino ADK Runtime M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace new Agent Harness execution behavior with a Coze-owned adapter over Eino ADK while preserving durable Coze run, event, checkpoint, resume, usage, and LangGraph contracts.

**Architecture:** Keep the existing `agentthread.RunExecutor` and `ResumeRunExecutor` boundaries so workers and run state machines remain stable. Add an ADK executor beside the legacy Harness, translate Eino events and checkpoints into Coze-owned DTOs, and select the runtime per run through a feature-gated resolver. Build middleware and tool integration only after the base event/checkpoint/resume contract passes.

**Tech Stack:** Go 1.24, Eino `v0.9.9`, Eino ADK, Hertz application services, MySQL-backed agentthread repositories, Go testing, Testify.

---

## File Structure

Create focused files in the existing `backend/application/agentthread` package:

- `runtime_selector.go`: runtime mode parsing and legacy/ADK executor selection.
- `adk_executor.go`: Eino Runner construction and run/resume entry points.
- `adk_agent_factory.go`: ChatModelAgent construction from Coze run configuration.
- `adk_event_mapper.go`: Eino `AgentEvent` to durable Coze event conversion.
- `adk_checkpoint.go`: versioned checkpoint envelope and Eino CheckPointStore.
- `adk_cancel.go`: active Eino cancellation registration and command bridge.
- `adk_usage.go`: callback and response usage attribution.
- `adk_middleware.go`: ordered Eino middleware assembly.
- `adk_turn_loop.go`: follow-up input and preemption integration.

Keep legacy `harness.go` unchanged except for compatibility fixes. New runtime
behavior must enter through the ADK files.

### Task 1: Runtime Selector And Feature Gate

**Files:**
- Create: `backend/application/agentthread/runtime_selector.go`
- Create: `backend/application/agentthread/runtime_selector_test.go`
- Modify: `backend/application/application.go`

- [x] **Step 1: Write the failing selector tests**

```go
func TestRuntimeModeFromRunDefaultsToLegacy(t *testing.T) {
	mode, err := runtimeModeFromRun(&RunSummary{})
	require.NoError(t, err)
	require.Equal(t, RuntimeModeLegacy, mode)
}

func TestRuntimeModeFromRunSelectsADK(t *testing.T) {
	mode, err := runtimeModeFromRun(&RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeModeEinoADK, mode)
}

func TestRuntimeSelectorUsesConfiguredExecutor(t *testing.T) {
	legacy := RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "legacy"}, nil
	})
	adkExecutor := RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "adk"}, nil
	})
	selector := NewRuntimeSelector(legacy, adkExecutor)

	result, err := selector.Execute(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "adk", result.Message)
}
```

- [x] **Step 2: Run the selector tests and verify failure**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestRuntime'
```

Expected: FAIL because `RuntimeMode` and `NewRuntimeSelector` do not exist.

- [x] **Step 3: Implement the selector**

```go
type RuntimeMode string

const (
	RuntimeModeLegacy  RuntimeMode = "legacy"
	RuntimeModeEinoADK RuntimeMode = "eino_adk"
)

type RuntimeSelector struct {
	legacy RunExecutor
	adk    RunExecutor
}

func NewRuntimeSelector(legacy, adkExecutor RunExecutor) *RuntimeSelector {
	return &RuntimeSelector{legacy: legacy, adk: adkExecutor}
}

func (s *RuntimeSelector) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
	mode, err := runtimeModeFromRun(run)
	if err != nil {
		return nil, err
	}
	if mode == RuntimeModeEinoADK {
		if s.adk == nil {
			return nil, fmt.Errorf("eino adk runtime is not configured")
		}
		return s.adk.Execute(ctx, run)
	}
	if s.legacy == nil {
		return nil, fmt.Errorf("legacy runtime is not configured")
	}
	return s.legacy.Execute(ctx, run)
}
```

Parse `runtime` from the existing run config JSON. Reject unknown non-empty
values instead of silently falling back.

- [x] **Step 4: Add resume selection**

Add a matching `RuntimeResumeSelector` implementing `ResumeRunExecutor`.
Until Task 3 adds the source checkpoint envelope, resume uses the run config.
Task 3 must change the precedence to checkpoint runtime first, with run config
used only for legacy checkpoints.

- [x] **Step 5: Wire selectors without enabling ADK by default**

In `backend/application/application.go`, construct:

```go
legacyExecutor := agentthread.NewApplicationHarnessExecutor(
	primaryServices.agentThreadSVC,
	agentthread.NewRuntimeSkillProvider(primaryServices.skillSVC.DomainSVC),
)
runtimeExecutor := agentthread.NewRuntimeSelector(legacyExecutor, nil)
```

Keep the current default behavior until the ADK executor is implemented.

- [x] **Step 6: Run tests**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/application/agentthread/runtime_selector.go backend/application/agentthread/runtime_selector_test.go backend/application/application.go
git commit -m "feat: add agent runtime selector"
```

### Task 2: Eino Agent Event Mapping

**Files:**
- Create: `backend/application/agentthread/adk_event_mapper.go`
- Create: `backend/application/agentthread/adk_event_mapper_test.go`

- [x] **Step 1: Write failing mapping tests**

Cover:

1. assistant message chunks;
2. tool result events with tool name;
3. reasoning content;
4. interrupt contexts;
5. retry/failover errors;
6. terminal errors;
7. subagent names and run path.

```go
func TestMapADKEventMapsAssistantMessage(t *testing.T) {
	event := &adk.AgentEvent{
		AgentName: "lead",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: schema.AssistantMessage("hello", nil),
				Role:    schema.Assistant,
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)
	require.NoError(t, err)
	require.Equal(t, "message.completed", mapped.EventType)
	require.JSONEq(t, `{"agent_name":"lead","role":"assistant","content":"hello"}`, mapped.Payload)
}
```

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestMapADKEvent'
```

Expected: FAIL because the mapper does not exist.

- [x] **Step 3: Implement a normalized mapping DTO**

```go
type ADKEventMapping struct {
	EventType string
	Payload   string
	Usage     *AgentTokenUsage
	FinalText string
	Interrupt *ADKInterruptMapping
}

type ADKInterruptMapping struct {
	Items []ADKInterruptItem `json:"items"`
}

type ADKInterruptItem struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	Info        any    `json:"info,omitempty"`
	IsRootCause bool   `json:"is_root_cause"`
}
```

Consume streaming message readers exactly once. Preserve tool call IDs,
reasoning parts, media metadata, finish reason, model usage, Agent name, and
run path in the payload.

- [x] **Step 4: Normalize errors**

Map:

- `*adk.WillRetryError` to `model.retrying`;
- `*adk.RetryExhaustedError` to `model.retry_exhausted`;
- `*adk.CancelError` to `run.canceling`;
- interrupt actions to `run.interrupted`;
- other errors to `run.runtime_error`.

- [x] **Step 5: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestMapADKEvent'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/application/agentthread/adk_event_mapper.go backend/application/agentthread/adk_event_mapper_test.go
git commit -m "feat: map eino agent events"
```

### Task 3: Versioned ADK Checkpoint Envelope

**Files:**
- Create: `backend/application/agentthread/adk_checkpoint.go`
- Create: `backend/application/agentthread/adk_checkpoint_test.go`
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/checkpoint_sink.go`
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Create: `docker/atlas/migrations/20260619000100_agent_checkpoint_runtime_keys.sql`

- [x] **Step 1: Write envelope round-trip tests**

```go
func TestADKCheckpointEnvelopeRoundTrip(t *testing.T) {
	input := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
		RunRevision:     4,
	}

	raw, err := input.Marshal()
	require.NoError(t, err)

	output, err := UnmarshalADKCheckpointEnvelope(raw)
	require.NoError(t, err)
	require.Equal(t, input, output)
}
```

Add rejection tests for unknown envelope versions, missing runtime version,
empty checkpoint bytes, and oversized payloads.

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKCheckpoint'
```

Expected: FAIL.

- [x] **Step 3: Implement the envelope**

```go
type ADKCheckpointEnvelope struct {
	EnvelopeVersion int                         `json:"envelope_version"`
	Runtime         string                      `json:"runtime"`
	RuntimeVersion  string                      `json:"runtime_version"`
	MessageType     string                      `json:"message_type"`
	Checkpoint      []byte                      `json:"checkpoint_bytes"`
	Interrupts      map[string]ADKInterruptItem `json:"interrupts,omitempty"`
	RunRevision     int64                       `json:"run_revision"`
	CreatedAt       int64                       `json:"created_at"`
	Migration       map[string]string           `json:"migration,omitempty"`
}
```

Use JSON only for the Coze envelope. Treat `Checkpoint` as opaque bytes.
Enforce a configurable maximum before database persistence.

- [x] **Step 4: Implement an Eino CheckPointStore**

```go
type ADKCheckpointStore struct {
	app *ApplicationService
	run *RunSummary
}

func (s *ADKCheckpointStore) Set(ctx context.Context, key string, value []byte) error
func (s *ADKCheckpointStore) Get(ctx context.Context, key string) ([]byte, bool, error)
```

Persist the envelope through `CreateCheckpoint`. Store the envelope JSON in
`ChannelValues`, `{}` in `ChannelVersions`, `[]` in `PendingSends`, and
runtime metadata in `Metadata`.

- [x] **Step 5: Add indexed runtime checkpoint identity**

Add these fields to `agent_checkpoints`:

```sql
ALTER TABLE `agent_checkpoints`
  ADD COLUMN `runtime_type` varchar(32) NOT NULL DEFAULT 'legacy',
  ADD COLUMN `runtime_key` varchar(255) NOT NULL DEFAULT '',
  ADD COLUMN `envelope_version` int NOT NULL DEFAULT 0,
  ADD COLUMN `runtime_deleted_at` bigint NOT NULL DEFAULT 0,
  ADD KEY `idx_agent_checkpoints_runtime_key`
    (`thread_id`, `run_id`, `runtime_type`, `runtime_key`, `created_at`);
```

Add repository methods:

```go
GetLatestRuntimeCheckpoint(
	ctx context.Context,
	threadID, runID int64,
	runtimeType, runtimeKey string,
) (*entity.Checkpoint, error)

DeleteRuntimeCheckpoint(
	ctx context.Context,
	threadID, runID int64,
	runtimeType, runtimeKey string,
	deletedAt int64,
) error
```

`Set` remains append-only for audit and resume history. `Get` returns the latest
non-deleted row for the indexed runtime key. `Delete` marks all active rows for
that key deleted without removing historical checkpoint data.

- [x] **Step 6: Implement Eino deletion semantics**

Implement `adk.CheckPointDeleter` through `DeleteRuntimeCheckpoint`. A clean
Eino completion must make the active runtime key unavailable to subsequent
resume calls while retaining its rows for audit and retention jobs.

- [x] **Step 7: Add legacy separation**

`loadHarnessResumeInput` must continue reading legacy Harness checkpoints.
Create a separate `loadADKResumeInput` that recognizes
`metadata.runtime == "eino_adk"` and decodes only the ADK envelope.

- [x] **Step 8: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'Checkpoint'
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add backend/application/agentthread/adk_checkpoint.go backend/application/agentthread/adk_checkpoint_test.go backend/application/agentthread/dto.go backend/application/agentthread/checkpoint_sink.go backend/domain/agentthread docker/atlas/migrations/20260619000100_agent_checkpoint_runtime_keys.sql
git commit -m "feat: persist eino checkpoint envelopes"
```

### Task 4: Minimal ADK Agent Factory

**Files:**
- Create: `backend/application/agentthread/adk_agent_factory.go`
- Create: `backend/application/agentthread/adk_agent_factory_test.go`
- Modify: `backend/application/agentthread/model_executor.go`

- [x] **Step 1: Write failing factory tests**

Verify:

- model resolution uses the existing `ChatModelProvider`;
- system prompt, model options, and max iterations are preserved;
- runtime uses `*schema.Message`;
- no tools produces a valid ChatModelAgent;
- unknown model configuration fails before execution.

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKAgentFactory'
```

Expected: FAIL.

- [x] **Step 3: Introduce the factory contract**

```go
type ADKAgentFactory interface {
	Build(ctx context.Context, run *RunSummary) (adk.ResumableAgent, error)
}

type ApplicationADKAgentFactory struct {
	modelProvider ChatModelProvider
	toolProvider  ADKToolProvider
	middlewares   ADKMiddlewareFactory
}
```

- [x] **Step 4: Build ChatModelAgent**

Use:

```go
adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
	Name:          "lead",
	Description:   "Coze task lead agent",
	Instruction:   cfg.SystemPrompt,
	Model:         chatModel,
	ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{ToolList: tools}},
	MaxIterations: cfg.MaxIterations,
	Handlers:      handlers,
	ModelRetryConfig: retryConfig,
	ModelFailoverConfig: failoverConfig,
})
```

Move shared run-config parsing out of `model_executor.go` instead of
duplicating model option rules.

- [x] **Step 5: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKAgentFactory'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/application/agentthread/adk_agent_factory.go backend/application/agentthread/adk_agent_factory_test.go backend/application/agentthread/model_executor.go
git commit -m "feat: build eino chat model agents"
```

### Task 5: ADK Run And Resume Executor

**Files:**
- Create: `backend/application/agentthread/adk_executor.go`
- Create: `backend/application/agentthread/adk_executor_test.go`
- Modify: `backend/application/agentthread/resume_runner.go`

- [x] **Step 1: Write a streaming execution test**

Use a fake resumable Agent emitting:

1. assistant stream chunks;
2. a tool event;
3. a final assistant message.

Assert durable events are emitted in order and `RunExecutionResult.Message`
contains the final materialized text.

- [x] **Step 2: Write an interrupt/resume test**

The first execution emits an interrupt and writes an ADK checkpoint. Resume
loads the envelope and calls `Runner.ResumeWithParams` using the persisted
target mapping.

- [x] **Step 3: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKExecutor'
```

Expected: FAIL.

- [x] **Step 4: Implement the executor**

```go
type ADKExecutor struct {
	factory         ADKAgentFactory
	eventSink       RunEventSink
	checkpointStoreFactory func(run *RunSummary) adk.CheckPointStore
	usageCollector  UsageCollector
	cancelRegistry  *ADKCancelRegistry
}
```

For a new run:

1. parse Coze messages into `[]*schema.Message`;
2. build the Agent;
3. create `adk.Runner` with streaming and checkpoint store;
4. call `Runner.Run` with `adk.WithCheckPointID`;
5. drain every event through `MapADKEvent`;
6. persist mapped events and usage;
7. return the final assistant text.

- [x] **Step 5: Implement resume**

Add:

```go
func (e *ADKExecutor) Resume(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error)
```

Replace `HarnessResumeInput` with a small discriminated resume DTO or add an
ADK-specific field so legacy state and ADK envelope data cannot be mixed.

- [x] **Step 6: Preserve interrupted state**

An Eino interrupt is not a failed run. Return a typed
`RunInterruptedError` carrying Coze interrupt IDs and checkpoint identity.
Update the processor in a later task to transition the run to `interrupted`
instead of `failed`.

- [x] **Step 7: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKExecutor'
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/application/agentthread/adk_executor.go backend/application/agentthread/adk_executor_test.go backend/application/agentthread/resume_runner.go
git commit -m "feat: execute and resume eino agents"
```

### Task 6: Run State Machine Support For Interrupt And Active Cancel

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/service/state_machine.go`
- Modify: `backend/domain/agentthread/service/state_machine_test.go`
- Modify: `backend/application/agentthread/runner.go`
- Modify: `backend/application/agentthread/resume_runner.go`
- Create: `backend/application/agentthread/adk_cancel.go`
- Create: `backend/application/agentthread/adk_cancel_test.go`

- [x] **Step 1: Add failing state transition tests**

Add `RunStatusInterrupted` and verify:

- running to interrupted is allowed;
- interrupted to queued resume is allowed;
- interrupted to canceled is allowed;
- interrupted to succeeded is rejected without resume.

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./domain/agentthread/service ./application/agentthread -run 'Interrupt|Cancel'
```

Expected: FAIL.

- [x] **Step 3: Implement the active cancel registry**

```go
type ADKCancelRegistry struct {
	mu      sync.Mutex
	handles map[int64]adk.AgentCancelFunc
}

func (r *ADKCancelRegistry) Register(runID int64, fn adk.AgentCancelFunc) func()
func (r *ADKCancelRegistry) Cancel(ctx context.Context, runID int64, mode adk.CancelMode, recursive bool) error
```

The unregister function must remove only the matching execution generation so
a stale worker cannot unregister a newer retry.

- [x] **Step 4: Pass Eino cancel options to Runner**

Use:

```go
cancelOption, cancelFn := adk.WithCancel()
cleanup := registry.Register(run.RunID, cancelFn)
defer cleanup()
iter := runner.Run(ctx, messages, cancelOption, adk.WithCheckPointID(checkpointID))
```

- [x] **Step 5: Bridge application cancel commands**

When `CancelRun` succeeds for an active ADK run, call the registry with:

```go
adk.CancelAfterToolCalls | adk.CancelAfterChatModel
```

Use `adk.WithRecursive()` and a bounded timeout that escalates to immediate
cancel.

- [x] **Step 6: Handle interrupt separately in processors**

`RunProcessor` and `ResumeRunProcessor` should detect
`RunInterruptedError`, persist `run.interrupted`, and transition the run to
`interrupted` without appending an empty assistant message.

- [x] **Step 7: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./domain/agentthread/service ./application/agentthread -run 'Interrupt|Cancel'
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/application/agentthread backend/domain/agentthread/entity/thread.go backend/domain/agentthread/service/state_machine.go backend/domain/agentthread/service/state_machine_test.go
git commit -m "feat: support eino interrupt and active cancel"
```

### Task 7: Eino Usage And Callback Bridge

**Files:**
- Create: `backend/application/agentthread/adk_usage.go`
- Create: `backend/application/agentthread/adk_usage_test.go`
- Modify: `backend/application/agentthread/usage_collector.go`

- [x] **Step 1: Write failing usage attribution tests**

Verify:

- lead Agent usage;
- subagent usage based on Agent name/run path;
- summarization usage as middleware;
- cached and reasoning tokens preserved in `RawUsage`;
- retried attempts are recorded once per provider call;
- final aggregate is not double-counted from both event metadata and callback.

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKUsage'
```

Expected: FAIL.

- [x] **Step 3: Implement an idempotency key**

Derive a stable key from:

```text
run_id + agent_name + model_call_id + retry_attempt + usage_kind
```

Persist it in usage metadata. Add a database uniqueness constraint before ADK
is enabled for production.

- [x] **Step 4: Build Eino callbacks**

Use `callbacks.NewHandlerBuilder` to observe ChatModel start/end/error and Agent
boundaries. Record raw provider usage through `ThreadUsageCollector`.

- [x] **Step 5: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKUsage'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/application/agentthread/adk_usage.go backend/application/agentthread/adk_usage_test.go backend/application/agentthread/usage_collector.go
git commit -m "feat: collect eino runtime usage"
```

### Task 8: Ordered Eino Middleware Assembly

**Files:**
- Create: `backend/application/agentthread/adk_middleware.go`
- Create: `backend/application/agentthread/adk_middleware_test.go`
- Modify: `backend/application/agentthread/adk_agent_factory.go`

- [x] **Step 1: Write an order contract test**

Create recording middleware and assert:

```text
summarization
reduction
agentsmd
memory
skill
toolsearch
patchtoolcalls
policy
audit
usage
filesystem
```

The test must assert hook and wrapper order, not only slice order.

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKMiddleware'
```

Expected: FAIL.

- [x] **Step 3: Add the middleware factory**

```go
type ADKMiddlewareFactory interface {
	Build(ctx context.Context, run *RunSummary) ([]adk.ChatModelAgentMiddleware, error)
}
```

Start by wiring Eino:

- `summarization.New`;
- `reduction.New`;
- `patchtoolcalls.New`;
- `toolsearch.New`.

Use no-op Coze handlers for policy, audit, memory, Skill, and filesystem until
their production adapters are added. The no-op handlers must emit no tools and
must not weaken authorization.

- [x] **Step 4: Add capability-driven configuration**

Do not enable model-native deferred tool search unless the selected model
adapter declares support. Use client-side Eino tool search otherwise.

- [x] **Step 5: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKMiddleware'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/application/agentthread/adk_middleware.go backend/application/agentthread/adk_middleware_test.go backend/application/agentthread/adk_agent_factory.go
git commit -m "feat: assemble eino agent middleware"
```

### Task 9: Coze-Backed TurnLoop

**Files:**
- Create: `backend/application/agentthread/adk_turn_loop.go`
- Create: `backend/application/agentthread/adk_turn_loop_test.go`
- Modify: `backend/application/agentthread/worker.go`

- [x] **Step 1: Write failing TurnLoop tests**

Cover:

- initial user input starts one turn;
- follow-up input pushed during model execution preempts at the configured safe
  point;
- follow-up input pushed between turns starts the next turn;
- stop with checkpoint can resume;
- late input is not silently dropped;
- cancellation propagates to the active Runner.

- [x] **Step 2: Run and verify failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKTurnLoop'
```

Expected: FAIL.

- [x] **Step 3: Define the durable turn item**

```go
type ADKTurnItem struct {
	MessageID int64
	ThreadID  int64
	RunID     int64
	Content   string
	CreatedAt int64
}
```

Register every concrete checkpoint type used behind interfaces before
checkpoint persistence.

- [x] **Step 4: Configure TurnLoop**

Use:

- `GenInput` to convert pending items into Eino messages;
- `PrepareAgent` to build the per-turn Agent;
- `OnAgentEvents` to reuse the event mapper;
- `Store` and `CheckpointID` to persist stop/interruption state;
- `GenResume` to merge interrupted, unhandled, and new items deterministically.

- [x] **Step 5: Expose push and stop handles**

Maintain an active loop registry keyed by thread ID and run ID. API handlers
will use it later for live follow-up, but M2 tests should exercise it directly.

- [x] **Step 6: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKTurnLoop'
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/application/agentthread/adk_turn_loop.go backend/application/agentthread/adk_turn_loop_test.go backend/application/agentthread/worker.go
git commit -m "feat: add resumable eino turn loop"
```

### Task 10: Enable ADK Behind A Production Feature Gate

**Files:**
- Modify: `backend/application/application.go`
- Modify: `backend/application/agentthread/worker.go`
- Modify: `backend/application/agentthread/runtime_selector.go`
- Modify: `backend/application/agentthread/runner_test.go`
- Modify: `backend/application/agentthread/resume_runner_test.go`
- Create: `backend/application/agentthread/adk_contract_test.go`

- [x] **Step 1: Add parity contract fixtures**

Run the same scenarios against legacy and ADK executors:

- simple answer;
- one tool call;
- model failure;
- cancellation;
- clarification interrupt and resume;
- checkpoint restart;
- token usage;
- stream event ordering.

Compare normalized Coze events and terminal states, not Eino internal bytes.

- [x] **Step 2: Add the feature gate**

Support:

```text
AGENT_THREAD_RUNTIME_DEFAULT=legacy|eino_adk
```

Per-run `config.runtime` may opt into ADK only when server policy allows it.
An unknown or disabled mode must fail before claiming execution.

- [x] **Step 3: Wire the ADK executor**

Construct the ADK Agent factory, checkpoint store factory, event sink, usage
collector, cancel registry, middleware factory, and executor in
`backend/application/application.go`.

- [x] **Step 4: Run focused package tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/agentthread ./domain/agentthread/...
```

Expected: PASS.

- [x] **Step 5: Run full backend verification**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./...
```

Expected: PASS.

- [x] **Step 6: Run static checks**

```bash
cd backend
go mod verify
git diff --check
```

Expected: both commands succeed.

- [x] **Step 7: Keep production default on legacy**

Do not switch the default to `eino_adk` until the M2 exit gate, restart tests,
and LangGraph SSE contract tests pass.

- [ ] **Step 8: Commit**

```bash
git add backend/application/application.go backend/application/agentthread backend/domain/agentthread
git commit -m "feat: enable eino adk runtime behind feature gate"
```

## M2 Exit Gate

M2 is complete only when:

1. [x] legacy and ADK executors pass the normalized contract suite;
2. [x] ADK checkpoints survive process restart and targeted resume;
3. [x] cancellation reaches active model, tool, and nested Agent work;
4. [x] interrupt is a durable non-failure run state;
5. [x] event replay has stable ordering and no logical duplicates;
6. [x] usage is idempotent across streaming, retry, failover, and resume;
7. [x] middleware order is contract-tested;
8. [x] `*schema.AgenticMessage` remains disabled unless it passes all equivalent
   gates;
9. [x] full backend tests pass with Mockey-compatible compiler flags.
