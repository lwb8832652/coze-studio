# M3.30 Eino ADK Subagent Replay Definition Snapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist the child subagent definition fields required for future
retry replay without broadening tool grants.

**Architecture:** `ApplicationADKSubagentRunRecorder` already writes child run
config. Extend that config to include the original full-chat input mode and
the normalized allowed static/dynamic tool grants.

**Tech Stack:** Go, Coze agentthread application service tests.

---

### Task 1: Add Red Test

**Files:**
- Modify: `backend/application/agentthread/adk_subagent_run_recorder_test.go`

- [x] **Step 1: Extend fixture definition**

In `TestApplicationADKSubagentRunRecorderStartsChildRun`, set:

```go
FullChatHistoryAsInput: true,
AllowedTools:           []string{"knowledge_lookup"},
AllowedDynamicTools:    []string{"web_search"},
```

- [x] **Step 2: Assert durable config snapshot**

Assert child run config JSON-equals:

```json
{
  "runtime": "eino_adk",
  "agent_name": "researcher",
  "agent_description": "Research public information.",
  "single_agent": {"agent_id": 1001, "version": "v1", "is_draft": false},
  "full_chat_history": true,
  "tool_policy": {
    "allowed_tools": ["knowledge_lookup"],
    "allowed_dynamic_tools": ["web_search"]
  }
}
```

- [x] **Step 3: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationADKSubagentRunRecorderStartsChildRun' -count=1 -gcflags="all=-l -N"
```

Expected red result: config does not yet contain `full_chat_history` or
`tool_policy`.

### Task 2: Implement Config Snapshot

**Files:**
- Modify: `backend/application/agentthread/adk_subagent_run_recorder.go`

- [x] **Step 1: Add full-chat mode**

Add `"full_chat_history": definition.FullChatHistoryAsInput` to the child
config payload.

- [x] **Step 2: Add tool policy**

Add `"tool_policy"` with normalized `allowed_tools` and
`allowed_dynamic_tools` slices. Use `normalizeConfigStringSlice` for both
fields so empty grants become stable empty arrays.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-replay-definition-snapshot-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-replay-definition-snapshot-m3.md`

- [x] **Step 1: Document replay definition snapshot**

State that child run config must preserve runtime, identity, SingleAgent
reference, full-chat mode, and tool policy grants for future backend replay.

- [x] **Step 2: Update roadmap**

Add M3.30 completion status and reduce the remaining task/person-month
estimate by one backend contract slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused backend test**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationADKSubagentRunRecorderStartsChildRun' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run broader affected backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestApplicationADKSubagentRunRecorder|TestADKSubagentToolProvider|TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor|TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Run frontend task tests and lint**

Run:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
cd frontend/apps/coze-studio && npx eslint src/pages/tasks/service.ts --cache --quiet
```

- [x] **Step 4: Check formatting and doc placeholders**

Run:

```bash
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-replay-definition-snapshot-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-replay-definition-snapshot-m3.md
```
