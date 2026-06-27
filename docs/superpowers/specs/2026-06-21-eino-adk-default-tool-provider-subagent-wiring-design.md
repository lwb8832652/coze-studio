# Eino ADK Default Tool Provider Subagent Wiring Design

## Goal

M3.11 wires SingleAgent-backed subagents into the production ADK tool provider
path. Before this slice, subagent providers and snapshot grants were covered by
unit contracts but the application factory still used `NewDefaultADKToolProvider`
directly.

The production default can now be built with
`NewDefaultADKToolProviderWithSingleAgentSubagents(source)`.

## Provider Order

The provider order is intentional:

```text
ADKToolPolicyProvider
  ADKSubagentToolProvider
    adkHumanInteractionToolProvider
      ADKRuntimeToolCatalogProvider
        ADKWebToolCatalog
```

The outer policy provider is the single allow-list boundary for the lead run.
It filters runtime tools, human-interaction tools, and subagent tools from one
resolved tool set.

When no SingleAgent source is configured, the helper returns the previous
default provider shape:

```text
ADKToolPolicyProvider
  adkHumanInteractionToolProvider
    ADKRuntimeToolCatalogProvider
      ADKWebToolCatalog
```

## SingleAgent Wiring

The subagent branch uses:

- `ADKRunConfigSubagentReferenceProvider` for the temporary M3 reference
  source;
- `ADKSingleAgentSubagentDefinitionProvider` for draft/version snapshot
  loading;
- `ADKSingleAgentSnapshotToolGrantProvider` for plugin/workflow grant names;
- `ADKSingleAgentSubagentAgentFactory` for child ADK agent construction.

Child SingleAgent runs are built with `NewDefaultADKToolProvider`, not the
subagent-aware provider. This keeps child runtime tools controlled by the
child run's generated `tool_policy` and avoids recursive subagent exposure
until durable AgentHub settings exist.

## Application Initialization

`initComplexServices` now runs before the ADK executor is built so the
production factory can receive `singleAgentSVC.DomainSVC` as the
`ADKSingleAgentSubagentService` source. Worker startup still happens after the
runtime selector is assembled.

## Deferred Work

Open follow-up work:

- replace temporary run-config `subagent_refs` with durable AgentHub settings;
- persist child run lifecycle rows before invoking `AgentTool`;
- add denied-grant audit events;
- verify runtime tool names against the future Tool Registry plugin/workflow
  executable adapters.
