# Agent Thread Workbench Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first production-grade thread-first foundation for the task workbench while keeping user-facing menu names as `新建任务`、`全部任务`、`我的任务`、`任务详情`.

**Architecture:** Phase 1 keeps the current task UI and workbench execution path alive, then adds canonical task thread routing and a Go domain/application skeleton for agent threads. The frontend routes move toward `/chats/*` as the canonical technical path, while labels remain task-oriented. The backend introduces an `agentthread` domain that can later own persistence, LangGraph-compatible runs, memory, artifacts, IM channels, and token usage without extending legacy `ChatTask` indefinitely.

**Tech Stack:** React 18, React Router 6, Vitest, TypeScript, Go, Hertz-compatible application services, GORM-oriented repositories, existing Coze task/workbench services.

---

## Scope

This plan implements the first vertical slice only:

1. Canonical frontend task thread routes and navigation helpers.
2. Frontend task pages and sidebar using canonical `/space/:space_id/chats/*` URLs while still calling existing task APIs.
3. Go-native `agentthread` domain types, repository contract, service validation, and in-memory tests.
4. Go application adapter that maps legacy `ChatTask` records into task thread summaries for read compatibility.

This plan does not implement SSE `/runs/stream`, persisted `agent_threads` tables, LangGraph HTTP compatibility, MCP tools, memory, artifacts, token accounting, or IM channel runtime. Those attach to the same `agentthread` domain in later plans.

## File Structure

- Create `frontend/apps/coze-studio/src/pages/chats/task-thread-routes.ts`
  - Owns canonical route construction and legacy route construction.
  - Prevents hard-coded `/tasks` paths from spreading.
- Modify `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
  - Keeps labels as task wording.
  - Changes `WORKBENCH` technical path to `chats/new` and `TASKS` to `chats`.
- Modify `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
  - Navigates sidebar `我的任务` rows to canonical task detail URLs.
- Modify `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
  - Navigates newly created workbench task results to canonical detail URLs.
  - Navigates direct/no-task responses to canonical all tasks URL.
- Modify `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
  - Navigates task rows to canonical detail URLs.
- Modify `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
  - Accepts either `task_id` or `thread_id` route params during compatibility.
- Modify `frontend/apps/coze-studio/src/routes/index.tsx`
  - Adds canonical `/chats`, `/chats/new`, `/chats/:thread_id`.
  - Keeps legacy `/workbench`, `/tasks`, `/tasks/:task_id` routes as replace redirects where possible.
- Update existing frontend tests:
  - `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`
  - `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
  - `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`
  - `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Create `backend/domain/agentthread/entity/thread.go`
  - Defines `Thread`, `Run`, `Message`, `RunEvent`, and status constants.
- Create `backend/domain/agentthread/repository/repository.go`
  - Defines repository contract for Phase 1 read/write boundaries.
- Create `backend/domain/agentthread/service/errors.go`
  - Defines client error helpers.
- Create `backend/domain/agentthread/service/service.go`
  - Defines `ThreadService`, `CreateThreadRequest`, `ListThreadsRequest`.
- Create `backend/domain/agentthread/service/service_impl.go`
  - Implements title validation, ID generation, defaults, and list normalization.
- Create `backend/domain/agentthread/service/service_impl_test.go`
  - Provides red-green coverage with an in-memory repository.
- Create `backend/application/agentthread/dto.go`
  - Defines API-facing application DTOs without depending on generated IDL in Phase 1.
- Create `backend/application/agentthread/thread_app.go`
  - Maps domain threads and legacy task records into task thread summaries.
- Create `backend/application/agentthread/thread_app_test.go`
  - Verifies legacy task mapping and task wording contracts.

## Task 1: Frontend Canonical Task Thread Routes

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/chats/task-thread-routes.ts`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Test: existing frontend test files listed above

- [ ] **Step 1: Write failing tests for canonical task paths**

Update `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`:

```tsx
expect(WORKSPACE_MENU_META[0]).toMatchObject({
  label: '新建任务',
  path: 'chats/new',
  variant: 'primary',
});
expect(WORKSPACE_MENU_META.at(-1)).toMatchObject({
  label: '全部任务',
  path: 'chats',
});
```

Update `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`:

```tsx
expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats/task-1');
expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats');
```

Update `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx` by clicking the first task row:

```tsx
const openButton = container.querySelector(
  'button[aria-label="打开任务 生成周报"]',
) as HTMLButtonElement;
act(() => {
  openButton.click();
});
expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats/task-1');
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: FAIL because current paths still use `workbench` and `tasks`.

- [ ] **Step 3: Add route helper implementation**

Create `frontend/apps/coze-studio/src/pages/chats/task-thread-routes.ts`:

```ts
/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

export const TASK_THREAD_ROUTES = {
  all: 'chats',
  new: 'chats/new',
} as const;

export const buildTaskThreadListPath = (spaceId: string) =>
  `/space/${spaceId}/${TASK_THREAD_ROUTES.all}`;

export const buildNewTaskThreadPath = (spaceId: string) =>
  `/space/${spaceId}/${TASK_THREAD_ROUTES.new}`;

export const buildTaskThreadDetailPath = (spaceId: string, threadId: string) =>
  `/space/${spaceId}/chats/${threadId}`;

export const buildLegacyTaskDetailPath = (spaceId: string, taskId: string) =>
  `/space/${spaceId}/tasks/${taskId}`;
```

- [ ] **Step 4: Replace hard-coded navigation**

Use the helper in:

```ts
import {
  TASK_THREAD_ROUTES,
  buildTaskThreadDetailPath,
  buildTaskThreadListPath,
} from '../chats/task-thread-routes';
```

Expected replacements:

```tsx
navigate(buildTaskThreadDetailPath(space_id, response.data.task.id));
navigate(buildTaskThreadListPath(space_id));
navigate(buildTaskThreadDetailPath(space_id, task.id));
```

In `workspace-task-list.tsx`, import with:

```ts
import { buildTaskThreadDetailPath } from '../../pages/chats/task-thread-routes';
```

Then navigate with:

```tsx
spaceId && navigate(buildTaskThreadDetailPath(spaceId, task.id))
```

- [ ] **Step 5: Update menu technical paths without changing labels**

Set `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`:

```ts
export const SPACE_SUB_MODULE = {
  WORKBENCH: 'chats/new',
  LIBRARY: 'library',
  SKILL: 'skill',
  DEVELOP: 'develop',
  TASK_TRIGGER: 'task-trigger',
  TASKS: 'chats',
} as const;
```

- [ ] **Step 6: Add canonical routes and compatibility redirects**

Update `frontend/apps/coze-studio/src/routes/index.tsx` under `space/:space_id`:

```tsx
{
  index: true,
  element: <Navigate to="chats/new" replace />,
},
{
  path: 'workbench',
  element: <Navigate to="../chats/new" replace relative="path" />,
},
{
  path: 'tasks',
  element: <Navigate to="../chats" replace relative="path" />,
},
{
  path: 'tasks/:task_id',
  Component: TaskDetailPage,
  loader: () => ({
    subMenuKey: SpaceSubModuleEnum.TASKS,
  }),
},
{
  path: 'chats/new',
  Component: Workbench,
  loader: () => ({
    subMenuKey: SpaceSubModuleEnum.WORKBENCH,
  }),
},
{
  path: 'chats/:thread_id',
  Component: TaskDetailPage,
  loader: () => ({
    subMenuKey: SpaceSubModuleEnum.TASKS,
  }),
},
{
  path: 'chats',
  Component: TasksPage,
  loader: () => ({
    subMenuKey: SpaceSubModuleEnum.TASKS,
  }),
},
```

Keep `tasks/:task_id` mounted to `TaskDetailPage` for Phase 1 because there is not yet a legacy resolver API.

- [ ] **Step 7: Let detail page accept both params**

In `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`:

```tsx
const { space_id, task_id, thread_id } = useParams();
const taskThreadId = thread_id ?? task_id;
```

Use `taskThreadId` in these exact places:

```tsx
if (!taskThreadId) {
  return;
}

const detail = await fetchTaskDetail(taskThreadId);
```

```tsx
if (!space_id || !taskThreadId) {
  setFollowUpError('缺少任务上下文，无法继续追问');

  return;
}
```

```tsx
await sendWorkbenchChat({
  space_id,
  task_id: taskThreadId,
  message: payload.message,
  mode: mapModeToChatMode(payload.mode),
  enable_skills: payload.enable_skills,
  enable_mcp: payload.enable_mcp,
  enable_kbs: payload.enable_kbs,
  enable_databases: payload.enable_databases,
});
```

```tsx
taskId={taskThreadId}
```

- [ ] **Step 8: Run frontend tests until green**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

- [ ] **Step 9: Commit frontend route slice**

```bash
git add frontend/apps/coze-studio/src/pages/chats/task-thread-routes.ts frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx frontend/apps/coze-studio/src/pages/workbench/index.tsx frontend/apps/coze-studio/src/pages/tasks/index.tsx frontend/apps/coze-studio/src/pages/tasks/detail.tsx frontend/apps/coze-studio/src/routes/index.tsx frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx
git commit -m "feat: add canonical task thread routes"
```

## Task 2: Go Agent Thread Domain

**Files:**
- Create: `backend/domain/agentthread/entity/thread.go`
- Create: `backend/domain/agentthread/repository/repository.go`
- Create: `backend/domain/agentthread/service/errors.go`
- Create: `backend/domain/agentthread/service/service.go`
- Create: `backend/domain/agentthread/service/service_impl.go`
- Create: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] **Step 1: Write failing service tests**

Create `backend/domain/agentthread/service/service_impl_test.go`:

```go
func TestCreateThreadRequiresTitle(t *testing.T) {
  repo := newMemoryRepo()
  svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

  _, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
    SpaceID: 1,
    UserID:  2,
    Title:   "  ",
  })

  require.Error(t, err)
  require.True(t, IsClientError(err))
}

func TestCreateThreadDefaultsToIdleWebTask(t *testing.T) {
  repo := newMemoryRepo()
  svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

  thread, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
    SpaceID: 1,
    UserID:  2,
    Title:   "分析订单异常",
  })

  require.NoError(t, err)
  require.Equal(t, int64(901), thread.ID)
  require.Equal(t, entity.ThreadStatusIdle, thread.Status)
  require.Equal(t, entity.ThreadSourceWeb, thread.Source)
  require.Equal(t, int64(1), thread.SpaceID)
  require.Equal(t, int64(2), thread.CreatorID)
}

func TestListThreadsNormalizesPaging(t *testing.T) {
  repo := newMemoryRepo()
  svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})
  _, _ = svc.CreateThread(context.Background(), &CreateThreadRequest{SpaceID: 1, UserID: 2, Title: "A"})
  _, _ = svc.CreateThread(context.Background(), &CreateThreadRequest{SpaceID: 1, UserID: 2, Title: "B"})

  threads, total, err := svc.ListThreads(context.Background(), &ListThreadsRequest{SpaceID: 1})

  require.NoError(t, err)
  require.Equal(t, int64(2), total)
  require.Len(t, threads, 2)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test ./domain/agentthread/service
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Add domain entities**

Create `backend/domain/agentthread/entity/thread.go`:

```go
package entity

type ThreadStatus string

const (
  ThreadStatusIdle      ThreadStatus = "idle"
  ThreadStatusRunning   ThreadStatus = "running"
  ThreadStatusFailed    ThreadStatus = "failed"
  ThreadStatusCompleted ThreadStatus = "completed"
  ThreadStatusCanceled  ThreadStatus = "canceled"
)

type ThreadSource string

const (
  ThreadSourceWeb ThreadSource = "web"
  ThreadSourceIM  ThreadSource = "im"
  ThreadSourceAPI ThreadSource = "api"
)

type RunStatus string

const (
  RunStatusQueued    RunStatus = "queued"
  RunStatusRunning   RunStatus = "running"
  RunStatusSucceeded RunStatus = "succeeded"
  RunStatusFailed    RunStatus = "failed"
  RunStatusCanceled  RunStatus = "canceled"
)

type MessageRole string

const (
  MessageRoleUser      MessageRole = "user"
  MessageRoleAssistant MessageRole = "assistant"
  MessageRoleTool      MessageRole = "tool"
  MessageRoleSystem    MessageRole = "system"
)

type Thread struct {
  ID             int64
  SpaceID        int64
  CreatorID      int64
  AgentID        int64
  Title          string
  Status         ThreadStatus
  Source         ThreadSource
  LegacyTaskID   int64
  Metadata       string
  CreatedAt      int64
  UpdatedAt      int64
  LastMessageAt  int64
}

type Run struct {
  ID        int64
  ThreadID  int64
  Status    RunStatus
  Input     string
  Output    string
  Error     string
  CreatedAt int64
  UpdatedAt int64
}

type Message struct {
  ID        int64
  ThreadID  int64
  RunID     int64
  Role      MessageRole
  Content   string
  Metadata  string
  CreatedAt int64
}

type RunEvent struct {
  ID        int64
  ThreadID  int64
  RunID     int64
  EventType string
  Payload   string
  CreatedAt int64
}
```

- [ ] **Step 4: Add repository and service implementation**

Create `backend/domain/agentthread/repository/repository.go`:

```go
package repository

import (
  "context"

  "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type ThreadRepository interface {
  CreateThread(ctx context.Context, thread *entity.Thread) error
  GetThread(ctx context.Context, id int64) (*entity.Thread, error)
  ListThreads(ctx context.Context, req ListThreadsRequest) ([]*entity.Thread, int64, error)
}

type ListThreadsRequest struct {
  SpaceID  int64
  UserID   int64
  Status   *entity.ThreadStatus
  Page     int32
  PageSize int32
}
```

Create `backend/domain/agentthread/service/errors.go`:

```go
package service

import (
  "errors"
  "fmt"
)

var ErrInvalidArgument = errors.New("invalid argument")

func InvalidArgumentErrorf(format string, args ...any) error {
  return fmt.Errorf("%w: %s", ErrInvalidArgument, fmt.Sprintf(format, args...))
}

func IsClientError(err error) bool {
  return errors.Is(err, ErrInvalidArgument)
}
```

Create `backend/domain/agentthread/service/service.go`:

```go
package service

import (
  "context"

  "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
  "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
  "github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type CreateThreadRequest struct {
  SpaceID      int64
  UserID       int64
  AgentID      int64
  Title        string
  Source       entity.ThreadSource
  LegacyTaskID int64
  Metadata     string
}

type ListThreadsRequest struct {
  SpaceID  int64
  UserID   int64
  Status   *entity.ThreadStatus
  Page     int32
  PageSize int32
}

type ThreadService interface {
  CreateThread(ctx context.Context, req *CreateThreadRequest) (*entity.Thread, error)
  GetThread(ctx context.Context, id int64) (*entity.Thread, error)
  ListThreads(ctx context.Context, req *ListThreadsRequest) ([]*entity.Thread, int64, error)
}

type Components struct {
  Repo  repository.ThreadRepository
  IDGen idgen.IDGenerator
}
```

Create `backend/domain/agentthread/service/service_impl.go`:

```go
package service

import (
  "context"
  "fmt"
  "strings"
  "time"

  "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
  "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
  "github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type threadService struct {
  repo  repository.ThreadRepository
  idGen idgen.IDGenerator
}

func NewService(c *Components) ThreadService {
  if c == nil {
    return &threadService{}
  }

  return &threadService{
    repo:  c.Repo,
    idGen: c.IDGen,
  }
}

func (s *threadService) CreateThread(ctx context.Context, req *CreateThreadRequest) (*entity.Thread, error) {
  if err := s.requireComponents(); err != nil {
    return nil, err
  }
  if req == nil {
    return nil, InvalidArgumentErrorf("create thread request is required")
  }
  title := strings.TrimSpace(req.Title)
  if title == "" {
    return nil, InvalidArgumentErrorf("thread title is required")
  }

  id, err := s.idGen.GenID(ctx)
  if err != nil {
    return nil, err
  }

  source := req.Source
  if source == "" {
    source = entity.ThreadSourceWeb
  }
  now := time.Now().UnixMilli()
  thread := &entity.Thread{
    ID:            id,
    SpaceID:       req.SpaceID,
    CreatorID:     req.UserID,
    AgentID:       req.AgentID,
    Title:         title,
    Status:        entity.ThreadStatusIdle,
    Source:        source,
    LegacyTaskID:  req.LegacyTaskID,
    Metadata:      req.Metadata,
    CreatedAt:     now,
    UpdatedAt:     now,
    LastMessageAt: now,
  }

  if err := s.repo.CreateThread(ctx, thread); err != nil {
    return nil, err
  }
  return thread, nil
}

func (s *threadService) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
  if err := s.requireRepo(); err != nil {
    return nil, err
  }
  return s.repo.GetThread(ctx, id)
}

func (s *threadService) ListThreads(ctx context.Context, req *ListThreadsRequest) ([]*entity.Thread, int64, error) {
  if err := s.requireRepo(); err != nil {
    return nil, 0, err
  }
  if req == nil {
    return nil, 0, InvalidArgumentErrorf("list threads request is required")
  }
  page := req.Page
  if page <= 0 {
    page = 1
  }
  pageSize := req.PageSize
  if pageSize <= 0 {
    pageSize = 20
  }
  return s.repo.ListThreads(ctx, repository.ListThreadsRequest{
    SpaceID:  req.SpaceID,
    UserID:   req.UserID,
    Status:   req.Status,
    Page:     page,
    PageSize: pageSize,
  })
}

func (s *threadService) requireComponents() error {
  if err := s.requireRepo(); err != nil {
    return err
  }
  if s.idGen == nil {
    return fmt.Errorf("agent thread id generator is required")
  }
  return nil
}

func (s *threadService) requireRepo() error {
  if s == nil || s.repo == nil {
    return fmt.Errorf("agent thread repository is required")
  }
  return nil
}
```

- [ ] **Step 5: Run domain tests until green**

Run:

```bash
cd backend
go test ./domain/agentthread/service
```

Expected: PASS.

- [ ] **Step 6: Commit domain slice**

```bash
git add backend/domain/agentthread
git commit -m "feat: add agent thread domain service"
```

## Task 3: Go Application Adapter for Legacy Task Threads

**Files:**
- Create: `backend/application/agentthread/dto.go`
- Create: `backend/application/agentthread/thread_app.go`
- Create: `backend/application/agentthread/thread_app_test.go`

- [ ] **Step 1: Write failing adapter tests**

Create `backend/application/agentthread/thread_app_test.go`:

```go
func TestTaskToThreadSummaryKeepsTaskWordingAndThreadID(t *testing.T) {
  task := &taskapi.ChatTask{
    ID:        100,
    SpaceID:   1,
    CreatorID: 2,
    Title:     "生成周报",
    Status:    taskapi.TaskStatus_Running,
    Progress:  40,
    CreatedAt: 1717000000000,
    UpdatedAt: 1717000300000,
  }

  summary := TaskToThreadSummary(task)

  require.Equal(t, int64(100), summary.ThreadID)
  require.Equal(t, int64(100), summary.LegacyTaskID)
  require.Equal(t, "生成周报", summary.Title)
  require.Equal(t, ThreadStatusRunning, summary.Status)
  require.Equal(t, ThreadSourceWeb, summary.Source)
}

func TestTaskToThreadSummaryMapsTerminalStatuses(t *testing.T) {
  summary := TaskToThreadSummary(&taskapi.ChatTask{
    ID:     101,
    Title:  "完成任务",
    Status: taskapi.TaskStatus_Succeeded,
  })

  require.Equal(t, ThreadStatusCompleted, summary.Status)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test ./application/agentthread
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Add DTO and mapper implementation**

Create `backend/application/agentthread/dto.go`:

```go
package agentthread

type ThreadStatus string

const (
  ThreadStatusIdle      ThreadStatus = "idle"
  ThreadStatusRunning   ThreadStatus = "running"
  ThreadStatusFailed    ThreadStatus = "failed"
  ThreadStatusCompleted ThreadStatus = "completed"
  ThreadStatusCanceled  ThreadStatus = "canceled"
)

type ThreadSource string

const (
  ThreadSourceWeb ThreadSource = "web"
  ThreadSourceIM  ThreadSource = "im"
  ThreadSourceAPI ThreadSource = "api"
)

type ThreadSummary struct {
  ThreadID          int64
  LegacyTaskID      int64
  SpaceID           int64
  CreatorID         int64
  Title             string
  Status            ThreadStatus
  Source            ThreadSource
  Progress          int32
  LastUserMessage   string
  LastAgentMessage  string
  CreatedAt         int64
  UpdatedAt         int64
}
```

Create `backend/application/agentthread/thread_app.go`:

```go
package agentthread

import (
  "encoding/json"
  "strings"

  taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
)

func TaskToThreadSummary(task *taskapi.ChatTask) *ThreadSummary {
  if task == nil {
    return nil
  }

  return &ThreadSummary{
    ThreadID:         task.ID,
    LegacyTaskID:     task.ID,
    SpaceID:          task.SpaceID,
    CreatorID:        task.CreatorID,
    Title:            task.Title,
    Status:           taskStatusToThreadStatus(task.Status),
    Source:           ThreadSourceWeb,
    Progress:         task.Progress,
    LastUserMessage:  extractTaskMessage(task.GetInput()),
    LastAgentMessage: extractTaskMessage(task.GetResult()),
    CreatedAt:        task.CreatedAt,
    UpdatedAt:        task.UpdatedAt,
  }
}

func taskStatusToThreadStatus(status taskapi.TaskStatus) ThreadStatus {
  switch status {
  case taskapi.TaskStatus_Running, taskapi.TaskStatus_Queued, taskapi.TaskStatus_Canceling:
    return ThreadStatusRunning
  case taskapi.TaskStatus_Succeeded:
    return ThreadStatusCompleted
  case taskapi.TaskStatus_Failed:
    return ThreadStatusFailed
  case taskapi.TaskStatus_Canceled:
    return ThreadStatusCanceled
  default:
    return ThreadStatusIdle
  }
}

func extractTaskMessage(raw string) string {
  text := strings.TrimSpace(raw)
  if text == "" {
    return ""
  }

  var payload map[string]any
  if err := json.Unmarshal([]byte(text), &payload); err != nil {
    return text
  }

  for _, key := range []string{"message", "answer", "title"} {
    if value, ok := payload[key].(string); ok {
      value = strings.TrimSpace(value)
      if value != "" {
        return value
      }
    }
  }

  return text
}
```

- [ ] **Step 4: Run adapter tests until green**

Run:

```bash
cd backend
go test ./application/agentthread
```

Expected: PASS.

- [ ] **Step 5: Commit adapter slice**

```bash
git add backend/application/agentthread
git commit -m "feat: add legacy task thread adapter"
```

## Task 4: Focused Verification

**Files:**
- No new files.

- [ ] **Step 1: Run frontend focused tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run backend focused tests**

```bash
cd backend
go test ./domain/agentthread/service ./application/agentthread ./application/workbench ./application/task ./domain/task/service
```

Expected: PASS.

- [ ] **Step 3: Run formatting checks**

```bash
cd backend
gofmt -w domain/agentthread application/agentthread
```

```bash
git diff --check
```

Expected: exit 0.

- [ ] **Step 4: Commit any verification-only formatting changes**

If gofmt changed files:

```bash
git add backend/domain/agentthread backend/application/agentthread
git commit -m "chore: format agent thread foundation"
```

## Phase 1 Acceptance Checklist

- [ ] User-facing labels remain `新建任务`、`全部任务`、`我的任务`、`任务详情`.
- [ ] Primary menu route for `新建任务` is canonical `chats/new`.
- [ ] `全部任务` route is canonical `chats`.
- [ ] Workbench submit navigates to `/space/:space_id/chats/:thread_id`.
- [ ] Sidebar `我的任务` navigates to `/space/:space_id/chats/:thread_id`.
- [ ] Legacy `/tasks/:task_id` remains readable.
- [ ] Backend has a Go-native `agentthread` domain package.
- [ ] Backend adapter can map legacy `ChatTask` to thread summary.
- [ ] Focused frontend and backend tests pass.

## Self-Review

- Spec coverage: Covers the workbench menu/task detail Phase 1 slice, including canonical routes, task wording, frontend compatibility, and Go-native thread domain foundation.
- Scope check: Does not attempt Skills/MCP, Memory, IM Channels, full LangGraph API, or security scanner in this first implementation plan; those are independent plans.
- Empty-slot scan: No task contains incomplete sections or undefined commands.
- Type consistency: Frontend helper uses `threadId` in TypeScript and backend DTO uses `ThreadID`; both intentionally map to legacy task IDs during Phase 1 compatibility.
