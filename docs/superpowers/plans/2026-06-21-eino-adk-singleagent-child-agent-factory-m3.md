# M3.3 SingleAgent Child ADK Agent Factory Plan

## Checklist

- [x] Confirm the stable SingleAgent snapshot fields that can be mapped without
      introducing a parallel runtime config model.
- [x] Add red tests for building an invokable child ADK agent from a
      SingleAgent version snapshot.
- [x] Implement `ADKSingleAgentSubagentAgentFactory`.
- [x] Map `PromptInfo.Prompt` and stable `ModelInfo` generation parameters into
      the child run config.
- [x] Share the SingleAgent draft/version snapshot loader with the M3.2
      definition provider.
- [x] Ensure the child run does not inherit parent web/tool/runtime config.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSingleAgentSubagentAgentFactory|TestADKSingleAgentSubagentRunSummary|TestADKSingleAgentSubagentDefinitionProvider' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
