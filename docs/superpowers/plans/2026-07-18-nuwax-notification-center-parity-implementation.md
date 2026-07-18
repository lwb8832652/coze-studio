# Nuwax Notification Center Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the static Coze notification bells with a persistent,
tenant-safe notification center that matches the Nuwax popover workflow and
supports explicit single-item and bulk read actions.

**Architecture:** Add a MySQL-backed `notification` domain with separate
message and recipient records, expose it through the existing Workbench Task
IDL, and inject a narrow publisher interface into AgentThread, ScheduledTask,
and Workspace application services. A shared React `NotificationBell`
component uses the generated client, cursor pagination, optimistic read state,
focus refresh, and bounded polling across all existing top bars.

**Tech Stack:** Go, Hertz, GORM, MySQL/SQLite tests, Atlas, Thrift/hz,
TypeScript, React, Vitest, Coze Design.

---

## File map

Create these focused files:

- `docker/atlas/migrations/20260718000100_notifications.sql`: notification
  tables and indexes.
- `backend/domain/notification/entity/notification.go`: domain enums and
  entities.
- `backend/domain/notification/entity/notification_test.go`: entity
  validation tests.
- `backend/domain/notification/repository/repository.go`: persistence
  contract and list filter.
- `backend/domain/notification/repository/mysql.go`: transactional MySQL
  repository.
- `backend/domain/notification/repository/mysql_test.go`: idempotency,
  isolation, pagination, and read-state tests.
- `backend/application/notification/service.go`: publish, list, count, and
  read use cases.
- `backend/application/notification/service_test.go`: application boundary
  tests.
- `backend/application/notification/init.go`: repository and service wiring.
- `backend/api/handler/coze/notification_service.go`: HTTP request/response
  mapping.
- `backend/api/handler/coze/notification_service_test.go`: handler tests.
- `backend/api/router/coze/notification_route_test.go`: generated route
  regression test.
- `frontend/apps/coze-studio/src/components/notification-center/service.ts`:
  generated-client aliases.
- `frontend/apps/coze-studio/src/components/notification-center/use-notifications.ts`:
  request, polling, pagination, and optimistic-state hook.
- `frontend/apps/coze-studio/src/components/notification-center/notification-bell.tsx`:
  shared trigger and popover UI.
- `frontend/apps/coze-studio/src/components/notification-center/index.less`:
  Nuwax-aligned responsive styles.
- `frontend/apps/coze-studio/src/components/notification-center/__tests__/notification-bell.test.tsx`:
  component and interaction tests.

Modify these source files:

- `idl/workbench/task.thrift`: notification enums, structs, and four APIs.
- `frontend/packages/arch/api-schema/__tests__/workbench-task-contract.test.ts`:
  source and generated-client contract tests.
- `backend/application/application.go`: initialize and inject notification
  service.
- `backend/application/agentthread/runner.go`: publish top-level run terminal
  events.
- `backend/application/agentthread/worker.go`: pass the terminal publisher to
  the run processor.
- `backend/application/agentthread/runner_test.go`: terminal notification
  tests.
- `backend/application/scheduledtask/init.go`: accept notification publisher.
- `backend/application/scheduledtask/dispatcher.go`: publish one result per
  scheduled execution.
- `backend/application/scheduledtask/executor_test.go`: success/failure
  notification tests.
- `backend/application/workspace/workspace.go`: publish member-change
  notifications.
- `backend/application/workspace/workspace_test.go`: member notification
  tests.
- `frontend/apps/coze-studio/src/pages/workbench/index.tsx`: replace the
  static bell.
- `frontend/apps/coze-studio/src/pages/tasks/task-top-bar.tsx`: replace the
  static bell.
- `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`:
  replace the static bell.
- `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`:
  shared bell integration assertion.
- `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`:
  shared bell integration assertion.

Generated files must come from generators, not hand edits:

- `backend/api/model/workbench/task/task.go`
- `backend/api/handler/coze/workbench_task_service.go`
- `backend/api/router/coze/api.go`
- `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- `docker/atlas/migrations/atlas.sum`

## Task 1: Lock the IDL and migration contract

**Files:**

- Modify: `idl/workbench/task.thrift`
- Modify:
  `frontend/packages/arch/api-schema/__tests__/workbench-task-contract.test.ts`
- Create: `docker/atlas/migrations/20260718000100_notifications.sql`
- Generate: `backend/api/model/workbench/task/task.go`
- Generate: `backend/api/handler/coze/workbench_task_service.go`
- Generate: `backend/api/router/coze/api.go`
- Generate: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Generate: `docker/atlas/migrations/atlas.sum`

- [ ] **Step 1: Add a failing API contract test**

Append this test to the existing contract suite:

```ts
it('declares and maps the notification center contract', () => {
  const source = readFileSync(taskThriftPath, 'utf8');
  const methods = [
    'ListNotifications',
    'GetNotificationUnreadCount',
    'MarkNotificationsRead',
    'MarkAllNotificationsRead',
  ];

  for (const method of methods) {
    expect(source).toContain(`${method}(`);
  }

  expect(workbenchTask.ListNotifications.meta).toMatchObject({
    method: 'GET',
    url: '/api/workbench/notifications',
    reqMapping: {
      query: expect.arrayContaining(['cursor', 'limit', 'unread_only']),
    },
  });
  expect(workbenchTask.MarkNotificationsRead.meta).toMatchObject({
    method: 'POST',
    url: '/api/workbench/notifications/read',
  });
  expect(workbenchTask.MarkAllNotificationsRead.meta).toMatchObject({
    method: 'POST',
    url: '/api/workbench/notifications/read_all',
  });
});
```

- [ ] **Step 2: Run the contract test and confirm RED**

Run:

```bash
cd frontend/packages/arch/api-schema
rushx test -- __tests__/workbench-task-contract.test.ts
```

Expected: FAIL because the four generated notification clients do not exist.

- [ ] **Step 3: Add the Thrift contract**

Add these definitions before `service WorkbenchTaskService`:

```thrift
enum NotificationCategory {
    Task = 1,
    ScheduledTask = 2,
    Workspace = 3,
    System = 4,
}

enum NotificationTargetType {
    None = 1,
    TaskThread = 2,
    ScheduledTaskCenter = 3,
    Workspace = 4,
}

struct WorkbenchNotification {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required NotificationCategory category
    3: required string event_type
    4: required string title
    5: required string content
    6: required NotificationTargetType target_type
    7: optional string target_id
    8: optional i64 space_id (agw.js_conv="str", api.js_conv="true")
    9: optional string sender_name
    10: optional string sender_avatar
    11: required bool read
    12: required i64 created_at
}

struct ListNotificationsRequest {
    1: optional i64 cursor (agw.js_conv="str", api.js_conv="true")
    2: optional i32 limit
    3: optional bool unread_only
    255: optional base.Base Base (api.none="true")
}

struct ListNotificationsData {
    1: required list<WorkbenchNotification> notifications
    2: optional i64 next_cursor (agw.js_conv="str", api.js_conv="true")
    3: required bool has_more
}

struct ListNotificationsResponse {
    1: optional ListNotificationsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct NotificationUnreadCountData {
    1: required i64 count
}

struct NotificationUnreadCountResponse {
    1: optional NotificationUnreadCountData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetNotificationUnreadCountRequest {
    255: optional base.Base Base (api.none="true")
}

struct MarkNotificationsReadRequest {
    1: required list<string> notification_ids
    255: optional base.Base Base (api.none="true")
}

struct MarkAllNotificationsReadRequest {
    255: optional base.Base Base (api.none="true")
}

struct NotificationMutationData {
    1: required i64 updated_count
}

struct NotificationMutationResponse {
    1: optional NotificationMutationData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}
```

Add these methods to `WorkbenchTaskService`:

```thrift
ListNotificationsResponse ListNotifications(1: ListNotificationsRequest request)(
    api.get="/api/workbench/notifications", api.category="workbench"
)
NotificationUnreadCountResponse GetNotificationUnreadCount(
    1: GetNotificationUnreadCountRequest request
)(
    api.get="/api/workbench/notifications/unread_count",
    api.category="workbench"
)
NotificationMutationResponse MarkNotificationsRead(
    1: MarkNotificationsReadRequest request
)(
    api.post="/api/workbench/notifications/read",
    api.category="workbench"
)
NotificationMutationResponse MarkAllNotificationsRead(
    1: MarkAllNotificationsReadRequest request
)(
    api.post="/api/workbench/notifications/read_all",
    api.category="workbench"
)
```

- [ ] **Step 4: Generate backend and frontend contracts**

Run:

```bash
cd backend
hz update -idl ../idl/api.thrift -enable_extends
cd ../frontend/packages/arch/api-schema
rushx update
```

Expected: generated Go models/routes and TypeScript clients contain all four
methods without hand-edited generated code.

- [ ] **Step 5: Add the migration**

Create the migration with this schema:

```sql
CREATE TABLE IF NOT EXISTS `notification_messages` (
  `id` bigint NOT NULL,
  `scope` varchar(32) NOT NULL,
  `space_id` bigint NOT NULL DEFAULT 0,
  `sender_id` bigint NOT NULL DEFAULT 0,
  `category` varchar(32) NOT NULL,
  `event_type` varchar(64) NOT NULL,
  `title` varchar(128) NOT NULL,
  `content` varchar(1024) NOT NULL DEFAULT '',
  `target_type` varchar(32) NOT NULL DEFAULT 'none',
  `target_id` varchar(128) NOT NULL DEFAULT '',
  `dedupe_key` varchar(191) NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_messages_dedupe` (`dedupe_key`),
  KEY `idx_notification_messages_space_created` (`space_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `notification_recipients` (
  `id` bigint NOT NULL,
  `notification_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `read_at` bigint NOT NULL DEFAULT 0,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_recipients_message_user`
    (`notification_id`, `user_id`),
  KEY `idx_notification_recipients_user_read_id`
    (`user_id`, `read_at`, `id`),
  KEY `idx_notification_recipients_notification` (`notification_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

- [ ] **Step 6: Hash and validate without applying the migration**

Run:

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Expected: Atlas reports a valid migration directory. Do not run
`atlas migrate apply` without explicit user authorization.

- [ ] **Step 7: Re-run the contract test and commit**

Run:

```bash
cd frontend/packages/arch/api-schema
rushx test -- __tests__/workbench-task-contract.test.ts
```

Expected: PASS.

Commit:

```bash
git add idl/workbench/task.thrift \
  frontend/packages/arch/api-schema/__tests__/workbench-task-contract.test.ts \
  frontend/packages/arch/api-schema/src/idl/workbench/task.ts \
  backend/api/model/workbench/task/task.go \
  backend/api/handler/coze/workbench_task_service.go \
  backend/api/router/coze/api.go \
  docker/atlas/migrations/20260718000100_notifications.sql \
  docker/atlas/migrations/atlas.sum
git commit -m "feat: add notification center contract"
```

## Task 2: Implement the notification domain and repository

**Files:**

- Create: `backend/domain/notification/entity/notification.go`
- Create: `backend/domain/notification/entity/notification_test.go`
- Create: `backend/domain/notification/repository/repository.go`
- Create: `backend/domain/notification/repository/mysql.go`
- Create: `backend/domain/notification/repository/mysql_test.go`

- [ ] **Step 1: Write failing entity and repository tests**

Cover these exact cases:

```go
func TestNotificationValidateBoundsContentAndRecipients(t *testing.T) {
    item := &Notification{
        Scope: ScopePersonal, Category: CategoryTask,
        EventType: "agent_run.succeeded", Title: "任务已完成",
        Content: strings.Repeat("x", 1025), DedupeKey: "run:1:succeeded",
    }
    require.Error(t, item.Validate([]int64{10}))
    item.Content = "结果已经生成"
    require.NoError(t, item.Validate([]int64{10}))
    require.Error(t, item.Validate(nil))
}

func TestRepositoryPublishIsIdempotentAndIsolated(t *testing.T) {
    repo := newTestRepository(t)
    ctx := context.Background()
    message := notificationFixture("run:1:succeeded")
    require.NoError(t, repo.Publish(ctx, message, []int64{10, 20}))
    require.NoError(t, repo.Publish(ctx, message, []int64{10, 20}))

    user10, _, err := repo.ListForUser(ctx, ListFilter{UserID: 10, Limit: 20})
    require.NoError(t, err)
    require.Len(t, user10, 1)
    user30, _, err := repo.ListForUser(ctx, ListFilter{UserID: 30, Limit: 20})
    require.NoError(t, err)
    require.Empty(t, user30)
}
```

Also test descending cursor pagination, unread-only filtering, recipient-owned
single read, foreign notification IDs, and the read-all cutoff.

- [ ] **Step 2: Run repository tests and confirm RED**

Run:

```bash
cd backend
go test ./domain/notification/... -count=1
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement entities and validation**

Define stable string enums:

```go
type Scope string
type Category string
type TargetType string

const (
    ScopePersonal Scope = "personal"
    ScopeWorkspace Scope = "workspace"
    ScopeSystem Scope = "system"

    CategoryTask Category = "task"
    CategoryScheduledTask Category = "scheduled_task"
    CategoryWorkspace Category = "workspace"
    CategorySystem Category = "system"

    TargetNone TargetType = "none"
    TargetTaskThread TargetType = "task_thread"
    TargetScheduledTaskCenter TargetType = "scheduled_task_center"
    TargetWorkspace TargetType = "workspace"
)

type Notification struct {
    ID int64
    Scope Scope
    SpaceID int64
    SenderID int64
    Category Category
    EventType string
    Title string
    Content string
    TargetType TargetType
    TargetID string
    DedupeKey string
    CreatedAt int64
}

type RecipientNotification struct {
    RecipientID int64
    Notification *Notification
    UserID int64
    ReadAt int64
    SenderName string
    SenderAvatar string
}
```

`Validate` must reject missing event type/title/dedupe key, unknown enums,
workspace scope without a positive space ID, empty recipients, duplicate or
non-positive recipients, title over 128 characters, content over 1024
characters, and target IDs over 128 characters.

- [ ] **Step 4: Implement the repository contract**

Use this interface:

```go
type ListFilter struct {
    UserID int64
    Cursor int64
    Limit int32
    UnreadOnly bool
}

type Repository interface {
    Publish(context.Context, *entity.Notification, []int64) error
    ListForUser(context.Context, ListFilter) (
        []*entity.RecipientNotification, int64, error,
    )
    CountUnread(context.Context, int64) (int64, error)
    MarkRead(context.Context, int64, []int64, int64) (int64, error)
    MarkAllRead(context.Context, int64, int64) (int64, error)
}
```

`ListForUser` returns `limit` items and a `nextCursor` recipient ID; it must
query `notification_recipients` first by authenticated `user_id`, join the
message table, sort by recipient ID descending, and fetch `limit + 1`.

- [ ] **Step 5: Implement transactional MySQL persistence**

`Publish` must:

1. validate and de-duplicate recipient IDs;
2. generate message and recipient IDs using `idgen.IDGenerator`;
3. insert the message and all recipients in one transaction;
4. handle duplicate `dedupe_key` by loading the existing message ID;
5. insert recipients with `ON CONFLICT DO NOTHING`;
6. never create a recipient outside the trusted recipient slice.

`MarkRead` must update only rows joined to the authenticated user. `MarkAllRead`
must first read the current maximum recipient ID for that user and then update
only unread rows at or below that cutoff, so concurrently arriving messages
remain unread.

- [ ] **Step 6: Run domain tests and commit**

Run:

```bash
cd backend
go test ./domain/notification/... -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/domain/notification
git commit -m "feat: persist user notifications"
```

## Task 3: Add application and HTTP use cases

**Files:**

- Create: `backend/application/notification/service.go`
- Create: `backend/application/notification/service_test.go`
- Create: `backend/application/notification/init.go`
- Modify: `backend/application/application.go`
- Create: `backend/api/handler/coze/notification_service.go`
- Create: `backend/api/handler/coze/notification_service_test.go`
- Create: `backend/api/router/coze/notification_route_test.go`

- [ ] **Step 1: Write failing application tests**

Use a fake repository and cover:

```go
func TestServiceListUsesAuthenticatedRecipient(t *testing.T) {
    repo := &fakeRepository{}
    service := &ApplicationService{Repository: repo}
    _, _, err := service.List(context.Background(), ListRequest{
        UserID: 42, Cursor: 100, Limit: 20, UnreadOnly: true,
    })
    require.NoError(t, err)
    require.Equal(t, int64(42), repo.listFilter.UserID)
}

func TestServiceMarkReadRejectsUnboundedBatch(t *testing.T) {
    service := &ApplicationService{Repository: &fakeRepository{}}
    ids := make([]int64, 101)
    _, err := service.MarkRead(context.Background(), 42, ids)
    require.ErrorIs(t, err, ErrInvalidRequest)
}
```

Also test default/max list limits, zero user IDs, content normalization,
recipient de-duplication, and repository errors.

- [ ] **Step 2: Run application tests and confirm RED**

Run:

```bash
cd backend
go test ./application/notification -count=1
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the application service**

Expose this narrow publisher contract to other modules:

```go
type PublishRequest struct {
    Scope entity.Scope
    SpaceID int64
    SenderID int64
    Category entity.Category
    EventType string
    Title string
    Content string
    TargetType entity.TargetType
    TargetID string
    DedupeKey string
    RecipientIDs []int64
}

type Publisher interface {
    Publish(context.Context, PublishRequest) error
}
```

`ApplicationService` owns `Publish`, `PublishSystemAnnouncement`, `List`,
`UnreadCount`, `MarkRead`, and `MarkAllRead`. Normalize whitespace, enforce the
domain limits, cap list limit at 50, cap batch read at 100, and use
`time.Now().UnixMilli()` through an injectable clock. System announcements
accept recipient IDs only from trusted application callers; this task does not
add a public announcement composer because the Nuwax notification UI has none.

Add a bounded `UserProfileReader` dependency with
`MGetUserProfiles(ctx, userIDs)` and enrich only sender name/avatar after the
recipient-filtered list query. Missing/deleted senders use the system fallback
instead of failing the list.

`InitService` must require non-nil DB and ID generator, construct the MySQL
repository, assign package `SVC`, and return an error for incomplete
dependencies.

- [ ] **Step 4: Write failing handler and route tests**

Register the four handlers on a Hertz test server and verify:

- unauthenticated/zero viewer is rejected;
- list query maps cursor, limit, and unread-only;
- notification IDs serialize as strings;
- a batch larger than 100 returns HTTP 400;
- internal repository failures return a generic message without raw errors;
- generated router contains all four expected method/path/handler triples.

The route expectation is:

```go
var notificationRoutes = []routeExpectation{
    {receiver: "_workbench", method: "GET",
        path: "/notifications", handler: "ListNotifications"},
    {receiver: "_workbench", method: "GET",
        path: "/notifications/unread_count",
        handler: "GetNotificationUnreadCount"},
    {receiver: "_workbench", method: "POST",
        path: "/notifications/read", handler: "MarkNotificationsRead"},
    {receiver: "_workbench", method: "POST",
        path: "/notifications/read_all",
        handler: "MarkAllNotificationsRead"},
}
```

- [ ] **Step 5: Implement HTTP mapping**

Use `workbenchViewerIDFromCtx(ctx)` as the only user ID. Define small request
DTOs that accept string-safe IDs, map domain enums to Thrift enum integers, and
return:

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "notifications": [],
    "next_cursor": "",
    "has_more": false
  }
}
```

Map invalid input to HTTP 400 and all unrecognized internal failures to HTTP
500 with `通知服务暂时不可用`. Do not return raw SQL, dedupe keys, recipient
IDs, or internal errors.

- [ ] **Step 6: Wire the service into application startup**

Initialize notification service after basic infrastructure and before modules
that publish events:

```go
notificationSVC, err := notification.InitService(
    &notification.ServiceComponents{
        DB: basicServices.infra.DB,
        IDGen: basicServices.infra.IDGenSVC,
        UserProfiles: basicServices.userSVC.DomainSVC,
    },
)
if err != nil {
    return nil, err
}
```

Store it on `primaryServices` and retain a single process-wide instance for
handler access and producer injection.

- [ ] **Step 7: Run API tests and commit**

Run:

```bash
cd backend
go test ./application/notification ./api/handler/coze ./api/router/coze \
  -run 'Test(Notification|RegisterIncludesNotification)' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/application/notification \
  backend/application/application.go \
  backend/api/handler/coze/notification_service.go \
  backend/api/handler/coze/notification_service_test.go \
  backend/api/router/coze/notification_route_test.go
git commit -m "feat: expose notification center APIs"
```

## Task 4: Publish task and workspace events

**Files:**

- Modify: `backend/application/agentthread/runner.go`
- Modify: `backend/application/agentthread/worker.go`
- Modify: `backend/application/agentthread/runner_test.go`
- Modify: `backend/application/scheduledtask/init.go`
- Modify: `backend/application/scheduledtask/dispatcher.go`
- Modify: `backend/application/scheduledtask/executor_test.go`
- Modify: `backend/application/workspace/workspace.go`
- Modify: `backend/application/workspace/workspace_test.go`
- Modify: `backend/application/application.go`

- [ ] **Step 1: Add failing AgentThread terminal tests**

Create a recording publisher and assert:

```go
require.Equal(t, notification.PublishRequest{
    Scope: notificationentity.ScopePersonal,
    SpaceID: 20,
    Category: notificationentity.CategoryTask,
    EventType: "agent_run.succeeded",
    Title: "任务已完成",
    Content: "你的任务已经完成，可以查看结果。",
    TargetType: notificationentity.TargetTaskThread,
    TargetID: "10",
    DedupeKey: "agent_run:30:succeeded:40",
    RecipientIDs: []int64{40},
}, publisher.requests[0])
```

Add corresponding failed and canceled assertions. Add a negative assertion
for run metadata `{"source":"scheduled_task"}` so scheduled runs do not create
two notifications.

- [ ] **Step 2: Inject and invoke the AgentThread publisher**

Add `TerminalPublisher notification.Publisher` to the AgentThread
`ApplicationService`, `RunProcessorOptions`, and `RunProcessor`.
`backend/application/application.go` assigns the initialized publisher to the
AgentThread service, and `worker.go` passes it when constructing the processor.

After a terminal run is durably recorded, call a helper that:

- ignores nil, interrupted, subagent, and scheduled-task runs;
- maps succeeded/failed/canceled to bounded Chinese titles and summaries;
- uses `context.WithoutCancel` plus a five-second timeout;
- logs publish errors without changing the run result;
- uses `agent_run:<run_id>:<status>:<creator_id>` as the dedupe key.

- [ ] **Step 3: Add failing ScheduledTask notification tests**

For both success and failure, assert one request with:

```go
notification.PublishRequest{
    Scope: notificationentity.ScopePersonal,
    SpaceID: task.SpaceID,
    Category: notificationentity.CategoryScheduledTask,
    EventType: "scheduled_task_execution.succeeded",
    Title: "定时任务执行成功",
    Content: task.Name,
    TargetType: notificationentity.TargetScheduledTaskCenter,
    TargetID: strconv.FormatInt(task.ID, 10),
    DedupeKey: "scheduled_task_execution:200:succeeded:100",
    RecipientIDs: []int64{task.CreatorID},
}
```

The failure case uses a generic safe summary and never includes executor raw
errors.

- [ ] **Step 4: Publish after durable ScheduledTask completion**

Add `Notifier notification.Publisher` to `ServiceComponents` and
`ExecutionDispatcher`. Change `finishFailure` to receive the task and execution
entities rather than bare IDs so success, ordinary failure, and panic recovery
all publish the same bounded event after `FinishExecution` and
`RecordTaskExecution` succeed. A notification error is logged and does not
replace the persisted execution result.

- [ ] **Step 5: Add failing workspace-member notification tests**

Assert:

- each newly added member receives one `workspace.member_added` notification;
- role changes notify only the affected member;
- removals notify the removed member after the domain mutation succeeds;
- skipped existing members do not receive duplicate notifications;
- a notification failure does not turn a successful member mutation into an
  API failure.

- [ ] **Step 6: Publish workspace member changes**

Add `Notifier notification.Publisher` to the workspace `ApplicationService`.
Use `workspace:<space_id>:member:<user_id>:<event>:<role>` dedupe keys, target
the workspace page, and include only role labels already available to the
affected user. Do not send owner-only metadata or another member's profile.

- [ ] **Step 7: Wire all publishers and run producer tests**

In `backend/application/application.go`, pass the same notification service to
AgentThread processor wiring, ScheduledTask `ServiceComponents`, and
`workspace.SVC`.

Run:

```bash
cd backend
go test ./application/agentthread ./application/scheduledtask \
  ./application/workspace \
  -run 'Test.*Notification' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/application/agentthread/runner.go \
  backend/application/agentthread/worker.go \
  backend/application/agentthread/runner_test.go \
  backend/application/scheduledtask/init.go \
  backend/application/scheduledtask/dispatcher.go \
  backend/application/scheduledtask/executor_test.go \
  backend/application/workspace/workspace.go \
  backend/application/workspace/workspace_test.go \
  backend/application/application.go
git commit -m "feat: publish user notification events"
```

## Task 5: Build the shared Nuwax-aligned notification popover

**Files:**

- Create:
  `frontend/apps/coze-studio/src/components/notification-center/service.ts`
- Create:
  `frontend/apps/coze-studio/src/components/notification-center/use-notifications.ts`
- Create:
  `frontend/apps/coze-studio/src/components/notification-center/notification-bell.tsx`
- Create:
  `frontend/apps/coze-studio/src/components/notification-center/index.less`
- Create:
  `frontend/apps/coze-studio/src/components/notification-center/__tests__/notification-bell.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-top-bar.tsx`
- Modify:
  `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`
- Modify:
  `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
- Modify:
  `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: Add failing component tests**

Mock the generated clients and cover:

```tsx
it('shows unread count and does not clear it when opened', async () => {
  mockGetUnreadCount.mockResolvedValue({ data: { count: 3 } });
  mockListNotifications.mockResolvedValue({
    data: {
      notifications: [unreadNotification],
      next_cursor: '',
      has_more: false,
    },
  });

  render(<NotificationBell />);
  expect(await screen.findByText('3')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: '通知' }));
  expect(await screen.findByText('任务已完成')).toBeInTheDocument();
  expect(mockMarkAllRead).not.toHaveBeenCalled();
});
```

Also test `99+`, loading, empty, error/retry, cursor load-more, single read,
bulk read, optimistic rollback, polling cleanup, and the three target routes.

- [ ] **Step 2: Run the component test and confirm RED**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/components/notification-center/__tests__/notification-bell.test.tsx
```

Expected: FAIL because the shared component does not exist.

- [ ] **Step 3: Add generated-client aliases**

Export only these bindings:

```ts
import { workbenchTask } from '@coze-studio/api-schema';

export const listNotifications = workbenchTask.ListNotifications;
export const getNotificationUnreadCount =
  workbenchTask.GetNotificationUnreadCount;
export const markNotificationsRead =
  workbenchTask.MarkNotificationsRead;
export const markAllNotificationsRead =
  workbenchTask.MarkAllNotificationsRead;

export type Notification = workbenchTask.WorkbenchNotification;
```

- [ ] **Step 4: Implement the notification hook**

The hook must expose:

```ts
interface NotificationCenterState {
  notifications: Notification[];
  unreadCount: number;
  loading: boolean;
  loadingMore: boolean;
  error: string;
  hasMore: boolean;
  open: boolean;
  setOpen: (open: boolean) => void;
  retry: () => Promise<void>;
  loadMore: () => Promise<void>;
  markRead: (notification: Notification) => Promise<void>;
  markAllRead: () => Promise<void>;
}
```

Implementation rules:

- fetch unread count on mount and when `document.visibilityState` returns to
  `visible`;
- poll every 30 seconds only while visible;
- fetch the first page when the popover first opens;
- merge pages by notification ID and keep descending order;
- cancel stale state writes on unmount;
- optimistically update read state and roll back on API failure;
- never call `MarkAllNotificationsRead` merely because the popover opened.

- [ ] **Step 5: Implement the popover UI**

Use `Popover` from `@coze-arch/coze-design` with `position="bottomRight"` and
`trigger="click"`. Preserve the existing button class through a
`className` prop.

The panel structure is:

```tsx
<section className="notification-center" aria-label="通知中心">
  <header className="notification-center__header">
    <strong>通知</strong>
    <Button
      size="small"
      theme="borderless"
      disabled={unreadCount === 0}
      onClick={markAllRead}
    >
      全部已读
    </Button>
  </header>
  <div className="notification-center__list">
    {content}
  </div>
</section>
```

Each row is a keyboard-accessible button with a 30px avatar/system icon,
14px title, 12px bounded plain-text summary, 12px time, and unread dot. Do not
render notification Markdown or HTML.

Map only these target types:

```ts
TaskThread -> `/space/${spaceID}/tasks/${targetID}`
ScheduledTaskCenter -> `/space/${spaceID}/task-center`
Workspace -> `/space/${spaceID}/workspace`
None -> no navigation
```

Ignore an invalid or missing space/target ID and show a non-blocking Toast.

- [ ] **Step 6: Add Nuwax-aligned responsive styles**

Use a 500px desktop panel with a 500px maximum list height, 10px item gap,
16px/12px item padding, neutral border, unread pale-blue background, and
Coze focus ring. At widths below 480px, cap the panel width to
`calc(100vw - 24px)` and height to `min(500px, 70vh)`.

- [ ] **Step 7: Replace all three static bells**

Use:

```tsx
<NotificationBell className="chat-workbench-icon-button" />
```

```tsx
<NotificationBell className="coze-prototype-icon-button" />
```

Remove direct `IconCozBell` imports from the three top bars. Keep the existing
assistant status and avatar unchanged.

- [ ] **Step 8: Run frontend tests and commit**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/components/notification-center/__tests__/notification-bell.test.tsx \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: PASS.

Commit:

```bash
git add frontend/apps/coze-studio/src/components/notification-center \
  frontend/apps/coze-studio/src/pages/workbench/index.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-top-bar.tsx \
  frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx \
  frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx
git commit -m "feat: add shared notification popover"
```

## Task 6: Production verification and acceptance

**Files:**

- Modify:
  `docs/superpowers/specs/2026-07-18-nuwax-notification-center-parity-design.md`

- [ ] **Step 1: Run backend targeted tests**

Run:

```bash
cd backend
go test ./domain/notification/... ./application/notification \
  ./application/scheduledtask ./application/workspace \
  ./api/handler/coze ./api/router/coze -count=1
go test ./application/agentthread \
  -run 'Test.*Notification' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend targeted tests and type checking**

Run:

```bash
cd frontend/packages/arch/api-schema
rushx test -- __tests__/workbench-task-contract.test.ts
cd ../../../apps/coze-studio
npm run test -- \
  src/components/notification-center/__tests__/notification-bell.test.tsx \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

Expected: PASS with no TypeScript errors.

- [ ] **Step 3: Revalidate Atlas**

Run:

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Expected: valid migration hash and syntax. Do not apply the migration until the
user explicitly authorizes the database change.

- [ ] **Step 4: Apply the authorized migration and restart services**

After explicit authorization, apply the migration using the repository's
debug database runbook, restart the Go backend, and keep the existing frontend
session or restart it only if generated clients require a rebuild.

- [ ] **Step 5: Seed notifications through real business actions**

Using the local Coze account, complete these flows:

1. run one successful Agent task;
2. trigger one failed Agent task through a safe invalid runtime input;
3. manually execute one scheduled task;
4. add or change a member in a team workspace.

Do not insert notification rows manually for acceptance.

- [ ] **Step 6: Validate with Codex in-app browser**

Open these URLs:

```text
http://localhost:8080/space/<personal-space-id>/chats/new
http://localhost:8080/space/<personal-space-id>/tasks/<thread-id>
http://localhost:8080/space/<team-space-id>/workspace
```

Verify:

- unread Badge appears and caps at `99+`;
- the Nuwax-style popover opens from every top bar;
- opening does not clear unread state;
- single and bulk read persist after refresh;
- cursor load-more preserves ordering without duplicates;
- target links stay inside the authorized current space;
- error/retry works when the backend is temporarily unavailable;
- browser console has no unhandled errors.

- [ ] **Step 7: Record evidence and commit**

Append the tested URL, account, space IDs, event source, visible result,
console result, test commands, and any residual risk to the design document.

Commit:

```bash
git add docs/superpowers/specs/2026-07-18-nuwax-notification-center-parity-design.md
git commit -m "docs: record notification center acceptance"
```

Only after all checks pass may the feature be described as fully aligned and
production-ready.
