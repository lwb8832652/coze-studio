# M3.28 Eino ADK Subagent Replay Input And API Redaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist child subagent `AgentTool` invocation arguments for future
retry replay while redacting those internal fields from Workbench run APIs.

**Architecture:** The ADK subagent lifecycle wrapper passes Eino tool-call
arguments to the child run recorder. The recorder stores a versioned internal
input payload on the child run. The Workbench API mapper keeps top-level task
runs unchanged and clears internal payload fields for child subagent run
responses.

**Tech Stack:** Go, Eino ADK tool adapters, Coze agentthread application
service, Hertz Workbench API handler tests.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/application/agentthread/adk_subagent_tool_provider_test.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder_test.go`
- Modify: `backend/api/handler/coze/workbench_thread_service_test.go`

- [x] **Step 1: Assert lifecycle forwards tool arguments**

In `TestADKSubagentToolProviderEmitsLifecycleEvents`, invoke the child tool
with `{"request":"find secret context"}` and assert
`recorder.startRequests[0].ArgumentsInJSON` JSON-equals that payload.

- [x] **Step 2: Assert recorder persists replay input**

In `TestApplicationADKSubagentRunRecorderStartsChildRun`, set
`ArgumentsInJSON: {"request":"find context"}` and assert the created child run
input JSON-equals:

```json
{
  "schema": "coze.subagent_tool_call.v1",
  "tool_name": "researcher",
  "arguments": {"request": "find context"}
}
```

- [x] **Step 3: Assert API redacts child internals**

Add `TestTaskThreadRunToAPIRedactsSubagentInternalPayloads` and assert that
`taskThreadRunToAPI` clears `command`, `input`, `config`, and `context` for a
`run_kind='subagent'` row while preserving metadata and error fields. Assert a
top-level `run_kind='task'` row still returns those fields unchanged.

- [x] **Step 4: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze -run 'TestADKSubagentToolProviderEmitsLifecycleEvents|TestApplicationADKSubagentRunRecorderStartsChildRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads' -count=1 -gcflags="all=-l -N"
```

Expected red result: `ArgumentsInJSON` is missing and subagent API fields are
still visible.

### Task 2: Implement Replay Input Persistence

**Files:**
- Modify: `backend/application/agentthread/adk_subagent_run_recorder.go`
- Modify: `backend/application/agentthread/adk_subagent_tool_provider.go`

- [x] **Step 1: Add start request field**

Add `ArgumentsInJSON string` to `ADKSubagentRunStartRequest`.

- [x] **Step 2: Pass invocation arguments**

In `adkSubagentLifecycleTool.InvokableRun`, pass `argumentsInJSON` into
`StartADKSubagentRun`.

- [x] **Step 3: Store versioned child input**

In `ApplicationADKSubagentRunRecorder.StartADKSubagentRun`, validate that
arguments are JSON, default empty arguments to `{}`, and create child run input
with schema `coze.subagent_tool_call.v1`, tool name, and raw JSON arguments.

### Task 3: Redact Subagent API Fields

**Files:**
- Modify: `backend/api/handler/coze/workbench_thread_service.go`

- [x] **Step 1: Redact child internals**

In `taskThreadRunToAPI`, when `run.RunKind == appagentthread.RunKindSubagent`,
clear `command`, `input`, `config`, and `context` before constructing
`TaskThreadRun`.

- [x] **Step 2: Preserve top-level task behavior**

Leave `run_kind='task'` mapping unchanged so task detail headers and follow-up
flows can still read the top-level run input and config.

### Task 4: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-replay-input-redaction-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-replay-input-redaction-m3.md`

- [x] **Step 1: Document internal replay input**

Add the rule that child run input may store `coze.subagent_tool_call.v1` for
future backend replay.

- [x] **Step 2: Document API redaction**

Add the rule that Workbench API responses must redact child subagent
`command`, `input`, `config`, and `context`.

- [x] **Step 3: Update roadmap**

Add M3.28 completion status and reduce the remaining task/person-month
estimate by one backend contract slice.

### Task 5: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze -run 'TestADKSubagentToolProviderEmitsLifecycleEvents|TestApplicationADKSubagentRunRecorderStartsChildRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run broader affected backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestADKSubagentToolProvider|TestApplicationADKSubagentRunRecorder|TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
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
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-replay-input-redaction-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-replay-input-redaction-m3.md
```
