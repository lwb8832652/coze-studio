# MCP Auth Catalog Codec Design

M6.26 adds the first at-rest credential boundary for durable MCP server auth.
The immediate goal is not to pick a KMS provider; it is to keep the catalog API
stable while allowing MySQL storage to encode auth before persistence and
decode it for internal runtime use.

## Scope

This slice introduces:

- `MCPAuthCodec`, a small encode/decode interface.
- `WithMySQLCatalogAuthCodec`, an optional constructor hook.
- Default passthrough behavior for local development, tests, and existing
  rows.
- Sanitized encode/decode errors that do not include auth JSON or codec
  provider details.

It does not add a KMS provider, rotate existing rows, change the database
schema, change Workbench response masking, add OAuth refresh, or expose public
auth-read APIs.

## Runtime Flow

1. `ApplicationService.UpsertServer` still resolves masked auth updates into
   raw internal auth JSON.
2. `MySQLCatalog.Upsert` encodes the raw auth with its configured codec before
   writing `mcp_tool_servers.auth`.
3. `MySQLCatalog.Get` and `List` decode stored auth before returning internal
   `MCPToolServer` values.
4. Public Workbench APIs still call `cloneServerForResponse`, which masks auth
   before response serialization.
5. Runtime `ResolveADKMCPRuntimeServer` receives decoded raw auth for M6.25
   stdio `auth_env` projection.

## Safety Contract

Codec failures return fixed errors:

- `mcp tool auth encode failed`
- `mcp tool auth decode failed`

Those errors must not include raw auth JSON, ciphertext, object keys, provider
payloads, KMS identifiers, stack traces, or secrets.

The codec must return valid JSON text because the current storage column type
is JSON. Future KMS codecs may store an envelope such as
`{"schema":"coze.mcp_auth_envelope.v1","ciphertext":"..."}` without another
catalog API change.
