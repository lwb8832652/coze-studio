# Eino ADK Deferred Tool Catalog Design

## Goal

Bridge Coze-owned runtime tool catalog results into Eino ADK dynamic tool
search without making Eino responsible for durable discovery, authorization,
MCP sessions, secrets, policy, or audit.

## Scope For M2.15

- Add a single-call `ADKToolSetProvider` path so static and dynamic tools come
  from the same Coze policy result.
- Add a generic `ADKRuntimeToolCatalog` adapter that converts policy-filtered
  runtime tool definitions into Eino `tool.BaseTool` values.
- Partition tools as visible static tools or deferred dynamic tools.
- Continue using Eino `dynamictool/toolsearch` for keyword search, direct
  selection, provider-native deferred tools, and client-side promotion.
- Preserve existing human-interaction tools and current dynamic tool tests.

## Runtime Boundary

Coze remains the system of record for:

- durable tool and MCP configuration;
- authorization and tenant policy;
- OAuth, encrypted secrets, and session lifecycle;
- stdio/container sandboxing;
- health checks, audit, timeout, and output-budget enforcement.

The M2.15 adapter receives an already policy-filtered catalog snapshot and
turns it into executable Eino tools for a single run. It does not invent tools,
look up unauthorized tools, or bypass Coze policy.

## Provider Shape

`ADKToolSetProvider` returns:

```go
type ADKToolSet struct {
    StaticTools  []tool.BaseTool
    DynamicTools []tool.BaseTool
}
```

`ApplicationADKAgentFactory` prefers `ResolveToolSet` when available. This
prevents separate `ResolveTools` and `ResolveDynamicTools` calls from observing
different catalog/policy snapshots.

## Catalog Tool Definition

A catalog item includes:

- stable tool name;
- description;
- optional JSON Schema input schema;
- visibility: `static` or `deferred`;
- invoker callback.

The adapter validates names, duplicate partitions, schema JSON, and invoker
presence before returning a tool set.

## Deferred Promotion

For client-side search, Eino `toolsearch` initially hides dynamic tools and
promotes selected matches after a `tool_search` result. For provider-native
search, the middleware moves dynamic tools into `DeferredToolInfos`.

The existing Coze tool-definition budget middleware continues to count visible
and deferred tool definitions and rejects oversized promoted tools before the
next model call.

## Deferred

- Durable MCP runtime adapters and stdio/SSE/HTTP transports.
- OAuth, encrypted secrets, session pooling, health checks, and sandboxed
  command execution.
- Tool retry eligibility taxonomy and per-tool policy decision records.
- UI for per-run static/deferred tool placement.
