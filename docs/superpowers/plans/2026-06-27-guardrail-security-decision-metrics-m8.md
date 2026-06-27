# Guardrail Security Decision Metrics M8

## Objective

Add a Coze-owned Guardrail metrics boundary for security decisions without
introducing a parallel OpenTelemetry or Prometheus implementation in the
Guardrail runtime code.

## Scope

- Add `GuardrailMetricsCollector`.
- Emit one `GuardrailEvaluationMetricsObservation` from `GuardrailEnforcer`
  for every runtime decision, including audit failures.
- Keep observations content-free and low-cardinality.
- Add optional logging metrics through
  `AGENT_GUARDRAIL_METRICS_LOG_ENABLED=false` by default.
- Wire the env-built collector through `NewGuardrailEnforcerFromEnv`.

## Safe Observation Fields

- target type
- operation
- source
- fail mode
- action
- provider
- sanitized error code
- allowed, warning, requires-confirmation flags
- audit-recorded flag
- elapsed milliseconds

## Excluded Fields

- space, thread, run, and user IDs
- target IDs
- reason codes and rule IDs
- decision messages
- prompts, model input/output
- tool arguments/results
- object URIs, raw URLs, filenames
- credentials
- checkpoint bytes
- scanner raw payloads and provider raw bodies
- hidden run config
- raw metadata JSON
- raw repository/provider errors

## Follow-Up Work

- Prometheus exporter
- OpenTelemetry metrics and trace linkage
- Guardrail audit archive/export jobs
- Legal-hold policy
- Policy UI
- Operator runbooks

## Verification

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailEnforcer.*Metrics|TestGuardrailMetricsCollectorFromEnv'
go test -count=1 ./application/agentthread -run 'TestGuardrailEnforcer|TestGuardrailEnforcerFromEnv'
go test -count=1 ./application/agentthread
cd ..
git diff --check
```
