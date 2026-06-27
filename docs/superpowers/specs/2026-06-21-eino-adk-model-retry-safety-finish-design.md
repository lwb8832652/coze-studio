# Eino ADK Model Retry And Safety Finish Design

## Goal

Use Eino ADK model retry primitives for production task runs and add Coze-owned model finish/error classification without creating a second model reliability loop.

## Scope For M2.13a

- Parse explicit run config for model retry.
- Pass `adk.ModelRetryConfig` into `adk.ChatModelAgentConfig`.
- Keep retry disabled by default.
- Classify safety/content-filter finish reasons in ADK message events.
- Preserve existing `WillRetryError` and `RetryExhaustedError` event mapping.

## Deferred To M2.13b

- Real failover model selection.
- Provider policy and model catalog candidate ranking.
- Per-provider transient/permanent error taxonomy.
- Pricing and quota-aware failover.

Failover must not be wired to a fake or implicit fallback. It needs a Coze-owned provider boundary that returns authorized, tenant-scoped model candidates.

## Run Config

Retry is configured under `model_retry` or `modelRetry`:

```json
{
  "model_retry": {
    "max_retries": 2,
    "backoff_ms": 0,
    "retry_empty_output": true,
    "retry_finish_reasons": ["length"]
  }
}
```

Rules:

- Missing config disables retry.
- `max_retries` must be between 1 and 5.
- `backoff_ms` must be between 0 and 60000.
- Model errors are retryable unless they are context cancellation, ADK cancellation, interrupt, stream cancellation, or safety-classified finish rewrite.
- Empty assistant output retries only when `retry_empty_output` is true.
- Configured finish reasons retry through Eino `ShouldRetry`.

## Safety Finish Classification

Safety finish reasons are normalized case-insensitively:

- `content_filter`
- `safety`
- `blocked`
- `prohibited_content`
- `recitation`

When a model message has a safety finish reason, `MapADKEvent` emits `model.safety_finish` instead of `message.completed`, keeps `FinalText` unchanged, and adds:

```json
{
  "finish_classification": {
    "category": "safety",
    "reason": "content_filter",
    "safety": true,
    "terminal": true
  }
}
```

## Test Gates

- Retry config parser tests.
- Agent factory retry success and exhaustion tests.
- Safety finish event mapping tests.
- Focused package test:

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
