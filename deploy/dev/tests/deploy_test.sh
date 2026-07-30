#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DEPLOY_SCRIPT=$SCRIPT_DIR/../deploy.sh
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/coze-deploy-test.XXXXXX")
trap 'rm -rf -- "$TEST_ROOT"' EXIT

if [ ! -f "$DEPLOY_SCRIPT" ]; then
  printf 'deploy test failure: deploy.sh is missing\n' >&2
  exit 1
fi

# shellcheck source=../deploy.sh
source "$DEPLOY_SCRIPT"

REV_A=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
REV_B=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
REV_C=cccccccccccccccccccccccccccccccccccccccc
passed=0

fail() {
  printf 'deploy test failure: %s\n' "$1" >&2
  exit 1
}

assert_file_contains() {
  file=$1
  pattern=$2
  message=$3
  grep -Eq -- "$pattern" "$file" || fail "$message"
}

assert_file_not_contains() {
  file=$1
  pattern=$2
  message=$3
  if [ -f "$file" ] && grep -Eq -- "$pattern" "$file"; then
    fail "$message"
  fi
}

failure_record_for() {
  find "$1/deployments" -maxdepth 1 -name 'failed-*.env' -print -quit 2>/dev/null
}

setup_transaction_case() {
  CASE_DIR=$1
  DEPLOYMENTS_DIR=$CASE_DIR/deployments
  COMMAND_LOG=$CASE_DIR/commands.log
  WAIT_COUNT_FILE=$CASE_DIR/wait-count
  SERVER_REPOSITORY=registry.example/coze-server
  WEB_REPOSITORY=registry.example/coze-web
  SERVER_IMAGE_REF=$SERVER_REPOSITORY:dev
  WEB_IMAGE_REF=$WEB_REPOSITORY:dev
  MOCK_OLD_SERVER_ID=sha256:old-server
  MOCK_OLD_WEB_ID=sha256:old-web
  MOCK_OLD_SERVER_REVISION=$REV_C
  MOCK_OLD_WEB_REVISION=$REV_C
  MOCK_SERVER_REVISION=$REV_A
  MOCK_WEB_REVISION=$REV_A
  MOCK_SERVER_IMAGE_ID=sha256:candidate-server
  MOCK_WEB_IMAGE_ID=sha256:candidate-web
  MOCK_PULL_FAILURE=0
  MOCK_COMPOSE_FAILURE=0
  MOCK_HEALTH_FAILURE=0
  MOCK_ROLLBACK_HEALTHY=0
  mkdir -p -- "$CASE_DIR"
  : > "$COMMAND_LOG"
  printf '0\n' > "$WAIT_COUNT_FILE"

  container_image_id() {
    case "$1" in
      coze-server) printf '%s\n' "$MOCK_OLD_SERVER_ID" ;;
      coze-web) printf '%s\n' "$MOCK_OLD_WEB_ID" ;;
      *) return 1 ;;
    esac
  }

  image_revision() {
    case "$1" in
      "$SERVER_IMAGE_REF") printf '%s\n' "$MOCK_SERVER_REVISION" ;;
      "$WEB_IMAGE_REF") printf '%s\n' "$MOCK_WEB_REVISION" ;;
      "$MOCK_OLD_SERVER_ID") printf '%s\n' "$MOCK_OLD_SERVER_REVISION" ;;
      "$MOCK_OLD_WEB_ID") printf '%s\n' "$MOCK_OLD_WEB_REVISION" ;;
      *) return 1 ;;
    esac
  }

  docker_cmd() {
    {
      printf 'docker'
      printf ' %s' "$@"
      printf '\n'
    } >> "$COMMAND_LOG"

    if [ "$1" = pull ]; then
      [ "$MOCK_PULL_FAILURE" -eq 0 ]
      return
    fi
    if [ "$1" = image ] && [ "$2" = inspect ]; then
      case "$5" in
        "$SERVER_IMAGE_REF") printf '%s\n' "$MOCK_SERVER_IMAGE_ID" ;;
        "$WEB_IMAGE_REF") printf '%s\n' "$MOCK_WEB_IMAGE_ID" ;;
        *) return 1 ;;
      esac
    fi
    return 0
  }

  compose_cmd() {
    printf 'compose SERVER_IMAGE_TAG=%s WEB_IMAGE_TAG=%s' \
      "${SERVER_IMAGE_TAG-}" "${WEB_IMAGE_TAG-}" >> "$COMMAND_LOG"
    printf ' %s' "$@" >> "$COMMAND_LOG"
    printf '\n' >> "$COMMAND_LOG"
    [ "$MOCK_COMPOSE_FAILURE" -eq 0 ]
  }

  wait_for_health() {
    count=$(<"$WAIT_COUNT_FILE")
    count=$((count + 1))
    printf '%s\n' "$count" > "$WAIT_COUNT_FILE"
    if [ "$count" -eq 1 ] && [ "$MOCK_HEALTH_FAILURE" -eq 1 ]; then
      return 1
    fi
    if [ "$count" -gt 1 ]; then
      [ "$MOCK_ROLLBACK_HEALTHY" -eq 1 ]
      return
    fi
    return 0
  }
}

test_mismatched_candidate_revisions_stop_before_up() (
  case_dir=$(mktemp -d "$TEST_ROOT/mismatch.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_WEB_REVISION=$REV_B

  if deploy_transaction "" >"$case_dir/output.log" 2>&1; then
    fail 'mismatched candidate revisions unexpectedly succeeded'
  fi
  assert_file_not_contains "$COMMAND_LOG" '^compose .* up ' \
    'mismatched candidate revisions reached compose up'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'revision mismatch did not retain a failure record'
  assert_file_contains "$failure_record" "^CANDIDATE_SERVER_REVISION=$REV_A$" \
    'failure record lost the candidate server revision'
  assert_file_contains "$failure_record" "^CANDIDATE_WEB_REVISION=$REV_B$" \
    'failure record lost the candidate web revision'
  assert_file_contains "$failure_record" '^CANDIDATE_SERVER_IMAGE_ID=sha256:candidate-server$' \
    'failure record lost the pulled candidate server image ID'
  assert_file_contains "$failure_record" '^CANDIDATE_WEB_IMAGE_ID=sha256:candidate-web$' \
    'failure record lost the pulled candidate web image ID'
  assert_file_contains "$failure_record" '^ROLLBACK_RESULT=not-attempted$' \
    'pre-update failure record has the wrong rollback result'
)

test_requested_revision_mismatch_stops_before_up() (
  case_dir=$(mktemp -d "$TEST_ROOT/requested-mismatch.XXXXXX")
  setup_transaction_case "$case_dir"

  if deploy_transaction "$REV_B" >"$case_dir/output.log" 2>&1; then
    fail 'requested revision mismatch unexpectedly succeeded'
  fi
  assert_file_not_contains "$COMMAND_LOG" '^compose .* up ' \
    'requested revision mismatch reached compose up'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'requested revision mismatch did not retain a failure record'
  assert_file_contains "$failure_record" '^FAILURE_REASON=candidate revision does not match the requested SHA$' \
    'requested revision mismatch reason was not recorded'
  assert_file_contains "$failure_record" '^CANDIDATE_SERVER_IMAGE_ID=sha256:candidate-server$' \
    'requested revision mismatch lost the candidate server image ID'
  assert_file_contains "$failure_record" '^CANDIDATE_WEB_IMAGE_ID=sha256:candidate-web$' \
    'requested revision mismatch lost the candidate web image ID'
)

test_invalid_candidate_revision_stops_before_up() (
  case_dir=$(mktemp -d "$TEST_ROOT/invalid-candidate.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_SERVER_REVISION=not-a-full-sha

  if deploy_transaction "" >"$case_dir/output.log" 2>&1; then
    fail 'invalid candidate revision unexpectedly succeeded'
  fi
  assert_file_not_contains "$COMMAND_LOG" '^compose .* up ' \
    'invalid candidate revision reached compose up'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'invalid candidate revision did not retain a failure record'
  assert_file_contains "$failure_record" '^CANDIDATE_SERVER_REVISION=not-a-full-sha$' \
    'invalid candidate revision value was not retained'
  assert_file_contains "$failure_record" '^CANDIDATE_SERVER_IMAGE_ID=sha256:candidate-server$' \
    'invalid candidate revision lost the candidate server image ID'
  assert_file_contains "$failure_record" '^CANDIDATE_WEB_IMAGE_ID=sha256:candidate-web$' \
    'invalid candidate revision lost the candidate web image ID'
)

test_pull_failure_stops_before_up() (
  case_dir=$(mktemp -d "$TEST_ROOT/pull-failure.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_PULL_FAILURE=1

  if deploy_transaction "" >"$case_dir/output.log" 2>&1; then
    fail 'candidate pull failure unexpectedly succeeded'
  fi
  assert_file_not_contains "$COMMAND_LOG" '^compose .* up ' \
    'candidate pull failure reached compose up'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'candidate pull failure did not retain a failure record'
  assert_file_contains "$failure_record" '^ROLLBACK_RESULT=not-attempted$' \
    'candidate pull failure has the wrong rollback result'
)

test_success_records_complete_current_environment() (
  case_dir=$(mktemp -d "$TEST_ROOT/success.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_OLD_SERVER_ID=
  MOCK_OLD_WEB_ID=

  deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1 || \
    fail 'equal candidate revision deployment failed'

  current=$DEPLOYMENTS_DIR/current.env
  [ -f "$current" ] || fail 'successful deployment did not create current.env'
  assert_file_contains "$current" '^REVISION=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa$' \
    'current.env has the wrong revision'
  assert_file_contains "$current" '^SERVER_IMAGE_REF=registry\.example/coze-server:dev$' \
    'current.env is missing the server image ref'
  assert_file_contains "$current" '^WEB_IMAGE_REF=registry\.example/coze-web:dev$' \
    'current.env is missing the web image ref'
  assert_file_contains "$current" '^SERVER_IMAGE_ID=sha256:candidate-server$' \
    'current.env is missing the server image ID'
  assert_file_contains "$current" '^WEB_IMAGE_ID=sha256:candidate-web$' \
    'current.env is missing the web image ID'
  assert_file_contains "$current" '^DEPLOYED_AT_UTC=[0-9]{4}-[0-9]{2}-[0-9]{2}T' \
    'current.env is missing a UTC timestamp'
  assert_file_contains "$COMMAND_LOG" \
    '^compose SERVER_IMAGE_TAG=dev WEB_IMAGE_TAG=dev up -d --no-build --remove-orphans coze-server coze-web$' \
    'successful deployment did not update both dev services together'
)

test_health_failure_rolls_back_both_images_and_stays_failed() (
  case_dir=$(mktemp -d "$TEST_ROOT/rollback.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_HEALTH_FAILURE=1
  MOCK_ROLLBACK_HEALTHY=1

  if deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'deployment returned success after a healthy rollback'
  fi

  server_tag=$(awk '/^docker tag sha256:old-server / { sub(/^.*coze-server:/, ""); print }' "$COMMAND_LOG")
  web_tag=$(awk '/^docker tag sha256:old-web / { sub(/^.*coze-web:/, ""); print }' "$COMMAND_LOG")
  [ -n "$server_tag" ] || fail 'rollback did not tag the old server image'
  [ "$server_tag" = "$web_tag" ] || fail 'rollback image tags do not share one transaction ID'
  case "$server_tag" in
    rollback-*) ;;
    *) fail 'rollback used an unexpected tag name' ;;
  esac
  assert_file_contains "$COMMAND_LOG" \
    "^compose SERVER_IMAGE_TAG=$server_tag WEB_IMAGE_TAG=$server_tag up -d --no-build --remove-orphans coze-server coze-web$" \
    'rollback compose did not recreate both services with both rollback tags'
  assert_file_contains "$case_dir/output.log" 'rollback succeeded' \
    'healthy rollback was not reported'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'failed deployment did not retain a failure record'
  assert_file_contains "$failure_record" '^ROLLBACK_RESULT=succeeded$' \
    'failure record did not preserve the successful rollback result'
)

test_first_deployment_failure_cannot_claim_rollback() (
  case_dir=$(mktemp -d "$TEST_ROOT/first-failure.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_OLD_SERVER_ID=
  MOCK_OLD_WEB_ID=
  MOCK_HEALTH_FAILURE=1

  if deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'failed first deployment unexpectedly succeeded'
  fi
  assert_file_contains "$case_dir/output.log" 'first deployment failed; rollback unavailable' \
    'first deployment failure was not identified'
  assert_file_not_contains "$case_dir/output.log" 'rollback succeeded' \
    'first deployment failure claimed rollback success'
  assert_file_not_contains "$COMMAND_LOG" '^docker tag ' \
    'first deployment failure attempted to create rollback tags'
)

write_direct_case_files() {
  case_dir=$1
  mkdir -p -- "$case_dir/bin"
  printf '%s\n' \
    'ACR_REGISTRY=registry.example' \
    'ACR_NAMESPACE=example' \
    'ACR_PASSWORD=sentinel-acr-password' \
    'APP_SECRET=sentinel-app-secret' \
    'BAOTA_WEBHOOK_TOKEN=sentinel-webhook-token' > "$case_dir/deploy.env"
  printf '%s\n' \
    '#!/usr/bin/env bash' \
    'printf "docker invoked\\n" >> "$DOCKER_MARKER"' \
    'exit 97' > "$case_dir/bin/docker"
  chmod +x "$case_dir/bin/docker"

  if ! command -v flock >/dev/null 2>&1; then
    command -v lsof >/dev/null 2>&1 || fail 'lock test requires lsof when flock is unavailable'
    command -v shlock >/dev/null 2>&1 || fail 'lock test requires shlock when flock is unavailable'
    printf '%s\n' \
      '#!/usr/bin/env bash' \
      'set -euo pipefail' \
      '[ "$1" = "-n" ]' \
      'fd=$2' \
      'lock_path=$(lsof -a -p "$PPID" -d "$fd" -Fn 2>/dev/null | sed -n "s/^n//p")' \
      '[ -n "$lock_path" ]' \
      'shlock -f "$lock_path.test-lock" -p "$PPID"' > "$case_dir/bin/flock"
    chmod +x "$case_dir/bin/flock"
  fi
}

test_lock_contention_fails_before_transaction() (
  case_dir=$(mktemp -d "$TEST_ROOT/lock.XXXXXX")
  write_direct_case_files "$case_dir"
  marker=$case_dir/docker-called
  ready=$case_dir/lock-ready
  (
    exec 9>"$case_dir/deploy.lock"
    PATH="$case_dir/bin:$PATH" flock -n 9
    : > "$ready"
    sleep 10
  ) &
  holder=$!
  for _ in {1..100}; do
    [ -f "$ready" ] && break
    sleep 0.05
  done
  [ -f "$ready" ] || fail 'lock holder did not become ready'

  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    kill "$holder" 2>/dev/null || true
    fail 'contended deployment lock unexpectedly succeeded'
  fi
  kill "$holder" 2>/dev/null || true
  wait "$holder" 2>/dev/null || true
  [ ! -e "$marker" ] || fail 'lock contention reached Docker transaction work'
  assert_file_contains "$case_dir/output.log" 'deployment already in progress' \
    'lock contention did not report the active deployment'
)

test_invalid_sha_fails_before_docker() (
  case_dir=$(mktemp -d "$TEST_ROOT/invalid-sha.XXXXXX")
  write_direct_case_files "$case_dir"
  marker=$case_dir/docker-called

  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" not-a-sha >"$case_dir/output.log" 2>&1; then
    fail 'invalid SHA unexpectedly succeeded'
  fi
  [ ! -e "$marker" ] || fail 'invalid SHA reached Docker'
  assert_file_contains "$case_dir/output.log" 'expected zero arguments or one full 40-hex SHA' \
    'invalid SHA error was not reported'
)

test_logs_never_disclose_secret_sentinels() (
  case_dir=$(mktemp -d "$TEST_ROOT/secrets.XXXXXX")
  write_direct_case_files "$case_dir"
  marker=$case_dir/docker-called

  DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" invalid >"$case_dir/output.log" 2>&1 || true
  assert_file_not_contains "$case_dir/output.log" 'sentinel-acr-password' \
    'logs disclosed ACR_PASSWORD'
  assert_file_not_contains "$case_dir/output.log" 'sentinel-app-secret' \
    'logs disclosed APP_SECRET'
  assert_file_not_contains "$case_dir/output.log" 'sentinel-webhook-token' \
    'logs disclosed BAOTA_WEBHOOK_TOKEN'
)

test_record_success_is_atomic_and_complete() (
  case_dir=$(mktemp -d "$TEST_ROOT/atomic-record.XXXXXX")
  DEPLOYMENTS_DIR=$case_dir/deployments
  record_success "$REV_A" registry.example/coze-server:dev registry.example/coze-web:dev \
    sha256:candidate-server sha256:candidate-web

  current=$DEPLOYMENTS_DIR/current.env
  [ -f "$current" ] || fail 'record_success did not create current.env'
  [ "$(wc -l < "$current" | tr -d ' ')" -eq 6 ] || \
    fail 'current.env is incomplete or contains unexpected fields'
  assert_file_contains "$current" '^REVISION=' 'current.env is missing REVISION'
  assert_file_contains "$current" '^SERVER_IMAGE_REF=' 'current.env is missing SERVER_IMAGE_REF'
  assert_file_contains "$current" '^WEB_IMAGE_REF=' 'current.env is missing WEB_IMAGE_REF'
  assert_file_contains "$current" '^SERVER_IMAGE_ID=' 'current.env is missing SERVER_IMAGE_ID'
  assert_file_contains "$current" '^WEB_IMAGE_ID=' 'current.env is missing WEB_IMAGE_ID'
  assert_file_contains "$current" '^DEPLOYED_AT_UTC=' 'current.env is missing DEPLOYED_AT_UTC'
  if find "$DEPLOYMENTS_DIR" -maxdepth 1 -name '.current.env.tmp.*' | grep -q .; then
    fail 'record_success left a temporary partial file behind'
  fi
)

run_test() {
  name=$1
  shift
  "$@"
  passed=$((passed + 1))
  printf 'ok %d - %s\n' "$passed" "$name"
}

run_test 'candidate revisions must match' test_mismatched_candidate_revisions_stop_before_up
run_test 'requested revision must match' test_requested_revision_mismatch_stops_before_up
run_test 'candidate revision labels must be full SHA values' test_invalid_candidate_revision_stops_before_up
run_test 'candidate pull failure stops before update' test_pull_failure_stops_before_up
run_test 'successful deployment records current.env' test_success_records_complete_current_environment
run_test 'health failure rolls back atomically' test_health_failure_rolls_back_both_images_and_stays_failed
run_test 'first deployment failure has no fake rollback' test_first_deployment_failure_cannot_claim_rollback
run_test 'deployment lock is nonblocking' test_lock_contention_fails_before_transaction
run_test 'invalid SHA fails before Docker' test_invalid_sha_fails_before_docker
run_test 'logs do not disclose secret sentinels' test_logs_never_disclose_secret_sentinels
run_test 'success record is atomic and complete' test_record_success_is_atomic_and_complete

printf 'deploy tests: %d passed\n' "$passed"
