# MCP Runtime Adapter Boundary Design

## Goal

M6.4 connects the metadata-only Workbench MCP registry to the Eino ADK runtime
tool catalog boundary without enabling real MCP transport execution yet. The
slice gives ADK policy and deferred tool resolution a stable MCP tool name
source while keeping MCP config, auth, schemas, arguments, results, sessions,
and provider payloads outside the model-visible contract.

## Runtime Catalog Boundary

`ADKMCPRuntimeToolCatalog` reads safe registry rows through an injected
`ADKMCPToolRegistry` interface. `mcptool.ApplicationService` implements this
interface with `ListMCPToolRegistryEntries(ctx, spaceID)`, which returns the
same safe metadata used by the public registry endpoint:

- stable Eino-safe name such as `mcp_{server_id}_{sanitized_tool_name}`;
- source/category/visibility metadata;
- server/tool identity;
- description;
- enabled state and bounded health metadata.

The runtime adapter converts only enabled `source="mcp"` entries with non-empty
descriptions and Eino-safe names. It does not copy `config`, `auth`,
`input_schema`, URLs, filenames, object keys, tool arguments/results, provider
raw payloads, prompt/model text, transcript content, or checkpoint bytes into
`ADKRuntimeToolDefinition`.

## Run Configuration

MCP runtime exposure is disabled by default. A run must explicitly configure:

```json
{
  "mcp_tools": {
    "enabled": true,
    "allowed_tools": ["mcp_100_search_docs"]
  }
}
```

`mcpTools` is accepted as the camelCase equivalent. `visibility` may be
`static` or `deferred`; the default is `deferred` so future large catalogs can
flow through Eino deferred tool search. `allowed_tools` / `allowedTools` is an
optional adapter-local filter. The existing outer `tool_policy` provider still
applies after catalog construction and remains the common runtime allow-list
boundary.

## Execution Behavior

M6.4 is intentionally non-executable. The generated Eino tool has safe metadata
only and an invoker that fails closed with
`mcp runtime tool execution is not enabled: <tool_name>`. The error may include
the safe tool name, but must not include arguments, server names, config, auth,
schemas, URLs, provider payloads, or raw MCP responses.

Real MCP invocation must be added later behind Coze-owned transport/session,
OAuth/secret, stdio sandbox, timeout/output-budget, authorization, audit, and
health layers before the Eino MCP adapter is allowed to execute calls.

## Wiring

The default ADK runtime catalog is now a composite catalog. It combines web
tools and optional MCP tools behind the existing `ADKRuntimeToolCatalogProvider`,
so model-visible tools, executable tools, middleware static tools, middleware
dynamic tools, and policy filtering continue to share one provider path.

Production bootstrap injects `primaryServices.mcpToolSVC` through
`WithDefaultADKToolProviderMCPRegistry`. Child SingleAgent factories still use
the default non-recursive provider and do not inherit the parent MCP registry
implicitly.

## Non-Goals

- No real MCP stdio/SSE/streamable HTTP transport.
- No OAuth/session lifecycle.
- No input schema exposure to the model.
- No MCP tool execution from ADK.
- No unified Tool Registry table.
- No new frontend surface in this slice.

## Future Work

Next M6 slices should replace the unsupported invoker with a Coze-owned MCP
transport/session executor that uses Eino MCP tool conversion internally, adds
authorization and audit records, enforces timeout/output budgets, handles
health failures, and keeps all model-visible, executable, audit, and UI grant
names aligned with the registry naming adapter.
