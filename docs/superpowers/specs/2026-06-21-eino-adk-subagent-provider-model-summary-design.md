# Eino ADK Subagent Provider Model Summary Design

## Goal

M3.24 makes provider/model attribution on subagent state cards safe for child
runs that use more than one model or provider. The card should stay compact,
metadata-only, and DeerFlow-style while still signaling that more than one
attribution source contributed usage.

## Data Source

Task detail continues to request parent token usage with:

```text
run_id=:parent_run_id&include_child_runs=true
```

The grouped response contains usage rows for the parent run and direct child
subagent runs. The frontend derives attribution labels from each child usage
row using:

- `usage.provider`;
- `usage.model_name`.

The label format remains `provider / model_name`, or whichever single field is
present when the other field is missing.

## Behavior

For each child run, task detail de-duplicates distinct attribution labels in
response order. If one label exists, the card displays that label. If more than
one distinct label exists, the card displays the first label with a bounded
count suffix:

```text
openai / gpt-4.1 +1
```

The suffix counts additional distinct labels, not total calls. Missing
attribution remains omitted.

## Safety Boundary

The attribution summary is observability metadata only. It must not display raw
usage JSON, provider response bodies, prompt text, completion text, tool
arguments, tool results, credentials, object identifiers, checkpoint bytes, or
file names. Rich provider/model breakdowns, pricing snapshots, and cost
details stay behind future backend-owned DTOs and policy review.

## Deferred Work

Open follow-up work:

- backend-owned attribution breakdown DTOs for richer debug views;
- cost and pricing snapshot presentation;
- retry/resume controls for child runs;
- recursive descendant usage rollups;
- browser visual QA against live multi-agent task fixtures.
