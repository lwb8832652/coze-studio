# Sandbox Control Plane Acceptance Evidence

Date: 2026-07-18

## Conclusion

The Sandbox control-plane implementation, runtime routing integration, automated
regression suite, local build, administrator route, and fail-closed disabled
state are accepted.

This is not yet a production Provider rollout sign-off. A controlled remote
Provider endpoint, persistent credential keyring, and exclusive MySQL
integration DSN were not available, so real Provider CRUD and workload E2E
remain environment-blocked rather than reported as passed.

## Automated gates

Backend regression and build:

```bash
cd backend
go test -gcflags="all=-l -N" \
  ./pkg/lang/conv ./pkg/safehttp ./domain/sandbox ./infra/sandbox \
  ./application/sandbox ./infra/appdev ./application/appdev \
  ./infra/coderunner/... ./domain/workflow/internal/nodes/code \
  ./bizpkg/llm/modelbuilder ./api/middleware ./api/handler/coze \
  ./api/router/coze ./application ./application/workbench \
  ./application/agentthread ./application/mcpruntime -count=1
cd ..
make build_server
```

Result: all selected packages passed and the server binary built successfully.

Frontend system-management regression:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__
npx tsc --noEmit --project tsconfig.json
```

Result: 15 test files and 99 tests passed; TypeScript completed with exit code
zero.

Migration integrity:

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Result: migration hash and validation passed.

## In-app browser acceptance

Acceptance URL: `http://localhost:8080/system/sandbox`

Acceptance identity: local administrator test account
`840582614@qq.com`.

Verified visible and interactive states:

- The system-management shell is accessible to the administrator.
- The `沙箱管理` secondary menu is present.
- Provider summary, Agent/MCP/AppDev default projections, filters, retry state,
  and pagination render.
- With `SANDBOX_CONTROL_PLANE_ENABLED` disabled, the capability banner shows
  `CONTROL_PLANE_DISABLED`.
- The create-Provider action is disabled while the control plane is unavailable.
- Provider form validation rejects a missing name, non-HTTPS remote endpoint,
  and missing remote credential before submission.
- `local_debug` creation follows the server capability and is disabled when the
  server does not declare it available.
- The page reports zero browser-console errors after the final refresh.
- A non-admin session was separately observed receiving the system-management
  permission-denied page and HTTP 403 before administrator bootstrap was
  enabled.

The in-app browser screenshot operation timed out at the browser-plugin layer on
both full-page and viewport attempts. DOM accessibility snapshots, real clicks,
visible validation messages, URL state, and console logs were retained as the
acceptance evidence. No external browser was substituted.

## Security findings closed during acceptance

- Successful HTTP access logs no longer print URL queries, request bodies, or
  response bodies. They retain status, latency, method, path, handler, client,
  locale, and log ID.
- `DebugJsonToStr` recursively redacts password, API key, access/secret key,
  credential, authorization, cookie, token, client-secret, and private-key
  fields while retaining non-sensitive debug metadata.
- Ark model construction no longer logs the complete model configuration.
- Administrator bootstrap through `COZE_SYSTEM_ADMIN_EMAILS` is accepted only
  while the persisted basic configuration is absent. Persisted configuration
  remains authoritative afterward.
- Custom Sandbox, MCP, task, workspace, and LangGraph routes are registered
  through the authenticated route groups and retain server-side authorization.

## Environment-blocked acceptance items

The following items must remain open until a controlled Provider environment is
supplied:

- Create, edit, enable, disable, rotate credentials, set defaults, delete, and
  inspect audit events for a real remote Provider.
- Execute real Agent, MCP stdio, AppDev build/preview, and workflow code
  workloads through the selected Provider.
- Exercise healthy, degraded, unhealthy, timeout, capacity, credential failure,
  and failover behavior against a real Provider.
- Prove credential key rotation and restart recovery with the deployment
  keyring.
- Run MySQL integration tests with a safe, exclusive
  `SANDBOX_MYSQL_INTEGRATION_DSN`.

Required inputs for that gate:

- An HTTPS remote Provider endpoint dedicated to acceptance.
- A non-production Provider credential.
- Persistent `SANDBOX_CREDENTIAL_KEYS_JSON` and
  `SANDBOX_CREDENTIAL_ACTIVE_KEY_ID` values managed outside tracked files.
- The approved endpoint allowlist and Provider authentication policy.
- An exclusive integration database DSN that may be mutated by tests.

## Local service state

- Frontend: `http://localhost:8080`
- Backend: `http://localhost:8888`
- Backend mode: `APP_ENV=debug`
- Sandbox management plane: disabled, intentionally exercising fail-closed
  behavior

