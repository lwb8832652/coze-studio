# Eino ADK Web Tools Design

## Goal

M2.18 adds policy-controlled Web tools to the Go-native Eino ADK runtime:

- `web_fetch`: fetch bounded text content from an explicitly allowed URL host.
- `web_search`: call a Coze-owned search backend and return bounded search
  results.

The tools are exposed as Eino `tool.BaseTool` values through the existing
`ADKRuntimeToolCatalogProvider`, so they participate in static/deferred tool
selection, tool-search promotion, tool budgeting, tool-error normalization,
audit, and future policy decisions.

## Eino-ext Review

The available Eino-ext tool modules are:

- `github.com/cloudwego/eino-ext/components/tool/httprequest`
  `v0.0.0-20260616080858-ab17b7308bf8`
- `github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2`
  `v2.0.0-20260630024214-84091ffbdce4`
- `github.com/cloudwego/eino-ext/components/tool/wikipedia`
  `v0.0.0-20260630024214-84091ffbdce4`
- `github.com/cloudwego/eino-ext/components/tool/googlesearch`
  `v0.0.0-20260616080858-ab17b7308bf8`

`httprequest` is not exposed directly because its tools accept arbitrary URLs,
read the full response body, and do not provide Coze-owned redirect, SSRF,
output-budget, event, or audit policy. `web_search` defaults to a Coze-owned
`auto` provider chain that mirrors DeerFlow's Python DDGS `backend=auto`
behavior without adding a Python sidecar: Eino-ext DuckDuckGo v2 is tried first,
then a bounded Brave HTML provider, then Eino-ext Wikipedia. The Go DuckDuckGo
v2 package exposes region, max results, timeout, and time range, but it does
not currently expose the Python DDGS `backend` multiplexer or runtime
`safesearch` application used by
`deerflow.community.ddg_search.tools:web_search_tool`; Coze keeps compatible
config fields at the boundary and implements the provider fallback in its own
adapter. Google still requires Coze secret storage and masked configuration
before production use.

## Run Config

Web tools are enabled by the DeerFlow-parity Workbench default run
configuration, with `web_search` on and raw HTTP fetch off. Server policy can
still fail closed by disabling the provider or by omitting the run-config web
tool switch:

```json
{
  "web_tools": {
    "enabled": true,
    "visibility": "static",
    "http": {
      "enabled": true,
      "allowed_hosts": ["docs.example.com"],
      "allow_http": false,
      "allow_private_ips": false,
      "timeout_ms": 10000,
      "max_response_bytes": 32768
    },
    "search": {
      "enabled": true,
      "max_results": 5
    }
  }
}
```

`webTools` is accepted as the camelCase alias. Tool visibility may be `static`
or `deferred`.

Search provider selection is server-side:

- default / `AGENT_THREAD_WEB_SEARCH_PROVIDER=auto`: tries DuckDuckGo v2,
  Brave HTML search, then Wikipedia, returning the first non-empty result set;
- `AGENT_THREAD_WEB_SEARCH_PROVIDER=duckduckgo`: Eino-ext DuckDuckGo v2, no
  API key required, using optional `AGENT_THREAD_WEB_SEARCH_DDG_REGION` and
  validated `AGENT_THREAD_WEB_SEARCH_DDG_SAFESEARCH`;
- `AGENT_THREAD_WEB_SEARCH_PROVIDER=brave`: bounded Brave HTML search, using
  optional `AGENT_THREAD_WEB_SEARCH_BRAVE_BASE_URL`;
- `AGENT_THREAD_WEB_SEARCH_PROVIDER=wikipedia`: Eino-ext Wikipedia search,
  using `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_LANGUAGE` and optional
  `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_BASE_URL`,
  `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_USER_AGENT`, and
  `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_DOC_MAX_CHARS`;
- `AGENT_THREAD_WEB_SEARCH_PROVIDER=http`: existing Coze-owned HTTP search
  backend, requiring `AGENT_THREAD_WEB_SEARCH_ENDPOINT`;
- `AGENT_THREAD_WEB_SEARCH_PROVIDER=disabled` or
  `AGENT_THREAD_WEB_SEARCH_ENABLED=false`: disables `web_search` even if the
  user run config requests it.

## Fetch Policy

`web_fetch`:

- supports GET only in M2.18;
- requires a non-empty allowed-host list;
- allows HTTPS by default and HTTP only with `allow_http`;
- rejects localhost, loopback, link-local, private, multicast, and unspecified
  IP literals unless `allow_private_ips` is explicitly enabled;
- checks redirects against the same policy;
- clamps timeout to `1s-60s`;
- clamps response budget to `1KiB-1MiB`;
- returns structured JSON with schema, host, status code, content type,
  truncation flag, and bounded body.

The tool result may include fetched content because it is model-visible tool
output. Run events and errors must not include raw URLs, request bodies,
credentials, object keys, or response bodies.

## Search Policy

`web_search`:

- uses Eino-ext DuckDuckGo v2 by default, or an injected/configured
  `ADKWebSearchBackend` when server policy selects another provider;
- clamps `max_results` to `1-10`;
- returns title, URL, snippet, and source fields only;
- leaves provider credentials, rate limits, and regional policy to the backend.

## Follow-up Work

- Add durable Tool Registry records for Web search/fetch providers.
- Add secret-backed Google Custom Search settings.
- Add advanced provider switching for DeerFlow Python DDGS `backend` parity
  beyond the currently wired `auto`, `duckduckgo`, `brave`, `wikipedia`,
  `http`, and `disabled` providers only when the Go provider exposes
  equivalent controls or Coze adds a reviewed provider abstraction.
- Add security scanner and per-space web-domain policy in M8.
