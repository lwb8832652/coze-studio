# Eino ADK Subagent Retry Runtime Routing Design

## Goal

M3.32 wires subagent retry replay through the production runtime selector. The
worker uses `RuntimeSelector` as its run executor, so ADKExecutor replay support
is not reachable until the selector exposes the same retry capability.

## Runtime Routing

`RuntimeSelector.ExecuteSubagentRetry` uses the same runtime policy and run
config selection as ordinary `Execute`:

- `runtime='eino_adk'` routes to the ADK executor only when ADK is enabled by
  server policy and the configured ADK executor implements
  `SubagentRetryRunExecutor`;
- `runtime='legacy'` routes to legacy only if legacy explicitly implements
  retry;
- missing selected executor still returns the existing configured-runtime
  errors;
- selected runtimes that do not implement retry return
  `SubagentRetryUnsupportedError`.

`RunProcessor` maps `SubagentRetryUnsupportedError` to the fixed failure
contract `error_code='subagent_retry_not_supported'`. This preserves M3.27
fail-closed behavior even after the selector itself implements the retry
interface.

## Production Wiring

The application ADK executor is constructed with
`NewApplicationADKSubagentRetrySourceResolver(agentThreadSVC)`. The resolver
loads source child rows through the application service and leaves source
identity validation to ADKExecutor before replay.

## Safety

Unsupported runtimes must not fall through to ordinary task execution or
generic `executor_error`. Runtime policy remains authoritative, so a crafted
run config cannot use retry replay to bypass an ADK-disabled server.
