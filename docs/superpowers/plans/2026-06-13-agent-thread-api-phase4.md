# Agent Thread API Phase 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose task-thread list and detail read APIs backed by `agent_threads`.

**Architecture:** Keep the visible product language as tasks while using `agentthread` as the backend technical model. Add application-level get/list response mapping with API-safe JSON DTOs, then add Hertz handlers and workbench routes under `/api/workbench/task_threads`. This phase deliberately avoids generated Thrift refresh so the endpoint can be used by the frontend before full IDL generation is wired.

**Tech Stack:** Go, Hertz handlers, existing `agentthread` application service, existing workbench route group, TDD with Go tests.

---

## Scope

This phase implements:

1. `GET /api/workbench/task_threads?space_id=...&page=...&page_size=...`.
2. `GET /api/workbench/task_threads/:thread_id`.
3. JSON response DTOs with `code`, `msg`, and `data`.
4. Application service `GetThread` method.

This phase does not implement:

1. Frontend list/detail source switching.
2. Generated Thrift model/router regeneration.
3. Run/message/event persistence tables.
4. SSE or LangGraph-compatible streaming APIs.

## File Structure

- Modify `backend/application/agentthread/dto.go`
  - Add `GetThreadRequest`, `GetThreadResponse`.
- Modify `backend/application/agentthread/service.go`
  - Add `GetThread`.
- Modify `backend/application/agentthread/service_test.go`
  - Verify get behavior and nil response handling.
- Create `backend/api/model/workbench/thread/thread.go`
  - Manual JSON DTOs for this transitional API slice.
- Create `backend/api/handler/coze/workbench_thread_service.go`
  - Bind query/path params, call `agentthread.SVC`, map response DTOs.
- Create `backend/api/handler/coze/workbench_thread_service_test.go`
  - Handler tests with an initialized in-memory `agentthread.SVC`.
- Modify `backend/api/router/coze/api.go`
  - Add `GET /api/workbench/task_threads` and `GET /api/workbench/task_threads/:thread_id`.

## Task 1: Application Get Thread

- [ ] Write failing tests in `backend/application/agentthread/service_test.go`:

```go
func TestApplicationGetThreadMapsDomainThread(t *testing.T) {
  domainSVC := &recordingThreadService{
    got: &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2, Title: "任务详情", Status: entity.ThreadStatusCompleted, Source: entity.ThreadSourceWeb},
  }
  app := &ApplicationService{ThreadSVC: domainSVC}

  resp, err := app.GetThread(context.Background(), &GetThreadRequest{ThreadID: 10})

  require.NoError(t, err)
  require.Equal(t, int64(10), domainSVC.getID)
  require.Equal(t, int64(10), resp.Thread.ThreadID)
  require.Equal(t, ThreadStatusCompleted, resp.Thread.Status)
}
```

- [ ] Run `cd backend && go test ./application/agentthread`.
  - Expected: FAIL because `GetThreadRequest`, `GetThreadResponse`, and `ApplicationService.GetThread` do not exist.
- [ ] Implement DTOs and method.
- [ ] Run `cd backend && go test ./application/agentthread`.
  - Expected: PASS.
- [ ] Commit with `feat: add agent thread get application method`.

## Task 2: Workbench Thread API DTOs and Handler

- [ ] Create failing handler tests in `backend/api/handler/coze/workbench_thread_service_test.go`:

```go
func TestListTaskThreadsHandlerReturnsAgentThreads(t *testing.T) {
  h := server.Default()
  h.GET("/api/workbench/task_threads", ListTaskThreads)
  installAgentThreadTestService(t)

  w := ut.PerformRequest(h.Engine, "GET", "/api/workbench/task_threads?space_id=1&page=1&page_size=10", nil)

  require.Equal(t, http.StatusOK, w.Code)
  require.Contains(t, string(w.Result().Body()), `"code":0`)
  require.Contains(t, string(w.Result().Body()), `"thread_id":"1"`)
}
```

- [ ] Run `cd backend && go test ./api/handler/coze -run 'Test(ListTaskThreads|GetTaskThread)'`.
  - Expected: FAIL because the handlers and DTO package do not exist.
- [ ] Create `backend/api/model/workbench/thread/thread.go` with request/response structs and JSON tags.
- [ ] Create `backend/api/handler/coze/workbench_thread_service.go`.
- [ ] Run `cd backend && go test ./api/handler/coze -run 'Test(ListTaskThreads|GetTaskThread)'`.
  - Expected: PASS.
- [ ] Commit with `feat: add workbench task thread handlers`.

## Task 3: Router Registration

- [ ] Add failing route test that registers `router/coze.Register` and calls `/api/workbench/task_threads`.
- [ ] Run the route test and confirm 404 or compile failure before route registration.
- [ ] Add the two manual routes in `backend/api/router/coze/api.go` inside the workbench group.
- [ ] Run handler route tests.
- [ ] Commit with `feat: register workbench task thread routes`.

## Task 4: Verification

- [ ] Run:

```bash
cd backend
go test ./application/agentthread ./api/handler/coze ./api/router/coze ./application/workbench ./domain/agentthread/repository ./domain/agentthread/service
```

- [ ] Run:

```bash
git diff --check
git status --short
```

## Acceptance Checklist

- [ ] Application service can get a single thread by ID.
- [ ] List API returns task-thread summaries from `agent_threads`.
- [ ] Detail API returns one task-thread summary.
- [ ] Responses use `code/msg/data` and snake_case JSON fields.
- [ ] Routes are registered under `/api/workbench/task_threads`.
- [ ] Existing workbench tests still pass.
