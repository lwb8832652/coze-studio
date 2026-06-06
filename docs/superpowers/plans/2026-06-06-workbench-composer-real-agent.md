# Workbench Composer Real Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify the Workbench composer across home and task detail, route every send through `sendWorkbenchChat`, persist `answer`/`agent_trace`/`report` results, and replace fake task execution with minimal real Ark/Ask/AgentRun behavior.

**Architecture:** `WorkbenchChat` becomes the single orchestration boundary. It creates or reuses a task, appends task events for each user turn, runs either the builtin chat model with optional Ask knowledge retrieval or the real AgentRun stream, and completes/fails the task with a typed result. The frontend extracts a reusable `WorkbenchComposer` and renders task detail by parsed `result_type`.

**Tech Stack:** Go, CloudWeGo Hertz/thrift-generated API models, Eino chat models and stream readers, Coze crossdomain knowledge/database/agentrun services, React 18, TypeScript, Vitest, Coze Design.

---

## Database Resource Reservation

Database querying is a later resource type for Agent and Workflow execution, not an Ask-mode tool in this plan. Keep the reservation explicit:

- `enable_databases` is added to the Workbench API and forwarded into AgentRun metadata.
- Empty `enable_databases` means default platform database resources; non-empty means a future whitelist.
- Do not add a new SQL executor in Workbench. The future implementation should reuse the existing database capability behind `backend/crossdomain/database/contract.go`, AgentFlow database tools, and Workflow database nodes.
- Reserve `agent.database_query` as the task event type for SQL/database trace rendering. Its payload should include `database_id`, `sql`, `row_count`, `status`, `message`, and `error` when that capability is enabled.

## File Structure

### Backend API Contract

- Modify: `idl/workbench/workbench.thrift`
  - Add `task_id`, `enable_skills`, `enable_mcp`, `enable_kbs`, `enable_databases`.
  - Add response metadata `result_type` and `execution_type`.
- Modify: `backend/api/model/workbench/chat/workbench.go`
  - Mirror the new request/response fields for HTTP JSON binding and local tests.
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/workbench.ts`
  - Mirror the new request/response fields and request body mapping.

### Backend Task Lifecycle

- Modify: `backend/domain/task/service/service.go`
  - Add `Start(ctx, id)` to `TaskService`.
- Modify: `backend/domain/task/service/service_impl.go`
  - Implement Created/Queued to Running transition.
- Modify: `backend/domain/task/service/state_machine.go`
  - Allow Created to Running for synchronous Workbench turns.
- Modify: `backend/application/task/task.go`
  - Add app-level helpers for creating a running task, appending events, completing, failing, and getting a task.
- Modify: `backend/application/task/task_test.go`
  - Cover running-task creation and app helpers.

### Backend Workbench Runtime

- Modify: `backend/application/workbench/init.go`
  - Add injectable chat model provider, knowledge service, agent run service, and ID generator as service components.
- Modify: `backend/application/application.go`
  - Wire Workbench services after complex services exist, especially AgentRun.
- Create: `backend/application/workbench/result_payload.go`
  - Parse/build result JSON for `answer`, `agent_trace`, `report`.
- Create: `backend/application/workbench/answer_runner.go`
  - Run Auto and Ask through builtin chat model; Ask may retrieve knowledge context.
- Create: `backend/application/workbench/answer_runner_test.go`
  - Test Auto and Ask result payloads and Ask knowledge retrieval.
- Create: `backend/application/workbench/agent_runner.go`
  - Run minimal AgentRun stream and append task events.
- Create: `backend/application/workbench/agent_runner_test.go`
  - Test stream event conversion and final result.
- Modify: `backend/application/workbench/chat_gateway.go`
  - Replace old canned route answers and fallback task creation with real orchestration.
- Modify: `backend/application/workbench/chat_gateway_test.go`
  - Cover create-vs-append, modes, response metadata, and no direct `CreateTask` fallback.
- Create: `backend/application/workbench/test_fakes_test.go`
  - Shared Workbench task and AgentRun fakes for backend tests.

### Frontend Workbench

- Create: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
  - Shared composer UI and submit payload.
- Create: `frontend/apps/coze-studio/src/pages/workbench/components/types.ts`
  - `WorkbenchMode`, submit payload, result type helpers.
- Modify: `frontend/apps/coze-studio/src/pages/workbench/extensions-popover.tsx`
  - Make selected resource IDs observable by the composer; include database reservation.
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
  - Use shared composer and only call `sendWorkbenchChat`.
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
  - Update expectations for single chat gateway submission.

### Frontend Task Detail

- Modify: `frontend/apps/coze-studio/src/pages/tasks/helpers.ts`
  - Parse result type and result payload; keep `agent.database_query` display metadata reserved.
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
  - Use shared composer with `taskId`; render answer/agent/report branches.
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
  - Cover follow-up send with task_id and result rendering branches.
- Modify: `frontend/apps/coze-studio/src/components/workspace-prototype.less`
  - Add only the styles required by shared composer variants and result branches.

---

## Task 1: Extend Workbench API Contract

**Files:**
- Modify: `idl/workbench/workbench.thrift`
- Modify: `backend/api/model/workbench/chat/workbench.go`
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/workbench.ts`
- Test: `backend/application/workbench/chat_gateway_test.go`
- Test: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [ ] **Step 1: Write failing backend compile test for new request and response fields**

Add this test to `backend/application/workbench/chat_gateway_test.go`:

```go
func TestWorkbenchChatContractHasTaskAndResourceFields(t *testing.T) {
	taskID := int64(100)
	req := &chatapi.WorkbenchChatRequest{
		SpaceID:         1,
		Message:         "hello",
		Mode:            chatapi.ChatMode_Ask,
		TaskID:          &taskID,
		EnableSkills:    []string{"skill-a"},
		EnableMcp:       []string{"mcp-a"},
		EnableKbs:       []string{"kb-a"},
		EnableDatabases: []string{"db-a"},
	}

	require.True(t, req.IsSetTaskID())
	require.Equal(t, int64(100), req.GetTaskID())
	require.Equal(t, []string{"skill-a"}, req.GetEnableSkills())
	require.Equal(t, []string{"mcp-a"}, req.GetEnableMcp())
	require.Equal(t, []string{"kb-a"}, req.GetEnableKbs())
	require.Equal(t, []string{"db-a"}, req.GetEnableDatabases())

	resultType := "answer"
	executionType := "Ark"
	data := &chatapi.WorkbenchChatData{
		RouteTarget:   chatapi.RouteTarget_TaskEngine,
		ResultType:    &resultType,
		ExecutionType: &executionType,
	}

	require.Equal(t, "answer", data.GetResultType())
	require.Equal(t, "Ark", data.GetExecutionType())
}
```

- [ ] **Step 2: Run backend test and verify it fails to compile**

Run:

```bash
cd backend && go test ./application/workbench -run TestWorkbenchChatContractHasTaskAndResourceFields -count=1
```

Expected: FAIL with missing fields such as `TaskID`, `EnableSkills`, or `ResultType`.

- [ ] **Step 3: Update IDL**

In `idl/workbench/workbench.thrift`, update the structs exactly:

```thrift
struct WorkbenchChatRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    3: required string message
    4: required ChatMode mode
    5: optional i64 selected_skill_id (agw.js_conv="str", api.js_conv="true")
    6: optional i64 task_id (agw.js_conv="str", api.js_conv="true")
    7: optional list<string> enable_skills
    8: optional list<string> enable_mcp
    9: optional list<string> enable_kbs
    10: optional list<string> enable_databases
    255: optional base.Base Base (api.none="true")
}

struct WorkbenchChatData {
    1: required RouteTarget route_target
    2: optional string answer
    3: optional task.ChatTask task
    4: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    5: optional string reason
    6: optional string result_type
    7: optional string execution_type
}
```

- [ ] **Step 4: Update Go generated model for HTTP binding**

In `backend/api/model/workbench/chat/workbench.go`, add fields to `WorkbenchChatRequest`:

```go
TaskID          *int64     `thrift:"task_id,6,optional" form:"task_id" json:"task_id,string,omitempty" query:"task_id"`
EnableSkills    []string   `thrift:"enable_skills,7,optional" form:"enable_skills" json:"enable_skills,omitempty" query:"enable_skills"`
EnableMcp       []string   `thrift:"enable_mcp,8,optional" form:"enable_mcp" json:"enable_mcp,omitempty" query:"enable_mcp"`
EnableKbs       []string   `thrift:"enable_kbs,9,optional" form:"enable_kbs" json:"enable_kbs,omitempty" query:"enable_kbs"`
EnableDatabases []string   `thrift:"enable_databases,10,optional" form:"enable_databases" json:"enable_databases,omitempty" query:"enable_databases"`
```

Add getters and set checks near the existing generated helpers:

```go
var WorkbenchChatRequest_TaskID_DEFAULT int64

func (p *WorkbenchChatRequest) GetTaskID() (v int64) {
	if !p.IsSetTaskID() {
		return WorkbenchChatRequest_TaskID_DEFAULT
	}
	return *p.TaskID
}

func (p *WorkbenchChatRequest) GetEnableSkills() (v []string) {
	return p.EnableSkills
}

func (p *WorkbenchChatRequest) GetEnableMcp() (v []string) {
	return p.EnableMcp
}

func (p *WorkbenchChatRequest) GetEnableKbs() (v []string) {
	return p.EnableKbs
}

func (p *WorkbenchChatRequest) GetEnableDatabases() (v []string) {
	return p.EnableDatabases
}

func (p *WorkbenchChatRequest) IsSetTaskID() bool {
	return p.TaskID != nil
}
```

Add fields to `WorkbenchChatData`:

```go
ResultType    *string `thrift:"result_type,6,optional" form:"result_type" json:"result_type,omitempty" query:"result_type"`
ExecutionType *string `thrift:"execution_type,7,optional" form:"execution_type" json:"execution_type,omitempty" query:"execution_type"`
```

Add getters:

```go
var WorkbenchChatData_ResultType_DEFAULT string

func (p *WorkbenchChatData) GetResultType() (v string) {
	if !p.IsSetResultType() {
		return WorkbenchChatData_ResultType_DEFAULT
	}
	return *p.ResultType
}

var WorkbenchChatData_ExecutionType_DEFAULT string

func (p *WorkbenchChatData) GetExecutionType() (v string) {
	if !p.IsSetExecutionType() {
		return WorkbenchChatData_ExecutionType_DEFAULT
	}
	return *p.ExecutionType
}

func (p *WorkbenchChatData) IsSetResultType() bool {
	return p.ResultType != nil
}

func (p *WorkbenchChatData) IsSetExecutionType() bool {
	return p.ExecutionType != nil
}
```

Update `fieldIDToName_WorkbenchChatRequest` and `fieldIDToName_WorkbenchChatData` so debug strings and thrift helpers stay consistent.

- [ ] **Step 5: Update frontend API schema**

In `frontend/packages/arch/api-schema/src/idl/workbench/workbench.ts`, update interfaces:

```ts
export interface WorkbenchChatRequest {
  space_id: string,
  conversation_id?: string,
  message: string,
  mode: ChatMode,
  selected_skill_id?: string,
  task_id?: string,
  enable_skills?: string[],
  enable_mcp?: string[],
  enable_kbs?: string[],
  enable_databases?: string[],
}

export interface WorkbenchChatData {
  route_target: RouteTarget,
  answer?: string,
  task?: task.ChatTask,
  conversation_id?: string,
  reason?: string,
  result_type?: string,
  execution_type?: string,
}
```

Update `reqMapping.body`:

```ts
"body": [
  "space_id",
  "conversation_id",
  "message",
  "mode",
  "selected_skill_id",
  "task_id",
  "enable_skills",
  "enable_mcp",
  "enable_kbs",
  "enable_databases"
]
```

- [ ] **Step 6: Run contract tests**

Run:

```bash
cd backend && go test ./application/workbench -run TestWorkbenchChatContractHasTaskAndResourceFields -count=1
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
```

Expected: backend contract test passes; frontend tests may still fail on old UI behavior until later tasks.

- [ ] **Step 7: Commit API contract changes**

```bash
git add idl/workbench/workbench.thrift backend/api/model/workbench/chat/workbench.go frontend/packages/arch/api-schema/src/idl/workbench/workbench.ts backend/application/workbench/chat_gateway_test.go
git commit -m "feat: extend workbench chat contract"
```

---

## Task 2: Add Task Lifecycle Helpers For Synchronous Workbench Turns

**Files:**
- Modify: `backend/domain/task/service/service.go`
- Modify: `backend/domain/task/service/state_machine.go`
- Modify: `backend/domain/task/service/service_impl.go`
- Modify: `backend/application/task/task.go`
- Modify: `backend/application/task/task_test.go`

- [ ] **Step 1: Write failing domain and app tests**

Add to `backend/application/task/task_test.go`:

```go
func TestCreateRunningTaskStartsWithoutQueueWorker(t *testing.T) {
	domainSVC := &recordingDomainService{
		created: &entity.Task{
			ID:        200,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "quick answer",
			Status:    entity.StatusCreated,
		},
		started: &entity.Task{
			ID:        200,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "quick answer",
			Status:    entity.StatusRunning,
			Progress:  10,
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.CreateRunningTask(context.Background(), &taskapi.CreateTaskRequest{
		SpaceID: 1,
		Title:   "quick answer",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.Equal(t, taskapi.TaskStatus_Running, resp.Data.Status)
	require.Equal(t, int64(200), domainSVC.startedID)
	require.Zero(t, domainSVC.enqueuedID)
}

func TestAppendAndCompleteTaskHelpers(t *testing.T) {
	domainSVC := &recordingDomainService{}
	app := &ApplicationService{DomainSVC: domainSVC}

	err := app.AppendTaskEvent(context.Background(), 300, "answer.completed", `{"message":"ok"}`)
	require.NoError(t, err)
	require.Len(t, domainSVC.events, 1)
	require.Equal(t, "answer.completed", domainSVC.events[0].eventType)

	err = app.CompleteTask(context.Background(), 300, `{"result_type":"answer"}`)
	require.NoError(t, err)
	require.Equal(t, int64(300), domainSVC.completedID)
	require.Contains(t, domainSVC.completedResult, `"result_type":"answer"`)
}
```

Extend `recordingDomainService` in the same file:

```go
started   *entity.Task
startedID int64
failedID  int64
failError string
```

Add methods:

```go
func (s *recordingDomainService) Start(ctx context.Context, id int64) (*entity.Task, error) {
	s.startedID = id
	return s.started, nil
}

func (s *recordingDomainService) Fail(ctx context.Context, id int64, errMsg string) error {
	s.failedID = id
	s.failError = errMsg
	return nil
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd backend && go test ./application/task -run 'TestCreateRunningTaskStartsWithoutQueueWorker|TestAppendAndCompleteTaskHelpers' -count=1
```

Expected: FAIL because `Start`, `CreateRunningTask`, `AppendTaskEvent`, or `CompleteTask` do not exist.

- [ ] **Step 3: Add domain Start method**

In `backend/domain/task/service/service.go`, add:

```go
Start(ctx context.Context, id int64) (*entity.Task, error)
```

In `backend/domain/task/service/state_machine.go`, allow Created to Running:

```go
entity.StatusCreated: {
	entity.StatusQueued:   true,
	entity.StatusRunning:  true,
	entity.StatusFailed:   true,
	entity.StatusCanceled: true,
},
```

In `backend/domain/task/service/service_impl.go`, add:

```go
func (s *taskService) Start(ctx context.Context, id int64) (*entity.Task, error) {
	return s.transition(ctx, id, entity.StatusRunning, 10, "", "")
}
```

- [ ] **Step 4: Add app helper methods**

In `backend/application/task/task.go`, add:

```go
func (s *ApplicationService) CreateRunningTask(ctx context.Context, req *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	creatorID := int64(0)
	if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
		creatorID = *uid
	}
	task, err := s.DomainSVC.Create(ctx, &domain.CreateRequest{
		SpaceID:        req.SpaceID,
		CreatorID:      creatorID,
		ConversationID: req.GetConversationID(),
		MessageID:      req.GetMessageID(),
		SkillID:        req.GetSkillID(),
		Title:          req.Title,
		Input:          req.GetInput(),
	})
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task service returned empty task")
	}
	running, err := s.DomainSVC.Start(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if running != nil {
		task = running
	}
	return &taskapi.CreateTaskResponse{Code: 0, Msg: "success", Data: entityToAPI(task)}, nil
}

func (s *ApplicationService) AppendTaskEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	if err := s.requireDomainSVC(); err != nil {
		return err
	}
	return s.DomainSVC.AppendEvent(ctx, taskID, eventType, payload)
}

func (s *ApplicationService) CompleteTask(ctx context.Context, taskID int64, result string) error {
	if err := s.requireDomainSVC(); err != nil {
		return err
	}
	return s.DomainSVC.Complete(ctx, taskID, result)
}

func (s *ApplicationService) FailTask(ctx context.Context, taskID int64, errMsg string) error {
	if err := s.requireDomainSVC(); err != nil {
		return err
	}
	return s.DomainSVC.Fail(ctx, taskID, errMsg)
}
```

- [ ] **Step 5: Remove fake task execution from worker path**

In `backend/application/task/task.go`, remove `emitTaskExecutionFlow` and the call to it from `ProcessQueuedTasks`.

Change `taskResultJSON` to explicit legacy fallback:

```go
func taskResultJSON(task *entity.Task) (string, error) {
	payload := map[string]string{
		"task_id":     strconv.FormatInt(task.ID, 10),
		"title":       task.Title,
		"result_type": "answer",
		"message":     "任务已完成。",
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
```

Remove tests that require fake steps. Replace `TestProcessQueuedTasksEmitsExecutionEvents` with:

```go
func TestProcessQueuedTasksDoesNotEmitFakeAgentTrace(t *testing.T) {
	domainSVC := &recordingDomainService{
		claimed: []*entity.Task{
			{ID: 100, Title: "generate report", Status: entity.StatusRunning},
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	err := app.ProcessQueuedTasks(context.Background(), 10)

	require.NoError(t, err)
	require.Empty(t, domainSVC.events)
	require.Contains(t, domainSVC.completedResult, `"result_type":"answer"`)
	require.NotContains(t, domainSVC.completedResult, "本地占位执行结果")
}
```

- [ ] **Step 6: Run task tests**

Run:

```bash
cd backend && go test ./domain/task/service ./application/task -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit task lifecycle changes**

```bash
git add backend/domain/task/service backend/application/task
git commit -m "feat: add workbench task lifecycle helpers"
```

---

## Task 3: Add Result Payload And Answer Runner

**Files:**
- Create: `backend/application/workbench/result_payload.go`
- Create: `backend/application/workbench/answer_runner.go`
- Create: `backend/application/workbench/answer_runner_test.go`
- Modify: `backend/application/workbench/init.go`

- [ ] **Step 1: Write failing answer runner tests**

Create `backend/application/workbench/answer_runner_test.go`:

```go
package workbench

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	knowledgemodel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
)

func TestRunAutoAnswerUsesBuiltinModelAndMarksArk(t *testing.T) {
	app := &ApplicationService{
		chatModelProvider: func(ctx context.Context) (chatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(index int, in []*schema.Message) (*schema.Message, error) {
					require.Contains(t, in[len(in)-1].Content, "hello")
					return schema.AssistantMessage("auto answer", nil), nil
				},
			}, true, nil
		},
	}

	result, err := app.runAnswer(context.Background(), answerRequest{
		mode:    ChatModeAuto,
		message: "hello",
	})

	require.NoError(t, err)
	require.Equal(t, resultTypeAnswer, result.ResultType)
	require.Equal(t, executionTypeArk, result.ExecutionType)
	require.Equal(t, "auto answer", result.Message)
	require.Empty(t, result.RetrievalSources)
}

func TestRunAskRetrievesKnowledgeContext(t *testing.T) {
	knowledge := &fakeKnowledgeService{
		retrieveResp: &knowledgemodel.RetrieveResponse{
			RetrieveSlices: []*knowledgemodel.RetrieveSlice{
				{
					Slice: &knowledgemodel.Slice{
						KnowledgeID: 10,
						DocumentName: "guide",
						RawContent: []*knowledgemodel.SliceContent{
							{Type: knowledgemodel.SliceContentTypeText, Text: "knowledge text"},
						},
					},
					Score: 0.9,
				},
			},
		},
	}
	app := &ApplicationService{
		knowledgeSVC: knowledge,
		chatModelProvider: func(ctx context.Context) (chatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(index int, in []*schema.Message) (*schema.Message, error) {
					require.Contains(t, in[0].Content, "knowledge text")
					return schema.AssistantMessage("ask answer", nil), nil
				},
			}, true, nil
		},
	}

	result, err := app.runAnswer(context.Background(), answerRequest{
		mode:    ChatModeAsk,
		message: "what is in docs",
	})

	require.NoError(t, err)
	require.Equal(t, "what is in docs", knowledge.lastQuery)
	require.Equal(t, []string{"knowledge"}, result.RetrievalSources)
	require.Equal(t, "ask answer", result.Message)
}
```

Add this fake knowledge service at the bottom of the file:

```go
type fakeKnowledgeService struct {
	retrieveResp *knowledgemodel.RetrieveResponse
	lastQuery    string
}

func (f *fakeKnowledgeService) ListKnowledge(ctx context.Context, request *knowledgemodel.ListKnowledgeRequest) (*knowledgemodel.ListKnowledgeResponse, error) {
	return &knowledgemodel.ListKnowledgeResponse{}, nil
}

func (f *fakeKnowledgeService) GetKnowledgeByID(ctx context.Context, request *knowledgemodel.GetKnowledgeByIDRequest) (*knowledgemodel.GetKnowledgeByIDResponse, error) {
	return &knowledgemodel.GetKnowledgeByIDResponse{}, nil
}

func (f *fakeKnowledgeService) Retrieve(ctx context.Context, req *knowledgemodel.RetrieveRequest) (*knowledgemodel.RetrieveResponse, error) {
	f.lastQuery = req.Query
	return f.retrieveResp, nil
}

func (f *fakeKnowledgeService) DeleteKnowledge(ctx context.Context, request *knowledgemodel.DeleteKnowledgeRequest) error {
	return nil
}

func (f *fakeKnowledgeService) MGetKnowledgeByID(ctx context.Context, request *knowledgemodel.MGetKnowledgeByIDRequest) (*knowledgemodel.MGetKnowledgeByIDResponse, error) {
	return &knowledgemodel.MGetKnowledgeByIDResponse{}, nil
}

func (f *fakeKnowledgeService) Store(ctx context.Context, document *knowledgemodel.CreateDocumentRequest) (*knowledgemodel.CreateDocumentResponse, error) {
	return &knowledgemodel.CreateDocumentResponse{}, nil
}

func (f *fakeKnowledgeService) Delete(ctx context.Context, r *knowledgemodel.DeleteDocumentRequest) (*knowledgemodel.DeleteDocumentResponse, error) {
	return &knowledgemodel.DeleteDocumentResponse{}, nil
}

func (f *fakeKnowledgeService) ListKnowledgeDetail(ctx context.Context, req *knowledgemodel.ListKnowledgeDetailRequest) (*knowledgemodel.ListKnowledgeDetailResponse, error) {
	return &knowledgemodel.ListKnowledgeDetailResponse{}, nil
}

func (f *fakeKnowledgeService) MGetSlice(ctx context.Context, request *knowledgemodel.MGetSliceRequest) (*knowledgemodel.MGetSliceResponse, error) {
	return &knowledgemodel.MGetSliceResponse{}, nil
}

func (f *fakeKnowledgeService) MGetDocument(ctx context.Context, request *knowledgemodel.MGetDocumentRequest) (*knowledgemodel.MGetDocumentResponse, error) {
	return &knowledgemodel.MGetDocumentResponse{}, nil
}
```

- [ ] **Step 2: Run answer runner tests and verify failure**

Run:

```bash
cd backend && go test ./application/workbench -run 'TestRunAutoAnswerUsesBuiltinModelAndMarksArk|TestRunAskRetrievesKnowledgeContext' -count=1
```

Expected: FAIL because `runAnswer`, `answerRequest`, result payload types, and injections do not exist.

- [ ] **Step 3: Add result payload helpers**

Create `backend/application/workbench/result_payload.go`:

```go
package workbench

import "encoding/json"

const (
	resultTypeAnswer     = "answer"
	resultTypeAgentTrace = "agent_trace"
	resultTypeReport     = "report"

	executionTypeArk   = "Ark"
	executionTypeAgent = "Agent"
)

type resultPayload struct {
	Message          string   `json:"message"`
	ResultType       string   `json:"result_type"`
	ExecutionType    string   `json:"execution_type,omitempty"`
	RetrievalSources []string `json:"retrieval_sources,omitempty"`
}

func marshalResultPayload(payload resultPayload) (string, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
```

- [ ] **Step 4: Add answer runner**

Create `backend/application/workbench/answer_runner.go`:

```go
package workbench

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/coze-dev/coze-studio/backend/bizpkg/llm/modelbuilder"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	knowledgemodel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
)

type chatModel interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

type chatModelProvider func(ctx context.Context) (chatModel, bool, error)

type answerRequest struct {
	mode    ChatMode
	message string
}

func defaultChatModelProvider(ctx context.Context) (chatModel, bool, error) {
	return modelbuilder.GetBuiltinChatModel(ctx, "WKB_")
}

func (s *ApplicationService) runAnswer(ctx context.Context, req answerRequest) (resultPayload, error) {
	provider := s.chatModelProvider
	if provider == nil {
		provider = defaultChatModelProvider
	}
	cm, configured, err := provider(ctx)
	if err != nil {
		return resultPayload{}, err
	}
	if !configured || cm == nil {
		return resultPayload{}, fmt.Errorf("builtin chat model is not configured")
	}

	messages := []*schema.Message{schema.UserMessage(req.message)}
	sources := []string(nil)
	if req.mode == ChatModeAsk {
		contextText, ok := s.retrieveAskKnowledge(ctx, req.message)
		if ok {
			sources = append(sources, "knowledge")
			messages = []*schema.Message{
				schema.SystemMessage("Use the following knowledge context when it is relevant. If it is not relevant, answer normally.\n\n" + contextText),
				schema.UserMessage(req.message),
			}
		}
	}

	output, err := cm.Generate(ctx, messages)
	if err != nil {
		return resultPayload{}, err
	}
	payload := resultPayload{
		Message:          strings.TrimSpace(output.Content),
		ResultType:       resultTypeAnswer,
		RetrievalSources: sources,
	}
	if req.mode == ChatModeAuto {
		payload.ExecutionType = executionTypeArk
	}
	return payload, nil
}

func (s *ApplicationService) retrieveAskKnowledge(ctx context.Context, message string) (string, bool) {
	knowledgeSVC := s.knowledgeSVC
	if knowledgeSVC == nil {
		knowledgeSVC = crossknowledge.DefaultSVC()
	}
	if knowledgeSVC == nil {
		return "", false
	}
	resp, err := knowledgeSVC.Retrieve(ctx, &knowledgemodel.RetrieveRequest{
		Query: message,
		Strategy: &knowledgemodel.RetrievalStrategy{
			SearchType: knowledgemodel.SearchTypeHybrid,
		},
	})
	if err != nil || resp == nil || len(resp.RetrieveSlices) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(resp.RetrieveSlices))
	for _, item := range resp.RetrieveSlices {
		if item == nil || item.Slice == nil {
			continue
		}
		content := strings.TrimSpace(item.Slice.GetSliceContent())
		if content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("- %s: %s", item.Slice.DocumentName, content))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}
```

- [ ] **Step 5: Extend Workbench service components**

In `backend/application/workbench/init.go`, add fields:

```go
KnowledgeSVC      crossknowledge.Knowledge
AgentRunSVC       agentrun.Run
ChatModelProvider chatModelProvider
```

In `ApplicationService`, add:

```go
knowledgeSVC       crossknowledge.Knowledge
agentRunSVC        agentrun.Run
chatModelProvider  chatModelProvider
```

Wire them in `InitService`.

- [ ] **Step 6: Run answer tests**

Run:

```bash
cd backend && go test ./application/workbench -run 'TestRunAutoAnswerUsesBuiltinModelAndMarksArk|TestRunAskRetrievesKnowledgeContext' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit answer runner changes**

```bash
git add backend/application/workbench/init.go backend/application/workbench/result_payload.go backend/application/workbench/answer_runner.go backend/application/workbench/answer_runner_test.go
git commit -m "feat: add workbench answer runner"
```

---

## Task 4: Add Minimal AgentRun Adapter

**Files:**
- Create: `backend/application/workbench/agent_runner.go`
- Create: `backend/application/workbench/agent_runner_test.go`
- Create: `backend/application/workbench/test_fakes_test.go`

- [ ] **Step 1: Write failing AgentRun adapter test**

Create `backend/application/workbench/agent_runner_test.go`:

```go
package workbench

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
)

func TestRunAgentAppendsStreamEventsAndFinalResult(t *testing.T) {
	stream := newAgentRunStream([]*agentrunentity.AgentRunResponse{
		{
			Event: agentrunentity.RunEventCreated,
			ChunkRunItem: &agentrunentity.ChunkRunItem{
				ID: 77,
			},
		},
		{
			Event: agentrunentity.RunEventMessageDelta,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: "answer",
				Content:     "hello ",
			},
		},
		{
			Event: agentrunentity.RunEventMessageCompleted,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: "answer",
				Content:     "hello world",
				IsFinish:   true,
			},
		},
		{Event: agentrunentity.RunEventStreamDone},
	})
	taskSVC := &recordingWorkbenchTaskApp{}
	agentRun := &fakeAgentRun{stream: stream}
	app := &ApplicationService{
		taskApp:     taskSVC,
		agentRunSVC: agentRun,
	}

	result, err := app.runAgent(context.Background(), agentRequest{
		taskID:          10,
		spaceID:         1,
		message:         "do work",
		enableDatabases: []string{"db-a"},
	})

	require.NoError(t, err)
	require.Equal(t, resultTypeAgentTrace, result.ResultType)
	require.Equal(t, executionTypeAgent, result.ExecutionType)
	require.Equal(t, "hello world", result.Message)
	require.NotNil(t, agentRun.req)
	require.Equal(t, "db-a", agentRun.req.Ext["enable_databases"])
	require.GreaterOrEqual(t, len(taskSVC.events), 3)
	require.Equal(t, "agent.run_started", taskSVC.events[0].eventType)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(taskSVC.events[len(taskSVC.events)-1].payload), &payload))
	require.Equal(t, "hello world", payload["message"])
}
```

Add helper fakes in this test file:

```go
func newAgentRunStream(chunks []*agentrunentity.AgentRunResponse) *schema.StreamReader[*agentrunentity.AgentRunResponse] {
	sr, sw := schema.Pipe[*agentrunentity.AgentRunResponse](10)
	go func() {
		defer sw.Close()
		for _, chunk := range chunks {
			sw.Send(chunk, nil)
		}
	}()
	return sr
}
```

- [ ] **Step 2: Add shared backend test fakes**

Create `backend/application/workbench/test_fakes_test.go`:

```go
package workbench

import (
	"context"

	"github.com/cloudwego/eino/schema"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
)

type recordedWorkbenchEvent struct {
	taskID    int64
	eventType string
	payload   string
}

type recordingWorkbenchTaskApp struct {
	created *taskapi.ChatTask
	got     *taskapi.ChatTask

	createCalls int

	events          []recordedWorkbenchEvent
	completedID     int64
	completedResult string
	failedID        int64
	failError       string
}

func (r *recordingWorkbenchTaskApp) CreateRunningTask(ctx context.Context, req *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	r.createCalls++
	if r.created == nil {
		r.created = &taskapi.ChatTask{
			ID:      1,
			SpaceID: req.SpaceID,
			Title:   req.Title,
			Status:  taskapi.TaskStatus_Running,
		}
	}
	return &taskapi.CreateTaskResponse{Code: 0, Msg: "success", Data: r.created}, nil
}

func (r *recordingWorkbenchTaskApp) GetTask(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error) {
	if r.got == nil {
		r.got = &taskapi.ChatTask{
			ID:     req.TaskID,
			Status: taskapi.TaskStatus_Running,
		}
	}
	return &taskapi.GetTaskResponse{Code: 0, Msg: "success", Data: r.got}, nil
}

func (r *recordingWorkbenchTaskApp) AppendTaskEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	r.events = append(r.events, recordedWorkbenchEvent{taskID: taskID, eventType: eventType, payload: payload})
	return nil
}

func (r *recordingWorkbenchTaskApp) CompleteTask(ctx context.Context, taskID int64, result string) error {
	r.completedID = taskID
	r.completedResult = result
	return nil
}

func (r *recordingWorkbenchTaskApp) FailTask(ctx context.Context, taskID int64, errMsg string) error {
	r.failedID = taskID
	r.failError = errMsg
	return nil
}

type fakeAgentRun struct {
	stream *schema.StreamReader[*agentrunentity.AgentRunResponse]
	req    *agentrunentity.AgentRunMeta
}

func (f *fakeAgentRun) AgentRun(ctx context.Context, req *agentrunentity.AgentRunMeta) (*schema.StreamReader[*agentrunentity.AgentRunResponse], error) {
	f.req = req
	return f.stream, nil
}

func (f *fakeAgentRun) Delete(ctx context.Context, runID []int64) error {
	return nil
}

func (f *fakeAgentRun) Create(ctx context.Context, runRecord *agentrunentity.AgentRunMeta) (*agentrunentity.RunRecordMeta, error) {
	return &agentrunentity.RunRecordMeta{}, nil
}

func (f *fakeAgentRun) List(ctx context.Context, meta *agentrunentity.ListRunRecordMeta) ([]*agentrunentity.RunRecordMeta, error) {
	return nil, nil
}

func (f *fakeAgentRun) GetByID(ctx context.Context, runID int64) (*agentrunentity.RunRecordMeta, error) {
	return &agentrunentity.RunRecordMeta{}, nil
}

func (f *fakeAgentRun) Cancel(ctx context.Context, req *agentrunentity.CancelRunMeta) (*agentrunentity.RunRecordMeta, error) {
	return &agentrunentity.RunRecordMeta{}, nil
}
```

- [ ] **Step 3: Run test and verify failure**

Run:

```bash
cd backend && go test ./application/workbench -run TestRunAgentAppendsStreamEventsAndFinalResult -count=1
```

Expected: FAIL because `runAgent`, `agentRequest`, and test fakes do not exist.

- [ ] **Step 4: Implement AgentRun adapter**

Create `backend/application/workbench/agent_runner.go`:

```go
package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/coze-dev/coze-studio/backend/api/model/conversation/common"
	crossmessage "github.com/coze-dev/coze-studio/backend/crossdomain/message/model"
	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type agentRequest struct {
	taskID          int64
	spaceID         int64
	conversationID  int64
	message         string
	enableSkills    []string
	enableMcp       []string
	enableKbs       []string
	enableDatabases []string
}

func (s *ApplicationService) runAgent(ctx context.Context, req agentRequest) (resultPayload, error) {
	runner := s.agentRunSVC
	if runner == nil {
		return resultPayload{}, fmt.Errorf("workbench agent run service is not initialized")
	}

	stream, err := runner.AgentRun(ctx, &agentrunentity.AgentRunMeta{
		ConversationID: req.conversationID,
		ConnectorID:    consts.CozeConnectorID,
		SpaceID:        req.spaceID,
		Scene:          common.Scene_SceneWebSDK,
		ContentType:    crossmessage.ContentTypeText,
		Content: []*crossmessage.InputMetaData{
			{Type: crossmessage.InputTypeText, Text: req.message},
		},
		DisplayContent: req.message,
		Ext: map[string]string{
			"workbench_task_id": fmt.Sprintf("%d", req.taskID),
			"enable_skills":     strings.Join(req.enableSkills, ","),
			"enable_mcp":        strings.Join(req.enableMcp, ","),
			"enable_kbs":        strings.Join(req.enableKbs, ","),
			"enable_databases":  strings.Join(req.enableDatabases, ","),
		},
	})
	if err != nil {
		return resultPayload{}, err
	}

	finalAnswer := ""
	for {
		chunk, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return resultPayload{}, recvErr
		}
		eventType, payload := agentChunkToTaskEvent(chunk)
		if eventType != "" {
			if err := s.appendTaskEvent(ctx, req.taskID, eventType, payload); err != nil {
				return resultPayload{}, err
			}
		}
		if chunk != nil && chunk.ChunkMessageItem != nil && chunk.ChunkMessageItem.IsFinish {
			finalAnswer = strings.TrimSpace(chunk.ChunkMessageItem.Content)
		}
	}

	return resultPayload{
		Message:       finalAnswer,
		ResultType:    resultTypeAgentTrace,
		ExecutionType: executionTypeAgent,
	}, nil
}

func agentChunkToTaskEvent(chunk *agentrunentity.AgentRunResponse) (string, string) {
	if chunk == nil {
		return "", ""
	}
	payload := map[string]string{
		"status": "running",
		"title":  string(chunk.Event),
	}
	switch chunk.Event {
	case agentrunentity.RunEventCreated:
		payload["title"] = "AgentRun 已启动"
		payload["status"] = "completed"
		return "agent.run_started", mustJSON(payload)
	case agentrunentity.RunEventMessageDelta:
		if chunk.ChunkMessageItem != nil {
			payload["title"] = "生成回答"
			payload["message"] = chunk.ChunkMessageItem.Content
		}
		return "agent.answer_delta", mustJSON(payload)
	case agentrunentity.RunEventMessageCompleted:
		if chunk.ChunkMessageItem != nil {
			payload["title"] = "Agent 最终结果"
			payload["message"] = chunk.ChunkMessageItem.Content
			payload["status"] = "completed"
		}
		return "agent.run_completed", mustJSON(payload)
	case agentrunentity.RunEventError:
		payload["title"] = "Agent 执行失败"
		payload["status"] = "failed"
		if chunk.Error != nil {
			payload["detail"] = chunk.Error.Msg
		}
		return "agent.run_failed", mustJSON(payload)
	default:
		return "", ""
	}
}

func mustJSON(payload map[string]string) string {
	bytes, _ := json.Marshal(payload)
	return string(bytes)
}

```

- [ ] **Step 5: Add task app wrapper for tests and production**

In `backend/application/workbench/chat_gateway.go` or a small helper file, add:

```go
type workbenchTaskApplication interface {
	CreateRunningTask(ctx context.Context, req *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error)
	GetTask(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error)
	AppendTaskEvent(ctx context.Context, taskID int64, eventType, payload string) error
	CompleteTask(ctx context.Context, taskID int64, result string) error
	FailTask(ctx context.Context, taskID int64, errMsg string) error
}

func (s *ApplicationService) appendTaskEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	if s.taskApp != nil {
		return s.taskApp.AppendTaskEvent(ctx, taskID, eventType, payload)
	}
	if s.taskSVC == nil {
		return fmt.Errorf("workbench task service is not initialized")
	}
	return s.taskSVC.AppendTaskEvent(ctx, taskID, eventType, payload)
}
```

Use the real task application in production and fake task application in tests.

- [ ] **Step 6: Run AgentRun adapter test**

Run:

```bash
cd backend && go test ./application/workbench -run TestRunAgentAppendsStreamEventsAndFinalResult -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit AgentRun adapter**

```bash
git add backend/application/workbench/agent_runner.go backend/application/workbench/agent_runner_test.go backend/application/workbench/test_fakes_test.go backend/application/workbench/chat_gateway.go
git commit -m "feat: add minimal workbench agent runner"
```

---

## Task 5: Orchestrate WorkbenchChat Create And Append Paths

**Files:**
- Modify: `backend/application/workbench/chat_gateway.go`
- Modify: `backend/application/workbench/chat_gateway_test.go`
- Modify: `backend/application/workbench/router_decision.go`
- Modify: `backend/application/application.go`

- [ ] **Step 1: Write failing orchestration tests**

Add to `backend/application/workbench/chat_gateway_test.go`:

```go
func TestHandleMessageAutoCreatesTaskAndCompletesAnswer(t *testing.T) {
	taskApp := &recordingWorkbenchTaskApp{
		created: &taskapi.ChatTask{ID: 10, SpaceID: 1, Title: "hello", Status: taskapi.TaskStatus_Running},
	}
	app := &ApplicationService{
		taskApp: taskApp,
		chatModelProvider: func(ctx context.Context) (chatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(index int, in []*schema.Message) (*schema.Message, error) {
					return schema.AssistantMessage("auto answer", nil), nil
				},
			}, true, nil
		},
	}

	resp, err := app.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: "hello",
		Mode:    chatapi.ChatMode_Auto,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Data.Task)
	require.Equal(t, int64(10), resp.Data.Task.ID)
	require.Equal(t, "answer", resp.Data.GetResultType())
	require.Equal(t, "Ark", resp.Data.GetExecutionType())
	require.Contains(t, taskApp.completedResult, `"message":"auto answer"`)
	require.Equal(t, "answer.completed", taskApp.events[len(taskApp.events)-1].eventType)
}

func TestHandleMessageWithTaskIDAppendsUserMessageWithoutCreatingTask(t *testing.T) {
	taskID := int64(20)
	taskApp := &recordingWorkbenchTaskApp{
		got: &taskapi.ChatTask{ID: 20, SpaceID: 1, Title: "existing", Status: taskapi.TaskStatus_Running},
	}
	app := &ApplicationService{
		taskApp: taskApp,
		chatModelProvider: func(ctx context.Context) (chatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(index int, in []*schema.Message) (*schema.Message, error) {
					return schema.AssistantMessage("follow answer", nil), nil
				},
			}, true, nil
		},
	}

	resp, err := app.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		TaskID:  &taskID,
		Message: "follow up",
		Mode:    chatapi.ChatMode_Ask,
	})

	require.NoError(t, err)
	require.Equal(t, int64(20), resp.Data.Task.ID)
	require.Equal(t, 0, taskApp.createCalls)
	require.Equal(t, "user.message", taskApp.events[0].eventType)
	require.Contains(t, taskApp.completedResult, `"message":"follow answer"`)
}
```

- [ ] **Step 2: Run orchestration tests and verify failure**

Run:

```bash
cd backend && go test ./application/workbench -run 'TestHandleMessageAutoCreatesTaskAndCompletesAnswer|TestHandleMessageWithTaskIDAppendsUserMessageWithoutCreatingTask' -count=1
```

Expected: FAIL because chat gateway still uses old route responses and fallback task creation.

- [ ] **Step 3: Replace old routing responses**

In `backend/application/workbench/chat_gateway.go`, change `HandleMessage` so it:

1. Validates request and mode.
2. Creates a running task when `TaskID` is absent.
3. Gets existing task and appends `user.message` when `TaskID` is present.
4. Runs `runAnswer` for Auto/Ask and `runAgent` for Agent.
5. Appends completion event.
6. Completes task with `marshalResultPayload`.
7. Returns `Task`, `ResultType`, `ExecutionType`, and `Answer`.
8. Fails task and appends failure event when model/agent execution errors.

Use this flow:

```go
func (s *ApplicationService) HandleMessage(ctx context.Context, req *chatapi.WorkbenchChatRequest) (*chatapi.WorkbenchChatResponse, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("workbench chat request is required")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, InvalidArgumentErrorf("message is required")
	}
	mode, err := chatModeFromAPI(req.Mode)
	if err != nil {
		return nil, err
	}

	task, err := s.prepareTask(ctx, req, message)
	if err != nil {
		return nil, err
	}

	result, err := s.executeTurn(ctx, task.ID, req, mode, message)
	if err != nil {
		_ = s.failTask(ctx, task.ID, err.Error())
		return nil, err
	}
	resultJSON, err := marshalResultPayload(result)
	if err != nil {
		_ = s.failTask(ctx, task.ID, err.Error())
		return nil, err
	}
	if err := s.completeTask(ctx, task.ID, resultJSON); err != nil {
		return nil, err
	}
	task.Result = &resultJSON

	return &chatapi.WorkbenchChatResponse{
		Code: 0,
		Msg:  "success",
		Data: &chatapi.WorkbenchChatData{
			RouteTarget:   chatapi.RouteTarget_TaskEngine,
			Answer:        stringPtr(result.Message),
			Task:          task,
			ResultType:    stringPtr(result.ResultType),
			ExecutionType: stringPtr(result.ExecutionType),
		},
	}, nil
}
```

- [ ] **Step 4: Implement prepareTask and executeTurn helpers**

Add helpers in the same file:

```go
func (s *ApplicationService) prepareTask(ctx context.Context, req *chatapi.WorkbenchChatRequest, message string) (*taskapi.ChatTask, error) {
	if req.IsSetTaskID() {
		task, err := s.getTask(ctx, req.GetTaskID())
		if err != nil {
			return nil, err
		}
		if err := s.appendTaskEvent(ctx, req.GetTaskID(), "user.message", mustJSON(map[string]string{
			"message": message,
			"status":  "completed",
			"title":   "用户追问",
		})); err != nil {
			return nil, err
		}
		return task, nil
	}

	input, err := taskInputJSON(message, executionTypeFromMode(req.Mode), resultTypeFromMode(req.Mode))
	if err != nil {
		return nil, err
	}
	return s.createRunningTask(ctx, &taskapi.CreateTaskRequest{
		SpaceID:        req.SpaceID,
		Title:          taskTitle(message),
		ConversationID: req.ConversationID,
		SkillID:        req.SelectedSkillID,
		Input:          &input,
	})
}

func (s *ApplicationService) executeTurn(ctx context.Context, taskID int64, req *chatapi.WorkbenchChatRequest, mode ChatMode, message string) (resultPayload, error) {
	if mode == ChatModeAgent {
		return s.runAgent(ctx, agentRequest{
			taskID:          taskID,
			spaceID:         req.SpaceID,
			conversationID:  req.GetConversationID(),
			message:         message,
			enableSkills:    req.GetEnableSkills(),
			enableMcp:       req.GetEnableMcp(),
			enableKbs:       req.GetEnableKbs(),
			enableDatabases: req.GetEnableDatabases(),
		})
	}
	result, err := s.runAnswer(ctx, answerRequest{mode: mode, message: message})
	if err != nil {
		return resultPayload{}, err
	}
	if err := s.appendTaskEvent(ctx, taskID, "answer.completed", mustJSON(map[string]string{
		"title":   "生成回答",
		"message": result.Message,
		"status":  "completed",
		"runtime": result.ExecutionType,
	})); err != nil {
		return resultPayload{}, err
	}
	return result, nil
}
```

Update `taskInputJSON` signature to include result type:

```go
func taskInputJSON(message, executionType, resultType string) (string, error) {
	payload := map[string]string{
		"message":        message,
		"execution_type": executionType,
		"result_type":    resultType,
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
```

- [ ] **Step 5: Wire real services after complex service init**

In `backend/application/application.go`, after `complexServices` is created and before returning from `Init`, call:

```go
workbench.InitService(&workbench.ServiceComponents{
	SkillSVC:      primaryServices.skillSVC,
	TaskSVC:       primaryServices.taskSVC,
	KnowledgeSVC:  crossknowledge.DefaultSVC(),
	AgentRunSVC:   complexServices.conversationSVC.AgentRunDomainSVC,
})
```

Keep the earlier `InitService` in `initPrimaryServices` for Skill/Task availability, then this second call enriches runtime services.

- [ ] **Step 6: Run Workbench backend tests**

Run:

```bash
cd backend && go test ./application/workbench -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit chat gateway orchestration**

```bash
git add backend/application/workbench backend/application/application.go
git commit -m "feat: route workbench chat through real runtimes"
```

---

## Task 6: Extract Shared WorkbenchComposer And Update Home Send

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/components/types.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/extensions-popover.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [ ] **Step 1: Write failing home tests for single chat gateway send**

Update `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`:

Remove expectations that `createWorkbenchTask` is called after direct answer or chat failure.

Add this test:

```tsx
it('sends through WorkbenchChat only and navigates to returned task', async () => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;

  mockSendWorkbenchChat.mockResolvedValue({
    data: {
      result_type: 'answer',
      execution_type: 'Ark',
      task: { id: 'task-1' },
      answer: '完成',
    },
    code: 0,
    msg: '',
  });

  act(() => {
    root = createRoot(container);
    root.render(<WorkbenchPage />);
  });

  const textarea = container.querySelector('textarea[aria-label="任务描述"]') as HTMLTextAreaElement;
  act(() => {
    Simulate.change(textarea, { target: { value: '帮我总结' } } as unknown as Event);
  });

  const sendButton = Array.from(container.querySelectorAll('button')).find(button =>
    button.textContent?.includes('发送'),
  ) as HTMLButtonElement;

  await act(async () => {
    sendButton.click();
    await Promise.resolve();
  });

  expect(mockSendWorkbenchChat).toHaveBeenCalledWith({
    space_id: 'space-1',
    message: '帮我总结',
    mode: workbench.ChatMode.Auto,
  });
  expect(mockCreateWorkbenchTask).not.toHaveBeenCalled();
  expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/tasks/task-1');

  act(() => root?.unmount());
  container.remove();
});
```

- [ ] **Step 2: Run home test and verify failure**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
```

Expected: FAIL because current page still has embedded composer and fallback task creation tests/behavior.

- [ ] **Step 3: Create shared types**

Create `frontend/apps/coze-studio/src/pages/workbench/components/types.ts`:

```ts
import { workbench } from '@coze-studio/api-schema';

export const WORKBENCH_MODES = ['Auto', 'Ask', 'Agent'] as const;

export type WorkbenchMode = (typeof WORKBENCH_MODES)[number];

export interface WorkbenchComposerSubmitPayload {
  message: string;
  mode: WorkbenchMode;
  enableSkills?: string[];
  enableMcp?: string[];
  enableKbs?: string[];
  enableDatabases?: string[];
}

export const mapModeToChatMode = (mode: WorkbenchMode): workbench.ChatMode => {
  const modeMap: Record<WorkbenchMode, workbench.ChatMode> = {
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};
```

- [ ] **Step 4: Extract WorkbenchComposer**

Create `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx` by moving the existing `WorkbenchComposer`, `AtMenu`, mode prompts, mode symbols, and `AT_RESOURCES` from `workbench/index.tsx`.

Use this public signature:

```tsx
interface WorkbenchComposerProps {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  variant?: 'home' | 'detail';
  taskId?: string;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void;
}
```

Inside the component:

```tsx
const canSend = Boolean(value.trim()) && !loading;

const handleSend = () => {
  const message = value.trim();
  if (!message || loading) {
    return;
  }
  onSubmit({
    message,
    mode,
  });
};
```

Keep `ExtensionsPopover` rendered for both variants.

- [ ] **Step 5: Update home page to use shared composer and remove fallback creation**

In `frontend/apps/coze-studio/src/pages/workbench/index.tsx`:

1. Import `WorkbenchComposer`, `WorkbenchMode`, and `mapModeToChatMode`.
2. Remove local `WorkbenchComposer`, `AtMenu`, mode constants, `createTaskInput`, `getExecutionTypeFromMode`, and `navigateToCreatedTask`.
3. Remove `createWorkbenchTask` from imports.
4. Make `handleSend` accept payload:

```tsx
const handleSend = async (payload: WorkbenchComposerSubmitPayload) => {
  if (!space_id || loading) {
    return;
  }

  setLoading(true);
  setError('');

  try {
    const response = await sendWorkbenchChat({
      space_id,
      message: payload.message,
      mode: mapModeToChatMode(payload.mode),
      enable_skills: payload.enableSkills,
      enable_mcp: payload.enableMcp,
      enable_kbs: payload.enableKbs,
      enable_databases: payload.enableDatabases,
    });

    setValue('');

    if (response.data?.task?.id) {
      navigate(`/space/${space_id}/tasks/${response.data.task.id}`);
      return;
    }

    navigate(`/space/${space_id}/tasks`);
  } catch (err) {
    setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
  } finally {
    setLoading(false);
  }
};
```

Render:

```tsx
<WorkbenchComposer
  value={value}
  mode={mode}
  loading={loading}
  error={error}
  variant="home"
  onValueChange={setValue}
  onModeChange={setMode}
  onSubmit={handleSend}
/>
```

- [ ] **Step 6: Run home tests**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit shared home composer**

```bash
git add frontend/apps/coze-studio/src/pages/workbench
git commit -m "feat: extract workbench composer"
```

---

## Task 7: Reuse Composer On Task Detail And Render Result Types

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/helpers.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-prototype.less`

- [ ] **Step 1: Write failing task detail tests**

Update `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`.

Mock `sendWorkbenchChat`:

```tsx
const mockSendWorkbenchChat = vi.hoisted(() => vi.fn());

vi.mock('../../workbench/service', () => ({
  sendWorkbenchChat: mockSendWorkbenchChat,
}));
```

Add test:

```tsx
it('sends follow-up with task_id and refreshes in place', async () => {
  mockSendWorkbenchChat.mockResolvedValue({
    data: {
      task: { id: 'task-1' },
      result_type: 'answer',
      answer: '追加回答',
    },
    code: 0,
    msg: '',
  });

  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;

  await act(async () => {
    root = createRoot(container);
    root.render(<TaskDetailPage />);
    await Promise.resolve();
  });

  const textarea = container.querySelector('textarea[aria-label="任务描述"]') as HTMLTextAreaElement;
  act(() => {
    textarea.value = '继续问';
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
  });

  const sendButton = Array.from(container.querySelectorAll('button')).find(button =>
    button.textContent?.includes('发送'),
  ) as HTMLButtonElement;

  await act(async () => {
    sendButton.click();
    await Promise.resolve();
  });

  expect(mockSendWorkbenchChat).toHaveBeenCalledWith(
    expect.objectContaining({
      space_id: 'space-1',
      task_id: 'task-1',
      message: '继续问',
    }),
  );
  expect(mockGetTask).toHaveBeenCalledTimes(2);
  expect(container.textContent).not.toContain('/space/space-1/tasks/');

  act(() => root?.unmount());
  container.remove();
});
```

Add rendering assertions for result types:

```tsx
it('renders answer result without report wrapper', async () => {
  mockGetTask.mockResolvedValueOnce({
    data: {
      id: 'task-1',
      space_id: 'space-1',
      creator_id: 'user-1',
      title: '普通问答',
      status: workbenchTask.TaskStatus.Succeeded,
      progress: 100,
      input: JSON.stringify({ message: '你好', result_type: 'answer' }),
      result: JSON.stringify({ message: '你好呀', result_type: 'answer' }),
      created_at: 1717000000000,
      updated_at: 1717000300000,
    },
    code: 0,
    msg: '',
  });

  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;

  await act(async () => {
    root = createRoot(container);
    root.render(<TaskDetailPage />);
    await Promise.resolve();
  });

  expect(container.textContent).toContain('你好呀');
  expect(container.textContent).not.toContain('一、任务输入');

  act(() => root?.unmount());
  container.remove();
});
```

- [ ] **Step 2: Run task detail tests and verify failure**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: FAIL because detail still uses `FollowUpComposer` and report wrapper.

- [ ] **Step 3: Add result parser helpers**

In `frontend/apps/coze-studio/src/pages/tasks/helpers.ts`, add:

```ts
export type WorkbenchResultType = 'answer' | 'agent_trace' | 'report';

export interface ParsedTaskResult {
  message: string;
  resultType: WorkbenchResultType;
  executionType?: string;
  retrievalSources: string[];
}

export const getParsedTaskResult = (
  result?: string,
  events: workbenchTask.TaskEvent[] = [],
): ParsedTaskResult => {
  const parsed = parseJSONObject(result);
  const resultType =
    getString(parsed, 'result_type') === 'agent_trace' ||
    getString(parsed, 'result_type') === 'report' ||
    getString(parsed, 'result_type') === 'answer'
      ? (getString(parsed, 'result_type') as WorkbenchResultType)
      : events.some(event => event.event_type?.startsWith('agent.'))
        ? 'agent_trace'
        : 'answer';

  const retrievalValue = parsed?.retrieval_sources;
  const retrievalSources = Array.isArray(retrievalValue)
    ? retrievalValue.filter((item): item is string => typeof item === 'string')
    : [];

  return {
    message: getTaskResultText(result),
    resultType,
    executionType: getString(parsed, 'execution_type'),
    retrievalSources,
  };
};

export const AGENT_TRACE_EVENT_LABELS: Record<string, string> = {
  'agent.run_started': 'Agent 启动',
  'agent.answer_delta': '生成回答',
  'agent.run_completed': 'Agent 完成',
  'agent.run_failed': 'Agent 失败',
  'agent.database_query': '数据库查询',
};
```

- [ ] **Step 4: Replace FollowUpComposer with shared composer**

In `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`:

1. Import `WorkbenchComposer`, `WorkbenchMode`, `mapModeToChatMode`, `sendWorkbenchChat`.
2. Add state:

```tsx
const [composerValue, setComposerValue] = useState('');
const [composerMode, setComposerMode] = useState<WorkbenchMode>('Ask');
const [sending, setSending] = useState(false);
```

3. Extract `loadTaskDetail` with `useCallback` so submit can reuse it.
4. Add:

```tsx
const handleFollowUpSubmit = async (payload: WorkbenchComposerSubmitPayload) => {
  if (!space_id || !task_id || sending) {
    return;
  }
  setSending(true);
  setError('');
  try {
    await sendWorkbenchChat({
      space_id,
      task_id,
      message: payload.message,
      mode: mapModeToChatMode(payload.mode),
      enable_skills: payload.enableSkills,
      enable_mcp: payload.enableMcp,
      enable_kbs: payload.enableKbs,
      enable_databases: payload.enableDatabases,
    });
    setComposerValue('');
    await loadTaskDetail();
  } catch (err) {
    setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
  } finally {
    setSending(false);
  }
};
```

5. Replace `<FollowUpComposer />` with:

```tsx
<WorkbenchComposer
  value={composerValue}
  mode={composerMode}
  loading={sending}
  variant="detail"
  taskId={task_id}
  onValueChange={setComposerValue}
  onModeChange={setComposerMode}
  onSubmit={handleFollowUpSubmit}
/>
```

- [ ] **Step 5: Render result type branches**

Replace `TaskReport` with:

```tsx
const TaskResult = ({ task, events }: { task: ChatTask; events: TaskEvent[] }) => {
  const parsed = getParsedTaskResult(task.result, events);

  if (parsed.resultType === 'report') {
    return (
      <article className="coze-prototype-report">
        <h2>{task.title}报告</h2>
        <p>{parsed.message || task.error || '结果生成中'}</p>
        <h3>一、任务输入</h3>
        <p>{getTaskInputText(task.input) || task.title}</p>
      </article>
    );
  }

  return (
    <article className="coze-prototype-answer">
      <p>{parsed.message || task.error || '结果生成中'}</p>
    </article>
  );
};
```

Keep `TaskEventsSection` visible for `agent_trace`; for `answer`, hide it when there are no agent events:

```tsx
const parsed = getParsedTaskResult(task.result, events);
{parsed.resultType === 'agent_trace' ? <TaskEventsSection events={events} task={task} /> : null}
<TaskResult task={task} events={events} />
```

- [ ] **Step 6: Add minimal answer styles**

In `frontend/apps/coze-studio/src/components/workspace-prototype.less`, add:

```less
.coze-prototype-answer {
  margin-top: 16px;
  padding: 16px;

  font-size: 14px;
  line-height: 22px;
  color: #232938;

  background: #fff;
  border: 1px solid rgb(77 101 148 / 12%);
  border-radius: 8px;
}

.coze-prototype-answer p {
  margin: 0;
  white-space: pre-wrap;
}
```

- [ ] **Step 7: Run task detail tests**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit task detail composer and rendering**

```bash
git add frontend/apps/coze-studio/src/pages/tasks frontend/apps/coze-studio/src/components/workspace-prototype.less
git commit -m "feat: reuse workbench composer in task detail"
```

---

## Task 8: Final Verification And Cleanup

**Files:**
- Review all touched backend and frontend files.

- [ ] **Step 1: Search for fake Agent/report code**

Run:

```bash
rg -n "本地占位执行结果|已进入 Agent 智能体模式|已进入 Chat 直接问答模式|不应该留在首页展示|生成结果报告|汇总处理结果并写入任务报告" backend frontend/apps/coze-studio/src
```

Expected: no production references to fake Agent or fake report behavior. Test names may remain only if explicitly testing absence; prefer removing old test cases.

- [ ] **Step 2: Run focused backend tests**

Run:

```bash
cd backend && go test ./application/workbench ./application/task ./domain/task/service -count=1
```

Expected: PASS.

- [ ] **Step 3: Run focused frontend tests**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Run broader package tests if focused tests pass**

Run:

```bash
cd frontend/apps/coze-studio && npm run test
cd backend && go test ./application/... ./domain/task/... ./domain/agent/singleagent/internal/agentflow -count=1
```

Expected: PASS. If unrelated existing failures appear, capture the failing package/test name and verify the focused changed tests still pass.

- [ ] **Step 5: Inspect final diff**

Run:

```bash
git diff --stat HEAD
git diff -- backend/application/workbench backend/application/task backend/domain/task/service idl/workbench frontend/apps/coze-studio/src/pages/workbench frontend/apps/coze-studio/src/pages/tasks frontend/packages/arch/api-schema/src/idl/workbench
```

Expected: diff only covers planned files and no generated noise outside Workbench/task/schema.

- [ ] **Step 6: Final commit**

If Task 8 produced cleanup changes:

```bash
git add backend frontend idl
git commit -m "test: verify workbench real agent flow"
```

If no cleanup changes exist, do not create an empty commit.

---

## Self-Review

**Spec coverage:**

- Shared `WorkbenchComposer`: Task 6 and Task 7.
- Single `sendWorkbenchChat` send path: Task 5, Task 6, Task 7.
- `task_id` create vs append behavior: Task 1 and Task 5.
- Auto Ark answer: Task 3 and Task 5.
- Ask knowledge retrieval and future search boundary: Task 3 and Task 7 result rendering.
- Minimal real AgentRun: Task 4 and Task 5.
- `enable_skills`, `enable_mcp`, `enable_kbs`, `enable_databases`: Task 1, Task 4, Task 6, Task 7.
- Result type rendering: Task 7.
- Remove fake Agent/report code: Task 2 and Task 8.

**Completeness scan:** The plan contains concrete file targets, test snippets, implementation snippets, and verification commands. Future product items are represented as reserved fields and explicit non-executed resource paths.

**Type consistency:** The plan consistently uses `task_id`, `enable_skills`, `enable_mcp`, `enable_kbs`, `enable_databases`, `result_type`, and `execution_type` across IDL, Go, TypeScript, and tests.
