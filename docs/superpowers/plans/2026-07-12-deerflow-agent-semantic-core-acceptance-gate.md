# DeerFlow Agent Semantic Core Acceptance Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` (recommended) or
> `superpowers:executing-plans` to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `AR-PARITY-002.5` with a repeatable, redacted contract suite and
paired live runner that proves the completed NewX Eino semantic core behaves
equivalently to locked DeerFlow for modes, events, durable state, interruption,
clarification follow-up, cancellation and terminal lifecycle.

**Architecture:** Compare both products through their authenticated
LangGraph-compatible HTTP surfaces instead of importing either runtime. A small
Go package loads versioned acceptance cases, normalizes unstable wire details
into bounded semantic observations, enforces redaction, and evaluates explicit
invariants and partial ordering. A command-line runner owns login, thread/run
creation, SSE collection and state/message/event retrieval for each product;
credentials and environment-specific IDs come only from environment variables.

**Tech Stack:** Go, Hertz-compatible HTTP/JSON, Server-Sent Events, Testify,
locked DeerFlow `5851f8250eb150ca23134c79b11ebc5073ac2789`, NewX LangGraph
adapter.

---

## Locked Scope And Evidence

- DeerFlow baseline is exactly
  `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- DeerFlow creates a thread with `POST /api/threads`, then runs the Lead Agent
  with `POST /api/threads/:thread_id/runs/stream`; the request contains
  `assistant_id`, `input.messages`, `config`, `context` and `stream_mode`.
- NewX exposes the same thread/run/state/history/messages/events surface, while
  authentication differs: DeerFlow uses access/CSRF cookies and NewX uses its
  email-login session cookie.
- This gate verifies only the semantic core already implemented in
  `AR-PARITY-002.1` through `.4`: runtime/mode projection, prompt-owned behavior,
  conditional Todo/subagent capabilities, normalized event lifecycle, durable
  state, clarification/follow-up, cancellation and terminal completion.
- Workspace file tools/uploads, Skill package execution, MCP, Artifact content,
  long-term memory and the final ten-row compatibility matrix remain owned by
  later delivery slices. This gate records those rows as `deferred`, not
  `passed`, so it cannot manufacture parity evidence for unimplemented scope.
- Live evidence must never contain passwords, cookies, access/CSRF tokens,
  complete prompts/completions, reasoning text, tool arguments/results,
  checkpoint bytes, provider payloads, object URIs or absolute user file paths.

## Semantic Core Case Matrix

| ID | Mode | Required observation |
| --- | --- | --- |
| `core.flash.direct` | flash | no Plan/subagent capability, assistant output, token usage, one successful terminal sequence |
| `core.thinking.direct` | thinking | thinking requested, no Plan/subagent capability, assistant output and successful terminal sequence |
| `core.pro.todo` | pro | Plan enabled, durable Todo state survives state reload, no subagent capability, all Todo items terminally complete |
| `core.ultra.subagents` | ultra | thinking and subagent behavior is actually observed, at least two child lifecycles complete before the parent |
| `core.clarify.followup` | pro | the first run emits an actual clarification request; the answer starts a second run on the same thread and completes |
| `core.cancel` | pro | cancel reaches one canceled terminal state; final readback contains no persisted assistant output or success terminal |
| `core.stream.reconnect` | flash | reconnect cursor does not duplicate events and terminal event remains exactly once |

The command may mark a live row `blocked` only for an external prerequisite
such as a missing debug migration or unavailable reference server. Unit fixture
coverage remains mandatory even when a live endpoint is unavailable.

## File Structure

- Create `backend/internal/deerflowparity/contract.go`: versioned case,
  observation, invariant, comparison and report types.
- Create `backend/internal/deerflowparity/cases.go`: embedded fixture loading,
  schema validation and semantic-core coverage checks.
- Create `backend/internal/deerflowparity/normalize.go`: bounded SSE, event,
  message, state, Todo, token and terminal normalization.
- Create `backend/internal/deerflowparity/compare.go`: invariant and partial
  ordering evaluator with `aligned`, `stronger`, `different` and `blocked`
  results.
- Create focused tests beside each package file and
  `backend/internal/deerflowparity/testdata/semantic_core_cases.json`.
- Create `backend/internal/deerflowparity/http_client.go`: shared cookie jar,
  JSON/form requests, bounded bodies, timeout and response validation.
- Create `backend/internal/deerflowparity/sse.go`: bounded SSE parser with
  cursor and terminal detection.
- Create `backend/internal/deerflowparity/deerflow_client.go`: DeerFlow login,
  CSRF, thread/run and readback adapter.
- Create `backend/internal/deerflowparity/newx_client.go`: NewX login,
  numeric-thread adapter and equivalent LangGraph readback.
- Create `backend/internal/deerflowparity/runner.go`: case orchestration,
  cancel/follow-up/reconnect actions and sanitized report assembly.
- Create `backend/cmd/deerflow-parity-acceptance/main.go`: environment parsing,
  selected-case execution and JSON/Markdown report output.
- Create `backend/internal/deerflowparity/runner_test.go` with in-process fake
  servers proving auth, CSRF, request shapes, pagination, cancellation and
  redaction without external services.
- Create
  `docs/superpowers/evidence/2026-07-12-deerflow-agent-semantic-core-acceptance.md`
  only after verification, and update the P0 tracker.

## Task 1: Versioned Cases And Coverage Gate

**Files:**

- Create: `backend/internal/deerflowparity/contract.go`
- Create: `backend/internal/deerflowparity/cases.go`
- Create: `backend/internal/deerflowparity/cases_test.go`
- Create:
  `backend/internal/deerflowparity/testdata/semantic_core_cases.json`

- [x] **Step 1: Write RED fixture tests.** Require schema
  `newx.deerflow.agent.acceptance.v1`, unique IDs, all seven matrix rows, a valid
  mode, at least one required terminal state, declared event ordering and no
  inline credential/content fields.
- [x] **Step 2: Run the focused test and verify RED.**

  Run:
  `cd backend && go test ./internal/deerflowparity -run 'TestLoadCases|TestSemanticCoreCoverage' -count=1`

  Expected: compile failure because the package does not exist.
- [x] **Step 3: Implement minimal contract types.** `Case` contains ID, mode,
  safe prompt key, action list and invariants; `Observation` contains only
  product, case ID, mode, capability flags, event families/order, Todo status
  counts, child count, token totals and terminal status. Free-form model text is
  represented only by `assistant_message_present` and byte count.
- [x] **Step 4: Add the seven safe fixtures.** Store short deterministic prompts
  needed to trigger behavior, but no credentials, model answers or provider
  configuration. Mark the case file as semantic-core scope rather than the
  final ten-row matrix.
- [x] **Step 5: Run the focused tests.**

  Run:
  `cd backend && go test ./internal/deerflowparity -run 'TestLoadCases|TestSemanticCoreCoverage' -count=1`

  Expected: PASS.

## Task 2: Normalization, Redaction And Comparison

**Files:**

- Create: `backend/internal/deerflowparity/normalize.go`
- Create: `backend/internal/deerflowparity/normalize_test.go`
- Create: `backend/internal/deerflowparity/compare.go`
- Create: `backend/internal/deerflowparity/compare_test.go`

- [x] **Step 1: Write RED normalization tests.** Feed representative DeerFlow
  and NewX metadata/values/updates/events/end frames and prove unstable IDs,
  timestamps and wording collapse into the same event families while cursor
  order is preserved.
- [x] **Step 2: Write RED redaction tests.** Inputs containing cookies, bearer
  tokens, password keys, checkpoint bytes, raw prompt/completion, reasoning,
  tool arguments/results, provider bodies, object URIs or absolute file paths
  must be rejected before report serialization.
- [x] **Step 3: Write RED comparison tests.** Verify required capabilities,
  event multiplicity, partial ordering, terminal state, Todo completion,
  subagent count, clarification/follow-up transition, cancellation fencing and token
  presence. Extra bounded diagnostics may be `stronger`; missing required
  semantics are always `different`.
- [x] **Step 4: Implement bounded normalizers.** Use allowlists for public
  fields, maximum event/message counts and maximum string lengths. Treat
  complete model text and raw tool payloads as input-only facts and discard
  them after deriving booleans/counts.
- [x] **Step 5: Implement deterministic comparison.** Return one result per
  invariant with expected/actual symbolic values, never raw payloads. A case is
  `aligned` only when every required invariant passes.
- [x] **Step 6: Run focused and race tests.**

  Run:
  `cd backend && go test ./internal/deerflowparity -run 'TestNormalize|TestRedaction|TestCompare' -count=1`

  Run:
  `cd backend && go test -race ./internal/deerflowparity -count=1`

  Expected: PASS.

## Task 3: Authenticated HTTP And SSE Adapters

**Files:**

- Create: `backend/internal/deerflowparity/http_client.go`
- Create: `backend/internal/deerflowparity/http_client_test.go`
- Create: `backend/internal/deerflowparity/sse.go`
- Create: `backend/internal/deerflowparity/sse_test.go`
- Create: `backend/internal/deerflowparity/deerflow_client.go`
- Create: `backend/internal/deerflowparity/newx_client.go`
- Create: `backend/internal/deerflowparity/client_test.go`

- [x] **Step 1: Write RED transport tests.** Use `httptest.Server` to require
  bounded timeouts/body sizes, cookie jars, DeerFlow form login plus CSRF header,
  NewX JSON login, and status/content-type validation.
- [x] **Step 2: Write RED SSE tests.** Cover multiline `data`, comments,
  metadata run ID, values/updates/events, `end`, `error`, reconnect cursor,
  malformed frames and body-size exhaustion.
- [x] **Step 3: Implement shared transport.** Disable automatic credential
  logging, accept only configured loopback/test base URLs by default, and retain
  cookies only inside the client. Returned errors contain status, endpoint name
  and bounded safe reason, never response bodies.
- [x] **Step 4: Implement DeerFlow adapter.** Login through
  `/api/v1/auth/login/local`, read `csrf_token`, create a UUID thread, stream
  `lead_agent`, then read thread state/history and run messages/events using the
  locked API shape.
- [x] **Step 5: Implement NewX adapter.** Login through
  `/api/passport/web/email/login/`, create a thread with server-owned user and
  configured `space_id`, stream the same `lead_agent` request, then read state,
  history, messages and events. Do not trust a fixture-supplied user ID.
- [x] **Step 6: Run focused transport tests.**

  Run:
  `cd backend && go test ./internal/deerflowparity -run 'TestHTTP|TestSSE|TestDeerFlowClient|TestNewXClient' -count=1`

  Expected: PASS.

## Task 4: Paired Runner And Action Semantics

**Files:**

- Create: `backend/internal/deerflowparity/runner.go`
- Create: `backend/internal/deerflowparity/runner_test.go`
- Create: `backend/cmd/deerflow-parity-acceptance/main.go`

- [x] **Step 1: Write RED runner tests.** Fake both products and prove ordinary
  run, clarification/follow-up, cancel, and reconnect cases execute the declared
  actions, retrieve all readback pages and produce one comparison result.
- [x] **Step 2: Add prerequisite classification.** Connection refusal, missing
  required environment variables, reference revision mismatch and known schema
  migration absence produce `blocked` with a safe symbolic reason. HTTP 4xx/5xx
  after prerequisites pass is a real `different` result.
- [x] **Step 3: Implement case orchestration.** Run products sequentially by
  default to reduce provider nondeterminism; apply per-case timeout; always
  attempt bounded readback after terminal/error; never retry a mutating request
  without an idempotency key.
- [x] **Step 4: Implement report writers.** JSON is machine-readable and
  Markdown summarizes revision, environment labels, case status, invariant
  results and blockers. Neither format can serialize raw captures. Empty or
  unknown-result reports fail closed; only non-empty `aligned`/`stronger`
  results may exit successfully.
- [x] **Step 5: Implement the command.** Read
  `DEERFLOW_PARITY_DEERFLOW_URL`, `DEERFLOW_PARITY_NEWX_URL`,
  `DEERFLOW_PARITY_DEERFLOW_SOURCE_DIR`,
  `DEERFLOW_PARITY_DEERFLOW_RUNTIME_ATTESTED`,
  `DEERFLOW_PARITY_EMAIL`, `DEERFLOW_PARITY_PASSWORD`,
  `DEERFLOW_PARITY_NEWX_SPACE_ID`, optional case IDs and output path from the
  environment/flags. The command verifies the clean source checkout against
  the revision locked by the fixture and requires an explicit operator runtime
  attestation. Secrets are never accepted as flags and never printed. A report
  containing any `different` or `blocked` row exits non-zero.
- [x] **Step 6: Run focused runner tests.**

  Run:
  `cd backend && go test ./internal/deerflowparity -run 'TestRunner|TestReport|TestCommandConfig' -count=1`

  Expected: PASS.

## Task 5: Contract Regression And Live Core Acceptance

**Files:**

- Modify: `backend/application/agentthread/adk_contract_test.go`
- Create:
  `docs/superpowers/evidence/2026-07-12-deerflow-agent-semantic-core-acceptance.md`
- Modify:
  `docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md`

- [x] **Step 1: Extend in-process contract tests.** Reuse the semantic case IDs
  to assert NewX runtime/mode capability projection, event partial ordering,
  durable Todo state, clarification follow-up, cancellation fencing and exactly-once terminal
  behavior even when the live reference server is unavailable.
- [x] **Step 2: Run the complete package verification.**

  Run:
  `cd backend && go test ./internal/deerflowparity ./application/agentthread ./api/handler/coze ./api/router/coze -gcflags='all=-N -l' -count=1`

  Run:
  `cd backend && go test -race ./internal/deerflowparity ./application/agentthread -run 'Test.*Parity|Test.*Contract' -count=1`

  Run:
  `cd backend && go vet ./internal/deerflowparity ./application/agentthread`

  Expected: PASS; the macOS linker may emit its existing benign
  `LC_DYSYMTAB` warning during race builds.
- [x] **Step 3: Check live prerequisites without mutation.** Confirm NewX and
  DeerFlow base URLs, locked DeerFlow revision, model availability and the NewX
  debug schema. If the schema lacks a required migration, record `blocked` and
  request explicit database-apply authorization instead of changing it.
- [ ] **Step 4: Run the seven paired cases when prerequisites pass.**

  Run:
  `cd backend && go run ./cmd/deerflow-parity-acceptance -cases semantic-core -format markdown -out ../docs/superpowers/evidence/.local-agent-semantic-core.md`

  Expected: all rows `aligned`/documented `stronger`, or an explicit safe
  blocker. A blocker does not close `.5`.
- [ ] **Step 5: Fix only evidence-backed semantic differences.** Add a RED test
  for each observed mismatch before changing production code; do not compensate
  for provider wording or timing differences.
- [x] **Step 6: Run full backend and build verification.**

  Run:
  `cd backend && go test -p 1 -gcflags='all=-N -l' ./...`

  Run:
  `APP_ENV=debug make build_server`

  Run:
  `gofmt -w backend/internal/deerflowparity backend/cmd/deerflow-parity-acceptance backend/application/agentthread/adk_contract_test.go`

  Run: `git diff --check`

  Expected: PASS.
- [ ] **Step 7: Record evidence and close the tracker.** The evidence document
  must distinguish fixture/in-process/live results, list every blocker and keep
  later-slice cases deferred. Mark `AR-PARITY-002.5` and parent
  `AR-PARITY-002` complete only after all seven live core rows pass.

  Current status: evidence is recorded, but closure remains blocked. DeerFlow
  is intentionally not running and the external debug database still has
  pending migration `20260711000100`; no live row is marked aligned.
- [x] **Step 8: Commit the completed mainline slice.**

  ```bash
  git add backend/internal/deerflowparity \
    backend/cmd/deerflow-parity-acceptance \
    backend/application/agentthread/adk_contract_test.go \
    docs/superpowers/plans/2026-07-12-deerflow-agent-semantic-core-acceptance-gate.md \
    docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md \
    docs/superpowers/evidence/2026-07-12-deerflow-agent-semantic-core-acceptance.md
  git commit -m "test: add DeerFlow semantic core acceptance gate"
  ```

  Recorded in the `test: add trustworthy DeerFlow semantic core gate` commit.
  This commit does not close the live seven-case gate.

## Self-Review

- The plan covers every completed semantic-core behavior and gives each one a
  deterministic fixture plus live observation path.
- The plan does not claim Workspace, Skill, MCP, Artifact or Memory parity and
  does not close the final ten-row compatibility gate early.
- Authentication and readback are product-specific; execution input and
  normalized comparison are shared.
- Reports are allowlist-built rather than sanitized after serializing raw
  payloads, so forbidden fields cannot accidentally leak into evidence.
- Database migration apply remains outside this plan unless the user explicitly
  authorizes it after a read-only status check.
