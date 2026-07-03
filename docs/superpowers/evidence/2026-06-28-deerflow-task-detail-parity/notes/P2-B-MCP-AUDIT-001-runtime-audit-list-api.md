# P2-B-MCP-AUDIT-001 MCP runtime audit list API

## Scope

Expose MCP runtime invocation history as bounded metadata for production
operations. This is a P2 hardening API, not a task-detail visible parity
change.

## Coze implementation

- API: `GET /api/workbench/task_threads/:thread_id/mcp_runtime_audit_events`
- IDL: `ListTaskThreadMCPRuntimeAuditEvents`
- Application service: `ApplicationService.ListMCPRuntimeAuditEvents`
- Storage: `agent_mcp_runtime_audit_events`
- Default authorization: thread owner via `ThreadOwnerMCPRuntimeAuditAuthorizer`

## Returned fields

- `event_id`
- `space_id`
- `thread_id`
- `run_id`
- `server_id`
- `runtime_tool_name`
- `event_type`
- `error_code`
- `elapsed_millis`
- `output_bytes`
- `created_at`

## Safety boundary

The API intentionally does not return tool arguments, tool results, prompts,
object URIs, credentials, provider raw payloads, checkpoint bytes, or filesystem
paths. Repository listing is scoped by `thread_id` and optional `run_id`.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestApplicationListMCPRuntimeAuditEventsReturnsMetadataOnlyPage|TestApplicationADKMCPRuntimeAuditRecorder' -count=1
go test ./domain/agentthread/repository -run 'TestMCPRuntimeAuditRepository' -count=1
go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestListTaskThreadMCPRuntimeAuditEventsHandler' -count=1
go test ./api/router/coze -run 'Test' -count=1
```

All targeted commands passed after adding the thread-scoped repository filter.
The broader `go test ./api/handler/coze -run Test -count=1` command still
depends on legacy workflow/MySQL fixtures and is not used as this slice's
acceptance gate.
