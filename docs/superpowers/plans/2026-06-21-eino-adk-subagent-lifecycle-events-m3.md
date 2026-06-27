# M3.12 Eino ADK Subagent Lifecycle Events Plan

## Checklist

- [x] Add red tests for successful subagent lifecycle events.
- [x] Add red tests for failed subagent lifecycle events.
- [x] Ensure lifecycle payloads do not include arguments or model output.
- [x] Add optional event sink wiring to `ADKSubagentToolProvider`.
- [x] Wrap Eino `AgentTool` invocation with lifecycle event emission.
- [x] Sanitize Eino-wrapped failure messages for event payloads.
- [x] Pass the production ADK event sink through the default subagent-aware
      tool provider.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused, application, and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSubagentToolProviderEmits.*Lifecycle|TestDefaultADKToolProviderWithSingleAgentSubagents' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
