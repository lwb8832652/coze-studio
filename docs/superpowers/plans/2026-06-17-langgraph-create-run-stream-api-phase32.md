# Phase 32 - LangGraph Create Run Stream API

## Goal

Add the thread-bound LangGraph create-and-stream endpoint:

- `POST /api/threads/:thread_id/runs/stream`

This phase lets SDK/front-end clients create a run and immediately consume the same SSE stream shape introduced in Phase 31.

## Scope

- Bind `thread_id` from the path and run creation payload from the JSON body.
- Preserve the existing create-run fields: `assistant_id`, `input`, `command`, `metadata`, `config`, `context`, `stream_mode`, `multitask_strategy`, `on_disconnect`, and `durability`.
- Reject requests without `input` before opening SSE.
- Create the run through the Go `agentthread` application service.
- Stream the created run through the existing LangGraph SSE helper.
- Register `POST /api/threads/:thread_id/runs/stream` without redirects.
- Cover successful create-and-stream, missing input rejection, and route registration with tests.

## Out of Scope

- Stateless `/api/runs/stream`.
- Real worker scheduling changes.
- Eino graph execution changes.
- Token chunk mapping for `messages`.
- Frontend execution-flow consumption.

## Verification

- Red: focused handler tests fail before implementation because `CreateLangGraphRunStream` is missing.
- Green: focused handler and router tests pass after implementation.
- Diff hygiene: `git diff --check`.
