# MCP Stdio Static Policy Design

## Goal

M6.9 adds a concrete fail-closed policy for the stdio MCP transport boundary.
It still does not execute local commands. The policy validates a parsed
`ADKMCPRuntimeStdioSandboxCall` before any future sandbox implementation can
start a process or MCP session.

## Policy Boundary

`ADKMCPRuntimeStdioStaticPolicy` implements `ADKMCPRuntimeStdioPolicy`.
It is configured through `ADKMCPRuntimeStdioStaticPolicyOptions`:

- `AllowedCommands`: exact command allow-list. Empty means deny all commands.
- `AllowedWorkingDirPrefixes`: absolute allowed working-directory roots.
- `AllowedEnvKeys`: allowed environment variable names. Empty means no env
  keys are allowed.
- `MaxArgs`: maximum argument count. Zero means no args are allowed.
- `MaxArgBytes`: maximum total argument bytes. Zero means non-empty args are
  rejected.
- `MaxEnvVars`: maximum environment variable count. Zero means no env vars are
  allowed.
- `MaxEnvValueBytes`: maximum byte length for each env value. Zero means
  non-empty env values are rejected.
- `RequireWorkingDir`: when true, working directory must be present and inside
  an allowed prefix.

The default empty options are intentionally unusable for execution. They fail
closed at command validation.

## Validation Order

Validation runs in this order:

1. command allow-list;
2. working directory presence and prefix isolation;
3. argument count and byte budget;
4. env key allow-list, env value byte budget, then env count budget.

Env key checks run before env count checks so an unknown variable is classified
as an env policy violation rather than a budget failure. Workdir prefix checks
use cleaned absolute paths and path-relative containment, so
`/mnt/coze/mcp-secret` does not match `/mnt/coze/mcp`.

## Errors And Redaction

The policy returns fixed, sanitized errors:

- `mcp runtime stdio command is not allowed`;
- `mcp runtime stdio working directory is required`;
- `mcp runtime stdio working directory is not allowed`;
- `mcp runtime stdio args exceed budget`;
- `mcp runtime stdio env is not allowed`;
- `mcp runtime stdio env exceeds budget`.

Errors must not include model arguments, command names, command args, working
directories, env keys or values, server names, raw config/auth, URLs, object
keys, provider diagnostics, prompt/model text, transcripts, checkpoint bytes,
or secret-adjacent data.

## Non-Goals

- No host command execution.
- No sandbox provider implementation.
- No directory creation, mount setup, or cleanup.
- No encrypted secret lookup or env value projection.
- No Eino MCP adapter invocation.
- No audit persistence or health classification.
- No production bootstrap wiring.

## Future Work

Next slices should add a sandbox implementation that consumes this policy
result, creates isolated workdirs, projects approved secrets, enforces process
limits, invokes the Eino MCP stdio adapter, and records audit/health outcomes
without exposing sensitive execution content.
