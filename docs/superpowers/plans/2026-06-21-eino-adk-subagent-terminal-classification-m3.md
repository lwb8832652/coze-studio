# M3.15 Eino ADK Subagent Terminal Classification Plan

## Checklist

- [x] Add subagent provider tests for timeout classification and lifecycle
      event payloads.
- [x] Add subagent provider tests for cancellation classification and
      `subagent.run.canceled`.
- [x] Add application recorder tests for canceled child runs and explicit
      error codes.
- [x] Add shared classification helper for child invocation terminal errors.
- [x] Pass terminal status and error code from lifecycle wrapper to child run
      recorder.
- [x] Update `ApplicationADKSubagentRunRecorder` to call `CancelRun` for
      canceled child runs.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSubagentToolProviderAppliesTimeoutPolicy|TestADKSubagentToolProviderClassifiesCanceledChildRun|TestApplicationADKSubagentRunRecorder' \
  -count=1
```

Final package-level verification is tracked from the active implementation
session before handoff.
