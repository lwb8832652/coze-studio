#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
WORKFLOW=$REPO_ROOT/.github/workflows/deploy-dev.yml

if [ ! -f "$WORKFLOW" ]; then
  printf 'workflow contract failure: %s is missing\n' "$WORKFLOW" >&2
  exit 1
fi

ruby - "$WORKFLOW" <<'RUBY'
require 'yaml'

workflow_path = ARGV.fetch(0)
workflow = YAML.safe_load(File.read(workflow_path), aliases: true)

def assert_contract(condition, message)
  raise "workflow contract failure: #{message}" unless condition
end

def job_text(job)
  step_text = job.fetch('steps', []).map do |step|
    [step['name'], step['uses'], step['if'], step['run'], step['with'], step['env']]
  end
  ([job['env']] + step_text).join("\n")
end

def needs(job)
  Array(job['needs'])
end

assert_contract(workflow.is_a?(Hash), 'workflow root must be a mapping')

triggers = workflow['on'] || workflow[true]
assert_contract(triggers.is_a?(Hash), 'on must be a mapping')
push = triggers['push']
assert_contract(push.is_a?(Hash), 'push trigger is missing')
assert_contract(Array(push['branches']) == ['dev'], 'push must target only dev')
dispatch = triggers['workflow_dispatch']
assert_contract(dispatch.is_a?(Hash), 'workflow_dispatch trigger is missing')
target_input = dispatch.fetch('inputs', {})['target_sha']
assert_contract(target_input.is_a?(Hash), 'workflow_dispatch target_sha input is missing')
assert_contract(target_input['required'] == true, 'target_sha must be required')

concurrency = workflow['concurrency']
assert_contract(concurrency.is_a?(Hash), 'concurrency must be configured')
assert_contract(concurrency['group'] == 'deploy-dev', 'concurrency group must be deploy-dev')
assert_contract(concurrency['cancel-in-progress'] == false, 'in-progress deployment must not be canceled')
assert_contract(concurrency['queue'] == 'max', 'all pending dev deployments must remain queued')
assert_contract(workflow['permissions'] == { 'contents' => 'read' }, 'permissions must be contents: read only')

jobs = workflow.fetch('jobs', {})
expected_jobs = %w[preflight build-server build-web verify-images migrate promote deploy]
assert_contract((expected_jobs - jobs.keys).empty?, 'required jobs are missing')
assert_contract(!jobs.key?('migration-hold'), 'manual migration-hold job must be removed')

preflight = jobs.fetch('preflight')
expected_preflight_outputs = {
  'target_sha' => '${{ steps.resolve.outputs.target_sha }}',
  'migration_changed' => '${{ steps.resolve.outputs.migration_changed }}',
  'deployment_blocked' => '${{ steps.resolve.outputs.deployment_blocked }}'
}
assert_contract(
  preflight.fetch('outputs', {}) == expected_preflight_outputs,
  'preflight outputs must map target_sha, migration_changed, and deployment_blocked to resolve'
)
preflight_text = job_text(preflight)
resolve_step = preflight.fetch('steps', []).find { |step| step['id'] == 'resolve' }
assert_contract(resolve_step.is_a?(Hash), 'preflight resolve step is missing')
resolve_run = resolve_step['run'].to_s
resolve_env = resolve_step.fetch('env', {})
expected_preflight_outputs.each_key do |output_name|
  shell_variable = '$' + output_name
  write_token = [
    "printf '#{output_name}=%s\\n'",
    "\"#{shell_variable}\"",
    '>> "$GITHUB_OUTPUT"'
  ].join(' ')
  write_count = resolve_run.scan(Regexp.new(Regexp.escape(write_token))).length
  assert_contract(write_count == 1,
                  "resolve must write #{output_name} to GITHUB_OUTPUT exactly once")
end
assert_contract(resolve_run.scan(/>> "\$GITHUB_OUTPUT"/).length == 3,
                'resolve must write exactly three values to GITHUB_OUTPUT')
assert_contract(resolve_env['TARGET_SHA_INPUT'].to_s.include?('inputs.target_sha'),
                'dispatch target_sha must enter the script through an environment variable')
assert_contract(!resolve_run.include?('${{ inputs.target_sha }}'),
                'dispatch target_sha must not be interpolated into shell source')
assert_contract(preflight_text.include?('actions/checkout@v7'), 'preflight must use checkout v7')
assert_contract(preflight_text.include?('docker/login-action@v4'), 'push preflight must log in to ACR')
assert_contract(preflight_text.include?('fetch-depth') && preflight_text.include?('0'),
                'preflight must fetch full history')
%w[github.event.before GITHUB_SHA origin/dev merge-base docker/atlas/migrations GITHUB_OUTPUT].each do |token|
  assert_contract(preflight_text.include?(token), "preflight is missing #{token}")
end
assert_contract(preflight_text.include?('deployment_blocked=true'),
                'preflight must fail closed with deployment_blocked')
assert_contract(preflight_text.include?('deployment_blocked=false'),
                'preflight must explicitly release verified deployments')
assert_contract(preflight_text.include?("tr '[:upper:]' '[:lower:]'") || preflight_text.include?(',,}'),
                'preflight must normalize dispatch target_sha to lowercase')
assert_contract(preflight_text.include?('git cat-file') && preflight_text.include?('git diff --quiet'),
                'preflight must validate the before object before diffing migrations')
%w[coze-server:dev coze-web:dev docker\ pull docker\ image\ inspect org.opencontainers.image.revision deployed_revision].each do |token|
  assert_contract(preflight_text.include?(token.gsub('\\ ', ' ')),
                  "preflight deployed baseline is missing #{token}")
end
assert_contract(preflight_text.include?('git merge-base --is-ancestor'),
                'deployed revision must belong to the target history')

{
  'build-server' => ['backend/Dockerfile', 'coze-server'],
  'build-web' => ['frontend/Dockerfile', 'coze-web']
}.each do |job_name, (dockerfile, repository)|
  job = jobs.fetch(job_name)
  text = job_text(job)
  assert_contract(needs(job) == ['preflight'], "#{job_name} must need preflight only")
  assert_contract(job['if'].to_s.include?("github.event_name == 'push'"),
                  "#{job_name} must run only for push")
  assert_contract(text.include?('docker/login-action@v4'), "#{job_name} must use login-action v4")
  assert_contract(text.include?('docker/setup-buildx-action@v4'), "#{job_name} must use setup-buildx v4")
  assert_contract(text.include?('docker/build-push-action@v7'), "#{job_name} must use build-push v7")
  assert_contract(text.include?(dockerfile), "#{job_name} uses the wrong Dockerfile")
  assert_contract(text.include?('GIT_REVISION=') && text.include?('SOURCE_URL='),
                  "#{job_name} must pass revision and source build args")
  assert_contract(text.include?("#{repository}:dev-") && text.include?('needs.preflight.outputs.target_sha'),
                  "#{job_name} must push the immutable full-SHA tag")
  assert_contract(!text.match?(%r{#{repository}:dev(?:['"\s]|$)}),
                  "#{job_name} must not publish the mutable dev tag")
end

verify = jobs.fetch('verify-images')
assert_contract(needs(verify).sort == %w[build-server build-web preflight],
                'verify-images must wait for preflight and both push builds')
verify_if = verify['if'].to_s
%w[always preflight build-server build-web].each do |token|
  assert_contract(verify_if.include?(token), "verify-images condition is missing #{token}")
end
assert_contract(verify_if.include?("needs.preflight.outputs.deployment_blocked != 'true'"),
                'verify-images must stop blocked deployments')
assert_contract(!verify_if.include?('migration_changed'),
                'verified migration pushes must reach image verification')
assert_contract(verify_if.include?("github.event_name == 'push'") &&
                verify_if.include?("github.event_name == 'workflow_dispatch'"),
                'verify-images must gate both push and workflow_dispatch')
assert_contract(verify_if.include?("needs.build-server.result == 'success'") &&
                verify_if.include?("needs.build-web.result == 'success'"),
                'push verification must require both successful builds')
assert_contract(verify_if.include?("needs.build-server.result == 'skipped'") &&
                verify_if.include?("needs.build-web.result == 'skipped'"),
                'workflow_dispatch verification must require both skipped builds')
verify_text = job_text(verify)
assert_contract(verify_text.include?('docker/login-action@v4'), 'verify-images must log in to ACR')
%w[coze-server:dev- coze-web:dev- docker\ pull docker\ image\ inspect org.opencontainers.image.revision].each do |token|
  assert_contract(verify_text.include?(token.gsub('\\ ', ' ')), "verify-images is missing #{token}")
end
assert_contract(!verify_text.include?('docker/build-push-action'), 'dispatch must not rebuild images')

migrate = jobs.fetch('migrate')
assert_contract(needs(migrate) == %w[preflight verify-images],
                'migrate must wait for preflight and immutable image verification only')
assert_contract(migrate['runs-on'] == 'ubuntu-latest', 'migrate must run on ubuntu-latest')
assert_contract(migrate['timeout-minutes'] == 10, 'migrate timeout must be 10 minutes')
migrate_if = migrate['if'].to_s
%w[always preflight verify-images deployment_blocked].each do |token|
  assert_contract(migrate_if.include?(token), "migrate condition is missing #{token}")
end
assert_contract(migrate_if.include?("needs.preflight.result == 'success'"),
                'migrate must require preflight success')
assert_contract(migrate_if.include?("needs.verify-images.result == 'success'"),
                'migrate must require immutable image verification success')
assert_contract(migrate_if.include?("needs.preflight.outputs.deployment_blocked != 'true'"),
                'migrate must stop blocked deployments')

migrate_steps = migrate.fetch('steps', [])
checkout_step = migrate_steps.find { |step| step['uses'] == 'actions/checkout@v7' }
assert_contract(checkout_step.is_a?(Hash), 'migrate must check out the verified target')
expected_migration_if = "${{ github.event_name == 'push' && needs.preflight.outputs.migration_changed == 'true' }}"
assert_contract(checkout_step['if'].to_s == expected_migration_if,
                'migration checkout must run only for migration pushes')
assert_contract(checkout_step.fetch('with', {})['ref'] == '${{ needs.preflight.outputs.target_sha }}',
                'migration checkout must use the preflight target SHA')

migration_step = migrate_steps.find do |step|
  step['name'] == 'Validate and apply Atlas migrations'
end
assert_contract(migration_step.is_a?(Hash), 'Atlas migration step is missing')
assert_contract(migration_step['if'].to_s == expected_migration_if,
                'Atlas migration must run only for migration pushes')
assert_contract(migration_step.fetch('env', {}) == {
                  'ATLAS_URL' => '${{ secrets.ATLAS_URL }}'
                }, 'ATLAS_URL must be injected only from the step secret environment')
migration_run = migration_step['run'].to_s
assert_contract(!migration_run.include?('${{ secrets.ATLAS_URL }}'),
                'ATLAS_URL must not be interpolated into shell source')
assert_contract(migrate.fetch('env', {}).keys.none? { |key| key == 'ATLAS_URL' },
                'ATLAS_URL must not be injected at job scope')
atlas_secret_references = File.read(workflow_path).scan(/\$\{\{\s*secrets\.ATLAS_URL\s*\}\}/).length
assert_contract(atlas_secret_references == 1,
                'ATLAS_URL secret must appear exactly once in the migration step environment')
assert_contract(migration_run.include?('set -euo pipefail'),
                'Atlas migration script must fail closed')
assert_contract(!migration_run.match?(/set\s+-[^\n]*x/),
                'Atlas migration script must not enable xtrace')
atlas_image = 'arigaio/atlas:0.35.0-community-alpine'
assert_contract(migration_run.scan(atlas_image).length == 2,
                'validate and apply must both use the repository Atlas version')
validate_index = migration_run.index('migrate validate')
apply_index = migration_run.index('migrate apply')
assert_contract(validate_index && apply_index && validate_index < apply_index,
                'migrate must validate before apply')

no_op_step = migrate_steps.find { |step| step['name'] == 'Record migration no-op' }
assert_contract(no_op_step.is_a?(Hash), 'migrate no-op step is missing')
no_op_if = no_op_step['if'].to_s
assert_contract(no_op_if.include?("github.event_name != 'push'") &&
                no_op_if.include?("needs.preflight.outputs.migration_changed != 'true'"),
                'migrate no-op must cover workflow_dispatch and pushes without migrations')
assert_contract(no_op_step['run'].to_s.include?('needs.preflight.outputs.target_sha'),
                'migrate no-op must identify the target SHA')

promote = jobs.fetch('promote')
assert_contract(
  needs(promote) == %w[preflight build-server build-web verify-images migrate],
  'promote dependencies are incomplete'
)
promote_if = promote['if'].to_s
%w[always deployment_blocked build-server build-web verify-images migrate].each do |token|
  assert_contract(promote_if.include?(token), "promote condition is missing #{token}")
end
assert_contract(promote_if.include?("needs.preflight.outputs.deployment_blocked != 'true'"),
                'promotion must stop blocked deployments')
assert_contract(promote_if.include?("needs.verify-images.result == 'success'"),
                'promotion must require immutable image verification')
assert_contract(promote_if.include?("needs.migrate.result == 'success'"),
                'promotion must require migration success')
assert_contract(promote_if.include?("needs.build-server.result == 'success'") &&
                promote_if.include?("needs.build-web.result == 'success'"),
                'push promotion must require both successful builds')
assert_contract(promote_if.include?("needs.build-server.result == 'skipped'") &&
                promote_if.include?("needs.build-web.result == 'skipped'"),
                'workflow_dispatch promotion must require both skipped builds')
assert_contract(!promote_if.include?('migration_changed'),
                'successful automatic migration must permit promotion')
assert_contract(!promote_if.include?("needs.verify-images.result == 'skipped'"),
                'push promotion must not skip immutable image verification')
promote_text = job_text(promote)
assert_contract(promote_text.include?('docker/login-action@v4'), 'promote must log in to ACR')
assert_contract(promote_text.include?('docker/setup-buildx-action@v4'), 'promote must set up Buildx')
assert_contract(promote_text.scan('docker buildx imagetools create').length == 2,
                'promote must update exactly two mutable tags')
%w[coze-server:dev coze-web:dev coze-server:dev- coze-web:dev-].each do |token|
  assert_contract(promote_text.include?(token), "promote is missing #{token}")
end

deploy = jobs.fetch('deploy')
assert_contract(needs(deploy) == ['promote'], 'deploy must need promote only')
deploy_text = job_text(deploy)
%w[curl --fail-with-body BAOTA_WEBHOOK_URL BAOTA_WEBHOOK_TOKEN BAOTA_WEBHOOK_PINNED_PUBKEY
   needs.promote.outputs.target_sha --pinnedpubkey --insecure].each do |token|
  assert_contract(deploy_text.include?(token), "deploy webhook is missing #{token}")
end
assert_contract(deploy_text.match?(/header|-H/i), 'optional webhook token must be sent in a header')

raw = File.read(workflow_path)
forbidden = {
  /ssh[-_ ]?(key|private)|id_rsa/i => 'SSH credentials',
  /\bprod(?:uction)?\b/i => 'production deployment'
}
forbidden.each do |pattern, label|
  assert_contract(!raw.match?(pattern), "workflow must not contain #{label}")
end

puts 'workflow contract: passed'
RUBY

SEMANTIC_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/coze-workflow-test.XXXXXX")
trap 'rm -rf -- "$SEMANTIC_ROOT"' EXIT
RESOLVE_SCRIPT=$SEMANTIC_ROOT/resolve.sh
TEST_REPO=$SEMANTIC_ROOT/repository
TEST_BIN=$SEMANTIC_ROOT/bin
GITHUB_OUTPUT_FILE=$SEMANTIC_ROOT/github-output

ruby - "$WORKFLOW" > "$RESOLVE_SCRIPT" <<'EXTRACT'
require 'yaml'

workflow = YAML.safe_load(File.read(ARGV.fetch(0)), aliases: true)
resolve_step = workflow.fetch('jobs').fetch('preflight').fetch('steps').find do |step|
  step['id'] == 'resolve'
end
abort 'resolve step is missing' unless resolve_step
puts resolve_step.fetch('run')
EXTRACT

mkdir -p -- "$TEST_REPO" "$TEST_BIN"
git -C "$TEST_REPO" init -q
printf 'base\n' > "$TEST_REPO/application.txt"
git -C "$TEST_REPO" add application.txt
git -C "$TEST_REPO" -c user.name=contract-test -c user.email=contract@example.invalid \
  commit -qm 'base deployment'
DEPLOYED_REVISION=$(git -C "$TEST_REPO" rev-parse HEAD)

mkdir -p -- "$TEST_REPO/docker/atlas/migrations"
printf 'migration\n' > "$TEST_REPO/docker/atlas/migrations/202607300001.sql"
git -C "$TEST_REPO" add docker/atlas/migrations/202607300001.sql
git -C "$TEST_REPO" -c user.name=contract-test -c user.email=contract@example.invalid \
  commit -qm 'add migration'
BEFORE_REVISION=$(git -C "$TEST_REPO" rev-parse HEAD)

printf 'ordinary change\n' >> "$TEST_REPO/application.txt"
git -C "$TEST_REPO" add application.txt
git -C "$TEST_REPO" -c user.name=contract-test -c user.email=contract@example.invalid \
  commit -qm 'ordinary follow-up'
TARGET_REVISION=$(git -C "$TEST_REPO" rev-parse HEAD)

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'if [ "$1" = pull ]; then' \
  '  image=${@: -1}' \
  '  case "${DOCKER_MODE:-deployed}" in' \
  '    deployed|inconsistent|server-inspect-error|web-inspect-error) exit 0 ;;' \
  '    missing)' \
  '      printf "Error response from daemon: manifest unknown: manifest unknown\\n" >&2' \
  '      exit 1' \
  '      ;;' \
  '    server-missing)' \
  '      if [[ "$image" == *coze-server:dev ]]; then' \
  '        printf "Error response from daemon: manifest unknown: manifest unknown\\n" >&2' \
  '        exit 1' \
  '      fi' \
  '      exit 0' \
  '      ;;' \
  '    registry-error)' \
  '      printf "Error response from daemon: registry connection timed out\\n" >&2' \
  '      exit 1' \
  '      ;;' \
  '    *) exit 98 ;;' \
  '  esac' \
  'fi' \
  'if [ "$1" = image ] && [ "$2" = inspect ]; then' \
  '  image=${@: -1}' \
  '  if [ "${DOCKER_MODE:-deployed}" = inconsistent ] &&' \
  '    [[ "$image" == *coze-web:dev ]]; then' \
  '    printf "%s\\n" "$ALTERNATE_REVISION"' \
  '  else' \
  '    printf "%s\\n" "$DEPLOYED_REVISION"' \
  '  fi' \
  '  if [ "${DOCKER_MODE:-deployed}" = server-inspect-error ] &&' \
  '    [[ "$image" == *coze-server:dev ]]; then' \
  '    exit 1' \
  '  fi' \
  '  if [ "${DOCKER_MODE:-deployed}" = web-inspect-error ] &&' \
  '    [[ "$image" == *coze-web:dev ]]; then' \
  '    exit 1' \
  '  fi' \
  '  exit 0' \
  'fi' \
  'exit 97' > "$TEST_BIN/docker"
chmod +x "$TEST_BIN/docker"

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

assert_output() {
  output_file=$1
  expected=$2
  message=$3

  grep -qx -- "$expected" "$output_file" || {
    printf 'workflow contract failure: %s\n' "$message" >&2
    exit 1
  }
}

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

for mode in server-inspect-error web-inspect-error; do
  blocked_output=$SEMANTIC_ROOT/$mode-output
  run_preflight "$mode" "$BEFORE_REVISION" "$BEFORE_REVISION" \
    "$TARGET_REVISION" "$blocked_output"
  assert_output "$blocked_output" 'migration_changed=false' \
    "$mode was incorrectly classified as a migration"
  assert_output "$blocked_output" 'deployment_blocked=true' \
    "$mode did not fail closed"
done

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
  'if [[ "$*" == *"migrate apply"* ]] &&' \
  '  [ "${MIGRATE_DOCKER_MODE:-success}" = apply-fail ]; then' \
  '  exit 43' \
  'fi' \
  'exit 0' > "$TEST_BIN/docker"
chmod +x "$TEST_BIN/docker"

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

DUMMY_ATLAS_URL='mysql://contract:masked@example.invalid/dev'
: > "$MIGRATE_DOCKER_LOG"
PATH="$TEST_BIN:$PATH" MIGRATE_DOCKER_LOG="$MIGRATE_DOCKER_LOG" \
  ATLAS_URL="$DUMMY_ATLAS_URL" bash "$MIGRATE_SCRIPT"
migration_call_count=$(wc -l < "$MIGRATE_DOCKER_LOG" | tr -d ' ')
[ "$migration_call_count" -eq 2 ] || {
  printf 'workflow contract failure: successful migration did not run Docker exactly twice\n' >&2
  exit 1
}
first_migration_call=$(sed -n '1p' "$MIGRATE_DOCKER_LOG")
second_migration_call=$(sed -n '2p' "$MIGRATE_DOCKER_LOG")
[[ "$first_migration_call" == *"migrate validate"* ]] || {
  printf 'workflow contract failure: Atlas validation did not run first\n' >&2
  exit 1
}
[[ "$second_migration_call" == *"migrate apply"* ]] || {
  printf 'workflow contract failure: Atlas apply did not run second\n' >&2
  exit 1
}

: > "$MIGRATE_DOCKER_LOG"
if PATH="$TEST_BIN:$PATH" MIGRATE_DOCKER_LOG="$MIGRATE_DOCKER_LOG" \
  MIGRATE_DOCKER_MODE=validate-fail ATLAS_URL="$DUMMY_ATLAS_URL" \
  bash "$MIGRATE_SCRIPT"; then
  printf 'workflow contract failure: Atlas validation failure was ignored\n' >&2
  exit 1
fi
migration_call_count=$(wc -l < "$MIGRATE_DOCKER_LOG" | tr -d ' ')
[ "$migration_call_count" -eq 1 ] || {
  printf 'workflow contract failure: apply ran after validation failure\n' >&2
  exit 1
}
first_migration_call=$(sed -n '1p' "$MIGRATE_DOCKER_LOG")
[[ "$first_migration_call" == *"migrate validate"* ]] || {
  printf 'workflow contract failure: validation failure did not come from validate\n' >&2
  exit 1
}

: > "$MIGRATE_DOCKER_LOG"
if PATH="$TEST_BIN:$PATH" MIGRATE_DOCKER_LOG="$MIGRATE_DOCKER_LOG" \
  MIGRATE_DOCKER_MODE=apply-fail ATLAS_URL="$DUMMY_ATLAS_URL" \
  bash "$MIGRATE_SCRIPT"; then
  printf 'workflow contract failure: Atlas apply failure was ignored\n' >&2
  exit 1
fi
migration_call_count=$(wc -l < "$MIGRATE_DOCKER_LOG" | tr -d ' ')
[ "$migration_call_count" -eq 2 ] || {
  printf 'workflow contract failure: apply failure did not follow one validation call\n' >&2
  exit 1
}
second_migration_call=$(sed -n '2p' "$MIGRATE_DOCKER_LOG")
[[ "$second_migration_call" == *"migrate apply"* ]] || {
  printf 'workflow contract failure: apply failure did not come from apply\n' >&2
  exit 1
}

DEPLOY_SCRIPT=$SEMANTIC_ROOT/deploy.sh
CURL_ARGS_FILE=$SEMANTIC_ROOT/curl-args

ruby - "$WORKFLOW" > "$DEPLOY_SCRIPT" <<'EXTRACT'
require 'yaml'

workflow = YAML.safe_load(File.read(ARGV.fetch(0)), aliases: true)
deploy_step = workflow.fetch('jobs').fetch('deploy').fetch('steps').find do |step|
  step['name'] == 'Trigger dev deployment'
end
abort 'deploy step is missing' unless deploy_step
puts deploy_step.fetch('run')
EXTRACT

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'printf "%s\\n" "$@" > "$CURL_ARGS_FILE"' > "$TEST_BIN/curl"
chmod +x "$TEST_BIN/curl"

run_deploy() {
  pinned_pubkey=$1
  : > "$CURL_ARGS_FILE"
  PATH="$TEST_BIN:$PATH" \
    CURL_ARGS_FILE="$CURL_ARGS_FILE" \
    BAOTA_WEBHOOK_URL='https://webhook.example.invalid/hook' \
    BAOTA_WEBHOOK_TOKEN= \
    BAOTA_WEBHOOK_PINNED_PUBKEY="$pinned_pubkey" \
    TARGET_SHA="$TARGET_REVISION" \
    bash "$DEPLOY_SCRIPT"
}

run_deploy ''
if grep -Eqx -- '--insecure|--pinnedpubkey' "$CURL_ARGS_FILE"; then
  printf 'workflow contract failure: public webhook unexpectedly disabled CA verification\n' >&2
  exit 1
fi

DUMMY_PIN='sha256//AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA='
run_deploy "$DUMMY_PIN"
assert_output "$CURL_ARGS_FILE" '--insecure' \
  'self-signed webhook did not enable pinned-key transport'
assert_output "$CURL_ARGS_FILE" '--pinnedpubkey' \
  'self-signed webhook did not pass the pinned public key option'
assert_output "$CURL_ARGS_FILE" "$DUMMY_PIN" \
  'self-signed webhook did not pass the configured public key pin'

printf 'workflow semantic contract: passed\n'
