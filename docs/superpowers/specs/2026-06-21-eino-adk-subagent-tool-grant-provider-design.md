# Eino ADK Subagent Tool Grant Provider Design

## Goal

M3.9 adds the durable-grant entry point for child agent tools. It connects
SingleAgent-backed subagent resolution to the `tool_policy` allow-list
boundary introduced in M3.7.

This slice does not implement the final Skill/MCP settings store. It defines
the interface that durable AgentHub, Skill, MCP, web, filesystem, and
human-interaction configuration must implement.

## Grant Interface

`ADKSubagentToolGrantProvider` resolves authorized tool names for one child
agent:

```go
type ADKSubagentToolGrant struct {
    AllowedTools        []string
    AllowedDynamicTools []string
}
```

The request includes:

- parent run summary;
- normalized subagent reference;
- resolved subagent definition;
- loaded SingleAgent draft/version snapshot.

The provider returns names only. It does not construct Eino tools, invoke MCP,
load Skill content, or decide model visibility directly. Tool construction and
execution remain behind the existing Tool Registry, MCP adapter, and
`ADKToolPolicyProvider`.

## Merge Semantics

When no grant provider is configured, temporary `subagents` /
`subagent_refs` allow-lists keep working for contract tests.

When a grant provider is configured:

- if the reference does not request tools, the durable grant becomes the final
  allow-list;
- if the reference requests tools, the final allow-list is the intersection of
  requested names and durable grants;
- if the grant provider returns no tools, the child remains deny-all.

This prevents temporary run config from broadening durable permissions.

## Deferred Work

Open follow-up work:

- implement a production grant provider backed by AgentHub / SingleAgent
  versioned settings;
- map Skill publications and MCP server tool definitions into stable tool
  names;
- persist grant snapshots on child run records;
- emit content-free audit events for denied grants;
- intersect grants with tenant, user, model, sandbox, and workspace policies.
