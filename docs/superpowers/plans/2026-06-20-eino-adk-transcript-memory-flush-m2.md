# Eino ADK Transcript And Memory Flush M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist complete Eino message transcripts before summarization and at
successful terminal states, then enqueue idempotent long-term-memory update
jobs without blocking successful runs on queue failures.

**Architecture:** Coze stores immutable transcript snapshots keyed by
`run_id + idempotency_key`, where the key is derived from snapshot kind and a
SHA-256 digest of canonical Eino message JSON. The immutable snapshot preserves
the complete original JSON, while digest canonicalization removes only Eino's
volatile `_eino_msg_id`; all business fields remain part of the digest. Eino
summarization remains the state-rewrite engine, but its callback must durably
persist the original messages before they are replaced. A terminal middleware
persists the final state. Eligible snapshots enqueue durable memory update jobs
that reference the snapshot; extraction and fact mutation remain asynchronous
work for the later Memory milestone.

**Tech Stack:** Go 1.24, Eino `v0.9.9` ADK middleware, GORM, MySQL 8, Atlas
migrations, Go testing, Testify.

---

## File Structure

- Modify `backend/domain/agentthread/entity/thread.go` for transcript and
  memory-job entities.
- Modify `backend/domain/agentthread/repository/repository.go` and
  `backend/domain/agentthread/repository/mysql.go` for idempotent persistence.
- Modify `backend/domain/agentthread/service/service.go` and
  `backend/domain/agentthread/service/service_impl.go` for validation and ID
  generation.
- Modify `backend/application/agentthread/dto.go`,
  `backend/application/agentthread/service.go`, and
  `backend/application/agentthread/thread_app.go` for internal application
  contracts.
- Create `backend/application/agentthread/adk_transcript.go` for stable Eino
  transcript encoding, persistence hooks, memory eligibility, and events.
- Create `backend/application/agentthread/adk_transcript_test.go` for hook and
  serialization contracts.
- Modify `backend/application/agentthread/adk_middleware.go` to attach the
  summarization callback and terminal middleware.
- Modify `backend/application/application.go` to wire the durable application
  store and event sink.
- Add `docker/atlas/migrations/20260620000200_agent_transcripts_memory_jobs.sql`
  and update `docker/atlas/opencoze_latest_schema.hcl`.

## Task 1: Durable Transcript And Memory Job Models

- [x] Add failing repository tests proving:
  - transcript JSON round trips without dropping tool, reasoning, or
    multimodal fields;
  - duplicate `(run_id, idempotency_key)` snapshot inserts return the original
    row;
  - duplicate memory job keys do not create a second row.
- [x] Add entities:

```go
type TranscriptSnapshot struct {
	ID             int64
	ThreadID       int64
	RunID          int64
	SpaceID        int64
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
	Metadata       string
	CreatedAt      int64
}

type MemoryFlushJob struct {
	ID                   int64
	ThreadID             int64
	RunID                int64
	SpaceID              int64
	UserID               int64
	AssistantID           string
	TranscriptSnapshotID int64
	IdempotencyKey       string
	Status               MemoryFlushJobStatus
	AttemptCount         int32
	LastError            string
	AvailableAt          int64
	CreatedAt            int64
	UpdatedAt            int64
}
```

- [x] Add repository create-or-get methods using
  `clause.OnConflict{DoNothing: true}` followed by lookup by the unique key.
- [x] Add service validation for ownership fields, JSON payloads, digest length,
  kind/status enums, and referenced snapshot identity.
- [x] Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags='all=-l -N' \
  ./domain/agentthread/repository ./domain/agentthread/service \
  -run 'TestThreadRepository.*Transcript|TestThreadRepository.*MemoryFlush|TestPersistTranscript|TestEnqueueMemoryFlush'
```

Expected: PASS.

## Task 2: Schema Migration

- [x] Add `agent_transcript_snapshots` with:
  - JSON `messages` and `metadata`;
  - unique key `(run_id, idempotency_key)`;
  - thread/run chronological indexes.
- [x] Add `agent_memory_flush_jobs` with:
  - unique key `(run_id, idempotency_key)`;
  - pending-work index `(status, available_at, created_at)`;
  - transcript snapshot reference ID and tenant attribution fields.
- [x] Update the canonical HCL and run:

```bash
atlas migrate hash --dir file://docker/atlas/migrations
```

- [x] Run SQLite AutoMigrate repository tests and `git diff --check`.

## Task 3: Stable Eino Transcript Encoding

- [x] Add failing tests that encode and decode complete `schema.Message`
  values including:
  - user and assistant text;
  - reasoning content and response metadata;
  - tool calls and tool results;
  - user and assistant multimodal parts.
- [x] Implement:

```go
func encodeADKTranscript(messages []*schema.Message) (
	raw string,
	digest string,
	messageCount int32,
	err error,
)
```

The digest must be SHA-256 of canonical persisted JSON that excludes only the
volatile top-level `extra._eino_msg_id`; the stored JSON remains exact. Nil
messages are discarded deterministically. Empty transcripts and
non-JSON-compatible `Extra` values fail explicitly.

- [x] Add `ADKTranscriptStore` and `ADKMemoryFlushQueue` interfaces plus the
application-backed adapter.

## Task 4: Summarization And Terminal Hooks

- [x] Add failing middleware tests proving:
  - summarization persists the full pre-summary transcript before replacement;
  - persistence failure prevents summarization;
  - successful terminal execution persists the final state;
  - interrupted, canceled, and failed executions do not create terminal
    snapshots through `AfterAgent`;
  - replay/resume of an identical state returns the same snapshot and memory
    job.
- [x] Add `ADKMiddlewareTranscript` immediately before summarization in the
  ordered handler list.
- [x] Attach the summarization `Callback`:

```go
Callback: func(
	ctx context.Context,
	before, after adk.ChatModelAgentState,
) error {
	return transcriptHooks.PersistSummaryInput(ctx, before.Messages)
}
```

- [x] Implement `AfterAgent` on the transcript middleware for terminal state.
- [x] Enqueue memory only when the snapshot contains at least one non-empty
  user message and one non-tool-call assistant answer.

## Task 5: Fail-Open Memory Queue And Observability

- [x] Add failing tests proving queue errors do not fail summary or terminal
  execution.
- [x] Emit payload-safe events:
  - `context.transcript_persisted`;
  - `memory.update_queued`;
  - `memory.update_failed`.

Events contain snapshot ID, kind, digest, message count, and error text only;
they never include transcript content.

- [x] Ensure transcript persistence remains fail-closed while queue persistence
  is fail-open.

## Task 6: End-To-End And Verification

- [x] Run a long ADK conversation that summarizes, interrupts, resumes with a
  fresh executor, and succeeds.
- [x] Assert exactly one pre-summary snapshot, one terminal snapshot, and one
  memory job per digest despite replay.
- [x] Run focused tests 20 times.
- [x] Run the agentthread race suite.
- [x] Run:

```bash
cd backend
go mod verify
git diff --check
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./...
```

- [x] Update the master roadmap. Keep the actual memory extraction worker,
  structured fact updates, debounce merging, and user-facing memory management
  in M7.
