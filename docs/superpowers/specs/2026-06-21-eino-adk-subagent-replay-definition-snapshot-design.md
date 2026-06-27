# Eino ADK Subagent Replay Definition Snapshot Design

## Goal

M3.30 makes durable child subagent rows sufficient for a future retry replay
executor to rebuild the original child call under the same definition and tool
grant boundary. M3.28 stored the original `AgentTool` invocation arguments; the
child run config must also preserve the definition fields needed to rebuild the
same subagent tool.

## Stored Config

`ApplicationADKSubagentRunRecorder` writes child run `config` with:

```json
{
  "runtime": "eino_adk",
  "agent_name": "researcher",
  "agent_description": "Research public information.",
  "single_agent": {
    "agent_id": 1001,
    "version": "v1",
    "is_draft": false
  },
  "full_chat_history": true,
  "tool_policy": {
    "allowed_tools": ["knowledge_lookup"],
    "allowed_dynamic_tools": ["web_search"]
  }
}
```

`allowed_tools` and `allowed_dynamic_tools` are normalized string slices. Empty
grants are encoded as empty arrays, not omitted, so retry replay cannot broaden
permissions by falling back to parent tools.

## Runtime Boundary

This config is an internal execution snapshot. The Workbench API redaction
introduced in M3.28 continues to hide child `config`, `input`, `command`, and
`context` from task detail clients. UI child cards should keep using safe
metadata, lifecycle events, and usage summaries.

## Follow-Up

The future ADKExecutor retry implementation should parse the child run config
into `ADKSubagentDefinition`, parse child run input
`coze.subagent_tool_call.v1`, rebuild the child AgentTool with policy, and
invoke it through the retry-capable executor dispatch added in M3.29.
