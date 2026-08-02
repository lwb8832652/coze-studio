# Dev Automatic Atlas Migration Implementation Plan

> **Compatibility update (2026-08-03):** Private CA review showed that Atlas 0.35.0 does not consume the MySQL `ssl-ca` parameter. The current implementation therefore pins Atlas 1.2.3 by full tag and manifest digest.
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an `origin/dev` push automatically apply verified Atlas migrations before promoting ACR images and invoking the Baota webhook.

**Architecture:** Split preflight's current overloaded migration flag into `migration_changed` and `deployment_blocked`, so only a proven Git migration diff can reach the database. Build and verify both immutable images first, run Atlas from a dedicated GitHub Actions job using the `ATLAS_URL` secret, and require that job to succeed before promotion and deployment.

> **Implementation status (2026-08-02):** Complete. The executable contract is the current
> `.github/workflows/deploy-dev.yml`, its workflow contract tests, and the dev integration
> runbook. The task snippets below describe the TDD progression; when a snippet is abbreviated,
> do not use it in place of the finalized workflow.

**Tech Stack:** GitHub Actions YAML, Bash, Ruby YAML contract tests, Atlas Community 1.2.3, Docker, Git.

---

## File Map

- Modify `.github/workflows/deploy-dev.yml`: separate preflight states, add the Atlas job, and update promotion dependencies.
- Modify `deploy/dev/tests/workflow_contract_test.sh`: statically and semantically test the new workflow state machine and Atlas command behavior.
- Modify `deploy/dev/README.md`: document `ATLAS_URL`, automatic migration order, failure behavior, and the one-time revision baseline prerequisite.
- Modify `docs/superpowers/context/project-context.md`: replace the manual migration hold as the current long-term dev deployment fact.
- Modify `docs/superpowers/runbooks/dev-integration-audit.md`: make automatic dev database mutation an explicit second-gate side effect for the exact target SHA.
- Modify `docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md`: mark the old manual migration sections as superseded by the approved 2026-08-02 design.

### Task 1: Add failing preflight state tests

**Files:**
- Modify: `deploy/dev/tests/workflow_contract_test.sh`
- Test: `deploy/dev/tests/workflow_contract_test.sh`

- [ ] **Step 1: Require the separate preflight output**

Replace the existing preflight output assertion with:

```ruby
assert_contract(
  preflight.fetch('outputs', {}).keys.sort ==
    %w[deployment_blocked migration_changed target_sha],
  'preflight must expose target_sha, migration_changed, and deployment_blocked'
)
assert_contract(preflight_text.include?('deployment_blocked=true'),
                'preflight must fail closed with deployment_blocked')
assert_contract(preflight_text.include?('deployment_blocked=false'),
                'preflight must explicitly release verified deployments')
```

Remove the assertion that treats `migration_changed=true` as the generic fail-closed result.

- [ ] **Step 2: Make the Docker baseline mock express independent failure modes**

Replace the mock's `docker pull` and `docker image inspect` branches with:

```bash
if [ "$1" = pull ]; then
  image=${@: -1}
  case "${DOCKER_MODE:-deployed}" in
    deployed|inconsistent) exit 0 ;;
    missing)
      printf "Error response from daemon: manifest unknown: manifest unknown\n" >&2
      exit 1
      ;;
    server-missing)
      if [[ "$image" == *coze-server:dev ]]; then
        printf "Error response from daemon: manifest unknown: manifest unknown\n" >&2
        exit 1
      fi
      exit 0
      ;;
    registry-error)
      printf "Error response from daemon: registry connection timed out\n" >&2
      exit 1
      ;;
    *) exit 98 ;;
  esac
fi
if [ "$1" = image ] && [ "$2" = inspect ]; then
  image=${@: -1}
  if [ "${DOCKER_MODE:-deployed}" = inconsistent ] &&
    [[ "$image" == *coze-web:dev ]]; then
    printf "%s\n" "$ALTERNATE_REVISION"
  else
    printf "%s\n" "$DEPLOYED_REVISION"
  fi
  exit 0
fi
```

Change `run_preflight` to accept an explicit deployed revision and export the alternate revision:

```bash
run_preflight() {
  docker_mode=$1
  deployed_revision=$2
  before_revision=$3
  target_revision=$4
  output_file=$5

  : > "$output_file"
  (
    cd -- "$TEST_REPO"
    PATH="$TEST_BIN:$PATH" \
      DOCKER_MODE="$docker_mode" \
      DEPLOYED_REVISION="$deployed_revision" \
      ALTERNATE_REVISION="$TARGET_REVISION" \
      GITHUB_EVENT_NAME=push \
      GITHUB_SHA="$target_revision" \
      TARGET_SHA_INPUT= \
      BEFORE_SHA="$before_revision" \
      SERVER_DEV_IMAGE=registry.example/coze-server:dev \
      WEB_DEV_IMAGE=registry.example/coze-web:dev \
      GITHUB_OUTPUT="$output_file" \
      bash "$RESOLVE_SCRIPT"
  )
}
```

- [ ] **Step 3: Assert actual migration and blocking states independently**

Replace the existing semantic preflight calls with:

```bash
run_preflight deployed "$DEPLOYED_REVISION" "$BEFORE_REVISION" \
  "$TARGET_REVISION" "$GITHUB_OUTPUT_FILE"
assert_output "$GITHUB_OUTPUT_FILE" "target_sha=$TARGET_REVISION" \
  'semantic preflight wrote the wrong target SHA'
assert_output "$GITHUB_OUTPUT_FILE" 'migration_changed=true' \
  'verified deployed baseline did not discover the pending migration'
assert_output "$GITHUB_OUTPUT_FILE" 'deployment_blocked=false' \
  'verified migration diff was incorrectly blocked'

NO_MIGRATION_OUTPUT=$SEMANTIC_ROOT/no-migration-output
run_preflight deployed "$BEFORE_REVISION" "$BEFORE_REVISION" \
  "$TARGET_REVISION" "$NO_MIGRATION_OUTPUT"
assert_output "$NO_MIGRATION_OUTPUT" 'migration_changed=false' \
  'ordinary follow-up was mistaken for a migration'
assert_output "$NO_MIGRATION_OUTPUT" 'deployment_blocked=false' \
  'ordinary verified deployment was blocked'

BOOTSTRAP_OUTPUT=$SEMANTIC_ROOT/bootstrap-output
run_preflight missing "$DEPLOYED_REVISION" "$BEFORE_REVISION" \
  "$TARGET_REVISION" "$BOOTSTRAP_OUTPUT"
assert_output "$BOOTSTRAP_OUTPUT" 'migration_changed=false' \
  'manifest-missing bootstrap invented a migration'
assert_output "$BOOTSTRAP_OUTPUT" 'deployment_blocked=false' \
  'verified bootstrap deployment was blocked'

BOOTSTRAP_MIGRATION_OUTPUT=$SEMANTIC_ROOT/bootstrap-migration-output
run_preflight missing "$DEPLOYED_REVISION" "$DEPLOYED_REVISION" \
  "$BEFORE_REVISION" "$BOOTSTRAP_MIGRATION_OUTPUT"
assert_output "$BOOTSTRAP_MIGRATION_OUTPUT" 'migration_changed=true' \
  'manifest-missing bootstrap missed a migration change'
assert_output "$BOOTSTRAP_MIGRATION_OUTPUT" 'deployment_blocked=false' \
  'verified bootstrap migration was incorrectly blocked'

for mode in registry-error server-missing inconsistent; do
  blocked_output=$SEMANTIC_ROOT/$mode-output
  run_preflight "$mode" "$DEPLOYED_REVISION" "$BEFORE_REVISION" \
    "$TARGET_REVISION" "$blocked_output"
  assert_output "$blocked_output" 'migration_changed=false' \
    "$mode was incorrectly classified as a migration"
  assert_output "$blocked_output" 'deployment_blocked=true' \
    "$mode did not fail closed"
done
```

- [ ] **Step 4: Run the contract and observe RED**

Run:

```bash
bash deploy/dev/tests/workflow_contract_test.sh
```

Expected: FAIL because `preflight.outputs.deployment_blocked` and the new output lines do not yet exist.

### Task 2: Implement the preflight state split

**Files:**
- Modify: `.github/workflows/deploy-dev.yml`
- Test: `deploy/dev/tests/workflow_contract_test.sh`

- [ ] **Step 1: Expose the new output**

Set the preflight outputs to:

```yaml
outputs:
  target_sha: ${{ steps.resolve.outputs.target_sha }}
  migration_changed: ${{ steps.resolve.outputs.migration_changed }}
  deployment_blocked: ${{ steps.resolve.outputs.deployment_blocked }}
```

- [ ] **Step 2: Initialize independent states**

In `workflow_dispatch`, set both flags explicitly:

```bash
migration_changed=false
deployment_blocked=false
```

At the start of the push branch, use fail-closed defaults:

```bash
migration_changed=false
deployment_blocked=true
```

- [ ] **Step 3: Release only verified comparisons**

For both the deployed-revision comparison and the two-manifest-missing bootstrap comparison,
use this complete diff result pattern:

```bash
if git diff --quiet "$comparison_base" "$target_sha" -- docker/atlas/migrations; then
  migration_changed=false
  deployment_blocked=false
else
  diff_status=$?
  if [ "$diff_status" -eq 1 ]; then
    migration_changed=true
    deployment_blocked=false
  else
    echo "migration diff failed; blocking deployment" >&2
  fi
fi
```

Set `comparison_base=$deployed_revision` after the two current image revisions pass format,
equality, Git-object, and ancestor validation. Set `comparison_base=$before` only when both
pull failures are recognized as missing manifests. Leave `deployment_blocked=true` for all
other paths and update diagnostics to say `blocking deployment`, not `holding migration`.

- [ ] **Step 4: Write both outputs**

Append:

```bash
printf 'target_sha=%s\n' "$target_sha" >> "$GITHUB_OUTPUT"
printf 'migration_changed=%s\n' "$migration_changed" >> "$GITHUB_OUTPUT"
printf 'deployment_blocked=%s\n' "$deployment_blocked" >> "$GITHUB_OUTPUT"
```

- [ ] **Step 5: Run the preflight contract and observe GREEN**

Run:

```bash
bash deploy/dev/tests/workflow_contract_test.sh
```

Expected: PASS for the static and semantic preflight assertions while the old migration-hold graph remains intact.

- [ ] **Step 6: Commit the state split**

```bash
git add .github/workflows/deploy-dev.yml deploy/dev/tests/workflow_contract_test.sh
git commit -m "fix: separate dev deployment block state"
```

### Task 3: Add failing automatic migration graph tests

**Files:**
- Modify: `deploy/dev/tests/workflow_contract_test.sh`
- Test: `deploy/dev/tests/workflow_contract_test.sh`

- [ ] **Step 1: Replace the expected job set**

Use:

```ruby
expected_jobs = %w[preflight build-server build-web verify-images migrate promote deploy]
assert_contract((expected_jobs - jobs.keys).empty?, 'required jobs are missing')
assert_contract(!jobs.key?('migration-hold'), 'manual migration-hold job must be removed')
```

- [ ] **Step 2: Require image verification before any database access**

Update the verify condition assertions:

```ruby
assert_contract(verify_if.include?('deployment_blocked'),
                'verify-images must stop blocked deployments')
assert_contract(!verify_if.include?("migration_changed != 'true'"),
                'verified migration pushes must reach image verification')
```

- [ ] **Step 3: Add the migration job contract**

Add:

```ruby
migrate = jobs.fetch('migrate')
assert_contract(needs(migrate).sort == %w[preflight verify-images],
                'migrate must wait for preflight and immutable image verification')
migrate_if = migrate['if'].to_s
%w[always preflight verify-images deployment_blocked].each do |token|
  assert_contract(migrate_if.include?(token), "migrate condition is missing #{token}")
end
migrate_text = job_text(migrate)
assert_contract(migrate_text.include?('actions/checkout@v7'),
                'migrate must check out the verified target')
assert_contract(migrate_text.include?('arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'),
                'migrate must use the repository Atlas version')
assert_contract(migrate_text.include?('migrate validate') &&
                migrate_text.include?('migrate apply'),
                'migrate must validate before apply')
assert_contract(migrate_text.include?("github.event_name == 'push'") &&
                migrate_text.include?('migration_changed'),
                'Atlas apply must be limited to verified migration pushes')
migration_step = migrate.fetch('steps', []).find do |step|
  step['name'] == 'Validate and apply Atlas migrations'
end
assert_contract(migration_step.is_a?(Hash), 'Atlas migration step is missing')
assert_contract(migration_step.fetch('env', {})['ATLAS_URL'].to_s.include?('secrets.ATLAS_URL'),
                'migrate must read ATLAS_URL from GitHub Secrets')
assert_contract(!migration_step['run'].to_s.include?('${{ secrets.ATLAS_URL }}'),
                'ATLAS_URL must not be interpolated into shell source')
```

- [ ] **Step 4: Require migration success before promotion**

Replace the promotion dependency and condition assertions with:

```ruby
assert_contract(
  needs(promote).sort == %w[build-server build-web migrate preflight verify-images],
  'promote dependencies are incomplete'
)
%w[always deployment_blocked build-server build-web verify-images migrate].each do |token|
  assert_contract(promote_if.include?(token), "promote condition is missing #{token}")
end
assert_contract(promote_if.include?("needs.migrate.result == 'success'"),
                'promotion must require migration success')
assert_contract(!promote_if.include?("migration_changed != 'true'"),
                'successful automatic migration must permit promotion')
```

Remove `Atlas migration apply` and database-access patterns from the forbidden map. Keep the
SSH-credential and production-deployment checks.

- [ ] **Step 5: Add executable Atlas script tests**

Extract the `Validate and apply Atlas migrations` step to `$MIGRATE_SCRIPT`. Use a fake Docker
binary that records calls and can fail validation:

```bash
MIGRATE_SCRIPT=$SEMANTIC_ROOT/migrate.sh
MIGRATE_DOCKER_LOG=$SEMANTIC_ROOT/migrate-docker.log

ruby - "$WORKFLOW" > "$MIGRATE_SCRIPT" <<'EXTRACT'
require 'yaml'
workflow = YAML.safe_load(File.read(ARGV.fetch(0)), aliases: true)
step = workflow.fetch('jobs').fetch('migrate').fetch('steps').find do |candidate|
  candidate['name'] == 'Validate and apply Atlas migrations'
end
abort 'Atlas migration step is missing' unless step
puts step.fetch('run')
EXTRACT

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'printf "%s\n" "$*" >> "$MIGRATE_DOCKER_LOG"' \
  'if [[ "$*" == *"migrate validate"* ]] &&' \
  '  [ "${MIGRATE_DOCKER_MODE:-success}" = validate-fail ]; then' \
  '  exit 42' \
  'fi' \
  'exit 0' > "$TEST_BIN/docker"
chmod +x "$TEST_BIN/docker"
```

Run these assertions:

```bash
: > "$MIGRATE_DOCKER_LOG"
if PATH="$TEST_BIN:$PATH" MIGRATE_DOCKER_LOG="$MIGRATE_DOCKER_LOG" \
  ATLAS_URL= bash "$MIGRATE_SCRIPT"; then
  printf 'workflow contract failure: empty ATLAS_URL was accepted\n' >&2
  exit 1
fi
[ ! -s "$MIGRATE_DOCKER_LOG" ] || {
  printf 'workflow contract failure: Docker ran with empty ATLAS_URL\n' >&2
  exit 1
}

DUMMY_ATLAS_URL='mysql://contract:masked@example.invalid/dev?tls=true'
: > "$MIGRATE_DOCKER_LOG"
PATH="$TEST_BIN:$PATH" MIGRATE_DOCKER_LOG="$MIGRATE_DOCKER_LOG" \
  ATLAS_URL="$DUMMY_ATLAS_URL" bash "$MIGRATE_SCRIPT"
grep -q 'migrate validate' "$MIGRATE_DOCKER_LOG"
grep -q 'migrate apply' "$MIGRATE_DOCKER_LOG"
[ "$(wc -l < "$MIGRATE_DOCKER_LOG" | tr -d ' ')" -eq 2 ]

: > "$MIGRATE_DOCKER_LOG"
if PATH="$TEST_BIN:$PATH" MIGRATE_DOCKER_LOG="$MIGRATE_DOCKER_LOG" \
  MIGRATE_DOCKER_MODE=validate-fail ATLAS_URL="$DUMMY_ATLAS_URL" \
  bash "$MIGRATE_SCRIPT"; then
  printf 'workflow contract failure: Atlas validation failure was ignored\n' >&2
  exit 1
fi
[ "$(wc -l < "$MIGRATE_DOCKER_LOG" | tr -d ' ')" -eq 1 ] || {
  printf 'workflow contract failure: apply ran after validation failure\n' >&2
  exit 1
}
```

- [ ] **Step 6: Run the contract and observe RED**

Run:

```bash
bash deploy/dev/tests/workflow_contract_test.sh
```

Expected: FAIL because `migrate` is missing and `migration-hold` still exists.

### Task 4: Implement the automatic Atlas job and promotion gate

**Files:**
- Modify: `.github/workflows/deploy-dev.yml`
- Test: `deploy/dev/tests/workflow_contract_test.sh`

- [ ] **Step 1: Let verified migration pushes reach image verification**

Set `verify-images.if` to:

```yaml
if: >-
  always() &&
  needs.preflight.result == 'success' &&
  needs.preflight.outputs.deployment_blocked != 'true' &&
  (
    (
      github.event_name == 'push' &&
      needs.build-server.result == 'success' &&
      needs.build-web.result == 'success'
    ) ||
    (
      github.event_name == 'workflow_dispatch' &&
      needs.build-server.result == 'skipped' &&
      needs.build-web.result == 'skipped'
    )
  )
```

- [ ] **Step 2: Replace `migration-hold` with `migrate`**

Add the `migrate` job after `verify-images` and delete `migration-hold`. The finalized job must
use the exact immutable Atlas image recorded in this plan, validate `ATLAS_URL` as a
`mysql://` URL with exactly one `tls=true`, and enforce the optional `ATLAS_CA_PEM` /
`ssl-ca=/atlas-ca.pem` pairing. Pass only the environment variable name to Docker, read it from
`.github/atlas-dev.hcl`, mount any temporary CA read-only with mode `0600`, and remove it via an
exit trap. Never place the URL value in Docker argv or interpolate either secret into shell
source. Use the current `.github/workflows/deploy-dev.yml` migration step as the executable
reference rather than duplicating that security-sensitive script here.

The no-op step is valid only when preflight explicitly returns `migration_changed == 'false'`.

- [ ] **Step 3: Gate promotion on migration success**

Set promotion dependencies and condition to:

```yaml
needs: [preflight, build-server, build-web, verify-images, migrate]
if: >-
  always() &&
  needs.preflight.result == 'success' &&
  needs.preflight.outputs.deployment_blocked != 'true' &&
  needs.verify-images.result == 'success' &&
  needs.migrate.result == 'success' &&
  (
    (
      github.event_name == 'push' &&
      needs.build-server.result == 'success' &&
      needs.build-web.result == 'success'
    ) ||
    (
      github.event_name == 'workflow_dispatch' &&
      needs.build-server.result == 'skipped' &&
      needs.build-web.result == 'skipped'
    )
  )
```

- [ ] **Step 4: Run the workflow contract and observe GREEN**

Run:

```bash
bash deploy/dev/tests/workflow_contract_test.sh
```

Expected:

```text
workflow contract: passed
workflow semantic contract: passed
```

- [ ] **Step 5: Commit the workflow implementation**

```bash
git add .github/workflows/deploy-dev.yml deploy/dev/tests/workflow_contract_test.sh
git commit -m "feat: automate dev atlas migrations"
```

### Task 5: Update deployment facts and operator instructions

**Files:**
- Modify: `deploy/dev/README.md`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/runbooks/dev-integration-audit.md`
- Modify: `docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md`

- [ ] **Step 1: Update the GitHub configuration table**

Add this README row:

```markdown
| Secret | `ATLAS_URL` | Atlas MySQL URL，仅用于自动迁移 dev 数据库 |
| Secret | `ATLAS_CA_PEM` | 可选；`ATLAS_URL` 使用 `ssl-ca=/atlas-ca.pem` 时提供私有 CA |
```

Replace “不要配置数据库连接串” with text requiring `ATLAS_URL` to remain a Secret, use
a least-privilege dev schema account, require `mysql://` with exactly one `tls=true`, URL-encode
password characters, and never copy it into Variables, `app.env`, logs, or tickets. Apply the
same handling rule to `ATLAS_CA_PEM`.

- [ ] **Step 2: Replace manual migration operations with the automatic sequence**

Document this exact order in `deploy/dev/README.md`:

```text
preflight -> build-server/build-web -> verify-images -> migrate -> promote -> deploy
```

State that real migration diffs run Atlas automatically; baseline uncertainty still blocks;
missing `ATLAS_URL`, connectivity errors, validate failures, and apply failures leave the old
`:dev` labels and running services unchanged. Document the one-time prerequisite that the
remote schema and Atlas revision table must already match the migration directory.

- [ ] **Step 3: Update long-term context**

Replace the manual apply paragraph in `project-context.md` with:

```markdown
Migration 门禁以当前两张已晋级 `dev` 镜像的一致 revision 为基线。只有在基线、
Git 祖先关系和 migration diff 都可验证时，workflow 才区分普通发布与自动 Atlas
apply；基线缺失、不一致或比较失败时继续 fail closed。自动 migration 使用仅注入
迁移 job 的 GitHub Secret，目标镜像验证和 Atlas apply 都成功后才能晋级并部署。
```

- [ ] **Step 4: Update the two-gate authorization contract**

In `dev-integration-audit.md`, require the second report to list every migration file and state
that an exact-SHA push can automatically mutate the dev database. Replace the old exclusion
with wording that the second explicit confirmation authorizes the reported dev Atlas apply,
ACR promotion, and Baota deployment for that SHA only; it still does not authorize production,
down migration, manual retry, backup changes, or unrelated database operations.

- [ ] **Step 5: Mark the old design as superseded**

At the start of sections 2, 3, 6, 7, 9, 10, and 11 in the 2026-07-30 design where manual
migration is stated, add a concise note that automatic dev migration behavior is superseded by
`docs/superpowers/specs/2026-08-02-dev-automatic-atlas-migration-design.md`. Do not rewrite
unrelated deployment architecture.

- [ ] **Step 6: Scan documentation for contradictory current instructions**

Run:

```bash
rg -n "migration-hold|手工.*Atlas|手动.*migration|不会自动执行数据库|不得连接数据库" \
  deploy/dev/README.md \
  docs/superpowers/context/project-context.md \
  docs/superpowers/runbooks/dev-integration-audit.md
```

Expected: no current instruction requiring manual Atlas apply; historical text may remain only
inside the explicitly superseded 2026-07-30 design.

- [ ] **Step 7: Commit documentation**

```bash
git add deploy/dev/README.md \
  docs/superpowers/context/project-context.md \
  docs/superpowers/runbooks/dev-integration-audit.md \
  docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md
git commit -m "docs: document automatic dev migrations"
```

### Task 6: Run complete local verification

**Files:**
- Verify: `.github/workflows/deploy-dev.yml`
- Verify: `deploy/dev/tests/workflow_contract_test.sh`
- Verify: `docker/atlas/migrations`

- [ ] **Step 1: Run all dev deployment contracts**

```bash
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
```

Expected: every script exits 0 and prints its pass summary.

- [ ] **Step 2: Validate shell and YAML syntax**

```bash
bash -n deploy/dev/tests/workflow_contract_test.sh
ruby -e 'require "yaml"; YAML.safe_load(File.read(".github/workflows/deploy-dev.yml"), aliases: true); puts "workflow yaml: passed"'
```

Expected: Bash exits 0 and Ruby prints `workflow yaml: passed`.

- [ ] **Step 3: Validate the Atlas migration directory**

```bash
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e \
  migrate validate --dir file:///migrations
```

Expected: Atlas reports a valid migration directory and exits 0. Do not run `migrate apply`
from local verification.

- [ ] **Step 4: Run repository hygiene checks**

```bash
git diff --check origin/dev...HEAD
git status --short --branch
rg -n "(mysql://[^[:space:]]+:[^[:space:]]+@|ATLAS_URL=.+@|MYSQL_PASSWORD=|QINIU_SECRET_KEY=)" \
  .github deploy/dev docs/superpowers
```

Expected: diff check is clean, only intentional branch changes exist, and the credential scan
finds no real secret values.

- [ ] **Step 5: Commit only if verification required a correction**

```bash
git add .github/workflows/deploy-dev.yml \
  deploy/dev/tests/workflow_contract_test.sh \
  deploy/dev/README.md \
  docs/superpowers/context/project-context.md \
  docs/superpowers/runbooks/dev-integration-audit.md \
  docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md
git commit -m "fix: harden automatic dev migration workflow"
```

Skip this step when verification required no edits; never create an empty commit.

### Task 7: Audit, integrate, and verify the real pipeline

**Files:**
- Follow: `docs/superpowers/runbooks/dev-integration-audit.md`

- [ ] **Step 1: Perform the first feature-branch audit**

Fetch `origin/dev`, verify it has not moved unexpectedly, ensure the branch contains only the
approved workflow/tests/docs scope, and rerun all Task 6 checks. Report the base SHA, branch
SHA, migration files, tests, and the exact automatic database side effect. Stop for the first
explicit user confirmation.

- [ ] **Step 2: Merge into local `dev` after confirmation**

First require local `dev` to contain no commits missing from `origin/dev`, as specified by the
integration runbook. Then fast-forward local `dev` to current `origin/dev`, merge the reviewed
feature SHA without editing on `dev`, and record both parents and the resulting merge SHA. Stop
instead of combining this feature with unrelated unpublished `dev` commits.

- [ ] **Step 3: Perform the second local-dev audit**

Rerun Task 6 from merged local `dev`, verify `origin/dev` is still the audited base, and report
the exact target SHA. The report must state that pushing this SHA can automatically execute all
pending forward migrations in the dev Atlas directory, promote both ACR images, and call Baota.
Stop for the second explicit user confirmation.

- [ ] **Step 4: Verify the external prerequisite without reading its value**

Before push, freshly verify that TencentDB accepts TLS and that the actual runner can reach it
through an approved network policy. Confirm that GitHub Actions has a Repository Secret named
`ATLAS_URL`; it must use `mysql://` and exactly one `tls=true`. When the URL declares
`ssl-ca=/atlas-ca.pem`, also confirm a Repository Secret named `ATLAS_CA_PEM`; otherwise that
secret must be absent. Do not print or retrieve either value. If the Atlas revision baseline for
the existing remote schema has not been established, stop: baseline writes revision state and
requires its own explicit database-mutation authorization.

- [ ] **Step 5: Push and monitor the exact SHA after confirmation**

Push local `dev` without force, verify `origin/dev` equals the audited merge SHA, and monitor the
matching `Publish and deploy dev images` run through `preflight`, both builds, `verify-images`,
`migrate`, `promote`, and `deploy`.

- [ ] **Step 6: Verify deployment evidence**

Confirm both immutable images and both mutable `:dev` labels carry the target OCI revision,
Atlas completed without exposing the URL, the Baota webhook returned success, and `/healthz`
reports the target SHA. If any stage fails, preserve the old running service and report the exact
failed stage before requesting any manual retry or database operation.
