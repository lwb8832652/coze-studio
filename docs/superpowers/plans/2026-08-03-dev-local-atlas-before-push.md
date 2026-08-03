# dev Local Atlas Before Push Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one audited local command that validates and applies dev Atlas migrations before pushing an exact `dev` SHA, while GitHub and Baota complete every post-push deployment step without database access from Actions.

**Architecture:** `deploy/dev/publish-dev.sh` owns the local validate/status/apply/push transaction and consumes a mode-`600` env file outside the repository. `.github/workflows/deploy-dev.yml` keeps image-baseline checks and immutable image verification, removes the remote migration job, and promotes/deploys after verification for both push and eligible dispatch runs. `AGENTS.md`, the integration runbook, and the dev deployment README make the local command the only Codex entry for pushing `dev`.

**Tech Stack:** Bash, Ruby YAML contract tests, Git, Docker, Atlas Community `v1.2.3`, GitHub Actions, ACR, Baota WebHook.

---

## File Structure

- Create `deploy/dev/publish-dev.sh`: validate the exact audited Git state, run pinned Atlas locally, recheck the remote race, and push the exact target SHA.
- Create `deploy/dev/tests/publish_dev_test.sh`: isolated fake-Git/fake-Docker behavioral tests for the local publishing command.
- Modify `.github/workflows/deploy-dev.yml`: remove GitHub Atlas execution and let verified pushes continue through promotion and deployment.
- Modify `deploy/dev/tests/workflow_contract_test.sh`: encode the new job graph and forward-dispatch retry behavior; remove extracted remote-Atlas tests.
- Modify `AGENTS.md`: require `publish-dev.sh` for Codex `dev` pushes after the second audit confirmation.
- Modify `docs/superpowers/runbooks/dev-integration-audit.md`: move Atlas execution and authorization from Actions into the pre-push local gate.
- Modify `deploy/dev/README.md`: document the local env file, one-command release, failures, and post-push responsibility boundary.
- Modify `docs/superpowers/context/project-context.md`: replace the GitHub-hosted migration fact with the local pre-push fact.
- Modify `docs/superpowers/specs/2026-08-02-dev-automatic-atlas-migration-design.md`: mark the previous direct-Actions migration design superseded.
- Modify `docs/superpowers/plans/2026-08-02-dev-automatic-atlas-migration.md`: mark the previous implementation plan superseded.

### Task 1: Build the Local Publish Command with TDD

**Files:**
- Create: `deploy/dev/tests/publish_dev_test.sh`
- Create: `deploy/dev/publish-dev.sh`

- [ ] **Step 1: Write the failing executable contract test**

Create a test harness with a temporary `bin` directory, a mode-`600` Atlas env file, and fake
`git` and `docker` executables. The fakes must append one sanitized command per line to
`COMMAND_LOG`; fake Docker must never read or log the env file contents.

Use these fixed test revisions:

```bash
EXPECTED_ORIGIN=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
TARGET_SHA=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
OTHER_SHA=cccccccccccccccccccccccccccccccccccccccc
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
```

The success assertion must require this command ordering:

```bash
assert_order "$COMMAND_LOG" \
  'git fetch --no-tags origin dev' \
  'docker run --rm -v' \
  'migrate validate --dir file:///migrations' \
  'migrate status --config file:///atlas.hcl --env dev' \
  'migrate apply --config file:///atlas.hcl --env dev' \
  'git fetch --no-tags origin dev' \
  "git push origin $TARGET_SHA:refs/heads/dev"
```

Add focused cases with these exact outcomes:

```bash
run_failure_case invalid-arguments invalid-arguments no-docker no-push
run_failure_case wrong-branch wrong-branch no-docker no-push
run_failure_case dirty-worktree dirty-worktree no-docker no-push
run_failure_case wrong-head wrong-head no-docker no-push
run_failure_case stale-origin stale-origin no-docker no-push
run_failure_case unrelated-target unrelated-target no-docker no-push
run_failure_case missing-env missing-env no-docker no-push
run_failure_case symlink-env symlink-env no-docker no-push
run_failure_case repo-env repo-env no-docker no-push
run_failure_case insecure-env-mode insecure-env-mode no-docker no-push
run_failure_case invalid-env-key invalid-env-key no-docker no-push
run_failure_case empty-atlas-url empty-atlas-url no-docker no-push
run_failure_case invalid-atlas-scheme invalid-atlas-scheme no-docker no-push
run_failure_case validate-failure validate-failure one-docker no-push
run_failure_case status-failure status-failure two-docker no-push
run_failure_case apply-failure apply-failure three-docker no-push
run_failure_case second-fetch-race second-fetch-race three-docker no-push
run_failure_case push-failure push-failure three-docker attempted-push
```

The success case must also reject any `gh`, `curl`, `workflow_dispatch`, `hook`, plaintext
`mysql://` URL, `--url`, or force-push token in the command log.

- [ ] **Step 2: Run the new test and verify RED**

Run:

```bash
bash deploy/dev/tests/publish_dev_test.sh
```

Expected: nonzero with `publish dev test failure: publish-dev.sh is missing`.

- [ ] **Step 3: Implement the minimal publishing script**

Use this public interface and fixed paths:

```bash
#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
MIGRATIONS_DIR=$REPO_ROOT/docker/atlas/migrations
ATLAS_CONFIG=$REPO_ROOT/.github/atlas-dev.hcl
ATLAS_ENV_FILE=${ATLAS_ENV_FILE:-${HOME:?HOME is required}/.config/coze-studio/dev-atlas.env}
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
GIT_BIN=${GIT_BIN:-git}
DOCKER_BIN=${DOCKER_BIN:-docker}
```

Add helpers with these responsibilities:

```bash
is_revision() { [[ "${1:-}" =~ ^[0-9a-fA-F]{40}$ ]]; }
normalize_revision() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }
git_cmd() { "$GIT_BIN" "$@"; }
docker_cmd() { "$DOCKER_BIN" "$@"; }
error() { printf '[publish-dev] error: %s\n' "$*" >&2; }
log() { printf '[publish-dev] %s\n' "$*"; }
```

`main` must accept exactly `EXPECTED_ORIGIN_DEV_SHA TARGET_DEV_SHA`, normalize both, and
perform this sequence:

```bash
current_branch=$(git_cmd symbolic-ref --quiet --short HEAD)
[ "$current_branch" = dev ]
[ -z "$(git_cmd status --porcelain)" ]
[ "$(normalize_revision "$(git_cmd rev-parse HEAD)")" = "$target_sha" ]
git_cmd fetch --no-tags origin dev
[ "$(normalize_revision "$(git_cmd rev-parse refs/remotes/origin/dev)")" = "$expected_origin_sha" ]
git_cmd cat-file -e "${target_sha}^{commit}"
git_cmd merge-base --is-ancestor "$expected_origin_sha" "$target_sha"
```

Resolve `ATLAS_ENV_FILE` without following a repository-relative shortcut. Reject symlinks,
non-regular files, paths under `REPO_ROOT`, and modes other than `600`. Parse without sourcing:
allow blank lines and `#` comments, require exactly one nonempty
`ATLAS_URL=mysql://MIGRATION_USER:URL_ENCODED_PASSWORD@DEV_MYSQL_HOST:PORT/DEV_DATABASE`
line,
and reject every other key. Never print the parsed value.

Run Atlas in this exact order:

```bash
docker_cmd run --rm \
  -v "$MIGRATIONS_DIR:/migrations:ro" \
  "$ATLAS_IMAGE" \
  migrate validate --dir file:///migrations

docker_cmd run --rm \
  --env-file "$atlas_env_file" \
  -v "$MIGRATIONS_DIR:/migrations:ro" \
  -v "$ATLAS_CONFIG:/atlas.hcl:ro" \
  "$ATLAS_IMAGE" \
  migrate status --config file:///atlas.hcl --env dev

docker_cmd run --rm \
  --env-file "$atlas_env_file" \
  -v "$MIGRATIONS_DIR:/migrations:ro" \
  -v "$ATLAS_CONFIG:/atlas.hcl:ro" \
  "$ATLAS_IMAGE" \
  migrate apply --config file:///atlas.hcl --env dev
```

After apply, repeat the clean-worktree, HEAD, fetch, remote-SHA, commit-object, and ancestor
checks. Push only the authorized object:

```bash
git_cmd push origin "$target_sha:refs/heads/dev"
log "pushed audited dev revision $target_sha; GitHub and Baota own all remaining steps"
```

The script must not contain `gh`, `curl`, a GitHub API URL, `workflow_dispatch`, `--force`,
`--force-with-lease`, or a literal database URL.

- [ ] **Step 4: Run the publish test and verify GREEN**

Run:

```bash
bash deploy/dev/tests/publish_dev_test.sh
bash -n deploy/dev/publish-dev.sh deploy/dev/tests/publish_dev_test.sh
```

Expected: `publish dev contract: passed`; Bash syntax exits `0`.

- [ ] **Step 5: Commit the local command**

```bash
git add deploy/dev/publish-dev.sh deploy/dev/tests/publish_dev_test.sh
git commit -m "feat: gate dev pushes with local atlas"
```

### Task 2: Simplify the GitHub Deployment State Machine with TDD

**Files:**
- Modify: `deploy/dev/tests/workflow_contract_test.sh`
- Modify: `.github/workflows/deploy-dev.yml`

- [ ] **Step 1: Change the workflow contract first**

Set the exact job set and prohibitions:

```ruby
expected_jobs = %w[preflight build-server build-web deployment-blocked verify-images promote deploy]
assert_contract(jobs.keys.sort == expected_jobs.sort,
                'workflow jobs must match the post-push state machine')
assert_contract(!jobs.key?('migration-hold'), 'migration-hold must not exist')
assert_contract(!jobs.key?('migrate'), 'GitHub must not execute Atlas')
workflow_text = File.read(workflow_path)
%w[ATLAS_URL ATLAS_CA_PEM migrate\ apply migrate\ status arigaio/atlas].each do |token|
  assert_contract(!workflow_text.include?(token.gsub('\\ ', ' ')),
                  "workflow must not contain #{token}")
end
```

Replace the promote dependency assertion with:

```ruby
assert_contract(
  needs(promote) == %w[preflight build-server build-web verify-images],
  'promote dependencies are incomplete'
)
assert_contract(!promote_if.include?('needs.migrate'),
                'promotion must not wait for a GitHub migration job')
```

Delete the old extraction and execution tests for the inline migration step. Change the semantic
forward-migration dispatch result to:

```bash
assert_output "$DISPATCH_MIGRATION_OUTPUT" 'migration_changed=true' \
  'forward migration dispatch did not preserve migration diagnostics'
assert_output "$DISPATCH_MIGRATION_OUTPUT" 'deployment_blocked=false' \
  'forward migration dispatch was blocked after the local pre-push gate'
```

- [ ] **Step 2: Run the workflow test and verify RED**

Run:

```bash
bash deploy/dev/tests/workflow_contract_test.sh
```

Expected: nonzero because `migrate` still exists and `promote` still depends on it.

- [ ] **Step 3: Implement the minimal workflow change**

In the dispatch-forward migration branch, preserve diagnostics and release the deployment:

```bash
if git diff --quiet "$deployed_revision" "$target_sha" -- docker/atlas/migrations; then
  migration_changed=false
  deployment_blocked=false
else
  diff_status=$?
  if [ "$diff_status" -eq 1 ]; then
    migration_changed=true
    deployment_blocked=false
  else
    echo "forward dispatch migration diff failed; blocking deployment" >&2
  fi
fi
```

Delete the complete `migrate:` job. Set `promote.needs` to:

```yaml
needs: [preflight, build-server, build-web, verify-images]
```

Remove `needs.migrate.result == 'success'` from `promote.if`. Keep all image verification,
immutable tags, dual promotion, concurrency, and `deployment-blocked` behavior unchanged.

- [ ] **Step 4: Run the workflow tests and verify GREEN**

Run:

```bash
bash deploy/dev/tests/workflow_contract_test.sh
ruby -e "require 'yaml'; YAML.safe_load(File.read('.github/workflows/deploy-dev.yml'), aliases: true); puts 'workflow yaml: passed'"
```

Expected: `workflow contract: passed`, `workflow semantic contract: passed`, and
`workflow yaml: passed`.

- [ ] **Step 5: Commit the workflow change**

```bash
git add .github/workflows/deploy-dev.yml deploy/dev/tests/workflow_contract_test.sh
git commit -m "fix: remove remote dev migration hold"
```

### Task 3: Update the Operational Contract

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/runbooks/dev-integration-audit.md`
- Modify: `deploy/dev/README.md`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/specs/2026-08-02-dev-automatic-atlas-migration-design.md`
- Modify: `docs/superpowers/plans/2026-08-02-dev-automatic-atlas-migration.md`

- [ ] **Step 1: Add the short repository rule to AGENTS.md**

Add under `dev 集成门禁`:

```markdown
远程 `dev` 发布禁止 Codex 直接执行 `git push origin dev`。第二次审计确认必须同时
覆盖报告中的本地 Atlas forward apply 和 exact-SHA push；确认后只能调用
`deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"`。该脚本
push 成功后
立即结束，后续镜像构建、晋级和宝塔部署完全由 GitHub Actions 与服务器完成。
```

- [ ] **Step 2: Rewrite the runbook migration and push sections**

Require the second report to include the ACR comparison base, exact migration list, SQL side
effects, old-app compatibility, local env-file presence/mode without reading its value, Atlas
status evidence, and both script arguments. State that the second confirmation authorizes only
the listed local forward apply and exact push.

Replace the direct push command with:

```bash
: "${AUDITED_ORIGIN_DEV_SHA:?set from the second audit report}"
: "${AUDITED_TARGET_DEV_SHA:?set from the second audit report}"
deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

Remove GitHub runner TLS, `ATLAS_URL` Repository Secret, remote `migrate` job, and post-push Atlas
log checks. Keep separate authorization for baseline, repair, data backfill, retry, rollback, and
all unlisted database operations.

- [ ] **Step 3: Rewrite the dev deployment README**

Document this local file without a real credential:

```text
~/.config/coze-studio/dev-atlas.env
ATLAS_URL=mysql://MIGRATION_USER:URL_ENCODED_PASSWORD@DEV_MYSQL_HOST:PORT/DEV_DATABASE
```

Document mode setup:

```bash
mkdir -p ~/.config/coze-studio
chmod 700 ~/.config/coze-studio
chmod 600 ~/.config/coze-studio/dev-atlas.env
```

Document the exact release command, its pre-push database mutation, the non-TLS public-network
risk, least-privilege migration account requirement, and the rule that local work ends when push
succeeds. Update daily release, dispatch, failure, and rollback sections to remove remote Atlas.

- [ ] **Step 4: Update long-term facts and supersession notices**

In `project-context.md`, record:

```markdown
dev migration 由双重审计确认后的本地 `publish-dev.sh` 在 push 前执行；脚本固定 Atlas
镜像、校验远程基准并只推送报告中的 exact SHA。Push 成功后 GitHub 不连接数据库，
只负责不可变镜像构建与验证、双 `dev` 标签晋级和宝塔部署。
```

At the top of both 2026-08-02 direct-Actions migration documents, add a dated note that the
2026-08-03 local pre-push design supersedes their operational instructions; retain them only as
history.

- [ ] **Step 5: Scan documentation for contradictory current instructions**

Run:

```bash
rg -n "migration-hold|GitHub-hosted runner|Repository Secret.*ATLAS|needs\.migrate|自动.*Atlas|git push origin dev" \
  AGENTS.md deploy/dev/README.md docs/superpowers/context/project-context.md \
  docs/superpowers/runbooks/dev-integration-audit.md
```

Expected: no active instruction for GitHub-hosted Atlas or direct `git push origin dev`; any
remaining match explains that those paths are forbidden or historical.

- [ ] **Step 6: Commit the operational contract**

```bash
git add AGENTS.md deploy/dev/README.md \
  docs/superpowers/context/project-context.md \
  docs/superpowers/runbooks/dev-integration-audit.md \
  docs/superpowers/specs/2026-08-02-dev-automatic-atlas-migration-design.md \
  docs/superpowers/plans/2026-08-02-dev-automatic-atlas-migration.md
git commit -m "docs: require local atlas before dev push"
```

### Task 4: Run the Full Local Verification Matrix

**Files:**
- Verify: all files changed by Tasks 1-3

- [ ] **Step 1: Run all deployment contracts**

```bash
bash deploy/dev/tests/publish_dev_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
```

Expected: every script exits `0`; deploy tests report all cases passed.

- [ ] **Step 2: Validate Bash and YAML syntax**

```bash
bash -n deploy/dev/publish-dev.sh deploy/dev/deploy.sh deploy/dev/tests/*.sh
ruby -e "require 'yaml'; YAML.safe_load(File.read('.github/workflows/deploy-dev.yml'), aliases: true); puts 'workflow yaml: passed'"
```

Expected: Bash exits `0`; Ruby prints `workflow yaml: passed`.

- [ ] **Step 3: Validate the migration directory with the pinned image**

```bash
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e \
  migrate validate --dir file:///migrations
```

Expected: Atlas reports a valid migration directory and exits `0`. This verification must not
run `migrate apply` against the real dev database.

- [ ] **Step 4: Run repository hygiene and secret scans**

```bash
git diff --check origin/dev...HEAD
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!graphify-out/**' --glob '!.git/**'
rg -n 'mysql://[^[:space:]]+:[^[:space:]@]+@|ATLAS_URL=.*@' \
  AGENTS.md .github deploy/dev docs/superpowers --glob '!**/2026-08-03-dev-local-atlas-before-push.md'
git status --short
```

Expected: no whitespace errors, conflict markers, real DSN, or unstaged implementation files.
Placeholder URLs in documentation must not contain a real host, account, or password.

- [ ] **Step 5: Request an independent code review**

Review the exact `origin/dev...HEAD` diff for security, database ordering, retry behavior,
workflow dependency mistakes, secret exposure, and missing tests. Resolve findings on the feature
branch, rerun the full matrix, and commit each verified fix.

### Task 5: Complete Both dev Integration Gates

**Files:**
- Follow: `docs/superpowers/runbooks/dev-integration-audit.md`

- [ ] **Step 1: Perform the first feature-branch audit**

Fetch `origin/dev`, prove the feature contains the latest remote base, review every commit and
file, rerun Task 4, inspect all migration SQL in the actual ACR-deployed revision to target range,
and report the exact branch SHA. Stop for the first explicit confirmation.

- [ ] **Step 2: Merge only the audited SHA into local dev**

After confirmation, re-fetch and verify the reported remote SHA is unchanged. Merge the exact
audited feature SHA into the clean local `dev` worktree with `--no-ff`. Abort on conflict and
return to the feature branch.

- [ ] **Step 3: Perform the second merged-dev audit**

From merged local `dev`, rerun every test, ACR revision lookup, migration-range review, Atlas
directory validation, local env-file presence/mode check without reading its value, and remote
race check. Report `AUDITED_ORIGIN_DEV_SHA` and `AUDITED_TARGET_DEV_SHA` with their concrete
40-hex values as the only permitted arguments. Stop for the second explicit confirmation.

- [ ] **Step 4: Execute the one local release command**

Only after the second confirmation, run:

```bash
: "${AUDITED_ORIGIN_DEV_SHA:?set from the confirmed second audit report}"
: "${AUDITED_TARGET_DEV_SHA:?set from the confirmed second audit report}"
deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

Expected: local Atlas validate/status/apply succeeds, the exact SHA push succeeds, and the script
returns immediately. If any step fails, stop and follow the runbook; never substitute a direct or
force push.

- [ ] **Step 5: Verify post-push remote evidence without running local deployment work**

Observe the GitHub run and report its URL/status. Verify the target immutable server/web images,
both promoted `:dev` revisions, Baota webhook result, and `/healthz` revision. Observation and
reporting are allowed; do not run another local migration, dispatch, server command, or retry
without a new explicit authorization.
