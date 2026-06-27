# SingleAgent-backed ADK Subagent Provider Design

## Goal

M3.2 connects the M3.1 Eino `AgentTool` adapter to Coze Studio's existing
SingleAgent records.

The run config source now references child agents by identity. The provider
loads the child draft/version snapshot through the existing SingleAgent service
and maps the snapshot to `ADKSubagentDefinition`.

## Runtime Shape

Temporary run config:

```json
{
  "subagent_refs": [
    {
      "name": "researcher",
      "agent_id": 1001,
      "version": "v1",
      "is_draft": false,
      "full_chat_history": false
    }
  ]
}
```

`subagentRefs` is accepted as a camelCase alias.

The `name` field is the Eino tool name and must remain stable and Eino-safe.
The child SingleAgent display name remains product UI state and is not used as
the tool name unless it already satisfies the runtime naming rule.

## Source Of Truth

`ADKSingleAgentSubagentDefinitionProvider` depends on a narrow interface:

- `GetSingleAgent(ctx, agentID, version)`
- `GetSingleAgentDraft(ctx, agentID)`

Production wiring should pass the existing SingleAgent domain service. The
provider must not read database tables directly, and it must not copy agent
configuration into a second Agent store.

## Follow-up Work

- Replace temporary run config references with durable AgentHub child-agent
  settings and UI.
- Build child ADK agents from the loaded SingleAgent snapshot, including model,
  skills, tools, MCP, memory, sandbox, policy, and quotas.
- Persist parent-child run relationships and token attribution.
