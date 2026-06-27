# M3.14 Eino ADK Subagent Child Run Recorder Plan

## Checklist

- [x] Add service test allowing initial `running` status only for subagent
      child runs.
- [x] Add application recorder tests for child run create, complete, and fail.
- [x] Add subagent provider tests proving recorder and lifecycle event payloads
      include `child_run_id`.
- [x] Add recorder-only provider test so child rows do not depend on event
      sink presence.
- [x] Implement `ADKSubagentRunRecorder` and
      `ApplicationADKSubagentRunRecorder`.
- [x] Wire recorder through default subagent-aware ADK tool provider and
      production application initialization.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSubagentToolProvider.*Lifecycle|TestADKSubagentToolProviderRecordsChildRunWithoutEventSink|TestApplicationADKSubagentRunRecorder|TestDefaultADKToolProviderWithSingleAgentSubagents' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" \
  ./domain/agentthread/... ./application/agentthread ./application \
  -count=1
```
