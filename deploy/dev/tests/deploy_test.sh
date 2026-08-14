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

assert_log_order() {
  file=$1
  first_pattern=$2
  second_pattern=$3
  message=$4
  first_line=$(grep -En -- "$first_pattern" "$file" | head -n 1 | cut -d: -f1 || true)
  second_line=$(grep -En -- "$second_pattern" "$file" | head -n 1 | cut -d: -f1 || true)
  [ -n "$first_line" ] && [ -n "$second_line" ] && [ "$first_line" -lt "$second_line" ] || \
    fail "$message"
}

failure_record_for() {
  find "$1/deployments" -maxdepth 1 -name 'failed-*.env' -print -quit 2>/dev/null
}

setup_transaction_case() {
  CASE_DIR=$1
  DEPLOYMENTS_DIR=$CASE_DIR/deployments
  COMMAND_LOG=$CASE_DIR/commands.log
  WAIT_COUNT_FILE=$CASE_DIR/wait-count
  WAIT_ARGS_LOG=$CASE_DIR/wait-args.log
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
  : > "$WAIT_ARGS_LOG"
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
    printf 'revision %s\n' "$1" >> "$COMMAND_LOG"
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
    printf '%s\n' "$*" >> "$WAIT_ARGS_LOG"
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

setup_runner_transaction_case() {
  setup_transaction_case "$1"
  RUNNER_SESSION_LOG=$CASE_DIR/runner-session.log
  DEPLOY_PROFILE=runner-2c4g
  SANDBOX_RUNNER_REPOSITORY=registry.example/coze-sandbox-runner
  SANDBOX_RUNNER_IMAGE_REF=$SANDBOX_RUNNER_REPOSITORY:dev
  SANDBOX_RUNTIME_REPOSITORY=registry.example/coze-sandbox-runtime
  SANDBOX_RUNTIME_IMAGE_REF=$SANDBOX_RUNTIME_REPOSITORY:dev
  SANDBOX_AIO_IMAGE_REF=ghcr.io/agent-infra/sandbox:latest
  MOCK_OLD_RUNNER_ID=sha256:old-runner
  MOCK_RUNNER_IMAGE_ID=sha256:candidate-runner
  MOCK_RUNNING_RUNNER_ID=$MOCK_RUNNER_IMAGE_ID
  MOCK_AIO_IMAGE_ID=sha256:official-aio-latest
  MOCK_AIO_RAW_FAILURE=0
  MOCK_MIGRATION_STATUS_FAILURE=0
  MOCK_RUNNER_REVISION=$REV_A
  MOCK_RUNTIME_REVISION=$REV_A
  SANDBOX_AIO_STATUS=not-managed
  SANDBOX_AIO_AVAILABLE=false
  : > "$RUNNER_SESSION_LOG"

  compose_cmd() {
    printf 'compose SERVER_IMAGE_TAG=%s WEB_IMAGE_TAG=%s' \
      "${SERVER_IMAGE_TAG-}" "${WEB_IMAGE_TAG-}" >> "$COMMAND_LOG"
    printf ' %s' "$@" >> "$COMMAND_LOG"
    printf '\n' >> "$COMMAND_LOG"
    case " $* " in
      *' coze-sandbox-runner '*)
        printf 'session=%s args=' "${SANDBOX_RUNNER_SESSION_ENABLED-<unset>}" >> "$RUNNER_SESSION_LOG"
        printf '%s ' "$@" >> "$RUNNER_SESSION_LOG"
        printf '\n' >> "$RUNNER_SESSION_LOG"
        ;;
    esac
    if [ "$*" = 'run --rm --no-deps coze-sandbox-runner migration-status' ]; then
      [ "$MOCK_MIGRATION_STATUS_FAILURE" -eq 0 ]
      return
    fi
    [ "$MOCK_COMPOSE_FAILURE" -eq 0 ]
  }

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
      coze-sandbox-runner)
        if [ "$restored" -eq 1 ]; then
          printf '%s\n' "$MOCK_OLD_RUNNER_ID"
        elif [ "$updated" -eq 1 ]; then
          printf '%s\n' "$MOCK_RUNNING_RUNNER_ID"
        else
          printf '%s\n' "$MOCK_OLD_RUNNER_ID"
        fi
        ;;
      coze-sandbox-aio) printf '%s\n' "$MOCK_AIO_IMAGE_ID" ;;
      *) return 1 ;;
    esac
  }

  image_revision() {
    printf 'revision %s\n' "$1" >> "$COMMAND_LOG"
    case "$1" in
      "$SERVER_IMAGE_REF") printf '%s\n' "$MOCK_SERVER_REVISION" ;;
      "$WEB_IMAGE_REF") printf '%s\n' "$MOCK_WEB_REVISION" ;;
      "$SANDBOX_RUNNER_IMAGE_REF") printf '%s\n' "$MOCK_RUNNER_REVISION" ;;
      "$SANDBOX_RUNTIME_IMAGE_REF") printf '%s\n' "$MOCK_RUNTIME_REVISION" ;;
      "$MOCK_OLD_SERVER_ID") printf '%s\n' "$MOCK_OLD_SERVER_REVISION" ;;
      "$MOCK_OLD_WEB_ID") printf '%s\n' "$MOCK_OLD_WEB_REVISION" ;;
      "$MOCK_OLD_RUNNER_ID") printf '%s\n' "$REV_C" ;;
      *) return 1 ;;
    esac
  }

  image_id() {
    case "$1" in
      "$SERVER_IMAGE_REF") printf '%s\n' "$MOCK_SERVER_IMAGE_ID" ;;
      "$WEB_IMAGE_REF") printf '%s\n' "$MOCK_WEB_IMAGE_ID" ;;
      "$SANDBOX_RUNNER_IMAGE_REF") printf '%s\n' "$MOCK_RUNNER_IMAGE_ID" ;;
      "$SANDBOX_AIO_IMAGE_REF") printf '%s\n' "$MOCK_AIO_IMAGE_ID" ;;
      *) return 1 ;;
    esac
  }

  runtime_image_digest_ref() {
    [ "$1" = "$SANDBOX_RUNTIME_IMAGE_REF" ] || return 1
    [ "$2" = "$SANDBOX_RUNTIME_REPOSITORY" ] || return 1
    printf '%s@sha256:%064d\n' "$SANDBOX_RUNTIME_REPOSITORY" 1
  }

  recorded_runtime_image_ref() {
    printf '%s@sha256:%064d\n' "$SANDBOX_RUNTIME_REPOSITORY" 2
  }

  wait_for_aio_raw_health() {
    printf 'raw-aio-health\n' >> "$COMMAND_LOG"
    [ "$MOCK_AIO_RAW_FAILURE" -eq 0 ]
  }
}

test_candidate_runner_session_gate_tracks_aio_availability() (
  failed_case=$(mktemp -d "$TEST_ROOT/runner-session-aio-failed.XXXXXX")
  setup_runner_transaction_case "$failed_case"
  SANDBOX_RUNNER_SESSION_ENABLED=true
  MOCK_AIO_RAW_FAILURE=1

  deploy_transaction "$REV_A" >"$failed_case/output.log" 2>&1 || \
    fail 'AIO failure blocked the fail-closed application deployment'
  assert_file_contains "$RUNNER_SESSION_LOG" \
    '^session=false args=up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web ' \
    'AIO failure did not force the candidate Runner Core gate off'

  healthy_case=$(mktemp -d "$TEST_ROOT/runner-session-aio-healthy.XXXXXX")
  setup_runner_transaction_case "$healthy_case"
  SANDBOX_RUNNER_SESSION_ENABLED=true

  deploy_transaction "$REV_A" >"$healthy_case/output.log" 2>&1 || \
    fail 'healthy AIO rejected an explicitly enabled candidate Runner'
  assert_file_contains "$RUNNER_SESSION_LOG" \
    '^session=true args=up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web ' \
    'healthy AIO did not preserve the explicit Runner Core gate'

  default_case=$(mktemp -d "$TEST_ROOT/runner-session-default.XXXXXX")
  setup_runner_transaction_case "$default_case"
  unset SANDBOX_RUNNER_SESSION_ENABLED

  deploy_transaction "$REV_A" >"$default_case/output.log" 2>&1 || \
    fail 'healthy AIO rejected the default-disabled candidate Runner'
  assert_file_contains "$RUNNER_SESSION_LOG" \
    '^session=false args=up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web ' \
    'candidate Runner Core gate did not default to false'
)

test_official_aio_ref_is_exact_and_readonly_after_environment_load() (
  (
    SANDBOX_AIO_IMAGE_REF=ghcr.io/agent-infra/sandbox:latest
    lock_official_aio_image_ref >/dev/null 2>&1 || \
      fail 'the exact official AIO ref could not be locked'
    declaration=$(declare -p SANDBOX_AIO_IMAGE_REF)
    [[ "$declaration" == 'declare -r'* ]] || \
      fail 'the official AIO ref was not made readonly'
    [ "$SANDBOX_AIO_IMAGE_REF" = ghcr.io/agent-infra/sandbox:latest ] || \
      fail 'locking changed the official latest AIO ref'
  )

  case_dir=$(mktemp -d "$TEST_ROOT/aio-ref-override.XXXXXX")
  write_direct_case_files "$case_dir"
  printf 'SANDBOX_AIO_IMAGE_REF=registry.invalid/private-aio:latest\n' >> "$case_dir/deploy.env"
  : > "$case_dir/docker-compose.yml"
  marker=$case_dir/docker-called

  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    fail 'deployment accepted an override of the official latest AIO ref'
  fi
  [ ! -e "$marker" ] || fail 'AIO ref override reached Docker'
  assert_file_contains "$case_dir/output.log" 'official AIO image ref cannot be overridden' \
    'AIO ref override did not fail closed with a safe error'
  assert_file_not_contains "$case_dir/output.log" 'registry\.invalid|private-aio' \
    'AIO ref override leaked the untrusted image ref'
)

test_runner_deployment_orders_official_aio_before_runner_and_records_evidence() (
  case_dir=$(mktemp -d "$TEST_ROOT/runner-aio-order.XXXXXX")
  setup_runner_transaction_case "$case_dir"

  deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1 || \
    fail 'runner deployment with healthy official AIO failed'

  assert_log_order "$COMMAND_LOG" \
    '^docker pull ghcr\.io/agent-infra/sandbox:latest$' \
    '^compose .* up -d --no-build coze-sandbox-aio$' \
    'official AIO was started before its latest image was pulled'
  assert_log_order "$COMMAND_LOG" \
    '^compose .* up -d --no-build coze-sandbox-aio$' \
    '^raw-aio-health$' \
    'official AIO raw health ran before the AIO service started'
  assert_log_order "$COMMAND_LOG" \
    '^raw-aio-health$' \
    '^compose SERVER_IMAGE_TAG=dev WEB_IMAGE_TAG=dev up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web$' \
    'Runner started before official AIO raw health completed'

  current=$DEPLOYMENTS_DIR/current.env
  assert_file_contains "$current" '^SANDBOX_AIO_IMAGE_REF=ghcr\.io/agent-infra/sandbox:latest$' \
    'deployment evidence lost the official latest AIO ref'
  assert_file_contains "$current" '^SANDBOX_AIO_IMAGE_ID=sha256:official-aio-latest$' \
    'deployment evidence lost the observed official AIO image ID'
  assert_file_contains "$current" '^SANDBOX_AIO_STATUS=healthy$' \
    'deployment evidence did not distinguish AIO health from image identity'
)

test_aio_first_start_failure_keeps_application_and_one_shot_available() (
  case_dir=$(mktemp -d "$TEST_ROOT/runner-aio-failure.XXXXXX")
  setup_runner_transaction_case "$case_dir"
  MOCK_AIO_RAW_FAILURE=1

  deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1 || \
    fail 'official AIO first-start failure blocked the application deployment'

  assert_file_contains "$COMMAND_LOG" \
    '^compose SERVER_IMAGE_TAG=dev WEB_IMAGE_TAG=dev up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web$' \
    'AIO failure blocked server/web/one-shot Runner startup'
  [ "$(grep -Ec '^compose .* up -d --no-build coze-sandbox-aio$' "$COMMAND_LOG")" -eq 1 ] || \
    fail 'AIO first-start failure triggered a restart loop'
  assert_file_contains "$case_dir/output.log" 'official AIO is unavailable; Core remains disabled' \
    'AIO failure was not reported as a fail-closed Core degradation'
  assert_file_contains "$DEPLOYMENTS_DIR/current.env" '^SANDBOX_AIO_STATUS=unavailable$' \
    'successful application deployment hid the unavailable AIO state'
)

test_aio_raw_health_uses_only_private_8080_endpoints() (
  case_dir=$(mktemp -d "$TEST_ROOT/aio-raw-health.XXXXXX")
  command_log=$case_dir/commands.log
  : > "$command_log"

  container_private_ipv4() {
    [ "$1" = coze-sandbox-aio ] || return 1
    printf '172.20.0.8\n'
  }
  curl() {
    output_file=
    url=${!#}
    {
      printf 'curl'
      printf ' %s' "$@"
      printf '\n'
    } >> "$command_log"
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --output)
          shift
          output_file=$1
          ;;
      esac
      shift
    done
    case "$url" in
      */v1/ping) response_body=pong ;;
      */v1/sandbox) response_body='{"home_dir":"/home/gem","version":"1.11.0","detail":{"system":{},"runtime":{},"utils":[]}}' ;;
      *) return 1 ;;
    esac
    printf '%s' "$response_body" > "$output_file"
    printf '200'
  }

  aio_raw_health_checks_pass || fail 'private official AIO raw health was rejected'
  assert_file_contains "$command_log" \
    'http://172\.20\.0\.8:8080/v1/ping$' \
    'AIO health did not probe raw /v1/ping over the private 8080 origin'
  assert_file_contains "$command_log" \
    'http://172\.20\.0\.8:8080/v1/sandbox$' \
    'AIO health did not probe raw /v1/sandbox over the private 8080 origin'
  assert_file_not_contains "$command_log" '8090|sessiond|https?://(127\.0\.0\.1|localhost)' \
    'AIO health used a legacy/internal or loopback endpoint'
)

test_aio_raw_health_requires_exact_bounded_response_contract() (
  case_dir=$(mktemp -d "$TEST_ROOT/aio-raw-contract.XXXXXX")
  command_log=$case_dir/commands.log
  : > "$command_log"
  TMPDIR=$case_dir
  PING_HTTP_STATUS=200
  PING_BODY=pong
  SANDBOX_HTTP_STATUS=200
  SANDBOX_BODY='{"home_dir":"/home/gem","version":"1.11.0","detail":{"system":{},"runtime":{},"utils":[]}}'

  container_private_ipv4() {
    [ "$1" = coze-sandbox-aio ] || return 1
    printf '172.20.0.8\n'
  }
  curl() {
    output_file=
    url=${!#}
    {
      printf 'curl'
      printf ' %s' "$@"
      printf '\n'
    } >> "$command_log"
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --output)
          shift
          output_file=$1
          ;;
      esac
      shift
    done
    case "$url" in
      */v1/ping)
        response_status=$PING_HTTP_STATUS
        response_body=$PING_BODY
        ;;
      */v1/sandbox)
        response_status=$SANDBOX_HTTP_STATUS
        response_body=$SANDBOX_BODY
        ;;
      *) return 1 ;;
    esac
    if [ -n "$output_file" ]; then
      printf '%s' "$response_body" > "$output_file"
    fi
    printf '%s' "$response_status"
  }

  aio_raw_health_checks_pass >"$case_dir/valid.log" 2>&1 || \
    fail 'exact bounded official AIO responses were rejected'

  PING_HTTP_STATUS=302
  PING_BODY='redirect-secret-body'
  if aio_raw_health_checks_pass >"$case_dir/redirect.log" 2>&1; then
    fail 'AIO raw health accepted a redirect response'
  fi
  assert_file_not_contains "$case_dir/redirect.log" 'redirect-secret-body' \
    'AIO redirect body escaped into deployment output'

  PING_HTTP_STATUS=200
  PING_BODY=$'pong\n'
  if aio_raw_health_checks_pass >"$case_dir/ping-newline.log" 2>&1; then
    fail 'AIO raw health accepted pong with a trailing newline'
  fi

  PING_BODY=pong
  SANDBOX_BODY='{"home_dir":"/home/gem","version":"1.11.0"}'
  if aio_raw_health_checks_pass >"$case_dir/malformed-sandbox.log" 2>&1; then
    fail 'AIO raw health accepted a sandbox response without detail context'
  fi

  SANDBOX_BODY='{"home_dir":"/home/gem","version":"1.11.0","detail":{"system":{},"runtime":{},"utils":[]},"secret":"bounded-secret-marker'
  SANDBOX_BODY+=$(printf '%05000d' 0)
  SANDBOX_BODY+='"}'
  if aio_raw_health_checks_pass >"$case_dir/oversized.log" 2>&1; then
    fail 'AIO raw health accepted an oversized sandbox response'
  fi
  assert_file_not_contains "$case_dir/oversized.log" 'bounded-secret-marker' \
    'oversized AIO body escaped into deployment output'

  assert_file_contains "$command_log" '--max-redirs 0' \
    'AIO raw health did not explicitly disable redirects'
  assert_file_contains "$command_log" '--max-filesize 4096' \
    'AIO raw health did not bound response bytes at curl'
)

test_runner_core_projection_requires_consistent_generation() (
  runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"disabled","aio_runtime_generation":0}' false || \
    fail 'disabled Core projection with generation zero was rejected'
  runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"ready","aio_runtime_generation":7}' true || \
    fail 'ready Core projection with a positive generation was rejected'
  if runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"ready","aio_runtime_generation":7}' false; then
    fail 'ready Core projection was accepted while raw AIO was unavailable'
  fi
  if runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"disabled","aio_runtime_generation":7}' true; then
    fail 'disabled Core projection was accepted with a nonzero generation'
  fi
  if runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"unknown","aio_runtime_generation":0}' true; then
    fail 'unknown Core projection was accepted as healthy'
  fi
)

test_runner_core_projection_allows_legacy_only_during_disabled_rollback() (
  legacy='{"schema":"coze.sandbox.runner_runtime_status.v1","applied_configuration_version":1,"queued":0,"running":0,"used_weight":0,"total_weight":2,"memory_reserve_state":"available"}'

  if runner_core_projection_matches "$legacy" false; then
    fail 'candidate health accepted a legacy Runner projection without Core fields'
  fi
  runner_core_projection_matches "$legacy" false true || \
    fail 'disabled rollback rejected the legacy pre-Task11 Runner projection'
  runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"disabled","aio_runtime_generation":0}' true true || \
    fail 'disabled rollback rejected a current Runner disabled/zero projection'
  if runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"ready","aio_runtime_generation":7}' true true; then
    fail 'rollback accepted a current Runner that was not disabled'
  fi
  if runner_core_projection_matches \
    '{"schema":"coze.sandbox.runner_runtime_status.v1","core_state":"disabled"}' false true; then
    fail 'rollback treated a partially upgraded Runner projection as legacy'
  fi
)

test_runner_rollback_disables_core_first_and_preserves_aio_state() (
  case_dir=$(mktemp -d "$TEST_ROOT/runner-safe-rollback.XXXXXX")
  setup_runner_transaction_case "$case_dir"
  MOCK_HEALTH_FAILURE=1
  MOCK_ROLLBACK_HEALTHY=1

  if deploy_transaction "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'failed runner candidate unexpectedly succeeded after rollback'
  fi

  assert_log_order "$COMMAND_LOG" \
    '^compose SERVER_IMAGE_TAG= WEB_IMAGE_TAG= stop coze-sandbox-runner$' \
    '^compose SERVER_IMAGE_TAG=rollback-.* WEB_IMAGE_TAG=rollback-.* up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web$' \
    'rollback restored application images before disabling Core capability'
  assert_file_not_contains "$COMMAND_LOG" \
    'down([[:space:]]|$)|[[:space:]]-v([[:space:]]|$)|volume rm|/mnt/user-data|SERVER_IMAGE_TAG=rollback-.*coze-sandbox-aio' \
    'rollback destroyed persistent state or tried to replace the unpinned AIO image'
  assert_file_contains "$case_dir/output.log" \
    'official latest AIO cannot be rolled back exactly; Core remains disabled' \
    'rollback hid the exact-rollback limitation of official latest'
  expected_wait_args=$(printf '%s\n%s true' "$REV_A" "$REV_C")
  actual_wait_args=$(<"$WAIT_ARGS_LOG")
  [ "$actual_wait_args" = "$expected_wait_args" ] || \
    fail "candidate/rollback health used the wrong legacy compatibility scope: $actual_wait_args"
)

test_migration_preflight_is_exact_and_read_only() (
  case_dir=$(mktemp -d "$TEST_ROOT/migration-preflight.XXXXXX")
  setup_runner_transaction_case "$case_dir"
  MYSQL_DSN='sentinel-migration-dsn-secret'

  declare -F preflight_additive_migration >/dev/null || \
    fail 'deploy is missing the additive migration preflight'
  preflight_additive_migration >"$case_dir/output.log" 2>&1 || \
    fail 'additive migration preflight rejected the supported migration'
  assert_file_contains "$COMMAND_LOG" \
    '^compose SERVER_IMAGE_TAG= WEB_IMAGE_TAG= run --rm --no-deps coze-sandbox-runner migration-status$' \
    'migration preflight did not invoke the candidate Runner read-only status CLI'
  assert_file_contains "$RUNNER_SESSION_LOG" \
    '^session=false args=run --rm --no-deps coze-sandbox-runner migration-status ' \
    'migration preflight did not force Core off in its one-shot Runner'
  assert_file_contains "$case_dir/output.log" '20260814000100' \
    'migration preflight did not identify the one required additive migration'
  assert_file_not_contains "$COMMAND_LOG" '20260814000100|sentinel-migration-dsn-secret|MYSQL_DSN' \
    'migration preflight put the migration ID or DSN in argv'
  assert_file_not_contains "$case_dir/output.log" 'sentinel-migration-dsn-secret' \
    'migration preflight leaked its dev database DSN'
  assert_file_not_contains "$DEPLOY_SCRIPT" \
    'migrate[[:space:]]+apply|AutoMigrate|DROP[[:space:]]+(TABLE|DATABASE)|TRUNCATE[[:space:]]+TABLE|docker[[:space:]]+volume[[:space:]]+rm|compose_cmd[[:space:]]+down' \
    'deploy contains migration apply, destructive database, or destructive volume lifecycle'
)

test_migration_preflight_uses_validated_candidate_and_blocks_all_up_on_failure() (
  success_case=$(mktemp -d "$TEST_ROOT/migration-order.XXXXXX")
  setup_runner_transaction_case "$success_case"

  deploy_transaction "$REV_A" >"$success_case/output.log" 2>&1 || \
    fail 'deployment with an applied migration unexpectedly failed'
  assert_log_order "$COMMAND_LOG" \
    '^revision registry\.example/coze-sandbox-runner:dev$' \
    '^compose SERVER_IMAGE_TAG= WEB_IMAGE_TAG= run --rm --no-deps coze-sandbox-runner migration-status$' \
    'migration preflight ran before candidate Runner revision validation'
  assert_log_order "$COMMAND_LOG" \
    '^compose SERVER_IMAGE_TAG= WEB_IMAGE_TAG= run --rm --no-deps coze-sandbox-runner migration-status$' \
    '^docker pull ghcr\.io/agent-infra/sandbox:latest$' \
    'AIO pull/up began before the migration status preflight passed'

  failure_case=$(mktemp -d "$TEST_ROOT/migration-failure.XXXXXX")
  setup_runner_transaction_case "$failure_case"
  MOCK_MIGRATION_STATUS_FAILURE=1

  if deploy_transaction "$REV_A" >"$failure_case/output.log" 2>&1; then
    fail 'deployment succeeded after migration status verification failed'
  fi
  assert_file_contains "$COMMAND_LOG" \
    '^compose SERVER_IMAGE_TAG= WEB_IMAGE_TAG= run --rm --no-deps coze-sandbox-runner migration-status$' \
    'migration failure path skipped the read-only Runner CLI'
  assert_file_not_contains "$COMMAND_LOG" '^compose .* up ' \
    'migration status failure reached an AIO or application compose up'
  assert_file_not_contains "$COMMAND_LOG" '^docker pull ghcr\.io/agent-infra/sandbox:latest$' \
    'migration status failure pulled official AIO before blocking deployment'
  assert_file_contains "$failure_case/output.log" 'required additive migration is not applied' \
    'migration status failure was not reported with a safe reason'
  assert_file_not_contains "$failure_case/output.log" 'MYSQL_DSN|@tcp|sentinel' \
    'migration status failure leaked database connection details'
)

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

test_wait_for_health_retries_then_succeeds() (
  DEPLOY_HEALTH_TIMEOUT_SECONDS=10
  SECONDS=0
  attempts=0
  sleeps=0

  health_checks_pass() {
    [ "$1" = "$REV_A" ] || fail 'wait_for_health forwarded the wrong revision'
    attempts=$((attempts + 1))
    [ "$attempts" -eq 2 ]
  }

  sleep() {
    [ "$1" = 2 ] || fail 'wait_for_health used the wrong retry interval'
    sleeps=$((sleeps + 1))
    SECONDS=$((SECONDS + 2))
  }

  wait_for_health "$REV_A" || fail 'wait_for_health rejected a successful retry'
  [ "$attempts" -eq 2 ] || fail "wait_for_health made $attempts attempts before success"
  [ "$sleeps" -eq 1 ] || fail "wait_for_health slept $sleeps times before success"
)

test_wait_for_health_stops_after_timeout() (
  case_dir=$(mktemp -d "$TEST_ROOT/health-timeout.XXXXXX")
  DEPLOY_HEALTH_TIMEOUT_SECONDS=5
  SECONDS=0
  attempts=0
  sleeps=0

  health_checks_pass() {
    [ "$1" = "$REV_A" ] || fail 'wait_for_health forwarded the wrong revision'
    attempts=$((attempts + 1))
    return 1
  }

  sleep() {
    [ "$1" = 2 ] || fail 'wait_for_health used the wrong retry interval'
    sleeps=$((sleeps + 1))
    SECONDS=$((SECONDS + 2))
  }

  if wait_for_health "$REV_A" >"$case_dir/output.log" 2>&1; then
    fail 'wait_for_health succeeded after persistent failures'
  fi
  [ "$attempts" -eq 3 ] || fail "wait_for_health made $attempts attempts before timeout"
  [ "$sleeps" -eq 3 ] || fail "wait_for_health slept $sleeps times before timeout"
  assert_file_contains "$case_dir/output.log" 'health checks did not pass within 5s' \
    'wait_for_health did not report the configured timeout'
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
    '01.2.3.4' \
    '1.02.3.4' \
    '1.2.03.4' \
    '1.2.3.04' \
    '00.0.0.0' \
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

test_leading_zero_web_bind_ip_fails_before_docker() (
  case_dir=$(mktemp -d "$TEST_ROOT/leading-zero-web-bind-ip.XXXXXX")
  write_direct_case_files "$case_dir"
  printf 'WEB_BIND_IP=01.2.3.4\n' >> "$case_dir/deploy.env"
  : > "$case_dir/docker-compose.yml"
  marker=$case_dir/docker-called

  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    fail 'leading-zero WEB_BIND_IP unexpectedly succeeded'
  fi
  [ ! -e "$marker" ] || fail 'leading-zero WEB_BIND_IP reached Docker'
  assert_file_contains "$case_dir/output.log" 'WEB_BIND_IP must be a valid IPv4 address' \
    'leading-zero WEB_BIND_IP error was not reported'
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

test_runner_private_file_validation_honors_override_and_rejects_weak_mode() (
	case_dir=$(mktemp -d "$TEST_ROOT/runner-private.XXXXXX")
	override=$case_dir/custom-runner.env
	printf 'SANDBOX_RUNNER_AUTH_TOKEN=test-only\n' > "$override"
	chmod 0644 "$override"
	if validate_private_deploy_file "$override" 'runner environment' "$(id -u)"; then
		fail 'runner private file validation accepted mode 0644'
	fi
	chmod 0600 "$override"
	if ! validate_private_deploy_file "$override" 'runner environment' "$(id -u)"; then
		fail 'runner private file validation rejected owned mode 0600 override'
	fi
	if [ "$(id -u)" != 10001 ] && validate_private_deploy_file "$override" 'runner TLS key' 10001; then
		fail 'runner TLS validation accepted a file not owned by container UID 10001'
	fi
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

run_test 'official AIO starts before Runner and records evidence' test_runner_deployment_orders_official_aio_before_runner_and_records_evidence
run_test 'candidate Runner gate follows AIO availability' test_candidate_runner_session_gate_tracks_aio_availability
run_test 'official AIO ref is exact and readonly' test_official_aio_ref_is_exact_and_readonly_after_environment_load
run_test 'official AIO failure preserves application and one-shot' test_aio_first_start_failure_keeps_application_and_one_shot_available
run_test 'official AIO health uses private raw 8080 only' test_aio_raw_health_uses_only_private_8080_endpoints
run_test 'official AIO health requires exact bounded responses' test_aio_raw_health_requires_exact_bounded_response_contract
run_test 'Runner Core projection fences generation' test_runner_core_projection_requires_consistent_generation
run_test 'Runner legacy projection is rollback-only' test_runner_core_projection_allows_legacy_only_during_disabled_rollback
run_test 'Runner rollback disables Core and preserves AIO state' test_runner_rollback_disables_core_first_and_preserves_aio_state
run_test 'migration preflight is exact and read-only' test_migration_preflight_is_exact_and_read_only
run_test 'migration preflight uses candidate Runner and blocks service up' test_migration_preflight_uses_validated_candidate_and_blocks_all_up_on_failure
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
run_test 'health wait retries transient failures' test_wait_for_health_retries_then_succeeds
run_test 'health wait stops after persistent failures' test_wait_for_health_stops_after_timeout
run_test 'leading-zero web bind IP fails before Docker' test_leading_zero_web_bind_ip_fails_before_docker
run_test 'malformed IPv4 addresses are rejected' test_is_ipv4_rejects_malformed_addresses
run_test 'invalid TCP port forms are rejected' test_is_tcp_port_rejects_non_strict_values
run_test 'deployment lock is nonblocking' test_lock_contention_fails_before_transaction
run_test 'invalid SHA fails before Docker' test_invalid_sha_fails_before_docker
run_test 'invalid web bind IP fails before Docker' test_invalid_web_bind_ip_fails_before_docker
run_test 'invalid web port fails before Docker' test_invalid_web_port_fails_before_docker
run_test 'logs do not disclose secret sentinels' test_logs_never_disclose_secret_sentinels
run_test 'runner private files honor overrides and require mode 0600' test_runner_private_file_validation_honors_override_and_rejects_weak_mode
run_test 'success record is atomic and complete' test_record_success_is_atomic_and_complete
run_test 'success record cleans failed temporary file' test_record_success_cleans_temporary_file_when_finalize_fails
run_test 'failure record cleans failed temporary file' test_record_failure_cleans_temporary_file_when_finalize_fails

printf 'deploy tests: %d passed\n' "$passed"
