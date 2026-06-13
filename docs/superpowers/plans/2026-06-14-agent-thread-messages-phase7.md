# Agent Thread Messages Phase 7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first production-grade message history persistence layer for canonical task threads.

**Architecture:** Keep this phase below HTTP/UI. Extend `domain/agentthread` with message repository and service methods, persist messages in `agent_thread_messages`, and expose application DTOs for later handlers. Thread list/detail APIs continue to work as they do today until the next phase wires message HTTP routes.

**Tech Stack:** Go, GORM, MySQL/Atlas migrations, existing agentthread DDD package, Go unit tests with sqlite.

---

### Task 1: Message Repository

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] **Step 1: Write the failing repository test**

Add `TestThreadRepositoryCreateAndListMessages`:
- Auto-migrate `threadPO` and `messagePO`
- Create a message with `ThreadID`, `RunID`, `Role`, `Content`, `Metadata`, and `CreatedAt`
- List by thread id and assert chronological order by `created_at ASC, id ASC`
- Assert pagination total and metadata round-trip

- [ ] **Step 2: Run test to verify it fails**

```bash
cd backend
go test ./domain/agentthread/repository -run TestThreadRepositoryCreateAndListMessages
```

Expected: FAIL to compile because `CreateMessage`, `ListMessages`, `ListMessagesRequest`, and `messagePO` do not exist.

- [ ] **Step 3: Implement repository support**

Add to `repository.go`:
- `CreateMessage(ctx context.Context, message *entity.Message) error`
- `ListMessages(ctx context.Context, req ListMessagesRequest) ([]*entity.Message, int64, error)`
- `ListMessagesRequest { ThreadID int64; Page int32; PageSize int32 }`

Add to `mysql.go`:
- `messagePO` table mapping for `agent_thread_messages`
- `messageToPO`, `(*messagePO).toEntity`
- `CreateMessage` default `CreatedAt` to `time.Now().UnixMilli()`
- `ListMessages` default page/page_size and order `created_at ASC, id ASC`
- Reuse `optionalJSON` for metadata validation

- [ ] **Step 4: Run repository test to verify it passes**

```bash
cd backend
go test ./domain/agentthread/repository -run TestThreadRepositoryCreateAndListMessages
```

Expected: PASS.

### Task 2: Message Domain Service

**Files:**
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests:
- `TestAppendMessageRequiresContentAndValidRole`
- `TestAppendMessageCreatesMessageWithGeneratedID`
- `TestListMessagesNormalizesPaging`

The tests should use the existing in-memory repo and assert generated ids, trimmed content, role preservation, run id preservation, and normalized list paging.

- [ ] **Step 2: Run service tests to verify they fail**

```bash
cd backend
go test ./domain/agentthread/service -run 'TestAppendMessage|TestListMessages'
```

Expected: FAIL because service methods and memory repo methods do not exist.

- [ ] **Step 3: Implement service support**

Add DTOs:
- `AppendMessageRequest { ThreadID int64; RunID int64; Role entity.MessageRole; Content string; Metadata string }`
- `ListMessagesRequest { ThreadID int64; Page int32; PageSize int32 }`

Extend `ThreadService`:
- `AppendMessage(ctx, req) (*entity.Message, error)`
- `ListMessages(ctx, req) ([]*entity.Message, int64, error)`

Implementation:
- require repository and id generator for append
- validate `ThreadID > 0`
- validate role is one of `user`, `assistant`, `tool`, `system`
- trim content and reject empty content
- generate message id
- set `CreatedAt` to current milliseconds
- delegate list with default page `1` and page_size `50`

- [ ] **Step 4: Run service tests to verify they pass**

```bash
cd backend
go test ./domain/agentthread/service -run 'TestAppendMessage|TestListMessages'
```

Expected: PASS.

### Task 3: Application DTO Boundary

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`

- [ ] **Step 1: Write failing application tests**

Add tests:
- `TestApplicationAppendMessageMapsDomainMessage`
- `TestApplicationListMessagesMapsDomainMessages`

Use `recordingThreadService` to capture domain requests and return domain messages.

- [ ] **Step 2: Run application tests to verify they fail**

```bash
cd backend
go test ./application/agentthread -run 'TestApplication(Append|List)Messages'
```

Expected: FAIL because application DTOs and methods do not exist.

- [ ] **Step 3: Implement application support**

Add:
- `MessageRole` constants mirroring domain roles
- `MessageSummary`
- `AppendMessageRequest/Response`
- `ListMessagesRequest/Response`
- `ApplicationService.AppendMessage`
- `ApplicationService.ListMessages`
- `DomainMessageToSummary`

- [ ] **Step 4: Run application tests to verify they pass**

```bash
cd backend
go test ./application/agentthread -run 'TestApplication(Append|List)Messages'
```

Expected: PASS.

### Task 4: Atlas Schema

**Files:**
- Create: `docker/atlas/migrations/20260614000100_agent_thread_messages.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`

- [ ] **Step 1: Add migration and schema table**

Create `agent_thread_messages` with:
- `id bigint` primary key
- `thread_id bigint not null`
- `run_id bigint not null default 0`
- `role varchar(32) not null`
- `content longtext not null`
- `metadata json null`
- `created_at bigint not null`
- indexes `idx_agent_thread_messages_thread_created (thread_id, created_at)` and `idx_agent_thread_messages_run_created (run_id, created_at)`

- [ ] **Step 2: Refresh atlas sum**

```bash
cd docker
make atlas-hash
```

If the local make target is unavailable, use the repository's existing Atlas hash command.

### Task 5: Verification and Commit

**Files:**
- Verify repository, service, and application packages
- Verify diff whitespace

- [ ] **Step 1: Run focused Go tests**

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/plans/2026-06-14-agent-thread-messages-phase7.md backend/domain/agentthread/repository/repository.go backend/domain/agentthread/repository/mysql.go backend/domain/agentthread/repository/mysql_test.go backend/domain/agentthread/service/service.go backend/domain/agentthread/service/service_impl.go backend/domain/agentthread/service/service_impl_test.go backend/application/agentthread/dto.go backend/application/agentthread/service.go backend/application/agentthread/service_test.go backend/application/agentthread/thread_app.go backend/application/agentthread/thread_app_test.go docker/atlas/migrations/20260614000100_agent_thread_messages.sql docker/atlas/opencoze_latest_schema.hcl docker/atlas/migrations/atlas.sum
git commit -m "feat: add agent thread messages"
```
