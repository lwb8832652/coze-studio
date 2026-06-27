# Eino ADK SingleAgent Snapshot Tool Grant Design

## Goal

M3.10 adds the first production-facing grant adapter for SingleAgent-backed
subagents. It reads the resolved SingleAgent draft/version snapshot and maps
configured plugin and workflow entries into stable tool-policy names.

This slice still does not construct Eino tools. It only produces the allowed
tool names consumed by `ADKSubagentToolGrantProvider` and then intersected by
`ADKToolPolicyProvider`.

## Mapping Rules

`ADKSingleAgentSnapshotToolGrantProvider` reads:

- `PluginInfo.PluginId`
- `PluginInfo.ApiId`
- `PluginInfo.ApiName`
- `WorkflowInfo.WorkflowId`
- `WorkflowInfo.WorkflowName`

Plugin entries map as follows:

- use `PluginInfo.ApiName` when it is already a valid Eino/Coze tool policy
  name;
- otherwise use the deterministic fallback `plugin_{plugin_id}_{api_id}`;
- skip incomplete plugin entries without both positive IDs.

Workflow entries map as follows:

- use `WorkflowInfo.WorkflowName` when it is already a valid Eino/Coze tool
  policy name;
- otherwise use the deterministic fallback `workflow_{workflow_id}`;
- skip incomplete workflow entries without a positive workflow ID.

Returned names are normalized with the existing config string normalization so
duplicates and empty names do not reach the child run policy.

## Boundary

The provider returns names only:

- no Eino `tool.BaseTool` construction;
- no MCP connection or invocation;
- no Skill content loading;
- no model-visible tool filtering decision;
- no tenant or user policy broadening.

The future Tool Registry and MCP adapter must reuse the same naming helpers so
the grant name, model-visible name, executable tool name, audit event, and UI
grant display all refer to one stable identifier.

## Deferred Work

Open follow-up work:

- wire this provider into the production SingleAgent subagent definition
  resolver once durable AgentHub settings are available;
- add Tool Registry contract tests proving plugin/workflow executable names
  match these grant names;
- persist the resolved grant snapshot on child run records;
- add denied-grant audit events without exposing tool payloads;
- extend grant mapping for Skill and MCP publications.
