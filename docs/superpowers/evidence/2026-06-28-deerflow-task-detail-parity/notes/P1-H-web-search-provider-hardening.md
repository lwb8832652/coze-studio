# P1-H Web Search Provider Hardening

Date: 2026-07-02

## Scope

This slice verifies the Go-native web search provider layer used by the Eino
ADK runtime. No new runtime code was required in this pass because the provider
chain and safety tests already exist in the current branch.

## External Source Check

Official CloudWeGo/Eino docs list tool integrations including DuckDuckGoSearch,
HTTPRequest, CommandLine, and Wikipedia:

- https://www.cloudwego.io/zh/docs/eino/ecosystem_integration/tool/

Official eino-ext components checked:

- https://github.com/cloudwego/eino-ext/tree/main/components/tool/duckduckgo/v2
- https://github.com/cloudwego/eino-ext/tree/main/components/tool/wikipedia

The DuckDuckGo v2 README states that it implements Eino `InvokableTool`, but
also warns that DuckDuckGo is not recommended as a single production dependency
because it does not use a stable OpenAPI. Coze therefore keeps DuckDuckGo v2
inside a fallback chain instead of treating it as the only production provider.

The Wikipedia README confirms an Eino `InvokableTool` with configurable
`BaseURL`, `UserAgent`, `DocMaxChars`, `Timeout`, `TopK`, and `Language`, which
matches the Coze `ADKWikipediaWebSearchBackend` adapter.

## Coze Source Evidence

- Provider selection and env policy:
  `backend/application/agentthread/adk_web_search_backend.go`
- DuckDuckGo v2 adapter:
  `backend/application/agentthread/adk_duckduckgo_web_search_backend.go`
- Brave HTML fallback adapter:
  `backend/application/agentthread/adk_brave_web_search_backend.go`
- Wikipedia adapter:
  `backend/application/agentthread/adk_wikipedia_web_search_backend.go`
- Runtime tool exposure:
  `backend/application/agentthread/adk_web_tools.go`
- Design record:
  `docs/superpowers/specs/2026-06-21-eino-adk-web-tools-design.md`

## Provider Contract

Server-side provider selection:

- `AGENT_THREAD_WEB_SEARCH_PROVIDER=auto` or unset:
  DuckDuckGo v2 -> Brave HTML -> Wikipedia, returning the first non-empty
  bounded result set.
- `duckduckgo` / `ddg`: Eino-ext DuckDuckGo v2.
- `brave`: bounded Brave HTML parser.
- `wikipedia` / `wiki`: Eino-ext Wikipedia.
- `http` / `coze_http`: Coze-owned JSON HTTP backend with SSRF and redirect
  policy.
- `disabled`, `off`, `none`, `false`, or
  `AGENT_THREAD_WEB_SEARCH_ENABLED=false`: fail closed with no backend.

Exposure is two-stage:

1. The server initializes a safe backend from env.
2. The tool is exposed to the model only when run config enables
   `web_tools.enabled` and `web_tools.search.enabled`.

This is why the backend can default to `auto` while the tool catalog is still
disabled by default for runs that do not request web tools.

## Safety Boundary

The provider layer must not expose:

- API keys or auth headers;
- raw provider response bodies;
- query text in internal error strings;
- private paths or redirect targets in errors;
- private IP endpoints unless explicitly allowed in test/local config;
- unbounded search result bodies.

Coze returns only `title`, `url`, `snippet`, and `source`, bounded to the
configured result limit.

## Verification

Passed:

```bash
cd backend && go test ./application/agentthread -run 'TestADK(Web|Duck|Auto|Brave|Wikipedia|HTTP|DefaultADKToolProviderExposesWebSearch)' -count=1 -gcflags="all=-N -l"
```

Covered cases include:

- tool catalog disabled by default without run config;
- env default provider resolves to `auto`;
- DuckDuckGo v2 adapter normalizes bounded results;
- auto provider falls back after provider failure or empty results;
- Brave HTML provider parses DeerFlow-style web results;
- Wikipedia provider normalizes snippet/extract;
- HTTP backend posts bounded JSON with secret header support;
- HTTP backend sanitizes errors and redirect failures;
- explicit env disable fails closed;
- invalid env config does not leak userinfo or API keys.

## Deferred

No direct CommandLine or raw HTTPRequest eino-ext tools are exposed to Agent
runs in P1-H because they accept broad command/URL input. Coze keeps its
reviewed `web_fetch` and JSON HTTP search backend instead. Any future direct
CommandLine/HTTPRequest exposure needs separate policy, sandbox, audit, and
output-budget work.

