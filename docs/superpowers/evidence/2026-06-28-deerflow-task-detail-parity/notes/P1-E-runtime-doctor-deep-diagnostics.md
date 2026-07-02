# P1-E Runtime Doctor Deep Diagnostics

Date: 2026-07-02

## Scope

This slice expands Coze Runtime Doctor from a shallow configuration check into
a bounded production diagnostic for the Go-native Agent Harness:

- default model configured state;
- optional live model connectivity probe;
- provider capability matrix from the same Agent Harness capability detector;
- sandbox runner policy summary;
- existing Skill, MCP, web fetch, and web search bounded summaries.

## DeerFlow Evidence

N/A for direct UI/API parity. DeerFlow does not expose a matching Runtime Doctor
control-plane diagnostic endpoint or detail-page module in the verified task
detail flow. This slice is Coze production hardening for the Go-native Eino
runtime while preserving the DeerFlow-facing task experience.

## Coze Source Evidence

- API response contract:
  `backend/api/model/workbench/diagnostic/diagnostic.go`
- Runtime Doctor use case:
  `backend/application/workbench/runtime_doctor.go`
- Agent Harness capability source:
  `backend/application/agentthread/adk_agent_factory.go`
- Task detail diagnostic panel:
  `frontend/apps/coze-studio/src/pages/tasks/task-runtime-doctor-section.tsx`

## Data Contract

New bounded response fields:

- `model.status`
- `model.configured`
- `model.live_probe`
- `model.capabilities`
- `sandbox.status`
- `sandbox.runner_type`
- `sandbox.network`
- `sandbox.process`
- `sandbox.ffi`
- `sandbox.node_modules`

The live model probe is opt-in through
`WORKBENCH_RUNTIME_DOCTOR_LIVE_MODEL_PROBE=true`. Probe prompts and provider
responses are never returned to API/UI.

## Safety Boundary

Runtime Doctor must not expose:

- prompt text;
- model completion text;
- tool arguments or tool results;
- checkpoint bytes;
- raw provider body;
- credentials, token values, bearer values, API keys, signed URLs;
- object URI or raw storage path;
- raw sandbox allowlist hosts or local paths.

The sandbox summary reports only coarse states such as `configured` or
`restricted`.

## Verification

Passed:

```bash
cd backend && go test ./application/workbench ./api/handler/coze -run 'TestRuntimeDoctor|TestWorkbenchRuntimeDoctor' -count=1 -gcflags="all=-N -l"
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx
```

