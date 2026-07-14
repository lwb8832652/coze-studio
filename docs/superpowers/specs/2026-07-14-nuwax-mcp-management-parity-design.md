# Nuwax MCP Management Parity Design

## Goal

Upgrade Coze Studio's existing MCP configuration surface to provide the complete
management workflow demonstrated by `nuwax-ai`, while keeping Coze's existing
navigation, Go services, Eino MCP runtime, tenant model, and security boundaries.

This design does not add a new first-level sidebar menu. The canonical full page
remains `/space/:space_id/tools`; the existing account settings `MCP 配置` tab
remains the discoverable entry and compact management surface.

## Verified Baseline

### Nuwax reference behavior

The reference implementation provides:

- custom and official service segments;
- creator, deployment-status, and keyword filters;
- service cards with owner, timestamps, install type, status, and actions;
- create and edit pages;
- `npx`, `uvx`, SSE, and Streamable HTTP install types;
- save, save-and-deploy, stop, delete, log, and configuration export actions;
- discovered tool, resource, and prompt tabs;
- schema-driven test execution with formatted results.

The Nuwax component-library install type combines internal plugins, workflows,
knowledge, and databases behind an MCP-shaped facade. Coze already models those
sources in its unified Tool Registry, so copying that mode would duplicate the
same tools and weaken source ownership.

### Coze baseline

Coze currently has:

- the `/space/:space_id/tools` page and reusable `MCPToolSettingsPanel`;
- an account settings `MCP 配置` tab;
- MySQL-backed MCP server persistence;
- list, upsert, get, delete, registry projection, and test-call APIs;
- masked-auth round-trip preservation;
- health metadata and runtime audit persistence;
- Eino-backed stdio, SSE, and Streamable HTTP runtime transports.

The current UI only lists servers and toggles `enabled`. The server accepts a
client-supplied tool list, does not discover resources or prompts, and
`TestMCPToolCall` returns `mcp_test_call_stub` instead of invoking the MCP
runtime. The management handlers also need explicit authenticated-space access
checks rather than trusting identifiers supplied by the client.

## Product Scope

### Single-page entry

- Keep `/space/:space_id/tools` as the only MCP management page.
- Keep the account settings `MCP 配置` tab.
- Do not add a sidebar first-level menu.
- Do not create a parallel `/mcp` route.
- Do not add create, edit, detail, or test child routes.
- The settings tab reuses the same single-page management component in a compact
  layout and can navigate to the existing `/tools` URL when more room is needed.

### Management list

The full page contains:

- `自定义服务` and `官方服务` segments;
- keyword search;
- `全部 / 已启用 / 异常 / 已停用` status filters;
- `所有人 / 由我创建` creator filters for custom services;
- refresh and create actions;
- responsive service cards showing source, transport, creator, updated time,
  enabled state, health, latency, and discovered capability counts;
- actions for edit, enable or stop, safe export, runtime log, and delete.

Official services are seeded by Coze configuration, read-only, and can be
enabled or stopped for the current space. Custom services can be edited or
deleted only when the current user's server-derived space role allows it.

### Create and edit

Create and edit stay inside the existing Tools page. Creating a service opens a
form drawer; selecting a service opens an edit drawer. Capability details,
schema-driven test execution, export, logs, and destructive confirmations use
nested drawers or dialogs without changing the URL.

The form contains name, description, transport, JSON connection configuration,
and separate JSON authentication configuration. Supported transports are:

- `stdio`, presented as `npx` or `uvx` templates;
- `sse`;
- `streamable_http`.

`保存` persists a disabled custom server without contacting it. `保存并启用`
persists the server, validates policy, connects to the server, discovers its
capabilities, records health, and enables it only after discovery succeeds.
Failed discovery leaves the saved server disabled or preserves the previous
enabled state and returns an actionable error.

The edit page exposes `概览`, `工具`, `资源`, `提示词`, and `运行日志` tabs.
Capability tabs are read-only projections of server discovery, not values the
browser is allowed to declare.

### Discovery and test execution

Tool definitions, resources, and prompts are server-owned data:

- the browser cannot authoritatively submit discovered capabilities;
- discovery runs through an injected MCP control-plane client;
- successful discovery atomically updates capabilities, health, and enabled
  state;
- failed discovery records bounded health metadata without exposing secrets;
- tool test execution invokes the same Coze-owned Eino MCP runtime path used by
  agents;
- resource reads and prompt retrieval use bounded MCP control-plane operations;
- test output is size-limited and sanitized before returning to the UI;
- every execution writes content-free runtime audit metadata.

The existing stub test response is removed. A disabled runtime, unavailable
runner, blocked stdio command, disallowed remote host, or invalid schema fails
closed with a stable error code and user-facing message.

### Export and logs

Export returns a formatted connection template containing transport, config,
and masked authentication placeholders. It never returns stored credentials,
raw audit payloads, tool arguments, tool results, or provider bodies.

The log tab reads the existing MCP runtime audit projection filtered by
`space_id` and `server_id`. It shows timestamp, operation, status, latency, and
bounded error code only.

## Data Contract

Extend the durable server projection with:

- `creator_id`;
- `source_type`: `custom` or `official`;
- discovered `resources` JSON;
- discovered `prompts` JSON.

Keep `enabled` and health as the source of truth. UI deployment state is derived:

- disabled: `已停用`;
- enabled and healthy: `已启用`;
- enabled and unknown: `检查中`;
- unhealthy: `异常`.

Responses include server-derived permission booleans for edit, enable, export,
log, and delete. They do not expose raw role records.

Upsert requests no longer require a client-authored tool list. Compatibility
fields may remain accepted during migration, but the service ignores them as an
authority and preserves or replaces capabilities only through discovery.

## API Shape

Retain existing endpoints and add bounded control-plane operations:

- `GET /api/workbench/mcp_tools`
- `POST /api/workbench/mcp_tools`
- `GET /api/workbench/mcp_tools/:server_id`
- `DELETE /api/workbench/mcp_tools/:server_id`
- `POST /api/workbench/mcp_tools/:server_id/discover`
- `POST /api/workbench/mcp_tools/:server_id/test_call`
- `POST /api/workbench/mcp_tools/:server_id/read_resource`
- `POST /api/workbench/mcp_tools/:server_id/get_prompt`
- `GET /api/workbench/mcp_tools/:server_id/export`
- `GET /api/workbench/mcp_tools/:server_id/audits`

Every handler resolves the authenticated user, verifies membership of the
server's persisted space, and authorizes the operation from the real space role.
For existing servers, `space_id` comes from persistence and cannot be changed by
the request.

## Security Boundaries

- Keep secret masking and masked round-trip preservation.
- Encrypt persisted authentication through the configured auth codec.
- Never include secrets in exports, discovery errors, logs, or test results.
- Permit stdio only under the existing debug/host-runtime or managed-runner
  policy; production remains fail closed without an approved runner.
- Continue command allowlisting, shell-metacharacter rejection, workdir leases,
  sandbox limits, timeout, output limit, and audit recording.
- Require HTTPS and configured host policy for non-local remote transports.
- Do not trust client `space_id`, `creator_id`, source type, permissions, health,
  or discovered capabilities.
- Official services cannot be edited or deleted through workspace APIs.

## Frontend Structure

Keep the single page decomposed into focused units:

- `ToolsPage`: the only route and full-page shell;
- `MCPToolSettingsPanel`: shared full or compact list controller;
- list toolbar and server card components;
- create/edit drawer and form component;
- capability tabs;
- schema-driven test drawer;
- export and delete confirmation dialogs;
- audit list.

Use Coze Design and existing workspace visual tokens. Match Nuwax's information
hierarchy and interaction density without importing Ant Design or copying its
global shell.

## Error and State Handling

Every view covers loading, empty, error, disabled, refreshing, saving,
discovering, testing, and deleting states. Mutations are single-flight. Business
errors use the backend `msg` and stable error code. A failed refresh keeps the
last successful list visible with an error banner rather than showing a false
empty state.

## Testing and Acceptance

Backend tests must cover:

- authenticated space membership and role enforcement;
- immutable server space and official-source restrictions;
- masked auth persistence and safe export;
- create, update, enable, stop, and soft delete;
- discovery success, timeout, malformed capability, and rollback behavior;
- real executor delegation instead of the stub response;
- tool, resource, and prompt operations with bounded output;
- audit filtering and metadata-only projection;
- stdio and remote fail-closed policy paths.

Frontend tests must cover:

- custom and official segments, filters, search, refresh, and compact mode;
- create and edit validation for every supported transport;
- save versus save-and-enable behavior;
- capability rendering and test input generation;
- health and permission states;
- export, stop, delete, and error handling;
- drawer lifecycle and state restoration on the existing `/tools` page.

Browser acceptance uses the Codex in-app browser and compares the Nuwax
reference flow with Coze at the same breakpoints. It records the URL, account,
space, list states, create/edit behavior, discovery, one successful or expected
fail-closed test call, export safety, delete confirmation, and console errors.

## Explicit Exclusions

- No new sidebar first-level menu.
- No `/mcp` parallel route.
- No MCP create, edit, or detail child route.
- No Nuwax component-library MCP mode.
- No duplicate plugin, workflow, knowledge, or database wrapping.
- No OAuth authorization-code UI in this slice; existing JSON auth and masked
  credentials remain supported.
- No IM channel or unrelated system-management work.
