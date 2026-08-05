# Automatic Execution Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the user-facing execution-mode selector, submit `requested_policy=auto`, and keep only the internal Pro and Ultra execution profiles for new Workbench runs.

**Architecture:** The frontend sends one automatic policy and no capability booleans. Backend normalization resolves `auto` to the existing superset execution profile: the lead Agent may answer directly, create a plan only for multi-step work, and use subagents only for genuinely decomposable complex work. Explicit `pro` and `ultra` remain backend-only overrides; Flash and Thinking are removed from the new-run contract.

**Tech Stack:** React, TypeScript, Vitest, Go, Hertz, Eino ADK

---

### Task 1: Lock the backend policy contract

**Files:**

- Modify: `backend/application/agentthread/runtime_config_test.go`
- Modify: `backend/application/agentthread/journal_feature_gate_test.go`

- [ ] **Step 1: Write failing runtime-config tests**

Cover these exact cases:

```go
func TestNormalizeNewDeerFlowRunConfigResolvesAutoPolicy(t *testing.T) {
    normalized, config, err := normalizeNewDeerFlowRunConfig(
        `{"requested_policy":"auto"}`,
        RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true},
    )
    require.NoError(t, err)
    require.Equal(t, DeerFlowRequestedPolicyAuto, config.RequestedPolicy)
    require.Equal(t, DeerFlowModeUltra, config.Mode)
    require.True(t, config.IsPlanMode)
    require.True(t, config.SubagentEnabled)
    require.JSONEq(t, `{"runtime":"eino_adk","requested_policy":"auto","mode":"ultra","thinking_enabled":true,"reasoning_effort":"high","is_plan_mode":true,"subagent_enabled":true,"max_concurrent_subagents":3}`, normalized)
}
```

Also assert that explicit `pro` and `ultra` policies resolve to their corresponding internal profiles, while `flash` and `thinking` are rejected for new normalization.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./application/agentthread -run 'TestNormalizeNewDeerFlowRunConfig|TestJournalFeatureGate'
```

Expected: failure because `requested_policy` and `DeerFlowRequestedPolicyAuto` do not exist.

- [ ] **Step 3: Update Journal enrollment tests**

Replace four public modes with automatic, explicit Pro, and explicit Ultra cases. Assert all resolve to a Journal-eligible internal profile and child/Subagent runs remain excluded.

### Task 2: Implement backend normalization

**Files:**

- Modify: `backend/application/agentthread/runtime_config.go`
- Modify: `backend/application/agentthread/adk_contract_test.go`
- Modify: `backend/application/agentthread/adk_agent_factory_test.go`
- Modify: `backend/application/agentthread/adk_lead_prompt_test.go`
- Modify: `backend/application/agentthread/adk_middleware_test.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/application/agentthread/adk_builtin_subagent_test.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder_test.go`
- Modify: `backend/api/handler/coze/langgraph_run_service_test.go`
- Modify: `backend/internal/deerflowparity/contract.go`
- Modify: `backend/internal/deerflowparity/cases.go`
- Modify: `backend/internal/deerflowparity/runner.go`

- [ ] **Step 1: Add the requested-policy type and resolver**

Use this public policy shape:

```go
type DeerFlowRequestedPolicy string

const (
    DeerFlowRequestedPolicyAuto  DeerFlowRequestedPolicy = "auto"
    DeerFlowRequestedPolicyPro   DeerFlowRequestedPolicy = "pro"
    DeerFlowRequestedPolicyUltra DeerFlowRequestedPolicy = "ultra"
)
```

`auto` resolves to the Ultra capability envelope. Existing prompt rules already prevent plans for one straightforward action and prevent subagent delegation for trivial work, so the Agent performs the task-level decision without a second classifier call.

- [ ] **Step 2: Remove Flash and Thinking from new-run parsing**

Keep only `pro` and `ultra` as internal `DeerFlowMode` values. Normalize a missing policy to `auto`; preserve explicit backend-only `pro` and `ultra`; reject removed modes.

- [ ] **Step 3: Persist both requested and resolved facts**

Normalized config must retain:

```json
{
  "requested_policy": "auto",
  "mode": "ultra"
}
```

The existing `mode` field remains the resolved execution profile used by retries, recovery, capability projection, and Journal enrollment.

- [ ] **Step 4: Update backend tests and parity fixtures**

Remove Flash/Thinking-only fixtures and aliases. Keep direct-answer coverage under automatic policy so simple questions still prove that no plan ceremony is required.

- [ ] **Step 5: Run focused backend tests and verify GREEN**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./application/agentthread ./api/handler/coze ./internal/deerflowparity
```

Expected: PASS.

### Task 3: Remove the frontend mode surface

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/types.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer-controls.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-deerflow-mode-icons.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/__tests__/workbench-composer-contract.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/__tests__/workbench-async-scope.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/__tests__/workbench-final-scope.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx`

- [ ] **Step 1: Write failing frontend contract tests**

Assert that the composer has no `选择模式` control and that run config contains:

```ts
expect(createWorkbenchRunConfig(payload)).toMatchObject({
  requested_policy: 'auto',
});
expect(createWorkbenchRunConfig(payload)).not.toHaveProperty('mode');
expect(createWorkbenchRunConfig(payload)).not.toHaveProperty(
  'thinking_enabled',
);
expect(createWorkbenchRunConfig(payload)).not.toHaveProperty('is_plan_mode');
expect(createWorkbenchRunConfig(payload)).not.toHaveProperty(
  'subagent_enabled',
);
```

- [ ] **Step 2: Run tests and verify RED**

Run the Workbench Vitest files. Expected: the selector is still rendered and mode fields are still serialized.

- [ ] **Step 3: Remove mode state, props, menus, icons, and CSS**

Keep model selection, extensions, runtime settings, attachments, context selection, and send/stop behavior unchanged. Remove only execution-mode presentation and mode-derived request fields.

- [ ] **Step 4: Submit the automatic policy**

The frontend run config must send only `requested_policy: 'auto'` for execution policy; backend owns the resolved mode and capability fields.

- [ ] **Step 5: Run focused frontend tests and verify GREEN**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/workbench/components/__tests__/workbench-composer-contract.test.tsx src/pages/workbench/components/__tests__/workbench-async-scope.test.tsx src/pages/workbench/components/__tests__/workbench-final-scope.test.tsx src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx
```

Expected: PASS.

### Task 4: Document and verify the production boundary

**Files:**

- Modify: `docs/superpowers/context/workbench-chat.md`

- [ ] **Step 1: Update the current product facts**

Document that Workbench sends `requested_policy=auto`, Pro/Ultra are internal profiles, and simple direct answers remain Journal-free even though auto has the full capability envelope.

- [ ] **Step 2: Run static searches**

Verify no Workbench UI references `flash`, `thinking`, `选择模式`, or mode-selector classes, and no backend production code accepts Flash/Thinking as new-run modes.

- [ ] **Step 3: Run final verification**

Run focused Go tests, frontend Vitest, TypeScript checking for the app, and inspect the final diff for unrelated behavior changes.
