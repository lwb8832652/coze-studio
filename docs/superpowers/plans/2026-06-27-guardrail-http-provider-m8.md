# Guardrail HTTP Provider M8

## Objective

Add a production-shaped external HTTP Guardrail scanner adapter for runtime
tools, subagent tools, and Skill content loading while keeping Coze Studio as
the policy boundary and system of record.

## Scope

- Add disabled-by-default `HTTPGuardrailProvider`.
- Load it from explicit env config only when
  `AGENT_GUARDRAIL_PROVIDER_TYPE=http`.
- Validate endpoint scheme and reject URL userinfo.
- Send only metadata-safe Guardrail scan requests with schema
  `coze.guardrail_scan_request.v1`.
- Accept only external actions `warn`, `confirm`, and `deny`; default/no-match
  allow remains Coze-owned.
- Reuse `GuardrailEnforcer`, shared decision sanitization, fail-mode handling,
  and existing audit/interrupt paths.
- Keep outbound requests, scanner decisions, errors, and docs free of prompts,
  model text, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, and client-controlled identity.

## Implemented

- `AGENT_GUARDRAIL_HTTP_URL`
- `AGENT_GUARDRAIL_HTTP_TOKEN`
- `AGENT_GUARDRAIL_HTTP_TIMEOUT_MS`
- `HTTPGuardrailProviderOptions`
- `NewHTTPGuardrailProvider`
- `NewGuardrailProviderFromEnvWithStatus` support for provider type `http`
- Docker env example entries for local and container development.

## Verification

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestHTTPGuardrailProvider|TestGuardrailProviderFromEnv.*HTTP'
go test -count=1 ./application/agentthread
go test -count=1 ./application
cd ..
git diff --check
```
