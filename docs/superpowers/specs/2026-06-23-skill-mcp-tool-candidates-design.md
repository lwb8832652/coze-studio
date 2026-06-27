# Skill MCP Tool Candidates Design

## Goal

M5.8 extends the Skill permission candidate API so configured MCP tools can
appear beside ADK built-ins. The API remains a Coze-owned metadata boundary:
the Skill UI receives safe grant names and display metadata only, not MCP
transport config, auth data, input schemas, or executable payloads.

## Scope

- Add optional source metadata to `SkillToolCandidate`:
  - `source`
  - `source_id`
  - `source_name`
- Introduce a Skill application `ToolCandidateProvider` interface.
- Keep ADK built-ins as the first candidate source.
- Adapt the current Workbench MCP tool service as a provider for enabled MCP
  servers in the requested space.
- Convert MCP tool definitions to stable Eino-safe grant names:
  `mcp_{server_id}_{sanitized_tool_name}`.
- Skip disabled MCP servers, nil tools, empty tool names, and tools without a
  description.
- Normalize the final candidate list in the Skill application:
  - trim and bound display metadata;
  - require Eino-safe names;
  - require non-empty descriptions;
  - default missing category/visibility;
  - deduplicate by grant name with earlier sources winning.
- Display MCP server source metadata in the Skill permission drawer while
  saving only `permissions.allowed_tools`.

## API Contract

The candidate response is still metadata-only:

```json
{
  "name": "mcp_100_search_docs",
  "display_name": "docs-mcp / search-docs",
  "description": "Search internal documentation.",
  "category": "mcp",
  "visibility": "static",
  "source": "mcp",
  "source_id": "100",
  "source_name": "docs-mcp"
}
```

`name` is the grant name users save in `permissions.allowed_tools`. For MCP
tools it is not the raw MCP tool name unless the naming adapter intentionally
produces that exact value. Runtime MCP invocation must use the same Coze-owned
naming adapter so UI grants, model-visible names, executable names, audit
records, and policy checks cannot drift.

The response must never include:

- MCP `config`;
- MCP `auth`;
- tool `input_schema`;
- endpoint URLs, object keys, or filenames;
- OAuth tokens or secret material;
- tool arguments/results;
- health-check raw payloads;
- provider request/response bodies;
- prompt, model, checkpoint, or transcript content.

## Error And Filtering Behavior

If the provider cannot read the configured tool catalog, the candidate API
returns an error rather than silently presenting a partial grant list. This
keeps users from saving an accidental reduced permission set while MCP
configuration is unavailable.

Invalid provider candidates are filtered at the Skill application boundary.
Static built-ins are appended before provider candidates, so a provider cannot
override core names such as `web_fetch`.

## Non-Goals

- No durable MCP database model change in this slice.
- No OAuth, secret encryption, session lifecycle, or health-check expansion.
- No MCP transport invocation from the Skill candidate API.
- No parameter schema rendering in the Skill drawer.
- No runtime Skill middleware enforcement change.
- No per-user or per-agent policy UI beyond the existing allow-list editor.

## Future Work

The current MCP provider adapts the existing in-memory Workbench MCP catalog.
M6 should replace that backing catalog with durable MCP server/tool storage,
encrypted and masked auth, health status, Tool Registry authorization, and the
Eino MCP adapter execution path. The candidate API shape should remain stable
so frontend permissions and saved Skill JSON do not need another migration.
