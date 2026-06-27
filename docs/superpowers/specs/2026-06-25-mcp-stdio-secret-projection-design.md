# MCP Stdio Secret Projection Design

M6.25 adds the first runtime secret-projection boundary for stdio MCP tools.
The goal is to let configured MCP servers receive credentials at execution
time without storing those credentials in public config, run events, audit
records, health metadata, or model-visible output.

## Scope

This slice introduces stdio `auth_env` projection:

```json
{
  "command": "npx",
  "args": ["-y", "@example/mcp-server"],
  "auth_env": {
    "API_TOKEN": "token",
    "CLIENT_SECRET": "oauth.client_secret"
  }
}
```

`auth_env` maps process environment variable names to dot-separated paths in
the server `auth` JSON. The actual secret values are read only from the raw
server auth returned by the internal runtime resolver.

This slice does not add OAuth refresh, KMS integration, external secret
managers, encrypted-at-rest migration, frontend credential pickers, session
pooling, SSE/HTTP transports, or public auth-read APIs.

## Runtime Flow

1. `ADKMCPRuntimeExecutor` resolves the internal MCP server record as before.
2. `ADKMCPRuntimeStdioTransport` parses bounded stdio config.
3. It parses `auth_env` as a bounded string map.
4. It parses server `auth` as a JSON object.
5. For each `env_name -> auth_path`, it resolves a string secret value.
6. The projected env value is added to `ADKMCPRuntimeStdioConfig.Env`.
7. Existing stdio static policy validates env allow-list, env count, and env
   value byte budget after projection.
8. Sandbox and Eino runner receive only the projected env map.

Config-provided `env` remains supported for non-secret environment values.
If an `auth_env` key collides with `env`, the auth-projected value wins so
operators can rotate credentials without changing public config.

## Safety Contract

All projection errors return fixed messages:

- invalid `auth_env` config;
- invalid auth JSON;
- missing auth field;
- non-string auth field;
- invalid env name.

Errors must not include auth paths, env values, command names, args, workdirs,
raw MCP config/auth, model arguments, tool results, provider payloads, object
keys, transcripts, checkpoint bytes, or secrets.

Projected secrets are subject to the existing `AllowedEnvKeys`,
`MaxEnvVars`, and `MaxEnvValueBytes` policy. If policy denies the projected
env, transport returns the existing sanitized policy-denied error.
