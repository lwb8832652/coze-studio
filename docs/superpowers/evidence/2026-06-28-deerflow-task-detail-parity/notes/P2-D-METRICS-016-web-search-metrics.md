# P2-D-METRICS-016 Web Search Metrics

## Scope

- Implement the safe Web Search metric families from the runtime metrics
  contract:
  - `coze_agent_thread_web_search_calls_total`
  - `coze_agent_thread_web_search_latency_ms`
  - `coze_agent_thread_web_search_results`
- Keep labels bounded to `provider`, `result`, and `error_code`.
- Do not expose search query text, URLs, redirect URLs, hosts, page titles,
  snippets, response bodies, credentials, raw provider payloads, thread IDs,
  run IDs, user IDs, space IDs, or tool arguments.

## Source Verification

- The ADK Web Search path is centralized behind `ADKWebSearchBackend`.
- Provider-specific implementations include Auto, DuckDuckGo, Brave,
  Wikipedia, and HTTP backends.
- `adkWebSearchInvoker` sanitizes returned results after backend execution, so
  metrics must record only aggregate metadata before any UI/API response leaves
  the runtime.
- The metrics contract explicitly requires Web Search metrics to avoid query,
  host, URL, title, snippet, and raw response body labels.

## Implementation

- Added `RuntimeWebSearchMetricsObservation` and
  `RecordRuntimeWebSearch` to the runtime metrics collector interface.
- Added disabled-by-default Prometheus metric families:
  - counter `coze_agent_thread_web_search_calls_total`
  - histogram `coze_agent_thread_web_search_latency_ms`
  - histogram `coze_agent_thread_web_search_results`
- Added `ADKInstrumentedWebSearchBackend`, a thin wrapper that records:
  - provider from configured backend selection;
  - success/failed result;
  - safe error codes (`none`, `search_failed`, `empty_response`);
  - call latency;
  - result count only on successful non-empty backend responses.
- `ADKWebSearchBackendFromEnv` wraps the resolved backend only when
  `AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED` is enabled. A typed-nil
  collector is checked before assigning to the metrics interface so disabled
  metrics do not accidentally wrap or alter provider tests.
- The collector sanitizes invalid labels to bounded fallbacks and clamps
  negative latency/result counts to zero.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestADKWebSearchBackendFromEnvDefaultsToAuto|TestADKWebSearchBackendFromEnvCanUseWikipedia|TestRuntimePrometheusMetricsCollectorRecordsWebSearchSafely|TestADKInstrumentedWebSearchBackendRecordsMetrics|TestADKInstrumentedWebSearchBackendRecordsFailureWithoutQueryLeak' -count=1
go test ./application/agentthread -count=1
```

Both commands passed locally after the implementation.

## Residual Work

- `P2-D-METRICS-017` has since implemented MCP health state aggregation.
- `P2-D-METRICS-018`: model call latency/failure remains open.
