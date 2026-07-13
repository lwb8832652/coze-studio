# DeerFlow Agent Semantic Core Acceptance Evidence

Date: 2026-07-13

Status: `AR-PARITY-002.5` is **complete**. All seven paired live rows pass the
versioned semantic-core gate: six are `aligned`, cancellation is `stronger`,
and none are `different`, `blocked` or `unknown`.

## Acceptance Boundary

- Locked DeerFlow source revision:
  `5851f8250eb150ca23134c79b11ebc5073ac2789`.
- Scope is the backend Agent semantic core: mode projection, event ordering,
  durable Todo, Ultra subagents, clarification/follow-up, cancellation,
  terminal lifecycle, Token usage and SSE reconnect.
- Per the user's explicit instruction, DeerFlow page startup and visual
  comparison are not required for this closure because its frontend is
  unavailable. Evidence comes from locked source, authenticated backend APIs,
  runtime events, persisted state and the paired acceptance runner.
- Workspace files, Skill execution, MCP, Artifact content and long-term memory
  remain owned by their separate delivery slices; this gate does not relabel
  those scopes as passed.
- Reports are allowlist-built and contain no passwords, cookies, access/CSRF
  tokens, raw prompts/completions, reasoning, tool arguments/results,
  checkpoint bytes, provider bodies, object URIs or absolute user file paths.

## Live Results

| Case | Result | Verified semantics |
| --- | --- | --- |
| `core.flash.direct` | `aligned` | direct answer, no Plan/subagent, positive Token usage, one successful terminal |
| `core.thinking.direct` | `aligned` | thinking observed, no Plan/subagent, ordered completion |
| `core.pro.todo` | `aligned` | Pro thinking/Plan, three completed Todos, durable reload |
| `core.ultra.subagents` | `aligned` | Ultra thinking, two paired successful child lifecycles, parent completion |
| `core.clarify.followup` | `aligned` | real clarification tool, original-run interrupt, answer resume, two terminal runs |
| `core.cancel` | `stronger` | ordered cancel, exactly one canceled terminal, no success after cancel; NewX emits extra bounded diagnostics |
| `core.stream.reconnect` | `aligned` | cursor resume, zero duplicate events, terminal SSE observed, exactly one successful terminal |

The full local report is generated at
`docs/superpowers/evidence/.local-agent-semantic-core.md`. It is intentionally
not tracked because it is environment-specific runtime evidence.

## Evidence-Backed Fixes

- NewX SSE collection follows 30-second server stream segments with
  `Last-Event-ID`. REST terminal status only triggers a final drain; the gate
  fails unless the reconnected stream itself delivers its terminal frame.
- Token snapshots, Todo mutations and DeerFlow journal tool calls are reduced
  to bounded event families without retaining their payloads.
- Ultra exposes a code-owned `general_purpose` subagent only when subagent mode
  is explicitly enabled. Child runs inherit the model identity but disable
  recursive subagents, thinking and Plan. The built-in name is always reserved,
  inherits the parent's enabled ordinary Web/MCP/Artifact tools while denying
  nested delegation, both clarification aliases, confirmation and
  `present_files`, matching DeerFlow's inherit-all-plus-deny-list contract.
- Ultra child completion is counted only when a successful `task` tool result
  matches a previously observed `tool_call_id`; failed, duplicated and
  unpaired results cannot manufacture a completed child lifecycle.
- NewX supports DeerFlow's `ask_clarification` name as an alias for the same
  Eino stateful human-interaction tool.
- Public interrupted-run projection preserves only bounded resume identifiers,
  interaction kind, title/question/summary, risk level, required/free-text
  flags and choice id/label. Runtime addresses, tool/policy identifiers,
  actions, consequences, choice values, arguments, provider data and checkpoint
  material remain hidden.
- Clarification answers call the Workbench resume API for the interrupted source
  run and stream the new resume run instead of creating an unrelated follow-up.
  Event lookup is fully paginated, so a late interrupt is not lost after the
  first 1,000 events; non-clarification interrupts fall back to a normal
  same-thread follow-up instead of sending an invalid clarification response.
- DeerFlow cancellation waits on the live `running` status and uses its locked
  `wait=true&action=interrupt` contract, avoiding journal-flush timing races.

## Prerequisites

- NewX backend: `http://127.0.0.1:8888`.
- DeerFlow reference backend: `http://127.0.0.1:2026`.
- Atlas Community `v0.35.0` reports `Migration Status: OK`, current version
  `20260711000100`, five executed files and zero pending files.
- The reference runtime was operator-attested as running from the locked clean
  source checkout.

## Verification

The focused RED/GREEN and package suites passed:

```bash
cd backend
go test ./application/agentthread -count=1
go test ./internal/deerflowparity ./cmd/deerflow-parity-acceptance -count=1
go test -race ./internal/deerflowparity ./application/agentthread -count=1
go vet ./internal/deerflowparity ./application/agentthread
go test -p 1 -gcflags='all=-N -l' ./...
```

The paired gate passed with the environment-driven command documented in the
implementation plan:

```bash
cd backend
go run ./cmd/deerflow-parity-acceptance \
  --cases=semantic-core \
  --format=markdown \
  --out=../docs/superpowers/evidence/.local-agent-semantic-core.md \
  --timeout=5m
```

Post-apply schema verification passed:

```text
Migration Status: OK
Current Version: 20260711000100
Next Version: Already at latest version
Executed Files: 5
Pending Files: 0
```

Final race, vet, build and diff checks are recorded in the closing commit.
