# Local Debug And Test Runbook

This runbook holds local-only setup details that are useful for DeerFlow
parity testing. Keep secrets out of tracked files unless the user explicitly
asks otherwise for a test-only account.

## Test Accounts

Coze Studio and the local DeerFlow reference use the same functional test
account for browser parity verification:

- Email: `840582614@qq.com`
- Password: `z8832652`

Reference task-detail page in DeerFlow:

- `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`

Local Coze Studio development targets:

- Frontend: `http://localhost:8080`
- Backend API: `http://localhost:8888`

## Debug Environment

Use `APP_ENV=debug` when starting the local backend from `bin`; otherwise the
server can load `bin/.env` instead of `bin/.env.debug` and reject requests that
select `{"runtime":"eino_adk"}`.

Recommended local backend command from `bin`:

```bash
APP_ENV=debug \
AGENT_THREAD_RUNTIME_DEFAULT=eino_adk \
AGENT_THREAD_EINO_ADK_ENABLED=true \
AGENT_THREAD_WORKER_ENABLED=true \
AGENT_THREAD_WORKER_INTERVAL_MS=2000 \
./opencoze -start
```

Confirm the startup log contains:

```text
load env file: .env.debug
```

### Local Web Search Proxy

DeerFlow's Python/DDGS path can pick up the host proxy in the local Docker
environment, but Go `net/http` does not automatically read macOS system proxy
settings. If local `web_search` hangs or reports provider request failures,
put explicit proxy variables in the ignored `bin/.env.debug` used by the
backend:

```bash
export HTTP_PROXY="http://127.0.0.1:7893"
export HTTPS_PROXY="http://127.0.0.1:7893"
export NO_PROXY="localhost,127.0.0.1,::1"
export http_proxy="http://127.0.0.1:7893"
export https_proxy="http://127.0.0.1:7893"
export no_proxy="localhost,127.0.0.1,::1"
```

Keep this local-only. `bin/.env.debug` is ignored, and production/test
environments should configure their own approved outbound proxy or search
provider instead of relying on a developer machine port.

## Debug MySQL

- Do not start or pull the local MySQL image for the normal debug path.
- Debug middleware should use the external MySQL-compatible test database
  configured in ignored env files such as `docker/.env.debug` and
  `bin/.env.debug`.
- Keep real MySQL passwords in ignored env files. Do not paste full
  `docker compose config --env-file docker/.env.debug` output into chat or
  logs, because Compose can print resolved credentials.
- `make env` should keep the non-secret P0 runtime switches normalized in
  `docker/.env.debug`:
  - `AGENT_THREAD_RUNTIME_DEFAULT=eino_adk`
  - `AGENT_THREAD_EINO_ADK_ENABLED=true`
  - `AGENT_THREAD_WORKER_ENABLED=true`

## Atlas CLI

Use the local Atlas CLI when available. This worktree expects Atlas Community
`v0.35.0`.

```bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Only fall back to the pinned Docker image or temporary installer if local
Atlas is missing or no longer reports `v0.35.0`.

Never hand-edit `docker/atlas/migrations/atlas.sum`.

## Branch And Test Promotion

- Day-to-day Codex work stays on `codex/deerflow-parity-mainline` unless the
  user explicitly chooses another branch.
- `dev` is the test-environment validation branch. Do not keep this Codex
  worktree checked out on `dev`, because the user's local tools may need that
  branch.
- Before promoting to test, commit the development branch and let the user
  review the diff.
- Promotion means merging the development branch into `dev`, pushing
  `origin/dev`, and switching this worktree back to the development branch.
- If `dev` is already used by another worktree, report the conflicting path
  instead of forcing checkout.
