# M3.8 Eino ADK Tool Policy Factory Contract Plan

## Checklist

- [x] Inspect factory tool flow into model options and middleware input.
- [x] Add contract test for policy-filtered static tools reaching both model
      visibility and middleware.
- [x] Add contract test for policy-filtered dynamic tools reaching middleware.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKAgentFactoryUsesPolicyFilteredToolSetForModelAndMiddleware|TestADKToolPolicyProvider' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
