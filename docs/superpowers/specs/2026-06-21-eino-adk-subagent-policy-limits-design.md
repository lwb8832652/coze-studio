# Eino ADK Subagent Policy Limits Design

## Goal

M3.5 adds provider-level guardrails for Eino ADK subagents before durable
parallel child-run scheduling is introduced. The guardrails prevent accidental
tool explosion and recursive delegation loops at the `ADKSubagentToolProvider`
boundary.

## Policy Source

`ADKSubagentToolProvider` reads policy from the active run config:

```json
{
  "subagent_policy": {
    "max_subagents": 16,
    "max_depth": 2
  }
}
```

Camel-case keys are also accepted: `subagentPolicy`, `maxSubagents`,
`maxDepth`.

Defaults:

- `max_subagents = 16`
- `max_depth = 2`

The provider checks limits after resolving definitions and before building any
Eino `AgentTool`, so rejected runs do not instantiate child agents.

## Depth Input

Child ADK run metadata now includes the same standardized subagent identity
payload used by events:

```json
{
  "source": "single_agent_subagent",
  "agent_id": 1002,
  "version": "v4",
  "is_draft": false,
  "subagent": {
    "name": "writer",
    "root_name": "lead",
    "parent_name": "lead",
    "step_id": "lead/writer",
    "run_path": ["lead", "writer"],
    "depth": 1
  }
}
```

Nested child agents append their name to the parent metadata run path and
increment `depth`.

## Deferred Work

These limits are not a full scheduler. Open work remains:

- concurrent subagent worker pools and per-run semaphores;
- wall-clock timeout and cancellation propagation;
- durable child run lifecycle rows;
- parent-child token/cost rollups;
- per-child tool, skill, MCP, memory, and sandbox policy intersections.
