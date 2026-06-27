# Eino ADK Subagent Tool Provider Design

## Goal

M3.1 starts the Subagents and Custom Agents milestone by adding an adapter
boundary from Coze-owned Agent definitions to Eino `AgentTool`.

The production system of record remains the existing SingleAgent control plane:

- `single_agent_draft`
- `single_agent_version`
- `single_agent_publish`
- existing tool, workflow, knowledge, database, prompt, model, and variable
  configuration attached to those records.

This slice does not create a parallel Agent table. It creates the runtime
interface that future AgentHub/SingleAgent providers can implement.

## Contracts

`ADKSubagentDefinitionProvider` resolves child-agent definitions for a parent
run. A definition contains the stable Coze identity, Eino-safe tool name,
description, version/draft selector, and whether the child receives full chat
history.

`ADKSubagentAgentFactory` builds the Eino `adk.Agent` for one definition. In
production this factory will load SingleAgent draft/version state, apply tool,
skill, MCP, model, memory, sandbox, and policy intersections, and then build a
child `ChatModelAgent` or DeepAgent.

`ADKSubagentToolProvider` preserves tools from an existing provider and appends
Eino `AgentTool` instances for resolved child agents.

## Temporary Run Config Provider

For contract tests and early internal configuration, `subagents` may be read
from run config:

```json
{
  "subagents": [
    {
      "name": "researcher",
      "description": "Research public information for the task.",
      "agent_id": 1001,
      "version": "v1",
      "is_draft": false,
      "full_chat_history": false
    }
  ]
}
```

This is not a durable product API. The durable AgentHub provider replaces it
when Agent management UI and permissions are wired.

## Safety Rules

- Tool names must be Eino-safe: `[A-Za-z_][A-Za-z0-9_]{0,63}`.
- Names must be unique across a parent run's subagents.
- Definitions must have non-empty descriptions because Eino `AgentTool.Info`
  requires them.
- Child tools are built through Eino `adk.NewAgentTool`; do not implement a
  second custom delegation loop.
- Parent and child authorization, allowed-tool intersection, quotas, tracing,
  token attribution, cancellation, and durable parent-child run records remain
  Coze-owned follow-up work.
