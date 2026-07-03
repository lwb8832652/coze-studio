# P2-B-TOOLS-POLICY-003 Tool policy controls review

## Scope

Review whether P2 needs new visible policy controls for Skills/MCP/Tools before
adding UI. This slice intentionally checks DeerFlow reference behavior and the
current Coze runtime implementation first, then keeps the product surface stable
unless a verified gap exists.

## DeerFlow reference

Relevant reference files:

- `backend/packages/harness/deerflow/skills/tool_policy.py`
- `backend/packages/harness/deerflow/agents/middlewares/deferred_tool_filter_middleware.py`
- `backend/packages/harness/deerflow/config/tool_output_config.py`
- `backend/packages/harness/deerflow/agents/middlewares/tool_output_budget_middleware.py`
- `frontend/src/components/workspace/settings/tool-settings-page.tsx`

Observed behavior:

- Visible settings are centered on MCP server configuration/enabled state.
- Skill `allowed-tools` is a runtime/package policy, not a task-detail chat UI.
- Deferred MCP tools are hidden from model binding until `tool_search` promotes
  them.
- Oversized tool results are budgeted and externalized/truncated at middleware
  level.

## Coze implementation status

Coze already has the equivalent production boundaries:

- `ADKToolPolicyProvider` reads `tool_policy` from run config. Missing policy
  preserves existing behavior; explicit empty allow-lists deny the tool class.
- `ADKMCPRuntimeToolCatalog` reads `mcp_tools.enabled`, `visibility`, and
  `allowed_tools`; empty `allowed_tools` denies all MCP runtime tools.
- `ADKToolDefinitionBudgetMiddleware` enforces a tool-definition token budget
  across visible and deferred tools before model binding.
- `ADKOffloadBackend` + Eino reduction middleware offloads large tool results to
  read-only virtual paths under `.coze/tool-results`; raw object URIs are not
  exposed to Workbench UI/API.
- MCP stdio runtime runner enforces a bounded output byte budget.
- Workbench composer already persists extension usage switches and supports MCP
  auto-discovery without manual per-task selection.

## Decision

Do not add a new visible policy-control panel in this P2 slice. Adding a new UI
would drift from DeerFlow's visible workflow and duplicate existing runtime
contracts. Keep policy controls as run-config/runtime contracts for now, and
surface only metadata-only observability in task detail.

Future UI work is allowed only if a concrete operator workflow is verified,
for example editing organization-level guardrail/tool policy with audit and
approval. That belongs to P2-C Guardrail 管理与审核, not this P2-B slice.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestADKToolPolicyProvider|TestADKToolBudget|TestADKMiddlewareEnablesReadOnlyOffloadAndReduction|TestADKMCPRuntimeToolCatalog|TestADKMCPRuntimeStdioEinoRunner' -count=1
```

Result: passed.
