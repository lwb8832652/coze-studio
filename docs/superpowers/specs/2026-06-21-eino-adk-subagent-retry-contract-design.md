# Eino ADK Subagent Retry Contract Design

## Goal

M3.26 establishes the backend contract for retrying a failed or canceled child
subagent run. It creates a real queued work item that future executors and UI
controls can target without mutating historical child-run rows or making the
ordinary worker queue claim child rows directly.

## API Contract

The Workbench task-thread API exposes:

```text
POST /api/workbench/task_threads/:thread_id/runs/:run_id/retry
```

The path `run_id` is the failed or canceled child `run_kind='subagent'` row.
The optional request body field is:

```json
{"idempotency_key":"retry-child-20"}
```

The response is a `TaskThreadRun` for the newly created retry run.

## Runtime Contract

The retry request validates that the source run belongs to the requested
thread, has `run_kind='subagent'`, has a parent run, and is `failed` or
`canceled`. It then creates a new top-level queued run with:

- `parent_run_id=0`;
- `run_kind='task'`;
- lead/parent run assistant, config, context, stream mode, durability, and
  scheduling options;
- `input={"messages":[]}`;
- a content-free `command.subagent_retry` payload containing source child run
  ID, parent run ID, source status, sanitized error code, requested time, and a
  `top_level_retry` worker contract marker.

The source child row remains immutable. Worker claim paths stay top-level-only;
future executor support should consume `command.subagent_retry` and decide how
to replay the child/subagent work.

## Events And Safety

The API appends `subagent.retry.requested` to the new retry run. The event
payload includes only metadata such as thread ID, source child run ID, parent
run ID, retry run ID, source status, and requested time. It must not include
model input/output, child final output, prompts, tool arguments/results,
provider payloads, checkpoint bytes, object URIs, URLs, filenames, or
credentials.

## Deferred Work

Open follow-up work:

- executor consumption of `command.subagent_retry`;
- retry-result joins from retry run back to source child card;
- frontend retry button and loading/error states;
- parent cancellation propagation into retry execution;
- recursive descendant retry policy and quotas;
- browser visual QA against live multi-agent task fixtures.
