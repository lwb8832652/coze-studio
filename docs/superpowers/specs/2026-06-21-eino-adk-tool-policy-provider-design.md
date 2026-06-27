# Eino ADK Tool Policy Provider Design

## Goal

M3.7 adds a Coze-owned tool policy boundary for the Eino ADK runtime. The
boundary prevents child agents from implicitly inheriting every parent tool and
gives future Skill, MCP, web, filesystem, and human-interaction tools one
shared allow-list contract.

## Provider Boundary

`ADKToolPolicyProvider` wraps any `ADKToolProvider`.

Behavior:

- If run config has no `tool_policy` / `toolPolicy`, the wrapped provider is
  returned unchanged.
- If policy is present, static and dynamic tool sets are filtered by tool name.
- An explicit empty allow-list denies all tools for that tool class.
- Tool metadata and invocation behavior are not modified.
- Tool names are resolved through `tool.BaseTool.Info(ctx)` so policy applies
  after every provider has built its real Eino tool objects.

Config:

```json
{
  "tool_policy": {
    "allowed_tools": ["read_file"],
    "allowed_dynamic_tools": ["search_docs"]
  }
}
```

Supported keys:

- `allowed_tools` / `allowedTools`: default allow-list for static and dynamic
  tools.
- `allowed_static_tools` / `allowedStaticTools`: static-tool override.
- `allowed_dynamic_tools` / `allowedDynamicTools`: dynamic-tool override.

## Default Runtime Wiring

`NewDefaultADKToolProvider` now wraps the existing default provider with
`ADKToolPolicyProvider`.

Lead runs without a tool policy keep the existing behavior. Child SingleAgent
runs always receive a `tool_policy` in their generated config.

## Child Agent Tool Policy

`ADKSubagentDefinition` and `ADKSubagentReference` now support:

- `AllowedTools`
- `AllowedDynamicTools`

Temporary run-config sources parse:

- `allowed_tools` / `allowedTools`
- `allowed_dynamic_tools` / `allowedDynamicTools`

`ADKSingleAgentSubagentAgentFactory` writes these fields into the child run
config. If no tools are declared, the child run receives explicit empty
allow-lists:

```json
{
  "tool_policy": {
    "allowed_tools": [],
    "allowed_dynamic_tools": []
  }
}
```

This means child agents are deny-all by default until AgentHub or SingleAgent
configuration explicitly grants a tool.

## Deferred Work

This slice does not yet implement durable Skill/MCP authorization. Open work:

- map published Skill and MCP settings into child `AllowedTools`;
- intersect child allow-lists with parent run, tenant, and model policy;
- expose denied-tool diagnostics as content-free audit events;
- add UI controls for child tool grants;
- persist policy snapshots with child run records.
