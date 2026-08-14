#!/usr/bin/env bash
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

readonly blocked_exit=75
readonly official_aio_ref='ghcr.io/agent-infra/sandbox:latest'
readonly runner_limit_bytes=201326592
readonly expected_host_memory_bytes=4294967296
readonly host_memory_reserve_bytes=1610612736
readonly max_aio_rss_bytes=2147483648
readonly max_aio_pids=256
readonly max_runner_pids=64
readonly max_aio_fds=4096
readonly max_runner_fds=1024
readonly max_container_cpu_percent=200
readonly drain_aio_rss_delta_bytes=268435456
readonly drain_runner_rss_delta_bytes=33554432
readonly drain_aio_pid_delta=8
readonly drain_runner_pid_delta=4
readonly drain_aio_fd_delta=128
readonly drain_runner_fd_delta=64
readonly baseline_sample_count=3
readonly drain_sample_count=4
readonly sample_interval_seconds=5
readonly isolated_deployment_declaration='ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC'
readonly workload_test_regex='^TestAIOCoreE2ESoakWorkload$'
readonly snapshot_test_regex='^TestAIOCoreE2ESoakSnapshot$'
readonly cleanup_test_regex='^TestAIOCoreE2ECleanupOnly$'
readonly snapshot_prefix='AIO_CORE_E2E_SNAPSHOT'

duration=''
self_test=false
repo_root=''
evidence_file=''
workload_pid=''
result='FAILED'
residual_state='NOT_VERIFIED'
observed_generation=''
signed_requests_closed=false
sample_phase='uninitialized'
controlled_env=''
compose_file=''
aio_id=''
runner_id=''
server_id=''
runner_image_id=''
runner_image_revision=''

usage() {
  printf 'usage: %s --duration 2m|30m\n       %s --self-test\n' "$0" "$0" >&2
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

blocked() {
  printf 'BLOCKED: %s\n' "$1" >&2
  exit "$blocked_exit"
}

parse_duration() {
  case "$1" in
    2m) printf '120\n' ;;
    30m) printf '1800\n' ;;
    *) return 1 ;;
  esac
}

is_non_negative_integer() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

validate_snapshot() {
  local snapshot=$1 expected field number

  set -- $snapshot
  [ "$#" -eq 7 ] || return 1
  [ "$1" = "$snapshot_prefix" ] || return 1
  shift
  for expected in queue running weight sessions shells generation; do
    field=$1
    [ "${field%%=*}" = "$expected" ] || return 1
    number=${field#*=}
    is_non_negative_integer "$number" || return 1
    shift
  done
  return 0
}

snapshot_value() {
  local snapshot=$1 wanted=$2 field

  for field in $snapshot; do
    case "$field" in
      "$wanted"=*) printf '%s\n' "${field#*=}"; return 0 ;;
    esac
  done
  return 1
}

snapshot_is_drained() {
  local snapshot=$1

  [ "$(snapshot_value "$snapshot" queue)" -eq 0 ] &&
    [ "$(snapshot_value "$snapshot" running)" -eq 0 ] &&
    [ "$(snapshot_value "$snapshot" weight)" -eq 0 ] &&
    [ "$(snapshot_value "$snapshot" sessions)" -eq 0 ] &&
    [ "$(snapshot_value "$snapshot" shells)" -eq 0 ]
}

extract_snapshot() {
  local output=$1 line snapshot='' marker_count=0

  while IFS= read -r line; do
    case "$line" in
      "$snapshot_prefix "*)
        snapshot=$line
        marker_count=$((marker_count + 1))
        ;;
    esac
  done <<<"$output"
  [ "$marker_count" -eq 1 ] || return 1
  validate_snapshot "$snapshot" || return 1
  printf '%s\n' "$snapshot"
}

worktree_snapshot_is_clean() {
  local expected_root=$1 actual_root=$2 expected_sha=$3 actual_sha=$4 porcelain=$5

  [ "$actual_root" = "$expected_root" ] &&
    [[ "$actual_sha" =~ ^[0-9a-f]{40}$ ]] &&
    [ "$actual_sha" = "$expected_sha" ] &&
    [ -z "$porcelain" ]
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

resource_sample_within_limits() {
  local aio_rss=$1 runner_rss=$2 aio_pid=$3 runner_pid=$4 aio_fd=$5 runner_fd=$6 host_available=$7

  [ "$aio_rss" -le "$max_aio_rss_bytes" ] &&
    [ "$runner_rss" -le "$runner_limit_bytes" ] &&
    [ "$aio_pid" -le "$max_aio_pids" ] &&
    [ "$runner_pid" -le "$max_runner_pids" ] &&
    [ "$aio_fd" -le "$max_aio_fds" ] &&
    [ "$runner_fd" -le "$max_runner_fds" ] &&
    [ "$host_available" -ge "$host_memory_reserve_bytes" ] &&
    [ "$host_available" -le "$expected_host_memory_bytes" ]
}

drain_sample_within_baseline() {
  local baseline_aio_rss=$1 drain_aio_rss=$2 baseline_runner_rss=$3 drain_runner_rss=$4
  local baseline_aio_pid=$5 drain_aio_pid=$6 baseline_runner_pid=$7 drain_runner_pid=$8
  local baseline_aio_fd=$9 drain_aio_fd=${10} baseline_runner_fd=${11} drain_runner_fd=${12}

  [ "$drain_aio_rss" -le $((baseline_aio_rss + drain_aio_rss_delta_bytes)) ] &&
    [ "$drain_runner_rss" -le $((baseline_runner_rss + drain_runner_rss_delta_bytes)) ] &&
    [ "$drain_aio_pid" -le $((baseline_aio_pid + drain_aio_pid_delta)) ] &&
    [ "$drain_runner_pid" -le $((baseline_runner_pid + drain_runner_pid_delta)) ] &&
    [ "$drain_aio_fd" -le $((baseline_aio_fd + drain_aio_fd_delta)) ] &&
    [ "$drain_runner_fd" -le $((baseline_runner_fd + drain_runner_fd_delta)) ]
}

signed_status_read_allowed() {
  [ "$signed_requests_closed" = false ]
}

run_self_test() {
  local sample fixture_sha

  [ "$(parse_duration 2m)" = 120 ] || fail '2m duration parsing failed'
  [ "$(parse_duration 30m)" = 1800 ] || fail '30m duration parsing failed'
  if parse_duration 1m >/dev/null 2>&1; then
    fail 'unsupported duration was accepted'
  fi
  sample='AIO_CORE_E2E_SNAPSHOT queue=0 running=2 weight=2 sessions=2 shells=2 generation=7'
  validate_snapshot "$sample" || fail 'safe aggregate snapshot was rejected'
  [ "$(extract_snapshot $'=== RUN   TestAIOCoreE2ESoakSnapshot\n'"$sample"$'\n--- PASS: TestAIOCoreE2ESoakSnapshot')" = "$sample" ] ||
    fail 'unique fixed snapshot record was not extracted'
  if extract_snapshot "$sample"$'\n'"$sample" >/dev/null; then
    fail 'duplicate fixed snapshot records were accepted'
  fi
  [ "$(snapshot_value "$sample" sessions)" = 2 ] || fail 'aggregate snapshot parsing failed'
  if validate_snapshot 'AIO_CORE_E2E_SNAPSHOT queue=0 command=1 weight=2 sessions=1 shells=1 generation=7'; then
    fail 'sensitive aggregate field was accepted'
  fi
  if validate_snapshot 'AIO_CORE_E2E_SNAPSHOT queue=0 running=0 weight=0 sessions=0 shells=0 generation=0 extra=0'; then
    fail 'snapshot with extra fields was accepted'
  fi
  if (load_controlled_environment <(printf 'SELF_TEST_DUPLICATE=one\nSELF_TEST_DUPLICATE=two\n')) >/dev/null 2>&1; then
    fail 'duplicate controlled environment keys were accepted'
  fi
  if (load_controlled_environment <(printf 'GOWORK=/tmp/redirect\n')) >/dev/null 2>&1; then
    fail 'Go workspace redirection was accepted from the controlled environment'
  fi
  if (load_controlled_environment <(printf 'GIT_DIR=/tmp/redirect\n')) >/dev/null 2>&1; then
    fail 'Git repository redirection was accepted from the controlled environment'
  fi
  fixture_sha=0123456789abcdef0123456789abcdef01234567
  worktree_snapshot_is_clean /fixture/repo /fixture/repo "$fixture_sha" "$fixture_sha" '' || fail 'clean worktree fixture was rejected'
  if worktree_snapshot_is_clean /fixture/repo /fixture/repo "$fixture_sha" "$fixture_sha" '?? untracked'; then
    fail 'dirty worktree fixture was accepted'
  fi
  runner_revision_matches_sha 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' "$fixture_sha" "$fixture_sha" ||
    fail 'matching Runner image evidence was rejected'
  if runner_endpoint_matches_container 'https://172.31.0.8:9443' '172.31.0.9'; then
    fail 'Runner URL detached from the selected Compose service was accepted'
  fi
  resource_sample_within_limits 1073741824 134217728 100 40 1000 500 2147483648 ||
    fail 'bounded resource sample was rejected'
  if resource_sample_within_limits 2147483649 134217728 100 40 1000 500 2147483648; then
    fail 'AIO RSS above the explicit maximum was accepted'
  fi
  drain_sample_within_baseline 1073741824 1073741824 134217728 134217728 100 100 40 40 1000 1000 500 500 ||
    fail 'steady drained resource fixture was rejected'
  signed_requests_closed=true
  if signed_status_read_allowed; then
    fail 'signed status reads remained available after the final snapshot'
  fi
  signed_requests_closed=false
  signed_status_read_allowed || fail 'signed status reads were unavailable before the final snapshot'
  printf 'aio 2c4g soak self-test: ok\n'
}

file_mode() {
  stat -c '%a' -- "$1" 2>/dev/null || stat -f '%Lp' -- "$1" 2>/dev/null
}

file_owner() {
  stat -c '%u' -- "$1" 2>/dev/null || stat -f '%u' -- "$1" 2>/dev/null
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
    seen_keys=${seen_keys}${key}$'\n'
    case "$key" in
      SANDBOX_SOAK_*|SANDBOX_AIO_CORE_E2E_ENV_FILE|SANDBOX_AIO_CORE_E2E_INTERNAL_*|SANDBOX_AIO_CORE_E2E_CONTROL_HELPER|SANDBOX_AIO_CORE_E2E_IMAGE_ID|SANDBOX_AIO_CORE_E2E_HELPER_ARMED|SANDBOX_AIO_CORE_E2E_EVIDENCE_DIR|SANDBOX_AIO_CORE_E2E_SOAK_DURATION)
        fail 'controlled environment file cannot override soak-owned state'
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
  done <"$file"
}

require_nonempty_variable() {
  local name=$1

  declare -p "$name" >/dev/null 2>&1 && [ -n "${!name}" ] || blocked "required controlled setting is missing: $name"
}

require_exact_gate() {
  local name=$1 expected=$2

  declare -p "$name" >/dev/null 2>&1 && [ -n "${!name}" ] || blocked "required safety acknowledgement is missing: $name"
  [ "${!name}" = "$expected" ] || fail "required safety acknowledgement is invalid: $name"
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
    git -C "$repo_root" "$@"
}

validate_clean_worktree_snapshot() {
  local actual_root actual_sha porcelain relative_script

  actual_root=$(clean_git rev-parse --show-toplevel 2>/dev/null) || fail 'dedicated soak worktree root cannot be resolved'
  actual_sha=$(clean_git rev-parse HEAD 2>/dev/null) || fail 'dedicated soak worktree HEAD cannot be resolved'
  porcelain=$(clean_git status --porcelain=v1 --untracked-files=all --ignore-submodules=none 2>/dev/null) ||
    fail 'dedicated soak worktree status cannot be resolved'
  worktree_snapshot_is_clean "$repo_root" "$actual_root" "$SANDBOX_AIO_CORE_E2E_CODE_SHA" "$actual_sha" "$porcelain" ||
    fail 'dedicated soak worktree must be completely clean against the configured HEAD'
  relative_script=${script_dir#"$repo_root"/}/$(basename -- "$0")
  [ "$relative_script" != "$script_dir/$(basename -- "$0")" ] || fail 'soak harness path escaped the dedicated worktree'
  clean_git cat-file -e "$actual_sha:$relative_script" 2>/dev/null ||
    fail 'AIO soak harness must be committed at the configured HEAD'
}

soak_compose_cmd() {
  docker compose --env-file "$controlled_env" -p "$SANDBOX_RUNNER_DEPLOYMENT_ID" -f "$compose_file" "$@"
}

container_for_service() {
  local service=$1 ids

  ids=$(soak_compose_cmd ps -q "$service" 2>/dev/null) || return 1
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

container_private_ip() {
  local container_id=$1 addresses

  addresses=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}' "$container_id" 2>/dev/null) || return 1
  set -- $addresses
  [ "$#" -eq 1 ] || return 1
  [[ "$1" =~ ^(10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.) ]] || return 1
  printf '%s\n' "$1"
}

validate_compose_target_binding() {
  local project runner_project aio_project server_project runner_service aio_service server_service
  local runner_deployment runner_upstream runner_ip runner_network aio_network server_network aio_aliases
  local current_runner_image_id current_runner_revision

  aio_id=$(container_for_service coze-sandbox-aio) || return 1
  runner_id=$(container_for_service coze-sandbox-runner) || return 1
  server_id=$(container_for_service coze-server) || return 1
  project=$SANDBOX_RUNNER_DEPLOYMENT_ID
  runner_project=$(container_label "$runner_id" com.docker.compose.project) || return 1
  aio_project=$(container_label "$aio_id" com.docker.compose.project) || return 1
  server_project=$(container_label "$server_id" com.docker.compose.project) || return 1
  [ "$runner_project" = "$project" ] && [ "$aio_project" = "$project" ] && [ "$server_project" = "$project" ] || return 1
  runner_service=$(container_label "$runner_id" com.docker.compose.service) || return 1
  aio_service=$(container_label "$aio_id" com.docker.compose.service) || return 1
  server_service=$(container_label "$server_id" com.docker.compose.service) || return 1
  [ "$runner_service" = coze-sandbox-runner ] && [ "$aio_service" = coze-sandbox-aio ] && [ "$server_service" = coze-server ] || return 1

  runner_deployment=$(container_environment_value "$runner_id" SANDBOX_RUNNER_DEPLOYMENT_ID) || return 1
  runner_upstream=$(container_environment_value "$runner_id" SANDBOX_RUNNER_AIO_UPSTREAM_URL) || return 1
  [ "$runner_deployment" = "$project" ] || return 1
  [ "$runner_upstream" = http://coze-sandbox-aio:8080 ] || return 1
  runner_ip=$(container_private_ip "$runner_id") || return 1
  runner_endpoint_matches_container "$SANDBOX_AIO_CORE_E2E_RUNNER_URL" "$runner_ip" || return 1
  runner_network=$(container_network "$runner_id") || return 1
  aio_network=$(container_network "$aio_id") || return 1
  server_network=$(container_network "$server_id") || return 1
  [ "$runner_network" = "$aio_network" ] && [ "$runner_network" = "$server_network" ] || return 1
  aio_aliases=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{range .Aliases}}{{println .}}{{end}}{{end}}' "$aio_id" 2>/dev/null) || return 1
  printf '%s\n' "$aio_aliases" | grep -Fqx coze-sandbox-aio || return 1

  current_runner_image_id=$(docker inspect --format '{{.Image}}' "$runner_id" 2>/dev/null) || return 1
  current_runner_revision=$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$current_runner_image_id" 2>/dev/null) || return 1
  runner_revision_matches_sha "$current_runner_image_id" "$current_runner_revision" "$code_sha" || return 1
  runner_image_id=$current_runner_image_id
  runner_image_revision=$current_runner_revision
}

cleanup() {
  local grace_deadline

  if [ -n "$workload_pid" ] && kill -0 "$workload_pid" 2>/dev/null; then
    result='FAILED'
    residual_state='BLOCKED_EXACT_PREFIX_CLEANUP_REQUIRED'
    printf 'BLOCKED: workload was interrupted; exact-prefix fixture cleanup must be verified before deployment reuse\n' >&2
    kill -INT "$workload_pid" 2>/dev/null || true
    grace_deadline=$((SECONDS + 30))
    while kill -0 "$workload_pid" 2>/dev/null && [ "$SECONDS" -lt "$grace_deadline" ]; do
      sleep 1
    done
    if kill -0 "$workload_pid" 2>/dev/null; then
      kill -TERM "$workload_pid" 2>/dev/null || true
      grace_deadline=$((SECONDS + 10))
      while kill -0 "$workload_pid" 2>/dev/null && [ "$SECONDS" -lt "$grace_deadline" ]; do
        sleep 1
      done
    fi
    if kill -0 "$workload_pid" 2>/dev/null; then
      kill -KILL "$workload_pid" 2>/dev/null || true
    fi
    wait "$workload_pid" 2>/dev/null || true
  fi
  if [ -n "$evidence_file" ] && [ -f "$evidence_file" ]; then
    printf 'result\t%s\n' "$result" >>"$evidence_file" 2>/dev/null || true
    printf 'residual_state\t%s\n' "$residual_state" >>"$evidence_file" 2>/dev/null || true
  fi
  return 0
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

while [ "$#" -gt 0 ]; do
  case "$1" in
    --self-test)
      [ "$#" -eq 1 ] || { usage; exit 2; }
      self_test=true
      shift
      ;;
    --duration)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      [ -z "$duration" ] || { usage; exit 2; }
      duration=$2
      shift 2
      ;;
    *) usage; exit 2 ;;
  esac
done

if [ "$self_test" = true ]; then
  [ -z "$duration" ] || { usage; exit 2; }
  run_self_test
  exit 0
fi

[ -n "$duration" ] || { usage; exit 2; }
duration_seconds=$(parse_duration "$duration") || { usage; exit 2; }

for command_name in docker env git go awk grep sed date sleep mktemp stat id; do
  command -v "$command_name" >/dev/null 2>&1 || blocked "required command is unavailable: $command_name"
done

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
repo_root=$(CDPATH= cd -- "$script_dir/../../.." && pwd -P)
compose_file=$repo_root/deploy/dev/docker-compose.runner-2c4g.yml
core_e2e_test=$script_dir/aio_core_e2e_test.sh
[ -x "$core_e2e_test" ] || blocked 'repository-owned AIO Core E2E harness is unavailable'
[ -f "$compose_file" ] || blocked 'repository-owned runner-2c4g Compose file is unavailable'

[ "${SANDBOX_SOAK_ENV:-}" = dev ] || blocked 'SANDBOX_SOAK_ENV must be exactly dev'
[ -n "${SANDBOX_SOAK_EVIDENCE_DIR:-}" ] || blocked 'SANDBOX_SOAK_EVIDENCE_DIR is required'
case "$SANDBOX_SOAK_EVIDENCE_DIR" in
  /*) ;;
  *) blocked 'evidence directory must be absolute' ;;
esac
[ -d "$SANDBOX_SOAK_EVIDENCE_DIR" ] && [ ! -L "$SANDBOX_SOAK_EVIDENCE_DIR" ] || blocked 'evidence directory does not exist or is a symlink'
evidence_root=$(CDPATH= cd -- "$SANDBOX_SOAK_EVIDENCE_DIR" && pwd -P)
case "$evidence_root/" in
  "$repo_root/"*) blocked 'evidence directory must be outside the repository' ;;
esac
[ -w "$evidence_root" ] || blocked 'evidence directory is not writable'
[ "$(file_mode "$evidence_root")" = 700 ] || fail 'evidence directory must have mode 0700'
[ "$(file_owner "$evidence_root")" = "$(id -u)" ] || fail 'evidence directory must be owned by the current user'

controlled_env=${SANDBOX_AIO_CORE_E2E_ENV_FILE:-}
case "$controlled_env" in
  /*) ;;
  *) blocked 'SANDBOX_AIO_CORE_E2E_ENV_FILE must use an absolute path' ;;
esac
[ -f "$controlled_env" ] && [ ! -L "$controlled_env" ] || blocked 'controlled Core E2E environment file is unavailable'
[ "$(file_mode "$controlled_env")" = 600 ] || fail 'controlled Core E2E environment file must have mode 0600'
[ "$(file_owner "$controlled_env")" = "$(id -u)" ] || fail 'controlled Core E2E environment file must be owned by the current user'
load_controlled_environment "$controlled_env"

for tool_redirect_name in ${!GIT_@}; do
  unset "$tool_redirect_name"
done
unset GOENV GOFLAGS GOMOD GOTOOLCHAIN GOWORK

require_exact_gate SANDBOX_AIO_CORE_E2E 1
require_exact_gate SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC
require_exact_gate SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES
require_exact_gate SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE
require_exact_gate SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE
[ "${SANDBOX_RUNNER_SESSION_ENABLED:-}" = true ] || blocked 'isolated Runner Session backend is not explicitly enabled'

for required_name in \
  SANDBOX_AIO_CORE_E2E_PREFIX \
  SANDBOX_RUNNER_DEPLOYMENT_ID \
  SANDBOX_AIO_CORE_E2E_RUNNER_URL \
  SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE \
  SANDBOX_AIO_CORE_E2E_CODE_SHA \
  SANDBOX_AIO_CORE_E2E_SPACE_ID \
  SANDBOX_AIO_CORE_E2E_USER_ID_A \
  SANDBOX_AIO_CORE_E2E_USER_ID_B \
  SANDBOX_RUNNER_AUTH_TOKEN \
  SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON \
  SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN \
  SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE \
  MYSQL_DSN \
  REDIS_ADDR \
  REDIS_DB; do
  require_nonempty_variable "$required_name"
done
[ "$SANDBOX_AIO_CORE_E2E_PREFIX" = "$SANDBOX_RUNNER_DEPLOYMENT_ID" ] ||
  fail 'isolated fixture prefix must match the selected Runner deployment'
REDIS_PASSWORD=${REDIS_PASSWORD:-}
export REDIS_PASSWORD

SANDBOX_AIO_CORE_E2E_ENV_FILE=$controlled_env
SANDBOX_AIO_CORE_E2E_CONTROL_HELPER=$core_e2e_test
SANDBOX_AIO_CORE_E2E_INTERNAL_ENV_FILE=$controlled_env
SANDBOX_AIO_CORE_E2E_INTERNAL_COMPOSE_FILE=$compose_file
SANDBOX_RUNNER_ENV_FILE=$controlled_env
COMPOSE_PROJECT_NAME=$SANDBOX_RUNNER_DEPLOYMENT_ID
export SANDBOX_AIO_CORE_E2E_ENV_FILE SANDBOX_AIO_CORE_E2E_CONTROL_HELPER
export SANDBOX_AIO_CORE_E2E_INTERNAL_ENV_FILE SANDBOX_AIO_CORE_E2E_INTERNAL_COMPOSE_FILE
export SANDBOX_RUNNER_ENV_FILE COMPOSE_PROJECT_NAME

code_sha=$(clean_git rev-parse HEAD 2>/dev/null) || blocked 'exact code SHA is unavailable'
[ "${#code_sha}" -eq 40 ] || blocked 'exact code SHA is invalid'
case "$code_sha" in
  *[!0-9a-f]*) blocked 'exact code SHA is invalid' ;;
esac
[ "$SANDBOX_AIO_CORE_E2E_CODE_SHA" = "$code_sha" ] || fail 'controlled code SHA does not match HEAD'
validate_clean_worktree_snapshot

docker_info=$(docker info --format '{{.NCPU}} {{.MemTotal}}' 2>/dev/null) || blocked 'Docker runtime is unavailable'
set -- $docker_info
[ "$#" -eq 2 ] || blocked 'Docker host resource projection is unavailable'
[ "$1" = 2 ] || blocked 'Docker host must provide exactly 2 CPUs'
[ "$2" = "$expected_host_memory_bytes" ] || blocked 'Docker host must provide exactly 4 GiB memory'

soak_compose_cmd config --quiet >/dev/null 2>&1 || blocked 'isolated runner-2c4g Compose configuration is invalid'
validate_compose_target_binding ||
  blocked 'selected Runner endpoint, deployment, revision, AIO upstream, or server is not bound to the isolated Compose project'
initial_aio_id=$aio_id
initial_runner_id=$runner_id
initial_server_id=$server_id
initial_runner_image_id=$runner_image_id
initial_runner_image_revision=$runner_image_revision

aio_ref=$(docker inspect --format '{{.Config.Image}}' "$aio_id" 2>/dev/null) || blocked 'AIO image reference is unavailable'
[ "$aio_ref" = "$official_aio_ref" ] || blocked 'AIO must use the official latest reference'
aio_image_id=$(docker inspect --format '{{.Image}}' "$aio_id" 2>/dev/null) || blocked 'AIO image ID is unavailable'
aio_image_hex=${aio_image_id#sha256:}
[ "$aio_image_hex" != "$aio_image_id" ] && [ "${#aio_image_hex}" -eq 64 ] || blocked 'AIO image ID is invalid'
case "$aio_image_hex" in
  *[!0-9a-f]*) blocked 'AIO image ID is invalid' ;;
esac
official_local_image_id=$(docker image inspect --format '{{.Id}}' "$official_aio_ref" 2>/dev/null) || blocked 'official latest AIO image is unavailable locally'
[ "$official_local_image_id" = "$aio_image_id" ] || blocked 'running AIO does not match the official latest local image ID'
aio_oom_before=$(docker inspect --format '{{.State.OOMKilled}}' "$aio_id" 2>/dev/null) || blocked 'AIO state is unavailable'
[ "$aio_oom_before" = false ] || blocked 'AIO was already OOM-killed'
aio_restart_before=$(docker inspect --format '{{.RestartCount}}' "$aio_id" 2>/dev/null) || blocked 'AIO restart count is unavailable'
is_non_negative_integer "$aio_restart_before" || blocked 'AIO restart count is invalid'
aio_host_contract=$(docker inspect --format '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.PidsLimit}} {{json .HostConfig.PortBindings}}' "$aio_id" 2>/dev/null) || blocked 'AIO host contract is unavailable'
case "$aio_host_contract" in
  '0 0 0 null'|'0 0 0 {}'|'0 0 <nil> null'|'0 0 <nil> {}') ;;
  *) blocked 'AIO must use official startup resources and no published host ports' ;;
esac

runner_contract=$(docker inspect --format '{{.HostConfig.Memory}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$runner_id" 2>/dev/null) || blocked 'Runner state is unavailable'
set -- $runner_contract
[ "$#" -eq 3 ] || blocked 'Runner state projection is invalid'
[ "$1" = "$runner_limit_bytes" ] || blocked 'Runner memory limit must be exactly 192 MiB'
[ "$2" = running ] && [ "$3" = healthy ] || blocked 'Runner must be running and healthy'
runner_oom_before=$(docker inspect --format '{{.State.OOMKilled}}' "$runner_id" 2>/dev/null) || blocked 'Runner OOM state is unavailable'
[ "$runner_oom_before" = false ] || blocked 'Runner was already OOM-killed'
runner_restart_before=$(docker inspect --format '{{.RestartCount}}' "$runner_id" 2>/dev/null) || blocked 'Runner restart count is unavailable'
is_non_negative_integer "$runner_restart_before" || blocked 'Runner restart count is invalid'

server_contract=$(docker inspect --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$server_id" 2>/dev/null) || blocked 'coze-server state is unavailable'
[ "$server_contract" = 'running healthy' ] || blocked 'coze-server must be running and healthy before the resource gate'
server_oom_before=$(docker inspect --format '{{.State.OOMKilled}}' "$server_id" 2>/dev/null) || blocked 'coze-server OOM state is unavailable'
[ "$server_oom_before" = false ] || blocked 'coze-server was already OOM-killed'
server_restart_before=$(docker inspect --format '{{.RestartCount}}' "$server_id" 2>/dev/null) || blocked 'coze-server restart count is unavailable'
is_non_negative_integer "$server_restart_before" || blocked 'coze-server restart count is invalid'

SANDBOX_AIO_CORE_E2E_IMAGE_ID=$aio_image_id
export SANDBOX_AIO_CORE_E2E_IMAGE_ID

ensure_fixed_test() {
  local regex=$1 test_name=$2 output count

  output=$(
    cd "$repo_root/backend"
    env -u GOMOD -u GOPATH -u GOROOT \
      GO111MODULE=on GOENV=off GOFLAGS= GOCACHE=/private/tmp/coze-go-build GOTOOLCHAIN=local GOWORK=off \
      go test ./internal/sandboxrunner -list "$regex" 2>/dev/null
  ) || fail "fixed backend test cannot be listed: $test_name"
  count=$(printf '%s\n' "$output" | grep -Fxc -- "$test_name" || true)
  [ "$count" -eq 1 ] || fail "fixed backend test is missing or ambiguous: $test_name"
}

scan_fixed_test_events() {
  local test_name=$1

  awk -v test_name="$test_name" '
    BEGIN {
      test_field = "\"Test\":\"" test_name "\""
      skip_field = "\"Action\":\"skip\""
      pass_field = "\"Action\":\"pass\""
    }
    index($0, test_field) && index($0, skip_field) { skipped=1 }
    index($0, test_field) && index($0, pass_field) { passed=1 }
    END { exit (skipped || !passed) }
  '
}

run_cleanup_only() {
  local go_status scan_status
  local -a pipeline_statuses

  set +e
  (
    cd "$repo_root/backend"
    env -u GOMOD -u GOPATH -u GOROOT \
      GO111MODULE=on GOENV=off GOFLAGS= GOCACHE=/private/tmp/coze-go-build GOTOOLCHAIN=local GOWORK=off \
      go test -json ./internal/sandboxrunner -run "$cleanup_test_regex" -count=1 -timeout=2m 2>/dev/null
  ) | scan_fixed_test_events TestAIOCoreE2ECleanupOnly
  pipeline_statuses=("${PIPESTATUS[@]}")
  go_status=${pipeline_statuses[0]}
  scan_status=${pipeline_statuses[1]}
  set -e
  [ "$go_status" -eq 0 ] && [ "$scan_status" -eq 0 ] ||
    fail 'cleanup-only exact fixture cursor rescan did not prove zero residual state'
}

resolved_backend_module=$(
  cd "$repo_root/backend"
  env -u GOMOD -u GOPATH -u GOROOT \
    GO111MODULE=on GOENV=off GOFLAGS= GOCACHE=/private/tmp/coze-go-build GOTOOLCHAIN=local GOWORK=off \
    go env GOMOD 2>/dev/null
) || fail 'backend Go module cannot be resolved with fixed execution controls'
[ "$resolved_backend_module" = "$repo_root/backend/go.mod" ] || fail 'backend Go module resolution escaped the repository module'

ensure_fixed_test "$workload_test_regex" TestAIOCoreE2ESoakWorkload
ensure_fixed_test "$snapshot_test_regex" TestAIOCoreE2ESoakSnapshot
ensure_fixed_test "$cleanup_test_regex" TestAIOCoreE2ECleanupOnly

umask 077
evidence_file=$(mktemp "$evidence_root/aio-2c4g-${duration}.XXXXXX.tsv") || blocked 'cannot create external evidence file'
{
  printf 'schema\tcoze.aio_2c4g_soak.v2\n'
  printf 'code_sha\t%s\n' "$code_sha"
  printf 'official_aio_ref\t%s\n' "$official_aio_ref"
  printf 'official_aio_image_id\t%s\n' "$aio_image_id"
  printf 'runner_image_id\t%s\n' "$runner_image_id"
  printf 'runner_image_revision\t%s\n' "$runner_image_revision"
  printf 'environment\tdev\n'
  printf 'deployment_isolation\t%s\n' "$isolated_deployment_declaration"
  printf 'host_cpus\t2\n'
  printf 'host_memory_bytes\t%s\n' "$expected_host_memory_bytes"
  printf 'host_memory_reserve_bytes\t%s\n' "$host_memory_reserve_bytes"
  printf 'max_aio_rss_bytes\t%s\n' "$max_aio_rss_bytes"
  printf 'max_runner_rss_bytes\t%s\n' "$runner_limit_bytes"
  printf 'max_aio_pids\t%s\n' "$max_aio_pids"
  printf 'max_runner_pids\t%s\n' "$max_runner_pids"
  printf 'max_aio_fds\t%s\n' "$max_aio_fds"
  printf 'max_runner_fds\t%s\n' "$max_runner_fds"
  printf 'duration\t%s\n' "$duration"
  printf 'failure_domain\tshared AIO; no per-session hostile CPU, PID, disk, network, credential, or filesystem isolation claim\n'
  printf 'epoch\tphase\thost_memory_available\taio_cpu\taio_rss\taio_pid\taio_fd\trunner_cpu\trunner_rss\trunner_pid\trunner_fd\tqueue\trunning\tweight\tsessions\tshells\tgeneration\n'
} >"$evidence_file"

container_scalar() {
  docker exec "$1" sh -c "$2" 2>/dev/null
}

container_rss() {
  container_scalar "$1" 'cat /sys/fs/cgroup/memory.current 2>/dev/null || cat /sys/fs/cgroup/memory/memory.usage_in_bytes'
}

container_pids() {
  container_scalar "$1" 'cat /sys/fs/cgroup/pids.current 2>/dev/null || { set -- /proc/[0-9]*; printf "%s\n" "$#"; }'
}

container_fds() {
  container_scalar "$1" 'n=0; for f in /proc/[0-9]*/fd/*; do [ ! -e "$f" ] || n=$((n+1)); done; printf "%s\n" "$n"'
}

host_memory_available() {
  container_scalar "$aio_id" 'awk '\''/^MemAvailable:/ { printf "%.0f\n", $2 * 1024; found=1 } END { if (!found) exit 1 }'\'' /proc/meminfo'
}

sample_cpu() {
  docker stats --no-stream --format '{{.CPUPerc}}' "$1" 2>/dev/null | sed 's/%$//'
}

decimal_lte() {
  awk -v value="$1" -v limit="$2" 'BEGIN { exit !(value <= limit) }' </dev/null
}

read_snapshot() {
  local output

  signed_status_read_allowed || return 1
  output=$(
    cd "$repo_root/backend"
    env -u GOMOD -u GOPATH -u GOROOT \
      GO111MODULE=on GOENV=off GOFLAGS= GOCACHE=/private/tmp/coze-go-build GOTOOLCHAIN=local GOWORK=off \
      go test -v ./internal/sandboxrunner -run "$snapshot_test_regex" -count=1 -timeout=2m 2>/dev/null
  ) || return 1
  extract_snapshot "$output"
}

all_aio_rss_samples=()
all_runner_rss_samples=()
all_aio_pid_samples=()
all_runner_pid_samples=()
all_aio_fd_samples=()
all_runner_fd_samples=()
all_host_available_samples=()
baseline_aio_rss_samples=()
baseline_runner_rss_samples=()
baseline_aio_pid_samples=()
baseline_runner_pid_samples=()
baseline_aio_fd_samples=()
baseline_runner_fd_samples=()
drain_aio_rss_samples=()
drain_runner_rss_samples=()
drain_aio_pid_samples=()
drain_runner_pid_samples=()
drain_aio_fd_samples=()
drain_runner_fd_samples=()

record_sample_from_snapshot() {
  local snapshot=$1 aio_cpu runner_cpu aio_rss runner_rss aio_pid runner_pid aio_fd runner_fd host_available
  local queue running weight sessions shells generation value

  aio_cpu=$(sample_cpu "$aio_id") || fail 'AIO CPU sample failed'
  runner_cpu=$(sample_cpu "$runner_id") || fail 'Runner CPU sample failed'
  aio_rss=$(container_rss "$aio_id") || fail 'AIO RSS sample failed'
  runner_rss=$(container_rss "$runner_id") || fail 'Runner RSS sample failed'
  aio_pid=$(container_pids "$aio_id") || fail 'AIO PID sample failed'
  runner_pid=$(container_pids "$runner_id") || fail 'Runner PID sample failed'
  aio_fd=$(container_fds "$aio_id") || fail 'AIO fd sample failed'
  runner_fd=$(container_fds "$runner_id") || fail 'Runner fd sample failed'
  host_available=$(host_memory_available) || fail 'host memory reserve sample failed'
  for value in "$aio_rss" "$runner_rss" "$aio_pid" "$runner_pid" "$aio_fd" "$runner_fd" "$host_available"; do
    is_non_negative_integer "$value" || fail 'container aggregate sample is invalid'
  done
  for value in "$aio_cpu" "$runner_cpu"; do
    [[ "$value" =~ ^[0-9]+([.][0-9]+)?$ ]] || fail 'container CPU aggregate sample is invalid'
    decimal_lte "$value" "$max_container_cpu_percent" || fail 'container CPU sample exceeded the 2 CPU host maximum'
  done
  resource_sample_within_limits "$aio_rss" "$runner_rss" "$aio_pid" "$runner_pid" "$aio_fd" "$runner_fd" "$host_available" ||
    fail 'resource sample exceeded an explicit maximum or the 1.5 GiB host reserve'
  queue=$(snapshot_value "$snapshot" queue)
  running=$(snapshot_value "$snapshot" running)
  weight=$(snapshot_value "$snapshot" weight)
  sessions=$(snapshot_value "$snapshot" sessions)
  shells=$(snapshot_value "$snapshot" shells)
  generation=$(snapshot_value "$snapshot" generation)
  if [ -z "$observed_generation" ]; then
    observed_generation=$generation
  elif [ "$generation" != "$observed_generation" ]; then
    fail 'runtime generation changed during the resource gate'
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$(date +%s)" "$sample_phase" "$host_available" "$aio_cpu" "$aio_rss" "$aio_pid" "$aio_fd" "$runner_cpu" "$runner_rss" "$runner_pid" "$runner_fd" \
    "$queue" "$running" "$weight" "$sessions" "$shells" "$generation" >>"$evidence_file"
  all_aio_rss_samples+=("$aio_rss")
  all_runner_rss_samples+=("$runner_rss")
  all_aio_pid_samples+=("$aio_pid")
  all_runner_pid_samples+=("$runner_pid")
  all_aio_fd_samples+=("$aio_fd")
  all_runner_fd_samples+=("$runner_fd")
  all_host_available_samples+=("$host_available")
  case "$sample_phase" in
    baseline)
      baseline_aio_rss_samples+=("$aio_rss")
      baseline_runner_rss_samples+=("$runner_rss")
      baseline_aio_pid_samples+=("$aio_pid")
      baseline_runner_pid_samples+=("$runner_pid")
      baseline_aio_fd_samples+=("$aio_fd")
      baseline_runner_fd_samples+=("$runner_fd")
      ;;
    drain*)
      drain_aio_rss_samples+=("$aio_rss")
      drain_runner_rss_samples+=("$runner_rss")
      drain_aio_pid_samples+=("$aio_pid")
      drain_runner_pid_samples+=("$runner_pid")
      drain_aio_fd_samples+=("$aio_fd")
      drain_runner_fd_samples+=("$runner_fd")
      ;;
  esac
}

record_sample() {
  local snapshot

  snapshot=$(read_snapshot) || fail 'fixed snapshot Go test returned an invalid aggregate snapshot'
  record_sample_from_snapshot "$snapshot"
}

sample_phase=baseline
for ((sample_index = 1; sample_index <= baseline_sample_count; sample_index++)); do
  baseline_snapshot=$(read_snapshot) || fail 'baseline signed snapshot failed or did not target the selected Runner deployment'
  snapshot_is_drained "$baseline_snapshot" ||
    fail 'isolated deployment was not drained before the workload baseline'
  record_sample_from_snapshot "$baseline_snapshot"
  if [ "$sample_index" -lt "$baseline_sample_count" ]; then
    sleep "$sample_interval_seconds"
  fi
done

workload_timeout_seconds=$((duration_seconds + 300))
sample_phase=workload
(
  cd "$repo_root/backend"
  export SANDBOX_AIO_CORE_E2E_SOAK_DURATION="$duration"
  exec env -u GOMOD -u GOPATH -u GOROOT \
    GO111MODULE=on GOENV=off GOFLAGS= GOCACHE=/private/tmp/coze-go-build GOTOOLCHAIN=local GOWORK=off \
    go test ./internal/sandboxrunner -run "$workload_test_regex" -count=1 -timeout="${workload_timeout_seconds}s"
) >/dev/null 2>&1 &
workload_pid=$!
deadline=$(( $(date +%s) + duration_seconds + 180 ))
next_sample_epoch=$(date +%s)
while kill -0 "$workload_pid" 2>/dev/null; do
  now=$(date +%s)
  [ "$now" -le "$deadline" ] || fail 'fixed workload Go test exceeded its bounded duration'
  if [ "$now" -ge "$next_sample_epoch" ]; then
    record_sample
    next_sample_epoch=$((next_sample_epoch + sample_interval_seconds))
    while [ "$next_sample_epoch" -le "$now" ]; do
      next_sample_epoch=$((next_sample_epoch + sample_interval_seconds))
    done
  fi
  now=$(date +%s)
  if [ "$next_sample_epoch" -gt "$now" ]; then
    sleep "$((next_sample_epoch - now))"
  fi
done
set +e
wait "$workload_pid"
workload_status=$?
set -e
workload_pid=''
if [ "$workload_status" -ne 0 ]; then
  residual_state='BLOCKED_EXACT_PREFIX_CLEANUP_REQUIRED'
  fail 'two normal Core workloads did not complete; exact-prefix residual cleanup remains BLOCKED'
fi

drain_deadline=$(( $(date +%s) + 60 ))
while :; do
  final_snapshot=$(read_snapshot) || fail 'final aggregate snapshot is invalid'
  if snapshot_is_drained "$final_snapshot"; then
    break
  fi
  [ "$(date +%s)" -lt "$drain_deadline" ] || fail 'queue, running, weight, Session, or Shell aggregates did not drain without residual probes'
  sleep "$sample_interval_seconds"
done

# This is the final signed status response. Cleanup-only performs exact DB/Redis
# cursor rescans without creating a Runner client; no later path may mint a
# signed status nonce or send another signed Runner request.
sample_phase=final_signed_snapshot
signed_requests_closed=true
record_sample_from_snapshot "$final_snapshot"
run_cleanup_only
residual_state='CLEAN_EXACT_PREFIX_CURSOR_RESCAN_ZERO'

sample_phase=drain_after_cleanup_no_signed_status
for ((sample_index = 1; sample_index <= drain_sample_count; sample_index++)); do
  record_sample_from_snapshot "$final_snapshot"
  if [ "$sample_index" -lt "$drain_sample_count" ]; then
    sleep "$sample_interval_seconds"
  fi
done

array_max() {
  local value maximum=-1
  for value in "$@"; do
    [ "$value" -gt "$maximum" ] && maximum=$value
  done
  [ "$maximum" -ge 0 ] || return 1
  printf '%s\n' "$maximum"
}

array_min() {
  local value minimum=
  for value in "$@"; do
    if [ -z "$minimum" ] || [ "$value" -lt "$minimum" ]; then
      minimum=$value
    fi
  done
  [ -n "$minimum" ] || return 1
  printf '%s\n' "$minimum"
}

baseline_aio_rss=$(array_max "${baseline_aio_rss_samples[@]}")
baseline_runner_rss=$(array_max "${baseline_runner_rss_samples[@]}")
baseline_aio_pid=$(array_max "${baseline_aio_pid_samples[@]}")
baseline_runner_pid=$(array_max "${baseline_runner_pid_samples[@]}")
baseline_aio_fd=$(array_max "${baseline_aio_fd_samples[@]}")
baseline_runner_fd=$(array_max "${baseline_runner_fd_samples[@]}")
drain_aio_rss=$(array_max "${drain_aio_rss_samples[@]}")
drain_runner_rss=$(array_max "${drain_runner_rss_samples[@]}")
drain_aio_pid=$(array_max "${drain_aio_pid_samples[@]}")
drain_runner_pid=$(array_max "${drain_runner_pid_samples[@]}")
drain_aio_fd=$(array_max "${drain_aio_fd_samples[@]}")
drain_runner_fd=$(array_max "${drain_runner_fd_samples[@]}")
drain_sample_within_baseline \
  "$baseline_aio_rss" "$drain_aio_rss" "$baseline_runner_rss" "$drain_runner_rss" \
  "$baseline_aio_pid" "$drain_aio_pid" "$baseline_runner_pid" "$drain_runner_pid" \
  "$baseline_aio_fd" "$drain_aio_fd" "$baseline_runner_fd" "$drain_runner_fd" ||
  fail 'post-cleanup drain window did not return within the explicit baseline deltas'

max_aio_rss_observed=$(array_max "${all_aio_rss_samples[@]}")
max_runner_rss_observed=$(array_max "${all_runner_rss_samples[@]}")
max_aio_pid_observed=$(array_max "${all_aio_pid_samples[@]}")
max_runner_pid_observed=$(array_max "${all_runner_pid_samples[@]}")
max_aio_fd_observed=$(array_max "${all_aio_fd_samples[@]}")
max_runner_fd_observed=$(array_max "${all_runner_fd_samples[@]}")
min_host_memory_available_observed=$(array_min "${all_host_available_samples[@]}")
{
  printf 'baseline_aio_rss_max\t%s\n' "$baseline_aio_rss"
  printf 'baseline_runner_rss_max\t%s\n' "$baseline_runner_rss"
  printf 'drain_aio_rss_max\t%s\n' "$drain_aio_rss"
  printf 'drain_runner_rss_max\t%s\n' "$drain_runner_rss"
  printf 'max_aio_rss_observed\t%s\n' "$max_aio_rss_observed"
  printf 'max_runner_rss_observed\t%s\n' "$max_runner_rss_observed"
  printf 'max_aio_pid_observed\t%s\n' "$max_aio_pid_observed"
  printf 'max_runner_pid_observed\t%s\n' "$max_runner_pid_observed"
  printf 'max_aio_fd_observed\t%s\n' "$max_aio_fd_observed"
  printf 'max_runner_fd_observed\t%s\n' "$max_runner_fd_observed"
  printf 'min_host_memory_available_observed\t%s\n' "$min_host_memory_available_observed"
  printf 'signed_status_closed_before_cleanup_only\ttrue\n'
} >>"$evidence_file"

aio_oom_after=$(docker inspect --format '{{.State.OOMKilled}}' "$aio_id" 2>/dev/null) || fail 'final AIO state is unavailable'
[ "$aio_oom_after" = false ] || fail 'AIO was OOM-killed during the resource gate'
runner_oom_after=$(docker inspect --format '{{.State.OOMKilled}}' "$runner_id" 2>/dev/null) || fail 'final Runner state is unavailable'
[ "$runner_oom_after" = false ] || fail 'Runner was OOM-killed during the resource gate'
aio_restart_after=$(docker inspect --format '{{.RestartCount}}' "$aio_id" 2>/dev/null) || fail 'final AIO restart count is unavailable'
[ "$aio_restart_after" = "$aio_restart_before" ] || fail 'AIO restarted during the resource gate'
runner_restart_after=$(docker inspect --format '{{.RestartCount}}' "$runner_id" 2>/dev/null) || fail 'final Runner restart count is unavailable'
[ "$runner_restart_after" = "$runner_restart_before" ] || fail 'Runner restarted during the resource gate'
runner_contract_after=$(docker inspect --format '{{.HostConfig.Memory}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$runner_id" 2>/dev/null) || fail 'final Runner state is unavailable'
[ "$runner_contract_after" = "$runner_limit_bytes running healthy" ] || fail 'Runner was not running and healthy after the resource gate'
server_oom_after=$(docker inspect --format '{{.State.OOMKilled}}' "$server_id" 2>/dev/null) || fail 'final coze-server OOM state is unavailable'
[ "$server_oom_after" = false ] || fail 'coze-server was OOM-killed during the resource gate'
server_restart_after=$(docker inspect --format '{{.RestartCount}}' "$server_id" 2>/dev/null) || fail 'final coze-server restart count is unavailable'
[ "$server_restart_after" = "$server_restart_before" ] || fail 'coze-server restarted during the resource gate'
server_contract_after=$(docker inspect --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$server_id" 2>/dev/null) || fail 'final coze-server state is unavailable'
[ "$server_contract_after" = 'running healthy' ] || fail 'coze-server was not running and healthy after the resource gate'
current_aio_image=$(docker inspect --format '{{.Image}}' "$aio_id" 2>/dev/null) || fail 'final AIO image ID is unavailable'
[ "$current_aio_image" = "$aio_image_id" ] || fail 'AIO instance changed during the resource gate'
aio_status_after=$(docker inspect --format '{{.State.Status}}' "$aio_id" 2>/dev/null) || fail 'final AIO state is unavailable'
[ "$aio_status_after" = running ] || fail 'AIO was not running after the resource gate'
validate_compose_target_binding || fail 'selected Compose target binding changed during the resource gate'
[ "$aio_id" = "$initial_aio_id" ] && [ "$runner_id" = "$initial_runner_id" ] && [ "$server_id" = "$initial_server_id" ] ||
  fail 'selected AIO, Runner, or coze-server container changed during the resource gate'
[ "$runner_image_id" = "$initial_runner_image_id" ] && [ "$runner_image_revision" = "$initial_runner_image_revision" ] ||
  fail 'selected Runner image or OCI revision changed during the resource gate'
validate_clean_worktree_snapshot
result=PASSED
printf 'aio 2c4g soak: PASS evidence=%s\n' "$evidence_file"
