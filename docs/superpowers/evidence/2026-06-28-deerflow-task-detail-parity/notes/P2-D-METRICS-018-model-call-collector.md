# P2-D-METRICS-018 Model Call Collector

## Scope

Close the remaining P2-D runtime metrics gap for model-call latency and failure
visibility while preserving the DeerFlow parity safety boundary: metrics must
never expose prompts, completions, provider payloads, raw errors, call IDs,
thread/run IDs, checkpoint bytes, credentials, or user content.

## Source Verification

- Existing token usage collection is callback/event based and only records
  persisted token totals. That path can miss provider failures because failed
  calls may not produce token usage.
- ADK runtime model creation flows through `ApplicationADKAgentFactory` and
  `prepareADKChatModelForRun`, where the final `model.BaseChatModel` is handed
  to Eino ADK. This is the safest shared point to observe both `Generate` and
  `Stream` calls without changing message rendering or token usage semantics.
- The runtime metrics contract already uses bounded labels and disabled-by-
  default Prometheus collection via
  `AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED`.

## Implementation

- Added `RuntimeModelCallMetricsObservation` and `RecordRuntimeModelCall` to
  the runtime metrics collector contract.
- Added disabled-by-default Prometheus metrics:
  - `coze_agent_thread_model_calls_total`
  - `coze_agent_thread_model_latency_ms`
- Labels are bounded to `runtime`, `model_family`, `result`, and `error_code`.
  Invalid or unsafe labels fall back to `unknown` or `failed`; provider errors
  are collapsed to `model_failed`.
- Added `runtimeInstrumentedChatModel`, a lightweight wrapper around
  `model.BaseChatModel`, to record `Generate` and `Stream` success/failure and
  latency.
- Wired the wrapper into `prepareADKChatModelForRun` only when runtime
  Prometheus metrics are enabled.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollectorRecordsModelCallSafely|TestRuntimeInstrumentedChatModelRecordsGenerateMetrics|TestRuntimeInstrumentedChatModelRecordsGenerateFailureMetrics|TestRuntimeInstrumentedChatModelRecordsStreamMetrics|TestRuntimeInstrumentedChatModelRecordsStreamFailureMetrics' -count=1
go test ./application/agentthread -count=1
```

Both commands passed locally after the implementation.

## Residual Work

- P2-D now needs a final gap scan before the workstream is marked complete.
