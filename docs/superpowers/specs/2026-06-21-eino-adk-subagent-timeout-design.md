# Eino ADK Subagent Timeout Design

## Goal

M3.6 adds a Coze-owned timeout wrapper around Eino ADK `AgentTool`
invocation. This gives subagent calls a bounded execution window before the
durable child-run scheduler and cancellation ledger are implemented.

## Runtime Behavior

`ADKSubagentToolProvider` wraps each `adk.NewAgentTool` result with
`adkSubagentPolicyTool` when `ADKSubagentPolicy.Timeout` is positive.

The wrapper:

- preserves `Info(ctx)` from the underlying Eino tool;
- requires the wrapped tool to implement `tool.InvokableTool`;
- calls `InvokableRun` with `context.WithTimeout`;
- propagates the original context cancellation and deadline errors;
- does not inspect or persist tool arguments or results.

## Policy

Timeout policy is read from run config:

```json
{
  "subagent_policy": {
    "timeout_ms": 300000
  }
}
```

Camel-case keys are also accepted: `subagentPolicy.timeoutMs`.

Default:

- `timeout_ms = 300000` (5 minutes)

## Deferred Work

This is invocation-level protection only. Open production work:

- durable child run lifecycle rows;
- parent cancellation propagation across child checkpoints;
- timeout-specific terminal events and state-card UI;
- per-agent and per-tenant concurrency semaphores;
- child run retry and resume semantics.
