# M3.33 Eino ADK Subagent Frontend Retry Controls Design

## Goal

Expose the existing Workbench subagent retry API from canonical task detail so
operators can retry a failed or canceled child subagent run without leaving the
task detail page.

## Scope

This slice is frontend-only. It assumes the backend retry contract already
exists: `POST /api/workbench/task_threads/:thread_id/runs/:run_id/retry`
creates a new top-level queued retry run and returns the created run. The UI
does not replay work locally, infer retry output, or mutate historical child
run rows.

## User Experience

The `子智能体执行` card list shows a `重试` action only for child runs whose
normalized status is `failed` or `canceled`. Running, pending, and completed
child runs do not show the action. Clicking the action disables that child
action while the request is in flight, calls the retry API with the canonical
thread ID and child run ID, then refreshes task detail through the existing
loader. If the API fails, the section shows a concise inline error and leaves
the original child card unchanged.

The product navigation and object names remain `任务`. The retry action is a
child-run recovery action inside a task detail page, not a new top-level task
type.

## Architecture

`useTaskDetailActions` owns retry side effects alongside follow-up and human
interaction actions. It adds a `handleRetrySubagentRun` callback, a
`retryingSubagentRunId` state for per-row loading, and
`subagentRetryError` for section-level feedback. The action is enabled only for
canonical thread detail because legacy task detail cannot address durable
thread run IDs.

`TaskSubagentRunsSection` remains presentation-focused. It receives optional
`onRetrySubagentRun`, `retryingRunId`, and `retryError` props, derives
retryability from normalized status, and renders a small Coze Design button in
the row action area. The component does not import the retry API directly.

## Data And Safety

The UI passes only `{ thread_id, run_id }` to the generated API wrapper. It does
not send child command, input, config, context, tool arguments, model text,
checkpoint bytes, object URIs, URLs, filenames, or raw provider payloads.
Failures are surfaced as bounded user-facing strings.

## Testing

Task detail tests cover canonical thread child retry from a failed subagent
card, verify the retry API receives the canonical thread ID and child run ID,
and verify detail data is refreshed after success. Existing task detail tests
continue to cover subagent cards, grouped token usage, lifecycle timelines,
human interaction resume, follow-up submission, and execution event streaming.
