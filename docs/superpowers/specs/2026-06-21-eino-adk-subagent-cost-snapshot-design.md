# Eino ADK Subagent Cost Snapshot Design

## Goal

M3.25 adds a compact cost snapshot to child subagent state cards using only
backend-provided token usage aggregation. The frontend does not calculate
pricing or infer currency; it only formats cost metadata that is already safe
to display.

## Data Source

Task detail uses the existing parent token usage request:

```text
run_id=:parent_run_id&include_child_runs=true
```

The response provides:

- `run_aggregates[].aggregate.cost_micros` for each child run;
- `usage[].currency` rows for each child run.

The frontend attaches a currency to a child aggregate only when all non-empty
usage row currencies for that run de-duplicate to exactly one value.

## Behavior

If a child run has positive `cost_micros` and one currency, the subagent card
renders:

```text
Cost USD 0.000250
```

The amount is `cost_micros / 1_000_000` formatted with six decimal places. If
`cost_micros` is zero, currency is missing, or more than one currency is
present for the child run, the card omits the cost label.

## Safety Boundary

The card displays only bounded cost metadata. It must not expose raw usage
JSON, provider responses, prompts, completions, tool arguments, tool results,
credentials, object identifiers, checkpoint bytes, file names, or pricing
rules. Backend services remain responsible for pricing snapshots, currency
normalization, cost accounting, tenant policy, and audit.

## Deferred Work

Open follow-up work:

- backend-owned pricing snapshot DTOs with model/provider price references;
- multi-currency cost breakdowns in a richer debug view;
- retry/resume controls for child runs;
- recursive descendant usage rollups;
- browser visual QA against live multi-agent task fixtures.
