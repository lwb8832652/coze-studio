# MCP Secret Masking And Delete Design

## Goal

M6.2 closes two production gaps in Workbench MCP configuration:

- MCP auth secrets must not be returned in Workbench API responses.
- MCP server configs must support a delete lifecycle that removes them from
  default list/detail/runtime candidate paths.

## Secret Masking

The Workbench MCP API keeps the existing `auth` string field for compatibility,
but response data is masked before it leaves the application service.

Sensitive JSON keys are masked recursively with `********`, including:

- `api_key`, `apikey`
- `access_key`, `secret_key`
- `token`, `access_token`, `refresh_token`, `id_token`
- `client_secret`, `secret`
- `password`
- `private_key`
- `credential`, `credentials`

Examples:

```json
{"type":"bearer","token":"secret-token"}
```

is returned as:

```json
{"type":"bearer","token":"********"}
```

The stored catalog row keeps the original auth JSON. If a client submits a
masked value while updating an existing server, the service merges that field
from the stored auth JSON so editing a display-safe payload does not wipe
credentials accidentally. Blank `auth` on update also preserves existing auth.

## Delete Lifecycle

`DELETE /api/workbench/mcp_tools/:server_id` soft-deletes the server. The
handler returns a masked snapshot of the deleted server and default `List`,
`Get`, `TestCall`, and Skill candidate paths no longer see that row.

`MySQLCatalog.Delete` sets `mcp_tool_servers.deleted_at`; `InMemoryCatalog`
removes the row because it is a test/harness implementation.

## Non-Goals

- No restore endpoint in this slice.
- No encrypted secret storage or KMS integration yet.
- No masked field-level edit UI beyond preserving masked API payloads.
- No Tool Registry normalization.
- No runtime Eino MCP adapter invocation.
- No OAuth/session lifecycle changes.

## Security Notes

The mask is an API presentation boundary, not encryption. Production still
needs encrypted-at-rest auth storage, masked round-trip tests across all MCP
auth types, audit events, tenant authorization, health status, and runtime
execution policy before MCP execution can be treated as production-ready.
