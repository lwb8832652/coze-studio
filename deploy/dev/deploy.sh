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
#

set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
DEPLOY_ROOT_DIR=${DEPLOY_ROOT_DIR:-$SCRIPT_DIR}
DEPLOY_ENV_FILE=${DEPLOY_ENV_FILE:-$DEPLOY_ROOT_DIR/deploy.env}
DEPLOY_LOCK_FILE=${DEPLOY_LOCK_FILE:-$DEPLOY_ROOT_DIR/deploy.lock}
DEPLOYMENTS_DIR=${DEPLOYMENTS_DIR:-$DEPLOY_ROOT_DIR/deployments}
COMPOSE_FILE=${COMPOSE_FILE:-}

SERVER_REPOSITORY=${SERVER_REPOSITORY:-}
WEB_REPOSITORY=${WEB_REPOSITORY:-}
SERVER_IMAGE_REF=${SERVER_IMAGE_REF:-}
WEB_IMAGE_REF=${WEB_IMAGE_REF:-}
SANDBOX_RUNNER_REPOSITORY=${SANDBOX_RUNNER_REPOSITORY:-}
SANDBOX_RUNNER_IMAGE_REF=${SANDBOX_RUNNER_IMAGE_REF:-}
SANDBOX_RUNTIME_REPOSITORY=${SANDBOX_RUNTIME_REPOSITORY:-}
SANDBOX_RUNTIME_IMAGE_REF=${SANDBOX_RUNTIME_IMAGE_REF:-}
SANDBOX_RUNNER_EXECUTION_IMAGE=${SANDBOX_RUNNER_EXECUTION_IMAGE:-}
readonly OFFICIAL_SANDBOX_AIO_IMAGE_REF=ghcr.io/agent-infra/sandbox:latest
SANDBOX_AIO_IMAGE_REF=$OFFICIAL_SANDBOX_AIO_IMAGE_REF
SANDBOX_AIO_IMAGE_ID=
SANDBOX_AIO_STATUS=not-managed
SANDBOX_AIO_AVAILABLE=false
REQUIRED_ADDITIVE_MIGRATION_ID=20260813000100
readonly AIO_RAW_MAX_RESPONSE_BYTES=4096
DEPLOY_PROFILE=${DEPLOY_PROFILE:-local-data}

log() {
  printf '[deploy] %s\n' "$*"
}

error() {
  printf '[deploy] error: %s\n' "$*" >&2
}

is_revision() {
  [[ "$1" =~ ^[0-9a-fA-F]{40}$ ]]
}

normalize_revision() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

is_ipv4() {
  local address=${1:-}
  local octet
  local -a octets

  [[ "$address" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
  IFS=. read -r -a octets <<< "$address"
  [ "${#octets[@]}" -eq 4 ] || return 1
  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^(0|[1-9][0-9]{0,2})$ ]] || return 1
    ((10#$octet <= 255)) || return 1
  done
}

is_tcp_port() {
  local port=${1:-}

  [[ "$port" =~ ^[1-9][0-9]{0,4}$ ]] || return 1
  ((10#$port <= 65535))
}

web_health_base_url() {
  local host=${WEB_BIND_IP:-0.0.0.0}
  local port=${WEB_PORT:-8888}

  if [ "$host" = 0.0.0.0 ]; then
    host=127.0.0.1
  fi
  printf 'http://%s:%s' "$host" "$port"
}

docker_cmd() {
  docker "$@"
}

compose_cmd() {
  docker compose --env-file "$DEPLOY_ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

runner_profile_enabled() {
  [ "$DEPLOY_PROFILE" = runner-2c4g ]
}

lock_official_aio_image_ref() {
  if [ "${SANDBOX_AIO_IMAGE_REF:-}" != "$OFFICIAL_SANDBOX_AIO_IMAGE_REF" ]; then
    error 'official AIO image ref cannot be overridden'
    return 1
  fi
  SANDBOX_AIO_IMAGE_REF=$OFFICIAL_SANDBOX_AIO_IMAGE_REF
  readonly SANDBOX_AIO_IMAGE_REF
  export SANDBOX_AIO_IMAGE_REF
}

preflight_additive_migration() {
  runner_profile_enabled || return 0
  if ! SANDBOX_RUNNER_SESSION_ENABLED=false SANDBOX_RUNNER_IMAGE_TAG=dev \
    SANDBOX_RUNNER_EXECUTION_IMAGE="$SANDBOX_RUNNER_EXECUTION_IMAGE" \
    compose_cmd run --rm --no-deps coze-sandbox-runner migration-status >/dev/null 2>&1; then
    error 'required additive migration is not applied'
    return 1
  fi
  log "verified pre-applied additive migration $REQUIRED_ADDITIVE_MIGRATION_ID without database mutation"
}

validate_private_deploy_file() {
	local file=${1:-}
	local label=${2:-private deployment file}
	local expected_owner=${3:-$(id -u)}
	local mode owner

	[ -f "$file" ] || {
		error "$label is missing: $file"
		return 1
	}
	if ! mode=$(stat -c '%a' -- "$file" 2>/dev/null); then
		mode=$(stat -f '%Lp' -- "$file" 2>/dev/null) || {
			error "$label permissions cannot be verified: $file"
			return 1
		}
	fi
	if ! owner=$(stat -c '%u' -- "$file" 2>/dev/null); then
		owner=$(stat -f '%u' -- "$file" 2>/dev/null) || {
			error "$label owner cannot be verified: $file"
			return 1
		}
	fi
	[ "$mode" = 600 ] || {
		error "$label must have mode 0600: $file"
		return 1
	}
	[ "$owner" = "$expected_owner" ] || {
		error "$label must be owned by uid $expected_owner: $file"
		return 1
	}
}

service_health_status() {
  local service=$1
  local container_id status

  if ! container_id=$(compose_cmd ps -q "$service"); then
    return 1
  fi
  [ -n "$container_id" ] || return 1
  if ! status=$(docker_cmd inspect --format '{{.State.Health.Status}}' "$container_id"); then
    return 1
  fi
  [ -n "$status" ] || return 1
  printf '%s\n' "$status"
}

service_is_healthy() {
  local status

  if ! status=$(service_health_status "$1"); then
    return 1
  fi
  [ "$status" = healthy ]
}

container_image_id() {
  local service=$1
  local container_id

  if ! container_id=$(compose_cmd ps -q "$service"); then
    return 1
  fi
  if [ -z "$container_id" ]; then
    return 0
  fi
  docker_cmd inspect --format '{{.Image}}' "$container_id"
}

is_private_ipv4() {
  local address=${1:-}
  local -a octets

  is_ipv4 "$address" || return 1
  IFS=. read -r -a octets <<< "$address"
  ((10#${octets[0]} == 10)) ||
    ((10#${octets[0]} == 172 && 10#${octets[1]} >= 16 && 10#${octets[1]} <= 31)) ||
    ((10#${octets[0]} == 192 && 10#${octets[1]} == 168))
}

container_private_ipv4() {
  local service=$1
  local container_id addresses address selected=

  container_id=$(compose_cmd ps -q "$service") || return 1
  [ -n "$container_id" ] || return 1
  addresses=$(docker_cmd inspect --format '{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}' "$container_id") || return 1
  while IFS= read -r address; do
    [ -n "$address" ] || continue
    is_private_ipv4 "$address" || return 1
    [ -z "$selected" ] || return 1
    selected=$address
  done <<< "$addresses"
  [ -n "$selected" ] || return 1
  printf '%s\n' "$selected"
}

aio_raw_fetch_exact_200() {
  local url=$1
  local output_file=$2
  local status size

  status=$(curl --noproxy '*' --silent --show-error --max-time 5 \
    --max-redirs 0 --max-filesize "$AIO_RAW_MAX_RESPONSE_BYTES" \
    --output "$output_file" --write-out '%{http_code}' "$url" 2>/dev/null) || return 1
  [ "$status" = 200 ] || return 1
  size=$(wc -c < "$output_file" | tr -d '[:space:]') || return 1
  [[ "$size" =~ ^[0-9]+$ ]] || return 1
  ((size <= AIO_RAW_MAX_RESPONSE_BYTES))
}

aio_ping_body_matches() {
  local body_file=$1
  local size bytes

  size=$(wc -c < "$body_file" | tr -d '[:space:]') || return 1
  [ "$size" = 4 ] || return 1
  bytes=$(LC_ALL=C od -An -tx1 "$body_file" | tr -d '[:space:]') || return 1
  [ "$bytes" = 706f6e67 ]
}

aio_sandbox_body_matches() {
  local body_file=$1
  local compact

  compact=$(tr -d '[:space:]' < "$body_file") || return 1
  [[ "$compact" == \{*\} ]] || return 1
  [[ "$compact" =~ \"home_dir\":\"/[^\"\\]+\" ]] || return 1
  [[ "$compact" =~ \"version\":\"[^\"\\]+\" ]] || return 1
  [[ "$compact" =~ \"detail\":\{ ]] || return 1
  [[ "$compact" =~ \"system\":\{ ]] || return 1
  [[ "$compact" =~ \"runtime\":\{ ]] || return 1
  [[ "$compact" =~ \"utils\":\[ ]]
}

aio_raw_health_checks_pass() {
  local address base_url ping_body sandbox_body result=1

  address=$(container_private_ipv4 coze-sandbox-aio) || return 1
  base_url="http://$address:8080"
  ping_body=$(mktemp "${TMPDIR:-/tmp}/coze-aio-ping.XXXXXX") || return 1
  sandbox_body=$(mktemp "${TMPDIR:-/tmp}/coze-aio-sandbox.XXXXXX") || {
    rm -f -- "$ping_body"
    return 1
  }
  chmod 600 "$ping_body" "$sandbox_body" || {
    rm -f -- "$ping_body" "$sandbox_body"
    return 1
  }

  if aio_raw_fetch_exact_200 "$base_url/v1/ping" "$ping_body" &&
    aio_ping_body_matches "$ping_body" &&
    aio_raw_fetch_exact_200 "$base_url/v1/sandbox" "$sandbox_body" &&
    aio_sandbox_body_matches "$sandbox_body"; then
    result=0
  fi
  rm -f -- "$ping_body" "$sandbox_body"
  return "$result"
}

wait_for_aio_raw_health() {
  local timeout=${DEPLOY_HEALTH_TIMEOUT_SECONDS:-120}
  local deadline

  [[ "$timeout" =~ ^[1-9][0-9]*$ ]] || return 1
  deadline=$((SECONDS + timeout))
  while ((SECONDS < deadline)); do
    if aio_raw_health_checks_pass; then
      return 0
    fi
    sleep 2
  done
  return 1
}

image_revision() {
  local image_ref=$1
  docker_cmd image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$image_ref"
}

image_id() {
  local image_ref=$1
  docker_cmd image inspect --format '{{.Id}}' "$image_ref"
}

runtime_image_digest_ref() {
  local image_ref=$1
  local repository=$2
  local value

  value=$(docker_cmd image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$image_ref" | awk -v repository="$repository" '$0 ~ "^" repository "@sha256:[0-9a-f]{64}$" { print; exit }') || return 1
  [[ "$value" =~ ^[A-Za-z0-9._/:/-]+@sha256:[0-9a-f]{64}$ ]] || return 1
  printf '%s\n' "$value"
}

recorded_runtime_image_ref() {
  local record="$DEPLOYMENTS_DIR/current.env"
  [ -f "$record" ] || return 1
  awk -F= '$1 == "SANDBOX_RUNTIME_IMAGE_REF" { print substr($0, index($0, "=") + 1); exit }' "$record"
}

health_body_matches() {
  local body=$1
  local expected_revision=$2
  local compact

  compact=$(printf '%s' "$body" | tr -d '[:space:]')
  [[ "$compact" == *'"status":"ok"'* ]] || return 1
  if [ -n "$expected_revision" ]; then
    [[ "$compact" == *"\"revision\":\"$expected_revision\""* ]] || return 1
  fi
}

runner_core_projection_matches() {
  local body=$1
  local aio_available=${2:-false}
  local allow_rollback_legacy=${3:-false}
  local compact state= generation= memory_state=
  local state_present=false generation_present=false

  compact=$(printf '%s' "$body" | tr -d '[:space:]')
  [[ "$compact" == *'"schema":"coze.sandbox.runner_runtime_status.v1"'* ]] || return 1
  if [[ "$compact" =~ \"core_state\":\"([^\"]+)\" ]]; then
    state=${BASH_REMATCH[1]}
    state_present=true
  elif [[ "$compact" == *'"core_state":'* ]]; then
    return 1
  fi
  if [[ "$compact" =~ \"aio_runtime_generation\":([0-9]+) ]]; then
    generation=${BASH_REMATCH[1]}
    generation_present=true
  elif [[ "$compact" == *'"aio_runtime_generation":'* ]]; then
    return 1
  fi

  if [ "$state_present" = false ] && [ "$generation_present" = false ]; then
    [ "$allow_rollback_legacy" = true ] || return 1
    [[ "$compact" =~ \"applied_configuration_version\":([1-9][0-9]*) ]] || return 1
    [[ "$compact" =~ \"queued\":([0-9]+) ]] || return 1
    [[ "$compact" =~ \"running\":([0-9]+) ]] || return 1
    [[ "$compact" =~ \"memory_reserve_state\":\"([^\"]+)\" ]] || return 1
    memory_state=${BASH_REMATCH[1]}
    case "$memory_state" in
      available|below_watermark|unknown) return 0 ;;
      *) return 1 ;;
    esac
  fi
  [ "$state_present" = true ] && [ "$generation_present" = true ] || return 1

  if [ "$allow_rollback_legacy" = true ]; then
    [ "$state" = disabled ] && [ "$generation" = 0 ]
    return
  fi

  case "$state" in
    disabled) [ "$generation" = 0 ] ;;
    ready) [ "$aio_available" = true ] && [[ "$generation" =~ ^[1-9][0-9]*$ ]] ;;
    *) return 1 ;;
  esac
}

runner_core_projection_checks_pass() {
  local allow_rollback_legacy=${1:-false}
  local body

  body=$(compose_cmd exec -T coze-sandbox-runner sh -ceu \
    'wget --no-check-certificate --quiet --tries=1 --header="Authorization: Bearer ${SANDBOX_RUNNER_AUTH_TOKEN}" --output-document=- https://127.0.0.1:9443/v1/runtime-status') || return 1
  runner_core_projection_matches "$body" "$SANDBOX_AIO_AVAILABLE" "$allow_rollback_legacy"
}

health_checks_pass() {
  local expected_revision=${1:-}
  local allow_rollback_legacy=${2:-false}
  local backend_body web_body base_url

  service_is_healthy nsqd || return 1

  if ! backend_body=$(compose_cmd exec -T coze-server curl --fail --silent --show-error --max-time 5 http://127.0.0.1:8888/healthz 2>/dev/null); then
    return 1
  fi
  health_body_matches "$backend_body" "$expected_revision" || return 1

  if runner_profile_enabled; then
    service_is_healthy coze-sandbox-runner || return 1
    compose_cmd exec -T coze-sandbox-runner wget --no-check-certificate --quiet --tries=1 --spider https://127.0.0.1:9443/v1/health >/dev/null 2>&1 || return 1
    if [ "$SANDBOX_AIO_AVAILABLE" = true ]; then
      aio_raw_health_checks_pass || return 1
    fi
    runner_core_projection_checks_pass "$allow_rollback_legacy" || return 1
  fi

  base_url=$(web_health_base_url)
  if ! web_body=$(curl --fail --silent --show-error --max-time 5 "$base_url/healthz" 2>/dev/null); then
    return 1
  fi
  health_body_matches "$web_body" "$expected_revision" || return 1
  curl --fail --silent --show-error --max-time 5 --output /dev/null "$base_url/" 2>/dev/null
}

wait_for_health() {
  local expected_revision=${1:-}
  local allow_rollback_legacy=${2:-false}
  local timeout=${DEPLOY_HEALTH_TIMEOUT_SECONDS:-120}
  local deadline

  [[ "$timeout" =~ ^[1-9][0-9]*$ ]] || {
    error 'DEPLOY_HEALTH_TIMEOUT_SECONDS must be a positive integer'
    return 1
  }
  deadline=$((SECONDS + timeout))

  while ((SECONDS < deadline)); do
    if health_checks_pass "$expected_revision" "$allow_rollback_legacy"; then
      return 0
    fi
    sleep 2
  done

  error "health checks did not pass within ${timeout}s"
  return 1
}

record_value_is_safe() {
  case "$1" in
    *$'\n'*|*$'\r'*) return 1 ;;
    *) return 0 ;;
  esac
}

failure_record_value() {
  if record_value_is_safe "$1"; then
    printf '%s' "$1"
  else
    printf '<unsafe-multiline-value>'
  fi
}

record_success() {
  local revision=$1
  local server_ref=$2
  local web_ref=$3
  local server_id=$4
  local web_id=$5
  local runner_id=${6:-}
  local runtime_ref=${7:-}
  local deployed_at tmp value

  for value in "$revision" "$server_ref" "$web_ref" "$server_id" "$web_id"; do
    record_value_is_safe "$value" || {
      error 'refusing to write a deployment record containing a newline'
      return 1
    }
  done

  mkdir -p -- "$DEPLOYMENTS_DIR"
  tmp=$(mktemp "$DEPLOYMENTS_DIR/.current.env.tmp.XXXXXX")
  deployed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  if ! {
    printf 'REVISION=%s\n' "$revision"
    printf 'SERVER_IMAGE_REF=%s\n' "$server_ref"
    printf 'WEB_IMAGE_REF=%s\n' "$web_ref"
    printf 'SERVER_IMAGE_ID=%s\n' "$server_id"
    printf 'WEB_IMAGE_ID=%s\n' "$web_id"
    if runner_profile_enabled; then
      printf 'SANDBOX_RUNNER_IMAGE_REF=%s\n' "$SANDBOX_RUNNER_IMAGE_REF"
      printf 'SANDBOX_RUNNER_IMAGE_ID=%s\n' "$runner_id"
      printf 'SANDBOX_RUNTIME_IMAGE_REF=%s\n' "$runtime_ref"
      printf 'SANDBOX_AIO_IMAGE_REF=%s\n' "$SANDBOX_AIO_IMAGE_REF"
      printf 'SANDBOX_AIO_IMAGE_ID=%s\n' "$SANDBOX_AIO_IMAGE_ID"
      printf 'SANDBOX_AIO_STATUS=%s\n' "$SANDBOX_AIO_STATUS"
    fi
    printf 'DEPLOYED_AT_UTC=%s\n' "$deployed_at"
  } > "$tmp"; then
    rm -f -- "$tmp"
    return 1
  fi
  if ! chmod 600 "$tmp" || ! mv -f -- "$tmp" "$DEPLOYMENTS_DIR/current.env"; then
    rm -f -- "$tmp"
    return 1
  fi
}

prepare_official_aio() {
  SANDBOX_AIO_IMAGE_ID=
  SANDBOX_AIO_STATUS=unavailable
  SANDBOX_AIO_AVAILABLE=false

  log "pulling official AIO image $SANDBOX_AIO_IMAGE_REF"
  docker_cmd pull "$SANDBOX_AIO_IMAGE_REF" || return 1
  SANDBOX_AIO_IMAGE_ID=$(image_id "$SANDBOX_AIO_IMAGE_REF") || return 1
  compose_cmd up -d --no-build coze-sandbox-aio || return 1
  wait_for_aio_raw_health || return 1

  SANDBOX_AIO_STATUS=healthy
  SANDBOX_AIO_AVAILABLE=true
}

record_failure() {
  local transaction_id=$1
  local candidate_server_revision=$2
  local candidate_web_revision=$3
  local candidate_server_id=$4
  local candidate_web_id=$5
  local old_server_id=$6
  local old_web_id=$7
  local old_server_revision=$8
  local old_web_revision=$9
  local rollback_result=${10}
  local failure_reason=${11}
  local failed_at record tmp value

  for value in "$transaction_id" "$rollback_result" "$failure_reason"; do
    record_value_is_safe "$value" || return 1
  done
  candidate_server_revision=$(failure_record_value "$candidate_server_revision")
  candidate_web_revision=$(failure_record_value "$candidate_web_revision")
  candidate_server_id=$(failure_record_value "$candidate_server_id")
  candidate_web_id=$(failure_record_value "$candidate_web_id")
  old_server_id=$(failure_record_value "$old_server_id")
  old_web_id=$(failure_record_value "$old_web_id")
  old_server_revision=$(failure_record_value "$old_server_revision")
  old_web_revision=$(failure_record_value "$old_web_revision")

  mkdir -p -- "$DEPLOYMENTS_DIR"
  record="$DEPLOYMENTS_DIR/failed-$transaction_id.env"
  tmp=$(mktemp "$DEPLOYMENTS_DIR/.failed.env.tmp.XXXXXX")
  failed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  if ! {
    printf 'CANDIDATE_SERVER_REVISION=%s\n' "$candidate_server_revision"
    printf 'CANDIDATE_WEB_REVISION=%s\n' "$candidate_web_revision"
    printf 'CANDIDATE_SERVER_IMAGE_ID=%s\n' "$candidate_server_id"
    printf 'CANDIDATE_WEB_IMAGE_ID=%s\n' "$candidate_web_id"
    printf 'OLD_SERVER_IMAGE_ID=%s\n' "$old_server_id"
    printf 'OLD_WEB_IMAGE_ID=%s\n' "$old_web_id"
    printf 'OLD_SERVER_REVISION=%s\n' "$old_server_revision"
    printf 'OLD_WEB_REVISION=%s\n' "$old_web_revision"
    printf 'ROLLBACK_RESULT=%s\n' "$rollback_result"
    printf 'FAILURE_REASON=%s\n' "$failure_reason"
    printf 'FAILED_AT_UTC=%s\n' "$failed_at"
  } > "$tmp"; then
    rm -f -- "$tmp"
    return 1
  fi
  if ! chmod 600 "$tmp" || ! mv -f -- "$tmp" "$record"; then
    rm -f -- "$tmp"
    return 1
  fi
}

record_pre_update_failure() {
  local reason=$1
  local transaction_id=$2
  local candidate_server_revision=$3
  local candidate_web_revision=$4
  local candidate_server_id=$5
  local candidate_web_id=$6
  local old_server_id=$7
  local old_web_id=$8
  local old_server_revision=$9
  local old_web_revision=${10}

  error "$reason"
  record_failure "$transaction_id" "$candidate_server_revision" "$candidate_web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision" 'not-attempted' "$reason" ||
    error 'failed to write the pre-update failure record'
  return 1
}

rollback_images() {
  local old_server_id=$1
  local old_web_id=$2
  local old_server_revision=$3
  local old_web_revision=$4
  local transaction_id=$5
  local old_runner_id=${6:-}
  local old_runtime_ref=${7:-}
  local rollback_tag rollback_revision=
  local restored_server_id restored_web_id restored_runner_id

  if runner_profile_enabled; then
    if ! SANDBOX_RUNNER_SESSION_ENABLED=false compose_cmd stop coze-sandbox-runner; then
      error 'failed to disable Core capability before rollback'
      return 1
    fi
    log 'Core capability disabled before application rollback'
    log 'official latest AIO cannot be rolled back exactly; Core remains disabled and persistent AIO state is preserved'
  fi

  if [ -z "$old_server_id" ] || [ -z "$old_web_id" ]; then
    error 'first deployment failed; rollback unavailable'
    return 1
  fi

  rollback_tag="rollback-$transaction_id"
  if ! docker_cmd tag "$old_server_id" "$SERVER_REPOSITORY:$rollback_tag"; then
    error 'failed to tag the previous server image'
    return 1
  fi
  if ! docker_cmd tag "$old_web_id" "$WEB_REPOSITORY:$rollback_tag"; then
    error 'failed to tag the previous web image'
    return 1
  fi
  if runner_profile_enabled; then
    if [ -z "$old_runner_id" ] || ! docker_cmd tag "$old_runner_id" "$SANDBOX_RUNNER_REPOSITORY:$rollback_tag"; then
      error 'failed to tag the previous sandbox runner image'
      return 1
    fi
  fi

  if runner_profile_enabled; then
    if [ -z "$old_runtime_ref" ] || ! SANDBOX_RUNNER_SESSION_ENABLED=false SERVER_IMAGE_TAG="$rollback_tag" WEB_IMAGE_TAG="$rollback_tag" SANDBOX_RUNNER_IMAGE_TAG="$rollback_tag" SANDBOX_RUNNER_EXECUTION_IMAGE="$old_runtime_ref" compose_cmd up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web; then
      error 'rollback compose update failed'
      return 1
    fi
  elif ! SERVER_IMAGE_TAG="$rollback_tag" WEB_IMAGE_TAG="$rollback_tag" compose_cmd up -d --no-build --remove-orphans coze-server coze-web; then
    error 'rollback compose update failed'
    return 1
  fi
  if ! restored_server_id=$(container_image_id coze-server) ||
    ! restored_web_id=$(container_image_id coze-web); then
    error 'cannot read container image IDs after rollback'
    return 1
  fi
  if [ "$restored_server_id" != "$old_server_id" ] ||
    [ "$restored_web_id" != "$old_web_id" ]; then
    error 'rollback container image IDs do not match the saved images'
    return 1
  fi
  if runner_profile_enabled; then
    if ! restored_runner_id=$(container_image_id coze-sandbox-runner) || [ "$restored_runner_id" != "$old_runner_id" ]; then
      error 'rollback sandbox runner image ID does not match the saved image'
      return 1
    fi
  fi

  if is_revision "$old_server_revision" &&
    [ "$(normalize_revision "$old_server_revision")" = "$(normalize_revision "$old_web_revision")" ]; then
    rollback_revision=$(normalize_revision "$old_server_revision")
  else
    log 'previous revision metadata unavailable; rollback health will omit revision matching'
  fi

  if wait_for_health "$rollback_revision" true; then
    log 'rollback succeeded'
    return 0
  fi

  error 'rollback health checks failed'
  return 1
}

deploy_transaction() {
  local requested_revision=${1:-}
  local old_server_id= old_web_id= old_runner_id= old_server_revision= old_web_revision= old_runner_revision= old_runtime_ref=
  local server_revision= web_revision= runner_revision= runtime_revision= candidate_revision=
  local candidate_server_id= candidate_web_id= candidate_runner_id=
  local running_server_id= running_web_id= running_runner_id=
  local transaction_id rollback_result failure_reason runner_session_enabled

  transaction_id="$(date -u '+%Y%m%dT%H%M%SZ')-$$"
  if ! old_server_id=$(container_image_id coze-server); then
    record_pre_update_failure 'cannot read the current server container image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if ! old_web_id=$(container_image_id coze-web); then
    record_pre_update_failure 'cannot read the current web container image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if [ -n "$old_server_id" ]; then
    old_server_revision=$(image_revision "$old_server_id" 2>/dev/null || true)
  fi
  if [ -n "$old_web_id" ]; then
    old_web_revision=$(image_revision "$old_web_id" 2>/dev/null || true)
  fi
  if runner_profile_enabled; then
    if ! old_runner_id=$(container_image_id coze-sandbox-runner); then
      record_pre_update_failure 'cannot read the current sandbox runner container image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
      return
    fi
    if [ -n "$old_runner_id" ]; then
      old_runner_revision=$(image_revision "$old_runner_id" 2>/dev/null || true)
    fi
    old_runtime_ref=$(recorded_runtime_image_ref 2>/dev/null || true)
    if [ -n "$old_runner_id" ] && [ -z "$old_runtime_ref" ]; then
      record_pre_update_failure 'current sandbox runtime digest is unavailable for rollback' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
      return
    fi
  fi

  log 'pulling candidate server and web images'
  if ! docker_cmd pull "$SERVER_IMAGE_REF"; then
    record_pre_update_failure 'failed to pull the candidate server image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if ! docker_cmd pull "$WEB_IMAGE_REF"; then
    record_pre_update_failure 'failed to pull the candidate web image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! docker_cmd pull "$SANDBOX_RUNNER_IMAGE_REF"; then
    record_pre_update_failure 'failed to pull the candidate sandbox runner image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! docker_cmd pull "$SANDBOX_RUNTIME_IMAGE_REF"; then
    record_pre_update_failure 'failed to pull the candidate sandbox runtime image' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi

  if ! candidate_server_id=$(image_id "$SERVER_IMAGE_REF"); then
    record_pre_update_failure 'cannot read the candidate server image ID' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if ! candidate_web_id=$(image_id "$WEB_IMAGE_REF"); then
    record_pre_update_failure 'cannot read the candidate web image ID' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! candidate_runner_id=$(image_id "$SANDBOX_RUNNER_IMAGE_REF"); then
    record_pre_update_failure 'cannot read the candidate sandbox runner image ID' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! SANDBOX_RUNNER_EXECUTION_IMAGE=$(runtime_image_digest_ref "$SANDBOX_RUNTIME_IMAGE_REF" "$SANDBOX_RUNTIME_REPOSITORY"); then
    record_pre_update_failure 'candidate sandbox runtime image has no immutable digest' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi

  if ! server_revision=$(image_revision "$SERVER_IMAGE_REF"); then
    record_pre_update_failure 'candidate server image has no readable revision' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if ! web_revision=$(image_revision "$WEB_IMAGE_REF"); then
    record_pre_update_failure 'candidate web image has no readable revision' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! runner_revision=$(image_revision "$SANDBOX_RUNNER_IMAGE_REF"); then
    record_pre_update_failure 'candidate sandbox runner image has no readable revision' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! runtime_revision=$(image_revision "$SANDBOX_RUNTIME_IMAGE_REF"); then
    record_pre_update_failure 'candidate sandbox runtime image has no readable revision' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if ! is_revision "$server_revision" || ! is_revision "$web_revision" || { runner_profile_enabled && { ! is_revision "$runner_revision" || ! is_revision "$runtime_revision"; }; }; then
    record_pre_update_failure 'candidate image revisions must both be full 40-hex SHA values' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  server_revision=$(normalize_revision "$server_revision")
  web_revision=$(normalize_revision "$web_revision")
  if runner_profile_enabled; then
    runner_revision=$(normalize_revision "$runner_revision")
    runtime_revision=$(normalize_revision "$runtime_revision")
  fi
  if [ "$server_revision" != "$web_revision" ] || { runner_profile_enabled && { [ "$server_revision" != "$runner_revision" ] || [ "$server_revision" != "$runtime_revision" ]; }; }; then
    record_pre_update_failure 'candidate server and web revisions do not match' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  candidate_revision=$server_revision
  if [ -n "$requested_revision" ] &&
    [ "$candidate_revision" != "$(normalize_revision "$requested_revision")" ]; then
    record_pre_update_failure 'candidate revision does not match the requested SHA' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi
  if runner_profile_enabled && ! preflight_additive_migration; then
    record_pre_update_failure 'required additive migration is not applied' "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision"
    return
  fi

  local candidate_started=false
  if runner_profile_enabled; then
    runner_session_enabled=${SANDBOX_RUNNER_SESSION_ENABLED:-false}
    case "$runner_session_enabled" in
      true|false) ;;
      *)
        runner_session_enabled=false
        log 'invalid Runner Core gate ignored; Core remains disabled'
        ;;
    esac
    if ! prepare_official_aio; then
      runner_session_enabled=false
      log 'official AIO is unavailable; Core remains disabled while server/web and one-shot deployment continue'
    fi
    if SANDBOX_RUNNER_SESSION_ENABLED="$runner_session_enabled" SERVER_IMAGE_TAG=dev WEB_IMAGE_TAG=dev SANDBOX_RUNNER_IMAGE_TAG=dev SANDBOX_RUNNER_EXECUTION_IMAGE="$SANDBOX_RUNNER_EXECUTION_IMAGE" compose_cmd up -d --no-build --remove-orphans coze-server coze-sandbox-runner coze-web && wait_for_health "$candidate_revision"; then
      candidate_started=true
    fi
  else
    if SERVER_IMAGE_TAG=dev WEB_IMAGE_TAG=dev compose_cmd up -d --no-build --remove-orphans coze-server coze-web && wait_for_health "$candidate_revision"; then
      candidate_started=true
    fi
  fi
  if [ "$candidate_started" = true ]; then
    if ! running_server_id=$(container_image_id coze-server) ||
      ! running_web_id=$(container_image_id coze-web); then
      failure_reason='cannot read container image IDs after candidate update'
    elif [ "$running_server_id" != "$candidate_server_id" ] ||
      [ "$running_web_id" != "$candidate_web_id" ]; then
      failure_reason='candidate container image IDs do not match the pulled images'
    elif runner_profile_enabled && { ! running_runner_id=$(container_image_id coze-sandbox-runner) || [ "$running_runner_id" != "$candidate_runner_id" ]; }; then
      failure_reason='candidate sandbox runner image ID does not match the pulled image'
    elif record_success "$candidate_revision" "$SERVER_IMAGE_REF" "$WEB_IMAGE_REF" "$candidate_server_id" "$candidate_web_id" "$candidate_runner_id" "$SANDBOX_RUNNER_EXECUTION_IMAGE"; then
      log "deployment succeeded for revision $candidate_revision"
      return 0
    else
      failure_reason='healthy candidate success record could not be written'
    fi
  else
    failure_reason='candidate update or health check failed'
  fi

  error "deployment failed for revision $candidate_revision: $failure_reason; starting rollback"
  rollback_result=failed
  if rollback_images "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision" "$transaction_id" "$old_runner_id" "$old_runtime_ref"; then
    rollback_result=succeeded
  fi
  record_failure "$transaction_id" "$server_revision" "$web_revision" "$candidate_server_id" "$candidate_web_id" "$old_server_id" "$old_web_id" "$old_server_revision" "$old_web_revision" "$rollback_result" "$failure_reason" ||
    error 'failed to write the deployment failure record'
  return 1
}

validate_component() {
  [[ "$1" =~ ^[A-Za-z0-9._:/-]+$ ]] && [[ "$1" != *'..'* ]]
}

main() {
  local requested_revision=

  umask 077
  if [ "$#" -gt 1 ] || { [ "$#" -eq 1 ] && ! is_revision "$1"; }; then
    error 'expected zero arguments or one full 40-hex SHA'
    return 1
  fi
  if [ "$#" -eq 1 ]; then
    requested_revision=$(normalize_revision "$1")
  fi

  mkdir -p -- "$DEPLOY_ROOT_DIR"
  cd -- "$DEPLOY_ROOT_DIR"
  command -v flock >/dev/null 2>&1 || {
    error 'flock is required'
    return 1
  }
  exec 9>"$DEPLOY_LOCK_FILE"
  if ! flock -n 9; then
    error 'deployment already in progress'
    return 1
  fi

  if [ ! -f "$DEPLOY_ENV_FILE" ]; then
    error "deployment environment file is missing: $DEPLOY_ENV_FILE"
    return 1
  fi
  set -a
  # shellcheck disable=SC1090
  source "$DEPLOY_ENV_FILE"
  set +a
  lock_official_aio_image_ref || return 1

  case "$DEPLOY_PROFILE" in
    local-data)
      COMPOSE_FILE=${COMPOSE_FILE:-$DEPLOY_ROOT_DIR/docker-compose.yml}
      ;;
    runner-2c4g)
      COMPOSE_FILE=${COMPOSE_FILE:-$DEPLOY_ROOT_DIR/docker-compose.runner-2c4g.yml}
		SANDBOX_RUNNER_ENV_FILE=${SANDBOX_RUNNER_ENV_FILE:-$DEPLOY_ROOT_DIR/sandbox-runner.env}
		SANDBOX_RUNNER_TLS_CERT_FILE=${SANDBOX_RUNNER_TLS_CERT_FILE:-$DEPLOY_ROOT_DIR/secrets/sandbox-runner.crt}
		SANDBOX_RUNNER_TLS_KEY_FILE=${SANDBOX_RUNNER_TLS_KEY_FILE:-$DEPLOY_ROOT_DIR/secrets/sandbox-runner.key}
      if [ -z "${SANDBOX_RUNNER_ROOTLESS_SOCKET:-}" ] ||
        [ -z "${SANDBOX_RUNNER_ROOTLESS_SOCKET_GID:-}" ] ||
        ! [[ "$SANDBOX_RUNNER_ROOTLESS_SOCKET_GID" =~ ^[0-9]+$ ]] ||
        [ ! -S "$SANDBOX_RUNNER_ROOTLESS_SOCKET" ]; then
        error 'runner-2c4g requires an existing dedicated rootless runtime socket and numeric group ID'
        return 1
      fi
		validate_private_deploy_file "$SANDBOX_RUNNER_ENV_FILE" 'sandbox runner environment file' "$(id -u)" || return 1
		validate_private_deploy_file "$SANDBOX_RUNNER_TLS_CERT_FILE" 'sandbox runner TLS certificate' 10001 || return 1
		validate_private_deploy_file "$SANDBOX_RUNNER_TLS_KEY_FILE" 'sandbox runner TLS key' 10001 || return 1
		export SANDBOX_RUNNER_ENV_FILE SANDBOX_RUNNER_TLS_CERT_FILE SANDBOX_RUNNER_TLS_KEY_FILE
      ;;
    *)
      error 'DEPLOY_PROFILE must be local-data or runner-2c4g'
      return 1
      ;;
  esac

  WEB_BIND_IP=${WEB_BIND_IP:-0.0.0.0}
  WEB_PORT=${WEB_PORT:-8888}
  if ! is_ipv4 "$WEB_BIND_IP"; then
    error 'WEB_BIND_IP must be a valid IPv4 address'
    return 1
  fi
  if ! is_tcp_port "$WEB_PORT"; then
    error 'WEB_PORT must be an integer from 1 to 65535'
    return 1
  fi
  export WEB_BIND_IP WEB_PORT

  if [ -z "${ACR_REGISTRY:-}" ] || [ -z "${ACR_NAMESPACE:-}" ]; then
    error 'ACR_REGISTRY and ACR_NAMESPACE are required'
    return 1
  fi
  if ! validate_component "$ACR_REGISTRY" || ! validate_component "$ACR_NAMESPACE"; then
    error 'ACR registry or namespace contains unsupported characters'
    return 1
  fi
  if ! [[ "${DEPLOY_HEALTH_TIMEOUT_SECONDS:-120}" =~ ^[1-9][0-9]*$ ]]; then
    error 'DEPLOY_HEALTH_TIMEOUT_SECONDS must be a positive integer'
    return 1
  fi
  DEPLOY_HEALTH_TIMEOUT_SECONDS=${DEPLOY_HEALTH_TIMEOUT_SECONDS:-120}

  SERVER_REPOSITORY="$ACR_REGISTRY/$ACR_NAMESPACE/coze-server"
  WEB_REPOSITORY="$ACR_REGISTRY/$ACR_NAMESPACE/coze-web"
  SANDBOX_RUNNER_REPOSITORY="$ACR_REGISTRY/$ACR_NAMESPACE/coze-sandbox-runner"
  SANDBOX_RUNTIME_REPOSITORY="$ACR_REGISTRY/$ACR_NAMESPACE/coze-sandbox-runtime"
  SERVER_IMAGE_REF="$SERVER_REPOSITORY:dev"
  WEB_IMAGE_REF="$WEB_REPOSITORY:dev"
  SANDBOX_RUNNER_IMAGE_REF="$SANDBOX_RUNNER_REPOSITORY:dev"
  SANDBOX_RUNTIME_IMAGE_REF="$SANDBOX_RUNTIME_REPOSITORY:dev"

  command -v docker >/dev/null 2>&1 || {
    error 'docker is required'
    return 1
  }
  command -v curl >/dev/null 2>&1 || {
    error 'curl is required'
    return 1
  }
  [ -f "$COMPOSE_FILE" ] || {
    error "compose file is missing: $COMPOSE_FILE"
    return 1
  }
  docker compose version >/dev/null 2>&1 || {
    error 'Docker Compose v2 is required'
    return 1
  }

  deploy_transaction "$requested_revision"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
