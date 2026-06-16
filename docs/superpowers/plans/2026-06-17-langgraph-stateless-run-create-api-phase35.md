# Phase 35 - LangGraph Stateless Run Creation API

## Goal

Add LangGraph stateless run creation endpoints:

- `POST /api/runs`
- `POST /api/runs/stream`

These endpoints create a backing task thread automatically, then create a run on that thread.

## Scope

- Bind LangGraph run creation payload without a path thread id.
- Reject requests without `input` before creating any backing thread.
- Build backing thread metadata from request `metadata`.
- Default backing thread title to `新建任务` and source to `api`, matching existing LangGraph thread creation.
- Read `space_id`, `user_id`, and `creator_id` from metadata when present.
- Preserve run fields: `assistant_id`, `input`, `command`, `metadata`, `config`, `context`, `stream_mode`, `multitask_strategy`, `on_disconnect`, and `durability`.
- `POST /api/runs` returns the LangGraph run JSON.
- `POST /api/runs/stream` streams the created run with the existing SSE shape.
- Register routes without redirects.

## Out of Scope

- Stateless thread-less persistence.
- IM channel defaults.
- Eino graph execution changes.
- Token chunk mapping for `messages`.

## Verification

- Red: focused handler tests fail before implementation because stateless create handlers/helpers/models are missing.
- Green: focused handler and router tests pass after implementation.
- Diff hygiene: `git diff --check`.
