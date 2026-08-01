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
  MOCK_RESTORED_SERVER_ID=$MOCK_OLD_SERVER_ID
  MOCK_RESTORED_WEB_ID=$MOCK_OLD_WEB_ID
  MOCK_OLD_SERVER_REVISION=$REV_C
  MOCK_OLD_WEB_REVISION=$REV_C
  MOCK_SERVER_REVISION=$REV_A
  MOCK_WEB_REVISION=$REV_A
  MOCK_SERVER_IMAGE_ID=sha256:candidate-server
  MOCK_WEB_IMAGE_ID=sha256:candidate-web
  MOCK_RUNNING_SERVER_ID=$MOCK_SERVER_IMAGE_ID
  MOCK_RUNNING_WEB_ID=$MOCK_WEB_IMAGE_ID
  MOCK_PULL_FAILURE=0
  MOCK_COMPOSE_FAILURE=0
  MOCK_HEALTH_FAILURE=0
  MOCK_ROLLBACK_HEALTHY=0
  mkdir -p -- "$CASE_DIR"
  : > "$COMMAND_LOG"
  printf '0\n' > "$WAIT_COUNT_FILE"

  container_image_id() {
    restored=0
    updated=0
    if grep -Eq '^compose SERVER_IMAGE_TAG=rollback-' "$COMMAND_LOG"; then
      restored=1
    elif grep -Eq '^compose SERVER_IMAGE_TAG=dev WEB_IMAGE_TAG=dev up ' "$COMMAND_LOG"; then
      updated=1
    fi
    case "$1" in
      coze-server)
        if [ "$restored" -eq 1 ]; then
          printf '%s\n' "$MOCK_RESTORED_SERVER_ID"
        elif [ "$updated" -eq 1 ]; then
          printf '%s\n' "$MOCK_RUNNING_SERVER_ID"
        else
          printf '%s\n' "$MOCK_OLD_SERVER_ID"
        fi
        ;;
      coze-web)
        if [ "$restored" -eq 1 ]; then
          printf '%s\n' "$MOCK_RESTORED_WEB_ID"
        elif [ "$updated" -eq 1 ]; then
          printf '%s\n' "$MOCK_RUNNING_WEB_ID"
        else
          printf '%s\n' "$MOCK_OLD_WEB_ID"
        fi
        ;;
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

test_multiline_candidate_revision_keeps_sanitized_failure_record() (
  case_dir=$(mktemp -d "$TEST_ROOT/multiline-candidate.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_SERVER_REVISION=$(printf 'invalid\nrevision')

  if deploy_transaction "" >"$case_dir/output.log" 2>&1; then
    fail 'multiline candidate revision unexpectedly succeeded'
  fi
  assert_file_not_contains "$COMMAND_LOG" '^compose .* up ' \
    'multiline candidate revision reached compose up'
  failure_record=$(failure_record_for "$case_dir" || true)
  [ -n "$failure_record" ] || fail 'multiline candidate revision lost the failure record'
  assert_file_contains "$failure_record" '^CANDIDATE_SERVER_REVISION=<unsafe-multiline-value>$' \
    'multiline candidate revision was not safely redacted'
  assert_file_contains "$failure_record" '^FAILURE_REASON=candidate image revisions must both be full 40-hex SHA values$' \
    'multiline candidate revision lost the failure reason'
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

test_candidate_container_image_mismatch_rolls_back() (
  case_dir=$(mktemp -d "$TEST_ROOT/candidate-image-mismatch.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_RUNNING_WEB_ID=$MOCK_OLD_WEB_ID
  MOCK_ROLLBACK_HEALTHY=1

  if deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'deployment succeeded while the web container kept the old image'
  fi
  assert_file_contains "$case_dir/output.log" 'candidate container image IDs do not match the pulled images' \
    'candidate image identity mismatch was not reported'
  [ ! -f "$DEPLOYMENTS_DIR/current.env" ] || \
    fail 'candidate image mismatch published current.env'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'candidate image mismatch lost the failure record'
  assert_file_contains "$failure_record" '^ROLLBACK_RESULT=succeeded$' \
    'candidate image mismatch did not roll back both services'
  assert_file_contains "$failure_record" '^FAILURE_REASON=candidate container image IDs do not match the pulled images$' \
    'candidate image mismatch lost its specific reason'
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

test_success_record_failure_rolls_back_both_images() (
  case_dir=$(mktemp -d "$TEST_ROOT/record-failure-rollback.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_ROLLBACK_HEALTHY=1
  record_success() {
    return 1
  }

  if deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'deployment succeeded after the success record failed'
  fi

  server_tag=$(awk '/^docker tag sha256:old-server / { sub(/^.*coze-server:/, ""); print }' "$COMMAND_LOG")
  web_tag=$(awk '/^docker tag sha256:old-web / { sub(/^.*coze-web:/, ""); print }' "$COMMAND_LOG")
  [ -n "$server_tag" ] || fail 'success record failure did not tag the old server image'
  [ "$server_tag" = "$web_tag" ] || fail 'success record failure used different rollback transactions'
  assert_file_contains "$COMMAND_LOG" \
    "^compose SERVER_IMAGE_TAG=$server_tag WEB_IMAGE_TAG=$server_tag up -d --no-build --remove-orphans coze-server coze-web$" \
    'success record failure did not restore both services together'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'success record failure did not retain a failure record'
  assert_file_contains "$failure_record" '^ROLLBACK_RESULT=succeeded$' \
    'success record failure lost the rollback result'
  assert_file_contains "$failure_record" '^FAILURE_REASON=healthy candidate success record could not be written$' \
    'success record failure lost its specific reason'
)

test_rollback_rejects_restored_container_image_mismatch() (
  case_dir=$(mktemp -d "$TEST_ROOT/rollback-image-mismatch.XXXXXX")
  setup_transaction_case "$case_dir"
  MOCK_HEALTH_FAILURE=1
  MOCK_ROLLBACK_HEALTHY=1
  MOCK_RESTORED_WEB_ID=sha256:candidate-web

  if deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'deployment succeeded after rollback restored the wrong web image'
  fi
  assert_file_not_contains "$case_dir/output.log" 'rollback succeeded' \
    'rollback claimed success with the wrong web image ID'
  assert_file_contains "$case_dir/output.log" 'rollback container image IDs do not match the saved images' \
    'rollback image identity mismatch was not reported'
  failure_record=$(failure_record_for "$case_dir")
  [ -n "$failure_record" ] || fail 'rollback image mismatch lost the failure record'
  assert_file_contains "$failure_record" '^ROLLBACK_RESULT=failed$' \
    'rollback image mismatch did not preserve a failed rollback result'
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

test_web_health_base_url_uses_configured_host_port() (
  WEB_BIND_IP=0.0.0.0
  WEB_PORT=18888
  actual=$(web_health_base_url) || fail 'wildcard web health URL could not be built'
  [ "$actual" = 'http://127.0.0.1:18888' ] || \
    fail "wildcard web health URL used the wrong host or port: $actual"

  WEB_BIND_IP=192.0.2.10
  WEB_PORT=18889
  actual=$(web_health_base_url) || fail 'specific web health URL could not be built'
  [ "$actual" = 'http://192.0.2.10:18889' ] || \
    fail "specific web health URL used the wrong host or port: $actual"
)

test_web_health_base_url_defaults_port() (
  WEB_BIND_IP=192.0.2.10
  unset WEB_PORT

  actual=$(web_health_base_url) || fail 'default web health URL could not be built'
  [ "$actual" = 'http://192.0.2.10:8888' ] || \
    fail "web health URL did not default to port 8888: $actual"
)

test_health_checks_stop_at_unhealthy_nsqd() (
  case_dir=$(mktemp -d "$TEST_ROOT/unhealthy-nsqd.XXXXXX")
  command_log=$case_dir/commands.log
  : > "$command_log"

  compose_cmd() {
    {
      printf 'compose'
      printf ' %s' "$@"
      printf '\n'
    } >> "$command_log"
    if [ "$#" -eq 3 ] && [ "$1" = ps ] && [ "$2" = -q ] && [ "$3" = nsqd ]; then
      printf 'nsqd-container\n'
    fi
  }

  docker_cmd() {
    {
      printf 'docker'
      printf ' %s' "$@"
      printf '\n'
    } >> "$command_log"
    printf 'starting\n'
  }

  curl() {
    {
      printf 'curl'
      printf ' %s' "$@"
      printf '\n'
    } >> "$command_log"
  }

  if health_checks_pass "$REV_A"; then
    fail 'health checks passed while nsqd was unhealthy'
  fi
  assert_file_contains "$command_log" '^compose ps -q nsqd$' \
    'health checks did not inspect the nsqd service first'
  assert_file_contains "$command_log" '^docker inspect --format \{\{\.State\.Health\.Status\}\} nsqd-container$' \
    'health checks did not inspect the nsqd container health'
  assert_file_not_contains "$command_log" '^compose exec ' \
    'health checks called the backend while nsqd was unhealthy'
  assert_file_not_contains "$command_log" '^curl ' \
    'health checks called the web endpoint while nsqd was unhealthy'
)

test_service_health_status_rejects_empty_inspect_output() (
  case_dir=$(mktemp -d "$TEST_ROOT/empty-nsqd-health.XXXXXX")
  command_log=$case_dir/commands.log
  : > "$command_log"

  compose_cmd() {
    [ "$#" -eq 3 ] && [ "$1" = ps ] && [ "$2" = -q ] && [ "$3" = nsqd ] || return 1
    printf 'nsqd-container\n'
  }

  docker_cmd() {
    {
      printf 'docker'
      printf ' %s' "$@"
      printf '\n'
    } >> "$command_log"
  }

  if service_health_status nsqd; then
    fail 'service health status accepted empty docker inspect output'
  fi
  assert_file_contains "$command_log" '^docker inspect --format \{\{\.State\.Health\.Status\}\} nsqd-container$' \
    'service health status used the wrong docker inspect template'
)

test_health_checks_use_configured_web_url() (
  case_dir=$(mktemp -d "$TEST_ROOT/custom-web-health-url.XXXXXX")
  url_log=$case_dir/urls.log
  WEB_BIND_IP=192.0.2.10
  WEB_PORT=18888
  : > "$url_log"

  service_is_healthy() {
    [ "$1" = nsqd ]
  }

  compose_cmd() {
    [ "$1" = exec ] && [ "$2" = -T ] && [ "$3" = coze-server ] || return 1
    printf '{"status":"ok","revision":"%s"}\n' "$REV_A"
  }

  curl() {
    url=${!#}
    printf '%s\n' "$url" >> "$url_log"
    case "$url" in
      'http://192.0.2.10:18888/healthz')
        printf '{"status":"ok","revision":"%s"}\n' "$REV_A"
        ;;
      'http://192.0.2.10:18888/') return 0 ;;
      *) return 1 ;;
    esac
  }

  health_checks_pass "$REV_A" || fail 'health checks rejected healthy mocked services'
  actual=$(<"$url_log")
  expected=$'http://192.0.2.10:18888/healthz\nhttp://192.0.2.10:18888/'
  [ "$actual" = "$expected" ] || \
    fail "health checks used unexpected web URLs: $actual"
)

test_is_ipv4_rejects_malformed_addresses() (
  for value in \
    '192.0.2' \
    '192.0.2.10.1' \
    '+192.0.2.10' \
    '192.-1.2.10' \
    ' 192.0.2.10' \
    '192.0.2.10 ' \
    '192. 0.2.10' \
    '256.0.0.1' \
    '1.2.3.999' \
    '1..2.3'; do
    if is_ipv4 "$value"; then
      fail "is_ipv4 accepted malformed address: $value"
    fi
  done
)

test_is_tcp_port_rejects_non_strict_values() (
  for value in \
    '0' \
    '+1' \
    '-1' \
    ' 80' \
    '80 ' \
    '65536' \
    '01' \
    '00080'; do
    if is_tcp_port "$value"; then
      fail "is_tcp_port accepted invalid value: $value"
    fi
  done
)

write_direct_case_files() {
  case_dir=$1
  mkdir -p -- "$case_dir/bin"
  printf '%s\n' \
    'ACR_REGISTRY=registry.example' \
    'ACR_NAMESPACE=example' \
    'WEB_BIND_IP=0.0.0.0' \
    'WEB_PORT=8888' \
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

test_invalid_web_bind_ip_fails_before_docker() (
  case_dir=$(mktemp -d "$TEST_ROOT/invalid-web-bind-ip.XXXXXX")
  write_direct_case_files "$case_dir"
  printf 'WEB_BIND_IP=999.0.0.1\n' >> "$case_dir/deploy.env"
  : > "$case_dir/docker-compose.yml"
  marker=$case_dir/docker-called

  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    fail 'invalid WEB_BIND_IP unexpectedly succeeded'
  fi
  [ ! -e "$marker" ] || fail 'invalid WEB_BIND_IP reached Docker'
  assert_file_contains "$case_dir/output.log" 'WEB_BIND_IP must be a valid IPv4 address' \
    'invalid WEB_BIND_IP error was not reported'
)

test_invalid_web_port_fails_before_docker() (
  case_dir=$(mktemp -d "$TEST_ROOT/invalid-web-port.XXXXXX")
  write_direct_case_files "$case_dir"
  printf 'WEB_PORT=65536\n' >> "$case_dir/deploy.env"
  : > "$case_dir/docker-compose.yml"
  marker=$case_dir/docker-called

  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    fail 'invalid WEB_PORT unexpectedly succeeded'
  fi
  [ ! -e "$marker" ] || fail 'invalid WEB_PORT reached Docker'
  assert_file_contains "$case_dir/output.log" 'WEB_PORT must be an integer from 1 to 65535' \
    'invalid WEB_PORT error was not reported'
)

test_logs_never_disclose_secret_sentinels() (
  case_dir=$(mktemp -d "$TEST_ROOT/secrets.XXXXXX")
  write_direct_case_files "$case_dir"
  : > "$case_dir/docker-compose.yml"
  marker=$case_dir/docker-called

  DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1 || true
  [ -e "$marker" ] || fail 'secret leak test did not reach post-environment Docker checks'
  assert_file_contains "$case_dir/output.log" 'Docker Compose v2 is required' \
    'secret leak test did not fail after loading the deployment environment'
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

test_record_success_cleans_temporary_file_when_finalize_fails() (
  case_dir=$(mktemp -d "$TEST_ROOT/atomic-record-failure.XXXXXX")
  DEPLOYMENTS_DIR=$case_dir/deployments
  chmod() {
    return 1
  }

  if record_success "$REV_A" registry.example/coze-server:dev registry.example/coze-web:dev \
    sha256:candidate-server sha256:candidate-web; then
    fail 'record_success unexpectedly succeeded when chmod failed'
  fi
  [ ! -f "$DEPLOYMENTS_DIR/current.env" ] || \
    fail 'record_success published current.env after finalize failure'
  if find "$DEPLOYMENTS_DIR" -maxdepth 1 -name '.current.env.tmp.*' | grep -q .; then
    fail 'record_success left a temporary file after finalize failure'
  fi
)

test_record_failure_cleans_temporary_file_when_finalize_fails() (
  case_dir=$(mktemp -d "$TEST_ROOT/atomic-failure-record.XXXXXX")
  DEPLOYMENTS_DIR=$case_dir/deployments
  chmod() {
    return 1
  }

  if record_failure transaction "$REV_A" "$REV_A" sha256:candidate-server \
    sha256:candidate-web sha256:old-server sha256:old-web "$REV_C" "$REV_C" \
    failed 'test failure'; then
    fail 'record_failure unexpectedly succeeded when chmod failed'
  fi
  if find "$DEPLOYMENTS_DIR" -maxdepth 1 -name 'failed-*.env' | grep -q .; then
    fail 'record_failure published a failure record after finalize failure'
  fi
  if find "$DEPLOYMENTS_DIR" -maxdepth 1 -name '.failed.env.tmp.*' | grep -q .; then
    fail 'record_failure left a temporary file after finalize failure'
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
run_test 'multiline revision retains a safe failure record' test_multiline_candidate_revision_keeps_sanitized_failure_record
run_test 'candidate pull failure stops before update' test_pull_failure_stops_before_up
run_test 'successful deployment records current.env' test_success_records_complete_current_environment
run_test 'successful health still requires candidate image identities' test_candidate_container_image_mismatch_rolls_back
run_test 'health failure rolls back atomically' test_health_failure_rolls_back_both_images_and_stays_failed
run_test 'success record failure rolls back atomically' test_success_record_failure_rolls_back_both_images
run_test 'rollback verifies restored image identities' test_rollback_rejects_restored_container_image_mismatch
run_test 'first deployment failure has no fake rollback' test_first_deployment_failure_cannot_claim_rollback
run_test 'web health URL uses the configured host port' test_web_health_base_url_uses_configured_host_port
run_test 'web health URL defaults to port 8888' test_web_health_base_url_defaults_port
run_test 'unhealthy nsqd blocks application health checks' test_health_checks_stop_at_unhealthy_nsqd
run_test 'empty inspect health status is rejected' test_service_health_status_rejects_empty_inspect_output
run_test 'health checks use the configured web URL' test_health_checks_use_configured_web_url
run_test 'malformed IPv4 addresses are rejected' test_is_ipv4_rejects_malformed_addresses
run_test 'invalid TCP port forms are rejected' test_is_tcp_port_rejects_non_strict_values
run_test 'deployment lock is nonblocking' test_lock_contention_fails_before_transaction
run_test 'invalid SHA fails before Docker' test_invalid_sha_fails_before_docker
run_test 'invalid web bind IP fails before Docker' test_invalid_web_bind_ip_fails_before_docker
run_test 'invalid web port fails before Docker' test_invalid_web_port_fails_before_docker
run_test 'logs do not disclose secret sentinels' test_logs_never_disclose_secret_sentinels
run_test 'success record is atomic and complete' test_record_success_is_atomic_and_complete
run_test 'success record cleans failed temporary file' test_record_success_cleans_temporary_file_when_finalize_fails
run_test 'failure record cleans failed temporary file' test_record_failure_cleans_temporary_file_when_finalize_fails

printf 'deploy tests: %d passed\n' "$passed"
