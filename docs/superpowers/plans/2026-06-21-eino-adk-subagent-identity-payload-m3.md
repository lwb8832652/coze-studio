# M3.4 Eino ADK Subagent Identity Payload Plan

## Checklist

- [x] Inspect existing ADK event and usage attribution behavior.
- [x] Add red tests for nested subagent message events and runtime errors.
- [x] Add a standard `subagent` object to nested ADK event payloads.
- [x] Add matching event-side token usage metadata.
- [x] Preserve existing callback usage attribution and duplicate suppression.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestMapADKEventMapsAssistantMessage|TestMapADKEventAddsSubagentIdentityToRuntimeErrors|TestADKUsage' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
