# M3.32 Eino ADK Subagent Retry Runtime Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route `subagent_retry` execution through RuntimeSelector so
production workers can reach ADKExecutor replay while unsupported runtimes keep
the fixed fail-closed contract.

**Architecture:** RuntimeSelector implements `SubagentRetryRunExecutor` and
uses the same runtime policy selection as ordinary execution. RunProcessor
recognizes `SubagentRetryUnsupportedError` and maps it to
`subagent_retry_not_supported`.

**Tech Stack:** Go, Coze agentthread runtime selector, ADK executor wiring.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/application/agentthread/runtime_selector_test.go`
- Modify: `backend/application/agentthread/runner_test.go`
- Modify: `backend/application/agentthread/adk_executor_test.go`

- [x] **Step 1: Assert selector ADK retry route**

Add `TestRuntimeSelectorRoutesSubagentRetryToConfiguredADKExecutor`. Build a
selector with legacy and ADK fake retry executors, run with
`config={"runtime":"eino_adk"}`, and assert only the ADK retry method is
called.

- [x] **Step 2: Assert unsupported maps to fixed code**

Add `TestRunProcessorMapsUnsupportedSubagentRetryExecutorToFixedFailure` with
an executor returning `SubagentRetryUnsupportedError`. Assert the run fails
with `subagent_retry_not_supported`, not `executor_error`.

- [x] **Step 3: Assert application resolver loads source run**

Add `TestApplicationADKSubagentRetrySourceResolverLoadsSourceRun` and verify
the resolver calls application `GetRun` for `source_run_id`.

- [x] **Step 4: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationADKSubagentRetrySourceResolverLoadsSourceRun|TestRuntimeSelectorRoutesSubagentRetryToConfiguredADKExecutor|TestRunProcessorMapsUnsupportedSubagentRetryExecutorToFixedFailure' -count=1 -gcflags="all=-l -N"
```

Expected red result: selector retry method, unsupported error type, and
application resolver are not defined.

### Task 2: Implement Routing And Unsupported Mapping

**Files:**
- Modify: `backend/application/agentthread/runner.go`
- Modify: `backend/application/agentthread/runtime_selector.go`
- Modify: `backend/application/agentthread/adk_subagent_retry_executor.go`
- Modify: `backend/application/application.go`

- [x] **Step 1: Add unsupported error type**

Add `SubagentRetryUnsupportedError` with default message
`subagent retry executor is not implemented`.

- [x] **Step 2: Map unsupported in RunProcessor**

When a retry-capable executor returns `SubagentRetryUnsupportedError`, fail the
run with fixed code `subagent_retry_not_supported`.

- [x] **Step 3: Route through RuntimeSelector**

Add `RuntimeSelector.ExecuteSubagentRetry`, select runtime with the same policy
as ordinary execution, and call the selected executor only if it implements
`SubagentRetryRunExecutor`.

- [x] **Step 4: Add application resolver**

Add `NewApplicationADKSubagentRetrySourceResolver` and wire it into
`NewADKExecutor` in `backend/application/application.go`.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-runtime-routing-design.md`
- Create: `docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-runtime-routing-m3.md`

- [x] **Step 1: Document selector routing**

State that RuntimeSelector owns retry routing under the same runtime policy and
that unsupported selected runtimes map to fixed fail-closed semantics.

- [x] **Step 2: Update roadmap**

Add M3.32 completion status and reduce the remaining task/person-month
estimate by one runtime routing slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused backend tests**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestApplicationADKSubagentRetrySourceResolverLoadsSourceRun|TestRuntimeSelectorRoutesSubagentRetryToConfiguredADKExecutor|TestRunProcessorMapsUnsupportedSubagentRetryExecutorToFixedFailure' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run broader affected backend tests**

Run:

```bash
cd backend && go test ./application/agentthread ./api/handler/coze ./api/router/coze -run 'TestADKExecutorReplaysSubagentRetryFromSourceChildRun|TestApplicationADKSubagentRetrySourceResolverLoadsSourceRun|TestRuntimeSelectorRoutesSubagentRetryToConfiguredADKExecutor|TestRunProcessorMapsUnsupportedSubagentRetryExecutorToFixedFailure|TestApplicationADKSubagentRunRecorder|TestADKSubagentToolProvider|TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor|TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport|TestApplicationRetrySubagentRun|TestRetryTaskThreadSubagentRun|TestTaskThreadRunToAPIRedactsSubagentInternalPayloads|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"
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
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-eino-adk-subagent-retry-runtime-routing-design.md docs/superpowers/plans/2026-06-21-eino-adk-subagent-retry-runtime-routing-m3.md
```
