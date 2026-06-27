# Guardrail Pattern Provider M8

## Objective

Add the first production-shaped Guardrail scanner provider for runtime tools,
subagent tools, and Skill content loading without introducing a Python sidecar
or a parallel policy engine.

## Scope

- Add a metadata-only `GuardrailPatternProvider`.
- Load it from disabled-by-default env config.
- Fail closed when guardrail env is explicitly enabled but invalid.
- Reuse `ApplicationGuardrailAuditRecorder` and `GuardrailEnforcer`.
- Wire the env-built enforcer into ADK runtime tools, subagent tools, and Skill
  middleware.
- Keep pattern rules and decisions free of prompts, model text, tool
  arguments/results, object URIs, filenames, credentials, checkpoint bytes,
  scanner raw payloads, provider raw bodies, hidden run config, and
  client-controlled identity.

## Implemented

- `AGENT_GUARDRAIL_PROVIDER_TYPE=pattern`
- `AGENT_GUARDRAIL_PATTERN_RULES_JSON`
- `NewGuardrailPatternProvider`
- `NewGuardrailProviderFromEnvWithStatus`
- `NewGuardrailEnforcerFromEnv`
- `ApplicationService.GuardrailProviderStatus`
- ADK production assembly wiring through
  `WithDefaultADKToolProviderGuardrailEnforcer` and
  `ADKMiddlewareAssemblerOptions.GuardrailEnforcer`.

## Verification

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailPatternProvider|TestGuardrailEnforcerFromEnv|TestDefaultADKToolProviderCanWireGuardrailRuntimeToolWrapper|TestADKSkillMiddlewarePassesGuardrailEnforcerToBackend|TestGuardrailEnforcer'
go test -count=1 ./application
```
