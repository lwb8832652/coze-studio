# Eino ADK Subagent Provider Model Attribution Design

## Goal

M3.23 adds lightweight provider/model attribution to subagent state cards.
This gives DeerFlow-style execution observability without introducing cost
pricing UI or exposing raw model payloads.

## Data Source

Task detail already requests parent token usage with:

```text
run_id=:parent_run_id&include_child_runs=true
```

The response includes usage rows for the parent and direct child runs. The
frontend derives the first non-empty attribution per child run from:

- `usage.provider`;
- `usage.model_name`.

The displayed label is `provider / model_name`, or whichever field is present.

## Boundaries

The attribution label is metadata only. It must not display raw usage JSON,
provider response bodies, prompt text, completion text, tool arguments, tool
results, credentials, object identifiers, checkpoint bytes, or file names.

If multiple providers or models appear for one child run, this slice displays
the first observed attribution only. Multi-provider summaries remain deferred.

## Deferred Work

Open follow-up work:

- cost and pricing snapshot presentation;
- multi-provider/model attribution summaries;
- browser visual QA against live multi-agent task fixtures;
- grouped backend attribution DTOs if UI needs richer breakdowns.
