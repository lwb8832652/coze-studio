# Eino ADK Provider Capability Gate Design

## Goal

M2.17 adds a Coze-owned provider capability gate for the Go-native Eino ADK
runtime. It closes the DeerFlow 2.x parity gap around thinking, reasoning
effort, image input, PDF/file input, audio, and video without introducing a
Python sidecar or model-provider-specific branching in the Agent loop.

The gate fails before provider invocation when a run asks for a feature the
selected model has not declared. It emits content-free events so the frontend
can explain the failure without exposing attachment URLs, base64 data, prompts,
or file names.

## Non-goals

- Do not fetch, parse, OCR, transcode, or summarize files.
- Do not grant filesystem, shell, artifact, or remote URL access.
- Do not replace the existing multimodal history budget.
- Do not implement provider-specific reasoning option APIs inside the generic
  Harness. Concrete model adapters may later project the normalized reasoning
  request to provider-specific options.

## Runtime Contracts

`ADKModelCapabilities` is extended beyond native tool search:

- `Thinking`
- `Reasoning`
- `Vision`
- `PDF`
- `Audio`
- `Video`

Capabilities come from two sources:

- the selected chat model, if it implements the optional
  `ADKProviderCapabilityModel` interface;
- run configuration overrides under `provider_capabilities`,
  `providerCapabilities`, `model_capabilities`, or `modelCapabilities`.

Run configuration may request reasoning behavior with:

- `reasoning_effort` or `reasoningEffort`: `minimal`, `low`, `medium`, or
  `high`;
- `thinking_enabled` or `thinkingEnabled`: boolean.

Requests are valid only when the selected capability is declared. A requested
reasoning effort requires `Reasoning`; thinking requires `Thinking`.

## Message Capability Checks

The middleware inspects the complete model input state before the multimodal
projection middleware runs:

- image parts require `Vision`;
- PDF file parts require `PDF`;
- non-PDF file parts require `File`;
- audio parts require `Audio`;
- video parts require `Video`.

PDF detection is intentionally lightweight: MIME type `application/pdf` or a
case-insensitive `.pdf` name/URL suffix. The check is a capability gate only,
not a file validation or security scanner.

## Event Contract

Unsupported capability failures emit `model.unsupported_capability`:

```json
{
  "capability": "vision",
  "part_type": "image_url",
  "count": 1,
  "error": "provider capability unsupported: vision is required for image_url"
}
```

The payload must not include raw attachment URLs, file names, base64 data,
prompt text, tool arguments, or provider response content.

## Middleware Order

`provider_capability` runs after filesystem tool registration and immediately
before `multimodalbudget`. This keeps Coze's transcript, reduction, audit,
usage, and offload middleware order intact while ensuring the capability gate
sees the original message source state before the provider-specific projection.

## Follow-up Work

- Model adapters can implement `ADKProviderCapabilityModel` based on their
  stable provider feature tables.
- Provider-specific reasoning option projection should live in model adapters,
  not in the Harness loop.
- M4 sandbox/workspace work can add safe file fetching, scanning, parsing, and
  artifact registration behind Coze-owned policy and audit boundaries.
