# DeerFlow Agent Semantic Core Acceptance Evidence

Date: 2026-07-12

Status: `AR-PARITY-002.5` remains **in progress**. The fixture, transport,
runner and NewX in-process contract layers pass. The seven paired live rows are
blocked by explicit external prerequisites and are not reported as aligned.

## Baseline And Scope

- Locked DeerFlow source revision:
  `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- Local source verification:
  `/Users/liuwenbo/code/BuildingAI/deer-flow` resolves to the locked revision
  and has no tracked or untracked changes.
- Scope is limited to the semantic core: mode projection, event ordering,
  durable Todo state, Ultra subagent capability, clarification/follow-up,
  cancellation fencing, terminal lifecycle and SSE reconnect.
- Workspace files, Skill execution, MCP, Artifact content, long-term memory and
  the final ten-row compatibility matrix remain deferred.

## Acceptance Harness

The implementation adds a bounded Go acceptance harness under
`backend/internal/deerflowparity` and an environment-driven command at
`backend/cmd/deerflow-parity-acceptance`.

The harness provides:

- a versioned seven-case fixture contract;
- authenticated DeerFlow and NewX LangGraph-compatible adapters;
- bounded HTTP/SSE parsing, pagination and reconnect cursor handling;
- deterministic normalization and partial-order comparison;
- explicit `aligned`, `stronger`, `different` and `blocked` outcomes;
- a fail-closed CLI where only non-empty `aligned`/`stronger` reports exit 0;
- clean DeerFlow source-checkout verification plus explicit operator attestation
  that the reference runtime was started from that checkout;
- JSON and Markdown reports with environment labels and per-invariant evidence;
- allowlist-only observations that exclude credentials, raw prompts,
  completions, reasoning, tool arguments/results, checkpoint bytes, provider
  payloads and object URIs.

## Case Evidence

| Case | Fixture/runner | NewX in-process | Paired live |
| --- | --- | --- | --- |
| `core.flash.direct` | pass | mode, direct answer event order and token contract pass | blocked |
| `core.thinking.direct` | pass | thinking projection, direct answer event order and token contract pass | blocked |
| `core.pro.todo` | pass | Pro/Plan projection and three completed Todos survive serialized state reload | blocked |
| `core.ultra.subagents` | pass | Ultra thinking/subagent projection and bounded concurrency contract pass; live child count still required | blocked |
| `core.clarify.followup` | pass | the first run requests clarification and a second run on the same thread consumes the answer | blocked |
| `core.cancel` | pass | ADK cancellation contract passes; the authoritative canceled status yields one canonical terminal, callback errors stay diagnostic, and final readback rejects persisted assistant output or a success terminal | blocked |
| `core.stream.reconnect` | pass | bounded SSE cursor and duplicate-event normalization pass; live reconnect still required | blocked |

The in-process assertions import the same case IDs and mode context used by the
paired runner. This prevents the fixture and NewX production runtime parser
from drifting independently.

## Live Prerequisites

Read-only checks produced the following result:

- NewX frontend: `http://127.0.0.1:8080`, reachable with HTTP 200.
- NewX backend: `http://127.0.0.1:8888`, reachable; an unauthenticated identity
  request correctly returns HTTP 401.
- DeerFlow reference service: not listening on port 2026. It was intentionally
  not started because the user requested that only NewX be started.
- Debug MySQL Atlas status: `PENDING`.
- Current migration: `20260710000100`.
- Required next migration: `20260711000100`.
- NewX runtime log confirms the corresponding schema failure:
  `Unknown column 'execution_generation'`.

No migration was applied. Applying the external debug database migration is a
separate mutating operation and requires explicit authorization.

## Verification

The following commands passed:

```bash
cd backend
go test ./internal/deerflowparity ./cmd/deerflow-parity-acceptance -count=1
go test -race ./internal/deerflowparity ./cmd/deerflow-parity-acceptance -count=1
go vet ./internal/deerflowparity ./cmd/deerflow-parity-acceptance
go test ./application/agentthread -run 'TestADKSemanticCoreAcceptance|TestADKParityCancellationTerminal|TestPublicRunJournalMessageRedactsInternalPayloads' -gcflags='all=-N -l' -count=1
go test ./api/handler/coze -run 'TestLangGraphRunMessagesHandlerReturnsDeerFlowPage|TestLangGraphPublicProjectionDoesNotExposeSensitiveRuntimeData' -gcflags='all=-N -l' -count=1
go test ./internal/deerflowparity ./application/agentthread ./api/handler/coze ./api/router/coze -gcflags='all=-N -l' -count=1
go test -race ./internal/deerflowparity ./application/agentthread -run 'Test.*Parity|Test.*Contract|TestADKSemanticCoreAcceptance' -count=1
go vet ./internal/deerflowparity ./application/agentthread
go test -p 1 -gcflags='all=-N -l' ./...
```

The macOS race build emitted the repository's known benign `LC_DYSYMTAB`
linker warning and exited successfully.

Repository-level checks also passed:

```bash
APP_ENV=debug make build_server
git diff --check
```

Atlas was checked without mutation:

```text
Migration Status: PENDING
Current Version: 20260710000100
Next Version: 20260711000100
```

## Remaining Gate

`AR-PARITY-002.5` and its parent cannot close until both blockers are removed
and all seven paired cases are executed against the locked DeerFlow service and
the migrated NewX debug schema. Any live mismatch must first receive a RED
regression test; provider wording or timing differences are not parity defects.
