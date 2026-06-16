# Phase 27: Agent Token Usage API and Task Detail Indicator

## Goal

Expose persisted Go-native Agent Harness token usage to the task thread API and surface a compact token indicator on the task detail page.

This phase keeps the product language task-oriented. It does not rename menus or routes to conversation wording.

## Scope

- Add `GET /api/workbench/task_threads/:thread_id/token_usage`.
- Return usage rows plus aggregate totals for a task thread.
- Support optional `run_id` query filtering while verifying the run belongs to the requested thread.
- Add frontend API schema and task service export.
- Fetch token usage for canonical task-thread detail pages.
- Render a compact token indicator in the task detail top bar.

## Out of Scope

- Pricing tables, currency conversion, budget alerts, or quota enforcement.
- LangGraph compatibility fields.
- IM channel usage views.
- Security scan policy pages.
- Legacy task token usage backfill.
- Menu naming changes.

## Tests First

- Backend route test must fail until the `token_usage` route is registered.
- Backend handler test must fail until usage rows and aggregate totals are mapped to API responses.
- Frontend service test must fail until the API client is exported.
- Frontend detail test must fail until canonical task-thread pages fetch and render token usage.

## Implementation Notes

- Keep IDs encoded as strings in JSON, matching existing task thread API behavior.
- Use application service methods added in Phase 26: `GetThreadTokenUsage`, `GetRunTokenUsage`, and `GetRun`.
- For `run_id` queries, load the run first and reject mismatched `thread_id` to avoid cross-thread token usage leakage.
- The UI indicator should be dense and unobtrusive: total tokens, input/output split, and source summary only.
- Legacy task detail remains on existing task APIs and should not call the thread usage API.

## Verification

- `go test ./backend/api/handler/coze ./backend/api/router/coze`
- Frontend focused tests for task detail and task service exports.
- `git diff --check`
