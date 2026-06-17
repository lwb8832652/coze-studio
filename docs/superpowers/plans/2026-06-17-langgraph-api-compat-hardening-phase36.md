# Phase 36 - LangGraph API Compatibility Hardening

## Goal

Harden the LangGraph-compatible run adapter after Phase 28-35 so common SDK-shaped payloads do not fail at the HTTP binding layer.

## Scope

- Accept flexible `input` JSON values for run creation, including object, array, string, number, and boolean values.
- Accept `stream_mode` as either a string or a string array, then persist it as a normalized JSON string array.
- Keep missing `input` rejected, while allowing an explicit empty object input.
- Map internal terminal run statuses to LangGraph-facing status names:
  - `succeeded` -> `success`
  - `failed` -> `error`
  - `canceled` -> `interrupted`
- Accept compatible status filters on thread-bound run list.

## Out Of Scope

- Eino runtime execution changes.
- Worker scheduling or run state-machine changes.
- IM channels.
- MCP, skills, memory, token usage, artifacts, or security scanning.
- Mode-specific SSE payload generation such as real `messages`, `updates`, or `values` event fan-out.

## Verification

- Add focused handler tests for flexible input, `stream_mode` normalization, empty object input, compatible status filters, cancel/join/stream terminal status mapping, and stateless terminal status mapping.
- Re-run the focused LangGraph handler regression suite from Phase 29-35.
- Re-run LangGraph router registration tests.
- Run `git diff --check` before committing.
