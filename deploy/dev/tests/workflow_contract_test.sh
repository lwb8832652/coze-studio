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
expected_jobs = %w[preflight build-server build-web migration-hold verify-images promote deploy]
assert_contract((expected_jobs - jobs.keys).empty?, 'required jobs are missing')

preflight = jobs.fetch('preflight')
assert_contract(preflight.fetch('outputs', {}).keys.sort == %w[migration_changed target_sha],
                'preflight must expose only target_sha and migration_changed')
preflight_text = job_text(preflight)
resolve_step = preflight.fetch('steps', []).find { |step| step['id'] == 'resolve' }
assert_contract(resolve_step.is_a?(Hash), 'preflight resolve step is missing')
resolve_run = resolve_step['run'].to_s
resolve_env = resolve_step.fetch('env', {})
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
assert_contract(preflight_text.include?('migration_changed=true'),
                'preflight must have a fail-closed migration result')
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

hold = jobs.fetch('migration-hold')
assert_contract((%w[preflight build-server build-web] - needs(hold)).empty?,
                'migration-hold must wait for preflight and both builds')
assert_contract(hold['if'].to_s.include?('migration_changed') &&
                hold['if'].to_s.include?("github.event_name == 'push'"),
                'migration-hold must be limited to migration pushes')
hold_text = job_text(hold)
%w[GITHUB_STEP_SUMMARY workflow_dispatch target_sha].each do |token|
  assert_contract(hold_text.include?(token), "migration-hold summary is missing #{token}")
end

verify = jobs.fetch('verify-images')
assert_contract(needs(verify) == ['preflight'], 'verify-images must need preflight only')
assert_contract(verify['if'].to_s.include?("github.event_name == 'workflow_dispatch'"),
                'verify-images must run only for workflow_dispatch')
verify_text = job_text(verify)
assert_contract(verify_text.include?('docker/login-action@v4'), 'verify-images must log in to ACR')
%w[coze-server:dev- coze-web:dev- docker\ pull docker\ image\ inspect org.opencontainers.image.revision].each do |token|
  assert_contract(verify_text.include?(token.gsub('\\ ', ' ')), "verify-images is missing #{token}")
end
assert_contract(!verify_text.include?('docker/build-push-action'), 'dispatch must not rebuild images')

promote = jobs.fetch('promote')
assert_contract((expected_jobs[0, 6] - ['migration-hold', 'promote'] - needs(promote)).empty?,
                'promote dependencies are incomplete')
promote_if = promote['if'].to_s
%w[always migration_changed build-server build-web verify-images].each do |token|
  assert_contract(promote_if.include?(token), "promote condition is missing #{token}")
end
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
%w[curl --fail-with-body BAOTA_WEBHOOK_URL BAOTA_WEBHOOK_TOKEN needs.promote.outputs.target_sha].each do |token|
  assert_contract(deploy_text.include?(token), "deploy webhook is missing #{token}")
end
assert_contract(deploy_text.match?(/header|-H/i), 'optional webhook token must be sent in a header')

raw = File.read(workflow_path)
forbidden = {
  /ssh[-_ ]?(key|private)|id_rsa/i => 'SSH credentials',
  /atlas\s+migrate\s+apply/i => 'Atlas migration apply',
  /mysql|database_url|db_password/i => 'database access',
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
  'if [ "$1" = pull ]; then exit 0; fi' \
  'if [ "$1" = image ] && [ "$2" = inspect ]; then' \
  '  printf "%s\\n" "$DEPLOYED_REVISION"' \
  '  exit 0' \
  'fi' \
  'exit 97' > "$TEST_BIN/docker"
chmod +x "$TEST_BIN/docker"

(
  cd -- "$TEST_REPO"
  PATH="$TEST_BIN:$PATH" \
    DEPLOYED_REVISION="$DEPLOYED_REVISION" \
    GITHUB_EVENT_NAME=push \
    GITHUB_SHA="$TARGET_REVISION" \
    TARGET_SHA_INPUT= \
    BEFORE_SHA="$BEFORE_REVISION" \
    SERVER_DEV_IMAGE=registry.example/coze-server:dev \
    WEB_DEV_IMAGE=registry.example/coze-web:dev \
    GITHUB_OUTPUT="$GITHUB_OUTPUT_FILE" \
    bash "$RESOLVE_SCRIPT"
)

grep -qx "target_sha=$TARGET_REVISION" "$GITHUB_OUTPUT_FILE" || {
  printf 'workflow contract failure: semantic preflight wrote the wrong target SHA\n' >&2
  exit 1
}
grep -qx 'migration_changed=true' "$GITHUB_OUTPUT_FILE" || {
  printf 'workflow contract failure: follow-up push bypassed the pending migration hold\n' >&2
  exit 1
}

printf 'workflow semantic contract: passed\n'
