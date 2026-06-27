# Skill Tool Candidates Design

## Goal

M5.7 gives the Skill permission editor a backend-owned list of safe runtime
tool grant candidates. Users no longer need to memorize tool names such as
`web_fetch`, and the frontend stops inventing candidate names independently.

## Scope

- Add `GET /api/workbench/skills/tool_candidates?space_id=...`.
- Return only safe metadata: name, display name, description, category, and
  visibility.
- Seed the candidate list from the current Go ADK built-ins:
  - `web_fetch`
  - `web_search`
  - `ask_user_clarification`
  - `request_human_confirmation`
- Add frontend API schema and service wiring.
- Load candidates in the Skill version drawer permission editor.
- Render compact candidate buttons that toggle tool names in
  `permissions.allowed_tools`.
- Keep the advanced text area as an escape hatch for future tools not yet
  exposed by the candidate API.

## Non-Goals

- No complete Tool Registry in this slice.
- No MCP OAuth/secret/session configuration.
- No run-specific availability evaluation.
- No parameter schema or endpoint exposure.
- No runtime enforcement change; `ADKToolPolicyProvider` remains the
  execution-time filter.

## API Contract

`ListSkillToolCandidates` requires `space_id` for future authorization and
tenant scoping. In M5.7 it returns a static, sanitized list because the durable
Tool Registry is not complete yet. Future registry-backed implementations must
keep the response metadata-only and policy-filtered.

The response must not include:

- input schema;
- executable arguments;
- provider endpoints;
- object keys or URLs;
- OAuth/secrets;
- raw MCP server config;
- health-check payloads;
- prompt/model/tool result content.

## Frontend Behavior

The version drawer loads candidates by `skill.space_id`. If loading fails, the
drawer keeps the text editor usable and shows a bounded error. Candidate
buttons toggle their corresponding safe tool name in the allowed-tools text
area. The existing save path still serializes `permissions` through the M5.6
logic, so unknown keys are preserved and tool-name validation remains shared.

## Future Expansion

The static list is an adapter boundary, not the final registry. Future M5/M6
work should source the same shape from Coze-owned Tool Registry and MCP
configuration after authorization, policy, audit, health, and secret masking
are implemented.
