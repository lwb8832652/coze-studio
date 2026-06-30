# TD-COMP-005 Weather MCP Runtime Evidence

Date: 2026-06-29

## Scope

Validate that Coze can auto-discover an enabled MCP tool and invoke it through
the real Eino stdio MCP runtime path without requiring a manual per-task MCP
selection.

This note stores only safe summaries. It intentionally omits full prompts,
raw tool arguments/results, cookies, credentials, and provider raw payloads.

## Browser Evidence

- Coze tools page:
  `http://localhost:8080/space/7656275718757679104/tools`
- DOM summary showed the DeerFlow-style MCP server switch list with three
  enabled servers: `weather`, `postgres`, and `github`.
- Coze task detail:
  `http://localhost:8080/space/7656275718757679104/tasks/7656842727241285632`
- The execution feed showed:
  - loading enabled Skills;
  - `tool_search` used to find the matching weather capability;
  - `mcp_7656840243668058112_get_weather` invoked automatically;
  - assistant rendered a concise weather summary for Wuhan.
- Follow-up browser/API smoke on task
  `http://localhost:8080/space/7656275718757679104/tasks/7656972508431646720`
  confirmed the same intent-based path after the mode config fix:
  `tool_search` first, then `mcp_7656840243668058112_get_weather` without
  manual MCP selection. That run surfaced one implementation bug: the model
  tool call used empty MCP arguments even though the user asked for Beijing.
- Post-fix local API/browser smoke on task
  `http://localhost:8080/space/7656275718757679104/tasks/7656986536797274112`
  confirmed the corrected schema propagation on a fresh backend process:
  `tool_search` selected `mcp_7656840243668058112_get_weather`, then the model
  called the MCP tool with `city=北京`. The MCP runtime returned a structured
  weather result including `condition=晴`, `temperature=31`, `humidity=38`,
  `wind=西南风 2级`, and the final assistant answer summarized the same data.

## Runtime Evidence

- Debug env for local validation is configured with:
  - `AGENT_THREAD_MCP_RUNTIME_ENABLED=true`
  - `AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED=false`
  - `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true`
  - `AGENT_THREAD_MCP_STDIO_ALLOWED_COMMANDS="npx,node"`
- The local `weather` MCP server is a stdio MCP fixture imported by
  `DefaultDeerFlowMCPConfigRaw`, using the same durable MCP catalog and ADK MCP
  runtime executor path as other MCP servers.
- Direct catalog inspection for workspace `7656275718757679104` confirmed the
  durable `weather/get_weather` tool has input schema:
  required `city` and optional `unit`.
- Root cause for the empty-argument weather call:
  `MCPToolRegistryEntry` did not carry `input_schema`, so
  `ADKMCPRuntimeToolCatalog` exposed the MCP tool to Eino without
  `ParamsOneOf`. The runtime could execute the tool, but the model had no
  parameter contract and called it with `{}`.
- Fix:
  `input_schema` is now copied from MCP server tool definition to registry
  entry, then into `ADKRuntimeToolDefinition`, so Eino `ToolInfo` receives the
  JSON schema.

## Verification Commands

```bash
cd backend && go test ./application/mcptool -count=1
cd backend && go test ./application/agentthread -run 'TestADKMCPRuntimeBootstrapConfigFromEnvParsesEinoStdio|TestNewADKMCPRuntimeToolExecutorFromConfigBuildsEinoStdio|TestADKMCPRuntimeStdioEinoRunner|TestADKMCPRuntimeToolCatalogTreatsEmptyAllowedToolsAsAutoDiscovery' -count=1
cd frontend/apps/coze-studio && npx vitest run src/pages/workbench/__tests__/workbench.test.tsx
```

Result: all commands passed.

Additional 2026-06-30 schema propagation verification:

```bash
cd backend && go test ./application/agentthread -run 'TestADKMCP|TestDefaultADKToolProviderCanWireMCP' -count=1
cd backend && go test ./application/mcptool -count=1
cd backend && go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestWorkbenchMCPToolHandlers' -count=1
cd frontend/apps/coze-studio && npx vitest run src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Result: all commands passed. The task-detail frontend command still prints an
existing local `localhost:3000` connection warning in one docked-composer test,
but the test suite passes.

Additional 2026-06-30 post-restart runtime smoke:

```text
thread_id=7656986536797274112
run_id=7656986537476751360
result=succeeded
tool_chain=tool_search -> mcp_7656840243668058112_get_weather
mcp_arguments_summary=city=北京
mcp_result_summary=condition=晴, temperature=31, humidity_percent=38, wind=西南风 2级
```

Result: passed. This confirms that empty `mcp_tools.allowed_tools` continues
to mean enabled-catalog auto-discovery and that the model-visible MCP tool
schema now lets the model generate the required `city` argument.

Additional 2026-06-30 composer extension verification:

```bash
cd frontend/apps/coze-studio && npx vitest run src/pages/workbench/__tests__/workbench.test.tsx -t 'extension|默认技能|MCP|拓展|persists extension usage|defaults without the legacy runtime panel|long skill titles'
```

Result: passed, 7 selected tests. This covers DeerFlow-style extension
defaults, disabling default Skill/MCP usage, persisted extension state across
composer remounts, selected-count rendering, and long skill title layout.

## Remaining Checks

- Capture final screenshots for the `/tools` server list and task-detail MCP
  tool step.
- Validate external credential-dependent MCP servers separately:
  - GitHub requires a valid token for non-dry-run calls.
  - Postgres requires a valid database URL and read-only query guard.
