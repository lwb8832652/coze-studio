# Eino ADK Tool Policy Factory Contract Design

## Goal

M3.8 locks the factory-level contract for tool policy enforcement. Once a
tool provider applies `tool_policy`, `ApplicationADKAgentFactory` must use the
same filtered tool set for every downstream consumer.

## Contract

`ApplicationADKAgentFactory` resolves tools once through `ADKToolProvider`.

The resolved set is then used for:

- `ToolsConfig.ToolsNodeConfig.Tools`, which defines executable tools;
- `ADKMiddlewareBuildInput.StaticTools`;
- `ADKMiddlewareBuildInput.DynamicTools`;
- Eino model input generation, where static tool infos are passed through
  `model.WithTools`.

This means model-visible tools and executable tools come from the same
policy-filtered source. Runtime code must not rebuild, merge, or append model
tool infos outside the provider/middleware contract.

## Verification

`TestADKAgentFactoryUsesPolicyFilteredToolSetForModelAndMiddleware` builds an
agent with static and dynamic allowed/blocked tools, runs the agent, and
asserts:

- the model receives only the allowed static tool through `WithTools`;
- middleware receives only the allowed static tool;
- middleware receives only the allowed dynamic tool.

## Deferred Work

Native model tool search and dynamic tool search middleware still need
end-to-end policy tests when child Skill/MCP grants are wired from durable
configuration.
