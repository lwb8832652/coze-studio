# M3.31 Eino ADK Subagent Retry Executor Replay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement ADKExecutor replay for queued subagent retry runs by
re-invoking the original child AgentTool call.

**Architecture:** ADKExecutor receives an injected source-child resolver. It
parses the retry command and source child run payloads, rebuilds the child
agent from the durable child config, and invokes it through Eino
`adk.NewAgentTool`.

**Tech Stack:** Go, Eino ADK, Coze agentthread ADK executor tests.

---

### Task 1: Add Red Test

**Files:**
- Modify: `backend/application/agentthread/adk_executor_test.go`

- [x] **Step 1: Add source child fixture**

Create a source child `RunSummary` with `run_kind='subagent'`,
`parent_run_id=15`, input schema `coze.subagent_tool_call.v1`, and config
containing `agent_name`, `agent_description`, SingleAgent reference,
`full_chat_history=false`, and empty tool grants.

- [x] **Step 2: Add resolver fixture**

Use `ADKSubagentRetrySourceResolverFunc` to assert the retry run ID, source
run ID, and parent run ID passed by the executor.

- [x] **Step 3: Add recording child agent**

Add `recordingSubagentReplayAgent` that records `AgentInput.Messages` and
returns `child replay answer`.

- [x] **Step 4: Assert replay**

Call `ExecuteSubagentRetry` on a retry run with
`command.subagent_retry.source_run_id=20`. Assert the result message is
`child replay answer`, metadata contains only source/parent IDs, factory input
uses the source child run, and the child agent receives `redo analysis`.

- [x] **Step 5: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestADKExecutorReplaysSubagentRetryFromSourceChildRun' -count=1 -gcflags="all=-l -N"
```

Expected red result: resolver types, option, and `ExecuteSubagentRetry` do not
exist.

### Task 2: Implement ADK Retry Replay

**Files:**
- Modify: `backend/application/agentthread/adk_executor.go`
- Create: `backend/application/agentthread/adk_subagent_retry_executor.go`

- [x] **Step 1: Add resolver option**

Add `ADKSubagentRetrySourceResolver` to `ADKExecutor` and expose
`WithADKSubagentRetrySourceResolver`.

- [x] **Step 2: Parse retry command**

Parse `command.subagent_retry` and require schema
`coze.subagent_retry.v1`, positive `source_run_id`, and positive
`parent_run_id`.

- [x] **Step 3: Resolve and validate source**

Call the resolver and verify source run ID, thread ID, parent run ID, and
`run_kind='subagent'`.

- [x] **Step 4: Parse source replay payloads**

Parse source child input schema `coze.subagent_tool_call.v1`, requiring a
non-empty tool name and valid JSON arguments. Parse source child config for
`agent_name` and `full_chat_history`; if `agent_name` is present, require it
to match the input tool name.

- [x] **Step 5: Invoke Eino AgentTool**

Build the child agent through the existing factory, wrap it in
`adk.NewAgentTool`, pass `adk.WithFullChatHistoryAsInput()` when the source
config requests it, invoke the tool with stored arguments, and return the
result text.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-executor-replay-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-executor-replay-m3.md`

- [x] **Step 1: Document ADK replay boundary**

State that ADKExecutor must load source child runs through a resolver, validate
identity, parse internal schemas, and use Eino AgentTool instead of hand-rolled
message conversion.

- [x] **Step 2: Update roadmap**

Add M3.31 completion status and reduce the remaining task/person-month
estimate by one backend replay slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused backend test**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestADKExecutorReplaysSubagentRetryFromSourceChildRun' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run broader affected backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestADKExecutorReplaysSubagentRetryFromSourceChildRun|TestApplicationADKSubagentRunRecorder|TestADKSubagentToolProvider|TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor|TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
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
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-executor-replay-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-executor-replay-m3.md
```
