# P2-C-GUARDRAIL-AUDIT-001 Guardrail management and audit review

## Scope

Verify whether P2-C still requires new implementation, and close the existing
metadata-only Guardrail audit management path if it already satisfies the
Phase 2 acceptance boundary.

## DeerFlow reference checked

DeerFlow source and docs were checked before making a tracker decision:

- `backend/docs/GUARDRAILS.md`
- `backend/docs/middleware-execution-flow.md`
- `backend/packages/harness/deerflow/guardrails/middleware.py`
- `backend/packages/harness/deerflow/guardrails/provider.py`
- `backend/packages/harness/deerflow/guardrails/builtin.py`
- `backend/packages/harness/deerflow/agents/middlewares/sandbox_audit_middleware.py`
- `backend/packages/harness/deerflow/agents/middlewares/safety_finish_reason_middleware.py`

Observed behavior:

- DeerFlow evaluates tool calls before execution through Guardrail middleware.
- Deny decisions are returned to the agent as bounded tool errors so the loop
  can choose another path.
- Provider failures can fail closed.
- Sandbox audit classifies shell command risk and records audit events.
- Safety finish middleware records content-safety termination metadata.

## Coze implementation verified

Coze already has the P2-C core path in place:

- Guardrail provider/enforcer contracts and fail-mode behavior.
- Pattern and HTTP provider options with sanitized scanner IO.
- Runtime tool, skill-load, and subagent tool guardrail wrappers.
- Confirmation/approval resume support for guardrail interrupts.
- Guardrail decision audit recorder with bounded identifiers.
- Repository-backed guardrail audit list/export APIs:
  - `GET /api/workbench/task_threads/:thread_id/guardrail_audit_events`
  - `GET /api/workbench/task_threads/:thread_id/guardrail_audit_events/export`
- Thread-owner authorization for list/export.
- Task-detail `详情` inspector renders `安全审计`.
- `安全审计` supports refresh, loading, empty, error, metadata rows, and JSON
  export.
- Archive, retention, legal-hold, metrics, and Prometheus runbook coverage
  exists in `docs/superpowers/runbooks/guardrail-audit-operations.md`.

## Safety boundary

The visible/API boundary is metadata-only. The reviewed code and tests keep
these out of the API/UI/export surface:

- prompts
- model input/output
- tool arguments/results
- checkpoint bytes
- object URIs, filenames, raw URLs, and raw storage keys
- credentials and bearer tokens
- raw scanner/provider payloads
- raw repository errors

The visible fields are bounded metadata: action, event type, target type,
bounded target ID, operation, source, fail mode, provider, reason code, rule
IDs, run/thread/space/actor IDs, and created time.

## Verification

```bash
cd backend
go test ./application/agentthread ./domain/agentthread/repository ./api/handler/coze -run 'Guardrail|ListTaskThreadGuardrailAuditEvents|ExportTaskThreadGuardrailAuditEvents' -count=1
```

Passed for application, repository, and handler packages.

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "guardrail audit"
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts
```

The task-detail guardrail audit tests passed. The full task service test file
passed all 14 tests.

## Decision

No new implementation is needed for P2-C in this slice. The existing Guardrail
management/audit path is already wired and verified, so the tracker can mark
P2-C complete with this evidence. Future broader operator policy editing can be
reopened as a new P2/P3 item only if the user asks for a dedicated admin
configuration page beyond the current DeerFlow-compatible audit workflow.
