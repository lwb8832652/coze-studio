# Human Interrupt Resume M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add production-ready human clarification and confirmation interrupts for the Eino ADK task runtime, with workbench and LangGraph resume support.

**Architecture:** Coze owns stable human-interaction DTOs, workbench API, event payloads, and frontend projection. Eino ADK remains the execution kernel through `StatefulInterrupt`, checkpoint envelopes, and `ResumeWithParams.Targets`. Existing run/message/event/checkpoint tables are reused; no new database migration is needed.

**Tech Stack:** Go, Hertz, GORM, Eino ADK `v0.9.9`, React 18, TypeScript, Vitest, `@coze-arch/coze-design`.

**Execution Mode:** The user has asked Codex to proceed automatically with the recommended approach. Execute inline, do not pause for phase confirmations, and do not create Git commits or stage files unless explicitly requested.

---

## File Map

- Create `backend/application/agentthread/adk_human_interaction.go`: DTOs, validation, built-in Eino tools, and helper functions for human interaction prompts and responses.
- Create `backend/application/agentthread/adk_human_interaction_test.go`: TDD coverage for clarification, confirmation approval/rejection, validation, and tool provider wrapping.
- Modify `backend/application/agentthread/adk_agent_factory.go`: allow production wiring to append built-in human interaction tools through the tool provider path.
- Modify `backend/application/application.go`: wire the human interaction tool provider into the Eino ADK executor.
- Modify `backend/application/agentthread/resume_runner.go`: parse `command.resume.targets`, validate ADK target IDs, and pass response payloads to `ResumeWithParams.Targets`.
- Modify `backend/application/agentthread/resume_runner_test.go`: cover target parsing, target validation, and backward-compatible nil target behavior.
- Modify `backend/application/agentthread/adk_event_mapper.go`: normalize Coze human interaction prompts into `run.interrupted` payloads.
- Modify `backend/application/agentthread/adk_event_mapper_test.go`: cover `human_interaction` and `human_interactions` event payloads.
- Modify `backend/domain/agentthread/repository/repository.go`: add idempotent run lookup by space and key.
- Modify `backend/domain/agentthread/repository/mysql.go`: implement idempotent run lookup.
- Modify `backend/domain/agentthread/repository/mysql_test.go`: verify idempotent run lookup.
- Modify `backend/domain/agentthread/service/service.go`: expose idempotent run lookup through the domain service.
- Modify `backend/domain/agentthread/service/service_impl.go`: implement domain lookup validation and mapping.
- Modify `backend/domain/agentthread/service/service_impl_test.go`: cover lookup validation and success.
- Modify `backend/application/agentthread/dto.go`: add human resume application DTOs.
- Modify `backend/application/agentthread/service.go`: add `ResumeHumanInteraction`.
- Modify `backend/application/agentthread/service_test.go`: cover workbench resume application behavior and idempotency.
- Modify `backend/api/model/workbench/thread/thread.go`: add workbench resume request and response structs.
- Modify `backend/api/handler/coze/workbench_thread_service.go`: add `ResumeTaskThreadRun` handler.
- Modify `backend/api/router/coze/api.go`: add `POST /api/workbench/task_threads/:thread_id/runs/:run_id/resume`.
- Modify `backend/api/router/coze/workbench_thread_route_test.go`: ensure the route is registered.
- Modify `backend/api/handler/coze/workbench_thread_service_test.go`: cover handler happy path and validation failures.
- Modify `backend/api/handler/coze/langgraph_run_service.go`: preserve and validate `command.resume.targets` in checkpoint resume.
- Modify `backend/api/handler/coze/langgraph_run_service_test.go`: cover LangGraph target preservation and invalid target rejection.
- Modify `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`: add `ResumeTaskThreadRun` request/response and createAPI binding.
- Modify `frontend/apps/coze-studio/src/pages/tasks/service.ts`: export `resumeTaskThreadRun`.
- Create `frontend/apps/coze-studio/src/pages/tasks/task-human-interaction.ts`: event projection and typed view models.
- Create `frontend/apps/coze-studio/src/pages/tasks/task-human-interrupt-card.tsx`: Semi/Coze Design UI for clarification and confirmation.
- Modify `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`: render pending interaction card and submit resume responses.
- Modify `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`: projection unit tests.
- Modify `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`: UI submission tests.
- Modify `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-status.ts`: treat `interrupted` as actionable/running-like for task status display if needed.

---

## Task 1: Human Interaction DTOs And Built-In ADK Tools

**Files:**
- Create: `backend/application/agentthread/adk_human_interaction.go`
- Create: `backend/application/agentthread/adk_human_interaction_test.go`

- [x] **Step 1: Write failing tests for tool interrupt and resume behavior**

Add tests:

```go
func TestADKClarificationToolInterruptsAndReturnsAnswer(t *testing.T)
func TestADKConfirmationToolReturnsRejectedResultWithoutError(t *testing.T)
func TestValidateHumanInteractionResponseRejectsInvalidDecision(t *testing.T)
func TestADKHumanInteractionToolProviderAppendsBuiltins(t *testing.T)
```

The first test should call `NewADKClarificationTool().InvokableRun(ctx, args)`,
expect `tool.StatefulInterrupt`, then call it again with Eino resume context and
expect a JSON result containing `answered=true`, `answer`, and `choice_id`.

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run 'TestADK(Clarification|Confirmation)|TestValidateHumanInteraction|TestADKHumanInteractionToolProvider' -count=1
```

Expected: fail because the new tool constructors, DTOs, and provider do not exist.

- [x] **Step 2: Implement minimal DTOs and validation**

Create these constants and types:

```go
const (
	humanInteractionSchema         = "coze.human_interaction.v1"
	humanInteractionResponseSchema = "coze.human_interaction_response.v1"
	humanInteractionResultSchema   = "coze.human_interaction_tool_result.v1"
)

type HumanInteractionKind string
type HumanInteractionDecision string
type HumanInteractionRiskLevel string

type HumanInteractionChoice struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
}

type HumanInteractionPrompt struct {
	Schema            string                   `json:"schema"`
	InteractionID     string                   `json:"interaction_id"`
	Kind              HumanInteractionKind     `json:"kind"`
	Title             string                   `json:"title,omitempty"`
	Question          string                   `json:"question,omitempty"`
	Description       string                   `json:"description,omitempty"`
	Required          bool                     `json:"required"`
	AllowFreeText     bool                     `json:"allow_free_text"`
	Choices           []HumanInteractionChoice `json:"choices,omitempty"`
	RiskLevel         HumanInteractionRiskLevel `json:"risk_level,omitempty"`
	ToolName          string                   `json:"tool_name,omitempty"`
	ToolCallID        string                   `json:"tool_call_id,omitempty"`
	PolicyRef         string                   `json:"policy_ref,omitempty"`
	Action            string                   `json:"action,omitempty"`
	Summary           string                   `json:"summary,omitempty"`
	Consequences      []string                 `json:"consequences,omitempty"`
	AffectedResources []string                 `json:"affected_resources,omitempty"`
	DefaultDecision   string                   `json:"default_decision,omitempty"`
	RejectionGuidance string                   `json:"rejection_guidance,omitempty"`
	CreatedAt         int64                    `json:"created_at,omitempty"`
}

type HumanInteractionResponse struct {
	Schema        string               `json:"schema"`
	InteractionID string               `json:"interaction_id"`
	Kind          HumanInteractionKind `json:"kind"`
	Decision      string               `json:"decision"`
	Answer        string               `json:"answer,omitempty"`
	ChoiceID      string               `json:"choice_id,omitempty"`
	Comment       string               `json:"comment,omitempty"`
	SubmittedBy   string               `json:"submitted_by,omitempty"`
	SubmittedAt   int64                `json:"submitted_at,omitempty"`
	Source        string               `json:"source,omitempty"`
}
```

Implement validation with explicit limits from the spec: prompt 32 KB, response
16 KB, answer/comment 8 KB, title/question/summary 2 KB, description/guidance
4 KB.

- [x] **Step 3: Implement the two Eino tools**

Use `toolutils.InferTool` or a small `tool.InvokableTool` implementation. Tool
names:

```go
const (
	adkClarificationToolName = "ask_user_clarification"
	adkConfirmationToolName  = "request_human_confirmation"
)
```

Behavior:

1. On first invocation, call `tool.StatefulInterrupt(ctx, prompt, state)`.
2. On resume, call `tool.GetResumeContext[HumanInteractionResponse](ctx)`.
3. Clarification accepts only `decision=answered`.
4. Confirmation accepts `decision=approved` or `decision=rejected`.
5. Rejection returns a normal JSON result with `approved=false`.

- [x] **Step 4: Implement the tool provider wrapper**

Add:

```go
func NewADKHumanInteractionToolProvider(base ADKToolProvider) ADKToolProvider
func NewADKHumanInteractionTools() ([]tool.BaseTool, error)
```

The wrapper resolves base tools first, appends both built-ins, and does not
deduplicate unrelated tools. If a base tool already has the same name, return an
error because model-visible and executable tools must match one source of truth.

- [x] **Step 5: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run 'TestADK(Clarification|Confirmation)|TestValidateHumanInteraction|TestADKHumanInteractionToolProvider' -count=1
```

Expected: pass.

---

## Task 2: Wire Human Tools Into The Eino ADK Runtime

**Files:**
- Modify: `backend/application/application.go`
- Modify: `backend/application/agentthread/adk_agent_factory_test.go`

- [x] **Step 1: Write failing factory/wiring test**

Add a test that builds a factory with `NewADKHumanInteractionToolProvider(nil)`,
uses a recording middleware factory, and asserts the static tool list includes
`ask_user_clarification` and `request_human_confirmation`.

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run TestApplicationADKAgentFactoryIncludesHumanInteractionTools -count=1
```

Expected: fail because production wiring is not connected yet.

- [x] **Step 2: Wire production provider**

In `backend/application/application.go`, change the `NewApplicationADKAgentFactory`
call from:

```go
agentthread.NewApplicationADKAgentFactory(nil, nil, assembler)
```

to:

```go
agentthread.NewApplicationADKAgentFactory(
	nil,
	agentthread.NewADKHumanInteractionToolProvider(nil),
	assembler,
)
```

- [x] **Step 3: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run TestApplicationADKAgentFactoryIncludesHumanInteractionTools -count=1
```

Expected: pass.

---

## Task 3: Resume Command Targets

**Files:**
- Modify: `backend/application/agentthread/resume_runner.go`
- Modify: `backend/application/agentthread/resume_runner_test.go`

- [x] **Step 1: Write failing tests for `command.resume.targets`**

Add tests:

```go
func TestParseResumeRunPayloadAcceptsTargets(t *testing.T)
func TestLoadADKResumeInputUsesCommandTargets(t *testing.T)
func TestLoadADKResumeInputRejectsUnknownCommandTarget(t *testing.T)
func TestLoadADKResumeInputKeepsNilTargetsWhenCommandTargetsMissing(t *testing.T)
```

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run 'Test(ParseResumeRunPayloadAcceptsTargets|LoadADKResumeInput)' -count=1
```

Expected: fail because `resumeRunPayload` does not parse target data.

- [x] **Step 2: Extend `resumeRunPayload`**

Add:

```go
Targets map[string]any
```

Parse `resume.targets` only when it is a JSON object. Reject non-object targets
with `resume run command.resume.targets must be an object`.

- [x] **Step 3: Validate ADK targets**

In `loadADKResumeInput`, after decoding the envelope:

1. If `resume.Targets` is empty, use existing `adkResumeTargets(envelope.Interrupts)`.
2. If targets exist, require each key to appear in `envelope.Interrupts`.
3. Copy target values into `ADKResumeTargets`.
4. Keep existing runtime version and checkpoint key validation.

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run 'Test(ParseResumeRunPayloadAcceptsTargets|LoadADKResumeInput)' -count=1
```

Expected: pass.

---

## Task 4: Normalize Human Interaction Interrupt Events

**Files:**
- Modify: `backend/application/agentthread/adk_event_mapper.go`
- Modify: `backend/application/agentthread/adk_event_mapper_test.go`

- [x] **Step 1: Write failing mapper tests**

Add tests:

```go
func TestMapADKInterruptedEventIncludesHumanInteraction(t *testing.T)
func TestMapADKInterruptedEventIncludesMultipleHumanInteractions(t *testing.T)
```

Use `adk.InterruptCtx.Info` with `HumanInteractionPrompt`.

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run TestMapADKInterruptedEventIncludesHumanInteraction -count=1
```

Expected: fail because payload does not include `human_interaction`.

- [x] **Step 2: Add prompt normalization helpers**

Add helper functions to `adk_human_interaction.go`:

```go
func humanInteractionPromptFromInfo(info any) (*HumanInteractionPrompt, bool)
func humanInteractionPromptsFromInterrupts(items []ADKInterruptItem) []HumanInteractionPrompt
```

Support `HumanInteractionPrompt`, `*HumanInteractionPrompt`, and
`map[string]any` with `schema=coze.human_interaction.v1`.

- [x] **Step 3: Add fields to `run.interrupted` payload**

When mapped interrupt items contain prompts:

1. Set `payload["human_interactions"]` to all prompts.
2. Set `payload["human_interaction"]` to the first prompt.
3. Do not remove the existing `interrupts` field.

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run TestMapADKInterruptedEventIncludesHumanInteraction -count=1
```

Expected: pass.

---

## Task 5: Idempotent Run Lookup

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [x] **Step 1: Write failing repository and service tests**

Add tests:

```go
func TestThreadRepositoryGetRunByIdempotencyKey(t *testing.T)
func TestThreadServiceGetRunByIdempotencyKey(t *testing.T)
```

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./domain/agentthread/repository ./domain/agentthread/service -run 'Test(ThreadRepositoryGetRunByIdempotencyKey|ThreadServiceGetRunByIdempotencyKey)' -count=1
```

Expected: fail because the lookup method does not exist.

- [x] **Step 2: Add repository method**

Add to `ThreadRepository`:

```go
GetRunByIdempotencyKey(ctx context.Context, spaceID int64, idempotencyKey string) (*entity.Run, error)
```

Implement in MySQL with:

```go
WHERE space_id = ? AND idempotency_key = ? AND deleted_at IS NULL
```

Return `nil, nil` when not found.

- [x] **Step 3: Add domain service method**

Add to `ThreadService`:

```go
GetRunByIdempotencyKey(ctx context.Context, spaceID int64, idempotencyKey string) (*entity.Run, error)
```

Validation:

1. `spaceID > 0`.
2. trimmed idempotency key is non-empty and at most 128 bytes.

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./domain/agentthread/repository ./domain/agentthread/service -run 'Test(ThreadRepositoryGetRunByIdempotencyKey|ThreadServiceGetRunByIdempotencyKey)' -count=1
```

Expected: pass.

---

## Task 6: Application-Level Human Resume

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Create: `backend/application/agentthread/human_interaction_resume.go`
- Modify: `backend/application/agentthread/service_test.go`

- [x] **Step 1: Write failing application tests**

Add tests:

```go
func TestApplicationResumeHumanInteractionCreatesQueuedRun(t *testing.T)
func TestApplicationResumeHumanInteractionRejectsNonInterruptedSourceRun(t *testing.T)
func TestApplicationResumeHumanInteractionRejectsUnknownInterrupt(t *testing.T)
func TestApplicationResumeHumanInteractionReturnsExistingIdempotentRun(t *testing.T)
```

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run TestApplicationResumeHumanInteraction -count=1
```

Expected: fail because `ResumeHumanInteraction` does not exist.

- [x] **Step 2: Add DTOs**

Add:

```go
type ResumeHumanInteractionRequest struct {
	ThreadID       int64
	SourceRunID    int64
	InterruptID    string
	Response       HumanInteractionResponse
	IdempotencyKey string
}

type ResumeHumanInteractionResponse struct {
	Run *RunSummary
}
```

- [x] **Step 3: Implement `ResumeHumanInteraction`**

Behavior:

1. Load source run by `SourceRunID`.
2. Require `sourceRun.ThreadID == req.ThreadID`.
3. Require `sourceRun.Status == RunStatusInterrupted`.
4. List source-run checkpoints and select the latest active ADK checkpoint.
5. Decode `ADKCheckpointEnvelope`.
6. Require `InterruptID` exists in `envelope.Interrupts`.
7. Validate `HumanInteractionResponse`.
8. Build a deterministic <=128 byte idempotency key when missing:
   `human-resume:{sha256(thread_id, source_run_id, interrupt_id, response)}`.
9. Use `GetRunByIdempotencyKey` to return an existing run before creating a new one.
10. Create queued run with:
    - `Status: RunStatusQueued`
    - `Input: {"messages":[]}`
    - `Command: {"resume":{...,"targets":{interrupt_id: response}}}`
    - metadata containing `checkpoint_resume` and `human_interaction`
11. Append a user message with content derived from the response.
12. Append `human.interaction.resolved` to the resume run.

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./application/agentthread -run TestApplicationResumeHumanInteraction -count=1
```

Expected: pass.

---

## Task 7: Workbench Resume API

**Files:**
- Modify: `backend/api/model/workbench/thread/thread.go`
- Modify: `backend/api/handler/coze/workbench_thread_service.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] **Step 1: Write failing route and handler tests**

Add route assertion for:

```text
POST /api/workbench/task_threads/1/runs/2/resume
```

Add handler tests:

```go
func TestResumeTaskThreadRunHandlerCreatesQueuedResumeRun(t *testing.T)
func TestResumeTaskThreadRunHandlerRejectsInvalidPayload(t *testing.T)
```

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/router/coze ./api/handler/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes|TestResumeTaskThreadRunHandler' -count=1
```

Expected: fail because route, request struct, and handler do not exist.

- [x] **Step 2: Add API structs**

Add:

```go
type ResumeTaskThreadRunRequest struct {
	ThreadID       int64                    `path:"thread_id,required" json:"-"`
	RunID          int64                    `path:"run_id,required" json:"-"`
	InterruptID    string                   `json:"interrupt_id,required"`
	Response       HumanInteractionResponse `json:"response,required"`
	IdempotencyKey string                   `json:"idempotency_key,omitempty"`
}

type ResumeTaskThreadRunResponse struct {
	Data *TaskThreadRun `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}
```

Use a local API-model response struct matching the application DTO shape if the
application DTO cannot be imported without a cycle.

- [x] **Step 3: Add handler and route**

Handler calls:

```go
appagentthread.SVC.ResumeHumanInteraction(ctx, &appagentthread.ResumeHumanInteractionRequest{...})
```

Route:

```go
_task_threads.POST("/:thread_id/runs/:run_id/resume", coze.ResumeTaskThreadRun)
```

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/router/coze ./api/handler/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes|TestResumeTaskThreadRunHandler' -count=1
```

Expected: pass.

---

## Task 8: LangGraph Resume Target Compatibility

**Files:**
- Modify: `backend/api/handler/coze/langgraph_run_service.go`
- Modify: `backend/api/handler/coze/langgraph_run_service_test.go`

- [x] **Step 1: Write failing LangGraph tests**

Add:

```go
func TestLangGraphRunCreatePreservesResumeTargets(t *testing.T)
func TestLangGraphRunCreateRejectsUnknownResumeTarget(t *testing.T)
```

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze -run TestLangGraphRunCreate.*ResumeTarget -count=1
```

Expected: fail if target validation/preservation is missing.

- [x] **Step 2: Preserve target data**

Ensure `langGraphRunCommandWithCheckpointResume` keeps any existing
`command.resume.targets` while merging checkpoint readiness fields.

- [x] **Step 3: Validate target IDs**

When checkpoint runtime is Eino ADK and `targets` is present, decode the
checkpoint envelope and reject target IDs not found in `envelope.Interrupts`.

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze -run TestLangGraphRunCreate.*ResumeTarget -count=1
```

Expected: pass.

---

## Task 9: Frontend API Schema And Projection

**Files:**
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-human-interaction.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`

- [x] **Step 1: Write failing projection tests**

Add tests:

```ts
it('extracts latest pending clarification prompt', () => {})
it('hides prompt after matching resolved event', () => {})
it('extracts confirmation decision metadata', () => {})
```

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: fail because `getPendingHumanInteraction` does not exist.

- [x] **Step 2: Add API schema binding**

Add interfaces and binding:

```ts
export interface HumanInteractionResponse {
  schema: string;
  interaction_id: string;
  kind: 'clarification' | 'confirmation';
  decision: 'answered' | 'approved' | 'rejected';
  answer?: string;
  choice_id?: string;
  comment?: string;
}

export interface ResumeTaskThreadRunRequest {
  thread_id: string;
  run_id: string;
  interrupt_id: string;
  response: HumanInteractionResponse;
  idempotency_key?: string;
}

export const ResumeTaskThreadRun = createAPI<ResumeTaskThreadRunRequest, CreateTaskThreadRunResponse>({...});
```

- [x] **Step 3: Add projection module**

`getPendingHumanInteraction(events)` should:

1. Parse `run.interrupted` payloads.
2. Accept `human_interaction` and `human_interactions`.
3. Parse `human.interaction.resolved` payloads.
4. Return the latest unresolved prompt keyed by `interrupt_id` and
   `interaction_id`.

- [x] **Step 4: Verify green**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: pass.

---

## Task 10: Task Detail Human Interaction UI

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-human-interrupt-card.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-status.ts`

- [x] **Step 1: Write failing UI tests**

Add tests:

```ts
it('renders clarification card and submits answer through resume API', async () => {})
it('renders confirmation card and submits rejection through resume API', async () => {})
```

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: fail because card and resume service are not wired.

- [x] **Step 2: Implement `TaskHumanInterruptCard`**

Use `Button`, `TextArea`, and existing Coze/Semi styling patterns. The card
renders:

1. question/title;
2. choices for clarification;
3. approve/reject buttons for confirmation;
4. loading and backend error state;
5. no Eino address as user-facing copy.

- [x] **Step 3: Wire task detail submit**

In `TaskDetailPage`:

1. derive `pendingHumanInteraction` from `events`;
2. call `resumeTaskThreadRun`;
3. refresh task detail after success;
4. keep normal follow-up behavior unchanged when no pending interaction exists.

- [x] **Step 4: Verify green**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: pass.

---

## Task 11: Full Verification

**Files:**
- No new source files.

- [x] **Step 1: Run targeted backend tests**

```bash
GOCACHE=/private/tmp/coze-go-build go test -race -gcflags='all=-l -N' ./application/agentthread ./domain/agentthread/repository ./domain/agentthread/service ./api/router/coze ./api/handler/coze -run 'Test(ADK|ApplicationResumeHumanInteraction|ParseResumeRunPayloadAcceptsTargets|LoadADKResumeInput|ThreadRepositoryGetRunByIdempotencyKey|ThreadServiceGetRunByIdempotencyKey|RegisterIncludesWorkbenchTaskThreadRoutes|ResumeTaskThreadRunHandler|LangGraphRunCreate.*ResumeTarget)' -count=1
```

Expected: pass. macOS linker `LC_DYSYMTAB` warnings are acceptable if tests pass. The full non-race package gate is covered in Step 4 with Mockey-required gcflags.

- [x] **Step 2: Run targeted frontend tests**

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: pass.

- [x] **Step 3: Run targeted frontend lint**

```bash
cd frontend/apps/coze-studio && ./node_modules/.bin/eslint src/pages/tasks/detail.tsx src/pages/tasks/task-detail-hooks.ts src/pages/tasks/task-event-projection.ts src/pages/tasks/task-human-interaction.ts src/pages/tasks/task-human-interrupt-card.tsx src/pages/tasks/service.ts src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/components/workspace-sub-menu/workspace-task-status.ts src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx --no-cache
```

Expected: no errors. Existing unrelated warnings in `helpers.ts` are not part of this command.

- [x] **Step 4: Run full backend test gate**

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./...
```

Expected: pass.

- [x] **Step 5: Check formatting and whitespace**

```bash
git diff --check
```

Expected: no output.

---

## Self-Review Checklist

- Spec coverage: tools, DTOs, resume targets, workbench API, LangGraph API, frontend card, rejection semantics, idempotency, and verification are all mapped to tasks.
- Scope control: no IM Channels, no Python sidecar, no security scanner tables, no new migration.
- Type consistency: `HumanInteractionPrompt`, `HumanInteractionResponse`, `human_interaction`, `human_interactions`, `human.interaction.resolved`, and `command.resume.targets` use the same names across backend and frontend.
- Execution constraint: no Git commit or staging step is included because the user has not requested commits.
