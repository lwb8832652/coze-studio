# Agent Thread Persistence Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add persistent `agent_threads` storage and an application service boundary for task-thread listing and creation.

**Architecture:** Keep Phase 2 narrow: implement the GORM repository for `agent_threads`, then add an application service that can create/list domain threads and map them to task-oriented DTOs. Legacy `ChatTask` compatibility remains available through the existing mapper, but this phase does not expose new HTTP routes or modify generated IDL.

**Tech Stack:** Go, GORM, sqlite-backed repository tests, existing `idgen.IDGenerator`, existing `workbench/task` generated models for legacy compatibility.

---

## Scope

This phase implements:

1. `agent_threads` repository persistence.
2. Domain-to-application `ThreadSummary` mapping.
3. Application service initialization from `*gorm.DB` and `idgen.IDGenerator`.
4. Application create/list tests.

This phase does not implement:

1. `/api/agent/threads` HTTP handlers.
2. thrift/idl2ts generation.
3. run/message/event persistence.
4. SSE streaming or LangGraph-compatible API.
5. database migration file updates.

Those pieces follow after the application service boundary is stable.

## File Structure

- Create `backend/domain/agentthread/repository/mysql.go`
  - Owns GORM PO, JSON validation, create/get/list behavior.
- Create `backend/domain/agentthread/repository/mysql_test.go`
  - Verifies create/get/list/paging/filtering and JSON validation with sqlite.
- Modify `backend/application/agentthread/dto.go`
  - Adds request/response DTOs for create/list.
- Modify `backend/application/agentthread/thread_app.go`
  - Adds domain thread mapping.
- Create `backend/application/agentthread/init.go`
  - Builds repository and domain service.
- Create `backend/application/agentthread/service.go`
  - Implements `CreateThread` and `ListThreads`.
- Create `backend/application/agentthread/service_test.go`
  - Verifies app service behavior with the domain service.

## Task 1: Agent Thread GORM Repository

**Files:**
- Create: `backend/domain/agentthread/repository/mysql.go`
- Create: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] **Step 1: Write failing repository tests**

Create `backend/domain/agentthread/repository/mysql_test.go`:

```go
func TestThreadRepositoryCreateAndGet(t *testing.T) {
  db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
  require.NoError(t, err)
  require.NoError(t, db.AutoMigrate(&threadPO{}))
  repo := NewThreadRepository(db)

  thread := &entity.Thread{
    ID: 1, SpaceID: 10, CreatorID: 20, Title: "生成周报",
    Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
    Metadata: `{"mode":"auto"}`, CreatedAt: 1, UpdatedAt: 2, LastMessageAt: 3,
  }

  require.NoError(t, repo.CreateThread(context.Background(), thread))
  got, err := repo.GetThread(context.Background(), 1)

  require.NoError(t, err)
  require.Equal(t, "生成周报", got.Title)
  require.Equal(t, entity.ThreadStatusIdle, got.Status)
  require.Equal(t, entity.ThreadSourceWeb, got.Source)
  require.Equal(t, `{"mode":"auto"}`, got.Metadata)
}
```

Also test list behavior:

```go
func TestThreadRepositoryListFiltersAndOrders(t *testing.T) {
  db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
  require.NoError(t, err)
  require.NoError(t, db.AutoMigrate(&threadPO{}))
  repo := NewThreadRepository(db)
  status := entity.ThreadStatusRunning
  require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{ID: 1, SpaceID: 10, CreatorID: 20, Title: "old", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 1}))
  require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{ID: 2, SpaceID: 10, CreatorID: 20, Title: "new", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 2}))
  require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{ID: 3, SpaceID: 11, CreatorID: 20, Title: "other space", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 3}))

  got, total, err := repo.ListThreads(context.Background(), ListThreadsRequest{SpaceID: 10, Status: &status, Page: 1, PageSize: 10})

  require.NoError(t, err)
  require.Equal(t, int64(2), total)
  require.Len(t, got, 2)
  require.Equal(t, int64(2), got[0].ID)
  require.Equal(t, int64(1), got[1].ID)
}
```

Also test invalid JSON:

```go
func TestThreadRepositoryRejectsInvalidMetadataJSON(t *testing.T) {
  db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
  require.NoError(t, err)
  require.NoError(t, db.AutoMigrate(&threadPO{}))
  repo := NewThreadRepository(db)

  err = repo.CreateThread(context.Background(), &entity.Thread{
    ID: 1, SpaceID: 10, CreatorID: 20, Title: "bad", Status: entity.ThreadStatusIdle,
    Source: entity.ThreadSourceWeb, Metadata: "{",
  })

  require.Error(t, err)
  require.Contains(t, err.Error(), "metadata")
}
```

- [ ] **Step 2: Run test to verify RED**

```bash
cd backend
go test ./domain/agentthread/repository
```

Expected: FAIL because `threadPO` and `NewThreadRepository` are not defined.

- [ ] **Step 3: Implement repository**

Create `backend/domain/agentthread/repository/mysql.go`:

```go
package repository

import (
  "context"
  "encoding/json"
  "fmt"
  "time"

  "gorm.io/datatypes"
  "gorm.io/gorm"

  "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type threadRepository struct {
  db *gorm.DB
}

func NewThreadRepository(db *gorm.DB) ThreadRepository {
  return &threadRepository{db: db}
}

type threadPO struct {
  ID            int64          `gorm:"column:id;primaryKey"`
  SpaceID       int64          `gorm:"column:space_id;index:idx_agent_threads_space_updated"`
  CreatorID     int64          `gorm:"column:creator_id;index:idx_agent_threads_creator_updated"`
  AgentID       int64          `gorm:"column:agent_id"`
  Title         string         `gorm:"column:title"`
  Status        string         `gorm:"column:status;index:idx_agent_threads_space_status"`
  Source        string         `gorm:"column:source"`
  LegacyTaskID  int64          `gorm:"column:legacy_task_id;index:idx_agent_threads_legacy_task"`
  Metadata      datatypes.JSON `gorm:"column:metadata;type:json"`
  CreatedAt     int64          `gorm:"column:created_at"`
  UpdatedAt     int64          `gorm:"column:updated_at;index:idx_agent_threads_space_updated;index:idx_agent_threads_creator_updated"`
  LastMessageAt int64          `gorm:"column:last_message_at"`
}

func (threadPO) TableName() string {
  return "agent_threads"
}

func (r *threadRepository) CreateThread(ctx context.Context, thread *entity.Thread) error {
  if thread == nil {
    return fmt.Errorf("thread is required")
  }
  now := time.Now().UnixMilli()
  if thread.CreatedAt == 0 {
    thread.CreatedAt = now
  }
  if thread.UpdatedAt == 0 {
    thread.UpdatedAt = thread.CreatedAt
  }
  if thread.LastMessageAt == 0 {
    thread.LastMessageAt = thread.UpdatedAt
  }
  po, err := threadToPO(thread)
  if err != nil {
    return err
  }
  return r.db.WithContext(ctx).Create(po).Error
}

func (r *threadRepository) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
  var po threadPO
  if err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error; err != nil {
    return nil, err
  }
  return po.toEntity(), nil
}

func (r *threadRepository) ListThreads(ctx context.Context, req ListThreadsRequest) ([]*entity.Thread, int64, error) {
  if req.Page <= 0 {
    req.Page = 1
  }
  if req.PageSize <= 0 {
    req.PageSize = 20
  }
  query := r.db.WithContext(ctx).Model(&threadPO{}).Where("space_id = ?", req.SpaceID)
  if req.UserID > 0 {
    query = query.Where("creator_id = ?", req.UserID)
  }
  if req.Status != nil {
    query = query.Where("status = ?", string(*req.Status))
  }
  var total int64
  if err := query.Count(&total).Error; err != nil {
    return nil, 0, err
  }
  pos := make([]*threadPO, 0)
  if err := query.Order("updated_at DESC, id DESC").Limit(int(req.PageSize)).Offset(int((req.Page - 1) * req.PageSize)).Find(&pos).Error; err != nil {
    return nil, 0, err
  }
  threads := make([]*entity.Thread, 0, len(pos))
  for _, po := range pos {
    threads = append(threads, po.toEntity())
  }
  return threads, total, nil
}

func threadToPO(thread *entity.Thread) (*threadPO, error) {
  metadata, err := optionalJSON("metadata", thread.Metadata)
  if err != nil {
    return nil, err
  }
  return &threadPO{
    ID: thread.ID, SpaceID: thread.SpaceID, CreatorID: thread.CreatorID, AgentID: thread.AgentID,
    Title: thread.Title, Status: string(thread.Status), Source: string(thread.Source), LegacyTaskID: thread.LegacyTaskID,
    Metadata: metadata, CreatedAt: thread.CreatedAt, UpdatedAt: thread.UpdatedAt, LastMessageAt: thread.LastMessageAt,
  }, nil
}

func (po *threadPO) toEntity() *entity.Thread {
  if po == nil {
    return nil
  }
  return &entity.Thread{
    ID: po.ID, SpaceID: po.SpaceID, CreatorID: po.CreatorID, AgentID: po.AgentID,
    Title: po.Title, Status: entity.ThreadStatus(po.Status), Source: entity.ThreadSource(po.Source), LegacyTaskID: po.LegacyTaskID,
    Metadata: string(po.Metadata), CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt, LastMessageAt: po.LastMessageAt,
  }
}

func optionalJSON(field, raw string) (datatypes.JSON, error) {
  if raw == "" {
    return nil, nil
  }
  if !json.Valid([]byte(raw)) {
    return nil, fmt.Errorf("%s must be valid json", field)
  }
  return datatypes.JSON([]byte(raw)), nil
}
```

- [ ] **Step 4: Run repository tests to verify GREEN**

```bash
cd backend
gofmt -w domain/agentthread/repository
go test ./domain/agentthread/repository
```

Expected: PASS.

- [ ] **Step 5: Commit repository slice**

```bash
git add backend/domain/agentthread/repository
git commit -m "feat: add agent thread repository"
```

## Task 2: Agent Thread Application Service

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Create: `backend/application/agentthread/init.go`
- Create: `backend/application/agentthread/service.go`
- Create: `backend/application/agentthread/service_test.go`

- [ ] **Step 1: Write failing application tests**

Create `backend/application/agentthread/service_test.go`:

```go
func TestApplicationCreateThreadReturnsTaskSummary(t *testing.T) {
  domainSVC := &recordingThreadService{
    created: &entity.Thread{
      ID: 10, SpaceID: 1, CreatorID: 2, Title: "生成周报",
      Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
      CreatedAt: 100, UpdatedAt: 100,
    },
  }
  app := &ApplicationService{ThreadSVC: domainSVC}

  resp, err := app.CreateThread(context.Background(), &CreateThreadRequest{
    SpaceID: 1, UserID: 2, Title: "生成周报",
  })

  require.NoError(t, err)
  require.Equal(t, int64(10), resp.Thread.ThreadID)
  require.Equal(t, "生成周报", domainSVC.createReq.Title)
  require.Equal(t, ThreadStatusIdle, resp.Thread.Status)
}

func TestApplicationListThreadsMapsDomainThreads(t *testing.T) {
  domainSVC := &recordingThreadService{
    listed: []*entity.Thread{
      {ID: 10, SpaceID: 1, CreatorID: 2, Title: "新任务", Status: entity.ThreadStatusRunning, Source: entity.ThreadSourceWeb, UpdatedAt: 200},
    },
    total: 1,
  }
  app := &ApplicationService{ThreadSVC: domainSVC}

  resp, err := app.ListThreads(context.Background(), &ListThreadsRequest{SpaceID: 1, UserID: 2})

  require.NoError(t, err)
  require.Equal(t, int64(1), resp.Total)
  require.Len(t, resp.Threads, 1)
  require.Equal(t, int64(10), resp.Threads[0].ThreadID)
  require.Equal(t, ThreadStatusRunning, resp.Threads[0].Status)
  require.Equal(t, int64(2), domainSVC.listReq.UserID)
}
```

- [ ] **Step 2: Run test to verify RED**

```bash
cd backend
go test ./application/agentthread
```

Expected: FAIL because `ApplicationService`, request DTOs, and domain mapper are not implemented.

- [ ] **Step 3: Add DTOs and mapper**

Append to `backend/application/agentthread/dto.go`:

```go
type CreateThreadRequest struct {
  SpaceID      int64
  UserID       int64
  AgentID      int64
  Title        string
  Source       ThreadSource
  LegacyTaskID int64
  Metadata     string
}

type CreateThreadResponse struct {
  Thread *ThreadSummary
}

type ListThreadsRequest struct {
  SpaceID  int64
  UserID   int64
  Status   *ThreadStatus
  Page     int32
  PageSize int32
}

type ListThreadsResponse struct {
  Threads []*ThreadSummary
  Total   int64
}
```

Append to `backend/application/agentthread/thread_app.go`:

```go
func DomainThreadToSummary(thread *entity.Thread) *ThreadSummary {
  if thread == nil {
    return nil
  }
  return &ThreadSummary{
    ThreadID: thread.ID, LegacyTaskID: thread.LegacyTaskID, SpaceID: thread.SpaceID, CreatorID: thread.CreatorID,
    Title: thread.Title, Status: ThreadStatus(thread.Status), Source: ThreadSource(thread.Source),
    CreatedAt: thread.CreatedAt, UpdatedAt: thread.UpdatedAt,
  }
}
```

- [ ] **Step 4: Add application service and initializer**

Create `backend/application/agentthread/service.go`:

```go
package agentthread

import (
  "context"
  "fmt"

  domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
  domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

type ApplicationService struct {
  ThreadSVC domainservice.ThreadService
}

func (s *ApplicationService) CreateThread(ctx context.Context, req *CreateThreadRequest) (*CreateThreadResponse, error) {
  if s == nil || s.ThreadSVC == nil {
    return nil, fmt.Errorf("agent thread service is not initialized")
  }
  thread, err := s.ThreadSVC.CreateThread(ctx, &domainservice.CreateThreadRequest{
    SpaceID: req.SpaceID, UserID: req.UserID, AgentID: req.AgentID, Title: req.Title,
    Source: domainentity.ThreadSource(req.Source), LegacyTaskID: req.LegacyTaskID, Metadata: req.Metadata,
  })
  if err != nil {
    return nil, err
  }
  return &CreateThreadResponse{Thread: DomainThreadToSummary(thread)}, nil
}

func (s *ApplicationService) ListThreads(ctx context.Context, req *ListThreadsRequest) (*ListThreadsResponse, error) {
  if s == nil || s.ThreadSVC == nil {
    return nil, fmt.Errorf("agent thread service is not initialized")
  }
  var status *domainentity.ThreadStatus
  if req != nil && req.Status != nil {
    mapped := domainentity.ThreadStatus(*req.Status)
    status = &mapped
  }
  listReq := &domainservice.ListThreadsRequest{}
  if req != nil {
    listReq.SpaceID = req.SpaceID
    listReq.UserID = req.UserID
    listReq.Status = status
    listReq.Page = req.Page
    listReq.PageSize = req.PageSize
  }
  threads, total, err := s.ThreadSVC.ListThreads(ctx, listReq)
  if err != nil {
    return nil, err
  }
  resp := &ListThreadsResponse{Threads: make([]*ThreadSummary, 0, len(threads)), Total: total}
  for _, thread := range threads {
    resp.Threads = append(resp.Threads, DomainThreadToSummary(thread))
  }
  return resp, nil
}
```

Create `backend/application/agentthread/init.go`:

```go
package agentthread

import (
  "gorm.io/gorm"

  "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
  domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
  "github.com/coze-dev/coze-studio/backend/infra/idgen"
)

var SVC = new(ApplicationService)

type ServiceComponents struct {
  DB    *gorm.DB
  IDGen idgen.IDGenerator
}

func InitService(c *ServiceComponents) *ApplicationService {
  if c == nil {
    return SVC
  }
  repo := repository.NewThreadRepository(c.DB)
  SVC.ThreadSVC = domainservice.NewService(&domainservice.Components{Repo: repo, IDGen: c.IDGen})
  return SVC
}
```

- [ ] **Step 5: Run application tests to verify GREEN**

```bash
cd backend
gofmt -w application/agentthread
go test ./application/agentthread
```

Expected: PASS.

- [ ] **Step 6: Commit application slice**

```bash
git add backend/application/agentthread
git commit -m "feat: add agent thread application service"
```

## Task 3: Focused Verification

**Files:**
- No new files.

- [ ] **Step 1: Run focused backend tests**

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread
```

Expected: PASS.

- [ ] **Step 2: Run broader backend affected tests**

```bash
cd backend
go test ./application/workbench ./application/task ./domain/task/service
```

Expected: PASS.

- [ ] **Step 3: Run diff check**

```bash
git diff --check
```

Expected: exit 0.

## Acceptance Checklist

- [ ] `agent_threads` GORM repository creates and reads threads.
- [ ] Repository list filters by space, user, and status.
- [ ] Repository list orders by `updated_at DESC, id DESC`.
- [ ] Repository rejects invalid metadata JSON.
- [ ] Application service creates threads through the domain service.
- [ ] Application service lists domain threads as `ThreadSummary`.
- [ ] Legacy `ChatTask` mapper remains covered.
- [ ] Focused backend tests pass.

## Self-Review

- Spec coverage: This phase implements persistence and application boundaries needed before HTTP/IDL work.
- Scope check: This phase deliberately avoids API handlers and schema migration files so generated-code and deployment changes can be reviewed separately.
- Empty-slot scan: No step contains undefined commands or incomplete sections.
- Type consistency: DTO fields match existing domain entity fields and Phase 1 `ThreadSummary` naming.
