#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -Eeuo pipefail

umask 077

HARNESS_SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
HARNESS_SCRIPT_PATH=$HARNESS_SCRIPT_DIR/$(basename -- "${BASH_SOURCE[0]}")
HARNESS_REPO_ROOT=$(CDPATH= cd -- "$HARNESS_SCRIPT_DIR/../../.." && pwd)
HARNESS_DEPLOY_DIR=$HARNESS_REPO_ROOT/deploy/dev
HARNESS_COMPOSE_FILE=$HARNESS_DEPLOY_DIR/docker-compose.runner-2c4g.yml
HARNESS_DEPLOY_HELPER=$HARNESS_DEPLOY_DIR/deploy.sh
HARNESS_OFFICIAL_AIO_REF=ghcr.io/agent-infra/sandbox:latest
HARNESS_GO_TEST_NAME=TestAIOCoreE2E
HARNESS_GO_TEST_REGEX='^TestAIOCoreE2E$'
HARNESS_BLOCKED_EXIT=78
HARNESS_RESULT=failed
HARNESS_ARMED=false
HARNESS_DEPLOY_INITIALIZED=false
HARNESS_RUNNER_CONTAINER_ID=
HARNESS_RUNNER_IMAGE_ID=
HARNESS_RUNNER_IMAGE_REVISION=

# Equivalent focused invocation: go test ... -run '^TestAIOCoreE2E$'.

fail() {
  printf 'AIO Core E2E failure: %s\n' "$1" >&2
  exit 1
}

blocked() {
  HARNESS_RESULT=blocked
  printf 'BLOCKED: %s\n' "$1" >&2
  exit "$HARNESS_BLOCKED_EXIT"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || blocked "required tool is unavailable: $1"
}

file_mode() {
  stat -c '%a' -- "$1" 2>/dev/null || stat -f '%Lp' -- "$1" 2>/dev/null
}

file_owner() {
  stat -c '%u' -- "$1" 2>/dev/null || stat -f '%u' -- "$1" 2>/dev/null
}

validate_private_environment_file() {
  local file=$1 mode owner

  [ "${file#/}" != "$file" ] || fail 'controlled environment file must use an absolute path'
  [ -f "$file" ] && [ ! -L "$file" ] || blocked 'controlled environment file is unavailable'
  mode=$(file_mode "$file") || fail 'controlled environment file mode cannot be verified'
  owner=$(file_owner "$file") || fail 'controlled environment file owner cannot be verified'
  [ "$mode" = 600 ] || fail 'controlled environment file must have mode 0600'
  [ "$owner" = "$(id -u)" ] || fail 'controlled environment file must be owned by the current user'
}

load_controlled_environment() {
  local file=$1 line key value first last seen_keys=$'\n'

  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    if [[ "$line" =~ ^[[:space:]]*$ ]] || [[ "$line" =~ ^[[:space:]]*# ]]; then
      continue
    fi
    [[ "$line" == *=* ]] || fail 'controlled environment file contains a non-assignment line'
    key=${line%%=*}
    value=${line#*=}
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || fail 'controlled environment file contains an invalid key'
    case "$seen_keys" in
      *$'\n'"$key"$'\n'*) fail 'controlled environment file contains a duplicate key' ;;
    esac
    seen_keys+="$key"$'\n'
    case "$key" in
      HARNESS_*|SANDBOX_AIO_CORE_E2E_INTERNAL_*|SANDBOX_AIO_CORE_E2E_CONTROL_HELPER|SANDBOX_AIO_CORE_E2E_IMAGE_ID|SANDBOX_AIO_CORE_E2E_HELPER_ARMED|SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR)
        fail 'controlled environment file cannot override harness-owned state'
        ;;
      COMPOSE_*|SANDBOX_RUNNER_ENV_FILE)
        fail 'controlled environment file cannot override Compose selection controls'
        ;;
      GIT_*|GO111MODULE|GO386|GOAMD64|GOARCH|GOARM|GODEBUG|GOENV|GOEXPERIMENT|GOFLAGS|GOMIPS|GOMIPS64|GOMOD|GOMODCACHE|GONOPROXY|GONOSUMDB|GOOS|GOPATH|GOPPC64|GOPRIVATE|GOPROXY|GORISCV64|GOROOT|GOSUMDB|GOTOOLDIR|GOTOOLCHAIN|GOTMPDIR|GOVCS|GOWASM|GOWORK|CC|CGO_ENABLED|CXX|PKG_CONFIG|PKG_CONFIG_PATH)
        fail 'controlled environment file cannot override Git or Go execution controls'
        ;;
      BASH_*|CDPATH|DYLD_*|ENV|GLOBIGNORE|IFS|LD_*|PATH|SHELLOPTS)
        fail 'controlled environment file contains a forbidden shell control key'
        ;;
    esac
    if [ "${#value}" -gt 0 ]; then
      first=${value:0:1}
      last=${value:${#value}-1:1}
      if [ "$first" = "'" ] || [ "$first" = '"' ]; then
        [ "$last" = "$first" ] || fail 'controlled environment file contains an unterminated quoted value'
        value=${value:1:${#value}-2}
      elif [ "$last" = "'" ] || [ "$last" = '"' ]; then
        fail 'controlled environment file contains an unmatched quote'
      fi
    fi
    printf -v "$key" '%s' "$value"
    export "$key"
  done < "$file"
}

variable_is_set() {
  declare -p "$1" >/dev/null 2>&1
}

require_nonempty_variable() {
  local name=$1 value

  if ! variable_is_set "$name"; then
    blocked "required controlled setting is missing: $name"
  fi
  value=${!name}
  [ -n "$value" ] || blocked "required controlled setting is empty: $name"
}

validate_exact_gate() {
  local name=$1 expected=$2 value

  if ! variable_is_set "$name"; then
    return "$HARNESS_BLOCKED_EXIT"
  fi
  value=${!name}
  [ -n "$value" ] || return "$HARNESS_BLOCKED_EXIT"
  [ "$value" = "$expected" ] || return 1
}

require_exact_gate() {
  local name=$1 expected=$2 status

  set +e
  validate_exact_gate "$name" "$expected"
  status=$?
  set -e
  case "$status" in
    0) ;;
    "$HARNESS_BLOCKED_EXIT") blocked "required safety acknowledgement is missing: $name" ;;
    *) fail "required safety acknowledgement is invalid: $name" ;;
  esac
}

valid_positive_int64() {
  local value=$1 LC_ALL=C

  [[ "$value" =~ ^[1-9][0-9]*$ ]] || return 1
  [ "${#value}" -lt 19 ] && return 0
  [ "${#value}" -eq 19 ] || return 1
  [[ "$value" < 9223372036854775808 ]]
}

validate_fixture_identity_contract() {
  local prefix=${SANDBOX_AIO_CORE_E2E_PREFIX:-}
  local deployment=${SANDBOX_RUNNER_DEPLOYMENT_ID:-}
  local space_id=${SANDBOX_AIO_CORE_E2E_SPACE_ID:-}
  local user_a=${SANDBOX_AIO_CORE_E2E_USER_ID_A:-}
  local user_b=${SANDBOX_AIO_CORE_E2E_USER_ID_B:-}

  [ -n "$prefix" ] && [ -n "$deployment" ] || return "$HARNESS_BLOCKED_EXIT"
  [[ "$prefix" =~ ^aio-e2e-[a-z0-9][a-z0-9-]+$ ]] || return 1
  [ "${#prefix}" -ge 16 ] && [ "${#prefix}" -le 48 ] || return 1
  [ "$prefix" = "$deployment" ] || return 1
  [ -n "$space_id" ] && [ -n "$user_a" ] && [ -n "$user_b" ] || return "$HARNESS_BLOCKED_EXIT"
  valid_positive_int64 "$space_id" || return 1
  valid_positive_int64 "$user_a" || return 1
  valid_positive_int64 "$user_b" || return 1
  [ "$user_a" != "$user_b" ] || return 1
}

require_fixture_identity_contract() {
  local status

  set +e
  validate_fixture_identity_contract
  status=$?
  set -e
  case "$status" in
    0) ;;
    "$HARNESS_BLOCKED_EXIT") blocked 'unique isolated fixture identity settings are incomplete' ;;
    *) fail 'unique isolated fixture identity settings are invalid or inconsistent' ;;
  esac
}

resolve_code_revision() {
  env -u GIT_ALTERNATE_OBJECT_DIRECTORIES \
    -u GIT_CEILING_DIRECTORIES \
    -u GIT_COMMON_DIR \
    -u GIT_DIR \
    -u GIT_DISCOVERY_ACROSS_FILESYSTEM \
    -u GIT_INDEX_FILE \
    -u GIT_NAMESPACE \
    -u GIT_OBJECT_DIRECTORY \
    -u GIT_WORK_TREE \
    git -C "$HARNESS_REPO_ROOT" rev-parse HEAD
}

clean_git() {
  env -u GIT_ALTERNATE_OBJECT_DIRECTORIES \
    -u GIT_CEILING_DIRECTORIES \
    -u GIT_COMMON_DIR \
    -u GIT_DIR \
    -u GIT_DISCOVERY_ACROSS_FILESYSTEM \
    -u GIT_INDEX_FILE \
    -u GIT_NAMESPACE \
    -u GIT_OBJECT_DIRECTORY \
    -u GIT_WORK_TREE \
    git -C "$HARNESS_REPO_ROOT" "$@"
}

worktree_snapshot_is_clean() {
  local expected_root=$1 actual_root=$2 expected_sha=$3 actual_sha=$4 porcelain=$5

  [ "$actual_root" = "$expected_root" ] &&
    [[ "$actual_sha" =~ ^[0-9a-f]{40}$ ]] &&
    [ "$actual_sha" = "$expected_sha" ] &&
    [ -z "$porcelain" ]
}

validate_clean_worktree_snapshot() {
  local actual_root actual_sha porcelain relative_script

  actual_root=$(clean_git rev-parse --show-toplevel 2>/dev/null) || fail 'dedicated test worktree root cannot be resolved'
  actual_sha=$(clean_git rev-parse HEAD 2>/dev/null) || fail 'dedicated test worktree HEAD cannot be resolved'
  porcelain=$(clean_git status --porcelain=v1 --untracked-files=all --ignore-submodules=none 2>/dev/null) ||
    fail 'dedicated test worktree status cannot be resolved'
  worktree_snapshot_is_clean "$HARNESS_REPO_ROOT" "$actual_root" "$SANDBOX_AIO_CORE_E2E_CODE_SHA" "$actual_sha" "$porcelain" ||
    fail 'dedicated test worktree must be completely clean against the configured HEAD'
  relative_script=${HARNESS_SCRIPT_PATH#"$HARNESS_REPO_ROOT"/}
  [ "$relative_script" != "$HARNESS_SCRIPT_PATH" ] || fail 'harness path escaped the dedicated test worktree'
  clean_git cat-file -e "$actual_sha:$relative_script" 2>/dev/null ||
    fail 'AIO Core E2E harness must be committed at the configured HEAD'
}

runner_revision_matches_sha() {
  local image_id=$1 image_revision=$2 expected_sha=$3 image_hex

  image_hex=${image_id#sha256:}
  [ "$image_hex" != "$image_id" ] && [ "${#image_hex}" -eq 64 ] || return 1
  [[ "$image_hex" =~ ^[0-9a-f]{64}$ ]] || return 1
  [[ "$image_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
  [ "$image_revision" = "$expected_sha" ]
}

runner_endpoint_matches_container() {
  local runner_url=$1 container_ip=$2 authority host port

  case "$runner_url" in
    https://*) authority=${runner_url#https://} ;;
    *) return 1 ;;
  esac
  case "$authority" in
    */*|*'?'*|*'#'*) return 1 ;;
  esac
  host=${authority%:*}
  port=${authority##*:}
  [ -n "$host" ] && [ "$host" != "$authority" ] && [ "$host" = "$container_ip" ] && [ "$port" = 9443 ]
}

compose_service_container_id() {
  local service=$1 ids

  ids=$(compose_cmd ps -q "$service" 2>/dev/null) || return 1
  set -- $ids
  [ "$#" -eq 1 ] || return 1
  printf '%s\n' "$1"
}

container_label() {
  docker inspect --format "{{ index .Config.Labels \"$2\" }}" "$1" 2>/dev/null
}

container_environment_value() {
  local container_id=$1 wanted=$2 values count

  values=$(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$container_id" 2>/dev/null |
    awk -F= -v wanted="$wanted" '$1 == wanted { print substr($0, index($0, "=") + 1) }') || return 1
  count=$(printf '%s\n' "$values" | awk 'NF { count++ } END { print count+0 }')
  [ "$count" -eq 1 ] || return 1
  printf '%s\n' "$values"
}

container_network() {
  local container_id=$1 networks

  networks=$(docker inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$container_id" 2>/dev/null) || return 1
  set -- $networks
  [ "$#" -eq 1 ] || return 1
  printf '%s\n' "$1"
}

validate_compose_target_binding() {
  local runner_id aio_id project runner_project aio_project runner_service aio_service
  local runner_deployment runner_upstream runner_ip runner_network aio_network aio_aliases
  local runner_image_id runner_image_revision

  runner_id=$(compose_service_container_id coze-sandbox-runner) || return 1
  aio_id=$(compose_service_container_id coze-sandbox-aio) || return 1
  project=$SANDBOX_RUNNER_DEPLOYMENT_ID
  runner_project=$(container_label "$runner_id" com.docker.compose.project) || return 1
  aio_project=$(container_label "$aio_id" com.docker.compose.project) || return 1
  runner_service=$(container_label "$runner_id" com.docker.compose.service) || return 1
  aio_service=$(container_label "$aio_id" com.docker.compose.service) || return 1
  [ "$runner_project" = "$project" ] && [ "$aio_project" = "$project" ] || return 1
  [ "$runner_service" = coze-sandbox-runner ] && [ "$aio_service" = coze-sandbox-aio ] || return 1

  runner_deployment=$(container_environment_value "$runner_id" SANDBOX_RUNNER_DEPLOYMENT_ID) || return 1
  runner_upstream=$(container_environment_value "$runner_id" SANDBOX_RUNNER_AIO_UPSTREAM_URL) || return 1
  [ "$runner_deployment" = "$project" ] || return 1
  [ "$runner_upstream" = http://coze-sandbox-aio:8080 ] || return 1
  runner_ip=$(container_private_ipv4 coze-sandbox-runner) || return 1
  runner_endpoint_matches_container "$SANDBOX_AIO_CORE_E2E_RUNNER_URL" "$runner_ip" || return 1
  runner_network=$(container_network "$runner_id") || return 1
  aio_network=$(container_network "$aio_id") || return 1
  [ "$runner_network" = "$aio_network" ] || return 1
  aio_aliases=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{range .Aliases}}{{println .}}{{end}}{{end}}' "$aio_id" 2>/dev/null) || return 1
  printf '%s\n' "$aio_aliases" | grep -Fqx coze-sandbox-aio || return 1

  runner_image_id=$(docker inspect --format '{{.Image}}' "$runner_id" 2>/dev/null) || return 1
  runner_image_revision=$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$runner_image_id" 2>/dev/null) || return 1
  runner_revision_matches_sha "$runner_image_id" "$runner_image_revision" "$SANDBOX_AIO_CORE_E2E_CODE_SHA" || return 1
  HARNESS_RUNNER_CONTAINER_ID=$runner_id
  HARNESS_RUNNER_IMAGE_ID=$runner_image_id
  HARNESS_RUNNER_IMAGE_REVISION=$runner_image_revision
}

validate_required_environment() {
  local name current_sha

  require_exact_gate SANDBOX_AIO_CORE_E2E 1
  require_exact_gate SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC
  require_exact_gate SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE
  require_exact_gate SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES
  require_exact_gate SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE
  require_fixture_identity_contract

  for name in \
    SANDBOX_AIO_CORE_E2E_RUNNER_URL \
    SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE \
    SANDBOX_AIO_CORE_E2E_CODE_SHA \
    SANDBOX_RUNNER_AUTH_TOKEN \
    SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON \
    SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN \
    SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE \
    MYSQL_DSN \
    REDIS_ADDR \
    REDIS_DB \
    ACR_REGISTRY \
    ACR_NAMESPACE \
    SANDBOX_RUNNER_EXECUTION_IMAGE \
    SANDBOX_RUNNER_ROOTLESS_SOCKET \
    SANDBOX_RUNNER_ROOTLESS_SOCKET_GID \
    SANDBOX_RUNNER_TLS_CERT_FILE \
    SANDBOX_RUNNER_TLS_KEY_FILE; do
    require_nonempty_variable "$name"
  done
  REDIS_PASSWORD=${REDIS_PASSWORD:-}
  export REDIS_PASSWORD

  [ "${SANDBOX_RUNNER_SESSION_ENABLED:-}" = true ] || blocked 'isolated Runner Session backend is not explicitly enabled'
  [[ "$SANDBOX_AIO_CORE_E2E_RUNNER_URL" =~ ^https://[^/@:?[:space:]]+:[1-9][0-9]{0,4}$ ]] ||
    fail 'isolated Runner URL must be an exact HTTPS origin'
  [ "${SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE#/}" != "$SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE" ] &&
    [ -f "$SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE" ] && [ -r "$SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE" ] ||
    blocked 'isolated Runner CA file is unavailable'
  [ -S "$SANDBOX_RUNNER_ROOTLESS_SOCKET" ] || blocked 'dedicated rootless runtime socket is unavailable'
  [ -f "$SANDBOX_RUNNER_TLS_CERT_FILE" ] && [ -f "$SANDBOX_RUNNER_TLS_KEY_FILE" ] ||
    blocked 'isolated Runner TLS files are unavailable'
  [[ "$REDIS_DB" =~ ^[0-9]+$ ]] || fail 'Redis database index is invalid'
  [[ "$SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE" =~ ^[A-Za-z0-9_-]+$ ]] || fail 'dev database name is invalid'

  current_sha=$(resolve_code_revision 2>/dev/null) || fail 'code revision cannot be resolved'
  [[ "$current_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'code revision is invalid'
  [ "$SANDBOX_AIO_CORE_E2E_CODE_SHA" = "$current_sha" ] || fail 'configured code revision does not match HEAD'
}

initialize_deploy_functions() {
  if [ "$HARNESS_DEPLOY_INITIALIZED" = true ]; then
    return 0
  fi
  if variable_is_set SANDBOX_AIO_IMAGE_REF && [ -n "${SANDBOX_AIO_IMAGE_REF:-}" ] &&
    [ "$SANDBOX_AIO_IMAGE_REF" != "$HARNESS_OFFICIAL_AIO_REF" ]; then
    return 1
  fi
  DEPLOY_ROOT_DIR=$HARNESS_DEPLOY_DIR
  DEPLOY_ENV_FILE=${SANDBOX_AIO_CORE_E2E_INTERNAL_ENV_FILE:-${SANDBOX_AIO_CORE_E2E_ENV_FILE:-}}
  COMPOSE_FILE=$HARNESS_COMPOSE_FILE
  DEPLOY_PROFILE=runner-2c4g
  SANDBOX_RUNNER_ENV_FILE=$DEPLOY_ENV_FILE
  COMPOSE_PROJECT_NAME=$SANDBOX_RUNNER_DEPLOYMENT_ID
  export DEPLOY_ROOT_DIR DEPLOY_ENV_FILE COMPOSE_FILE DEPLOY_PROFILE SANDBOX_RUNNER_ENV_FILE COMPOSE_PROJECT_NAME
  # shellcheck source=../../dev/deploy.sh
  source "$HARNESS_DEPLOY_HELPER"
  lock_official_aio_image_ref
  HARNESS_DEPLOY_INITIALIZED=true
}

wait_for_service_healthy() {
  local service=$1 timeout=${DEPLOY_HEALTH_TIMEOUT_SECONDS:-120} deadline

  [[ "$timeout" =~ ^[1-9][0-9]*$ ]] || return 1
  deadline=$((SECONDS + timeout))
  while ((SECONDS < deadline)); do
    service_is_healthy "$service" && return 0
    sleep 2
  done
  return 1
}

control_verb_is_valid() {
  case "$1" in
    stop-aio|start-aio|recreate-aio-preserve-volumes|restart-runner) return 0 ;;
    *) return 1 ;;
  esac
}

control_main() {
  local verb=$1

  control_verb_is_valid "$verb" || return 1
  [ "${SANDBOX_AIO_CORE_E2E_HELPER_ARMED:-}" = armed-v1 ] || return 1
  [ "${SANDBOX_AIO_CORE_E2E_CONTROL_HELPER:-}" = "$HARNESS_SCRIPT_PATH" ] || return 1
  [ "${SANDBOX_AIO_CORE_E2E_INTERNAL_COMPOSE_FILE:-}" = "$HARNESS_COMPOSE_FILE" ] || return 1
  [ "${SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT:-}" = ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC ] || return 1
  [ "${SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY:-}" = ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE ] || return 1
  [ "${SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP:-}" = EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES ] || return 1
  [ "${SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES:-}" = EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE ] || return 1
  initialize_deploy_functions >/dev/null 2>&1 || return 1

  case "$verb" in
    stop-aio)
      compose_cmd stop --timeout 30 coze-sandbox-aio >/dev/null 2>&1
      ;;
    start-aio)
      compose_cmd up -d --no-build --no-deps --pull never coze-sandbox-aio >/dev/null 2>&1 &&
        wait_for_aio_raw_health >/dev/null 2>&1
      ;;
    recreate-aio-preserve-volumes)
      compose_cmd rm --stop --force coze-sandbox-aio >/dev/null 2>&1 &&
        compose_cmd up -d --no-build --no-deps --pull never coze-sandbox-aio >/dev/null 2>&1 &&
        wait_for_aio_raw_health >/dev/null 2>&1
      ;;
    restart-runner)
      compose_cmd restart coze-sandbox-runner >/dev/null 2>&1 &&
        wait_for_service_healthy coze-sandbox-runner >/dev/null 2>&1
      ;;
  esac
}

write_evidence() {
  local status=$1 file=${SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR:-}/summary.env

  [ -n "${SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR:-}" ] || return 0
  {
    printf 'SCHEMA=aio-core-e2e-evidence-v1\n'
    printf 'STATUS=%s\n' "$status"
    printf 'CODE_SHA=%s\n' "${SANDBOX_AIO_CORE_E2E_CODE_SHA:-unavailable}"
    printf 'OFFICIAL_IMAGE_REF=%s\n' "$HARNESS_OFFICIAL_AIO_REF"
    printf 'OFFICIAL_IMAGE_ID=%s\n' "${SANDBOX_AIO_CORE_E2E_IMAGE_ID:-unavailable}"
    printf 'RUNNER_IMAGE_ID=%s\n' "${HARNESS_RUNNER_IMAGE_ID:-unavailable}"
    printf 'RUNNER_IMAGE_REVISION=%s\n' "${HARNESS_RUNNER_IMAGE_REVISION:-unavailable}"
    printf 'RECORDED_AT_UTC=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } > "$file"
  chmod 600 "$file"
}

cleanup_harness() {
  local original_status=$?

  trap - EXIT INT TERM
  if [ "$HARNESS_ARMED" = true ]; then
    control_main start-aio >/dev/null 2>&1 || true
  fi
  write_evidence "$HARNESS_RESULT" >/dev/null 2>&1 || true
  return "$original_status"
}

resolve_backend_module() {
  (
    cd "$HARNESS_REPO_ROOT/backend"
    env -u GOMOD -u GOPATH -u GOROOT \
      GO111MODULE=on GOENV=off GOFLAGS= GOTOOLCHAIN=local GOWORK=off \
      GOCACHE=/private/tmp/coze-go-build go env GOMOD
  )
}

scan_go_e2e_events() {
  awk -v test_name="$HARNESS_GO_TEST_NAME" '
    BEGIN {
      test_field = "\"Test\":\"" test_name "\""
      skip_field = "\"Action\":\"skip\""
      pass_field = "\"Action\":\"pass\""
    }
    index($0, test_field) && index($0, skip_field) { skipped=1 }
    index($0, test_field) && index($0, pass_field) { passed=1 }
    END {
      if (skipped) exit 78
      if (!passed) exit 1
    }
  '
}

run_go_e2e() {
  local go_status scan_status resolved_module
  local -a pipeline_statuses

  resolved_module=$(resolve_backend_module 2>/dev/null) || fail 'backend Go module cannot be resolved with fixed execution controls'
  [ "$resolved_module" = "$HARNESS_REPO_ROOT/backend/go.mod" ] || fail 'backend Go module resolution escaped the repository module'

  set +e
  (
    cd "$HARNESS_REPO_ROOT/backend"
    env -u GOMOD -u GOPATH -u GOROOT \
      GO111MODULE=on GOENV=off GOFLAGS= GOTOOLCHAIN=local GOWORK=off \
      GOCACHE=/private/tmp/coze-go-build \
      go test -json ./internal/sandboxrunner -run "$HARNESS_GO_TEST_REGEX" -count=1 -timeout=20m 2>/dev/null
  ) | scan_go_e2e_events
  pipeline_statuses=("${PIPESTATUS[@]}")
  go_status=${pipeline_statuses[0]}
  scan_status=${pipeline_statuses[1]}
  set -e

  [ "$go_status" -eq 0 ] || fail 'focused backend Core E2E failed; sensitive test output was suppressed'
  case "$scan_status" in
    0) ;;
    "$HARNESS_BLOCKED_EXIT") blocked 'focused backend Core E2E reported an unavailable real dependency' ;;
    *) fail 'focused backend Core E2E did not produce a passing test event' ;;
  esac
}

self_test() {
  local status unsigned_runner_probe duplicate_env control_key resolved_module resolved_revision expected_revision

  for literal in \
    "$HARNESS_OFFICIAL_AIO_REF" \
    SANDBOX_AIO_CORE_E2E_CONTROL_HELPER \
    SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT \
    ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC \
    SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP \
    EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES \
    SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES \
    EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE \
    SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY \
    ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE \
    recreate-aio-preserve-volumes \
    restart-runner \
    GOWORK=off \
    GOENV=off \
    GOTOOLCHAIN=local \
    'isolated exact-PK cleanup gates'; do
    grep -F -- "$literal" "$HARNESS_SCRIPT_PATH" >/dev/null || return 1
  done
  unsigned_runner_probe='$SANDBOX_AIO_CORE_E2E_RUNNER_URL''/v1/health'
  ! grep -F -- "$unsigned_runner_probe" "$HARNESS_SCRIPT_PATH" >/dev/null

  control_verb_is_valid stop-aio
  control_verb_is_valid start-aio
  control_verb_is_valid recreate-aio-preserve-volumes
  control_verb_is_valid restart-runner
  ! control_verb_is_valid delete-volume

  unset SELF_TEST_GATE
  set +e
  validate_exact_gate SELF_TEST_GATE expected
  status=$?
  set -e
  [ "$status" -eq "$HARNESS_BLOCKED_EXIT" ] || return 1
  SELF_TEST_GATE=wrong
  ! validate_exact_gate SELF_TEST_GATE expected
  SELF_TEST_GATE=expected
  validate_exact_gate SELF_TEST_GATE expected

  SANDBOX_AIO_CORE_E2E_PREFIX=aio-e2e-contract-001
  SANDBOX_RUNNER_DEPLOYMENT_ID=$SANDBOX_AIO_CORE_E2E_PREFIX
  SANDBOX_AIO_CORE_E2E_SPACE_ID=101
  SANDBOX_AIO_CORE_E2E_USER_ID_A=201
  SANDBOX_AIO_CORE_E2E_USER_ID_B=202
  validate_fixture_identity_contract
  SANDBOX_AIO_CORE_E2E_USER_ID_B=$SANDBOX_AIO_CORE_E2E_USER_ID_A
  ! validate_fixture_identity_contract

  duplicate_env=$(mktemp /private/tmp/aio-core-e2e-duplicate-env.XXXXXX)
  printf 'SANDBOX_AIO_CORE_E2E=1\nSANDBOX_AIO_CORE_E2E=1\n' > "$duplicate_env"
  set +e
  (load_controlled_environment "$duplicate_env") >/dev/null 2>&1
  status=$?
  set -e
  rm -f -- "$duplicate_env"
  [ "$status" -ne 0 ] || return 1

  for control_key in GIT_DIR GIT_WORK_TREE GOWORK GOMOD GOENV GOTOOLCHAIN GOFLAGS GOPATH GOROOT GOEXPERIMENT; do
    duplicate_env=$(mktemp /private/tmp/aio-core-e2e-control-env.XXXXXX)
    printf '%s=forbidden\n' "$control_key" > "$duplicate_env"
    set +e
    (load_controlled_environment "$duplicate_env") >/dev/null 2>&1
    status=$?
    set -e
    rm -f -- "$duplicate_env"
    [ "$status" -ne 0 ] || return 1
  done

  set +e
  resolved_module=$(GOWORK=/private/tmp/forbidden.work GOENV=/private/tmp/forbidden.goenv \
    GOTOOLCHAIN=auto resolve_backend_module 2>/dev/null)
  status=$?
  set -e
  [ "$status" -eq 0 ] || return 1
  [ "$resolved_module" = "$HARNESS_REPO_ROOT/backend/go.mod" ] || return 1

  expected_revision=$(env -u GIT_DIR -u GIT_WORK_TREE git -C "$HARNESS_REPO_ROOT" rev-parse HEAD 2>/dev/null) || return 1
  set +e
  resolved_revision=$(GIT_DIR=/private/tmp/forbidden.git GIT_WORK_TREE=/private/tmp/forbidden-worktree \
    resolve_code_revision 2>/dev/null)
  status=$?
  set -e
  [ "$status" -eq 0 ] || return 1
  [ "$resolved_revision" = "$expected_revision" ] || return 1

  worktree_snapshot_is_clean /fixture/repo /fixture/repo "$expected_revision" "$expected_revision" ''
  ! worktree_snapshot_is_clean /fixture/repo /fixture/repo "$expected_revision" "$expected_revision" ' M tracked.txt'
  ! worktree_snapshot_is_clean /fixture/repo /fixture/repo "$expected_revision" "$expected_revision" '?? untracked.txt'
  ! worktree_snapshot_is_clean /fixture/repo /other "$expected_revision" "$expected_revision" ''
  runner_revision_matches_sha 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' "$expected_revision" "$expected_revision"
  ! runner_revision_matches_sha 'sha256:short' "$expected_revision" "$expected_revision"
  ! runner_revision_matches_sha 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' "${expected_revision%?}0" "$expected_revision"
  runner_endpoint_matches_container 'https://172.31.0.9:9443' '172.31.0.9'
  ! runner_endpoint_matches_container 'https://172.31.0.8:9443' '172.31.0.9'

  set +e
  printf '%s\n' \
    '{"Test":"TestAIOCoreE2E/accepted-limitation","Action":"skip"}' \
    '{"Test":"TestAIOCoreE2E","Action":"pass"}' | scan_go_e2e_events >/dev/null
  status=$?
  set -e
  [ "$status" -eq 0 ] || return 1

  set +e
  printf '%s\n' \
    '{"Test":"TestAIOCoreE2E/accepted-limitation","Action":"pass"}' \
    '{"Test":"TestAIOCoreE2E","Action":"skip"}' | scan_go_e2e_events >/dev/null
  status=$?
  set -e
  [ "$status" -eq "$HARNESS_BLOCKED_EXIT" ] || return 1

  printf 'AIO Core E2E harness self-test passed.\n'
}

main() {
  local controlled_env image_id running_image_id aio_compose_block compose_services
  local initial_runner_container_id initial_runner_image_id initial_runner_image_revision

  if [ "$#" -eq 1 ] && control_verb_is_valid "$1"; then
    control_main "$1"
    return
  fi
  if [ "$#" -eq 1 ] && [ "$1" = --self-test ]; then
    self_test
    return
  fi
  [ "$#" -eq 0 ] || fail 'expected no arguments or --self-test'

  require_command awk
  require_command curl
  require_command docker
  require_command git
  require_command go
  require_command stat
  docker compose version >/dev/null 2>&1 || blocked 'Docker Compose v2 is unavailable'

  controlled_env=${SANDBOX_AIO_CORE_E2E_ENV_FILE:-}
  [ -n "$controlled_env" ] || blocked 'SANDBOX_AIO_CORE_E2E_ENV_FILE is required'
  validate_private_environment_file "$controlled_env"
  load_controlled_environment "$controlled_env"
  SANDBOX_AIO_CORE_E2E_ENV_FILE=$controlled_env
  SANDBOX_AIO_CORE_E2E_INTERNAL_ENV_FILE=$controlled_env
  SANDBOX_AIO_CORE_E2E_INTERNAL_COMPOSE_FILE=$HARNESS_COMPOSE_FILE
  SANDBOX_AIO_CORE_E2E_CONTROL_HELPER=$HARNESS_SCRIPT_PATH
  export SANDBOX_AIO_CORE_E2E_ENV_FILE SANDBOX_AIO_CORE_E2E_INTERNAL_ENV_FILE
  export SANDBOX_AIO_CORE_E2E_INTERNAL_COMPOSE_FILE SANDBOX_AIO_CORE_E2E_CONTROL_HELPER

  validate_required_environment
  validate_clean_worktree_snapshot
  [ -f "$HARNESS_COMPOSE_FILE" ] && [ -f "$HARNESS_DEPLOY_HELPER" ] || fail 'deploy-owned AIO helper files are missing'
  grep -Fq 'image: ghcr.io/agent-infra/sandbox:latest' "$HARNESS_COMPOSE_FILE" || fail 'Compose does not use the official latest AIO image'
  grep -Fq 'source: sandbox-aio-user-data' "$HARNESS_COMPOSE_FILE" || fail 'Compose does not own the persistent AIO user-data volume'
  grep -Fq 'target: /mnt/user-data' "$HARNESS_COMPOSE_FILE" || fail 'Compose AIO workspace mount is invalid'
  aio_compose_block=$(awk '
    $0 == "  coze-sandbox-aio:" { inside=1 }
    inside && /^  [^[:space:]]/ && $0 != "  coze-sandbox-aio:" { exit }
    inside && /^[^[:space:]]/ { exit }
    inside { print }
  ' "$HARNESS_COMPOSE_FILE")
  printf '%s\n' "$aio_compose_block" | grep -Eq '^    ports:' && fail 'AIO service must not publish a host port'
  printf '%s\n' "$aio_compose_block" | grep -Fq '      - "8080"' || fail 'AIO service must expose only its private port'
  compose_services=$(awk '
    /^services:/ { inside=1; next }
    inside && /^[^[:space:]]/ { exit }
    inside && /^  [^[:space:]]/ { value=$0; sub(/^  /, "", value); sub(/:$/, "", value); print value }
  ' "$HARNESS_COMPOSE_FILE")
  if printf '%s\n' "$compose_services" | grep -Eq '^(mysql|mariadb|oceanbase|postgres|postgresql)$'; then
    fail 'runner profile must not define a local database service'
  fi

  SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR=$(mktemp -d /private/tmp/newx-aio-core-e2e.XXXXXX) || fail 'external evidence directory cannot be created'
  chmod 700 "$SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR"
  export SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR
  trap cleanup_harness EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  initialize_deploy_functions >/dev/null 2>&1 || fail 'official AIO deploy helper initialization failed'
  compose_cmd config --quiet >/dev/null 2>&1 || fail 'isolated deployment Compose configuration is invalid'
  preflight_additive_migration >/dev/null 2>&1 || blocked 'required additive migration is not applied in the dev database'
  prepare_official_aio >/dev/null 2>&1 || fail 'official latest AIO failed its deploy-owned startup health gate'

  image_id=$SANDBOX_AIO_IMAGE_ID
  running_image_id=$(container_image_id coze-sandbox-aio 2>/dev/null) || fail 'running AIO image evidence cannot be read'
  [[ "$image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'official AIO image evidence is invalid'
  [ "$running_image_id" = "$image_id" ] || fail 'running AIO does not match the freshly pulled official image'
  SANDBOX_AIO_CORE_E2E_IMAGE_ID=$image_id
  SANDBOX_AIO_CORE_E2E_HELPER_ARMED=armed-v1
  export SANDBOX_AIO_CORE_E2E_IMAGE_ID SANDBOX_AIO_CORE_E2E_HELPER_ARMED
  HARNESS_ARMED=true

  wait_for_service_healthy coze-sandbox-runner >/dev/null 2>&1 ||
    blocked 'isolated Runner container is not already healthy'
  validate_compose_target_binding ||
    blocked 'selected Runner endpoint, deployment, revision, or AIO upstream is not bound to the isolated Compose project'
  initial_runner_container_id=$HARNESS_RUNNER_CONTAINER_ID
  initial_runner_image_id=$HARNESS_RUNNER_IMAGE_ID
  initial_runner_image_revision=$HARNESS_RUNNER_IMAGE_REVISION

  run_go_e2e
  wait_for_service_healthy coze-sandbox-runner >/dev/null 2>&1 ||
    fail 'isolated Runner was not healthy after the Core E2E'
  validate_compose_target_binding || fail 'selected Runner target binding changed during the Core E2E'
  [ "$HARNESS_RUNNER_CONTAINER_ID" = "$initial_runner_container_id" ] &&
    [ "$HARNESS_RUNNER_IMAGE_ID" = "$initial_runner_image_id" ] &&
    [ "$HARNESS_RUNNER_IMAGE_REVISION" = "$initial_runner_image_revision" ] ||
    fail 'selected Runner container or image changed during the Core E2E'
  validate_clean_worktree_snapshot
  HARNESS_RESULT=passed
  write_evidence "$HARNESS_RESULT"
  printf 'AIO Core E2E passed with isolated exact-PK cleanup gates.\n'
}

main "$@"
