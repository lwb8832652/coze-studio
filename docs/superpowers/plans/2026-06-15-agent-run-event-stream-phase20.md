# Agent Run Event Stream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose persisted Go-native agent run events to the task detail page through JSON history and SSE streaming.

**Architecture:** Reuse the Phase 18/19 `agent_run_events` table as the source of truth. Add workbench task-thread APIs for listing run events and streaming new events, then map those events into the existing task detail execution panel. The stream is polling-backed in this phase so it does not introduce a new event bus dependency before Tool Registry/MCP work begins.

**Tech Stack:** Go, Hertz, `github.com/hertz-contrib/sse`, React 18, TypeScript, Vitest.

---

## Scope

- Add `GET /api/workbench/task_threads/:thread_id/run_events`.
- Add `GET /api/workbench/task_threads/:thread_id/run_events/stream`.
- Add frontend API schema/service exports for run events.
- Load historical run events for canonical task-thread details.
- Subscribe to SSE run events for canonical task-thread details and merge incremental events without duplicates.
- Keep legacy `/tasks/:task_id` event loading unchanged.

## Non-Goals

- No WebSocket transport.
- No event bus fanout.
- No MCP/tool/skill runtime work.
- No LangGraph compatibility API.
- No IM channel work.

## Event Stream Contract

- SSE event type: `run.event`
- SSE event ID: run event ID
- SSE event data: JSON `TaskThreadRunEvent`
- SSE done event: `done`, only when a requested `run_id` reaches a terminal state.
- Query params:
  - `run_id`: optional run filter.
  - `after_event_id`: optional client cursor.
  - `interval_ms`: optional polling interval, clamped server-side.
  - `timeout_ms`: optional max stream duration, clamped server-side.

## Testing

- Backend handler tests verify JSON history and SSE formatting.
- Router tests verify both new routes are registered.
- Frontend service tests verify the API export exists.
- Frontend detail tests verify canonical threads render run events and subscribe through EventSource.
- Helper tests verify Phase 19 event types render as human-readable execution steps.
