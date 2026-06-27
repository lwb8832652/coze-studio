# SingleAgent Child ADK Agent Factory Design

## Goal

M3.3 turns a Coze SingleAgent draft/version snapshot into a runnable Eino ADK
child agent. This is the next slice after M3.1 `AgentTool` wiring and M3.2
SingleAgent-backed subagent definition resolution.

The implementation keeps SingleAgent as the durable system of record and uses
Eino ADK only as the execution kernel.

## Runtime Boundary

`ADKSingleAgentSubagentAgentFactory` implements `ADKSubagentAgentFactory`:

1. Normalize the incoming `ADKSubagentDefinition`.
2. Load the SingleAgent draft or version through
   `ADKSingleAgentSubagentService`.
3. Fill missing tool-facing name, description, and version from the snapshot.
4. Build a child `RunSummary` containing only the stable child-agent runtime
   configuration.
5. Delegate construction to an injected `ADKAgentFactory`, normally
   `ApplicationADKAgentFactory`.

This avoids a second agent loop and lets future child agents reuse the same
model, tool, middleware, retry, and capability adapters as lead agents.

## Snapshot Mapping

The first production-safe mapping is intentionally narrow:

- `ADKSubagentDefinition.Name` -> `agent_name`
- `ADKSubagentDefinition.Description` -> `agent_description`
- `SingleAgent.Prompt.Prompt` -> `system_prompt`
- `SingleAgent.ModelInfo.ModelId` -> `model_id`
- `SingleAgent.ModelInfo.Temperature` -> `temperature`
- `SingleAgent.ModelInfo.MaxTokens` -> `max_tokens`
- `SingleAgent.ModelInfo.TopP` -> `top_p`

The child run sets `AssistantID` to `singleagent:{agent_id}` and records
metadata with source `single_agent_subagent`, `agent_id`, `version`, and
`is_draft`.

The child run copies only parent execution identity fields needed by Coze
boundaries today: `RunID`, `PlanScopeRunID`, `ThreadID`, `SpaceID`,
`CreatorID`, and selected stream/durability flags. It does not inherit the
parent run config, input, context, metadata, tool settings, web tool settings,
or other model overrides.

## Deferred Production Work

This slice does not yet create durable child run records. Until that lands,
child ADK usage is still attributed through the active parent run identity.

Open follow-up work:

- parent-child run records and replayable child checkpoints;
- child-specific allowed tool, skill, MCP, model, memory, and sandbox
  intersections;
- child token attribution and cost rollups;
- concurrent subagent quotas, timeout, cancellation, and terminal race
  protection;
- state cards and frontend execution detail views.
